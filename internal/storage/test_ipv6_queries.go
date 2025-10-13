package storage

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/logging"
)

// IPv6QueryOperations handles IPv6-specific test result queries
type IPv6QueryOperations struct {
	db           database.DatabaseInterface
	queryBuilder *TestQueryBuilder
	resultParser ResultParserInterface
	mu           sync.RWMutex
}

// NewIPv6QueryOperations creates a new IPv6 query operations instance
func NewIPv6QueryOperations(db database.DatabaseInterface, queryBuilder *TestQueryBuilder, resultParser ResultParserInterface) *IPv6QueryOperations {
	return &IPv6QueryOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// getAllHostnamesForNode fetches all tested hostnames for a specific node that have IPv6
func (ipv6 *IPv6QueryOperations) getAllHostnamesForNode(zone, net, node int, days int) ([]string, error) {
	conn := ipv6.db.Conn()

	var query string
	if _, isClickHouse := ipv6.db.(*database.ClickHouseDB); isClickHouse {
		query = `
			SELECT DISTINCT tested_hostname
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ?
				AND test_time >= now() - INTERVAL ? DAY
				AND length(tested_hostname) > 0
				AND hostname_index >= 0
				AND length(resolved_ipv6) > 0
			ORDER BY hostname_index`
	} else {
		query = `
			SELECT DISTINCT tested_hostname
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ?
				AND test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
				AND length(tested_hostname) > 0
				AND hostname_index >= 0
				AND array_length(resolved_ipv6) > 0
			ORDER BY hostname_index`
	}

	rows, err := conn.Query(query, zone, net, node, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get hostnames: %w", err)
	}
	defer rows.Close()

	var hostnames []string
	for rows.Next() {
		var hostname string
		if err := rows.Scan(&hostname); err != nil {
			return nil, fmt.Errorf("failed to scan hostname: %w", err)
		}
		if hostname != "" {
			hostnames = append(hostnames, hostname)
		}
	}

	return hostnames, nil
}

// GetIPv6EnabledNodes returns nodes that have been successfully tested with IPv6
func (ipv6 *IPv6QueryOperations) GetIPv6EnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	ipv6.mu.RLock()
	defer ipv6.mu.RUnlock()

	conn := ipv6.db.Conn()

	// Build node filter condition
	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	var query string
	if _, isClickHouse := ipv6.db.(*database.ClickHouseDB); isClickHouse {
		query = fmt.Sprintf(`
			WITH latest_tests AS (
				SELECT
					zone, net, node,
					max(test_time) as latest_test_time
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL ? DAY
					AND length(resolved_ipv6) > 0
					AND is_operational = true
					AND (binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true)
					%s
				GROUP BY zone, net, node
			),
			latest_nodes AS (
				SELECT
					zone, net, node,
					argMax(system_name, nodelist_date) as system_name
				FROM nodes
				GROUP BY zone, net, node
			),
			ranked_results AS (
				SELECT
					r.test_time, r.zone, r.net, r.node, r.address, r.hostname,
					r.resolved_ipv4, r.resolved_ipv6, r.dns_error,
					r.country, r.country_code, r.city, r.region, r.latitude, r.longitude, r.isp, r.org, r.asn,
					r.binkp_tested, r.binkp_success, r.binkp_response_ms, r.binkp_system_name,
					r.binkp_sysop, r.binkp_location, r.binkp_version, r.binkp_addresses, r.binkp_capabilities, r.binkp_error,
					r.ifcico_tested, r.ifcico_success, r.ifcico_response_ms, r.ifcico_mailer_info, r.ifcico_system_name,
					r.ifcico_addresses, r.ifcico_response_type, r.ifcico_error,
					r.telnet_tested, r.telnet_success, r.telnet_response_ms, r.telnet_error,
					r.ftp_tested, r.ftp_success, r.ftp_response_ms, r.ftp_error,
					r.vmodem_tested, r.vmodem_success, r.vmodem_response_ms, r.vmodem_error,
					r.binkp_ipv6_tested, r.binkp_ipv6_success, r.binkp_ipv6_error,
					r.ifcico_ipv6_tested, r.ifcico_ipv6_success, r.ifcico_ipv6_error,
					r.telnet_ipv6_tested, r.telnet_ipv6_success, r.telnet_ipv6_error,
					r.is_operational, r.has_connectivity_issues, r.address_validated,
					r.tested_hostname, r.hostname_index, r.is_aggregated,
					r.total_hostnames, r.hostnames_tested, r.hostnames_operational,
					row_number() OVER (PARTITION BY r.zone, r.net, r.node ORDER BY r.is_aggregated DESC, r.hostname_index ASC) as rn
				FROM node_test_results r
				INNER JOIN latest_tests lt ON r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node
					AND r.test_time = lt.latest_test_time
			)
			SELECT
				rr.test_time, rr.zone, rr.net, rr.node, rr.address, rr.hostname,
				rr.resolved_ipv4, rr.resolved_ipv6, rr.dns_error,
				rr.country, rr.country_code, rr.city, rr.region, rr.latitude, rr.longitude, rr.isp, rr.org, rr.asn,
				rr.binkp_tested, rr.binkp_success, rr.binkp_response_ms,
				COALESCE(n.system_name, rr.binkp_system_name) as binkp_system_name,
				rr.binkp_sysop, rr.binkp_location, rr.binkp_version, rr.binkp_addresses, rr.binkp_capabilities, rr.binkp_error,
				rr.ifcico_tested, rr.ifcico_success, rr.ifcico_response_ms, rr.ifcico_mailer_info,
				COALESCE(n.system_name, rr.ifcico_system_name) as ifcico_system_name,
				rr.ifcico_addresses, rr.ifcico_response_type, rr.ifcico_error,
				rr.telnet_tested, rr.telnet_success, rr.telnet_response_ms, rr.telnet_error,
				rr.ftp_tested, rr.ftp_success, rr.ftp_response_ms, rr.ftp_error,
				rr.vmodem_tested, rr.vmodem_success, rr.vmodem_response_ms, rr.vmodem_error,
				rr.binkp_ipv6_tested, rr.binkp_ipv6_success, rr.binkp_ipv6_error,
				rr.ifcico_ipv6_tested, rr.ifcico_ipv6_success, rr.ifcico_ipv6_error,
				rr.telnet_ipv6_tested, rr.telnet_ipv6_success, rr.telnet_ipv6_error,
				rr.is_operational, rr.has_connectivity_issues, rr.address_validated,
				rr.tested_hostname, rr.hostname_index, rr.is_aggregated,
				rr.total_hostnames, rr.hostnames_tested, rr.hostnames_operational
			FROM ranked_results rr
			LEFT JOIN latest_nodes n ON rr.zone = n.zone AND rr.net = n.net AND rr.node = n.node
			WHERE rr.rn = 1
			ORDER BY rr.test_time DESC
			LIMIT ?`, nodeFilter)
	} else {
		query = fmt.Sprintf(`
			WITH latest_tests AS (
				SELECT
					zone, net, node,
					max(test_time) as latest_test_time
				FROM node_test_results
				WHERE test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
					AND array_length(resolved_ipv6) > 0
					AND is_operational = true
					AND (binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true)
					%s
				GROUP BY zone, net, node
			),
			latest_nodes AS (
				SELECT
					zone, net, node,
					FIRST(system_name ORDER BY nodelist_date DESC) as system_name
				FROM nodes
				GROUP BY zone, net, node
			)
			SELECT DISTINCT ON (r.zone, r.net, r.node)
				r.test_time, r.zone, r.net, r.node, r.address, r.hostname,
				r.resolved_ipv4, r.resolved_ipv6, r.dns_error,
				r.country, r.country_code, r.city, r.region, r.latitude, r.longitude, r.isp, r.org, r.asn,
				r.binkp_tested, r.binkp_success, r.binkp_response_ms,
				COALESCE(n.system_name, r.binkp_system_name) as binkp_system_name,
				r.binkp_sysop, r.binkp_location, r.binkp_version, r.binkp_addresses, r.binkp_capabilities, r.binkp_error,
				r.ifcico_tested, r.ifcico_success, r.ifcico_response_ms, r.ifcico_mailer_info,
				COALESCE(n.system_name, r.ifcico_system_name) as ifcico_system_name,
				r.ifcico_addresses, r.ifcico_response_type, r.ifcico_error,
				r.telnet_tested, r.telnet_success, r.telnet_response_ms, r.telnet_error,
				r.ftp_tested, r.ftp_success, r.ftp_response_ms, r.ftp_error,
				r.vmodem_tested, r.vmodem_success, r.vmodem_response_ms, r.vmodem_error,
				r.binkp_ipv6_tested, r.binkp_ipv6_success, r.binkp_ipv6_error,
				r.ifcico_ipv6_tested, r.ifcico_ipv6_success, r.ifcico_ipv6_error,
				r.telnet_ipv6_tested, r.telnet_ipv6_success, r.telnet_ipv6_error,
				r.is_operational, r.has_connectivity_issues, r.address_validated,
				r.tested_hostname, r.hostname_index, r.is_aggregated,
				r.total_hostnames, r.hostnames_tested, r.hostnames_operational
			FROM node_test_results r
			INNER JOIN latest_tests lt ON r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node
				AND r.test_time = lt.latest_test_time
			LEFT JOIN latest_nodes n ON r.zone = n.zone AND r.net = n.net AND r.node = n.node
			ORDER BY r.zone, r.net, r.node, r.test_time DESC
			LIMIT ?`, nodeFilter)
	}

	rows, err := conn.Query(query, days, limit)
	if err != nil {
		logging.Error("GetIPv6EnabledNodes: Query failed", slog.Any("error", err))
		return nil, fmt.Errorf("failed to search IPv6 enabled nodes: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	rowCount := 0
	for rows.Next() {
		rowCount++
		var r NodeTestResult
		err := ipv6.resultParser.ParseTestResultRow(rows, &r)
		if err != nil {
			logging.Error("GetIPv6EnabledNodes: Failed to parse row", slog.Int("row", rowCount), slog.Any("error", err))
			return nil, fmt.Errorf("failed to parse test result row %d: %w", rowCount, err)
		}
		results = append(results, r)
	}

	// Fetch all hostnames for each node
	for i := range results {
		hostnames, err := ipv6.getAllHostnamesForNode(results[i].Zone, results[i].Net, results[i].Node, days)
		if err != nil {
			logging.Warn("Failed to get all hostnames for node",
				slog.Int("zone", results[i].Zone),
				slog.Int("net", results[i].Net),
				slog.Int("node", results[i].Node),
				slog.Any("error", err))
		} else {
			results[i].AllHostnames = hostnames
		}
	}

	return results, nil
}

// GetIPv6NonWorkingNodes returns nodes that have IPv6 addresses but no working IPv6 services
func (ipv6 *IPv6QueryOperations) GetIPv6NonWorkingNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	ipv6.mu.RLock()
	defer ipv6.mu.RUnlock()

	conn := ipv6.db.Conn()

	// Build node filter condition
	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	var query string
	if _, isClickHouse := ipv6.db.(*database.ClickHouseDB); isClickHouse {
		query = fmt.Sprintf(`
			WITH
			-- Find nodes that have IPv6 addresses and were tested
			nodes_with_ipv6 AS (
				SELECT DISTINCT zone, net, node
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL ? DAY
					AND length(resolved_ipv6) > 0
					AND (binkp_ipv6_tested = true OR ifcico_ipv6_tested = true OR telnet_ipv6_tested = true)
					%s
			),
			-- Count successful IPv6 tests per node in the period
			ipv6_success_counts AS (
				SELECT
					zone, net, node,
					countIf(binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true) as success_count
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL ? DAY
					AND (zone, net, node) IN (SELECT zone, net, node FROM nodes_with_ipv6)
				GROUP BY zone, net, node
			),
			-- Get latest test for nodes with zero successful IPv6 tests
			latest_failed_tests AS (
				SELECT
					zone, net, node,
					max(test_time) as latest_test_time
				FROM node_test_results
				WHERE (zone, net, node) IN (
					SELECT zone, net, node
					FROM ipv6_success_counts
					WHERE success_count = 0
				)
				AND test_time >= now() - INTERVAL ? DAY
				GROUP BY zone, net, node
			),
			latest_nodes AS (
				SELECT
					zone, net, node,
					argMax(system_name, nodelist_date) as system_name
				FROM nodes
				GROUP BY zone, net, node
			),
			ranked_results AS (
				SELECT
					r.test_time, r.zone, r.net, r.node, r.address, r.hostname,
					r.resolved_ipv4, r.resolved_ipv6, r.dns_error,
					r.country, r.country_code, r.city, r.region, r.latitude, r.longitude, r.isp, r.org, r.asn,
					r.binkp_tested, r.binkp_success, r.binkp_response_ms, r.binkp_system_name,
					r.binkp_sysop, r.binkp_location, r.binkp_version, r.binkp_addresses, r.binkp_capabilities, r.binkp_error,
					r.ifcico_tested, r.ifcico_success, r.ifcico_response_ms, r.ifcico_mailer_info, r.ifcico_system_name,
					r.ifcico_addresses, r.ifcico_response_type, r.ifcico_error,
					r.telnet_tested, r.telnet_success, r.telnet_response_ms, r.telnet_error,
					r.ftp_tested, r.ftp_success, r.ftp_response_ms, r.ftp_error,
					r.vmodem_tested, r.vmodem_success, r.vmodem_response_ms, r.vmodem_error,
					r.binkp_ipv6_tested, r.binkp_ipv6_success, r.binkp_ipv6_error,
					r.ifcico_ipv6_tested, r.ifcico_ipv6_success, r.ifcico_ipv6_error,
					r.telnet_ipv6_tested, r.telnet_ipv6_success, r.telnet_ipv6_error,
					r.is_operational, r.has_connectivity_issues, r.address_validated,
					r.tested_hostname, r.hostname_index, r.is_aggregated,
					r.total_hostnames, r.hostnames_tested, r.hostnames_operational,
					row_number() OVER (PARTITION BY r.zone, r.net, r.node ORDER BY r.is_aggregated DESC, r.hostname_index ASC) as rn
				FROM node_test_results r
				INNER JOIN latest_failed_tests lft ON r.zone = lft.zone AND r.net = lft.net AND r.node = lft.node
					AND r.test_time = lft.latest_test_time
			)
			SELECT
				rr.test_time, rr.zone, rr.net, rr.node, rr.address, rr.hostname,
				rr.resolved_ipv4, rr.resolved_ipv6, rr.dns_error,
				rr.country, rr.country_code, rr.city, rr.region, rr.latitude, rr.longitude, rr.isp, rr.org, rr.asn,
				rr.binkp_tested, rr.binkp_success, rr.binkp_response_ms,
				COALESCE(n.system_name, rr.binkp_system_name) as binkp_system_name,
				rr.binkp_sysop, rr.binkp_location, rr.binkp_version, rr.binkp_addresses, rr.binkp_capabilities, rr.binkp_error,
				rr.ifcico_tested, rr.ifcico_success, rr.ifcico_response_ms, rr.ifcico_mailer_info,
				COALESCE(n.system_name, rr.ifcico_system_name) as ifcico_system_name,
				rr.ifcico_addresses, rr.ifcico_response_type, rr.ifcico_error,
				rr.telnet_tested, rr.telnet_success, rr.telnet_response_ms, rr.telnet_error,
				rr.ftp_tested, rr.ftp_success, rr.ftp_response_ms, rr.ftp_error,
				rr.vmodem_tested, rr.vmodem_success, rr.vmodem_response_ms, rr.vmodem_error,
				rr.binkp_ipv6_tested, rr.binkp_ipv6_success, rr.binkp_ipv6_error,
				rr.ifcico_ipv6_tested, rr.ifcico_ipv6_success, rr.ifcico_ipv6_error,
				rr.telnet_ipv6_tested, rr.telnet_ipv6_success, rr.telnet_ipv6_error,
				rr.is_operational, rr.has_connectivity_issues, rr.address_validated,
				rr.tested_hostname, rr.hostname_index, rr.is_aggregated,
				rr.total_hostnames, rr.hostnames_tested, rr.hostnames_operational
			FROM ranked_results rr
			LEFT JOIN latest_nodes n ON rr.zone = n.zone AND rr.net = n.net AND rr.node = n.node
			WHERE rr.rn = 1
			ORDER BY rr.test_time DESC
			LIMIT ?`, nodeFilter)
	} else {
		query = fmt.Sprintf(`
			WITH
			-- Find nodes that have IPv6 addresses and were tested
			nodes_with_ipv6 AS (
				SELECT DISTINCT zone, net, node
				FROM node_test_results
				WHERE test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
					AND array_length(resolved_ipv6) > 0
					AND (binkp_ipv6_tested = true OR ifcico_ipv6_tested = true OR telnet_ipv6_tested = true)
					%s
			),
			-- Count successful IPv6 tests per node in the period
			ipv6_success_counts AS (
				SELECT
					zone, net, node,
					SUM(CASE WHEN (binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true) THEN 1 ELSE 0 END) as success_count
				FROM node_test_results
				WHERE test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
					AND (zone, net, node) IN (SELECT zone, net, node FROM nodes_with_ipv6)
				GROUP BY zone, net, node
			),
			-- Get latest test for nodes with zero successful IPv6 tests
			latest_failed_tests AS (
				SELECT
					zone, net, node,
					max(test_time) as latest_test_time
				FROM node_test_results
				WHERE (zone, net, node) IN (
					SELECT zone, net, node
					FROM ipv6_success_counts
					WHERE success_count = 0
				)
				AND test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
				GROUP BY zone, net, node
			),
			latest_nodes AS (
				SELECT
					zone, net, node,
					FIRST(system_name ORDER BY nodelist_date DESC) as system_name
				FROM nodes
				GROUP BY zone, net, node
			)
			SELECT DISTINCT ON (r.zone, r.net, r.node)
				r.test_time, r.zone, r.net, r.node, r.address, r.hostname,
				r.resolved_ipv4, r.resolved_ipv6, r.dns_error,
				r.country, r.country_code, r.city, r.region, r.latitude, r.longitude, r.isp, r.org, r.asn,
				r.binkp_tested, r.binkp_success, r.binkp_response_ms,
				COALESCE(n.system_name, r.binkp_system_name) as binkp_system_name,
				r.binkp_sysop, r.binkp_location, r.binkp_version, r.binkp_addresses, r.binkp_capabilities, r.binkp_error,
				r.ifcico_tested, r.ifcico_success, r.ifcico_response_ms, r.ifcico_mailer_info,
				COALESCE(n.system_name, r.ifcico_system_name) as ifcico_system_name,
				r.ifcico_addresses, r.ifcico_response_type, r.ifcico_error,
				r.telnet_tested, r.telnet_success, r.telnet_response_ms, r.telnet_error,
				r.ftp_tested, r.ftp_success, r.ftp_response_ms, r.ftp_error,
				r.vmodem_tested, r.vmodem_success, r.vmodem_response_ms, r.vmodem_error,
				r.binkp_ipv6_tested, r.binkp_ipv6_success, r.binkp_ipv6_error,
				r.ifcico_ipv6_tested, r.ifcico_ipv6_success, r.ifcico_ipv6_error,
				r.telnet_ipv6_tested, r.telnet_ipv6_success, r.telnet_ipv6_error,
				r.is_operational, r.has_connectivity_issues, r.address_validated,
				r.tested_hostname, r.hostname_index, r.is_aggregated,
				r.total_hostnames, r.hostnames_tested, r.hostnames_operational
			FROM node_test_results r
			INNER JOIN latest_failed_tests lft ON r.zone = lft.zone AND r.net = lft.net AND r.node = lft.node
				AND r.test_time = lft.latest_test_time
			LEFT JOIN latest_nodes n ON r.zone = n.zone AND r.net = n.net AND r.node = n.node
			ORDER BY r.zone, r.net, r.node, r.test_time DESC
			LIMIT ?`, nodeFilter)
	}

	rows, err := conn.Query(query, days, days, days, limit)
	if err != nil {
		logging.Error("GetIPv6NonWorkingNodes: Query failed", slog.Any("error", err))
		return nil, fmt.Errorf("failed to search IPv6 non-working nodes: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	rowCount := 0
	for rows.Next() {
		rowCount++
		var r NodeTestResult
		err := ipv6.resultParser.ParseTestResultRow(rows, &r)
		if err != nil {
			logging.Error("GetIPv6NonWorkingNodes: Failed to parse row", slog.Int("row", rowCount), slog.Any("error", err))
			return nil, fmt.Errorf("failed to parse test result row %d: %w", rowCount, err)
		}
		results = append(results, r)
	}

	// Fetch all hostnames for each node
	for i := range results {
		hostnames, err := ipv6.getAllHostnamesForNode(results[i].Zone, results[i].Net, results[i].Node, days)
		if err != nil {
			logging.Warn("Failed to get all hostnames for node",
				slog.Int("zone", results[i].Zone),
				slog.Int("net", results[i].Net),
				slog.Int("node", results[i].Node),
				slog.Any("error", err))
		} else {
			results[i].AllHostnames = hostnames
		}
	}

	return results, nil
}
