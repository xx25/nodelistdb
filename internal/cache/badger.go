package cache

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/nodelistdb/internal/logging"
)

type BadgerCache struct {
	db      *badger.DB
	metrics *Metrics
	config  *BadgerConfig
	stopGC  chan struct{}

	// gcDone is closed when the GC goroutine has returned. Close waits on it:
	// stopGC only stops future ticks, and a cycle already in flight is inside
	// RunValueLogGC or DropAll, where closing the DB underneath it is a
	// use-after-close.
	gcDone chan struct{}

	// resetMu excludes every other operation while the store is being reset.
	// Badger documents DropAll as "resilient to concurrent writes, but not to
	// reads ... otherwise they may result in panics" (badger/db.go), and the GC
	// goroutine that calls it runs independently of request handling. Readers
	// take RLock, the reset takes Lock. Writers take RLock too - not because
	// Badger needs it, but so "held for reading" means "in flight" without a
	// per-method table of which operations are safe to race.
	resetMu sync.RWMutex

	// admissionBlocked suspends Set/SetMulti while the cache is over its disk
	// budget. Reads are unaffected.
	admissionBlocked atomic.Bool

	// overBudgetTicks counts consecutive over-budget GC ticks with admission
	// already suspended. Touched only by the GC goroutine.
	overBudgetTicks int
}

type BadgerConfig struct {
	Path             string
	MaxMemoryMB      int
	ValueLogMaxMB    int
	CompactL0OnClose bool
	NumGoroutines    int
	GCInterval       time.Duration
	GCDiscardRatio   float64
	MaxDiskMB        int
}

func NewBadgerCache(config *BadgerConfig) (*BadgerCache, error) {
	// Both guards are <= rather than ==, because a zero-check lets a negative
	// through and neither destination tolerates one. GCInterval reaches
	// time.NewTicker on a goroutine with no recover(), and NewTicker panics on a
	// non-positive interval - which takes down the process, not just the cache.
	// GCDiscardRatio outside (0,1) makes badger return ErrInvalidRequest on
	// every tick (badger/db.go RunValueLogGC), so GC silently never runs.
	//
	// Config validation rejects both loudly for operators; this is the guard for
	// callers that build a BadgerConfig in code, which every caller but the
	// server currently does.
	if config.GCInterval <= 0 {
		config.GCInterval = 10 * time.Minute
	}
	if config.GCDiscardRatio <= 0 || config.GCDiscardRatio >= 1 {
		config.GCDiscardRatio = 0.5
	}

	opts := badger.DefaultOptions(config.Path)

	// Memory budgeting: MaxMemoryMB is the TOTAL memory budget for Badger.
	// Badger uses NumMemtables * MemTableSize for in-memory tables,
	// so we divide the budget across tables to stay within limits.
	// Default: 2 memtables * (budget/4) each = budget/2 for memtables,
	// leaving room for block cache, index, and overhead.
	const numMemtables = 2
	if config.MaxMemoryMB > 0 {
		memTableSize := int64(config.MaxMemoryMB) << 20 / 4 // 1/4 of budget per memtable
		if memTableSize < 4<<20 {
			memTableSize = 4 << 20 // minimum 4MB per memtable
		}
		opts = opts.WithMemTableSize(memTableSize)
		opts = opts.WithNumMemtables(numMemtables)

		// Badger derives its transaction batch limit from the memtable size
		// (maxBatchSize = 15% of it) and refuses to open when ValueThreshold
		// exceeds that limit. Shrinking the memtable without shrinking the
		// threshold made every budget below 27MB fail at Open with an error
		// pointing at BaseTableSize, an option that does not even feed the
		// limit. Keep the threshold at half the batch size so the largest
		// value still stored inline cannot by itself exceed the limit: a value
		// at or above the threshold goes to the value log and costs ~14 bytes
		// against the batch, but one just below it is charged in full.
		const badgerMaxValueThreshold = 1 << 20 // badger's own ceiling
		valueThreshold := memTableSize * 15 / 100 / 2
		if valueThreshold > badgerMaxValueThreshold {
			valueThreshold = badgerMaxValueThreshold
		}
		opts = opts.WithValueThreshold(valueThreshold)

		// Limit block cache and index cache to fit within budget
		blockCacheSize := int64(config.MaxMemoryMB) << 20 / 4
		indexCacheSize := int64(config.MaxMemoryMB) << 20 / 8
		opts = opts.WithBlockCacheSize(blockCacheSize)
		opts = opts.WithIndexCacheSize(indexCacheSize)
	}
	if config.ValueLogMaxMB > 0 {
		opts = opts.WithValueLogFileSize(int64(config.ValueLogMaxMB) << 20)
	}
	opts = opts.WithCompactL0OnClose(config.CompactL0OnClose)
	if config.NumGoroutines > 0 {
		opts = opts.WithNumGoroutines(config.NumGoroutines)
	}

	// Performance optimizations
	opts = opts.WithNumVersionsToKeep(1)
	opts = opts.WithNumLevelZeroTables(3)
	opts = opts.WithNumLevelZeroTablesStall(5)
	opts = opts.WithLoggingLevel(badger.ERROR)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}

	cache := &BadgerCache{
		db:      db,
		metrics: &Metrics{},
		config:  config,
		stopGC:  make(chan struct{}),
		gcDone:  make(chan struct{}),
	}

	// Start background GC
	go cache.runGC()

	// Initialize metrics
	cache.updateSizeMetrics()

	return cache, nil
}

