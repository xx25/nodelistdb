// Package main provides NodelistDB server result submission for modem test results.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// NodelistDBWriter submits test results to NodelistDB server
type NodelistDBWriter struct {
	client    *http.Client
	baseURL   string
	apiKey    string
	enabled   bool
	batchSize int

	batch      []nodelistDBResultRequest
	batchMutex sync.Mutex
}

// nodelistDBResultRequest matches the API's ModemTestResultRequest
type nodelistDBResultRequest struct {
	Zone             uint16   `json:"zone"`
	Net              uint16   `json:"net"`
	Node             uint16   `json:"node"`
	ConflictSequence uint8    `json:"conflict_sequence"`
	TestTime         string   `json:"test_time"` // RFC3339 format
	Success          bool     `json:"success"`
	ResponseMs       uint32   `json:"response_ms"`
	SystemName       string   `json:"system_name,omitempty"`
	MailerInfo       string   `json:"mailer_info,omitempty"`
	Addresses        []string `json:"addresses,omitempty"`
	AddressValid     bool     `json:"address_valid"`
	ResponseType     string   `json:"response_type,omitempty"`
	SoftwareSource   string   `json:"software_source,omitempty"`
	Error            string   `json:"error,omitempty"`
	ConnectSpeed     uint32   `json:"connect_speed,omitempty"`
	ModemProtocol    string   `json:"modem_protocol,omitempty"`
	PhoneDialed      string   `json:"phone_dialed,omitempty"`
	RingCount        uint8    `json:"ring_count,omitempty"`
	CarrierTimeMs    uint32   `json:"carrier_time_ms,omitempty"`
	ModemUsed        string   `json:"modem_used,omitempty"`
	MatchReason      string   `json:"match_reason,omitempty"`
	ModemLineStats   string   `json:"modem_line_stats,omitempty"`

	// Extended fields
	LineStats     *lineStatsRequest     `json:"line_stats,omitempty"`
	AudioCodesCDR *audioCodesCDRRequest `json:"audiocodes_cdr,omitempty"`
	AsteriskCDR   *asteriskCDRRequest   `json:"asterisk_cdr,omitempty"`

	// Operator routing
	OperatorName   string `json:"operator_name,omitempty"`
	OperatorPrefix string `json:"operator_prefix,omitempty"`
	DialTimeMs     uint32 `json:"dial_time_ms,omitempty"`
	EMSITimeMs     uint32 `json:"emsi_time_ms,omitempty"`
	ConnectString  string `json:"connect_string,omitempty"`

	// EMSI remote details
	RemoteLocation string `json:"remote_location,omitempty"`
	RemoteSysop    string `json:"remote_sysop,omitempty"`
}

type lineStatsRequest struct {
	TXSpeed           uint32  `json:"tx_speed,omitempty"`
	RXSpeed           uint32  `json:"rx_speed,omitempty"`
	Compression       string  `json:"compression,omitempty"`
	Modulation        string  `json:"modulation,omitempty"`
	LineQuality       uint8   `json:"line_quality,omitempty"`
	SNR               float32 `json:"snr,omitempty"`
	RxLevel           int16   `json:"rx_level,omitempty"`
	TxPower           int16   `json:"tx_power,omitempty"`
	RoundTripDelay    uint16  `json:"round_trip_delay,omitempty"`
	LocalRetrains     uint8   `json:"local_retrains,omitempty"`
	RemoteRetrains    uint8   `json:"remote_retrains,omitempty"`
	TerminationReason string  `json:"termination_reason,omitempty"`
	StatsNotes        string  `json:"stats_notes,omitempty"`
}

type audioCodesCDRRequest struct {
	SessionID        string `json:"session_id,omitempty"`
	Codec            string `json:"codec,omitempty"`
	RTPJitterMs      uint16 `json:"rtp_jitter_ms,omitempty"`
	RTPDelayMs       uint16 `json:"rtp_delay_ms,omitempty"`
	PacketLoss       uint8  `json:"packet_loss,omitempty"`
	RemotePacketLoss uint8  `json:"remote_packet_loss,omitempty"`
	LocalMOS         uint8  `json:"local_mos,omitempty"`
	RemoteMOS        uint8  `json:"remote_mos,omitempty"`
	LocalRFactor     uint8  `json:"local_r_factor,omitempty"`
	RemoteRFactor    uint8  `json:"remote_r_factor,omitempty"`
	TermReason       string `json:"term_reason,omitempty"`
	TermCategory     string `json:"term_category,omitempty"`
}

