package storage

import (
	"fmt"

	"github.com/nodelistdb/internal/database"
)

// TestQueryBuilder centralizes query generation for test operations
// Eliminates duplication between ClickHouse and DuckDB queries
type TestQueryBuilder struct {
	isClickHouse bool
}

// NewTestQueryBuilder creates a new test query builder
func NewTestQueryBuilder(db database.DatabaseInterface) *TestQueryBuilder {
	_, isClickHouse := db.(*database.ClickHouseDB)
	return &TestQueryBuilder{
		isClickHouse: isClickHouse,
	}
}

// BuildTestHistoryQuery builds a query to retrieve test history for a specific node
func (tqb *TestQueryBuilder) BuildTestHistoryQuery() string {
	if tqb.isClickHouse {
		return `
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
				binkp_ipv6_tested, binkp_ipv6_success, binkp_ipv6_error,
				ifcico_ipv6_tested, ifcico_ipv6_success, ifcico_ipv6_error,
				telnet_ipv6_tested, telnet_ipv6_success, telnet_ipv6_error,
				is_operational, has_connectivity_issues, address_validated,
				tested_hostname, hostname_index, is_aggregated,
				total_hostnames, hostnames_tested, hostnames_operational
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ?
			AND test_time >= now() - INTERVAL ? DAY
			ORDER BY test_time DESC, hostname_index`
	}

	return `
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
			binkp_ipv6_tested, binkp_ipv6_success, binkp_ipv6_error,
			ifcico_ipv6_tested, ifcico_ipv6_success, ifcico_ipv6_error,
			telnet_ipv6_tested, telnet_ipv6_success, telnet_ipv6_error,
			is_operational, has_connectivity_issues, address_validated,
			tested_hostname, hostname_index, is_aggregated,
			total_hostnames, hostnames_tested, hostnames_operational
		FROM node_test_results
		WHERE zone = ? AND net = ? AND node = ?
		AND test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
		ORDER BY test_time ASC`
}

// BuildDetailedTestResultQuery builds a query for a specific test result
func (tqb *TestQueryBuilder) BuildDetailedTestResultQuery() string {
	selectClause := `
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
			binkp_ipv6_tested, binkp_ipv6_success, binkp_ipv6_error,
			ifcico_ipv6_tested, ifcico_ipv6_success, ifcico_ipv6_error,
			telnet_ipv6_tested, telnet_ipv6_success, telnet_ipv6_error,
			is_operational, has_connectivity_issues, address_validated,
			tested_hostname, hostname_index, is_aggregated,
			total_hostnames, hostnames_tested, hostnames_operational
		FROM node_test_results`

	if tqb.isClickHouse {
		return selectClause + `
			WHERE zone = ? AND net = ? AND node = ? AND test_time = parseDateTimeBestEffort(?)
			ORDER BY is_aggregated DESC, hostname_index ASC
			LIMIT 1`
	}

	return selectClause + `
		WHERE zone = ? AND net = ? AND node = ? AND test_time = ?
		ORDER BY is_aggregated DESC, hostname_index ASC
		LIMIT 1`
}

// BuildReachabilityStatsQuery builds a query for node reachability statistics
func (tqb *TestQueryBuilder) BuildReachabilityStatsQuery() string {
	if tqb.isClickHouse {
		return `
			SELECT
				zone, net, node,
				count(*) as total_tests,

				-- Fully successful tests: all tested protocols succeeded (IPv4 and IPv6 if available)
				countIf(
					is_operational AND
					(length(resolved_ipv6) = 0 OR (
						(NOT binkp_tested OR binkp_ipv6_success OR length(resolved_ipv6) = 0) AND
						(NOT ifcico_tested OR ifcico_ipv6_success OR length(resolved_ipv6) = 0) AND
						(NOT telnet_tested OR telnet_ipv6_success OR length(resolved_ipv6) = 0)
					))
				) as fully_successful_tests,

				-- Partially failed tests: operational but some IPv6 tests failed
				countIf(
					is_operational AND
					length(resolved_ipv6) > 0 AND (
						(binkp_tested AND NOT binkp_ipv6_success) OR
						(ifcico_tested AND NOT ifcico_ipv6_success) OR
						(telnet_tested AND NOT telnet_ipv6_success)
					)
				) as partially_failed_tests,

				-- Fully failed tests: not operational at all
				countIf(NOT is_operational) as failed_tests,

				-- For backward compatibility
				countIf(is_operational) as successful_tests,
				avg(is_operational) * 100 as success_rate,

				avgIf(least(
					if(binkp_response_ms > 0, binkp_response_ms, 999999),
					if(ifcico_response_ms > 0, ifcico_response_ms, 999999),
					if(telnet_response_ms > 0, telnet_response_ms, 999999)
				), is_operational AND least(
					if(binkp_response_ms > 0, binkp_response_ms, 999999),
					if(ifcico_response_ms > 0, ifcico_response_ms, 999999),
					if(telnet_response_ms > 0, telnet_response_ms, 999999)
				) < 999999) as avg_response_ms,
				max(test_time) as last_test_time,
				argMax(is_operational, test_time) as last_status,

				-- IPv4 success rates
				avgIf(binkp_success, binkp_tested) * 100 as binkp_success_rate,
				avgIf(ifcico_success, ifcico_tested) * 100 as ifcico_success_rate,
				avgIf(telnet_success, telnet_tested) * 100 as telnet_success_rate,

				-- IPv6 success rates
				avgIf(binkp_ipv6_success, binkp_ipv6_tested AND length(resolved_ipv6) > 0) * 100 as binkp_ipv6_success_rate,
				avgIf(ifcico_ipv6_success, ifcico_ipv6_tested AND length(resolved_ipv6) > 0) * 100 as ifcico_ipv6_success_rate,
				avgIf(telnet_ipv6_success, telnet_ipv6_tested AND length(resolved_ipv6) > 0) * 100 as telnet_ipv6_success_rate
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ?
			AND test_time >= now() - INTERVAL ? DAY
			GROUP BY zone, net, node`
	}

	return `
		SELECT
			zone, net, node,
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
		GROUP BY zone, net, node`
}

