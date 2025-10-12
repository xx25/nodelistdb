package storage

import (
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
func (r *ReachabilityOperations) GetNodeReachabilityStats(zone, net, node int, days int) (*NodeReachabilityStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn := r.db.Conn()
	query := r.queryBuilder.BuildReachabilityStatsQuery()

	row := conn.QueryRow(query, zone, net, node, days)

	var stats NodeReachabilityStats
	var lastStatus bool
	err := row.Scan(
		&stats.Zone,
		&stats.Net,
		&stats.Node,
		&stats.TotalTests,
		&stats.SuccessfulTests,
		&stats.FailedTests,
		&stats.SuccessRate,
		&stats.AverageResponseMs,
		&stats.LastTestTime,
		&lastStatus,
		&stats.BinkPSuccessRate,
		&stats.IfcicoSuccessRate,
		&stats.TelnetSuccessRate,
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

// GetReachabilityTrends gets daily reachability trends
func (r *ReachabilityOperations) GetReachabilityTrends(days int) ([]ReachabilityTrend, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn := r.db.Conn()
	query := r.queryBuilder.BuildReachabilityTrendsQuery()

	rows, err := conn.Query(query, days)
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

	return trends, nil
}

// SearchNodesByReachability searches for nodes by reachability status
func (r *ReachabilityOperations) SearchNodesByReachability(operational bool, limit int, days int) ([]NodeTestResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn := r.db.Conn()
	query := r.queryBuilder.BuildSearchByReachabilityQuery()

	rows, err := conn.Query(query, days, operational, limit)
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

	return results, nil
}
