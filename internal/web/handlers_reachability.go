package web

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// ReachabilityHandler serves the reachability history main page
func (s *Server) ReachabilityHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters for filtering
	query := r.URL.Query()

	statusFilter := query.Get("status")
	protocolFilter := query.Get("protocol")

	// Parse period filter (default to 1 day for nodes, 90 days for trends)
	trendsPeriodFilter := 90 // For trends chart (3 months)
	nodesPeriodFilter := 1   // Default to 1 day for recently tested nodes
	if p := query.Get("trends_period"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 && parsed <= 365 {
			trendsPeriodFilter = parsed
		}
	}
	if p := query.Get("nodes_period"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 && parsed <= 365 {
			nodesPeriodFilter = parsed
		}
	}

	// Parse limit filter (default to 25 if not specified)
	limitFilter := 25
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limitFilter = parsed
		}
	}

	// Get overall trends
	trends, err := s.storage.GetReachabilityTrends(trendsPeriodFilter)
	if err != nil {
		log.Printf("Error getting reachability trends: %v", err)
		trends = []storage.ReachabilityTrend{}
	}

	data := map[string]interface{}{
		"Title":               "Reachability History",
		"Version":             version.GetVersionInfo(),
		"ActivePage":          "reachability",
		"Trends":              trends,
		"StatusFilter":        statusFilter,
		"TrendsPeriodFilter":  trendsPeriodFilter,
		"NodesPeriodFilter":   nodesPeriodFilter,
		"ProtocolFilter":      protocolFilter,
		"LimitFilter":         limitFilter,
	}

	// Always show data by default - get recently tested nodes for the last day
	// If filters are applied, get filtered results
	if statusFilter != "" || protocolFilter != "" || query.Get("nodes_period") != "" || query.Get("limit") != "" {
		filteredNodes, err := s.getFilteredReachabilityNodes(statusFilter, protocolFilter, nodesPeriodFilter, limitFilter)
		if err != nil {
			log.Printf("Error getting filtered nodes: %v", err)
			filteredNodes = []storage.NodeTestResult{}
		}
		data["FilteredNodes"] = filteredNodes
	} else {
		// Default behavior - get recently tested nodes (both operational and failed) for the last day
		operational, err := s.storage.SearchNodesByReachability(true, 10, nodesPeriodFilter)
		if err != nil {
			log.Printf("Error getting operational nodes: %v", err)
			operational = []storage.NodeTestResult{}
		}

		failed, err := s.storage.SearchNodesByReachability(false, 10, nodesPeriodFilter)
		if err != nil {
			log.Printf("Error getting failed nodes: %v", err)
			failed = []storage.NodeTestResult{}
		}

		data["OperationalNodes"] = operational
		data["FailedNodes"] = failed
	}

	if err := s.templates["reachability"].Execute(w, data); err != nil {
		log.Printf("Error executing reachability template: %v", err)
	}
}

// getFilteredReachabilityNodes retrieves nodes based on the applied filters
func (s *Server) getFilteredReachabilityNodes(statusFilter, protocolFilter string, periodFilter, limitFilter int) ([]storage.NodeTestResult, error) {
	// For now, use the existing SearchNodesByReachability method and apply additional filtering
	// This could be optimized by adding dedicated database queries for these filters

	var allNodes []storage.NodeTestResult

	switch statusFilter {
	case "operational":
		nodes, err := s.storage.SearchNodesByReachability(true, limitFilter*2, periodFilter) // Get more than needed for protocol filtering
		if err != nil {
			return nil, err
		}
		allNodes = nodes
	case "failed":
		nodes, err := s.storage.SearchNodesByReachability(false, limitFilter*2, periodFilter)
		if err != nil {
			return nil, err
		}
		allNodes = nodes
	default: // "all" or empty
		// Get both operational and failed nodes - fetch more to ensure we get both types
		// When status=all, we want to show a mix of both operational and failed
		// Fetch more than needed to account for protocol filtering
		fetchLimit := limitFilter * 2
		operational, err := s.storage.SearchNodesByReachability(true, fetchLimit, periodFilter)
		if err != nil {
			return nil, err
		}
		failed, err := s.storage.SearchNodesByReachability(false, fetchLimit, periodFilter)
		if err != nil {
			return nil, err
		}
		allNodes = append(operational, failed...)

		// Sort by test time (most recent first)
		sort.Slice(allNodes, func(i, j int) bool {
			return allNodes[i].TestTime.After(allNodes[j].TestTime)
		})
	}

	// Apply protocol filtering
	var filteredNodes []storage.NodeTestResult
	for _, node := range allNodes {
		switch protocolFilter {
		case "binkp":
			if node.BinkPSuccess {
				filteredNodes = append(filteredNodes, node)
			}
		case "ifcico":
			if node.IfcicoSuccess {
				filteredNodes = append(filteredNodes, node)
			}
		case "telnet":
			if node.TelnetSuccess {
				filteredNodes = append(filteredNodes, node)
			}
		default: // "any" or empty
			filteredNodes = append(filteredNodes, node)
		}

		// Limit results
		if len(filteredNodes) >= limitFilter {
			break
		}
	}

	return filteredNodes, nil
}

