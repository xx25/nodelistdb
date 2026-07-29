package web

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// ReachabilityHandler serves the reachability history main page
func (s *Server) ReachabilityHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters for filtering
	query := r.URL.Query()

	statusFilter := query.Get("status")
	protocolFilter := query.Get("protocol")

	// Parse period filter (default to 1 day for nodes, 0=all time for trends)
	trendsPeriodFilter := 0 // 0 means all time (from first test date)
	nodesPeriodFilter := 1  // Default to 1 day for recently tested nodes
	if p := query.Get("trends_period"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			trendsPeriodFilter = parsed
		}
	}
	if p := query.Get("nodes_period"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 && parsed <= 365 {
			nodesPeriodFilter = parsed
		}
	}

	// Parse limit filter (default to the 10 most recent tests)
	limitFilter := 10
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limitFilter = parsed
		}
	}

	// Scope everything to the globally selected network
	domain := requestDomain(r)

	// Get overall trends
	var trends []storage.ReachabilityTrend
	var err error
	if trendsPeriodFilter == 0 {
		trends, err = s.storage.GetReachabilityTrendsAllTime(r.Context(), domain)
	} else {
		trends, err = s.storage.GetReachabilityTrends(r.Context(), trendsPeriodFilter, domain)
	}
	if err != nil {
		if clientGone("Reachability: trends", err) {
			return
		}
		logging.Errorf("Error getting reachability trends: %v", err)
		trends = []storage.ReachabilityTrend{}
	}

	data := map[string]interface{}{
		"Title":              "Reachability History",
		"Version":            version.GetVersionInfo(),
		"ActivePage":         "reachability",
		"Trends":             trends,
		"StatusFilter":       statusFilter,
		"TrendsPeriodFilter": trendsPeriodFilter,
		"NodesPeriodFilter":  nodesPeriodFilter,
		"ProtocolFilter":     protocolFilter,
		"LimitFilter":        limitFilter,
	}

	// The list is always populated, filters or not: an unfiltered visit is just
	// the defaults above - every status, every protocol, the last 24 hours, the
	// 10 most recently tested nodes - so the page opens on data instead of on an
	// empty "no nodes found" panel.
	filteredNodes, err := s.getFilteredReachabilityNodes(r.Context(), statusFilter, protocolFilter, nodesPeriodFilter, limitFilter, domain)
	if err != nil {
		if clientGone("Reachability: filtered nodes", err) {
			return
		}
		logging.Errorf("Error getting filtered nodes: %v", err)
		filteredNodes = []storage.NodeTestResult{}
	}
	data["FilteredNodes"] = filteredNodes

	s.render(w, "reachability", data)
}

// reachabilityFetchFloor is the smallest pre-filter pool the protocol filter is
// given to work with, whatever the requested page size.
//
// The protocol filter runs in Go over rows the status query already returned, so
// a pool the size of the page finds nothing for a protocol that is absent from
// the newest tests but present a little further back - a rare protocol like FTP
// can easily be missing from the 20 most recent results. The pool is a floor
// rather than a multiple of the page size because the default page is only 10
// rows.
const reachabilityFetchFloor = 50

