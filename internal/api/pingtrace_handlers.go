package api

import (
	"net/http"
	"strconv"

	"github.com/nodelistdb/internal/pingtrace"
	"github.com/nodelistdb/internal/storage"
)

// GetPingTraceStats returns the FTS-4010 netmail PING/TRACE report.
// GET /api/analytics/pingtrace?domain=fidonet&days=90
func (s *Server) GetPingTraceStats(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r.URL.Query(), 90)
	summary, err := s.storage.GetPingTraceSummary(r.Context(), domainOrAll(r), days)
	if err != nil {
		writeStorageError(w, "Failed to get PING/TRACE summary", err)
		return
	}
	WriteJSONSuccess(w, summary)
}

// nodePingResponse is GET /api/nodes/{zone}/{net}/{node}/ping.
type nodePingResponse struct {
	Pings   []pingtrace.Ping       `json:"pings"`
	Replies []storage.PingReplyRow `json:"replies"`
}

// GetNodePingHandler returns a node's netmail PING history and the replies
// behind it.
// GET /api/nodes/{zone}/{net}/{node}/ping?domain=fidonet&limit=50
func (s *Server) GetNodePingHandler(w http.ResponseWriter, r *http.Request) {
	zone, net, node, _, ok := parse4DPathParams(w, r, false)
	if !ok {
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	domain, availableDomains := s.resolveNodeDomain(r, zone, net, node)
	pings, err := s.storage.GetNodePings(r.Context(), domain, zone, net, node, limit)
	if err != nil {
		writeStorageErrorf(w, "Failed to get node pings", err)
		return
	}
	replies, err := s.storage.GetNodePingReplies(r.Context(), domain, zone, net, node, limit*4)
	if err != nil {
		writeStorageErrorf(w, "Failed to get node ping replies", err)
		return
	}
	if pings == nil {
		pings = []pingtrace.Ping{}
	}
	if replies == nil {
		replies = []storage.PingReplyRow{}
	}
	response := addressEnvelope(zone, net, node, -1, domain, availableDomains)
	response["pings"] = pings
	response["replies"] = replies
	WriteJSONSuccess(w, response)
}
