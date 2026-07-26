package storage

import (
	"fmt"

	"github.com/nodelistdb/internal/database"
)

// TestQueryBuilder centralizes query generation for test operations
// ClickHouse-only implementation
type TestQueryBuilder struct{}

// NewTestQueryBuilder creates a new test query builder
func NewTestQueryBuilder(db database.DatabaseInterface) *TestQueryBuilder {
	return &TestQueryBuilder{}
}

// BuildTestHistoryQuery builds a query to retrieve test history for a specific node (ClickHouse)
func (tqb *TestQueryBuilder) BuildTestHistoryQuery() string {
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
			vmodem_variant, vmodem_conformant, vmodem_software, vmodem_system_name,
			vmodem_sysop, vmodem_location, vmodem_addresses,
			vmodem_detail, vmodem_call_outcome, vmodem_banner,
			binkp_ipv4_tested, binkp_ipv4_success, binkp_ipv4_response_ms, binkp_ipv4_address, binkp_ipv4_error,
			binkp_ipv6_tested, binkp_ipv6_success, binkp_ipv6_response_ms, binkp_ipv6_address, binkp_ipv6_error,
			ifcico_ipv4_tested, ifcico_ipv4_success, ifcico_ipv4_response_ms, ifcico_ipv4_address, ifcico_ipv4_error,
			ifcico_ipv6_tested, ifcico_ipv6_success, ifcico_ipv6_response_ms, ifcico_ipv6_address, ifcico_ipv6_error,
			telnet_ipv4_tested, telnet_ipv4_success, telnet_ipv4_response_ms, telnet_ipv4_address, telnet_ipv4_error,
			telnet_ipv6_tested, telnet_ipv6_success, telnet_ipv6_response_ms, telnet_ipv6_address, telnet_ipv6_error,
			ftp_ipv4_tested, ftp_ipv4_success, ftp_ipv4_response_ms, ftp_ipv4_address, ftp_ipv4_error,
			ftp_ipv6_tested, ftp_ipv6_success, ftp_ipv6_response_ms, ftp_ipv6_address, ftp_ipv6_error,
			vmodem_ipv4_tested, vmodem_ipv4_success, vmodem_ipv4_response_ms, vmodem_ipv4_address, vmodem_ipv4_error,
			vmodem_ipv6_tested, vmodem_ipv6_success, vmodem_ipv6_response_ms, vmodem_ipv6_address, vmodem_ipv6_error,
			is_operational, has_connectivity_issues, address_validated,
			tested_hostname, hostname_index, is_aggregated,
			total_hostnames, hostnames_tested, hostnames_operational,
			ftp_anon_success, domain, derived_from_address
		FROM node_test_results
		WHERE zone = ? AND net = ? AND node = ?
		AND test_time >= now() - INTERVAL ? DAY
		AND (? = '' OR domain = ?)
		ORDER BY test_time ASC, hostname_index`
}

// BuildDetailedTestResultQuery builds a query for a specific test result (ClickHouse)
func (tqb *TestQueryBuilder) BuildDetailedTestResultQuery() string {
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
			vmodem_variant, vmodem_conformant, vmodem_software, vmodem_system_name,
			vmodem_sysop, vmodem_location, vmodem_addresses,
			vmodem_detail, vmodem_call_outcome, vmodem_banner,
			binkp_ipv4_tested, binkp_ipv4_success, binkp_ipv4_response_ms, binkp_ipv4_address, binkp_ipv4_error,
			binkp_ipv6_tested, binkp_ipv6_success, binkp_ipv6_response_ms, binkp_ipv6_address, binkp_ipv6_error,
			ifcico_ipv4_tested, ifcico_ipv4_success, ifcico_ipv4_response_ms, ifcico_ipv4_address, ifcico_ipv4_error,
			ifcico_ipv6_tested, ifcico_ipv6_success, ifcico_ipv6_response_ms, ifcico_ipv6_address, ifcico_ipv6_error,
			telnet_ipv4_tested, telnet_ipv4_success, telnet_ipv4_response_ms, telnet_ipv4_address, telnet_ipv4_error,
			telnet_ipv6_tested, telnet_ipv6_success, telnet_ipv6_response_ms, telnet_ipv6_address, telnet_ipv6_error,
			ftp_ipv4_tested, ftp_ipv4_success, ftp_ipv4_response_ms, ftp_ipv4_address, ftp_ipv4_error,
			ftp_ipv6_tested, ftp_ipv6_success, ftp_ipv6_response_ms, ftp_ipv6_address, ftp_ipv6_error,
			vmodem_ipv4_tested, vmodem_ipv4_success, vmodem_ipv4_response_ms, vmodem_ipv4_address, vmodem_ipv4_error,
			vmodem_ipv6_tested, vmodem_ipv6_success, vmodem_ipv6_response_ms, vmodem_ipv6_address, vmodem_ipv6_error,
			is_operational, has_connectivity_issues, address_validated,
			tested_hostname, hostname_index, is_aggregated,
			total_hostnames, hostnames_tested, hostnames_operational,
			ftp_anon_success, domain, derived_from_address
		FROM node_test_results
		WHERE zone = ? AND net = ? AND node = ? AND test_time = parseDateTimeBestEffort(?)
		AND (? = '' OR domain = ?)
		ORDER BY is_aggregated DESC, hostname_index ASC
		LIMIT 1`
}

