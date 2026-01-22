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
	Modulation  string // V.32bis, V.34, V.90, etc.

	// Line quality
	LineQuality   int     // 0-100, higher is better (some modems use 0-255)
	RxLevel       int     // Signal level in -dBm (as positive integer)
	TxPower       int     // Transmit power in -dBm (as positive integer)
	SNR           float64 // Signal-to-noise ratio in dB
	EQMSum        int     // Error Quality Metric (hex value)
	DigitalLoss   string
	RateDrops     int
	RoundTripDelay int    // Round trip delay in ms

	// Echo levels (ZyXEL)
	NearEndEcho float64 // Near end echo in dB
	FarEndEcho  float64 // Far end echo in dB

	// State info
	HighestRxState int
	HighestTxState int

	// Retrain counts
	LocalRetrain  int
	RemoteRetrain int

	// V.9x capabilities (ZyXEL)
	LocalV9xCapability  string // e.g., "V.92 / APCM"
	RemoteV9xCapability string // e.g., "None"
	ConnectionType      string // "Analogue" or "Digital"

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
	case "multitech":
		return parseMultiTechStats(raw)
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

// parseZyXELStats parses ZyXEL modem statistics from multiple ATI commands
// Supported commands:
//   - ATI2:  Link Status Report (connection stats, speed/protocol, disconnect reason)
//   - ATI12: Physical Layer Status Report (modulation, SNR, Rx level, Tx power, echo)
//   - ATI13: Graphical EQ display (skipped - not useful for stats)
//   - ATI14: V.9x Capability Report (V.92/APCM capabilities)
//   - ATI15: V.9x Power Management Report (power levels)
func parseZyXELStats(raw string) *LineStats {
	stats := &LineStats{
		RawFields: make(map[string]string),
	}

	// Track which section we're in based on headers
	var currentSection string
	lines := strings.Split(raw, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and standard responses
		if line == "" || line == "OK" {
			continue
		}
		if strings.HasPrefix(line, "ATI") || line == "ATI2" || line == "ATI12" || line == "ATI13" || line == "ATI14" || line == "ATI15" {
			continue
		}

		// Detect section headers
		if strings.Contains(line, "LINK STATUS REPORT") {
			currentSection = "ATI2"
			continue
		}
		if strings.Contains(line, "PHYSICAL LAYER STATUS REPORT") {
			currentSection = "ATI12"
			continue
		}
		if strings.Contains(line, "V.9x CAPABILITY REPORT") || strings.Contains(line, "V.9X CAPABILITY REPORT") {
			currentSection = "ATI14"
			continue
		}
		if strings.Contains(line, "V.9x POWER MANAGEMENT REPORT") || strings.Contains(line, "V.9X POWER MANAGEMENT REPORT") {
			currentSection = "ATI15"
			continue
		}

		// Skip ATI13 graphical EQ section (detect by pattern)
		if strings.HasPrefix(line, "-") && strings.Contains(line, "|") && strings.Contains(line, "=") {
			currentSection = "ATI13"
			continue
		}
		if currentSection == "ATI13" {
			// Skip all ATI13 content until next section
			if strings.Contains(line, "REPORT") {
				currentSection = ""
			}
			continue
		}

		// Parse based on current section
		switch currentSection {
		case "ATI2":
			parseZyXELATI2Line(stats, line)
		case "ATI12":
			parseZyXELATI12Line(stats, line)
		case "ATI14":
			parseZyXELATI14Line(stats, line)
		case "ATI15":
			parseZyXELATI15Line(stats, line)
		default:
			// Try to auto-detect format
			if strings.Contains(line, "=") {
				parseZyXELEqualsLine(stats, line)
			} else if strings.Contains(line, ":") && !strings.Contains(line, "Speed/Protocol") {
				parseZyXELColonLine(stats, line)
			} else {
				parseZyXELATI2Line(stats, line)
			}
		}
	}

	return stats
}

// parseZyXELATI2Line parses ATI2 Link Status Report lines
// Format: "Key1    Value1    Key2    Value2" (space-aligned two-column)
func parseZyXELATI2Line(stats *LineStats, line string) {
	// Check for "Last Speed/Protocol" line first (special format)
	if strings.HasPrefix(line, "Last Speed/Protocol") {
		parts := regexp.MustCompile(`\s{2,}`).Split(line, 2)
		if len(parts) == 2 {
			value := strings.TrimSpace(parts[1])
			stats.RawFields["Last Speed/Protocol"] = value
			parseZyXELSpeedProtocol(stats, value)
		}
		return
	}

	// Check for "Disconnect Reason" line
	if strings.HasPrefix(line, "Disconnect Reason") {
		parts := regexp.MustCompile(`\s{2,}`).Split(line, 2)
		if len(parts) == 2 {
			stats.TerminationReason = strings.TrimSpace(parts[1])
			stats.RawFields["Disconnect Reason"] = stats.TerminationReason
		}
		return
	}

	// Parse two-column format
	parseZyXELTwoColumnLine(stats, line)
}

