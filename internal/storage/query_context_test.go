package storage

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every server-facing read runs under the caller's context, so a closed
// browser tab or an abandoned analytics page stops the query on ClickHouse
// instead of leaving it to finish for nobody. The driver makes that real:
// clickhouse-go writes ClientCancel to the server before dropping the socket,
// so this is server CPU saved, not just a Go goroutine released.
//
// Nothing in the type system enforces it. A new reader that reaches for
// conn.Query instead of conn.QueryContext compiles, passes every test, and
// silently opts back out. That is what this guard is for.
//
// It matches by selector name, textually, inside each function body - the same
// shape as query_markers_test.go and test_result_columns_test.go. Resolving
// receiver types with go/types would be new machinery for one check, and the
// bare names are unambiguous enough here: `.Query(`, `.QueryRow(` and `.Exec(`
// on anything other than a database handle do not occur in this package.
//
// Matching QueryRow matters as much as matching Query. Four whole
// server-facing methods - GetNodeDateRange, GetLatestStatsDate,
// GetNearestAvailableDate and GetStats - contain no `.Query(` call at all;
// they are built entirely out of QueryRow. A guard watching only `.Query(`
// would have reported green over twenty-two uncancellable calls.
func TestServerReadsUseQueryContext(t *testing.T) {
	// The import path has no request behind it and nothing to cancel it: it is
	// driven by cmd/parser, which runs to completion or fails. These functions
	// keep the context-free calls on purpose.
	//
	// Adding a name here is a decision, not a formality - it says "this code
	// can never be reached from an HTTP handler". Check that before you do it.
	allowed := map[string]string{
		"InsertNodesInChunks":   "nodelist import: bulk INSERT driven by cmd/parser",
		"FindConflictingNode":   "nodelist import: duplicate-address check",
		"IsNodelistProcessed":   "nodelist import: already-imported gate",
		"UpdateFlagStatistics":  "nodelist import: post-import aggregation",
		"insertPointsSQL":       "pointlist import: bulk INSERT",
		"IsPointlistImported":   "pointlist import: already-imported gate",
		"RegisterPointlistFile": "pointlist import: gate registration",
		"DeletePointlistData":   "pointlist import: -reimport replay",
		"NearestPointsCount":    "pointlist import: shrink-check sanity guard",
	}
	if len(allowed) == 0 {
		t.Fatal("allowlist emptied by accident - the guard would still pass but stop documenting anything")
	}

	// `.Query(`, `.QueryRow(` and `.Exec(` - the three that take no context.
	// Written so that the *Context spellings do not match: the name must be
	// followed directly by an open paren.
	bare := regexp.MustCompile(`\.(Query|QueryRow|Exec)\(`)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	var offenders []string

	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if _, ok := allowed[fn.Name.Name]; ok {
				continue
			}
			// Positions are FileSet-global, not file-local, so they have to be
			// resolved back to an offset before indexing into src.
			body := string(src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
			for _, m := range bare.FindAllString(body, -1) {
				offenders = append(offenders, fmt.Sprintf(
					"%s: %s uses %s - use %sContext(ctx, ...) so the caller can cancel it",
					path, fn.Name.Name, m, strings.TrimSuffix(strings.TrimPrefix(m, "."), "(")))
			}
		}
	}

	if len(offenders) > 0 {
		t.Errorf("uncancellable database calls outside the import path:\n\t%s",
			strings.Join(offenders, "\n\t"))
	}
}

// TestQueryContextAllowlistIsNecessary keeps the allowlist from outliving the
// code it excuses, and from pre-excusing code that does not need excusing.
//
// Name-existence is not enough. An entry naming a function that no longer
// makes a bare call is a standing exemption for whatever that function does
// next - and the first draft of this file carried three of them, one being
// StoreModemTestResult, which is reachable from POST /api/modem/results/direct.
// A bare Exec added there later would have passed the guard silently.
func TestQueryContextAllowlistIsNecessary(t *testing.T) {
	// Rebuilt here rather than shared, so that shrinking the list above cannot
	// accidentally shrink the check.
	allowed := []string{
		"InsertNodesInChunks", "FindConflictingNode", "IsNodelistProcessed",
		"UpdateFlagStatistics", "insertPointsSQL", "IsPointlistImported",
		"RegisterPointlistFile", "DeletePointlistData", "NearestPointsCount",
	}

	bare := regexp.MustCompile(`\.(Query|QueryRow|Exec)\(`)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	// name -> does its body still contain a context-free call?
	needed := map[string]bool{}
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			body := src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset]
			if bare.Match(body) {
				needed[fn.Name.Name] = true
			}
		}
	}

	for _, name := range allowed {
		if !needed[name] {
			t.Errorf("allowlist entry %q no longer makes a context-free call - "+
				"remove it, or it silently exempts whatever that function does next", name)
		}
	}
}
