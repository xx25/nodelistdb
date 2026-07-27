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

// The long analytics queries are assembled from {{MARKER}} placeholders instead
// of threading one more %s through a hundred-line positional format string.
// The cost of that is a failure mode with no compile-time signal: a query that
// carries a marker no one expands reaches ClickHouse verbatim and fails at
// runtime, on that page only, for whoever opens it first.
//
// This nearly shipped while the nodelist gate was being added — two markers
// went into GetGeoHostingDistribution, whose two queries are built and run
// inline and so hit neither of the ReplaceAll blocks the other functions share.
//
// A function that mentions a marker must therefore also expand it. Checking per
// function rather than per file is the point: expansion in a neighbouring
// function is exactly the mistake.
func TestEveryQueryMarkerIsExpanded(t *testing.T) {
	// Markers expanded by a helper call. A function may hold several queries in
	// separate variables and each needs its own call, so the gate marker is
	// counted rather than merely looked for: one call expands one variable, and
	// GetGeoHostingDistribution builds two. (The cycle markers cannot be counted
	// the same way — one applyCycleWindows call expands every {{CYCLE_*}} in the
	// query, so several markers legitimately share one call.)
	byHelper := map[string]string{
		"{{CYCLE_LT}}":  "applyCycleWindows",
		"{{CYCLE_LFT}}": "applyCycleWindows",
		"{{CYCLE_LIT}}": "applyCycleWindows",
	}
	byCountedHelper := map[string]string{
		"{{NODELIST_GATE}}": "applyNodelistGate(",
	}
	// Markers expanded inline by strings.ReplaceAll naming the marker.
	byReplaceAll := []string{"{{NODE_WINDOW}}", "{{DOMAIN_FILTER}}", "{{DOMAIN_FILTER_R}}"}

	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	for _, src := range sources {
		if strings.HasSuffix(src, "_test.go") {
			continue
		}
		body, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, src, body, 0)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			text := string(body[fset.Position(fn.Pos()).Offset:fset.Position(fn.End()).Offset])

			for marker, helper := range byHelper {
				if strings.Contains(text, marker) && !strings.Contains(text, helper+"(") {
					t.Errorf("%s: %s carries %s but never calls %s; the marker would reach ClickHouse verbatim",
						src, fn.Name.Name, marker, helper)
				}
			}
			for marker, call := range byCountedHelper {
				// The marker also appears inside the helper's own definition and
				// doc comment; those live in nodelist_gate.go, which defines no
				// query, so counting there is harmless as long as it balances.
				if markers, calls := strings.Count(text, marker), strings.Count(text, call); markers > calls {
					t.Errorf("%s: %s carries %s %d time(s) but calls %s only %d time(s); "+
						"a query built into a second variable is left unexpanded and would reach ClickHouse verbatim",
						src, fn.Name.Name, marker, markers, call, calls)
				}
			}
			for _, marker := range byReplaceAll {
				if !strings.Contains(text, marker) {
					continue
				}
				expand := regexp.MustCompile(`ReplaceAll\([^,]+,\s*"` + regexp.QuoteMeta(marker) + `"`)
				if !expand.MatchString(text) {
					t.Errorf("%s: %s carries %s but never ReplaceAlls it; the marker would reach ClickHouse verbatim",
						src, fn.Name.Name, marker)
				}
			}
		}
	}
}
