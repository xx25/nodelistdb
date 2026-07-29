package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// resolveNodeDomain picks the FTN network for a node endpoint, along with the
// full list of networks the address exists in for available_domains.
func (s *Server) resolveNodeDomain(r *http.Request, zone, net, node int) (string, []string) {
	domains, err := s.storage.GetNodeDomains(r.Context(), zone, net, node)
	if err != nil {
		domains = nil
	}
	return preferDomain(explicitDomain(r), domains), domains
}

// SearchNodesHandler handles node search requests.
// GET /api/nodes?zone=1&net=234&node=56&date_from=2023-01-01&limit=100
func (s *Server) SearchNodesHandler(w http.ResponseWriter, r *http.Request) {
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
	nodes, err := s.storage.GetNodes(r.Context(), filter)
	if err != nil {
		writeStorageErrorf(w, "Search failed", err)
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
	zone, net, node, _, ok := parse4DPathParams(w, r, false)
	if !ok {
		return
	}

	// Search for the specific node within the resolved network
	domain, availableDomains := s.resolveNodeDomain(r, zone, net, node)
	if len(availableDomains) > 1 {
		// The body stays a bare node object for backward compatibility; the
		// header tells clients the address exists in several networks
		w.Header().Set("X-Available-Domains", strings.Join(availableDomains, ","))
	}
	filter := database.NodeFilter{
		Zone:   &zone,
		Net:    &net,
		Node:   &node,
		Domain: &domain,
		Limit:  1, // Get only the most recent version
	}

	nodes, err := s.storage.GetNodes(r.Context(), filter)
	if err != nil {
		writeStorageErrorf(w, "Node lookup failed", err)
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
	zone, net, node, _, ok := parse4DPathParams(w, r, false)
	if !ok {
		return
	}

	// Get node history within the resolved network
	domain, availableDomains := s.resolveNodeDomain(r, zone, net, node)
	history, err := s.storage.GetNodeHistory(r.Context(), zone, net, node, domain)
	if err != nil {
		writeStorageErrorf(w, "Failed to get node history", err)
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
	firstDate, lastDate, _ := s.storage.GetNodeDateRange(r.Context(), zone, net, node, domain)

	response := addressEnvelope(zone, net, node, -1, domain, availableDomains)
	response["history"] = history
	response["count"] = len(history)
	response["first_date"] = firstDate
	response["last_date"] = lastDate

	WriteJSONSuccess(w, response)
}

// GetNodeChangesHandler returns detected changes for a node.
// GET /api/nodes/{zone}/{net}/{node}/changes
func (s *Server) GetNodeChangesHandler(w http.ResponseWriter, r *http.Request) {
	zone, net, node, _, ok := parse4DPathParams(w, r, false)
	if !ok {
		return
	}

	// Get all node changes within the resolved network
	domain, availableDomains := s.resolveNodeDomain(r, zone, net, node)
	changes, err := s.storage.GetNodeChanges(r.Context(), zone, net, node, domain)
	if err != nil {
		writeStorageErrorf(w, "Failed to get node changes", err)
		return
	}

	response := addressEnvelope(zone, net, node, -1, domain, availableDomains)
	response["changes"] = changes
	response["count"] = len(changes)

	WriteJSONSuccess(w, response)
}

// GetNodeTimelineHandler returns timeline data for visualization.
// GET /api/nodes/{zone}/{net}/{node}/timeline
func (s *Server) GetNodeTimelineHandler(w http.ResponseWriter, r *http.Request) {
	zone, net, node, _, ok := parse4DPathParams(w, r, false)
	if !ok {
		return
	}

	// Get node history within the resolved network
	domain, availableDomains := s.resolveNodeDomain(r, zone, net, node)
	history, err := s.storage.GetNodeHistory(r.Context(), zone, net, node, domain)
	if err != nil {
		writeStorageErrorf(w, "Failed to get node history", err)
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

	response := addressEnvelope(zone, net, node, -1, domain, availableDomains)
	response["timeline"] = timeline
	response["count"] = len(timeline)

	WriteJSONSuccess(w, response)
}

// GetPSTNNodesHandler returns all nodes with valid phone numbers from the latest nodelist.
// GET /api/nodes/pstn?zone=2&limit=5000
func (s *Server) GetPSTNNodesHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse optional zone filter
	zone := 0
	if zoneStr := query.Get("zone"); zoneStr != "" {
		parsed, err := strconv.Atoi(zoneStr)
		if err != nil || parsed < 0 {
			WriteJSONError(w, "Invalid zone parameter", http.StatusBadRequest)
			return
		}
		zone = parsed
	}

	// Parse limit (default 5000, max 10000)
	limit := 5000
	if limitStr := query.Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 {
			WriteJSONError(w, "Invalid limit parameter", http.StatusBadRequest)
			return
		}
		if parsed > 10000 {
			parsed = 10000
		}
		limit = parsed
	}

	// queryDomain defaults to fidonet, preserving this endpoint's original
	// fidonet-only behavior for the modem-test CLI
	nodes, err := s.storage.GetPSTNNodes(r.Context(), limit, zone, domainOrDefault(r))
	if err != nil {
		writeStorageErrorf(w, "Failed to fetch PSTN nodes", err)
		return
	}

	response := map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	}

	WriteJSONSuccess(w, response)
}

// GetRecentModemSuccessPhonesHandler returns phone numbers that were successfully tested via modem
// within a recent time window.
// GET /api/nodes/pstn/recent-success?days=7
func (s *Server) GetRecentModemSuccessPhonesHandler(w http.ResponseWriter, r *http.Request) {
	days := 7
	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		parsed, err := strconv.Atoi(daysStr)
		if err != nil || parsed < 1 || parsed > 90 {
			WriteJSONError(w, "Invalid days parameter (must be 1-90)", http.StatusBadRequest)
			return
		}
		days = parsed
	}

	phones, err := s.storage.GetRecentModemSuccessPhones(r.Context(), days)
	if err != nil {
		writeStorageErrorf(w, "Failed to fetch recent modem success phones", err)
		return
	}

	if phones == nil {
		phones = []string{}
	}

	response := map[string]interface{}{
		"phones": phones,
		"count":  len(phones),
	}

	WriteJSONSuccess(w, response)
}
