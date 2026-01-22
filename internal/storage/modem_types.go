package storage

import (
	"fmt"
	"time"
)

// ModemQueueEntry represents a node entry in the modem test queue
type ModemQueueEntry struct {
	Zone             uint16    `json:"zone"`
	Net              uint16    `json:"net"`
	Node             uint16    `json:"node"`
	ConflictSequence uint8     `json:"conflict_sequence"`
	Phone            string    `json:"phone"`
	PhoneNormalized  string    `json:"phone_normalized"`
	ModemFlags       []string  `json:"modem_flags"`
	Flags            []string  `json:"flags"`
	IsCM             bool      `json:"is_cm"`
	TimeFlags        []string  `json:"time_flags"`
	AssignedTo       string    `json:"assigned_to"`
	AssignedAt       time.Time `json:"assigned_at"`
	Priority         uint8     `json:"priority"`
	RetryCount       uint8     `json:"retry_count"`
	NextAttemptAfter time.Time `json:"next_attempt_after"`
	Status           string    `json:"status"` // pending, in_progress, completed, failed
	InProgressSince  time.Time `json:"in_progress_since"`
	LastTestedAt     time.Time `json:"last_tested_at"`
	LastError        string    `json:"last_error"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Address returns the FidoNet address string for this entry
func (e *ModemQueueEntry) Address() string {
	return formatAddress(int(e.Zone), int(e.Net), int(e.Node))
}

// ModemQueueNode extends ModemQueueEntry with computed time availability fields
type ModemQueueNode struct {
	ModemQueueEntry
	IsCallableNow  bool        `json:"is_callable_now"`
	NextCallWindow *CallWindow `json:"next_call_window,omitempty"`
}

// CallWindow represents a time window when a node is callable
type CallWindow struct {
	StartUTC time.Time `json:"start_utc"`
	EndUTC   time.Time `json:"end_utc"`
	Source   string    `json:"source"` // "T-flag", "ZMH", "#nn", "CM", "ICM"
}

// ModemCallerStatus represents the runtime status of a modem daemon
type ModemCallerStatus struct {
	CallerID        string    `json:"caller_id"`
	LastHeartbeat   time.Time `json:"last_heartbeat"`
	Status          string    `json:"status"` // active, inactive, maintenance
	ModemsAvailable uint8     `json:"modems_available"`
	ModemsInUse     uint8     `json:"modems_in_use"`
	TestsCompleted  uint32    `json:"tests_completed"`
	TestsFailed     uint32    `json:"tests_failed"`
	LastTestTime    time.Time `json:"last_test_time"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// NodeIdentifier uniquely identifies a node in the queue
type NodeIdentifier struct {
	Zone             uint16 `json:"zone"`
	Net              uint16 `json:"net"`
	Node             uint16 `json:"node"`
	ConflictSequence uint8  `json:"conflict_sequence"`
}

// Address returns the FidoNet address string for this identifier
func (n *NodeIdentifier) Address() string {
	return formatAddress(int(n.Zone), int(n.Net), int(n.Node))
}

// HeartbeatStats contains statistics sent by a modem daemon in a heartbeat
type HeartbeatStats struct {
	Status          string    `json:"status"`
	ModemsAvailable uint8     `json:"modems_available"`
	ModemsInUse     uint8     `json:"modems_in_use"`
	TestsCompleted  uint32    `json:"tests_completed"`
	TestsFailed     uint32    `json:"tests_failed"`
	LastTestTime    time.Time `json:"last_test_time,omitempty"`
}

// ModemTestResult contains the result of a modem test for a single node
type ModemTestResult struct {
	Zone             uint16   `json:"zone"`
	Net              uint16   `json:"net"`
	Node             uint16   `json:"node"`
	ConflictSequence uint8    `json:"conflict_sequence"`
	TestTime         time.Time `json:"test_time"`

	// EMSI results (same as IFCICO)
	Success        bool     `json:"success"`
	ResponseMs     uint32   `json:"response_ms"`
	SystemName     string   `json:"system_name,omitempty"`
	MailerInfo     string   `json:"mailer_info,omitempty"`
	Addresses      []string `json:"addresses,omitempty"`
	AddressValid   bool     `json:"address_valid"`
	ResponseType   string   `json:"response_type,omitempty"` // REQ, ACK, NAK, CLI, HBT
	SoftwareSource string   `json:"software_source,omitempty"`
	Error          string   `json:"error,omitempty"`

	// Modem-specific fields
	ConnectSpeed   uint32 `json:"connect_speed,omitempty"`   // Actual bps (33600, etc.)
	ModemProtocol  string `json:"modem_protocol,omitempty"`  // V.34, V.92, etc.
	PhoneDialed    string `json:"phone_dialed,omitempty"`
	RingCount      uint8  `json:"ring_count,omitempty"`
	CarrierTimeMs  uint32 `json:"carrier_time_ms,omitempty"`
	ModemUsed      string `json:"modem_used,omitempty"`      // Daemon's local modem ID
	MatchReason    string `json:"match_reason,omitempty"`    // Why this modem was selected
	ModemLineStats string `json:"modem_line_stats,omitempty"`
}

// ModemQueueStats contains statistics about the modem test queue
type ModemQueueStats struct {
	TotalNodes      int            `json:"total_nodes"`
	PendingNodes    int            `json:"pending_nodes"`
	InProgressNodes int            `json:"in_progress_nodes"`
	CompletedNodes  int            `json:"completed_nodes"`
	FailedNodes     int            `json:"failed_nodes"`
	ByDaemon        map[string]int `json:"by_daemon"`
}

// Modem queue status constants
const (
	ModemQueueStatusPending    = "pending"
	ModemQueueStatusInProgress = "in_progress"
	ModemQueueStatusCompleted  = "completed"
	ModemQueueStatusFailed     = "failed"
)

// Modem caller status constants
const (
	ModemCallerStatusActive      = "active"
	ModemCallerStatusInactive    = "inactive"
	ModemCallerStatusMaintenance = "maintenance"
)

// formatAddress formats a FidoNet address
func formatAddress(zone, net, node int) string {
	return fmt.Sprintf("%d:%d/%d", zone, net, node)
}
