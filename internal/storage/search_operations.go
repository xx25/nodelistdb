package storage

import (
	"fmt"
	"strings"
	"sync"

	"github.com/nodelistdb/internal/database"
)

// SearchOperations handles all search-related database operations
type SearchOperations struct {
	db           database.DatabaseInterface
	queryBuilder QueryBuilderInterface
	resultParser ResultParserInterface
	nodeOps      *NodeOperations // Reference for getting node history
	mu           sync.RWMutex
}

// NewSearchOperations creates a new SearchOperations instance
func NewSearchOperations(db database.DatabaseInterface, queryBuilder QueryBuilderInterface, resultParser ResultParserInterface, nodeOps *NodeOperations) *SearchOperations {
	return &SearchOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
		nodeOps:      nodeOps,
	}
}

// SearchNodesBySystemName finds nodes by system name (case-insensitive partial match)
func (so *SearchOperations) SearchNodesBySystemName(systemName string, limit int) ([]database.Node, error) {
	if systemName == "" {
		return nil, fmt.Errorf("system name cannot be empty")
	}

	systemName = so.resultParser.SanitizeStringInput(systemName)

	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	filter := database.NodeFilter{
		SystemName: &systemName,
		Limit:      limit,
	}

	return so.nodeOps.GetNodes(filter)
}

// SearchNodesByLocation finds nodes by location (case-insensitive partial match)
func (so *SearchOperations) SearchNodesByLocation(location string, limit int) ([]database.Node, error) {
	if location == "" {
		return nil, fmt.Errorf("location cannot be empty")
	}

	location = so.resultParser.SanitizeStringInput(location)

	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	filter := database.NodeFilter{
		Location: &location,
		Limit:    limit,
	}

	return so.nodeOps.GetNodes(filter)
}

// SearchActiveNodes finds currently active nodes with optional filters
func (so *SearchOperations) SearchActiveNodes(filter database.NodeFilter) ([]database.Node, error) {
	// Force active filter by using latest_only
	latest := true
	filter.LatestOnly = &latest

	// Set default limit if not specified
	if filter.Limit == 0 {
		filter.Limit = DefaultSearchLimit
	}

	return so.nodeOps.GetNodes(filter)
}

// SearchNodesWithProtocol finds nodes supporting a specific internet protocol
func (so *SearchOperations) SearchNodesWithProtocol(protocol string, limit int) ([]database.Node, error) {
	if protocol == "" {
		return nil, fmt.Errorf("protocol cannot be empty")
	}

	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	// Map common protocol names to boolean fields
	var filter database.NodeFilter
	filter.Limit = limit

	switch strings.ToUpper(protocol) {
	case "BINKP", "IBN":
		hasBinkp := true
		filter.HasBinkp = &hasBinkp
	case "TELNET", "ITN":
		// For telnet, we'll need a custom query since it's not a simple boolean
		// This would require extending the NodeFilter or using a custom query
		return nil, fmt.Errorf("telnet search not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	return so.nodeOps.GetNodes(filter)
}

// SearchNodesWithLifetime finds nodes based on filter criteria and returns them with lifetime information
func (so *SearchOperations) SearchNodesWithLifetime(filter database.NodeFilter) ([]NodeSummary, error) {
	// Validate filter
	if err := so.resultParser.ValidateNodeFilter(filter); err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}

	if filter.Limit <= 0 {
		filter.Limit = DefaultSearchLimit
	} else if filter.Limit > MaxSearchLimit {
		filter.Limit = MaxSearchLimit
	}

	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	// Build a modified query that returns summary information with lifetime data
	query := so.queryBuilder.NodeSummarySearchSQL()
	args := so.buildNodeSummaryArgs(filter)

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search nodes with lifetime: %w", err)
	}
	defer rows.Close()

	var results []NodeSummary
	for rows.Next() {
		ns, err := so.resultParser.ParseNodeSummaryRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to parse node summary row: %w", err)
		}
		results = append(results, ns)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating node summary rows: %w", err)
	}

	return results, nil
}

