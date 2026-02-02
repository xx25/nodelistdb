// Package main provides the modem-test CLI tool configuration.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete configuration for modem testing
type Config struct {
	Modem           ModemConfig           `yaml:"modem"`            // Single modem (backward compat)
	Modems          []ModemInstanceConfig `yaml:"modems"`           // Multi-modem array
	ModemDefaults   ModemConfig           `yaml:"modem_defaults"`   // Shared defaults for multi-modem
	Test            TestConfig            `yaml:"test"`
	EMSI            EMSIConfig            `yaml:"emsi"`
	Logging         LoggingConfig         `yaml:"logging"`
	CDR             CDRConfig             `yaml:"cdr"`              // AudioCodes CDR database (optional)
	AsteriskCDR     AsteriskCDRConfig     `yaml:"asterisk_cdr"`     // Asterisk CDR database (optional)
	PostgresResults PostgresResultsConfig `yaml:"postgres_results"` // PostgreSQL results storage (optional)
	MySQLResults    MySQLResultsConfig    `yaml:"mysql_results"`    // MySQL results storage (optional)
	NodelistDB      NodelistDBConfig      `yaml:"nodelistdb"`       // NodelistDB API server (for -prefix mode)
}

// NodelistDBConfig contains API connection settings for fetching PSTN nodes.
type NodelistDBConfig struct {
	URL string `yaml:"url"` // API server URL, e.g., "http://localhost:8080"
}

// ModemInstanceConfig extends ModemConfig with instance-specific fields
type ModemInstanceConfig struct {
	ModemConfig `yaml:",inline"` // Embed all modem config fields
	Name        string           `yaml:"name"`    // Friendly name (e.g., "modem1", "USR Courier")
	Enabled     *bool            `yaml:"enabled"` // Allow disabling individual modems (nil = true)
}

// IsEnabled returns true if this modem instance is enabled
func (m *ModemInstanceConfig) IsEnabled() bool {
	return m.Enabled == nil || *m.Enabled
}

// ModemConfig contains modem hardware and timing settings
type ModemConfig struct {
	Device       string `yaml:"device"`
	BaudRate     int    `yaml:"baud_rate"`
	DialPrefix   string `yaml:"dial_prefix"`
	HangupMethod string `yaml:"hangup_method"` // "dtr" or "escape"

	// Timeouts
	DialTimeout      Duration `yaml:"dial_timeout"`
	CarrierTimeout   Duration `yaml:"carrier_timeout"`
	ATCommandTimeout Duration `yaml:"at_command_timeout"`
	ReadTimeout      Duration `yaml:"read_timeout"`

	// DTR Hangup timing (only used when hangup_method is "dtr")
	DTRHoldTime      Duration `yaml:"dtr_hold_time"`      // How long to hold DTR low initially (default 500ms)
	DTRWaitInterval  Duration `yaml:"dtr_wait_interval"`  // Interval between DCD checks (default 150ms)
	DTRMaxWaitTime   Duration `yaml:"dtr_max_wait_time"`  // Max time to wait for DCD drop (default 1500ms)
	DTRStabilizeTime Duration `yaml:"dtr_stabilize_time"` // Time to wait after raising DTR (default 200ms)

	// Init commands - executed in order during modem initialization
	InitCommands []string `yaml:"init_commands"`

	// Post-disconnect commands - executed after hangup to get line stats
	PostDisconnectCommands []string `yaml:"post_disconnect_commands"`

	// Delay before post-disconnect commands (modem needs time to compute stats)
	PostDisconnectDelay Duration `yaml:"post_disconnect_delay"`

	// Stats profile for parsing line statistics: "rockwell", "usr", "zyxel", "raw" (default)
	StatsProfile string `yaml:"stats_profile"`

	// StatsPagination enables handling of paginated stats output (e.g., MT5634ZBA with ATI11)
	// When true, sends space to continue when "Press any key" prompt is detected
	StatsPagination bool `yaml:"stats_pagination"`

	// BusyRetryCount is the number of times to retry dialing when BUSY is received.
	// Set to 0 to disable retries. Default is 0 (no retries).
	// When enabled, the modem will keep retrying the same number with the same operator
	// until a non-BUSY result is received or the retry count is exhausted.
	BusyRetryCount int `yaml:"busy_retry_count"`

	// BusyRetryDelay is the delay between retry attempts on BUSY (default: 5s)
	BusyRetryDelay Duration `yaml:"busy_retry_delay"`
}

