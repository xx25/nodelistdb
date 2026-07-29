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

// testResultColumnCount is the number of columns ParseTestResultRow scans, and
// therefore the number every SELECT that feeds it must return, in the same
// order. It is derived from testResultColumnGroups rather than written down,
// so the one place a column is added is the one place that has to be right.
// This is the read-side counterpart of resultToValuesColumns in
// internal/testing/storage, which pins the write side.
func testResultColumnCount() int {
	n := 0
	for _, group := range testResultColumnGroups {
		n += len(group)
	}
	return n
}

// countingScanner records how many destinations a parser asks for.
type countingScanner struct{ n int }

func (c *countingScanner) Scan(dest ...interface{}) error {
	c.n = len(dest)
	return nil
}

func TestParseTestResultRowScanArity(t *testing.T) {
	var result NodeTestResult
	cs := &countingScanner{}
	if err := NewResultParser().ParseTestResultRow(cs, &result); err != nil {
		t.Fatalf("ParseTestResultRow returned %v", err)
	}
	if want := testResultColumnCount(); cs.n != want {
		t.Fatalf("ParseTestResultRow scans %d destinations but testResultColumnGroups lists %d columns; "+
			"a column reached one and not the other, and every field after it now holds its neighbour's value", cs.n, want)
	}
}

// TestTestResultProjectionRendering pins what the three markers expand to: the
// same columns in the same order, differing only in the alias they hang off and
// in the two expressions that prefer the nodelist's system name.
func TestTestResultProjectionRendering(t *testing.T) {
	for _, tc := range []struct {
		marker     string
		wantPrefix string
		overridden []string
	}{
		{testColumnsMarker, "", nil},
		{testColumnsMarkerR, "r.", nil},
		{testColumnsMarkerRRNodeName, "rr.", []string{"binkp_system_name", "ifcico_system_name"}},
	} {
		sql := applyTestResultColumns(tc.marker)
		cols := selectListColumns(t, sql)

		if len(cols) != testResultColumnCount() {
			t.Fatalf("%s renders %d columns, want %d", tc.marker, len(cols), testResultColumnCount())
		}

		var flat []string
		for _, group := range testResultColumnGroups {
			flat = append(flat, group...)
		}
		for i, got := range cols {
			if got != flat[i] {
				t.Fatalf("%s column %d is %q, want %q", tc.marker, i+1, got, flat[i])
			}
		}

		// The alias has to be on every column that is not overridden, or the
		// query is ambiguous the moment it joins a second table.
		for _, group := range testResultColumnGroups {
			for _, col := range group {
				if contains(tc.overridden, col) {
					continue
				}
				if !strings.Contains(sql, tc.wantPrefix+col) {
					t.Errorf("%s: column %s is not qualified with %q", tc.marker, col, tc.wantPrefix)
				}
			}
		}
		for _, col := range tc.overridden {
			if !strings.Contains(sql, "COALESCE(NULLIF(n.system_name, ''), "+tc.wantPrefix+col+") as "+col) {
				t.Errorf("%s: %s is not read from the joined nodelist row", tc.marker, col)
			}
		}
	}
}

// TestTestResultQueryColumnCounts renders every query builder that feeds
// ParseTestResultRow and compares their projections against one another. The
// parser scans by position, so they must return the same columns in the same
// ORDER, not merely the same number of them.
func TestTestResultQueryColumnCounts(t *testing.T) {
	qb := NewTestQueryBuilder()
	am := &AKAMismatchOperations{}
	queries := []struct {
		name string
		sql  string
	}{
		{"BuildTestHistoryQuery", qb.BuildTestHistoryQuery()},
		{"BuildDetailedTestResultQuery", qb.BuildDetailedTestResultQuery()},
		{"BuildProtocolEnabledQuery", qb.BuildProtocolEnabledQuery("binkp", "", "", 30)},
		{"BuildVModemUnconfirmedQuery", qb.BuildVModemUnconfirmedQuery("", "", 30)},
		{"BuildSearchByReachabilityQuery", qb.BuildSearchByReachabilityQuery()},
		{"buildAKAMismatchQuery", am.buildAKAMismatchQuery("", "fidonet", 30)},
	}

	var want []string
	for _, q := range queries {
		got := outerSelectColumns(t, q.sql)
		if len(got) != testResultColumnCount() {
			t.Errorf("%s selects %d columns, want %d (it feeds ParseTestResultRow)", q.name, len(got), testResultColumnCount())
			continue
		}
		if want == nil {
			want = got
			continue
		}
		for j := range got {
			if got[j] != want[j] {
				t.Errorf("%s column %d is %q, but %s has %q there - positional scans require identical order",
					q.name, j+1, got[j], queries[0].name, want[j])
				break
			}
		}
	}
}