func (bc *BadgerCache) Get(ctx context.Context, key string) ([]byte, error) {
	bc.resetMu.RLock()
	defer bc.resetMu.RUnlock()

	var value []byte

	err := bc.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				// Don't count miss here - let the caller decide
				return err
			}
			return err
		}

		// Check TTL expiration
		if item.IsDeletedOrExpired() {
			// Don't count miss here - let the caller decide
			return badger.ErrKeyNotFound
		}

		value, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}

		// Skip the TTL bytes prefix if present
		if len(value) >= 8 {
			value = value[8:]
		}

		// Don't count hit here - let the caller decide
		return nil
	})

	return value, err
}

func (bc *BadgerCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if bc.admissionBlocked.Load() {
		atomic.AddUint64(&bc.metrics.Rejected, 1)
		return nil
	}

	bc.resetMu.RLock()
	defer bc.resetMu.RUnlock()

	// Prefix value with TTL information for consistent storage
	fullValue := make([]byte, 8+len(value))
	binary.LittleEndian.PutUint64(fullValue[:8], uint64(time.Now().Add(ttl).Unix()))
	copy(fullValue[8:], value)

	err := bc.db.Update(func(txn *badger.Txn) error {
		entry := badger.NewEntry([]byte(key), fullValue)
		if ttl > 0 {
			entry = entry.WithTTL(ttl)
		}
		return txn.SetEntry(entry)
	})

	if err == nil {
		atomic.AddUint64(&bc.metrics.Sets, 1)
	}

	return err
}

func (bc *BadgerCache) Delete(ctx context.Context, key string) error {
	bc.resetMu.RLock()
	defer bc.resetMu.RUnlock()

	err := bc.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})

	if err == nil {
		atomic.AddUint64(&bc.metrics.Deletes, 1)
	}

	return err
}

func (bc *BadgerCache) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	bc.resetMu.RLock()
	defer bc.resetMu.RUnlock()

	results := make(map[string][]byte)

	err := bc.db.View(func(txn *badger.Txn) error {
		for _, key := range keys {
			item, err := txn.Get([]byte(key))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					// Don't count - let caller handle metrics
					continue
				}
				return err
			}

			if item.IsDeletedOrExpired() {
				// Don't count - let caller handle metrics
				continue
			}

			value, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			// Skip the TTL bytes prefix
			if len(value) >= 8 {
				value = value[8:]
			}

			results[key] = value
			// Don't count - let caller handle metrics
		}
		return nil
	})

	return results, err
}

func (bc *BadgerCache) SetMulti(ctx context.Context, items map[string]CacheItem) error {
	if bc.admissionBlocked.Load() {
		atomic.AddUint64(&bc.metrics.Rejected, uint64(len(items)))
		return nil
	}

	bc.resetMu.RLock()
	defer bc.resetMu.RUnlock()

	err := bc.db.Update(func(txn *badger.Txn) error {
		for key, item := range items {
			// Prefix value with TTL information
			fullValue := make([]byte, 8+len(item.Value))
			binary.LittleEndian.PutUint64(fullValue[:8], uint64(time.Now().Add(item.TTL).Unix()))
			copy(fullValue[8:], item.Value)

			entry := badger.NewEntry([]byte(key), fullValue)
			if item.TTL > 0 {
				entry = entry.WithTTL(item.TTL)
			}
			if err := txn.SetEntry(entry); err != nil {
				return err
			}
		}
		return nil
	})

	if err == nil {
		atomic.AddUint64(&bc.metrics.Sets, uint64(len(items)))
	}

	return err
}

