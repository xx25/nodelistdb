package storage

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The analytics queries in this package share two identity mistakes that have
// each shipped a user-visible bug, and neither shows up as an error - the query
// just quietly answers a different question:
//
//   - anchoring the "latest test" on one exact test_time, which drops the other
//     hostnames of the same test cycle (see testSessionWindowSeconds);
//   - matching a node by (zone, net, node) without its FTN network, when zones
//     are reused across networks (see domainFilterSQL).
//
// Both are invisible in today's data - no (zone,net,node) triple currently
// exists in two networks, and sibling hostnames usually agree - so a
// reintroduced instance would pass every functional test. Hence a source guard.
func TestAnalyticsQueriesScopeIdentityCorrectly(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	banned := []struct {
		name    string
		pattern *regexp.Regexp
		why     string
	}{
		{
			name:    "exact test_time anchor",
			pattern: regexp.MustCompile(`test_time\s*=\s*\w+\.latest_test_time`),
			why:     "use a {{CYCLE_*}} marker (applyCycleWindows) so the whole test cycle is matched, not just its last hostname",
		},
		{
			name:    "domain-blind membership test",
			pattern: regexp.MustCompile(`\(zone, net, node\)\s+(NOT\s+)?IN\b`),
			why:     "include domain: (domain, zone, net, node) IN (...), because zones are reused across FTN networks",
		},
		{
			name:    "domain-blind per-node aggregation",
			pattern: regexp.MustCompile(`GROUP BY zone, net, node\b`),
			why:     "group by domain, zone, net, node so two networks' nodes are not merged",
		},
		{
			name:    "domain-blind identity join",
			pattern: regexp.MustCompile(`(?:JOIN [^\n]*?)?\bON \w+\.zone = `),
			why:     "join on domain first, so a triple present in two networks does not fan out",
		},
		{
			// This commit series fixed three of these in test_modem_queries.go;
			// the guard missed them because it only looked at GROUP BY.
			name:    "domain-blind window partition",
			pattern: regexp.MustCompile(`PARTITION BY (?:\w+\.)?zone, `),
			why:     "partition by domain first, or two networks' nodes share one dedup/numbering group",
		},
	}

	for _, src := range sources {
		if strings.HasSuffix(src, "_test.go") {
			continue
		}
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, b := range banned {
			for _, hit := range b.pattern.FindAllString(text, -1) {
				t.Errorf("%s: %s (%q)\n    %s", src, b.name, hit, b.why)
			}
		}
	}
}

// latestNodesCTE matches a `latest_nodes AS ( ... FROM nodes ... )` block up to
// the GROUP BY that closes it, so the body can be inspected for its bounds.
var latestNodesCTE = regexp.MustCompile(`(?s)latest_nodes AS \(.*?FROM nodes\b(.*?)GROUP BY`)

// funcBodies returns each top-level function in src paired with its source
// text. The window guard has to work per function, not per file: a file whose
// other functions expand {{NODE_WINDOW}} would otherwise vouch for a new one
// that forgot to.
func funcBodies(t *testing.T, src string, body []byte) map[string]string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, src, body, 0)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}

	out := make(map[string]string)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		start := fset.Position(fn.Pos()).Offset
		end := fset.Position(fn.End()).Offset
		out[fn.Name.Name] = string(body[start:end])
	}
	return out
}

// TestLatestNodesCTEsAreBounded keeps the `nodes` history scan behind the
// latest_nodes CTEs bounded.
//
// Unbounded, such a CTE aggregates all 31.5M rows of `nodes` to put a name on a
// few hundred result rows: a country drill-down measured 16.7s and 31.7M rows
// read before the window was added and 0.22s after, and the same pattern once
// cost 19.4M rows on a single-node lookup. Nothing about it looks wrong - the
// results are correct, just slow - so it comes back easily. See
// nodeIdentityWindowSQL.
func TestLatestNodesCTEsAreBounded(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	var found int
	for _, src := range sources {
		if strings.HasSuffix(src, "_test.go") {
			continue
		}
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}

		// Only {{NODE_WINDOW}} counts. An earlier version of this guard also
		// accepted a bare %s, on the theory that the Go side fed it
		// nodeIdentityWindowSQL - but nothing checked that it did, so reverting
		// the fix at the three fmt.Sprintf call sites left the guard green.
		for name, fn := range funcBodies(t, src, body) {
			matches := latestNodesCTE.FindAllStringSubmatch(fn, -1)
			if len(matches) == 0 {
				continue
			}
			found += len(matches)

			for _, m := range matches {
				if !strings.Contains(m[1], "{{NODE_WINDOW}}") {
					t.Errorf("%s: %s has a latest_nodes CTE that scans all of `nodes` unbounded\n"+
						"    add {{NODE_WINDOW}} between FROM nodes and GROUP BY so the history scan is date-bounded", src, name)
				}
			}

			// The marker is inert unless this same function expands it.
			if !strings.Contains(fn, "nodeIdentityWindowSQL(") {
				t.Errorf("%s: %s uses {{NODE_WINDOW}} but never calls nodeIdentityWindowSQL\n"+
					"    the marker would reach ClickHouse verbatim", src, name)
			}
		}
	}

	if found == 0 {
		t.Fatal("no latest_nodes CTE found - this guard has drifted from the queries it protects")
	}
}

