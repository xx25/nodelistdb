package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nodelistdb/internal/cache"
	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/ftp"
	"github.com/nodelistdb/internal/storage"
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
	Protocol string `yaml:"protocol,omitempty"` // "native" (default) or "http"

	// Connection settings
	MaxOpenConns int    `yaml:"max_open_conns,omitempty"`
	MaxIdleConns int    `yaml:"max_idle_conns,omitempty"`
	DialTimeout  string `yaml:"dial_timeout,omitempty"`
	ReadTimeout  string `yaml:"read_timeout,omitempty"`
	WriteTimeout string `yaml:"write_timeout,omitempty"`
	Compression  string `yaml:"compression,omitempty"` // none, zstd, lz4, gzip
}

// NetworkConfig describes one FTN network the system knows about
type NetworkConfig struct {
	Name            string `yaml:"name"`             // Lowercase network name (fidonet, fsxnet, ...)
	NodelistPattern string `yaml:"nodelist_pattern"` // Regex matched against nodelist filenames (.gz stripped first)
	Path            string `yaml:"path,omitempty"`   // Optional default nodelist directory for this network

	compiledPattern *regexp.Regexp
}

// Pattern returns the compiled nodelist filename pattern.
func (nc *NetworkConfig) Pattern() *regexp.Regexp {
	return nc.compiledPattern
}

// Config represents the complete application configuration
type Config struct {
	ClickHouse        ClickHouseConfig  `yaml:"clickhouse"`
	Cache             CacheConfig       `yaml:"cache"`
	FTP               FTPConfig         `yaml:"ftp"`
	ModemAPI          ModemAPIConfig    `yaml:"modem_api"`
	Networks          []NetworkConfig   `yaml:"networks,omitempty"` // FTN networks (defaults to fidonet if absent)
	LinksFile         string            `yaml:"links_file"`         // Path to links.yaml for external FidoNet links
	QueryBudget       QueryBudgetConfig `yaml:"query_budget,omitempty"`
	RateLimit         RateLimitConfig   `yaml:"rate_limit,omitempty"`
	ServerLogging     LoggingConfig     `yaml:"server_logging"`
	ParserLogging     LoggingConfig     `yaml:"parser_logging"`
	TestdaemonLogging LoggingConfig     `yaml:"testdaemon_logging"`

	// Deprecated: Use component-specific logging configs instead
	Logging LoggingConfig `yaml:"logging,omitempty"`
}

// Network returns the configuration for a network by name, or nil if unknown.
func (c *Config) Network(name string) *NetworkConfig {
	for i := range c.Networks {
		if c.Networks[i].Name == name {
			return &c.Networks[i]
		}
	}
	return nil
}

// QueryBudgetConfig bounds how long one HTTP request's database work may run.
//
// Off by default, and deliberately so: cancellation (which needs no deadline)
// ships on for everyone, while a deadline is a policy with real failure modes
// - notably that clickhouse-go converts it into a max_execution_time setting
// that readonly HTTP users are not permitted to send. See internal/querybudget
// for the full reasoning; it withholds the budget on protocol: http by itself.
//
// The analytics pages get their own, longer value because the latest_nodes
// CTEs behind them are the slow ones; everything else runs under Default.
type QueryBudgetConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Default   string `yaml:"default,omitempty"`   // budget for ordinary pages and API reads
	Analytics string `yaml:"analytics,omitempty"` // budget for the heavy analytics reports
}

// Durations parses the two budgets. A budget left empty is zero, which
// querybudget.New reads as "no budget for this group".
func (q QueryBudgetConfig) Durations() (def, analytics time.Duration, err error) {
	if q.Default != "" {
		if def, err = time.ParseDuration(q.Default); err != nil {
			return 0, 0, fmt.Errorf("invalid query_budget.default: %w", err)
		}
	}
	if q.Analytics != "" {
		if analytics, err = time.ParseDuration(q.Analytics); err != nil {
			return 0, 0, fmt.Errorf("invalid query_budget.analytics: %w", err)
		}
	}
	return def, analytics, nil
}

