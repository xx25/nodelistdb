package storage

import "fmt"

// TestQueryBuilder centralizes query generation for test operations
// ClickHouse-only implementation
type TestQueryBuilder struct{}

// NewTestQueryBuilder creates a new test query builder
func NewTestQueryBuilder() *TestQueryBuilder {
	return &TestQueryBuilder{}
}

// BuildTestHistoryQuery builds a query to retrieve test history for a specific node (ClickHouse)
func (tqb *TestQueryBuilder) BuildTestHistoryQuery() string {
	return applyTestResultColumns(`
		SELECT
			{{TEST_RESULT_COLUMNS}}
		FROM node_test_results
		WHERE zone = ? AND net = ? AND node = ?
		AND test_time >= now() - INTERVAL ? DAY
		AND (? = '' OR domain = ?)
		ORDER BY test_time ASC, hostname_index`)
}

// BuildDetailedTestResultQuery builds a query for a specific test result (ClickHouse)
func (tqb *TestQueryBuilder) BuildDetailedTestResultQuery() string {
	return applyTestResultColumns(`
		SELECT
			{{TEST_RESULT_COLUMNS}}
		FROM node_test_results
		WHERE zone = ? AND net = ? AND node = ? AND test_time = parseDateTimeBestEffort(?)
		AND (? = '' OR domain = ?)
		ORDER BY is_aggregated DESC, hostname_index ASC
		LIMIT 1`)
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

// BuildReachabilityTrendsQuery builds a query for reachability trends over time (ClickHouse).
//
// Semantics: for every report date (yesterday back through ? days ago — today is
// excluded to avoid incomplete data at day boundaries), each node carries the
// status and response time of its last test up to the end of that date, until
// explicitly retested. Bind order: domain, domain, days.
//
// The carry-forward is computed by collapsing each node to one row per tested
// day, then arrayJoin-expanding that day's verdict over the report dates it
// covers (its own day up to the eve of the node's next tested day). An earlier
// shape CROSS JOINed the date series against node_test_results and re-aggregated
// the full history once per report date — O(days x rows), 2.7s and growing with
// both factors on the /reachability landing page; this one scans the table once
// and emits at most nodes x days rows (0.11s on the same data, byte-identical
// output).
//
// The argMax comparands carry the usual same-second tie-break (aggregated row
// first, then lowest hostname_index): a whole test cycle often lands inside one
// second, and bare argMax by test_time could hand a day's verdict to one
// per-hostname row instead of the node's aggregate. Day attribution uses
// toDate(test_time), not the materialized test_date column — the two are
// stamped at different moments and disagree on a small number of rows, and the
// carry-forward must stay consistent with the test_time ordering inside a day.
func (tqb *TestQueryBuilder) BuildReachabilityTrendsQuery() string {
	return `
		WITH
		per_day AS (
			SELECT
				domain, zone, net, node,
				toDate(test_time) AS day,
				argMax(is_operational, (test_time, is_aggregated, -hostname_index)) AS day_status,
				argMax(least(
					if(binkp_response_ms > 0, binkp_response_ms, 999999),
					if(ifcico_response_ms > 0, ifcico_response_ms, 999999),
					if(telnet_response_ms > 0, telnet_response_ms, 999999)
				), (test_time, is_aggregated, -hostname_index)) AS day_response
			FROM node_test_results
			WHERE (? = '' OR domain = ?)
			GROUP BY domain, zone, net, node, day
		),
		carried AS (
			SELECT
				domain, zone, net, node, day, day_status, day_response,
				leadInFrame(day, 1, toDate(now())) OVER (
					PARTITION BY domain, zone, net, node ORDER BY day ASC
					ROWS BETWEEN CURRENT ROW AND UNBOUNDED FOLLOWING
				) AS next_day
			FROM per_day
		)
		SELECT
			report_date AS test_date,
			count() AS total_nodes,
			countIf(day_status = 1) AS operational_nodes,
			countIf(day_status = 0) AS failed_nodes,
			avg(toUInt8(day_status)) * 100 AS success_rate,
			avgIf(day_response, day_status = 1 AND day_response < 999999) AS avg_response_ms
		FROM (
			SELECT
				domain, zone, net, node, day_status, day_response,
				greatest(day, toDate(now()) - ?) AS span_start,
				least(next_day - 1, toDate(now()) - 1) AS span_end,
				arrayJoin(arrayMap(x -> span_start + x, range(toUInt64(greatest(toInt64(span_end - span_start) + 1, 0))))) AS report_date
			FROM carried
		)
		GROUP BY report_date
		ORDER BY report_date ASC`
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
//
// Candidates are gated on announced_nodes, whose strength depends on what the
// page claims: the VModem page says "...on their announced IVM port", so it
// gets the flag test; the others say "successfully tested", so they get
// nodelist membership only. See announcementGatedProtocols for why the
// difference matters. Either way the gate drops nodes that left the nodelist or
// went Down/Hold since their last successful test.
func (tqb *TestQueryBuilder) BuildProtocolEnabledQuery(protocol, nodeFilter, domainFilter string, days int) string {
	predicate, ok := protocolSuccessPredicates[protocol]
	if !ok {
		predicate = protocolSuccessPredicates["binkp"] // fallback
	}
	rowPredicate := fmt.Sprintf(predicate, "")    // FROM node_test_results, unaliased
	joinPredicate := fmt.Sprintf(predicate, "r.") // FROM node_test_results r JOIN ...

	// An unknown protocol fell back to binkp's predicate above, and binkp is not
	// announcement-gated, so it lands on membership only — the weaker gate, which
	// is the safe direction for a protocol nobody has classified yet.
	gate := currentNodesSQL(domainFilter, nodeFilter)
	if announcementGatedProtocols[protocol] {
		gate = announcedProtocolNodesSQL(protocolAnnouncementFlags[protocol], domainFilter, nodeFilter)
	}

	return applyTestResultColumns(applyCycleWindows(fmt.Sprintf(`
		WITH announced_nodes AS (%s
		),
		latest_tests AS (
			SELECT
				domain, zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				AND %s
				AND is_operational = true
				AND (domain, zone, net, node) IN (SELECT domain, zone, net, node FROM announced_nodes)
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
			{{TEST_RESULT_COLUMNS_R}}
		FROM node_test_results r
		JOIN best_results br ON r.domain = br.domain AND r.zone = br.zone AND r.net = br.net AND r.node = br.node AND r.test_time = br.test_time
			AND r.hostname_index = br.hostname_index AND r.is_aggregated = br.is_aggregated AND br.rn = 1
		ORDER BY r.test_time DESC
		LIMIT ?`, gate, rowPredicate, nodeFilter, domainFilter, joinPredicate), days))
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
//
// Candidates are gated on announced_nodes because "was probed" is not the same
// population as "announces IVM" — see nodelist_gate.go for the ways a node with
// no IVM flag acquires a vmodem_tested row. Unlike BuildProtocolEnabledQuery
// this query has no success predicate to fall back on: its row predicate is
// merely vmodem_tested, which a stray probe satisfies by definition, so the
// gate is the only thing standing between the report and its claim. The gate
// is a node-level predicate, so unlike the row-level ones it needs no
// reapplication against the widened session window below: best_results reaches
// rows only through latest_tests, which has already dropped the whole node.
// domainFilter is a ready-made SQL clause (see domainFilterSQL); "" means all FTN networks.
func (tqb *TestQueryBuilder) BuildVModemUnconfirmedQuery(nodeFilter, domainFilter string, days int) string {
	return applyTestResultColumns(applyCycleWindows(fmt.Sprintf(`
		WITH announced_nodes AS (%s
		),
		latest_tests AS (
			SELECT
				domain, zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				AND vmodem_tested = true
				AND (domain, zone, net, node) IN (SELECT domain, zone, net, node FROM announced_nodes)
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
			{{TEST_RESULT_COLUMNS_R}}
		FROM node_test_results r
		JOIN best_results br ON r.domain = br.domain AND r.zone = br.zone AND r.net = br.net AND r.node = br.node AND r.test_time = br.test_time
			AND r.hostname_index = br.hostname_index AND r.is_aggregated = br.is_aggregated AND br.rn = 1
		WHERE NOT (r.vmodem_variant = 'vmp' AND r.vmodem_conformant = true)
		ORDER BY r.test_time DESC
		LIMIT ?`, announcedProtocolNodesSQL("IVM", domainFilter, nodeFilter), nodeFilter, domainFilter), days))
}

// BuildSearchByReachabilityQuery builds a query to search nodes by reachability status (ClickHouse)
//
// test_time is second-resolution and a multi-hostname node's per-hostname rows
// routinely share one second, so ORDER BY test_time alone leaves rn = 1
// undefined among them - and the callers run this query twice, once per
// is_operational value. Two executions free to break the tie differently put the
// same node in both result sets whenever one of its hostnames failed and another
// succeeded in the same second (real: 2:263/0, 2:240/5833), which is how one
// node ends up listed twice with contradictory badges. The tie-break is the same
// one BuildVMODEMNodesQuery uses: the aggregated row is the node's own verdict
// over all its hostnames, so it wins when present, and hostname order decides
// otherwise.
func (tqb *TestQueryBuilder) BuildSearchByReachabilityQuery() string {
	return applyTestResultColumns(`
		SELECT
			{{TEST_RESULT_COLUMNS}}
		FROM (
			SELECT *, row_number() OVER (PARTITION BY domain, zone, net, node ORDER BY test_time DESC, is_aggregated DESC, hostname_index ASC) as rn
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
			AND (? = '' OR domain = ?)
		)
		WHERE rn = 1 AND is_operational = ?
		ORDER BY test_time DESC
		LIMIT ?`)
}
