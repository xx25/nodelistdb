package config

import (
	"fmt"
	"time"
)

// ModemAPIConfig holds configuration for the distributed modem testing API
type ModemAPIConfig struct {
	Enabled bool `yaml:"enabled"`

	// Daemon definitions (all configuration here, not in database)
	Callers []ModemCallerConfig `yaml:"callers,omitempty"`

	// Queue management intervals
	OrphanCheckInterval      time.Duration `yaml:"orphan_check_interval"`       // How often to check for offline daemons
	OfflineThreshold         time.Duration `yaml:"offline_threshold"`           // How long until a daemon is considered offline
	StaleInProgressThreshold time.Duration `yaml:"stale_in_progress_threshold"` // Reclaim nodes stuck in_progress longer than this

	// Rate limits for API endpoints
	RateLimits RateLimitConfig `yaml:"rate_limits,omitempty"`

	// Request limits
	MaxBatchSize  int `yaml:"max_batch_size"`   // Max nodes per request
	MaxBodySizeMB int `yaml:"max_body_size_mb"` // Max request body size in MB
}

// ModemCallerConfig holds configuration for a single modem daemon
type ModemCallerConfig struct {
	CallerID   string `yaml:"caller_id"`     // Unique identifier for this daemon (e.g., "modem-eu-01")
	Name       string `yaml:"name"`          // Human-readable name (e.g., "Europe Modem Server")
	APIKeyHash string `yaml:"api_key_hash"`  // SHA256 hash of API key (e.g., "sha256:abc123...")
	Location   string `yaml:"location"`      // Physical location (e.g., "Frankfurt, Germany")
	Priority   int    `yaml:"priority"`      // Higher = preferred for overlapping prefixes
	PrefixMode string `yaml:"prefix_mode"`   // "include", "exclude", or "all"
	Prefixes   []string `yaml:"prefixes,omitempty"` // Prefixes to include/exclude
}

// RateLimitConfig holds rate limiting configuration for the modem API
type RateLimitConfig struct {
	RequestsPerSecond    float64 `yaml:"requests_per_second"`
	BurstSize            int     `yaml:"burst_size"`
	MaxRequestsPerMinute int     `yaml:"max_requests_per_minute"`
}

// DefaultModemAPIConfig returns default configuration for the modem API
func DefaultModemAPIConfig() *ModemAPIConfig {
	return &ModemAPIConfig{
		Enabled:                  false,
		Callers:                  []ModemCallerConfig{},
		OrphanCheckInterval:      5 * time.Minute,
		OfflineThreshold:         10 * time.Minute,
		StaleInProgressThreshold: 1 * time.Hour,
		RateLimits: RateLimitConfig{
			RequestsPerSecond:    10,
			BurstSize:            20,
			MaxRequestsPerMinute: 600,
		},
		MaxBatchSize:  100,
		MaxBodySizeMB: 1,
	}
}

// GetCaller returns the caller configuration for the given caller ID
func (c *ModemAPIConfig) GetCaller(callerID string) *ModemCallerConfig {
	for i := range c.Callers {
		if c.Callers[i].CallerID == callerID {
			return &c.Callers[i]
		}
	}
	return nil
}

// validateModemAPI validates the modem API configuration and sets defaults
func (c *Config) validateModemAPI() error {
	if !c.ModemAPI.Enabled {
		return nil
	}

	// Set defaults for intervals
	if c.ModemAPI.OrphanCheckInterval == 0 {
		c.ModemAPI.OrphanCheckInterval = 5 * time.Minute
	}
	if c.ModemAPI.OfflineThreshold == 0 {
		c.ModemAPI.OfflineThreshold = 10 * time.Minute
	}
	if c.ModemAPI.StaleInProgressThreshold == 0 {
		c.ModemAPI.StaleInProgressThreshold = 1 * time.Hour
	}

	// Set defaults for rate limits
	if c.ModemAPI.RateLimits.RequestsPerSecond == 0 {
		c.ModemAPI.RateLimits.RequestsPerSecond = 10
	}
	if c.ModemAPI.RateLimits.BurstSize == 0 {
		c.ModemAPI.RateLimits.BurstSize = 20
	}
	if c.ModemAPI.RateLimits.MaxRequestsPerMinute == 0 {
		c.ModemAPI.RateLimits.MaxRequestsPerMinute = 600
	}

	// Set defaults for request limits
	if c.ModemAPI.MaxBatchSize == 0 {
		c.ModemAPI.MaxBatchSize = 100
	}
	if c.ModemAPI.MaxBodySizeMB == 0 {
		c.ModemAPI.MaxBodySizeMB = 1
	}

	// Validate callers
	seenCallerIDs := make(map[string]bool)
	for i, caller := range c.ModemAPI.Callers {
		if caller.CallerID == "" {
			return fmt.Errorf("modem_api.callers[%d].caller_id is required", i)
		}
		if seenCallerIDs[caller.CallerID] {
			return fmt.Errorf("modem_api.callers[%d].caller_id '%s' is duplicated", i, caller.CallerID)
		}
		seenCallerIDs[caller.CallerID] = true

		if caller.APIKeyHash == "" {
			return fmt.Errorf("modem_api.callers[%d].api_key_hash is required for caller '%s'", i, caller.CallerID)
		}

		// Validate prefix mode
		validPrefixModes := []string{"include", "exclude", "all"}
		prefixModeValid := false
		for _, mode := range validPrefixModes {
			if caller.PrefixMode == mode {
				prefixModeValid = true
				break
			}
		}
		if !prefixModeValid {
			return fmt.Errorf("modem_api.callers[%d].prefix_mode must be one of %v, got: %s", i, validPrefixModes, caller.PrefixMode)
		}

		// Set default priority if not set
		if c.ModemAPI.Callers[i].Priority == 0 {
			c.ModemAPI.Callers[i].Priority = 10
		}
	}

	return nil
}
