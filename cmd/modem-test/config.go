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
	Modem   ModemConfig   `yaml:"modem"`
	Test    TestConfig    `yaml:"test"`
	EMSI    EMSIConfig    `yaml:"emsi"`
	Logging LoggingConfig `yaml:"logging"`
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

	// Init commands - executed in order during modem initialization
	InitCommands []string `yaml:"init_commands"`

	// Post-disconnect commands - executed after hangup to get line stats
	PostDisconnectCommands []string `yaml:"post_disconnect_commands"`

	// Delay before post-disconnect commands (modem needs time to compute stats)
	PostDisconnectDelay Duration `yaml:"post_disconnect_delay"`

	// Stats profile for parsing line statistics: "rockwell", "usr", "zyxel", "raw" (default)
	StatsProfile string `yaml:"stats_profile"`
}

// TestConfig contains test execution parameters
type TestConfig struct {
	Count      int      `yaml:"count"`
	InterDelay Duration `yaml:"inter_delay"`
	Phone      string   `yaml:"phone"`   // Single phone (for backward compatibility)
	Phones     []string `yaml:"phones"`  // Multiple phones (called in circular order)
	CSVFile    string   `yaml:"csv_file"` // Path to CSV output file (optional)
}

// EMSIConfig contains EMSI handshake parameters
type EMSIConfig struct {
	OurAddress string   `yaml:"our_address"`
	SystemName string   `yaml:"system_name"`
	Sysop      string   `yaml:"sysop"`
	Location   string   `yaml:"location"`
	Timeout    Duration `yaml:"timeout"`
	Protocols  []string `yaml:"protocols"` // Empty = NCP (no file transfer)
}

// LoggingConfig controls output verbosity and format
type LoggingConfig struct {
	Debug      bool `yaml:"debug"`
	Timestamps bool `yaml:"timestamps"`
	ShowRS232  bool `yaml:"show_rs232"`
	ShowHex    bool `yaml:"show_hex"` // Show hex dump of data
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
	if c.Modem.Device == "" {
		return fmt.Errorf("modem.device is required")
	}
	if c.Modem.BaudRate <= 0 {
		return fmt.Errorf("modem.baud_rate must be positive")
	}
	if c.Test.Count <= 0 {
		return fmt.Errorf("test.count must be positive")
	}
	if c.Modem.HangupMethod != "dtr" && c.Modem.HangupMethod != "escape" {
		return fmt.Errorf("modem.hangup_method must be 'dtr' or 'escape'")
	}
	return nil
}

// ApplyCLIOverrides applies command-line flag values to the config
// count: -1 means "not specified" (use config default), 0 means infinite, >0 is the count
func (c *Config) ApplyCLIOverrides(device, phone string, count int, debug bool, csvFile string) {
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