// ReachabilityNodeHandler serves the reachability history for a specific node
func (s *Server) ReachabilityNodeHandler(w http.ResponseWriter, r *http.Request) {
	// Parse node address from form or URL
	var zone, net, node int
	var err error

	if r.Method == "POST" {
		// Parse form data
		address := r.FormValue("address")
		if address == "" {
			http.Error(w, "Node address is required", http.StatusBadRequest)
			return
		}

		// Parse address (format: zone:net/node)
		parts := strings.Split(address, ":")
		if len(parts) != 2 {
			http.Error(w, "Invalid address format", http.StatusBadRequest)
			return
		}

		zone, err = strconv.Atoi(parts[0])
		if err != nil {
			http.Error(w, "Invalid zone", http.StatusBadRequest)
			return
		}

		netNode := strings.Split(parts[1], "/")
		if len(netNode) != 2 {
			http.Error(w, "Invalid address format", http.StatusBadRequest)
			return
		}

		net, err = strconv.Atoi(netNode[0])
		if err != nil {
			http.Error(w, "Invalid net", http.StatusBadRequest)
			return
		}

		node, err = strconv.Atoi(netNode[1])
		if err != nil {
			http.Error(w, "Invalid node", http.StatusBadRequest)
			return
		}
	} else {
		// Try to get from query params
		zoneStr := r.URL.Query().Get("zone")
		netStr := r.URL.Query().Get("net")
		nodeStr := r.URL.Query().Get("node")

		if zoneStr == "" || netStr == "" || nodeStr == "" {
			// Show form
			data := map[string]interface{}{
				"Title":      "Node Reachability History",
				"Version":    version.GetVersionInfo(),
				"ActivePage": "reachability",
			}
			if err := s.templates["reachability"].Execute(w, data); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		zone, err = strconv.Atoi(zoneStr)
		if err != nil {
			http.Error(w, "Invalid zone", http.StatusBadRequest)
			return
		}

		net, err = strconv.Atoi(netStr)
		if err != nil {
			http.Error(w, "Invalid net", http.StatusBadRequest)
			return
		}

		node, err = strconv.Atoi(nodeStr)
		if err != nil {
			http.Error(w, "Invalid node", http.StatusBadRequest)
			return
		}
	}

	// Get days parameter (default 30)
	days := 30
	if daysStr := r.FormValue("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
			days = d
		}
	}

	// Get test history
	history, err := s.storage.GetNodeTestHistory(zone, net, node, days)
	if err != nil {
		log.Printf("Error getting node test history: %v", err)
		history = []storage.NodeTestResult{}
	}

	// Auto-detect data format
	hasPerHostnameData := false
	for _, r := range history {
		if r.HostnameIndex >= 0 {
			hasPerHostnameData = true
			break
		}
	}

	// Group results by test session if using per-hostname format
	var groupedHistory []interface{}
	if hasPerHostnameData {
		// Group by test time
		sessionMap := make(map[time.Time][]storage.NodeTestResult)
		for _, r := range history {
			sessionMap[r.TestTime] = append(sessionMap[r.TestTime], r)
		}

		// Convert to sorted list
		type TestSession struct {
			TestTime     time.Time
			Aggregated   *storage.NodeTestResult
			PerHostname  []storage.NodeTestResult
			HasMultiple  bool
		}

		var sessions []TestSession
		for testTime, results := range sessionMap {
			session := TestSession{TestTime: testTime}

			// Separate aggregated from per-hostname
			for _, r := range results {
				if r.IsAggregated {
					session.Aggregated = &r
				} else {
					session.PerHostname = append(session.PerHostname, r)
				}
			}

			// Sort per-hostname by index
			sort.Slice(session.PerHostname, func(i, j int) bool {
				return session.PerHostname[i].HostnameIndex < session.PerHostname[j].HostnameIndex
			})

			session.HasMultiple = len(session.PerHostname) > 1
			sessions = append(sessions, session)
		}

		// Sort sessions by time (newest first)
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].TestTime.After(sessions[j].TestTime)
		})

		// Convert to interface for template
		for _, s := range sessions {
			groupedHistory = append(groupedHistory, s)
		}
	} else {
		// Legacy format - just use history as-is
		for _, r := range history {
			groupedHistory = append(groupedHistory, r)
		}
	}

	// Get statistics
	stats, err := s.storage.GetNodeReachabilityStats(zone, net, node, days)
	if err != nil {
		log.Printf("Error getting node reachability stats: %v", err)
	}

	// Get node info from main database
	nodeHistory, err := s.storage.GetNodeHistory(zone, net, node)
	var nodeInfo *database.Node
	if err == nil && len(nodeHistory) > 0 {
		// Get the most recent entry
		nodeInfo = &nodeHistory[len(nodeHistory)-1]
	}

	data := map[string]interface{}{
		"Title":              "Node Reachability History",
		"Version":            version.GetVersionInfo(),
		"ActivePage":         "reachability",
		"Zone":               zone,
		"Net":                net,
		"Node":               node,
		"Address":            fmt.Sprintf("%d:%d/%d", zone, net, node),
		"Days":               days,
		"History":            history,
		"GroupedHistory":     groupedHistory,
		"Stats":              stats,
		"NodeInfo":           nodeInfo,
		"HasResults":         len(history) > 0,
		"HasPerHostnameData": hasPerHostnameData,
	}

	if err := s.templates["reachability"].Execute(w, data); err != nil {
		log.Printf("Error executing reachability template: %v", err)
	}
}

