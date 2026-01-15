package modem

import (
	"regexp"
	"strings"
)

var (
	// phoneCleanRegex removes non-digit characters except leading +
	phoneCleanRegex = regexp.MustCompile(`[^\d+]`)
	// phoneDigitsOnly removes everything except digits
	phoneDigitsOnly = regexp.MustCompile(`[^\d]`)
)

// NormalizePhone converts a phone number to a standard format for prefix matching.
// Examples:
//   - "+49-555-1234567" -> "+495551234567"
//   - "49-555-1234567" -> "+495551234567"
//   - "1-800-555-1234" -> "+18005551234"
//   - "-Unpublished-" -> "" (invalid)
func NormalizePhone(phone string) string {
	if phone == "" {
		return ""
	}

	// Check for invalid phone markers
	phone = strings.TrimSpace(phone)
	lowerPhone := strings.ToLower(phone)
	if lowerPhone == "-unpublished-" || lowerPhone == "unpublished" ||
		lowerPhone == "-" || lowerPhone == "none" {
		return ""
	}

	// Remove dashes, spaces, parentheses, dots
	cleaned := phoneCleanRegex.ReplaceAllString(phone, "")

	// Ensure it starts with +
	if !strings.HasPrefix(cleaned, "+") {
		cleaned = "+" + cleaned
	}

	// Must have at least country code + some digits
	if len(cleaned) < 4 {
		return ""
	}

	return cleaned
}

// NormalizePrefix normalizes a prefix for matching.
// Examples:
//   - "+7" -> "+7"
//   - "+49" -> "+49"
//   - "+33-75" -> "+3375"
func NormalizePrefix(prefix string) string {
	if prefix == "" {
		return ""
	}

	// Remove dashes, spaces, keep + and digits
	cleaned := phoneCleanRegex.ReplaceAllString(prefix, "")

	// Ensure it starts with +
	if !strings.HasPrefix(cleaned, "+") {
		cleaned = "+" + cleaned
	}

	return cleaned
}

// ExtractCountryCode extracts the country code from a normalized phone number.
// This is a simplified extraction that handles common cases.
// Returns the country code with + prefix.
func ExtractCountryCode(phoneNorm string) string {
	if phoneNorm == "" || !strings.HasPrefix(phoneNorm, "+") {
		return ""
	}

	// Remove the leading +
	digits := phoneNorm[1:]
	if len(digits) == 0 {
		return ""
	}

	// Common country code lengths (simplified approach)
	// 1 digit: +1 (USA/Canada)
	// 2 digits: +49 (Germany), +44 (UK), +7 (Russia), etc.
	// 3 digits: +380 (Ukraine), +375 (Belarus), etc.

	// Check for NANP (North America) +1
	if digits[0] == '1' && len(digits) >= 1 {
		return "+1"
	}

	// Check for +7 (Russia/Kazakhstan)
	if digits[0] == '7' && len(digits) >= 1 {
		return "+7"
	}

	// For other cases, try common 2-digit codes first
	if len(digits) >= 2 {
		twoDigit := "+" + digits[:2]
		// Common 2-digit country codes
		switch twoDigit {
		case "+20", "+27", "+30", "+31", "+32", "+33", "+34", "+36", "+39",
			"+40", "+41", "+43", "+44", "+45", "+46", "+47", "+48", "+49",
			"+51", "+52", "+53", "+54", "+55", "+56", "+57", "+58",
			"+60", "+61", "+62", "+63", "+64", "+65", "+66",
			"+81", "+82", "+84", "+86",
			"+90", "+91", "+92", "+93", "+94", "+95", "+98":
			return twoDigit
		}
	}

	// Try 3-digit codes
	if len(digits) >= 3 {
		threeDigit := "+" + digits[:3]
		// Common 3-digit country codes
		switch threeDigit {
		case "+212", "+213", "+216", "+218",
			"+220", "+221", "+222", "+223", "+224", "+225", "+226", "+227", "+228", "+229",
			"+230", "+231", "+232", "+233", "+234", "+235", "+236", "+237", "+238", "+239",
			"+240", "+241", "+242", "+243", "+244", "+245", "+246", "+247", "+248", "+249",
			"+250", "+251", "+252", "+253", "+254", "+255", "+256", "+257", "+258",
			"+260", "+261", "+262", "+263", "+264", "+265", "+266", "+267", "+268", "+269",
			"+290", "+291", "+297", "+298", "+299",
			"+350", "+351", "+352", "+353", "+354", "+355", "+356", "+357", "+358", "+359",
			"+370", "+371", "+372", "+373", "+374", "+375", "+376", "+377", "+378", "+379",
			"+380", "+381", "+382", "+383", "+385", "+386", "+387", "+389",
			"+420", "+421", "+423":
			return threeDigit
		}
	}

	// Default: assume 2-digit country code
	if len(digits) >= 2 {
		return "+" + digits[:2]
	}

	return phoneNorm
}

// HasPhonePrefix checks if a normalized phone number starts with a given prefix.
func HasPhonePrefix(phoneNorm, prefix string) bool {
	if phoneNorm == "" || prefix == "" {
		return false
	}

	normalizedPrefix := NormalizePrefix(prefix)
	return strings.HasPrefix(phoneNorm, normalizedPrefix)
}

// IsValidPhone checks if a phone string appears to be a valid phone number.
func IsValidPhone(phone string) bool {
	normalized := NormalizePhone(phone)
	if normalized == "" {
		return false
	}

	// Must have at least 7 digits (minimum for most countries)
	digitCount := len(phoneDigitsOnly.ReplaceAllString(normalized, ""))
	return digitCount >= 7
}
