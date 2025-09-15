package storage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

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
func (so *SearchOperations) GetNodeChanges(zone, net, node int) ([]database.NodeChange, error) {
	// Get all historical entries using node operations
	history, err := so.nodeOps.GetNodeHistory(zone, net, node)
	if err != nil {
		return nil, fmt.Errorf("failed to get node history: %w", err)
	}

	if len(history) == 0 {
		return nil, nil
	}

	// Pre-load all unique nodelist dates for efficient gap checking
	allDates, err := so.getAllNodelistDates()
	if err != nil {
		return nil, fmt.Errorf("failed to load nodelist dates: %w", err)
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
		if !so.isConsecutiveNodelist(prev.NodelistDate, curr.NodelistDate, allDates) {
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
		fieldChanges := so.detectFieldChanges(prev, curr)

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
func (so *SearchOperations) detectFieldChanges(prev, curr *database.Node) map[string]string {
	fieldChanges := make(map[string]string)

	if prev.NodeType != curr.NodeType {
		fieldChanges["status"] = fmt.Sprintf("%s → %s", prev.NodeType, curr.NodeType)
	}
	if prev.SystemName != curr.SystemName {
		fieldChanges["name"] = fmt.Sprintf("%s → %s", prev.SystemName, curr.SystemName)
	}
	if prev.Location != curr.Location {
		fieldChanges["location"] = fmt.Sprintf("%s → %s", prev.Location, curr.Location)
	}
	if prev.SysopName != curr.SysopName {
		fieldChanges["sysop"] = fmt.Sprintf("%s → %s", prev.SysopName, curr.SysopName)
	}
	if prev.Phone != curr.Phone {
		fieldChanges["phone"] = fmt.Sprintf("%s → %s", prev.Phone, curr.Phone)
	}
	if prev.MaxSpeed != curr.MaxSpeed {
		fieldChanges["speed"] = fmt.Sprintf("%d → %d", prev.MaxSpeed, curr.MaxSpeed)
	}
	if !so.equalStringSlices(prev.Flags, curr.Flags) {
		fieldChanges["flags"] = fmt.Sprintf("%v → %v", prev.Flags, curr.Flags)
	}

	// Internet connectivity changes
	prevBinkp := so.hasBinkpFromJSON(prev.InternetConfig)
	currBinkp := so.hasBinkpFromJSON(curr.InternetConfig)
	if prevBinkp != currBinkp {
		fieldChanges["binkp"] = fmt.Sprintf("%t → %t", prevBinkp, currBinkp)
	}

	if !so.equalStringSlices(prev.ModemFlags, curr.ModemFlags) {
		fieldChanges["modem_flags"] = fmt.Sprintf("%v → %v", prev.ModemFlags, curr.ModemFlags)
	}

	// Detect internet configuration changes from JSON
	internetChanges := so.detectInternetConfigChanges(prev.InternetConfig, curr.InternetConfig)
	for key, change := range internetChanges {
		fieldChanges[key] = change
	}

	// Check has_inet changes
	if prev.HasInet != curr.HasInet {
		fieldChanges["has_inet"] = fmt.Sprintf("%t → %t", prev.HasInet, curr.HasInet)
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

	// Check if previous config is empty (no protocols, defaults, or email protocols)
	prevIsEmpty := prevConfig == nil || (len(prevConfig.Protocols) == 0 && len(prevConfig.Defaults) == 0 && len(prevConfig.EmailProtocols) == 0 && len(prevConfig.InfoFlags) == 0)
	currIsEmpty := currConfig == nil || (len(currConfig.Protocols) == 0 && len(currConfig.Defaults) == 0 && len(currConfig.EmailProtocols) == 0 && len(currConfig.InfoFlags) == 0)

	if prevIsEmpty && !currIsEmpty {
		// New config added (was empty or nil before)
		for proto, detail := range currConfig.Protocols {
			if detail.Address != "" && detail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s:%d", detail.Address, detail.Port)
			} else if detail.Address != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s", detail.Address)
			} else if detail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added port %d", detail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Added"
			}
		}
		for key, val := range currConfig.Defaults {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Added %s", val)
		}
		for key, detail := range currConfig.EmailProtocols {
			if detail.Email != "" {
				changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Added %s", detail.Email)
			} else {
				changes[fmt.Sprintf("inet_%s", key)] = "Added"
			}
		}
		return changes
	}

	if !prevIsEmpty && currIsEmpty {
		// Config removed (became empty or nil)
		for proto, detail := range prevConfig.Protocols {
			if detail.Address != "" && detail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s:%d", detail.Address, detail.Port)
			} else if detail.Address != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s", detail.Address)
			} else if detail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed port %d", detail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Removed"
			}
		}
		for key, val := range prevConfig.Defaults {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Removed %s", val)
		}
		for key, detail := range prevConfig.EmailProtocols {
			if detail.Email != "" {
				changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Removed %s", detail.Email)
			} else {
				changes[fmt.Sprintf("inet_%s", key)] = "Removed"
			}
		}
		return changes
	}

	// Both configs exist and are non-empty, compare them
	if prevIsEmpty && currIsEmpty {
		return changes
	}

	// Compare protocols
	for proto, currDetail := range currConfig.Protocols {
		prevDetail, existed := prevConfig.Protocols[proto]
		if !existed {
			if currDetail.Address != "" && currDetail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s:%d", currDetail.Address, currDetail.Port)
			} else if currDetail.Address != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s", currDetail.Address)
			} else if currDetail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added port %d", currDetail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Added"
			}
		} else if prevDetail.Address != currDetail.Address || prevDetail.Port != currDetail.Port {
			// Format old and new values
			oldStr := formatProtocolDetail(prevDetail)
			newStr := formatProtocolDetail(currDetail)
			changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("%s → %s", oldStr, newStr)
		}
	}

	// Check for removed protocols
	for proto, prevDetail := range prevConfig.Protocols {
		if _, exists := currConfig.Protocols[proto]; !exists {
			if prevDetail.Address != "" && prevDetail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s:%d", prevDetail.Address, prevDetail.Port)
			} else if prevDetail.Address != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s", prevDetail.Address)
			} else if prevDetail.Port > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed port %d", prevDetail.Port)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Removed"
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

	// First try to unmarshal into a flexible structure that handles both string and int ports
	var rawConfig struct {
		Protocols      map[string]json.RawMessage `json:"protocols,omitempty"`
		Defaults       map[string]string          `json:"defaults,omitempty"`
		EmailProtocols map[string]json.RawMessage `json:"email_protocols,omitempty"`
		InfoFlags      []string                   `json:"info_flags,omitempty"`
	}
	
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		// If parsing fails, return nil config (treated as empty)
		return nil, nil
	}

	// Convert to the proper InternetConfiguration structure
	config := &database.InternetConfiguration{
		Protocols:      make(map[string]database.InternetProtocolDetail),
		Defaults:       rawConfig.Defaults,
		EmailProtocols: make(map[string]database.EmailProtocolDetail),
		InfoFlags:      rawConfig.InfoFlags,
	}

	// Parse protocols with flexible port handling
	for proto, rawDetail := range rawConfig.Protocols {
		var flexDetail struct {
			Address interface{} `json:"address,omitempty"`
			Port    interface{} `json:"port,omitempty"`
		}
		if err := json.Unmarshal(rawDetail, &flexDetail); err != nil {
			continue
		}

		detail := database.InternetProtocolDetail{}
		
		// Handle address
		switch v := flexDetail.Address.(type) {
		case string:
			detail.Address = v
		}
		
		// Handle port (can be string or number)
		switch v := flexDetail.Port.(type) {
		case float64:
			detail.Port = int(v)
		case string:
			// Try to parse string as int
			if portNum, err := strconv.Atoi(v); err == nil {
				detail.Port = portNum
			}
		}
		
		config.Protocols[proto] = detail
	}

	// Parse email protocols
	for proto, rawDetail := range rawConfig.EmailProtocols {
		var detail database.EmailProtocolDetail
		if err := json.Unmarshal(rawDetail, &detail); err != nil {
			continue
		}
		config.EmailProtocols[proto] = detail
	}

	return config, nil
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