// RateLimitConfig throttles requests per client IP.
//
// On by default. The public front end is a 1 OCPU / 954 MB host serving an
// archive with roughly 110,000 crawlable node pages at about 12 seconds each;
// left open, a single crawler walking that surface keeps a core busy for days,
// and several were. A limiter is the only one of the available defences that
// does not depend on the caller's cooperation the way robots.txt does.
//
// TrustedProxies decides whose X-Forwarded-For is believed and is therefore
// the security-relevant field: leave it empty when the reverse proxy is on
// the same host (the default trusts loopback only), and set it to the proxy's
// address when it is not. Naming a range wider than the actual proxies lets
// anything inside that range choose its own limiter key.
type RateLimitConfig struct {
	Enabled        *bool    `yaml:"enabled,omitempty"`         // nil = on
	TrustedProxies []string `yaml:"trusted_proxies,omitempty"` // empty = loopback
	ExpensiveRPS   float64  `yaml:"expensive_rps,omitempty"`   // sustained, heavy pages
	ExpensiveBurst float64  `yaml:"expensive_burst,omitempty"`
	DefaultRPS     float64  `yaml:"default_rps,omitempty"`
	DefaultBurst   float64  `yaml:"default_burst,omitempty"`
	DownloadRPS    float64  `yaml:"download_rps,omitempty"`
	DownloadBurst  float64  `yaml:"download_burst,omitempty"`
	MaxKeys        int      `yaml:"max_keys,omitempty"`
	Idle           string   `yaml:"idle,omitempty"` // forget a caller after this
}

// On reports whether the limiter runs. The field is a pointer so that a config
// file written before this section existed still gets the protection, while an
// explicit "enabled: false" still turns it off.
func (r RateLimitConfig) On() bool { return r.Enabled == nil || *r.Enabled }

