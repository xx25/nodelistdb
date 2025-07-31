package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"nodelistdb/internal/database"
)

// Storage provides thread-safe database operations using specialized components
type Storage struct {
	db               database.DatabaseInterface
	queryBuilder     QueryBuilderInterface
	resultParser     *ResultParser
	nodeOperations   *NodeOperations
	searchOperations *SearchOperations
	statsOperations  *StatisticsOperations

	// Bulk mode transaction state
	bulkTx         *sql.Tx
	bulkMode       bool
	bulkNodesCount int // Track total nodes processed in bulk mode
	mu             sync.RWMutex
}

// New creates a new Storage instance with all specialized components
func New(db database.DatabaseInterface) (*Storage, error) {
	var queryBuilder QueryBuilderInterface
	var resultParser *ResultParser

	// Check if this is a ClickHouse database and use specialized components
	if _, isClickHouse := db.(*database.ClickHouseDB); isClickHouse {
		chQueryBuilder := NewClickHouseQueryBuilder()
		chResultParser := NewClickHouseResultParser()
		queryBuilder = chQueryBuilder
		resultParser = chResultParser.ResultParser
	} else {
		// Default to DuckDB components
		queryBuilder = NewQueryBuilder()
		resultParser = NewResultParser()
	}

	// Create the storage instance
	storage := &Storage{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}

	// Create specialized operation components
	storage.nodeOperations = NewNodeOperations(db, queryBuilder, resultParser)
	storage.searchOperations = NewSearchOperations(db, queryBuilder, resultParser, storage.nodeOperations)
	storage.statsOperations = NewStatisticsOperations(db, queryBuilder, resultParser)

	return storage, nil
}

// Close closes all database connections and prepared statements
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// The individual components don't have close methods currently,
	// but we maintain this for backward compatibility
	return nil
}

// --- Node Operations (delegated to NodeOperations) ---

// InsertNodes inserts a batch of nodes using optimized batch processing
func (s *Storage) InsertNodes(nodes []database.Node) error {
	// If bulk mode is active, use the bulk transaction
	if s.bulkMode && s.bulkTx != nil {
		return s.insertNodesWithTransaction(s.bulkTx, nodes)
	}
	return s.nodeOperations.InsertNodes(nodes)
}

// insertNodesWithTransaction inserts nodes using a provided transaction
func (s *Storage) insertNodesWithTransaction(tx *sql.Tx, nodes []database.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	chunkSize := 10000 // Use smaller chunks for bulk mode to avoid memory issues
	totalChunks := (len(nodes) + chunkSize - 1) / chunkSize

	for i := 0; i < len(nodes); i += chunkSize {
		end := i + chunkSize
		if end > len(nodes) {
			end = len(nodes)
		}
		chunk := nodes[i:end]
		chunkNum := (i / chunkSize) + 1

		fmt.Printf("  Bulk inserting chunk %d/%d: %d nodes...\n", chunkNum, totalChunks, len(chunk))
		start := time.Now()

		// Use direct SQL generation for bulk imports
		insertSQL := s.queryBuilder.BuildDirectBatchInsertSQL(chunk, s.resultParser)
		if _, err := tx.Exec(insertSQL); err != nil {
			return fmt.Errorf("failed to insert chunk of %d nodes: %w", len(chunk), err)
		}

		duration := time.Since(start)
		speed := float64(len(chunk)) / duration.Seconds()
		s.bulkNodesCount += len(chunk)
		fmt.Printf("  ✓ Chunk %d inserted in %v (%.0f nodes/sec) - Total: %d nodes\n", chunkNum, duration, speed, s.bulkNodesCount)
	}

	return nil
}

// GetNodes retrieves nodes based on filter criteria
func (s *Storage) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	return s.nodeOperations.GetNodes(filter)
}

// GetNodeHistory retrieves all historical entries for a specific node
func (s *Storage) GetNodeHistory(zone, net, node int) ([]database.Node, error) {
	return s.nodeOperations.GetNodeHistory(zone, net, node)
}

// GetNodeDateRange returns the first and last date when a node was active
func (s *Storage) GetNodeDateRange(zone, net, node int) (firstDate, lastDate time.Time, err error) {
	return s.nodeOperations.GetNodeDateRange(zone, net, node)
}

// FindConflictingNode checks if a node already exists for the same date
func (s *Storage) FindConflictingNode(zone, net, node int, date time.Time) (bool, error) {
	return s.nodeOperations.FindConflictingNode(zone, net, node, date)
}

// IsNodelistProcessed checks if a nodelist has already been processed based on date
func (s *Storage) IsNodelistProcessed(nodelistDate time.Time) (bool, error) {
	return s.nodeOperations.IsNodelistProcessed(nodelistDate)
}

// GetMaxNodelistDate returns the most recent nodelist date in the database
func (s *Storage) GetMaxNodelistDate() (time.Time, error) {
	return s.nodeOperations.GetMaxNodelistDate()
}

// --- Search Operations (delegated to SearchOperations) ---

