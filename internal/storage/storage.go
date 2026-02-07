package storage

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
)

// Storage provides thread-safe database operations using specialized components.
// Instead of delegating all methods, Storage exposes sub-components directly via
// accessor methods (NodeOps(), SearchOps(), etc.) to reduce boilerplate and improve maintainability.
type Storage struct {
	db                  database.DatabaseInterface
	queryBuilder        QueryBuilderInterface
	resultParser        ResultParserInterface
	nodeOperations      *NodeOperations
	searchOperations    *SearchOperations
	statsOperations     *StatisticsOperations
	analyticsOperations *AnalyticsOperations
	testOperations      *TestOperationsRefactored
	whoisOperations     *WhoisOperations
	pstnDeadOperations  *PSTNDeadOperations

	mu sync.RWMutex
}

// GetDatabase returns the underlying database interface
func (s *Storage) GetDatabase() database.DatabaseInterface {
	return s.db
}

// NodeOps returns the node operations component for CRUD operations on nodes
func (s *Storage) NodeOps() *NodeOperations {
	return s.nodeOperations
}

// SearchOps returns the search operations component for advanced search queries
func (s *Storage) SearchOps() *SearchOperations {
	return s.searchOperations
}

// StatsOps returns the statistics operations component for network statistics
func (s *Storage) StatsOps() *StatisticsOperations {
	return s.statsOperations
}

// AnalyticsOps returns the analytics operations component for historical analytics
func (s *Storage) AnalyticsOps() *AnalyticsOperations {
	return s.analyticsOperations
}

// TestOps returns the test operations component for node testing and reachability
func (s *Storage) TestOps() *TestOperationsRefactored {
	return s.testOperations
}

// WhoisOps returns the WHOIS operations component for domain expiration data
func (s *Storage) WhoisOps() *WhoisOperations {
	return s.whoisOperations
}

// PSTNDeadOps returns the PSTN dead node operations component
func (s *Storage) PSTNDeadOps() *PSTNDeadOperations {
	return s.pstnDeadOperations
}

// New creates a new Storage instance with ClickHouse-specific components
func New(db database.DatabaseInterface) (*Storage, error) {
	// Always use ClickHouse components (only supported database type)
	queryBuilder := NewQueryBuilder()
	resultParser := NewClickHouseResultParser()

	// Create the storage instance with ClickHouse components
	storage := &Storage{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}

	// Create specialized operation components
	storage.nodeOperations = NewNodeOperations(db, queryBuilder, resultParser)
	storage.searchOperations = NewSearchOperations(db, queryBuilder, resultParser, storage.nodeOperations)
	storage.statsOperations = NewStatisticsOperations(db, queryBuilder, resultParser)
	storage.pstnDeadOperations = NewPSTNDeadOperations(db)
	storage.analyticsOperations = NewAnalyticsOperations(db, queryBuilder, resultParser, storage.pstnDeadOperations)
	storage.testOperations = NewTestOperationsRefactored(db, queryBuilder, resultParser)
	storage.whoisOperations = NewWhoisOperations(db)

	return storage, nil
}

// Close closes all database connections and prepared statements
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// The individual components don't have close methods currently,
	// but we maintain this for backward compatibility
	return nil
}

// --- Legacy Delegation Methods for Operations Interface Compatibility ---
// These methods provide backward compatibility for code using the Operations interface.
// New code should use the component accessors directly (e.g., storage.NodeOps().GetNodes()).

// Node Operations delegated methods
func (s *Storage) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	return s.nodeOperations.GetNodes(filter)
}

func (s *Storage) GetNodeHistory(zone, net, node int) ([]database.Node, error) {
	return s.nodeOperations.GetNodeHistory(zone, net, node)
}

func (s *Storage) GetNodeDateRange(zone, net, node int) (firstDate, lastDate time.Time, err error) {
	return s.nodeOperations.GetNodeDateRange(zone, net, node)
}

func (s *Storage) InsertNodes(nodes []database.Node) error {
	return s.nodeOperations.InsertNodes(nodes)
}

func (s *Storage) IsNodelistProcessed(nodelistDate time.Time) (bool, error) {
	return s.nodeOperations.IsNodelistProcessed(nodelistDate)
}

func (s *Storage) FindConflictingNode(zone, net, node int, date time.Time) (bool, error) {
	return s.nodeOperations.FindConflictingNode(zone, net, node, date)
}

func (s *Storage) GetMaxNodelistDate() (time.Time, error) {
	return s.nodeOperations.GetMaxNodelistDate()
}

// Search Operations delegated methods
func (s *Storage) SearchNodesBySysop(sysopName string, limit int) ([]NodeSummary, error) {
	return s.searchOperations.SearchNodesBySysop(sysopName, limit)
}

func (s *Storage) GetNodeChanges(zone, net, node int) ([]database.NodeChange, error) {
	return s.searchOperations.GetNodeChanges(zone, net, node)
}

func (s *Storage) GetUniqueSysops(nameFilter string, limit, offset int) ([]SysopInfo, error) {
	return s.searchOperations.GetUniqueSysops(nameFilter, limit, offset)
}

