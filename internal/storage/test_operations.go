package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
)

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

	IsOperational         bool `json:"is_operational"`
	HasConnectivityIssues bool `json:"has_connectivity_issues"`
	AddressValidated      bool `json:"address_validated"`
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

// TestOperations handles test result database operations
type TestOperations struct {
	db           database.DatabaseInterface
	queryBuilder QueryBuilderInterface
	resultParser ResultParserInterface
	mu           sync.RWMutex
}

// NewTestOperations creates a new TestOperations instance
func NewTestOperations(db database.DatabaseInterface, queryBuilder QueryBuilderInterface, resultParser ResultParserInterface) *TestOperations {
	return &TestOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// GetNodeTestHistory retrieves test history for a specific node
func (to *TestOperations) GetNodeTestHistory(zone, net, node int, days int) ([]NodeTestResult, error) {
	to.mu.RLock()
	defer to.mu.RUnlock()

	conn := to.db.Conn()
	
	// Build query based on database type
	var query string
	if _, isClickHouse := to.db.(*database.ClickHouseDB); isClickHouse {
		query = `
			SELECT
				test_time,
				zone,
				net,
				node,
				address,
				hostname,
				resolved_ipv4,
				resolved_ipv6,
				dns_error,
				country,
				country_code,
				city,
				region,
				latitude,
				longitude,
				isp,
				org,
				asn,
				binkp_tested,
				binkp_success,
				binkp_response_ms,
				binkp_system_name,
				binkp_sysop,
				binkp_location,
				binkp_version,
				binkp_addresses,
				binkp_capabilities,
				binkp_error,
				ifcico_tested,
				ifcico_success,
				ifcico_response_ms,
				ifcico_mailer_info,
				ifcico_system_name,
				ifcico_addresses,
				ifcico_response_type,
				ifcico_error,
				telnet_tested,
				telnet_success,
				telnet_response_ms,
				telnet_error,
				ftp_tested,
				ftp_success,
				ftp_response_ms,
				ftp_error,
				vmodem_tested,
				vmodem_success,
				vmodem_response_ms,
				vmodem_error,
				is_operational,
				has_connectivity_issues,
				address_validated
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ?
			AND test_time >= now() - INTERVAL ? DAY
			ORDER BY test_time ASC
		`
	} else {
		// DuckDB query
		query = `
			SELECT
				test_time,
				zone,
				net,
				node,
				address,
				hostname,
				resolved_ipv4,
				resolved_ipv6,
				dns_error,
				country,
				country_code,
				city,
				region,
				latitude,
				longitude,
				isp,
				org,
				asn,
				binkp_tested,
				binkp_success,
				binkp_response_ms,
				binkp_system_name,
				binkp_sysop,
				binkp_location,
				binkp_version,
				binkp_addresses,
				binkp_capabilities,
				binkp_error,
				ifcico_tested,
				ifcico_success,
				ifcico_response_ms,
				ifcico_mailer_info,
				ifcico_system_name,
				ifcico_addresses,
				ifcico_response_type,
				ifcico_error,
				telnet_tested,
				telnet_success,
				telnet_response_ms,
				telnet_error,
				ftp_tested,
				ftp_success,
				ftp_response_ms,
				ftp_error,
				vmodem_tested,
				vmodem_success,
				vmodem_response_ms,
				vmodem_error,
				is_operational,
				has_connectivity_issues,
				address_validated
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ?
			AND test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
			ORDER BY test_time ASC
		`
	}

	rows, err := conn.Query(query, zone, net, node, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query node test history: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	for rows.Next() {
		var r NodeTestResult
		err := to.resultParser.ParseTestResultRow(rows, &r)
		if err != nil {
			return nil, fmt.Errorf("failed to parse test result: %w", err)
		}
		results = append(results, r)
	}

	return results, nil
}

// GetDetailedTestResult retrieves a detailed test result for a specific node and timestamp
func (to *TestOperations) GetDetailedTestResult(zone, net, node int, testTime string) (*NodeTestResult, error) {
	to.mu.RLock()
	defer to.mu.RUnlock()

	conn := to.db.Conn()

	// Build query based on database type
	var query string
	if _, isClickHouse := to.db.(*database.ClickHouseDB); isClickHouse {
		query = `
			SELECT
				test_time, zone, net, node, address, hostname,
				resolved_ipv4, resolved_ipv6, dns_error,
				country, country_code, city, region, latitude, longitude, isp, org, asn,
				binkp_tested, binkp_success, binkp_response_ms, binkp_system_name,
				binkp_sysop, binkp_location, binkp_version, binkp_addresses, binkp_capabilities, binkp_error,
				ifcico_tested, ifcico_success, ifcico_response_ms, ifcico_mailer_info,
				ifcico_system_name, ifcico_addresses, ifcico_response_type, ifcico_error,
				telnet_tested, telnet_success, telnet_response_ms, telnet_error,
				ftp_tested, ftp_success, ftp_response_ms, ftp_error,
				vmodem_tested, vmodem_success, vmodem_response_ms, vmodem_error,
				is_operational, has_connectivity_issues, address_validated
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ? AND test_time = parseDateTimeBestEffort(?)
			LIMIT 1
		`
	} else {
		// DuckDB query
		query = `
			SELECT
				test_time, zone, net, node, address, hostname,
				resolved_ipv4, resolved_ipv6, dns_error,
				country, country_code, city, region, latitude, longitude, isp, org, asn,
				binkp_tested, binkp_success, binkp_response_ms, binkp_system_name,
				binkp_sysop, binkp_location, binkp_version, binkp_addresses, binkp_capabilities, binkp_error,
				ifcico_tested, ifcico_success, ifcico_response_ms, ifcico_mailer_info,
				ifcico_system_name, ifcico_addresses, ifcico_response_type, ifcico_error,
				telnet_tested, telnet_success, telnet_response_ms, telnet_error,
				ftp_tested, ftp_success, ftp_response_ms, ftp_error,
				vmodem_tested, vmodem_success, vmodem_response_ms, vmodem_error,
				is_operational, has_connectivity_issues, address_validated
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ? AND test_time = ?
			LIMIT 1
		`
	}

	row := conn.QueryRow(query, zone, net, node, testTime)

	var result NodeTestResult
	err := to.resultParser.ParseTestResultRow(&singleRowScanner{row}, &result)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to parse detailed test result: %w", err)
	}

	return &result, nil
}

// singleRowScanner wraps sql.Row to implement RowScanner interface
type singleRowScanner struct {
	*sql.Row
}

// GetNodeReachabilityStats calculates reachability statistics for a node
func (to *TestOperations) GetNodeReachabilityStats(zone, net, node int, days int) (*NodeReachabilityStats, error) {
	to.mu.RLock()
	defer to.mu.RUnlock()

	conn := to.db.Conn()
	
	var query string
	if _, isClickHouse := to.db.(*database.ClickHouseDB); isClickHouse {
		query = `
			SELECT
				zone,
				net,
				node,
				count(*) as total_tests,
				countIf(is_operational) as successful_tests,
				countIf(NOT is_operational) as failed_tests,
				avg(is_operational) * 100 as success_rate,
				avgIf(least(binkp_response_ms, ifcico_response_ms, telnet_response_ms), is_operational) as avg_response_ms,
				max(test_time) as last_test_time,
				argMax(is_operational, test_time) as last_status,
				avgIf(binkp_success, binkp_tested) * 100 as binkp_success_rate,
				avgIf(ifcico_success, ifcico_tested) * 100 as ifcico_success_rate,
				avgIf(telnet_success, telnet_tested) * 100 as telnet_success_rate
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ?
			AND test_time >= now() - INTERVAL ? DAY
			GROUP BY zone, net, node
		`
	} else {
		// DuckDB query
		query = `
			SELECT
				zone,
				net,
				node,
				count(*) as total_tests,
				sum(CASE WHEN is_operational THEN 1 ELSE 0 END) as successful_tests,
				sum(CASE WHEN NOT is_operational THEN 1 ELSE 0 END) as failed_tests,
				avg(CASE WHEN is_operational THEN 1.0 ELSE 0.0 END) * 100 as success_rate,
				avg(CASE WHEN is_operational THEN 
					LEAST(
						CASE WHEN binkp_response_ms > 0 THEN binkp_response_ms ELSE 999999 END,
						CASE WHEN ifcico_response_ms > 0 THEN ifcico_response_ms ELSE 999999 END,
						CASE WHEN telnet_response_ms > 0 THEN telnet_response_ms ELSE 999999 END
					) 
				END) as avg_response_ms,
				max(test_time) as last_test_time,
				bool_or(is_operational) FILTER (WHERE test_time = max(test_time)) as last_status,
				avg(CASE WHEN binkp_tested THEN CASE WHEN binkp_success THEN 1.0 ELSE 0.0 END END) * 100 as binkp_success_rate,
				avg(CASE WHEN ifcico_tested THEN CASE WHEN ifcico_success THEN 1.0 ELSE 0.0 END END) * 100 as ifcico_success_rate,
				avg(CASE WHEN telnet_tested THEN CASE WHEN telnet_success THEN 1.0 ELSE 0.0 END END) * 100 as telnet_success_rate
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ?
			AND test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
			GROUP BY zone, net, node
		`
	}

	row := conn.QueryRow(query, zone, net, node, days)
	
	var stats NodeReachabilityStats
	var lastStatus bool
	err := row.Scan(
		&stats.Zone,
		&stats.Net,
		&stats.Node,
		&stats.TotalTests,
		&stats.SuccessfulTests,
		&stats.FailedTests,
		&stats.SuccessRate,
		&stats.AverageResponseMs,
		&stats.LastTestTime,
		&lastStatus,
		&stats.BinkPSuccessRate,
		&stats.IfcicoSuccessRate,
		&stats.TelnetSuccessRate,
	)
	
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get reachability stats: %w", err)
	}
	
	if lastStatus {
		stats.LastStatus = "Operational"
	} else {
		stats.LastStatus = "Failed"
	}
	
	return &stats, nil
}

