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

// ParseNodeRow parses a database row into a Node struct (supports both DuckDB and ClickHouse)
func (rp *ResultParser) ParseNodeRow(scanner RowScanner) (database.Node, error) {
	var node database.Node
	var flags, modemFlags interface{}
	var internetConfig interface{} // Use interface{} to support both DuckDB and ClickHouse JSON types

	err := scanner.Scan(
		&node.Zone, &node.Net, &node.Node, &node.NodelistDate, &node.DayNumber,
		&node.SystemName, &node.Location, &node.SysopName, &node.Phone,
		&node.NodeType, &node.Region, &node.MaxSpeed,
		&node.IsCM, &node.IsMO,
		&flags, &modemFlags,
		&node.ConflictSequence, &node.HasConflict,
		&node.HasInet, &internetConfig, &node.FtsId,
	)
	if err != nil {
		return node, fmt.Errorf("failed to scan node: %w", err)
	}

	// Parse arrays (compatible with both DuckDB and ClickHouse)
	node.Flags = rp.parseInterfaceToStringArray(flags)
	node.ModemFlags = rp.parseInterfaceToStringArray(modemFlags)

	// Handle JSON field (support both DuckDB sql.NullString and ClickHouse JSON types)
	rp.parseInternetConfig(&node, internetConfig)

	return node, nil
}

// parseInternetConfig handles JSON parsing for both DuckDB and ClickHouse
func (rp *ResultParser) parseInternetConfig(node *database.Node, internetConfig interface{}) {
	if internetConfig == nil {
		return
	}

	switch config := internetConfig.(type) {
	case sql.NullString:
		// DuckDB format
		if config.Valid && config.String != "" && config.String != "{}" {
			node.InternetConfig = json.RawMessage(config.String)
		}
	case string:
		// ClickHouse string format
		if config != "" && config != "{}" {
			node.InternetConfig = json.RawMessage(config)
		}
	case []byte:
		// ClickHouse byte array format
		if len(config) > 0 && string(config) != "{}" {
			node.InternetConfig = json.RawMessage(config)
		}
	case map[string]interface{}:
		// ClickHouse JSON object format
		if len(config) > 0 {
			if jsonBytes, err := json.Marshal(config); err == nil {
				node.InternetConfig = json.RawMessage(jsonBytes)
			}
		}
	}
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

// parseInterfaceToStringArray converts array results to []string (supports both DuckDB and ClickHouse)
func (rp *ResultParser) parseInterfaceToStringArray(v interface{}) []string {
	if v == nil {
		return []string{}
	}

	switch arr := v.(type) {
	case []interface{}:
		// DuckDB format
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		// ClickHouse native []string format
		return arr
	case string:
		// Fallback for string representations
		return rp.parseStringArray(arr)
	default:
		// Unknown type, return empty
		return []string{}
	}
}

// parseInterfaceToIntArray converts array results to []int (supports both DuckDB and ClickHouse)
func (rp *ResultParser) parseInterfaceToIntArray(v interface{}) []int {
	if v == nil {
		return []int{}
	}

	switch arr := v.(type) {
	case []interface{}:
		// DuckDB format
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
	case []int:
		// ClickHouse native []int format
		return arr
	case []int32:
		// ClickHouse Int32 arrays
		result := make([]int, len(arr))
		for i, v := range arr {
			result[i] = int(v)
		}
		return result
	case string:
		// Fallback for string representations
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
	// Compute FTS ID if not already set
	if node.FtsId == "" {
		// Create a mutable copy to compute FTS ID
		nodeCopy := node
		nodeCopy.ComputeFtsId()
		node = nodeCopy
	}

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
		node.IsCM, node.IsMO,
		rp.formatArrayForDB(node.Flags),
		rp.formatArrayForDB(node.ModemFlags),
		node.ConflictSequence, node.HasConflict,
		node.HasInet, configJSON, node.FtsId,
	}
}

// formatArrayForDB formats a string array for database storage
// Returns optimized DuckDB ARRAY[] literal (faster than JSON casting)
func (rp *ResultParser) formatArrayForDB(arr []string) interface{} {
	if len(arr) == 0 {
		return "ARRAY[]::TEXT[]"
	}

	// Pre-allocate buffer for better performance
	var buf strings.Builder
	buf.WriteString("ARRAY[")

	for i, s := range arr {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\'')
		// Fast escape - only escape single quotes
		buf.WriteString(strings.ReplaceAll(s, "'", "''"))
		buf.WriteByte('\'')
	}

	buf.WriteByte(']')
	return buf.String()
}

// formatIntArrayForDB formats an int array for database storage
// Returns optimized DuckDB ARRAY[] literal (faster than JSON casting)
func (rp *ResultParser) formatIntArrayForDB(arr []int) interface{} {
	if len(arr) == 0 {
		return "ARRAY[]::INTEGER[]"
	}

	// Pre-allocate buffer for better performance
	var buf strings.Builder
	buf.WriteString("ARRAY[")

	for i, n := range arr {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(strconv.Itoa(n))
	}

	buf.WriteByte(']')
	return buf.String()
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