// BuildReachabilityStatsQuery builds a query for node reachability statistics (ClickHouse)
//
// The whole-node counters and success rate are computed per TEST CYCLE, not per
// stored row. A cycle of a multi-hostname node writes one row per hostname plus
// an aggregated summary, so counting rows made a node with one permanently
// broken backup hostname look partly unreachable: 2:240/5833 reported 24 tests
// at a 66.7% success rate when it had 8 cycles and answered in every one. Rows
// are grouped into cycles with the same measured bound the AKA queries use (see
// testSessionWindowSeconds) and each cycle contributes its aggregated row when
// it has one, otherwise its single row - the daemon writes no aggregate for a
// single-hostname node, and total_hostnames is 0 on every per-hostname row, so
// the cycle is the only thing that can tell the two apart.
//
// The per-protocol rates below are deliberately left per HOSTNAME instance:
// they answer "how often does a BinkP attempt against this node succeed",
// which is the more diagnostic view for a node with a flaky backup host, and
// folding them into cycles would silently change what they mean. That is why
// their denominators can exceed total_tests.
func (tqb *TestQueryBuilder) BuildReachabilityStatsQuery() string {
	return fmt.Sprintf(`
		WITH
		-- Number each row's test cycle: a gap larger than the session window
		-- starts a new one.
		cycle_marked AS (
			SELECT *,
				if(toInt64(toUnixTimestamp(test_time)) - toInt64(toUnixTimestamp(
						lagInFrame(test_time, 1, toDateTime(0)) OVER (
							PARTITION BY domain, zone, net, node ORDER BY test_time)
					)) > %d, 1, 0) as new_cycle
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ?
			AND test_time >= now() - INTERVAL ? DAY
			AND (? = '' OR domain = ?)
		),
		-- rn = 1 marks the row that represents its cycle: the aggregated
		-- summary when the cycle has one, else the node's only row.
		ranked AS (
			SELECT *,
				row_number() OVER (
					PARTITION BY domain, zone, net, node, cycle_id
					ORDER BY is_aggregated DESC, hostname_index ASC
				) as rn
			FROM (
				SELECT *,
					sum(new_cycle) OVER (
						PARTITION BY domain, zone, net, node ORDER BY test_time
						ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
					) as cycle_id
				FROM cycle_marked
			)
		)
		SELECT
			zone, net, node,
			countIf(rn = 1) as total_tests,

			-- Fully successful tests: all tested protocols succeeded (IPv4 and IPv6 if available)
			countIf(
				rn = 1 AND
				is_operational AND
				(length(resolved_ipv6) = 0 OR (
					(NOT binkp_tested OR binkp_ipv6_success OR length(resolved_ipv6) = 0) AND
					(NOT ifcico_tested OR ifcico_ipv6_success OR length(resolved_ipv6) = 0) AND
					(NOT telnet_tested OR telnet_ipv6_success OR length(resolved_ipv6) = 0)
				))
			) as fully_successful_tests,

			-- Partially failed tests: operational but some IPv6 tests failed
			countIf(
				rn = 1 AND
				is_operational AND
				length(resolved_ipv6) > 0 AND (
					(binkp_tested AND NOT binkp_ipv6_success) OR
					(ifcico_tested AND NOT ifcico_ipv6_success) OR
					(telnet_tested AND NOT telnet_ipv6_success)
				)
			) as partially_failed_tests,

			-- Fully failed tests: not operational at all
			countIf(rn = 1 AND NOT is_operational) as failed_tests,

			-- For backward compatibility
			countIf(rn = 1 AND is_operational) as successful_tests,
			avgIf(is_operational, rn = 1) * 100 as success_rate,

			avgIf(least(
				if(binkp_response_ms > 0, binkp_response_ms, 999999),
				if(ifcico_response_ms > 0, ifcico_response_ms, 999999),
				if(telnet_response_ms > 0, telnet_response_ms, 999999)
			), rn = 1 AND is_operational AND least(
				if(binkp_response_ms > 0, binkp_response_ms, 999999),
				if(ifcico_response_ms > 0, ifcico_response_ms, 999999),
				if(telnet_response_ms > 0, telnet_response_ms, 999999)
			) < 999999) as avg_response_ms,
			max(test_time) as last_test_time,
			-- Current status is the latest CYCLE's verdict, not whichever row was
			-- written last (that can be a failing backup hostname).
			argMaxIf(is_operational, test_time, rn = 1) as last_status,

			-- Combined success rates (IPv4 OR IPv6)
			avgIf(binkp_success, binkp_tested) * 100 as binkp_success_rate,
			avgIf(ifcico_success, ifcico_tested) * 100 as ifcico_success_rate,
			avgIf(telnet_success, telnet_tested) * 100 as telnet_success_rate,

			-- IPv4-only success rates
			avgIf(binkp_ipv4_success, binkp_ipv4_tested AND length(resolved_ipv4) > 0) * 100 as binkp_ipv4_success_rate,
			avgIf(ifcico_ipv4_success, ifcico_ipv4_tested AND length(resolved_ipv4) > 0) * 100 as ifcico_ipv4_success_rate,
			avgIf(telnet_ipv4_success, telnet_ipv4_tested AND length(resolved_ipv4) > 0) * 100 as telnet_ipv4_success_rate,

			-- IPv6-only success rates
			avgIf(binkp_ipv6_success, binkp_ipv6_tested AND length(resolved_ipv6) > 0) * 100 as binkp_ipv6_success_rate,
			avgIf(ifcico_ipv6_success, ifcico_ipv6_tested AND length(resolved_ipv6) > 0) * 100 as ifcico_ipv6_success_rate,
			avgIf(telnet_ipv6_success, telnet_ipv6_tested AND length(resolved_ipv6) > 0) * 100 as telnet_ipv6_success_rate
		FROM ranked
		GROUP BY domain, zone, net, node`, testSessionWindowSeconds)
}

