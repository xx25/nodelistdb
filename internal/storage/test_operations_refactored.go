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
	akaMismatchOps     *AKAMismatchOperations
	otherNetworksOps   *OtherNetworksOperations
	modemOps           *ModemQueryOperations
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
		akaMismatchOps:     NewAKAMismatchOperations(db, testQueryBuilder, resultParser),
		otherNetworksOps:   NewOtherNetworksOperations(db),
		modemOps:           NewModemQueryOperations(db),
	}
}

// ===== Test History Operations (delegated to TestHistoryOperations) =====

// GetNodeTestHistory retrieves test history for a specific node
func (to *TestOperationsRefactored) GetNodeTestHistory(zone, net, node int, days int, domain string) ([]NodeTestResult, error) {
	return to.historyOps.GetNodeTestHistory(zone, net, node, days, domain)
}

// GetDetailedTestResult retrieves a detailed test result for a specific node and timestamp
func (to *TestOperationsRefactored) GetDetailedTestResult(zone, net, node int, testTime string, domain string) (*NodeTestResult, error) {
	return to.historyOps.GetDetailedTestResult(zone, net, node, testTime, domain)
}

// ===== Reachability Operations (delegated to ReachabilityOperations) =====

// GetNodeReachabilityStats calculates reachability statistics for a node
func (to *TestOperationsRefactored) GetNodeReachabilityStats(zone, net, node int, days int, domain string) (*NodeReachabilityStats, error) {
	return to.reachabilityOps.GetNodeReachabilityStats(zone, net, node, days, domain)
}

// GetReachabilityTrendsAllTime gets all-time daily reachability trends from pre-aggregated stats
func (to *TestOperationsRefactored) GetReachabilityTrendsAllTime(domain string) ([]ReachabilityTrend, error) {
	return to.reachabilityOps.GetReachabilityTrendsAllTime(domain)
}

// GetReachabilityTrends gets daily reachability trends
func (to *TestOperationsRefactored) GetReachabilityTrends(days int, domain string) ([]ReachabilityTrend, error) {
	return to.reachabilityOps.GetReachabilityTrends(days, domain)
}

// SearchNodesByReachability searches for nodes by reachability status
func (to *TestOperationsRefactored) SearchNodesByReachability(operational bool, limit int, days int, domain string) ([]NodeTestResult, error) {
	return to.reachabilityOps.SearchNodesByReachability(operational, limit, days, domain)
}

// ===== Protocol Operations (delegated to ProtocolQueryOperations) =====

// GetBinkPEnabledNodes returns nodes that have been successfully tested with BinkP
func (to *TestOperationsRefactored) GetBinkPEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return to.protocolOps.GetBinkPEnabledNodes(limit, days, includeZeroNodes, domain)
}

// GetIfcicoEnabledNodes returns nodes that have been successfully tested with IFCICO
func (to *TestOperationsRefactored) GetIfcicoEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return to.protocolOps.GetIfcicoEnabledNodes(limit, days, includeZeroNodes, domain)
}

// GetTelnetEnabledNodes returns nodes that have been successfully tested with Telnet
func (to *TestOperationsRefactored) GetTelnetEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return to.protocolOps.GetTelnetEnabledNodes(limit, days, includeZeroNodes, domain)
}

// GetVModemEnabledNodes returns nodes that have been successfully tested with VModem
func (to *TestOperationsRefactored) GetVModemEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return to.protocolOps.GetVModemEnabledNodes(limit, days, includeZeroNodes, domain)
}

// GetFTPEnabledNodes returns nodes that have been successfully tested with FTP
func (to *TestOperationsRefactored) GetFTPEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return to.protocolOps.GetFTPEnabledNodes(limit, days, includeZeroNodes, domain)
}

// ===== Software Analytics Operations (delegated to SoftwareAnalyticsOperations) =====

// GetBinkPSoftwareDistribution returns BinkP software distribution statistics
func (to *TestOperationsRefactored) GetBinkPSoftwareDistribution(days int, domain string) (*SoftwareDistribution, error) {
	return to.softwareOps.GetBinkPSoftwareDistribution(days, domain)
}

// GetIFCICOSoftwareDistribution returns IFCICO software distribution statistics
func (to *TestOperationsRefactored) GetIFCICOSoftwareDistribution(days int, domain string) (*SoftwareDistribution, error) {
	return to.softwareOps.GetIFCICOSoftwareDistribution(days, domain)
}

// GetBinkdDetailedStats returns detailed binkd statistics
func (to *TestOperationsRefactored) GetBinkdDetailedStats(days int, domain string) (*SoftwareDistribution, error) {
	return to.softwareOps.GetBinkdDetailedStats(days, domain)
}

// ===== Geo Analytics Operations (delegated to GeoAnalyticsOperations) =====

// GetGeoHostingDistribution returns geographic hosting distribution statistics
func (to *TestOperationsRefactored) GetGeoHostingDistribution(days int, domain string) (*GeoHostingDistribution, error) {
	return to.geoOps.GetGeoHostingDistribution(days, domain)
}

// GetNodesByCountry returns all operational nodes for a specific country
func (to *TestOperationsRefactored) GetNodesByCountry(countryCode string, days int, domain string) ([]NodeTestResult, error) {
	return to.geoOps.GetNodesByCountry(countryCode, days, domain)
}

// GetNodesByProvider returns all operational nodes for a specific provider
func (to *TestOperationsRefactored) GetNodesByProvider(isp string, days int, domain string) ([]NodeTestResult, error) {
	return to.geoOps.GetNodesByProvider(isp, days, domain)
}

// ===== IPv6 Operations (delegated to IPv6QueryOperations) =====

