package daemon

import (
	"fmt"
	"os"
	"time"

	"github.com/nodelistdb/internal/testing/protocols/emsi"
	"gopkg.in/yaml.v3"
)

// Config represents the complete daemon configuration
type Config struct {
	Daemon          DaemonConfig       `yaml:"daemon"`
	ClickHouse      *ClickHouseConfig  `yaml:"clickhouse"`
	Protocols       ProtocolsConfig    `yaml:"protocols"`
	Services        ServicesConfig     `yaml:"services"`
	Testing         TestingConfig      `yaml:"testing"` // Protocol-specific testing configurations
	Cache           CacheConfig        `yaml:"cache"` // Not used by testdaemon
	TestdaemonCache CacheConfig        `yaml:"testdaemon_cache"` // Required cache config for testdaemon
	Logging         LoggingConfig      `yaml:"testdaemon_logging"` // Testdaemon-specific logging config
	CLI             CLIConfig          `yaml:"cli"`
	ConfigPath      string             `yaml:"-"` // Path to config file, set when loading
	Version         string             `yaml:"-"` // Version string, set from main
}

// TestingConfig contains protocol-specific testing configurations
type TestingConfig struct {
	EMSI EMSITestingConfig `yaml:"emsi"`
}

// EMSITestingConfig contains EMSI handshake configuration
// Used by IFCICO protocol tester for per-node handshake tuning
type EMSITestingConfig struct {
	// Global EMSI config (optional - FSC-0056.001 defaults used if omitted)
	Global *emsi.Config `yaml:"global"`
	// Per-node overrides keyed by FidoNet address.
	// Address format must match the output of node.Address() - typically "zone:net/node"
	// Examples: "2:5020/2021", "1:123/456"
	// Note: Point addresses should omit ".0" suffix (use "2:5020/2021" not "2:5020/2021.0")
	// Domain-qualified addresses (e.g., "2:5020/2021@fidonet") are NOT supported.
	Overrides map[string]*emsi.NodeOverride `yaml:"overrides"`
}

// DaemonConfig contains daemon-specific settings
type DaemonConfig struct {
	TestInterval      time.Duration `yaml:"test_interval"`
	Workers           int           `yaml:"workers"`
	BatchSize         int           `yaml:"batch_size"`
	StaleTestThreshold time.Duration `yaml:"stale_test_threshold"` // Consider test stale after this duration (default: same as test_interval)
	FailedRetryInterval time.Duration `yaml:"failed_retry_interval"` // Retry failed nodes after this duration (default: 24h)
	RunOnce           bool          `yaml:"-"` // Set from command line
	DryRun            bool          `yaml:"-"` // Set from command line
	CLIOnly           bool          `yaml:"-"` // Set from command line - disable automatic testing
	TestLimit         string        `yaml:"-"` // Set from command line - limit to specific node(s)
}

