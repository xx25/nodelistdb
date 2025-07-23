package web

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nodelistdb/internal/database"
	"nodelistdb/internal/storage"
)

// Server represents the web server
type Server struct {
	storage   *storage.Storage
	templates map[string]*template.Template
}

// New creates a new web server
func New(storage *storage.Storage) *Server {
	server := &Server{
		storage:   storage,
		templates: make(map[string]*template.Template),
	}
	
	server.loadTemplates()
	return server
}

// IndexHandler handles the home page
func (s *Server) IndexHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "FidoNet Nodelist Database",
	}
	
	if err := s.templates["index"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SearchHandler handles node search
func (s *Server) SearchHandler(w http.ResponseWriter, r *http.Request) {
	nodes, count, searchErr := s.performNodeSearch(r)
	
	data := struct {
		Title string
		Nodes []database.Node
		Count int
		Error error
	}{
		Title: "Search Nodes",
		Nodes: nodes,
		Count: count,
		Error: searchErr,
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
	}
	
	nodes, err := s.storage.GetNodes(filter)
	if err != nil {
		return nil, 0, err
	}
	
	return nodes, len(nodes), nil
}

// StatsHandler handles statistics page
func (s *Server) StatsHandler(w http.ResponseWriter, r *http.Request) {
	// Get the latest available nodelist date
	latestDate, err := s.storage.GetLatestStatsDate()
	if err != nil {
		data := struct {
			Title  string
			Stats  *database.NetworkStats
			Error  error
			NoData bool
		}{
			Title:  "Network Statistics",
			Stats:  nil,
			Error:  fmt.Errorf("Failed to find latest nodelist date: %v", err),
			NoData: true,
		}
		
		if err := s.templates["stats"].Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	
	// Get stats for the latest date
	stats, err := s.storage.GetStats(latestDate)
	
	data := struct {
		Title  string
		Stats  *database.NetworkStats
		Error  error
		NoData bool
	}{
		Title: "Network Statistics",
		Stats: stats,
		Error: err,
		NoData: stats == nil || stats.TotalNodes == 0,
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
			nodes, searchErr = s.storage.SearchNodesBySysop(sysopName, 50)
			count = len(nodes)
		}
	}
	
	data := struct {
		Title     string
		Nodes     []storage.NodeSummary
		Count     int
		Error     error
		SysopName string
	}{
		Title:     "Search by Sysop",
		Nodes:     nodes,
		Count:     count,
		Error:     searchErr,
		SysopName: sysopName,
	}
	
	if err := s.templates["sysop_search"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// NodeHistoryHandler handles node history view
func (s *Server) NodeHistoryHandler(w http.ResponseWriter, r *http.Request) {
	// Parse URL path: /node/{zone}/{net}/{node}
	path := strings.TrimPrefix(r.URL.Path, "/node/")
	parts := strings.Split(path, "/")
	
	if len(parts) < 3 {
		http.Error(w, "Invalid node address", http.StatusBadRequest)
		return
	}
	
	zone, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid zone", http.StatusBadRequest)
		return
	}
	
	net, err := strconv.Atoi(parts[1])
	if err != nil {
		http.Error(w, "Invalid net", http.StatusBadRequest)
		return
	}
	
	node, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "Invalid node", http.StatusBadRequest)
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
	
	// Parse filter options from query parameters
	filter := storage.ChangeFilter{
		IgnoreFlags:            r.URL.Query().Get("noflags") == "1",
		IgnorePhone:            r.URL.Query().Get("nophone") == "1",
		IgnoreSpeed:            r.URL.Query().Get("nospeed") == "1",
		IgnoreStatus:           r.URL.Query().Get("nostatus") == "1",
		IgnoreLocation:         r.URL.Query().Get("nolocation") == "1",
		IgnoreName:             r.URL.Query().Get("noname") == "1",
		IgnoreSysop:            r.URL.Query().Get("nosysop") == "1",
		IgnoreConnectivity:     r.URL.Query().Get("noconnectivity") == "1",
		IgnoreModemFlags:       r.URL.Query().Get("nomodemflags") == "1",
		IgnoreInternetProtocols: r.URL.Query().Get("nointernetprotocols") == "1",
		IgnoreInternetHostnames: r.URL.Query().Get("nointernethostnames") == "1",
		IgnoreInternetPorts:    r.URL.Query().Get("nointernetports") == "1",
		IgnoreInternetEmails:   r.URL.Query().Get("nointernetemails") == "1",
	}
	
	// Get node changes with filter applied
	changes, err := s.storage.GetNodeChanges(zone, net, node, filter)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving node changes: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Get first and last dates
	var firstDate, lastDate time.Time
	var currentlyActive bool
	
	if len(history) > 0 {
		firstDate = history[0].NodelistDate
		lastDate = history[len(history)-1].NodelistDate
		
		// Check if currently active (last entry within 30 days)
		daysSinceLastSeen := time.Since(lastDate).Hours() / 24
		currentlyActive = daysSinceLastSeen <= 30
	}
	
	data := struct {
		Title           string
		Address         string
		History         []database.Node
		Changes         []database.NodeChange
		Filter          storage.ChangeFilter
		FirstDate       time.Time
		LastDate        time.Time
		CurrentlyActive bool
	}{
		Title:           "Node History",
		Address:         fmt.Sprintf("%d:%d/%d", zone, net, node),
		History:         history,
		Changes:         changes,
		Filter:          filter,
		FirstDate:       firstDate,
		LastDate:        lastDate,
		CurrentlyActive: currentlyActive,
	}
	
	if err := s.templates["node_history"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// APIHelpHandler shows API documentation
func (s *Server) APIHelpHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "API Documentation",
	}
	
	if err := s.templates["api_help"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}