// buildNodeSummaryArgs builds arguments for the node summary search query
func (so *SearchOperations) buildNodeSummaryArgs(filter database.NodeFilter) []interface{} {
	var args []interface{}

	// Add WHERE clause arguments based on filter - each condition uses 2 parameters for NULL checks
	if filter.Zone != nil {
		args = append(args, *filter.Zone, *filter.Zone)
	} else {
		args = append(args, nil, nil)
	}

	if filter.Net != nil {
		args = append(args, *filter.Net, *filter.Net)
	} else {
		args = append(args, nil, nil)
	}

	if filter.Node != nil {
		args = append(args, *filter.Node, *filter.Node)
	} else {
		args = append(args, nil, nil)
	}

	if filter.SystemName != nil {
		pattern := "%" + *filter.SystemName + "%"
		args = append(args, pattern, pattern)
	} else {
		args = append(args, nil, nil)
	}

	if filter.Location != nil {
		pattern := "%" + *filter.Location + "%"
		args = append(args, pattern, pattern)
	} else {
		args = append(args, nil, nil)
	}

	if filter.SysopName != nil {
		pattern := "%" + *filter.SysopName + "%"
		args = append(args, pattern, pattern)
	} else {
		args = append(args, nil, nil)
	}

	// Add LIMIT argument
	args = append(args, filter.Limit)

	return args
}

// PioneerNode represents a pioneer FidoNet node (first appearance in a region)
type PioneerNode struct {
	Zone         int
	Net          int
	Node         int
	Region       int
	SysopName    string
	SystemName   string
	Location     string
	NodelistDate string
	DayNumber    int
	RawLine      string // The original nodelist line
}

// GetPioneersByRegion retrieves the first N unique sysops that appeared in a specific FidoNet region
// A region is identified by zone:region (e.g., zone=2, region=50 means Region 50 in Zone 2)
// Each sysop appears only once, showing their first node in the region
func (so *SearchOperations) GetPioneersByRegion(zone int, region int, limit int) ([]PioneerNode, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	// Use ROW_NUMBER() window function to get the first appearance of each unique sysop
	// The inner query assigns row numbers partitioned by sysop, ordered by date
	// The outer query filters to keep only the first appearance (rn = 1)
	query := `
		SELECT
			zone, net, node, region, sysop_name, system_name, location,
			formatDateTime(nodelist_date, '%Y-%m-%d') as nodelist_date,
			day_number, raw_line
		FROM (
			SELECT
				zone, net, node, region, sysop_name, system_name, location,
				nodelist_date,
				day_number,
				concat(
					if(node_type != '', node_type, ''),
					if(node_type != '', ',', ''),
					toString(node), ',',
					system_name, ',',
					location, ',',
					sysop_name, ',',
					phone, ',',
					toString(max_speed),
					if(length(flags) > 0, ',', ''),
					arrayStringConcat(flags, ',')
				) as raw_line,
				ROW_NUMBER() OVER (PARTITION BY sysop_name ORDER BY nodelist_date ASC, zone ASC, net ASC, node ASC) as rn
			FROM nodes
			WHERE zone = ? AND region = ?
		) AS first_appearances
		WHERE rn = 1
		ORDER BY nodelist_date ASC, zone ASC, net ASC, node ASC
		LIMIT ?
	`

	rows, err := so.db.Conn().Query(query, zone, region, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pioneers: %w", err)
	}
	defer rows.Close()

	var pioneers []PioneerNode
	for rows.Next() {
		var p PioneerNode
		err := rows.Scan(
			&p.Zone,
			&p.Net,
			&p.Node,
			&p.Region,
			&p.SysopName,
			&p.SystemName,
			&p.Location,
			&p.NodelistDate,
			&p.DayNumber,
			&p.RawLine,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pioneer row: %w", err)
		}
		pioneers = append(pioneers, p)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating pioneer rows: %w", err)
	}

	return pioneers, nil
}