// BuildReachabilityTrendsQuery builds a query for reachability trends over time (ClickHouse)
func (tqb *TestQueryBuilder) BuildReachabilityTrendsQuery() string {
	return `
		WITH
		-- Generate date series for the report period (starting from yesterday)
		-- Exclude today to avoid incomplete data at day boundaries
		date_series AS (
			SELECT toDate(now() - INTERVAL (number + 1) DAY) as report_date
			FROM numbers(?)
		),
		-- For each date, find the last known status of each node up to that date
		-- A node keeps its last known status until explicitly retested.
		daily_status AS (
			SELECT
				d.report_date,
				r.domain, r.zone, r.net, r.node,
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
			AND (? = '' OR r.domain = ?)
			GROUP BY d.report_date, r.domain, r.zone, r.net, r.node
		)
		SELECT
			report_date as test_date,
			count(DISTINCT (domain, zone, net, node)) as total_nodes,
			countDistinctIf((domain, zone, net, node), last_status = 1) as operational_nodes,
			countDistinctIf((domain, zone, net, node), last_status = 0) as failed_nodes,
			avg(toUInt8(last_status)) * 100 as success_rate,
			avgIf(last_response_ms, last_status = 1 AND last_response_ms < 999999) as avg_response_ms
		FROM daily_status
		GROUP BY report_date
		ORDER BY report_date ASC`
}

