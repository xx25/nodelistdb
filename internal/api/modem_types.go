package api

// SubmitResultsRequest is the request body for POST /api/modem/results/direct
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
	ConnectSpeed   uint32 `json:"connect_speed,omitempty"`
	ModemProtocol  string `json:"modem_protocol,omitempty"`
	PhoneDialed    string `json:"phone_dialed,omitempty"`
	RingCount      uint8  `json:"ring_count,omitempty"`
	CarrierTimeMs  uint32 `json:"carrier_time_ms,omitempty"`
	ModemUsed      string `json:"modem_used,omitempty"`
	MatchReason    string `json:"match_reason,omitempty"`
	ModemLineStats string `json:"modem_line_stats,omitempty"`

	// Extended fields for CLI submissions
	LineStats     *LineStatsRequest     `json:"line_stats,omitempty"`
	AudioCodesCDR *AudioCodesCDRRequest `json:"audiocodes_cdr,omitempty"`
	AsteriskCDR   *AsteriskCDRRequest   `json:"asterisk_cdr,omitempty"`

	// Operator routing info
	OperatorName   string `json:"operator_name,omitempty"`
	OperatorPrefix string `json:"operator_prefix,omitempty"`
	DialTimeMs     uint32 `json:"dial_time_ms,omitempty"`
	EMSITimeMs     uint32 `json:"emsi_time_ms,omitempty"`
	ConnectString  string `json:"connect_string,omitempty"`

	// EMSI remote system details
	RemoteLocation string `json:"remote_location,omitempty"`
	RemoteSysop    string `json:"remote_sysop,omitempty"`
}

// LineStatsRequest contains parsed modem line statistics
type LineStatsRequest struct {
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

// AudioCodesCDRRequest contains VoIP quality metrics from AudioCodes gateway
type AudioCodesCDRRequest struct {
	SessionID        string `json:"session_id,omitempty"`
	Codec            string `json:"codec,omitempty"`
	RTPJitterMs      uint16 `json:"rtp_jitter_ms,omitempty"`
	RTPDelayMs       uint16 `json:"rtp_delay_ms,omitempty"`
	PacketLoss       uint8  `json:"packet_loss,omitempty"`
	RemotePacketLoss uint8  `json:"remote_packet_loss,omitempty"`
	LocalMOS         uint8  `json:"local_mos,omitempty"`  // MOS * 10 (e.g., 43 = 4.3)
	RemoteMOS        uint8  `json:"remote_mos,omitempty"` // MOS * 10
	LocalRFactor     uint8  `json:"local_r_factor,omitempty"`
	RemoteRFactor    uint8  `json:"remote_r_factor,omitempty"`
	TermReason       string `json:"term_reason,omitempty"`
	TermCategory     string `json:"term_category,omitempty"`
}

// AsteriskCDRRequest contains call routing info from Asterisk
type AsteriskCDRRequest struct {
	Disposition  string `json:"disposition,omitempty"` // ANSWERED, NO ANSWER, BUSY, FAILED
	Peer         string `json:"peer,omitempty"`        // Outbound peer/trunk name
	Duration     uint16 `json:"duration,omitempty"`    // Total duration (ring + talk)
	BillSec      uint16 `json:"billsec,omitempty"`     // Billable seconds (talk time)
	HangupCause  uint8  `json:"hangup_cause,omitempty"`
	HangupSource string `json:"hangup_source,omitempty"`
	EarlyMedia   bool   `json:"early_media,omitempty"`
}

// SubmitResultsResponse is the response for POST /api/modem/results/direct
type SubmitResultsResponse struct {
	Accepted int `json:"accepted"`
	Stored   int `json:"stored"`
}
