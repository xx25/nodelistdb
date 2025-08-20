package storage

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Cache provides persistent caching with BadgerDB
type Cache struct {
	db        *badger.DB
	mu        sync.RWMutex
	ttlMap    map[string]time.Duration
	cachePath string
}

// CacheItem represents a cached item with metadata
type CacheItem struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Timestamp time.Time       `json:"timestamp"`
	TTL       time.Duration   `json:"ttl"`
}

// NewCache creates a new persistent cache
func NewCache(cachePath string) (*Cache, error) {
	// Open BadgerDB
	opts := badger.DefaultOptions(cachePath)
	opts.Logger = nil // Disable BadgerDB logging
	
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}
	
	cache := &Cache{
		db:        db,
		ttlMap:    make(map[string]time.Duration),
		cachePath: cachePath,
	}
	
	// Start garbage collection routine
	go cache.runGC()
	
	return cache, nil
}

// Close closes the cache database
func (c *Cache) Close() error {
	return c.db.Close()
}

// SetTTL sets the TTL for a specific key prefix
func (c *Cache) SetTTL(prefix string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttlMap[prefix] = ttl
}

// Set stores a value in the cache
func (c *Cache) Set(key string, value interface{}) error {
	return c.SetWithTTL(key, value, c.getTTLForKey(key))
}

// SetWithTTL stores a value in the cache with specific TTL
func (c *Cache) SetWithTTL(key string, value interface{}, ttl time.Duration) error {
	// Marshal value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	
	item := CacheItem{
		Key:       key,
		Value:     data,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
	
	// Marshal cache item
	itemData, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal cache item: %w", err)
	}
	
	// Store in BadgerDB with TTL
	err = c.db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry([]byte(key), itemData)
		if ttl > 0 {
			e = e.WithTTL(ttl)
		}
		return txn.SetEntry(e)
	})
	
	return err
}

// Get retrieves a value from the cache
func (c *Cache) Get(key string, dest interface{}) error {
	var item CacheItem
	
	err := c.db.View(func(txn *badger.Txn) error {
		bItem, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		
		return bItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &item)
		})
	})
	
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return fmt.Errorf("key not found: %s", key)
		}
		return err
	}
	
	// Check if item is expired (belt and suspenders with BadgerDB TTL)
	if item.TTL > 0 && time.Since(item.Timestamp) > item.TTL {
		// Item is expired, delete it
		c.Delete(key)
		return fmt.Errorf("key expired: %s", key)
	}
	
	// Unmarshal value
	return json.Unmarshal(item.Value, dest)
}

// Delete removes a key from the cache
func (c *Cache) Delete(key string) error {
	return c.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

// DeletePrefix removes all keys with a specific prefix
func (c *Cache) DeletePrefix(prefix string) error {
	return c.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		
		it := txn.NewIterator(opts)
		defer it.Close()
		
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			if err := txn.Delete(item.Key()); err != nil {
				return err
			}
		}
		
		return nil
	})
}

// Exists checks if a key exists in the cache
func (c *Cache) Exists(key string) bool {
	err := c.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(key))
		return err
	})
	return err == nil
}

// GetStats returns cache statistics
func (c *Cache) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})
	
	// Count total keys
	keyCount := 0
	c.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		
		it := txn.NewIterator(opts)
		defer it.Close()
		
		for it.Rewind(); it.Valid(); it.Next() {
			keyCount++
		}
		return nil
	})
	
	stats["total_keys"] = keyCount
	stats["cache_path"] = c.cachePath
	
	// Get LSM size
	lsm, vlog := c.db.Size()
	stats["lsm_size"] = lsm
	stats["vlog_size"] = vlog
	stats["total_size"] = lsm + vlog
	
	return stats
}

// Clean removes expired entries
func (c *Cache) Clean() error {
	now := time.Now()
	keysToDelete := [][]byte{}
	
	err := c.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			
			// Check if item is expired
			var cacheItem CacheItem
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &cacheItem)
			})
			
			if err == nil && cacheItem.TTL > 0 {
				if now.Sub(cacheItem.Timestamp) > cacheItem.TTL {
					keysToDelete = append(keysToDelete, item.KeyCopy(nil))
				}
			}
		}
		return nil
	})
	
	if err != nil {
		return err
	}
	
	// Delete expired keys
	if len(keysToDelete) > 0 {
		err = c.db.Update(func(txn *badger.Txn) error {
			for _, key := range keysToDelete {
				if err := txn.Delete(key); err != nil {
					return err
				}
			}
			return nil
		})
	}
	
	return err
}

// runGC runs garbage collection periodically
func (c *Cache) runGC() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		// Run BadgerDB garbage collection
		c.db.RunValueLogGC(0.5)
	}
}

// getTTLForKey determines TTL based on key prefix
func (c *Cache) getTTLForKey(key string) time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// Check prefixes to determine TTL
	for prefix, ttl := range c.ttlMap {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return ttl
		}
	}
	
	// Default TTL
	return 24 * time.Hour
}

// DNSCache wraps Cache for DNS-specific operations
type DNSCache struct {
	cache *Cache
}

// NewDNSCache creates a new DNS cache
func NewDNSCache(cache *Cache) *DNSCache {
	cache.SetTTL("dns:", 24*time.Hour)
	return &DNSCache{cache: cache}
}

// Set stores a DNS result
func (d *DNSCache) Set(hostname string, result interface{}) error {
	key := fmt.Sprintf("dns:%s", hostname)
	return d.cache.Set(key, result)
}

// Get retrieves a DNS result
func (d *DNSCache) Get(hostname string, dest interface{}) error {
	key := fmt.Sprintf("dns:%s", hostname)
	return d.cache.Get(key, dest)
}

// GeolocationCache wraps Cache for geolocation-specific operations
type GeolocationCache struct {
	cache *Cache
}

// NewGeolocationCache creates a new geolocation cache
func NewGeolocationCache(cache *Cache) *GeolocationCache {
	cache.SetTTL("geo:", 7*24*time.Hour)
	return &GeolocationCache{cache: cache}
}

// Set stores a geolocation result
func (g *GeolocationCache) Set(ip string, result interface{}) error {
	key := fmt.Sprintf("geo:%s", ip)
	return g.cache.Set(key, result)
}

// Get retrieves a geolocation result
func (g *GeolocationCache) Get(ip string, dest interface{}) error {
	key := fmt.Sprintf("geo:%s", ip)
	return g.cache.Get(key, dest)
}