package storage

import (
	"time"
)

// Result shapes returned to consumers of this package. Structs that belong to
// one feature live with that feature's code (email_analytics.go,
// point_operations.go, ...); what is left here is the shared set, plus the
// per-report row types that have no other home. The test-result shapes are in
// types_test_results.go and the interfaces in interfaces.go.

// NodeSummary represents a summary of a node for search results
type NodeSummary struct {
	Zone            int       `json:"zone"`
	Net             int       `json:"net"`
	Node            int       `json:"node"`
	Domain          string    `json:"domain,omitempty"`
	SystemName      string    `json:"system_name"`
	Location        string    `json:"location"`
	SysopName       string    `json:"sysop_name"`
	FirstDate       time.Time `json:"first_date"`
	LastDate        time.Time `json:"last_date"`
	CurrentlyActive bool      `json:"currently_active"`
}

// DomainInfo describes one FTN network present in the database
type DomainInfo struct {
	Domain     string    `json:"domain"`
	LatestDate time.Time `json:"latest_date"`
	NodeCount  int       `json:"node_count"`
}

// SysopInfo represents information about a sysop
type SysopInfo struct {
	Name        string    `json:"name"`
	NodeCount   int       `json:"node_count"`
	ActiveNodes int       `json:"active_nodes"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	Zones       []int     `json:"zones"`
}

// SoftwareDistribution represents software distribution statistics
type SoftwareDistribution struct {
	Protocol         string                 `json:"protocol"`
	TotalNodes       int                    `json:"total_nodes"`
	SoftwareTypes    []SoftwareTypeStats    `json:"software_types"`
	VersionBreakdown []SoftwareVersionStats `json:"version_breakdown"`
	OSDistribution   []OSStats              `json:"os_distribution"`
	LastUpdated      time.Time              `json:"last_updated"`
}

// SoftwareTypeStats represents statistics for a software type
type SoftwareTypeStats struct {
	Software   string  `json:"software"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

// SoftwareVersionStats represents statistics for a software version
type SoftwareVersionStats struct {
	Software   string  `json:"software"`
	Version    string  `json:"version"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

// OSStats represents operating system statistics
type OSStats struct {
	OS         string  `json:"os"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

// GeoHostingDistribution represents hosting distribution by geography
type GeoHostingDistribution struct {
	TotalNodes           int             `json:"total_nodes"`
	CountryDistribution  []CountryStats  `json:"country_distribution"`
	ProviderDistribution []ProviderStats `json:"provider_distribution"`
	TopCountries         []CountryStats  `json:"top_countries"` // Top 20
	TopProviders         []ProviderStats `json:"top_providers"` // Top 20
	LastUpdated          time.Time       `json:"last_updated"`
}

// CountryStats represents statistics for a country
type CountryStats struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	NodeCount   int     `json:"node_count"`
	Percentage  float64 `json:"percentage"`
}

// ProviderStats represents statistics for a hosting provider
type ProviderStats struct {
	Provider     string   `json:"provider"`     // ISP name
	Organization string   `json:"organization"` // Org name (optional)
	ASN          uint32   `json:"asn"`          // AS number (optional)
	NodeCount    int      `json:"node_count"`
	Percentage   float64  `json:"percentage"`
	Countries    []string `json:"countries"` // Countries where this provider hosts nodes
}

// BatchInsertConfig holds configuration for batch insert operations
type BatchInsertConfig struct {
	ChunkSize       int  // Number of nodes per chunk
	UseTransactions bool // Whether to wrap inserts in transactions
}

// DefaultBatchInsertConfig returns the default configuration for batch inserts
func DefaultBatchInsertConfig() BatchInsertConfig {
	return BatchInsertConfig{
		ChunkSize:       5000, // Increased from 100 for much better bulk insert performance
		UseTransactions: true,
	}
}

// Constants for default values and limits
const (
	DefaultSearchLimit = 100
	MaxSearchLimit     = 1000
	DefaultChunkSize   = 100
	MaxChunkSize       = 1000
	DefaultSysopLimit  = 100
	MaxSysopLimit      = 200
	DefaultRegionLimit = 10
	DefaultNetLimit    = 10
)

// PSTNNode represents a node with PSTN (phone) access from the nodelist
// Used for PSTN analytics reports showing nodes with phone numbers
type PSTNNode struct {
	Zone            int       `json:"zone"`
	Net             int       `json:"net"`
	Node            int       `json:"node"`
	SystemName      string    `json:"system_name"`
	Location        string    `json:"location"`
	SysopName       string    `json:"sysop_name"`
	Phone           string    `json:"phone"`
	PhoneNormalized string    `json:"phone_normalized"` // Normalized via modem.NormalizePhone
	IsCM            bool      `json:"is_cm"`            // Continuous Mail (24/7 availability)
	NodelistDate    time.Time `json:"nodelist_date"`    // Date of nodelist entry
	NodeType        string    `json:"node_type"`        // Zone, Region, Host, Hub, Pvt, Down, Hold
	MaxSpeed        uint32    `json:"max_speed"`        // Maximum baud rate
	Flags           []string  `json:"flags"`            // Node flags
	ModemFlags      []string  `json:"modem_flags"`      // Modem capability flags (V34, V42B, etc.)
	IsPSTNDead      bool      `json:"is_pstn_dead"`
	PSTNDeadReason  string    `json:"pstn_dead_reason,omitempty"`
}

// PSTNDeadNode represents a node marked as having a dead/disconnected PSTN phone number
type PSTNDeadNode struct {
	Zone     int       `json:"zone"`
	Net      int       `json:"net"`
	Node     int       `json:"node"`
	Reason   string    `json:"reason"`
	MarkedBy string    `json:"marked_by"`
	MarkedAt time.Time `json:"marked_at"`
}

// OnThisDayNode represents a node that was first added on this day in a previous year
// It tracks when a new sysop appeared with a node address that wasn't theirs before
type OnThisDayNode struct {
	Zone          int       `json:"zone"`
	Net           int       `json:"net"`
	Node          int       `json:"node"`
	SysopName     string    `json:"sysop_name"`
	SystemName    string    `json:"system_name"`
	Location      string    `json:"location"`
	FirstAppeared time.Time `json:"first_appeared"` // When this sysop first got this node
	LastSeen      time.Time `json:"last_seen"`      // Final appearance (ignoring temporary gaps)
	YearsActive   int       `json:"years_active"`   // Years from first to last appearance
	StillActive   bool      `json:"still_active"`   // Whether still in latest nodelist
	RawLine       string    `json:"raw_line"`       // Original nodelist line from first appearance
}

// FileRequestNode represents a node with file request capabilities (XA, XB, XC, XP, XR, XW, XX)
// Used for File Request analytics reports based on FTS-5001 specification
type FileRequestNode struct {
	Zone            int       `json:"zone"`
	Net             int       `json:"net"`
	Node            int       `json:"node"`
	SystemName      string    `json:"system_name"`
	Location        string    `json:"location"`
	SysopName       string    `json:"sysop_name"`
	FileRequestFlag string    `json:"file_request_flag"` // XA, XB, XC, XP, XR, XW, or XX
	NodelistDate    time.Time `json:"nodelist_date"`
	NodeType        string    `json:"node_type"`
	Flags           []string  `json:"flags"`
}

// ModemAccessibleNode represents a node successfully reached via modem (PSTN) test
// Used for the PSTN Accessible Nodes analytics report showing verified modem connectivity
type ModemAccessibleNode struct {
	Zone                int       `json:"zone"`
	Net                 int       `json:"net"`
	Node                int       `json:"node"`
	Address             string    `json:"address"`
	TestTime            time.Time `json:"test_time"`
	ModemPhoneDialed    string    `json:"modem_phone_dialed"`
	ModemConnectSpeed   uint32    `json:"modem_connect_speed"`
	ModemProtocol       string    `json:"modem_protocol"`
	ModemSystemName     string    `json:"modem_system_name"`
	ModemMailerInfo     string    `json:"modem_mailer_info"`
	ModemOperatorName   string    `json:"modem_operator_name"`
	ModemConnectString  string    `json:"modem_connect_string"`
	ModemResponseMs     uint32    `json:"modem_response_ms"`
	ModemAddressValid   bool      `json:"modem_address_valid"`
	ModemRemoteLocation string    `json:"modem_remote_location"`
	ModemRemoteSysop    string    `json:"modem_remote_sysop"`
	ModemTxSpeed        uint32    `json:"modem_tx_speed"`
	ModemRxSpeed        uint32    `json:"modem_rx_speed"`
	ModemModulation     string    `json:"modem_modulation"`
	TestSource          string    `json:"test_source"`
}

// ModemNoAnswerNode represents a node that was tested via modem but never answered
// Used for the PSTN No Answer analytics report showing nodes that are always unreachable
type ModemNoAnswerNode struct {
	Zone                int       `json:"zone"`
	Net                 int       `json:"net"`
	Node                int       `json:"node"`
	Address             string    `json:"address"`
	TestTime            time.Time `json:"test_time"`
	ModemPhoneDialed    string    `json:"modem_phone_dialed"`
	ModemOperatorName   string    `json:"modem_operator_name"`
	ModemAstDisposition string    `json:"modem_ast_disposition"`
	ModemAstHangupCause uint8     `json:"modem_ast_hangup_cause"`
	TestSource          string    `json:"test_source"`
	AttemptCount        uint32    `json:"attempt_count"`
	IsPSTNDead          bool      `json:"is_pstn_dead"`
	PSTNDeadReason      string    `json:"pstn_dead_reason,omitempty"`
}

// ModemTestDetail represents detailed modem test data for a single test result
type ModemTestDetail struct {
	// Basic identification
	Zone       int       `json:"zone"`
	Net        int       `json:"net"`
	Node       int       `json:"node"`
	Address    string    `json:"address"`
	TestTime   time.Time `json:"test_time"`
	TestSource string    `json:"test_source"`

	// Connection info
	ConnectSpeed  uint32 `json:"modem_connect_speed"`
	Protocol      string `json:"modem_protocol"`
	PhoneDialed   string `json:"modem_phone_dialed"`
	RingCount     uint8  `json:"modem_ring_count"`
	CarrierTimeMs uint32 `json:"modem_carrier_time_ms"`
	ConnectString string `json:"modem_connect_string"`
	ResponseMs    uint32 `json:"modem_response_ms"`

	// EMSI handshake
	SystemName     string   `json:"modem_system_name"`
	MailerInfo     string   `json:"modem_mailer_info"`
	Addresses      []string `json:"modem_addresses"`
	AddressValid   bool     `json:"modem_address_valid"`
	ResponseType   string   `json:"modem_response_type"`
	RemoteLocation string   `json:"modem_remote_location"`
	RemoteSysop    string   `json:"modem_remote_sysop"`
	Error          string   `json:"modem_error"`

	// Operator routing
	OperatorName   string `json:"modem_operator_name"`
	OperatorPrefix string `json:"modem_operator_prefix"`
	DialTimeMs     uint32 `json:"modem_dial_time_ms"`
	EmsiTimeMs     uint32 `json:"modem_emsi_time_ms"`

	// Line statistics
	TxSpeed           uint32  `json:"modem_tx_speed"`
	RxSpeed           uint32  `json:"modem_rx_speed"`
	Compression       string  `json:"modem_compression"`
	Modulation        string  `json:"modem_modulation"`
	LineQuality       uint8   `json:"modem_line_quality"`
	SNR               float32 `json:"modem_snr"`
	RxLevel           int16   `json:"modem_rx_level"`
	TxPower           int16   `json:"modem_tx_power"`
	RoundTripDelay    uint16  `json:"modem_round_trip_delay"`
	LocalRetrains     uint8   `json:"modem_local_retrains"`
	RemoteRetrains    uint8   `json:"modem_remote_retrains"`
	TerminationReason string  `json:"modem_termination_reason"`
	StatsNotes        string  `json:"modem_stats_notes"`
	RawLineStats      string  `json:"modem_line_stats"`

	// AudioCodes CDR
	CdrSessionId        string `json:"modem_cdr_session_id"`
	CdrCodec            string `json:"modem_cdr_codec"`
	CdrRtpJitterMs      uint16 `json:"modem_cdr_rtp_jitter_ms"`
	CdrRtpDelayMs       uint16 `json:"modem_cdr_rtp_delay_ms"`
	CdrPacketLoss       uint8  `json:"modem_cdr_packet_loss"`
	CdrRemotePacketLoss uint8  `json:"modem_cdr_remote_packet_loss"`
	CdrLocalMos         uint8  `json:"modem_cdr_local_mos"`
	CdrRemoteMos        uint8  `json:"modem_cdr_remote_mos"`
	CdrLocalRFactor     uint8  `json:"modem_cdr_local_r_factor"`
	CdrRemoteRFactor    uint8  `json:"modem_cdr_remote_r_factor"`
	CdrTermReason       string `json:"modem_cdr_term_reason"`
	CdrTermCategory     string `json:"modem_cdr_term_category"`

	// Asterisk CDR
	AstDisposition  string `json:"modem_ast_disposition"`
	AstPeer         string `json:"modem_ast_peer"`
	AstDuration     uint16 `json:"modem_ast_duration"`
	AstBillsec      uint16 `json:"modem_ast_billsec"`
	AstHangupCause  uint8  `json:"modem_ast_hangup_cause"`
	AstHangupSource string `json:"modem_ast_hangup_source"`
	AstEarlyMedia   bool   `json:"modem_ast_early_media"`

	// Test metadata
	CallerID    string `json:"modem_caller_id"`
	ModemUsed   string `json:"modem_used"`
	MatchReason string `json:"modem_match_reason"`
}

// IPv6NodeListEntry represents a node for the IPv6 node list report (Michiel's format)
type IPv6NodeListEntry struct {
	Zone         int       `json:"zone"`
	Net          int       `json:"net"`
	Node         int       `json:"node"`
	SysopName    string    `json:"sysop_name"`
	ResolvedIPv6 []string  `json:"resolved_ipv6"`
	ISP          string    `json:"isp"`
	Org          string    `json:"org"`
	TestTime     time.Time `json:"test_time"`

	// Raw IPv4 status for INO4 detection
	BinkPIPv4Success  bool `json:"binkp_ipv4_success"`
	IfcicoIPv4Success bool `json:"ifcico_ipv4_success"`
	TelnetIPv4Success bool `json:"telnet_ipv4_success"`

	// Computed fields (populated in Go after query)
	IPv6Type     string `json:"ipv6_type"`      // "Native", "T-6in4", "T-6to4", "T-Teredo"
	Provider     string `json:"provider"`       // Cleaned ISP/Org name
	HasFidoAddr  bool   `json:"has_fido_addr"`  // f flag: has ::f1d0:z:n:nn style address
	FidoIPv6Addr string `json:"fido_ipv6_addr"` // The actual f1d0 IPv6 address found
	HasNoIPv4    bool   `json:"has_no_ipv4"`    // INO4: no working IPv4
	IsUnstable   bool   `json:"is_unstable"`    // 6UNS: failed >2 times in 30 days
	Remarks      string `json:"remarks"`        // Combined remarks string
}