type asteriskCDRRequest struct {
	Disposition  string `json:"disposition,omitempty"`
	Peer         string `json:"peer,omitempty"`
	Duration     uint16 `json:"duration,omitempty"`
	BillSec      uint16 `json:"billsec,omitempty"`
	HangupCause  uint8  `json:"hangup_cause,omitempty"`
	HangupSource string `json:"hangup_source,omitempty"`
	EarlyMedia   bool   `json:"early_media,omitempty"`
}

type submitResultsRequest struct {
	Results []nodelistDBResultRequest `json:"results"`
}

type submitResultsResponse struct {
	Accepted int    `json:"accepted"`
	Stored   int    `json:"stored"`
	Error    string `json:"error,omitempty"`
}

// NewNodelistDBWriter creates a new NodelistDB results writer
func NewNodelistDBWriter(cfg NodelistDBConfig) (*NodelistDBWriter, error) {
	if !cfg.Submit || cfg.URL == "" {
		return &NodelistDBWriter{enabled: false}, nil
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for NodelistDB result submission")
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 10 // Default batch size
	}

	w := &NodelistDBWriter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:   strings.TrimSuffix(cfg.URL, "/"),
		apiKey:    cfg.APIKey,
		enabled:   true,
		batchSize: batchSize,
		batch:     make([]nodelistDBResultRequest, 0, batchSize),
	}

	// Test connection with a health check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.healthCheck(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to NodelistDB server: %w", err)
	}

	return w, nil
}

// IsEnabled returns true if the writer is active
func (w *NodelistDBWriter) IsEnabled() bool {
	return w.enabled
}

