package cache

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// backends runs fn against every Cache implementation, so the DeleteByPrefix
// contract is asserted for both the ordered-keyspace Badger scan and the
// map iteration in MemoryCache.
func backends(t *testing.T, fn func(t *testing.T, c Cache)) {
	t.Helper()

	t.Run("memory", func(t *testing.T) {
		c := NewMemoryCache(&MemoryConfig{GCInterval: time.Hour})
		defer c.Close()
		fn(t, c)
	})

	t.Run("badger", func(t *testing.T) {
		c, err := NewBadgerCache(&BadgerConfig{
			Path: t.TempDir(),
			// 64MB is DefaultConfig()'s budget. Much below that and Badger
			// refuses to open: the derived memtable size puts its default 1MB
			// value threshold over the max batch size.
			MaxMemoryMB: 64,
			GCInterval:  time.Hour,
		})
		if err != nil {
			t.Fatalf("open badger: %v", err)
		}
		defer c.Close()
		fn(t, c)
	})
}

func seed(t *testing.T, c Cache, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := c.Set(context.Background(), k, []byte("v"), time.Hour); err != nil {
			t.Fatalf("seed %q: %v", k, err)
		}
	}
}

func present(t *testing.T, c Cache, key string) bool {
	t.Helper()
	_, err := c.Get(context.Background(), key)
	return err == nil
}

func assertGone(t *testing.T, c Cache, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if present(t, c, k) {
			t.Errorf("key %q should have been deleted, but is still cached", k)
		}
	}
}

func assertKept(t *testing.T, c Cache, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if !present(t, c, k) {
			t.Errorf("key %q should have survived, but was deleted", k)
		}
	}
}

// TestDeleteByPrefixNamespace covers the ordinary bulk-invalidation case: one
// namespace goes, its neighbours stay.
func TestDeleteByPrefixNamespace(t *testing.T) {
	backends(t, func(t *testing.T, c Cache) {
		kg := NewKeyGenerator("ndb")

		statsKey := kg.StatsKey(time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC))
		latestKey := kg.LatestStatsDateKey()
		nodeKey := kg.NodeKey(2, 5001, 100)
		datesKey := kg.AvailableDatesKey()

		seed(t, c, statsKey, latestKey, nodeKey, datesKey)

		if err := c.DeleteByPrefix(context.Background(), kg.StatsPrefix()); err != nil {
			t.Fatalf("DeleteByPrefix: %v", err)
		}

		assertGone(t, c, statsKey, latestKey)
		assertKept(t, c, nodeKey, datesKey)
	})
}

// TestDeleteByPrefixDelimiterBoundary pins the decimal-prefix hazard: node 100
// and node 1000 share a textual prefix, so a prefix that does not end on the
// ":" delimiter reaches into the wrong node's entries. Both halves are asserted
// so the trap stays documented rather than being rediscovered.
func TestDeleteByPrefixDelimiterBoundary(t *testing.T) {
	backends(t, func(t *testing.T, c Cache) {
		kg := NewKeyGenerator("ndb")

		// "ndb:changes:2:5001:100:fidonet" vs "ndb:changes:2:5001:1000:fidonet"
		target := kg.NodeChangesKey(2, 5001, 100, "fidonet")
		sibling := kg.NodeChangesKey(2, 5001, 1000, "fidonet")

		t.Run("delimiter terminated spares the sibling", func(t *testing.T) {
			seed(t, c, target, sibling)

			if err := c.DeleteByPrefix(context.Background(), "ndb:changes:2:5001:100:"); err != nil {
				t.Fatalf("DeleteByPrefix: %v", err)
			}

			assertGone(t, c, target)
			assertKept(t, c, sibling)
		})

		t.Run("unterminated also catches the sibling", func(t *testing.T) {
			seed(t, c, target, sibling)

			// Deliberately missing the trailing ":" - this is the shape a
			// caller must avoid, and the reason DeleteByPrefix documents it.
			if err := c.DeleteByPrefix(context.Background(), "ndb:changes:2:5001:100"); err != nil {
				t.Fatalf("DeleteByPrefix: %v", err)
			}

			assertGone(t, c, target, sibling)
		})
	})
}

