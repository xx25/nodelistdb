package storage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// ClickHouseResultParser handles parsing of ClickHouse database query results
// with ClickHouse-specific array handling
type ClickHouseResultParser struct {
	*ResultParser // Embed the base parser for common functionality
}

// NewClickHouseResultParser creates a new ClickHouse-specific ResultParser instance
func NewClickHouseResultParser() *ClickHouseResultParser {
	return &ClickHouseResultParser{
		ResultParser: NewResultParser(),
	}
}

// ParseNodeRow parses a ClickHouse database row into a Node struct
func (crp *ClickHouseResultParser) ParseNodeRow(scanner RowScanner) (database.Node, error) {
	var node database.Node
	var flags, modemFlags interface{}
	var internetConfig interface{} // ClickHouse JSON type is different

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

	// Parse arrays from ClickHouse native format
	node.Flags = crp.parseClickHouseInterfaceToStringArray(flags)
	node.ModemFlags = crp.parseClickHouseInterfaceToStringArray(modemFlags)

	// Handle JSON field (ClickHouse JSON type)
	if internetConfig != nil {
		switch config := internetConfig.(type) {
		case string:
			if config != "" && config != "{}" {
				node.InternetConfig = json.RawMessage(config)
			}
		case []byte:
			if len(config) > 0 && string(config) != "{}" {
				node.InternetConfig = json.RawMessage(config)
			}
		case map[string]interface{}:
			// ClickHouse may return JSON as a Go map
			if len(config) > 0 {
				if jsonBytes, err := json.Marshal(config); err == nil {
					node.InternetConfig = json.RawMessage(jsonBytes)
				}
			}
		}
	}

	return node, nil
}

// parseClickHouseInterfaceToStringArray converts ClickHouse array results to []string
func (crp *ClickHouseResultParser) parseClickHouseInterfaceToStringArray(v interface{}) []string {
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
	case []string:
		// ClickHouse may return []string directly
		return arr
	case string:
		// Fallback for string representations
		if arr == "[]" || arr == "" {
			return []string{}
		}
		// Try JSON unmarshaling
		var result []string
		if err := json.Unmarshal([]byte(arr), &result); err == nil {
			return result
		}
		// Simple split fallback
		arr = strings.Trim(arr, "[]")
		if arr == "" {
			return []string{}
		}
		parts := strings.Split(arr, ",")
		cleaned := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			part = strings.Trim(part, `"'`)
			if part != "" {
				cleaned = append(cleaned, part)
			}
		}
		return cleaned
	default:
		return []string{}
	}
}

// parseClickHouseInterfaceToIntArray converts ClickHouse array results to []int
func (crp *ClickHouseResultParser) parseClickHouseInterfaceToIntArray(v interface{}) []int {
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
			case string:
				if intVal, err := strconv.Atoi(val); err == nil {
					result = append(result, intVal)
				}
			}
		}
		return result
	case []int:
		// ClickHouse may return []int directly
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
		if arr == "[]" || arr == "" {
			return []int{}
		}
		// Try JSON unmarshaling
		var result []int
		if err := json.Unmarshal([]byte(arr), &result); err == nil {
			return result
		}
		// Simple split fallback
		arr = strings.Trim(arr, "[]")
		if arr == "" {
			return []int{}
		}
		parts := strings.Split(arr, ",")
		result = make([]int, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if intVal, err := strconv.Atoi(part); err == nil {
				result = append(result, intVal)
			}
		}
		return result
	default:
		return []int{}
	}
}

// formatArrayForDB formats a string array for ClickHouse database storage
// Returns ClickHouse Array() literal
func (crp *ClickHouseResultParser) formatArrayForDB(arr []string) interface{} {
	if len(arr) == 0 {
		return "[]"
	}

	// Pre-allocate buffer for better performance
	var buf strings.Builder
	buf.WriteByte('[')

	for i, s := range arr {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\'')
		// Escape single quotes for ClickHouse
		buf.WriteString(strings.ReplaceAll(s, "'", "\\'"))
		buf.WriteByte('\'')
	}

	buf.WriteByte(']')
	return buf.String()
}

// formatIntArrayForDB formats an int array for ClickHouse database storage
// Returns ClickHouse Array() literal
func (crp *ClickHouseResultParser) formatIntArrayForDB(arr []int) interface{} {
	if len(arr) == 0 {
		return "[]"
	}

	// Pre-allocate buffer for better performance
	var buf strings.Builder
	buf.WriteByte('[')

	for i, n := range arr {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(strconv.Itoa(n))
	}

	buf.WriteByte(']')
	return buf.String()
}

// NodeToArgsClickHouse converts a Node to a slice of arguments for ClickHouse parameterized queries
func (crp *ClickHouseResultParser) NodeToArgsClickHouse(node database.Node) []interface{} {
	var regionVal interface{}
	if node.Region != nil {
		regionVal = *node.Region
	} else {
		regionVal = nil
	}

	var configJSON string
	if node.InternetConfig != nil && len(node.InternetConfig) > 0 {
		configJSON = string(node.InternetConfig)
	}

	// Compute FTS ID if not set
	if node.FtsId == "" {
		node.ComputeFtsId()
	}

	return []interface{}{
		node.Zone, node.Net, node.Node,
		node.NodelistDate, node.DayNumber,
		node.SystemName, node.Location, node.SysopName, node.Phone,
		node.NodeType, regionVal, node.MaxSpeed,
		node.IsCM, node.IsMO,
		node.Flags,      // Pass as Go slice for ClickHouse native batch
		node.ModemFlags, // Pass as Go slice for ClickHouse native batch
		node.ConflictSequence, node.HasConflict,
		node.HasInet, configJSON, node.FtsId,
	}
}
