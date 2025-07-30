package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nodelistdb/internal/database"
)

// DatabaseType represents the supported database types
type DatabaseType string

const (
	DatabaseTypeDuckDB     DatabaseType = "duckdb"
	DatabaseTypeClickHouse DatabaseType = "clickhouse"
)

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	Type     DatabaseType `json:"type"`
	DuckDB   *DuckDBConfig `json:"duckdb,omitempty"`
	ClickHouse *ClickHouseConfig `json:"clickhouse,omitempty"`
}

// DuckDBConfig holds DuckDB-specific configuration
type DuckDBConfig struct {
	Path        string `json:"path"`
	MemoryLimit string `json:"memory_limit,omitempty"`
	Threads     int    `json:"threads,omitempty"`
	ReadOnly    bool   `json:"read_only,omitempty"`
	
	// Performance settings for bulk loading
	BulkMode             bool   `json:"bulk_mode,omitempty"`              // Enable bulk loading optimizations
	DisableWAL           bool   `json:"disable_wal,omitempty"`            // Disable Write-Ahead Logging
	CheckpointThreshold  string `json:"checkpoint_threshold,omitempty"`   // Memory threshold before checkpoint
	WALAutoCheckpoint    int    `json:"wal_auto_checkpoint,omitempty"`    // Pages before auto checkpoint (0 = disabled)
	TempDirectory        string `json:"temp_directory,omitempty"`         // Temporary files directory
}

// ClickHouseConfig holds ClickHouse-specific configuration
type ClickHouseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`
	UseSSL   bool   `json:"use_ssl,omitempty"`
	
	// Connection settings
	MaxOpenConns    int    `json:"max_open_conns,omitempty"`
	MaxIdleConns    int    `json:"max_idle_conns,omitempty"`
	DialTimeout     string `json:"dial_timeout,omitempty"`
	ReadTimeout     string `json:"read_timeout,omitempty"`
	WriteTimeout    string `json:"write_timeout,omitempty"`
	Compression     string `json:"compression,omitempty"` // none, zstd, lz4, gzip
}

// Config represents the complete application configuration
type Config struct {
	Database DatabaseConfig `json:"database"`
}

// Default configurations
func DefaultDuckDBConfig() *DuckDBConfig {
	return &DuckDBConfig{
		Path:                "./nodelist.duckdb",
		MemoryLimit:         "16GB",
		Threads:             8,
		ReadOnly:            false,
		BulkMode:            false,
		DisableWAL:          false,
		CheckpointThreshold: "1GB",
		WALAutoCheckpoint:   1000,
		TempDirectory:       "/tmp",
	}
}

func DefaultClickHouseConfig() *ClickHouseConfig {
	return &ClickHouseConfig{
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

// LoadConfig loads configuration from a JSON file
func LoadConfig(configPath string) (*Config, error) {
	// If config file doesn't exist, return default DuckDB config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Config{
			Database: DatabaseConfig{
				Type:   DatabaseTypeDuckDB,
				DuckDB: DefaultDuckDBConfig(),
			},
		}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate and set defaults
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// SaveConfig saves configuration to a JSON file
func SaveConfig(config *Config, configPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
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
	switch c.Database.Type {
	case DatabaseTypeDuckDB:
		if c.Database.DuckDB == nil {
			c.Database.DuckDB = DefaultDuckDBConfig()
		}
		if c.Database.DuckDB.Path == "" {
			return fmt.Errorf("duckdb path is required")
		}
		if c.Database.DuckDB.MemoryLimit == "" {
			c.Database.DuckDB.MemoryLimit = "16GB"
		}
		if c.Database.DuckDB.Threads == 0 {
			c.Database.DuckDB.Threads = 8
		}
	case DatabaseTypeClickHouse:
		if c.Database.ClickHouse == nil {
			c.Database.ClickHouse = DefaultClickHouseConfig()
		}
		if c.Database.ClickHouse.Host == "" {
			return fmt.Errorf("clickhouse host is required")
		}
		if c.Database.ClickHouse.Port == 0 {
			c.Database.ClickHouse.Port = 9000
		}
		if c.Database.ClickHouse.Database == "" {
			return fmt.Errorf("clickhouse database name is required")
		}
		if c.Database.ClickHouse.Username == "" {
			c.Database.ClickHouse.Username = "default"
		}
		// Set defaults for connection settings
		if c.Database.ClickHouse.MaxOpenConns == 0 {
			c.Database.ClickHouse.MaxOpenConns = 10
		}
		if c.Database.ClickHouse.MaxIdleConns == 0 {
			c.Database.ClickHouse.MaxIdleConns = 5
		}
		if c.Database.ClickHouse.DialTimeout == "" {
			c.Database.ClickHouse.DialTimeout = "30s"
		}
		if c.Database.ClickHouse.ReadTimeout == "" {
			c.Database.ClickHouse.ReadTimeout = "5m"
		}
		if c.Database.ClickHouse.WriteTimeout == "" {
			c.Database.ClickHouse.WriteTimeout = "1m"
		}
		if c.Database.ClickHouse.Compression == "" {
			c.Database.ClickHouse.Compression = "lz4"
		}
	default:
		return fmt.Errorf("unsupported database type: %s", c.Database.Type)
	}

	return nil
}

// GetDSN returns the appropriate DSN string based on database type
func (c *Config) GetDSN() (string, error) {
	switch c.Database.Type {
	case DatabaseTypeDuckDB:
		dsn := c.Database.DuckDB.Path + "?"
		
		// Basic settings
		if c.Database.DuckDB.ReadOnly {
			dsn += "access_mode=read_only&"
		}
		dsn += fmt.Sprintf("memory_limit=%s&threads=%d", 
			c.Database.DuckDB.MemoryLimit, c.Database.DuckDB.Threads)
		
		// Performance settings for bulk mode (only valid connection string options)
		if c.Database.DuckDB.BulkMode || c.Database.DuckDB.DisableWAL {
			// Note: checkpoint_threshold and wal_autocheckpoint must be set via PRAGMA after connection
		}
		
		if c.Database.DuckDB.TempDirectory != "" {
			dsn += fmt.Sprintf("&temp_directory=%s", c.Database.DuckDB.TempDirectory)
		}
		
		return dsn, nil
	case DatabaseTypeClickHouse:
		// ClickHouse DSN will be handled by the ClickHouse adapter
		return "", nil
	default:
		return "", fmt.Errorf("unsupported database type: %s", c.Database.Type)
	}
}

// CreateExampleConfigs creates example configuration files
func CreateExampleConfigs(dir string) error {
	// DuckDB example
	duckdbConfig := &Config{
		Database: DatabaseConfig{
			Type:   DatabaseTypeDuckDB,
			DuckDB: DefaultDuckDBConfig(),
		},
	}

	if err := SaveConfig(duckdbConfig, filepath.Join(dir, "config-duckdb-example.json")); err != nil {
		return fmt.Errorf("failed to create DuckDB example config: %w", err)
	}

	// ClickHouse example
	clickhouseConfig := &Config{
		Database: DatabaseConfig{
			Type:       DatabaseTypeClickHouse,
			ClickHouse: DefaultClickHouseConfig(),
		},
	}

	if err := SaveConfig(clickhouseConfig, filepath.Join(dir, "config-clickhouse-example.json")); err != nil {
		return fmt.Errorf("failed to create ClickHouse example config: %w", err)
	}

	return nil
}

// ToClickHouseDatabaseConfig converts config ClickHouseConfig to database ClickHouseConfig
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