func (s *Storage) GetNodesBySysop(sysopName string, limit int) ([]database.Node, error) {
	return s.searchOperations.GetNodesBySysop(sysopName, limit)
}

func (s *Storage) SearchNodesWithLifetime(filter database.NodeFilter) ([]NodeSummary, error) {
	return s.searchOperations.SearchNodesWithLifetime(filter)
}

// Analytics Operations delegated methods
func (s *Storage) GetFlagFirstAppearance(flagName string) (*FlagFirstAppearance, error) {
	return s.analyticsOperations.GetFlagFirstAppearance(flagName)
}

func (s *Storage) GetFlagUsageByYear(flagName string) ([]FlagUsageByYear, error) {
	return s.analyticsOperations.GetFlagUsageByYear(flagName)
}

func (s *Storage) GetNetworkHistory(zone, net int) (*NetworkHistory, error) {
	return s.analyticsOperations.GetNetworkHistory(zone, net)
}

func (s *Storage) UpdateFlagStatistics(nodelistDate time.Time) error {
	return s.analyticsOperations.UpdateFlagStatistics(nodelistDate)
}

// Statistics Operations delegated methods
func (s *Storage) GetStats(date time.Time) (*database.NetworkStats, error) {
	return s.statsOperations.GetStats(date)
}

func (s *Storage) GetLatestStatsDate() (time.Time, error) {
	return s.statsOperations.GetLatestStatsDate()
}

func (s *Storage) GetAvailableDates() ([]time.Time, error) {
	return s.statsOperations.GetAvailableDates()
}

func (s *Storage) GetNearestAvailableDate(requestedDate time.Time) (time.Time, error) {
	return s.statsOperations.GetNearestAvailableDate(requestedDate)
}

// Test Operations delegated methods
func (s *Storage) GetNodeTestHistory(zone, net, node int, days int) ([]NodeTestResult, error) {
	return s.testOperations.GetNodeTestHistory(zone, net, node, days)
}

func (s *Storage) GetDetailedTestResult(zone, net, node int, testTime string) (*NodeTestResult, error) {
	return s.testOperations.GetDetailedTestResult(zone, net, node, testTime)
}

func (s *Storage) GetNodeReachabilityStats(zone, net, node int, days int) (*NodeReachabilityStats, error) {
	return s.testOperations.GetNodeReachabilityStats(zone, net, node, days)
}

func (s *Storage) GetReachabilityTrends(days int) ([]ReachabilityTrend, error) {
	return s.testOperations.GetReachabilityTrends(days)
}

func (s *Storage) SearchNodesByReachability(operational bool, limit int, days int) ([]NodeTestResult, error) {
	return s.testOperations.SearchNodesByReachability(operational, limit, days)
}

func (s *Storage) GetIPv6EnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return s.testOperations.GetIPv6EnabledNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetIPv6NonWorkingNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return s.testOperations.GetIPv6NonWorkingNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetIPv6AdvertisedIPv4OnlyNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return s.testOperations.GetIPv6AdvertisedIPv4OnlyNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetIPv6OnlyNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return s.testOperations.GetIPv6OnlyNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetPureIPv6OnlyNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return s.testOperations.GetPureIPv6OnlyNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetIPv6NodeList(limit int, days int, includeZeroNodes bool) ([]IPv6NodeListEntry, error) {
	return s.testOperations.GetIPv6NodeList(limit, days, includeZeroNodes)
}

func (s *Storage) GetIPv6WeeklyNews(limit int, includeZeroNodes bool) (*IPv6WeeklyNews, error) {
	return s.testOperations.GetIPv6WeeklyNews(limit, includeZeroNodes)
}

