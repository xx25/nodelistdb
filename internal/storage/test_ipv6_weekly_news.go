package storage

import (
	"fmt"
	"log/slog"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/logging"
)

// IPv6WeeklyNews represents weekly changes in IPv6 connectivity
type IPv6WeeklyNews struct {
	NewNodesWorking    []NodeTestResult `json:"new_nodes_working"`     // New nodes with working IPv6
	NewNodesNonWorking []NodeTestResult `json:"new_nodes_non_working"` // New nodes with non-working IPv6
	OldNodesLostIPv6   []NodeTestResult `json:"old_nodes_lost_ipv6"`   // Old nodes that lost IPv6
	OldNodesGainedIPv6 []NodeTestResult `json:"old_nodes_gained_ipv6"` // Old nodes that gained IPv6
}

// GetIPv6WeeklyNews returns weekly IPv6 connectivity changes
func (ipv6 *IPv6QueryOperations) GetIPv6WeeklyNews(limit int, includeZeroNodes bool) (*IPv6WeeklyNews, error) {
	ipv6.mu.RLock()
	defer ipv6.mu.RUnlock()

	news := &IPv6WeeklyNews{}
	var err error

	// Get new nodes with working IPv6
	news.NewNodesWorking, err = ipv6.getNewNodesWithWorkingIPv6(limit, includeZeroNodes)
	if err != nil {
		logging.Error("GetIPv6WeeklyNews: Failed to get new nodes with working IPv6", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get new nodes with working IPv6: %w", err)
	}

	// Get new nodes with non-working IPv6
	news.NewNodesNonWorking, err = ipv6.getNewNodesWithNonWorkingIPv6(limit, includeZeroNodes)
	if err != nil {
		logging.Error("GetIPv6WeeklyNews: Failed to get new nodes with non-working IPv6", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get new nodes with non-working IPv6: %w", err)
	}

	// Get old nodes that lost IPv6
	news.OldNodesLostIPv6, err = ipv6.getOldNodesThatLostIPv6(limit, includeZeroNodes)
	if err != nil {
		logging.Error("GetIPv6WeeklyNews: Failed to get old nodes that lost IPv6", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get old nodes that lost IPv6: %w", err)
	}

	// Get old nodes that gained IPv6
	news.OldNodesGainedIPv6, err = ipv6.getOldNodesThatGainedIPv6(limit, includeZeroNodes)
	if err != nil {
		logging.Error("GetIPv6WeeklyNews: Failed to get old nodes that gained IPv6", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get old nodes that gained IPv6: %w", err)
	}

	return news, nil
}

// getNewNodesWithWorkingIPv6 returns nodes that appeared in nodelist in the last 7 days
// (were not in nodelist 7-14 days ago) and have working IPv6 services
func (ipv6 *IPv6QueryOperations) getNewNodesWithWorkingIPv6(limit int, includeZeroNodes bool) ([]NodeTestResult, error) {
	conn := ipv6.db.Conn()

	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	var query string
	if _, isClickHouse := ipv6.db.(*database.ClickHouseDB); isClickHouse {
		query = fmt.Sprintf(`
			WITH
			-- Nodes that were NOT in nodelist 7-14 days ago
			old_period_nodes AS (
				SELECT DISTINCT zone, net, node
				FROM nodes
				WHERE nodelist_date >= now() - INTERVAL 14 DAY
					AND nodelist_date < now() - INTERVAL 7 DAY
			),
			-- Nodes that ARE in nodelist in last 7 days (new arrivals)
			recent_nodes AS (
				SELECT DISTINCT zone, net, node
				FROM nodes
				WHERE nodelist_date >= now() - INTERVAL 7 DAY
					AND (zone, net, node) NOT IN (SELECT zone, net, node FROM old_period_nodes)
					%s
			),
			-- Get latest test results for these new nodes with working IPv6
			latest_tests AS (
				SELECT
					zone, net, node,
					max(test_time) as latest_test_time
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL 7 DAY
					AND length(resolved_ipv6) > 0
					AND is_operational = true
					AND (binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true OR ftp_success = true)
					AND (zone, net, node) IN (SELECT zone, net, node FROM recent_nodes)
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
					r.binkp_ipv4_tested, r.binkp_ipv4_success, r.binkp_ipv4_response_ms, r.binkp_ipv4_address, r.binkp_ipv4_error,
					r.binkp_ipv6_tested, r.binkp_ipv6_success, r.binkp_ipv6_response_ms, r.binkp_ipv6_address, r.binkp_ipv6_error,
					r.ifcico_ipv4_tested, r.ifcico_ipv4_success, r.ifcico_ipv4_response_ms, r.ifcico_ipv4_address, r.ifcico_ipv4_error,
					r.ifcico_ipv6_tested, r.ifcico_ipv6_success, r.ifcico_ipv6_response_ms, r.ifcico_ipv6_address, r.ifcico_ipv6_error,
					r.telnet_ipv4_tested, r.telnet_ipv4_success, r.telnet_ipv4_response_ms, r.telnet_ipv4_address, r.telnet_ipv4_error,
					r.telnet_ipv6_tested, r.telnet_ipv6_success, r.telnet_ipv6_response_ms, r.telnet_ipv6_address, r.telnet_ipv6_error,
					r.ftp_ipv4_tested, r.ftp_ipv4_success, r.ftp_ipv4_response_ms, r.ftp_ipv4_address, r.ftp_ipv4_error,
					r.ftp_ipv6_tested, r.ftp_ipv6_success, r.ftp_ipv6_response_ms, r.ftp_ipv6_address, r.ftp_ipv6_error,
					r.vmodem_ipv4_tested, r.vmodem_ipv4_success, r.vmodem_ipv4_response_ms, r.vmodem_ipv4_address, r.vmodem_ipv4_error,
					r.vmodem_ipv6_tested, r.vmodem_ipv6_success, r.vmodem_ipv6_response_ms, r.vmodem_ipv6_address, r.vmodem_ipv6_error,
					r.is_operational, r.has_connectivity_issues, r.address_validated,
					r.tested_hostname, r.hostname_index, r.is_aggregated,
					r.total_hostnames, r.hostnames_tested, r.hostnames_operational,
					r.ftp_anon_success,
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
				rr.binkp_ipv4_tested, rr.binkp_ipv4_success, rr.binkp_ipv4_response_ms, rr.binkp_ipv4_address, rr.binkp_ipv4_error,
				rr.binkp_ipv6_tested, rr.binkp_ipv6_success, rr.binkp_ipv6_response_ms, rr.binkp_ipv6_address, rr.binkp_ipv6_error,
				rr.ifcico_ipv4_tested, rr.ifcico_ipv4_success, rr.ifcico_ipv4_response_ms, rr.ifcico_ipv4_address, rr.ifcico_ipv4_error,
				rr.ifcico_ipv6_tested, rr.ifcico_ipv6_success, rr.ifcico_ipv6_response_ms, rr.ifcico_ipv6_address, rr.ifcico_ipv6_error,
				rr.telnet_ipv4_tested, rr.telnet_ipv4_success, rr.telnet_ipv4_response_ms, rr.telnet_ipv4_address, rr.telnet_ipv4_error,
				rr.telnet_ipv6_tested, rr.telnet_ipv6_success, rr.telnet_ipv6_response_ms, rr.telnet_ipv6_address, rr.telnet_ipv6_error,
				rr.ftp_ipv4_tested, rr.ftp_ipv4_success, rr.ftp_ipv4_response_ms, rr.ftp_ipv4_address, rr.ftp_ipv4_error,
				rr.ftp_ipv6_tested, rr.ftp_ipv6_success, rr.ftp_ipv6_response_ms, rr.ftp_ipv6_address, rr.ftp_ipv6_error,
				rr.vmodem_ipv4_tested, rr.vmodem_ipv4_success, rr.vmodem_ipv4_response_ms, rr.vmodem_ipv4_address, rr.vmodem_ipv4_error,
				rr.vmodem_ipv6_tested, rr.vmodem_ipv6_success, rr.vmodem_ipv6_response_ms, rr.vmodem_ipv6_address, rr.vmodem_ipv6_error,
				rr.is_operational, rr.has_connectivity_issues, rr.address_validated,
				rr.tested_hostname, rr.hostname_index, rr.is_aggregated,
				rr.total_hostnames, rr.hostnames_tested, rr.hostnames_operational,
				rr.ftp_anon_success
			FROM ranked_results rr
			LEFT JOIN latest_nodes n ON rr.zone = n.zone AND rr.net = n.net AND rr.node = n.node
			WHERE rr.rn = 1
			ORDER BY rr.test_time DESC
			LIMIT ?`, nodeFilter)
	} else {
		return nil, fmt.Errorf("DuckDB support not implemented for weekly IPv6 news")
	}

	rows, err := conn.Query(query, limit)
	if err != nil {
		logging.Error("getNewNodesWithWorkingIPv6: Query failed", slog.Any("error", err))
		return nil, fmt.Errorf("failed to query new nodes with working IPv6: %w", err)
	}
	defer rows.Close()

	return ipv6.parseTestResults(rows)
}

// getNewNodesWithNonWorkingIPv6 returns nodes that appeared in nodelist in the last 7 days
// but have IPv6 addresses with non-working services
func (ipv6 *IPv6QueryOperations) getNewNodesWithNonWorkingIPv6(limit int, includeZeroNodes bool) ([]NodeTestResult, error) {
	conn := ipv6.db.Conn()

	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	var query string
	if _, isClickHouse := ipv6.db.(*database.ClickHouseDB); isClickHouse {
		query = fmt.Sprintf(`
			WITH
			-- Nodes that were NOT in nodelist 7-14 days ago
			old_period_nodes AS (
				SELECT DISTINCT zone, net, node
				FROM nodes
				WHERE nodelist_date >= now() - INTERVAL 14 DAY
					AND nodelist_date < now() - INTERVAL 7 DAY
			),
			-- Nodes that ARE in nodelist in last 7 days (new arrivals)
			recent_nodes AS (
				SELECT DISTINCT zone, net, node
				FROM nodes
				WHERE nodelist_date >= now() - INTERVAL 7 DAY
					AND (zone, net, node) NOT IN (SELECT zone, net, node FROM old_period_nodes)
					%s
			),
			-- Nodes with IPv6 but no working services
			nodes_with_ipv6 AS (
				SELECT DISTINCT zone, net, node
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL 7 DAY
					AND length(resolved_ipv6) > 0
					AND (binkp_ipv6_tested = true OR ifcico_ipv6_tested = true OR telnet_ipv6_tested = true)
					AND (zone, net, node) IN (SELECT zone, net, node FROM recent_nodes)
			),
			-- Count successful IPv6 tests
			ipv6_success_counts AS (
				SELECT
					zone, net, node,
					countIf(binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true) as success_count
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL 7 DAY
					AND (zone, net, node) IN (SELECT zone, net, node FROM nodes_with_ipv6)
				GROUP BY zone, net, node
			),
			-- Get latest test for nodes with zero successes
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
				AND test_time >= now() - INTERVAL 7 DAY
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
					r.binkp_ipv4_tested, r.binkp_ipv4_success, r.binkp_ipv4_response_ms, r.binkp_ipv4_address, r.binkp_ipv4_error,
					r.binkp_ipv6_tested, r.binkp_ipv6_success, r.binkp_ipv6_response_ms, r.binkp_ipv6_address, r.binkp_ipv6_error,
					r.ifcico_ipv4_tested, r.ifcico_ipv4_success, r.ifcico_ipv4_response_ms, r.ifcico_ipv4_address, r.ifcico_ipv4_error,
					r.ifcico_ipv6_tested, r.ifcico_ipv6_success, r.ifcico_ipv6_response_ms, r.ifcico_ipv6_address, r.ifcico_ipv6_error,
					r.telnet_ipv4_tested, r.telnet_ipv4_success, r.telnet_ipv4_response_ms, r.telnet_ipv4_address, r.telnet_ipv4_error,
					r.telnet_ipv6_tested, r.telnet_ipv6_success, r.telnet_ipv6_response_ms, r.telnet_ipv6_address, r.telnet_ipv6_error,
					r.ftp_ipv4_tested, r.ftp_ipv4_success, r.ftp_ipv4_response_ms, r.ftp_ipv4_address, r.ftp_ipv4_error,
					r.ftp_ipv6_tested, r.ftp_ipv6_success, r.ftp_ipv6_response_ms, r.ftp_ipv6_address, r.ftp_ipv6_error,
					r.vmodem_ipv4_tested, r.vmodem_ipv4_success, r.vmodem_ipv4_response_ms, r.vmodem_ipv4_address, r.vmodem_ipv4_error,
					r.vmodem_ipv6_tested, r.vmodem_ipv6_success, r.vmodem_ipv6_response_ms, r.vmodem_ipv6_address, r.vmodem_ipv6_error,
					r.is_operational, r.has_connectivity_issues, r.address_validated,
					r.tested_hostname, r.hostname_index, r.is_aggregated,
					r.total_hostnames, r.hostnames_tested, r.hostnames_operational,
					r.ftp_anon_success,
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
				rr.binkp_ipv4_tested, rr.binkp_ipv4_success, rr.binkp_ipv4_response_ms, rr.binkp_ipv4_address, rr.binkp_ipv4_error,
				rr.binkp_ipv6_tested, rr.binkp_ipv6_success, rr.binkp_ipv6_response_ms, rr.binkp_ipv6_address, rr.binkp_ipv6_error,
				rr.ifcico_ipv4_tested, rr.ifcico_ipv4_success, rr.ifcico_ipv4_response_ms, rr.ifcico_ipv4_address, rr.ifcico_ipv4_error,
				rr.ifcico_ipv6_tested, rr.ifcico_ipv6_success, rr.ifcico_ipv6_response_ms, rr.ifcico_ipv6_address, rr.ifcico_ipv6_error,
				rr.telnet_ipv4_tested, rr.telnet_ipv4_success, rr.telnet_ipv4_response_ms, rr.telnet_ipv4_address, rr.telnet_ipv4_error,
				rr.telnet_ipv6_tested, rr.telnet_ipv6_success, rr.telnet_ipv6_response_ms, rr.telnet_ipv6_address, rr.telnet_ipv6_error,
				rr.ftp_ipv4_tested, rr.ftp_ipv4_success, rr.ftp_ipv4_response_ms, rr.ftp_ipv4_address, rr.ftp_ipv4_error,
				rr.ftp_ipv6_tested, rr.ftp_ipv6_success, rr.ftp_ipv6_response_ms, rr.ftp_ipv6_address, rr.ftp_ipv6_error,
				rr.vmodem_ipv4_tested, rr.vmodem_ipv4_success, rr.vmodem_ipv4_response_ms, rr.vmodem_ipv4_address, rr.vmodem_ipv4_error,
				rr.vmodem_ipv6_tested, rr.vmodem_ipv6_success, rr.vmodem_ipv6_response_ms, rr.vmodem_ipv6_address, rr.vmodem_ipv6_error,
				rr.is_operational, rr.has_connectivity_issues, rr.address_validated,
				rr.tested_hostname, rr.hostname_index, rr.is_aggregated,
				rr.total_hostnames, rr.hostnames_tested, rr.hostnames_operational,
				rr.ftp_anon_success
			FROM ranked_results rr
			LEFT JOIN latest_nodes n ON rr.zone = n.zone AND rr.net = n.net AND rr.node = n.node
			WHERE rr.rn = 1
			ORDER BY rr.test_time DESC
			LIMIT ?`, nodeFilter)
	} else {
		return nil, fmt.Errorf("DuckDB support not implemented for weekly IPv6 news")
	}

	rows, err := conn.Query(query, limit)
	if err != nil {
		logging.Error("getNewNodesWithNonWorkingIPv6: Query failed", slog.Any("error", err))
		return nil, fmt.Errorf("failed to query new nodes with non-working IPv6: %w", err)
	}
	defer rows.Close()

	return ipv6.parseTestResults(rows)
}

// getOldNodesThatLostIPv6 returns nodes that have been in nodelist for >14 days
// had IPv6 7-14 days ago, but lost it in the last 7 days
func (ipv6 *IPv6QueryOperations) getOldNodesThatLostIPv6(limit int, includeZeroNodes bool) ([]NodeTestResult, error) {
	conn := ipv6.db.Conn()

	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	var query string
	if _, isClickHouse := ipv6.db.(*database.ClickHouseDB); isClickHouse {
		query = fmt.Sprintf(`
			WITH
			-- Nodes that have been in nodelist for >14 days
			old_nodes AS (
				SELECT DISTINCT zone, net, node
				FROM nodes
				WHERE nodelist_date < now() - INTERVAL 14 DAY
					%s
			),
			-- Nodes with working IPv6 in 7-14 days ago period
			had_ipv6_before AS (
				SELECT DISTINCT zone, net, node
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL 14 DAY
					AND test_time < now() - INTERVAL 7 DAY
					AND length(resolved_ipv6) > 0
					AND (binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true)
					AND (zone, net, node) IN (SELECT zone, net, node FROM old_nodes)
			),
			-- Nodes without working IPv6 in last 7 days
			no_ipv6_now AS (
				SELECT zone, net, node
				FROM (
					SELECT
						zone, net, node,
						countIf(length(resolved_ipv6) > 0 AND (binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true)) as ipv6_success_count
					FROM node_test_results
					WHERE test_time >= now() - INTERVAL 7 DAY
						AND (zone, net, node) IN (SELECT zone, net, node FROM had_ipv6_before)
					GROUP BY zone, net, node
				)
				WHERE ipv6_success_count = 0
			),
			-- Get latest test results
			latest_tests AS (
				SELECT
					zone, net, node,
					max(test_time) as latest_test_time
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL 7 DAY
					AND (zone, net, node) IN (SELECT zone, net, node FROM no_ipv6_now)
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
					r.binkp_ipv4_tested, r.binkp_ipv4_success, r.binkp_ipv4_response_ms, r.binkp_ipv4_address, r.binkp_ipv4_error,
					r.binkp_ipv6_tested, r.binkp_ipv6_success, r.binkp_ipv6_response_ms, r.binkp_ipv6_address, r.binkp_ipv6_error,
					r.ifcico_ipv4_tested, r.ifcico_ipv4_success, r.ifcico_ipv4_response_ms, r.ifcico_ipv4_address, r.ifcico_ipv4_error,
					r.ifcico_ipv6_tested, r.ifcico_ipv6_success, r.ifcico_ipv6_response_ms, r.ifcico_ipv6_address, r.ifcico_ipv6_error,
					r.telnet_ipv4_tested, r.telnet_ipv4_success, r.telnet_ipv4_response_ms, r.telnet_ipv4_address, r.telnet_ipv4_error,
					r.telnet_ipv6_tested, r.telnet_ipv6_success, r.telnet_ipv6_response_ms, r.telnet_ipv6_address, r.telnet_ipv6_error,
					r.ftp_ipv4_tested, r.ftp_ipv4_success, r.ftp_ipv4_response_ms, r.ftp_ipv4_address, r.ftp_ipv4_error,
					r.ftp_ipv6_tested, r.ftp_ipv6_success, r.ftp_ipv6_response_ms, r.ftp_ipv6_address, r.ftp_ipv6_error,
					r.vmodem_ipv4_tested, r.vmodem_ipv4_success, r.vmodem_ipv4_response_ms, r.vmodem_ipv4_address, r.vmodem_ipv4_error,
					r.vmodem_ipv6_tested, r.vmodem_ipv6_success, r.vmodem_ipv6_response_ms, r.vmodem_ipv6_address, r.vmodem_ipv6_error,
					r.is_operational, r.has_connectivity_issues, r.address_validated,
					r.tested_hostname, r.hostname_index, r.is_aggregated,
					r.total_hostnames, r.hostnames_tested, r.hostnames_operational,
					r.ftp_anon_success,
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
				rr.binkp_ipv4_tested, rr.binkp_ipv4_success, rr.binkp_ipv4_response_ms, rr.binkp_ipv4_address, rr.binkp_ipv4_error,
				rr.binkp_ipv6_tested, rr.binkp_ipv6_success, rr.binkp_ipv6_response_ms, rr.binkp_ipv6_address, rr.binkp_ipv6_error,
				rr.ifcico_ipv4_tested, rr.ifcico_ipv4_success, rr.ifcico_ipv4_response_ms, rr.ifcico_ipv4_address, rr.ifcico_ipv4_error,
				rr.ifcico_ipv6_tested, rr.ifcico_ipv6_success, rr.ifcico_ipv6_response_ms, rr.ifcico_ipv6_address, rr.ifcico_ipv6_error,
				rr.telnet_ipv4_tested, rr.telnet_ipv4_success, rr.telnet_ipv4_response_ms, rr.telnet_ipv4_address, rr.telnet_ipv4_error,
				rr.telnet_ipv6_tested, rr.telnet_ipv6_success, rr.telnet_ipv6_response_ms, rr.telnet_ipv6_address, rr.telnet_ipv6_error,
				rr.ftp_ipv4_tested, rr.ftp_ipv4_success, rr.ftp_ipv4_response_ms, rr.ftp_ipv4_address, rr.ftp_ipv4_error,
				rr.ftp_ipv6_tested, rr.ftp_ipv6_success, rr.ftp_ipv6_response_ms, rr.ftp_ipv6_address, rr.ftp_ipv6_error,
				rr.vmodem_ipv4_tested, rr.vmodem_ipv4_success, rr.vmodem_ipv4_response_ms, rr.vmodem_ipv4_address, rr.vmodem_ipv4_error,
				rr.vmodem_ipv6_tested, rr.vmodem_ipv6_success, rr.vmodem_ipv6_response_ms, rr.vmodem_ipv6_address, rr.vmodem_ipv6_error,
				rr.is_operational, rr.has_connectivity_issues, rr.address_validated,
				rr.tested_hostname, rr.hostname_index, rr.is_aggregated,
				rr.total_hostnames, rr.hostnames_tested, rr.hostnames_operational,
				rr.ftp_anon_success
			FROM ranked_results rr
			LEFT JOIN latest_nodes n ON rr.zone = n.zone AND rr.net = n.net AND rr.node = n.node
			WHERE rr.rn = 1
			ORDER BY rr.test_time DESC
			LIMIT ?`, nodeFilter)
	} else {
		return nil, fmt.Errorf("DuckDB support not implemented for weekly IPv6 news")
	}

	rows, err := conn.Query(query, limit)
	if err != nil {
		logging.Error("getOldNodesThatLostIPv6: Query failed", slog.Any("error", err))
		return nil, fmt.Errorf("failed to query old nodes that lost IPv6: %w", err)
	}
	defer rows.Close()

	return ipv6.parseTestResults(rows)
}

// getOldNodesThatGainedIPv6 returns nodes that have been in nodelist for >14 days
// did not have working IPv6 7-14 days ago, but gained it in the last 7 days
func (ipv6 *IPv6QueryOperations) getOldNodesThatGainedIPv6(limit int, includeZeroNodes bool) ([]NodeTestResult, error) {
	conn := ipv6.db.Conn()

	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	var query string
	if _, isClickHouse := ipv6.db.(*database.ClickHouseDB); isClickHouse {
		query = fmt.Sprintf(`
			WITH
			-- Nodes that have been in nodelist for >14 days
			old_nodes AS (
				SELECT DISTINCT zone, net, node
				FROM nodes
				WHERE nodelist_date < now() - INTERVAL 14 DAY
					%s
			),
			-- Nodes tested 7-14 days ago (to check previous state)
			tested_before AS (
				SELECT DISTINCT zone, net, node
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL 14 DAY
					AND test_time < now() - INTERVAL 7 DAY
					AND (zone, net, node) IN (SELECT zone, net, node FROM old_nodes)
			),
			-- Nodes without working IPv6 in 7-14 days ago period
			no_ipv6_before AS (
				SELECT zone, net, node
				FROM (
					SELECT
						zone, net, node,
						countIf(length(resolved_ipv6) > 0 AND (binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true)) as ipv6_success_count
					FROM node_test_results
					WHERE test_time >= now() - INTERVAL 14 DAY
						AND test_time < now() - INTERVAL 7 DAY
						AND (zone, net, node) IN (SELECT zone, net, node FROM tested_before)
					GROUP BY zone, net, node
				)
				WHERE ipv6_success_count = 0
			),
			-- Nodes with working IPv6 in last 7 days
			has_ipv6_now AS (
				SELECT DISTINCT zone, net, node
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL 7 DAY
					AND length(resolved_ipv6) > 0
					AND (binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true)
					AND (zone, net, node) IN (SELECT zone, net, node FROM no_ipv6_before)
			),
			-- Get latest test results
			latest_tests AS (
				SELECT
					zone, net, node,
					max(test_time) as latest_test_time
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL 7 DAY
					AND (zone, net, node) IN (SELECT zone, net, node FROM has_ipv6_now)
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
					r.binkp_ipv4_tested, r.binkp_ipv4_success, r.binkp_ipv4_response_ms, r.binkp_ipv4_address, r.binkp_ipv4_error,
					r.binkp_ipv6_tested, r.binkp_ipv6_success, r.binkp_ipv6_response_ms, r.binkp_ipv6_address, r.binkp_ipv6_error,
					r.ifcico_ipv4_tested, r.ifcico_ipv4_success, r.ifcico_ipv4_response_ms, r.ifcico_ipv4_address, r.ifcico_ipv4_error,
					r.ifcico_ipv6_tested, r.ifcico_ipv6_success, r.ifcico_ipv6_response_ms, r.ifcico_ipv6_address, r.ifcico_ipv6_error,
					r.telnet_ipv4_tested, r.telnet_ipv4_success, r.telnet_ipv4_response_ms, r.telnet_ipv4_address, r.telnet_ipv4_error,
					r.telnet_ipv6_tested, r.telnet_ipv6_success, r.telnet_ipv6_response_ms, r.telnet_ipv6_address, r.telnet_ipv6_error,
					r.ftp_ipv4_tested, r.ftp_ipv4_success, r.ftp_ipv4_response_ms, r.ftp_ipv4_address, r.ftp_ipv4_error,
					r.ftp_ipv6_tested, r.ftp_ipv6_success, r.ftp_ipv6_response_ms, r.ftp_ipv6_address, r.ftp_ipv6_error,
					r.vmodem_ipv4_tested, r.vmodem_ipv4_success, r.vmodem_ipv4_response_ms, r.vmodem_ipv4_address, r.vmodem_ipv4_error,
					r.vmodem_ipv6_tested, r.vmodem_ipv6_success, r.vmodem_ipv6_response_ms, r.vmodem_ipv6_address, r.vmodem_ipv6_error,
					r.is_operational, r.has_connectivity_issues, r.address_validated,
					r.tested_hostname, r.hostname_index, r.is_aggregated,
					r.total_hostnames, r.hostnames_tested, r.hostnames_operational,
					r.ftp_anon_success,
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
				rr.binkp_ipv4_tested, rr.binkp_ipv4_success, rr.binkp_ipv4_response_ms, rr.binkp_ipv4_address, rr.binkp_ipv4_error,
				rr.binkp_ipv6_tested, rr.binkp_ipv6_success, rr.binkp_ipv6_response_ms, rr.binkp_ipv6_address, rr.binkp_ipv6_error,
				rr.ifcico_ipv4_tested, rr.ifcico_ipv4_success, rr.ifcico_ipv4_response_ms, rr.ifcico_ipv4_address, rr.ifcico_ipv4_error,
				rr.ifcico_ipv6_tested, rr.ifcico_ipv6_success, rr.ifcico_ipv6_response_ms, rr.ifcico_ipv6_address, rr.ifcico_ipv6_error,
				rr.telnet_ipv4_tested, rr.telnet_ipv4_success, rr.telnet_ipv4_response_ms, rr.telnet_ipv4_address, rr.telnet_ipv4_error,
				rr.telnet_ipv6_tested, rr.telnet_ipv6_success, rr.telnet_ipv6_response_ms, rr.telnet_ipv6_address, rr.telnet_ipv6_error,
				rr.ftp_ipv4_tested, rr.ftp_ipv4_success, rr.ftp_ipv4_response_ms, rr.ftp_ipv4_address, rr.ftp_ipv4_error,
				rr.ftp_ipv6_tested, rr.ftp_ipv6_success, rr.ftp_ipv6_response_ms, rr.ftp_ipv6_address, rr.ftp_ipv6_error,
				rr.vmodem_ipv4_tested, rr.vmodem_ipv4_success, rr.vmodem_ipv4_response_ms, rr.vmodem_ipv4_address, rr.vmodem_ipv4_error,
				rr.vmodem_ipv6_tested, rr.vmodem_ipv6_success, rr.vmodem_ipv6_response_ms, rr.vmodem_ipv6_address, rr.vmodem_ipv6_error,
				rr.is_operational, rr.has_connectivity_issues, rr.address_validated,
				rr.tested_hostname, rr.hostname_index, rr.is_aggregated,
				rr.total_hostnames, rr.hostnames_tested, rr.hostnames_operational,
				rr.ftp_anon_success
			FROM ranked_results rr
			LEFT JOIN latest_nodes n ON rr.zone = n.zone AND rr.net = n.net AND rr.node = n.node
			WHERE rr.rn = 1
			ORDER BY rr.test_time DESC
			LIMIT ?`, nodeFilter)
	} else {
		return nil, fmt.Errorf("DuckDB support not implemented for weekly IPv6 news")
	}

	rows, err := conn.Query(query, limit)
	if err != nil {
		logging.Error("getOldNodesThatGainedIPv6: Query failed", slog.Any("error", err))
		return nil, fmt.Errorf("failed to query old nodes that gained IPv6: %w", err)
	}
	defer rows.Close()

	return ipv6.parseTestResults(rows)
}

// parseTestResults is a helper to parse test result rows
func (ipv6 *IPv6QueryOperations) parseTestResults(rows interface{ Next() bool; Scan(dest ...interface{}) error }) ([]NodeTestResult, error) {
	var results []NodeTestResult
	rowCount := 0
	for rows.Next() {
		rowCount++
		var r NodeTestResult
		err := ipv6.resultParser.ParseTestResultRow(rows, &r)
		if err != nil {
			logging.Error("parseTestResults: Failed to parse row", slog.Int("row", rowCount), slog.Any("error", err))
			return nil, fmt.Errorf("failed to parse test result row %d: %w", rowCount, err)
		}
		results = append(results, r)
	}
	return results, nil
}