// TestConfig contains test execution parameters
type TestConfig struct {
	Count            int              `yaml:"count"`
	CallsPerOperator int              `yaml:"calls_per_operator"` // Calls per operator per phone (overrides Count if set)
	InterDelay       Duration         `yaml:"inter_delay"`
	Phone            string           `yaml:"phone"`     // Single phone (for backward compatibility)
	Phones           []string         `yaml:"phones"`    // Multiple phones (called in circular order)
	Operators        []OperatorConfig `yaml:"operators"` // Operator prefixes for routing comparison (optional)
	CSVFile          string           `yaml:"csv_file"`  // Path to CSV output file (optional)
	Prefix           string           `yaml:"prefix"`    // Phone prefix to fetch PSTN nodes from API (e.g., "+7")

	// Retry settings for failed calls (CDR-based: NO ANSWER, BUSY, FAILED, CONGESTION with billsec=0)
	RetryCount int      `yaml:"retry_count"` // Number of retries (0 = disabled, default: 20)
	RetryDelay Duration `yaml:"retry_delay"` // Delay between retries (default: 5s)
}

// OperatorConfig contains operator/carrier routing configuration
type OperatorConfig struct {
	Name   string `yaml:"name"`   // Friendly name for reports (e.g., "Verizon", "VoIP-A")
	Prefix string `yaml:"prefix"` // Dial prefix to prepend (e.g., "1#", "2#", "" for direct)
}

// EMSIConfig contains EMSI handshake parameters
type EMSIConfig struct {
	OurAddress      string   `yaml:"our_address"`
	SystemName      string   `yaml:"system_name"`
	Sysop           string   `yaml:"sysop"`
	Location        string   `yaml:"location"`
	Timeout         Duration `yaml:"timeout"`
	Protocols       []string `yaml:"protocols"`        // Empty = NCP (no file transfer)
	InitialStrategy string   `yaml:"initial_strategy"` // "wait" (default), "send_inq", "send_cr"
}

// LoggingConfig controls output verbosity and format
type LoggingConfig struct {
	Debug      bool `yaml:"debug"`
	Timestamps bool `yaml:"timestamps"`
	ShowRS232  bool `yaml:"show_rs232"`
	ShowHex    bool `yaml:"show_hex"` // Show hex dump of data
}

// CDRConfig contains AudioCodes CDR database settings for VoIP quality metrics
type CDRConfig struct {
	Enabled       bool   `yaml:"enabled"`         // Enable CDR lookup (default: false)
	Driver        string `yaml:"driver"`          // Database driver: "postgres" or "mysql" (default: "postgres")
	DSN           string `yaml:"dsn"`             // Database connection string
	TableName     string `yaml:"table_name"`      // CDR table name (default: "cdr")
	TimeWindowSec int    `yaml:"time_window_sec"` // Time window for matching calls (default: 120)
}

// AsteriskCDRConfig contains Asterisk CDR database settings for call routing info
type AsteriskCDRConfig struct {
	Enabled        bool     `yaml:"enabled"`          // Enable Asterisk CDR lookup (default: false)
	Driver         string   `yaml:"driver"`           // Database driver: "postgres" or "mysql" (default: "postgres")
	DSN            string   `yaml:"dsn"`              // Database connection string
	TableName      string   `yaml:"table_name"`       // CDR table name (default: "cdr")
	TimeWindowSec  int      `yaml:"time_window_sec"`  // Time window for matching calls (default: 120)
	CDRLookupDelay Duration `yaml:"cdr_lookup_delay"` // Delay before CDR lookup for retry decision (default: 2s)
}

// Duration wraps time.Duration for YAML unmarshaling
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler for Duration
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(dur)
	return nil
}