// TestJoinedNodeColumnsUseNullif guards the fallbacks on joined `nodes` columns.
//
// Prod runs with join_use_nulls = false and nodes.system_name/sysop_name are
// plain String, so an unmatched LEFT JOIN row yields the empty string rather
// than NULL. COALESCE returns its first non-NULL argument, so a bare
// COALESCE(n.system_name, r.binkp_system_name) keeps that empty string and
// never reaches the mailer-reported fallback; wrapping the first argument in
// NULLIF against the empty string restores the intent. This is invisible while
// the join always matches, and the date window above is exactly what starts
// making it miss.
func TestJoinedNodeColumnsUseNullif(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	// Keyed on the identity columns rather than on the join alias: the alias is
	// whatever the query picked (`n` today, `ln` until this change set deleted
	// it), but system_name/sysop_name only ever come from `nodes`. The trailing
	// [a-zA-Z] means "fallback is another column, not a literal" - a literal
	// fallback such as COALESCE(n.sysop_name, '') already yields '' either way.
	bare := regexp.MustCompile(`COALESCE\(\s*\w+\.(?:system_name|sysop_name)\s*,\s*[a-zA-Z]`)

	for _, src := range sources {
		if strings.HasSuffix(src, "_test.go") {
			continue
		}
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		for _, hit := range bare.FindAllString(string(body), -1) {
			t.Errorf("%s: %q falls back on a joined `nodes` column without NULLIF\n"+
				"    write COALESCE(NULLIF(n.col, ''), ...): join_use_nulls is false, so a miss yields '' and COALESCE keeps it", src, hit)
		}
	}
}

// TestApplyCycleWindowsExpandsEveryMarker makes sure no query ships with an
// unexpanded marker, which ClickHouse would reject at runtime rather than at
// build time.
func TestApplyCycleWindowsExpandsEveryMarker(t *testing.T) {
	for marker, alias := range cycleWindowMarkers {
		got := applyCycleWindows("SELECT 1 WHERE "+marker, 30)
		if strings.Contains(got, "{{") {
			t.Errorf("%s was not expanded: %s", marker, got)
		}
		if !strings.Contains(got, alias+".latest_test_time") {
			t.Errorf("%s expanded without its anchor alias %q: %s", marker, alias, got)
		}
		if !strings.Contains(got, "INTERVAL 120 SECOND") {
			t.Errorf("%s expanded without the session window bound: %s", marker, got)
		}
	}

	// Every marker used in the package's queries must be one applyCycleWindows
	// knows about, and every file that uses one must actually call it - a
	// forgotten call leaves a literal marker in the SQL, which fails only when
	// the query runs.
	sources, _ := filepath.Glob("*.go")
	used := regexp.MustCompile(`\{\{CYCLE_\w+\}\}`)
	for _, src := range sources {
		if strings.HasSuffix(src, "_test.go") || src == "session_window.go" {
			continue
		}
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		markers := used.FindAllString(text, -1)
		for _, marker := range markers {
			if _, ok := cycleWindowMarkers[marker]; !ok {
				t.Errorf("%s uses unknown marker %s - add it to cycleWindowMarkers", src, marker)
			}
		}
		if len(markers) > 0 && !strings.Contains(text, "applyCycleWindows(") {
			t.Errorf("%s contains %d cycle marker(s) but never calls applyCycleWindows - the marker would reach ClickHouse verbatim",
				src, len(markers))
		}
	}
}
