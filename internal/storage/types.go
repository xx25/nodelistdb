package storage

import (
	"time"

	"github.com/nodelistdb/internal/database"
)

// NodeSummary represents a summary of a node for search results
type NodeSummary struct {
	Zone            int       `json:"zone"`
	Net             int       `json:"net"`
	Node            int       `json:"node"`
	SystemName      string    `json:"system_name"`
	Location        string    `json:"location"`
	SysopName       string    `json:"sysop_name"`
	FirstDate       time.Time `json:"first_date"`
	LastDate        time.Time `json:"last_date"`
	CurrentlyActive bool      `json:"currently_active"`
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
	Protocol         string                  `json:"protocol"`
	TotalNodes       int                     `json:"total_nodes"`
	SoftwareTypes    []SoftwareTypeStats     `json:"software_types"`
	VersionBreakdown []SoftwareVersionStats  `json:"version_breakdown"`
	OSDistribution   []OSStats               `json:"os_distribution"`
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
	TotalNodes           int              `json:"total_nodes"`
	CountryDistribution  []CountryStats   `json:"country_distribution"`
	ProviderDistribution []ProviderStats  `json:"provider_distribution"`
	TopCountries         []CountryStats   `json:"top_countries"`  // Top 20
	TopProviders         []ProviderStats  `json:"top_providers"`  // Top 20
	LastUpdated          time.Time        `json:"last_updated"`
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
	Provider     string   `json:"provider"`      // ISP name
	Organization string   `json:"organization"`  // Org name (optional)
	ASN          uint32   `json:"asn"`           // AS number (optional)
	NodeCount    int      `json:"node_count"`
	Percentage   float64  `json:"percentage"`
	Countries    []string `json:"countries"` // Countries where this provider hosts nodes
}

