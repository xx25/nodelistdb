package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nodelistdb/internal/database"
)

// resolvePointDomain picks the FTN network for a point endpoint. It mirrors
// resolveNodeDomain but consults the points table itself: resolving via the
// boss node could 404 a point that exists only in a network the node-level
// heuristic does not prefer. A nil point resolves at boss level.
func (s *Server) resolvePointDomain(r *http.Request, zone, net, node int, point *int) (string, []string) {
	domains, err := s.storage.GetPointDomains(zone, net, node, point)
	if err != nil || domains == nil {
		// Non-nil so JSON bodies carry "available_domains": [] rather than null
		domains = []string{}
	}

	if d := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain"))); d != "" {
		return d, domains
	}

	switch len(domains) {
	case 0:
		return database.DefaultDomain, domains
	case 1:
		return domains[0], domains
	default:
		for _, d := range domains {
			if d == database.DefaultDomain {
				return d, domains
			}
		}
		return domains[0], domains
	}
}

// parse4DPathParams extracts zone/net/node (and optionally point) from Chi
// URL parameters, writing the error response itself on failure.
func parse4DPathParams(w http.ResponseWriter, r *http.Request, withPoint bool) (zone, net, node, point int, ok bool) {
	for _, p := range []struct {
		name string
		dest *int
	}{
		{"zone", &zone}, {"net", &net}, {"node", &node},
	} {
		v, err := strconv.Atoi(chi.URLParam(r, p.name))
		if err != nil {
			WriteJSONError(w, fmt.Sprintf("Invalid %s parameter", p.name), http.StatusBadRequest)
			return 0, 0, 0, 0, false
		}
		*p.dest = v
	}
	if withPoint {
		v, err := strconv.Atoi(chi.URLParam(r, "point"))
		if err != nil {
			WriteJSONError(w, "Invalid point parameter", http.StatusBadRequest)
			return 0, 0, 0, 0, false
		}
		point = v
	}
	return zone, net, node, point, true
}

// SearchPointsHandler handles point search requests.
// GET /api/points?zone=2&net=5001&sysop_name=...&latest_only=true&limit=100
func (s *Server) SearchPointsHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	filter, hasConstraint, err := parsePointFilter(r)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !hasConstraint {
		WriteJSONError(w, "Search requires at least one specific constraint (zone, net, node, point, system_name, location, sysop_name, list_source, or date range)", http.StatusBadRequest)
		return
	}

	points, err := s.storage.SearchPoints(filter)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Search failed: %v", err), http.StatusInternalServerError)
		return
	}
	if points == nil {
		points = []database.Point{}
	}

	response := map[string]interface{}{
		"points": points,
		"count":  len(points),
		"filter": map[string]interface{}{
			"zone":        filter.Zone,
			"net":         filter.Net,
			"node":        filter.Node,
			"point":       filter.PointNum,
			"domain":      filter.Domain,
			"list_source": filter.ListSource,
			"system_name": filter.SystemName,
			"location":    filter.Location,
			"sysop_name":  filter.SysopName,
			"date_from":   filter.DateFrom,
			"date_to":     filter.DateTo,
			"latest_only": filter.LatestOnly,
			"limit":       filter.Limit,
			"offset":      filter.Offset,
		},
	}

	WriteJSONSuccess(w, response)
}

// GetNodePointsHandler returns the snapshot points under a boss node.
// GET /api/nodes/{zone}/{net}/{node}/points?date=2024-01-01&domain=fidonet
func (s *Server) GetNodePointsHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	zone, net, node, _, ok := parse4DPathParams(w, r, false)
	if !ok {
		return
	}

	var asOf *time.Time
	if dateStr := r.URL.Query().Get("date"); dateStr != "" {
		d, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			WriteJSONError(w, "Invalid date format. Use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		asOf = &d
	}

	domain, availableDomains := s.resolvePointDomain(r, zone, net, node, nil)
	points, err := s.storage.GetPointsByBoss(domain, zone, net, node, asOf)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get points: %v", err), http.StatusInternalServerError)
		return
	}
	if points == nil {
		points = []database.Point{}
	}

	response := map[string]interface{}{
		"address":           fmt.Sprintf("%d:%d/%d", zone, net, node),
		"domain":            domain,
		"available_domains": availableDomains,
		"points":            points,
		"count":             len(points),
	}
	if asOf != nil {
		response["as_of"] = asOf.Format("2006-01-02")
	}

	WriteJSONSuccess(w, response)
}

// GetPointHandler returns the current snapshot entry of one 4-D address.
// GET /api/points/{zone}/{net}/{node}/{point}
func (s *Server) GetPointHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	zone, net, node, point, ok := parse4DPathParams(w, r, true)
	if !ok {
		return
	}

	domain, availableDomains := s.resolvePointDomain(r, zone, net, node, &point)
	if len(availableDomains) > 1 {
		w.Header().Set("X-Available-Domains", strings.Join(availableDomains, ","))
	}

	latestOnly := true
	filter := database.PointFilter{
		Zone:       &zone,
		Net:        &net,
		Node:       &node,
		PointNum:   &point,
		Domain:     &domain,
		LatestOnly: &latestOnly,
		Limit:      1,
	}

	points, err := s.storage.SearchPoints(filter)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Point lookup failed: %v", err), http.StatusInternalServerError)
		return
	}

	if len(points) == 0 {
		WriteJSONError(w, "Point not found", http.StatusNotFound)
		return
	}

	WriteJSONSuccess(w, points[0])
}

// GetPointHistoryHandler returns every stored entry of one 4-D address across
// all pointlist sources and dates.
// GET /api/points/{zone}/{net}/{node}/{point}/history
func (s *Server) GetPointHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	zone, net, node, point, ok := parse4DPathParams(w, r, true)
	if !ok {
		return
	}

	domain, availableDomains := s.resolvePointDomain(r, zone, net, node, &point)
	history, err := s.storage.GetPointHistory(domain, zone, net, node, point)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get point history: %v", err), http.StatusInternalServerError)
		return
	}

	if len(history) == 0 {
		WriteJSONError(w, "Point not found", http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"address":           fmt.Sprintf("%d:%d/%d.%d", zone, net, node, point),
		"domain":            domain,
		"available_domains": availableDomains,
		"history":           history,
		"count":             len(history),
	}

	WriteJSONSuccess(w, response)
}

// PointlistDatesHandler lists imported pointlist files (issue dates per
// series), newest first.
// GET /api/pointlists/dates?domain=fidonet&source=z2
func (s *Server) PointlistDatesHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	source := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source")))
	files, err := s.storage.GetPointlistDates(queryDomain(r), source)
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get pointlist dates: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"files": files,
		"count": len(files),
	}

	WriteJSONSuccess(w, response)
}

// PointlistSourcesHandler summarizes the imported pointlist series.
// GET /api/pointlists/sources?domain=fidonet
func (s *Server) PointlistSourcesHandler(w http.ResponseWriter, r *http.Request) {
	if !CheckMethod(w, r, http.MethodGet) {
		return
	}

	sources, err := s.storage.GetPointlistSources(queryDomain(r))
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("Failed to get pointlist sources: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"sources": sources,
		"count":   len(sources),
	}

	WriteJSONSuccess(w, response)
}