// getFilteredReachabilityNodes retrieves nodes based on the applied filters
func (s *Server) getFilteredReachabilityNodes(ctx context.Context, statusFilter, protocolFilter string, periodFilter, limitFilter int, domain string) ([]storage.NodeTestResult, error) {
	// For now, use the existing SearchNodesByReachability method and apply additional filtering
	// This could be optimized by adding dedicated database queries for these filters

	var allNodes []storage.NodeTestResult

	fetchLimit := max(limitFilter*2, reachabilityFetchFloor)

	switch statusFilter {
	case "operational":
		nodes, err := s.storage.SearchNodesByReachability(ctx, true, fetchLimit, periodFilter, domain)
		if err != nil {
			return nil, err
		}
		allNodes = nodes
	case "failed":
		nodes, err := s.storage.SearchNodesByReachability(ctx, false, fetchLimit, periodFilter, domain)
		if err != nil {
			return nil, err
		}
		allNodes = nodes
	default: // "all" or empty
		// Get both operational and failed nodes - fetch more to ensure we get both types
		// When status=all, we want to show a mix of both operational and failed
		operational, err := s.storage.SearchNodesByReachability(ctx, true, fetchLimit, periodFilter, domain)
		if err != nil {
			return nil, err
		}
		failed, err := s.storage.SearchNodesByReachability(ctx, false, fetchLimit, periodFilter, domain)
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
		case "ftp":
			if node.FTPSuccess {
				filteredNodes = append(filteredNodes, node)
			}
		case "vmodem":
			// VModemSuccess only says something answered on the announced IVM
			// port — usually an EMSI mailer or a telnet login, not VMODEM. The
			// filter asks for VMODEM, so it takes confirmed VMP responders only,
			// same as the /analytics/vmodem page.
			if node.IsConfirmedVMODEM() {
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

		zone, net, node, err = parseNodeAddress(address)
		if err != nil {
			http.Error(w, "Invalid address format", http.StatusBadRequest)
			return
		}
	} else {
		var present, ok bool
		zone, net, node, present, ok = parseAddressQuery(w, r)
		if !ok {
			return
		}
		if !present {
			// No address given at all: show the lookup form.
			s.render(w, "reachability", map[string]interface{}{
				"Title":      "Node Reachability History",
				"Version":    version.GetVersionInfo(),
				"ActivePage": "reachability",
			})
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

	// Resolve the network like the node page: explicit ?domain= wins, then
	// the global switcher when the address exists there, then wherever the
	// address actually lives — so the page never silently merges test
	// history from several networks sharing one zone:net/node.
	availableDomains, _ := s.storage.GetNodeDomains(r.Context(), zone, net, node)
	domain := resolveEntityDomain(r, availableDomains)
	history, err := s.storage.GetNodeTestHistory(r.Context(), zone, net, node, days, domain)
	if err != nil {
		if clientGone("Reachability: node test history", err) {
			return
		}
		logging.Errorf("Error getting node test history: %v", err)
		history = []storage.NodeTestResult{}
	}

	// Get statistics
	stats, err := s.storage.GetNodeReachabilityStats(r.Context(), zone, net, node, days, domain)
	if err != nil {
		if clientGone("Reachability: node stats", err) {
			return
		}
		logging.Errorf("Error getting node reachability stats: %v", err)
	}

	// Get node info from main database (same resolved network)
	nodeHistory, err := s.storage.GetNodeHistory(r.Context(), zone, net, node, domain)
	var nodeInfo *database.Node
	if err == nil && len(nodeHistory) > 0 {
		// Get the most recent entry
		nodeInfo = &nodeHistory[len(nodeHistory)-1]
	}

	data := map[string]interface{}{
		"Title":      "Node Reachability History",
		"Version":    version.GetVersionInfo(),
		"ActivePage": "reachability",
		"Zone":       zone,
		"Net":        net,
		"Node":       node,
		"Address":    fmt.Sprintf("%d:%d/%d", zone, net, node),
		"Days":       days,
		"History":    history,
		"Stats":      stats,
		"NodeInfo":   nodeInfo,
		"HasResults": len(history) > 0,
	}

	s.render(w, "reachability", data)
}

// parseAddressQuery reads ?zone=&net=&node= and answers 400 on a malformed
// one. present is false when none of the three was given, which is not an
// error - the reachability page shows its lookup form instead. It is reported
// separately from the values because 0:0/0 is a real address here: the test
// daemon files ad-hoc host:port probes under zone 0.
func parseAddressQuery(w http.ResponseWriter, r *http.Request) (zone, net, node int, present, ok bool) {
	query := r.URL.Query()
	zoneStr, netStr, nodeStr := query.Get("zone"), query.Get("net"), query.Get("node")
	if zoneStr == "" && netStr == "" && nodeStr == "" {
		return 0, 0, 0, false, true
	}

	for _, part := range []struct {
		name string
		raw  string
		dest *int
	}{
		{"zone", zoneStr, &zone},
		{"net", netStr, &net},
		{"node", nodeStr, &node},
	} {
		v, err := strconv.Atoi(part.raw)
		if err != nil {
			http.Error(w, "Invalid "+part.name, http.StatusBadRequest)
			return 0, 0, 0, false, false
		}
		*part.dest = v
	}
	return zone, net, node, true, true
}

// testDetailPage parameterises the two test-result detail pages. They differ
// in which store they read and which template renders it; everything else -
// parameter parsing, the node-info lookup for context, the payload shape - was
// duplicated between them.
type testDetailPage struct {
	title    string
	template string
	subject  string // for log lines and the not-found message
	fetch    func(zone, net, node int, testTime, domain string) (result any, found bool, err error)
}

// TestResultDetailHandler shows detailed information about a specific test result
func (s *Server) TestResultDetailHandler(w http.ResponseWriter, r *http.Request) {
	s.renderTestDetail(w, r, testDetailPage{
		title:    "Test Result Details",
		template: "test_detail",
		subject:  "test result",
		fetch: func(zone, net, node int, testTime, domain string) (any, bool, error) {
			result, err := s.storage.GetDetailedTestResult(r.Context(), zone, net, node, testTime, domain)
			return result, result != nil, err
		},
	})
}

// ModemTestDetailHandler shows detailed information about a specific modem test result
func (s *Server) ModemTestDetailHandler(w http.ResponseWriter, r *http.Request) {
	s.renderTestDetail(w, r, testDetailPage{
		title:    "Modem Test Details",
		template: "modem_test_detail",
		subject:  "modem test result",
		fetch: func(zone, net, node int, testTime, domain string) (any, bool, error) {
			result, err := s.storage.GetDetailedModemTestResult(r.Context(), zone, net, node, testTime)
			return result, result != nil, err
		},
	})
}

func (s *Server) renderTestDetail(w http.ResponseWriter, r *http.Request, page testDetailPage) {
	testTime := r.URL.Query().Get("time")
	zone, net, node, present, ok := parseAddressQuery(w, r)
	if !ok {
		return
	}
	if testTime == "" || !present {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	domain := explicitDomain(r)
	result, found, err := page.fetch(zone, net, node, testTime, domain)
	if err != nil {
		httpStorageError(w, "Reachability: detailed "+page.subject, "Internal server error", err)
		return
	}
	if !found {
		http.Error(w, strings.ToUpper(page.subject[:1])+page.subject[1:]+" not found", http.StatusNotFound)
		return
	}

	// Node info from the nodelist, for context on the page.
	var nodeInfo *database.Node
	if nodeHistory, err := s.storage.GetNodeHistory(r.Context(), zone, net, node, domain); err == nil && len(nodeHistory) > 0 {
		nodeInfo = &nodeHistory[len(nodeHistory)-1]
	}

	s.render(w, page.template, map[string]interface{}{
		"Title":      page.title,
		"Version":    version.GetVersionInfo(),
		"ActivePage": "reachability",
		"TestResult": result,
		"NodeInfo":   nodeInfo,
		"Address":    fmt.Sprintf("%d:%d/%d", zone, net, node),
	})
}
