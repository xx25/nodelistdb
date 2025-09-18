package web

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// Server represents the web server
type Server struct {
	storage     storage.Operations
	templates   map[string]*template.Template
	templatesFS embed.FS
	staticFS    embed.FS
}

// parseNodeURLPath extracts zone, net, and node from URL path /node/{zone}/{net}/{node}
func parseNodeURLPath(path string) (zone, net, node int, err error) {
	path = strings.TrimPrefix(path, "/node/")
	parts := strings.Split(path, "/")

	if len(parts) < 3 {
		return 0, 0, 0, fmt.Errorf("invalid node address")
	}

	zone, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid zone")
	}

	net, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid net")
	}

	node, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid node")
	}

	return zone, net, node, nil
}


// NodeActivityInfo holds information about a node's activity
type NodeActivityInfo struct {
	FirstDate       time.Time
	LastDate        time.Time
	CurrentlyActive bool
}

// analyzeNodeActivity analyzes node history to determine activity information
func analyzeNodeActivity(history []database.Node) NodeActivityInfo {
	var info NodeActivityInfo

	if len(history) > 0 {
		info.FirstDate = history[0].NodelistDate
		info.LastDate = history[len(history)-1].NodelistDate

		// Check if currently active (last entry within 30 days)
		daysSinceLastSeen := time.Since(info.LastDate).Hours() / 24
		info.CurrentlyActive = daysSinceLastSeen <= 30
	}

	return info
}

// New creates a new web server
func New(storage storage.Operations, templatesFS embed.FS, staticFS embed.FS) *Server {
	server := &Server{
		storage:     storage,
		templates:   make(map[string]*template.Template),
		templatesFS: templatesFS,
		staticFS:    staticFS,
	}

	server.loadTemplates()
	return server
}