// TestDeleteByPrefixRejectsWildcard is the regression guard for the original
// defect: a glob was accepted, matched nothing because both backends compare a
// literal prefix, and returned nil. The exact old pattern is used as input.
func TestDeleteByPrefixRejectsWildcard(t *testing.T) {
	backends(t, func(t *testing.T, c Cache) {
		kg := NewKeyGenerator("ndb")
		nodeKey := kg.NodeKey(2, 5001, 100)
		seed(t, c, nodeKey)

		wildcards := []string{
			"ndb:*:2:5001:100*", // the pattern the deleted NodePattern emitted
			"ndb:stats:*",       // trailing-only, previously trimmed and honoured
			"*",
		}

		for _, w := range wildcards {
			err := c.DeleteByPrefix(context.Background(), w)
			if !errors.Is(err, ErrWildcardPrefix) {
				t.Errorf("DeleteByPrefix(%q) error = %v, want ErrWildcardPrefix", w, err)
			}
		}

		// A rejected call must not delete anything.
		assertKept(t, c, nodeKey)
	})
}

// TestInvalidationPrefixesAreLiteral guards every generator feeding
// DeleteByPrefix: no wildcard, and terminated on the delimiter so it cannot
// bleed into an adjacent namespace.
func TestInvalidationPrefixesAreLiteral(t *testing.T) {
	kg := NewKeyGenerator("ndb")

	prefixes := map[string]string{
		"AllPrefix":       kg.AllPrefix(),
		"StatsPrefix":     kg.StatsPrefix(),
		"SearchPrefix":    kg.SearchPrefix(),
		"SysopsPrefix":    kg.SysopsPrefix(),
		"DatesPrefix":     kg.DatesPrefix(),
		"AnalyticsPrefix": kg.AnalyticsPrefix(),
	}

	for name, p := range prefixes {
		if strings.Contains(p, "*") {
			t.Errorf("%s() = %q, must be a literal prefix with no wildcard", name, p)
		}
		if !strings.HasSuffix(p, ":") {
			t.Errorf("%s() = %q, must end on the \":\" delimiter", name, p)
		}
	}
}

// TestInvalidationPrefixesMatchTheirKeys ties each prefix to a real key from
// its namespace, so renaming a key family without its prefix fails here.
func TestInvalidationPrefixesMatchTheirKeys(t *testing.T) {
	kg := NewKeyGenerator("ndb")
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		prefix string
		key    string
	}{
		{kg.StatsPrefix(), kg.StatsKey(now)},
		{kg.StatsPrefix(), kg.LatestStatsDateKey()},
		{kg.SearchPrefix(), kg.SearchKey(map[string]string{"zone": "2"})},
		{kg.SysopsPrefix(), kg.UniqueSysopsKey("smith", 10, 0)},
		{kg.DatesPrefix(), kg.AvailableDatesKey()},
		{kg.DatesPrefix(), kg.NearestDateKey(now)},
		{kg.AllPrefix(), kg.NodeKey(2, 5001, 100)},
		{kg.AllPrefix(), kg.NodeHistoryKey(2, 5001, 100)},
	}

	for _, c := range cases {
		if !strings.HasPrefix(c.key, c.prefix) {
			t.Errorf("key %q is not covered by prefix %q", c.key, c.prefix)
		}
	}

	// The analytics namespace is built by CachedStorage.analyticsKey rather
	// than by a method here, so its prefix is pinned in
	// internal/storage (TestAnalyticsKeysAreSweepable). What this file still
	// owns is the prefix string itself.
	if want := "ndb:analytics:"; kg.AnalyticsPrefix() != want {
		t.Errorf("AnalyticsPrefix() = %q, want %q", kg.AnalyticsPrefix(), want)
	}
}
