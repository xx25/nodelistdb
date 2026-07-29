package storage

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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

// getAllHostnamesForNode fetches all tested hostnames for a specific node
// that have IPv6, within one FTN network ("" = any). Callers pass the result
// row's own domain so a scoped report never mixes in another network's
// hostnames for a colliding zone:net/node.
func (ipv6 *IPv6QueryOperations) getAllHostnamesForNode(ctx context.Context, zone, net, node int, days int, domain string) ([]string, error) {
	conn := ipv6.db.Conn()

	query := `
		SELECT DISTINCT tested_hostname
		FROM node_test_results
		WHERE zone = ? AND net = ? AND node = ?
			AND test_time >= now() - INTERVAL ? DAY
			AND (? = '' OR domain = ?)
			AND length(tested_hostname) > 0
			AND hostname_index >= 0
			AND length(resolved_ipv6) > 0
		ORDER BY hostname_index`

	rows, err := conn.QueryContext(ctx, query, zone, net, node, days, domain, domain)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read node hostnames: %w", err)
	}

	return hostnames, nil
}

// GetIPv6EnabledNodes returns nodes that have been successfully tested with IPv6
func (ipv6 *IPv6QueryOperations) GetIPv6EnabledNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	ipv6.mu.RLock()
	defer ipv6.mu.RUnlock()

	conn := ipv6.db.Conn()

	// Build node filter condition
	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	query := fmt.Sprintf(`
		WITH latest_tests AS (
			SELECT
				domain, zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				{{NODELIST_GATE}}
				AND length(resolved_ipv6) > 0
				AND is_operational = true
				AND (binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true)
				%s
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		latest_nodes AS (
			SELECT
				domain, zone, net, node,
				argMax(system_name, nodelist_date) as system_name
			FROM nodes
			WHERE 1 = 1
				{{NODE_WINDOW}}
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		ranked_results AS (
			SELECT
				{{TEST_RESULT_COLUMNS_R}},
				row_number() OVER (PARTITION BY r.domain, r.zone, r.net, r.node ORDER BY r.is_aggregated DESC, r.hostname_index ASC) as rn
			FROM node_test_results r
			INNER JOIN latest_tests lt ON r.domain = lt.domain AND r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node
				AND {{CYCLE_LT}}
			-- Re-apply the report's criteria to the candidate rows. The window
			-- above admits the whole test cycle, including hostnames that carry
			-- no working IPv6, and the ordering below would happily pick one of
			-- those to represent the node.
			WHERE length(r.resolved_ipv6) > 0
				AND r.is_operational = true
				AND (r.binkp_ipv6_success = true OR r.ifcico_ipv6_success = true OR r.telnet_ipv6_success = true)
		)
		SELECT
			{{TEST_RESULT_COLUMNS_RR_NODENAME}}
		FROM ranked_results rr
		LEFT JOIN latest_nodes n ON rr.domain = n.domain AND rr.zone = n.zone AND rr.net = n.net AND rr.node = n.node
		WHERE rr.rn = 1
		ORDER BY rr.test_time DESC
		LIMIT ?`, nodeFilter)

	query = strings.ReplaceAll(query, "{{NODE_WINDOW}}", nodeIdentityWindowSQL(days))
	query = applyTestResultColumns(query)
	query = strings.ReplaceAll(query, "{{DOMAIN_FILTER}}", domainFilterSQL(domain, ""))
	query = applyCycleWindows(query, days)
	query = applyNodelistGate(query, "", domainFilterSQL(domain, ""), nodeFilter)

	rows, err := conn.QueryContext(ctx, query, days, limit)
	if err != nil {
		logQueryFailure("GetIPv6EnabledNodes: Query failed", err)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read IPv6-enabled nodes: %w", err)
	}

	// Fetch all hostnames for each node
	for i := range results {
		hostnames, err := ipv6.getAllHostnamesForNode(ctx, results[i].Zone, results[i].Net, results[i].Node, days, results[i].Domain)
		if cerr := contextErr(err); cerr != nil {
			// One missing hostname list is worth degrading over; a cancelled
			// request is not, and the rest of the loop would fail the same way.
			// CachedStorage would otherwise store the half-filled result.
			return nil, cerr
		}
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
func (ipv6 *IPv6QueryOperations) GetIPv6NonWorkingNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	ipv6.mu.RLock()
	defer ipv6.mu.RUnlock()

	conn := ipv6.db.Conn()

	// Build node filter condition
	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	query := fmt.Sprintf(`
		WITH
		-- Find nodes that have IPv6 addresses and were tested
		nodes_with_ipv6 AS (
			SELECT DISTINCT domain, zone, net, node
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				{{NODELIST_GATE}}
				AND length(resolved_ipv6) > 0
				AND (binkp_ipv6_tested = true OR ifcico_ipv6_tested = true OR telnet_ipv6_tested = true)
				%s
				{{DOMAIN_FILTER}}
		),
		-- Count successful IPv6 tests per node in the period
		ipv6_success_counts AS (
			SELECT
				domain, zone, net, node,
				countIf(binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true) as success_count
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				AND (domain, zone, net, node) IN (SELECT domain, zone, net, node FROM nodes_with_ipv6)
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		-- Get latest test for nodes with zero successful IPv6 tests
		latest_failed_tests AS (
			SELECT
				domain, zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE (domain, zone, net, node) IN (
				SELECT domain, zone, net, node
				FROM ipv6_success_counts
				WHERE success_count = 0
			)
			AND test_time >= now() - INTERVAL ? DAY
			{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		latest_nodes AS (
			SELECT
				domain, zone, net, node,
				argMax(system_name, nodelist_date) as system_name
			FROM nodes
			WHERE 1 = 1
				{{NODE_WINDOW}}
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		ranked_results AS (
			SELECT
				{{TEST_RESULT_COLUMNS_R}},
				row_number() OVER (PARTITION BY r.domain, r.zone, r.net, r.node ORDER BY r.is_aggregated DESC, r.hostname_index ASC) as rn
			FROM node_test_results r
			INNER JOIN latest_failed_tests lft ON r.domain = lft.domain AND r.zone = lft.zone AND r.net = lft.net AND r.node = lft.node
				AND {{CYCLE_LFT}}
		)
		SELECT
			{{TEST_RESULT_COLUMNS_RR_NODENAME}}
		FROM ranked_results rr
		LEFT JOIN latest_nodes n ON rr.domain = n.domain AND rr.zone = n.zone AND rr.net = n.net AND rr.node = n.node
		WHERE rr.rn = 1
		ORDER BY rr.test_time DESC
		LIMIT ?`, nodeFilter)

	query = strings.ReplaceAll(query, "{{NODE_WINDOW}}", nodeIdentityWindowSQL(days))
	query = applyTestResultColumns(query)
	query = strings.ReplaceAll(query, "{{DOMAIN_FILTER}}", domainFilterSQL(domain, ""))
	query = applyCycleWindows(query, days)
	query = applyNodelistGate(query, "", domainFilterSQL(domain, ""), nodeFilter)

	rows, err := conn.QueryContext(ctx, query, days, days, days, limit)
	if err != nil {
		logQueryFailure("GetIPv6NonWorkingNodes: Query failed", err)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read IPv6 non-working nodes: %w", err)
	}

	// Fetch all hostnames for each node
	for i := range results {
		hostnames, err := ipv6.getAllHostnamesForNode(ctx, results[i].Zone, results[i].Net, results[i].Node, days, results[i].Domain)
		if cerr := contextErr(err); cerr != nil {
			// One missing hostname list is worth degrading over; a cancelled
			// request is not, and the rest of the loop would fail the same way.
			// CachedStorage would otherwise store the half-filled result.
			return nil, cerr
		}
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

// GetIPv6AdvertisedIPv4OnlyNodes returns nodes that advertise IPv6 addresses but are only accessible via IPv4
// (IPv4 services work, but IPv6 services don't work despite having IPv6 addresses)
func (ipv6 *IPv6QueryOperations) GetIPv6AdvertisedIPv4OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	ipv6.mu.RLock()
	defer ipv6.mu.RUnlock()

	conn := ipv6.db.Conn()

	// Build node filter condition
	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	query := fmt.Sprintf(`
		WITH
		-- Find nodes that have IPv6 addresses and working IPv4 services
		nodes_with_working_ipv4 AS (
			SELECT DISTINCT domain, zone, net, node
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				{{NODELIST_GATE}}
				AND length(resolved_ipv6) > 0
				AND is_operational = true
				AND (binkp_success = true OR ifcico_success = true OR telnet_success = true)
				%s
				{{DOMAIN_FILTER}}
		),
		-- Count successful IPv6 tests per node in the period
		ipv6_success_counts AS (
			SELECT
				domain, zone, net, node,
				countIf(binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true) as success_count
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				AND (domain, zone, net, node) IN (SELECT domain, zone, net, node FROM nodes_with_working_ipv4)
				AND (binkp_ipv6_tested = true OR ifcico_ipv6_tested = true OR telnet_ipv6_tested = true)
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		-- Get latest test for nodes with zero successful IPv6 tests but working IPv4
		latest_ipv4_only_tests AS (
			SELECT
				domain, zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE (domain, zone, net, node) IN (
				SELECT domain, zone, net, node
				FROM ipv6_success_counts
				WHERE success_count = 0
			)
			AND test_time >= now() - INTERVAL ? DAY
			{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		latest_nodes AS (
			SELECT
				domain, zone, net, node,
				argMax(system_name, nodelist_date) as system_name
			FROM nodes
			WHERE 1 = 1
				{{NODE_WINDOW}}
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		ranked_results AS (
			SELECT
				{{TEST_RESULT_COLUMNS_R}},
				row_number() OVER (PARTITION BY r.domain, r.zone, r.net, r.node ORDER BY r.is_aggregated DESC, r.hostname_index ASC) as rn
			FROM node_test_results r
			INNER JOIN latest_ipv4_only_tests lit ON r.domain = lit.domain AND r.zone = lit.zone AND r.net = lit.net AND r.node = lit.node
				AND {{CYCLE_LIT}}
			WHERE 1 = 1 {{DOMAIN_FILTER_R}}
		)
		SELECT
			{{TEST_RESULT_COLUMNS_RR_NODENAME}}
		FROM ranked_results rr
		LEFT JOIN latest_nodes n ON rr.domain = n.domain AND rr.zone = n.zone AND rr.net = n.net AND rr.node = n.node
		WHERE rr.rn = 1
		ORDER BY rr.test_time DESC
		LIMIT ?`, nodeFilter)

	query = strings.ReplaceAll(query, "{{NODE_WINDOW}}", nodeIdentityWindowSQL(days))
	query = applyTestResultColumns(query)
	query = strings.ReplaceAll(query, "{{DOMAIN_FILTER}}", domainFilterSQL(domain, ""))
	query = applyCycleWindows(query, days)
	query = applyNodelistGate(query, "", domainFilterSQL(domain, ""), nodeFilter)
	query = strings.ReplaceAll(query, "{{DOMAIN_FILTER_R}}", domainFilterSQL(domain, "r."))

	rows, err := conn.QueryContext(ctx, query, days, days, days, limit)
	if err != nil {
		logQueryFailure("GetIPv6AdvertisedIPv4OnlyNodes: Query failed", err)
		return nil, fmt.Errorf("failed to search IPv6-advertised IPv4-only nodes: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	rowCount := 0
	for rows.Next() {
		rowCount++
		var r NodeTestResult
		err := ipv6.resultParser.ParseTestResultRow(rows, &r)
		if err != nil {
			logging.Error("GetIPv6AdvertisedIPv4OnlyNodes: Failed to parse row", slog.Int("row", rowCount), slog.Any("error", err))
			return nil, fmt.Errorf("failed to parse test result row %d: %w", rowCount, err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read IPv6-advertised IPv4-only nodes: %w", err)
	}

	// Fetch all hostnames for each node
	for i := range results {
		hostnames, err := ipv6.getAllHostnamesForNode(ctx, results[i].Zone, results[i].Net, results[i].Node, days, results[i].Domain)
		if cerr := contextErr(err); cerr != nil {
			// One missing hostname list is worth degrading over; a cancelled
			// request is not, and the rest of the loop would fail the same way.
			// CachedStorage would otherwise store the half-filled result.
			return nil, cerr
		}
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

// GetIPv6OnlyNodes returns nodes that have working IPv6 services but NO working IPv4 services
// This shows nodes with IPv6 connectivity where IPv4 services failed or were not tested
// (These nodes may still have IPv4 addresses configured, but IPv4 protocols don't work)
func (ipv6 *IPv6QueryOperations) GetIPv6OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	ipv6.mu.RLock()
	defer ipv6.mu.RUnlock()

	conn := ipv6.db.Conn()

	// Build node filter condition
	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	query := fmt.Sprintf(`
		WITH latest_tests AS (
			SELECT
				domain, zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				{{NODELIST_GATE}}
				AND length(resolved_ipv6) > 0
				AND (binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true)
				AND NOT (binkp_ipv4_success = true OR ifcico_ipv4_success = true OR telnet_ipv4_success = true)
				%s
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		latest_nodes AS (
			SELECT
				domain, zone, net, node,
				argMax(system_name, nodelist_date) as system_name
			FROM nodes
			WHERE 1 = 1
				{{NODE_WINDOW}}
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		ranked_results AS (
			SELECT
				{{TEST_RESULT_COLUMNS_R}},
				row_number() OVER (PARTITION BY r.domain, r.zone, r.net, r.node ORDER BY r.is_aggregated DESC, r.hostname_index ASC) as rn
			FROM node_test_results r
			INNER JOIN latest_tests lt ON r.domain = lt.domain AND r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node
				AND {{CYCLE_LT}}
			-- Re-apply the report's own criteria to the candidate rows. The
			-- aggregated row ORs protocol successes across hostnames, so a node
			-- that qualified through one IPv6-only hostname would otherwise be
			-- represented by an aggregate that shows IPv4 working - contradicting
			-- the report. The anchor is the latest QUALIFYING test, so at least
			-- one row always survives this and no node is dropped.
			WHERE length(r.resolved_ipv6) > 0
				AND (r.binkp_ipv6_success = true OR r.ifcico_ipv6_success = true OR r.telnet_ipv6_success = true)
				AND NOT (r.binkp_ipv4_success = true OR r.ifcico_ipv4_success = true OR r.telnet_ipv4_success = true)
		)
		SELECT
			{{TEST_RESULT_COLUMNS_RR_NODENAME}}
		FROM ranked_results rr
		LEFT JOIN latest_nodes n ON rr.domain = n.domain AND rr.zone = n.zone AND rr.net = n.net AND rr.node = n.node
		WHERE rr.rn = 1
		ORDER BY rr.test_time DESC
		LIMIT ?`, nodeFilter)

	query = strings.ReplaceAll(query, "{{NODE_WINDOW}}", nodeIdentityWindowSQL(days))
	query = applyTestResultColumns(query)
	query = strings.ReplaceAll(query, "{{DOMAIN_FILTER}}", domainFilterSQL(domain, ""))
	query = applyCycleWindows(query, days)
	query = applyNodelistGate(query, "", domainFilterSQL(domain, ""), nodeFilter)

	rows, err := conn.QueryContext(ctx, query, days, limit)
	if err != nil {
		logQueryFailure("GetIPv6OnlyNodes: Query failed", err)
		return nil, fmt.Errorf("failed to search IPv6-only nodes: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	rowCount := 0
	for rows.Next() {
		rowCount++
		var r NodeTestResult
		err := ipv6.resultParser.ParseTestResultRow(rows, &r)
		if err != nil {
			logging.Error("GetIPv6OnlyNodes: Failed to parse row", slog.Int("row", rowCount), slog.Any("error", err))
			return nil, fmt.Errorf("failed to parse test result row %d: %w", rowCount, err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read IPv6-only nodes: %w", err)
	}

	// Fetch all hostnames for each node
	for i := range results {
		hostnames, err := ipv6.getAllHostnamesForNode(ctx, results[i].Zone, results[i].Net, results[i].Node, days, results[i].Domain)
		if cerr := contextErr(err); cerr != nil {
			// One missing hostname list is worth degrading over; a cancelled
			// request is not, and the rest of the loop would fail the same way.
			// CachedStorage would otherwise store the half-filled result.
			return nil, cerr
		}
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

// GetPureIPv6OnlyNodes returns nodes that ONLY advertise IPv6 addresses (no IPv4 addresses at all)
// This is different from GetIPv6OnlyNodes which includes nodes with IPv4 addresses but non-working IPv4 services
func (ipv6 *IPv6QueryOperations) GetPureIPv6OnlyNodes(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	ipv6.mu.RLock()
	defer ipv6.mu.RUnlock()

	conn := ipv6.db.Conn()

	// Build node filter condition
	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	query := fmt.Sprintf(`
		WITH latest_tests AS (
			SELECT
				domain, zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				{{NODELIST_GATE}}
				AND length(resolved_ipv6) > 0
				AND length(resolved_ipv4) = 0
				AND (binkp_ipv6_success = true OR ifcico_ipv6_success = true OR telnet_ipv6_success = true)
				%s
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		latest_nodes AS (
			SELECT
				domain, zone, net, node,
				argMax(system_name, nodelist_date) as system_name
			FROM nodes
			WHERE 1 = 1
				{{NODE_WINDOW}}
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		ranked_results AS (
			SELECT
				{{TEST_RESULT_COLUMNS_R}},
				row_number() OVER (PARTITION BY r.domain, r.zone, r.net, r.node ORDER BY r.is_aggregated DESC, r.hostname_index ASC) as rn
			FROM node_test_results r
			INNER JOIN latest_tests lt ON r.domain = lt.domain AND r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node
				AND {{CYCLE_LT}}
			-- Re-apply the report's own criteria: the aggregated row UNIONs
			-- resolved addresses across hostnames, so it can carry IPv4 addresses
			-- that the qualifying IPv6-only hostname does not have. See the same
			-- guard in GetIPv6OnlyNodes.
			WHERE length(r.resolved_ipv6) > 0
				AND length(r.resolved_ipv4) = 0
				AND (r.binkp_ipv6_success = true OR r.ifcico_ipv6_success = true OR r.telnet_ipv6_success = true)
		)
		SELECT
			{{TEST_RESULT_COLUMNS_RR_NODENAME}}
		FROM ranked_results rr
		LEFT JOIN latest_nodes n ON rr.domain = n.domain AND rr.zone = n.zone AND rr.net = n.net AND rr.node = n.node
		WHERE rr.rn = 1
		ORDER BY rr.test_time DESC
		LIMIT ?`, nodeFilter)

	query = strings.ReplaceAll(query, "{{NODE_WINDOW}}", nodeIdentityWindowSQL(days))
	query = applyTestResultColumns(query)
	query = strings.ReplaceAll(query, "{{DOMAIN_FILTER}}", domainFilterSQL(domain, ""))
	query = applyCycleWindows(query, days)
	query = applyNodelistGate(query, "", domainFilterSQL(domain, ""), nodeFilter)

	rows, err := conn.QueryContext(ctx, query, days, limit)
	if err != nil {
		logQueryFailure("GetPureIPv6OnlyNodes: Query failed", err)
		return nil, fmt.Errorf("failed to search pure IPv6-only nodes: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	rowCount := 0
	for rows.Next() {
		rowCount++
		var r NodeTestResult
		err := ipv6.resultParser.ParseTestResultRow(rows, &r)
		if err != nil {
			logging.Error("GetPureIPv6OnlyNodes: Failed to parse row", slog.Int("row", rowCount), slog.Any("error", err))
			return nil, fmt.Errorf("failed to parse test result row %d: %w", rowCount, err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read pure IPv6-only nodes: %w", err)
	}

	// Fetch all hostnames for each node
	for i := range results {
		hostnames, err := ipv6.getAllHostnamesForNode(ctx, results[i].Zone, results[i].Net, results[i].Node, days, results[i].Domain)
		if cerr := contextErr(err); cerr != nil {
			// One missing hostname list is worth degrading over; a cancelled
			// request is not, and the rest of the loop would fail the same way.
			// CachedStorage would otherwise store the half-filled result.
			return nil, cerr
		}
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

// GetIPv6NodeList returns verified working IPv6 nodes for the IPv6 node list report (Michiel's format).
// Only includes nodes where BinkP or IFCICO succeeded over IPv6 AND address_validated = true.
// Uses the general address_validated field (populated for all tests) rather than address_validated_ipv6
// (only populated after the per-IPv4/IPv6 AKA split was deployed). As the daemon re-tests nodes,
// address_validated_ipv6 will gradually be populated; this can be switched later if needed.
func (ipv6 *IPv6QueryOperations) GetIPv6NodeList(ctx context.Context, limit int, days int, includeZeroNodes bool, domain string) ([]IPv6NodeListEntry, error) {
	ipv6.mu.RLock()
	defer ipv6.mu.RUnlock()

	conn := ipv6.db.Conn()

	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	query := fmt.Sprintf(`
		WITH latest_tests AS (
			SELECT domain, zone, net, node, max(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				{{NODELIST_GATE}}
				AND is_aggregated = false
				AND (binkp_ipv6_success = true OR ifcico_ipv6_success = true)
				AND address_validated = true
				%s
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		latest_nodes AS (
			SELECT domain, zone, net, node,
				argMax(sysop_name, nodelist_date) as sysop_name
			FROM nodes
			WHERE 1 = 1 {{NODE_WINDOW}} {{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		stability AS (
			SELECT domain, zone, net, node,
				uniqExact(test_time) as ipv6_failure_count
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL 30 DAY
				AND is_aggregated = false
				AND (
					(binkp_ipv6_tested = true AND binkp_ipv6_success = false) OR
					(ifcico_ipv6_tested = true AND ifcico_ipv6_success = false)
				)
				AND NOT (binkp_ipv6_success = true OR ifcico_ipv6_success = true)
				%s
				{{DOMAIN_FILTER}}
			GROUP BY domain, zone, net, node
		),
		best_results AS (
			SELECT r.domain, r.test_time, r.zone, r.net, r.node,
				r.resolved_ipv6, r.isp, r.org,
				r.binkp_ipv4_success, r.ifcico_ipv4_success, r.telnet_ipv4_success,
				row_number() OVER (PARTITION BY r.domain, r.zone, r.net, r.node
					ORDER BY r.hostname_index ASC) as rn
			FROM node_test_results r
			INNER JOIN latest_tests lt ON r.domain = lt.domain AND r.zone = lt.zone AND r.net = lt.net
				AND r.node = lt.node AND {{CYCLE_LT}}
			WHERE r.is_aggregated = false
				AND (r.binkp_ipv6_success = true OR r.ifcico_ipv6_success = true)
				AND r.address_validated = true
				{{DOMAIN_FILTER_R}}
		)
		SELECT br.test_time, br.zone, br.net, br.node,
			COALESCE(n.sysop_name, '') as sysop_name,
			br.resolved_ipv6, br.isp, br.org,
			br.binkp_ipv4_success, br.ifcico_ipv4_success, br.telnet_ipv4_success,
			COALESCE(s.ipv6_failure_count, 0) as ipv6_failure_count
		FROM best_results br
		LEFT JOIN latest_nodes n ON br.domain = n.domain AND br.zone = n.zone AND br.net = n.net AND br.node = n.node
		LEFT JOIN stability s ON br.domain = s.domain AND br.zone = s.zone AND br.net = s.net AND br.node = s.node
		WHERE br.rn = 1
		ORDER BY br.zone, br.net, br.node
		LIMIT ?`, nodeFilter, nodeFilter)

	query = strings.ReplaceAll(query, "{{NODE_WINDOW}}", nodeIdentityWindowSQL(days))
	query = applyTestResultColumns(query)
	query = strings.ReplaceAll(query, "{{DOMAIN_FILTER}}", domainFilterSQL(domain, ""))
	query = applyCycleWindows(query, days)
	query = applyNodelistGate(query, "", domainFilterSQL(domain, ""), nodeFilter)
	query = strings.ReplaceAll(query, "{{DOMAIN_FILTER_R}}", domainFilterSQL(domain, "r."))

	rows, err := conn.QueryContext(ctx, query, days, limit)
	if err != nil {
		logQueryFailure("GetIPv6NodeList: Query failed", err)
		return nil, fmt.Errorf("failed to query IPv6 node list: %w", err)
	}
	defer rows.Close()

	var results []IPv6NodeListEntry
	for rows.Next() {
		var entry IPv6NodeListEntry
		var ipv6FailureCount uint64
		err := rows.Scan(
			&entry.TestTime, &entry.Zone, &entry.Net, &entry.Node,
			&entry.SysopName,
			&entry.ResolvedIPv6, &entry.ISP, &entry.Org,
			&entry.BinkPIPv4Success, &entry.IfcicoIPv4Success, &entry.TelnetIPv4Success,
			&ipv6FailureCount,
		)
		if err != nil {
			logging.Error("GetIPv6NodeList: Failed to scan row", slog.Any("error", err))
			return nil, fmt.Errorf("failed to scan IPv6 node list row: %w", err)
		}

		// Compute derived fields
		entry.IPv6Type = detectIPv6Type(entry.ResolvedIPv6)
		entry.Provider = detectProvider(entry.ISP, entry.Org)
		entry.FidoIPv6Addr = findFidoStyleAddress(entry.ResolvedIPv6)
		entry.HasFidoAddr = entry.FidoIPv6Addr != ""
		entry.HasNoIPv4 = !entry.BinkPIPv4Success && !entry.IfcicoIPv4Success && !entry.TelnetIPv4Success
		entry.IsUnstable = ipv6FailureCount > 2
		entry.Remarks = buildRemarks(entry)

		results = append(results, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating IPv6 node list rows: %w", err)
	}

	return results, nil
}

// detectIPv6Type determines IPv6 connectivity type from resolved addresses.
// If a node has both tunneled and native addresses, prefers "Native".
func detectIPv6Type(addresses []string) string {
	if len(addresses) == 0 {
		return "Unknown"
	}
	tunnelType := ""
	for _, addr := range addresses {
		if strings.HasPrefix(addr, "2002:") {
			// 6to4 tunnel: 2002::/16 (RFC 3056)
			if tunnelType == "" {
				tunnelType = "T-6to4"
			}
		} else if strings.HasPrefix(addr, "2001:0000:") || strings.HasPrefix(addr, "2001:0:") {
			// Teredo tunnel: 2001:0000::/32 (RFC 4380)
			if tunnelType == "" {
				tunnelType = "T-Teredo"
			}
		} else if strings.HasPrefix(addr, "2001:470:") || strings.HasPrefix(addr, "2001:0470:") {
			// Hurricane Electric tunnel broker: 2001:470::/32
			// Note: this is HE's entire allocation; native HE customers also use this range,
			// but in the FidoNet community most HE users are tunnel broker users.
			if tunnelType == "" {
				tunnelType = "T-6in4"
			}
		} else if strings.HasPrefix(addr, "2001:5c0:") || strings.HasPrefix(addr, "2001:05c0:") {
			// Freenet6/GoGo6 tunnel broker: 2001:5c0::/32
			if tunnelType == "" {
				tunnelType = "T-6in4"
			}
		} else {
			// Non-tunnel address found, this is native
			return "Native"
		}
	}
	if tunnelType != "" {
		return tunnelType
	}
	return "Native"
}

// detectProvider returns a cleaned provider name from ISP/Org fields.
func detectProvider(isp, org string) string {
	provider := isp
	if provider == "" {
		provider = org
	}
	if provider == "" {
		return "Unknown"
	}
	return provider
}

// findFidoStyleAddress returns the first resolved IPv6 address containing an f1d0 segment,
// indicating a FidoNet-style IPv6 address convention. Returns empty string if none found.
func findFidoStyleAddress(addresses []string) string {
	for _, addr := range addresses {
		lower := strings.ToLower(addr)
		if strings.Contains(lower, ":f1d0:") || strings.HasPrefix(lower, "f1d0:") {
			return addr
		}
	}
	return ""
}

// buildRemarks constructs the remarks string for a node list entry.
func buildRemarks(entry IPv6NodeListEntry) string {
	var parts []string
	if entry.HasFidoAddr {
		parts = append(parts, "f")
	}
	if entry.HasNoIPv4 {
		parts = append(parts, "INO4")
	}
	if entry.IsUnstable {
		parts = append(parts, "6UNS")
	}
	return strings.Join(parts, " ")
}