// TestNoHandWrittenTestResultProjection is what keeps the list in one place.
// The projection used to be spelled out at 24 sites across four files, which is
// how it drifted: adding a column meant finding every copy, and the failure of
// missing one is a silently shifted result set, not an error.
//
// The signature of a copy is unmistakable: the projection's opening run of
// columns, optionally table-qualified, followed somewhere by
// derived_from_address. Two neighbouring queries (GetNodesInNetwork,
// buildIPVersionMismatchQuery) open with the same run but feed their own
// scanners with their own shorter column sets, and stop well before
// derived_from_address.
func TestNoHandWrittenTestResultProjection(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	for _, src := range sources {
		if strings.HasSuffix(src, "_test.go") || src == "test_result_columns.go" {
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
			if projectionOpenerRe.MatchString(text) && strings.Contains(text, "derived_from_address") {
				t.Errorf("%s: %s spells out the ParseTestResultRow projection; use one of the "+
					"{{TEST_RESULT_COLUMNS*}} markers so it cannot drift from the parser",
					src, fn.Name.Name)
			}
		}
	}
}

// projectionOpenerRe matches the first line of the ParseTestResultRow
// projection, with or without a table alias on each column.
var projectionOpenerRe = regexp.MustCompile(
	`(?:\w+\.)?test_time,\s*(?:\w+\.)?zone,\s*(?:\w+\.)?net,\s*(?:\w+\.)?node,\s*(?:\w+\.)?address,\s*(?:\w+\.)?hostname`)

func contains(list []string, v string) bool {
	for _, e := range list {
		if e == v {
			return true
		}
	}
	return false
}

// selectListColumns normalizes a bare projection (no surrounding SELECT).
func selectListColumns(t *testing.T, list string) []string {
	t.Helper()
	return splitSelectList(list)
}

// outerSelectColumns returns the column names of the projection that ends at
// derived_from_address - the last column every test-result query returns -
// back to the SELECT that opens it. Commas inside parentheses (COALESCE, window
// functions) do not separate columns; a table alias is dropped and an AS alias
// wins, so `COALESCE(n.system_name, r.binkp_system_name) as binkp_system_name`
// and a plain `binkp_system_name` compare equal across queries.
func outerSelectColumns(t *testing.T, query string) []string {
	t.Helper()

	end := strings.LastIndex(query, "derived_from_address")
	if end < 0 {
		t.Fatalf("query does not select derived_from_address:\n%s", query)
	}
	end += len("derived_from_address")
	start := strings.LastIndex(query[:end], "SELECT")
	if start < 0 {
		t.Fatalf("no SELECT before derived_from_address:\n%s", query)
	}

	list := query[start+len("SELECT") : end]
	if i := strings.Index(list, "DISTINCT ON ("); i >= 0 {
		if j := strings.Index(list[i:], ")"); j >= 0 {
			list = list[i+j+1:]
		}
	}
	return splitSelectList(list)
}

func splitSelectList(list string) []string {
	var cols []string
	var cur strings.Builder
	depth := 0
	flush := func() {
		if name := normalizeColumn(cur.String()); name != "" {
			cols = append(cols, name)
		}
		cur.Reset()
	}
	for _, r := range list {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				flush()
				continue
			}
		}
		cur.WriteRune(r)
	}
	flush()
	return cols
}

// normalizeColumn reduces one select-list expression to the name the row
// carries: its AS alias when it has one, otherwise the column without its
// table alias.
func normalizeColumn(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	lower := strings.ToLower(expr)
	if i := strings.LastIndex(lower, " as "); i >= 0 {
		expr = strings.TrimSpace(expr[i+len(" as "):])
	}
	if i := strings.LastIndex(expr, "."); i >= 0 {
		expr = expr[i+1:]
	}
	return strings.TrimSpace(expr)
}
