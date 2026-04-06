package cache

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type memoryEntry struct {
	value     []byte
	expiresAt time.Time
}

func (e *memoryEntry) expired() bool {
	return !e.expiresAt.IsZero() && time.Now().After(e.expiresAt)
}

// MemoryCache is a simple in-memory cache with TTL support.
// Suitable for small deployments where BadgerDB overhead is not justified.
type MemoryCache struct {
	mu      sync.RWMutex
	entries map[string]*memoryEntry
	metrics *Metrics
	stopGC  chan struct{}
}

type MemoryConfig struct {
	GCInterval time.Duration
}

func NewMemoryCache(config *MemoryConfig) *MemoryCache {
	if config == nil {
		config = &MemoryConfig{}
	}
	if config.GCInterval == 0 {
		config.GCInterval = 5 * time.Minute
	}

	mc := &MemoryCache{
		entries: make(map[string]*memoryEntry),
		metrics: &Metrics{},
		stopGC:  make(chan struct{}),
	}

	go mc.runGC(config.GCInterval)
	return mc
}

func (mc *MemoryCache) Get(_ context.Context, key string) ([]byte, error) {
	mc.mu.RLock()
	entry, ok := mc.entries[key]
	mc.mu.RUnlock()

	if !ok || entry.expired() {
		if ok {
			// Lazy delete expired entry
			mc.mu.Lock()
			delete(mc.entries, key)
			mc.mu.Unlock()
		}
		return nil, &ErrKeyNotFound{Key: key}
	}

	return entry.value, nil
}

func (mc *MemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	// Copy value to avoid caller mutations
	copied := make([]byte, len(value))
	copy(copied, value)

	mc.mu.Lock()
	mc.entries[key] = &memoryEntry{value: copied, expiresAt: expiresAt}
	mc.mu.Unlock()

	atomic.AddUint64(&mc.metrics.Sets, 1)
	return nil
}

func (mc *MemoryCache) Delete(_ context.Context, key string) error {
	mc.mu.Lock()
	delete(mc.entries, key)
	mc.mu.Unlock()

	atomic.AddUint64(&mc.metrics.Deletes, 1)
	return nil
}

func (mc *MemoryCache) GetMulti(_ context.Context, keys []string) (map[string][]byte, error) {
	results := make(map[string][]byte, len(keys))
	var expired []string

	mc.mu.RLock()
	for _, key := range keys {
		entry, ok := mc.entries[key]
		if !ok {
			continue
		}
		if entry.expired() {
			expired = append(expired, key)
			continue
		}
		results[key] = entry.value
	}
	mc.mu.RUnlock()

	if len(expired) > 0 {
		mc.mu.Lock()
		for _, key := range expired {
			delete(mc.entries, key)
		}
		mc.mu.Unlock()
	}

	return results, nil
}

func (mc *MemoryCache) SetMulti(_ context.Context, items map[string]CacheItem) error {
	mc.mu.Lock()
	for key, item := range items {
		var expiresAt time.Time
		if item.TTL > 0 {
			expiresAt = time.Now().Add(item.TTL)
		}
		copied := make([]byte, len(item.Value))
		copy(copied, item.Value)
		mc.entries[key] = &memoryEntry{value: copied, expiresAt: expiresAt}
	}
	mc.mu.Unlock()

	atomic.AddUint64(&mc.metrics.Sets, uint64(len(items)))
	return nil
}

func (mc *MemoryCache) DeleteMulti(_ context.Context, keys []string) error {
	mc.mu.Lock()
	for _, key := range keys {
		delete(mc.entries, key)
	}
	mc.mu.Unlock()

	atomic.AddUint64(&mc.metrics.Deletes, uint64(len(keys)))
	return nil
}

func (mc *MemoryCache) DeleteByPattern(_ context.Context, pattern string) error {
	prefix := strings.TrimSuffix(pattern, "*")

	mc.mu.Lock()
	var count uint64
	for key := range mc.entries {
		if strings.HasPrefix(key, prefix) {
			delete(mc.entries, key)
			count++
		}
	}
	mc.mu.Unlock()

	atomic.AddUint64(&mc.metrics.Deletes, count)
	return nil
}

func (mc *MemoryCache) GetMetrics() *Metrics {
	mc.mu.RLock()
	mc.metrics.Keys = uint64(len(mc.entries))
	mc.mu.RUnlock()
	return mc.metrics
}

func (mc *MemoryCache) Close() error {
	close(mc.stopGC)
	return nil
}

func (mc *MemoryCache) runGC(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mc.evictExpired()
		case <-mc.stopGC:
			return
		}
	}
}

func (mc *MemoryCache) evictExpired() {
	now := time.Now()
	mc.mu.Lock()
	for key, entry := range mc.entries {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			delete(mc.entries, key)
		}
	}
	mc.mu.Unlock()
}
