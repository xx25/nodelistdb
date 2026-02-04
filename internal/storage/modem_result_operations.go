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
	TestSource       string // "daemon", "cli", "manual"

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
	ConnectSpeed   uint32
	ModemProtocol  string
	PhoneDialed    string
	RingCount      uint8
	CarrierTimeMs  uint32
	ModemUsed      string
	MatchReason    string
	ModemLineStats string

	// Operator/routing info
	OperatorName   string
	OperatorPrefix string
	DialTimeMs     uint32
	EMSITimeMs     uint32
	ConnectString  string

	// Line statistics (parsed from AT commands)
	TXSpeed           uint32
	RXSpeed           uint32
	Compression       string
	Modulation        string
	LineQuality       uint8
	SNR               float32
	RxLevel           int16
	TxPower           int16
	RoundTripDelay    uint16
	LocalRetrains     uint8
	RemoteRetrains    uint8
	TerminationReason string
	StatsNotes        string

	// EMSI remote system details
	RemoteLocation string
	RemoteSysop    string

	// AudioCodes VoIP CDR quality metrics
	CDRSessionID        string
	CDRCodec            string
	CDRRTPJitterMs      uint16
	CDRRTPDelayMs       uint16
	CDRPacketLoss       uint8
	CDRRemotePacketLoss uint8
	CDRLocalMOS         uint8 // MOS * 10 (e.g., 43 = 4.3)
	CDRRemoteMOS        uint8
	CDRLocalRFactor     uint8
	CDRRemoteRFactor    uint8
	CDRTermReason       string
	CDRTermCategory     string

	// Asterisk CDR call routing info
	AstDisposition  string
	AstPeer         string
	AstDuration     uint16
	AstBillSec      uint16
	AstHangupCause  uint8
	AstHangupSource string
	AstEarlyMedia   bool
}

// StoreModemTestResult stores a modem test result in the node_test_results table
func (m *ModemResultOperations) StoreModemTestResult(ctx context.Context, input *ModemTestResultInput) error {
	now := time.Now()
	if input.TestTime.IsZero() {
		input.TestTime = now
	}

	// Default test source to "daemon" for backward compatibility
	testSource := input.TestSource
	if testSource == "" {
		testSource = "daemon"
	}

	// Compute FidoNet address string
	address := fmt.Sprintf("%d:%d/%d", input.Zone, input.Net, input.Node)

	query := `
		INSERT INTO node_test_results (
			test_time, zone, net, node, address, test_source,
			modem_tested, modem_success, modem_response_ms,
			modem_system_name, modem_mailer_info, modem_addresses,
			modem_connect_speed, modem_protocol, modem_caller_id,
			modem_phone_dialed, modem_ring_count, modem_carrier_time_ms,
			modem_error, modem_address_valid, modem_response_type,
			modem_software_source, modem_used, modem_match_reason, modem_line_stats,
			modem_operator_name, modem_operator_prefix, modem_dial_time_ms,
			modem_emsi_time_ms, modem_connect_string,
			modem_tx_speed, modem_rx_speed, modem_compression, modem_modulation,
			modem_line_quality, modem_snr, modem_rx_level, modem_tx_power,
			modem_round_trip_delay, modem_local_retrains, modem_remote_retrains,
			modem_termination_reason, modem_stats_notes,
			modem_remote_location, modem_remote_sysop,
			modem_cdr_session_id, modem_cdr_codec,
			modem_cdr_rtp_jitter_ms, modem_cdr_rtp_delay_ms,
			modem_cdr_packet_loss, modem_cdr_remote_packet_loss,
			modem_cdr_local_mos, modem_cdr_remote_mos,
			modem_cdr_local_r_factor, modem_cdr_remote_r_factor,
			modem_cdr_term_reason, modem_cdr_term_category,
			modem_ast_disposition, modem_ast_peer,
			modem_ast_duration, modem_ast_billsec,
			modem_ast_hangup_cause, modem_ast_hangup_source, modem_ast_early_media,
			is_operational, hostname
		) VALUES (
			?, ?, ?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?,
			?, ?,
			?, ?,
			?, ?,
			?, ?,
			?, ?,
			?, ?,
			?, ?,
			?, ?,
			?, ?,
			?, ?, ?,
			?, ?
		)
	`

	_, err := m.db.Conn().ExecContext(ctx, query,
		input.TestTime, input.Zone, input.Net, input.Node, address, testSource,
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
		input.OperatorName,
		input.OperatorPrefix,
		input.DialTimeMs,
		input.EMSITimeMs,
		input.ConnectString,
		input.TXSpeed,
		input.RXSpeed,
		input.Compression,
		input.Modulation,
		input.LineQuality,
		input.SNR,
		input.RxLevel,
		input.TxPower,
		input.RoundTripDelay,
		input.LocalRetrains,
		input.RemoteRetrains,
		input.TerminationReason,
		input.StatsNotes,
		input.RemoteLocation,
		input.RemoteSysop,
		input.CDRSessionID,
		input.CDRCodec,
		input.CDRRTPJitterMs,
		input.CDRRTPDelayMs,
		input.CDRPacketLoss,
		input.CDRRemotePacketLoss,
		input.CDRLocalMOS,
		input.CDRRemoteMOS,
		input.CDRLocalRFactor,
		input.CDRRemoteRFactor,
		input.CDRTermReason,
		input.CDRTermCategory,
		input.AstDisposition,
		input.AstPeer,
		input.AstDuration,
		input.AstBillSec,
		input.AstHangupCause,
		input.AstHangupSource,
		input.AstEarlyMedia,
		input.Success, // is_operational = success for modem tests
		"modem",       // hostname indicator
	)

	if err != nil {
		return fmt.Errorf("failed to store modem test result: %w", err)
	}

	return nil
}
