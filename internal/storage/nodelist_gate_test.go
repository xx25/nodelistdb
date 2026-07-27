package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The /analytics pages are censuses of a live FTN network computed from a log
// of daemon activity. The three ways those populations drift apart — a departed
// node still under test, a Down/Hold node still answering, a hand-run probe of
// a protocol the node never announced — are all invisible as errors: the query
// simply answers a neighbouring question. 2:221/1, IVM-free since 2014, was
// listed on /analytics/vmodem-unavailable this way. Hence source guards rather
// than functional tests.
func TestProtocolReportsGateOnAnnouncedFlag(t *testing.T) {
	tqb := &TestQueryBuilder{}

	for protocol := range protocolAnnouncementFlags {
		t.Run(protocol, func(t *testing.T) {
			query := tqb.BuildProtocolEnabledQuery(protocol, "AND node != 0", domainFilterSQL("fidonet", ""), 30)
			flag := ""
			if announcementGatedProtocols[protocol] {
				flag = protocolAnnouncementFlags[protocol]
			}
			assertGate(t, query, flag)
		})
	}

	t.Run("vmodem-unconfirmed", func(t *testing.T) {
		// The inverse report, whose row predicate is merely vmodem_tested —
		// satisfied by any stray probe, so the gate is load-bearing here in a
		// way it is not for the success-predicated reports above.
		query := tqb.BuildVModemUnconfirmedQuery("AND node != 0", domainFilterSQL("fidonet", ""), 30)
		assertGate(t, query, "IVM")
	})
}

// assertGate checks the shape of a report's candidate gate. A non-empty flag
// demands the announcement test as well as nodelist membership; an empty one
// demands membership AND the absence of any flag test, since a page claiming
// only "successfully tested" must not quietly drop AKA-derived results.
func assertGate(t *testing.T, query, flag string) {
	t.Helper()

	gate := "(domain, zone, net, node) IN (SELECT domain, zone, net, node FROM announced_nodes)"
	if !strings.Contains(query, gate) {
		t.Fatalf("query does not gate candidates on the nodelist; want %q in:\n%s", gate, query)
	}

	// The gate belongs to the anchor CTE. Applied later it would still filter,
	// but latest_tests would first pick a max(test_time) over rows the report
	// rejects, so a node's newest ungated probe could hide its newest real one.
	if strings.Index(query, gate) > strings.Index(query, "best_results AS (") {
		t.Error("gate must sit in the latest_tests anchor CTE, not after best_results")
	}

	cte := query[strings.Index(query, "WITH announced_nodes AS ("):strings.Index(query, "latest_tests AS (")]

	if flag == "" {
		if strings.Contains(cte, "JSON_EXISTS") {
			t.Errorf("gate CTE tests a protocol flag, but this page claims only that a test succeeded; "+
				"AKA-derived results would be dropped and the page would contradict its subtitle:\n%s", cte)
		}
	} else {
		if want := "JSON_EXISTS(toString(internet_config), '$.protocols." + flag + "')"; !strings.Contains(cte, want) {
			t.Errorf("gate CTE does not test the right flag; want %q in:\n%s", want, cte)
		}
		// The bloom-indexed superset column is only an accelerator, so it must
		// accompany the exact test, never replace it.
		if !strings.Contains(cte, "hasAny(json_protocols, ['"+flag+"'])") {
			t.Errorf("gate CTE drops the json_protocols prefilter for %s; the exact JSON_EXISTS then reads every granule:\n%s", flag, cte)
		}
	}

	if !strings.Contains(cte, "AND node_type NOT IN ('Down', 'Hold')") {
		t.Errorf("gate CTE admits Down/Hold nodes:\n%s", cte)
	}
	// Zones are reused across FTN networks, so an unscoped gate would let one
	// network's flag admit another network's node with the same triple.
	if n := strings.Count(cte, "AND domain = 'fidonet'"); n < 2 {
		t.Errorf("gate CTE scopes domain %d times, want >= 2 (the nodes scan and its MAX(nodelist_date) subquery):\n%s", n, cte)
	}
	if !strings.Contains(cte, "AND node != 0") {
		t.Errorf("gate CTE ignores the caller's node filter:\n%s", cte)
	}
}

// A protocol with a success predicate but no announcement flag would fall back
// to binkp's flag and quietly gate the wrong page — /analytics/<new> would list
// nodes announcing IBN. Keeping the two maps in lockstep makes that a build-time
// concern instead of a data-shaped one.
func TestEveryTestedProtocolHasAnAnnouncementFlag(t *testing.T) {
	for protocol := range protocolSuccessPredicates {
		if _, ok := protocolAnnouncementFlags[protocol]; !ok {
			t.Errorf("protocol %q has a success predicate but no announcement flag", protocol)
		}
	}
	for protocol := range protocolAnnouncementFlags {
		if _, ok := protocolSuccessPredicates[protocol]; !ok {
			t.Errorf("protocol %q has an announcement flag but no success predicate", protocol)
		}
	}
}

// An unvalidated flag must never reach SQL: like domainFilterSQL's domain, it
// can only ever come from a literal map today, but the cost of the guard is one
// regexp and the cost of being wrong is injection.
func TestAnnouncedProtocolRejectsUnvalidatedFlag(t *testing.T) {
	for _, bad := range []string{"", "ivm", "IVMX", "IV", "IVM'); DROP", "IVM OR 1=1"} {
		got := announcedProtocolPredicateSQL(bad, "", "")
		if !strings.Contains(got, "AND 1 = 0") {
			t.Errorf("flag %q produced a live clause instead of a non-matching one: %s", bad, got)
		}
		if strings.Contains(got, bad) && bad != "" {
			t.Errorf("flag %q was inlined into SQL: %s", bad, got)
		}
	}
}

// The reports built in test_*.go all take node_test_results as their subject
// and reach into `nodes` only to ask what a node claims — exactly the question
// nodelist_gate.go answers. A second spelling in one of them would drift
// silently: nodes could appear in two complementary reports at once or in
// neither, and only a hand count against the nodelist would reveal it.
//
// Scoped to those files on purpose. The nodelist-only reports elsewhere in this
// package (statistics, node search, email) take `nodes` as their subject and
// legitimately write their own node_type and protocol predicates; they have no
// test-log population to reconcile, so the gate does not apply to them.
func TestNodelistGateHasOneSpelling(t *testing.T) {
	sources, err := filepath.Glob("test_*.go")
	if err != nil {
		t.Fatal(err)
	}

	banned := []struct{ fragment, why string }{
		{"$.protocols.", "call announcedProtocolPredicateSQL/announcedProtocolNodesSQL"},
		{"node_type NOT IN", "call nodelistMembershipPredicateSQL/currentNodesSQL"},
	}

	for _, src := range sources {
		if strings.HasSuffix(src, "_test.go") {
			continue
		}
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range banned {
			if strings.Contains(string(body), b.fragment) {
				t.Errorf("%s spells %q itself; %s", src, b.fragment, b.why)
			}
		}
	}
}

// The json_protocols prefilter is only sound next to the exact JSON_EXISTS it
// accelerates — alone it over-matches, because the column is every three-letter
// run in the JSON, not the protocol keys. Keeping it to one file keeps the pair
// together.
func TestJSONProtocolsPrefilterStaysWithItsExactTest(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	for _, src := range sources {
		if strings.HasSuffix(src, "_test.go") || src == "nodelist_gate.go" {
			continue
		}
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "json_protocols") {
			t.Errorf("%s uses the json_protocols prefilter outside nodelist_gate.go, "+
				"where the exact JSON_EXISTS that makes it sound lives", src)
		}
	}
}
