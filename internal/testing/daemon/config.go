package daemon

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete daemon configuration
type Config struct {
	Daemon     DaemonConfig    `yaml:"daemon"`
	Database   DatabaseConfig  `yaml:"database"`
	Protocols  ProtocolsConfig `yaml:"protocols"`
	Services   ServicesConfig  `yaml:"services"`
	Cache      CacheConfig     `yaml:"cache"`
	Logging    LoggingConfig   `yaml:"logging"`
	CLI        CLIConfig       `yaml:"cli"`
	ConfigPath string          `yaml:"-"` // Path to config file, set when loading
}

// DaemonConfig contains daemon-specific settings
type DaemonConfig struct {
	TestInterval time.Duration `yaml:"test_interval"`
	Workers      int           `yaml:"workers"`
	BatchSize    int           `yaml:"batch_size"`
	RunOnce      bool          `yaml:"-"` // Set from command line
	DryRun       bool          `yaml:"-"` // Set from command line
	CLIOnly      bool          `yaml:"-"` // Set from command line - disable automatic testing
}

// DatabaseConfig contains database connection settings
type DatabaseConfig struct {
	Type       string                `yaml:"type"` // "duckdb" or "clickhouse"
	DuckDB     *DuckDBConfig        `yaml:"duckdb,omitempty"`
	ClickHouse *ClickHouseConfig    `yaml:"clickhouse,omitempty"`
}

// DuckDBConfig for DuckDB database
type DuckDBConfig struct {
	NodesPath   string `yaml:"nodes_path"`   // Path to nodes database
	ResultsPath string `yaml:"results_path"` // Path to test results database
}

// ClickHouseConfig for ClickHouse database
type ClickHouseConfig struct {
	Host          string        `yaml:"host"`
	Port          int           `yaml:"port"`
	Database      string        `yaml:"database"`
	Username      string        `yaml:"username"`
	Password      string        `yaml:"password"`
	BatchSize     int           `yaml:"batch_size"`
	FlushInterval time.Duration `yaml:"flush_interval"`
}

// ProtocolsConfig contains protocol testing settings
type ProtocolsConfig struct {
	BinkP  ProtocolConfig `yaml:"binkp"`
	Ifcico ProtocolConfig `yaml:"ifcico"`
	Telnet ProtocolConfig `yaml:"telnet"`
	FTP    ProtocolConfig `yaml:"ftp"`
	VModem ProtocolConfig `yaml:"vmodem"`
}

// ProtocolConfig for individual protocol settings
type ProtocolConfig struct {
	Enabled    bool          `yaml:"enabled"`
	Port       int           `yaml:"port"`
	Timeout    time.Duration `yaml:"timeout"`
	OurAddress string        `yaml:"our_address,omitempty"` // For BinkP
}

// ServicesConfig contains external service settings
type ServicesConfig struct {
	Geolocation GeolocationConfig `yaml:"geolocation"`
	DNS         DNSConfig         `yaml:"dns"`
}

// GeolocationConfig for IP geolocation service
type GeolocationConfig struct {
	Provider  string        `yaml:"provider"`
	APIKey    string        `yaml:"api_key"`
	CacheTTL  time.Duration `yaml:"cache_ttl"`
	RateLimit int           `yaml:"rate_limit"`
}

// DNSConfig for DNS resolution service
type DNSConfig struct {
	Workers  int           `yaml:"workers"`
	Timeout  time.Duration `yaml:"timeout"`
	CacheTTL time.Duration `yaml:"cache_ttl"`
}

// CacheConfig for local caching
type CacheConfig struct {
	Enabled bool   `yaml:"enabled"`
	Type    string `yaml:"type"` // badger or bolt
	Path    string `yaml:"path"`
}

// LoggingConfig for logging settings
type LoggingConfig struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
}

// CLIConfig contains CLI server settings
type CLIConfig struct {
	Enabled        bool          `yaml:"enabled"`
	Host           string        `yaml:"host"`
	Port           int           `yaml:"port"`
	MaxClients     int           `yaml:"max_clients"`
	Timeout        time.Duration `yaml:"timeout"`
	Prompt         string        `yaml:"prompt"`
	WelcomeMessage string        `yaml:"welcome_message"`
}

