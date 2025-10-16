package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger wraps slog with configuration and lifecycle management
type Logger struct {
	config *Config
	file   io.WriteCloser
	logger *slog.Logger
}

// Config holds logging configuration
type Config struct {
	Level      string `yaml:"level"`       // debug, info, warn, error
	File       string `yaml:"file"`        // log file path (optional)
	MaxSize    int    `yaml:"max_size"`    // megabytes
	MaxBackups int    `yaml:"max_backups"` // number of old log files to keep
	MaxAge     int    `yaml:"max_age"`     // days
	Console    bool   `yaml:"console"`     // also log to console
	JSON       bool   `yaml:"json"`        // JSON format instead of text
}

// FromStruct creates a Config from any struct with matching fields (duck typing)
// This allows conversion from config.LoggingConfig to logging.Config
func FromStruct(src interface{}) *Config {
	// Use type assertion for known config package type
	type configLike struct {
		Level      string
		File       string
		MaxSize    int
		MaxBackups int
		MaxAge     int
		Console    bool
		JSON       bool
	}

	// Try to convert via interface
	if cfg, ok := src.(configLike); ok {
		return &Config{
			Level:      cfg.Level,
			File:       cfg.File,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Console:    cfg.Console,
			JSON:       cfg.JSON,
		}
	}

	// If direct conversion doesn't work, try pointer
	if cfg, ok := src.(*configLike); ok {
		return &Config{
			Level:      cfg.Level,
			File:       cfg.File,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Console:    cfg.Console,
			JSON:       cfg.JSON,
		}
	}

	// Default config if conversion fails
	return &Config{
		Level:   "info",
		Console: true,
	}
}

var globalLogger *Logger

// Initialize sets up the global logger
func Initialize(cfg *Config) error {
	if cfg == nil {
		cfg = &Config{
			Level:   "info",
			Console: true,
		}
	}

	globalLogger = &Logger{
		config: cfg,
	}
	return globalLogger.configure()
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	if globalLogger == nil {
		// Create default console logger if not initialized
		globalLogger = &Logger{
			config: &Config{
				Level:   "info",
				Console: true,
			},
		}
		_ = globalLogger.configure()
	}
	return globalLogger
}

// configure sets up the logger based on config
func (l *Logger) configure() error {
	// Parse log level
	level := parseLevel(l.config.Level)

	// Setup writers
	var writers []io.Writer

	// Console output if enabled
	if l.config.Console {
		writers = append(writers, os.Stdout)
	}

	// File output if configured
	if l.config.File != "" {
		// Close previous file if any
		if l.file != nil {
			l.file.Close()
		}

		// Setup log rotation
		rotator := &lumberjack.Logger{
			Filename:   l.config.File,
			MaxSize:    l.config.MaxSize,    // megabytes
			MaxBackups: l.config.MaxBackups,
			MaxAge:     l.config.MaxAge, // days
			Compress:   true,
		}
		l.file = rotator
		writers = append(writers, rotator)
	}

	// Create multi-writer
	var writer io.Writer
	if len(writers) == 0 {
		// Default to console if no writers configured
		writer = os.Stdout
	} else if len(writers) == 1 {
		writer = writers[0]
	} else {
		writer = io.MultiWriter(writers...)
	}

	// Create handler
	var handler slog.Handler
	if l.config.JSON {
		handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		handler = slog.NewTextHandler(writer, &slog.HandlerOptions{
			Level: level,
		})
	}

	// Create logger
	l.logger = slog.New(handler)

	// Set as default logger
	slog.SetDefault(l.logger)

	return nil
}

// parseLevel converts string level to slog.Level
func parseLevel(levelStr string) slog.Level {
	switch strings.ToLower(levelStr) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Reload reconfigures the logger with new settings
func (l *Logger) Reload(cfg *Config) error {
	l.config = cfg
	return l.configure()
}

// Close closes any open file handles
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Underlying returns the underlying *slog.Logger for advanced usage
func (l *Logger) Underlying() *slog.Logger {
	return l.logger
}

// Logger methods for different levels with structured fields

func (l *Logger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	l.logger.Debug(fmt.Sprintf(format, v...))
}

func (l *Logger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

func (l *Logger) Infof(format string, v ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, v...))
}

func (l *Logger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, v...))
}

func (l *Logger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, v...))
}

func (l *Logger) Fatal(msg string, args ...any) {
	l.logger.Error(msg, args...)
	os.Exit(1)
}

func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, v...))
	os.Exit(1)
}

// With returns a logger with the given attributes added
func (l *Logger) With(args ...any) *slog.Logger {
	return l.logger.With(args...)
}

// WithError returns a logger with an error field
func (l *Logger) WithError(err error) *slog.Logger {
	return l.logger.With(slog.Any("error", err))
}

// WithField returns a logger with a single field
func (l *Logger) WithField(key string, value any) *slog.Logger {
	return l.logger.With(slog.Any(key, value))
}

// WithFields returns a logger with multiple fields
func (l *Logger) WithFields(fields map[string]any) *slog.Logger {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, slog.Any(k, v))
	}
	return l.logger.With(args...)
}

// Package-level convenience functions

// Debug logs at debug level
func Debug(msg string, args ...any) {
	GetLogger().Debug(msg, args...)
}

// Debugf logs formatted message at debug level
func Debugf(format string, v ...interface{}) {
	GetLogger().Debugf(format, v...)
}

// Info logs at info level
func Info(msg string, args ...any) {
	GetLogger().Info(msg, args...)
}

// Infof logs formatted message at info level
func Infof(format string, v ...interface{}) {
	GetLogger().Infof(format, v...)
}

// Warn logs at warn level
func Warn(msg string, args ...any) {
	GetLogger().Warn(msg, args...)
}

// Warnf logs formatted message at warn level
func Warnf(format string, v ...interface{}) {
	GetLogger().Warnf(format, v...)
}

// Error logs at error level
func Error(msg string, args ...any) {
	GetLogger().Error(msg, args...)
}

// Errorf logs formatted message at error level
func Errorf(format string, v ...interface{}) {
	GetLogger().Errorf(format, v...)
}

// Fatal logs at error level and exits
func Fatal(msg string, args ...any) {
	GetLogger().Fatal(msg, args...)
}

// Fatalf logs formatted message at error level and exits
func Fatalf(format string, v ...interface{}) {
	GetLogger().Fatalf(format, v...)
}

// Printf implements a log.Printf compatible interface
func Printf(format string, v ...interface{}) {
	GetLogger().Infof(format, v...)
}

// Println implements a log.Println compatible interface
func Println(v ...interface{}) {
	GetLogger().Info(fmt.Sprint(v...))
}

// With returns a logger with the given attributes added
func With(args ...any) *slog.Logger {
	return GetLogger().With(args...)
}

// WithError returns a logger with an error field
func WithError(err error) *slog.Logger {
	return GetLogger().WithError(err)
}

// WithField returns a logger with a single field
func WithField(key string, value any) *slog.Logger {
	return GetLogger().WithField(key, value)
}

// WithFields returns a logger with multiple fields
func WithFields(fields map[string]any) *slog.Logger {
	return GetLogger().WithFields(fields)
}
