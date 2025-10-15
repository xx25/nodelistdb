package config

import (
	"fmt"
	"strings"
)

// Validator interface for config validation
type Validator interface {
	Validate() error
}

// ValidationErrors collects multiple validation errors
type ValidationErrors struct {
	Errors []error
}

func (ve *ValidationErrors) Add(err error) {
	if err != nil {
		ve.Errors = append(ve.Errors, err)
	}
}

func (ve *ValidationErrors) Error() string {
	if len(ve.Errors) == 0 {
		return ""
	}

	messages := make([]string, len(ve.Errors))
	for i, err := range ve.Errors {
		messages[i] = fmt.Sprintf("  - %s", err.Error())
	}

	return fmt.Sprintf("configuration validation failed:\n%s",
		strings.Join(messages, "\n"))
}

func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.Errors) > 0
}

// Validate validates the entire configuration
func (c *Config) Validate() error {
	var errs ValidationErrors

	// Validate ClickHouse config
	if c.ClickHouse.Host == "" {
		errs.Add(fmt.Errorf("clickhouse configuration is required"))
	} else {
		errs.Add(c.ClickHouse.Validate())
	}

	// Validate cache config if enabled
	if c.Cache.Enabled {
		errs.Add(c.Cache.Validate())
	}

	// Validate FTP config if enabled
	if c.FTP.Enabled {
		errs.Add(c.FTP.Validate())
	}

	// Validate logging config
	errs.Add(c.Logging.Validate())

	if errs.HasErrors() {
		return &errs
	}
	return nil
}

// Validate validates ClickHouse configuration
func (c *ClickHouseConfig) Validate() error {
	var errs ValidationErrors

	if c.Host == "" {
		errs.Add(fmt.Errorf("clickhouse.host is required"))
	}

	if c.Port < 1 || c.Port > 65535 {
		errs.Add(fmt.Errorf("clickhouse.port must be between 1-65535, got %d", c.Port))
	}

	if c.Database == "" {
		errs.Add(fmt.Errorf("clickhouse.database is required"))
	}

	if c.MaxOpenConns < 0 {
		errs.Add(fmt.Errorf("clickhouse.max_open_conns cannot be negative"))
	}

	if c.MaxIdleConns < 0 {
		errs.Add(fmt.Errorf("clickhouse.max_idle_conns cannot be negative"))
	}

	if c.MaxOpenConns > 0 && c.MaxIdleConns > c.MaxOpenConns {
		errs.Add(fmt.Errorf("clickhouse.max_idle_conns (%d) cannot exceed max_open_conns (%d)",
			c.MaxIdleConns, c.MaxOpenConns))
	}

	if errs.HasErrors() {
		return &errs
	}
	return nil
}

// Validate validates cache configuration
func (c *CacheConfig) Validate() error {
	var errs ValidationErrors

	if c.Path == "" {
		errs.Add(fmt.Errorf("cache.path is required when cache is enabled"))
	}

	if c.MaxMemoryMB < 1 {
		errs.Add(fmt.Errorf("cache.max_memory_mb must be positive, got %d", c.MaxMemoryMB))
	}

	if c.ValueLogMaxMB < 1 {
		errs.Add(fmt.Errorf("cache.value_log_max_mb must be positive, got %d", c.ValueLogMaxMB))
	}

	if c.MaxSearchResults < 1 {
		errs.Add(fmt.Errorf("cache.max_search_results must be positive, got %d", c.MaxSearchResults))
	}

	if c.GCDiscardRatio < 0 || c.GCDiscardRatio > 1 {
		errs.Add(fmt.Errorf("cache.gc_discard_ratio must be between 0 and 1, got %.2f", c.GCDiscardRatio))
	}

	if errs.HasErrors() {
		return &errs
	}
	return nil
}

// Validate validates FTP configuration
func (c *FTPConfig) Validate() error {
	var errs ValidationErrors

	if c.Host == "" {
		errs.Add(fmt.Errorf("ftp.host is required when FTP is enabled"))
	}

	if c.Port < 1 || c.Port > 65535 {
		errs.Add(fmt.Errorf("ftp.port must be between 1-65535, got %d", c.Port))
	}

	if c.NodelistPath == "" {
		errs.Add(fmt.Errorf("ftp.nodelist_path is required when FTP is enabled"))
	}

	if c.MaxConnections < 1 {
		errs.Add(fmt.Errorf("ftp.max_connections must be at least 1, got %d", c.MaxConnections))
	}

	if c.PassivePortMin < 1024 || c.PassivePortMin > 65535 {
		errs.Add(fmt.Errorf("ftp.passive_port_min must be between 1024-65535, got %d", c.PassivePortMin))
	}

	if c.PassivePortMax < 1024 || c.PassivePortMax > 65535 {
		errs.Add(fmt.Errorf("ftp.passive_port_max must be between 1024-65535, got %d", c.PassivePortMax))
	}

	if c.PassivePortMin > c.PassivePortMax {
		errs.Add(fmt.Errorf("ftp.passive_port_min (%d) cannot exceed passive_port_max (%d)",
			c.PassivePortMin, c.PassivePortMax))
	}

	if c.IdleTimeout < 1 {
		errs.Add(fmt.Errorf("ftp.idle_timeout must be positive"))
	}

	if errs.HasErrors() {
		return &errs
	}
	return nil
}

// Validate validates logging configuration
func (c *LoggingConfig) Validate() error {
	var errs ValidationErrors

	validLevels := []string{"debug", "info", "warn", "error"}
	levelValid := false
	for _, l := range validLevels {
		if c.Level == l {
			levelValid = true
			break
		}
	}
	if !levelValid && c.Level != "" {
		errs.Add(fmt.Errorf("logging.level must be one of: %v, got %s", validLevels, c.Level))
	}

	if c.MaxSize < 0 {
		errs.Add(fmt.Errorf("logging.max_size cannot be negative, got %d", c.MaxSize))
	}

	if c.MaxBackups < 0 {
		errs.Add(fmt.Errorf("logging.max_backups cannot be negative, got %d", c.MaxBackups))
	}

	if c.MaxAge < 0 {
		errs.Add(fmt.Errorf("logging.max_age cannot be negative, got %d", c.MaxAge))
	}

	if errs.HasErrors() {
		return &errs
	}
	return nil
}
