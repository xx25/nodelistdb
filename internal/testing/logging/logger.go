package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger wraps zerolog with configuration
type Logger struct {
	logger zerolog.Logger
	config *Config
	file   io.WriteCloser
}

// Config holds logging configuration
type Config struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxSize    int    `yaml:"max_size"`    // megabytes
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`     // days
	Console    bool   `yaml:"console"`     // log to console as well
}

var globalLogger *Logger

// Initialize sets up the global logger
func Initialize(cfg *Config) error {
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
		globalLogger.configure()
	}
	return globalLogger
}

// configure sets up the logger based on config
func (l *Logger) configure() error {
	// Parse log level
	level, err := zerolog.ParseLevel(strings.ToLower(l.config.Level))
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Configure time format
	zerolog.TimeFieldFormat = time.RFC3339

	// Setup writers
	var writers []io.Writer

	// Console output if enabled
	if l.config.Console {
		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}
		writers = append(writers, consoleWriter)
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
		writer = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}
	} else if len(writers) == 1 {
		writer = writers[0]
	} else {
		writer = zerolog.MultiLevelWriter(writers...)
	}

	// Create logger
	l.logger = zerolog.New(writer).With().Timestamp().Logger()
	
	// Set global logger
	log.Logger = l.logger
	
	return nil
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

// Logger methods for different levels

func (l *Logger) Debug(msg string) {
	l.logger.Debug().Msg(msg)
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	l.logger.Debug().Msgf(format, v...)
}

func (l *Logger) Info(msg string) {
	l.logger.Info().Msg(msg)
}

func (l *Logger) Infof(format string, v ...interface{}) {
	l.logger.Info().Msgf(format, v...)
}

func (l *Logger) Warn(msg string) {
	l.logger.Warn().Msg(msg)
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	l.logger.Warn().Msgf(format, v...)
}

func (l *Logger) Error(msg string) {
	l.logger.Error().Msg(msg)
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	l.logger.Error().Msgf(format, v...)
}

func (l *Logger) Fatal(msg string) {
	l.logger.Fatal().Msg(msg)
}

func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.logger.Fatal().Msgf(format, v...)
}

// WithField returns a logger with a single field
func (l *Logger) WithField(key string, value interface{}) *zerolog.Logger {
	logger := l.logger.With().Interface(key, value).Logger()
	return &logger
}

// WithFields returns a logger with multiple fields
func (l *Logger) WithFields(fields map[string]interface{}) *zerolog.Logger {
	context := l.logger.With()
	for k, v := range fields {
		context = context.Interface(k, v)
	}
	logger := context.Logger()
	return &logger
}

// WithError returns a logger with an error field
func (l *Logger) WithError(err error) *zerolog.Logger {
	logger := l.logger.With().Err(err).Logger()
	return &logger
}

// Package-level convenience functions

func Debug(msg string) {
	GetLogger().Debug(msg)
}

func Debugf(format string, v ...interface{}) {
	GetLogger().Debugf(format, v...)
}

func Info(msg string) {
	GetLogger().Info(msg)
}

func Infof(format string, v ...interface{}) {
	GetLogger().Infof(format, v...)
}

func Warn(msg string) {
	GetLogger().Warn(msg)
}

func Warnf(format string, v ...interface{}) {
	GetLogger().Warnf(format, v...)
}

func Error(msg string) {
	GetLogger().Error(msg)
}

func Errorf(format string, v ...interface{}) {
	GetLogger().Errorf(format, v...)
}

func Fatal(msg string) {
	GetLogger().Fatal(msg)
}

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