// parseZyXELATI12Line parses ATI12 Physical Layer Status Report lines
// Format: "Tx Bit Rate       =    14400 bps      Rx Bit Rate           =    14400 bps"
func parseZyXELATI12Line(stats *LineStats, line string) {
	parseZyXELEqualsLine(stats, line)
}

// parseZyXELEqualsLine parses lines with "Key = Value" format (ATI12, ATI14)
// Can have two key-value pairs per line
// Examples:
//   "Modulation mode   =     V32b"
//   "Tx Bit Rate       =    14400 bps      Rx Bit Rate           =    14400 bps"
//   "SNR               =    71.27 dB       Round Trip Delay      =   234.00 ms"
//   "Local  Modem V.9x Capability  =  V.92 / APCM"
//   "SNR (dB)          =    71.27"
//   "Tx/Rx Bit Rate    =    14400 bps"
func parseZyXELEqualsLine(stats *LineStats, line string) {
	// Use regex to find all "Key = Value" patterns
	// Pattern matches: key (letters/numbers/spaces/periods/punctuation), =, spaces, value
	// Key can contain periods (e.g., "V.9x"), numbers, spaces, parentheses, slashes, hyphens
	pattern := regexp.MustCompile(`([A-Za-z][A-Za-z0-9.()/\- ]*?)\s*=\s*([^\s=][^=]*?)(?:\s{2,}|$)`)

	matches := pattern.FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			key := strings.TrimSpace(match[1])
			value := strings.TrimSpace(match[2])

			if key == "" {
				continue
			}

			// Store raw value
			stats.RawFields[key] = value

			// Map to LineStats fields
			parseZyXELPhysicalField(stats, key, value)
		}
	}
}

// parseZyXELPhysicalField maps ATI12 physical layer fields to LineStats
func parseZyXELPhysicalField(stats *LineStats, key, value string) {
	keyLower := strings.ToLower(key)

	switch {
	case strings.Contains(keyLower, "modulation mode"):
		stats.Modulation = value
		if stats.Protocol == "" {
			stats.Protocol = value
		}

	case strings.Contains(keyLower, "tx bit rate"):
		stats.LastTXRate = parseZyXELRate(value)

	case strings.Contains(keyLower, "rx bit rate"):
		stats.LastRXRate = parseZyXELRate(value)

	case strings.Contains(keyLower, "tx baud rate"):
		// Store in raw, but prefer bit rate
		if stats.HighestTXRate == 0 {
			stats.HighestTXRate = parseZyXELRate(value)
		}

	case strings.Contains(keyLower, "rx baud rate"):
		if stats.HighestRXRate == 0 {
			stats.HighestRXRate = parseZyXELRate(value)
		}

	case strings.Contains(keyLower, "tx power"):
		stats.TxPower = parseZyXELdBm(value)

	case strings.Contains(keyLower, "rx level"):
		stats.RxLevel = parseZyXELdBm(value)

	case keyLower == "snr":
		stats.SNR = parseZyXELFloat(value)

	case strings.Contains(keyLower, "round trip delay"):
		stats.RoundTripDelay = parseZyXELDelay(value)

	case strings.Contains(keyLower, "near end echo"):
		stats.NearEndEcho = parseZyXELFloat(value)

	case strings.Contains(keyLower, "far end echo"):
		stats.FarEndEcho = parseZyXELFloat(value)

	// ATI14 V.9x capability fields
	case strings.Contains(keyLower, "local") && strings.Contains(keyLower, "v.9") && strings.Contains(keyLower, "capability"):
		stats.LocalV9xCapability = value

	case strings.Contains(keyLower, "remote") && strings.Contains(keyLower, "v.9") && strings.Contains(keyLower, "capability"):
		stats.RemoteV9xCapability = value

	case strings.Contains(keyLower, "pstn connection"):
		if strings.Contains(keyLower, "local") {
			stats.ConnectionType = value
		}

	case strings.Contains(keyLower, "probing report"):
		// Contains modulation info like "V.34 / 2400"
		if stats.Modulation == "" && strings.Contains(keyLower, "local") {
			stats.Modulation = value
		}
	}
}

