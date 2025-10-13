package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// SearchNodesHandler handles node search requests.
// GET /api/nodes?zone=1&net=234&node=56&date_from=2023-01-01&limit=100
func (s *Server) SearchNodesHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	// Parse query parameters and build filter
	filter, hasConstraint, err := parseNodeFilter(r)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Prevent overly broad searches that can cause memory exhaustion
	if !hasConstraint {
		WriteJSONError(w, "Search requires at least one specific constraint (zone, net, node, system_name, location, sysop_name, node_type, is_cm, or date range)", http.StatusBadRequest)
		return
	}

	// Search nodes
	nodes, err := s.storage.NodeOps().GetNodes(filter)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Search failed: %v", err), http.StatusInternalServerError)
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

	WriteJSONSuccess(w, response)
}

// GetNodeHandler handles individual node lookups.
// GET /api/nodes/{zone}/{net}/{node}
func (s *Server) GetNodeHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	// Parse path parameters
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
	if len(pathParts) < 3 {
		WriteJSONError(w, "Invalid path format. Expected: /api/nodes/{zone}/{net}/{node}", http.StatusBadRequest)
		return
	}

	zone, err := parsePathInt(pathParts[0], "zone")
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	net, err := parsePathInt(pathParts[1], "net")
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	node, err := parsePathInt(pathParts[2], "node")
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Search for the specific node
	filter := database.NodeFilter{
		Zone:  &zone,
		Net:   &net,
		Node:  &node,
		Limit: 1, // Get only the most recent version
	}

	nodes, err := s.storage.NodeOps().GetNodes(filter)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Node lookup failed: %v", err), http.StatusInternalServerError)
		return
	}

	if len(nodes) == 0 {
		WriteJSONError(w, "Node not found", http.StatusNotFound)
		return
	}

	// Return only the current/latest node data
	WriteJSONSuccess(w, nodes[0])
}

// GetNodeHistoryHandler returns the complete history of a node.
// GET /api/nodes/{zone}/{net}/{node}/history
func (s *Server) GetNodeHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	zone, net, node, err := parseNodeAddress(r.URL.Path, "/api/nodes/")
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get node history
	history, err := s.storage.NodeOps().GetNodeHistory(zone, net, node)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get node history: %v", err), http.StatusInternalServerError)
		return
	}

	if len(history) == 0 {
		WriteJSONError(w, "Node not found", http.StatusNotFound)
		return
	}

	// Get date range
	// Note: Errors from GetNodeDateRange are not critical - if it fails,
	// firstDate and lastDate will be zero values which is acceptable.
	// The history data itself is sufficient for the response.
	firstDate, lastDate, err := s.storage.NodeOps().GetNodeDateRange(zone, net, node)
	if err != nil {
		// Date range query failed, but we still have history data
		// firstDate and lastDate will be zero values (time.Time{})
	}

	response := map[string]interface{}{
		"address":    fmt.Sprintf("%d:%d/%d", zone, net, node),
		"history":    history,
		"count":      len(history),
		"first_date": firstDate,
		"last_date":  lastDate,
	}

	WriteJSONSuccess(w, response)
}

// GetNodeChangesHandler returns detected changes for a node.
// GET /api/nodes/{zone}/{net}/{node}/changes
func (s *Server) GetNodeChangesHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	zone, net, node, err := parseNodeAddress(r.URL.Path, "/api/nodes/")
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get all node changes without filtering
	changes, err := s.storage.SearchOps().GetNodeChanges(zone, net, node)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get node changes: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"address": fmt.Sprintf("%d:%d/%d", zone, net, node),
		"changes": changes,
		"count":   len(changes),
	}

	WriteJSONSuccess(w, response)
}

// GetNodeTimelineHandler returns timeline data for visualization.
// GET /api/nodes/{zone}/{net}/{node}/timeline
func (s *Server) GetNodeTimelineHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	zone, net, node, err := parseNodeAddress(r.URL.Path, "/api/nodes/")
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get node history
	history, err := s.storage.NodeOps().GetNodeHistory(zone, net, node)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get node history: %v", err), http.StatusInternalServerError)
		return
	}

	if len(history) == 0 {
		WriteJSONError(w, "Node not found", http.StatusNotFound)
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

	WriteJSONSuccess(w, response)
}

// parseNodeAddress extracts zone, net, node from a URL path.
func parseNodeAddress(path, prefix string) (zone, net, node int, err error) {
	pathParts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(pathParts) < 3 {
		return 0, 0, 0, fmt.Errorf("invalid path format")
	}

	zone, err = parsePathInt(pathParts[0], "zone")
	if err != nil {
		return 0, 0, 0, err
	}

	net, err = parsePathInt(pathParts[1], "net")
	if err != nil {
		return 0, 0, 0, err
	}

	node, err = parsePathInt(pathParts[2], "node")
	if err != nil {
		return 0, 0, 0, err
	}

	return zone, net, node, nil
}