// getAllNodelistDates loads all unique nodelist dates from the database
func (so *SearchOperations) getAllNodelistDates() ([]time.Time, error) {
	conn := so.db.Conn()
	query := "SELECT DISTINCT nodelist_date FROM nodes ORDER BY nodelist_date"

	rows, err := conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var date time.Time
		if err := rows.Scan(&date); err != nil {
			return nil, err
		}
		dates = append(dates, date)
	}

	return dates, rows.Err()
}

// formatProtocolDetail formats an InternetProtocolDetail for display
func formatProtocolDetail(detail database.InternetProtocolDetail) string {
	if detail.Address != "" && detail.Port > 0 {
		return fmt.Sprintf("%s:%d", detail.Address, detail.Port)
	} else if detail.Address != "" {
		return detail.Address
	} else if detail.Port > 0 {
		return fmt.Sprintf("port %d", detail.Port)
	}
	return ""
}

func (so *SearchOperations) isConsecutiveNodelist(date1, date2 time.Time, allDates []time.Time) bool {
	// Use the pre-loaded dates list instead of expensive database query
	// Check if there's any date between date1 and date2
	for _, date := range allDates {
		if date.After(date1) && date.Before(date2) {
			return false
		}
	}
	return true
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

// hasBinkpFromJSON checks if IBN or BND protocols exist in the JSON config
func (so *SearchOperations) hasBinkpFromJSON(config json.RawMessage) bool {
	if len(config) == 0 {
		return false
	}
	
	var internetConfig database.InternetConfiguration
	if err := json.Unmarshal(config, &internetConfig); err != nil {
		return false
	}
	
	_, hasIBN := internetConfig.Protocols["IBN"] 
	_, hasBND := internetConfig.Protocols["BND"]
	return hasIBN || hasBND
}