// MarshalYAML implements yaml.Marshaler for Duration
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// Duration returns the time.Duration value
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Modem: ModemConfig{
			Device:           "/dev/ttyACM0",
			BaudRate:         115200,
			DialPrefix:       "ATDT",
			HangupMethod:     "dtr",
			DialTimeout:      Duration(200 * time.Second),
			CarrierTimeout:   Duration(5 * time.Second),
			ATCommandTimeout: Duration(5 * time.Second),
			ReadTimeout:      Duration(1 * time.Second),
			InitCommands: []string{
				"ATZ",    // Reset modem
				"ATE0",   // Disable echo
				"ATV1",   // Verbose result codes
				"ATX4",   // Extended result codes
				"ATS0=0", // Disable auto-answer
			},
			PostDisconnectCommands: []string{
				"AT&V1", // Line quality stats (USR modems)
			},
			PostDisconnectDelay: Duration(2 * time.Second), // Wait for modem to compute stats
			StatsProfile:        "rockwell",                // Default parser profile
		},
		Test: TestConfig{
			Count:      10,
			InterDelay: Duration(5 * time.Second),
			RetryCount: 20,
			RetryDelay: Duration(5 * time.Second),
		},
		EMSI: EMSIConfig{
			OurAddress: "2:5001/5001",
			SystemName: "NodelistDB Tester",
			Sysop:      "Test",
			Location:   "Test",
			Timeout:    Duration(30 * time.Second),
			Protocols:  []string{}, // NCP - no file transfer
		},
		Logging: LoggingConfig{
			Debug:      true,
			Timestamps: true,
			ShowRS232:  true,
			ShowHex:    false,
		},
		CDR: CDRConfig{
			Enabled:       false,
			Driver:        "postgres",
			TableName:     "cdr",
			TimeWindowSec: 120,
		},
		AsteriskCDR: AsteriskCDRConfig{
			Enabled:       true, // Required for CDR-based retry logic
			Driver:        "postgres",
			TableName:     "cdr",
			TimeWindowSec: 120,
		},
		PostgresResults: PostgresResultsConfig{
			Enabled:   false,
			TableName: "modem_test_results",
		},
		MySQLResults: MySQLResultsConfig{
			Enabled:   false,
			TableName: "modem_test_results",
		},
	}
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks configuration for errors
func (c *Config) Validate() error {
	// Prefix mode validation
	if c.Test.Prefix != "" && c.NodelistDB.URL == "" {
		return fmt.Errorf("nodelistdb.url is required when test.prefix is set")
	}

	// Multi-modem mode validation
	if c.IsMultiModem() {
		return c.validateMultiModem()
	}

	// Single modem validation
	if c.Modem.Device == "" {
		return fmt.Errorf("modem.device is required")
	}
	if c.Modem.BaudRate <= 0 {
		return fmt.Errorf("modem.baud_rate must be positive")
	}
	// count and calls_per_operator are mutually exclusive
	if c.Test.Count > 0 && c.Test.CallsPerOperator > 0 {
		return fmt.Errorf("test.count and test.calls_per_operator are mutually exclusive; set only one")
	}
	// Require either count, calls_per_operator, or prefix to be set
	if c.Test.Count <= 0 && c.Test.CallsPerOperator <= 0 && c.Test.Prefix == "" {
		return fmt.Errorf("either test.count or test.calls_per_operator must be positive")
	}
	if c.Modem.HangupMethod != "dtr" && c.Modem.HangupMethod != "escape" {
		return fmt.Errorf("modem.hangup_method must be 'dtr' or 'escape'")
	}
	// Asterisk CDR is required for CDR-based retry logic (skip in prefix mode)
	if c.AsteriskCDR.DSN == "" && c.Test.Prefix == "" {
		return fmt.Errorf("asterisk_cdr.dsn is required for CDR-based retry logic")
	}
	return nil
}

