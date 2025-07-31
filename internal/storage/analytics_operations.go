package storage

import (
	"fmt"
	"sync"
	"time"

	"nodelistdb/internal/database"
)

// FlagFirstAppearance represents the first occurrence of a flag
type FlagFirstAppearance struct {
	Zone         int       `json:"zone"`
	Net          int       `json:"net"`
	Node         int       `json:"node"`
	NodelistDate time.Time `json:"nodelist_date"`
	SystemName   string    `json:"system_name"`
	Location     string    `json:"location"`
	SysopName    string    `json:"sysop_name"`
}

// FlagUsageByYear represents flag usage statistics for a year
type FlagUsageByYear struct {
	Year       int     `json:"year"`
	NodeCount  int     `json:"node_count"`
	TotalNodes int     `json:"total_nodes"`
	Percentage float64 `json:"percentage"`
}

// AnalyticsOperations handles analytics-related database operations
type AnalyticsOperations struct {
	db            database.DatabaseInterface
	queryBuilder  QueryBuilderInterface
	resultParser  ResultParserInterface
	mu            sync.RWMutex
}

// NewAnalyticsOperations creates a new AnalyticsOperations instance
func NewAnalyticsOperations(db database.DatabaseInterface, queryBuilder QueryBuilderInterface, resultParser ResultParserInterface) *AnalyticsOperations {
	return &AnalyticsOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// GetFlagFirstAppearance finds the first node that used a specific flag
func (ao *AnalyticsOperations) GetFlagFirstAppearance(flag string) (*FlagFirstAppearance, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	if flag == "" {
		return nil, fmt.Errorf("flag cannot be empty")
	}

	conn := ao.db.Conn()
	query := ao.queryBuilder.FlagFirstAppearanceSQL()
	
	row := conn.QueryRow(query, flag)
	
	var fa FlagFirstAppearance
	err := row.Scan(
		&fa.Zone,
		&fa.Net,
		&fa.Node,
		&fa.NodelistDate,
		&fa.SystemName,
		&fa.Location,
		&fa.SysopName,
	)
	
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil // No results found
		}
		return nil, fmt.Errorf("failed to scan flag first appearance: %w", err)
	}

	return &fa, nil
}

// GetFlagUsageByYear returns the usage statistics of a flag by year
func (ao *AnalyticsOperations) GetFlagUsageByYear(flag string) ([]FlagUsageByYear, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	if flag == "" {
		return nil, fmt.Errorf("flag cannot be empty")
	}

	conn := ao.db.Conn()
	query := ao.queryBuilder.FlagUsageByYearSQL()
	
	rows, err := conn.Query(query, flag)
	if err != nil {
		return nil, fmt.Errorf("failed to query flag usage by year: %w", err)
	}
	defer rows.Close()

	var results []FlagUsageByYear
	for rows.Next() {
		var fu FlagUsageByYear
		err := rows.Scan(&fu.Year, &fu.NodeCount, &fu.TotalNodes, &fu.Percentage)
		if err != nil {
			return nil, fmt.Errorf("failed to scan flag usage row: %w", err)
		}
		results = append(results, fu)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return results, nil
}

// Close closes the analytics operations
func (ao *AnalyticsOperations) Close() error {
	ao.mu.Lock()
	defer ao.mu.Unlock()
	// Nothing to close at this level
	return nil
}