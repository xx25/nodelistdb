package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"nodelistdb/internal/database"
)

// SearchOperations handles all search-related database operations
type SearchOperations struct {
	db           *database.DB
	queryBuilder QueryBuilderInterface
	resultParser *ResultParser
	nodeOps      *NodeOperations // Reference for getting node history
	mu           sync.RWMutex
}

// NewSearchOperations creates a new SearchOperations instance
func NewSearchOperations(db *database.DB, queryBuilder QueryBuilderInterface, resultParser *ResultParser, nodeOps *NodeOperations) *SearchOperations {
	return &SearchOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
		nodeOps:      nodeOps,
	}
}

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

// GetNodeChanges analyzes the history of a node and returns detected changes
func (so *SearchOperations) GetNodeChanges(zone, net, node int, filter ChangeFilter) ([]database.NodeChange, error) {
	// Get all historical entries using node operations
	history, err := so.nodeOps.GetNodeHistory(zone, net, node)
	if err != nil {
		return nil, fmt.Errorf("failed to get node history: %w", err)
	}

	if len(history) == 0 {
		return nil, nil
	}

	var changes []database.NodeChange

	// Add the first appearance
	changes = append(changes, database.NodeChange{
		Date:       history[0].NodelistDate,
		DayNumber:  history[0].DayNumber,
		ChangeType: "added",
		Changes:    make(map[string]string),
		NewNode:    &history[0],
	})

	// Track changes between consecutive entries
	for i := 1; i < len(history); i++ {
		prev := &history[i-1]
		curr := &history[i]

		// Skip if same date but different conflict sequence
		if prev.NodelistDate.Equal(curr.NodelistDate) {
			continue
		}

		// Check if node was removed (gap in dates)
		if !so.isConsecutiveNodelist(prev.NodelistDate, curr.NodelistDate) {
			// Node was removed and then re-added
			changes = append(changes, database.NodeChange{
				Date:       so.getNextNodelistDate(prev.NodelistDate),
				DayNumber:  so.getNextDayNumber(prev.DayNumber),
				ChangeType: "removed",
				Changes:    make(map[string]string),
				OldNode:    prev,
			})

			changes = append(changes, database.NodeChange{
				Date:       curr.NodelistDate,
				DayNumber:  curr.DayNumber,
				ChangeType: "added",
				Changes:    make(map[string]string),
				NewNode:    curr,
			})
			continue
		}

		// Detect field changes
		fieldChanges := so.detectFieldChanges(prev, curr, filter)

		if len(fieldChanges) > 0 {
			changes = append(changes, database.NodeChange{
				Date:       curr.NodelistDate,
				DayNumber:  curr.DayNumber,
				ChangeType: "modified",
				Changes:    fieldChanges,
				OldNode:    prev,
				NewNode:    curr,
			})
		}
	}

	// Check if node is currently removed
	lastNode := &history[len(history)-1]
	if !so.isCurrentlyActive(lastNode) {
		changes = append(changes, database.NodeChange{
			Date:       so.getNextNodelistDate(lastNode.NodelistDate),
			DayNumber:  so.getNextDayNumber(lastNode.DayNumber),
			ChangeType: "removed",
			Changes:    make(map[string]string),
			OldNode:    lastNode,
		})
	}

	return changes, nil
}

