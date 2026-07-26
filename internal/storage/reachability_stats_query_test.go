package storage

import (
	"fmt"
	"strings"
	"testing"
)

// TestReachabilityStatsQueryCountsCycles pins the per-cycle scoping of the
// whole-node counters on /reachability/node.
//
// A cycle of a multi-hostname node stores one row per hostname plus an
// aggregated summary. Counting rows made a node with one permanently broken
// backup hostname look partly unreachable - 2:240/5833 reported 24 tests at a
// 66.7% success rate for 8 cycles it answered in every time.
func TestReachabilityStatsQueryCountsCycles(t *testing.T) {
	query := (&TestQueryBuilder{}).BuildReachabilityStatsQuery()

	// Every whole-node counter must be restricted to the cycle's representative
	// row, and the current status must come from the latest cycle rather than
	// whichever row happened to be written last.
	perCycle := []string{
		"countIf(rn = 1) as total_tests",
		") as fully_successful_tests",
		") as partially_failed_tests",
		"countIf(rn = 1 AND NOT is_operational) as failed_tests",
		"countIf(rn = 1 AND is_operational) as successful_tests",
		"avgIf(is_operational, rn = 1) * 100 as success_rate",
		"argMaxIf(is_operational, test_time, rn = 1) as last_status",
	}
	for _, want := range perCycle {
		if !strings.Contains(query, want) {
			t.Errorf("whole-node counter not scoped to one row per cycle: missing %q", want)
		}
	}

	// The two tiers built from multi-line countIf(...) expressions need their
	// guard checked explicitly.
	for _, tier := range []string{"fully_successful_tests", "partially_failed_tests"} {
		idx := strings.Index(query, tier)
		if idx < 0 {
			t.Fatalf("%s tier missing from query", tier)
		}
		// Walk back to the countIf that produces this tier and confirm the guard.
		start := strings.LastIndex(query[:idx], "countIf(")
		if start < 0 || !strings.Contains(query[start:idx], "rn = 1 AND") {
			t.Errorf("%s is counted over every stored row, not once per cycle", tier)
		}
	}

	// The cycle grouping itself: rows are clustered by the measured session
	// window and the aggregated row represents its cycle.
	for _, want := range []string{
		fmt.Sprintf("> %d, 1, 0) as new_cycle", testSessionWindowSeconds),
		"sum(new_cycle) OVER (",
		"ORDER BY is_aggregated DESC, hostname_index ASC",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("cycle grouping incomplete: missing %q", want)
		}
	}

	// The per-protocol rates stay per hostname instance on purpose - they answer
	// "how often does an attempt against this node succeed". If they are ever
	// folded into cycles that is a deliberate product change, not a typo, and
	// this expectation should be updated along with the query comment.
	if strings.Contains(query, "avgIf(binkp_success, rn = 1") {
		t.Error("per-protocol rates were changed to per-cycle; update the query comment and this test together")
	}
	if !strings.Contains(query, "avgIf(binkp_success, binkp_tested) * 100 as binkp_success_rate") {
		t.Error("per-hostname protocol rate lost its original meaning")
	}
}
