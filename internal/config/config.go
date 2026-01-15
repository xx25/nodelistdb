package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nodelistdb/internal/database"
	"gopkg.in/yaml.v3"
)

// ClickHouseConfig holds ClickHouse database connection configuration
type ClickHouseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	UseSSL   bool   `yaml:"use_ssl,omitempty"`

	// Connection settings
	MaxOpenConns int    `yaml:"max_open_conns,omitempty"`
	MaxIdleConns int    `yaml:"max_idle_conns,omitempty"`
	DialTimeout  string `yaml:"dial_timeout,omitempty"`
	ReadTimeout  string `yaml:"read_timeout,omitempty"`
	WriteTimeout string `yaml:"write_timeout,omitempty"`
	Compression  string `yaml:"compression,omitempty"` // none, zstd, lz4, gzip
}

// Config represents the complete application configuration
type Config struct {
	ClickHouse        ClickHouseConfig `yaml:"clickhouse"`
	Cache             CacheConfig      `yaml:"cache"`
	FTP               FTPConfig        `yaml:"ftp"`
	ModemAPI          ModemAPIConfig   `yaml:"modem_api"`
	ServerLogging     LoggingConfig    `yaml:"server_logging"`
	ParserLogging     LoggingConfig    `yaml:"parser_logging"`
	TestdaemonLogging LoggingConfig    `yaml:"testdaemon_logging"`

	// Deprecated: Use component-specific logging configs instead
	Logging LoggingConfig `yaml:"logging,omitempty"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string `yaml:"level"`       // debug, info, warn, error
	File       string `yaml:"file"`        // log file path (optional)
	MaxSize    int    `yaml:"max_size"`    // megabytes
	MaxBackups int    `yaml:"max_backups"` // number of old log files to keep
	MaxAge     int    `yaml:"max_age"`     // days
	Console    bool   `yaml:"console"`     // also log to console
	JSON       bool   `yaml:"json"`        // JSON format instead of text
}

// CacheConfig holds cache configuration
// Note: Only BadgerCache is supported. MemoryCache and NoOpCache have been removed.
type CacheConfig struct {
	Enabled           bool          `yaml:"enabled"`
	Path              string        `yaml:"path"`
	MaxMemoryMB       int           `yaml:"max_memory_mb"`
	ValueLogMaxMB     int           `yaml:"value_log_max_mb"`
	DefaultTTL        time.Duration `yaml:"default_ttl"`
	NodeTTL           time.Duration `yaml:"node_ttl"`
	StatsTTL          time.Duration `yaml:"stats_ttl"`
	SearchTTL         time.Duration `yaml:"search_ttl"`
	MaxSearchResults  int           `yaml:"max_search_results"`
	WarmupOnStart     bool          `yaml:"warmup_on_start"`
	CompactOnClose    bool          `yaml:"compact_on_close"`
	ClearAllOnImport  bool          `yaml:"clear_all_on_import"`
	GCInterval        time.Duration `yaml:"gc_interval"`
	GCDiscardRatio    float64       `yaml:"gc_discard_ratio"`
}

// FTPMount represents a virtual path mount in the FTP server
type FTPMount struct {
	VirtualPath string `yaml:"virtual_path"` // Virtual path in FTP (e.g., /fidonet/nodelists)
	RealPath    string `yaml:"real_path"`    // Real filesystem path
}

// FTPConfig holds FTP server configuration
type FTPConfig struct {
	Enabled                    bool          `yaml:"enabled"`
	Host                       string        `yaml:"host"`
	Port                       int           `yaml:"port"`
	NodelistPath               string        `yaml:"nodelist_path"`               // Deprecated: use mounts instead
	Mounts                     []FTPMount    `yaml:"mounts,omitempty"`            // Virtual path mounts
	MaxConnections             int           `yaml:"max_connections"`
	PassivePortMin             int           `yaml:"passive_port_min"`
	PassivePortMax             int           `yaml:"passive_port_max"`
	IdleTimeout                time.Duration `yaml:"idle_timeout"`
	PublicHost                 string        `yaml:"public_host"`
	DisableActiveIPCheck       bool          `yaml:"disable_active_ip_check"` // Disable IP matching for active mode (PORT/EPRT)
}

// Default configurations
func DefaultClickHouseConfig() ClickHouseConfig {
	return ClickHouseConfig{
		Host:         "localhost",
		Port:         9000,
		Database:     "nodelist",
		Username:     "default",
		Password:     "",
		UseSSL:       false,
		MaxOpenConns: 10,
		MaxIdleConns: 5,
		DialTimeout:  "30s",
		ReadTimeout:  "5m",
		WriteTimeout: "1m",
		Compression:  "lz4",
	}
}

func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		Enabled:           false,
		Path:              "./cache/badger",
		MaxMemoryMB:       256,
		ValueLogMaxMB:     100,
		DefaultTTL:        5 * time.Minute,
		NodeTTL:           15 * time.Minute,
		StatsTTL:          1 * time.Hour,
		SearchTTL:         5 * time.Minute,
		MaxSearchResults:  500,
		WarmupOnStart:     false,
		CompactOnClose:    true,
		ClearAllOnImport:  false,
		GCInterval:        10 * time.Minute,
		GCDiscardRatio:    0.5,
	}
}

func DefaultFTPConfig() *FTPConfig {
	return &FTPConfig{
		Enabled:              false,
		Host:                 "0.0.0.0",
		Port:                 2121,
		NodelistPath:         "/home/dp/nodelists",
		MaxConnections:       10,
		PassivePortMin:       50000,
		PassivePortMax:       50100,
		IdleTimeout:          300 * time.Second,
		PublicHost:           "",
		DisableActiveIPCheck: false, // Security enabled by default
	}
}

func DefaultLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		Level:      "info",
		Console:    true,
		JSON:       false,
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     28,
	}
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(configPath string) (*Config, error) {
	// If config file doesn't exist, return default database config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Config{
			ClickHouse: DefaultClickHouseConfig(),
		}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate and set defaults
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Run comprehensive validation
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfig saves configuration to a YAML file
func SaveConfig(config *Config, configPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// validate ensures the configuration is valid and sets defaults where needed
func (c *Config) validate() error {
	// Validate cache configuration (BadgerCache only)
	if c.Cache.Enabled && c.Cache.Path == "" {
		c.Cache.Path = "./cache/badger"
	}
	if c.Cache.MaxMemoryMB == 0 {
		c.Cache.MaxMemoryMB = 256
	}
	if c.Cache.ValueLogMaxMB == 0 {
		c.Cache.ValueLogMaxMB = 100
	}
	if c.Cache.MaxSearchResults == 0 {
		c.Cache.MaxSearchResults = 500
	}
	if c.Cache.GCInterval == 0 {
		c.Cache.GCInterval = 10 * time.Minute
	}
	if c.Cache.GCDiscardRatio == 0 {
		c.Cache.GCDiscardRatio = 0.5
	}

	// Validate FTP configuration
	if c.FTP.Port == 0 {
		c.FTP.Port = 2121
	}
	if c.FTP.Host == "" {
		c.FTP.Host = "0.0.0.0"
	}
	if c.FTP.MaxConnections == 0 {
		c.FTP.MaxConnections = 10
	}
	if c.FTP.PassivePortMin == 0 {
		c.FTP.PassivePortMin = 50000
	}
	if c.FTP.PassivePortMax == 0 {
		c.FTP.PassivePortMax = 50100
	}
	if c.FTP.IdleTimeout == 0 {
		c.FTP.IdleTimeout = 300 * time.Second
	}
	if c.FTP.NodelistPath == "" {
		// Try to get from environment or use default
		if path := os.Getenv("NODELIST_PATH"); path != "" {
			c.FTP.NodelistPath = path
		} else {
			c.FTP.NodelistPath = "/home/dp/nodelists"
		}
	}

	// Handle mounts configuration
	// For backward compatibility: if mounts is empty but nodelist_path is set,
	// create a default mount at /fidonet/nodelists
	if len(c.FTP.Mounts) == 0 && c.FTP.NodelistPath != "" {
		c.FTP.Mounts = []FTPMount{
			{
				VirtualPath: "/fidonet/nodelists",
				RealPath:    c.FTP.NodelistPath,
			},
		}
	}

	// Validate ClickHouse configuration
	if c.ClickHouse.Host == "" {
		return fmt.Errorf("clickhouse host is required")
	}
	if c.ClickHouse.Port == 0 {
		c.ClickHouse.Port = 9000
	}
	if c.ClickHouse.Database == "" {
		return fmt.Errorf("clickhouse database name is required")
	}
	if c.ClickHouse.Username == "" {
		c.ClickHouse.Username = "default"
	}
	// Set defaults for connection settings
	if c.ClickHouse.MaxOpenConns == 0 {
		c.ClickHouse.MaxOpenConns = 10
	}
	if c.ClickHouse.MaxIdleConns == 0 {
		c.ClickHouse.MaxIdleConns = 5
	}
	if c.ClickHouse.DialTimeout == "" {
		c.ClickHouse.DialTimeout = "30s"
	}
	if c.ClickHouse.ReadTimeout == "" {
		c.ClickHouse.ReadTimeout = "5m"
	}
	if c.ClickHouse.WriteTimeout == "" {
		c.ClickHouse.WriteTimeout = "1m"
	}
	if c.ClickHouse.Compression == "" {
		c.ClickHouse.Compression = "lz4"
	}

	// Validate logging configurations for all components
	validLevels := []string{"debug", "info", "warn", "error"}

	// Helper function to validate and set defaults for a logging config
	validateLogging := func(cfg *LoggingConfig, componentName string) error {
		if cfg.Level == "" {
			cfg.Level = "info"
		}
		levelValid := false
		for _, l := range validLevels {
			if cfg.Level == l {
				levelValid = true
				break
			}
		}
		if !levelValid {
			return fmt.Errorf("%s.level must be one of: %v, got: %s", componentName, validLevels, cfg.Level)
		}
		// Set logging defaults if not configured
		if !cfg.Console && cfg.File == "" {
			cfg.Console = true // Default to console if neither configured
		}
		if cfg.MaxSize == 0 {
			cfg.MaxSize = 100
		}
		if cfg.MaxBackups == 0 {
			cfg.MaxBackups = 3
		}
		if cfg.MaxAge == 0 {
			cfg.MaxAge = 28
		}
		return nil
	}

	// Validate each component's logging config
	if err := validateLogging(&c.ServerLogging, "server_logging"); err != nil {
		return err
	}
	if err := validateLogging(&c.ParserLogging, "parser_logging"); err != nil {
		return err
	}
	if err := validateLogging(&c.TestdaemonLogging, "testdaemon_logging"); err != nil {
		return err
	}

	// Handle deprecated 'logging' field for backward compatibility
	if c.Logging.Level != "" || c.Logging.File != "" {
		// If old 'logging' field is present and new ones are empty, copy to all
		if c.ServerLogging.Level == "" && c.ServerLogging.File == "" {
			c.ServerLogging = c.Logging
		}
		if c.ParserLogging.Level == "" && c.ParserLogging.File == "" {
			c.ParserLogging = c.Logging
		}
		if c.TestdaemonLogging.Level == "" && c.TestdaemonLogging.File == "" {
			c.TestdaemonLogging = c.Logging
		}
	}

	// Validate modem API configuration
	if err := c.validateModemAPI(); err != nil {
		return err
	}

	return nil
}

// CreateExampleConfig creates example configuration file
func CreateExampleConfig(dir string) error {
	// ClickHouse database configuration (only supported database)
	config := &Config{
		ClickHouse: DefaultClickHouseConfig(),
		Cache:      *DefaultCacheConfig(),
		FTP:        *DefaultFTPConfig(),
		ModemAPI:   *DefaultModemAPIConfig(),
		Logging:    *DefaultLoggingConfig(),
	}

	if err := SaveConfig(config, filepath.Join(dir, "config.example.yaml")); err != nil {
		return fmt.Errorf("failed to create example config: %w", err)
	}

	return nil
}


// ToClickHouseDatabaseConfig converts ClickHouseConfig to database.ClickHouseConfig
func (c *ClickHouseConfig) ToClickHouseDatabaseConfig() (*database.ClickHouseConfig, error) {
	dialTimeout, err := time.ParseDuration(c.DialTimeout)
	if err != nil {
		return nil, fmt.Errorf("invalid dial_timeout: %w", err)
	}

	readTimeout, err := time.ParseDuration(c.ReadTimeout)
	if err != nil {
		return nil, fmt.Errorf("invalid read_timeout: %w", err)
	}

	writeTimeout, err := time.ParseDuration(c.WriteTimeout)
	if err != nil {
		return nil, fmt.Errorf("invalid write_timeout: %w", err)
	}

	return &database.ClickHouseConfig{
		Host:         c.Host,
		Port:         c.Port,
		Database:     c.Database,
		Username:     c.Username,
		Password:     c.Password,
		UseSSL:       c.UseSSL,
		MaxOpenConns: c.MaxOpenConns,
		MaxIdleConns: c.MaxIdleConns,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		Compression:  c.Compression,
	}, nil
}
