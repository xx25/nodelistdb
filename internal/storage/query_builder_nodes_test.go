package storage

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
)

func intp(v int) *int              { return &v }
func strp2(v string) *string       { return &v }
func boolp(v bool) *bool           { return &v }
func timep(v time.Time) *time.Time { return &v }

// nodeFilterShapes covers every branch of BuildNodesQuery: identity-only (the
// fast path), attribute-only, mixed, and both LatestOnly variants.
var nodeFilterShapes = map[string]database.NodeFilter{
	"empty":                  {},
	"identity only":          {Zone: intp(2), Limit: 5},
	"full identity":          {Domain: strp2("fidonet"), Zone: intp(2), Net: intp(5001), Node: intp(100), Limit: 10},
	"attribute only":         {Location: strp2("moscow"), Limit: 100},
	"mixed":                  {Zone: intp(2), Location: strp2("moscow"), SysopName: strp2("doe"), Limit: 100, Offset: 20},
	"dates":                  {DateFrom: timep(time.Now().Add(-24 * time.Hour)), DateTo: timep(time.Now())},
	"flags":                  {IsCM: boolp(true), IsMO: boolp(true), HasInet: boolp(true), HasBinkp: boolp(false)},
	"latest only":            {LatestOnly: boolp(true), Limit: 100},
	"latest only + identity": {LatestOnly: boolp(true), Zone: intp(2), Limit: 5},
	"latest only + mixed":    {LatestOnly: boolp(true), Zone: intp(2), NodeType: strp2("Node"), Limit: 5, Offset: 3},
	"everything":             {Domain: strp2("fsxnet"), Zone: intp(21), Net: intp(1), Location: strp2("x"), IsCM: boolp(true), Limit: 50, Offset: 5},
}

// TestBuildNodesQueryBindCounts is the invariant that a placeholder/argument
// mismatch fails at build time rather than at query time. It matters here
// because identity predicates are deliberately emitted twice - once inside a
// subquery and once on the outer query - so the argument list has to repeat
// them in the same order the placeholders appear.
func TestBuildNodesQueryBindCounts(t *testing.T) {
	qb := NewQueryBuilder()

	for name, filter := range nodeFilterShapes {
		t.Run(name, func(t *testing.T) {
			query, args := qb.BuildNodesQuery(filter)
			if got, want := strings.Count(query, "?"), len(args); got != want {
				t.Errorf("%d placeholders but %d args\n%s", got, want, query)
			}
		})
	}
}

// TestBuildNodesQueryIsSyntacticallySane checks the structural properties that
// a result-set diff cannot catch, because a query with either defect never runs.
func TestBuildNodesQueryIsSyntacticallySane(t *testing.T) {
	qb := NewQueryBuilder()

	for name, filter := range nodeFilterShapes {
		t.Run(name, func(t *testing.T) {
			query, _ := qb.BuildNodesQuery(filter)

			// A single SELECT cannot carry two ORDER BY clauses; appending a
			// generic footer to a query that already ordered itself produced
			// exactly this and failed with SYNTAX_ERROR at runtime.
			if n := strings.Count(query, "ORDER BY"); n != 1 {
				t.Errorf("expected exactly 1 ORDER BY, got %d\n%s", n, query)
			}
			if strings.Contains(query, "LIMIT 1 BY") {
				// The limit must stay in the same SELECT as LIMIT 1 BY, after
				// it - that adjacency is what lets ClickHouse stop reading
				// early. A derived-table wrapper is correct but ~50x slower.
				if strings.Contains(query, ") ranked") {
					t.Errorf("LIMIT 1 BY wrapped in a derived table - forfeits early stop\n%s", query)
				}
				if filter.Limit > 0 {
					if strings.LastIndex(query, "LIMIT 1 BY") > strings.LastIndex(query, "LIMIT ") {
						t.Errorf("outer LIMIT must follow LIMIT 1 BY\n%s", query)
					}
				}
			}
			if filter.Offset > 0 && !strings.Contains(query, "OFFSET") {
				t.Errorf("offset requested but absent\n%s", query)
			}
		})
	}
}

