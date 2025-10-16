package storage

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/database"
)

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

// Helper functions for change detection

// equalStringSlices compares two string slices for equality
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

// isConsecutiveNodelist checks if two dates are consecutive in the nodelist
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

// getNextNodelistDate gets the next nodelist date after the given date
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

// getNextDayNumber calculates the next day number
func (so *SearchOperations) getNextDayNumber(afterDay int) int {
	// Simple increment, could be improved with actual lookup
	return afterDay + 7
}

// isCurrentlyActive checks if a node is currently active in the latest nodelist
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
