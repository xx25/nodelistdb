// Package main provides tests for modem-test configuration validation.
package main

import (
	"strings"
	"testing"
	"time"
)

// Test operator name validation in Config.Validate
func TestConfig_ValidateOperatorNames(t *testing.T) {
	tests := []struct {
		name      string
		operators []OperatorConfig
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "empty operators list is valid",
			operators: []OperatorConfig{},
			wantErr:   false,
		},
		{
			name: "single operator without name is valid",
			operators: []OperatorConfig{
				{Prefix: "01"},
			},
			wantErr: false,
		},
		{
			name: "single operator with name is valid",
			operators: []OperatorConfig{
				{Name: "Primary", Prefix: "01"},
			},
			wantErr: false,
		},
		{
			name: "multiple operators require names - first missing",
			operators: []OperatorConfig{
				{Prefix: "01"},
				{Name: "Secondary", Prefix: "02"},
			},
			wantErr:   true,
			errSubstr: "name is required",
		},
		{
			name: "multiple operators require names - second missing",
			operators: []OperatorConfig{
				{Name: "Primary", Prefix: "01"},
				{Prefix: "02"},
			},
			wantErr:   true,
			errSubstr: "name is required",
		},
		{
			name: "multiple operators with all names is valid",
			operators: []OperatorConfig{
				{Name: "Primary", Prefix: "01"},
				{Name: "Secondary", Prefix: "02"},
			},
			wantErr: false,
		},
		{
			name: "duplicate operator names rejected",
			operators: []OperatorConfig{
				{Name: "SameName", Prefix: "01"},
				{Name: "SameName", Prefix: "02"},
			},
			wantErr:   true,
			errSubstr: "duplicate",
		},
		{
			name: "three operators with unique names is valid",
			operators: []OperatorConfig{
				{Name: "A", Prefix: "01"},
				{Name: "B", Prefix: "02"},
				{Name: "C", Prefix: "03"},
			},
			wantErr: false,
		},
		{
			name: "three operators with one duplicate is rejected",
			operators: []OperatorConfig{
				{Name: "A", Prefix: "01"},
				{Name: "B", Prefix: "02"},
				{Name: "A", Prefix: "03"},
			},
			wantErr:   true,
			errSubstr: "duplicate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Test.Operators = tt.operators

			err := cfg.Validate()

			if tt.wantErr {
				if err == nil {
					t.Error("Validate() error = nil, want error")
					return
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Validate() error = %q, want substring %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// Test OperatorCacheConfig defaults
func TestConfig_OperatorCacheDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// Verify default TTL is 360 days (8640 hours)
	expectedTTL := Duration(360 * 24 * time.Hour)
	if cfg.Test.OperatorCache.TTL != expectedTTL {
		t.Errorf("OperatorCache.TTL = %v, want %v", cfg.Test.OperatorCache.TTL, expectedTTL)
	}

	// Verify default path is empty (uses ~/.modem-test/operator_cache)
	if cfg.Test.OperatorCache.Path != "" {
		t.Errorf("OperatorCache.Path = %q, want empty", cfg.Test.OperatorCache.Path)
	}

	// Verify Enabled defaults to true
	if !cfg.Test.OperatorCache.Enabled {
		t.Error("OperatorCache.Enabled = false, want true")
	}
}

// Test GetOperators returns configured operators
func TestConfig_GetOperators(t *testing.T) {
	tests := []struct {
		name      string
		operators []OperatorConfig
		wantLen   int
	}{
		{
			name:      "returns empty slice when not configured",
			operators: nil,
			wantLen:   0,
		},
		{
			name:      "returns empty slice for empty config",
			operators: []OperatorConfig{},
			wantLen:   0,
		},
		{
			name: "returns single operator",
			operators: []OperatorConfig{
				{Name: "Primary", Prefix: "01"},
			},
			wantLen: 1,
		},
		{
			name: "returns multiple operators",
			operators: []OperatorConfig{
				{Name: "A", Prefix: "01"},
				{Name: "B", Prefix: "02"},
				{Name: "C", Prefix: "03"},
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Test.Operators = tt.operators

			got := cfg.GetOperators()

			if len(got) != tt.wantLen {
				t.Errorf("GetOperators() len = %d, want %d", len(got), tt.wantLen)
			}

			// Verify the returned slice matches what was set
			if tt.operators != nil {
				for i, op := range got {
					if op != tt.operators[i] {
						t.Errorf("GetOperators()[%d] = %v, want %v", i, op, tt.operators[i])
					}
				}
			}
		})
	}
}

// Test GetPause returns configured or default value
func TestConfig_GetPause(t *testing.T) {
	tests := []struct {
		name     string
		pause    Duration
		expected time.Duration
	}{
		{
			name:     "returns default 60s when not configured",
			pause:    Duration(0),
			expected: 60 * time.Second,
		},
		{
			name:     "returns configured value",
			pause:    Duration(30 * time.Second),
			expected: 30 * time.Second,
		},
		{
			name:     "returns longer configured value",
			pause:    Duration(5 * time.Minute),
			expected: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Test.Pause = tt.pause

			got := cfg.GetPause()

			if got != tt.expected {
				t.Errorf("GetPause() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Test parsePhoneList function
func TestParsePhoneList(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{
			name:   "single phone",
			input:  "917",
			expect: []string{"917"},
		},
		{
			name:   "comma-separated phones",
			input:  "917,918,919",
			expect: []string{"917", "918", "919"},
		},
		{
			name:   "phone range",
			input:  "901-905",
			expect: []string{"901", "902", "903", "904", "905"},
		},
		{
			name:   "mixed comma and range",
			input:  "901-903,917,920-922",
			expect: []string{"901", "902", "903", "917", "920", "921", "922"},
		},
		{
			name:   "handles whitespace",
			input:  " 917 , 918 , 919 ",
			expect: []string{"917", "918", "919"},
		},
		{
			name:   "empty string returns empty",
			input:  "",
			expect: nil,
		},
		{
			name:   "reversed range gets swapped",
			input:  "905-901",
			expect: []string{"901", "902", "903", "904", "905"},
		},
		{
			name:   "single number range",
			input:  "917-917",
			expect: []string{"917"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePhoneList(tt.input)

			if len(got) != len(tt.expect) {
				t.Errorf("parsePhoneList(%q) len = %d, want %d\ngot: %v", tt.input, len(got), len(tt.expect), got)
				return
			}

			for i := range got {
				if got[i] != tt.expect[i] {
					t.Errorf("parsePhoneList(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expect[i])
				}
			}
		})
	}
}

// Test GetPhones integrates phone list parsing
func TestConfig_GetPhones(t *testing.T) {
	tests := []struct {
		name     string
		phone    string   // Single phone field
		phones   []string // Phones list field
		expected []string
	}{
		{
			name:     "returns nil when both empty",
			phone:    "",
			phones:   nil,
			expected: nil,
		},
		{
			name:     "returns single phone",
			phone:    "917",
			phones:   nil,
			expected: []string{"917"},
		},
		{
			name:     "returns phones list",
			phone:    "",
			phones:   []string{"917", "918"},
			expected: []string{"917", "918"},
		},
		{
			name:     "phones list takes precedence",
			phone:    "999",
			phones:   []string{"917", "918"},
			expected: []string{"917", "918"},
		},
		{
			name:     "expands range in phones list",
			phone:    "",
			phones:   []string{"901-903"},
			expected: []string{"901", "902", "903"},
		},
		{
			name:     "expands range in single phone",
			phone:    "901-903",
			phones:   nil,
			expected: []string{"901", "902", "903"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Test.Phone = tt.phone
			cfg.Test.Phones = tt.phones

			got := cfg.GetPhones()

			if len(got) != len(tt.expected) {
				t.Errorf("GetPhones() len = %d, want %d\ngot: %v", len(got), len(tt.expected), got)
				return
			}

			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("GetPhones()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// Test modem validation
func TestConfig_ValidateModem(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*Config)
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "default config is valid",
			setup:   func(c *Config) {},
			wantErr: false,
		},
		{
			name: "empty device is invalid",
			setup: func(c *Config) {
				c.Modem.Device = ""
			},
			wantErr:   true,
			errSubstr: "device",
		},
		{
			name: "zero baud rate is invalid",
			setup: func(c *Config) {
				c.Modem.BaudRate = 0
			},
			wantErr:   true,
			errSubstr: "baud_rate",
		},
		{
			name: "negative baud rate is invalid",
			setup: func(c *Config) {
				c.Modem.BaudRate = -9600
			},
			wantErr:   true,
			errSubstr: "baud_rate",
		},
		{
			name: "invalid hangup method is rejected",
			setup: func(c *Config) {
				c.Modem.HangupMethod = "invalid"
			},
			wantErr:   true,
			errSubstr: "hangup_method",
		},
		{
			name: "dtr hangup method is valid",
			setup: func(c *Config) {
				c.Modem.HangupMethod = "dtr"
			},
			wantErr: false,
		},
		{
			name: "escape hangup method is valid",
			setup: func(c *Config) {
				c.Modem.HangupMethod = "escape"
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.setup(cfg)

			err := cfg.Validate()

			if tt.wantErr {
				if err == nil {
					t.Error("Validate() error = nil, want error")
					return
				}
				if tt.errSubstr != "" && !strings.Contains(strings.ToLower(err.Error()), tt.errSubstr) {
					t.Errorf("Validate() error = %q, want substring %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// Test prefix mode validation
func TestConfig_ValidatePrefixMode(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		apiURL    string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "no prefix mode is valid",
			prefix:  "",
			apiURL:  "",
			wantErr: false,
		},
		{
			name:    "prefix with API URL is valid",
			prefix:  "+7",
			apiURL:  "http://localhost:8080",
			wantErr: false,
		},
		{
			name:      "prefix without API URL is invalid",
			prefix:    "+7",
			apiURL:    "",
			wantErr:   true,
			errSubstr: "nodelistdb.url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Test.Prefix = tt.prefix
			cfg.NodelistDB.URL = tt.apiURL

			err := cfg.Validate()

			if tt.wantErr {
				if err == nil {
					t.Error("Validate() error = nil, want error")
					return
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Validate() error = %q, want substring %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// Test Duration YAML unmarshaling
func TestDuration_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "parses seconds",
			input:    "30s",
			expected: 30 * time.Second,
			wantErr:  false,
		},
		{
			name:     "parses minutes",
			input:    "5m",
			expected: 5 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "parses hours",
			input:    "2h",
			expected: 2 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "parses combined",
			input:    "1h30m",
			expected: 90 * time.Minute,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := time.ParseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("ParseDuration() error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseDuration() error = %v", err)
				return
			}

			if d != tt.expected {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, d, tt.expected)
			}
		})
	}
}

// Test ModemInstanceConfig.IsEnabled
func TestModemInstanceConfig_IsEnabled(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{
			name:    "nil means enabled",
			enabled: nil,
			want:    true,
		},
		{
			name:    "true is enabled",
			enabled: &trueVal,
			want:    true,
		},
		{
			name:    "false is disabled",
			enabled: &falseVal,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := ModemInstanceConfig{
				Enabled: tt.enabled,
			}

			if got := m.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test Config.IsMultiModem
func TestConfig_IsMultiModem(t *testing.T) {
	tests := []struct {
		name   string
		modems []ModemInstanceConfig
		want   bool
	}{
		{
			name:   "empty modems is not multi-modem",
			modems: nil,
			want:   false,
		},
		{
			name:   "nil modems is not multi-modem",
			modems: []ModemInstanceConfig{},
			want:   false,
		},
		{
			name: "one modem is multi-modem",
			modems: []ModemInstanceConfig{
				{ModemConfig: ModemConfig{Device: "/dev/ttyACM0"}},
			},
			want: true,
		},
		{
			name: "two modems is multi-modem",
			modems: []ModemInstanceConfig{
				{ModemConfig: ModemConfig{Device: "/dev/ttyACM0"}},
				{ModemConfig: ModemConfig{Device: "/dev/ttyACM1"}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Modems = tt.modems

			if got := cfg.IsMultiModem(); got != tt.want {
				t.Errorf("IsMultiModem() = %v, want %v", got, tt.want)
			}
		})
	}
}
