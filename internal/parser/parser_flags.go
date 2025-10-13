package parser

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// defaultPorts contains the standard port numbers for FidoNet internet protocols
var defaultPorts = map[string]int{
	"IBN": 24554, // BinkP
	"ITN": 23,    // Telnet
	"IFC": 60179, // EMSI over TCP
	"IFT": 21,    // FTP
}

// parseFlagsWithConfig extracts flags and builds structured internet configuration.
// It returns a slice of flag strings and a JSON-encoded InternetConfiguration.
func (p *Parser) parseFlagsWithConfig(flagsStr string) ([]string, []byte) {
	// Pre-allocate flags slice with typical capacity
	flags := make([]string, 0, 10)

	// Create new maps for each node to avoid cross-contamination
	protocols := make(map[string][]database.InternetProtocolDetail, 10)
	defaults := make(map[string]string, 5)
	emailProtocols := make(map[string][]database.EmailProtocolDetail, 3)
	infoFlags := make([]string, 0, 3)

	if flagsStr == "" {
		return flags, nil
	}

	// Parse comma-separated flags
	parts := strings.Split(flagsStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Handle flags with parameters (e.g., INA:hostname, U:time)
		if colonIndex := strings.Index(part, ":"); colonIndex > 0 {
			p.parseFlagWithValue(part, colonIndex, &flags, protocols, defaults, emailProtocols)
		} else {
			p.parseFlagWithoutValue(part, &flags, protocols, emailProtocols, &infoFlags)
		}
	}

	// Build JSON config
	internetConfig := p.buildInternetConfig(protocols, defaults, emailProtocols, infoFlags)

	return flags, internetConfig
}

// parseFlagWithValue handles flags that have a value after a colon (e.g., INA:hostname).
func (p *Parser) parseFlagWithValue(
	part string,
	colonIndex int,
	flags *[]string,
	protocols map[string][]database.InternetProtocolDetail,
	defaults map[string]string,
	emailProtocols map[string][]database.EmailProtocolDetail,
) {
	flagName := strings.TrimSpace(part[:colonIndex])
	flagValue := strings.TrimSpace(part[colonIndex+1:])

	switch flagName {
	// Connection protocols
	case "IBN", "IFC", "ITN", "IVM", "IFT":
		p.addProtocolDetail(flagName, flagValue, protocols)

	// Default internet address
	case "INA":
		if flagValue != "" {
			defaults["INA"] = flagValue
		}

	// Email protocols
	case "IEM":
		if flagValue != "" {
			defaults["IEM"] = flagValue // Default email
		}

	case "IMI", "ITX", "ISE":
		p.addEmailProtocol(flagName, flagValue, emailProtocols)

	// General IP flag
	case "IP":
		p.addProtocolDetail(flagName, flagValue, protocols)

	// User flags with values (U:time, T:zone)
	case "U", "T", "Tyz":
		*flags = append(*flags, part)

	default:
		// Unknown flag with parameter - store as-is
		*flags = append(*flags, part)
	}
}

// parseFlagWithoutValue handles flags without values (e.g., IBN, CM, MO).
func (p *Parser) parseFlagWithoutValue(
	part string,
	flags *[]string,
	protocols map[string][]database.InternetProtocolDetail,
	emailProtocols map[string][]database.EmailProtocolDetail,
	infoFlags *[]string,
) {
	switch part {
	// Connection protocol flags without values
	case "IBN", "IFC", "ITN", "IVM", "IFT", "INA", "IP":
		detail := database.InternetProtocolDetail{}
		if defaultPort, ok := defaultPorts[part]; ok {
			detail.Port = defaultPort
		}
		if protocols[part] == nil {
			protocols[part] = []database.InternetProtocolDetail{}
		}
		protocols[part] = append(protocols[part], detail)

	// Email protocol flags without values
	case "IMI", "ITX", "ISE", "IUC", "EMA", "EVY":
		if emailProtocols[part] == nil {
			emailProtocols[part] = []database.EmailProtocolDetail{}
		}
		emailProtocols[part] = append(emailProtocols[part], database.EmailProtocolDetail{})

	// Information flags
	case "INO4", "INO6", "ICM":
		*infoFlags = append(*infoFlags, part)

	// Alternative protocol names
	case "BND": // Alternative name for IBN
		if protocols["IBN"] == nil {
			protocols["IBN"] = []database.InternetProtocolDetail{}
		}
		protocols["IBN"] = append(protocols["IBN"], database.InternetProtocolDetail{Port: 24554})

	case "TEL": // Alternative name for ITN
		if protocols["ITN"] == nil {
			protocols["ITN"] = []database.InternetProtocolDetail{}
		}
		protocols["ITN"] = append(protocols["ITN"], database.InternetProtocolDetail{Port: 23})

	default:
		// Regular flag
		*flags = append(*flags, part)
	}
}

// addProtocolDetail adds a protocol detail to the protocols map.
func (p *Parser) addProtocolDetail(
	flagName string,
	flagValue string,
	protocols map[string][]database.InternetProtocolDetail,
) {
	detail := database.InternetProtocolDetail{}
	if flagValue != "" {
		addr, port := p.parseProtocolValue(flagValue)
		if addr != "" {
			detail.Address = addr
		}
		if port > 0 {
			detail.Port = port
		} else if defaultPort, ok := defaultPorts[flagName]; ok && addr != "" {
			detail.Port = defaultPort
		}
	}
	if detail.Address != "" || detail.Port > 0 {
		if protocols[flagName] == nil {
			protocols[flagName] = []database.InternetProtocolDetail{}
		}
		protocols[flagName] = append(protocols[flagName], detail)
	}
}

