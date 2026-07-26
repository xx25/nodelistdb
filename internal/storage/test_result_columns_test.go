package storage

import (
	"strings"
	"testing"
)

// testResultColumns is the number of columns ParseTestResultRow scans, and
// therefore the number every SELECT that feeds it must return, in the same
// order. ClickHouse rejects the query outright when the counts differ, so a
// column added to the table has to reach BOTH the parser and every query
// listed below. This is the read-side counterpart of resultToValuesColumns in
// internal/testing/storage, which pins the write side.
const testResultColumns = 120

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
	if cs.n != testResultColumns {
		t.Fatalf("ParseTestResultRow scans %d destinations, want %d (update the queries and this constant together)", cs.n, testResultColumns)
	}
}

func TestTestResultQueryColumnCounts(t *testing.T) {
	// Every one of these feeds ParseTestResultRow, which scans by position. So
	// they must not merely return the right NUMBER of columns — they must
	// return the same columns in the same ORDER, both as each other and as the
	// parser expects. Comparing the sequences against one another catches a
	// transposition inside a run of same-typed columns, which a count alone
	// cannot see.
	names := []string{
		"BuildTestHistoryQuery",
		"BuildDetailedTestResultQuery",
		"BuildProtocolEnabledQuery",
		"BuildSearchByReachabilityQuery",
	}
	qb := NewTestQueryBuilder(nil)
	queries := []string{
		qb.BuildTestHistoryQuery(),
		qb.BuildDetailedTestResultQuery(),
		qb.BuildProtocolEnabledQuery("binkp", "", "", 30),
		qb.BuildSearchByReachabilityQuery(),
	}

	var want []string
	for i, query := range queries {
		got := outerSelectColumns(t, query)
		if len(got) != testResultColumns {
			t.Errorf("%s selects %d columns, want %d (it feeds ParseTestResultRow)", names[i], len(got), testResultColumns)
			continue
		}
		if want == nil {
			want = got
			continue
		}
		for j := range got {
			if got[j] != want[j] {
				t.Errorf("%s column %d is %q, but %s has %q there — positional scans require identical order",
					names[i], j+1, got[j], names[0], want[j])
				break
			}
		}
	}
}

// outerSelectColumns returns the column names of the projection that ends at
// derived_from_address — the last column every test-result query returns —
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
