package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"nodelistdb/internal/database"
)

// ResultParser handles parsing of database query results
type ResultParser struct{}

// NewResultParser creates a new ResultParser instance
func NewResultParser() *ResultParser {
	return &ResultParser{}
}

// ParseNodeRow parses a database row into a Node struct
func (rp *ResultParser) ParseNodeRow(scanner RowScanner) (database.Node, error) {
	var node database.Node
	var flags, modemFlags, protocols, hosts, emails interface{}
	var ports interface{}
	var internetConfig sql.NullString

	err := scanner.Scan(
		&node.Zone, &node.Net, &node.Node, &node.NodelistDate, &node.DayNumber,
		&node.SystemName, &node.Location, &node.SysopName, &node.Phone,
		&node.NodeType, &node.Region, &node.MaxSpeed,
		&node.IsCM, &node.IsMO, &node.HasBinkp, &node.HasTelnet,
		&node.IsDown, &node.IsHold, &node.IsPvt, &node.IsActive,
		&flags, &modemFlags, &protocols, &hosts, &ports, &emails,
		&node.ConflictSequence, &node.HasConflict,
		&node.HasInet, &internetConfig,
	)
	if err != nil {
		return node, fmt.Errorf("failed to scan node: %w", err)
	}

	// Parse arrays from DuckDB native format
	node.Flags = rp.parseInterfaceToStringArray(flags)
	node.ModemFlags = rp.parseInterfaceToStringArray(modemFlags)
	node.InternetProtocols = rp.parseInterfaceToStringArray(protocols)
	node.InternetHostnames = rp.parseInterfaceToStringArray(hosts)
	node.InternetPorts = rp.parseInterfaceToIntArray(ports)
	node.InternetEmails = rp.parseInterfaceToStringArray(emails)
	
	// Handle JSON field
	if internetConfig.Valid {
		node.InternetConfig = json.RawMessage(internetConfig.String)
	}

	return node, nil
}

// ParseNodeSummaryRow parses a database row into a NodeSummary struct
func (rp *ResultParser) ParseNodeSummaryRow(scanner RowScanner) (NodeSummary, error) {
	var ns NodeSummary
	err := scanner.Scan(
		&ns.Zone, &ns.Net, &ns.Node,
		&ns.SystemName, &ns.Location, &ns.SysopName,
		&ns.FirstDate, &ns.LastDate, &ns.CurrentlyActive,
	)
	if err != nil {
		return ns, fmt.Errorf("failed to scan node summary: %w", err)
	}
	return ns, nil
}

// ParseNetworkStatsRow parses a database row into a NetworkStats struct
func (rp *ResultParser) ParseNetworkStatsRow(scanner RowScanner) (*database.NetworkStats, error) {
	var stats database.NetworkStats
	err := scanner.Scan(
		&stats.Date, &stats.TotalNodes, &stats.ActiveNodes,
		&stats.CMNodes, &stats.MONodes, &stats.BinkpNodes, &stats.TelnetNodes,
		&stats.PvtNodes, &stats.DownNodes, &stats.HoldNodes, &stats.InternetNodes,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan network stats: %w", err)
	}
	return &stats, nil
}

// ParseRegionInfoRow parses a database row into a RegionInfo struct
func (rp *ResultParser) ParseRegionInfoRow(scanner RowScanner) (database.RegionInfo, error) {
	var region database.RegionInfo
	var regionName sql.NullString
	
	err := scanner.Scan(&region.Zone, &region.Region, &region.NodeCount, &regionName)
	if err != nil {
		return region, fmt.Errorf("failed to scan region info: %w", err)
	}
	
	if regionName.Valid {
		region.Name = regionName.String
	}
	
	return region, nil
}

// ParseNetInfoRow parses a database row into a NetInfo struct
func (rp *ResultParser) ParseNetInfoRow(scanner RowScanner) (database.NetInfo, error) {
	var net database.NetInfo
	var hostName sql.NullString
	
	err := scanner.Scan(&net.Zone, &net.Net, &net.NodeCount, &hostName)
	if err != nil {
		return net, fmt.Errorf("failed to scan net info: %w", err)
	}
	
	if hostName.Valid {
		net.Name = hostName.String
	}
	
	return net, nil
}


// parseInterfaceToStringArray converts DuckDB array results to []string
func (rp *ResultParser) parseInterfaceToStringArray(v interface{}) []string {
	if v == nil {
		return []string{}
	}
	
	switch arr := v.(type) {
	case []interface{}:
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		// Fallback for old format or string representations
		return rp.parseStringArray(arr)
	default:
		return []string{}
	}
}

// parseInterfaceToIntArray converts DuckDB array results to []int
func (rp *ResultParser) parseInterfaceToIntArray(v interface{}) []int {
	if v == nil {
		return []int{}
	}
	
	switch arr := v.(type) {
	case []interface{}:
		result := make([]int, 0, len(arr))
		for _, item := range arr {
			switch val := item.(type) {
			case int:
				result = append(result, val)
			case int32:
				result = append(result, int(val))
			case int64:
				result = append(result, int(val))
			case float64:
				result = append(result, int(val))
			}
		}
		return result
	case string:
		// Fallback for old format or string representations
		return rp.parseIntArray(arr)
	default:
		return []int{}
	}
}

