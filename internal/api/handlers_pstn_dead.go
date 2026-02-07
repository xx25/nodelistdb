package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// pstnDeadRequest is the request body for marking/unmarking PSTN dead nodes
type pstnDeadRequest struct {
	Zone   int    `json:"zone"`
	Net    int    `json:"net"`
	Node   int    `json:"node"`
	Reason string `json:"reason"`
}

// MarkPSTNDeadHandler marks a node's PSTN number as dead/disconnected.
// POST /api/modem/pstn-dead (authenticated)
func (s *Server) MarkPSTNDeadHandler(w http.ResponseWriter, r *http.Request) {
	var req pstnDeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Zone <= 0 || req.Net <= 0 || req.Node <= 0 {
		WriteJSONError(w, "zone, net, and node must be positive integers", http.StatusBadRequest)
		return
	}

	callerID := GetCallerIDFromContext(r.Context())
	if err := s.storage.MarkPSTNDead(req.Zone, req.Net, req.Node, req.Reason, callerID); err != nil {
		WriteJSONError(w, fmt.Sprintf("failed to mark node dead: %v", err), http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Node %d:%d/%d marked as PSTN dead", req.Zone, req.Net, req.Node),
	})
}

// UnmarkPSTNDeadHandler revives a previously dead PSTN node.
// DELETE /api/modem/pstn-dead (authenticated)
func (s *Server) UnmarkPSTNDeadHandler(w http.ResponseWriter, r *http.Request) {
	var req pstnDeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Zone <= 0 || req.Net <= 0 || req.Node <= 0 {
		WriteJSONError(w, "zone, net, and node must be positive integers", http.StatusBadRequest)
		return
	}

	callerID := GetCallerIDFromContext(r.Context())
	if err := s.storage.UnmarkPSTNDead(req.Zone, req.Net, req.Node, callerID); err != nil {
		WriteJSONError(w, fmt.Sprintf("failed to unmark node dead: %v", err), http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Node %d:%d/%d unmarked as PSTN dead", req.Zone, req.Net, req.Node),
	})
}

// ListPSTNDeadHandler returns all currently dead PSTN nodes.
// GET /api/nodes/pstn/dead (unauthenticated)
func (s *Server) ListPSTNDeadHandler(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.storage.GetPSTNDeadNodes()
	if err != nil {
		WriteJSONError(w, fmt.Sprintf("failed to fetch PSTN dead nodes: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"count": len(nodes),
	}
	if nodes == nil {
		response["nodes"] = []struct{}{}
	} else {
		response["nodes"] = nodes
	}

	WriteJSONSuccess(w, response)
}
