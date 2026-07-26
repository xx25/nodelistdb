package storage

import (
	"fmt"
	"strings"
	"testing"
)

// TestAKAMismatchQueryScoping pins the three conditions that decide which row
// of a multi-hostname node's test cycle gets reported. Each has produced a
// user-visible bug when missing:
//
//   - is_aggregated = false in the FINAL SELECT: the aggregated summary row
//     shares its siblings' test_time and carries no hostname, so it was
//     reported instead of the hostname that mismatched (blank hostname column).
//   - hostname_index pinned in the final join: without it every row of the
//     cycle at that timestamp matched, duplicating nodes.
//   - a session window instead of test_time = max(test_time): the rows of one
//     cycle carry different timestamps, so exact matching dropped every
//     hostname but the last one tested.
func TestAKAMismatchQueryScoping(t *testing.T) {
	am := &AKAMismatchOperations{}

	queries := map[string]string{
		"aka_mismatch": am.buildAKAMismatchQuery("AND node != 0", "fidonet"),
		"ipv6_incorrect_ipv4_correct": am.buildIPVersionMismatchQuery("AND node != 0",
			"r.address_validated_ipv4 = true AND r.address_validated_ipv6 = false",
			"(r.binkp_ipv6_success = true OR r.ifcico_ipv6_success = true)", "fidonet"),
	}

	window := fmt.Sprintf("r.test_time >= lt.latest_test_time - INTERVAL %d SECOND", testSessionWindowSeconds)

	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			// The final SELECT is everything after the closing paren of the
			// last CTE; both queries end with the best_results CTE.
			idx := strings.LastIndex(query, "FROM node_test_results r\n\t\tJOIN best_results br")
			if idx < 0 {
				t.Fatal("could not locate the final SELECT's FROM clause")
			}
			finalSelect := query[idx:]

			if !strings.Contains(finalSelect, "r.is_aggregated = false") {
				t.Error("final SELECT must exclude aggregated rows; they carry no hostname")
			}
			if !strings.Contains(finalSelect, "r.hostname_index = br.hostname_index") {
				t.Error("final join must pin hostname_index to the row best_results chose")
			}
			if !strings.Contains(query, window) {
				t.Errorf("best_results must match the latest test as a session window (%s)", window)
			}
			if strings.Contains(query, "r.test_time = lt.latest_test_time") {
				t.Error("exact test_time matching drops the other hostnames of the same cycle")
			}
		})
	}
}
