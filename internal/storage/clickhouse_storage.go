package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"nodelistdb/internal/database"
)

// ClickHouseStorage provides ClickHouse-specific database operations
type ClickHouseStorage struct {
	db                database.DatabaseInterface
	nodeOps          *ClickHouseNodeOperations
	statsOps         *StatisticsOperations
	searchOps        *SearchOperations
	queryBuilder     *ClickHouseQueryBuilder
	resultParser     *ClickHouseResultParser
	mu               sync.RWMutex
}

// NewClickHouseStorage creates a new ClickHouse-specific storage instance
func NewClickHouseStorage(db database.DatabaseInterface) *ClickHouseStorage {
	queryBuilder := NewClickHouseQueryBuilder()
	resultParser := NewClickHouseResultParser()
	
	storage := &ClickHouseStorage{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
	
	// Create specialized operations with ClickHouse components
	storage.nodeOps = NewClickHouseNodeOperations(db, queryBuilder, resultParser)
	storage.statsOps = NewStatisticsOperations(db, queryBuilder, resultParser.ResultParser)
	// Create a regular NodeOperations adapter for SearchOperations
	nodeOpsAdapter := &NodeOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser.ResultParser,
	}
	storage.searchOps = NewSearchOperations(db, queryBuilder, resultParser.ResultParser, nodeOpsAdapter)
	
	return storage
}

// NodeOperations returns the ClickHouse-specific node operations
func (cs *ClickHouseStorage) NodeOperations() *ClickHouseNodeOperations {
	return cs.nodeOps
}

// StatisticsOperations returns the statistics operations
func (cs *ClickHouseStorage) StatisticsOperations() *StatisticsOperations {
	return cs.statsOps
}

// SearchOperations returns the search operations
func (cs *ClickHouseStorage) SearchOperations() *SearchOperations {
	return cs.searchOps
}

// ClickHouseNodeOperations handles ClickHouse-specific node operations
type ClickHouseNodeOperations struct {
	db           database.DatabaseInterface
	queryBuilder *ClickHouseQueryBuilder
	resultParser *ClickHouseResultParser
	mu           sync.RWMutex
}

// NewClickHouseNodeOperations creates a new ClickHouse-specific NodeOperations instance
func NewClickHouseNodeOperations(db database.DatabaseInterface, queryBuilder *ClickHouseQueryBuilder, resultParser *ClickHouseResultParser) *ClickHouseNodeOperations {
	return &ClickHouseNodeOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// InsertNodes inserts a batch of nodes using ClickHouse-optimized batch insertion
func (cno *ClickHouseNodeOperations) InsertNodes(nodes []database.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	cno.mu.Lock()
	defer cno.mu.Unlock()

	// Use SQL connection for better compatibility with array formatting
	return cno.insertNodesSQL(nodes)
}

// insertNodesNative uses ClickHouse native connection for optimal batch insertion
func (cno *ClickHouseNodeOperations) insertNodesNative(chDB *database.ClickHouseDB, nodes []database.Node) error {
	conn := chDB.NativeConn()
	ctx := context.Background()

	// Prepare batch insert
	batch, err := conn.PrepareBatch(ctx, `
		INSERT INTO nodes (
			zone, net, node, nodelist_date, day_number,
			system_name, location, sysop_name, phone, node_type, region, max_speed,
			is_cm, is_mo, has_binkp, has_telnet, is_down, is_hold, is_pvt, is_active,
			flags, modem_flags, internet_protocols, internet_hostnames, internet_ports, internet_emails,
			conflict_sequence, has_conflict, has_inet, internet_config, fts_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	// Add all nodes to batch
	for _, node := range nodes {
		if node.FtsId == "" {
			node.ComputeFtsId()
		}

		args := cno.resultParser.NodeToArgsClickHouse(node)
		if err := batch.Append(args...); err != nil {
			return fmt.Errorf("failed to append to batch: %w", err)
		}
	}

	// Execute batch
	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	return nil
}

// insertNodesSQL uses standard SQL interface with ClickHouse-specific query generation
func (cno *ClickHouseNodeOperations) insertNodesSQL(nodes []database.Node) error {
	conn := cno.db.Conn()

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

		if err := cno.insertChunkClickHouse(tx, chunk); err != nil {
			return fmt.Errorf("failed to insert chunk: %w", err)
		}
	}

	return tx.Commit()
}

// insertChunkClickHouse inserts a chunk of nodes using ClickHouse-specific SQL generation
func (cno *ClickHouseNodeOperations) insertChunkClickHouse(tx *sql.Tx, chunk []database.Node) error {
	if len(chunk) == 0 {
		return nil
	}

	// Use ClickHouse-specific direct SQL generation
	insertSQL := cno.queryBuilder.BuildDirectBatchInsertSQL(chunk, cno.resultParser.ResultParser)

	_, err := tx.Exec(insertSQL)
	if err != nil {
		return fmt.Errorf("failed to execute ClickHouse batch insert: %w", err)
	}

	return nil
}

// GetNodes retrieves nodes using ClickHouse-optimized queries
func (cno *ClickHouseNodeOperations) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	// Validate filter
	if err := cno.resultParser.ValidateNodeFilter(filter); err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}

	cno.mu.RLock()
	defer cno.mu.RUnlock()

	conn := cno.db.Conn()

	// Use ClickHouse-specific FTS query
	query, args, usedFTS := cno.queryBuilder.BuildClickHouseFTSQuery(filter)
	if !usedFTS {
		// Fallback to base query builder if no FTS was used
		query, args = cno.queryBuilder.BuildNodesQuery(filter)
	}

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	var nodes []database.Node
	for rows.Next() {
		node, err := cno.resultParser.ParseNodeRow(rows)
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
func (cno *ClickHouseNodeOperations) GetNodeHistory(zone, net, node int) ([]database.Node, error) {
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

	cno.mu.RLock()
	defer cno.mu.RUnlock()

	conn := cno.db.Conn()

	query := cno.queryBuilder.NodeHistorySQL()
	rows, err := conn.Query(query, zone, net, node)
	if err != nil {
		return nil, fmt.Errorf("failed to query node history: %w", err)
	}
	defer rows.Close()

	var nodes []database.Node
	for rows.Next() {
		nodeEntry, err := cno.resultParser.ParseNodeRow(rows)
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

// InsertSingleNode inserts a single node (convenience method)
func (cno *ClickHouseNodeOperations) InsertSingleNode(node database.Node) error {
	return cno.InsertNodes([]database.Node{node})
}

// CountNodes returns the total number of nodes using ClickHouse-optimized counting
func (cno *ClickHouseNodeOperations) CountNodes(date time.Time) (int, error) {
	cno.mu.RLock()
	defer cno.mu.RUnlock()

	conn := cno.db.Conn()

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