// detectFieldChanges analyzes two consecutive node entries for changes
func (so *SearchOperations) detectFieldChanges(prev, curr *database.Node, filter ChangeFilter) map[string]string {
	fieldChanges := make(map[string]string)

	if !filter.IgnoreStatus && prev.NodeType != curr.NodeType {
		fieldChanges["status"] = fmt.Sprintf("%s → %s", prev.NodeType, curr.NodeType)
	}
	if !filter.IgnoreName && prev.SystemName != curr.SystemName {
		fieldChanges["name"] = fmt.Sprintf("%s → %s", prev.SystemName, curr.SystemName)
	}
	if !filter.IgnoreLocation && prev.Location != curr.Location {
		fieldChanges["location"] = fmt.Sprintf("%s → %s", prev.Location, curr.Location)
	}
	if !filter.IgnoreSysop && prev.SysopName != curr.SysopName {
		fieldChanges["sysop"] = fmt.Sprintf("%s → %s", prev.SysopName, curr.SysopName)
	}
	if !filter.IgnorePhone && prev.Phone != curr.Phone {
		fieldChanges["phone"] = fmt.Sprintf("%s → %s", prev.Phone, curr.Phone)
	}
	if !filter.IgnoreSpeed && prev.MaxSpeed != curr.MaxSpeed {
		fieldChanges["speed"] = fmt.Sprintf("%s → %s", prev.MaxSpeed, curr.MaxSpeed)
	}
	if !filter.IgnoreFlags && !so.equalStringSlices(prev.Flags, curr.Flags) {
		fieldChanges["flags"] = fmt.Sprintf("%v → %v", prev.Flags, curr.Flags)
	}

	// Internet connectivity changes
	if !filter.IgnoreConnectivity {
		if prev.HasBinkp != curr.HasBinkp {
			fieldChanges["binkp"] = fmt.Sprintf("%t → %t", prev.HasBinkp, curr.HasBinkp)
		}
		if prev.HasTelnet != curr.HasTelnet {
			fieldChanges["telnet"] = fmt.Sprintf("%t → %t", prev.HasTelnet, curr.HasTelnet)
		}
	}

	if !filter.IgnoreModemFlags && !so.equalStringSlices(prev.ModemFlags, curr.ModemFlags) {
		fieldChanges["modem_flags"] = fmt.Sprintf("%v → %v", prev.ModemFlags, curr.ModemFlags)
	}

	if !filter.IgnoreInternetProtocols && !so.equalStringSlices(prev.InternetProtocols, curr.InternetProtocols) {
		fieldChanges["internet_protocols"] = fmt.Sprintf("%v → %v", prev.InternetProtocols, curr.InternetProtocols)
	}

	if !filter.IgnoreInternetHostnames && !so.equalStringSlices(prev.InternetHostnames, curr.InternetHostnames) {
		fieldChanges["internet_hostnames"] = fmt.Sprintf("%v → %v", prev.InternetHostnames, curr.InternetHostnames)
	}

	if !filter.IgnoreInternetPorts && !so.equalIntSlices(prev.InternetPorts, curr.InternetPorts) {
		fieldChanges["internet_ports"] = fmt.Sprintf("%v → %v", prev.InternetPorts, curr.InternetPorts)
	}

	if !filter.IgnoreInternetEmails && !so.equalStringSlices(prev.InternetEmails, curr.InternetEmails) {
		fieldChanges["internet_emails"] = fmt.Sprintf("%v → %v", prev.InternetEmails, curr.InternetEmails)
	}

	// Check has_inet changes
	if !filter.IgnoreConnectivity && prev.HasInet != curr.HasInet {
		fieldChanges["has_inet"] = fmt.Sprintf("%t → %t", prev.HasInet, curr.HasInet)
	}

	// Detect internet config changes using JSON-based detection
	if !filter.IgnoreConnectivity || !filter.IgnoreInternetProtocols || !filter.IgnoreInternetHostnames {
		configChanges := so.detectInternetConfigChanges(prev.InternetConfig, curr.InternetConfig)
		for key, value := range configChanges {
			fieldChanges[key] = value
		}
	}

	return fieldChanges
}

// detectInternetConfigChanges compares two JSON configs and returns detailed changes
func (so *SearchOperations) detectInternetConfigChanges(prev, curr json.RawMessage) map[string]string {
	changes := make(map[string]string)

	prevConfig, prevErr := so.parseInternetConfig(prev)
	currConfig, currErr := so.parseInternetConfig(curr)

	// Handle errors or nil configs
	if prevErr != nil || currErr != nil {
		if len(prev) > 0 && len(curr) == 0 {
			changes["internet_config"] = "Removed all internet configuration"
		} else if len(prev) == 0 && len(curr) > 0 {
			changes["internet_config"] = "Added internet configuration"
		}
		return changes
	}

	if prevConfig == nil && currConfig == nil {
		return changes
	}

	if prevConfig == nil && currConfig != nil {
		// New config added
		for proto, detail := range currConfig.Protocols {
			if detail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s:%d", detail.Address, detail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s", detail.Address)
			}
		}
		for key, val := range currConfig.Defaults {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Added %s", val)
		}
		return changes
	}

	if prevConfig != nil && currConfig == nil {
		// Config removed
		for proto, detail := range prevConfig.Protocols {
			if detail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s:%d", detail.Address, detail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s", detail.Address)
			}
		}
		return changes
	}

	// Compare protocols
	for proto, currDetail := range currConfig.Protocols {
		prevDetail, existed := prevConfig.Protocols[proto]
		if !existed {
			if currDetail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s:%d", currDetail.Address, currDetail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s", currDetail.Address)
			}
		} else if prevDetail.Address != currDetail.Address || prevDetail.Port != currDetail.Port {
			oldStr := prevDetail.Address
			newStr := currDetail.Address
			if prevDetail.Port > 0 {
				oldStr = fmt.Sprintf("%s:%d", prevDetail.Address, prevDetail.Port)
			}
			if currDetail.Port > 0 {
				newStr = fmt.Sprintf("%s:%d", currDetail.Address, currDetail.Port)
			}
			changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("%s → %s", oldStr, newStr)
		}
	}

	// Check for removed protocols
	for proto, prevDetail := range prevConfig.Protocols {
		if _, exists := currConfig.Protocols[proto]; !exists {
			if prevDetail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s:%d", prevDetail.Address, prevDetail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s", prevDetail.Address)
			}
		}
	}

	// Compare defaults
	for key, currVal := range currConfig.Defaults {
		prevVal, existed := prevConfig.Defaults[key]
		if !existed {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Added %s", currVal)
		} else if prevVal != currVal {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("%s → %s", prevVal, currVal)
		}
	}

	// Check for removed defaults
	for key, prevVal := range prevConfig.Defaults {
		if _, exists := currConfig.Defaults[key]; !exists {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Removed %s", prevVal)
		}
	}

	// Compare email protocols
	for proto, currDetail := range currConfig.EmailProtocols {
		prevDetail, existed := prevConfig.EmailProtocols[proto]
		if !existed {
			if currDetail.Email != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s", currDetail.Email)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Added (uses default email)"
			}
		} else if prevDetail.Email != currDetail.Email {
			if currDetail.Email != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("%s → %s", prevDetail.Email, currDetail.Email)
			}
		}
	}

	// Check for removed email protocols
	for proto, prevDetail := range prevConfig.EmailProtocols {
		if _, exists := currConfig.EmailProtocols[proto]; !exists {
			if prevDetail.Email != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s", prevDetail.Email)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Removed"
			}
		}
	}

	// Compare info flags
	prevFlags := make(map[string]bool)
	currFlags := make(map[string]bool)

	for _, flag := range prevConfig.InfoFlags {
		prevFlags[flag] = true
	}
	for _, flag := range currConfig.InfoFlags {
		currFlags[flag] = true
	}

	for flag := range currFlags {
		if !prevFlags[flag] {
			changes[fmt.Sprintf("inet_flag_%s", flag)] = "Added"
		}
	}
	for flag := range prevFlags {
		if !currFlags[flag] {
			changes[fmt.Sprintf("inet_flag_%s", flag)] = "Removed"
		}
	}

	return changes
}

