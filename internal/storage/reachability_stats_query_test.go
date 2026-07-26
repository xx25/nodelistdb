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
		"countIf(rn = 1 AND NOT cyc_operational) as failed_tests",
		"countIf(rn = 1 AND cyc_operational) as successful_tests",
		"avgIf(cyc_operational, rn = 1) * 100 as success_rate",
		"argMaxIf(cyc_operational, test_time, rn = 1) as last_status",
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
		fmt.Sprintf("> %d", testSessionWindowSeconds),
		") as new_cycle",
		"sum(new_cycle) OVER (",
		"ORDER BY is_aggregated DESC, is_operational DESC, hostname_index ASC, test_time ASC",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("cycle grouping incomplete: missing %q", want)
		}
	}

	// Two tests landing inside the window must still be two cycles. Comparing
	// only against the immediately preceding row is not enough: for two complete
	// multi-hostname cycles the transition is aggregate(index 0) -> next
	// primary(index 0), which no single-pair comparison reveals. The boundary is
	// therefore "the previous row was an aggregate" plus "a per-hostname index
	// that does not advance".
	if !strings.Contains(query, "lagInFrame(is_aggregated, 1, true) OVER seq") {
		t.Error("an aggregate must end its cycle, or two back-to-back multi-hostname cycles merge into one")
	}
	if !strings.Contains(query, "hostname_index <= lagInFrame(hostname_index") {
		t.Error("a per-hostname index that does not advance must start a new cycle (re-test, or a cycle whose aggregate was never written)")
	}
	// The ordering the boundary rules depend on: per-hostname rows ascending,
	// then the aggregate, even when the whole cycle shares one test_time.
	if !strings.Contains(query, "ORDER BY test_time, is_aggregated, hostname_index") {
		t.Error("cycle detection needs a deterministic within-timestamp order or ties decide the boundaries")
	}

	// The tiers must be folded across the cycle, not read off whichever row
	// represents it: an interrupted cycle's working hostname would otherwise
	// report a node as fully successful while a different hostname is the one
	// with broken IPv6.
	for _, want := range []string{
		"max(is_operational) OVER cyc as cyc_operational",
		"max(length(resolved_ipv6) > 0) OVER cyc as cyc_has_ipv6",
		"max(binkp_ipv6_success) OVER cyc as cyc_binkp_ipv6_success",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("cycle-level fold missing: %q", want)
		}
	}
	if strings.Contains(query, "countIf(rn = 1 AND NOT is_operational)") {
		t.Error("failed_tests still reads one row's is_operational instead of the cycle's")
	}

	// The exact ORDER BY asserted above already pins the precedence that matters:
	// is_operational outranks hostname_index, so a cycle whose aggregate was
	// never written is not reported as failed on the strength of one broken
	// primary hostname, and test_time last keeps the pick deterministic.

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
