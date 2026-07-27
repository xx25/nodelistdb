package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Setting max_disk_mb used to arm DropAll() on the GC goroutine with nothing
// excluding readers, which badger documents as a way to panic the process
// rather than a way to bound the disk. These tests pin the replacement:
// suspend writes while over budget, keep serving reads, and reset only when
// draining has demonstrably stalled.
//
// They drive applyDiskBudget with a synthetic size rather than filling a real
// store, because db.Size() is refreshed by badger's own one-minute ticker -
// a size-driven test would spend a minute per transition and still be timing
// dependent. Everything below the measurement is the real code path.

func budgetCache(t *testing.T, maxDiskMB int) *BadgerCache {
	t.Helper()
	bc, err := NewBadgerCache(&BadgerConfig{
		Path:        t.TempDir(),
		MaxMemoryMB: 64,
		MaxDiskMB:   maxDiskMB,
		GCInterval:  time.Hour, // no background ticks; tests drive the policy
	})
	if err != nil {
		t.Fatalf("NewBadgerCache: %v", err)
	}
	t.Cleanup(func() { bc.Close() })
	return bc
}

func mustSet(t *testing.T, bc *BadgerCache, key, value string) {
	t.Helper()
	if err := bc.Set(context.Background(), key, []byte(value), time.Hour); err != nil {
		t.Fatalf("Set(%q): %v", key, err)
	}
}

func mustGet(t *testing.T, bc *BadgerCache, key string) string {
	t.Helper()
	got, err := bc.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("Get(%q): %v", key, err)
	}
	return string(got)
}

// 0 is the shipped value - no config sets max_disk_mb - and must mean "no cap",
// not "a cap of zero that is always exceeded".
func TestDiskBudgetZeroMeansNoCap(t *testing.T) {
	bc := budgetCache(t, 0)

	for i := 0; i < 5; i++ {
		bc.enforceDiskBudget()
	}

	if bc.admissionBlocked.Load() {
		t.Fatal("max_disk_mb: 0 suspended writes; 0 must mean no limit")
	}
	mustSet(t, bc, "k", "v")
	if got := mustGet(t, bc, "k"); got != "v" {
		t.Errorf("Get = %q, want %q", got, "v")
	}
}

// The point of the new policy: over budget costs writes, never reads. Reads
// falling through to ClickHouse is a latency hit; a wiped cache is that hit for
// every key including the ones that were inside the budget.
func TestOverBudgetSuspendsWritesButNotReads(t *testing.T) {
	bc := budgetCache(t, 100)
	mustSet(t, bc, "warm", "cached")

	bc.applyDiskBudget(150)

	if !bc.admissionBlocked.Load() {
		t.Fatal("150MB against a 100MB budget did not suspend writes")
	}

	// A declined write reports success: every call site discards the error, and
	// a full cache declining an entry is normal operation, not a failure.
	if err := bc.Set(context.Background(), "cold", []byte("rejected"), time.Hour); err != nil {
		t.Errorf("Set while suspended returned %v, want nil", err)
	}
	if _, err := bc.Get(context.Background(), "cold"); err == nil {
		t.Error("Set while suspended stored the entry anyway")
	}

	// The pre-existing entry is untouched - this is what DropAll destroyed.
	if got := mustGet(t, bc, "warm"); got != "cached" {
		t.Errorf("Get(warm) = %q, want %q - suspending writes must not drop data", got, "cached")
	}

	if bc.metrics.Rejected != 1 {
		t.Errorf("metrics.Rejected = %d, want 1 - a silent decline needs a counter", bc.metrics.Rejected)
	}
}