// parseInternetConfig unmarshals JSON into InternetConfiguration struct
func (so *SearchOperations) parseInternetConfig(data json.RawMessage) (*database.InternetConfiguration, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var config database.InternetConfiguration
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// Helper functions for change detection
func (so *SearchOperations) equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (so *SearchOperations) equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (so *SearchOperations) isConsecutiveNodelist(date1, date2 time.Time) bool {
	// Check if there's a nodelist between these two dates
	conn := so.db.Conn()
	var count int
	query := so.queryBuilder.ConsecutiveNodelistCheckSQL()
	err := conn.QueryRow(query, date1, date2).Scan(&count)

	return err == nil && count == 0
}

func (so *SearchOperations) getNextNodelistDate(afterDate time.Time) time.Time {
	conn := so.db.Conn()
	var nextDate time.Time
	query := so.queryBuilder.NextNodelistDateSQL()
	err := conn.QueryRow(query, afterDate).Scan(&nextDate)

	if err != nil {
		return afterDate.AddDate(0, 0, 7) // Assume weekly
	}
	return nextDate
}

func (so *SearchOperations) getNextDayNumber(afterDay int) int {
	// Simple increment, could be improved with actual lookup
	return afterDay + 7
}

func (so *SearchOperations) isCurrentlyActive(node *database.Node) bool {
	conn := so.db.Conn()
	var maxDate time.Time
	query := so.queryBuilder.LatestDateSQL()
	err := conn.QueryRow(query).Scan(&maxDate)

	if err != nil {
		return false
	}

	return node.NodelistDate.Equal(maxDate)
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
	// Force active filter
	active := true
	filter.IsActive = &active

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
		query = `
			WITH sysop_stats AS (
				SELECT 
					sysop_name,
					COUNT(DISTINCT (zone || ':' || net || '/' || node)) as node_count,
					COUNT(DISTINCT CASE WHEN is_active = true THEN (zone || ':' || net || '/' || node) END) as active_nodes,
					MIN(nodelist_date) as first_seen,
					MAX(nodelist_date) as last_seen,
					ARRAY_AGG(DISTINCT zone ORDER BY zone) as zones
				FROM nodes
				WHERE sysop_name ILIKE ?
				GROUP BY sysop_name
			)
			SELECT 
				sysop_name,
				node_count,
				active_nodes,
				first_seen,
				last_seen,
				zones
			FROM sysop_stats
			ORDER BY node_count DESC, sysop_name
			LIMIT ? OFFSET ?
		`
		args = []interface{}{"%" + nameFilter + "%", limit, offset}
	} else {
		query = `
			WITH sysop_stats AS (
				SELECT 
					sysop_name,
					COUNT(DISTINCT (zone || ':' || net || '/' || node)) as node_count,
					COUNT(DISTINCT CASE WHEN is_active = true THEN (zone || ':' || net || '/' || node) END) as active_nodes,
					MIN(nodelist_date) as first_seen,
					MAX(nodelist_date) as last_seen,
					ARRAY_AGG(DISTINCT zone ORDER BY zone) as zones
				FROM nodes
				GROUP BY sysop_name
			)
			SELECT 
				sysop_name,
				node_count,
				active_nodes,
				first_seen,
				last_seen,
				zones
			FROM sysop_stats
			ORDER BY node_count DESC, sysop_name
			LIMIT ? OFFSET ?
		`
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
		var zones []int
		
		err := rows.Scan(
			&info.Name,
			&info.NodeCount,
			&info.ActiveNodes,
			&info.FirstSeen,
			&info.LastSeen,
			&zones,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan sysop row: %w", err)
		}
		
		info.Zones = zones
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
