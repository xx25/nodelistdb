package storage

import (
	"fmt"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// SearchNodesBySysop finds all nodes associated with a sysop name
func (so *SearchOperations) SearchNodesBySysop(sysopName string, limit int) ([]NodeSummary, error) {
	// Validate and sanitize input
	if sysopName == "" {
		return nil, fmt.Errorf("sysop name cannot be empty")
	}

	sysopName = so.resultParser.SanitizeStringInput(sysopName)

	if limit <= 0 {
		limit = DefaultSysopLimit
	} else if limit > MaxSysopLimit {
		limit = MaxSysopLimit
	}

	so.mu.RLock()
	defer so.mu.RUnlock()

	conn := so.db.Conn()

	query := so.queryBuilder.SysopSearchSQL()
	rows, err := conn.Query(query, sysopName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search nodes by sysop: %w", err)
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
		return nil, fmt.Errorf("error iterating sysop search rows: %w", err)
	}

	return results, nil
}

// GetUniqueSysops returns a list of unique sysops with their node counts
func (so *SearchOperations) GetUniqueSysops(nameFilter string, limit, offset int) ([]SysopInfo, error) {
	so.mu.RLock()
	defer so.mu.RUnlock()

	if limit <= 0 {
		limit = DefaultSysopLimit
	} else if limit > MaxSysopLimit {
		limit = MaxSysopLimit
	}

	if offset < 0 {
		offset = 0
	}

	conn := so.db.Conn()

	// Build query - if nameFilter is provided, filter by it
	var query string
	var args []interface{}

	if nameFilter != "" {
		// Sanitize the filter
		nameFilter = so.resultParser.SanitizeStringInput(nameFilter)
		query = so.queryBuilder.UniqueSysopsWithFilterSQL()
		args = []interface{}{nameFilter, limit, offset}
	} else {
		query = so.queryBuilder.UniqueSysopsSQL()
		args = []interface{}{limit, offset}
	}

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query unique sysops: %w", err)
	}
	defer rows.Close()

	var sysops []SysopInfo
	for rows.Next() {
		var info SysopInfo
		var zonesInterface interface{}

		err := rows.Scan(
			&info.Name,
			&info.NodeCount,
			&info.ActiveNodes,
			&info.FirstSeen,
			&info.LastSeen,
			&zonesInterface,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan sysop row: %w", err)
		}

		// Convert zones to []int - handle both []int and []int32 from different databases
		switch zones := zonesInterface.(type) {
		case []int:
			info.Zones = zones
		case []int32:
			info.Zones = make([]int, len(zones))
			for i, z := range zones {
				info.Zones[i] = int(z)
			}
		case []interface{}:
			info.Zones = make([]int, len(zones))
			for i, z := range zones {
				switch v := z.(type) {
				case int:
					info.Zones[i] = v
				case int32:
					info.Zones[i] = int(v)
				case int64:
					info.Zones[i] = int(v)
				case float64:
					info.Zones[i] = int(v)
				}
			}
		default:
			// Fallback to empty zones if type is unexpected
			info.Zones = []int{}
		}

		sysops = append(sysops, info)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sysop rows: %w", err)
	}

	return sysops, nil
}

// GetNodesBySysop returns all nodes for a specific sysop
func (so *SearchOperations) GetNodesBySysop(sysopName string, limit int) ([]database.Node, error) {
	if sysopName == "" {
		return nil, fmt.Errorf("sysop name cannot be empty")
	}

	// Convert spaces to underscores as that's how data is stored
	sysopName = strings.ReplaceAll(sysopName, " ", "_")

	if limit <= 0 {
		limit = DefaultSearchLimit
	} else if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	// Use NodeFilter with exact match on sysop name
	filter := database.NodeFilter{
		SysopName: &sysopName,
		Limit:     limit,
	}

	return so.nodeOps.GetNodes(filter)
}