// BuildReachabilityTrendsQuery builds a query for reachability trends over time
func (tqb *TestQueryBuilder) BuildReachabilityTrendsQuery() string {
	if tqb.isClickHouse {
		return `
			WITH
			-- Generate date series for the report period
			date_series AS (
				SELECT toDate(now() - INTERVAL number DAY) as report_date
				FROM numbers(?)
			),
			-- For each date, find the last known status of each node up to that date
			-- Look back up to 3 days since operational nodes are tested every 72 hours
			daily_status AS (
				SELECT
					d.report_date,
					r.zone, r.net, r.node,
					argMax(r.is_operational, r.test_time) as last_status,
					max(r.test_time) as last_test_time,
					argMax(least(
						if(r.binkp_response_ms > 0, r.binkp_response_ms, 999999),
						if(r.ifcico_response_ms > 0, r.ifcico_response_ms, 999999),
						if(r.telnet_response_ms > 0, r.telnet_response_ms, 999999)
					), r.test_time) as last_response_ms
				FROM date_series d
				CROSS JOIN node_test_results r
				WHERE r.test_time <= d.report_date + INTERVAL 1 DAY
					AND r.test_time >= d.report_date - INTERVAL 3 DAY
				GROUP BY d.report_date, r.zone, r.net, r.node
			)
			SELECT
				report_date as test_date,
				count(DISTINCT (zone, net, node)) as total_nodes,
				countDistinctIf((zone, net, node), last_status = 1) as operational_nodes,
				countDistinctIf((zone, net, node), last_status = 0) as failed_nodes,
				avg(toUInt8(last_status)) * 100 as success_rate,
				avgIf(last_response_ms, last_status = 1 AND last_response_ms < 999999) as avg_response_ms
			FROM daily_status
			GROUP BY report_date
			ORDER BY report_date ASC`
	}

	return `
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
		ORDER BY test_date ASC`
}

