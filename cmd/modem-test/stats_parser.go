// Package main provides modem line statistics parsing.
package main

import (
	"regexp"
	"strconv"
	"strings"
)

// LineStats contains parsed modem line statistics
type LineStats struct {
	// Connection result
	TerminationReason string

	// Speed statistics
	LastTXRate    int
	HighestTXRate int
	LastRXRate    int
	HighestRXRate int

	// Protocol info
	Protocol    string
	Compression string

	// Line quality
	LineQuality   int  // 0-100, higher is better (some modems use 0-255)
	RxLevel       int  // Signal level in -dBm
	EQMSum        int  // Error Quality Metric (hex value)
	DigitalLoss   string
	RateDrops     int

	// State info
	HighestRxState int
	HighestTxState int

	// Retrain counts
	LocalRetrain  int
	RemoteRetrain int

	// Raw fields for unknown/extra data
	RawFields map[string]string

	// Any error or warning messages
	Messages []string
}

// ParseStats parses modem statistics based on profile
func ParseStats(raw string, profile string) *LineStats {
	switch strings.ToLower(profile) {
	case "rockwell", "usr", "conexant":
		return parseRockwellStats(raw)
	case "zyxel":
		return parseZyXELStats(raw)
	default:
		return nil // raw mode - no parsing
	}
}

// parseRockwellStats parses Rockwell/USR/Conexant chipset statistics
// Format: "KEY................ VALUE"
func parseRockwellStats(raw string) *LineStats {
	stats := &LineStats{
		RawFields: make(map[string]string),
	}

	// Pattern: Key followed by dots and value
	// e.g., "LAST TX rate................ 24000 BPS"
	linePattern := regexp.MustCompile(`^([A-Za-z][A-Za-z0-9 ]+?)\.{2,}\s*(.+)$`)

	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "OK" {
			continue
		}

		matches := linePattern.FindStringSubmatch(line)
		if matches == nil {
			// Check for standalone messages like "Flex fail"
			if line != "" && !strings.HasPrefix(line, "AT") {
				stats.Messages = append(stats.Messages, line)
			}
			continue
		}

		key := strings.TrimSpace(strings.ToUpper(matches[1]))
		value := strings.TrimSpace(matches[2])

		// Store raw value
		stats.RawFields[key] = value

		// Parse known fields
		switch {
		case strings.Contains(key, "TERMINATION"):
			stats.TerminationReason = value

		case key == "LAST TX RATE":
			stats.LastTXRate = parseSpeed(value)

		case key == "HIGHEST TX RATE":
			stats.HighestTXRate = parseSpeed(value)

		case key == "LAST RX RATE":
			stats.LastRXRate = parseSpeed(value)

		case key == "HIGHEST RX RATE":
			stats.HighestRXRate = parseSpeed(value)

		case key == "PROTOCOL":
			stats.Protocol = value

		case key == "COMPRESSION":
			stats.Compression = value

		case strings.Contains(key, "LINE QUALITY"):
			stats.LineQuality = parseInt(value)

		case strings.Contains(key, "RX LEVEL"):
			stats.RxLevel = parseInt(value)

		case strings.Contains(key, "EQM SUM"):
			stats.EQMSum = parseHex(value)

		case strings.Contains(key, "DIGITAL LOSS"):
			stats.DigitalLoss = value

		case strings.Contains(key, "RATE DROP"):
			if value != "FF" && value != "None" {
				stats.RateDrops = parseInt(value)
			}

		case strings.Contains(key, "HIGHEST RX STATE"):
			stats.HighestRxState = parseInt(value)

		case strings.Contains(key, "HIGHEST TX STATE"):
			stats.HighestTxState = parseInt(value)

		case strings.Contains(key, "LOCAL") && strings.Contains(key, "RTRN"):
			stats.LocalRetrain = parseHex(value)

		case strings.Contains(key, "REMOTE") && strings.Contains(key, "RTRN"):
			stats.RemoteRetrain = parseHex(value)
		}
	}

	return stats
}

// parseZyXELStats parses ZyXEL modem statistics (ATI11 format)
func parseZyXELStats(raw string) *LineStats {
	// TODO: Implement ZyXEL-specific parsing
	// For now, use Rockwell parser as base
	return parseRockwellStats(raw)
}

// parseSpeed extracts numeric speed from string like "24000 BPS"
func parseSpeed(s string) int {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, " BPS")
	s = strings.TrimSuffix(s, " bps")
	s = strings.TrimSuffix(s, "BPS")
	s = strings.TrimSuffix(s, "bps")
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// parseInt parses an integer, returning 0 on error
func parseInt(s string) int {
	s = strings.TrimSpace(s)
	n, _ := strconv.Atoi(s)
	return n
}

// parseHex parses a hexadecimal value
func parseHex(s string) int {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	n, _ := strconv.ParseInt(s, 16, 32)
	return int(n)
}
