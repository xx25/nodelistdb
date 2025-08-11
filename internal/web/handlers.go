package web

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nodelistdb/internal/database"
	"nodelistdb/internal/flags"
	"nodelistdb/internal/storage"
	"nodelistdb/internal/version"
)

// Server represents the web server
type Server struct {
	storage     *storage.Storage
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

// buildChangeFilter creates a ChangeFilter from URL query parameters
func buildChangeFilter(query url.Values) storage.ChangeFilter {
	return storage.ChangeFilter{
		IgnoreFlags:             query.Get("noflags") == "1",
		IgnorePhone:             query.Get("nophone") == "1",
		IgnoreSpeed:             query.Get("nospeed") == "1",
		IgnoreStatus:            query.Get("nostatus") == "1",
		IgnoreLocation:          query.Get("nolocation") == "1",
		IgnoreName:              query.Get("noname") == "1",
		IgnoreSysop:             query.Get("nosysop") == "1",
		IgnoreConnectivity:      query.Get("noconnectivity") == "1",
		IgnoreModemFlags:        query.Get("nomodemflags") == "1",
		// Internet array fields are no longer available - these options are ignored
	}
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
func New(storage *storage.Storage, templatesFS embed.FS, staticFS embed.FS) *Server {
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

	filter := buildChangeFilter(r.URL.Query())

	// Get node changes with filter applied
	changes, err := s.storage.GetNodeChanges(zone, net, node, filter)
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
		Filter           storage.ChangeFilter
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
		Filter:           filter,
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
