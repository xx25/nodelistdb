package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/database"
)

// ModemResultOperations handles modem test result storage
type ModemResultOperations struct {
	db database.DatabaseInterface
}

// NewModemResultOperations creates a new ModemResultOperations instance
func NewModemResultOperations(db database.DatabaseInterface) *ModemResultOperations {
	return &ModemResultOperations{db: db}
}

// ModemTestResultInput contains input data for storing a modem test result
type ModemTestResultInput struct {
	Zone             uint16
	Net              uint16
	Node             uint16
	ConflictSequence uint8
	TestTime         time.Time
	CallerID         string

	// Result data
	Success        bool
	ResponseMs     uint32
	SystemName     string
	MailerInfo     string
	Addresses      []string
	AddressValid   bool
	ResponseType   string
	SoftwareSource string
	Error          string

	// Modem-specific fields
	ConnectSpeed  uint32
	ModemProtocol string
	PhoneDialed   string
	RingCount     uint8
	CarrierTimeMs uint32
	ModemUsed     string
	MatchReason   string
	ModemLineStats string
}

// StoreModemTestResult stores a modem test result in the node_test_results table
func (m *ModemResultOperations) StoreModemTestResult(ctx context.Context, input *ModemTestResultInput) error {
	now := time.Now()
	if input.TestTime.IsZero() {
		input.TestTime = now
	}

	// Compute FidoNet address string
	address := fmt.Sprintf("%d:%d/%d", input.Zone, input.Net, input.Node)

	query := `
		INSERT INTO node_test_results (
			test_time, zone, net, node, address,
			modem_tested, modem_success, modem_response_ms,
			modem_system_name, modem_mailer_info, modem_addresses,
			modem_connect_speed, modem_protocol, modem_caller_id,
			modem_phone_dialed, modem_ring_count, modem_carrier_time_ms,
			modem_error, modem_address_valid, modem_response_type,
			modem_software_source, modem_used, modem_match_reason, modem_line_stats,
			is_operational, hostname
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := m.db.Conn().ExecContext(ctx, query,
		input.TestTime, input.Zone, input.Net, input.Node, address,
		true, // modem_tested
		input.Success,
		input.ResponseMs,
		input.SystemName,
		input.MailerInfo,
		input.Addresses,
		input.ConnectSpeed,
		input.ModemProtocol,
		input.CallerID,
		input.PhoneDialed,
		input.RingCount,
		input.CarrierTimeMs,
		input.Error,
		input.AddressValid,
		input.ResponseType,
		input.SoftwareSource,
		input.ModemUsed,
		input.MatchReason,
		input.ModemLineStats,
		input.Success, // is_operational = success for modem tests
		"modem",       // hostname indicator
	)

	if err != nil {
		return fmt.Errorf("failed to store modem test result: %w", err)
	}

	return nil
}
