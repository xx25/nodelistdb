package api

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/nodelistdb/internal/storage"
)

// The reachability endpoints publish the testdaemon's raw results: what the
// web pages under /reachability render, as JSON. They read the same storage
// methods, so the two cannot disagree.

// maxReachabilityDays bounds every per-node look-back window here; the web
// pages use the same ceiling.
const maxReachabilityDays = 365

// parseBoundedInt reads an integer query parameter with a default and an
// inclusive range, rejecting values outside it rather than clamping: a caller
// who asked for more than the endpoint gives should learn that, not silently
// get less.
func parseBoundedInt(query url.Values, key string, def, min, max int) (int, error) {
	raw := query.Get(key)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, &ParamError{Field: key, Value: raw, Message: key + " must be a whole number"}
	}
	if n < min || n > max {
		return 0, &ParamError{Field: key, Value: raw, Message: key + " must be between " + strconv.Itoa(min) + " and " + strconv.Itoa(max)}
	}
	return n, nil
}

// reachabilityTrendsResponse is the GET /api/reachability/trends body.
type reachabilityTrendsResponse struct {
	Domain string                      `json:"domain"`
	Days   int                         `json:"days"`
	Trends []storage.ReachabilityTrend `json:"trends"`
	Count  int                         `json:"count"`
}

// GetReachabilityTrendsHandler serves the daily operational/failed counts.
// GET /api/reachability/trends?days=90&domain=fidonet - days omitted or 0
// means the whole history, as on the web page.
func (s *Server) GetReachabilityTrendsHandler(w http.ResponseWriter, r *http.Request) {
	days, err := parseBoundedInt(r.URL.Query(), "days", 0, 0, maxAnalyticsDays)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	domain := domainOrAll(r)
	var trends []storage.ReachabilityTrend
	if days == 0 {
		trends, err = s.storage.GetReachabilityTrendsAllTime(r.Context(), domain)
	} else {
		trends, err = s.storage.GetReachabilityTrends(r.Context(), days, domain)
	}
	if err != nil {
		writeStorageError(w, "Failed to get reachability trends", err)
		return
	}
	if trends == nil {
		trends = []storage.ReachabilityTrend{}
	}
	WriteJSONSuccess(w, reachabilityTrendsResponse{Domain: domain, Days: days, Trends: trends, Count: len(trends)})
}

// reachabilitySearchResponse is the GET /api/reachability/nodes body.
type reachabilitySearchResponse struct {
	Nodes  []storage.NodeTestResult `json:"nodes"`
	Count  int                      `json:"count"`
	Filter reachabilityFilterEcho   `json:"filter"`
}

// reachabilityFilterEcho is the search's reading of its parameters. An empty
// status or protocol means "any"; an empty domain means every network.
type reachabilityFilterEcho struct {
	Status   string `json:"status"`
	Protocol string `json:"protocol"`
	Days     int    `json:"days"`
	Limit    int    `json:"limit"`
	Domain   string `json:"domain"`
}

// SearchReachabilityHandler lists each node's newest test result in the
// window, narrowed by status and protocol.
// GET /api/reachability/nodes?status=failed&protocol=binkp&days=1&limit=50
func (s *Server) SearchReachabilityHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	days, err := parseBoundedInt(query, "days", 1, 1, maxReachabilityDays)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	limit, err := parseBoundedInt(query, "limit", 50, 1, 1000)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	f := storage.ReachabilityFilter{
		Status:   query.Get("status"),
		Protocol: query.Get("protocol"),
		Days:     days,
		Limit:    limit,
		Domain:   domainOrAll(r),
	}
	// The web page's selects spell "any" as all/any; accept them here too.
	if f.Status == "all" {
		f.Status = ""
	}
	if f.Protocol == "any" {
		f.Protocol = ""
	}
	if err := f.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	nodes, err := s.storage.SearchNodesByReachability(r.Context(), f)
	if err != nil {
		writeStorageError(w, "Failed to search test results", err)
		return
	}
	if nodes == nil {
		nodes = []storage.NodeTestResult{}
	}
	WriteJSONSuccess(w, reachabilitySearchResponse{
		Nodes: nodes,
		Count: len(nodes),
		Filter: reachabilityFilterEcho{
			Status: f.Status, Protocol: f.Protocol, Days: f.Days, Limit: f.Limit, Domain: f.Domain,
		},
	})
}

// GetNodeTestsHandler serves one node's test history and its statistics
// over the window.
// GET /api/nodes/{zone}/{net}/{node}/tests?days=30
func (s *Server) GetNodeTestsHandler(w http.ResponseWriter, r *http.Request) {
	zone, net, node, _, ok := parse4DPathParams(w, r, false)
	if !ok {
		return
	}
	days, err := parseBoundedInt(r.URL.Query(), "days", 30, 1, maxReachabilityDays)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	domain, availableDomains := s.resolveNodeDomain(r, zone, net, node)
	tests, err := s.storage.GetNodeTestHistory(r.Context(), zone, net, node, days, domain)
	if err != nil {
		writeStorageError(w, "Failed to get node test history", err)
		return
	}
	stats, err := s.storage.GetNodeReachabilityStats(r.Context(), zone, net, node, days, domain)
	if err != nil {
		writeStorageError(w, "Failed to get node reachability statistics", err)
		return
	}
	if tests == nil {
		tests = []storage.NodeTestResult{}
	}
	// A node that was never tested is an empty list, not an error: "has this
	// node been tested" deserves a 200 either way.
	response := addressEnvelope(zone, net, node, -1, domain, availableDomains)
	response["days"] = days
	response["tests"] = tests
	response["count"] = len(tests)
	response["stats"] = stats
	WriteJSONSuccess(w, response)
}

// GetNodeTestDetailHandler serves one test result in full.
// GET /api/nodes/{zone}/{net}/{node}/tests/detail?time=2026-09-10T12:00:00Z
//
// The timestamp travels as a query parameter, not a path segment: chi hands
// a handler the raw path, so a client that percent-encodes the "+" of a
// zone offset would get it back still encoded.
func (s *Server) GetNodeTestDetailHandler(w http.ResponseWriter, r *http.Request) {
	zone, net, node, _, ok := parse4DPathParams(w, r, false)
	if !ok {
		return
	}
	raw := r.URL.Query().Get("time")
	if raw == "" {
		WriteJSONError(w, "time is required (RFC 3339, as test_time is reported)", http.StatusBadRequest)
		return
	}
	testTime, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		WriteJSONError(w, "time must be an RFC 3339 timestamp", http.StatusBadRequest)
		return
	}
	domain, _ := s.resolveNodeDomain(r, zone, net, node)
	result, err := s.storage.GetDetailedTestResult(r.Context(), zone, net, node, testTime.UTC().Format(time.RFC3339), domain)
	if err != nil {
		writeStorageError(w, "Failed to get test result", err)
		return
	}
	if result == nil {
		WriteJSONError(w, "Test result not found", http.StatusNotFound)
		return
	}
	WriteJSONSuccess(w, result)
}
