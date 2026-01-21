package emsi

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// EMSI protocol constants
const (
	EMSI_INQ = "**EMSI_INQC816"
	EMSI_REQ = "**EMSI_REQA77E"
	EMSI_ACK = "**EMSI_ACKA490"
	EMSI_NAK = "**EMSI_NAKEEC3"
	EMSI_CLI = "**EMSI_CLIFA8C"
	EMSI_HBT = "**EMSI_HBTEAEE"
	EMSI_DAT = "**EMSI_DAT"
)

// Link codes (FSC-0056.001 + FSC-0088.001)
const (
	// Base pickup/hold codes (mutually exclusive)
	LinkCodePUA = "PUA" // Pickup All addresses
	LinkCodePUP = "PUP" // Pickup Primary only
	LinkCodeNPU = "NPU" // No Pickup desired
	LinkCodeHAT = "HAT" // Hold All Traffic

	// Pickup qualifiers (FSC-0088.001)
	LinkQualPMO = "PMO" // Pickup Mail Only (ARCmail/Packets)
	LinkQualNFE = "NFE" // No Files/attaches (no TICs)
	LinkQualNXP = "NXP" // No compressed mail pickup
	LinkQualNRQ = "NRQ" // No file Requests accepted

	// Hold qualifiers (FSC-0088.001)
	LinkQualHNM = "HNM" // Hold Non-Mail (hold except mail)
	LinkQualHXT = "HXT" // Hold compressed mail (eXtended Traffic)
	LinkQualHFE = "HFE" // Hold Files/attaches (TICs)
	LinkQualHRQ = "HRQ" // Hold file Requests

	// Session options
	LinkQualFNC = "FNC" // Filename Conversion (8.3 format)
	LinkQualRMA = "RMA" // Multiple file requests capable
	LinkQualRH1 = "RH1" // Hydra batch separation
)

// Compatibility codes (FSC-0088.001)
const (
	CompatEII = "EII" // EMSI-II capable
	CompatDFB = "DFB" // Fall-back to FTS1/WAZOO
	CompatFRQ = "FRQ" // File Request on outbound
	CompatNRQ = "NRQ" // No Request capability

	// Deprecated (don't send in EMSI-II mode)
	CompatARC = "ARC" // ARCmail
	CompatXMA = "XMA" // Extended Mail
)

// EMSIData represents the EMSI handshake data
type EMSIData struct {
	// System information
	SystemName    string
	Location      string
	Sysop         string
	Phone         string
	Speed         string
	Flags         string

	// Mailer information
	MailerName    string
	MailerVersion string
	MailerSerial  string

	// Addresses
	Addresses []string

	// Capabilities
	Protocols    []string // ZMO, ZAP, DZA, JAN, HYD
	Capabilities []string // Additional capabilities

	// Password (for authentication)
	Password string

	// EMSI-II Fields (FSC-0088.001)
	EMSI2Mode      bool              // Both sides presented EII (set during negotiation)
	LinkCode       string            // Parsed base link code (PUA, PUP, NPU, HAT)
	LinkQualifiers []string          // Parsed link qualifiers (PMO, NFE, etc.)
	PerAKAFlags    map[int][]string  // Per-address flags parsed from XXn format

	// Parsed compatibility flags (from remote)
	HasEII bool // Remote presented EII (EMSI-II capable)
	HasDFB bool // Remote presented DFB (fall-back capable)
	HasFRQ bool // Remote accepts file requests on outbound
	HasNRQ bool // Remote has no file request capability
	HasFNC bool // Remote requires filename conversion
	HasRMA bool // Remote supports multiple file requests (Hydra)
	HasRH1 bool // Remote uses Hydra batch separation
}