// BuildReachabilityTrendsFromDailyStatsQuery reads pre-aggregated data from node_test_daily_stats.
// Much faster than the CROSS JOIN approach — suitable for long date ranges (all time).
func (tqb *TestQueryBuilder) BuildReachabilityTrendsFromDailyStatsQuery() string {
	return `
		SELECT
			date AS test_date,
			max(total_nodes_tested) AS total_nodes,
			max(nodes_operational) AS operational_nodes,
			max(total_nodes_tested) - max(nodes_operational) AS failed_nodes,
			if(max(total_nodes_tested) > 0,
				max(nodes_operational) / max(total_nodes_tested) * 100, 0) AS success_rate,
			least(
				if(max(avg_binkp_response_ms) > 0, max(avg_binkp_response_ms), 999999),
				if(max(avg_ifcico_response_ms) > 0, max(avg_ifcico_response_ms), 999999)
			) AS avg_response_ms
		FROM node_test_daily_stats
		WHERE date >= '2025-09-01'
		GROUP BY date
		ORDER BY date ASC`
}

// protocolSuccessPredicates maps a protocol to the SQL condition that decides
// whether a test row counts as a working instance of it. `%[1]s` is the table
// alias prefix, so the same condition can be applied both where rows are
// selected and where one row per node is picked.
//
// Every protocol but VModem is a plain success flag. A VModem probe reports
// success for whatever answers on the announced IVM port — an EMSI mailer over
// telnet, binkd, even a bare telnet login prompt — because reaching any of
// those still proves the port is alive. Listing them as "VModem enabled" is
// wrong, so this page asks for the thing the flag actually promises: a
// confirmed genuine VMODEM (Gwinn VMP) responder. That also excludes rows
// written before the tester classified variants, whose bare vmodem_success
// carries no evidence at all (vmodem_variant defaults to the empty string).
var protocolSuccessPredicates = map[string]string{
	"binkp":  "%[1]sbinkp_success = true",
	"ifcico": "%[1]sifcico_success = true",
	"telnet": "%[1]stelnet_success = true",
	"ftp":    "%[1]sftp_success = true",
	"vmodem": "%[1]svmodem_variant = 'vmp' AND %[1]svmodem_conformant = true",
}

// BuildProtocolEnabledQuery builds a query for nodes with a specific protocol enabled (ClickHouse)
// protocol should be one of: "binkp", "ifcico", "telnet", "ftp", "vmodem"
// domainFilter is a ready-made SQL clause (see domainFilterSQL); "" means all FTN networks.
func (tqb *TestQueryBuilder) BuildProtocolEnabledQuery(protocol, nodeFilter, domainFilter string) string {
	predicate, ok := protocolSuccessPredicates[protocol]
	if !ok {
		predicate = protocolSuccessPredicates["binkp"] // fallback
	}
	rowPredicate := fmt.Sprintf(predicate, "")    // FROM node_test_results, unaliased
	joinPredicate := fmt.Sprintf(predicate, "r.") // FROM node_test_results r JOIN ...

	return fmt.Sprintf(`
		WITH latest_tests AS (
			SELECT
				domain, zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				AND %s
				AND is_operational = true
				%s
				%s
			GROUP BY domain, zone, net, node
		),
		-- Prefer aggregated results for multi-hostname nodes, otherwise take single result.
		-- The predicate is reapplied here: latest_tests only fixes the timestamp, and a
		-- node can have several rows at that timestamp (the aggregated summary plus one
		-- per hostname). Without it the preferred row can be one that doesn't qualify —
		-- e.g. a multi-hostname node whose aggregated row took its VModem diagnosis from
		-- a hostname running an EMSI mailer while another hostname is the real VMODEM.
		best_results AS (
			SELECT
				r.domain, r.zone, r.net, r.node, r.test_time, r.hostname_index, r.is_aggregated,
				row_number() OVER (PARTITION BY r.domain, r.zone, r.net, r.node ORDER BY r.is_aggregated DESC, r.hostname_index ASC) as rn
			FROM node_test_results r
			JOIN latest_tests lt ON r.domain = lt.domain AND r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node AND r.test_time = lt.latest_test_time
			WHERE %s
		),
		latest_nodes AS (
			SELECT
				domain, zone, net, node,
				argMax(system_name, nodelist_date) as system_name
			FROM nodes
			GROUP BY domain, zone, net, node
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
			r.vmodem_variant, r.vmodem_conformant, r.vmodem_software, r.vmodem_system_name,
			r.vmodem_sysop, r.vmodem_location, r.vmodem_addresses,
			r.vmodem_detail, r.vmodem_call_outcome, r.vmodem_banner,
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
			r.ftp_anon_success, r.domain, r.derived_from_address
		FROM node_test_results r
		JOIN best_results br ON r.domain = br.domain AND r.zone = br.zone AND r.net = br.net AND r.node = br.node AND r.test_time = br.test_time
			AND r.hostname_index = br.hostname_index AND r.is_aggregated = br.is_aggregated AND br.rn = 1
		LEFT JOIN latest_nodes ln ON r.domain = ln.domain AND r.zone = ln.zone AND r.net = ln.net AND r.node = ln.node
		ORDER BY r.test_time DESC
		LIMIT ?`, rowPredicate, nodeFilter, domainFilter, joinPredicate)
}

