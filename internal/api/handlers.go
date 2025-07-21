package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nodelistdb/internal/database"
	"nodelistdb/internal/storage"
)

// Server represents the API server
type Server struct {
	storage *storage.Storage
}

// New creates a new API server
func New(storage *storage.Storage) *Server {
	return &Server{
		storage: storage,
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

	if zone := query.Get("zone"); zone != "" {
		if z, err := strconv.Atoi(zone); err == nil {
			filter.Zone = &z
		}
	}

	if net := query.Get("net"); net != "" {
		if n, err := strconv.Atoi(net); err == nil {
			filter.Net = &n
		}
	}

	if node := query.Get("node"); node != "" {
		if n, err := strconv.Atoi(node); err == nil {
			filter.Node = &n
		}
	}

	if systemName := query.Get("system_name"); systemName != "" {
		filter.SystemName = &systemName
	}

	if location := query.Get("location"); location != "" {
		filter.Location = &location
	}

	if nodeType := query.Get("node_type"); nodeType != "" {
		filter.NodeType = &nodeType
	}

	if isActive := query.Get("is_active"); isActive != "" {
		if active := strings.ToLower(isActive) == "true"; active {
			filter.IsActive = &active
		} else {
			inactive := false
			filter.IsActive = &inactive
		}
	}

	if isCM := query.Get("is_cm"); isCM != "" {
		cm := strings.ToLower(isCM) == "true"
		filter.IsCM = &cm
	}

	if dateFrom := query.Get("date_from"); dateFrom != "" {
		if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
			filter.DateFrom = &t
		}
	}

	if dateTo := query.Get("date_to"); dateTo != "" {
		if t, err := time.Parse("2006-01-02", dateTo); err == nil {
			filter.DateTo = &t
		}
	}

	// Pagination
	if limit := query.Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
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
			"is_active":   filter.IsActive,
			"is_cm":       filter.IsCM,
			"date_from":   filter.DateFrom,
			"date_to":     filter.DateTo,
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
		Limit: 10, // Get recent versions
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

	response := map[string]interface{}{
		"node":     nodes[0], // Most recent version
		"history":  nodes,    // All versions
		"versions": len(nodes),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			http.Error(w, "Invalid date format. Use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
	} else {
		// Default to today
		date = time.Now().Truncate(24 * time.Hour)
	}

	// Get statistics
	stats, err := s.storage.GetStats(date)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get statistics: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// SetupRoutes sets up HTTP routes for the API server
func (s *Server) SetupRoutes(mux *http.ServeMux) {
	// API routes
	mux.HandleFunc("/api/health", s.HealthHandler)
	mux.HandleFunc("/api/nodes", s.SearchNodesHandler)
	mux.HandleFunc("/api/stats", s.StatsHandler)
	
	// Node lookup with path parameters
	mux.HandleFunc("/api/nodes/", func(w http.ResponseWriter, r *http.Request) {
		// Handle both search and specific node lookup
		if strings.Count(r.URL.Path, "/") >= 5 { // /api/nodes/{zone}/{net}/{node}
			s.GetNodeHandler(w, r)
		} else {
			s.SearchNodesHandler(w, r)
		}
	})
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