// ClickHouseConfig for ClickHouse database
type ClickHouseConfig struct {
	Host          string        `yaml:"host"`
	Port          int           `yaml:"port"`
	Database      string        `yaml:"database"`
	Username      string        `yaml:"username"`
	Password      string        `yaml:"password"`
	UseSSL        bool          `yaml:"use_ssl"`
	MaxOpenConns  int           `yaml:"max_open_conns"`
	MaxIdleConns  int           `yaml:"max_idle_conns"`
	DialTimeout   time.Duration `yaml:"dial_timeout"`
	ReadTimeout   time.Duration `yaml:"read_timeout"`
	WriteTimeout  time.Duration `yaml:"write_timeout"`
	Compression   string        `yaml:"compression"`
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
	SystemName string        `yaml:"system_name,omitempty"` // SYS field
	Sysop      string        `yaml:"sysop,omitempty"`       // ZYZ field
	Location   string        `yaml:"location,omitempty"`    // LOC field
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
	Console    bool   `yaml:"console"`
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
	// Set StaleTestThreshold to same as TestInterval if not specified
	if cfg.Daemon.StaleTestThreshold == 0 {
		cfg.Daemon.StaleTestThreshold = cfg.Daemon.TestInterval
	}
	// Set FailedRetryInterval to 24h if not specified
	if cfg.Daemon.FailedRetryInterval == 0 {
		cfg.Daemon.FailedRetryInterval = 24 * 3600  // 24 hours, will be converted to Duration later
	}
	
	// ClickHouse-specific defaults
	if cfg.ClickHouse != nil {
		if cfg.ClickHouse.BatchSize == 0 {
			cfg.ClickHouse.BatchSize = 1000
		}
		if cfg.ClickHouse.FlushInterval == 0 {
			cfg.ClickHouse.FlushInterval = 30  // Will be converted to Duration later
		}
	}
	
	if cfg.Services.DNS.Workers == 0 {
		cfg.Services.DNS.Workers = 20
	}
	if cfg.Services.DNS.Timeout == 0 {
		cfg.Services.DNS.Timeout = 5  // Will be converted to Duration later
	}
	
	// Set defaults for testdaemon_cache
	if cfg.TestdaemonCache.Type == "" {
		cfg.TestdaemonCache.Type = "badger"
	}
	if cfg.TestdaemonCache.Path == "" {
		cfg.TestdaemonCache.Path = "./cache/badger-testdaemon"
	}
	
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}

	// Convert seconds to Duration for YAML unmarshaling
	// Only convert if the value looks like plain seconds
	// Values >= 1e9 (1 second in nanoseconds) are likely already time.Duration
	const oneSecondInNanos = int64(time.Second)

	if cfg.Daemon.TestInterval < time.Duration(oneSecondInNanos) {
		cfg.Daemon.TestInterval *= time.Second
	}
	if cfg.Daemon.StaleTestThreshold < time.Duration(oneSecondInNanos) {
		cfg.Daemon.StaleTestThreshold *= time.Second
	}
	if cfg.Daemon.FailedRetryInterval < time.Duration(oneSecondInNanos) {
		cfg.Daemon.FailedRetryInterval *= time.Second
	}
	
	if cfg.ClickHouse != nil {
		if cfg.ClickHouse.FlushInterval < time.Duration(oneSecondInNanos) {
			cfg.ClickHouse.FlushInterval *= time.Second
		}
	}

	if cfg.Protocols.BinkP.Timeout < time.Duration(oneSecondInNanos) {
		cfg.Protocols.BinkP.Timeout *= time.Second
	}
	// Set default system info for BinkP if not specified
	if cfg.Protocols.BinkP.SystemName == "" {
		cfg.Protocols.BinkP.SystemName = "NodelistDB Test Daemon"
	}
	if cfg.Protocols.BinkP.Sysop == "" {
		cfg.Protocols.BinkP.Sysop = "Test Operator"
	}
	if cfg.Protocols.BinkP.Location == "" {
		cfg.Protocols.BinkP.Location = "Test Location"
	}
	
	if cfg.Protocols.Ifcico.Timeout < time.Duration(oneSecondInNanos) {
		cfg.Protocols.Ifcico.Timeout *= time.Second
	}
	// Set default system info for Ifcico if not specified
	if cfg.Protocols.Ifcico.SystemName == "" {
		cfg.Protocols.Ifcico.SystemName = "NodelistDB Test Daemon"
	}
	if cfg.Protocols.Ifcico.Sysop == "" {
		cfg.Protocols.Ifcico.Sysop = "Test Operator"
	}
	if cfg.Protocols.Ifcico.Location == "" {
		cfg.Protocols.Ifcico.Location = "Test Location"
	}
	if cfg.Protocols.Telnet.Timeout < time.Duration(oneSecondInNanos) {
		cfg.Protocols.Telnet.Timeout *= time.Second
	}
	if cfg.Protocols.FTP.Timeout < time.Duration(oneSecondInNanos) {
		cfg.Protocols.FTP.Timeout *= time.Second
	}
	if cfg.Protocols.VModem.Timeout < time.Duration(oneSecondInNanos) {
		cfg.Protocols.VModem.Timeout *= time.Second
	}
	if cfg.Services.Geolocation.CacheTTL < time.Duration(oneSecondInNanos) {
		cfg.Services.Geolocation.CacheTTL *= time.Second
	}
	if cfg.Services.DNS.Timeout < time.Duration(oneSecondInNanos) {
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

	// Normalize EMSI duration fields (convert numeric seconds to time.Duration)
	// EMSI config uses time.Duration which YAML may unmarshal as plain nanoseconds
	normalizeEMSIConfig(&cfg)

	return &cfg, nil
}