func (bc *BadgerCache) DeleteMulti(ctx context.Context, keys []string) error {
	bc.resetMu.RLock()
	defer bc.resetMu.RUnlock()

	err := bc.db.Update(func(txn *badger.Txn) error {
		for _, key := range keys {
			if err := txn.Delete([]byte(key)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}
		return nil
	})

	if err == nil {
		atomic.AddUint64(&bc.metrics.Deletes, uint64(len(keys)))
	}

	return err
}

func (bc *BadgerCache) DeleteByPrefix(ctx context.Context, prefix string) error {
	// Literal prefix only: the scan below is an ordered Seek over the keyspace,
	// which a leading or embedded wildcard could not use.
	if strings.Contains(prefix, "*") {
		return fmt.Errorf("%w: %q", ErrWildcardPrefix, prefix)
	}

	bc.resetMu.RLock()
	defer bc.resetMu.RUnlock()

	var keysToDelete [][]byte

	err := bc.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefixBytes := []byte(prefix)
		for it.Seek(prefixBytes); it.ValidForPrefix(prefixBytes); it.Next() {
			item := it.Item()
			key := item.KeyCopy(nil)
			keysToDelete = append(keysToDelete, key)
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Delete collected keys
	err = bc.db.Update(func(txn *badger.Txn) error {
		for _, key := range keysToDelete {
			if err := txn.Delete(key); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}
		return nil
	})

	if err == nil {
		atomic.AddUint64(&bc.metrics.Deletes, uint64(len(keysToDelete)))
	}

	return err
}

func (bc *BadgerCache) GetMetrics() *Metrics {
	bc.updateSizeMetrics()
	return bc.metrics
}

func (bc *BadgerCache) Close() error {
	// Wait for the GC goroutine to be out of badger before closing the DB. Not
	// holding resetMu while waiting: a reset in flight is blocked on it until
	// the current readers drain, so taking the lock first would deadlock
	// against the very goroutine being waited on.
	close(bc.stopGC)
	<-bc.gcDone

	// Then exclude in-flight readers, for the reason the reset does.
	bc.resetMu.Lock()
	defer bc.resetMu.Unlock()

	return bc.db.Close()
}

func (bc *BadgerCache) runGC() {
	defer close(bc.gcDone)

	ticker := time.NewTicker(bc.config.GCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bc.performGC()
		case <-bc.stopGC:
			return
		}
	}
}

func (bc *BadgerCache) performGC() {
	startTime := time.Now()
	cycles := 0

	for {
		err := bc.db.RunValueLogGC(bc.config.GCDiscardRatio)
		if err != nil {
			if errors.Is(err, badger.ErrNoRewrite) {
				// No more cleanup possible
				if cycles > 0 {
					logging.Debug("Badger value log GC completed",
						slog.Int("cycles", cycles),
						slog.Duration("duration", time.Since(startTime)))
				}
				break
			}
			logging.Warn("Badger value log GC error",
				slog.Int("cycles", cycles),
				slog.String("error", err.Error()))
			break
		}
		cycles++
	}

	bc.enforceDiskBudget()
}

const (
	// Admission resumes at this fraction of the budget rather than at the line
	// itself, so a cache sitting on the boundary does not flap open and shut on
	// every tick.
	diskBudgetResumeRatio = 0.9

	// Consecutive over-budget ticks with admission already suspended before
	// falling back to a full reset. At the default 10m gc_interval that is half
	// an hour of draining before giving up on draining.
	diskBudgetResetAfterTicks = 3
)

// enforceDiskBudget keeps the store inside config.MaxDiskMB.
//
// The policy used to be: any tick that found the cache over budget called
// DropAll() on the whole keyspace. That was wrong twice over. Badger documents
// DropAll as "resilient to concurrent writes, but not to reads ... otherwise
// they may result in panics" (badger/db.go), and this runs on the GC goroutine
// with nothing excluding readers - so setting a disk cap armed a process panic,
// not a cache wipe. Even when it got away with it, it discarded a warm cache in
// full to reclaim whatever slice of it was over the line.
//
// Now: stop admitting new entries while over budget and let TTL expiry drain
// the store. Reads keep being served the whole time. CachedStorage is pure
// cache-aside - every miss falls through to ClickHouse - so a suspended cache
// costs latency and never correctness.
//
// The reset survives as a backstop because draining is not guaranteed. Badger
// reclaims expired keys during compaction, and compaction is driven by writes;
// a cache held shut can sit full of expired keys that nothing will collect,
// which is a 100% miss rate with the disk still pinned. So after
// diskBudgetResetAfterTicks over-budget ticks with admission already suspended,
// reset - but under the exclusive lock every reader honours, which is what
// makes the operation Badger warns about safe to call at all.
func (bc *BadgerCache) enforceDiskBudget() {
	// 0 means no cap. Negative is rejected by config validation and coerced
	// nowhere, so treat it the same as 0 rather than as a cap of "always over".
	if bc.config.MaxDiskMB <= 0 {
		return
	}

	// db.Size() does not measure anything; it reads counters that badger
	// refreshes from a directory walk on its own one-minute ticker, and which
	// read zero until the first of those fires (the walk badger does at Open
	// runs before the memtables and value log exist). So the budget is enforced
	// against a figure up to a minute stale, and never fires at all during the
	// first minute of process life. At a 10m gc_interval that is noise, but it
	// is why the reset backstop is measured in ticks rather than bytes.
	lsm, vlog := bc.db.Size()
	bc.applyDiskBudget((lsm + vlog) / (1 << 20))
}

// applyDiskBudget is the policy half of enforceDiskBudget, split from the
// measurement so tests can drive the state machine directly - with the real
// db.Size() in the loop, observing a single transition would take a minute.
func (bc *BadgerCache) applyDiskBudget(totalMB int64) {
	limitMB := int64(bc.config.MaxDiskMB)
	resumeMB := int64(float64(limitMB) * diskBudgetResumeRatio)
	blocked := bc.admissionBlocked.Load()

	switch {
	case blocked && totalMB <= resumeMB:
		bc.admissionBlocked.Store(false)
		bc.overBudgetTicks = 0
		logging.Info("Cache back inside its disk budget, resuming writes",
			slog.Int64("size_mb", totalMB),
			slog.Int64("limit_mb", limitMB))

	case blocked && totalMB > limitMB:
		bc.overBudgetTicks++
		if bc.overBudgetTicks >= diskBudgetResetAfterTicks {
			bc.reset(totalMB, limitMB)
		}

	case blocked:
		// Between the resume mark and the limit: draining is working, so hold
		// admission shut but do not count this as a stalled tick.

	case totalMB > limitMB:
		bc.admissionBlocked.Store(true)
		bc.overBudgetTicks = 1
		logging.Warn("Cache over its disk budget, suspending writes until it drains",
			slog.Int64("size_mb", totalMB),
			slog.Int64("limit_mb", limitMB))
	}
}

// reset drops the whole store. Callable only from the GC goroutine.
func (bc *BadgerCache) reset(totalMB, limitMB int64) {
	logging.Warn("Cache still over its disk budget after suspending writes, resetting",
		slog.Int64("size_mb", totalMB),
		slog.Int64("limit_mb", limitMB),
		slog.Int("ticks", bc.overBudgetTicks))

	// Exclusive: this is the call Badger warns may panic against a live read.
	bc.resetMu.Lock()
	err := bc.db.DropAll()
	bc.resetMu.Unlock()

	if err != nil {
		// Stay suspended and try again next tick.
		logging.Error("Cache reset failed", slog.String("error", err.Error()))
		return
	}

	bc.admissionBlocked.Store(false)
	bc.overBudgetTicks = 0

	// No post-reset size is logged: DropAll deletes every value log file and
	// restarts them from zero, but db.Size() reads counters that badger's own
	// ticker refreshes about once a minute, so the number here would be the
	// pre-reset one. Deliberately not calling RunValueLogGC afterwards either -
	// DropAll has already deleted the files it would collect.
	logging.Info("Cache reset, writes resumed")
}

func (bc *BadgerCache) updateSizeMetrics() {
	// Iterates the keyspace, so it is a read in the sense DropAll cares about.
	bc.resetMu.RLock()
	defer bc.resetMu.RUnlock()

	// Store, not assign: GetMetrics calls this, CachedStorage calls GetMetrics
	// on every hit and every miss, and two concurrent requests would otherwise
	// race writing these two fields. Every other field in Metrics is already
	// touched atomically; these two were not. Surfaced by -race in
	// TestResetDoesNotRaceReaders.
	lsm, vlog := bc.db.Size()
	atomic.StoreUint64(&bc.metrics.Size, uint64(lsm+vlog))

	// Count keys
	var keyCount uint64
	err := bc.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			keyCount++
		}
		return nil
	})

	if err == nil {
		atomic.StoreUint64(&bc.metrics.Keys, keyCount)
	}
}
