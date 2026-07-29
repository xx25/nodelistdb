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

	"github.com/nodelistdb/internal/cache"
)

func newKeyTestStorage() *CachedStorage {
	return &CachedStorage{keyGen: cache.NewKeyGenerator("ndb")}
}

// TestAnalyticsKeyFormat pins what analyticsKey produces. The 35 one-line
// Sprintf methods it replaced used %d/%t/%s per argument; %v renders each of
// those identically, which is why the collapse cost no cache entries.
func TestAnalyticsKeyFormat(t *testing.T) {
	cs := newKeyTestStorage()

	for _, tc := range []struct {
		got  string
		want string
	}{
		{cs.analyticsKey("ipv6:enabled", 500, 30, false, "fidonet"), "ndb:analytics:ipv6:enabled:500:30:false:fidonet"},
		{cs.analyticsKey("binkp:software", 365, ""), "ndb:analytics:binkp:software:365:"},
		{cs.analyticsKey("email:trend", "fsxnet"), "ndb:analytics:email:trend:fsxnet"},
		{cs.analyticsKey("whois:results:v4", "*"), "ndb:analytics:whois:results:v4:*"},
		{cs.analyticsKey("geo:hosting"), "ndb:analytics:geo:hosting"},
	} {
		if tc.got != tc.want {
			t.Errorf("analyticsKey = %q, want %q", tc.got, tc.want)
		}
	}
}

// TestAnalyticsKeysAreSweepable is the guard the four un-prefixed keys needed:
// a key built outside the "ndb:analytics:" namespace cannot be reached by
// InvalidateAll or InvalidateAnalytics, so it is not evicted on import - it
// only expires.
func TestAnalyticsKeysAreSweepable(t *testing.T) {
	cs := newKeyTestStorage()
	prefix := cs.keyGen.AnalyticsPrefix()
	if !strings.HasPrefix(cs.analyticsKey("anything", 1), prefix) {
		t.Fatalf("analyticsKey does not build into %q", prefix)
	}

	// No cache wrapper may hand-roll a key: the four that did were unreachable
	// by both invalidators for their whole TTL.
	sources, err := filepath.Glob("cached_storage*.go")
	if err != nil {
		t.Fatal(err)
	}
	literalKey := regexp.MustCompile(`key\s*:?=\s*fmt\.Sprintf\("[^%]`)
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
			if literalKey.MatchString(text) {
				t.Errorf("%s: %s builds a cache key that does not start with the key generator's prefix; "+
					"use cs.analyticsKey or a cache.KeyGenerator method so invalidation can reach it",
					src, fn.Name.Name)
			}
		}
	}
}