// validateMultiModem validates multi-modem configuration
func (c *Config) validateMultiModem() error {
	enabledCount := 0
	devices := make(map[string]bool)
	names := make(map[string]bool)

	for i, m := range c.Modems {
		if !m.IsEnabled() {
			continue
		}
		enabledCount++

		// Merge with defaults to validate the final config
		merged := c.mergeModemConfig(c.ModemDefaults, m.ModemConfig)

		// Check device is specified (required, no default)
		if merged.Device == "" {
			return fmt.Errorf("modems[%d].device is required", i)
		}

		// Check for duplicate devices
		if devices[merged.Device] {
			return fmt.Errorf("duplicate modem device: %s", merged.Device)
		}
		devices[merged.Device] = true

		// Check for duplicate names (if specified)
		if m.Name != "" {
			if names[m.Name] {
				return fmt.Errorf("duplicate modem name: %s", m.Name)
			}
			names[m.Name] = true
		}

		// Validate required fields after merge
		if merged.BaudRate <= 0 {
			return fmt.Errorf("modems[%d]: baud_rate must be positive (set in modem_defaults or per-modem)", i)
		}

		// Validate hangup method (use default if not set)
		hangup := merged.HangupMethod
		if hangup == "" {
			hangup = "dtr" // Default
		}
		if hangup != "dtr" && hangup != "escape" {
			return fmt.Errorf("modems[%d].hangup_method must be 'dtr' or 'escape'", i)
		}
	}

	if enabledCount == 0 {
		return fmt.Errorf("at least one modem must be enabled")
	}

	// count and calls_per_operator are mutually exclusive
	if c.Test.Count > 0 && c.Test.CallsPerOperator > 0 {
		return fmt.Errorf("test.count and test.calls_per_operator are mutually exclusive; set only one")
	}
	// Require either count, calls_per_operator, or prefix to be set
	if c.Test.Count <= 0 && c.Test.CallsPerOperator <= 0 && c.Test.Prefix == "" {
		return fmt.Errorf("either test.count or test.calls_per_operator must be positive")
	}

	// Asterisk CDR is required for CDR-based retry logic (skip in prefix mode)
	if c.AsteriskCDR.DSN == "" && c.Test.Prefix == "" {
		return fmt.Errorf("asterisk_cdr.dsn is required for CDR-based retry logic")
	}

	return nil
}

// IsMultiModem returns true if multi-modem mode is configured
func (c *Config) IsMultiModem() bool {
	return len(c.Modems) > 0
}

// GetModemConfigs returns the list of modem configurations to use.
// For multi-modem mode, merges defaults with per-modem settings.
// For single modem mode, returns single config wrapped in slice.
func (c *Config) GetModemConfigs() []ModemInstanceConfig {
	if !c.IsMultiModem() {
		// Single modem mode - wrap in ModemInstanceConfig
		return []ModemInstanceConfig{{
			ModemConfig: c.Modem,
			Name:        "modem",
			Enabled:     nil, // nil = enabled
		}}
	}

	// Collect all user-provided names first to avoid collisions
	usedNames := make(map[string]bool)
	for _, m := range c.Modems {
		if m.IsEnabled() && m.Name != "" {
			usedNames[m.Name] = true
		}
	}

	// Multi-modem mode - merge defaults with per-modem settings
	result := make([]ModemInstanceConfig, 0, len(c.Modems))
	autoIndex := 1
	for _, m := range c.Modems {
		if !m.IsEnabled() {
			continue
		}

		// Start with defaults, overlay per-modem settings
		merged := c.mergeModemConfig(c.ModemDefaults, m.ModemConfig)

		name := m.Name
		if name == "" {
			// Generate unique name that doesn't collide with user-provided names
			for {
				name = fmt.Sprintf("modem%d", autoIndex)
				autoIndex++
				if !usedNames[name] {
					usedNames[name] = true
					break
				}
			}
		}

		result = append(result, ModemInstanceConfig{
			ModemConfig: merged,
			Name:        name,
			Enabled:     m.Enabled,
		})
	}

	return result
}

