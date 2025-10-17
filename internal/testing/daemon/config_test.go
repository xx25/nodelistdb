package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary test config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")

	configContent := `
daemon:
  test_interval: 3600s
  workers: 5
  batch_size: 50
  stale_test_threshold: 7200s
  failed_retry_interval: 86400s

clickhouse:
  host: testhost
  port: 9000
  database: testdb
  username: testuser
  password: testpass

protocols:
  binkp:
    enabled: true
    port: 24554
    timeout: 30s
    our_address: "2:5001/100"
  ifcico:
    enabled: true
    timeout: 20s

services:
  dns:
    workers: 10
    timeout: 5s
    cache_ttl: 3600s
  geolocation:
    provider: ip-api
    cache_ttl: 86400s
    rate_limit: 150

testdaemon_cache:
  enabled: true
  type: badger
  path: ./test-cache

logging:
  level: debug
  console: true

cli:
  enabled: true
  host: 0.0.0.0
  port: 2323
  max_clients: 10
  timeout: 600s
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Test loading the config
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify daemon config
	if cfg.Daemon.TestInterval != 3600*time.Second {
		t.Errorf("Expected TestInterval 3600s, got %v", cfg.Daemon.TestInterval)
	}
	if cfg.Daemon.Workers != 5 {
		t.Errorf("Expected Workers 5, got %d", cfg.Daemon.Workers)
	}
	if cfg.Daemon.BatchSize != 50 {
		t.Errorf("Expected BatchSize 50, got %d", cfg.Daemon.BatchSize)
	}
	if cfg.Daemon.StaleTestThreshold != 7200*time.Second {
		t.Errorf("Expected StaleTestThreshold 7200s, got %v", cfg.Daemon.StaleTestThreshold)
	}
	if cfg.Daemon.FailedRetryInterval != 86400*time.Second {
		t.Errorf("Expected FailedRetryInterval 86400s, got %v", cfg.Daemon.FailedRetryInterval)
	}

	// Verify ClickHouse config
	if cfg.ClickHouse.Host != "testhost" {
		t.Errorf("Expected ClickHouse host 'testhost', got %s", cfg.ClickHouse.Host)
	}
	if cfg.ClickHouse.Port != 9000 {
		t.Errorf("Expected ClickHouse port 9000, got %d", cfg.ClickHouse.Port)
	}
	if cfg.ClickHouse.Database != "testdb" {
		t.Errorf("Expected ClickHouse database 'testdb', got %s", cfg.ClickHouse.Database)
	}

	// Verify protocol config
	if !cfg.Protocols.BinkP.Enabled {
		t.Error("Expected BinkP to be enabled")
	}
	if cfg.Protocols.BinkP.Timeout != 30*time.Second {
		t.Errorf("Expected BinkP timeout 30s, got %v", cfg.Protocols.BinkP.Timeout)
	}
	if cfg.Protocols.BinkP.OurAddress != "2:5001/100" {
		t.Errorf("Expected BinkP OurAddress '2:5001/100', got %s", cfg.Protocols.BinkP.OurAddress)
	}

	// Verify services config
	if cfg.Services.DNS.Workers != 10 {
		t.Errorf("Expected DNS workers 10, got %d", cfg.Services.DNS.Workers)
	}
	if cfg.Services.DNS.Timeout != 5*time.Second {
		t.Errorf("Expected DNS timeout 5s, got %v", cfg.Services.DNS.Timeout)
	}

	// Verify testdaemon_cache config
	if !cfg.TestdaemonCache.Enabled {
		t.Error("Expected testdaemon_cache to be enabled")
	}
	if cfg.TestdaemonCache.Type != "badger" {
		t.Errorf("Expected cache type 'badger', got %s", cfg.TestdaemonCache.Type)
	}

	// Verify CLI config
	if !cfg.CLI.Enabled {
		t.Error("Expected CLI to be enabled")
	}
	if cfg.CLI.Port != 2323 {
		t.Errorf("Expected CLI port 2323, got %d", cfg.CLI.Port)
	}
	if cfg.CLI.Timeout != 600*time.Second {
		t.Errorf("Expected CLI timeout 600s, got %v", cfg.CLI.Timeout)
	}

	// Verify ConfigPath is set
	if cfg.ConfigPath != configPath {
		t.Errorf("Expected ConfigPath %s, got %s", configPath, cfg.ConfigPath)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	// Create a minimal config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "minimal_config.yaml")

	configContent := `
clickhouse:
  host: localhost
  database: testdb