// IndexHandler handles the home page
func (s *Server) IndexHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title      string
		ActivePage string
		Version    string
	}{
		Title:      "FidoNet Nodelist Database",
		ActivePage: "home",
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["index"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SearchHandler handles node search
func (s *Server) SearchHandler(w http.ResponseWriter, r *http.Request) {
	nodes, count, searchErr := s.performNodeSearchWithLifetime(r)

	data := struct {
		Title      string
		ActivePage string
		Nodes      []storage.NodeSummary
		Count      int
		Error      error
		Version    string
	}{
		Title:      "Search Nodes",
		ActivePage: "search",
		Nodes:      nodes,
		Count:      count,
		Error:      searchErr,
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["search"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// performNodeSearch handles the actual node search logic
func (s *Server) performNodeSearch(r *http.Request) ([]database.Node, int, error) {
	if r.Method != "POST" {
		return nil, 0, nil
	}

	r.ParseForm()

	var filter database.NodeFilter
	var err error

	// Check if full address was provided
	if fullAddress := r.FormValue("full_address"); fullAddress != "" {
		filter, err = buildNodeFilterFromAddress(fullAddress)
		if err != nil {
			return nil, 0, fmt.Errorf("Invalid address format: %v", err)
		}
	} else {
		// Build filter from individual fields
		filter = buildNodeFilterFromForm(r)

		// Check if search would be too resource-intensive
		if filter.Limit == 0 {
			return nil, 0, fmt.Errorf("Search requires more specific criteria. Please specify zone or net along with node number, or search by system name/location (minimum 2 characters)")
		}
	}

	nodes, err := s.storage.GetNodes(filter)
	if err != nil {
		return nil, 0, err
	}

	return nodes, len(nodes), nil
}

// performNodeSearchWithLifetime handles the actual node search logic and returns NodeSummary with lifetime info
func (s *Server) performNodeSearchWithLifetime(r *http.Request) ([]storage.NodeSummary, int, error) {
	if r.Method != "POST" {
		return nil, 0, nil
	}

	r.ParseForm()

	var filter database.NodeFilter
	var err error

	// Check if full address was provided
	if fullAddress := r.FormValue("full_address"); fullAddress != "" {
		filter, err = buildNodeFilterFromAddress(fullAddress)
		if err != nil {
			return nil, 0, fmt.Errorf("Invalid address format: %v", err)
		}
	} else {
		// Build filter from individual fields
		filter = buildNodeFilterFromForm(r)

		// Check if search would be too resource-intensive
		if filter.Limit == 0 {
			return nil, 0, fmt.Errorf("Search requires more specific criteria. Please specify zone or net along with node number, or search by system name/location (minimum 2 characters)")
		}
	}

	nodes, err := s.storage.SearchNodesWithLifetime(filter)
	if err != nil {
		return nil, 0, err
	}

	return nodes, len(nodes), nil
}

// StatsHandler handles statistics page
func (s *Server) StatsHandler(w http.ResponseWriter, r *http.Request) {
	var selectedDate time.Time
	var actualDate time.Time
	var err error
	var dateAdjusted bool
	var availableDates []time.Time

	// Get available dates for the dropdown
	availableDates, err = s.storage.GetAvailableDates()
	if err != nil {
		data := struct {
			Title          string
			ActivePage     string
			Stats          *database.NetworkStats
			Error          error
			NoData         bool
			AvailableDates []time.Time
			SelectedDate   string
			ActualDate     string
			DateAdjusted   bool
			Version        string
		}{
			Title:          "Network Statistics",
			ActivePage:     "stats",
			Stats:          nil,
			Error:          fmt.Errorf("Failed to get available dates: %v", err),
			NoData:         true,
			AvailableDates: []time.Time{},
			SelectedDate:   "",
			ActualDate:     "",
			DateAdjusted:   false,
			Version:        version.GetVersionInfo(),
		}

		if err := s.templates["stats"].Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Parse date parameter from query string
	dateStr := r.URL.Query().Get("date")
	if dateStr != "" {
		selectedDate, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			// Invalid date format, fall back to latest
			actualDate, err = s.storage.GetLatestStatsDate()
			if err != nil {
				data := struct {
					Title          string
					ActivePage     string
					Stats          *database.NetworkStats
					Error          error
					NoData         bool
					AvailableDates []time.Time
					SelectedDate   string
					ActualDate     string
					DateAdjusted   bool
					Version        string
				}{
					Title:          "Network Statistics",
					ActivePage:     "stats",
					Stats:          nil,
					Error:          fmt.Errorf("Invalid date format and failed to get latest date: %v", err),
					NoData:         true,
					AvailableDates: availableDates,
					SelectedDate:   dateStr,
					ActualDate:     "",
					DateAdjusted:   false,
					Version:        version.GetVersionInfo(),
				}

				if err := s.templates["stats"].Execute(w, data); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			dateAdjusted = true
		} else {
			// Find the nearest available date
			actualDate, err = s.storage.GetNearestAvailableDate(selectedDate)
			if err != nil {
				data := struct {
					Title          string
					ActivePage     string
					Stats          *database.NetworkStats
					Error          error
					NoData         bool
					AvailableDates []time.Time
					SelectedDate   string
					ActualDate     string
					DateAdjusted   bool
					Version        string
				}{
					Title:          "Network Statistics",
					ActivePage:     "stats",
					Stats:          nil,
					Error:          fmt.Errorf("Failed to find available date: %v", err),
					NoData:         true,
					AvailableDates: availableDates,
					SelectedDate:   dateStr,
					ActualDate:     "",
					DateAdjusted:   false,
					Version:        version.GetVersionInfo(),
				}

				if err := s.templates["stats"].Execute(w, data); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
			dateAdjusted = !actualDate.Equal(selectedDate)
		}
	} else {
		// No date specified, use latest
		actualDate, err = s.storage.GetLatestStatsDate()
		if err != nil {
			data := struct {
				Title          string
				ActivePage     string
				Stats          *database.NetworkStats
				Error          error
				NoData         bool
				AvailableDates []time.Time
				SelectedDate   string
				ActualDate     string
				DateAdjusted   bool
				Version        string
			}{
				Title:          "Network Statistics",
				ActivePage:     "stats",
				Stats:          nil,
				Error:          fmt.Errorf("Failed to find latest nodelist date: %v", err),
				NoData:         true,
				AvailableDates: availableDates,
				SelectedDate:   "",
				ActualDate:     "",
				DateAdjusted:   false,
				Version:        version.GetVersionInfo(),
			}

			if err := s.templates["stats"].Execute(w, data); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}

	// Get stats for the actual date
	stats, err := s.storage.GetStats(actualDate)

	data := struct {
		Title          string
		ActivePage     string
		Stats          *database.NetworkStats
		Error          error
		NoData         bool
		AvailableDates []time.Time
		SelectedDate   string
		ActualDate     string
		DateAdjusted   bool
		Version        string
	}{
		Title:          "Network Statistics",
		ActivePage:     "stats",
		Stats:          stats,
		Error:          err,
		NoData:         stats == nil || stats.TotalNodes == 0,
		AvailableDates: availableDates,
		SelectedDate:   dateStr,
		ActualDate:     actualDate.Format("2006-01-02"),
		DateAdjusted:   dateAdjusted,
		Version:        version.GetVersionInfo(),
	}

	if data.NoData && err == nil {
		data.Error = fmt.Errorf("No nodelist data available. Please import nodelist files first.")
	}

	if err := s.templates["stats"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SysopSearchHandler handles sysop search
func (s *Server) SysopSearchHandler(w http.ResponseWriter, r *http.Request) {
	var nodes []storage.NodeSummary
	var count int
	var searchErr error
	var sysopName string

	if r.Method == "POST" {
		r.ParseForm()
		sysopName = r.FormValue("sysop_name")

		if sysopName != "" {
			nodes, searchErr = s.storage.SearchNodesBySysop(sysopName, 100)
			count = len(nodes)
		}
	}

	data := struct {
		Title      string
		ActivePage string
		Nodes      []storage.NodeSummary
		Count      int
		Error      error
		SysopName  string
		Version    string
	}{
		Title:      "Search by Sysop",
		ActivePage: "sysop",
		Nodes:      nodes,
		Count:      count,
		Error:      searchErr,
		SysopName:  sysopName,
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["sysop_search"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// NodeHistoryHandler handles node history view
func (s *Server) NodeHistoryHandler(w http.ResponseWriter, r *http.Request) {
	zone, net, node, err := parseNodeURLPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get node history
	history, err := s.storage.GetNodeHistory(zone, net, node)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving node history: %v", err), http.StatusInternalServerError)
		return
	}

	if len(history) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Get all node changes without filtering
	changes, err := s.storage.GetNodeChanges(zone, net, node)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving node changes: %v", err), http.StatusInternalServerError)
		return
	}

	activityInfo := analyzeNodeActivity(history)

	data := struct {
		Title            string
		Address          string
		History          []database.Node
		Changes          []database.NodeChange
		FirstDate        time.Time
		LastDate         time.Time
		CurrentlyActive  bool
		FlagDescriptions map[string]flags.FlagInfo
		Version          string
		ActivePage       string
	}{
		Title:            "Node History",
		Address:          fmt.Sprintf("%d:%d/%d", zone, net, node),
		History:          history,
		Changes:          changes,
		FirstDate:        activityInfo.FirstDate,
		LastDate:         activityInfo.LastDate,
		CurrentlyActive:  activityInfo.CurrentlyActive,
		FlagDescriptions: flags.GetFlagDescriptions(),
		Version:          version.GetVersionInfo(),
		ActivePage:       "",
	}

	if err := s.templates["node_history"].Execute(w, data); err != nil {
		log.Printf("Error executing node_history template: %v", err)
	}
}

// APIHelpHandler shows API documentation
func (s *Server) APIHelpHandler(w http.ResponseWriter, r *http.Request) {
	// Determine the scheme (http or https)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// Check for X-Forwarded-Proto header (common with reverse proxies)
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	// Get the host from the request
	host := r.Host
	if host == "" {
		host = "localhost:8080" // fallback
	}

	// Construct the base URL
	apiURL := fmt.Sprintf("%s://%s/api/", scheme, host)
	siteURL := fmt.Sprintf("%s://%s", scheme, host)

	data := struct {
		Title      string
		ActivePage string
		BaseURL    string
		SiteURL    string
		Version    string
	}{
		Title:      "API Documentation",
		ActivePage: "api",
		BaseURL:    apiURL,
		SiteURL:    siteURL,
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["api_help"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// AnalyticsHandler shows the analytics page
func (s *Server) AnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title           string
		ActivePage      string
		Flag            string
		FirstAppearance *storage.FlagFirstAppearance
		YearlyUsage     []storage.FlagUsageByYear
		Network         string
		NetworkHistory  *storage.NetworkHistory
		Error           error
		Version         string
	}{
		Title:      "Analytics",
		ActivePage: "analytics",
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["analytics"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// AnalyticsFlagHandler handles flag analytics requests
func (s *Server) AnalyticsFlagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/analytics", http.StatusSeeOther)
		return
	}

	flag := r.FormValue("flag")

	data := struct {
		Title           string
		ActivePage      string
		Flag            string
		FirstAppearance *storage.FlagFirstAppearance
		YearlyUsage     []storage.FlagUsageByYear
		Network         string
		NetworkHistory  *storage.NetworkHistory
		Error           error
		Version         string
	}{
		Title:      "Analytics",
		ActivePage: "analytics",
		Flag:       flag,
		Version:    version.GetVersionInfo(),
	}

	if flag == "" {
		data.Error = fmt.Errorf("Flag cannot be empty")
	} else {
		// Get first appearance
		firstAppearance, err := s.storage.GetFlagFirstAppearance(flag)
		if err != nil {
			data.Error = fmt.Errorf("Failed to get first appearance: %v", err)
		} else {
			data.FirstAppearance = firstAppearance
		}

		// Get yearly usage
		if data.Error == nil {
			yearlyUsage, err := s.storage.GetFlagUsageByYear(flag)
			if err != nil {
				data.Error = fmt.Errorf("Failed to get yearly usage: %v", err)
			} else {
				data.YearlyUsage = yearlyUsage
			}
		}
	}

	if err := s.templates["analytics"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// AnalyticsNetworkHandler handles network analytics requests
func (s *Server) AnalyticsNetworkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/analytics", http.StatusSeeOther)
		return
	}

	network := r.FormValue("network")

	data := struct {
		Title           string
		ActivePage      string
		Flag            string
		FirstAppearance *storage.FlagFirstAppearance
		YearlyUsage     []storage.FlagUsageByYear
		Network         string
		NetworkHistory  *storage.NetworkHistory
		Error           error
		Version         string
	}{
		Title:      "Analytics",
		ActivePage: "analytics",
		Network:    network,
		Version:    version.GetVersionInfo(),
	}

	if network == "" {
		data.Error = fmt.Errorf("Please enter a network address (e.g., 2:5000)")
	} else {
		// Parse network address (zone:net)
		var zone, net int
		_, err := fmt.Sscanf(network, "%d:%d", &zone, &net)
		if err != nil {
			data.Error = fmt.Errorf("Invalid network format. Use zone:net (e.g., 2:5000)")
		} else {
			// Get network history
			history, err := s.storage.GetNetworkHistory(zone, net)
			if err != nil {
				data.Error = fmt.Errorf("Failed to fetch network history: %v", err)
			} else if history == nil {
				data.Error = fmt.Errorf("Network %s not found", network)
			} else {
				data.NetworkHistory = history
			}
		}
	}

	if err := s.templates["analytics"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// ReachabilityHandler serves the reachability history main page
func (s *Server) ReachabilityHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters for filtering
	query := r.URL.Query()

	statusFilter := query.Get("status")
	protocolFilter := query.Get("protocol")

	// Parse period filter (default to 1 day for nodes, 30 days for trends)
	trendsPeriodFilter := 30 // For trends chart
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

// IPv6AnalyticsHandler shows IPv6 enabled nodes analytics
func (s *Server) IPv6AnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()

	// Days parameter
	daysStr := query.Get("days")
	days := 30 // default
	if daysStr != "" {
		if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	// Limit parameter
	limitStr := query.Get("limit")
	limit := 1000 // default
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	// Include /0 nodes parameter (default: false)
	includeZeroNodes := query.Get("includeZero") == "true"

	// Get IPv6 enabled nodes
	ipv6Nodes, err := s.storage.GetIPv6EnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		log.Printf("[ERROR] IPv6Analytics: Error getting IPv6 enabled nodes: %v", err)
		ipv6Nodes = []storage.NodeTestResult{}
	}

	data := struct {
		Title            string
		ActivePage       string
		Version          string
		IPv6Nodes        []storage.NodeTestResult
		Days             int
		Limit            int
		IncludeZeroNodes bool
		Error            error
	}{
		Title:            "IPv6 Enabled Nodes",
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		IPv6Nodes:        ipv6Nodes,
		Days:             days,
		Limit:            limit,
		IncludeZeroNodes: includeZeroNodes,
		Error:            err,
	}

	if err := s.templates["ipv6_analytics"].Execute(w, data); err != nil {
		log.Printf("Error executing IPv6 analytics template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// BinkPAnalyticsHandler shows BinkP enabled nodes analytics
func (s *Server) BinkPAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()

	// Days parameter
	daysStr := query.Get("days")
	days := 30 // default
	if daysStr != "" {
		if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	// Limit parameter
	limitStr := query.Get("limit")
	limit := 1000 // default
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	// Include /0 nodes parameter (default: false)
	includeZeroNodes := query.Get("includeZero") == "true"

	// Get BinkP enabled nodes
	binkpNodes, err := s.storage.GetBinkPEnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		log.Printf("Error getting BinkP enabled nodes: %v", err)
		binkpNodes = []storage.NodeTestResult{}
	}

	data := struct {
		Title            string
		ActivePage       string
		Version          string
		ProtocolNodes    []storage.NodeTestResult
		Days             int
		Limit            int
		IncludeZeroNodes bool
		Error            error
	}{
		Title:            "BinkP Enabled Nodes",
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		ProtocolNodes:    binkpNodes,
		Days:             days,
		Limit:            limit,
		IncludeZeroNodes: includeZeroNodes,
		Error:            err,
	}

	if err := s.templates["binkp_analytics"].Execute(w, data); err != nil {
		log.Printf("Error executing BinkP analytics template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// IfcicoAnalyticsHandler shows IFCICO enabled nodes analytics
func (s *Server) IfcicoAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()

	// Days parameter
	daysStr := query.Get("days")
	days := 30 // default
	if daysStr != "" {
		if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	// Limit parameter
	limitStr := query.Get("limit")
	limit := 1000 // default
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	// Include /0 nodes parameter (default: false)
	includeZeroNodes := query.Get("includeZero") == "true"

	// Get IFCICO enabled nodes
	ifcicoNodes, err := s.storage.GetIfcicoEnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		log.Printf("Error getting IFCICO enabled nodes: %v", err)
		ifcicoNodes = []storage.NodeTestResult{}
	}

	data := struct {
		Title            string
		ActivePage       string
		Version          string
		ProtocolNodes    []storage.NodeTestResult
		Days             int
		Limit            int
		IncludeZeroNodes bool
		Error            error
	}{
		Title:            "IFCICO Enabled Nodes",
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		ProtocolNodes:    ifcicoNodes,
		Days:             days,
		Limit:            limit,
		IncludeZeroNodes: includeZeroNodes,
		Error:            err,
	}

	if err := s.templates["ifcico_analytics"].Execute(w, data); err != nil {
		log.Printf("Error executing IFCICO analytics template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// BinkPSoftwareHandler shows BinkP software distribution analytics
func (s *Server) BinkPSoftwareHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title      string
		ActivePage string
		Version    string
	}{
		Title:      "BinkP Software Distribution",
		ActivePage: "analytics",
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["binkp_software"].Execute(w, data); err != nil {
		log.Printf("Error executing BinkP software template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// IfcicoSoftwareHandler shows IFCICO software distribution analytics
func (s *Server) IfcicoSoftwareHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title      string
		ActivePage string
		Version    string
	}{
		Title:      "IFCICO Software Distribution",
		ActivePage: "analytics",
		Version:    version.GetVersionInfo(),
	}

	if err := s.templates["ifcico_software"].Execute(w, data); err != nil {
		log.Printf("Error executing IFCICO software template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// TelnetAnalyticsHandler shows Telnet enabled nodes analytics
func (s *Server) TelnetAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()

	// Days parameter
	daysStr := query.Get("days")
	days := 30 // default
	if daysStr != "" {
		if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	// Limit parameter
	limitStr := query.Get("limit")
	limit := 1000 // default
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	// Include /0 nodes parameter (default: false)
	includeZeroNodes := query.Get("includeZero") == "true"

	// Get Telnet enabled nodes
	telnetNodes, err := s.storage.GetTelnetEnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		log.Printf("Error getting Telnet enabled nodes: %v", err)
		telnetNodes = []storage.NodeTestResult{}
	}

	data := struct {
		Title            string
		ActivePage       string
		Version          string
		ProtocolNodes    []storage.NodeTestResult
		Days             int
		Limit            int
		IncludeZeroNodes bool
		Error            error
	}{
		Title:            "Telnet Enabled Nodes",
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		ProtocolNodes:    telnetNodes,
		Days:             days,
		Limit:            limit,
		IncludeZeroNodes: includeZeroNodes,
		Error:            err,
	}

	if err := s.templates["telnet_analytics"].Execute(w, data); err != nil {
		log.Printf("Error executing Telnet analytics template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// VModemAnalyticsHandler shows VModem enabled nodes analytics
func (s *Server) VModemAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()

	// Days parameter
	daysStr := query.Get("days")
	days := 30 // default
	if daysStr != "" {
		if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	// Limit parameter
	limitStr := query.Get("limit")
	limit := 1000 // default
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	// Include /0 nodes parameter (default: false)
	includeZeroNodes := query.Get("includeZero") == "true"

	// Get VModem enabled nodes
	vmodemNodes, err := s.storage.GetVModemEnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		log.Printf("Error getting VModem enabled nodes: %v", err)
		vmodemNodes = []storage.NodeTestResult{}
	}

	data := struct {
		Title            string
		ActivePage       string
		Version          string
		ProtocolNodes    []storage.NodeTestResult
		Days             int
		Limit            int
		IncludeZeroNodes bool
		Error            error
	}{
		Title:            "VModem Enabled Nodes",
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		ProtocolNodes:    vmodemNodes,
		Days:             days,
		Limit:            limit,
		IncludeZeroNodes: includeZeroNodes,
		Error:            err,
	}

	if err := s.templates["vmodem_analytics"].Execute(w, data); err != nil {
		log.Printf("Error executing VModem analytics template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// FTPAnalyticsHandler shows FTP enabled nodes analytics
func (s *Server) FTPAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()

	// Days parameter
	daysStr := query.Get("days")
	days := 30 // default
	if daysStr != "" {
		if parsed, err := strconv.Atoi(daysStr); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	// Limit parameter
	limitStr := query.Get("limit")
	limit := 1000 // default
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	// Include /0 nodes parameter (default: false)
	includeZeroNodes := query.Get("includeZero") == "true"

	// Get FTP enabled nodes
	ftpNodes, err := s.storage.GetFTPEnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		log.Printf("Error getting FTP enabled nodes: %v", err)
		ftpNodes = []storage.NodeTestResult{}
	}

	data := struct {
		Title            string
		ActivePage       string
		Version          string
		ProtocolNodes    []storage.NodeTestResult
		Days             int
		Limit            int
		IncludeZeroNodes bool
		Error            error
	}{
		Title:            "FTP Enabled Nodes",
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		ProtocolNodes:    ftpNodes,
		Days:             days,
		Limit:            limit,
		IncludeZeroNodes: includeZeroNodes,
		Error:            err,
	}

	if err := s.templates["ftp_analytics"].Execute(w, data); err != nil {
		log.Printf("Error executing FTP analytics template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