// mergeModemConfig merges defaults with overrides, non-zero values in override take precedence
func (c *Config) mergeModemConfig(defaults, override ModemConfig) ModemConfig {
	result := defaults

	if override.Device != "" {
		result.Device = override.Device
	}
	if override.BaudRate != 0 {
		result.BaudRate = override.BaudRate
	}
	if override.DialPrefix != "" {
		result.DialPrefix = override.DialPrefix
	}
	if override.HangupMethod != "" {
		result.HangupMethod = override.HangupMethod
	}
	if override.DialTimeout != 0 {
		result.DialTimeout = override.DialTimeout
	}
	if override.CarrierTimeout != 0 {
		result.CarrierTimeout = override.CarrierTimeout
	}
	if override.ATCommandTimeout != 0 {
		result.ATCommandTimeout = override.ATCommandTimeout
	}
	if override.ReadTimeout != 0 {
		result.ReadTimeout = override.ReadTimeout
	}
	// DTR hangup timing overrides
	if override.DTRHoldTime != 0 {
		result.DTRHoldTime = override.DTRHoldTime
	}
	if override.DTRWaitInterval != 0 {
		result.DTRWaitInterval = override.DTRWaitInterval
	}
	if override.DTRMaxWaitTime != 0 {
		result.DTRMaxWaitTime = override.DTRMaxWaitTime
	}
	if override.DTRStabilizeTime != 0 {
		result.DTRStabilizeTime = override.DTRStabilizeTime
	}
	if len(override.InitCommands) > 0 {
		result.InitCommands = override.InitCommands
	}
	if len(override.PostDisconnectCommands) > 0 {
		result.PostDisconnectCommands = override.PostDisconnectCommands
	}
	if override.PostDisconnectDelay != 0 {
		result.PostDisconnectDelay = override.PostDisconnectDelay
	}
	if override.StatsProfile != "" {
		result.StatsProfile = override.StatsProfile
	}
	if override.StatsPagination {
		result.StatsPagination = override.StatsPagination
	}
	if override.BusyRetryCount != 0 {
		result.BusyRetryCount = override.BusyRetryCount
	}
	if override.BusyRetryDelay != 0 {
		result.BusyRetryDelay = override.BusyRetryDelay
	}

	return result
}

// ApplyCLIOverrides applies command-line flag values to the config.
// count and perOperator use -1 as sentinel for "not specified" (use config default).
// They are mutually exclusive (validated before this call).
func (c *Config) ApplyCLIOverrides(device, phone string, count, perOperator int, debug bool, csvFile string) {
	if device != "" {
		c.Modem.Device = device
	}
	if phone != "" {
		// CLI phone overrides both Phone and Phones from config
		phones := parsePhoneList(phone)
		c.Test.Phones = phones
		c.Test.Phone = "" // Clear single phone - use Phones list
	}
	if count >= 0 {
		c.Test.Count = count // 0 = infinite
		c.Test.CallsPerOperator = 0
	}
	if perOperator >= 0 {
		c.Test.CallsPerOperator = perOperator
		c.Test.Count = 0
	}
	if csvFile != "" {
		c.Test.CSVFile = csvFile
	}
	if debug {
		c.Logging.Debug = true
	}
}

// GetPhones returns the list of phone numbers to dial.
// Returns phones from Phones list if set, otherwise single Phone as a slice.
// Applies range expansion (e.g., "901-905") to all phone values.
func (c *Config) GetPhones() []string {
	if len(c.Test.Phones) > 0 {
		// Expand any ranges in the phones list
		var result []string
		for _, p := range c.Test.Phones {
			expanded := parsePhoneList(p)
			result = append(result, expanded...)
		}
		return result
	}
	if c.Test.Phone != "" {
		// Single phone may also contain ranges or comma-separated values
		return parsePhoneList(c.Test.Phone)
	}
	return nil
}

// GetOperators returns the list of operator configurations for routing comparison.
// Returns empty slice if no operators are configured (no prefix rotation).
func (c *Config) GetOperators() []OperatorConfig {
	return c.Test.Operators
}

