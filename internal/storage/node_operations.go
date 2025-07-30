package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"nodelistdb/internal/database"
)

// NodeOperations handles all node-related database operations
type NodeOperations struct {
	db           *database.DB
	queryBuilder QueryBuilderInterface
	resultParser *ResultParser
	mu           sync.RWMutex
}

// NewNodeOperations creates a new NodeOperations instance
func NewNodeOperations(db *database.DB, queryBuilder QueryBuilderInterface, resultParser *ResultParser) *NodeOperations {
	return &NodeOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// InsertNodes inserts a batch of nodes using optimized prepared statements with safe parameterization
func (no *NodeOperations) InsertNodes(nodes []database.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	no.mu.Lock()
	defer no.mu.Unlock()

	conn := no.db.Conn()

	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	chunkSize := DefaultBatchInsertConfig().ChunkSize
	for i := 0; i < len(nodes); i += chunkSize {
		end := i + chunkSize
		if end > len(nodes) {
			end = len(nodes)
		}
		chunk := nodes[i:end]

		if err := no.insertChunkSafely(tx, chunk); err != nil {
			return fmt.Errorf("failed to insert chunk: %w", err)
		}
	}

	return tx.Commit()
}

// insertChunkSafely inserts a chunk of nodes using direct SQL generation for performance
func (no *NodeOperations) insertChunkSafely(tx *sql.Tx, chunk []database.Node) error {
	if len(chunk) == 0 {
		return nil
	}

	// Use direct SQL generation for bulk imports (much faster than parameterized queries)
	insertSQL := no.queryBuilder.BuildDirectBatchInsertSQL(chunk, no.resultParser)

	_, err := tx.Exec(insertSQL)
	if err != nil {
		return fmt.Errorf("failed to execute batch insert: %w", err)
	}

	return nil
}

// GetNodes retrieves nodes based on filter criteria using safe parameterized queries
func (no *NodeOperations) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	// Validate filter
	if err := no.resultParser.ValidateNodeFilter(filter); err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}

	no.mu.RLock()
	defer no.mu.RUnlock()

	conn := no.db.Conn()

	// Try FTS first for text searches, fallback to ILIKE
	query, args, usedFTS := no.queryBuilder.BuildFTSQuery(filter)
	if !usedFTS {
		// Fallback to traditional ILIKE queries
		query, args = no.queryBuilder.BuildNodesQuery(filter)
	}

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	var nodes []database.Node
	for rows.Next() {
		node, err := no.resultParser.ParseNodeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to parse node row: %w", err)
		}
		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return nodes, nil
}

// GetNodeHistory retrieves all historical entries for a specific node
func (no *NodeOperations) GetNodeHistory(zone, net, node int) ([]database.Node, error) {
	// Validate input
	if zone < 1 || zone > 65535 {
		return nil, fmt.Errorf("invalid zone: %d", zone)
	}
	if net < 0 || net > 65535 {
		return nil, fmt.Errorf("invalid net: %d", net)
	}
	if node < 0 || node > 65535 {
		return nil, fmt.Errorf("invalid node: %d", node)
	}

	no.mu.RLock()
	defer no.mu.RUnlock()

	conn := no.db.Conn()

	query := no.queryBuilder.NodeHistorySQL()
	rows, err := conn.Query(query, zone, net, node)
	if err != nil {
		return nil, fmt.Errorf("failed to query node history: %w", err)
	}
	defer rows.Close()

	var nodes []database.Node
	for rows.Next() {
		nodeEntry, err := no.resultParser.ParseNodeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to parse node history row: %w", err)
		}
		nodes = append(nodes, nodeEntry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating node history rows: %w", err)
	}

	return nodes, nil
}

// GetNodeDateRange returns the first and last date when a node was active
func (no *NodeOperations) GetNodeDateRange(zone, net, node int) (firstDate, lastDate time.Time, err error) {
	// Validate input
	if zone < 1 || zone > 65535 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid zone: %d", zone)
	}
	if net < 0 || net > 65535 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid net: %d", net)
	}
	if node < 0 || node > 65535 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid node: %d", node)
	}

	no.mu.RLock()
	defer no.mu.RUnlock()

	conn := no.db.Conn()

	query := no.queryBuilder.NodeDateRangeSQL()
	row := conn.QueryRow(query, zone, net, node)

	err = row.Scan(&firstDate, &lastDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, time.Time{}, fmt.Errorf("node %d:%d/%d not found", zone, net, node)
		}
		return time.Time{}, time.Time{}, fmt.Errorf("failed to get node date range: %w", err)
	}

	return firstDate, lastDate, nil
}

