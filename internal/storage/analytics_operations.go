package storage

import (
	"fmt"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
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

// NetworkAppearance represents a continuous period when a network was active
type NetworkAppearance struct {
	StartDate    time.Time `json:"start_date"`
	EndDate      time.Time `json:"end_date"`
	StartDayNum  int       `json:"start_day_num"`
	EndDayNum    int       `json:"end_day_num"`
	DurationDays int       `json:"duration_days"`
	NodelistCount int      `json:"nodelist_count"`
}

// NetworkHistory represents the complete history of a network
type NetworkHistory struct {
	Zone         int                 `json:"zone"`
	Net          int                 `json:"net"`
	NetworkName  string              `json:"network_name"`
	FirstSeen    time.Time           `json:"first_seen"`
	LastSeen     time.Time           `json:"last_seen"`
	TotalDays    int                 `json:"total_days"`
	ActiveDays   int                 `json:"active_days"`
	Appearances  []NetworkAppearance `json:"appearances"`
}

// AnalyticsOperations handles analytics-related database operations
type AnalyticsOperations struct {
	db           database.DatabaseInterface
	queryBuilder QueryBuilderInterface
	resultParser ResultParserInterface
	mu           sync.RWMutex
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

	row := conn.QueryRow(query, flag, flag, flag)

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

	rows, err := conn.Query(query, flag, flag, flag, flag, flag, flag)
	if err != nil {
		return nil, fmt.Errorf("failed to query flag usage by year: %w", err)
	}
	defer rows.Close()

	var results []FlagUsageByYear
	for rows.Next() {
		var fu FlagUsageByYear
		err := rows.Scan(&fu.Year, &fu.TotalNodes, &fu.NodeCount, &fu.Percentage)
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

// GetNetworkHistory returns the complete appearance history of a network
func (ao *AnalyticsOperations) GetNetworkHistory(zone, net int) (*NetworkHistory, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	conn := ao.db.Conn()

	// First, get the network name from node 0 if it exists
	nameQuery := ao.queryBuilder.NetworkNameSQL()
	var networkName string
	err := conn.QueryRow(nameQuery, zone, net).Scan(&networkName)
	if err != nil {
		// Network might not have a coordinator, use default name
		networkName = fmt.Sprintf("Network %d:%d", zone, net)
	}

	// Get all appearances of the network
	historyQuery := ao.queryBuilder.NetworkHistorySQL()
	rows, err := conn.Query(historyQuery, zone, net)
	if err != nil {
		return nil, fmt.Errorf("failed to query network history: %w", err)
	}
	defer rows.Close()

	var appearances []NetworkAppearance
	var firstSeen, lastSeen time.Time
	var totalNodelistCount int

	for rows.Next() {
		var app NetworkAppearance
		err := rows.Scan(
			&app.StartDate,
			&app.EndDate,
			&app.StartDayNum,
			&app.EndDayNum,
			&app.NodelistCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan network appearance: %w", err)
		}

		app.DurationDays = int(app.EndDate.Sub(app.StartDate).Hours()/24) + 1
		appearances = append(appearances, app)
		totalNodelistCount += app.NodelistCount

		if firstSeen.IsZero() || app.StartDate.Before(firstSeen) {
			firstSeen = app.StartDate
		}
		if app.EndDate.After(lastSeen) {
			lastSeen = app.EndDate
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	if len(appearances) == 0 {
		return nil, nil // Network not found
	}

	totalDays := int(lastSeen.Sub(firstSeen).Hours()/24) + 1

	return &NetworkHistory{
		Zone:        zone,
		Net:         net,
		NetworkName: networkName,
		FirstSeen:   firstSeen,
		LastSeen:    lastSeen,
		TotalDays:   totalDays,
		ActiveDays:  totalNodelistCount,
		Appearances: appearances,
	}, nil
}

// Close closes the analytics operations
func (ao *AnalyticsOperations) Close() error {
	ao.mu.Lock()
	defer ao.mu.Unlock()
	// Nothing to close at this level
	return nil
}
