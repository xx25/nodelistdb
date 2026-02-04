package storage

import (
	"fmt"
	"sync"

	"github.com/nodelistdb/internal/database"
)

// AKAMismatchOperations handles AKA address validation queries
type AKAMismatchOperations struct {
	db           database.DatabaseInterface
	queryBuilder *TestQueryBuilder
	resultParser ResultParserInterface
	mu           sync.RWMutex
}

// NewAKAMismatchOperations creates a new AKA mismatch operations instance
func NewAKAMismatchOperations(db database.DatabaseInterface, queryBuilder *TestQueryBuilder, resultParser ResultParserInterface) *AKAMismatchOperations {
	return &AKAMismatchOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// GetAKAMismatchNodes retrieves nodes where the announced AKA doesn't match the expected nodelist address
func (am *AKAMismatchOperations) GetAKAMismatchNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	conn := am.db.Conn()

	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	query := am.buildAKAMismatchQuery(nodeFilter)

	rows, err := conn.Query(query, days, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query AKA mismatch nodes: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	for rows.Next() {
		var r NodeTestResult
		err := am.resultParser.ParseTestResultRow(rows, &r)
		if err != nil {
			return nil, fmt.Errorf("failed to parse test result: %w", err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating AKA mismatch rows: %w", err)
	}

	return results, nil
}

// buildAKAMismatchQuery builds the query for nodes with AKA mismatches
// Only shows nodes where the LATEST test has a mismatch (excludes historical mismatches that were fixed)
// Excludes aggregated results since they don't track address_validated
func (am *AKAMismatchOperations) buildAKAMismatchQuery(nodeFilter string) string {
	return fmt.Sprintf(`
		WITH
		-- First, find the latest test time for each node (regardless of mismatch status)
		-- Only consider non-aggregated results since aggregated rows don't set address_validated
		latest_tests AS (
			SELECT
				zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				AND is_aggregated = false
				AND is_operational = true
				AND (binkp_success = true OR ifcico_success = true)
				%s
			GROUP BY zone, net, node
		),
		-- Get the best non-aggregated result at the latest test time
		-- Prioritize rows with address_validated=false (mismatched) first, then by hostname_index
		-- Only consider operational rows with successful BinkP or IFCICO handshake
		best_results AS (
			SELECT
				r.zone, r.net, r.node, r.test_time,
				row_number() OVER (
					PARTITION BY r.zone, r.net, r.node
					ORDER BY r.address_validated ASC, r.hostname_index ASC
				) as rn
			FROM node_test_results r
			JOIN latest_tests lt ON r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node AND r.test_time = lt.latest_test_time
			WHERE r.is_aggregated = false
				AND r.is_operational = true
				AND (r.binkp_success = true OR r.ifcico_success = true)
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
			r.total_hostnames, r.hostnames_tested, r.hostnames_operational
		FROM node_test_results r
		JOIN best_results br ON r.zone = br.zone AND r.net = br.net AND r.node = br.node AND r.test_time = br.test_time AND br.rn = 1
		WHERE r.address_validated = false
		ORDER BY r.test_time DESC
		LIMIT ?`, nodeFilter)
}