// GetTotalTestCount returns the total number of tests to run based on configuration.
// If CallsPerOperator is set, returns calls_per_operator * phones * operators.
// Otherwise returns Count for legacy mode.
// Returns -1 for infinite mode (count <= 0 and calls_per_operator not set).
func (c *Config) GetTotalTestCount() int {
	phones := c.GetPhones()
	operators := c.GetOperators()

	// If using calls_per_operator mode
	if c.Test.CallsPerOperator > 0 {
		numOperators := len(operators)
		if numOperators == 0 {
			numOperators = 1 // At least one "no operator" entry
		}
		return c.Test.CallsPerOperator * len(phones) * numOperators
	}

	// Legacy mode: return count as-is (0 or negative means infinite)
	return c.Test.Count
}

// IsPerOperatorMode returns true if calls_per_operator mode is enabled
func (c *Config) IsPerOperatorMode() bool {
	return c.Test.CallsPerOperator > 0
}

// GetCDRLookupDelay returns the configured delay before CDR lookup for retry decisions.
// Returns default of 2 seconds if not configured.
func (c *Config) GetCDRLookupDelay() time.Duration {
	if c.AsteriskCDR.CDRLookupDelay.Duration() > 0 {
		return c.AsteriskCDR.CDRLookupDelay.Duration()
	}
	return 2 * time.Second
}

// GetRetryCount returns the number of retries for failed calls.
// Checks test.retry_count first, falls back to modem.busy_retry_count for backward compatibility.
func (c *Config) GetRetryCount() int {
	if c.Test.RetryCount > 0 {
		return c.Test.RetryCount
	}
	// Backward compatibility: check modem config
	if c.Modem.BusyRetryCount > 0 {
		return c.Modem.BusyRetryCount
	}
	return 20 // Default
}

// GetRetryDelay returns the delay between retry attempts.
// Checks test.retry_delay first, falls back to modem.busy_retry_delay for backward compatibility.
func (c *Config) GetRetryDelay() time.Duration {
	if c.Test.RetryDelay.Duration() > 0 {
		return c.Test.RetryDelay.Duration()
	}
	// Backward compatibility: check modem config
	if c.Modem.BusyRetryDelay.Duration() > 0 {
		return c.Modem.BusyRetryDelay.Duration()
	}
	return 5 * time.Second // Default
}

// parsePhoneList splits a comma-separated phone list into individual numbers.
// Supports ranges like "901-917" which expands to all numbers from 901 to 917.
// Examples:
//   - "917" -> ["917"]
//   - "917,918,919" -> ["917", "918", "919"]
//   - "901-905" -> ["901", "902", "903", "904", "905"]
//   - "901-903,917,920-922" -> ["901", "902", "903", "917", "920", "921", "922"]
func parsePhoneList(phones string) []string {
	var result []string
	for _, p := range strings.Split(phones, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// Check for range (e.g., "901-917")
		if strings.Contains(p, "-") {
			expanded := expandPhoneRange(p)
			result = append(result, expanded...)
		} else {
			result = append(result, p)
		}
	}
	return result
}

// expandPhoneRange expands a phone range like "901-917" into individual numbers.
// Returns the original string as single element if parsing fails.
func expandPhoneRange(rangeStr string) []string {
	parts := strings.SplitN(rangeStr, "-", 2)
	if len(parts) != 2 {
		return []string{rangeStr}
	}

	startStr := strings.TrimSpace(parts[0])
	endStr := strings.TrimSpace(parts[1])

	start, err1 := strconv.Atoi(startStr)
	end, err2 := strconv.Atoi(endStr)

	if err1 != nil || err2 != nil {
		// Not numeric, return as-is
		return []string{rangeStr}
	}

	if start > end {
		// Swap if reversed
		start, end = end, start
	}

	// Limit range to prevent accidental huge expansions
	const maxRange = 100
	if end-start+1 > maxRange {
		fmt.Fprintf(os.Stderr, "WARNING: Phone range %s exceeds %d numbers, truncating\n", rangeStr, maxRange)
		end = start + maxRange - 1
	}

	var result []string
	for i := start; i <= end; i++ {
		result = append(result, strconv.Itoa(i))
	}
	return result
}
