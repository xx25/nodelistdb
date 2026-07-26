package cache

import (
	"context"
	"errors"
	"time"
)

// ErrWildcardPrefix is returned by DeleteByPrefix when handed a glob pattern
// instead of a literal key prefix. Both backends match a literal prefix only,
// so a "*" anywhere in the argument used to match nothing and report success.
var ErrWildcardPrefix = errors.New("cache: DeleteByPrefix takes a literal key prefix, not a glob pattern")

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
	
	// Prefix operations
	//
	// DeleteByPrefix removes every key starting with prefix. Matching is on a
	// literal prefix, not a glob: the prefix must end on a key delimiter to
	// avoid catching deeper siblings, since "ndb:node:2:5001:100" is also a
	// prefix of "ndb:node:2:5001:1000". Supplying a "*" returns
	// ErrWildcardPrefix rather than silently deleting nothing.
	DeleteByPrefix(ctx context.Context, prefix string) error
	
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