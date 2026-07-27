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
		-- Number each row's test cycle. Rows are ordered so that one cycle reads
		-- as its per-hostname rows in ascending hostname_index followed by its
		-- aggregated summary - which is also the order the daemon writes them,
		-- and pins the order when several rows share a test_time (common: a whole
		-- cycle often lands inside one second).
		--
		-- A boundary is any of:
		--   * a gap wider than the session window;
		--   * the previous row being an aggregate, since that ends its cycle -
		--     this is what catches two complete multi-hostname cycles run
		--     back-to-back, where the transition is aggregate(index 0) to the
		--     next cycle's primary(index 0) and neither the gap nor a repeated
		--     (hostname_index, is_aggregated) pair would reveal it;
		--   * either side of an AKA-derived row. Such a row is not this node's
		--     test at all: deriveAKAResults clones ANOTHER node's successful
		--     result into this partition for a shared host, so letting it share
		--     a cycle lets max(is_operational) report someone else's success and
		--     erase this node's own failure;
		--   * a per-hostname row whose index does not advance, or that carries
		--     the -1 "no index" sentinel on either side of the step, since a real
		--     cycle numbers its hostnames 0,1,2. This covers a re-test of the
		--     same host, a cycle whose aggregate was never written (index 1
		--     followed by index 0), and a legacy/CLI row abutting a modern one.
		--     It is skipped when the timestamp has not advanced: test_time has
		--     one-second resolution, so rows inside the same second cannot be
		--     attributed to one cycle or the next, and splitting on index there
		--     shreds a single cycle into one fragment per hostname.
		-- 2:467/70 was probed twice 73s apart (once against a nodelist flag
		-- misparsed as a hostname, which fails DNS, then against its real host),
		-- and 1:342/806 was re-tested by hand 60s after an automatic run.
		cycle_marked AS (
			SELECT *,
				if(toInt64(toUnixTimestamp(test_time)) - toInt64(toUnixTimestamp(
							lagInFrame(test_time, 1, toDateTime(0)) OVER seq
						)) > %d
					OR lagInFrame(is_aggregated, 1, true) OVER seq
					OR derived_from_address != ''
					OR lagInFrame(derived_from_address, 1, '') OVER seq != ''
					OR (
						NOT is_aggregated
						AND test_time != lagInFrame(test_time, 1, toDateTime(0)) OVER seq
						AND (
							hostname_index <= lagInFrame(hostname_index, 1, CAST(2147483647 AS Int32)) OVER seq
							OR hostname_index < 0
							OR lagInFrame(hostname_index, 1, CAST(0 AS Int32)) OVER seq < 0
						)
					), 1, 0) as new_cycle
			FROM node_test_results
			WHERE zone = ? AND net = ? AND node = ?
			AND test_time >= now() - INTERVAL ? DAY
			AND (? = '' OR domain = ?)
			WINDOW seq AS (PARTITION BY domain, zone, net, node ORDER BY test_time, is_aggregated, hostname_index)
		),
		-- Fold each cycle the way the daemon's own aggregation does - a protocol
		-- counts as reached if any hostname reached it - so the tiers below do
		-- not depend on which single row represents the cycle. That matters for
		-- a cycle whose aggregate was never written: reading IPv6 flags off one
		-- hostname's row would report a node as fully successful when a
		-- different hostname is the one with broken IPv6. Where the aggregate
		-- does exist its values are already these ORs, so folding is idempotent.
		--
		-- rn = 1 still marks a representative row, now only for the response
		-- time and the timestamp; is_operational DESC keeps an interrupted
		-- cycle's working hostname in front of a broken one, and test_time last
		-- makes the pick deterministic.
		ranked AS (
			SELECT *,
				max(is_operational) OVER cyc as cyc_operational,
				max(length(resolved_ipv6) > 0) OVER cyc as cyc_has_ipv6,
				max(binkp_tested) OVER cyc as cyc_binkp_tested,
				max(binkp_ipv6_success) OVER cyc as cyc_binkp_ipv6_success,
				max(ifcico_tested) OVER cyc as cyc_ifcico_tested,
				max(ifcico_ipv6_success) OVER cyc as cyc_ifcico_ipv6_success,
				max(telnet_tested) OVER cyc as cyc_telnet_tested,
				max(telnet_ipv6_success) OVER cyc as cyc_telnet_ipv6_success,
				row_number() OVER (
					PARTITION BY domain, zone, net, node, cycle_id
					ORDER BY is_aggregated DESC, is_operational DESC, hostname_index ASC, test_time ASC
				) as rn
			FROM (
				SELECT *,
					sum(new_cycle) OVER (
						PARTITION BY domain, zone, net, node
						ORDER BY test_time, is_aggregated, hostname_index
						ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
					) as cycle_id
				FROM cycle_marked
			)
			WINDOW cyc AS (PARTITION BY domain, zone, net, node, cycle_id)
		)
		SELECT
			zone, net, node,
			countIf(rn = 1) as total_tests,

			-- Fully successful tests: all tested protocols succeeded (IPv4 and IPv6 if available)
			countIf(
				rn = 1 AND
				cyc_operational AND
				(NOT cyc_has_ipv6 OR (
					(NOT cyc_binkp_tested OR cyc_binkp_ipv6_success) AND
					(NOT cyc_ifcico_tested OR cyc_ifcico_ipv6_success) AND
					(NOT cyc_telnet_tested OR cyc_telnet_ipv6_success)
				))
			) as fully_successful_tests,

			-- Partially failed tests: operational but some IPv6 tests failed
			countIf(
				rn = 1 AND
				cyc_operational AND
				cyc_has_ipv6 AND (
					(cyc_binkp_tested AND NOT cyc_binkp_ipv6_success) OR
					(cyc_ifcico_tested AND NOT cyc_ifcico_ipv6_success) OR
					(cyc_telnet_tested AND NOT cyc_telnet_ipv6_success)
				)
			) as partially_failed_tests,

			-- Fully failed tests: not operational at all
			countIf(rn = 1 AND NOT cyc_operational) as failed_tests,

			-- For backward compatibility
			countIf(rn = 1 AND cyc_operational) as successful_tests,
			avgIf(cyc_operational, rn = 1) * 100 as success_rate,

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
			argMaxIf(cyc_operational, test_time, rn = 1) as last_status,

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
func (tqb *TestQueryBuilder) BuildProtocolEnabledQuery(protocol, nodeFilter, domainFilter string, days int) string {
	predicate, ok := protocolSuccessPredicates[protocol]
	if !ok {
		predicate = protocolSuccessPredicates["binkp"] // fallback
	}
	rowPredicate := fmt.Sprintf(predicate, "")    // FROM node_test_results, unaliased
	joinPredicate := fmt.Sprintf(predicate, "r.") // FROM node_test_results r JOIN ...

	return applyCycleWindows(fmt.Sprintf(`
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
			JOIN latest_tests lt ON r.domain = lt.domain AND r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node AND {{CYCLE_LT}}
			WHERE %s
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
		ORDER BY r.test_time DESC
		LIMIT ?`, rowPredicate, nodeFilter, domainFilter, joinPredicate), days)
}