// BuildProtocolEnabledQuery builds a query for nodes with a specific protocol enabled
// protocol should be one of: "binkp", "ifcico", "telnet", "ftp", "vmodem"
func (tqb *TestQueryBuilder) BuildProtocolEnabledQuery(protocol, nodeFilter string) string {
	protocolMap := map[string]string{
		"binkp":  "binkp_success",
		"ifcico": "ifcico_success",
		"telnet": "telnet_success",
		"ftp":    "ftp_success",
		"vmodem": "vmodem_success",
	}

	protocolColumn, ok := protocolMap[protocol]
	if !ok {
		protocolColumn = "binkp_success" // fallback
	}

	if tqb.isClickHouse {
		return fmt.Sprintf(`
			WITH latest_tests AS (
				SELECT
					zone, net, node,
					max(test_time) as latest_test_time
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL ? DAY
					AND %s = true
					AND is_operational = true
					%s
				GROUP BY zone, net, node
			),
			-- Prefer aggregated results for multi-hostname nodes, otherwise take single result
			best_results AS (
				SELECT
					r.zone, r.net, r.node, r.test_time,
					row_number() OVER (PARTITION BY r.zone, r.net, r.node ORDER BY r.is_aggregated DESC, r.hostname_index ASC) as rn
				FROM node_test_results r
				JOIN latest_tests lt ON r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node AND r.test_time = lt.latest_test_time
			),
			latest_nodes AS (
				SELECT
					zone, net, node,
					argMax(system_name, nodelist_date) as system_name
				FROM nodes
				GROUP BY zone, net, node
			)
			SELECT
				r.test_time, r.zone, r.net, r.node, r.address, r.hostname,
				r.resolved_ipv4, r.resolved_ipv6, r.dns_error,
				r.country, r.country_code, r.city, r.region, r.latitude, r.longitude, r.isp, r.org, r.asn,
				r.binkp_tested, r.binkp_success, r.binkp_response_ms, r.binkp_system_name,
				r.binkp_sysop, r.binkp_location, r.binkp_version, r.binkp_addresses, r.binkp_capabilities, r.binkp_error,
				r.ifcico_tested, r.ifcico_success, r.ifcico_response_ms, r.ifcico_mailer_info,
				r.ifcico_system_name, r.ifcico_addresses, r.ifcico_response_type, r.ifcico_error,
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
			JOIN best_results br ON r.zone = br.zone AND r.net = br.net AND r.node = br.node AND r.test_time = br.test_time AND br.rn = 1
			LEFT JOIN latest_nodes ln ON r.zone = ln.zone AND r.net = ln.net AND r.node = ln.node
			ORDER BY r.test_time DESC
			LIMIT ?`, protocolColumn, nodeFilter)
	}

	return fmt.Sprintf(`
		WITH latest_tests AS (
			SELECT
				zone, net, node,
				MAX(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
				AND %s = true
				AND is_operational = true
				%s
			GROUP BY zone, net, node
		),
		-- Prefer aggregated results for multi-hostname nodes, otherwise take single result
		best_results AS (
			SELECT DISTINCT ON (zone, net, node)
				r.zone, r.net, r.node, r.test_time
			FROM node_test_results r
			JOIN latest_tests lt ON r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node AND r.test_time = lt.latest_test_time
			ORDER BY r.zone, r.net, r.node, r.is_aggregated DESC, r.hostname_index ASC
		)
		SELECT
			r.test_time, r.zone, r.net, r.node, r.address, r.hostname,
			r.resolved_ipv4, r.resolved_ipv6, r.dns_error,
			r.country, r.country_code, r.city, r.region, r.latitude, r.longitude, r.isp, r.org, r.asn,
			r.binkp_tested, r.binkp_success, r.binkp_response_ms, r.binkp_system_name,
			r.binkp_sysop, r.binkp_location, r.binkp_version, r.binkp_addresses, r.binkp_capabilities, r.binkp_error,
			r.ifcico_tested, r.ifcico_success, r.ifcico_response_ms, r.ifcico_mailer_info,
			r.ifcico_system_name, r.ifcico_addresses, r.ifcico_response_type, r.ifcico_error,
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
		JOIN best_results br ON r.zone = br.zone AND r.net = br.net AND r.node = br.node AND r.test_time = br.test_time
		ORDER BY r.test_time DESC
		LIMIT ?`, protocolColumn, nodeFilter)
}

// BuildSearchByReachabilityQuery builds a query to search nodes by reachability status
func (tqb *TestQueryBuilder) BuildSearchByReachabilityQuery() string {
	if tqb.isClickHouse {
		return `
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
				binkp_ipv6_tested, binkp_ipv6_success, binkp_ipv6_error,
				ifcico_ipv6_tested, ifcico_ipv6_success, ifcico_ipv6_error,
				telnet_ipv6_tested, telnet_ipv6_success, telnet_ipv6_error,
				is_operational, has_connectivity_issues, address_validated,
				tested_hostname, hostname_index, is_aggregated,
				total_hostnames, hostnames_tested, hostnames_operational
			FROM (
				SELECT *, row_number() OVER (PARTITION BY zone, net, node ORDER BY test_time DESC) as rn
				FROM node_test_results
				WHERE test_time >= now() - INTERVAL ? DAY
			)
			WHERE rn = 1 AND is_operational = ?
			ORDER BY test_time DESC
			LIMIT ?`
	}

	return `
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
			binkp_ipv6_tested, binkp_ipv6_success, binkp_ipv6_error,
			ifcico_ipv6_tested, ifcico_ipv6_success, ifcico_ipv6_error,
			telnet_ipv6_tested, telnet_ipv6_success, telnet_ipv6_error,
			is_operational, has_connectivity_issues, address_validated,
			tested_hostname, hostname_index, is_aggregated,
			total_hostnames, hostnames_tested, hostnames_operational
		FROM node_test_results
		WHERE test_time >= CURRENT_TIMESTAMP - INTERVAL ? DAY
		AND is_operational = ?
		ORDER BY zone, net, node, test_time DESC
		LIMIT ?`
}

// IntervalFunc returns the appropriate time interval function for the database
func (tqb *TestQueryBuilder) IntervalFunc() string {
	if tqb.isClickHouse {
		return "now() - INTERVAL ? DAY"
	}
	return "CURRENT_TIMESTAMP - INTERVAL ? DAY"
}

// ArrayLengthFunc returns the appropriate array length function for the database
func (tqb *TestQueryBuilder) ArrayLengthFunc(column string) string {
	if tqb.isClickHouse {
		return fmt.Sprintf("length(%s)", column)
	}
	return fmt.Sprintf("array_length(%s)", column)
}
