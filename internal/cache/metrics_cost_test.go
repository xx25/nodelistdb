package cache

import (
	"fmt"
	"testing"
	"time"
)

// internal/storage increments hit/miss counters as
// atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1), from 110 call sites - once
// per cache hit and once per miss. GetMetrics used to refresh Size and Keys,
// and refreshing Keys means iterating the whole keyspace, so every cache
// operation was O(keys). This pins the accessor as a plain read.
//
// The assertion is behavioural rather than a timing measurement: a stale Keys
// is the observable signature of "no scan happened", and it cannot flake.
func TestGetMetricsDoesNotScanTheKeyspace(t *testing.T) {
	bc := budgetCache(t, 0) // GCInterval 1h, so nothing refreshes behind the test

	const keys = 50
	for i := 0; i < keys; i++ {
		mustSet(t, bc, fmt.Sprintf("k%03d", i), "v")
	}

	// NewBadgerCache scanned an empty store, and nothing has scanned since.
	if got := bc.GetMetrics().Keys; got != 0 {
		t.Errorf("GetMetrics().Keys = %d, want 0 - reading metrics must not scan the keyspace", got)
	}

	// The counters callers actually increment stay exact and immediate.
	if got := bc.GetMetrics().Sets; got != keys {
		t.Errorf("GetMetrics().Sets = %d, want %d - counters must not go stale", got, keys)
	}

	// A GC tick is what refreshes the expensive pair.
	bc.performGC()
	if got := bc.GetMetrics().Keys; got != keys {
		t.Errorf("GetMetrics().Keys = %d after a GC tick, want %d", got, keys)
	}
}

// The memory backend counts keys with len() under a read lock, which is O(1),
// so it refreshes on every call and its Keys is always live. Pinned so the two
// backends' differing freshness is a decision rather than a surprise.
func TestMemoryGetMetricsKeysStayLive(t *testing.T) {
	mc := NewMemoryCache(&MemoryConfig{GCInterval: time.Hour})
	defer mc.Close()

	for i := 0; i < 10; i++ {
		if err := mc.Set(t.Context(), fmt.Sprintf("k%02d", i), []byte("v"), time.Hour); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}

	if got := mc.GetMetrics().Keys; got != 10 {
		t.Errorf("GetMetrics().Keys = %d, want 10", got)
	}
}
