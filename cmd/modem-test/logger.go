// Package main provides formatted logging for modem testing.
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// TestLogger provides formatted debug logging for modem testing
type TestLogger struct {
	config    LoggingConfig
	startTime time.Time
}

// NewTestLogger creates a new test logger with the given configuration
func NewTestLogger(cfg LoggingConfig) *TestLogger {
	return &TestLogger{
		config:    cfg,
		startTime: time.Now(),
	}
}

// timestamp returns the current time formatted for log output
func (l *TestLogger) timestamp() string {
	if l.config.Timestamps {
		return time.Now().Format("15:04:05.000")
	}
	return ""
}

// log outputs a formatted log line with optional timestamp and category
func (l *TestLogger) log(category, color, message string, args ...interface{}) {
	if !l.config.Debug && category != "RESULT" && category != "SUMMARY" {
		return
	}

	var prefix string
	if l.config.Timestamps {
		prefix = fmt.Sprintf("[%s] ", l.timestamp())
	}

	// Format category with fixed width
	cat := fmt.Sprintf("%-6s", category)

	// Apply color codes for terminal
	if color != "" {
		cat = color + cat + "\033[0m"
	}

	formattedMsg := fmt.Sprintf(message, args...)
	fmt.Fprintf(os.Stderr, "%s%s %s\n", prefix, cat, formattedMsg)
}

// Colors for terminal output
const (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// Init logs an initialization message
func (l *TestLogger) Init(format string, args ...interface{}) {
	l.log("INIT", colorCyan, format, args...)
}

// TX logs data being sent to modem
func (l *TestLogger) TX(data []byte) {
	l.log("TX", colorYellow, "%s", formatBytes(data))
	if l.config.ShowHex && len(data) > 0 {
		l.logHexDump(data, "TX")
	}
}

// TXString logs a string being sent to modem
func (l *TestLogger) TXString(s string) {
	l.TX([]byte(s))
}

// RX logs data received from modem
func (l *TestLogger) RX(data []byte) {
	l.log("RX", colorBlue, "%s", formatBytes(data))
	if l.config.ShowHex && len(data) > 0 {
		l.logHexDump(data, "RX")
	}
}

// RXString logs a string received from modem
func (l *TestLogger) RXString(s string) {
	l.RX([]byte(s))
}

// RS232 logs RS232 control line status
func (l *TestLogger) RS232(dcd, dsr, cts, ri bool) {
	if !l.config.ShowRS232 {
		return
	}
	status := fmt.Sprintf("DCD=%s DSR=%s CTS=%s RI=%s",
		boolToOnOff(dcd), boolToOnOff(dsr), boolToOnOff(cts), boolToOnOff(ri))
	l.log("RS232", colorGray, "%s", status)
}

// OK logs a success message
func (l *TestLogger) OK(format string, args ...interface{}) {
	l.log("OK", colorGreen, format, args...)
}

// Fail logs a failure message
func (l *TestLogger) Fail(format string, args ...interface{}) {
	l.log("FAIL", colorRed, format, args...)
}

// Dial logs dial-related messages
func (l *TestLogger) Dial(format string, args ...interface{}) {
	l.log("DIAL", colorPurple, format, args...)
}

// EMSI logs EMSI handshake messages
func (l *TestLogger) EMSI(format string, args ...interface{}) {
	l.log("EMSI", colorCyan, format, args...)
}

// Hangup logs hangup messages
func (l *TestLogger) Hangup(format string, args ...interface{}) {
	l.log("HANGUP", colorYellow, format, args...)
}

// Stats logs statistics output
func (l *TestLogger) Stats(format string, args ...interface{}) {
	l.log("STATS", colorGray, format, args...)
}

// Result logs test result (always shown)
func (l *TestLogger) Result(format string, args ...interface{}) {
	l.log("RESULT", colorGreen, format, args...)
}

// Info logs general information
func (l *TestLogger) Info(format string, args ...interface{}) {
	l.log("INFO", "", format, args...)
}

// Debug logs debug information (only when debug enabled)
func (l *TestLogger) Debug(format string, args ...interface{}) {
	if l.config.Debug {
		l.log("DEBUG", colorGray, format, args...)
	}
}

// Error logs error messages
func (l *TestLogger) Error(format string, args ...interface{}) {
	l.log("ERROR", colorRed, format, args...)
}

// PrintHeader prints the session header
func (l *TestLogger) PrintHeader(configPath, device string, phones []string, testCount int) {
	fmt.Fprintln(os.Stderr, strings.Repeat("=", 80))
	fmt.Fprintln(os.Stderr, "                        MODEM TEST SESSION")
	fmt.Fprintln(os.Stderr, strings.Repeat("=", 80))
	fmt.Fprintf(os.Stderr, "Config:    %s\n", configPath)
	fmt.Fprintf(os.Stderr, "Device:    %s\n", device)
	if len(phones) == 1 {
		fmt.Fprintf(os.Stderr, "Phone:     %s\n", phones[0])
	} else {
		fmt.Fprintf(os.Stderr, "Phones:    %s (circular)\n", strings.Join(phones, ", "))
	}
	fmt.Fprintf(os.Stderr, "Tests:     %d\n", testCount)
	fmt.Fprintf(os.Stderr, "Started:   %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintln(os.Stderr, strings.Repeat("=", 80))
	fmt.Fprintln(os.Stderr)
}

// PrintTestHeader prints a test section header
func (l *TestLogger) PrintTestHeader(testNum, totalTests int) {
	fmt.Fprintln(os.Stderr, strings.Repeat("=", 80))
	fmt.Fprintf(os.Stderr, "TEST %d/%d\n", testNum, totalTests)
	fmt.Fprintln(os.Stderr, strings.Repeat("=", 80))
}

// PhoneStats holds statistics for a single phone number
type PhoneStats struct {
	Phone         string
	Total         int
	Success       int
	Failed        int
	TotalDialTime time.Duration
	TotalEmsiTime time.Duration
}

// SuccessRate returns the success percentage
func (s *PhoneStats) SuccessRate() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Success) / float64(s.Total) * 100
}