// NodeTestResult represents a test result for a node
type NodeTestResult struct {
	TestTime              time.Time `json:"test_time"`
	Zone                  int       `json:"zone"`
	Net                   int       `json:"net"`
	Node                  int       `json:"node"`
	Address               string    `json:"address"`
	Hostname              string    `json:"hostname"`
	ResolvedIPv4          []string  `json:"resolved_ipv4"`
	ResolvedIPv6          []string  `json:"resolved_ipv6"`
	DNSError              string    `json:"dns_error"`

	// Geolocation
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`
	Region      string  `json:"region"`
	Latitude    float32 `json:"latitude"`
	Longitude   float32 `json:"longitude"`
	ISP         string  `json:"isp"`
	Org         string  `json:"org"`
	ASN         uint32  `json:"asn"`

	// BinkP Test Results
	BinkPTested       bool     `json:"binkp_tested"`
	BinkPSuccess      bool     `json:"binkp_success"`
	BinkPResponseMs   uint32   `json:"binkp_response_ms"`
	BinkPSystemName   string   `json:"binkp_system_name"`
	BinkPSysop        string   `json:"binkp_sysop"`
	BinkPLocation     string   `json:"binkp_location"`
	BinkPVersion      string   `json:"binkp_version"`
	BinkPAddresses    []string `json:"binkp_addresses"`
	BinkPCapabilities []string `json:"binkp_capabilities"`
	BinkPError        string   `json:"binkp_error"`

	// IFCICO Test Results
	IfcicoTested       bool     `json:"ifcico_tested"`
	IfcicoSuccess      bool     `json:"ifcico_success"`
	IfcicoResponseMs   uint32   `json:"ifcico_response_ms"`
	IfcicoMailerInfo   string   `json:"ifcico_mailer_info"`
	IfcicoSystemName   string   `json:"ifcico_system_name"`
	IfcicoAddresses    []string `json:"ifcico_addresses"`
	IfcicoResponseType string   `json:"ifcico_response_type"`
	IfcicoError        string   `json:"ifcico_error"`

	// Telnet Test Results
	TelnetTested     bool   `json:"telnet_tested"`
	TelnetSuccess    bool   `json:"telnet_success"`
	TelnetResponseMs uint32 `json:"telnet_response_ms"`
	TelnetError      string `json:"telnet_error"`

	// FTP Test Results
	FTPTested      bool   `json:"ftp_tested"`
	FTPSuccess     bool   `json:"ftp_success"`
	FTPResponseMs  uint32 `json:"ftp_response_ms"`
	FTPError       string `json:"ftp_error"`
	FTPAnonSuccess *bool  `json:"ftp_anon_success"` // nil=not attempted, true=success, false=rejected

	// VModem Test Results
	VModemTested     bool   `json:"vmodem_tested"`
	VModemSuccess    bool   `json:"vmodem_success"`
	VModemResponseMs uint32 `json:"vmodem_response_ms"`
	VModemError      string `json:"vmodem_error"`

	// IPv4-specific Test Results
	BinkPIPv4Tested      bool   `json:"binkp_ipv4_tested"`
	BinkPIPv4Success     bool   `json:"binkp_ipv4_success"`
	BinkPIPv4ResponseMs  uint32 `json:"binkp_ipv4_response_ms"`
	BinkPIPv4Address     string `json:"binkp_ipv4_address"`
	BinkPIPv4Error       string `json:"binkp_ipv4_error"`
	IfcicoIPv4Tested     bool   `json:"ifcico_ipv4_tested"`
	IfcicoIPv4Success    bool   `json:"ifcico_ipv4_success"`
	IfcicoIPv4ResponseMs uint32 `json:"ifcico_ipv4_response_ms"`
	IfcicoIPv4Address    string `json:"ifcico_ipv4_address"`
	IfcicoIPv4Error      string `json:"ifcico_ipv4_error"`
	TelnetIPv4Tested     bool   `json:"telnet_ipv4_tested"`
	TelnetIPv4Success    bool   `json:"telnet_ipv4_success"`
	TelnetIPv4ResponseMs uint32 `json:"telnet_ipv4_response_ms"`
	TelnetIPv4Address    string `json:"telnet_ipv4_address"`
	TelnetIPv4Error      string `json:"telnet_ipv4_error"`
	FTPIPv4Tested        bool   `json:"ftp_ipv4_tested"`
	FTPIPv4Success       bool   `json:"ftp_ipv4_success"`
	FTPIPv4ResponseMs    uint32 `json:"ftp_ipv4_response_ms"`
	FTPIPv4Address       string `json:"ftp_ipv4_address"`
	FTPIPv4Error         string `json:"ftp_ipv4_error"`
	VModemIPv4Tested     bool   `json:"vmodem_ipv4_tested"`
	VModemIPv4Success    bool   `json:"vmodem_ipv4_success"`
	VModemIPv4ResponseMs uint32 `json:"vmodem_ipv4_response_ms"`
	VModemIPv4Address    string `json:"vmodem_ipv4_address"`
	VModemIPv4Error      string `json:"vmodem_ipv4_error"`

	// IPv6-specific Test Results
	BinkPIPv6Tested      bool   `json:"binkp_ipv6_tested"`
	BinkPIPv6Success     bool   `json:"binkp_ipv6_success"`
	BinkPIPv6ResponseMs  uint32 `json:"binkp_ipv6_response_ms"`
	BinkPIPv6Address     string `json:"binkp_ipv6_address"`
	BinkPIPv6Error       string `json:"binkp_ipv6_error"`
	IfcicoIPv6Tested     bool   `json:"ifcico_ipv6_tested"`
	IfcicoIPv6Success    bool   `json:"ifcico_ipv6_success"`
	IfcicoIPv6ResponseMs uint32 `json:"ifcico_ipv6_response_ms"`
	IfcicoIPv6Address    string `json:"ifcico_ipv6_address"`
	IfcicoIPv6Error      string `json:"ifcico_ipv6_error"`
	TelnetIPv6Tested     bool   `json:"telnet_ipv6_tested"`
	TelnetIPv6Success    bool   `json:"telnet_ipv6_success"`
	TelnetIPv6ResponseMs uint32 `json:"telnet_ipv6_response_ms"`
	TelnetIPv6Address    string `json:"telnet_ipv6_address"`
	TelnetIPv6Error      string `json:"telnet_ipv6_error"`
	FTPIPv6Tested        bool   `json:"ftp_ipv6_tested"`
	FTPIPv6Success       bool   `json:"ftp_ipv6_success"`
	FTPIPv6ResponseMs    uint32 `json:"ftp_ipv6_response_ms"`
	FTPIPv6Address       string `json:"ftp_ipv6_address"`
	FTPIPv6Error         string `json:"ftp_ipv6_error"`
	VModemIPv6Tested     bool   `json:"vmodem_ipv6_tested"`
	VModemIPv6Success    bool   `json:"vmodem_ipv6_success"`
	VModemIPv6ResponseMs uint32 `json:"vmodem_ipv6_response_ms"`
	VModemIPv6Address    string `json:"vmodem_ipv6_address"`
	VModemIPv6Error      string `json:"vmodem_ipv6_error"`

	IsOperational         bool `json:"is_operational"`
	HasConnectivityIssues bool `json:"has_connectivity_issues"`
	AddressValidated      bool `json:"address_validated"`

	// Per-hostname testing fields (simplified migration)
	TestedHostname        string   `json:"tested_hostname"`         // Which hostname was tested
	HostnameIndex         int32    `json:"hostname_index"`          // -1=legacy, 0=primary, 1+=backup
	IsAggregated          bool     `json:"is_aggregated"`           // false=per-hostname, true=summary
	TotalHostnames        int32    `json:"total_hostnames"`         // Total number of hostnames for this node
	HostnamesTested       int32    `json:"hostnames_tested"`        // Number of hostnames actually tested
	HostnamesOperational  int32    `json:"hostnames_operational"`   // Number of operational hostnames
	AllHostnames          []string `json:"all_hostnames"`           // All hostnames for this node (for display)
}

// NodeReachabilityStats represents aggregated reachability statistics for a node
type NodeReachabilityStats struct {
	Zone                  int       `json:"zone"`
	Net                   int       `json:"net"`
	Node                  int       `json:"node"`
	TotalTests            int       `json:"total_tests"`
	FullySuccessfulTests  int       `json:"fully_successful_tests"`
	PartiallyFailedTests  int       `json:"partially_failed_tests"`
	FailedTests           int       `json:"failed_tests"`
	SuccessfulTests       int       `json:"successful_tests"` // For backward compatibility (operational)
	SuccessRate           float64   `json:"success_rate"`
	AverageResponseMs     float64   `json:"average_response_ms"`
	LastTestTime          time.Time `json:"last_test_time"`
	LastStatus            string    `json:"last_status"`
	BinkPSuccessRate      float64   `json:"binkp_success_rate"`      // Combined (IPv4 OR IPv6)
	IfcicoSuccessRate     float64   `json:"ifcico_success_rate"`     // Combined (IPv4 OR IPv6)
	TelnetSuccessRate     float64   `json:"telnet_success_rate"`     // Combined (IPv4 OR IPv6)
	BinkPIPv4SuccessRate  float64   `json:"binkp_ipv4_success_rate"` // IPv4-only
	IfcicoIPv4SuccessRate float64   `json:"ifcico_ipv4_success_rate"`// IPv4-only
	TelnetIPv4SuccessRate float64   `json:"telnet_ipv4_success_rate"`// IPv4-only
	BinkPIPv6SuccessRate  float64   `json:"binkp_ipv6_success_rate"` // IPv6-only
	IfcicoIPv6SuccessRate float64   `json:"ifcico_ipv6_success_rate"`// IPv6-only
	TelnetIPv6SuccessRate float64   `json:"telnet_ipv6_success_rate"`// IPv6-only
}

// ReachabilityTrend represents reachability trend over time
type ReachabilityTrend struct {
	Date              time.Time `json:"date"`
	TotalNodes        int       `json:"total_nodes"`
	OperationalNodes  int       `json:"operational_nodes"`
	FailedNodes       int       `json:"failed_nodes"`
	SuccessRate       float64   `json:"success_rate"`
	AvgResponseMs     float64   `json:"avg_response_ms"`
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

// Operations interface defines the contract for storage operations.
// After refactoring, this interface exposes component accessors for cleaner API organization.
// Instead of having 60+ delegation methods, consumers access operations through:
// - NodeOps() for node CRUD operations
// - SearchOps() for search and queries
// - StatsOps() for statistics
// - AnalyticsOps() for historical analytics
// - TestOps() for node testing and reachability
type Operations interface {
	// Component accessors - provides direct access to specialized operation components
	NodeOps() *NodeOperations
	SearchOps() *SearchOperations
	StatsOps() *StatisticsOperations
	AnalyticsOps() *AnalyticsOperations
	TestOps() *TestOperationsRefactored

	// Legacy delegation methods for backward compatibility
	// These methods delegate to the appropriate component operations.
	// New code should use the component accessors directly (e.g., storage.NodeOps().GetNodes())
	// but these are kept for backward compatibility with existing code.

	// Node operations (delegated to NodeOps())
	GetNodes(filter database.NodeFilter) ([]database.Node, error)
	GetNodeHistory(zone, net, node int) ([]database.Node, error)
	GetNodeDateRange(zone, net, node int) (firstDate, lastDate time.Time, err error)
	InsertNodes(nodes []database.Node) error

	// Search operations (delegated to SearchOps())
	SearchNodesBySysop(sysopName string, limit int) ([]NodeSummary, error)
	GetNodeChanges(zone, net, node int) ([]database.NodeChange, error)
	GetUniqueSysops(nameFilter string, limit, offset int) ([]SysopInfo, error)
	GetNodesBySysop(sysopName string, limit int) ([]database.Node, error)
	SearchNodesWithLifetime(filter database.NodeFilter) ([]NodeSummary, error)

	// Analytics operations (delegated to AnalyticsOps())
	GetFlagFirstAppearance(flagName string) (*FlagFirstAppearance, error)
	GetFlagUsageByYear(flagName string) ([]FlagUsageByYear, error)
	GetNetworkHistory(zone, net int) (*NetworkHistory, error)

	// Statistics operations (delegated to StatsOps())
	GetStats(date time.Time) (*database.NetworkStats, error)
	GetLatestStatsDate() (time.Time, error)
	GetAvailableDates() ([]time.Time, error)
	GetNearestAvailableDate(requestedDate time.Time) (time.Time, error)

	// Test operations (delegated to TestOps())
	GetNodeTestHistory(zone, net, node int, days int) ([]NodeTestResult, error)
	GetDetailedTestResult(zone, net, node int, testTime string) (*NodeTestResult, error)
	GetNodeReachabilityStats(zone, net, node int, days int) (*NodeReachabilityStats, error)
	GetReachabilityTrends(days int) ([]ReachabilityTrend, error)
	SearchNodesByReachability(operational bool, limit int, days int) ([]NodeTestResult, error)
	GetIPv6EnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetIPv6NonWorkingNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetIPv6AdvertisedIPv4OnlyNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetIPv6OnlyNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetPureIPv6OnlyNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetIPv6WeeklyNews(limit int, includeZeroNodes bool) (*IPv6WeeklyNews, error)
	GetBinkPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetIfcicoEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetTelnetEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetVModemEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetFTPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetAKAMismatchNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetIPv6IncorrectIPv4CorrectNodes(limit int, days int, includeZeroNodes bool) ([]AKAIPVersionMismatchNode, error)
	GetIPv4IncorrectIPv6CorrectNodes(limit int, days int, includeZeroNodes bool) ([]AKAIPVersionMismatchNode, error)
	GetOtherNetworksSummary(days int) ([]OtherNetworkSummary, error)
	GetNodesInNetwork(networkName string, limit int, days int) ([]OtherNetworkNode, error)
	GetBinkPSoftwareDistribution(days int) (*SoftwareDistribution, error)
	GetIFCICOSoftwareDistribution(days int) (*SoftwareDistribution, error)
	GetBinkdDetailedStats(days int) (*SoftwareDistribution, error)
	GetGeoHostingDistribution(days int) (*GeoHostingDistribution, error)
	GetNodesByCountry(countryCode string, days int) ([]NodeTestResult, error)
	GetNodesByProvider(provider string, days int) ([]NodeTestResult, error)
	GetOnThisDayNodes(month, day, limit int, activeOnly bool) ([]OnThisDayNode, error)
	GetPioneersByRegion(zone, region, limit int) ([]PioneerNode, error)
	GetPSTNCMNodes(limit int) ([]PSTNNode, error)
	GetPSTNNodes(limit int, zone int) ([]PSTNNode, error)
	MarkPSTNDead(zone, net, node int, reason, markedBy string) error
	UnmarkPSTNDead(zone, net, node int, markedBy string) error
	GetPSTNDeadNodes() ([]PSTNDeadNode, error)
	GetFileRequestNodes(limit int) ([]FileRequestNode, error)
	GetModemAccessibleNodes(limit int, days int, includeZeroNodes bool) ([]ModemAccessibleNode, error)
	GetModemNoAnswerNodes(limit int, days int, includeZeroNodes bool) ([]ModemNoAnswerNode, error)
	GetRecentModemSuccessPhones(days int) ([]string, error)
	GetDetailedModemTestResult(zone, net, node int, testTime string) (*ModemTestDetail, error)
	GetIPv6NodeList(limit int, days int, includeZeroNodes bool) ([]IPv6NodeListEntry, error)

	// WHOIS operations (delegated to WhoisOps())
	GetAllWhoisResults() ([]DomainWhoisResult, error)
	WhoisOps() *WhoisOperations

	// Utility operations (delegated to NodeOps())
	IsNodelistProcessed(nodelistDate time.Time) (bool, error)
	FindConflictingNode(zone, net, node int, date time.Time) (bool, error)
	GetMaxNodelistDate() (time.Time, error)

	// Lifecycle
	Close() error
}

// QueryBuilderInterface defines the contract for query building
type QueryBuilderInterface interface {
	// Basic queries
	InsertNodeSQL() string
	NodeSelectSQL() string
	BuildBatchInsertSQL(batchSize int) string
	BuildDirectBatchInsertSQL(nodes []database.Node, rp *ResultParser) string
	InsertNodesInChunks(db database.DatabaseInterface, nodes []database.Node) error
	BuildNodesQuery(filter database.NodeFilter) (string, []interface{})
	BuildFTSQuery(filter database.NodeFilter) (string, []interface{}, bool)

	// Statistics queries
	StatsSQL() string
	ZoneDistributionSQL() string
	LargestRegionsSQL() string
	LargestNetsSQL() string
	// Optimized statistics queries for better performance
	OptimizedLargestRegionsSQL() string
	OptimizedLargestNetsSQL() string

	// Node-specific queries
	NodeHistorySQL() string
	NodeDateRangeSQL() string
	SysopSearchSQL() string
	NodeSummarySearchSQL() string

	// Utility queries
	ConflictCheckSQL() string
	MarkConflictSQL() string
	IsProcessedSQL() string
	LatestDateSQL() string
	AvailableDatesSQL() string
	ExactDateExistsSQL() string
	NearestDateBeforeSQL() string
	NearestDateAfterSQL() string
	ConsecutiveNodelistCheckSQL() string
	NextNodelistDateSQL() string

	// Sysop queries
	UniqueSysopsWithFilterSQL() string
	UniqueSysopsSQL() string

	// Analytics queries
	FlagFirstAppearanceSQL() string
	FlagUsageByYearSQL() string
	NetworkNameSQL() string
	NetworkHistorySQL() string
}

// ResultParserInterface defines the contract for parsing database results
type ResultParserInterface interface {
	ParseNodeRow(scanner RowScanner) (database.Node, error)
	ParseNodeSummaryRow(scanner RowScanner) (NodeSummary, error)
	ParseNetworkStatsRow(scanner RowScanner) (*database.NetworkStats, error)
	ParseRegionInfoRow(scanner RowScanner) (database.RegionInfo, error)
	ParseNetInfoRow(scanner RowScanner) (database.NetInfo, error)
	ParseTestResultRow(scanner RowScanner, result *NodeTestResult) error
	ValidateNodeFilter(filter database.NodeFilter) error
	SanitizeStringInput(input string) string
}

// RowScanner interface abstracts sql.Rows and sql.Row for easier testing
type RowScanner interface {
	Scan(dest ...interface{}) error
}

// singleRowScanner wraps sql.Row to implement RowScanner interface
type singleRowScanner struct {
	Row interface{ Scan(dest ...interface{}) error }
}

func (s *singleRowScanner) Scan(dest ...interface{}) error {
	return s.Row.Scan(dest...)
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

// Common SQL field lists to avoid duplication
const (
	NodeSelectFields = `zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo,
		flags, modem_flags,
		conflict_sequence, has_conflict, has_inet, internet_config`

	NodeInsertFields = `zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo,
		flags, modem_flags,
		conflict_sequence, has_conflict, has_inet, internet_config`

	NodeInsertPlaceholders = `?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?`
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
	Zone               int       `json:"zone"`
	Net                int       `json:"net"`
	Node               int       `json:"node"`
	Address            string    `json:"address"`
	TestTime           time.Time `json:"test_time"`
	ModemPhoneDialed   string    `json:"modem_phone_dialed"`
	ModemOperatorName  string    `json:"modem_operator_name"`
	ModemAstDisposition string   `json:"modem_ast_disposition"`
	ModemAstHangupCause uint8   `json:"modem_ast_hangup_cause"`
	TestSource         string    `json:"test_source"`
	AttemptCount       uint32    `json:"attempt_count"`
	IsPSTNDead         bool      `json:"is_pstn_dead"`
	PSTNDeadReason     string    `json:"pstn_dead_reason,omitempty"`
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
	IPv6Type    string `json:"ipv6_type"`    // "Native", "T-6in4", "T-6to4", "T-Teredo"
	Provider    string `json:"provider"`     // Cleaned ISP/Org name
	HasFidoAddr bool   `json:"has_fido_addr"`  // f flag: has ::f1d0:z:n:nn style address
	FidoIPv6Addr string `json:"fido_ipv6_addr"` // The actual f1d0 IPv6 address found
	HasNoIPv4   bool   `json:"has_no_ipv4"`   // INO4: no working IPv4
	IsUnstable  bool   `json:"is_unstable"`   // 6UNS: failed >2 times in 30 days
	Remarks     string `json:"remarks"`       // Combined remarks string
}

// Error messages for consistent error handling
const (
	ErrNodeNotFound       = "node not found"
	ErrInvalidZone        = "invalid zone number"
	ErrInvalidNet         = "invalid net number"
	ErrInvalidNode        = "invalid node number"
	ErrInvalidDateFormat  = "invalid date format"
	ErrNoDataAvailable    = "no data available for the specified criteria"
	ErrDatabaseConnection = "database connection error"
	ErrQueryExecution     = "query execution error"
	ErrResultParsing      = "result parsing error"
	ErrTransactionFailed  = "transaction failed"
	ErrBatchInsertFailed  = "batch insert failed"
)
