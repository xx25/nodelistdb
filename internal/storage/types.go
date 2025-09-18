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
