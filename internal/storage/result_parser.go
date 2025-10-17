package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/logging"
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
		&node.HasInet, &internetConfig, &node.FtsId, &node.RawLine,
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
		&stats.PvtNodes, &stats.DownNodes, &stats.HoldNodes,
		&stats.HubNodes, &stats.ZoneNodes, &stats.RegionNodes, &stats.HostNodes,
		&stats.InternetNodes,
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

// ParseTestResultRow parses a database row into a NodeTestResult struct
func (rp *ResultParser) ParseTestResultRow(scanner RowScanner, result *NodeTestResult) error {
	var resolvedIPv4, resolvedIPv6 interface{}
	var binkpAddresses, binkpCapabilities interface{}
	var ifcicoAddresses interface{}

	// First try to scan with new fields (per-hostname testing)
	err := scanner.Scan(
		&result.TestTime,
		&result.Zone,
		&result.Net,
		&result.Node,
		&result.Address,
		&result.Hostname,
		&resolvedIPv4,
		&resolvedIPv6,
		&result.DNSError,
		&result.Country,
		&result.CountryCode,
		&result.City,
		&result.Region,
		&result.Latitude,
		&result.Longitude,
		&result.ISP,
		&result.Org,
		&result.ASN,
		&result.BinkPTested,
		&result.BinkPSuccess,
		&result.BinkPResponseMs,
		&result.BinkPSystemName,
		&result.BinkPSysop,
		&result.BinkPLocation,
		&result.BinkPVersion,
		&binkpAddresses,
		&binkpCapabilities,
		&result.BinkPError,
		&result.IfcicoTested,
		&result.IfcicoSuccess,
		&result.IfcicoResponseMs,
		&result.IfcicoMailerInfo,
		&result.IfcicoSystemName,
		&ifcicoAddresses,
		&result.IfcicoResponseType,
		&result.IfcicoError,
		&result.TelnetTested,
		&result.TelnetSuccess,
		&result.TelnetResponseMs,
		&result.TelnetError,
		&result.FTPTested,
		&result.FTPSuccess,
		&result.FTPResponseMs,
		&result.FTPError,
		&result.VModemTested,
		&result.VModemSuccess,
		&result.VModemResponseMs,
		&result.VModemError,
		&result.BinkPIPv4Tested,
		&result.BinkPIPv4Success,
		&result.BinkPIPv4ResponseMs,
		&result.BinkPIPv4Address,
		&result.BinkPIPv4Error,
		&result.BinkPIPv6Tested,
		&result.BinkPIPv6Success,
		&result.BinkPIPv6ResponseMs,
		&result.BinkPIPv6Address,
		&result.BinkPIPv6Error,
		&result.IfcicoIPv4Tested,
		&result.IfcicoIPv4Success,
		&result.IfcicoIPv4ResponseMs,
		&result.IfcicoIPv4Address,
		&result.IfcicoIPv4Error,
		&result.IfcicoIPv6Tested,
		&result.IfcicoIPv6Success,
		&result.IfcicoIPv6ResponseMs,
		&result.IfcicoIPv6Address,
		&result.IfcicoIPv6Error,
		&result.TelnetIPv4Tested,
		&result.TelnetIPv4Success,
		&result.TelnetIPv4ResponseMs,
		&result.TelnetIPv4Address,
		&result.TelnetIPv4Error,
		&result.TelnetIPv6Tested,
		&result.TelnetIPv6Success,
		&result.TelnetIPv6ResponseMs,
		&result.TelnetIPv6Address,
		&result.TelnetIPv6Error,
		&result.FTPIPv4Tested,
		&result.FTPIPv4Success,
		&result.FTPIPv4ResponseMs,
		&result.FTPIPv4Address,
		&result.FTPIPv4Error,
		&result.FTPIPv6Tested,
		&result.FTPIPv6Success,
		&result.FTPIPv6ResponseMs,
		&result.FTPIPv6Address,
		&result.FTPIPv6Error,
		&result.VModemIPv4Tested,
		&result.VModemIPv4Success,
		&result.VModemIPv4ResponseMs,
		&result.VModemIPv4Address,
		&result.VModemIPv4Error,
		&result.VModemIPv6Tested,
		&result.VModemIPv6Success,
		&result.VModemIPv6ResponseMs,
		&result.VModemIPv6Address,
		&result.VModemIPv6Error,
		&result.IsOperational,
		&result.HasConnectivityIssues,
		&result.AddressValidated,
		&result.TestedHostname,
		&result.HostnameIndex,
		&result.IsAggregated,
		&result.TotalHostnames,
		&result.HostnamesTested,
		&result.HostnamesOperational,
	)

	if err != nil {
		logging.Error("ParseTestResultRow: Scan failed", slog.Any("error", err))
		return fmt.Errorf("failed to scan test result row: %w", err)
	}

	// Parse arrays (compatible with both DuckDB and ClickHouse)
	result.ResolvedIPv4 = rp.parseInterfaceToStringArray(resolvedIPv4)
	result.ResolvedIPv6 = rp.parseInterfaceToStringArray(resolvedIPv6)
	result.BinkPAddresses = rp.parseInterfaceToStringArray(binkpAddresses)
	result.BinkPCapabilities = rp.parseInterfaceToStringArray(binkpCapabilities)
	result.IfcicoAddresses = rp.parseInterfaceToStringArray(ifcicoAddresses)

	return nil
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