// addEmailProtocol adds an email protocol detail to the emailProtocols map.
func (p *Parser) addEmailProtocol(
	flagName string,
	flagValue string,
	emailProtocols map[string][]database.EmailProtocolDetail,
) {
	emailDetail := database.EmailProtocolDetail{}
	if flagValue != "" {
		emailDetail.Email = flagValue
	}
	if emailProtocols[flagName] == nil {
		emailProtocols[flagName] = []database.EmailProtocolDetail{}
	}
	emailProtocols[flagName] = append(emailProtocols[flagName], emailDetail)
}

// parseProtocolValue determines if a value is an address, port, or both.
// It returns the address (if present) and port number (if present).
func (p *Parser) parseProtocolValue(value string) (address string, port int) {
	// Check if it's a port number only
	if portNum, err := strconv.Atoi(value); err == nil && portNum > 0 && portNum < 65536 {
		return "", portNum
	}

	// Check for IPv6 address in brackets
	if strings.HasPrefix(value, "[") && strings.Contains(value, "]") {
		// IPv6 with optional port: [::1]:port
		bracketEnd := strings.Index(value, "]")
		address = value[:bracketEnd+1]
		if bracketEnd+1 < len(value) && value[bracketEnd+1] == ':' {
			if p, err := strconv.Atoi(value[bracketEnd+2:]); err == nil {
				port = p
			}
		}
		return address, port
	}

	// Check for standard address:port format
	if lastColon := strings.LastIndex(value, ":"); lastColon > 0 {
		// Make sure it's not part of IPv6 without brackets (IPv6 has multiple colons)
		if strings.Count(value[:lastColon], ":") <= 1 {
			// Standard host:port or IPv4:port
			possiblePort := value[lastColon+1:]
			if p, err := strconv.Atoi(possiblePort); err == nil && p > 0 && p < 65536 {
				return value[:lastColon], p
			}
		}
	}

	// It's just an address (hostname, IPv4, or unbracketed IPv6)
	return value, 0
}

// buildInternetConfig builds the JSON configuration from parsed flag data.
func (p *Parser) buildInternetConfig(
	protocols map[string][]database.InternetProtocolDetail,
	defaults map[string]string,
	emailProtocols map[string][]database.EmailProtocolDetail,
	infoFlags []string,
) []byte {
	// Build JSON config if we have any internet-related data
	if len(protocols) > 0 || len(defaults) > 0 || len(emailProtocols) > 0 || len(infoFlags) > 0 {
		config := database.InternetConfiguration{
			Protocols:      protocols,
			Defaults:       defaults,
			EmailProtocols: emailProtocols,
			InfoFlags:      infoFlags,
		}

		configJSON, err := json.Marshal(config)
		if err == nil {
			return configJSON
		}
	}

	return nil
}

// parseAdvancedFlags parses flags with full categorization for backward compatibility.
// This function is primarily kept for extracting modem flags.
func (p *Parser) parseAdvancedFlags(flagsStr string) ([]string, []string, []string, []int, []string, []string) {
	var allFlags []string
	var internetProtocols []string
	var internetHostnames []string
	var internetPorts []int
	var internetEmails []string
	var modemFlags []string

	if flagsStr == "" {
		return allFlags, internetProtocols, internetHostnames, internetPorts, internetEmails, modemFlags
	}

	// Parse comma-separated flags
	parts := strings.Split(flagsStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Categorize flag
		flagKey := strings.Split(part, ":")[0]
		if info, exists := p.ModernFlagMap[flagKey]; exists {
			switch info.Category {
			case "modem":
				modemFlags = append(modemFlags, part)
			case "internet":
				// Handle internet flags with values
				if colonIndex := strings.Index(part, ":"); colonIndex > 0 && info.HasValue {
					flagName := part[:colonIndex]
					value := part[colonIndex+1:]
					internetProtocols = append(internetProtocols, flagName)

					// Parse address/port
					if strings.Contains(value, ".") || strings.Contains(value, ":") {
						internetHostnames = append(internetHostnames, value)
					} else if port, err := strconv.Atoi(value); err == nil {
						internetPorts = append(internetPorts, port)
					}

					// Handle email addresses
					if flagName == "IEM" || flagName == "IMI" || flagName == "ITX" {
						internetEmails = append(internetEmails, value)
					}
				} else {
					internetProtocols = append(internetProtocols, part)
				}
			}
		}

		// Always add to all flags
		allFlags = append(allFlags, part)
	}

	return allFlags, internetProtocols, internetHostnames, internetPorts, internetEmails, modemFlags
}

// convertLegacyFlags converts 1986-style flags to modern equivalents.
func (p *Parser) convertLegacyFlags(flagsStr string) string {
	for old, new := range p.LegacyFlagMap {
		flagsStr = strings.ReplaceAll(flagsStr, old, new)
	}
	return flagsStr
}

// hasFlag checks if a specific flag exists in the flags slice (case-insensitive).
func (p *Parser) hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

// hasProtocol checks if a specific protocol exists in the protocols slice (case-insensitive).
func (p *Parser) hasProtocol(protocols []string, protocol string) bool {
	for _, p := range protocols {
		if strings.EqualFold(p, protocol) {
			return true
		}
	}
	return false
}
