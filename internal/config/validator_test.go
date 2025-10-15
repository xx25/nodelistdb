package config

import (
	"strings"
	"testing"
	"time"
)

func TestClickHouseValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  ClickHouseConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ClickHouseConfig{
				Host:         "localhost",
				Port:         9000,
				Database:     "test",
				MaxOpenConns: 10,
				MaxIdleConns: 5,
			},
			wantErr: false,
		},
		{
			name: "missing host",
			config: ClickHouseConfig{
				Port:     9000,
				Database: "test",
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			config: ClickHouseConfig{
				Host:     "localhost",
				Port:     99999,
				Database: "test",
			},
			wantErr: true,
		},
		{
			name: "missing database",
			config: ClickHouseConfig{
				Host: "localhost",
				Port: 9000,
			},
			wantErr: true,
		},
		{
			name: "idle conns exceed open conns",
			config: ClickHouseConfig{
				Host:         "localhost",
				Port:         9000,
				Database:     "test",
				MaxOpenConns: 5,
				MaxIdleConns: 10,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCacheValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  CacheConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: CacheConfig{
				Path:              "/tmp/cache",
				MaxMemoryMB:       256,
				ValueLogMaxMB:     100,
				MaxSearchResults:  500,
				GCDiscardRatio:    0.5,
				Enabled:           true,
			},
			wantErr: false,
		},
		{
			name: "missing path",
			config: CacheConfig{
				MaxMemoryMB:      256,
				ValueLogMaxMB:    100,
				MaxSearchResults: 500,
				Enabled:          true,
			},
			wantErr: true,
		},
		{
			name: "invalid max memory",
			config: CacheConfig{
				Path:             "/tmp/cache",
				MaxMemoryMB:      0,
				ValueLogMaxMB:    100,
				MaxSearchResults: 500,
				Enabled:          true,
			},
			wantErr: true,
		},
		{
			name: "invalid gc ratio",
			config: CacheConfig{
				Path:             "/tmp/cache",
				MaxMemoryMB:      256,
				ValueLogMaxMB:    100,
				MaxSearchResults: 500,
				GCDiscardRatio:   1.5,
				Enabled:          true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFTPValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  FTPConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: FTPConfig{
				Host:           "0.0.0.0",
				Port:           2121,
				NodelistPath:   "/tmp/nodelists",
				MaxConnections: 10,
				PassivePortMin: 50000,
				PassivePortMax: 50100,
				IdleTimeout:    300 * time.Second,
				Enabled:        true,
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			config: FTPConfig{
				Host:           "0.0.0.0",
				Port:           99999,
				NodelistPath:   "/tmp/nodelists",
				MaxConnections: 10,
				Enabled:        true,
			},
			wantErr: true,
		},
		{
			name: "missing nodelist path",
			config: FTPConfig{
				Host:           "0.0.0.0",
				Port:           2121,
				MaxConnections: 10,
				Enabled:        true,
			},
			wantErr: true,
		},
		{
			name: "passive port range invalid",
			config: FTPConfig{
				Host:           "0.0.0.0",
				Port:           2121,
				NodelistPath:   "/tmp/nodelists",
				MaxConnections: 10,
				PassivePortMin: 50100,
				PassivePortMax: 50000,
				Enabled:        true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoggingValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  LoggingConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: LoggingConfig{
				Level:      "info",
				Console:    true,
				MaxSize:    100,
				MaxBackups: 3,
				MaxAge:     28,
			},
			wantErr: false,
		},
		{
			name: "invalid level",
			config: LoggingConfig{
				Level:   "invalid",
				Console: true,
			},
			wantErr: true,
		},
		{
			name: "negative max size",
			config: LoggingConfig{
				Level:   "info",
				Console: true,
				MaxSize: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidationErrors(t *testing.T) {
	var errs ValidationErrors

	if errs.HasErrors() {
		t.Error("Empty ValidationErrors should not have errors")
	}

	if errs.Error() != "" {
		t.Error("Empty ValidationErrors should return empty string")
	}

	// Add some errors
	errs.Add(nil) // Should be ignored
	if errs.HasErrors() {
		t.Error("Adding nil should not create errors")
	}

	errs.Add(ErrInvalidConfig("test error 1"))
	errs.Add(ErrInvalidConfig("test error 2"))

	if !errs.HasErrors() {
		t.Error("Should have errors after adding")
	}

	if len(errs.Errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(errs.Errors))
	}

	errMsg := errs.Error()
	if !strings.Contains(errMsg, "test error 1") || !strings.Contains(errMsg, "test error 2") {
		t.Errorf("Error message doesn't contain expected errors: %s", errMsg)
	}
}

func TestConfigValidate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &Config{
			ClickHouse: ClickHouseConfig{
				Host:     "localhost",
				Port:     9000,
				Database: "test",
			},
			Logging: LoggingConfig{
				Level:   "info",
				Console: true,
			},
		}

		if err := cfg.Validate(); err != nil {
			t.Errorf("Valid config should not error: %v", err)
		}
	})

	t.Run("invalid clickhouse config", func(t *testing.T) {
		cfg := &Config{
			ClickHouse: ClickHouseConfig{
				// Missing required fields
			},
			Logging: LoggingConfig{
				Level:   "info",
				Console: true,
			},
		}

		if err := cfg.Validate(); err == nil {
			t.Error("Invalid config should error")
		}
	})

	t.Run("multiple validation errors", func(t *testing.T) {
		cfg := &Config{
			ClickHouse: ClickHouseConfig{
				Host: "localhost",
				Port: 99999, // Invalid
				// Missing database
			},
			Cache: CacheConfig{
				Enabled:     true,
				MaxMemoryMB: -1, // Invalid
			},
			Logging: LoggingConfig{
				Level: "invalid", // Invalid
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("Expected validation errors")
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "configuration validation failed") {
			t.Errorf("Error message should indicate validation failure: %s", errMsg)
		}
	})
}

// Helper function for creating config errors
func ErrInvalidConfig(msg string) error {
	return &ValidationErrors{
		Errors: []error{
			&configError{msg: msg},
		},
	}
}

type configError struct {
	msg string
}

func (e *configError) Error() string {
	return e.msg
}
