// Package modem provides modem abstraction for FidoNet node testing via PSTN.
package modem

import (
	"regexp"
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

// connectSpeedRegex matches CONNECT responses like "CONNECT 33600/ARQ/V34/LAPM"
var connectSpeedRegex = regexp.MustCompile(`CONNECT\s+(\d+)(?:/(.+))?`)

// ParseConnectSpeed extracts speed and protocol from CONNECT response.
// Examples:
//   - "CONNECT 33600" -> 33600, "", nil
//   - "CONNECT 33600/ARQ/V34/LAPM" -> 33600, "ARQ/V34/LAPM", nil
//   - "CONNECT" -> 0, "", nil (some modems just say CONNECT)
func ParseConnectSpeed(response string) (speed int, protocol string, err error) {
	// Find the CONNECT line
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, ResponseConnect) {
			matches := connectSpeedRegex.FindStringSubmatch(line)
			if matches != nil {
				if len(matches) > 1 && matches[1] != "" {
					speed, _ = strconv.Atoi(matches[1])
				}
				if len(matches) > 2 && matches[2] != "" {
					protocol = matches[2]
				}
				return speed, protocol, nil
			}
			// Just "CONNECT" without speed
			return 0, "", nil
		}
	}
	return 0, "", nil
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