// buildLinkCodes constructs link codes string from config
// FSC-0088.001: For single-address, use PUA/PUP/NPU/HAT or PU0/HA0
// For multi-address, don't emit both PUA and PUn flags
func buildLinkCodes(cfg *Config, numAddresses int) string {
	var codes []string

	// Determine if we're using per-AKA flags
	hasPerAKAFlags := cfg.EnableEMSI2 && cfg.PerAKAFlags != nil && len(cfg.PerAKAFlags) > 0

	// FSC-0088.001: Neither PUA nor PUP should be presented if only one address
	// and per-AKA flags are being used. Use PU0/HA0 instead.
	if numAddresses == 1 && hasPerAKAFlags {
		// Single address with per-AKA config - use PU0/HA0 format
		if akaFlags, ok := cfg.PerAKAFlags[0]; ok {
			codes = append(codes, buildPerAKACodes(0, akaFlags)...)
		}
	} else if numAddresses > 1 && hasPerAKAFlags {
		// Multiple addresses with per-AKA config
		// FSC-0088.001: Use per-AKA flags, don't also emit PUA/PUP
		// Sort indices for deterministic output
		indices := make([]int, 0, len(cfg.PerAKAFlags))
		for idx := range cfg.PerAKAFlags {
			if idx < numAddresses { // Only include valid indices
				indices = append(indices, idx)
			}
		}
		sort.Ints(indices)
		for _, idx := range indices {
			codes = append(codes, buildPerAKACodes(idx, cfg.PerAKAFlags[idx])...)
		}
	} else {
		// No per-AKA flags or EMSI-I mode - use base link code
		if cfg.LinkCode != "" {
			codes = append(codes, cfg.LinkCode)
		} else {
			codes = append(codes, LinkCodePUA)
		}
	}

	// Add qualifiers (PMO, NFE, HXT, etc. without address suffix)
	codes = append(codes, cfg.LinkQualifiers...)

	// Add FNC as link-session option if required (EMSI-II)
	if cfg.EnableEMSI2 && cfg.RequireFNC {
		codes = append(codes, LinkQualFNC)
	}

	return strings.Join(codes, ",")
}

// buildPerAKACodes builds XXn format codes for a specific AKA index
func buildPerAKACodes(idx int, cfg *PerAKAConfig) []string {
	if cfg == nil {
		return nil
	}

	var codes []string
	idxStr := strconv.Itoa(idx)

	// Pickup flags
	if cfg.Pickup != nil && *cfg.Pickup {
		codes = append(codes, "PU"+idxStr)
	}
	if cfg.PickupMail != nil && *cfg.PickupMail {
		codes = append(codes, "PM"+idxStr)
	}
	if cfg.NoFiles != nil && *cfg.NoFiles {
		codes = append(codes, "NF"+idxStr)
	}
	if cfg.NoCompressed != nil && *cfg.NoCompressed {
		codes = append(codes, "NX"+idxStr)
	}
	if cfg.NoRequests != nil && *cfg.NoRequests {
		codes = append(codes, "NR"+idxStr)
	}

	// Hold flags
	if cfg.Hold != nil && *cfg.Hold {
		codes = append(codes, "HA"+idxStr)
	}
	if cfg.HoldNonMail != nil && *cfg.HoldNonMail {
		codes = append(codes, "HN"+idxStr)
	}
	if cfg.HoldCompress != nil && *cfg.HoldCompress {
		codes = append(codes, "HX"+idxStr)
	}
	if cfg.HoldFiles != nil && *cfg.HoldFiles {
		codes = append(codes, "HF"+idxStr)
	}
	if cfg.HoldRequests != nil && *cfg.HoldRequests {
		codes = append(codes, "HR"+idxStr)
	}

	return codes
}

// buildCompatibilityCodes constructs compatibility codes from config
// FSC-0088.001: EII must be FIRST if present, then protocols in preference order
func buildCompatibilityCodes(cfg *Config, dataProtocols []string) string {
	var codes []string

	// EMSI-II flag MUST be first per FSC-0088.001
	if cfg.EnableEMSI2 {
		codes = append(codes, CompatEII)
	}

	// Protocols in preference order (EMSI-II: caller-prefs)
	// Use dataProtocols if provided (backward compatibility), otherwise use config
	protocols := dataProtocols
	if len(protocols) == 0 {
		protocols = cfg.Protocols
	}
	if len(protocols) > 0 {
		codes = append(codes, protocols...)
	} else {
		codes = append(codes, "ZMO", "ZAP")
	}

	// DFB flag
	if cfg.EnableDFB {
		codes = append(codes, CompatDFB)
	}

	// File request flags
	if cfg.FileRequestCapable {
		codes = append(codes, CompatFRQ)
	}
	if cfg.NoFileRequests {
		codes = append(codes, CompatNRQ)
	}

	// Deprecated flags (only if not suppressed and not in EMSI-II mode)
	if !cfg.SuppressDeprecated && !cfg.EnableEMSI2 {
		codes = append(codes, CompatARC, CompatXMA)
	}

	return strings.Join(codes, ",")
}

