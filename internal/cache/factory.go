package cache

import (
	"fmt"
	"time"
)

// Config holds configuration for cache creation
type Config struct {
	// Enabled determines if caching is enabled
	Enabled bool

	// Type selects the cache backend: "badger" or "memory" (default: "badger")
	Type string

	// Badger cache configuration
	BadgerPath           string
	BadgerMaxMemoryMB    int
	BadgerValueLogMaxMB  int
	BadgerCompactL0      bool
	BadgerNumGoroutines  int
	BadgerGCInterval     time.Duration
	BadgerGCDiscardRatio float64
	BadgerMaxDiskMB      int

	// Memory cache configuration
	MemoryGCInterval time.Duration
}

// DefaultConfig returns a default cache configuration for BadgerCache
func DefaultConfig() *Config {
	return &Config{
		Enabled:              true,
		Type:                 "badger",
		BadgerPath:           "./cache/badger",
		BadgerMaxMemoryMB:    64,
		BadgerValueLogMaxMB:  256,
		BadgerCompactL0:      true,
		BadgerNumGoroutines:  4,
		BadgerGCInterval:     10 * time.Minute,
		BadgerGCDiscardRatio: 0.5,
	}
}

// New creates a cache based on the configuration.
// Returns nil if caching is disabled.
func New(config *Config) (Cache, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if !config.Enabled {
		return nil, nil
	}

	switch config.Type {
	case "memory":
		return NewMemoryCache(&MemoryConfig{
			GCInterval: config.MemoryGCInterval,
		}), nil

	case "badger", "":
		if config.BadgerPath == "" {
			return nil, fmt.Errorf("BadgerPath is required when cache type is badger")
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

	default:
		return nil, fmt.Errorf("unsupported cache type: %q (supported: badger, memory)", config.Type)
	}
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
