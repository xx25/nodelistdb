// Package main provides formatted logging for modem testing.
package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// TestLogger provides formatted debug logging for modem testing
type TestLogger struct {
	config    LoggingConfig
	startTime time.Time
	output    io.Writer
}

// NewTestLogger creates a new test logger with the given configuration
func NewTestLogger(cfg LoggingConfig) *TestLogger {
	return &TestLogger{
		config:    cfg,
		startTime: time.Now(),
		output:    os.Stderr,
	}
}

// SetOutput sets the output writer (can be used with io.MultiWriter)
func (l *TestLogger) SetOutput(w io.Writer) {
	l.output = w
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
	fmt.Fprintf(l.output, "%s%s %s\n", prefix, cat, formattedMsg)
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

// EMSIDetails represents EMSI data for printing (to avoid import cycle)
type EMSIDetails struct {
	SystemName    string
	Location      string
	Sysop         string
	Addresses     []string
	MailerName    string
	MailerVersion string
	Protocols     []string
	Capabilities  []string
}

// PrintEMSIDetails prints detailed EMSI remote system information
func (l *TestLogger) PrintEMSIDetails(info *EMSIDetails) {
	if info == nil {
		return
	}

	// Print each field with EMSI category
	if len(info.Addresses) > 0 {
		l.log("EMSI", colorCyan, "Address:  %s", strings.Join(info.Addresses, ", "))
	}
	if info.SystemName != "" {
		l.log("EMSI", colorCyan, "System:   %s", info.SystemName)
	}
	if info.Location != "" {
		l.log("EMSI", colorCyan, "Location: %s", info.Location)
	}
	if info.Sysop != "" {
		l.log("EMSI", colorCyan, "Sysop:    %s", info.Sysop)
	}
	if info.MailerName != "" {
		mailer := info.MailerName
		if info.MailerVersion != "" {
			mailer += " " + info.MailerVersion
		}
		l.log("EMSI", colorCyan, "Mailer:   %s", mailer)
	}
	if len(info.Protocols) > 0 {
		l.log("EMSI", colorCyan, "Protocols: %s", strings.Join(info.Protocols, ", "))
	}
	if len(info.Capabilities) > 0 {
		l.log("EMSI", colorCyan, "Caps:     %s", strings.Join(info.Capabilities, ", "))
	}
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

// Warn logs warning messages
func (l *TestLogger) Warn(format string, args ...interface{}) {
	l.log("WARN", colorYellow, format, args...)
}

// PrintHeader prints the session header
func (l *TestLogger) PrintHeader(configPath, device string, phones []string, testCount int) {
	fmt.Fprintln(l.output, strings.Repeat("=", 80))
	fmt.Fprintln(l.output, "                        MODEM TEST SESSION")
	fmt.Fprintln(l.output, strings.Repeat("=", 80))
	fmt.Fprintf(l.output, "Config:    %s\n", configPath)
	fmt.Fprintf(l.output, "Device:    %s\n", device)
	if len(phones) == 1 {
		fmt.Fprintf(l.output, "Phone:     %s\n", phones[0])
	} else {
		fmt.Fprintf(l.output, "Phones:    %s (circular)\n", strings.Join(phones, ", "))
	}
	if testCount <= 0 {
		fmt.Fprintln(l.output, "Tests:     ∞ (infinite, Ctrl+C to stop)")
	} else {
		fmt.Fprintf(l.output, "Tests:     %d\n", testCount)
	}
	fmt.Fprintf(l.output, "Started:   %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintln(l.output, strings.Repeat("=", 80))
	fmt.Fprintln(os.Stderr)
}

// PrintTestHeader prints a test section header
func (l *TestLogger) PrintTestHeader(testNum, totalTests int) {
	fmt.Fprintln(l.output, strings.Repeat("=", 80))
	if totalTests <= 0 {
		fmt.Fprintf(l.output, "TEST %d\n", testNum)
	} else {
		fmt.Fprintf(l.output, "TEST %d/%d\n", testNum, totalTests)
	}
	fmt.Fprintln(l.output, strings.Repeat("=", 80))
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
	fmt.Fprintln(l.output, strings.Repeat("=", 80))
	fmt.Fprintln(l.output, "                           SUMMARY")
	fmt.Fprintln(l.output, strings.Repeat("=", 80))

	fmt.Fprintf(l.output, "Total:     %d tests\n", total)

	successPct := 0.0
	failedPct := 0.0
	if total > 0 {
		successPct = float64(success) / float64(total) * 100
		failedPct = float64(failed) / float64(total) * 100
	}

	fmt.Fprintf(l.output, "Success:   %d (%.1f%%)\n", success, successPct)
	fmt.Fprintf(l.output, "Failed:    %d (%.1f%%)\n", failed, failedPct)
	fmt.Fprintf(l.output, "Duration:  %s\n", formatDuration(totalDuration))
	fmt.Fprintln(os.Stderr)

	if success > 0 {
		fmt.Fprintf(l.output, "Avg dial time:  %.1fs\n", avgDialTime.Seconds())
		fmt.Fprintf(l.output, "Avg EMSI time:  %.1fs\n", avgEmsiTime.Seconds())
		fmt.Fprintln(os.Stderr)
	}

	// Print per-phone statistics if multiple phones were tested
	if len(phoneStats) > 1 {
		fmt.Fprintln(l.output, "PER-PHONE STATISTICS:")
		fmt.Fprintf(l.output, "  %-12s %6s %6s %6s %10s %10s\n", "PHONE", "TOTAL", "OK", "FAIL", "AVG DIAL", "AVG EMSI")
		fmt.Fprintf(l.output, "  %s\n", strings.Repeat("-", 58))

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
			fmt.Fprintf(l.output, "  %-12s %6d %6d %6d %10s %10s  (%.0f%%)\n",
				stats.Phone, stats.Total, stats.Success, stats.Failed, avgDial, avgEmsi, stats.SuccessRate())
		}
		fmt.Fprintln(os.Stderr)
	}

	fmt.Fprintln(l.output, "RESULTS:")
	for _, r := range results {
		fmt.Fprintf(l.output, "  %s\n", r)
	}
	fmt.Fprintln(l.output, strings.Repeat("=", 80))
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

// PrintLineStats prints post-disconnect line statistics (raw format)
func (l *TestLogger) PrintLineStats(stats string) {
	l.PrintLineStatsWithProfile(stats, "raw")
}

// PrintLineStatsWithProfile prints post-disconnect line statistics with optional parsing
func (l *TestLogger) PrintLineStatsWithProfile(raw string, profile string) {
	if strings.TrimSpace(raw) == "" {
		return
	}

	// Try to parse stats if profile is specified
	if profile != "" && profile != "raw" {
		parsed := ParseStats(raw, profile)
		if parsed != nil {
			l.printParsedStats(parsed)
			return
		}
	}

	// Fallback to raw output
	l.Stats("Post-disconnect line statistics:")
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != "OK" {
			fmt.Fprintf(l.output, "               %s\n", line)
		}
	}
}

// printParsedStats prints parsed line statistics in compact format
func (l *TestLogger) printParsedStats(stats *LineStats) {
	l.Stats("Line statistics:")

	// Speed info - compact format
	if stats.LastTXRate > 0 || stats.LastRXRate > 0 {
		txSpeed := formatSpeed(stats.LastTXRate)
		rxSpeed := formatSpeed(stats.LastRXRate)
		if stats.LastTXRate == stats.LastRXRate {
			l.log("STATS", colorGray, "Speed:    %s (TX=RX)", txSpeed)
		} else {
			l.log("STATS", colorGray, "Speed:    TX:%s RX:%s", txSpeed, rxSpeed)
		}
	}

	// Protocol and compression
	if stats.Protocol != "" {
		proto := stats.Protocol
		if stats.Compression != "" && stats.Compression != "None" {
			proto += "/" + stats.Compression
		}
		l.log("STATS", colorGray, "Protocol: %s", proto)
	}

	// Line quality - show on single line
	var quality []string
	if stats.LineQuality > 0 {
		quality = append(quality, fmt.Sprintf("Quality:%d", stats.LineQuality))
	}
	if stats.RxLevel > 0 {
		quality = append(quality, fmt.Sprintf("RxLevel:-%ddBm", stats.RxLevel))
	}
	if stats.EQMSum > 0 {
		quality = append(quality, fmt.Sprintf("EQM:%04X", stats.EQMSum))
	}
	if len(quality) > 0 {
		l.log("STATS", colorGray, "Line:     %s", strings.Join(quality, " "))
	}

	// Retrains (only if non-zero)
	if stats.LocalRetrain > 0 || stats.RemoteRetrain > 0 {
		l.log("STATS", colorGray, "Retrains: Local:%d Remote:%d", stats.LocalRetrain, stats.RemoteRetrain)
	}

	// Termination reason
	if stats.TerminationReason != "" {
		l.log("STATS", colorGray, "Result:   %s", stats.TerminationReason)
	}

	// Any warning messages (like "Flex fail")
	for _, msg := range stats.Messages {
		l.log("STATS", colorYellow, "Note:     %s", msg)
	}
}

// formatSpeed formats speed in human-readable format
func formatSpeed(bps int) string {
	if bps >= 1000 {
		return fmt.Sprintf("%.1fk", float64(bps)/1000)
	}
	return fmt.Sprintf("%d", bps)
}

// logHexDump outputs a hex dump of data
func (l *TestLogger) logHexDump(data []byte, direction string) {
	dump := hex.Dump(data)
	lines := strings.Split(dump, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			fmt.Fprintf(l.output, "       %s %s\n", direction, line)
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