// GetReachabilityTrends gets daily reachability trends
func (to *TestOperations) GetReachabilityTrends(days int) ([]ReachabilityTrend, error) {
	to.mu.RLock()
	defer to.mu.RUnlock()

	conn := to.db.Conn()
	
	var query string
	if _, isClickHouse := to.db.(*database.ClickHouseDB); isClickHouse {
		query = `
			SELECT
				toDate(test_time) as test_date,
				count(DISTINCT (zone, net, node)) as total_nodes,
				countDistinctIf((zone, net, node), is_operational) as operational_nodes,
				countDistinctIf((zone, net, node), NOT is_operational) as failed_nodes,
				avg(is_operational) * 100 as success_rate,
				avgIf(least(binkp_response_ms, ifcico_response_ms, telnet_response_ms), is_operational) as avg_response_ms
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
			GROUP BY test_date
			ORDER BY test_date ASC
		`
	} else {
		// DuckDB query
		query = `
			SELECT
				DATE(test_time) as test_date,
				count(DISTINCT (zone, net, node)) as total_nodes,
				count(DISTINCT CASE WHEN is_operational THEN (zone, net, node) END) as operational_nodes,
				count(DISTINCT CASE WHEN NOT is_operational THEN (zone, net, node) END) as failed_nodes,
				avg(CASE WHEN is_operational THEN 1.0 ELSE 0.0 END) * 100 as success_rate,
				avg(CASE WHEN is_operational THEN 
					LEAST(
						CASE WHEN binkp_response_ms > 0 THEN binkp_response_ms ELSE 999999 END,
						CASE WHEN ifcico_response_ms > 0 THEN ifcico_response_ms ELSE 999999 END,
						CASE WHEN telnet_response_ms > 0 THEN telnet_response_ms ELSE 999999 END
					) 
				END) as avg_response_ms
			FROM node_test_results
			WHERE test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
			GROUP BY test_date
			ORDER BY test_date ASC
		`
	}

	rows, err := conn.Query(query, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query reachability trends: %w", err)
	}
	defer rows.Close()

	var trends []ReachabilityTrend
	for rows.Next() {
		var t ReachabilityTrend
		err := rows.Scan(
			&t.Date,
			&t.TotalNodes,
			&t.OperationalNodes,
			&t.FailedNodes,
			&t.SuccessRate,
			&t.AvgResponseMs,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trend: %w", err)
		}
		trends = append(trends, t)
	}

	return trends, nil
}