// LoadConfig loads configuration from YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	
	// Store the config path for reloading
	cfg.ConfigPath = path

	// Set defaults
	if cfg.Daemon.Workers == 0 {
		cfg.Daemon.Workers = 10
	}
	if cfg.Daemon.BatchSize == 0 {
		cfg.Daemon.BatchSize = 100
	}
	if cfg.Daemon.TestInterval == 0 {
		cfg.Daemon.TestInterval = 3600  // Will be converted to Duration later
	}
	
	// Database-specific defaults
	if cfg.Database.Type == "clickhouse" && cfg.Database.ClickHouse != nil {
		if cfg.Database.ClickHouse.BatchSize == 0 {
			cfg.Database.ClickHouse.BatchSize = 1000
		}
		if cfg.Database.ClickHouse.FlushInterval == 0 {
			cfg.Database.ClickHouse.FlushInterval = 30  // Will be converted to Duration later
		}
	}
	
	if cfg.Services.DNS.Workers == 0 {
		cfg.Services.DNS.Workers = 20
	}
	if cfg.Services.DNS.Timeout == 0 {
		cfg.Services.DNS.Timeout = 5  // Will be converted to Duration later
	}
	if cfg.Cache.Type == "" {
		cfg.Cache.Type = "badger"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}

	// Convert seconds to Duration for YAML unmarshaling
	// Only convert if the value looks like plain seconds (< 1000)
	// Values >= 1000 are likely already time.Duration in nanoseconds
	
	if cfg.Daemon.TestInterval < 1000 {
		cfg.Daemon.TestInterval *= time.Second
	}
	
	if cfg.Database.Type == "clickhouse" && cfg.Database.ClickHouse != nil {
		if cfg.Database.ClickHouse.FlushInterval < 1000 {
			cfg.Database.ClickHouse.FlushInterval *= time.Second
		}
	}
	
	if cfg.Protocols.BinkP.Timeout < 1000 {
		cfg.Protocols.BinkP.Timeout *= time.Second
	}
	if cfg.Protocols.Ifcico.Timeout < 1000 {
		cfg.Protocols.Ifcico.Timeout *= time.Second
	}
	if cfg.Protocols.Telnet.Timeout < 1000 {
		cfg.Protocols.Telnet.Timeout *= time.Second
	}
	if cfg.Protocols.FTP.Timeout < 1000 {
		cfg.Protocols.FTP.Timeout *= time.Second
	}
	if cfg.Protocols.VModem.Timeout < 1000 {
		cfg.Protocols.VModem.Timeout *= time.Second
	}
	if cfg.Services.Geolocation.CacheTTL < 1000 {
		cfg.Services.Geolocation.CacheTTL *= time.Second
	}
	if cfg.Services.DNS.Timeout < 1000 {
		cfg.Services.DNS.Timeout *= time.Second
	}
	if cfg.Services.DNS.CacheTTL < 1000 {
		cfg.Services.DNS.CacheTTL *= time.Second
	}
	
	// CLI defaults
	if cfg.CLI.Host == "" {
		cfg.CLI.Host = "127.0.0.1"
	}
	if cfg.CLI.Port == 0 {
		cfg.CLI.Port = 2323
	}
	if cfg.CLI.MaxClients == 0 {
		cfg.CLI.MaxClients = 5
	}
	if cfg.CLI.Timeout == 0 {
		cfg.CLI.Timeout = 300  // 300 seconds default
	}
	// Convert CLI timeout to Duration if needed
	if cfg.CLI.Timeout < 1000 {
		cfg.CLI.Timeout *= time.Second
	}
	if cfg.CLI.Prompt == "" {
		cfg.CLI.Prompt = "> "
	}
	if cfg.CLI.WelcomeMessage == "" {
		cfg.CLI.WelcomeMessage = "NodelistDB Test Daemon CLI v1.0.0\nType 'help' for available commands.\n"
	}

	return &cfg, nil
}

// Validate checks if configuration is valid
func (c *Config) Validate() error {
	// Check database configuration
	if c.Database.Type == "" {
		return fmt.Errorf("database.type is required (duckdb or clickhouse)")
	}
	
	switch c.Database.Type {
	case "duckdb":
		if c.Database.DuckDB == nil {
			return fmt.Errorf("database.duckdb configuration is required when type is duckdb")
		}
		if c.Database.DuckDB.NodesPath == "" {
			return fmt.Errorf("database.duckdb.nodes_path is required")
		}
		if c.Database.DuckDB.ResultsPath == "" {
			return fmt.Errorf("database.duckdb.results_path is required")
		}
	case "clickhouse":
		if c.Database.ClickHouse == nil {
			return fmt.Errorf("database.clickhouse configuration is required when type is clickhouse")
		}
		if c.Database.ClickHouse.Host == "" {
			return fmt.Errorf("database.clickhouse.host is required")
		}
		if c.Database.ClickHouse.Database == "" {
			return fmt.Errorf("database.clickhouse.database is required")
		}
	default:
		return fmt.Errorf("unsupported database type: %s (must be duckdb or clickhouse)", c.Database.Type)
	}
	
	// Check if at least one protocol is enabled
	if !c.Protocols.BinkP.Enabled && !c.Protocols.Ifcico.Enabled &&
		!c.Protocols.Telnet.Enabled && !c.Protocols.FTP.Enabled &&
		!c.Protocols.VModem.Enabled {
		return fmt.Errorf("at least one protocol must be enabled")
	}
	
	return nil
}