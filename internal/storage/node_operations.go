package storage

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
)

// NodeOperations handles all node-related database operations
type NodeOperations struct {
	db           database.DatabaseInterface
	queryBuilder QueryBuilderInterface
	resultParser ResultParserInterface
	mu           sync.RWMutex
}

// NewNodeOperations creates a new NodeOperations instance
func NewNodeOperations(db database.DatabaseInterface, queryBuilder QueryBuilderInterface, resultParser ResultParserInterface) *NodeOperations {
	return &NodeOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// InsertNodes inserts a batch of nodes using optimized chunked inserts with smaller memory footprint
func (no *NodeOperations) InsertNodes(nodes []database.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	no.mu.Lock()
	defer no.mu.Unlock()

	// Use optimized chunked inserts that avoid large memory allocations
	return no.queryBuilder.InsertNodesInChunks(no.db, nodes)
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

	// DEBUG: Log the query being executed if debug mode is enabled
	if os.Getenv("DEBUG_SQL") == "true" {
		fmt.Printf("\n[DEBUG SQL] Query (NodeOperations):\n%s\n", query)
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

// GetNodeHistory retrieves all historical entries for a specific node.
// An empty domain matches all networks.
func (no *NodeOperations) GetNodeHistory(zone, net, node int, domain string) ([]database.Node, error) {
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
	rows, err := conn.Query(query, zone, net, node, domain, domain)
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

// GetNodeDateRange returns the first and last date when a node was active.
// An empty domain matches all networks.
func (no *NodeOperations) GetNodeDateRange(zone, net, node int, domain string) (firstDate, lastDate time.Time, err error) {
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
	row := conn.QueryRow(query, zone, net, node, domain, domain)

	err = row.Scan(&firstDate, &lastDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, time.Time{}, fmt.Errorf("node %d:%d/%d not found", zone, net, node)
		}
		return time.Time{}, time.Time{}, fmt.Errorf("failed to get node date range: %w", err)
	}

	return firstDate, lastDate, nil
}

// GetNodeDomains lists the FTN networks a 3D address exists in
func (no *NodeOperations) GetNodeDomains(zone, net, node int) ([]string, error) {
	no.mu.RLock()
	defer no.mu.RUnlock()

	conn := no.db.Conn()
	rows, err := conn.Query(`SELECT DISTINCT domain FROM nodes WHERE zone = ? AND net = ? AND node = ? ORDER BY domain`, zone, net, node)
	if err != nil {
		return nil, fmt.Errorf("failed to query node domains: %w", err)
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("failed to scan node domain: %w", err)
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

// FindConflictingNode checks if a node already exists for the same date within one network
func (no *NodeOperations) FindConflictingNode(zone, net, node int, date time.Time, domain string) (bool, error) {
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
	defer func() { _ = tx.Rollback() }()

	if domain == "" {
		domain = database.DefaultDomain
	}

	var count int
	query := no.queryBuilder.ConflictCheckSQL()
	queryErr := tx.QueryRow(query, zone, net, node, date, domain).Scan(&count)

	if queryErr != nil {
		if queryErr == sql.ErrNoRows {
			return false, nil // No conflict found in committed data
		}
		return false, fmt.Errorf("failed to find conflicting node: %w", queryErr)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return count > 0, nil
}

// IsNodelistProcessed checks if a nodelist has already been processed based on
// date, within one network. Different networks may publish nodelists for the
// same date, so the check must never span domains.
func (no *NodeOperations) IsNodelistProcessed(nodelistDate time.Time, domain string) (bool, error) {
	no.mu.RLock()
	defer no.mu.RUnlock()

	if domain == "" {
		domain = database.DefaultDomain
	}

	conn := no.db.Conn()

	var count int
	query := no.queryBuilder.IsProcessedSQL()
	err := conn.QueryRow(query, domain, nodelistDate).Scan(&count)

	if err != nil {
		return false, fmt.Errorf("failed to check if nodelist is processed: %w", err)
	}

	return count > 0, nil
}

// GetMaxNodelistDate returns the most recent nodelist date in the database.
// An empty domain returns the newest date across all networks.
func (no *NodeOperations) GetMaxNodelistDate(domain string) (time.Time, error) {
	no.mu.RLock()
	defer no.mu.RUnlock()

	conn := no.db.Conn()

	var maxDate time.Time
	query := no.queryBuilder.LatestDateSQL()
	err := conn.QueryRow(query, domain, domain).Scan(&maxDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get max nodelist date: %w", err)
	}

	return maxDate, nil
}

// GetDomains lists all FTN networks present in the database with their latest
// nodelist date and node count on that date.
func (no *NodeOperations) GetDomains() ([]DomainInfo, error) {
	no.mu.RLock()
	defer no.mu.RUnlock()

	conn := no.db.Conn()
	rows, err := conn.Query(`
		SELECT n.domain, l.latest AS latest_date, count(*) AS node_count
		FROM nodes n
		INNER JOIN (
			SELECT domain, max(nodelist_date) AS latest FROM nodes GROUP BY domain
		) l ON n.domain = l.domain AND n.nodelist_date = l.latest
		GROUP BY n.domain, l.latest
		ORDER BY n.domain`)
	if err != nil {
		return nil, fmt.Errorf("failed to query domains: %w", err)
	}
	defer rows.Close()

	var result []DomainInfo
	for rows.Next() {
		var di DomainInfo
		if err := rows.Scan(&di.Domain, &di.LatestDate, &di.NodeCount); err != nil {
			return nil, fmt.Errorf("failed to scan domain info: %w", err)
		}
		result = append(result, di)
	}
	return result, rows.Err()
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

// CountNodes returns the total number of nodes for a given date (or all if date
// is zero). An empty domain counts across all networks.
func (no *NodeOperations) CountNodes(date time.Time, domain string) (int, error) {
	no.mu.RLock()
	defer no.mu.RUnlock()

	conn := no.db.Conn()

	var query string
	var args []interface{}

	if date.IsZero() {
		query = "SELECT COUNT(*) FROM nodes WHERE " + domainFilterSQL
		args = []interface{}{domain, domain}
	} else {
		query = "SELECT COUNT(*) FROM nodes WHERE nodelist_date = ? AND " + domainFilterSQL
		args = []interface{}{date, domain, domain}
	}

	var count int
	err := conn.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count nodes: %w", err)
	}

	return count, nil
}

// DeleteNodesForDate removes all nodes for a specific date within one network
// (for re-import scenarios)
func (no *NodeOperations) DeleteNodesForDate(date time.Time, domain string) error {
	no.mu.Lock()
	defer no.mu.Unlock()

	if domain == "" {
		domain = database.DefaultDomain
	}

	conn := no.db.Conn()

	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := "DELETE FROM nodes WHERE nodelist_date = ? AND domain = ?"
	_, err = tx.Exec(query, date, domain)
	if err != nil {
		return fmt.Errorf("failed to delete nodes for date %v: %w", date, err)
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
