package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
	"github.com/nodelistdb/internal/storage"
)

// Server represents the API server
type Server struct {
	storage    *storage.Storage
	dbFilePath string
}

// New creates a new API server
func New(storage *storage.Storage) *Server {
	return &Server{
		storage: storage,
	}
}

// NewWithDBPath creates a new API server with database file path
func NewWithDBPath(storage *storage.Storage, dbFilePath string) *Server {
	return &Server{
		storage:    storage,
		dbFilePath: dbFilePath,
	}
}

// HealthHandler handles health check requests
func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status": "ok",
		"time":   time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// SearchNodesHandler handles node search requests
// GET /api/nodes?zone=1&net=234&node=56&date_from=2023-01-01&limit=100
func (s *Server) SearchNodesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	filter := database.NodeFilter{}
	query := r.URL.Query()

	// Track if we have any specific constraints to prevent overly broad searches
	hasSpecificConstraint := false

	if zone := query.Get("zone"); zone != "" {
		if z, err := strconv.Atoi(zone); err == nil {
			filter.Zone = &z
			hasSpecificConstraint = true
		}
	}

	if net := query.Get("net"); net != "" {
		if n, err := strconv.Atoi(net); err == nil {
			filter.Net = &n
			hasSpecificConstraint = true
		}
	}

	if node := query.Get("node"); node != "" {
		if n, err := strconv.Atoi(node); err == nil {
			filter.Node = &n
			hasSpecificConstraint = true
		}
	}

	if systemName := query.Get("system_name"); systemName != "" {
		// Prevent memory exhaustion from very short search strings
		if len(strings.TrimSpace(systemName)) < 2 {
			http.Error(w, "system_name must be at least 2 characters long", http.StatusBadRequest)
			return
		}
		filter.SystemName = &systemName
		hasSpecificConstraint = true
	}

	if location := query.Get("location"); location != "" {
		// Prevent memory exhaustion from very short search strings
		if len(strings.TrimSpace(location)) < 2 {
			http.Error(w, "location must be at least 2 characters long", http.StatusBadRequest)
			return
		}
		filter.Location = &location
		hasSpecificConstraint = true
	}

	if sysopName := query.Get("sysop_name"); sysopName != "" {
		// Prevent memory exhaustion from very short search strings
		if len(strings.TrimSpace(sysopName)) < 2 {
			http.Error(w, "sysop_name must be at least 2 characters long", http.StatusBadRequest)
			return
		}
		filter.SysopName = &sysopName
		hasSpecificConstraint = true
	}

	if nodeType := query.Get("node_type"); nodeType != "" {
		filter.NodeType = &nodeType
		hasSpecificConstraint = true
	}

	if isCM := query.Get("is_cm"); isCM != "" {
		cm := strings.ToLower(isCM) == "true"
		filter.IsCM = &cm
		hasSpecificConstraint = true
	}

	if dateFrom := query.Get("date_from"); dateFrom != "" {
		if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
			filter.DateFrom = &t
			hasSpecificConstraint = true
		}
	}

	if dateTo := query.Get("date_to"); dateTo != "" {
		if t, err := time.Parse("2006-01-02", dateTo); err == nil {
			filter.DateTo = &t
			hasSpecificConstraint = true
		}
	}

	// Latest only filter (default: false, includes historical data)
	if latestOnly := query.Get("latest_only"); latestOnly != "" {
		latest := strings.ToLower(latestOnly) == "true"
		filter.LatestOnly = &latest
	}

	// Prevent overly broad searches that can cause memory exhaustion
	if !hasSpecificConstraint {
		http.Error(w, "Search requires at least one specific constraint (zone, net, node, system_name, location, sysop_name, node_type, is_cm, or date range)", http.StatusBadRequest)
		return
	}

	// Pagination
	if limit := query.Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			// Cap maximum limit to prevent memory exhaustion
			if l > 500 {
				l = 500
			}
			filter.Limit = l
		}
	} else {
		filter.Limit = 100 // Default limit
	}

	if offset := query.Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			filter.Offset = o
		}
	}

	// Search nodes
	nodes, err := s.storage.GetNodes(filter)
	if err != nil {
		http.Error(w, fmt.Sprintf("Search failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Prepare response
	response := map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
		"filter": map[string]interface{}{
			"zone":        filter.Zone,
			"net":         filter.Net,
			"node":        filter.Node,
			"system_name": filter.SystemName,
			"location":    filter.Location,
			"node_type":   filter.NodeType,
			"is_cm":       filter.IsCM,
			"date_from":   filter.DateFrom,
			"date_to":     filter.DateTo,
			"latest_only": filter.LatestOnly,
			"limit":       filter.Limit,
			"offset":      filter.Offset,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetNodeHandler handles individual node lookups
// GET /api/nodes/{zone}/{net}/{node}
func (s *Server) GetNodeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path parameters
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid path format. Expected: /api/nodes/{zone}/{net}/{node}", http.StatusBadRequest)
		return
	}

	zone, err := strconv.Atoi(pathParts[0])
	if err != nil {
		http.Error(w, "Invalid zone number", http.StatusBadRequest)
		return
	}

	net, err := strconv.Atoi(pathParts[1])
	if err != nil {
		http.Error(w, "Invalid net number", http.StatusBadRequest)
		return
	}

	node, err := strconv.Atoi(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid node number", http.StatusBadRequest)
		return
	}

	// Search for the specific node
	filter := database.NodeFilter{
		Zone:  &zone,
		Net:   &net,
		Node:  &node,
		Limit: 1, // Get only the most recent version
	}

	nodes, err := s.storage.GetNodes(filter)
	if err != nil {
		http.Error(w, fmt.Sprintf("Node lookup failed: %v", err), http.StatusInternalServerError)
		return
	}

	if len(nodes) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Return only the current/latest node data
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes[0])
}

// StatsHandler handles statistics requests
// GET /api/stats?date=2023-01-01
func (s *Server) StatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse date parameter
	dateStr := r.URL.Query().Get("date")
	var date time.Time
	var err error
	var actualDate time.Time

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			http.Error(w, "Invalid date format. Use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		// Find the nearest available date
		actualDate, err = s.storage.GetNearestAvailableDate(date)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to find available date: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Default to latest available date
		actualDate, err = s.storage.GetLatestStatsDate()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get latest date: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Get statistics for the actual date
	stats, err := s.storage.GetStats(actualDate)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get statistics: %v", err), http.StatusInternalServerError)
		return
	}

	// Include information about date selection in the response
	response := map[string]interface{}{
		"stats":          stats,
		"requested_date": dateStr,
		"actual_date":    actualDate.Format("2006-01-02"),
		"date_adjusted":  dateStr != "" && actualDate.Format("2006-01-02") != dateStr,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetNodeHistoryHandler returns the complete history of a node
// GET /api/nodes/{zone}/{net}/{node}/history
func (s *Server) GetNodeHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path parameters
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	zone, err := strconv.Atoi(pathParts[0])
	if err != nil {
		http.Error(w, "Invalid zone number", http.StatusBadRequest)
		return
	}

	net, err := strconv.Atoi(pathParts[1])
	if err != nil {
		http.Error(w, "Invalid net number", http.StatusBadRequest)
		return
	}

	node, err := strconv.Atoi(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid node number", http.StatusBadRequest)
		return
	}

	// Get node history
	history, err := s.storage.GetNodeHistory(zone, net, node)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get node history: %v", err), http.StatusInternalServerError)
		return
	}

	if len(history) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Get date range
	firstDate, lastDate, _ := s.storage.GetNodeDateRange(zone, net, node)

	response := map[string]interface{}{
		"address":    fmt.Sprintf("%d:%d/%d", zone, net, node),
		"history":    history,
		"count":      len(history),
		"first_date": firstDate,
		"last_date":  lastDate,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetNodeChangesHandler returns detected changes for a node
// GET /api/nodes/{zone}/{net}/{node}/changes?noflags=1&nophone=1
func (s *Server) GetNodeChangesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path parameters
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	zone, err := strconv.Atoi(pathParts[0])
	if err != nil {
		http.Error(w, "Invalid zone number", http.StatusBadRequest)
		return
	}

	net, err := strconv.Atoi(pathParts[1])
	if err != nil {
		http.Error(w, "Invalid net number", http.StatusBadRequest)
		return
	}

	node, err := strconv.Atoi(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid node number", http.StatusBadRequest)
		return
	}

	// Parse filter options
	query := r.URL.Query()
	filter := storage.ChangeFilter{}

	// Check for new exclude parameter format
	if excludeStr := query.Get("exclude"); excludeStr != "" {
		// Parse comma-separated list of fields to exclude
		excludeFields := strings.Split(excludeStr, ",")
		for _, field := range excludeFields {
			field = strings.TrimSpace(strings.ToLower(field))
			switch field {
			case "flags":
				filter.IgnoreFlags = true
			case "phone":
				filter.IgnorePhone = true
			case "speed":
				filter.IgnoreSpeed = true
			case "status":
				filter.IgnoreStatus = true
			case "location":
				filter.IgnoreLocation = true
			case "name":
				filter.IgnoreName = true
			case "sysop":
				filter.IgnoreSysop = true
			case "connectivity":
				filter.IgnoreConnectivity = true
			case "internetprotocols", "internet_protocols", "internethostnames", "internet_hostnames", "internetports", "internet_ports", "internetemails", "internet_emails":
				// These fields are now handled through internet_config JSON - ignore them
				continue
			case "modemflags", "modem_flags":
				filter.IgnoreModemFlags = true
			}
		}
	} else {
		// Maintain backward compatibility with old format
		filter.IgnoreFlags = query.Get("noflags") == "1"
		filter.IgnorePhone = query.Get("nophone") == "1"
		filter.IgnoreSpeed = query.Get("nospeed") == "1"
		filter.IgnoreStatus = query.Get("nostatus") == "1"
		filter.IgnoreLocation = query.Get("nolocation") == "1"
		filter.IgnoreName = query.Get("noname") == "1"
		filter.IgnoreSysop = query.Get("nosysop") == "1"
		filter.IgnoreConnectivity = query.Get("noconnectivity") == "1"
		// Internet array fields are no longer available - these options are ignored
		filter.IgnoreModemFlags = query.Get("nomodemflags") == "1"
	}

	// Get node changes
	changes, err := s.storage.GetNodeChanges(zone, net, node, filter)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get node changes: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"address": fmt.Sprintf("%d:%d/%d", zone, net, node),
		"changes": changes,
		"count":   len(changes),
		"filter":  filter,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetNodeTimelineHandler returns timeline data for visualization
// GET /api/nodes/{zone}/{net}/{node}/timeline
func (s *Server) GetNodeTimelineHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path parameters
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	zone, err := strconv.Atoi(pathParts[0])
	if err != nil {
		http.Error(w, "Invalid zone number", http.StatusBadRequest)
		return
	}

	net, err := strconv.Atoi(pathParts[1])
	if err != nil {
		http.Error(w, "Invalid net number", http.StatusBadRequest)
		return
	}

	node, err := strconv.Atoi(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid node number", http.StatusBadRequest)
		return
	}

	// Get node history
	history, err := s.storage.GetNodeHistory(zone, net, node)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get node history: %v", err), http.StatusInternalServerError)
		return
	}

	if len(history) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Build timeline data
	var timeline []map[string]interface{}
	for i, node := range history {
		event := map[string]interface{}{
			"date":       node.NodelistDate,
			"day_number": node.DayNumber,
			"type":       "active",
			"data":       node,
		}

		// Check for gaps to detect removal periods
		if i < len(history)-1 {
			nextNode := history[i+1]
			if !node.NodelistDate.AddDate(0, 0, 14).After(nextNode.NodelistDate) {
				// Gap detected - node was removed
				timeline = append(timeline, event)
				timeline = append(timeline, map[string]interface{}{
					"date":       node.NodelistDate.AddDate(0, 0, 7),
					"day_number": node.DayNumber + 7,
					"type":       "removed",
					"duration":   nextNode.NodelistDate.Sub(node.NodelistDate),
				})
				continue
			}
		}
		timeline = append(timeline, event)
	}

	response := map[string]interface{}{
		"address":  fmt.Sprintf("%d:%d/%d", zone, net, node),
		"timeline": timeline,
		"count":    len(timeline),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetAvailableDatesHandler returns all available dates for stats
// GET /api/stats/dates
func (s *Server) GetAvailableDatesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dates, err := s.storage.GetAvailableDates()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get available dates: %v", err), http.StatusInternalServerError)
		return
	}

	// Format dates as strings for JSON response
	formattedDates := make([]string, len(dates))
	for i, date := range dates {
		formattedDates[i] = date.Format("2006-01-02")
	}

	response := map[string]interface{}{
		"dates": formattedDates,
		"count": len(formattedDates),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// SysopsHandler handles requests for listing sysops
// GET /api/sysops?name=John&limit=50&offset=0
func (s *Server) SysopsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	nameFilter := query.Get("name")

	// Parse limit
	limit := 50
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	// Parse offset
	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get unique sysops
	sysops, err := s.storage.GetUniqueSysops(nameFilter, limit, offset)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get sysops: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"sysops": sysops,
		"count":  len(sysops),
		"filter": map[string]interface{}{
			"name":   nameFilter,
			"limit":  limit,
			"offset": offset,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// SysopNodesHandler handles requests for getting nodes by sysop
// GET /api/sysops/{name}/nodes?limit=100
func (s *Server) SysopNodesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract sysop name from path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/sysops/"), "/")
	if len(pathParts) < 2 || pathParts[1] != "nodes" {
		http.Error(w, "Invalid path format. Expected: /api/sysops/{name}/nodes", http.StatusBadRequest)
		return
	}

	sysopName := pathParts[0]
	if sysopName == "" {
		http.Error(w, "Sysop name cannot be empty", http.StatusBadRequest)
		return
	}

	// URL decode the sysop name
	decodedName, err := url.PathUnescape(sysopName)
	if err != nil {
		http.Error(w, "Invalid sysop name encoding", http.StatusBadRequest)
		return
	}

	// Get limit
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	// Get nodes for this sysop
	nodes, err := s.storage.GetNodesBySysop(decodedName, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get nodes: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"sysop_name": decodedName,
		"nodes":      nodes,
		"count":      len(nodes),
		"limit":      limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// FlagsDocumentationHandler returns flag descriptions and categories
// GET /api/flags
func (s *Server) FlagsDocumentationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get flag filter from query parameters
	category := r.URL.Query().Get("category")
	specificFlag := r.URL.Query().Get("flag")

	flagDescriptions := flags.GetFlagDescriptions()

	// If a specific flag is requested, check if it's a T-flag that needs dynamic generation
	if specificFlag != "" && len(specificFlag) == 3 && specificFlag[0] == 'T' {
		if _, exists := flagDescriptions[specificFlag]; !exists {
			// Try to generate T-flag description dynamically
			if info, ok := flags.GetTFlagInfo(specificFlag); ok {
				flagDescriptions[specificFlag] = info
			}
		}
	}

	// Filter by category if specified
	if category != "" {
		filteredFlags := make(map[string]flags.FlagInfo)
		for flag, info := range flagDescriptions {
			if info.Category == category {
				filteredFlags[flag] = info
			}
		}
		flagDescriptions = filteredFlags
	}

	// Group flags by category
	categories := make(map[string][]map[string]interface{})
	for flag, info := range flagDescriptions {
		if categories[info.Category] == nil {
			categories[info.Category] = []map[string]interface{}{}
		}

		flagData := map[string]interface{}{
			"flag":        flag,
			"has_value":   info.HasValue,
			"description": info.Description,
		}
		categories[info.Category] = append(categories[info.Category], flagData)
	}

	response := map[string]interface{}{
		"flags":      flagDescriptions,
		"categories": categories,
		"count":      len(flagDescriptions),
		"filter": map[string]interface{}{
			"category": category,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DownloadDatabaseHandler serves the DuckDB database file for download
// GET /api/download/database
func (s *Server) DownloadDatabaseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if database file path is set
	if s.dbFilePath == "" {
		http.Error(w, "Database file path not configured", http.StatusInternalServerError)
		return
	}

	// Open the database file
	file, err := os.Open(s.dbFilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open database file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get file info: %v", err), http.StatusInternalServerError)
		return
	}

	// Set headers for file download
	filename := filepath.Base(s.dbFilePath)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Last-Modified", fileInfo.ModTime().UTC().Format(http.TimeFormat))

	// Serve the file
	_, err = io.Copy(w, file)
	if err != nil {
		// Log error but don't send response as headers are already sent
		log.Printf("Error serving database file: %v", err)
	}
}

// SetupRoutes sets up HTTP routes for the API server
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	// API routes
	mux.HandleFunc("/api/health", s.HealthHandler)
	mux.HandleFunc("/api/nodes", s.SearchNodesHandler)
	mux.HandleFunc("/api/stats", s.StatsHandler)
	mux.HandleFunc("/api/stats/dates", s.GetAvailableDatesHandler)
	mux.HandleFunc("/api/flags", s.FlagsDocumentationHandler)
	mux.HandleFunc("/api/sysops", s.SysopsHandler)
	mux.HandleFunc("/api/download/database", s.DownloadDatabaseHandler)
	mux.HandleFunc("/api/nodelist/latest", s.LatestNodelistAPIHandler)

	// OpenAPI documentation routes
	mux.HandleFunc("/api/openapi.yaml", s.OpenAPISpecHandler)
	mux.HandleFunc("/api/docs", s.SwaggerUIHandler)

	// Sysop-specific routes
	mux.HandleFunc("/api/sysops/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		pathParts := strings.Split(strings.TrimPrefix(path, "/api/sysops/"), "/")

		// Check if this is /api/sysops/{name}/nodes pattern
		if len(pathParts) >= 2 && pathParts[1] == "nodes" {
			s.SysopNodesHandler(w, r)
		} else {
			// For /api/sysops or /api/sysops/, redirect to the base handler
			s.SysopsHandler(w, r)
		}
	})

	// Node lookup with path parameters
	mux.HandleFunc("/api/nodes/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		pathParts := strings.Split(strings.TrimPrefix(path, "/api/nodes/"), "/")

		// Route to appropriate handler based on path structure
		if len(pathParts) >= 4 && pathParts[3] == "history" {
			// /api/nodes/{zone}/{net}/{node}/history
			s.GetNodeHistoryHandler(w, r)
		} else if len(pathParts) >= 4 && pathParts[3] == "changes" {
			// /api/nodes/{zone}/{net}/{node}/changes
			s.GetNodeChangesHandler(w, r)
		} else if len(pathParts) >= 4 && pathParts[3] == "timeline" {
			// /api/nodes/{zone}/{net}/{node}/timeline
			s.GetNodeTimelineHandler(w, r)
		} else if strings.Count(path, "/") >= 5 {
			// /api/nodes/{zone}/{net}/{node}
			s.GetNodeHandler(w, r)
		} else {
			s.SearchNodesHandler(w, r)
		}
	})
}

// OpenAPISpecHandler serves the OpenAPI specification
func (s *Server) OpenAPISpecHandler(w http.ResponseWriter, r *http.Request) {
	// Set appropriate headers
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle CORS preflight
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Serve the embedded specification
	w.WriteHeader(http.StatusOK)
	w.Write(OpenAPISpec)
}

// SwaggerUIHandler serves the Swagger UI interface
func (s *Server) SwaggerUIHandler(w http.ResponseWriter, r *http.Request) {
	// Get the base URL for the API spec
	scheme := "http"

	// Check for HTTPS in multiple ways to handle reverse proxies
	if r.TLS != nil {
		scheme = "https"
	} else if r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	} else if r.Header.Get("X-Forwarded-Ssl") == "on" {
		scheme = "https"
	} else if r.Header.Get("X-Url-Scheme") == "https" {
		scheme = "https"
	} else if r.Header.Get("Forwarded") != "" {
		// Parse RFC 7239 Forwarded header
		forwarded := r.Header.Get("Forwarded")
		if strings.Contains(strings.ToLower(forwarded), "proto=https") {
			scheme = "https"
		}
	}

	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}

	specURL := fmt.Sprintf("%s://%s/api/openapi.yaml", scheme, host)

	// Serve a simple Swagger UI HTML page
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>NodelistDB API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.10.3/swagger-ui.css" />
    <style>
        html {
            box-sizing: border-box;
            overflow: -moz-scrollbars-vertical;
            overflow-y: scroll;
        }
        *, *:before, *:after {
            box-sizing: inherit;
        }
        body {
            margin:0;
            background: #fafafa;
        }
        .swagger-ui .topbar {
            background-color: #2c3e50;
        }
        .swagger-ui .topbar .download-url-wrapper .download-url-button {
            background-color: #34495e;
        }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.10.3/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.10.3/swagger-ui-standalone-preset.js"></script>
    <script>
    window.onload = function() {
        const ui = SwaggerUIBundle({
            url: '%s',
            dom_id: '#swagger-ui',
            deepLinking: true,
            presets: [
                SwaggerUIBundle.presets.apis,
                SwaggerUIStandalonePreset
            ],
            plugins: [
                SwaggerUIBundle.plugins.DownloadUrl
            ],
            layout: "StandaloneLayout",
            validatorUrl: null,
            docExpansion: "list",
            operationsSorter: "alpha",
            tagsSorter: "alpha",
            tryItOutEnabled: true,
            filter: true,
            supportedSubmitMethods: ["get", "post", "put", "delete", "patch"],
            onComplete: function() {
                console.log("NodelistDB API documentation loaded");
            }
        });
    };
    </script>
</body>
</html>`, specURL)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// LatestNodelistAPIHandler returns the latest nodelist file
// GET /api/nodelist/latest
func (s *Server) LatestNodelistAPIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Find the latest nodelist file
	latest, err := findLatestNodelistAPI()
	if err != nil {
		http.Error(w, "No nodelist files found", http.StatusNotFound)
		return
	}

	// Open the file
	file, err := os.Open(latest.Path)
	if err != nil {
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Check if file is gzipped
	if latest.IsCompressed {
		// For API, we'll return metadata about the file instead of decompressing
		response := map[string]interface{}{
			"filename":     strings.TrimSuffix(latest.Name, ".gz"),
			"year":         latest.Year,
			"day_number":   latest.DayNumber,
			"date":         latest.Date.Format("2006-01-02"),
			"compressed":   true,
			"download_url": fmt.Sprintf("/download/nodelist/%s/%s", latest.Year, latest.Name),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	} else {
		// Return the file content directly
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", latest.Name))
		io.Copy(w, file)
	}
}

// Helper function to find latest nodelist for API
func findLatestNodelistAPI() (*NodelistFileAPI, error) {
	basePath := getNodelistPathAPI()

	// Read year directories
	yearDirs, err := os.ReadDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read nodelist directory: %v", err)
	}

	var latestFile *NodelistFileAPI
	var latestYear int
	var latestDay int

	for _, yearDir := range yearDirs {
		if !yearDir.IsDir() {
			continue
		}

		yearName := yearDir.Name()
		if len(yearName) != 4 {
			continue
		}
		yearInt, err := strconv.Atoi(yearName)
		if err != nil {
			continue
		}

		yearPath := filepath.Join(basePath, yearName)
		files, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			name := file.Name()
			if !strings.HasPrefix(strings.ToLower(name), "nodelist.") {
				continue
			}

			parts := strings.Split(name, ".")
			if len(parts) < 2 {
				continue
			}

			dayStr := parts[1]
			if len(dayStr) != 3 {
				continue
			}
			dayNum, err := strconv.Atoi(dayStr)
			if err != nil {
				continue
			}

			// Check if this is the latest file
			if yearInt > latestYear || (yearInt == latestYear && dayNum > latestDay) {
				latestYear = yearInt
				latestDay = dayNum

				info, _ := file.Info()
				date := time.Date(yearInt, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)

				latestFile = &NodelistFileAPI{
					Name:         name,
					Year:         yearName,
					DayNumber:    dayNum,
					Date:         date,
					Path:         filepath.Join(yearPath, name),
					Size:         info.Size(),
					IsCompressed: strings.HasSuffix(strings.ToLower(name), ".gz"),
				}
			}
		}
	}

	if latestFile == nil {
		return nil, fmt.Errorf("no nodelist files found")
	}

	return latestFile, nil
}

// NodelistFileAPI represents a nodelist file for API responses
type NodelistFileAPI struct {
	Name         string
	Year         string
	DayNumber    int
	Date         time.Time
	Path         string
	Size         int64
	IsCompressed bool
}

// getNodelistPathAPI returns the base path for nodelist files
func getNodelistPathAPI() string {
	if path := os.Getenv("NODELIST_PATH"); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/home/dp/nodelists"
	}
	return filepath.Join(home, "nodelists")
}

// CORS middleware
func (s *Server) EnableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}
