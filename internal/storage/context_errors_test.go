package storage

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestContextErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"ordinary failure", errors.New("connection refused"), false},
		{"cancelled", context.Canceled, true},
		{"deadline", context.DeadlineExceeded, true},
		// Every storage method wraps what the driver returned, often twice, so
		// matching the bare sentinel would stop working the moment a wrapper
		// was added. That is exactly how these arrive at the swallow sites.
		{"wrapped cancelled", fmt.Errorf("a: %w", fmt.Errorf("b: %w", context.Canceled)), true},
		{"wrapped deadline", fmt.Errorf("a: %w", fmt.Errorf("b: %w", context.DeadlineExceeded)), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := contextErr(tc.err) != nil; got != tc.want {
				t.Errorf("contextErr(%v) non-nil = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestSecondaryFetchesAbortOnContextError is a source guard, not a behaviour
// test: the sites it covers need a live ClickHouse and a mid-flight
// cancellation to exercise, which no unit test here can arrange.
//
// What it pins is the rule those four sites now follow. Each runs a secondary
// query whose failure is deliberately tolerated - a missing dead-node set, a
// missing hostname list - and each sits behind CachedStorage. Tolerating a
// context error the same way returns a half-built answer as a success, and the
// cache then serves it for the rest of the TTL. For GetPSTNNodes that means an
// operator-marked-dead phone number reads as callable to the modem tester for
// an hour.
//
// So: wherever a tolerated secondary failure is followed by a cached success,
// contextErr must be consulted first.
func TestSecondaryFetchesAbortOnContextError(t *testing.T) {
	sites := []struct {
		file string
		fn   string
		call string
		why  string
	}{
		{"analytics_operations.go", "GetPSTNNodes", "GetDeadNodeSet(ctx)",
			"every operator-marked-dead phone number would be cached as callable"},
		{"point_operations.go", "SearchPointsWithLifetime", "latestPointlistDateLocked(ctx",
			"every point in the domain would be cached as Historical"},
		{"test_ipv6_queries.go", "GetIPv6EnabledNodes", "getAllHostnamesForNode(ctx", ipv6Why},
		{"test_ipv6_queries.go", "GetIPv6NonWorkingNodes", "getAllHostnamesForNode(ctx", ipv6Why},
		{"test_ipv6_queries.go", "GetIPv6AdvertisedIPv4OnlyNodes", "getAllHostnamesForNode(ctx", ipv6Why},
		{"test_ipv6_queries.go", "GetIPv6OnlyNodes", "getAllHostnamesForNode(ctx", ipv6Why},
		{"test_ipv6_queries.go", "GetPureIPv6OnlyNodes", "getAllHostnamesForNode(ctx", ipv6Why},
	}

	for _, site := range sites {
		t.Run(site.file+"/"+site.fn, func(t *testing.T) {
			body := functionBody(t, site.file, site.fn)
			if !strings.Contains(body, site.call) {
				t.Fatalf("%s no longer calls %s - if the call moved, move this guard with it",
					site.fn, site.call)
			}
			if !strings.Contains(body, "contextErr") {
				t.Errorf("%s tolerates a failure of %s without consulting contextErr - %s",
					site.fn, site.call, site.why)
			}
		})
	}
}

const ipv6Why = "rows with missing AllHostnames would be cached as complete"

// TestGetNodeChangesChecksContextBeforeInventingRemoval covers the fourth site,
// which needs a different shape: isCurrentlyActive returns a bare bool and
// reads false on any error, so a cancelled request would append a fabricated
// "removed" event with an invented date - and cache it. GetNodeChanges asks the
// context directly instead.
func TestGetNodeChangesChecksContextBeforeInventingRemoval(t *testing.T) {
	body := functionBody(t, "search_operations_changes.go", "GetNodeChanges")
	active := strings.Index(body, "isCurrentlyActive(")
	if active < 0 {
		t.Fatal("GetNodeChanges no longer calls isCurrentlyActive - move this guard with it")
	}
	if !strings.Contains(body[:active], "ctx.Err()") {
		t.Error("GetNodeChanges reaches isCurrentlyActive without checking ctx.Err() first; " +
			"a cancelled request would fabricate a \"removed\" event and cache it")
	}
}

// functionBody returns the source text of one function's body.
func functionBody(t *testing.T, file, name string) string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, src, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name.Name != name {
			continue
		}
		return string(src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
	}
	t.Fatalf("%s: no function named %s", file, name)
	return ""
}
