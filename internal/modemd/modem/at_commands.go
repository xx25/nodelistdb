// Package modem provides modem abstraction for FidoNet node testing via PSTN.
package modem

import (
	"strconv"
	"strings"
)

// Common AT commands
const (
	ATZ  = "ATZ"  // Reset to profile 0
	ATI  = "ATI"  // Modem identification
	ATI3 = "ATI3" // Detailed modem identification
	ATDT = "ATDT" // Dial tone
	ATDP = "ATDP" // Dial pulse
	ATD  = "ATD"  // Dial (mode depends on modem settings)
	ATH  = "ATH"  // Hangup
	ATH0 = "ATH0" // Hangup (explicit)
	ATO  = "ATO"  // Return to online mode
	ATE0 = "ATE0" // Echo off
	ATE1 = "ATE1" // Echo on
	ATQ0 = "ATQ0" // Result codes on
	ATV1 = "ATV1" // Verbose result codes
	ATX4 = "ATX4" // Extended result codes (BUSY, NO DIALTONE, etc.)
	ATS0 = "ATS0=0" // Auto-answer off
	ATLineStats = "AT&V1" // Line quality statistics
)

// Escape sequence for returning to command mode from data mode
const (
	EscapeSequence = "+++"
	EscapeGuardTime = 1000 // milliseconds
)

// Response codes from modem
const (
	ResponseOK         = "OK"
	ResponseConnect    = "CONNECT"
	ResponseRing       = "RING"
	ResponseNoCarrier  = "NO CARRIER"
	ResponseError      = "ERROR"
	ResponseNoDialtone = "NO DIALTONE"
	ResponseBusy       = "BUSY"
	ResponseNoAnswer   = "NO ANSWER"
	ResponseRinging    = "RINGING"
)

// ModemInfo contains modem identification from ATI response
type ModemInfo struct {
	Manufacturer string // e.g., "USRobotics", "ZyXEL", "Hayes"
	Model        string // e.g., "Courier", "U-1496"
	Firmware     string // Firmware version if available
	RawResponse  string // Full ATI response for debugging
}

// ConnectInfo contains parsed CONNECT string information
type ConnectInfo struct {
	Speed    int    // Line speed - min(TX,RX) if available, else first number
	SpeedTX  int    // Explicit TX speed (0 if not reported)
	SpeedRX  int    // Explicit RX speed (0 if not reported)
	Protocol string // Protocol info (e.g., "V34/LAPM/V42BIS")
}

// ParseConnectSpeed extracts speed and protocol from CONNECT response.
// Examples:
//   - "CONNECT 33600" -> 33600, "", nil
//   - "CONNECT 33600/ARQ/V34/LAPM" -> 33600, "ARQ/V34/LAPM", nil
//   - "CONNECT" -> 0, "", nil (some modems just say CONNECT)
//
// Deprecated: Use ParseConnectInfo for full TX/RX speed support.
func ParseConnectSpeed(response string) (speed int, protocol string, err error) {
	info := ParseConnectInfo(response)
	return info.Speed, info.Protocol, nil
}

