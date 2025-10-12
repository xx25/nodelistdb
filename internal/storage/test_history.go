package storage

import (
	"fmt"
	"sync"

	"github.com/nodelistdb/internal/database"
)

// TestHistoryOperations handles test history retrieval operations
type TestHistoryOperations struct {
	db           database.DatabaseInterface
	queryBuilder *TestQueryBuilder
	resultParser ResultParserInterface
	mu           sync.RWMutex
}

// NewTestHistoryOperations creates a new test history operations instance
func NewTestHistoryOperations(db database.DatabaseInterface, queryBuilder *TestQueryBuilder, resultParser ResultParserInterface) *TestHistoryOperations {
	return &TestHistoryOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// GetNodeTestHistory retrieves test history for a specific node
func (th *TestHistoryOperations) GetNodeTestHistory(zone, net, node int, days int) ([]NodeTestResult, error) {
	th.mu.RLock()
	defer th.mu.RUnlock()

	conn := th.db.Conn()
	query := th.queryBuilder.BuildTestHistoryQuery()

	rows, err := conn.Query(query, zone, net, node, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query node test history: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	for rows.Next() {
		var r NodeTestResult
		err := th.resultParser.ParseTestResultRow(rows, &r)
		if err != nil {
			return nil, fmt.Errorf("failed to parse test result: %w", err)
		}
		results = append(results, r)
	}

	return results, nil
}

// GetDetailedTestResult retrieves a detailed test result for a specific node and timestamp
func (th *TestHistoryOperations) GetDetailedTestResult(zone, net, node int, testTime string) (*NodeTestResult, error) {
	th.mu.RLock()
	defer th.mu.RUnlock()

	conn := th.db.Conn()
	query := th.queryBuilder.BuildDetailedTestResultQuery()

	row := conn.QueryRow(query, zone, net, node, testTime)

	var result NodeTestResult
	err := th.resultParser.ParseTestResultRow(&singleRowScanner{row}, &result)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to parse detailed test result: %w", err)
	}

	return &result, nil
}
