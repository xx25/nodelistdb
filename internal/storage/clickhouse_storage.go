package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/nodelistdb/internal/database"
)

// ClickHouseStorage provides ClickHouse-specific database operations
type ClickHouseStorage struct {
	db           database.DatabaseInterface
	nodeOps      *ClickHouseNodeOperations
	statsOps     *StatisticsOperations
	searchOps    *SearchOperations
	queryBuilder *QueryBuilder
	resultParser *ClickHouseResultParser
}

// NewClickHouseStorage creates a new ClickHouse-specific storage instance
func NewClickHouseStorage(db database.DatabaseInterface) *ClickHouseStorage {
	queryBuilder := NewQueryBuilder()
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
	queryBuilder *QueryBuilder
	resultParser *ClickHouseResultParser
	mu           sync.RWMutex
}

// NewClickHouseNodeOperations creates a new ClickHouse-specific NodeOperations instance
func NewClickHouseNodeOperations(db database.DatabaseInterface, queryBuilder *QueryBuilder, resultParser *ClickHouseResultParser) *ClickHouseNodeOperations {
	return &ClickHouseNodeOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// nativeConnProvider is an interface for database implementations that provide native connections
type nativeConnProvider interface {
	NativeConn() driver.Conn
}

// InsertNodes inserts a batch of nodes using ClickHouse's native batch API.
// This uses parameterized inserts via PrepareBatch for safety and performance.
func (cno *ClickHouseNodeOperations) InsertNodes(nodes []database.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	cno.mu.Lock()
	defer cno.mu.Unlock()

	// Try to get native connection for optimal batch insert
	if provider, ok := cno.db.(nativeConnProvider); ok {
		return cno.insertNodesNative(provider.NativeConn(), nodes)
	}

	// Fallback to SQL-based insert (should not happen with ClickHouse)
	return cno.insertNodesSQL(nodes)
}

// insertNodesNative uses ClickHouse's native batch API for safe, parameterized inserts.
// This is the preferred method as it avoids SQL string building entirely.
func (cno *ClickHouseNodeOperations) insertNodesNative(conn driver.Conn, nodes []database.Node) error {
	ctx := context.Background()

	// Process in chunks to avoid memory issues with very large batches
	chunkSize := DefaultBatchInsertConfig().ChunkSize
	for i := 0; i < len(nodes); i += chunkSize {
		end := i + chunkSize
		if end > len(nodes) {
			end = len(nodes)
		}
		chunk := nodes[i:end]

		if err := cno.insertChunkNative(ctx, conn, chunk); err != nil {
			return fmt.Errorf("failed to insert chunk %d-%d: %w", i, end-1, err)
		}
	}

	return nil
}

// insertChunkNative inserts a chunk using ClickHouse's PrepareBatch API.
// Values are passed as parameters, eliminating SQL injection risks.
func (cno *ClickHouseNodeOperations) insertChunkNative(ctx context.Context, conn driver.Conn, chunk []database.Node) error {
	if len(chunk) == 0 {
		return nil
	}

	batch, err := conn.PrepareBatch(ctx, `INSERT INTO nodes (
		zone, net, node, nodelist_date, day_number,
		system_name, location, sysop_name, phone, node_type, region, max_speed,
		is_cm, is_mo,
		flags, modem_flags,
		conflict_sequence, has_conflict, has_inet, internet_config, fts_id, raw_line
	)`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	for _, node := range chunk {
		// Compute FTS ID if not set
		if node.FtsId == "" {
			node.ComputeFtsId()
		}

		// Convert InternetConfig to string for insertion
		internetConfigStr := "{}"
		if len(node.InternetConfig) > 0 {
			internetConfigStr = string(node.InternetConfig)
		}

		// Append row with properly typed values - no SQL escaping needed
		err := batch.Append(
			node.Zone,
			node.Net,
			node.Node,
			node.NodelistDate,
			node.DayNumber,
			node.SystemName,
			node.Location,
			node.SysopName,
			node.Phone,
			node.NodeType,
			node.Region, // Nullable - driver handles nil correctly
			node.MaxSpeed,
			node.IsCM,
			node.IsMO,
			node.Flags,      // []string - driver handles arrays natively
			node.ModemFlags, // []string - driver handles arrays natively
			node.ConflictSequence,
			node.HasConflict,
			node.HasInet,
			internetConfigStr,
			node.FtsId,
			node.RawLine,
		)
		if err != nil {
			return fmt.Errorf("failed to append row: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	return nil
}

// insertNodesSQL is a fallback using standard SQL interface (legacy, kept for compatibility)
func (cno *ClickHouseNodeOperations) insertNodesSQL(nodes []database.Node) error {
	conn := cno.db.Conn()

	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	chunkSize := DefaultBatchInsertConfig().ChunkSize
	for i := 0; i < len(nodes); i += chunkSize {
		end := i + chunkSize
		if end > len(nodes) {
			end = len(nodes)
		}
		chunk := nodes[i:end]

		if err := cno.insertChunkSQL(tx, chunk); err != nil {
			return fmt.Errorf("failed to insert chunk: %w", err)
		}
	}

	return tx.Commit()
}

// insertChunkSQL inserts using parameterized SQL statements (fallback method)
func (cno *ClickHouseNodeOperations) insertChunkSQL(tx *sql.Tx, chunk []database.Node) error {
	if len(chunk) == 0 {
		return nil
	}

	// Use parameterized insert for each node
	stmt, err := tx.Prepare(cno.queryBuilder.InsertNodeSQL())
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, node := range chunk {
		if node.FtsId == "" {
			node.ComputeFtsId()
		}

		internetConfigStr := "{}"
		if len(node.InternetConfig) > 0 {
			internetConfigStr = string(node.InternetConfig)
		}

		_, err := stmt.Exec(
			node.Zone, node.Net, node.Node, node.NodelistDate, node.DayNumber,
			node.SystemName, node.Location, node.SysopName, node.Phone, node.NodeType,
			node.Region, node.MaxSpeed, node.IsCM, node.IsMO,
			node.Flags, node.ModemFlags,
			node.ConflictSequence, node.HasConflict, node.HasInet,
			internetConfigStr, node.FtsId, node.RawLine,
		)
		if err != nil {
			return fmt.Errorf("failed to execute insert: %w", err)
		}
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
	query, args, usedFTS := cno.queryBuilder.BuildFTSQuery(filter)
	if !usedFTS {
		// Fallback to base query builder if no FTS was used
		query, args = cno.queryBuilder.BuildNodesQuery(filter)
	}

	// DEBUG: Log the query being executed if debug mode is enabled
	if os.Getenv("DEBUG_SQL") == "true" {
		fmt.Printf("\n[DEBUG SQL] ClickHouse Query:\n%s\n", query)
		fmt.Printf("[DEBUG SQL] Args: %v\n", args)
		fmt.Printf("[DEBUG SQL] Filter: %+v\n", filter)
		if filter.LatestOnly != nil {
			fmt.Printf("[DEBUG SQL] LatestOnly = %v\n", *filter.LatestOnly)
		}
		fmt.Printf("[DEBUG SQL] UsedFTS = %v\n\n", usedFTS)
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