// ParseConnectInfo extracts detailed connection info from CONNECT response.
// Handles various modem formats including explicit TX/RX speeds.
// Examples:
//   - "CONNECT 33600" -> Speed=33600
//   - "CONNECT 33600/ARQ/V34/LAPM" -> Speed=33600, Protocol="ARQ/V34/LAPM"
//   - "CONNECT 115200/V34/LAPM/V42B/31200:TX/28800:RX" -> Speed=28800, TX=31200, RX=28800
func ParseConnectInfo(response string) ConnectInfo {
	var info ConnectInfo

	// Find the CONNECT line
	lines := strings.Split(response, "\n")
	var connectLine string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "CONNECT") {
			connectLine = line
			break
		}
	}
	// Try \r as separator if not found
	if connectLine == "" {
		lines = strings.Split(response, "\r")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToUpper(line), "CONNECT") {
				connectLine = line
				break
			}
		}
	}

	if connectLine == "" {
		return info
	}

	// Remove "CONNECT" prefix
	upper := strings.ToUpper(connectLine)
	idx := strings.Index(upper, "CONNECT")
	if idx < 0 {
		return info
	}
	tail := strings.TrimSpace(connectLine[idx+len("CONNECT"):])

	// Split into tokens by / and whitespace
	tokens := strings.FieldsFunc(tail, func(r rune) bool {
		return r == '/' || r == ' ' || r == '\t'
	})

	// Pass 1: Look for explicit TX/RX speeds (e.g., "31200:TX" or "TX:31200")
	var protocolParts []string
	for _, t := range tokens {
		upperT := strings.ToUpper(t)

		if strings.Contains(t, ":") {
			parts := strings.SplitN(t, ":", 2)
			if len(parts) == 2 {
				// Format: "31200:TX" or "31200:RX"
				if isDigits(parts[0]) && (strings.ToUpper(parts[1]) == "TX" || strings.ToUpper(parts[1]) == "RX") {
					v, _ := strconv.Atoi(parts[0])
					if strings.ToUpper(parts[1]) == "TX" {
						info.SpeedTX = v
					} else {
						info.SpeedRX = v
					}
					continue
				}
				// Format: "TX:31200" or "RX:31200"
				if isDigits(parts[1]) && (strings.ToUpper(parts[0]) == "TX" || strings.ToUpper(parts[0]) == "RX") {
					v, _ := strconv.Atoi(parts[1])
					if strings.ToUpper(parts[0]) == "TX" {
						info.SpeedTX = v
					} else {
						info.SpeedRX = v
					}
					continue
				}
			}
		}

		// Not a TX/RX token - might be protocol info or speed
		if !strings.Contains(upperT, "TX") && !strings.Contains(upperT, "RX") {
			protocolParts = append(protocolParts, t)
		}
	}

	// Calculate line speed
	if info.SpeedTX > 0 && info.SpeedRX > 0 {
		// Use minimum of TX/RX (conservative)
		if info.SpeedTX < info.SpeedRX {
			info.Speed = info.SpeedTX
		} else {
			info.Speed = info.SpeedRX
		}
	} else if info.SpeedTX > 0 {
		info.Speed = info.SpeedTX
	} else if info.SpeedRX > 0 {
		info.Speed = info.SpeedRX
	} else {
		// Pass 2: First pure numeric token is the speed
		for _, t := range protocolParts {
			if isDigits(t) {
				info.Speed, _ = strconv.Atoi(t)
				break
			}
		}
	}

	// Build protocol string (exclude the first numeric token which is DTE speed)
	var protoTokens []string
	skippedFirst := false
	for _, t := range protocolParts {
		if !skippedFirst && isDigits(t) {
			skippedFirst = true
			continue
		}
		protoTokens = append(protoTokens, t)
	}
	info.Protocol = strings.Join(protoTokens, "/")

	return info
}

// isDigits returns true if s contains only digits
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ParseModemInfo extracts modem information from ATI response
func ParseModemInfo(response string) *ModemInfo {
	info := &ModemInfo{
		RawResponse: response,
	}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "OK" {
			continue
		}

		lineLower := strings.ToLower(line)

		// Try to identify manufacturer
		switch {
		case strings.Contains(lineLower, "usrobotics") || strings.Contains(lineLower, "usr"):
			info.Manufacturer = "USRobotics"
		case strings.Contains(lineLower, "zyxel"):
			info.Manufacturer = "ZyXEL"
		case strings.Contains(lineLower, "hayes"):
			info.Manufacturer = "Hayes"
		case strings.Contains(lineLower, "motorola"):
			info.Manufacturer = "Motorola"
		case strings.Contains(lineLower, "lucent"):
			info.Manufacturer = "Lucent"
		case strings.Contains(lineLower, "conexant"):
			info.Manufacturer = "Conexant"
		case strings.Contains(lineLower, "rockwell"):
			info.Manufacturer = "Rockwell"
		}

		// Look for model info
		if strings.Contains(lineLower, "courier") {
			info.Model = "Courier"
		} else if strings.Contains(lineLower, "sportster") {
			info.Model = "Sportster"
		} else if strings.Contains(lineLower, "u-1496") || strings.Contains(lineLower, "u1496") {
			info.Model = "U-1496"
		}

		// Look for version/firmware info
		if strings.Contains(lineLower, "version") || strings.Contains(lineLower, "ver") {
			if info.Firmware == "" {
				info.Firmware = line
			}
		}
	}

	// If we couldn't identify anything, use the first non-empty line as model
	if info.Model == "" && info.Manufacturer == "" {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && line != "OK" {
				info.Model = line
				break
			}
		}
	}

	return info
}

// IsSuccessResponse checks if response indicates success
func IsSuccessResponse(response string) bool {
	return strings.Contains(response, ResponseOK)
}

// IsConnectResponse checks if response indicates connection
func IsConnectResponse(response string) bool {
	return strings.Contains(response, ResponseConnect)
}

// IsFailureResponse checks if response indicates dial failure
func IsFailureResponse(response string) (bool, string) {
	failures := []string{
		ResponseNoCarrier,
		ResponseBusy,
		ResponseNoAnswer,
		ResponseNoDialtone,
		ResponseError,
	}

	for _, failure := range failures {
		if strings.Contains(response, failure) {
			return true, failure
		}
	}
	return false, ""
}

// FormatPhoneNumber cleans phone number for dialing.
// Removes all characters except digits, +, *, #, and , (pause).
func FormatPhoneNumber(phone string) string {
	var result strings.Builder
	for _, ch := range phone {
		switch {
		case ch >= '0' && ch <= '9':
			result.WriteRune(ch)
		case ch == '+' || ch == '*' || ch == '#' || ch == ',':
			result.WriteRune(ch)
		// Skip spaces, dashes, parentheses, etc.
		}
	}
	return result.String()
}