// GetIPv6EnabledNodes returns nodes that have been successfully tested with IPv6
func (to *TestOperationsRefactored) GetIPv6EnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return to.ipv6Ops.GetIPv6EnabledNodes(limit, days, includeZeroNodes, domain)
}

// GetIPv6NonWorkingNodes returns nodes that have IPv6 addresses but no working IPv6 services
func (to *TestOperationsRefactored) GetIPv6NonWorkingNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return to.ipv6Ops.GetIPv6NonWorkingNodes(limit, days, includeZeroNodes, domain)
}

// GetIPv6AdvertisedIPv4OnlyNodes returns nodes that advertise IPv6 addresses but are only accessible via IPv4
func (to *TestOperationsRefactored) GetIPv6AdvertisedIPv4OnlyNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return to.ipv6Ops.GetIPv6AdvertisedIPv4OnlyNodes(limit, days, includeZeroNodes, domain)
}

// GetIPv6OnlyNodes returns nodes that have working IPv6 services but NO working IPv4 services
func (to *TestOperationsRefactored) GetIPv6OnlyNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return to.ipv6Ops.GetIPv6OnlyNodes(limit, days, includeZeroNodes, domain)
}

// GetPureIPv6OnlyNodes returns nodes that ONLY advertise IPv6 addresses (no IPv4 addresses at all)
func (to *TestOperationsRefactored) GetPureIPv6OnlyNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return to.ipv6Ops.GetPureIPv6OnlyNodes(limit, days, includeZeroNodes, domain)
}

// GetIPv6NodeList returns verified working IPv6 nodes for the node list report (Michiel's format)
func (to *TestOperationsRefactored) GetIPv6NodeList(limit int, days int, includeZeroNodes bool, domain string) ([]IPv6NodeListEntry, error) {
	return to.ipv6Ops.GetIPv6NodeList(limit, days, includeZeroNodes, domain)
}

// GetIPv6WeeklyNews returns weekly IPv6 connectivity changes
func (to *TestOperationsRefactored) GetIPv6WeeklyNews(limit int, includeZeroNodes bool, domain string) (*IPv6WeeklyNews, error) {
	return to.ipv6Ops.GetIPv6WeeklyNews(limit, includeZeroNodes, domain)
}

// ===== AKA Mismatch Operations (delegated to AKAMismatchOperations) =====

// GetAKAMismatchNodes returns nodes where announced AKA doesn't match expected nodelist address
func (to *TestOperationsRefactored) GetAKAMismatchNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return to.akaMismatchOps.GetAKAMismatchNodes(limit, days, includeZeroNodes, domain)
}

// GetIPv6IncorrectIPv4CorrectNodes returns nodes where IPv6 AKA is incorrect but IPv4 AKA is correct
func (to *TestOperationsRefactored) GetIPv6IncorrectIPv4CorrectNodes(limit int, days int, includeZeroNodes bool, domain string) ([]AKAIPVersionMismatchNode, error) {
	return to.akaMismatchOps.GetIPv6IncorrectIPv4CorrectNodes(limit, days, includeZeroNodes, domain)
}

// GetIPv4IncorrectIPv6CorrectNodes returns nodes where IPv4 AKA is incorrect but IPv6 AKA is correct
func (to *TestOperationsRefactored) GetIPv4IncorrectIPv6CorrectNodes(limit int, days int, includeZeroNodes bool, domain string) ([]AKAIPVersionMismatchNode, error) {
	return to.akaMismatchOps.GetIPv4IncorrectIPv6CorrectNodes(limit, days, includeZeroNodes, domain)
}

// ===== Other Networks Operations (delegated to OtherNetworksOperations) =====

// GetOtherNetworksSummary returns a summary of non-FidoNet networks found in AKAs
func (to *TestOperationsRefactored) GetOtherNetworksSummary(days int, domain string) ([]OtherNetworkSummary, error) {
	return to.otherNetworksOps.GetOtherNetworksSummary(days, domain)
}

// GetNodesInNetwork returns nodes that announce AKAs in a specific network
func (to *TestOperationsRefactored) GetNodesInNetwork(networkName string, limit int, days int, domain string) ([]OtherNetworkNode, error) {
	return to.otherNetworksOps.GetNodesInNetwork(networkName, limit, days, domain)
}

// ===== Modem Operations (delegated to ModemQueryOperations) =====

// GetModemAccessibleNodes returns nodes successfully reached via modem tests
func (to *TestOperationsRefactored) GetModemAccessibleNodes(limit int, days int, includeZeroNodes bool, domain string) ([]ModemAccessibleNode, error) {
	return to.modemOps.GetModemAccessibleNodes(limit, days, includeZeroNodes, domain)
}

// GetModemNoAnswerNodes returns nodes tested via modem that never answered
func (to *TestOperationsRefactored) GetModemNoAnswerNodes(limit int, days int, includeZeroNodes bool, domain string) ([]ModemNoAnswerNode, error) {
	return to.modemOps.GetModemNoAnswerNodes(limit, days, includeZeroNodes, domain)
}

// GetRecentModemSuccessPhones returns phone numbers successfully tested via modem within N days
func (to *TestOperationsRefactored) GetRecentModemSuccessPhones(days int) ([]string, error) {
	return to.modemOps.GetRecentModemSuccessPhones(days)
}

// GetDetailedModemTestResult returns detailed modem test data for a specific test
func (to *TestOperationsRefactored) GetDetailedModemTestResult(zone, net, node int, testTime string) (*ModemTestDetail, error) {
	return to.modemOps.GetDetailedModemTestResult(zone, net, node, testTime)
}