func (s *Storage) GetBinkPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return s.testOperations.GetBinkPEnabledNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetIfcicoEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return s.testOperations.GetIfcicoEnabledNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetTelnetEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return s.testOperations.GetTelnetEnabledNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetVModemEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return s.testOperations.GetVModemEnabledNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetFTPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return s.testOperations.GetFTPEnabledNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetAKAMismatchNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return s.testOperations.GetAKAMismatchNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetIPv6IncorrectIPv4CorrectNodes(limit int, days int, includeZeroNodes bool) ([]AKAIPVersionMismatchNode, error) {
	return s.testOperations.GetIPv6IncorrectIPv4CorrectNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetIPv4IncorrectIPv6CorrectNodes(limit int, days int, includeZeroNodes bool) ([]AKAIPVersionMismatchNode, error) {
	return s.testOperations.GetIPv4IncorrectIPv6CorrectNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetOtherNetworksSummary(days int) ([]OtherNetworkSummary, error) {
	return s.testOperations.GetOtherNetworksSummary(days)
}

func (s *Storage) GetNodesInNetwork(networkName string, limit int, days int) ([]OtherNetworkNode, error) {
	return s.testOperations.GetNodesInNetwork(networkName, limit, days)
}

func (s *Storage) GetBinkPSoftwareDistribution(days int) (*SoftwareDistribution, error) {
	return s.testOperations.GetBinkPSoftwareDistribution(days)
}

func (s *Storage) GetIFCICOSoftwareDistribution(days int) (*SoftwareDistribution, error) {
	return s.testOperations.GetIFCICOSoftwareDistribution(days)
}

func (s *Storage) GetBinkdDetailedStats(days int) (*SoftwareDistribution, error) {
	return s.testOperations.GetBinkdDetailedStats(days)
}

func (s *Storage) GetGeoHostingDistribution(days int) (*GeoHostingDistribution, error) {
	return s.testOperations.GetGeoHostingDistribution(days)
}

func (s *Storage) GetNodesByCountry(countryCode string, days int) ([]NodeTestResult, error) {
	return s.testOperations.GetNodesByCountry(countryCode, days)
}

func (s *Storage) GetNodesByProvider(provider string, days int) ([]NodeTestResult, error) {
	return s.testOperations.GetNodesByProvider(provider, days)
}

func (s *Storage) GetOnThisDayNodes(month, day, limit int, activeOnly bool) ([]OnThisDayNode, error) {
	return s.analyticsOperations.GetOnThisDayNodes(month, day, limit, activeOnly)
}

func (s *Storage) GetPioneersByRegion(zone, region, limit int) ([]PioneerNode, error) {
	return s.searchOperations.GetPioneersByRegion(zone, region, limit)
}

func (s *Storage) GetPSTNCMNodes(limit int) ([]PSTNNode, error) {
	return s.analyticsOperations.GetPSTNCMNodes(limit)
}

func (s *Storage) GetPSTNNodes(limit int, zone int) ([]PSTNNode, error) {
	return s.analyticsOperations.GetPSTNNodes(limit, zone)
}

func (s *Storage) MarkPSTNDead(zone, net, node int, reason, markedBy string) error {
	return s.pstnDeadOperations.MarkDead(zone, net, node, reason, markedBy)
}

func (s *Storage) UnmarkPSTNDead(zone, net, node int, markedBy string) error {
	return s.pstnDeadOperations.UnmarkDead(zone, net, node, markedBy)
}

func (s *Storage) GetPSTNDeadNodes() ([]PSTNDeadNode, error) {
	return s.pstnDeadOperations.GetAllDeadNodes()
}

func (s *Storage) GetFileRequestNodes(limit int) ([]FileRequestNode, error) {
	return s.analyticsOperations.GetFileRequestNodes(limit)
}

func (s *Storage) GetModemAccessibleNodes(limit int, days int, includeZeroNodes bool) ([]ModemAccessibleNode, error) {
	return s.testOperations.GetModemAccessibleNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetModemNoAnswerNodes(limit int, days int, includeZeroNodes bool) ([]ModemNoAnswerNode, error) {
	return s.testOperations.GetModemNoAnswerNodes(limit, days, includeZeroNodes)
}

func (s *Storage) GetRecentModemSuccessPhones(days int) ([]string, error) {
	return s.testOperations.GetRecentModemSuccessPhones(days)
}

func (s *Storage) GetDetailedModemTestResult(zone, net, node int, testTime string) (*ModemTestDetail, error) {
	return s.testOperations.GetDetailedModemTestResult(zone, net, node, testTime)
}

// WHOIS Operations delegated methods
func (s *Storage) GetAllWhoisResults() ([]DomainWhoisResult, error) {
	return s.whoisOperations.GetAllWhoisResults()
}

// --- Utility Methods ---

// GetQueryBuilder returns the query builder for direct access
func (s *Storage) GetQueryBuilder() QueryBuilderInterface {
	return s.queryBuilder
}

// GetResultParser returns the result parser for direct access
func (s *Storage) GetResultParser() *ResultParser {
	// Type assert to get concrete type when needed
	if rp, ok := s.resultParser.(*ResultParser); ok {
		return rp
	}
	// For ClickHouse, extract the embedded ResultParser
	if crp, ok := s.resultParser.(*ClickHouseResultParser); ok {
		return crp.ResultParser
	}
	return nil
}

// --- Health and Monitoring ---

// HealthCheck performs a basic health check on all storage components
func (s *Storage) HealthCheck() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Test database connection
	if err := s.db.Ping(); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	// Test basic query functionality
	_, err := s.statsOperations.GetLatestStatsDate()
	if err != nil {
		// This might fail if no data exists, which is okay
		// Only fail if it's a connection or query syntax error
		if !strings.Contains(err.Error(), "no rows") {
			return fmt.Errorf("query execution failed: %w", err)
		}
	}

	return nil
}

// GetComponentInfo returns information about the storage components
func (s *Storage) GetComponentInfo() map[string]interface{} {
	return map[string]interface{}{
		"version":             "3.0.0-refactored",
		"architecture":        "component-based with direct access",
		"query_builder":       "safe parameterized queries",
		"result_parser":       "type-safe parsing",
		"node_operations":     "CRUD operations with validation",
		"search_operations":   "advanced search and change detection",
		"stats_operations":    "comprehensive statistics",
		"analytics_operations": "historical analytics",
		"test_operations":     "node testing and reachability",
		"thread_safety":       "mutex-protected operations",
		"boilerplate_removed": "~200 lines of delegation eliminated",
	}
}
