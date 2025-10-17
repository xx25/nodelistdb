package daemon

import (
	"os"
	"testing"
	"time"
)

func TestDaemonPause(t *testing.T) {
	daemon := &Daemon{
		paused: false,
	}

	err := daemon.Pause()
	if err != nil {
		t.Errorf("Expected no error on first pause, got %v", err)
	}

	if !daemon.IsPaused() {
		t.Error("Expected daemon to be paused")
	}
}

func TestDaemonPauseAlreadyPaused(t *testing.T) {
	daemon := &Daemon{
		paused: true,
	}

	err := daemon.Pause()
	if err == nil {
		t.Error("Expected error when pausing already paused daemon")
	}

	expectedMsg := "daemon is already paused"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestDaemonResume(t *testing.T) {
	daemon := &Daemon{
		paused: true,
	}

	err := daemon.Resume()
	if err != nil {
		t.Errorf("Expected no error on resume, got %v", err)
	}

	if daemon.IsPaused() {
		t.Error("Expected daemon to not be paused")
	}
}

func TestDaemonResumeNotPaused(t *testing.T) {
	daemon := &Daemon{
		paused: false,
	}

	err := daemon.Resume()
	if err == nil {
		t.Error("Expected error when resuming non-paused daemon")
	}

	expectedMsg := "daemon is not paused"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestDaemonPauseResumeSequence(t *testing.T) {
	daemon := &Daemon{
		paused: false,
	}

	// Initial state
	if daemon.IsPaused() {
		t.Error("Daemon should start unpaused")
	}

	// Pause
	if err := daemon.Pause(); err != nil {
		t.Errorf("Pause failed: %v", err)
	}
	if !daemon.IsPaused() {
		t.Error("Daemon should be paused")
	}

	// Resume
	if err := daemon.Resume(); err != nil {
		t.Errorf("Resume failed: %v", err)
	}
	if daemon.IsPaused() {
		t.Error("Daemon should be unpaused")
	}

	// Pause again
	if err := daemon.Pause(); err != nil {
		t.Errorf("Second pause failed: %v", err)
	}
	if !daemon.IsPaused() {
		t.Error("Daemon should be paused again")
	}
}

func TestDaemonSetDebugMode(t *testing.T) {
	daemon := &Daemon{
		debug: false,
	}

	err := daemon.SetDebugMode(true)
	if err != nil {
		t.Errorf("Expected no error setting debug mode, got %v", err)
	}

	if !daemon.GetDebugMode() {
		t.Error("Expected debug mode to be enabled")
	}
}

func TestDaemonSetDebugModeOff(t *testing.T) {
	daemon := &Daemon{
		debug: true,
	}

	err := daemon.SetDebugMode(false)
	if err != nil {
		t.Errorf("Expected no error disabling debug mode, got %v", err)
	}

	if daemon.GetDebugMode() {
		t.Error("Expected debug mode to be disabled")
	}
}

func TestDaemonGetDebugMode(t *testing.T) {
	tests := []struct {
		name     string
		initial  bool
		expected bool
	}{
		{"debug enabled", true, true},
		{"debug disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			daemon := &Daemon{
				debug: tt.initial,
			}

			result := daemon.GetDebugMode()
			if result != tt.expected {
				t.Errorf("GetDebugMode() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDaemonIsPausedConcurrent(t *testing.T) {
	daemon := &Daemon{
		paused: false,
	}

	// Test concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_ = daemon.IsPaused()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestDaemonPauseResumeConcurrent(t *testing.T) {
	daemon := &Daemon{
		paused: false,
	}

	// This tests thread safety of pause/resume operations
	// Start with pause
	err := daemon.Pause()
	if err != nil {
		t.Fatalf("Initial pause failed: %v", err)
	}

	// Try concurrent resume operations (only one should succeed)
	results := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func() {
			results <- daemon.Resume()
		}()
	}

	// Collect results
	successCount := 0
	errorCount := 0
	for i := 0; i < 5; i++ {
		err := <-results
		if err == nil {
			successCount++
		} else {
			errorCount++
		}
	}

	// Exactly one should succeed, others should fail
	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful resume, got %d", successCount)
	}
	if errorCount != 4 {
		t.Errorf("Expected 4 failed resumes, got %d", errorCount)
	}

	if daemon.IsPaused() {
		t.Error("Daemon should not be paused after successful resume")
	}
}

func TestConfigValidationInReloadConfig(t *testing.T) {
	// Test that invalid config is rejected
	tmpDir := t.TempDir()
	configPath := tmpDir + "/invalid_config.yaml"

	// Create an invalid config (missing required fields)
	invalidConfig := `
daemon:
  test_interval: 3600s

# Missing clickhouse config (required)
protocols:
  binkp:
    enabled: false
  ifcico:
    enabled: false
  telnet:
    enabled: false
  ftp:
    enabled: false
  vmodem:
    enabled: false
`

	if err := writeFile(configPath, []byte(invalidConfig)); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Create a daemon with minimal valid config
	daemon := &Daemon{
		config: &Config{
			ClickHouse: &ClickHouseConfig{
				Host:     "localhost",
				Database: "testdb",
			},
			Protocols: ProtocolsConfig{
				BinkP: ProtocolConfig{Enabled: true},
			},
		},
	}

	// Try to reload with invalid config
	err := daemon.ReloadConfig(configPath)
	if err == nil {
		t.Error("Expected error when reloading invalid config, got nil")
	}
}

func TestConfigReloadUpdatesScheduler(t *testing.T) {
	// This test verifies that scheduler intervals are updated on config reload
	tmpDir := t.TempDir()
	configPath := tmpDir + "/test_config.yaml"

	newConfig := `
daemon:
  test_interval: 7200s
  stale_test_threshold: 14400s
  failed_retry_interval: 86400s
  workers: 5
  batch_size: 100

clickhouse:
  host: localhost
  port: 9000
  database: testdb

protocols:
  binkp:
    enabled: true
    port: 24554
    timeout: 30s
    our_address: "2:5001/100"

services:
  dns:
    workers: 10
    timeout: 5s
    cache_ttl: 3600s
  geolocation:
    provider: ip-api
    cache_ttl: 86400s
    rate_limit: 150
`

	if err := writeFile(configPath, []byte(newConfig)); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Create daemon with scheduler
	daemon := &Daemon{
		config: &Config{
			Daemon: DaemonConfig{
				TestInterval:        3600 * time.Second,
				StaleTestThreshold:  3600 * time.Second,
				FailedRetryInterval: 24 * time.Hour,
			},
			ClickHouse: &ClickHouseConfig{
				Host:     "localhost",
				Database: "testdb",
			},
			Protocols: ProtocolsConfig{
				BinkP: ProtocolConfig{
					Enabled:    true,
					OurAddress: "2:5001/100",
				},
			},
			Services: ServicesConfig{
				DNS: DNSConfig{
					Workers:  20,
					Timeout:  5 * time.Second,
					CacheTTL: 3600 * time.Second,
				},
				Geolocation: GeolocationConfig{
					Provider:  "ip-api",
					CacheTTL:  24 * time.Hour,
					RateLimit: 150,
				},
			},
		},
		scheduler: &Scheduler{
			baseInterval:        3600 * time.Second,
			staleTestThreshold:  3600 * time.Second,
			failedRetryInterval: 24 * time.Hour,
		},
	}

	// Reload config
	err := daemon.ReloadConfig(configPath)
	if err != nil {
		t.Fatalf("ReloadConfig failed: %v", err)
	}

	// Verify scheduler was updated
	if daemon.scheduler.baseInterval != 7200*time.Second {
		t.Errorf("Expected scheduler baseInterval 7200s, got %v", daemon.scheduler.baseInterval)
	}
	if daemon.scheduler.staleTestThreshold != 14400*time.Second {
		t.Errorf("Expected scheduler staleTestThreshold 14400s, got %v", daemon.scheduler.staleTestThreshold)
	}
	if daemon.scheduler.failedRetryInterval != 86400*time.Second {
		t.Errorf("Expected scheduler failedRetryInterval 86400s, got %v", daemon.scheduler.failedRetryInterval)
	}
}

func TestConfigReloadUpdatesProtocolTesters(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/test_config.yaml"

	newConfig := `
daemon:
  test_interval: 3600s
  workers: 5
  batch_size: 100

clickhouse:
  host: localhost
  port: 9000
  database: testdb

protocols:
  binkp:
    enabled: true
    port: 24554
    timeout: 45s
    our_address: "2:5001/100"
  ifcico:
    enabled: false

services:
  dns:
    workers: 10
    timeout: 5s
    cache_ttl: 3600s
  geolocation:
    provider: ip-api
    cache_ttl: 86400s
    rate_limit: 150
`

	if err := writeFile(configPath, []byte(newConfig)); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	daemon := &Daemon{
		config: &Config{
			Daemon: DaemonConfig{
				TestInterval: 3600 * time.Second,
			},
			ClickHouse: &ClickHouseConfig{
				Host:     "localhost",
				Database: "testdb",
			},
			Protocols: ProtocolsConfig{
				BinkP: ProtocolConfig{
					Enabled:    true,
					OurAddress: "2:5001/100",
					Timeout:    30 * time.Second,
				},
				Ifcico: ProtocolConfig{
					Enabled: true,
					Timeout: 20 * time.Second,
				},
			},
			Services: ServicesConfig{
				DNS: DNSConfig{
					Workers:  20,
					Timeout:  5 * time.Second,
					CacheTTL: 3600 * time.Second,
				},
				Geolocation: GeolocationConfig{
					Provider:  "ip-api",
					CacheTTL:  24 * time.Hour,
					RateLimit: 150,
				},
			},
		},
		scheduler: &Scheduler{
			baseInterval:        3600 * time.Second,
			staleTestThreshold:  3600 * time.Second,
			failedRetryInterval: 24 * time.Hour,
		},
	}

	// Reload config
	err := daemon.ReloadConfig(configPath)
	if err != nil {
		t.Fatalf("ReloadConfig failed: %v", err)
	}

	// Verify BinkP tester was recreated (should not be nil)
	if daemon.binkpTester == nil {
		t.Error("Expected binkpTester to be initialized after reload")
	}

	// Verify Ifcico tester was disabled (should be nil)
	if daemon.ifcicoTester != nil {
		t.Error("Expected ifcicoTester to be nil after reload (disabled in config)")
	}
}

// Helper function to write files for tests
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
