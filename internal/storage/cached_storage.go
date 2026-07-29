package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nodelistdb/internal/cache"
	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/logging"
)

// CachedStorage wraps Storage with caching capabilities
type CachedStorage struct {
	*Storage
	cache  cache.Cache
	keyGen *cache.KeyGenerator
	config *CacheStorageConfig
}

// CacheStorageConfig configures the caching behavior.
//
// There is no DefaultTTL here on purpose. config.Cache.DefaultTTL is the
// fallback for an omitted node/stats/search TTL and is resolved during config
// validation, so by the time a config reaches this layer every TTL is already
// specific. Carrying it further would just be a second field nobody reads.
type CacheStorageConfig struct {
	Enabled          bool
	NodeTTL          time.Duration
	StatsTTL         time.Duration
	SearchTTL        time.Duration
	MaxSearchResults int
	WarmupOnStart    bool

	// The analytics reports are cached on four horizons, chosen by what can
	// change their answer rather than by how stale a reader will tolerate.
	//
	// TestAnalyticsTTL covers everything computed from node_test_results,
	// which the test daemon appends to continuously. AnalyticsTTL and
	// LongAnalyticsTTL cover progressively heavier aggregates over the same
	// data. HistoricalTTL covers answers only a new nodelist import can move -
	// a flag's first appearance, a network's history, a per-year trend.
	TestAnalyticsTTL time.Duration
	AnalyticsTTL     time.Duration
	LongAnalyticsTTL time.Duration
	HistoricalTTL    time.Duration
}

// NewCachedStorage creates a new CachedStorage instance
func NewCachedStorage(storage *Storage, cacheImpl cache.Cache, config *CacheStorageConfig) *CachedStorage {
	if config == nil {
		config = &CacheStorageConfig{
			Enabled:          true,
			NodeTTL:          15 * time.Minute,
			StatsTTL:         1 * time.Hour,
			SearchTTL:        5 * time.Minute,
			MaxSearchResults: 500,
		}
	}
	// A caller that built the config by hand - tests, and any consumer added
	// before these fields existed - leaves the analytics horizons at zero,
	// which both cache backends read as "never expires".
	for _, ttl := range []struct {
		field *time.Duration
		def   time.Duration
	}{
		{&config.TestAnalyticsTTL, 15 * time.Minute},
		{&config.AnalyticsTTL, 30 * time.Minute},
		{&config.LongAnalyticsTTL, 1 * time.Hour},
		{&config.HistoricalTTL, 24 * time.Hour},
	} {
		if *ttl.field == 0 {
			*ttl.field = ttl.def
		}
	}

	cs := &CachedStorage{
		Storage: storage,
		cache:   cacheImpl,
		keyGen:  cache.NewKeyGenerator("ndb"),
		config:  config,
	}

	if config.WarmupOnStart {
		go cs.warmupCache()
	}

	return cs
}

// warmupCache pre-populates cache with frequently accessed data.
//
// Background context by design: the warmup runs on its own goroutine at
// startup with no request behind it, so there is nothing whose cancellation
// should abort it.
func (cs *CachedStorage) warmupCache() {
	ctx := context.Background()
	logging.Info("Starting cache warmup")

	// Pre-cache latest stats (default network)
	if date, err := cs.Storage.StatsOps().GetLatestStatsDate(ctx, database.DefaultDomain); err == nil {
		_, _ = cs.GetStats(ctx, date, database.DefaultDomain)
	}

	// Pre-cache available dates
	_, _ = cs.GetAvailableDates(ctx, database.DefaultDomain)

	// Pre-cache some popular nodes (example addresses)
	popularNodes := []struct{ Zone, Net, Node int }{
		{2, 450, 1024},
		{1, 1, 1},
		{2, 2, 20},
	}

	for _, node := range popularNodes {
		_, _ = cs.GetNodeHistory(ctx, node.Zone, node.Net, node.Node, "")
	}

	logging.Info("Cache warmup completed")
}

// Close closes the cache
func (cs *CachedStorage) Close() error {
	if cs.cache != nil {
		return cs.cache.Close()
	}
	return nil
}

