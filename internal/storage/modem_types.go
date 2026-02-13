package storage

import (
	"time"
)

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