// SearchNodesBySysop finds all nodes associated with a sysop name
func (s *Storage) SearchNodesBySysop(sysopName string, limit int) ([]NodeSummary, error) {
	return s.searchOperations.SearchNodesBySysop(sysopName, limit)
}

// SearchNodesWithLifetime finds nodes based on filter criteria and returns them with lifetime information
func (s *Storage) SearchNodesWithLifetime(filter database.NodeFilter) ([]NodeSummary, error) {
	return s.searchOperations.SearchNodesWithLifetime(filter)
}

// GetUniqueSysops returns a list of unique sysops with their node counts
func (s *Storage) GetUniqueSysops(nameFilter string, limit, offset int) ([]SysopInfo, error) {
	return s.searchOperations.GetUniqueSysops(nameFilter, limit, offset)
}

// GetNodesBySysop returns all nodes for a specific sysop
func (s *Storage) GetNodesBySysop(sysopName string, limit int) ([]database.Node, error) {
	return s.searchOperations.GetNodesBySysop(sysopName, limit)
}

// GetNodeChanges analyzes the history of a node and returns detected changes
func (s *Storage) GetNodeChanges(zone, net, node int, filter ChangeFilter) ([]database.NodeChange, error) {
	return s.searchOperations.GetNodeChanges(zone, net, node, filter)
}

// --- Statistics Operations (delegated to StatisticsOperations) ---

// GetStats retrieves network statistics for a specific date
func (s *Storage) GetStats(date time.Time) (*database.NetworkStats, error) {
	return s.statsOperations.GetStats(date)
}

// GetLatestStatsDate retrieves the most recent date that has statistics
func (s *Storage) GetLatestStatsDate() (time.Time, error) {
	return s.statsOperations.GetLatestStatsDate()
}

// GetAvailableDates returns all unique dates that have nodelist data
func (s *Storage) GetAvailableDates() ([]time.Time, error) {
	return s.statsOperations.GetAvailableDates()
}

// GetNearestAvailableDate finds the closest available date to the requested date
func (s *Storage) GetNearestAvailableDate(requestedDate time.Time) (time.Time, error) {
	return s.statsOperations.GetNearestAvailableDate(requestedDate)
}

// --- Extended API Methods (new functionality) ---

// InsertSingleNode inserts a single node (convenience method)
func (s *Storage) InsertSingleNode(node database.Node) error {
	return s.nodeOperations.InsertSingleNode(node)
}

// NodeExists checks if a specific node exists in the database
func (s *Storage) NodeExists(zone, net, node int) (bool, error) {
	return s.nodeOperations.NodeExists(zone, net, node)
}

// GetLatestNodeVersion gets the most recent version of a specific node
func (s *Storage) GetLatestNodeVersion(zone, net, node int) (*database.Node, error) {
	return s.nodeOperations.GetLatestNodeVersion(zone, net, node)
}

// CountNodes returns the total number of nodes for a given date (or all if date is zero)
func (s *Storage) CountNodes(date time.Time) (int, error) {
	return s.nodeOperations.CountNodes(date)
}

// DeleteNodesForDate removes all nodes for a specific date (for re-import scenarios)
func (s *Storage) DeleteNodesForDate(date time.Time) error {
	return s.nodeOperations.DeleteNodesForDate(date)
}

// GetNodesByZone retrieves all nodes for a specific zone
func (s *Storage) GetNodesByZone(zone int, limit int) ([]database.Node, error) {
	return s.nodeOperations.GetNodesByZone(zone, limit)
}

// GetNodesByNet retrieves all nodes for a specific net within a zone
func (s *Storage) GetNodesByNet(zone, net int, limit int) ([]database.Node, error) {
	return s.nodeOperations.GetNodesByNet(zone, net, limit)
}

// SearchNodesBySystemName finds nodes by system name (case-insensitive partial match)
func (s *Storage) SearchNodesBySystemName(systemName string, limit int) ([]database.Node, error) {
	return s.searchOperations.SearchNodesBySystemName(systemName, limit)
}

// SearchNodesByLocation finds nodes by location (case-insensitive partial match)
func (s *Storage) SearchNodesByLocation(location string, limit int) ([]database.Node, error) {
	return s.searchOperations.SearchNodesByLocation(location, limit)
}

// SearchActiveNodes finds currently active nodes with optional filters
func (s *Storage) SearchActiveNodes(filter database.NodeFilter) ([]database.Node, error) {
	return s.searchOperations.SearchActiveNodes(filter)
}

// SearchNodesWithProtocol finds nodes supporting a specific internet protocol
func (s *Storage) SearchNodesWithProtocol(protocol string, limit int) ([]database.Node, error) {
	return s.searchOperations.SearchNodesWithProtocol(protocol, limit)
}

// GetDateRangeStats returns statistics for a range of dates
func (s *Storage) GetDateRangeStats(startDate, endDate time.Time) ([]database.NetworkStats, error) {
	return s.statsOperations.GetDateRangeStats(startDate, endDate)
}

// GetZoneStats returns statistics for a specific zone across all dates
func (s *Storage) GetZoneStats(zone int) (map[time.Time]int, error) {
	return s.statsOperations.GetZoneStats(zone)
}

