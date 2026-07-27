package cache

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// TestBadgerOpensAtEveryBudget covers the startup failure that used to make any
// max_memory_mb below 27 fatal: the memtable was shrunk to fit the budget while
// ValueThreshold stayed at Badger's 1MiB default, so Badger refused to open
// because the threshold exceeded its 15%-of-memtable transaction batch limit.
//
// Each case also writes on both sides of the resulting threshold. That matters:
// a value at or above the threshold goes to the value log and is charged ~14
// bytes against the batch limit, but one just below it is charged in full, so
// only the just-below write can catch a threshold left without batch headroom.
func TestBadgerOpensAtEveryBudget(t *testing.T) {
	// 0 bypasses the budgeting block entirely (Badger's own 64MB defaults);
	// 26/27 straddle the old cliff; 64 and 256 are the values this repo
	// actually uses (factory.DefaultConfig, config.yaml, testdaemon).
	budgets := []int{0, 1, 8, 26, 27, 64, 256}

	for _, mb := range budgets {
		t.Run(budgetName(mb), func(t *testing.T) {
			c, err := NewBadgerCache(&BadgerConfig{
				Path:        t.TempDir(),
				MaxMemoryMB: mb,
				GCInterval:  time.Hour,
			})
			if err != nil {
				t.Fatalf("MaxMemoryMB=%d: open failed: %v", mb, err)
			}
			// Close inside the subtest: NewBadgerCache starts a GC goroutine
			// unconditionally, so a deferred close at the parent level would
			// leak one per budget for the life of the test binary.
			defer c.Close()

			threshold := expectedValueThreshold(mb)

			// The largest value that still stays inline: exactly one byte under
			// the threshold once Set's 8-byte header is added. It has to be
			// this tight. Badger charges the transaction len(key)+len(value)+12,
			// so a value even a kilobyte below the threshold still fits under a
			// batch limit equal to it, and the test would pass with the safety
			// margin removed - verified by mutation, which is how this size was
			// arrived at.
			assertRoundTrip(t, c, "inline", int(threshold)-9)

			// At the threshold: diverted to the value log and charged ~14 bytes.
			assertRoundTrip(t, c, "vlog", int(threshold))
		})
	}
}

// expectedValueThreshold mirrors the sizing in NewBadgerCache so the test fails
// if that arithmetic drifts, rather than silently testing whatever it produces.
func expectedValueThreshold(maxMemoryMB int) int64 {
	if maxMemoryMB <= 0 {
		return 1 << 20 // budgeting skipped; Badger's default applies
	}
	memTableSize := int64(maxMemoryMB) << 20 / 4
	if memTableSize < 4<<20 {
		memTableSize = 4 << 20
	}
	threshold := memTableSize * 15 / 100 / 2
	if threshold > 1<<20 {
		threshold = 1 << 20
	}
	return threshold
}

func assertRoundTrip(t *testing.T, c Cache, name string, size int) {
	t.Helper()
	if size <= 0 {
		t.Fatalf("%s: computed a non-positive value size (%d)", name, size)
	}

	value := bytes.Repeat([]byte("x"), size)
	key := "ndb:node:2:5001:100:" + name

	if err := c.Set(context.Background(), key, value, time.Hour); err != nil {
		t.Fatalf("%s: Set of %d bytes failed: %v", name, size, err)
	}

	got, err := c.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("%s: Get of %d bytes failed: %v", name, size, err)
	}
	if !bytes.Equal(got, value) {
		t.Errorf("%s: round-tripped %d bytes, want %d bytes identical", name, len(got), size)
	}
}

func budgetName(mb int) string {
	if mb == 0 {
		return "unbudgeted"
	}
	return "MaxMemoryMB=" + itoa(mb)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
