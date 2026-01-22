package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/modem"
	"github.com/nodelistdb/internal/storage"
)

// ModemHandler handles modem API endpoints
type ModemHandler struct {
	config       *config.ModemAPIConfig
	queueOps     *storage.ModemQueueOperations
	callerOps    *storage.ModemCallerOperations
	assigner     *modem.ModemAssigner
	resultOps    *storage.ModemResultOperations
}

// NewModemHandler creates a new ModemHandler instance
func NewModemHandler(
	cfg *config.ModemAPIConfig,
	queueOps *storage.ModemQueueOperations,
	callerOps *storage.ModemCallerOperations,
	assigner *modem.ModemAssigner,
	resultOps *storage.ModemResultOperations,
) *ModemHandler {
	return &ModemHandler{
		config:    cfg,
		queueOps:  queueOps,
		callerOps: callerOps,
		assigner:  assigner,
		resultOps: resultOps,
	}
}

// GetAssignedNodes handles GET /api/modem/nodes
func (h *ModemHandler) GetAssignedNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	callerID := GetCallerIDFromContext(ctx)
	if callerID == "" {
		WriteJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	limit := 50 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > h.config.MaxBatchSize {
				limit = h.config.MaxBatchSize
			}
		}
	}

	onlyCallableStr := r.URL.Query().Get("only_callable")
	onlyCallable := true // default
	if onlyCallableStr == "false" {
		onlyCallable = false
	}

	// Get assigned nodes
	nodes, err := h.queueOps.GetAssignedNodes(ctx, callerID, limit, onlyCallable)
	if err != nil {
		logging.Error("failed to get assigned nodes", "caller_id", callerID, "error", err)
		WriteJSONError(w, "failed to retrieve nodes", http.StatusInternalServerError)
		return
	}

	// Get remaining count
	remaining, err := h.queueOps.GetPendingCount(ctx, callerID)
	if err != nil {
		remaining = 0 // Non-fatal
	}
	remaining -= len(nodes) // Approximate remaining
	if remaining < 0 {
		remaining = 0
	}

	// Convert to response format
	response := GetNodesResponse{
		Nodes:     make([]ModemNodeResponse, len(nodes)),
		Remaining: remaining,
	}

	for i, node := range nodes {
		response.Nodes[i] = ModemNodeResponse{
			Zone:             node.Zone,
			Net:              node.Net,
			Node:             node.Node,
			ConflictSequence: node.ConflictSequence,
			Phone:            node.Phone,
			PhoneNormalized:  node.PhoneNormalized,
			Address:          node.Address(),
			ModemFlags:       node.ModemFlags,
			Flags:            node.Flags,
			Priority:         node.Priority,
			RetryCount:       node.RetryCount,
			IsCallableNow:    node.IsCallableNow,
		}
		if node.NextCallWindow != nil {
			response.Nodes[i].NextCallWindow = &CallWindowResponse{
				StartUTC: node.NextCallWindow.StartUTC.Format(time.RFC3339),
				EndUTC:   node.NextCallWindow.EndUTC.Format(time.RFC3339),
				Source:   node.NextCallWindow.Source,
			}
		}
	}

	WriteJSONSuccess(w, response)
}

// MarkInProgress handles POST /api/modem/in-progress
func (h *ModemHandler) MarkInProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	callerID := GetCallerIDFromContext(ctx)
	if callerID == "" {
		WriteJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req InProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Nodes) == 0 {
		WriteJSONError(w, "no nodes specified", http.StatusBadRequest)
		return
	}

	if len(req.Nodes) > h.config.MaxBatchSize {
		WriteJSONError(w, fmt.Sprintf("too many nodes (max %d)", h.config.MaxBatchSize), http.StatusBadRequest)
		return
	}

	// Convert to storage type
	nodes := make([]storage.NodeIdentifier, len(req.Nodes))
	for i, n := range req.Nodes {
		nodes[i] = storage.NodeIdentifier{
			Zone:             n.Zone,
			Net:              n.Net,
			Node:             n.Node,
			ConflictSequence: n.ConflictSequence,
		}
	}

	marked, err := h.queueOps.MarkNodesInProgress(ctx, callerID, nodes)
	if err != nil {
		logging.Error("failed to mark nodes in progress", "caller_id", callerID, "error", err)
		WriteJSONError(w, "failed to mark nodes", http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, InProgressResponse{Marked: marked})
}