// parseStringArray parses a string representation of a string array
func (rp *ResultParser) parseStringArray(s string) []string {
	if s == "[]" || s == "" {
		return []string{}
	}
	
	// Try JSON unmarshaling first
	var result []string
	if err := json.Unmarshal([]byte(s), &result); err == nil {
		return result
	}
	
	// Fallback to simple parsing
	s = strings.Trim(s, "[]")
	if s == "" {
		return []string{}
	}
	
	// Split by comma and clean up quotes
	parts := strings.Split(s, ",")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, `"'`)
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	
	return cleaned
}

// parseIntArray parses a string representation of an int array
func (rp *ResultParser) parseIntArray(s string) []int {
	if s == "[]" || s == "" {
		return []int{}
	}
	
	// Try JSON unmarshaling first
	var result []int
	if err := json.Unmarshal([]byte(s), &result); err == nil {
		return result
	}
	
	// Fallback to simple parsing
	s = strings.Trim(s, "[]")
	if s == "" {
		return []int{}
	}
	
	parts := strings.Split(s, ",")
	result = make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if val, err := strconv.Atoi(part); err == nil {
			result = append(result, val)
		}
	}
	
	return result
}

// Helper functions for array formatting (DuckDB native array syntax)
func (rp *ResultParser) formatArrayDuckDB(arr []string) string {
	if len(arr) == 0 {
		return "ARRAY[]::TEXT[]"
	}
	
	escaped := make([]string, len(arr))
	for i, s := range arr {
		escaped[i] = "'" + rp.escapeSingleQuotes(s) + "'"
	}
	return fmt.Sprintf("ARRAY[%s]", strings.Join(escaped, ","))
}

// formatIntArrayDuckDB formats an integer array for DuckDB
func (rp *ResultParser) formatIntArrayDuckDB(arr []int) string {
	if len(arr) == 0 {
		return "ARRAY[]::INTEGER[]"
	}
	
	strs := make([]string, len(arr))
	for i, n := range arr {
		strs[i] = fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("ARRAY[%s]", strings.Join(strs, ","))
}

// formatNullableInt formats a nullable integer for SQL
func (rp *ResultParser) formatNullableInt(val *int) string {
	if val == nil {
		return "NULL"
	}
	return fmt.Sprintf("%d", *val)
}

// escapeSingleQuotes escapes single quotes for SQL string literals
func (rp *ResultParser) escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// NodeToInsertArgs converts a Node to arguments for parameterized INSERT
func (rp *ResultParser) NodeToInsertArgs(node database.Node) []interface{} {
	// Handle internet_config JSON
	var configJSON interface{}
	if node.InternetConfig != nil && len(node.InternetConfig) > 0 {
		configJSON = string(node.InternetConfig)
	}

	return []interface{}{
		node.Zone, node.Net, node.Node,
		node.NodelistDate, node.DayNumber,
		node.SystemName, node.Location, node.SysopName, node.Phone,
		node.NodeType, node.Region, node.MaxSpeed,
		node.IsCM, node.IsMO, node.HasBinkp, node.HasTelnet,
		node.IsDown, node.IsHold, node.IsPvt, node.IsActive,
		rp.formatArrayForDB(node.Flags),
		rp.formatArrayForDB(node.ModemFlags),
		rp.formatArrayForDB(node.InternetProtocols),
		rp.formatArrayForDB(node.InternetHostnames),
		rp.formatIntArrayForDB(node.InternetPorts),
		rp.formatArrayForDB(node.InternetEmails),
		node.ConflictSequence, node.HasConflict,
		node.HasInet, configJSON,
	}
}

// formatArrayForDB formats a string array for database storage
func (rp *ResultParser) formatArrayForDB(arr []string) interface{} {
	if len(arr) == 0 {
		return []string{} // Empty slice for DuckDB
	}
	return arr
}

// formatIntArrayForDB formats an int array for database storage
func (rp *ResultParser) formatIntArrayForDB(arr []int) interface{} {
	if len(arr) == 0 {
		return []int{} // Empty slice for DuckDB
	}
	return arr
}

// ValidateNodeFilter validates a NodeFilter for basic sanity checks
func (rp *ResultParser) ValidateNodeFilter(filter database.NodeFilter) error {
	if filter.Zone != nil && (*filter.Zone < 1 || *filter.Zone > 65535) {
		return fmt.Errorf("invalid zone: must be between 1 and 65535")
	}
	
	if filter.Net != nil && (*filter.Net < 0 || *filter.Net > 65535) {
		return fmt.Errorf("invalid net: must be between 0 and 65535")
	}
	
	if filter.Node != nil && (*filter.Node < 0 || *filter.Node > 65535) {
		return fmt.Errorf("invalid node: must be between 0 and 65535")
	}
	
	if filter.Limit < 0 || filter.Limit > MaxSearchLimit {
		return fmt.Errorf("invalid limit: must be between 0 and %d", MaxSearchLimit)
	}
	
	if filter.Offset < 0 {
		return fmt.Errorf("invalid offset: must be non-negative")
	}
	
	if filter.DateFrom != nil && filter.DateTo != nil && filter.DateFrom.After(*filter.DateTo) {
		return fmt.Errorf("invalid date range: date_from cannot be after date_to")
	}
	
	return nil
}

// SanitizeStringInput sanitizes string input for database operations
func (rp *ResultParser) SanitizeStringInput(input string) string {
	// Remove null bytes and control characters
	cleaned := strings.Map(func(r rune) rune {
		if r == 0 || (r < 32 && r != 9 && r != 10 && r != 13) {
			return -1 // Remove character
		}
		return r
	}, input)
	
	// Limit length to prevent excessive memory usage
	const maxLength = 1000
	if len(cleaned) > maxLength {
		cleaned = cleaned[:maxLength]
	}
	
	return cleaned
}