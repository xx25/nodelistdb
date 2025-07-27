package storage

import (
	"time"

	"nodelistdb/internal/database"
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

// ChangeFilter allows filtering out specific types of changes when analyzing node history
type ChangeFilter struct {
	IgnoreFlags              bool // Ignore changes in flag arrays
	IgnorePhone              bool // Ignore phone number changes
	IgnoreSpeed              bool // Ignore max speed changes
	IgnoreStatus             bool // Ignore node type/status changes
	IgnoreLocation           bool // Ignore location changes
	IgnoreName               bool // Ignore system name changes
	IgnoreSysop              bool // Ignore sysop name changes
	IgnoreConnectivity       bool // Ignore Binkp, Telnet capability changes
	IgnoreInternetProtocols  bool // Ignore internet protocol changes
	IgnoreInternetHostnames  bool // Ignore internet hostname changes
	IgnoreInternetPorts      bool // Ignore internet port changes
	IgnoreInternetEmails     bool // Ignore internet email changes
	IgnoreModemFlags         bool // Ignore modem flag changes
}

// BatchInsertConfig holds configuration for batch insert operations
type BatchInsertConfig struct {
	ChunkSize    int  // Number of nodes per chunk
	UseTransactions bool // Whether to wrap inserts in transactions
}

// DefaultBatchInsertConfig returns the default configuration for batch inserts
func DefaultBatchInsertConfig() BatchInsertConfig {
	return BatchInsertConfig{
		ChunkSize:       100,
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
	GetNodeChanges(zone, net, node int, filter ChangeFilter) ([]database.NodeChange, error)
	
	// Statistics operations
	GetStats(date time.Time) (*database.NetworkStats, error)
	GetLatestStatsDate() (time.Time, error)
	GetAvailableDates() ([]time.Time, error)
	GetNearestAvailableDate(requestedDate time.Time) (time.Time, error)
	
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
	BuildNodesQuery(filter database.NodeFilter) (string, []interface{})
	
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
}

// ResultParserInterface defines the contract for parsing database results
type ResultParserInterface interface {
	ParseNodeRow(scanner RowScanner) (database.Node, error)
	ParseNodeSummaryRow(scanner RowScanner) (NodeSummary, error)
	ParseNetworkStatsRow(scanner RowScanner) (*database.NetworkStats, error)
	ParseRegionInfoRow(scanner RowScanner) (database.RegionInfo, error)
	ParseNetInfoRow(scanner RowScanner) (database.NetInfo, error)
}

// RowScanner interface abstracts sql.Rows and sql.Row for easier testing
type RowScanner interface {
	Scan(dest ...interface{}) error
}

// Constants for default values and limits
const (
	DefaultSearchLimit     = 100
	MaxSearchLimit        = 1000
	DefaultChunkSize      = 100
	MaxChunkSize          = 1000
	DefaultSysopLimit     = 50
	MaxSysopLimit         = 200
	DefaultRegionLimit    = 10
	DefaultNetLimit       = 10
)

// Common SQL field lists to avoid duplication
const (
	NodeSelectFields = `zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		conflict_sequence, has_conflict, has_inet, internet_config`
		
	NodeInsertFields = `zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
		flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
		conflict_sequence, has_conflict, has_inet, internet_config`
		
	NodeInsertPlaceholders = `?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?`
)

// Error messages for consistent error handling
const (
	ErrNodeNotFound          = "node not found"
	ErrInvalidZone          = "invalid zone number"
	ErrInvalidNet           = "invalid net number" 
	ErrInvalidNode          = "invalid node number"
	ErrInvalidDateFormat    = "invalid date format"
	ErrNoDataAvailable      = "no data available for the specified criteria"
	ErrDatabaseConnection   = "database connection error"
	ErrQueryExecution       = "query execution error"
	ErrResultParsing        = "result parsing error"
	ErrTransactionFailed    = "transaction failed"
	ErrBatchInsertFailed    = "batch insert failed"
)