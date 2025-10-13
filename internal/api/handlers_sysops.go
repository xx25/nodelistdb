package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// SysopsHandler handles requests for listing sysops.
// GET /api/sysops?name=John&limit=50&offset=0
func (s *Server) SysopsHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	nameFilter := query.Get("name")

	// Parse pagination with custom limits for sysops
	limit, offset := parsePaginationParams(query, 50, 200)

	// Get unique sysops
	sysops, err := s.storage.SearchOps().GetUniqueSysops(nameFilter, limit, offset)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get sysops: %v", err), http.StatusInternalServerError)
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

	WriteJSONSuccess(w, response)
}

// SysopNodesHandler handles requests for getting nodes by sysop.
// GET /api/sysops/{name}/nodes?limit=100
func (s *Server) SysopNodesHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	// Extract sysop name from path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/sysops/"), "/")
	if len(pathParts) < 2 || pathParts[1] != "nodes" {
		WriteJSONError(w, "Invalid path format. Expected: /api/sysops/{name}/nodes", http.StatusBadRequest)
		return
	}

	sysopName := pathParts[0]
	if sysopName == "" {
		WriteJSONError(w, "Sysop name cannot be empty", http.StatusBadRequest)
		return
	}

	// URL decode the sysop name
	decodedName, err := url.PathUnescape(sysopName)
	if err != nil {
		WriteJSONError(w, "Invalid sysop name encoding", http.StatusBadRequest)
		return
	}

	// Parse limit with higher max for sysop nodes
	limit, _ := parsePaginationParams(r.URL.Query(), 100, 1000)

	// Get nodes for this sysop
	nodes, err := s.storage.SearchOps().GetNodesBySysop(decodedName, limit)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get nodes: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"sysop_name": decodedName,
		"nodes":      nodes,
		"count":      len(nodes),
		"limit":      limit,
	}

	WriteJSONSuccess(w, response)
}
