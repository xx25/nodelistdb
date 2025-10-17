package storage

import (
	"github.com/nodelistdb/internal/database"
)

// TestOperations handles test result database operations using sub-operations
// This is the facade that coordinates all test-related operations
type TestOperationsRefactored struct {
	db                 database.DatabaseInterface
	testQueryBuilder   *TestQueryBuilder
	resultParser       ResultParserInterface
	historyOps         *TestHistoryOperations
	reachabilityOps    *ReachabilityOperations
	protocolOps        *ProtocolQueryOperations
	ipv6Ops            *IPv6QueryOperations
	softwareOps        *SoftwareAnalyticsOperations
	geoOps             *GeoAnalyticsOperations
}

// NewTestOperationsRefactored creates a new refactored TestOperations instance
func NewTestOperationsRefactored(db database.DatabaseInterface, queryBuilder QueryBuilderInterface, resultParser ResultParserInterface) *TestOperationsRefactored {
	// Create specialized query builder for tests
	testQueryBuilder := NewTestQueryBuilder(db)

	return &TestOperationsRefactored{
		db:                 db,
		testQueryBuilder:   testQueryBuilder,
		resultParser:       resultParser,
		historyOps:         NewTestHistoryOperations(db, testQueryBuilder, resultParser),
		reachabilityOps:    NewReachabilityOperations(db, testQueryBuilder, resultParser),
		protocolOps:        NewProtocolQueryOperations(db, testQueryBuilder, resultParser),
		ipv6Ops:            NewIPv6QueryOperations(db, testQueryBuilder, resultParser),
		softwareOps:        NewSoftwareAnalyticsOperations(db),
		geoOps:             NewGeoAnalyticsOperations(db),
	}
}

// ===== Test History Operations (delegated to TestHistoryOperations) =====

// GetNodeTestHistory retrieves test history for a specific node
func (to *TestOperationsRefactored) GetNodeTestHistory(zone, net, node int, days int) ([]NodeTestResult, error) {
	return to.historyOps.GetNodeTestHistory(zone, net, node, days)
}

// GetDetailedTestResult retrieves a detailed test result for a specific node and timestamp
func (to *TestOperationsRefactored) GetDetailedTestResult(zone, net, node int, testTime string) (*NodeTestResult, error) {
	return to.historyOps.GetDetailedTestResult(zone, net, node, testTime)
}

// ===== Reachability Operations (delegated to ReachabilityOperations) =====

// GetNodeReachabilityStats calculates reachability statistics for a node
func (to *TestOperationsRefactored) GetNodeReachabilityStats(zone, net, node int, days int) (*NodeReachabilityStats, error) {
	return to.reachabilityOps.GetNodeReachabilityStats(zone, net, node, days)
}

// GetReachabilityTrends gets daily reachability trends
func (to *TestOperationsRefactored) GetReachabilityTrends(days int) ([]ReachabilityTrend, error) {
	return to.reachabilityOps.GetReachabilityTrends(days)
}

// SearchNodesByReachability searches for nodes by reachability status
func (to *TestOperationsRefactored) SearchNodesByReachability(operational bool, limit int, days int) ([]NodeTestResult, error) {
	return to.reachabilityOps.SearchNodesByReachability(operational, limit, days)
}

// ===== Protocol Operations (delegated to ProtocolQueryOperations) =====

// GetBinkPEnabledNodes returns nodes that have been successfully tested with BinkP
func (to *TestOperationsRefactored) GetBinkPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return to.protocolOps.GetBinkPEnabledNodes(limit, days, includeZeroNodes)
}

// GetIfcicoEnabledNodes returns nodes that have been successfully tested with IFCICO
func (to *TestOperationsRefactored) GetIfcicoEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return to.protocolOps.GetIfcicoEnabledNodes(limit, days, includeZeroNodes)
}

