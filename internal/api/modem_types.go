package api

import (
	"time"
)

// GetNodesResponse is the response for GET /api/modem/nodes
type GetNodesResponse struct {
	Nodes     []ModemNodeResponse `json:"nodes"`
	Remaining int                 `json:"remaining"`
}

// ModemNodeResponse represents a node in the API response
type ModemNodeResponse struct {
	Zone             uint16           `json:"zone"`
	Net              uint16           `json:"net"`
	Node             uint16           `json:"node"`
	ConflictSequence uint8            `json:"conflict_sequence"`
	Phone            string           `json:"phone"`
	PhoneNormalized  string           `json:"phone_normalized"`
	Address          string           `json:"address"`
	ModemFlags       []string         `json:"modem_flags"`
	Flags            []string         `json:"flags"`
	Priority         uint8            `json:"priority"`
	RetryCount       uint8            `json:"retry_count"`
	IsCallableNow    bool             `json:"is_callable_now"`
	NextCallWindow   *CallWindowResponse `json:"next_call_window,omitempty"`
}

// CallWindowResponse represents a time window when a node is callable
type CallWindowResponse struct {
	StartUTC string `json:"start_utc"`
	EndUTC   string `json:"end_utc"`
	Source   string `json:"source"`
}

// InProgressRequest is the request body for POST /api/modem/in-progress
type InProgressRequest struct {
	Nodes []NodeIdentifierRequest `json:"nodes"`
}

// NodeIdentifierRequest identifies a specific node
type NodeIdentifierRequest struct {
	Zone             uint16 `json:"zone"`
	Net              uint16 `json:"net"`
	Node             uint16 `json:"node"`
	ConflictSequence uint8  `json:"conflict_sequence"`
}

// InProgressResponse is the response for POST /api/modem/in-progress
type InProgressResponse struct {
	Marked int `json:"marked"`
}

// SubmitResultsRequest is the request body for POST /api/modem/results
type SubmitResultsRequest struct {
	Results []ModemTestResultRequest `json:"results"`
}

// ModemTestResultRequest contains the result of a modem test
type ModemTestResultRequest struct {
	Zone             uint16   `json:"zone"`
	Net              uint16   `json:"net"`
	Node             uint16   `json:"node"`
	ConflictSequence uint8    `json:"conflict_sequence"`
	TestTime         string   `json:"test_time"` // RFC3339 format

	// EMSI results
	Success        bool     `json:"success"`
	ResponseMs     uint32   `json:"response_ms"`
	SystemName     string   `json:"system_name,omitempty"`
	MailerInfo     string   `json:"mailer_info,omitempty"`
	Addresses      []string `json:"addresses,omitempty"`
	AddressValid   bool     `json:"address_valid"`
	ResponseType   string   `json:"response_type,omitempty"`
	SoftwareSource string   `json:"software_source,omitempty"`
	Error          string   `json:"error,omitempty"`

	// Modem-specific fields
	ConnectSpeed  uint32 `json:"connect_speed,omitempty"`
	ModemProtocol string `json:"modem_protocol,omitempty"`
	PhoneDialed   string `json:"phone_dialed,omitempty"`
	RingCount     uint8  `json:"ring_count,omitempty"`
	CarrierTimeMs uint32 `json:"carrier_time_ms,omitempty"`
	ModemUsed     string `json:"modem_used,omitempty"`
	MatchReason   string `json:"match_reason,omitempty"`
}

// SubmitResultsResponse is the response for POST /api/modem/results
type SubmitResultsResponse struct {
	Accepted int `json:"accepted"`
	Stored   int `json:"stored"`
}

// HeartbeatRequest is the request body for POST /api/modem/heartbeat
type HeartbeatRequest struct {
	Status          string `json:"status"` // active, inactive, maintenance
	ModemsAvailable uint8  `json:"modems_available"`
	ModemsInUse     uint8  `json:"modems_in_use"`
	TestsCompleted  uint32 `json:"tests_completed"`
	TestsFailed     uint32 `json:"tests_failed"`
	LastTestTime    string `json:"last_test_time,omitempty"` // RFC3339 format
}

// HeartbeatResponse is the response for POST /api/modem/heartbeat
type HeartbeatResponse struct {
	Ack           bool `json:"ack"`
	AssignedNodes int  `json:"assigned_nodes"`
}

// ReleaseRequest is the request body for POST /api/modem/release
type ReleaseRequest struct {
	Nodes  []NodeIdentifierRequest `json:"nodes"`
	Reason string                  `json:"reason,omitempty"`
}

// ReleaseResponse is the response for POST /api/modem/release
type ReleaseResponse struct {
	Released     int    `json:"released"`
	ReassignedTo string `json:"reassigned_to,omitempty"`
}

// QueueStatsResponse is the response for GET /api/modem/stats
type QueueStatsResponse struct {
	TotalNodes      int            `json:"total_nodes"`
	PendingNodes    int            `json:"pending_nodes"`
	InProgressNodes int            `json:"in_progress_nodes"`
	CompletedNodes  int            `json:"completed_nodes"`
	FailedNodes     int            `json:"failed_nodes"`
	ByDaemon        map[string]int `json:"by_daemon"`
}

// CallerStatusResponse represents status information for a modem daemon
type CallerStatusResponse struct {
	CallerID        string    `json:"caller_id"`
	Name            string    `json:"name"`
	Location        string    `json:"location"`
	Status          string    `json:"status"`
	LastHeartbeat   time.Time `json:"last_heartbeat"`
	ModemsAvailable uint8     `json:"modems_available"`
	ModemsInUse     uint8     `json:"modems_in_use"`
	TestsCompleted  uint32    `json:"tests_completed"`
	TestsFailed     uint32    `json:"tests_failed"`
	AssignedNodes   int       `json:"assigned_nodes"`
}
