// Package main provides operator caching for modem testing with failover.
// The cache stores working operator configurations per phone number using BadgerDB,
// allowing the system to remember which operator worked for a given destination.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// OperatorCacheConfig contains configuration for the operator cache.
type OperatorCacheConfig struct {
	Enabled bool     `yaml:"enabled"` // Enable operator caching (default: true when multiple operators)
	Path    string   `yaml:"path"`    // Cache directory path (default: ~/.modem-test/operator_cache)
	TTL     Duration `yaml:"ttl"`     // Cache entry TTL (default: 8640h = 360 days)
}

// CachedOperator stores the cached operator information for a phone number.
type CachedOperator struct {
	OperatorName   string    `json:"operator_name"`
	OperatorPrefix string    `json:"operator_prefix"`
	LastSuccess    time.Time `json:"last_success"`
}

// OperatorCache provides BadgerDB-based caching for phone → operator mappings.
type OperatorCache struct {
	db     *badger.DB
	ttl    time.Duration
	log    *TestLogger
	stopGC chan struct{}
}

// NewOperatorCache creates a new operator cache from configuration.
func NewOperatorCache(cfg OperatorCacheConfig, log *TestLogger) (*OperatorCache, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	// Determine cache path
	path := cfg.Path
	if path == "" {
		// Default to ~/.modem-test/operator_cache
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, ".modem-test", "operator_cache")
	} else if strings.HasPrefix(path, "~/") {
		// Expand ~ to home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[2:])
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory %s: %w", path, err)
	}

	// Configure BadgerDB options
	opts := badger.DefaultOptions(path)
	opts = opts.WithNumVersionsToKeep(1)
	opts = opts.WithNumLevelZeroTables(2)
	opts = opts.WithNumLevelZeroTablesStall(4)
	opts = opts.WithLoggingLevel(badger.WARNING)
	// Disable compression for small values
	opts = opts.WithCompression(0)

	// Open database
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open operator cache database: %w", err)
	}

	// Determine TTL
	ttl := cfg.TTL.Duration()
	if ttl == 0 {
		ttl = 360 * 24 * time.Hour // Default: 360 days
	}

	cache := &OperatorCache{
		db:     db,
		ttl:    ttl,
		log:    log,
		stopGC: make(chan struct{}),
	}

	// Start background GC
	go cache.runGC()

	if log != nil {
		log.Info("Operator cache enabled: %s (TTL: %v)", path, ttl)
	}

	return cache, nil
}

// keyForPhone returns the BadgerDB key for a phone number.
func (c *OperatorCache) keyForPhone(phone string) []byte {
	return []byte("operator:" + phone)
}

// Get retrieves the cached operator for a phone number.
// Returns (nil, false) if no cache entry exists.
func (c *OperatorCache) Get(phone string) (*CachedOperator, bool) {
	if c == nil || c.db == nil {
		return nil, false
	}

	var cached *CachedOperator

	err := c.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(c.keyForPhone(phone))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			cached = &CachedOperator{}
			return json.Unmarshal(val, cached)
		})
	})

	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, false
		}
		if c.log != nil {
			c.log.Warn("Operator cache read error for %s: %v", phone, err)
		}
		return nil, false
	}

	return cached, true
}

// Set stores the working operator for a phone number.
func (c *OperatorCache) Set(phone string, op OperatorConfig) error {
	if c == nil || c.db == nil {
		return nil
	}

	cached := CachedOperator{
		OperatorName:   op.Name,
		OperatorPrefix: op.Prefix,
		LastSuccess:    time.Now(),
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("failed to marshal operator cache entry: %w", err)
	}

	err = c.db.Update(func(txn *badger.Txn) error {
		entry := badger.NewEntry(c.keyForPhone(phone), data)
		entry = entry.WithTTL(c.ttl)
		return txn.SetEntry(entry)
	})

	if err != nil {
		return fmt.Errorf("failed to write operator cache entry: %w", err)
	}

	if c.log != nil {
		c.log.Info("Cached operator %q for phone %s", op.Name, phone)
	}

	return nil
}

// Delete removes the cached operator for a phone number.
func (c *OperatorCache) Delete(phone string) error {
	if c == nil || c.db == nil {
		return nil
	}

	err := c.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(c.keyForPhone(phone))
	})

	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return fmt.Errorf("failed to delete operator cache entry: %w", err)
	}

	if c.log != nil {
		c.log.Info("Cleared cached operator for phone %s", phone)
	}

	return nil
}

// Close closes the operator cache database.
func (c *OperatorCache) Close() error {
	if c == nil || c.db == nil {
		return nil
	}

	close(c.stopGC)
	return c.db.Close()
}

// runGC runs periodic garbage collection on the BadgerDB database.
func (c *OperatorCache) runGC() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.performGC()
		case <-c.stopGC:
			return
		}
	}
}

// performGC runs a single GC cycle.
func (c *OperatorCache) performGC() {
	for {
		err := c.db.RunValueLogGC(0.5)
		if err != nil {
			// ErrNoRewrite means nothing to GC, which is expected
			if !errors.Is(err, badger.ErrNoRewrite) && c.log != nil {
				c.log.Warn("Operator cache GC error: %v", err)
			}
			break
		}
	}
}

// FindOperatorByName looks up an operator by name in the given list.
// Returns (OperatorConfig, index, found).
func FindOperatorByName(operators []OperatorConfig, name string) (OperatorConfig, int, bool) {
	for i, op := range operators {
		if op.Name == name {
			return op, i, true
		}
	}
	return OperatorConfig{}, -1, false
}

// ReorderOperatorsWithCached returns a new operator list with the cached operator first.
// If no cached operator exists or it's not in the list, returns the original order.
func ReorderOperatorsWithCached(operators []OperatorConfig, cached *CachedOperator) []OperatorConfig {
	if cached == nil || len(operators) == 0 {
		return operators
	}

	// Find cached operator in the list
	_, idx, found := FindOperatorByName(operators, cached.OperatorName)
	if !found {
		return operators // Cached operator no longer in config
	}

	// If already first, return as-is
	if idx == 0 {
		return operators
	}

	// Reorder: put cached operator first
	reordered := make([]OperatorConfig, len(operators))
	reordered[0] = operators[idx]
	copy(reordered[1:idx+1], operators[:idx])
	copy(reordered[idx+1:], operators[idx+1:])

	return reordered
}