// TestResultDetailHandler shows detailed information about a specific test result
func (s *Server) TestResultDetailHandler(w http.ResponseWriter, r *http.Request) {
	// Parse parameters from URL query
	zoneStr := r.URL.Query().Get("zone")
	netStr := r.URL.Query().Get("net")
	nodeStr := r.URL.Query().Get("node")
	testTime := r.URL.Query().Get("time")

	if zoneStr == "" || netStr == "" || nodeStr == "" || testTime == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	zone, err := strconv.Atoi(zoneStr)
	if err != nil {
		http.Error(w, "Invalid zone", http.StatusBadRequest)
		return
	}

	net, err := strconv.Atoi(netStr)
	if err != nil {
		http.Error(w, "Invalid net", http.StatusBadRequest)
		return
	}

	node, err := strconv.Atoi(nodeStr)
	if err != nil {
		http.Error(w, "Invalid node", http.StatusBadRequest)
		return
	}

	// Get detailed test result
	testResult, err := s.storage.GetDetailedTestResult(zone, net, node, testTime)
	if err != nil {
		log.Printf("Error getting detailed test result: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if testResult == nil {
		http.Error(w, "Test result not found", http.StatusNotFound)
		return
	}

	// Get node info from main database for context
	nodeHistory, err := s.storage.GetNodeHistory(zone, net, node)
	var nodeInfo *database.Node
	if err == nil && len(nodeHistory) > 0 {
		// Get the most recent entry
		nodeInfo = &nodeHistory[len(nodeHistory)-1]
	}

	data := map[string]interface{}{
		"Title":      "Test Result Details",
		"Version":    version.GetVersionInfo(),
		"ActivePage": "reachability",
		"TestResult": testResult,
		"NodeInfo":   nodeInfo,
		"Address":    fmt.Sprintf("%d:%d/%d", zone, net, node),
	}

	if err := s.templates["test_detail"].Execute(w, data); err != nil {
		log.Printf("Error executing test detail template: %v", err)
	}
}

// ModemTestDetailHandler shows detailed information about a specific modem test result
func (s *Server) ModemTestDetailHandler(w http.ResponseWriter, r *http.Request) {
	zoneStr := r.URL.Query().Get("zone")
	netStr := r.URL.Query().Get("net")
	nodeStr := r.URL.Query().Get("node")
	testTime := r.URL.Query().Get("time")

	if zoneStr == "" || netStr == "" || nodeStr == "" || testTime == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	zone, err := strconv.Atoi(zoneStr)
	if err != nil {
		http.Error(w, "Invalid zone", http.StatusBadRequest)
		return
	}

	net, err := strconv.Atoi(netStr)
	if err != nil {
		http.Error(w, "Invalid net", http.StatusBadRequest)
		return
	}

	node, err := strconv.Atoi(nodeStr)
	if err != nil {
		http.Error(w, "Invalid node", http.StatusBadRequest)
		return
	}

	result, err := s.storage.GetDetailedModemTestResult(zone, net, node, testTime)
	if err != nil {
		log.Printf("Error getting detailed modem test result: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if result == nil {
		http.Error(w, "Modem test result not found", http.StatusNotFound)
		return
	}

	// Get node info from main database for context
	nodeHistory, err := s.storage.GetNodeHistory(zone, net, node)
	var nodeInfo *database.Node
	if err == nil && len(nodeHistory) > 0 {
		nodeInfo = &nodeHistory[len(nodeHistory)-1]
	}

	data := map[string]interface{}{
		"Title":      "Modem Test Details",
		"Version":    version.GetVersionInfo(),
		"ActivePage": "analytics",
		"TestResult": result,
		"NodeInfo":   nodeInfo,
		"Address":    fmt.Sprintf("%d:%d/%d", zone, net, node),
	}

	if err := s.templates["modem_test_detail"].Execute(w, data); err != nil {
		log.Printf("Error executing modem test detail template: %v", err)
	}
}
