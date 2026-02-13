package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nodelistdb/internal/config"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/storage"
)

// ModemHandler handles modem API endpoints
type ModemHandler struct {
	config    *config.ModemAPIConfig
	resultOps *storage.ModemResultOperations
}

// NewModemHandler creates a new ModemHandler instance
func NewModemHandler(
	cfg *config.ModemAPIConfig,
	resultOps *storage.ModemResultOperations,
) *ModemHandler {
	return &ModemHandler{
		config:    cfg,
		resultOps: resultOps,
	}
}

// SubmitResultsDirect handles POST /api/modem/results/direct
// This endpoint allows CLI tools to submit results directly
func (h *ModemHandler) SubmitResultsDirect(w http.ResponseWriter, r *http.Request) {
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

	// Store results directly
	stored := 0
	for _, result := range req.Results {
		if err := h.storeModemTestResultDirect(ctx, callerID, result); err != nil {
			logging.Warn("failed to store direct modem test result",
				"zone", result.Zone, "net", result.Net, "node", result.Node,
				"error", err)
			continue
		}
		stored++
	}

	WriteJSONSuccess(w, SubmitResultsResponse{
		Accepted: len(req.Results),
		Stored:   stored,
	})
}

// baseModemResultInput creates a ModemTestResultInput with common fields.
func baseModemResultInput(callerID, testSource string, result ModemTestResultRequest) *storage.ModemTestResultInput {
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

	return &storage.ModemTestResultInput{
		Zone:             result.Zone,
		Net:              result.Net,
		Node:             result.Node,
		ConflictSequence: result.ConflictSequence,
		TestTime:         testTime,
		CallerID:         callerID,
		TestSource:       testSource,
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
}

// storeModemTestResultDirect stores a modem test result from CLI submissions
func (h *ModemHandler) storeModemTestResultDirect(ctx context.Context, callerID string, result ModemTestResultRequest) error {
	if h.resultOps == nil {
		return nil // Result storage not configured
	}

	input := baseModemResultInput(callerID, "cli", result)

	// CLI-specific extended fields
	input.OperatorName = result.OperatorName
	input.OperatorPrefix = result.OperatorPrefix
	input.DialTimeMs = result.DialTimeMs
	input.EMSITimeMs = result.EMSITimeMs
	input.ConnectString = result.ConnectString
	input.RemoteLocation = result.RemoteLocation
	input.RemoteSysop = result.RemoteSysop

	// Map line stats if provided
	if result.LineStats != nil {
		input.TXSpeed = result.LineStats.TXSpeed
		input.RXSpeed = result.LineStats.RXSpeed
		input.Compression = result.LineStats.Compression
		input.Modulation = result.LineStats.Modulation
		input.LineQuality = result.LineStats.LineQuality
		input.SNR = result.LineStats.SNR
		input.RxLevel = result.LineStats.RxLevel
		input.TxPower = result.LineStats.TxPower
		input.RoundTripDelay = result.LineStats.RoundTripDelay
		input.LocalRetrains = result.LineStats.LocalRetrains
		input.RemoteRetrains = result.LineStats.RemoteRetrains
		input.TerminationReason = result.LineStats.TerminationReason
		input.StatsNotes = result.LineStats.StatsNotes
	}

	// Map AudioCodes CDR if provided
	if result.AudioCodesCDR != nil {
		input.CDRSessionID = result.AudioCodesCDR.SessionID
		input.CDRCodec = result.AudioCodesCDR.Codec
		input.CDRRTPJitterMs = result.AudioCodesCDR.RTPJitterMs
		input.CDRRTPDelayMs = result.AudioCodesCDR.RTPDelayMs
		input.CDRPacketLoss = result.AudioCodesCDR.PacketLoss
		input.CDRRemotePacketLoss = result.AudioCodesCDR.RemotePacketLoss
		input.CDRLocalMOS = result.AudioCodesCDR.LocalMOS
		input.CDRRemoteMOS = result.AudioCodesCDR.RemoteMOS
		input.CDRLocalRFactor = result.AudioCodesCDR.LocalRFactor
		input.CDRRemoteRFactor = result.AudioCodesCDR.RemoteRFactor
		input.CDRTermReason = result.AudioCodesCDR.TermReason
		input.CDRTermCategory = result.AudioCodesCDR.TermCategory
	}

	// Map Asterisk CDR if provided
	if result.AsteriskCDR != nil {
		input.AstDisposition = result.AsteriskCDR.Disposition
		input.AstPeer = result.AsteriskCDR.Peer
		input.AstDuration = result.AsteriskCDR.Duration
		input.AstBillSec = result.AsteriskCDR.BillSec
		input.AstHangupCause = result.AsteriskCDR.HangupCause
		input.AstHangupSource = result.AsteriskCDR.HangupSource
		input.AstEarlyMedia = result.AsteriskCDR.EarlyMedia
	}

	return h.resultOps.StoreModemTestResult(ctx, input)
}
