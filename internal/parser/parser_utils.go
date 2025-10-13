package parser

import (
	"strings"
	"unicode/utf8"
)

// sanitizeUTF8 ensures string is valid UTF-8 for database storage.
// It replaces invalid UTF-8 sequences with the replacement character '?'.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}

	// Build a new string with valid UTF-8 characters only
	var result strings.Builder
	for _, r := range s {
		if r == utf8.RuneError {
			// Replace invalid sequences with replacement character
			result.WriteRune('?')
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// detectFormat analyzes line patterns to determine nodelist format.
// It identifies which era of FidoNet nodelist format is being used.
func detectFormat(line string, firstLine string) NodelistFormat {
	// Check for modern internet flags
	if strings.Contains(line, "IBN") || strings.Contains(line, "ITN") || strings.Contains(line, "INA:") {
		return Format2020
	}

	// Check for 2000s era flags
	if strings.Contains(line, "V34") || strings.Contains(line, "V90") || strings.Contains(line, "X75") {
		return Format2000
	}

	// Check for 1990s basic flags
	if strings.Contains(line, "XA") || strings.Contains(line, "CM") || strings.Contains(line, "MO") {
		return Format1990
	}

	// Check for 1986 colon format
	if strings.Contains(line, "XP:") || strings.Contains(line, "MO:") || strings.Contains(line, "CM:") {
		return Format1986
	}

	// Default to 1990s format
	return Format1990
}