// healthCheck verifies connectivity to the NodelistDB server
func (w *NodelistDBWriter) healthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", w.baseURL+"/api/health", nil)
	if err != nil {
		return err
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// WriteRecord adds a test record to the batch and flushes if batch is full
func (w *NodelistDBWriter) WriteRecord(rec *TestRecord) error {
	if !w.enabled {
		return nil
	}

	// Convert TestRecord to API request
	apiReq := w.convertRecord(rec)

	w.batchMutex.Lock()
	w.batch = append(w.batch, apiReq)
	shouldFlush := len(w.batch) >= w.batchSize
	w.batchMutex.Unlock()

	if shouldFlush {
		return w.Flush()
	}

	return nil
}

// convertRecord transforms a TestRecord into an API request
func (w *NodelistDBWriter) convertRecord(rec *TestRecord) nodelistDBResultRequest {
	// Parse node address (e.g., "2:5020/100") into zone/net/node
	var zone, net, node uint16
	if rec.NodeAddress != "" {
		_, _ = fmt.Sscanf(rec.NodeAddress, "%d:%d/%d", &zone, &net, &node)
	}

	req := nodelistDBResultRequest{
		Zone:           zone,
		Net:            net,
		Node:           node,
		TestTime:       rec.Timestamp.Format(time.RFC3339),
		Success:        rec.Success,
		ResponseMs:     uint32(rec.DialTime.Milliseconds() + rec.EMSITime.Milliseconds()),
		SystemName:     rec.RemoteSystem,
		MailerInfo:     rec.RemoteMailer,
		Addresses:      parseAddresses(rec.RemoteAddress),
		AddressValid:   validateModemAddress(rec.RemoteAddress, rec.NodeAddress),
		ResponseType:   "", // Not tracked in CLI
		Error:          rec.EMSIError,
		ConnectSpeed:   uint32(rec.ConnectSpeed),
		ModemProtocol:  rec.Protocol,
		PhoneDialed:    rec.Phone,
		RingCount:      0,  // Not tracked
		CarrierTimeMs:  uint32(rec.DialTime.Milliseconds()),
		ModemUsed:      rec.ModemName,
		ConnectString:  rec.ConnectString,
		OperatorName:   rec.OperatorName,
		OperatorPrefix: rec.OperatorPrefix,
		DialTimeMs:     uint32(rec.DialTime.Milliseconds()),
		EMSITimeMs:     uint32(rec.EMSITime.Milliseconds()),
		RemoteLocation: rec.RemoteLocation,
		RemoteSysop:    rec.RemoteSysop,
	}

	// Line stats
	if rec.TXSpeed > 0 || rec.RXSpeed > 0 || rec.Protocol != "" {
		req.LineStats = &lineStatsRequest{
			TXSpeed:           uint32(rec.TXSpeed),
			RXSpeed:           uint32(rec.RXSpeed),
			Compression:       rec.Compression,
			Modulation:        rec.Protocol, // Protocol field contains modulation
			LineQuality:       uint8(rec.LineQuality),
			RxLevel:           int16(rec.RxLevel),
			LocalRetrains:     uint8(rec.Retrains),
			TerminationReason: rec.Termination,
			StatsNotes:        rec.StatsNotes,
		}
	}

	// AudioCodes CDR
	if rec.CDRSessionID != "" || rec.CDRCodec != "" {
		req.AudioCodesCDR = &audioCodesCDRRequest{
			SessionID:        rec.CDRSessionID,
			Codec:            rec.CDRCodec,
			RTPJitterMs:      uint16(rec.CDRRTPJitter),
			RTPDelayMs:       uint16(rec.CDRRTPDelay),
			PacketLoss:       uint8(rec.CDRPacketLoss),
			RemotePacketLoss: uint8(rec.CDRRemotePacketLoss),
			LocalMOS:         uint8(rec.CDRLocalMOS * 10), // Convert 1.0-5.0 to 10-50
			RemoteMOS:        uint8(rec.CDRRemoteMOS * 10),
			LocalRFactor:     uint8(rec.CDRLocalRFactor),
			RemoteRFactor:    uint8(rec.CDRRemoteRFactor),
			TermReason:       rec.CDRTermReason,
			TermCategory:     rec.CDRTermCategory,
		}
	}

	// Asterisk CDR
	if rec.AstDisposition != "" || rec.AstPeer != "" {
		req.AsteriskCDR = &asteriskCDRRequest{
			Disposition:  rec.AstDisposition,
			Peer:         rec.AstPeer,
			Duration:     uint16(rec.AstDuration),
			BillSec:      uint16(rec.AstBillSec),
			HangupCause:  uint8(rec.AstHangupCause),
			HangupSource: rec.AstHangupSource,
			EarlyMedia:   rec.AstEarlyMedia,
		}
	}

	return req
}

// parseAddresses splits a comma-separated address string into a slice
func parseAddresses(addresses string) []string {
	if addresses == "" {
		return nil
	}
	parts := strings.Split(addresses, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// normalizeAddress strips domain suffix, point .0, and lowercases a FidoNet address
func normalizeAddress(addr string) string {
	addr = strings.TrimSpace(strings.ToLower(addr))
	if idx := strings.Index(addr, "@"); idx != -1 {
		addr = addr[:idx]
	}
	addr = strings.TrimSuffix(addr, ".0")
	return addr
}

// validateModemAddress checks if any address in a comma/space-separated list matches the expected node address
func validateModemAddress(remoteAddresses, expectedAddress string) bool {
	if remoteAddresses == "" || expectedAddress == "" {
		return false
	}
	expected := normalizeAddress(expectedAddress)
	for _, addr := range strings.FieldsFunc(remoteAddresses, func(r rune) bool {
		return r == ',' || r == ' '
	}) {
		if normalizeAddress(addr) == expected {
			return true
		}
	}
	return false
}

// Flush sends all buffered results to the server
func (w *NodelistDBWriter) Flush() error {
	if !w.enabled {
		return nil
	}

	w.batchMutex.Lock()
	if len(w.batch) == 0 {
		w.batchMutex.Unlock()
		return nil
	}
	// Take the batch and reset
	toSend := w.batch
	w.batch = make([]nodelistDBResultRequest, 0, w.batchSize)
	w.batchMutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return w.sendBatch(ctx, toSend)
}

// sendBatch sends a batch of results to the server
func (w *NodelistDBWriter) sendBatch(ctx context.Context, results []nodelistDBResultRequest) error {
	reqBody := submitResultsRequest{Results: results}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", w.baseURL+"/api/modem/results/direct", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+w.apiKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send results: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result submitResultsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != "" {
		return fmt.Errorf("server error: %s", result.Error)
	}

	return nil
}

// Close flushes any remaining results and closes the writer
func (w *NodelistDBWriter) Close() error {
	return w.Flush()
}
