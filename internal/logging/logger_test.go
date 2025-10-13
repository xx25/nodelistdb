package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitialize(t *testing.T) {
	// Test default initialization
	err := Initialize(nil)
	if err != nil {
		t.Fatalf("Failed to initialize with default config: %v", err)
	}

	logger := GetLogger()
	if logger == nil {
		t.Fatal("GetLogger returned nil")
	}

	// Test with custom config
	cfg := &Config{
		Level:   "debug",
		Console: true,
		JSON:    false,
	}
	err = Initialize(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize with custom config: %v", err)
	}
}

func TestGetLogger(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	logger := GetLogger()
	if logger == nil {
		t.Fatal("GetLogger returned nil")
	}

	// Should return same instance
	logger2 := GetLogger()
	if logger != logger2 {
		t.Error("GetLogger should return same instance")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"invalid", slog.LevelInfo}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseLevel(tt.input)
			if level != tt.expected {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, level, tt.expected)
			}
		})
	}
}

func TestLoggingLevels(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Initialize logger with buffer
	cfg := &Config{
		Level:   "debug",
		Console: false,
	}

	// We can't directly test output since slog writes to configured writer
	// But we can test that methods don't panic
	err := Initialize(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Test all log levels
	t.Run("Debug", func(t *testing.T) {
		Debug("debug message")
		Debugf("debug formatted %s", "message")
	})

	t.Run("Info", func(t *testing.T) {
		Info("info message")
		Infof("info formatted %s", "message")
	})

	t.Run("Warn", func(t *testing.T) {
		Warn("warn message")
		Warnf("warn formatted %s", "message")
	})

	t.Run("Error", func(t *testing.T) {
		Error("error message")
		Errorf("error formatted %s", "message")
	})

	_ = buf // prevent unused variable warning
}

func TestStructuredLogging(t *testing.T) {
	err := Initialize(&Config{
		Level:   "debug",
		Console: false,
	})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Test structured fields
	t.Run("WithField", func(t *testing.T) {
		logger := WithField("key", "value")
		if logger == nil {
			t.Error("WithField returned nil")
		}
	})

	t.Run("WithFields", func(t *testing.T) {
		logger := WithFields(map[string]any{
			"zone": 2,
			"net":  450,
			"node": 1024,
		})
		if logger == nil {
			t.Error("WithFields returned nil")
		}
	})

	t.Run("WithError", func(t *testing.T) {
		logger := WithError(os.ErrNotExist)
		if logger == nil {
			t.Error("WithError returned nil")
		}
	})

	t.Run("With", func(t *testing.T) {
		logger := With(
			slog.Int("zone", 2),
			slog.Int("net", 450),
			slog.String("protocol", "binkp"),
		)
		if logger == nil {
			t.Error("With returned nil")
		}
	})
}

func TestFileLogging(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	cfg := &Config{
		Level:   "info",
		File:    logFile,
		Console: false,
	}

	err := Initialize(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize with file: %v", err)
	}

	// Write some logs
	Info("test message 1")
	Info("test message 2")

	// Close to flush
	err = GetLogger().Close()
	if err != nil {
		t.Fatalf("Failed to close logger: %v", err)
	}

	// Check file exists and has content
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "test message 1") {
		t.Error("Log file doesn't contain expected message")
	}
}

func TestReload(t *testing.T) {
	err := Initialize(&Config{
		Level:   "info",
		Console: true,
	})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	logger := GetLogger()

	// Reload with new config
	newCfg := &Config{
		Level:   "debug",
		Console: true,
		JSON:    true,
	}

	err = logger.Reload(newCfg)
	if err != nil {
		t.Fatalf("Failed to reload: %v", err)
	}

	if logger.config.Level != "debug" {
		t.Error("Config level not updated")
	}
}

func TestJSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	cfg := &Config{
		Level:   "info",
		File:    logFile,
		Console: false,
		JSON:    true,
	}

	err := Initialize(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize with JSON format: %v", err)
	}

	Info("json test message")

	// Close to flush
	err = GetLogger().Close()
	if err != nil {
		t.Fatalf("Failed to close logger: %v", err)
	}

	// Check file contains JSON
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content := string(data)
	// JSON logs should contain quoted strings and structured format
	if !strings.Contains(content, `"msg"`) && !strings.Contains(content, `"level"`) {
		t.Error("Log file doesn't appear to contain JSON formatted logs")
	}
}

func TestCompatibilityFunctions(t *testing.T) {
	err := Initialize(&Config{
		Level:   "info",
		Console: false,
	})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Test compatibility functions don't panic
	Printf("printf test %d", 42)
	Println("println test", 42)
}