protocols:
  binkp:
    enabled: true
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify defaults are applied
	if cfg.Daemon.Workers != 10 {
		t.Errorf("Expected default Workers 10, got %d", cfg.Daemon.Workers)
	}
	if cfg.Daemon.BatchSize != 100 {
		t.Errorf("Expected default BatchSize 100, got %d", cfg.Daemon.BatchSize)
	}
	if cfg.Daemon.TestInterval != 3600*time.Second {
		t.Errorf("Expected default TestInterval 3600s, got %v", cfg.Daemon.TestInterval)
	}
	if cfg.Daemon.StaleTestThreshold != cfg.Daemon.TestInterval {
		t.Errorf("Expected StaleTestThreshold to equal TestInterval, got %v", cfg.Daemon.StaleTestThreshold)
	}
	if cfg.Daemon.FailedRetryInterval != 24*time.Hour {
		t.Errorf("Expected default FailedRetryInterval 24h, got %v", cfg.Daemon.FailedRetryInterval)
	}

	if cfg.Services.DNS.Workers != 20 {
		t.Errorf("Expected default DNS workers 20, got %d", cfg.Services.DNS.Workers)
	}
	if cfg.Services.DNS.Timeout != 5*time.Second {
		t.Errorf("Expected default DNS timeout 5s, got %v", cfg.Services.DNS.Timeout)
	}

	// Verify testdaemon_cache defaults
	if cfg.TestdaemonCache.Type != "badger" {
		t.Errorf("Expected default cache type 'badger', got %s", cfg.TestdaemonCache.Type)
	}
	if cfg.TestdaemonCache.Path != "./cache/badger-testdaemon" {
		t.Errorf("Expected default cache path './cache/badger-testdaemon', got %s", cfg.TestdaemonCache.Path)
	}

	// Verify logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected default logging level 'info', got %s", cfg.Logging.Level)
	}

	// Verify BinkP system info defaults
	if cfg.Protocols.BinkP.SystemName == "" {
		t.Error("Expected BinkP SystemName to have default value")
	}
	if cfg.Protocols.BinkP.Sysop == "" {
		t.Error("Expected BinkP Sysop to have default value")
	}
	if cfg.Protocols.BinkP.Location == "" {
		t.Error("Expected BinkP Location to have default value")
	}

	// Verify CLI defaults
	if cfg.CLI.Host != "127.0.0.1" {
		t.Errorf("Expected default CLI host '127.0.0.1', got %s", cfg.CLI.Host)
	}
	if cfg.CLI.Port != 2323 {
		t.Errorf("Expected default CLI port 2323, got %d", cfg.CLI.Port)
	}
	if cfg.CLI.MaxClients != 5 {
		t.Errorf("Expected default CLI max_clients 5, got %d", cfg.CLI.MaxClients)
	}
	if cfg.CLI.Timeout != 300*time.Second {
		t.Errorf("Expected default CLI timeout 300s, got %v", cfg.CLI.Timeout)
	}
}

func TestLoadConfigInvalidPath(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent config file, got nil")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
	}{
		{
			name: "valid config",
			config: &Config{
				ClickHouse: &ClickHouseConfig{
					Host:     "localhost",
					Database: "testdb",
				},
				Protocols: ProtocolsConfig{
					BinkP: ProtocolConfig{
						Enabled: true,
					},
				},
			},
			wantError: false,
		},
		{
			name: "missing clickhouse config",
			config: &Config{
				Protocols: ProtocolsConfig{
					BinkP: ProtocolConfig{
						Enabled: true,
					},
				},
			},
			wantError: true,
		},
		{
			name: "missing clickhouse host",
			config: &Config{
				ClickHouse: &ClickHouseConfig{
					Database: "testdb",
				},
				Protocols: ProtocolsConfig{
					BinkP: ProtocolConfig{
						Enabled: true,
					},
				},
			},
			wantError: true,
		},
		{
			name: "missing clickhouse database",
			config: &Config{
				ClickHouse: &ClickHouseConfig{
					Host: "localhost",
				},
				Protocols: ProtocolsConfig{
					BinkP: ProtocolConfig{
						Enabled: true,
					},
				},
			},
			wantError: true,
		},
		{
			name: "no protocols enabled",
			config: &Config{
				ClickHouse: &ClickHouseConfig{
					Host:     "localhost",
					Database: "testdb",
				},
				Protocols: ProtocolsConfig{
					BinkP:  ProtocolConfig{Enabled: false},
					Ifcico: ProtocolConfig{Enabled: false},
					Telnet: ProtocolConfig{Enabled: false},
					FTP:    ProtocolConfig{Enabled: false},
					VModem: ProtocolConfig{Enabled: false},
				},
			},
			wantError: true,
		},
		{
			name: "multiple protocols enabled",
			config: &Config{
				ClickHouse: &ClickHouseConfig{
					Host:     "localhost",
					Database: "testdb",
				},
				Protocols: ProtocolsConfig{
					BinkP:  ProtocolConfig{Enabled: true},
					Ifcico: ProtocolConfig{Enabled: true},
					Telnet: ProtocolConfig{Enabled: false},
				},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestConfigDurationConversion(t *testing.T) {
	// Create a config with durations specified as plain seconds
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "duration_config.yaml")

	configContent := `
daemon:
  test_interval: 7200s
  stale_test_threshold: 14400s
  failed_retry_interval: 43200s

clickhouse:
  host: localhost
  database: testdb

protocols:
  binkp:
    enabled: true
    timeout: 45s

services:
  dns:
    timeout: 10s
    cache_ttl: 7200s
  geolocation:
    cache_ttl: 172800s

cli:
  timeout: 900s
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify durations are properly converted from seconds to time.Duration
	if cfg.Daemon.TestInterval != 7200*time.Second {
		t.Errorf("Expected TestInterval 7200s, got %v", cfg.Daemon.TestInterval)
	}
	if cfg.Daemon.StaleTestThreshold != 14400*time.Second {
		t.Errorf("Expected StaleTestThreshold 14400s, got %v", cfg.Daemon.StaleTestThreshold)
	}
	if cfg.Daemon.FailedRetryInterval != 43200*time.Second {
		t.Errorf("Expected FailedRetryInterval 43200s, got %v", cfg.Daemon.FailedRetryInterval)
	}
	if cfg.Protocols.BinkP.Timeout != 45*time.Second {
		t.Errorf("Expected BinkP timeout 45s, got %v", cfg.Protocols.BinkP.Timeout)
	}
	if cfg.Services.DNS.Timeout != 10*time.Second {
		t.Errorf("Expected DNS timeout 10s, got %v", cfg.Services.DNS.Timeout)
	}
	if cfg.Services.Geolocation.CacheTTL != 172800*time.Second {
		t.Errorf("Expected Geolocation CacheTTL 172800s, got %v", cfg.Services.Geolocation.CacheTTL)
	}
	if cfg.CLI.Timeout != 900*time.Second {
		t.Errorf("Expected CLI timeout 900s, got %v", cfg.CLI.Timeout)
	}
}