// normalizeEMSIConfig normalizes duration fields in EMSI configuration.
// YAML may unmarshal numeric values (e.g., 60) as nanoseconds instead of seconds.
// This function converts values that look like plain seconds to proper time.Duration.
func normalizeEMSIConfig(cfg *Config) {
	const oneSecondInNanos = int64(time.Second)

	// Helper to normalize a duration value
	normalizeDuration := func(d time.Duration) time.Duration {
		if d > 0 && d < time.Duration(oneSecondInNanos) {
			return d * time.Second
		}
		return d
	}

	// Helper to normalize a duration pointer
	normalizeDurationPtr := func(d *time.Duration) {
		if d != nil && *d > 0 && *d < time.Duration(oneSecondInNanos) {
			*d *= time.Second
		}
	}

	// Normalize global EMSI config
	if cfg.Testing.EMSI.Global != nil {
		g := cfg.Testing.EMSI.Global
		g.MasterTimeout = normalizeDuration(g.MasterTimeout)
		g.StepTimeout = normalizeDuration(g.StepTimeout)
		g.FirstStepTimeout = normalizeDuration(g.FirstStepTimeout)
		g.CharacterTimeout = normalizeDuration(g.CharacterTimeout)
		g.RetryDelay = normalizeDuration(g.RetryDelay)
		g.InitialCRInterval = normalizeDuration(g.InitialCRInterval)
		g.InitialCRTimeout = normalizeDuration(g.InitialCRTimeout)
		g.PreventiveINQTimeout = normalizeDuration(g.PreventiveINQTimeout)
		g.INQInterval = normalizeDuration(g.INQInterval)
	}

	// Normalize per-node EMSI overrides
	for _, override := range cfg.Testing.EMSI.Overrides {
		if override == nil {
			continue
		}
		normalizeDurationPtr(override.MasterTimeout)
		normalizeDurationPtr(override.StepTimeout)
		normalizeDurationPtr(override.FirstStepTimeout)
		normalizeDurationPtr(override.CharacterTimeout)
		normalizeDurationPtr(override.RetryDelay)
		normalizeDurationPtr(override.InitialCRInterval)
		normalizeDurationPtr(override.InitialCRTimeout)
		normalizeDurationPtr(override.PreventiveINQTimeout)
		normalizeDurationPtr(override.INQInterval)
	}
}

// Validate checks if configuration is valid
func (c *Config) Validate() error {
	// Check ClickHouse database configuration
	if c.ClickHouse == nil {
		return fmt.Errorf("clickhouse configuration is required")
	}
	if c.ClickHouse.Host == "" {
		return fmt.Errorf("clickhouse.host is required")
	}
	if c.ClickHouse.Database == "" {
		return fmt.Errorf("clickhouse.database is required")
	}
	
	// Check if at least one protocol is enabled
	if !c.Protocols.BinkP.Enabled && !c.Protocols.Ifcico.Enabled &&
		!c.Protocols.Telnet.Enabled && !c.Protocols.FTP.Enabled &&
		!c.Protocols.VModem.Enabled {
		return fmt.Errorf("at least one protocol must be enabled")
	}
	
	return nil
}