// CreateEMSI_DAT creates an EMSI_DAT packet (legacy version without config)
// Deprecated: Use CreateEMSI_DATWithConfig for EMSI-II support
func CreateEMSI_DAT(data *EMSIData) string {
	return CreateEMSI_DATWithConfig(data, nil)
}

// CreateEMSI_DATWithConfig creates an EMSI_DAT packet using configuration
func CreateEMSI_DATWithConfig(data *EMSIData, cfg *Config) string {
	// Use default config if none provided
	if cfg == nil {
		cfg = DefaultConfig()
	}
	// Build the data section
	var dataPart strings.Builder
	
	// {EMSI} identifier
	dataPart.WriteString("{EMSI}")
	
	// {addresses}
	dataPart.WriteString("{")
	dataPart.WriteString(strings.Join(data.Addresses, " "))
	dataPart.WriteString("}")
	
	// {password}
	dataPart.WriteString("{")
	if data.Password != "" {
		dataPart.WriteString(data.Password)
	}
	dataPart.WriteString("}")
	
	// {link_codes} - link capabilities (built from config)
	dataPart.WriteString("{")
	dataPart.WriteString(buildLinkCodes(cfg, len(data.Addresses)))
	dataPart.WriteString("}")

	// {compatibility_codes} - protocol capabilities (built from config)
	// Pass data.Protocols for backward compatibility with legacy callers
	dataPart.WriteString("{")
	if len(cfg.Protocols) > 0 || len(data.Protocols) > 0 {
		dataPart.WriteString(buildCompatibilityCodes(cfg, data.Protocols))
	} else {
		dataPart.WriteString("NCP") // No Compatible Protocols
	}
	dataPart.WriteString("}")
	
	// {mailer_prod}{mailer_name}{mailer_version}{mailer_serial}
	// Note: These are separate braced fields, not one field
	dataPart.WriteString("{PID}")  // Mailer product code
	dataPart.WriteString("{")
	dataPart.WriteString(data.MailerName)
	dataPart.WriteString("}")
	dataPart.WriteString("{")
	dataPart.WriteString(data.MailerVersion)
	dataPart.WriteString("}")
	dataPart.WriteString("{")
	dataPart.WriteString(data.MailerSerial)
	dataPart.WriteString("}")
	
	// {extra_data} - IDENT block with bracketed fields
	dataPart.WriteString("{IDENT}")
	dataPart.WriteString("{")
	
	// IDENT has bracketed fields: [system][location][sysop][phone][baud][flags]
	dataPart.WriteString("[")
	dataPart.WriteString(data.SystemName)
	dataPart.WriteString("]")
	
	dataPart.WriteString("[")
	dataPart.WriteString(data.Location)
	dataPart.WriteString("]")
	
	dataPart.WriteString("[")
	dataPart.WriteString(data.Sysop)
	dataPart.WriteString("]")
	
	dataPart.WriteString("[")
	if data.Phone != "" {
		dataPart.WriteString(data.Phone)
	} else {
		dataPart.WriteString("-Unpublished-")
	}
	dataPart.WriteString("]")
	
	dataPart.WriteString("[")
	if data.Speed != "" {
		dataPart.WriteString(data.Speed)
	} else {
		dataPart.WriteString("TCP/IP")
	}
	dataPart.WriteString("]")
	
	dataPart.WriteString("[")
	if data.Flags != "" {
		dataPart.WriteString(data.Flags)
	} else {
		dataPart.WriteString("XA")
	}
	dataPart.WriteString("]")
	
	dataPart.WriteString("}")
	
	// Get the data part
	dataStr := dataPart.String()
	
	// Build the data section (without the ** prefix)
	lengthHex := fmt.Sprintf("%04X", len(dataStr))
	dataSection := fmt.Sprintf("EMSI_DAT%s%s", lengthHex, dataStr)
	
	// Calculate CRC16 of the data section (EMSI_DAT + length + data) 
	// Note: CRC is calculated WITHOUT the ** prefix (like bforce does)
	crc := CalculateCRC16([]byte(dataSection))
	
	// Build the final packet with ** prefix and CRC
	finalPacket := fmt.Sprintf("**%s%04X", dataSection, crc)
	
	return finalPacket
}

