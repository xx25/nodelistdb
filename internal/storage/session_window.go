package storage

import (
	"fmt"
	"strings"
)

// A report that anchors on max(test_time) per node must then decide which rows
// belong to that "latest test". Matching the anchor timestamp exactly looks
// right but silently drops rows: one cycle of a multi-hostname node writes a row
// per hostname as the tester reaches it, each with its own test_time, so only
// the hostname tested last survives an equality join. Where inclusion in the
// report depends on a per-hostname row - an announced AKA, a network membership,
// a protocol result - that is a false negative with no visible signal.
//
// These helpers expand the {{CYCLE_*}} markers used in place of an equality
// join, one marker per anchor CTE alias. The bound comes from
// testSessionWindowSeconds, which documents the measurement behind it.
//
// IMPORTANT: widening the anchor to a window admits rows the anchor CTE's own
// predicate rejected. Any query whose candidate ordering prefers the aggregated
// row (ORDER BY is_aggregated DESC) or a low hostname_index must therefore
// re-apply that predicate to the candidates, or it can end up representing a
// node with a row that contradicts the report which selected it.
func sessionWindowJoinSQL(rowAlias, anchorAlias string) string {
	return fmt.Sprintf(
		"%[1]s.test_time <= %[2]s.latest_test_time AND %[1]s.test_time >= %[2]s.latest_test_time - INTERVAL %[3]d SECOND",
		rowAlias, anchorAlias, testSessionWindowSeconds)
}

// cycleWindowMarkers maps each marker to the anchor CTE alias it belongs to.
var cycleWindowMarkers = map[string]string{
	"{{CYCLE_LT}}":  "lt",
	"{{CYCLE_LFT}}": "lft",
	"{{CYCLE_LIT}}": "lit",
}

// applyCycleWindows expands every {{CYCLE_*}} marker in a query. Markers are
// independent, so expansion order does not matter.
func applyCycleWindows(query string) string {
	for marker, alias := range cycleWindowMarkers {
		query = strings.ReplaceAll(query, marker, sessionWindowJoinSQL("r", alias))
	}
	return query
}
