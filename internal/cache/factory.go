package cache

import (
	"fmt"
	"time"
)

// Config holds configuration for cache creation
// Note: Only BadgerCache is supported. MemoryCache and NoOpCache have been removed.
type Config struct {
	// Enabled determines if caching is enabled
	Enabled bool

	// Badger cache configuration
	BadgerPath           string
	BadgerMaxMemoryMB    int
	BadgerValueLogMaxMB  int
	BadgerCompactL0      bool
	BadgerNumGoroutines  int
	BadgerGCInterval     time.Duration
	BadgerGCDiscardRatio float64
	BadgerMaxDiskMB      int
}

// DefaultConfig returns a default cache configuration for BadgerCache
func DefaultConfig() *Config {
	return &Config{
		Enabled:              true,
		BadgerPath:           "./cache/badger",
		BadgerMaxMemoryMB:    64,
		BadgerValueLogMaxMB:  256,
		BadgerCompactL0:      true,
		BadgerNumGoroutines:  4,
		BadgerGCInterval:     10 * time.Minute,
		BadgerGCDiscardRatio: 0.5,
	}
}

// New creates a new BadgerCache based on the configuration
// Returns nil if caching is disabled
func New(config *Config) (Cache, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Return nil if cache is disabled
	if !config.Enabled {
		return nil, nil
	}

	// Validate path
	if config.BadgerPath == "" {
		return nil, fmt.Errorf("BadgerPath is required when cache is enabled")
	}

	// Create BadgerCache
	return NewBadgerCache(&BadgerConfig{
		Path:              config.BadgerPath,
		MaxMemoryMB:       config.BadgerMaxMemoryMB,
		ValueLogMaxMB:     config.BadgerValueLogMaxMB,
		CompactL0OnClose:  config.BadgerCompactL0,
		NumGoroutines:     config.BadgerNumGoroutines,
		GCInterval:        config.BadgerGCInterval,
		GCDiscardRatio:    config.BadgerGCDiscardRatio,
		MaxDiskMB:         config.BadgerMaxDiskMB,
	})
}

// NewBadgerCacheFromConfig creates a badger cache from config
// This is a convenience function for direct BadgerCache creation
func NewBadgerCacheFromConfig(config *Config) (Cache, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if config.BadgerPath == "" {
		return nil, fmt.Errorf("BadgerPath is required")
	}
	return NewBadgerCache(&BadgerConfig{
		Path:              config.BadgerPath,
		MaxMemoryMB:       config.BadgerMaxMemoryMB,
		ValueLogMaxMB:     config.BadgerValueLogMaxMB,
		CompactL0OnClose:  config.BadgerCompactL0,
		NumGoroutines:     config.BadgerNumGoroutines,
		GCInterval:        config.BadgerGCInterval,
		GCDiscardRatio:    config.BadgerGCDiscardRatio,
		MaxDiskMB:         config.BadgerMaxDiskMB,
	})
}