// parseZyXELATI14Line parses ATI14 V.9x Capability Report lines
func parseZyXELATI14Line(stats *LineStats, line string) {
	parseZyXELEqualsLine(stats, line)
}

// parseZyXELATI15Line parses ATI15 V.9x Power Management Report lines
// Format: "DPCM nominal tx power for phase 2  :     -0.00  dBm"
func parseZyXELATI15Line(stats *LineStats, line string) {
	parseZyXELColonLine(stats, line)
}

// parseZyXELColonLine parses lines with "Key : Value" format (ATI15)
func parseZyXELColonLine(stats *LineStats, line string) {
	colonIdx := strings.Index(line, ":")
	if colonIdx == -1 {
		return
	}

	key := strings.TrimSpace(line[:colonIdx])
	value := strings.TrimSpace(line[colonIdx+1:])

	if key == "" {
		return
	}

	stats.RawFields[key] = value

	// Map ATI15 power management fields
	keyLower := strings.ToLower(key)
	switch {
	case strings.Contains(keyLower, "digital pad loss"):
		stats.DigitalLoss = value
	case strings.Contains(keyLower, "dpcm") && strings.Contains(keyLower, "tx power"):
		// Could store additional power info if needed
	}
}

// parseZyXELSpeedProtocol parses the "Last Speed/Protocol" value
// Format: "T16800/R19200/ARQ/V.34/V42b" or "T33600/R33600/ARQ/V34/V42bis"
// Also handles: "T16800 bps/R19200 bps/..." with suffixes
func parseZyXELSpeedProtocol(stats *LineStats, value string) {
	parts := strings.Split(value, "/")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		partLower := strings.ToLower(part)

		// Check for TX rate first (T followed by digits, possibly with suffix)
		if strings.HasPrefix(part, "T") || strings.HasPrefix(part, "t") {
			rate := extractLeadingDigits(part[1:])
			if rate > 0 {
				stats.LastTXRate = rate
				continue
			}
		}

		// Check for RX rate (R followed by digits, possibly with suffix)
		if strings.HasPrefix(part, "R") || strings.HasPrefix(part, "r") {
			rate := extractLeadingDigits(part[1:])
			if rate > 0 {
				stats.LastRXRate = rate
				continue
			}
		}

		// Check for V.42bis compression variants (must check before protocol)
		if strings.Contains(partLower, "v42") {
			stats.Compression = part
			continue
		}

		// Check for MNP compression
		if strings.HasPrefix(partLower, "mnp") {
			stats.Compression = part
			continue
		}

		// Check for ARQ (error correction)
		if part == "ARQ" || part == "LAPM" {
			// Store as part of protocol info if we don't have one yet
			if stats.Protocol == "" {
				stats.Protocol = part
			}
			continue
		}

		// Check for V.xx modulation protocol (V.32, V.34, V.90, etc)
		if strings.HasPrefix(partLower, "v") && !strings.Contains(partLower, "v42") {
			// This is the modulation protocol - also set Modulation field
			stats.Protocol = part
			if stats.Modulation == "" {
				stats.Modulation = part
			}
			continue
		}
	}
}

// extractLeadingDigits extracts leading digits from a string like "16800 bps" -> 16800
func extractLeadingDigits(s string) int {
	s = strings.TrimSpace(s)
	var digits strings.Builder
	for _, c := range s {
		if c >= '0' && c <= '9' {
			digits.WriteRune(c)
		} else {
			break
		}
	}
	if digits.Len() == 0 {
		return 0
	}
	n, _ := strconv.Atoi(digits.String())
	return n
}

// parseZyXELTwoColumnLine parses a line with two key-value pairs
// Format: "Chars Sent           339     Chars Received         744"
func parseZyXELTwoColumnLine(stats *LineStats, line string) {
	// Split by multiple spaces
	parts := regexp.MustCompile(`\s{2,}`).Split(line, -1)

	// Clean up parts
	var cleaned []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}

	// Process key-value pairs
	// Patterns can be: "Key Value Key Value" or "Key Value"
	// Values are typically numeric
	i := 0
	for i < len(cleaned) {
		// Find key (text) and value (typically next item if numeric)
		key := cleaned[i]

		// Check if the next item looks like a value (numeric or special)
		if i+1 < len(cleaned) {
			nextItem := cleaned[i+1]
			if looksLikeValue(nextItem) {
				// Found a key-value pair
				stats.RawFields[key] = nextItem
				parseZyXELField(stats, key, nextItem)
				i += 2
				continue
			}
		}
		// Skip unmatched items
		i++
	}
}

