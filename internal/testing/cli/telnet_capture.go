package cli

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
)

// TelnetCapture captures debug output for telnet sessions
type TelnetCapture struct {
	mu        sync.Mutex
	enabled   bool
	writer    *bufio.Writer
	formatter *Formatter
}

// NewTelnetCapture creates a new debug capturer
func NewTelnetCapture(writer *bufio.Writer, formatter *Formatter) *TelnetCapture {
	return &TelnetCapture{
		writer:    writer,
		formatter: formatter,
		enabled:   false,
	}
}

// Enable starts capturing debug output
func (dc *TelnetCapture) Enable() {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.enabled = true
}

// Disable stops capturing debug output
func (dc *TelnetCapture) Disable() {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.enabled = false
}

// IsEnabled returns if capture is enabled
func (dc *TelnetCapture) IsEnabled() bool {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	return dc.enabled
}

// Write implements io.Writer to capture debug output
func (dc *TelnetCapture) Write(p []byte) (n int, err error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if !dc.enabled {
		return len(p), nil
	}

	// Format and write debug output to telnet
	lines := strings.Split(string(p), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			// Add [DEBUG] prefix and write to telnet
			dc.formatter.WriteDebug(line)
		}
	}
	dc.writer.Flush()

	return len(p), nil
}

// TeeLogger creates a logger that writes to both original and capture
type TeeLogger struct {
	original io.Writer
	capture  *TelnetCapture
}

// NewTeeLogger creates a new tee logger
func NewTeeLogger(original io.Writer, capture *TelnetCapture) *TeeLogger {
	return &TeeLogger{
		original: original,
		capture:  capture,
	}
}

// Write writes to both original and capture if enabled
func (tl *TeeLogger) Write(p []byte) (n int, err error) {
	// Always write to original
	n, err = tl.original.Write(p)

	// Also write to capture if enabled (ignore errors from capture)
	if tl.capture.IsEnabled() {
		_, _ = tl.capture.Write(p)
	}

	return n, err
}

// SetupTelnetCapture sets up log capturing for a handler
func SetupTelnetCapture(handler *Handler) *TelnetCapture {
	capture := NewTelnetCapture(handler.writer, handler.formatter)

	// Create a tee logger that writes to both stderr and capture
	teeLogger := NewTeeLogger(log.Writer(), capture)

	// Set the default logger to use our tee logger
	log.SetOutput(teeLogger)

	return capture
}

// WriteDebugLine writes a debug line directly to telnet
func (dc *TelnetCapture) WriteDebugLine(format string, args ...interface{}) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.enabled {
		line := fmt.Sprintf(format, args...)
		dc.formatter.WriteDebug(line)
		dc.writer.Flush()
	}
}