// BuildVModemUnconfirmedQuery builds a query for nodes whose VModem probe in
// the report window did not confirm a genuine VMP responder — either the IVM
// port was down/unreachable, or it answered as something else entirely (an
// EMSI mailer, binkd, ssh, a telnet login prompt, ...).
//
// Structurally identical to BuildProtocolEnabledQuery's three-CTE shape, but
// anchored on "was probed" (vmodem_tested) rather than "is_operational AND
// succeeded": a down/unreachable result is exactly the case this report
// exists to surface, and such a row has is_operational = false (see
// determineOperationalStatus in internal/testing/daemon/test_executor.go —
// is_operational is true if ANY protocol succeeded on that row, not
// specifically VModem, so filtering on it here would drop the "down" case).
//
// The final WHERE is the SQL form of NodeTestResult.IsConfirmedVMODEM(),
// negated: a row is included unless it is a genuine, confirmed VMP responder.
// domainFilter is a ready-made SQL clause (see domainFilterSQL); "" means all FTN networks.
func (tqb *TestQueryBuilder) BuildVModemUnconfirmedQuery(nodeFilter, domainFilter string, days int) string {
	return applyCycleWindows(fmt.Sprintf(`
		WITH latest_tests AS (
			SELECT
				domain, zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				AND vmodem_tested = true
				%s
				%s
			GROUP BY domain, zone, net, node
		),
		-- Prefer the aggregated row, same ordering BuildProtocolEnabledQuery uses:
		-- for a multi-hostname node, test_aggregator.go's preferConfirmedVMODEM
		-- makes the aggregated row absorb confirmation from ANY hostname, so this
		-- ordering alone gets a multi-hostname node right without extra logic.
		-- The candidate predicate is reapplied here per session_window.go's rule:
		-- widening the anchor to a session window admits rows the anchor CTE's own
		-- predicate rejected.
		best_results AS (
			SELECT
				r.domain, r.zone, r.net, r.node, r.test_time, r.hostname_index, r.is_aggregated,
				row_number() OVER (PARTITION BY r.domain, r.zone, r.net, r.node ORDER BY r.is_aggregated DESC, r.hostname_index ASC) as rn
			FROM node_test_results r
			JOIN latest_tests lt ON r.domain = lt.domain AND r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node AND {{CYCLE_LT}}
			WHERE r.vmodem_tested = true
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
		WHERE NOT (r.vmodem_variant = 'vmp' AND r.vmodem_conformant = true)
		ORDER BY r.test_time DESC
		LIMIT ?`, nodeFilter, domainFilter), days)
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