// looksLikeValue checks if a string looks like a value (number, float, special text)
func looksLikeValue(s string) bool {
	// Integer
	if _, err := strconv.Atoi(s); err == nil {
		return true
	}
	// Float (possibly negative)
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	// Check if starts with digit or minus sign (for values with units like "71.27", "-36.10")
	if len(s) > 0 && (s[0] >= '0' && s[0] <= '9' || s[0] == '-') {
		return true
	}
	// Looks like time/delay value (case-insensitive)
	sLower := strings.ToLower(s)
	if strings.HasSuffix(sLower, "ms") || strings.HasSuffix(sLower, "s") {
		return true
	}
	// Looks like dBm or dB value
	if strings.HasSuffix(sLower, "dbm") || strings.HasSuffix(sLower, "db") {
		return true
	}
	// Looks like Hz value
	if strings.HasSuffix(sLower, "hz") {
		return true
	}
	// Looks like bps/baud value
	if strings.HasSuffix(sLower, "bps") || strings.HasSuffix(sLower, "baud") {
		return true
	}
	// Special ZyXEL values
	if s == "0" || s == "---" || s == "N/A" || s == "OFF" || s == "ON" {
		return true
	}
	return false
}

// parseZyXELField maps ZyXEL ATI2 fields to LineStats
func parseZyXELField(stats *LineStats, key, value string) {
	keyLower := strings.ToLower(key)

	switch {
	case strings.Contains(keyLower, "round trip delay"):
		// Store as-is, no direct mapping
	case strings.Contains(keyLower, "retrains requested"):
		stats.LocalRetrain = parseInt(value)
	case strings.Contains(keyLower, "retrains granted"):
		stats.RemoteRetrain = parseInt(value)
	case strings.Contains(keyLower, "fcs errors"):
		// FCS errors indicate line quality issues
		errors := parseInt(value)
		if errors > 0 {
			stats.Messages = append(stats.Messages, "FCS Errors: "+value)
		}
	case strings.Contains(keyLower, "blocks resent"):
		// Store in raw fields, indicates retransmissions
		if parseInt(value) > 0 {
			stats.Messages = append(stats.Messages, "Blocks Resent: "+value)
		}
	}
}

// parseMultiTechStats parses Multi-Tech modem ATI11 statistics
// Format is two-column with space alignment:
//
//	Description                         Status
//	---------------                     ------------
//	Last Connection                     V34
//	Initial Transmit Carrier Rate       33600
//
// Handles multi-page output (pages separated by "Press any key to continue")
func parseMultiTechStats(raw string) *LineStats {
	stats := &LineStats{
		RawFields: make(map[string]string),
	}

	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines, headers, separators, and prompts
		if line == "" || line == "OK" {
			continue
		}
		if strings.HasPrefix(line, "Description") || strings.HasPrefix(line, "---") {
			continue
		}
		if strings.Contains(strings.ToLower(line), "press any key") {
			continue
		}
		if strings.HasPrefix(line, "AT") {
			continue
		}

		// Parse "Key                    Value" format
		// Find where the value starts (look for multiple spaces)
		key, value := parseMultiTechLine(line)
		if key == "" {
			continue
		}

		// Store raw value
		stats.RawFields[key] = value

		// Map to LineStats fields
		keyUpper := strings.ToUpper(key)
		switch {
		case keyUpper == "LAST CONNECTION":
			stats.Protocol = value

		case strings.Contains(keyUpper, "INITIAL TRANSMIT CARRIER RATE"):
			stats.HighestTXRate = parseInt(value)

		case strings.Contains(keyUpper, "INITIAL RECEIVE CARRIER RATE"):
			stats.HighestRXRate = parseInt(value)

		case strings.Contains(keyUpper, "FINAL TRANSMIT CARRIER RATE"):
			stats.LastTXRate = parseInt(value)

		case strings.Contains(keyUpper, "FINAL RECEIVE CARRIER RATE"):
			stats.LastRXRate = parseInt(value)

		case strings.Contains(keyUpper, "PROTOCOL NEGOTIATION"):
			stats.Protocol = value

		case strings.Contains(keyUpper, "DATA COMPRESSION"):
			stats.Compression = value

		case strings.Contains(keyUpper, "ESTIMATED NOISE LEVEL"):
			stats.LineQuality = parseInt(value)

		case strings.Contains(keyUpper, "RECEIVE SIGNAL POWER LEVEL"):
			stats.RxLevel = parseInt(value)

		case strings.Contains(keyUpper, "ROUND TRIP DELAY"):
			// Store in RawFields, no direct mapping

		case strings.Contains(keyUpper, "RETRAIN BY LOCAL"):
			stats.LocalRetrain = parseInt(value)

		case strings.Contains(keyUpper, "RETRAIN BY REMOTE"):
			stats.RemoteRetrain = parseInt(value)

		case strings.Contains(keyUpper, "CALL TERMINATION CAUSE"):
			stats.TerminationReason = value

		case strings.Contains(keyUpper, "DIGITAL LOSS"):
			stats.DigitalLoss = value
		}
	}

	return stats
}

