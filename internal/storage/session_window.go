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
// The window is floored at the report's own period. Without that floor a
// candidate can be pulled from up to testSessionWindowSeconds BEFORE the period
// the report summed over, so a node whose latest test sits within the window of
// the period cutoff could be represented by a row the report never accounted
// for - showing IPv6 working, say, on a page listing nodes that lost it. The
// floor cannot be a plain min(test_time) over the anchor CTE: for a node with a
// single qualifying row that collapses the window back to one exact timestamp
// and reinstates the bug this whole mechanism exists to fix.
//
// periodDays is inlined rather than bound, like the validated domain literals in
// domainFilterSQL, so that widening a query costs no change to its positional
// argument list.
//
// IMPORTANT: widening the anchor to a window admits rows the anchor CTE's own
// predicate rejected. Any query whose candidate ordering prefers the aggregated
// row (ORDER BY is_aggregated DESC) or a low hostname_index must therefore
// re-apply that predicate to the candidates, or it can end up representing a
// node with a row that contradicts the report which selected it.
func sessionWindowJoinSQL(rowAlias, anchorAlias string, periodDays int) string {
	return fmt.Sprintf(
		"%[1]s.test_time <= %[2]s.latest_test_time AND %[1]s.test_time >= greatest(%[2]s.latest_test_time - INTERVAL %[3]d SECOND, now() - INTERVAL %[4]d DAY)",
		rowAlias, anchorAlias, testSessionWindowSeconds, periodDays)
}

// cycleWindowMarkers maps each marker to the anchor CTE alias it belongs to.
var cycleWindowMarkers = map[string]string{
	"{{CYCLE_LT}}":  "lt",
	"{{CYCLE_LFT}}": "lft",
	"{{CYCLE_LIT}}": "lit",
}

// applyCycleWindows expands every {{CYCLE_*}} marker in a query. Markers are
// independent, so expansion order does not matter. periodDays must match the
// period the query's anchor CTE filters on.
func applyCycleWindows(query string, periodDays int) string {
	for marker, alias := range cycleWindowMarkers {
		query = strings.ReplaceAll(query, marker, sessionWindowJoinSQL("r", alias, periodDays))
	}
	return query
}

// weeklyNewsPeriodDays is the fixed window the IPv6 weekly-news queries filter
// on; they hardcode it in SQL rather than taking a days parameter.
const weeklyNewsPeriodDays = 7