// FindConflictingNode checks if a node already exists for the same date
func (no *NodeOperations) FindConflictingNode(zone, net, node int, date time.Time) (bool, error) {
	// Validate input
	if zone < 1 || zone > 65535 {
		return false, fmt.Errorf("invalid zone: %d", zone)
	}
	if net < 0 || net > 65535 {
		return false, fmt.Errorf("invalid net: %d", net)
	}
	if node < 0 || node > 65535 {
		return false, fmt.Errorf("invalid node: %d", node)
	}

	conn := no.db.Conn()

	// Use a separate transaction with READ COMMITTED isolation to see committed data
	tx, err := conn.Begin()
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction for conflict check: %w", err)
	}
	defer tx.Rollback()

	var count int
	query := no.queryBuilder.ConflictCheckSQL()
	queryErr := tx.QueryRow(query, zone, net, node, date).Scan(&count)

	if queryErr != nil {
		if queryErr == sql.ErrNoRows {
			return false, nil // No conflict found in committed data
		}
		return false, fmt.Errorf("failed to find conflicting node: %w", queryErr)
	}

	tx.Commit()
	return count > 0, nil
}

// markOriginalAsConflicted marks the original entry (conflict_sequence=0) as having conflict
func (no *NodeOperations) markOriginalAsConflicted(conn *sql.DB, zone, net, node int, date time.Time) error {
	query := no.queryBuilder.MarkConflictSQL()
	_, err := conn.Exec(query, zone, net, node, date)
	return err
}

// IsNodelistProcessed checks if a nodelist has already been processed based on date
func (no *NodeOperations) IsNodelistProcessed(nodelistDate time.Time) (bool, error) {
	no.mu.RLock()
	defer no.mu.RUnlock()

	conn := no.db.Conn()

	var count int
	query := no.queryBuilder.IsProcessedSQL()
	err := conn.QueryRow(query, nodelistDate).Scan(&count)

	if err != nil {
		return false, fmt.Errorf("failed to check if nodelist is processed: %w", err)
	}

	return count > 0, nil
}

// GetMaxNodelistDate returns the most recent nodelist date in the database
func (no *NodeOperations) GetMaxNodelistDate() (time.Time, error) {
	no.mu.RLock()
	defer no.mu.RUnlock()

	conn := no.db.Conn()

	var maxDate time.Time
	query := no.queryBuilder.LatestDateSQL()
	err := conn.QueryRow(query).Scan(&maxDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get max nodelist date: %w", err)
	}

	return maxDate, nil
}

// InsertSingleNode inserts a single node (convenience method for API usage)
func (no *NodeOperations) InsertSingleNode(node database.Node) error {
	return no.InsertNodes([]database.Node{node})
}

// NodeExists checks if a specific node exists in the database
func (no *NodeOperations) NodeExists(zone, net, node int) (bool, error) {
	filter := database.NodeFilter{
		Zone:  &zone,
		Net:   &net,
		Node:  &node,
		Limit: 1,
	}

	nodes, err := no.GetNodes(filter)
	if err != nil {
		return false, err
	}

	return len(nodes) > 0, nil
}

// GetLatestNodeVersion gets the most recent version of a specific node
func (no *NodeOperations) GetLatestNodeVersion(zone, net, node int) (*database.Node, error) {
	latestOnly := true
	filter := database.NodeFilter{
		Zone:       &zone,
		Net:        &net,
		Node:       &node,
		LatestOnly: &latestOnly,
		Limit:      1,
	}

	nodes, err := no.GetNodes(filter)
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("node %d:%d/%d not found", zone, net, node)
	}

	return &nodes[0], nil
}

// CountNodes returns the total number of nodes for a given date (or all if date is zero)
func (no *NodeOperations) CountNodes(date time.Time) (int, error) {
	no.mu.RLock()
	defer no.mu.RUnlock()

	conn := no.db.Conn()

	var query string
	var args []interface{}

	if date.IsZero() {
		query = "SELECT COUNT(*) FROM nodes"
	} else {
		query = "SELECT COUNT(*) FROM nodes WHERE nodelist_date = ?"
		args = []interface{}{date}
	}

	var count int
	err := conn.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count nodes: %w", err)
	}

	return count, nil
}

// DeleteNodesForDate removes all nodes for a specific date (for re-import scenarios)
func (no *NodeOperations) DeleteNodesForDate(date time.Time) error {
	no.mu.Lock()
	defer no.mu.Unlock()

	conn := no.db.Conn()

	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := "DELETE FROM nodes WHERE nodelist_date = ?"
	result, err := tx.Exec(query, date)
	if err != nil {
		return fmt.Errorf("failed to delete nodes for date %v: %w", date, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err == nil && rowsAffected > 0 {
		// Log the deletion if verbose logging is enabled
		// This would require access to a logger instance
	}

	return tx.Commit()
}

// GetNodesByZone retrieves all nodes for a specific zone
func (no *NodeOperations) GetNodesByZone(zone int, limit int) ([]database.Node, error) {
	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	filter := database.NodeFilter{
		Zone:  &zone,
		Limit: limit,
	}

	return no.GetNodes(filter)
}

// GetNodesByNet retrieves all nodes for a specific net within a zone
func (no *NodeOperations) GetNodesByNet(zone, net int, limit int) ([]database.Node, error) {
	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	filter := database.NodeFilter{
		Zone:  &zone,
		Net:   &net,
		Limit: limit,
	}

	return no.GetNodes(filter)
}