// extractBrackets extracts bracketed fields from IDENT data
func extractBrackets(data string) []string {
	var result []string
	var current strings.Builder
	inBracket := false
	escapeNext := false
	
	for _, char := range data {
		if escapeNext {
			current.WriteRune(char)
			escapeNext = false
			continue
		}
		
		switch char {
		case '\\':
			escapeNext = true
		case '[':
			if inBracket {
				current.WriteRune(char)
			} else {
				inBracket = true
			}
		case ']':
			if inBracket {
				result = append(result, current.String())
				current.Reset()
				inBracket = false
			}
		default:
			if inBracket {
				current.WriteRune(char)
			}
		}
	}
	
	return result
}

// perAKAPattern matches per-AKA flags like PU2, HA3, PM0, etc.
var perAKAPattern = regexp.MustCompile(`^(PU|PM|NF|NX|NR|HA|HN|HX|HF|HR)(\d+)$`)

// LinkCodesResult holds the parsed link codes
type LinkCodesResult struct {
	BaseCode   string
	Qualifiers []string
	PerAKA     map[int][]string
	HasFNC     bool // Filename conversion required
	HasRMA     bool // Multiple file requests (Hydra)
	HasRH1     bool // Hydra batch separation
}

// parseLinkCodes parses link codes field including per-AKA flags (FSC-0088.001)
// Returns structured result with base code, qualifiers, per-AKA flags, and session options
func parseLinkCodes(field string) (string, []string, map[int][]string) {
	result := parseLinkCodesEx(field)
	return result.BaseCode, result.Qualifiers, result.PerAKA
}

// parseLinkCodesEx parses link codes with full EMSI-II support including session options
func parseLinkCodesEx(field string) *LinkCodesResult {
	result := &LinkCodesResult{
		PerAKA: make(map[int][]string),
	}
	codes := strings.Split(field, ",")

	for _, code := range codes {
		code = strings.TrimSpace(strings.ToUpper(code))
		if code == "" {
			continue
		}

		// Check for per-AKA flag (XXn format)
		if matches := perAKAPattern.FindStringSubmatch(code); matches != nil {
			idx, _ := strconv.Atoi(matches[2])
			result.PerAKA[idx] = append(result.PerAKA[idx], matches[1])
			continue
		}

		// Check for base link code
		switch code {
		case LinkCodePUA, LinkCodePUP, LinkCodeNPU, LinkCodeHAT:
			result.BaseCode = code
		// Link-session options (FSC-0088.001)
		case LinkQualFNC:
			result.HasFNC = true
		case LinkQualRMA:
			result.HasRMA = true
		case LinkQualRH1:
			result.HasRH1 = true
		default:
			// Assume it's a qualifier (PMO, NFE, HXT, etc.)
			result.Qualifiers = append(result.Qualifiers, code)
		}
	}

	// Default to PUA if no base code found
	if result.BaseCode == "" {
		result.BaseCode = LinkCodePUA
	}

	return result
}

// parseCompatibilityCodes extracts EMSI-II flags from compatibility field
// Note: FNC/RMA/RH1 are link-session options parsed from link codes, not compat codes
func parseCompatibilityCodes(field string, data *EMSIData) {
	codes := strings.Split(field, ",")

	for _, code := range codes {
		code = strings.TrimSpace(strings.ToUpper(code))
		switch code {
		case CompatEII:
			data.HasEII = true
		case CompatDFB:
			data.HasDFB = true
		case CompatFRQ:
			data.HasFRQ = true
		case CompatNRQ:
			data.HasNRQ = true
		case "NCP":
			// No compatible protocols - skip
		case CompatARC, CompatXMA:
			// Deprecated - ignore
		case LinkQualFNC, LinkQualRMA, LinkQualRH1:
			// FSC-0088.001: These are link-session options, not compatibility codes
			// They should be parsed from link codes field, ignore here
		default:
			// Protocol or other code - add to protocols if it looks like one
			if code != "" && len(code) == 3 {
				// Likely a protocol code (ZMO, ZAP, HYD, etc.)
				data.Protocols = append(data.Protocols, code)
			}
		}
	}
}

