package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/nodelistdb/internal/database"
)

// ReachabilityOperations handles reachability-related test queries
type ReachabilityOperations struct {
	db           database.DatabaseInterface
	queryBuilder *TestQueryBuilder
	resultParser ResultParserInterface
	mu           sync.RWMutex
}

// NewReachabilityOperations creates a new reachability operations instance
func NewReachabilityOperations(db database.DatabaseInterface, queryBuilder *TestQueryBuilder, resultParser ResultParserInterface) *ReachabilityOperations {
	return &ReachabilityOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// GetNodeReachabilityStats calculates reachability statistics for a node
func (r *ReachabilityOperations) GetNodeReachabilityStats(ctx context.Context, zone, net, node int, days int, domain string) (*NodeReachabilityStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn := r.db.Conn()
	query := r.queryBuilder.BuildReachabilityStatsQuery()

	row := conn.QueryRowContext(ctx, query, zone, net, node, days, domain, domain)

	var stats NodeReachabilityStats
	var lastStatus bool
	err := row.Scan(
		&stats.Zone,
		&stats.Net,
		&stats.Node,
		&stats.TotalTests,
		&stats.FullySuccessfulTests,
		&stats.PartiallyFailedTests,
		&stats.FailedTests,
		&stats.SuccessfulTests,
		&stats.SuccessRate,
		&stats.AverageResponseMs,
		&stats.LastTestTime,
		&lastStatus,
		&stats.BinkPSuccessRate,
		&stats.IfcicoSuccessRate,
		&stats.TelnetSuccessRate,
		&stats.BinkPIPv4SuccessRate,
		&stats.IfcicoIPv4SuccessRate,
		&stats.TelnetIPv4SuccessRate,
		&stats.BinkPIPv6SuccessRate,
		&stats.IfcicoIPv6SuccessRate,
		&stats.TelnetIPv6SuccessRate,
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

// GetReachabilityTrendsAllTime returns trends from the first test date to now.
// Uses the same carry-forward query as GetReachabilityTrends but computes the
// day range dynamically from the earliest test result.
func (r *ReachabilityOperations) GetReachabilityTrendsAllTime(ctx context.Context, domain string) ([]ReachabilityTrend, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn := r.db.Conn()

	// Find how many days back the earliest test is (within the selected
	// network, if any)
	var days int
	err := conn.QueryRowContext(ctx, `
		SELECT toUInt32(dateDiff('day', min(test_date), today()))
		FROM node_test_results
		WHERE test_date >= '2025-09-18'
		AND (? = '' OR domain = ?)
	`, domain, domain).Scan(&days)
	if err != nil || days == 0 {
		days = 90 // fallback
	}

	return r.queryTrends(ctx, conn, days, domain)
}

// GetReachabilityTrends gets daily reachability trends
func (r *ReachabilityOperations) GetReachabilityTrends(ctx context.Context, days int, domain string) ([]ReachabilityTrend, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.queryTrends(ctx, r.db.Conn(), days, domain)
}

func (r *ReachabilityOperations) queryTrends(ctx context.Context, conn *sql.DB, days int, domain string) ([]ReachabilityTrend, error) {
	query := r.queryBuilder.BuildReachabilityTrendsQuery()

	rows, err := conn.QueryContext(ctx, query, domain, domain, days)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read reachability trends: %w", err)
	}

	return trends, nil
}

// ReachabilityFilter selects the latest test result per node.
//
// Status is "operational", "failed" or "" for both. Protocol is one of
// ReachabilityProtocols or "" for any; it means "that protocol succeeded in
// the node's latest test" (for vmodem, a confirmed VMP responder). Days is
// the look-back window, Limit the page size, Domain the FTN network ("" for
// every network).
type ReachabilityFilter struct {
	Status   string
	Protocol string
	Days     int
	Limit    int
	Domain   string
}

// ReachabilityStatuses lists the accepted Status values besides "".
var ReachabilityStatuses = []string{"operational", "failed"}

// ReachabilityProtocols lists the accepted Protocol values besides "".
var ReachabilityProtocols = []string{"binkp", "ifcico", "telnet", "ftp", "vmodem"}

// Validate reports the first field that holds a value the query cannot
// express, so a handler can turn it into a 400 instead of a silent "any".
func (f ReachabilityFilter) Validate() error {
	if f.Status != "" && !containsString(ReachabilityStatuses, f.Status) {
		return fmt.Errorf("status must be one of operational, failed")
	}
	if f.Protocol != "" && !containsString(ReachabilityProtocols, f.Protocol) {
		return fmt.Errorf("protocol must be one of binkp, ifcico, telnet, ftp, vmodem")
	}
	if f.Days <= 0 {
		return fmt.Errorf("days must be positive")
	}
	if f.Limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}
	return nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// SearchNodesByReachability returns each node's latest test result in the
// window, newest first, narrowed by status and protocol in SQL so that a
// page is never short of rows that exist.
func (r *ReachabilityOperations) SearchNodesByReachability(ctx context.Context, f ReachabilityFilter) ([]NodeTestResult, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn := r.db.Conn()
	query := r.queryBuilder.BuildSearchByReachabilityQuery(f.Protocol)

	rows, err := conn.QueryContext(ctx, query, f.Days, f.Domain, f.Domain, f.Status, f.Status, f.Status, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search nodes by reachability: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	for rows.Next() {
		var result NodeTestResult
		err := r.resultParser.ParseTestResultRow(rows, &result)
		if err != nil {
			return nil, fmt.Errorf("failed to parse test result: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read nodes by reachability: %w", err)
	}

	return results, nil
}
