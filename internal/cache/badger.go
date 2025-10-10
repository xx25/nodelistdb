package cache

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
)

type BadgerCache struct {
	db      *badger.DB
	metrics *Metrics
	config  *BadgerConfig
	stopGC  chan struct{}
}

type BadgerConfig struct {
	Path              string
	MaxMemoryMB       int
	ValueLogMaxMB     int
	CompactL0OnClose  bool
	NumGoroutines     int
	GCInterval        time.Duration
	GCDiscardRatio    float64
}

func NewBadgerCache(config *BadgerConfig) (*BadgerCache, error) {
	if config.GCInterval == 0 {
		config.GCInterval = 10 * time.Minute
	}
	if config.GCDiscardRatio == 0 {
		config.GCDiscardRatio = 0.5
	}

	opts := badger.DefaultOptions(config.Path)
	
	if config.MaxMemoryMB > 0 {
		opts = opts.WithMemTableSize(int64(config.MaxMemoryMB) << 20)
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
	opts = opts.WithNumLevelZeroTables(5)
	opts = opts.WithNumLevelZeroTablesStall(10)
	opts = opts.WithLoggingLevel(badger.WARNING)
	
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}
	
	cache := &BadgerCache{
		db:      db,
		metrics: &Metrics{},
		config:  config,
		stopGC:  make(chan struct{}),
	}
	
	// Start background GC
	go cache.runGC()
	
	// Initialize metrics
	cache.updateSizeMetrics()
	
	return cache, nil
}

func (bc *BadgerCache) Get(ctx context.Context, key string) ([]byte, error) {
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
	err := bc.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
	
	if err == nil {
		atomic.AddUint64(&bc.metrics.Deletes, 1)
	}
	
	return err
}

func (bc *BadgerCache) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
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

func (bc *BadgerCache) DeleteByPattern(ctx context.Context, pattern string) error {
	// Convert pattern to prefix (simple implementation)
	// Supports patterns like "prefix:*"
	prefix := strings.TrimSuffix(pattern, "*")
	
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
	close(bc.stopGC)
	return bc.db.Close()
}

func (bc *BadgerCache) runGC() {
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
					log.Printf("Badger GC completed %d cycles in %v", cycles, time.Since(startTime))
				}
				break
			}
			log.Printf("Badger GC error after %d cycles: %v", cycles, err)
			break
		}
		cycles++
	}
}

func (bc *BadgerCache) updateSizeMetrics() {
	lsm, vlog := bc.db.Size()
	bc.metrics.Size = uint64(lsm + vlog)
	
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
		bc.metrics.Keys = keyCount
	}
}

// Helper function to check if pattern matches string
func matchPattern(pattern, str string) bool {
	// Simple wildcard matching
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(str, prefix)
	}
	return pattern == str
}