// ParseEMSI_DAT parses an EMSI_DAT packet
func ParseEMSI_DAT(packet string) (*EMSIData, error) {
	// Remove the header if present
	if strings.HasPrefix(packet, "**EMSI_DAT") {
		packet = packet[14:] // Skip **EMSI_DATxxxx
	}
	
	// Find the data between braces
	data := &EMSIData{
		Addresses:    []string{},
		Protocols:    []string{},
		Capabilities: []string{},
	}
	
	// Simple parser - look for address patterns
	parts := strings.Split(packet, "}")
	for i, part := range parts {
		part = strings.TrimPrefix(part, "{")
		
		switch i {
		case 0: // EMSI identifier
			// Skip
		case 1: // Addresses
			addrs := strings.Fields(part)
			for _, addr := range addrs {
				if strings.Contains(addr, ":") && strings.Contains(addr, "/") {
					data.Addresses = append(data.Addresses, addr)
				}
			}
		case 2: // Password
			data.Password = part
		case 3: // Link codes (FSC-0088.001 enhanced parsing)
			linkResult := parseLinkCodesEx(part)
			data.LinkCode = linkResult.BaseCode
			data.LinkQualifiers = linkResult.Qualifiers
			data.PerAKAFlags = linkResult.PerAKA
			data.HasFNC = linkResult.HasFNC
			data.HasRMA = linkResult.HasRMA
			data.HasRH1 = linkResult.HasRH1
		case 4: // Compatibility codes (protocols + EMSI-II flags)
			parseCompatibilityCodes(part, data)
		case 5: // Mailer product code
			// This is just the product code (like "FE", "PID", etc)
			// Skip it or store it if needed
		case 6: // Mailer name
			data.MailerName = part
		case 7: // Mailer version
			data.MailerVersion = part
		case 8: // Mailer serial/OS
			data.MailerSerial = part
		default: // Extra data (9 and beyond)
			// IDENT comes as a separate field followed by bracketed data in the next field
			if part == "IDENT" && i+1 < len(parts) {
				// The next part contains the bracketed fields
				identData := strings.TrimPrefix(parts[i+1], "{")
				brackets := extractBrackets(identData)
				if len(brackets) > 0 {
					data.SystemName = brackets[0]
				}
				if len(brackets) > 1 {
					data.Location = brackets[1]
				}
				if len(brackets) > 2 {
					data.Sysop = brackets[2]
				}
				if len(brackets) > 3 {
					data.Phone = brackets[3]
				}
				// brackets[4] would be baud
				if len(brackets) > 5 {
					data.Flags = brackets[5]
				}
				// Skip the next part since we processed it
				continue
			}
			
			// Also parse simple key:value fields
			extras := strings.Split(part, ",")
			for _, extra := range extras {
				if strings.HasPrefix(extra, "SNAME:") {
					sname := strings.TrimPrefix(extra, "SNAME:")
					if data.SystemName == "" {
						data.SystemName = sname
					}
				} else if strings.HasPrefix(extra, "LOC:") {
					loc := strings.TrimPrefix(extra, "LOC:")
					if data.Location == "" {
						data.Location = loc
					}
				} else if strings.HasPrefix(extra, "SYSOP:") {
					sysop := strings.TrimPrefix(extra, "SYSOP:")
					if data.Sysop == "" {
						data.Sysop = sysop
					}
				} else if strings.HasPrefix(extra, "PHONE:") {
					phone := strings.TrimPrefix(extra, "PHONE:")
					if data.Phone == "" {
						data.Phone = phone
					}
				} else if strings.HasPrefix(extra, "FLAGS:") {
					flags := strings.TrimPrefix(extra, "FLAGS:")
					if data.Flags == "" {
						data.Flags = flags
					}
				}
			}
		}
	}
	
	return data, nil
}

// CalculateCRC16 calculates CRC16-XMODEM for EMSI (not CCITT!)
func CalculateCRC16(data []byte) uint16 {
	var crc uint16 = 0
	
	for _, b := range data {
		crc = crc16XmodemTable[(crc>>8)^uint16(b)] ^ (crc << 8)
	}
	
	return crc
}

