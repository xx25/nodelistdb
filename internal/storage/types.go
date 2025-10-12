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
	FTPTested     bool   `json:"ftp_tested"`
	FTPSuccess    bool   `json:"ftp_success"`
	FTPResponseMs uint32 `json:"ftp_response_ms"`
	FTPError      string `json:"ftp_error"`

	// VModem Test Results
	VModemTested     bool   `json:"vmodem_tested"`
	VModemSuccess    bool   `json:"vmodem_success"`
	VModemResponseMs uint32 `json:"vmodem_response_ms"`
	VModemError      string `json:"vmodem_error"`

	// IPv6-specific Test Results
	BinkPIPv6Tested     bool   `json:"binkp_ipv6_tested"`
	BinkPIPv6Success    bool   `json:"binkp_ipv6_success"`
	BinkPIPv6Error      string `json:"binkp_ipv6_error"`
	IfcicoIPv6Tested    bool   `json:"ifcico_ipv6_tested"`
	IfcicoIPv6Success   bool   `json:"ifcico_ipv6_success"`
	IfcicoIPv6Error     string `json:"ifcico_ipv6_error"`
	TelnetIPv6Tested    bool   `json:"telnet_ipv6_tested"`
	TelnetIPv6Success   bool   `json:"telnet_ipv6_success"`
	TelnetIPv6Error     string `json:"telnet_ipv6_error"`

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
	Zone                int     `json:"zone"`
	Net                 int     `json:"net"`
	Node                int     `json:"node"`
	TotalTests          int     `json:"total_tests"`
	SuccessfulTests     int     `json:"successful_tests"`
	FailedTests         int     `json:"failed_tests"`
	SuccessRate         float64 `json:"success_rate"`
	AverageResponseMs   float64 `json:"average_response_ms"`
	LastTestTime        time.Time `json:"last_test_time"`
	LastStatus          string  `json:"last_status"`
	BinkPSuccessRate    float64 `json:"binkp_success_rate"`
	IfcicoSuccessRate   float64 `json:"ifcico_success_rate"`
	TelnetSuccessRate   float64 `json:"telnet_success_rate"`
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

// Operations interface defines the contract for storage operations
type Operations interface {
	// Node operations
	GetNodes(filter database.NodeFilter) ([]database.Node, error)
	GetNodeHistory(zone, net, node int) ([]database.Node, error)
	GetNodeDateRange(zone, net, node int) (firstDate, lastDate time.Time, err error)
	InsertNodes(nodes []database.Node) error

	// Search operations
	SearchNodesBySysop(sysopName string, limit int) ([]NodeSummary, error)
	GetNodeChanges(zone, net, node int) ([]database.NodeChange, error)
	GetUniqueSysops(nameFilter string, limit, offset int) ([]SysopInfo, error)
	GetNodesBySysop(sysopName string, limit int) ([]database.Node, error)
	SearchNodesWithLifetime(filter database.NodeFilter) ([]NodeSummary, error)
	GetFlagFirstAppearance(flagName string) (*FlagFirstAppearance, error)
	GetFlagUsageByYear(flagName string) ([]FlagUsageByYear, error)
	GetNetworkHistory(zone, net int) (*NetworkHistory, error)

	// Statistics operations
	GetStats(date time.Time) (*database.NetworkStats, error)
	GetLatestStatsDate() (time.Time, error)
	GetAvailableDates() ([]time.Time, error)
	GetNearestAvailableDate(requestedDate time.Time) (time.Time, error)

	// Test operations
	GetNodeTestHistory(zone, net, node int, days int) ([]NodeTestResult, error)
	GetDetailedTestResult(zone, net, node int, testTime string) (*NodeTestResult, error)
	GetNodeReachabilityStats(zone, net, node int, days int) (*NodeReachabilityStats, error)
	GetReachabilityTrends(days int) ([]ReachabilityTrend, error)
	SearchNodesByReachability(operational bool, limit int, days int) ([]NodeTestResult, error)
	GetIPv6EnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetIPv6NonWorkingNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetBinkPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetIfcicoEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetTelnetEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetVModemEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)
	GetFTPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error)

	// Software analytics operations (ClickHouse only)
	GetBinkPSoftwareDistribution(days int) (*SoftwareDistribution, error)
	GetIFCICOSoftwareDistribution(days int) (*SoftwareDistribution, error)
	GetBinkdDetailedStats(days int) (*SoftwareDistribution, error)

	// Utility operations
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