// GetNodeTypeDistribution returns the distribution of node types for a given date
func (s *Storage) GetNodeTypeDistribution(date time.Time) (map[string]int, error) {
	return s.statsOperations.GetNodeTypeDistribution(date)
}

// GetConnectivityStats returns connectivity statistics for a given date
func (s *Storage) GetConnectivityStats(date time.Time) (*ConnectivityStats, error) {
	return s.statsOperations.GetConnectivityStats(date)
}

// GetTopSysops returns the sysops managing the most nodes for a given date
func (s *Storage) GetTopSysops(date time.Time, limit int) ([]SysopStats, error) {
	return s.statsOperations.GetTopSysops(date, limit)
}

// GetGrowthStats calculates growth statistics between two dates
func (s *Storage) GetGrowthStats(startDate, endDate time.Time) (*GrowthStats, error) {
	return s.statsOperations.GetGrowthStats(startDate, endDate)
}

// --- Direct Component Access (for advanced usage) ---

// GetNodeOperations returns the node operations component for direct access
func (s *Storage) GetNodeOperations() *NodeOperations {
	return s.nodeOperations
}

// GetSearchOperations returns the search operations component for direct access
func (s *Storage) GetSearchOperations() *SearchOperations {
	return s.searchOperations
}

// --- Bulk Mode Operations ---

// BeginBulkMode starts a bulk transaction mode for improved performance
// This disables autocommit and uses a single transaction for all operations
func (s *Storage) BeginBulkMode() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.bulkMode {
		return fmt.Errorf("bulk mode already active")
	}

	conn := s.db.Conn()
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin bulk transaction: %w", err)
	}

	s.bulkTx = tx
	s.bulkMode = true
	s.bulkNodesCount = 0 // Reset counter

	// Apply DuckDB-specific performance settings if it's DuckDB
	if _, isDuckDB := s.db.(*database.DB); isDuckDB {
		// Try to set performance optimizations, but don't fail if they don't work
		tx.Exec("PRAGMA wal_autocheckpoint=0")
		tx.Exec("PRAGMA enable_object_cache")
	}

	return nil
}

// EndBulkMode commits the bulk transaction and returns to normal mode
func (s *Storage) EndBulkMode() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.bulkMode {
		return fmt.Errorf("bulk mode not active")
	}

	var err error
	if s.bulkTx != nil {
		err = s.bulkTx.Commit()
		s.bulkTx = nil
	}

	s.bulkMode = false

	// Report final bulk mode statistics
	if s.bulkNodesCount > 0 {
		fmt.Printf("Bulk mode completed: %d total nodes processed\n", s.bulkNodesCount)
	}

	// Re-enable WAL checkpointing if it's DuckDB
	if _, isDuckDB := s.db.(*database.DB); isDuckDB {
		conn := s.db.Conn()
		if _, wErr := conn.Exec("PRAGMA wal_autocheckpoint=1000"); wErr != nil {
			// Log but don't fail - this is non-critical
		}
	}

	if err != nil {
		return fmt.Errorf("failed to commit bulk transaction: %w", err)
	}

	return nil
}

// IsBulkMode returns whether bulk mode is currently active
func (s *Storage) IsBulkMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bulkMode
}

// GetStatisticsOperations returns the statistics operations component for direct access
func (s *Storage) GetStatisticsOperations() *StatisticsOperations {
	return s.statsOperations
}

// GetQueryBuilder returns the query builder for direct access
func (s *Storage) GetQueryBuilder() QueryBuilderInterface {
	return s.queryBuilder
}

// GetResultParser returns the result parser for direct access
func (s *Storage) GetResultParser() *ResultParser {
	return s.resultParser
}

// --- Health and Monitoring ---

// HealthCheck performs a basic health check on all storage components
func (s *Storage) HealthCheck() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Test database connection
	if err := s.db.Ping(); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	// Test basic query functionality
	_, err := s.statsOperations.GetLatestStatsDate()
	if err != nil {
		// This might fail if no data exists, which is okay
		// Only fail if it's a connection or query syntax error
		if !strings.Contains(err.Error(), "no rows") {
			return fmt.Errorf("query execution failed: %w", err)
		}
	}

	return nil
}

// GetComponentInfo returns information about the storage components
func (s *Storage) GetComponentInfo() map[string]interface{} {
	return map[string]interface{}{
		"version":             "2.0.0-refactored",
		"architecture":        "component-based",
		"query_builder":       "safe parameterized queries",
		"result_parser":       "type-safe parsing",
		"node_operations":     "CRUD operations with validation",
		"search_operations":   "advanced search and change detection",
		"stats_operations":    "comprehensive statistics",
		"thread_safety":       "mutex-protected operations",
		"backward_compatible": true,
	}
}

// --- Migration Helper (for transitioning from old storage.go) ---

// MigrateFromLegacyStorage is a helper method for transitioning from the old storage implementation
// This method is intended to be used during the migration period and can be removed later
func (s *Storage) MigrateFromLegacyStorage() error {
	// This method can be used to perform any necessary data migrations
	// or validation checks when upgrading from the old storage implementation

	// For now, just perform a health check
	return s.HealthCheck()
}
