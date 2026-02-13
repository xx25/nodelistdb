package config

import (
	"fmt"
)

// ModemAPIConfig holds configuration for the modem testing API
type ModemAPIConfig struct {
	Enabled bool `yaml:"enabled"`

	// Caller definitions for API key authentication
	Callers []ModemCallerConfig `yaml:"callers,omitempty"`

	// Request limits
	MaxBatchSize  int `yaml:"max_batch_size"`   // Max results per request
	MaxBodySizeMB int `yaml:"max_body_size_mb"` // Max request body size in MB
}

// ModemCallerConfig holds configuration for a single API caller
type ModemCallerConfig struct {
	CallerID   string `yaml:"caller_id"`    // Unique identifier (e.g., "modem-cli")
	Name       string `yaml:"name"`         // Human-readable name (e.g., "CLI Modem Tester")
	APIKeyHash string `yaml:"api_key_hash"` // SHA256 hash of API key (e.g., "sha256:abc123...")
}

// DefaultModemAPIConfig returns default configuration for the modem API
func DefaultModemAPIConfig() *ModemAPIConfig {
	return &ModemAPIConfig{
		Enabled:       false,
		Callers:       []ModemCallerConfig{},
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
	}

	return nil
}
