package storage

import (
	"log"
	"time"

	"github.com/nodelistdb/internal/cache"
)

// CachedStorage wraps Storage with caching capabilities
type CachedStorage struct {
	*Storage
	cache  cache.Cache
	keyGen *cache.KeyGenerator
	config *CacheStorageConfig
}

// CacheStorageConfig configures the caching behavior
type CacheStorageConfig struct {
	Enabled           bool
	DefaultTTL        time.Duration
	NodeTTL           time.Duration
	StatsTTL          time.Duration
	SearchTTL         time.Duration
	MaxSearchResults  int
	WarmupOnStart     bool
}

// NewCachedStorage creates a new CachedStorage instance
func NewCachedStorage(storage *Storage, cacheImpl cache.Cache, config *CacheStorageConfig) *CachedStorage {
	if config == nil {
		config = &CacheStorageConfig{
			Enabled:          true,
			DefaultTTL:       5 * time.Minute,
			NodeTTL:          15 * time.Minute,
			StatsTTL:         1 * time.Hour,
			SearchTTL:        5 * time.Minute,
			MaxSearchResults: 500,
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

// GetCacheMetrics returns cache performance metrics
func (cs *CachedStorage) GetCacheMetrics() *cache.Metrics {
	if cs.cache == nil {
		return nil
	}
	return cs.cache.GetMetrics()
}

// warmupCache pre-populates cache with frequently accessed data
func (cs *CachedStorage) warmupCache() {
	log.Println("Starting cache warmup...")

	// Pre-cache latest stats
	if date, err := cs.Storage.GetLatestStatsDate(); err == nil {
		_, _ = cs.GetStats(date)
	}

	// Pre-cache available dates
	_, _ = cs.GetAvailableDates()

	// Pre-cache some popular nodes (example addresses)
	popularNodes := []struct{ Zone, Net, Node int }{
		{2, 450, 1024},
		{1, 1, 1},
		{2, 2, 20},
	}

	for _, node := range popularNodes {
		_, _ = cs.GetNodeHistory(node.Zone, node.Net, node.Node)
	}

	log.Println("Cache warmup completed")
}

// Close closes the cache
func (cs *CachedStorage) Close() error {
	if cs.cache != nil {
		return cs.cache.Close()
	}
	return nil
}

// SetTemporaryTTL temporarily reduces TTL for cache entries (used after imports)
func (cs *CachedStorage) SetTemporaryTTL(ttl time.Duration) {
	// This would require a more complex implementation to track
	// and restore the original TTL values
	// For now, we'll just log the intent
	log.Printf("Would set temporary TTL to %v (not implemented)", ttl)
}