// IdleDuration parses Idle, defaulting to 15 minutes.
func (r RateLimitConfig) IdleDuration() (time.Duration, error) {
	if r.Idle == "" {
		return 15 * time.Minute, nil
	}
	d, err := time.ParseDuration(r.Idle)
	if err != nil {
		return 0, fmt.Errorf("invalid rate_limit.idle: %w", err)
	}
	return d, nil
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
// Supported types: "badger" (disk-based) and "memory" (in-process).
type CacheConfig struct {
	Enabled       bool          `yaml:"enabled"`
	Type          string        `yaml:"type"` // "badger" or "memory" (default: "badger")
	Path          string        `yaml:"path"`
	MaxMemoryMB   int           `yaml:"max_memory_mb"`
	ValueLogMaxMB int           `yaml:"value_log_max_mb"`
	DefaultTTL    time.Duration `yaml:"default_ttl"`
	NodeTTL       time.Duration `yaml:"node_ttl"`
	StatsTTL      time.Duration `yaml:"stats_ttl"`
	SearchTTL     time.Duration `yaml:"search_ttl"`
	// The analytics reports are cached on four horizons, by what can change
	// their answer. test_analytics_ttl covers everything computed from
	// node_test_results, which the test daemon appends to continuously;
	// historical_ttl covers answers only a new nodelist import can move.
	TestAnalyticsTTL time.Duration `yaml:"test_analytics_ttl"`
	AnalyticsTTL     time.Duration `yaml:"analytics_ttl"`
	LongAnalyticsTTL time.Duration `yaml:"long_analytics_ttl"`
	HistoricalTTL    time.Duration `yaml:"historical_ttl"`
	MaxSearchResults int           `yaml:"max_search_results"`
	WarmupOnStart    bool          `yaml:"warmup_on_start"`
	CompactOnClose   bool          `yaml:"compact_on_close"`
	ClearAllOnImport bool          `yaml:"clear_all_on_import"`
	GCInterval       time.Duration `yaml:"gc_interval"`
	GCDiscardRatio   float64       `yaml:"gc_discard_ratio"`
	MaxDiskMB        int           `yaml:"max_disk_mb"`
}

// FTPMount represents a virtual path mount in the FTP server
type FTPMount struct {
	VirtualPath string `yaml:"virtual_path"` // Virtual path in FTP (e.g., /nodelists)
	RealPath    string `yaml:"real_path"`    // Real filesystem path
}

// FTPConfig holds FTP server configuration
type FTPConfig struct {
	Enabled              bool          `yaml:"enabled"`
	Host                 string        `yaml:"host"`
	Port                 int           `yaml:"port"`
	NodelistPath         string        `yaml:"nodelist_path"`    // Deprecated: use mounts instead
	Mounts               []FTPMount    `yaml:"mounts,omitempty"` // Virtual path mounts
	MaxConnections       int           `yaml:"max_connections"`
	PassivePortMin       int           `yaml:"passive_port_min"`
	PassivePortMax       int           `yaml:"passive_port_max"`
	IdleTimeout          time.Duration `yaml:"idle_timeout"`
	PublicHost           string        `yaml:"public_host"`
	DisableActiveIPCheck bool          `yaml:"disable_active_ip_check"` // Disable IP matching for active mode (PORT/EPRT)
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
		Enabled:          false,
		Path:             "./cache/badger",
		MaxMemoryMB:      256,
		ValueLogMaxMB:    100,
		DefaultTTL:       5 * time.Minute,
		NodeTTL:          15 * time.Minute,
		StatsTTL:         1 * time.Hour,
		SearchTTL:        5 * time.Minute,
		TestAnalyticsTTL: 15 * time.Minute,
		AnalyticsTTL:     30 * time.Minute,
		LongAnalyticsTTL: 1 * time.Hour,
		HistoricalTTL:    24 * time.Hour,
		MaxSearchResults: 500,
		WarmupOnStart:    false,
		CompactOnClose:   true,
		ClearAllOnImport: false,
		GCInterval:       10 * time.Minute,
		GCDiscardRatio:   0.5,
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
		cfg := &Config{
			ClickHouse: DefaultClickHouseConfig(),
		}
		if err := cfg.validateNetworks(); err != nil {
			return nil, err
		}
		return cfg, nil
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
	// LoadConfig unmarshals onto a zero-value Config, so a cache section that
	// omits a TTL leaves it at 0 - and 0 means "never expires" in both backends
	// (memory.go, badger.go both gate expiry on ttl > 0), not "expires now".
	// Resolve each one: explicit value, then default_ttl, then its own default.
	// Per field, so a single default_ttl cannot flatten the four distinct ones.
	cacheDefaults := DefaultCacheConfig()
	for _, ttl := range []struct {
		field *time.Duration
		def   time.Duration
	}{
		{&c.Cache.NodeTTL, cacheDefaults.NodeTTL},
		{&c.Cache.StatsTTL, cacheDefaults.StatsTTL},
		{&c.Cache.SearchTTL, cacheDefaults.SearchTTL},
	} {
		if *ttl.field != 0 {
			continue
		}
		if c.Cache.DefaultTTL > 0 {
			*ttl.field = c.Cache.DefaultTTL
			continue
		}
		*ttl.field = ttl.def
	}
	if c.Cache.DefaultTTL == 0 {
		c.Cache.DefaultTTL = cacheDefaults.DefaultTTL
	}
	// The four analytics horizons deliberately do NOT fall back to
	// default_ttl. They are not a freshness preference; each one says what can
	// change that family of answers, and the widest is a day. Letting a
	// default_ttl of a few minutes flatten them would recompute a year of
	// nodelist history on a schedule set for search results.
	for _, ttl := range []struct {
		field *time.Duration
		def   time.Duration
	}{
		{&c.Cache.TestAnalyticsTTL, cacheDefaults.TestAnalyticsTTL},
		{&c.Cache.AnalyticsTTL, cacheDefaults.AnalyticsTTL},
		{&c.Cache.LongAnalyticsTTL, cacheDefaults.LongAnalyticsTTL},
		{&c.Cache.HistoricalTTL, cacheDefaults.HistoricalTTL},
	} {
		if *ttl.field == 0 {
			*ttl.field = ttl.def
		}
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
	// create a default mount at /nodelists. This was /fidonet/nodelists until
	// 2026-07-30; the /fidonet namespace was retired because it held exactly
	// one mount and had become misleading once other FTN networks landed
	// inside it (/fidonet/nodelists/fsxnet/). The FTP path now matches the
	// HTTP one. Deployments pinning the old path must set mounts explicitly.
	if len(c.FTP.Mounts) == 0 && c.FTP.NodelistPath != "" {
		c.FTP.Mounts = []FTPMount{
			{
				VirtualPath: "/nodelists",
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

	// Query budgets: parsed here so a typo fails at startup rather than being
	// silently dropped on the first slow page. The defaults only apply once
	// the feature is switched on.
	if c.QueryBudget.Enabled {
		if c.QueryBudget.Default == "" {
			c.QueryBudget.Default = "30s"
		}
		if c.QueryBudget.Analytics == "" {
			c.QueryBudget.Analytics = "120s"
		}
	}
	if _, _, err := c.QueryBudget.Durations(); err != nil {
		return err
	}

	// Rate limiting: the durations and CIDRs are parsed at startup for the
	// same reason as the query budgets - a typo must not quietly leave the
	// front end unprotected.
	if c.RateLimit.On() {
		if _, err := c.RateLimit.IdleDuration(); err != nil {
			return err
		}
		for _, cidr := range c.RateLimit.TrustedProxies {
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				if net.ParseIP(strings.TrimSpace(cidr)) == nil {
					return fmt.Errorf("invalid rate_limit.trusted_proxies entry %q: want a CIDR or IP", cidr)
				}
			}
		}
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

	// Validate networks configuration; inject the default fidonet entry when
	// the section is absent so single-network installs keep working unchanged
	if err := c.validateNetworks(); err != nil {
		return err
	}

	return nil
}

// DefaultNetworkConfigs returns the built-in network list used when the
// `networks:` section is absent from the configuration file.
func DefaultNetworkConfigs() []NetworkConfig {
	return []NetworkConfig{
		{Name: database.DefaultDomain, NodelistPattern: `(?i)^nodelist`},
	}
}

// validateNetworks checks the networks section and compiles filename patterns.
func (c *Config) validateNetworks() error {
	if len(c.Networks) == 0 {
		c.Networks = DefaultNetworkConfigs()
	}

	seen := make(map[string]bool)
	for i := range c.Networks {
		n := &c.Networks[i]
		if n.Name == "" {
			return fmt.Errorf("networks[%d]: name is required", i)
		}
		if n.Name != strings.ToLower(n.Name) {
			return fmt.Errorf("networks[%d]: name %q must be lowercase", i, n.Name)
		}
		if seen[n.Name] {
			return fmt.Errorf("networks[%d]: duplicate network name %q", i, n.Name)
		}
		seen[n.Name] = true

		if n.NodelistPattern == "" {
			// Default pattern: <name>.ddd (e.g. fsxnet.191); fidonet keeps its
			// traditional nodelist.* naming
			if n.Name == database.DefaultDomain {
				// Prefix match keeps historical fidonet behavior
				// (nodelist.216, nodelist_2024_001, ...)
				n.NodelistPattern = `(?i)^nodelist`
			} else {
				n.NodelistPattern = `(?i)^` + regexp.QuoteMeta(n.Name) + `\.\d{3}$`
			}
		}

		compiled, err := regexp.Compile(n.NodelistPattern)
		if err != nil {
			return fmt.Errorf("networks[%d] (%s): invalid nodelist_pattern: %w", i, n.Name, err)
		}
		n.compiledPattern = compiled
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

// ToCacheConfig converts CacheConfig to cache.Config.
func (c *CacheConfig) ToCacheConfig() *cache.Config {
	cacheType := c.Type
	if cacheType == "" {
		cacheType = "badger"
	}
	return &cache.Config{
		Enabled:              true,
		Type:                 cacheType,
		BadgerPath:           c.Path,
		BadgerMaxMemoryMB:    c.MaxMemoryMB,
		BadgerValueLogMaxMB:  c.ValueLogMaxMB,
		BadgerCompactL0:      c.CompactOnClose,
		BadgerNumGoroutines:  4,
		BadgerGCInterval:     c.GCInterval,
		BadgerGCDiscardRatio: c.GCDiscardRatio,
		BadgerMaxDiskMB:      c.MaxDiskMB,
	}
}

// ToCacheStorageConfig converts CacheConfig to storage.CacheStorageConfig.
func (c *CacheConfig) ToCacheStorageConfig() *storage.CacheStorageConfig {
	return &storage.CacheStorageConfig{
		Enabled:          true,
		NodeTTL:          c.NodeTTL,
		StatsTTL:         c.StatsTTL,
		SearchTTL:        c.SearchTTL,
		MaxSearchResults: c.MaxSearchResults,
		WarmupOnStart:    c.WarmupOnStart,
		TestAnalyticsTTL: c.TestAnalyticsTTL,
		AnalyticsTTL:     c.AnalyticsTTL,
		LongAnalyticsTTL: c.LongAnalyticsTTL,
		HistoricalTTL:    c.HistoricalTTL,
	}
}

// ToFTPConfig converts FTPConfig to ftp.Config.
func (c *FTPConfig) ToFTPConfig() *ftp.Config {
	mounts := make([]ftp.MountConfig, len(c.Mounts))
	for i, m := range c.Mounts {
		mounts[i] = ftp.MountConfig{VirtualPath: m.VirtualPath, RealPath: m.RealPath}
	}
	return &ftp.Config{
		Enabled:              c.Enabled,
		Host:                 c.Host,
		Port:                 c.Port,
		Mounts:               mounts,
		MaxConnections:       c.MaxConnections,
		PassivePortMin:       c.PassivePortMin,
		PassivePortMax:       c.PassivePortMax,
		IdleTimeout:          c.IdleTimeout,
		PublicHost:           c.PublicHost,
		DisableActiveIPCheck: c.DisableActiveIPCheck,
	}
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
		Protocol:     c.Protocol,
		MaxOpenConns: c.MaxOpenConns,
		MaxIdleConns: c.MaxIdleConns,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		Compression:  c.Compression,
	}, nil
}