// cachedFetch is the cache-aside path every read wrapper on CachedStorage
// follows: look the key up, count the hit or the miss, call fetch on a miss,
// store what came back. A cached entry that will not unmarshal is treated as a
// miss - the shape of a stored value changes with the code that wrote it, and
// a key whose shape moved without its version segment moving would otherwise
// keep failing for its whole TTL.
//
// The three helpers keep their signatures now that reads carry a request
// context: the fetch closure captures the caller's ctx, so cancellation
// reaches the database without the cache layer having to know about it.
//
// The cache store itself deliberately stays on context.Background(), and that
// is not an oversight left over from the migration:
//
//   - request ctx on Get would turn every cancelled request's lookup into an
//     error, hence a miss, hence a thundering herd on whatever it was about to
//     stop caring about;
//   - request ctx on Set would drop the fill for a fetch that already
//     succeeded, because the client happened to leave a millisecond later. The
//     query has been paid for; store the answer.
//   - and BadgerCache.Get/Set accept a ctx and never look at it - Get blocks on
//     resetMu regardless - so there is no cancellation to propagate even if the
//     first two reasons did not apply.
//
// All three helpers return early on a fetch error without calling cache.Set,
// so a cancelled request can never poison the cache with a partial answer.
func cachedFetch[T any](cs *CachedStorage, key string, ttl time.Duration, fetch func() (T, error)) (T, error) {
	if !cs.config.Enabled {
		return fetch()
	}

	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var result T
		if err := json.Unmarshal(data, &result); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return result, nil
		}
		logging.Warn("Failed to unmarshal cached data", slog.String("key", key), slog.Any("error", err))
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	result, err := fetch()
	if err != nil {
		return result, err
	}

	if data, err := json.Marshal(result); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, ttl)
	}
	return result, nil
}

// cachedFetchPtr is cachedFetch for the readers that return a pointer and use
// nil for "nothing here yet".
//
// Those two are not the same answer. A nil from GetNodeReachabilityStats means
// the daemon has not tested this node inside the window, and from
// GetFlagFirstAppearance that the flag is not in flag_statistics - both states
// a later import or test cycle ends. Storing them turns a temporary absence
// into a cached fact for the whole TTL, which for the historical readers is a
// day. Every one of these readers guarded its cache.Set on a non-nil result
// before this helper existed.
func cachedFetchPtr[T any](cs *CachedStorage, key string, ttl time.Duration, fetch func() (*T, error)) (*T, error) {
	if !cs.config.Enabled {
		return fetch()
	}

	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var result *T
		// A stored "null" would unmarshal without error; treat it as a miss so
		// the invariant holds even if an older entry got in.
		if err := json.Unmarshal(data, &result); err == nil && result != nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return result, nil
		} else if err != nil {
			logging.Warn("Failed to unmarshal cached data", slog.String("key", key), slog.Any("error", err))
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	result, err := fetch()
	if err != nil {
		return nil, err
	}

	if result != nil {
		if data, err := json.Marshal(result); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, ttl)
		}
	}
	return result, nil
}

// cachedFetchSlice is cachedFetch for the readers that decline to store an
// empty answer.
//
// The asymmetry is deliberate and predates this helper. An analytics report
// comes back empty for two very different reasons - nobody matches the
// criteria, or the window/gate excluded everything - and caching the second
// for an hour hides a freshly imported nodelist behind an answer computed
// before it arrived. The single-value readers below (a node, a stats row, a
// date) have no such ambiguity and cache whatever they get, including nothing.
func cachedFetchSlice[T any](cs *CachedStorage, key string, ttl time.Duration, fetch func() ([]T, error)) ([]T, error) {
	if !cs.config.Enabled {
		return fetch()
	}

	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []T
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
		logging.Warn("Failed to unmarshal cached data", slog.String("key", key), slog.Any("error", err))
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	results, err := fetch()
	if err != nil {
		return nil, err
	}

	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, ttl)
		}
	}
	return results, nil
}

// analyticsKey builds a cache key in the analytics namespace, which
// InvalidateAnalytics sweeps by prefix. Every part is appended in order, so
// two calls that differ in any argument cannot collide.
func (cs *CachedStorage) analyticsKey(name string, parts ...any) string {
	var b strings.Builder
	b.WriteString(cs.keyGen.Prefix)
	b.WriteString(":analytics:")
	b.WriteString(name)
	for _, p := range parts {
		b.WriteString(":")
		fmt.Fprintf(&b, "%v", p)
	}
	return b.String()
}