// parseMultiTechLine parses a Multi-Tech stats line with space-aligned columns
// Returns key and value, handling embedded units like "(-dBm)" in the key
func parseMultiTechLine(line string) (string, string) {
	// Multi-Tech format: key is left-aligned, value is right portion after spaces
	// Example: "Initial Transmit Carrier Rate       33600"
	// Example: "Receive  Signal Power Level  (-dBm) 14"

	// Find runs of 2+ spaces to separate key from value
	// The value typically starts around column 36-40
	parts := regexp.MustCompile(`\s{2,}`).Split(line, -1)
	if len(parts) < 2 {
		return "", ""
	}

	// Key is the first part, value is the last non-empty part
	key := strings.TrimSpace(parts[0])
	value := ""
	for i := len(parts) - 1; i >= 1; i-- {
		v := strings.TrimSpace(parts[i])
		if v != "" {
			value = v
			break
		}
	}

	// If the key contains units like "(-dBm)", include middle parts in key
	if len(parts) > 2 {
		// Rebuild key from all parts except the last value
		var keyParts []string
		for i := 0; i < len(parts)-1; i++ {
			p := strings.TrimSpace(parts[i])
			if p != "" {
				keyParts = append(keyParts, p)
			}
		}
		key = strings.Join(keyParts, " ")
	}

	return key, value
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

// parseZyXELRate extracts rate from ZyXEL format "14400 bps" or "2400 Baud"
// Case-insensitive suffix matching
func parseZyXELRate(s string) int {
	s = strings.TrimSpace(s)
	sLower := strings.ToLower(s)
	// Remove common suffixes (case-insensitive)
	for _, suffix := range []string{" bps", "bps", " baud", "baud", " hz", "hz"} {
		if strings.HasSuffix(sLower, suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}
	s = strings.TrimSpace(s)
	n, _ := strconv.Atoi(s)
	return n
}

// parseZyXELdBm extracts dBm value from ZyXEL format "-36.10 dBm"
// Returns positive integer (absolute value), case-insensitive
func parseZyXELdBm(s string) int {
	s = strings.TrimSpace(s)
	sLower := strings.ToLower(s)
	// Remove dBm suffix (case-insensitive)
	for _, suffix := range []string{" dbm", "dbm", " db", "db"} {
		if strings.HasSuffix(sLower, suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}
	s = strings.TrimSpace(s)
	// Parse as float and take absolute value
	f, _ := strconv.ParseFloat(s, 64)
	if f < 0 {
		f = -f
	}
	return int(f)
}

// parseZyXELFloat extracts float value from ZyXEL format "71.27 dB"
// Case-insensitive suffix matching
func parseZyXELFloat(s string) float64 {
	s = strings.TrimSpace(s)
	sLower := strings.ToLower(s)
	// Remove common suffixes (case-insensitive)
	for _, suffix := range []string{" db", "db", " degree", "degree", " hz", "hz", " ms", "ms"} {
		if strings.HasSuffix(sLower, suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}
	s = strings.TrimSpace(s)
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// parseZyXELDelay extracts delay in ms from "234.00 ms"
// Case-insensitive suffix matching
func parseZyXELDelay(s string) int {
	s = strings.TrimSpace(s)
	sLower := strings.ToLower(s)
	// Remove ms suffix (case-insensitive)
	for _, suffix := range []string{" ms", "ms", " s", "s"} {
		if strings.HasSuffix(sLower, suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}
	s = strings.TrimSpace(s)
	f, _ := strconv.ParseFloat(s, 64)
	return int(f)
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