// GetTelnetEnabledNodes returns nodes that have been successfully tested with Telnet
func (to *TestOperationsRefactored) GetTelnetEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return to.protocolOps.GetTelnetEnabledNodes(limit, days, includeZeroNodes)
}

// GetVModemEnabledNodes returns nodes that have been successfully tested with VModem
func (to *TestOperationsRefactored) GetVModemEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return to.protocolOps.GetVModemEnabledNodes(limit, days, includeZeroNodes)
}

// GetFTPEnabledNodes returns nodes that have been successfully tested with FTP
func (to *TestOperationsRefactored) GetFTPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return to.protocolOps.GetFTPEnabledNodes(limit, days, includeZeroNodes)
}

// ===== Software Analytics Operations (delegated to SoftwareAnalyticsOperations) =====

// GetBinkPSoftwareDistribution returns BinkP software distribution statistics
func (to *TestOperationsRefactored) GetBinkPSoftwareDistribution(days int) (*SoftwareDistribution, error) {
	return to.softwareOps.GetBinkPSoftwareDistribution(days)
}

// GetIFCICOSoftwareDistribution returns IFCICO software distribution statistics
func (to *TestOperationsRefactored) GetIFCICOSoftwareDistribution(days int) (*SoftwareDistribution, error) {
	return to.softwareOps.GetIFCICOSoftwareDistribution(days)
}

// GetBinkdDetailedStats returns detailed binkd statistics
func (to *TestOperationsRefactored) GetBinkdDetailedStats(days int) (*SoftwareDistribution, error) {
	return to.softwareOps.GetBinkdDetailedStats(days)
}

// ===== Geo Analytics Operations (delegated to GeoAnalyticsOperations) =====

// GetGeoHostingDistribution returns geographic hosting distribution statistics
func (to *TestOperationsRefactored) GetGeoHostingDistribution(days int) (*GeoHostingDistribution, error) {
	return to.geoOps.GetGeoHostingDistribution(days)
}

// GetNodesByCountry returns all operational nodes for a specific country
func (to *TestOperationsRefactored) GetNodesByCountry(countryCode string, days int) ([]NodeTestResult, error) {
	return to.geoOps.GetNodesByCountry(countryCode, days)
}

// GetNodesByProvider returns all operational nodes for a specific provider
func (to *TestOperationsRefactored) GetNodesByProvider(isp string, days int) ([]NodeTestResult, error) {
	return to.geoOps.GetNodesByProvider(isp, days)
}

// ===== IPv6 Operations (delegated to IPv6QueryOperations) =====

// GetIPv6EnabledNodes returns nodes that have been successfully tested with IPv6
func (to *TestOperationsRefactored) GetIPv6EnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return to.ipv6Ops.GetIPv6EnabledNodes(limit, days, includeZeroNodes)
}

// GetIPv6NonWorkingNodes returns nodes that have IPv6 addresses but no working IPv6 services
func (to *TestOperationsRefactored) GetIPv6NonWorkingNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return to.ipv6Ops.GetIPv6NonWorkingNodes(limit, days, includeZeroNodes)
}

// GetIPv6AdvertisedIPv4OnlyNodes returns nodes that advertise IPv6 addresses but are only accessible via IPv4
func (to *TestOperationsRefactored) GetIPv6AdvertisedIPv4OnlyNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return to.ipv6Ops.GetIPv6AdvertisedIPv4OnlyNodes(limit, days, includeZeroNodes)
}

// GetIPv6OnlyNodes returns nodes that have working IPv6 services but NO working IPv4 services
func (to *TestOperationsRefactored) GetIPv6OnlyNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return to.ipv6Ops.GetIPv6OnlyNodes(limit, days, includeZeroNodes)
}

// GetIPv6WeeklyNews returns weekly IPv6 connectivity changes
func (to *TestOperationsRefactored) GetIPv6WeeklyNews(limit int, includeZeroNodes bool) (*IPv6WeeklyNews, error) {
	return to.ipv6Ops.GetIPv6WeeklyNews(limit, includeZeroNodes)
}