func TestSetMultiRejectedWhileSuspended(t *testing.T) {
	bc := budgetCache(t, 100)
	bc.applyDiskBudget(150)

	items := map[string]CacheItem{
		"a": {Value: []byte("1"), TTL: time.Hour},
		"b": {Value: []byte("2"), TTL: time.Hour},
	}
	if err := bc.SetMulti(context.Background(), items); err != nil {
		t.Errorf("SetMulti while suspended returned %v, want nil", err)
	}

	got, err := bc.GetMulti(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("GetMulti: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("SetMulti while suspended stored %d entries, want 0", len(got))
	}
	if bc.metrics.Rejected != uint64(len(items)) {
		t.Errorf("metrics.Rejected = %d, want %d", bc.metrics.Rejected, len(items))
	}
}

// Draining is not guaranteed: badger reclaims expired keys during compaction,
// and compaction is driven by writes, so a suspended cache can sit full of
// expired keys forever. The reset is the backstop for exactly that - but it
// must not fire on the first over-budget tick, which is the behaviour being
// replaced.
func TestResetOnlyAfterDrainingStalls(t *testing.T) {
	bc := budgetCache(t, 100)
	mustSet(t, bc, "warm", "cached")

	for tick := 1; tick < diskBudgetResetAfterTicks; tick++ {
		bc.applyDiskBudget(150)
		if got := mustGet(t, bc, "warm"); got != "cached" {
			t.Fatalf("reset fired on tick %d of %d; it must wait for draining to stall",
				tick, diskBudgetResetAfterTicks)
		}
	}

	bc.applyDiskBudget(150)

	if _, err := bc.Get(context.Background(), "warm"); err == nil {
		t.Errorf("still over budget after %d suspended ticks but no reset", diskBudgetResetAfterTicks)
	}
	if bc.admissionBlocked.Load() {
		t.Error("writes still suspended after a reset; the store is empty now")
	}
	if bc.overBudgetTicks != 0 {
		t.Errorf("overBudgetTicks = %d after reset, want 0", bc.overBudgetTicks)
	}
}

// A tick that finds the cache draining - below the limit but not yet below the
// resume mark - is progress, not a stall, and must not count toward the reset.
// Without this the reset fires on any three ticks that happen to be suspended.
func TestDrainingTicksDoNotCountTowardReset(t *testing.T) {
	bc := budgetCache(t, 100)
	mustSet(t, bc, "warm", "cached")

	bc.applyDiskBudget(150) // suspend
	for i := 0; i < diskBudgetResetAfterTicks*2; i++ {
		bc.applyDiskBudget(95) // under the limit, above the 90MB resume mark
	}

	if got := mustGet(t, bc, "warm"); got != "cached" {
		t.Error("reset fired while the cache was draining normally")
	}
	if !bc.admissionBlocked.Load() {
		t.Error("resumed above the resume mark; that is the flapping the hysteresis prevents")
	}
}

func TestResumesBelowResumeMark(t *testing.T) {
	bc := budgetCache(t, 100)

	bc.applyDiskBudget(150)
	if !bc.admissionBlocked.Load() {
		t.Fatal("did not suspend")
	}

	bc.applyDiskBudget(90) // == the resume mark
	if bc.admissionBlocked.Load() {
		t.Fatal("still suspended at the resume mark")
	}
	if bc.overBudgetTicks != 0 {
		t.Errorf("overBudgetTicks = %d after resuming, want 0", bc.overBudgetTicks)
	}

	mustSet(t, bc, "k", "v")
	if got := mustGet(t, bc, "k"); got != "v" {
		t.Errorf("Get = %q, want %q after resuming", got, "v")
	}
}

// Close's wait on gcDone has no test here on purpose. Reaching a long-running
// GC cycle from a test means getting the budget check to fire, and that check
// reads db.Size(), which stays at zero for the first minute of process life -
// so any such test drives a GC goroutine that never does more than a no-op
// RunValueLogGC, and passes whether or not Close waits for it. An earlier
// version of this file asserted it anyway; it passed against three separate
// mutations of Close, including one that inverted the lock ordering into a
// deadlock, and was removed rather than left as false coverage.

// Reads and writes keep working, and nothing deadlocks, while resets fire
// underneath them.
//
// Be precise about what this covers. Verified by mutation: reverting the atomic
// metric stores fails it with a DATA RACE on every run. Also verified, and
// worth stating so nobody trusts it for more than it does: stripping resetMu
// from Get - the guard this test looks like it is about - leaves it green
// across repeated -race runs. The DropAll-versus-read window badger warns about
// is not reachable at unit-test scale. resetMu is there because badger's own
// doc comment is unambiguous ("resilient to concurrent writes, but not to
// reads ... otherwise they may result in panics", db.go), not because anything
// here proves it necessary.
func TestResetDoesNotRaceReaders(t *testing.T) {
	bc := budgetCache(t, 100)
	for i := 0; i < 200; i++ {
		mustSet(t, bc, fmt.Sprintf("k%03d", i), "v")
	}

	ctx := context.Background()
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				// Get, the batch read, and the metrics scan are all reads in
				// the sense DropAll cares about.
				_, _ = bc.Get(ctx, fmt.Sprintf("k%03d", i%200))
				_, _ = bc.GetMulti(ctx, []string{"k000", "k100"})
				_ = bc.GetMetrics()
				_ = bc.Set(ctx, fmt.Sprintf("w%d-%d", r, i), []byte("v"), time.Hour)
			}
		}(r)
	}

	for i := 0; i < 20; i++ {
		bc.reset(150, 100)
	}

	close(stop)
	wg.Wait()
}