// SearchNodesByReachability searches for nodes by reachability status
func (to *TestOperations) SearchNodesByReachability(operational bool, limit int, days int) ([]NodeTestResult, error) {
	to.mu.RLock()
	defer to.mu.RUnlock()

	conn := to.db.Conn()

	var query string
	if _, isClickHouse := to.db.(*database.ClickHouseDB); isClickHouse {
		query = `
			SELECT
				test_time, zone, net, node, address, hostname,
				resolved_ipv4, resolved_ipv6, dns_error,
				country, country_code, city, region, latitude, longitude, isp, org, asn,
				binkp_tested, binkp_success, binkp_response_ms, binkp_system_name,
				binkp_sysop, binkp_location, binkp_version, binkp_addresses, binkp_capabilities, binkp_error,
				ifcico_tested, ifcico_success, ifcico_response_ms, ifcico_mailer_info,
				ifcico_system_name, ifcico_addresses, ifcico_response_type, ifcico_error,
				telnet_tested, telnet_success, telnet_response_ms, telnet_error,
				ftp_tested, ftp_success, ftp_response_ms, ftp_error,
				vmodem_tested, vmodem_success, vmodem_response_ms, vmodem_error,
				is_operational, has_connectivity_issues, address_validated
			FROM (
				SELECT *, row_number() OVER (PARTITION BY zone, net, node ORDER BY test_time DESC) as rn
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL ? DAY
			)
			WHERE rn = 1 AND is_operational = ?
			ORDER BY test_time DESC
			LIMIT ?
		`
	} else {
		// DuckDB query
		query = `
			SELECT DISTINCT ON (zone, net, node)
				test_time, zone, net, node, address, hostname,
				resolved_ipv4, resolved_ipv6, dns_error,
				country, country_code, city, region, latitude, longitude, isp, org, asn,
				binkp_tested, binkp_success, binkp_response_ms, binkp_system_name,
				binkp_sysop, binkp_location, binkp_version, binkp_addresses, binkp_capabilities, binkp_error,
				ifcico_tested, ifcico_success, ifcico_response_ms, ifcico_mailer_info,
				ifcico_system_name, ifcico_addresses, ifcico_response_type, ifcico_error,
				telnet_tested, telnet_success, telnet_response_ms, telnet_error,
				ftp_tested, ftp_success, ftp_response_ms, ftp_error,
				vmodem_tested, vmodem_success, vmodem_response_ms, vmodem_error,
				is_operational, has_connectivity_issues, address_validated
			FROM node_test_results
			WHERE test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
			AND is_operational = ?
			ORDER BY zone, net, node, test_time DESC
			LIMIT ?
		`
	}

	rows, err := conn.Query(query, days, operational, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search nodes by reachability: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	for rows.Next() {
		var r NodeTestResult
		err := to.resultParser.ParseTestResultRow(rows, &r)
		if err != nil {
			return nil, fmt.Errorf("failed to parse test result: %w", err)
		}
		results = append(results, r)
	}

	return results, nil
}