// CRC16-XMODEM table
var crc16XmodemTable = [256]uint16{
	0x0000, 0x1021, 0x2042, 0x3063, 0x4084, 0x50a5, 0x60c6, 0x70e7,
	0x8108, 0x9129, 0xa14a, 0xb16b, 0xc18c, 0xd1ad, 0xe1ce, 0xf1ef,
	0x1231, 0x0210, 0x3273, 0x2252, 0x52b5, 0x4294, 0x72f7, 0x62d6,
	0x9339, 0x8318, 0xb37b, 0xa35a, 0xd3bd, 0xc39c, 0xf3ff, 0xe3de,
	0x2462, 0x3443, 0x0420, 0x1401, 0x64e6, 0x74c7, 0x44a4, 0x5485,
	0xa56a, 0xb54b, 0x8528, 0x9509, 0xe5ee, 0xf5cf, 0xc5ac, 0xd58d,
	0x3653, 0x2672, 0x1611, 0x0630, 0x76d7, 0x66f6, 0x5695, 0x46b4,
	0xb75b, 0xa77a, 0x9719, 0x8738, 0xf7df, 0xe7fe, 0xd79d, 0xc7bc,
	0x48c4, 0x58e5, 0x6886, 0x78a7, 0x0840, 0x1861, 0x2802, 0x3823,
	0xc9cc, 0xd9ed, 0xe98e, 0xf9af, 0x8948, 0x9969, 0xa90a, 0xb92b,
	0x5af5, 0x4ad4, 0x7ab7, 0x6a96, 0x1a71, 0x0a50, 0x3a33, 0x2a12,
	0xdbfd, 0xcbdc, 0xfbbf, 0xeb9e, 0x9b79, 0x8b58, 0xbb3b, 0xab1a,
	0x6ca6, 0x7c87, 0x4ce4, 0x5cc5, 0x2c22, 0x3c03, 0x0c60, 0x1c41,
	0xedae, 0xfd8f, 0xcdec, 0xddcd, 0xad2a, 0xbd0b, 0x8d68, 0x9d49,
	0x7e97, 0x6eb6, 0x5ed5, 0x4ef4, 0x3e13, 0x2e32, 0x1e51, 0x0e70,
	0xff9f, 0xefbe, 0xdfdd, 0xcffc, 0xbf1b, 0xaf3a, 0x9f59, 0x8f78,
	0x9188, 0x81a9, 0xb1ca, 0xa1eb, 0xd10c, 0xc12d, 0xf14e, 0xe16f,
	0x1080, 0x00a1, 0x30c2, 0x20e3, 0x5004, 0x4025, 0x7046, 0x6067,
	0x83b9, 0x9398, 0xa3fb, 0xb3da, 0xc33d, 0xd31c, 0xe37f, 0xf35e,
	0x02b1, 0x1290, 0x22f3, 0x32d2, 0x4235, 0x5214, 0x6277, 0x7256,
	0xb5ea, 0xa5cb, 0x95a8, 0x8589, 0xf56e, 0xe54f, 0xd52c, 0xc50d,
	0x34e2, 0x24c3, 0x14a0, 0x0481, 0x7466, 0x6447, 0x5424, 0x4405,
	0xa7db, 0xb7fa, 0x8799, 0x97b8, 0xe75f, 0xf77e, 0xc71d, 0xd73c,
	0x26d3, 0x36f2, 0x0691, 0x16b0, 0x6657, 0x7676, 0x4615, 0x5634,
	0xd94c, 0xc96d, 0xf90e, 0xe92f, 0x99c8, 0x89e9, 0xb98a, 0xa9ab,
	0x5844, 0x4865, 0x7806, 0x6827, 0x18c0, 0x08e1, 0x3882, 0x28a3,
	0xcb7d, 0xdb5c, 0xeb3f, 0xfb1e, 0x8bf9, 0x9bd8, 0xabbb, 0xbb9a,
	0x4a75, 0x5a54, 0x6a37, 0x7a16, 0x0af1, 0x1ad0, 0x2ab3, 0x3a92,
	0xfd2e, 0xed0f, 0xdd6c, 0xcd4d, 0xbdaa, 0xad8b, 0x9de8, 0x8dc9,
	0x7c26, 0x6c07, 0x5c64, 0x4c45, 0x3ca2, 0x2c83, 0x1ce0, 0x0cc1,
	0xef1f, 0xff3e, 0xcf5d, 0xdf7c, 0xaf9b, 0xbfba, 0x8fd9, 0x9ff8,
	0x6e17, 0x7e36, 0x4e55, 0x5e74, 0x2e93, 0x3eb2, 0x0ed1, 0x1ef0,
}