// AvgDialTime returns average dial time for successful calls
func (s *PhoneStats) AvgDialTime() time.Duration {
	if s.Success == 0 {
		return 0
	}
	return s.TotalDialTime / time.Duration(s.Success)
}

// AvgEmsiTime returns average EMSI time for successful calls
func (s *PhoneStats) AvgEmsiTime() time.Duration {
	if s.Success == 0 {
		return 0
	}
	return s.TotalEmsiTime / time.Duration(s.Success)
}

// PrintSummary prints the final test summary
func (l *TestLogger) PrintSummary(total, success, failed int, totalDuration time.Duration, avgDialTime, avgEmsiTime time.Duration, results []string) {
	l.PrintSummaryWithPhoneStats(total, success, failed, totalDuration, avgDialTime, avgEmsiTime, results, nil)
}

// PrintSummaryWithPhoneStats prints the final test summary with per-phone statistics
func (l *TestLogger) PrintSummaryWithPhoneStats(total, success, failed int, totalDuration time.Duration, avgDialTime, avgEmsiTime time.Duration, results []string, phoneStats map[string]*PhoneStats) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, strings.Repeat("=", 80))
	fmt.Fprintln(os.Stderr, "                           SUMMARY")
	fmt.Fprintln(os.Stderr, strings.Repeat("=", 80))

	fmt.Fprintf(os.Stderr, "Total:     %d tests\n", total)

	successPct := 0.0
	failedPct := 0.0
	if total > 0 {
		successPct = float64(success) / float64(total) * 100
		failedPct = float64(failed) / float64(total) * 100
	}

	fmt.Fprintf(os.Stderr, "Success:   %d (%.1f%%)\n", success, successPct)
	fmt.Fprintf(os.Stderr, "Failed:    %d (%.1f%%)\n", failed, failedPct)
	fmt.Fprintf(os.Stderr, "Duration:  %s\n", formatDuration(totalDuration))
	fmt.Fprintln(os.Stderr)

	if success > 0 {
		fmt.Fprintf(os.Stderr, "Avg dial time:  %.1fs\n", avgDialTime.Seconds())
		fmt.Fprintf(os.Stderr, "Avg EMSI time:  %.1fs\n", avgEmsiTime.Seconds())
		fmt.Fprintln(os.Stderr)
	}

	// Print per-phone statistics if multiple phones were tested
	if len(phoneStats) > 1 {
		fmt.Fprintln(os.Stderr, "PER-PHONE STATISTICS:")
		fmt.Fprintf(os.Stderr, "  %-12s %6s %6s %6s %10s %10s\n", "PHONE", "TOTAL", "OK", "FAIL", "AVG DIAL", "AVG EMSI")
		fmt.Fprintf(os.Stderr, "  %s\n", strings.Repeat("-", 58))

		// Sort phones for consistent output
		phones := make([]string, 0, len(phoneStats))
		for phone := range phoneStats {
			phones = append(phones, phone)
		}
		sortStrings(phones)

		for _, phone := range phones {
			stats := phoneStats[phone]
			avgDial := "-"
			avgEmsi := "-"
			if stats.Success > 0 {
				avgDial = fmt.Sprintf("%.1fs", stats.AvgDialTime().Seconds())
				avgEmsi = fmt.Sprintf("%.1fs", stats.AvgEmsiTime().Seconds())
			}
			fmt.Fprintf(os.Stderr, "  %-12s %6d %6d %6d %10s %10s  (%.0f%%)\n",
				stats.Phone, stats.Total, stats.Success, stats.Failed, avgDial, avgEmsi, stats.SuccessRate())
		}
		fmt.Fprintln(os.Stderr)
	}

	fmt.Fprintln(os.Stderr, "RESULTS:")
	for _, r := range results {
		fmt.Fprintf(os.Stderr, "  %s\n", r)
	}
	fmt.Fprintln(os.Stderr, strings.Repeat("=", 80))
}

// sortStrings sorts a slice of strings in place (simple bubble sort)
func sortStrings(s []string) {
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

// PrintLineStats prints post-disconnect line statistics
func (l *TestLogger) PrintLineStats(stats string) {
	if strings.TrimSpace(stats) == "" {
		return
	}

	l.Stats("Post-disconnect line statistics:")
	lines := strings.Split(stats, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != "OK" {
			fmt.Fprintf(os.Stderr, "               %s\n", line)
		}
	}
}

// logHexDump outputs a hex dump of data
func (l *TestLogger) logHexDump(data []byte, direction string) {
	dump := hex.Dump(data)
	lines := strings.Split(dump, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			fmt.Fprintf(os.Stderr, "       %s %s\n", direction, line)
		}
	}
}

// formatBytes formats bytes for display, showing control characters escaped
func formatBytes(data []byte) string {
	return strconv.Quote(string(data))
}

// boolToOnOff converts bool to "1" or "0" for compact display
func boolToOnOff(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dh %dm %ds", hours, mins, secs)
}