// BuildSearchByReachabilityQuery builds a query to search nodes by reachability status (ClickHouse)
func (tqb *TestQueryBuilder) BuildSearchByReachabilityQuery() string {
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
			vmodem_variant, vmodem_conformant, vmodem_software, vmodem_system_name,
			vmodem_sysop, vmodem_location, vmodem_addresses,
			vmodem_detail, vmodem_call_outcome, vmodem_banner,
			binkp_ipv4_tested, binkp_ipv4_success, binkp_ipv4_response_ms, binkp_ipv4_address, binkp_ipv4_error,
			binkp_ipv6_tested, binkp_ipv6_success, binkp_ipv6_response_ms, binkp_ipv6_address, binkp_ipv6_error,
			ifcico_ipv4_tested, ifcico_ipv4_success, ifcico_ipv4_response_ms, ifcico_ipv4_address, ifcico_ipv4_error,
			ifcico_ipv6_tested, ifcico_ipv6_success, ifcico_ipv6_response_ms, ifcico_ipv6_address, ifcico_ipv6_error,
			telnet_ipv4_tested, telnet_ipv4_success, telnet_ipv4_response_ms, telnet_ipv4_address, telnet_ipv4_error,
			telnet_ipv6_tested, telnet_ipv6_success, telnet_ipv6_response_ms, telnet_ipv6_address, telnet_ipv6_error,
			ftp_ipv4_tested, ftp_ipv4_success, ftp_ipv4_response_ms, ftp_ipv4_address, ftp_ipv4_error,
			ftp_ipv6_tested, ftp_ipv6_success, ftp_ipv6_response_ms, ftp_ipv6_address, ftp_ipv6_error,
			vmodem_ipv4_tested, vmodem_ipv4_success, vmodem_ipv4_response_ms, vmodem_ipv4_address, vmodem_ipv4_error,
			vmodem_ipv6_tested, vmodem_ipv6_success, vmodem_ipv6_response_ms, vmodem_ipv6_address, vmodem_ipv6_error,
			is_operational, has_connectivity_issues, address_validated,
			tested_hostname, hostname_index, is_aggregated,
			total_hostnames, hostnames_tested, hostnames_operational,
			ftp_anon_success, domain, derived_from_address
		FROM (
			SELECT *, row_number() OVER (PARTITION BY domain, zone, net, node ORDER BY test_time DESC) as rn
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
			AND (? = '' OR domain = ?)
		)
		WHERE rn = 1 AND is_operational = ?
		ORDER BY test_time DESC
		LIMIT ?`
}

// IntervalFunc returns the appropriate time interval function (ClickHouse)
func (tqb *TestQueryBuilder) IntervalFunc() string {
	return "now() - INTERVAL ? DAY"
}

// ArrayLengthFunc returns the appropriate array length function (ClickHouse)
func (tqb *TestQueryBuilder) ArrayLengthFunc(column string) string {
	return fmt.Sprintf("length(%s)", column)
}