// SubmitResults handles POST /api/modem/results
func (h *ModemHandler) SubmitResults(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	callerID := GetCallerIDFromContext(ctx)
	if callerID == "" {
		WriteJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req SubmitResultsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Results) == 0 {
		WriteJSONError(w, "no results specified", http.StatusBadRequest)
		return
	}

	if len(req.Results) > h.config.MaxBatchSize {
		WriteJSONError(w, fmt.Sprintf("too many results (max %d)", h.config.MaxBatchSize), http.StatusBadRequest)
		return
	}

	// Process results
	stored := 0
	for _, result := range req.Results {
		node := storage.NodeIdentifier{
			Zone:             result.Zone,
			Net:              result.Net,
			Node:             result.Node,
			ConflictSequence: result.ConflictSequence,
		}

		// Pre-check: verify caller owns this node and it's in_progress
		owns, err := h.queueOps.VerifyNodeOwnership(ctx, callerID, node)
		if err != nil {
			logging.Warn("failed to verify node ownership", "node", node.Address(), "error", err)
			continue
		}
		if !owns {
			logging.Warn("caller does not own node or node not in_progress",
				"caller_id", callerID, "node", node.Address())
			continue
		}

		// Attempt mutation
		var expectedStatus string
		if result.Success {
			expectedStatus = storage.ModemQueueStatusCompleted
			if err := h.queueOps.MarkNodeCompleted(ctx, callerID, node); err != nil {
				logging.Warn("failed to mark node completed", "node", node.Address(), "error", err)
				continue
			}
		} else {
			expectedStatus = storage.ModemQueueStatusFailed
			if err := h.queueOps.MarkNodeFailed(ctx, callerID, node, result.Error); err != nil {
				logging.Warn("failed to mark node failed", "node", node.Address(), "error", err)
				continue
			}
		}

		// Post-check: verify mutation took effect (eliminates TOCTOU race)
		// ClickHouse mutations are async but complete quickly; this catches races
		verified, err := h.queueOps.VerifyNodeStatus(ctx, callerID, node, expectedStatus)
		if err != nil {
			logging.Warn("failed to verify node status after mutation", "node", node.Address(), "error", err)
			continue
		}
		if !verified {
			logging.Warn("mutation did not take effect (possible race)",
				"caller_id", callerID, "node", node.Address(), "expected_status", expectedStatus)
			continue
		}

		// Store detailed modem test result (only after verified mutation)
		if err := h.storeModemTestResult(ctx, callerID, result); err != nil {
			logging.Warn("failed to store modem test result", "node", node.Address(), "error", err)
			// Continue anyway - queue status update succeeded
		}

		stored++
	}

	WriteJSONSuccess(w, SubmitResultsResponse{
		Accepted: len(req.Results),
		Stored:   stored,
	})
}

