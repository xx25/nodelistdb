package storage

import (
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
