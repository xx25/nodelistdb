package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

// GetNodesWithInternet retrieves nodes from ClickHouse nodes table
func (s *ClickHouseStorage) GetNodesWithInternet(ctx context.Context, limit int) ([]*models.Node, error) {
	// This query extracts ALL addresses from ALL protocols (supporting arrays)
	query := `
		SELECT
			zone, net, node, system_name, sysop_name, location,
			-- Extract all hostnames from all protocols as a comma-separated string
			-- This will be parsed later to handle multiple addresses per protocol
			'' as internet_hostnames,  -- Will be populated from config_json
			arrayStringConcat(JSONExtractKeys(toString(internet_config), 'protocols'), ',') as internet_protocols,
			has_inet,
			toString(internet_config) as config_json,
			domain
		FROM nodes
		WHERE has_inet = true
			AND JSONLength(toString(internet_config), 'protocols') > 0
			AND (domain, nodelist_date) IN (SELECT domain, MAX(nodelist_date) FROM nodes GROUP BY domain)
		ORDER BY domain, zone, net, node
	`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	// Use native connection to avoid SQL DB issues
	rows, err := s.conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	return scanNodesNative(rows)
}

// GetNodesByZone retrieves nodes from a specific zone
func (s *ClickHouseStorage) GetNodesByZone(ctx context.Context, zone int) ([]*models.Node, error) {
	query := `
		SELECT
			zone, net, node, system_name, sysop_name, location,
			'' as internet_hostnames,  -- Will be populated from config_json
			arrayStringConcat(JSONExtractKeys(toString(internet_config), 'protocols'), ',') as internet_protocols,
			has_inet,
			toString(internet_config) as config_json,
			domain
		FROM nodes
		WHERE zone = ? AND has_inet = true
			AND JSONLength(toString(internet_config), 'protocols') > 0
			AND (domain, nodelist_date) IN (SELECT domain, MAX(nodelist_date) FROM nodes GROUP BY domain)
		ORDER BY domain, net, node
	`

	// Use native connection with positional parameters
	rows, err := s.conn.Query(ctx, query, zone)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes by zone: %w", err)
	}
	defer rows.Close()

	return scanNodesNative(rows)
}

// GetNodesByProtocol retrieves nodes that support a specific protocol
func (s *ClickHouseStorage) GetNodesByProtocol(ctx context.Context, protocol string, limit int) ([]*models.Node, error) {
	query := `
		SELECT
			zone, net, node, system_name, sysop_name, location,
			'' as internet_hostnames,  -- Will be populated from config_json
			arrayStringConcat(JSONExtractKeys(toString(internet_config), 'protocols'), ',') as internet_protocols,
			has_inet,
			toString(internet_config) as config_json,
			domain
		FROM nodes
		WHERE has_inet = true
			AND JSONHas(toString(internet_config), 'protocols', ?)
			AND (domain, nodelist_date) IN (SELECT domain, MAX(nodelist_date) FROM nodes GROUP BY domain)
		ORDER BY domain, zone, net, node
	`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	// Use native connection with positional parameters
	rows, err := s.conn.Query(ctx, query, protocol)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes by protocol: %w", err)
	}
	defer rows.Close()

	return scanNodesNative(rows)
}

// GetLatestNodelistDate returns the most recent nodelist date in the database
func (s *ClickHouseStorage) GetLatestNodelistDate(ctx context.Context) (time.Time, error) {
	query := `SELECT MAX(nodelist_date) FROM nodes`

	var maxDate time.Time
	row := s.conn.QueryRow(ctx, query)
	err := row.Scan(&maxDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get latest nodelist date: %w", err)
	}

	return maxDate, nil
}

// GetNodelistFingerprint returns a stable string identifying the latest
// nodelist date of every network (e.g. "fidonet=2026-07-16,fsxnet=2026-07-10").
// Unlike a global MAX(nodelist_date), it changes whenever ANY network imports
// a new nodelist — including networks whose latest date is older than another
// network's, which a global max would never notice.
func (s *ClickHouseStorage) GetNodelistFingerprint(ctx context.Context) (string, error) {
	query := `
		SELECT arrayStringConcat(
			arraySort(groupArray(concat(domain, '=', toString(latest)))), ',')
		FROM (
			SELECT domain, max(nodelist_date) AS latest FROM nodes GROUP BY domain
		)`

	var fingerprint string
	if err := s.conn.QueryRow(ctx, query).Scan(&fingerprint); err != nil {
		return "", fmt.Errorf("failed to get nodelist fingerprint: %w", err)
	}
	return fingerprint, nil
}

// GetCurrentNodeStatus gets the latest status for each node from test results
func (s *ClickHouseStorage) GetCurrentNodeStatus(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT
			zone, net, node, address,
			argMax(test_time, test_time) as last_test_time,
			argMax(is_operational, test_time) as is_operational,
			argMax(binkp_success, test_time) as binkp_works,
			argMax(country, test_time) as country,
			argMax(isp, test_time) as isp
		FROM node_test_results
		WHERE test_time > now() - INTERVAL 7 DAY
		GROUP BY zone, net, node, address
		ORDER BY zone, net, node
	`

	rows, err := s.conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get current node status: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var zone, net, node int32
		var address, country, isp string
		var lastTestTime time.Time
		var isOperational, binkpWorks bool

		err := rows.Scan(&zone, &net, &node, &address, &lastTestTime,
			&isOperational, &binkpWorks, &country, &isp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		results = append(results, map[string]interface{}{
			"zone":           zone,
			"net":            net,
			"node":           node,
			"address":        address,
			"last_test_time": lastTestTime,
			"is_operational": isOperational,
			"binkp_works":    binkpWorks,
			"country":        country,
			"isp":            isp,
		})
	}

	return results, nil
}