// Heartbeat handles POST /api/modem/heartbeat
func (h *ModemHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	callerID := GetCallerIDFromContext(ctx)
	if callerID == "" {
		WriteJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate status
	validStatuses := []string{storage.ModemCallerStatusActive, storage.ModemCallerStatusInactive, storage.ModemCallerStatusMaintenance}
	statusValid := false
	for _, s := range validStatuses {
		if req.Status == s {
			statusValid = true
			break
		}
	}
	if !statusValid {
		WriteJSONError(w, "invalid status value", http.StatusBadRequest)
		return
	}

	// Parse last test time if provided
	var lastTestTime time.Time
	if req.LastTestTime != "" {
		var err error
		lastTestTime, err = time.Parse(time.RFC3339, req.LastTestTime)
		if err != nil {
			WriteJSONError(w, "invalid last_test_time format", http.StatusBadRequest)
			return
		}
	}

	// Update heartbeat
	stats := storage.HeartbeatStats{
		Status:          req.Status,
		ModemsAvailable: req.ModemsAvailable,
		ModemsInUse:     req.ModemsInUse,
		TestsCompleted:  req.TestsCompleted,
		TestsFailed:     req.TestsFailed,
		LastTestTime:    lastTestTime,
	}

	if err := h.callerOps.UpdateHeartbeat(ctx, callerID, stats); err != nil {
		logging.Error("failed to update heartbeat", "caller_id", callerID, "error", err)
		WriteJSONError(w, "failed to update heartbeat", http.StatusInternalServerError)
		return
	}

	// Call assigner's heartbeat handler
	if err := h.assigner.OnDaemonHeartbeat(ctx, callerID); err != nil {
		logging.Warn("heartbeat handler error", "caller_id", callerID, "error", err)
	}

	// Get assigned nodes count
	assignedNodes, err := h.queueOps.GetPendingCount(ctx, callerID)
	if err != nil {
		assignedNodes = 0
	}

	WriteJSONSuccess(w, HeartbeatResponse{
		Ack:           true,
		AssignedNodes: assignedNodes,
	})
}

// ReleaseNodes handles POST /api/modem/release
func (h *ModemHandler) ReleaseNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	callerID := GetCallerIDFromContext(ctx)
	if callerID == "" {
		WriteJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req ReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Nodes) == 0 {
		WriteJSONError(w, "no nodes specified", http.StatusBadRequest)
		return
	}

	if len(req.Nodes) > h.config.MaxBatchSize {
		WriteJSONError(w, fmt.Sprintf("too many nodes (max %d)", h.config.MaxBatchSize), http.StatusBadRequest)
		return
	}

	released := 0
	for _, n := range req.Nodes {
		// Release node with ownership check and clear assignment for reassignment
		if err := h.queueOps.ReleaseNode(ctx, callerID, n.Zone, n.Net, n.Node, n.ConflictSequence); err != nil {
			logging.Warn("failed to release node", "node", fmt.Sprintf("%d:%d/%d", n.Zone, n.Net, n.Node), "error", err)
			continue
		}
		released++
	}

	// Nodes will be reassigned by the queue manager's orphan check

	WriteJSONSuccess(w, ReleaseResponse{
		Released: released,
	})
}

// GetQueueStats handles GET /api/modem/stats (optional admin endpoint)
func (h *ModemHandler) GetQueueStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := h.queueOps.GetQueueStats(ctx)
	if err != nil {
		WriteJSONError(w, "failed to get queue stats", http.StatusInternalServerError)
		return
	}

	WriteJSONSuccess(w, QueueStatsResponse{
		TotalNodes:      stats.TotalNodes,
		PendingNodes:    stats.PendingNodes,
		InProgressNodes: stats.InProgressNodes,
		CompletedNodes:  stats.CompletedNodes,
		FailedNodes:     stats.FailedNodes,
		ByDaemon:        stats.ByDaemon,
	})
}

// storeModemTestResult stores a detailed modem test result in the database
func (h *ModemHandler) storeModemTestResult(ctx context.Context, callerID string, result ModemTestResultRequest) error {
	if h.resultOps == nil {
		return nil // Result storage not configured
	}

	// Parse test time
	var testTime time.Time
	if result.TestTime != "" {
		var err error
		testTime, err = time.Parse(time.RFC3339, result.TestTime)
		if err != nil {
			testTime = time.Now()
		}
	} else {
		testTime = time.Now()
	}

	input := &storage.ModemTestResultInput{
		Zone:             result.Zone,
		Net:              result.Net,
		Node:             result.Node,
		ConflictSequence: result.ConflictSequence,
		TestTime:         testTime,
		CallerID:         callerID,
		Success:          result.Success,
		ResponseMs:       result.ResponseMs,
		SystemName:       result.SystemName,
		MailerInfo:       result.MailerInfo,
		Addresses:        result.Addresses,
		AddressValid:     result.AddressValid,
		ResponseType:     result.ResponseType,
		SoftwareSource:   result.SoftwareSource,
		Error:            result.Error,
		ConnectSpeed:     result.ConnectSpeed,
		ModemProtocol:    result.ModemProtocol,
		PhoneDialed:      result.PhoneDialed,
		RingCount:        result.RingCount,
		CarrierTimeMs:    result.CarrierTimeMs,
		ModemUsed:        result.ModemUsed,
		MatchReason:      result.MatchReason,
		ModemLineStats:   result.ModemLineStats,
	}

	return h.resultOps.StoreModemTestResult(ctx, input)
}
