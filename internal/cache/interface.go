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