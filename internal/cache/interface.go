package cache

import (
	"context"
	"time"
)

// Cache defines the cache operations interface
type Cache interface {
	// Basic operations
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	
	// Batch operations
	GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)
	SetMulti(ctx context.Context, items map[string]CacheItem) error
	DeleteMulti(ctx context.Context, keys []string) error
	
	// Pattern operations
	DeleteByPattern(ctx context.Context, pattern string) error
	
	// Metrics
	GetMetrics() *Metrics
	
	// Lifecycle
	Close() error
}

// CacheItem represents a single cache entry
type CacheItem struct {
	Value []byte
	TTL   time.Duration
}

// Metrics tracks cache performance
type Metrics struct {
	Hits       uint64
	Misses     uint64
	Sets       uint64
	Deletes    uint64
	Evictions  uint64
	Size       uint64
	Keys       uint64
}

// HitRate calculates the cache hit rate percentage
func (m *Metrics) HitRate() float64 {
	total := m.Hits + m.Misses
	if total == 0 {
		return 0.0
	}
	return float64(m.Hits) / float64(total) * 100.0
}

// MissRate calculates the cache miss rate percentage
func (m *Metrics) MissRate() float64 {
	total := m.Hits + m.Misses
	if total == 0 {
		return 0.0
	}
	return float64(m.Misses) / float64(total) * 100.0
}

// ErrKeyNotFound is returned when a key is not found in the cache
type ErrKeyNotFound struct {
	Key string
}

func (e *ErrKeyNotFound) Error() string {
	return "cache: key not found: " + e.Key
}

// IsKeyNotFound checks if an error is a key not found error
func IsKeyNotFound(err error) bool {
	_, ok := err.(*ErrKeyNotFound)
	return ok
}