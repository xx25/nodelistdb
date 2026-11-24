package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nodelistdb/internal/storage"
)

// statusMessage is the body of a write that has nothing to return but "done".
type statusMessage struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// pstnDeadRequest is the request body for marking/unmarking PSTN dead nodes
type pstnDeadRequest struct {
	Zone   int    `json:"zone"`
	Net    int    `json:"net"`
	Node   int    `json:"node"`
	Reason string `json:"reason"`
	// Kind is "dead" (the default) or "voice"; marking only.
	Kind string `json:"kind"`
	// Phone is the number the mark is made for; the mark stops applying
	// when the node lists another. Optional, marking only.
	Phone string `json:"phone"`
}

// MarkPSTNDeadHandler marks a node's PSTN number as not to be dialled:
// dead/disconnected, or (kind "voice") answered by a person.
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

	// Testers that predate kinds send none and mean "dead". Anything else
	// unknown is refused rather than stored: a mistyped "voice" silently
	// filed as "dead" would lose the one distinction that matters.
	kind := req.Kind
	if kind == "" {
		kind = storage.PSTNDeadKindDead
	}
	if !storage.ValidPSTNDeadKind(kind) {
		WriteJSONError(w, fmt.Sprintf("kind must be %q or %q", storage.PSTNDeadKindDead, storage.PSTNDeadKindVoice), http.StatusBadRequest)
		return
	}

	mark := storage.PSTNDeadNode{
		Zone:     req.Zone,
		Net:      req.Net,
		Node:     req.Node,
		Reason:   req.Reason,
		Kind:     kind,
		Phone:    storage.PhoneDigits(req.Phone),
		MarkedBy: GetCallerIDFromContext(r.Context()),
	}
	if err := s.storage.MarkPSTNDead(r.Context(), mark); err != nil {
		writeStorageErrorf(w, "failed to mark node dead", err)
		return
	}

	message := fmt.Sprintf("Node %d:%d/%d marked as PSTN dead", req.Zone, req.Net, req.Node)
	if kind == storage.PSTNDeadKindVoice {
		message = fmt.Sprintf("Node %d:%d/%d marked as a voice line, do not call", req.Zone, req.Net, req.Node)
	}
	WriteJSONSuccess(w, statusMessage{
		Status:  "ok",
		Message: message,
	})
}

// UnmarkPSTNDeadHandler revives a previously marked PSTN node, whatever the
// mark's kind.
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
	if err := s.storage.UnmarkPSTNDead(r.Context(), req.Zone, req.Net, req.Node, callerID); err != nil {
		writeStorageErrorf(w, "failed to unmark node dead", err)
		return
	}

	WriteJSONSuccess(w, statusMessage{
		Status:  "ok",
		Message: fmt.Sprintf("Node %d:%d/%d unmarked as PSTN dead", req.Zone, req.Net, req.Node),
	})
}

// ListPSTNDeadHandler returns all currently marked PSTN nodes, of both kinds.
// GET /api/nodes/pstn/dead (unauthenticated)
func (s *Server) ListPSTNDeadHandler(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.storage.GetPSTNDeadNodes(r.Context())
	if err != nil {
		writeStorageErrorf(w, "failed to fetch PSTN dead nodes", err)
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