// TestBuildNodesQueryPicksTheRightShape pins the shape choice, which is where
// the performance lives: an identity-only filter must not go through the
// history subquery, and an attribute filter must.
func TestBuildNodesQueryPicksTheRightShape(t *testing.T) {
	qb := NewQueryBuilder()

	t.Run("identity only skips the history subquery", func(t *testing.T) {
		query, args := qb.BuildNodesQuery(database.NodeFilter{Zone: intp(2), Limit: 5})
		if strings.Contains(query, "SELECT DISTINCT") {
			t.Errorf("identity-only filter still pre-scans history; this is the 30.9s -> 0.33s case\n%s", query)
		}
		if !strings.Contains(query, "LIMIT 1 BY domain, zone, net, node") {
			t.Errorf("expected per-node dedup\n%s", query)
		}
		if len(args) != 1 {
			t.Errorf("expected 1 arg (zone), got %d", len(args))
		}
	})

	t.Run("attribute filter keeps the history subquery", func(t *testing.T) {
		query, _ := qb.BuildNodesQuery(database.NodeFilter{Location: strp2("moscow"), Limit: 5})
		if !strings.Contains(query, "SELECT DISTINCT") {
			t.Errorf("attribute filter must match on any historical row\n%s", query)
		}
	})

	t.Run("latest only never collapses conflict rows", func(t *testing.T) {
		query, _ := qb.BuildNodesQuery(database.NodeFilter{LatestOnly: boolp(true), Zone: intp(2)})
		if strings.Contains(query, "LIMIT 1 BY") {
			t.Errorf("LIMIT 1 BY drops duplicate-nodelist-entry rows this shape must keep\n%s", query)
		}
		if !strings.Contains(query, "MAX(nodelist_date)") {
			t.Errorf("expected max-date shape\n%s", query)
		}
	})

	t.Run("latest only pushes identity into the max subquery", func(t *testing.T) {
		query, args := qb.BuildNodesQuery(database.NodeFilter{LatestOnly: boolp(true), Zone: intp(2)})
		sub := query[strings.Index(query, "MAX(nodelist_date)"):strings.Index(query, "GROUP BY")]
		if !strings.Contains(sub, "zone = ?") {
			t.Errorf("identity predicate not pushed into the max subquery\n%s", query)
		}
		// zone appears twice: once in the subquery, once on the outer query.
		if len(args) != 2 {
			t.Errorf("expected zone bound twice, got %d args", len(args))
		}
	})

	t.Run("latest only does not push attributes into the max subquery", func(t *testing.T) {
		query, _ := qb.BuildNodesQuery(database.NodeFilter{LatestOnly: boolp(true), DateFrom: timep(time.Now())})
		sub := query[strings.Index(query, "MAX(nodelist_date)"):strings.Index(query, "GROUP BY")]
		if strings.Contains(sub, "nodelist_date >=") {
			t.Errorf("date pushed into max subquery: would return the newest row inside the window, not the node's latest\n%s", query)
		}
	})
}

// TestBuildFTSQueryDelegates keeps text searches on one definition of
// latest_only. BuildFTSQuery used to answer "who is listed right now" while
// BuildNodesQuery answered "one row per node", and callers got whichever one
// their query's shape happened to select.
func TestBuildFTSQueryDelegates(t *testing.T) {
	qb := NewQueryBuilder()

	for _, latestOnly := range []*bool{nil, boolp(false), boolp(true)} {
		filter := database.NodeFilter{SysopName: strp2("Ivanov"), LatestOnly: latestOnly, Limit: 100}

		got, gotArgs, usedFTS := qb.BuildFTSQuery(filter)
		if !usedFTS {
			t.Fatalf("text filter must produce a usable query (latestOnly=%v)", latestOnly)
		}
		want, wantArgs := qb.BuildNodesQuery(filter)
		if got != want {
			t.Errorf("text search diverges from BuildNodesQuery (latestOnly=%v)\ngot:  %s\nwant: %s", latestOnly, got, want)
		}
		if len(gotArgs) != len(wantArgs) {
			t.Errorf("arg count %d != %d (latestOnly=%v)", len(gotArgs), len(wantArgs), latestOnly)
		}
		// The retired shape restricted to each domain's newest nodelist.
		if strings.Contains(got, "GROUP BY domain)") {
			t.Errorf("domain-only max is the retired active_only meaning, not latest_only\n%s", got)
		}
	}

	// No text term: the caller falls through to BuildNodesQuery itself.
	if _, _, usedFTS := qb.BuildFTSQuery(database.NodeFilter{Zone: intp(2)}); usedFTS {
		t.Error("a filter with no text term must not claim a usable FTS query")
	}
	// An empty text term is not a text search.
	if _, _, usedFTS := qb.BuildFTSQuery(database.NodeFilter{SysopName: strp2("")}); usedFTS {
		t.Error("empty text term must not count as a text search")
	}
}

// TestNodeFilterFieldsAreClassified fails when a field is added to NodeFilter
// without deciding whether it identifies a node or describes one moment of its
// history. Getting that wrong picks the wrong query shape and silently changes
// results, so the omission has to be loud.
func TestNodeFilterFieldsAreClassified(t *testing.T) {
	classified := map[string]string{
		// Invariant across a node's history - safe to apply to a row directly.
		"Zone": "identity", "Net": "identity", "Node": "identity", "Domain": "identity",
		// Describe one moment - must be matched across history.
		"DateFrom": "attribute", "DateTo": "attribute", "SystemName": "attribute",
		"Location": "attribute", "SysopName": "attribute", "NodeType": "attribute",
		"IsCM": "attribute", "HasInet": "attribute", "HasBinkp": "attribute",
		"IsMO": "attribute",
		// Not predicates.
		"LatestOnly": "option", "Limit": "option", "Offset": "option",
	}

	ft := reflect.TypeOf(database.NodeFilter{})
	for i := 0; i < ft.NumField(); i++ {
		name := ft.Field(i).Name
		if _, ok := classified[name]; !ok {
			t.Errorf("NodeFilter.%s is unclassified - decide whether it is identity "+
				"(invariant per node) or attribute (varies over history) and add it to "+
				"buildFilterConditions, then record it here", name)
		}
	}
	for name := range classified {
		if _, ok := ft.FieldByName(name); !ok {
			t.Errorf("classification lists NodeFilter.%s, which no longer exists", name)
		}
	}
}
