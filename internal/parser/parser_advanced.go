package parser

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nodelistdb/internal/database"
)

// NodelistFormat represents different historical nodelist formats
type NodelistFormat int

const (
	Format1986 NodelistFormat = iota // XP:, MO:, etc.
	Format1990                       // XA, MO, basic flags
	Format2000                       // Internet flags introduced
	Format2020                       // Modern complex flags
)

// FlagInfo contains metadata about flag types
type FlagInfo struct {
	Category    string // modem, internet, capability, schedule, user
	HasValue    bool   // whether flag takes a parameter
	Description string
}

// Context tracks the current parsing context
type Context struct {
	CurrentZone   int
	CurrentNet    int  
	CurrentRegion *int
}

// AdvancedParser extends Parser with all sophisticated features
type AdvancedParser struct {
	*Parser
	DetectedFormat NodelistFormat
	LegacyFlagMap  map[string]string
	ModernFlagMap  map[string]FlagInfo
	HeaderPattern  *regexp.Regexp
	LinePattern    *regexp.Regexp
	Context        Context
}

// NewAdvanced creates a new advanced parser with all features
func NewAdvanced(verbose bool) *AdvancedParser {
	return &AdvancedParser{
		Parser: New(verbose),
		Context: Context{
			CurrentZone: 1,
			CurrentNet:  1,
		},
		LegacyFlagMap: map[string]string{
			"XP:": "XA",      // 1986 file requests -> modern
			"MO:": "MO",      // Mail only
			"RE:": "Region",  // Regional
			"DA:": "DAY",     // Daytime hours
			"WK:": "WORK",    // Work hours
			"WE:": "WEEKEND", // Weekend
		},
		ModernFlagMap: map[string]FlagInfo{
			// Capability flags
			"CM": {Category: "capability", HasValue: false, Description: "Continuous Mail"},
			"MO": {Category: "capability", HasValue: false, Description: "Mail Only"},
			"LO": {Category: "capability", HasValue: false, Description: "Local Only"},
			"XA": {Category: "capability", HasValue: false, Description: "File Requests"},
			"XB": {Category: "capability", HasValue: false, Description: "File Requests with WaZOO"},
			"XC": {Category: "capability", HasValue: false, Description: "File Requests with ZedZap"},
			"XP": {Category: "capability", HasValue: false, Description: "File Requests with Hydra"},
			"XR": {Category: "capability", HasValue: false, Description: "File Requests with Janus"},
			"XX": {Category: "capability", HasValue: false, Description: "Listed but not accepting calls"},

			// Modem flags
			"V22":  {Category: "modem", HasValue: false, Description: "V.22 2400 bps"},
			"V29":  {Category: "modem", HasValue: false, Description: "V.29 9600 bps"},
			"V32":  {Category: "modem", HasValue: false, Description: "V.32 9600 bps"},
			"V32B": {Category: "modem", HasValue: false, Description: "V.32bis 14400 bps"},
			"V33":  {Category: "modem", HasValue: false, Description: "V.33 14400 bps"},
			"V34":  {Category: "modem", HasValue: false, Description: "V.34 28800 bps"},
			"V42":  {Category: "modem", HasValue: false, Description: "V.42 error correction"},
			"V42B": {Category: "modem", HasValue: false, Description: "V.42bis compression"},
			"HST":  {Category: "modem", HasValue: false, Description: "USRobotics HST"},
			"H96":  {Category: "modem", HasValue: false, Description: "HST 9600"},
			"H14":  {Category: "modem", HasValue: false, Description: "HST 14400"},
			"H16":  {Category: "modem", HasValue: false, Description: "HST 16800"},
			"MAX":  {Category: "modem", HasValue: false, Description: "Microcom AX/96xx"},
			"PEP":  {Category: "modem", HasValue: false, Description: "Packet Ensemble Protocol"},
			"CSP":  {Category: "modem", HasValue: false, Description: "Compucom SpeedModem"},
			"ZYX":  {Category: "modem", HasValue: false, Description: "Zyxel modem"},
			"VFC":  {Category: "modem", HasValue: false, Description: "V.Fast Class"},
			"V90C": {Category: "modem", HasValue: false, Description: "V.90 Client"},
			"V90S": {Category: "modem", HasValue: false, Description: "V.90 Server"},

			// Internet flags
			"IBN":   {Category: "internet", HasValue: true, Description: "Binkp over Internet"},
			"IFC":   {Category: "internet", HasValue: true, Description: "Raw FidoNet over Internet"},
			"IFT":   {Category: "internet", HasValue: true, Description: "Telnet to FidoNet"},
			"ITN":   {Category: "internet", HasValue: true, Description: "Telnet"},
			"IVM":   {Category: "internet", HasValue: true, Description: "VModem over Internet"},
			"ITX":   {Category: "internet", HasValue: true, Description: "Txy over Internet"},
			"IUC":   {Category: "internet", HasValue: true, Description: "UUcp"},
			"IMI":   {Category: "internet", HasValue: true, Description: "MIME"},
			"ISE":   {Category: "internet", HasValue: true, Description: "SMTP"},
			"INA":   {Category: "internet", HasValue: true, Description: "No specific protocol"},
			"IEM":   {Category: "internet", HasValue: true, Description: "Email"},
			"IP":    {Category: "internet", HasValue: true, Description: "IP address"},
			"PING":  {Category: "internet", HasValue: false, Description: "Accepts ping"},
			"TRACE": {Category: "internet", HasValue: false, Description: "Accepts traceroute"},

			// Schedule flags
			"T24":  {Category: "schedule", HasValue: false, Description: "24 hours"},
			"Tyz":  {Category: "schedule", HasValue: false, Description: "Varies"},
		},
	}
}

// sanitizeUTF8 converts non-UTF8 characters to valid UTF-8 for database storage
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	
	// Convert invalid UTF-8 sequences to replacement characters
	var result strings.Builder
	for _, r := range s {
		if r == utf8.RuneError {
			result.WriteRune('�') // Unicode replacement character
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// detectFormat detects the nodelist format based on content
func (ap *AdvancedParser) detectFormat(headerLine, firstNodeLine string) NodelistFormat {
	// Check for 1986 format indicators
	if strings.Contains(headerLine, "-- Day number") && strings.Contains(headerLine, " : ") {
		return Format1986
	}
	
	// Check for flags in node lines to determine format
	if strings.Contains(firstNodeLine, "XP:") || strings.Contains(firstNodeLine, "MO:") {
		return Format1986
	}
	
	// Check for modern internet flags
	if strings.Contains(firstNodeLine, "IBN:") || strings.Contains(firstNodeLine, "ITN:") {
		if strings.Contains(firstNodeLine, "IEM:") {
			return Format2020
		}
		return Format2000
	}
	
	// Default to 1990s format
	return Format1990
}

// parseHeaderLine parses header information with CRC validation
func (ap *AdvancedParser) parseHeaderLine(line string) (time.Time, int, uint16, error) {
	// Remove ";A " prefix
	line = strings.TrimPrefix(line, ";A ")

	// Handle different separators: ":" (modern) or "--" (1986 format)
	var dateStr, crcStr string
	if strings.Contains(line, " : ") {
		// Modern format: "Friday, October 3, 1986 : 20179"
		parts := strings.Split(line, " : ")
		if len(parts) < 2 {
			return time.Time{}, 0, 0, fmt.Errorf("invalid header format: missing CRC after ':'")
		}
		dateStr = strings.TrimSpace(parts[0])
		crcStr = strings.TrimSpace(parts[1])
	} else if strings.Contains(line, " -- Day number ") && strings.Contains(line, " : ") {
		// 1986 format: "Friday, October 3, 1986 -- Day number 276 : 20179"
		if colonIdx := strings.LastIndex(line, " : "); colonIdx != -1 {
			dateStr = strings.TrimSpace(line[:colonIdx])
			crcStr = strings.TrimSpace(line[colonIdx+3:])
		} else {
			return time.Time{}, 0, 0, fmt.Errorf("invalid 1986 header format: missing final ':'")
		}
	} else {
		return time.Time{}, 0, 0, fmt.Errorf("invalid header format: expected ':' or '-- Day number' pattern. Got: %s", line)
	}

	// Parse CRC
	crc, err := strconv.ParseUint(crcStr, 10, 16)
	if err != nil {
		return time.Time{}, 0, 0, fmt.Errorf("invalid CRC '%s': %w", crcStr, err)
	}

	// Parse date - try different formats
	date, dayNumber, err := ap.parseDate(dateStr)
	if err != nil {
		return time.Time{}, 0, 0, fmt.Errorf("invalid date '%s': %w", dateStr, err)
	}

	return date, dayNumber, uint16(crc), nil
}

// parseDate handles various date formats found in nodelists
func (ap *AdvancedParser) parseDate(dateStr string) (time.Time, int, error) {
	// Handle modern format: "FidoNet Nodelist for Monday, January 28, 2019 -- Day number 028"
	if strings.Contains(dateStr, "FidoNet Nodelist for") {
		return ap.parseModernFormat(dateStr)
	}

	// Handle 1986 format: "Friday, October 3, 1986 -- Day number 276"
	if strings.Contains(dateStr, " -- Day number ") {
		return ap.parse1986Format(dateStr)
	}

	// Common patterns in nodelist headers
	patterns := []string{
		"Monday, January 2, 2006", // Full format
		"January 2, 2006",         // No day of week
		"Jan 2, 2006",             // Short month
		"2 Jan 2006",              // European style
		"2006-01-02",              // ISO format
		"02/01/2006",              // US format
		"02-01-2006",              // US format with dashes
	}

	for _, pattern := range patterns {
		if date, err := time.Parse(pattern, dateStr); err == nil {
			dayNumber := date.YearDay()
			return date, dayNumber, nil
		}
	}

	return time.Time{}, 0, fmt.Errorf("unrecognized date format: %s", dateStr)
}

// parseModernFormat handles modern format
func (ap *AdvancedParser) parseModernFormat(dateStr string) (time.Time, int, error) {
	// Extract: "FidoNet Nodelist for Monday, January 28, 2019 -- Day number 028"
	re := regexp.MustCompile(`FidoNet Nodelist for (.+?)\s+--\s+Day number\s+(\d+)`)
	matches := re.FindStringSubmatch(dateStr)
	if len(matches) < 3 {
		return time.Time{}, 0, fmt.Errorf("invalid modern format: %s", dateStr)
	}

	datePart := strings.TrimSpace(matches[1])
	dayNumberStr := strings.TrimSpace(matches[2])

	// Parse day number
	dayNumber, err := strconv.Atoi(dayNumberStr)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid day number %s: %w", dayNumberStr, err)
	}

	// Parse the date part
	patterns := []string{
		"Monday, January 2, 2006",
		"January 2, 2006",
		"Jan 2, 2006",
	}

	for _, pattern := range patterns {
		if date, err := time.Parse(pattern, datePart); err == nil {
			return date, dayNumber, nil
		}
	}

	return time.Time{}, 0, fmt.Errorf("unrecognized date in modern format: %s", datePart)
}

// parse1986Format handles the 1986 format
func (ap *AdvancedParser) parse1986Format(dateStr string) (time.Time, int, error) {
	// Extract date part: "Friday, October 3, 1986 -- Day number 276"
	re := regexp.MustCompile(`(.+?)\s+--\s+Day number\s+(\d+)`)
	matches := re.FindStringSubmatch(dateStr)
	if len(matches) < 3 {
		return time.Time{}, 0, fmt.Errorf("invalid 1986 format: %s", dateStr)
	}

	datePart := strings.TrimSpace(matches[1])
	dayNumberStr := strings.TrimSpace(matches[2])

	// Parse day number
	dayNumber, err := strconv.Atoi(dayNumberStr)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid day number %s: %w", dayNumberStr, err)
	}

	// Parse the date part
	patterns := []string{
		"Monday, January 2, 2006", // Full format with day of week
		"January 2, 2006",         // No day of week
		"Jan 2, 2006",             // Short month
		"2 Jan 2006",              // European style
	}

	for _, pattern := range patterns {
		if date, err := time.Parse(pattern, datePart); err == nil {
			return date, dayNumber, nil
		}
	}

	return time.Time{}, 0, fmt.Errorf("unrecognized date in 1986 format: %s", datePart)
}

// splitFlags splits flag string handling quoted strings and special separators
func (ap *AdvancedParser) splitFlags(flagsStr string) []string {
	var flags []string
	var current strings.Builder
	inQuotes := false
	
	for i, r := range flagsStr {
		switch r {
		case '"':
			inQuotes = !inQuotes
			current.WriteRune(r)
		case ',':
			if !inQuotes {
				if flag := strings.TrimSpace(current.String()); flag != "" {
					flags = append(flags, flag)
				}
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
		
		// Handle end of string
		if i == len(flagsStr)-1 {
			if flag := strings.TrimSpace(current.String()); flag != "" {
				flags = append(flags, flag)
			}
		}
	}
	
	return flags
}

// parseFlagValue parses a flag and its value (e.g., "IBN:hostname:port")
func (ap *AdvancedParser) parseFlagValue(flag string) (string, string) {
	parts := strings.SplitN(flag, ":", 2)
	if len(parts) > 1 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(flag), ""
}

// parseInternetFlag parses complex internet flags
func (ap *AdvancedParser) parseInternetFlag(flagName, flagValue string) interface{} {
	switch flagName {
	case "IBN", "ITN", "IFC", "IFT":
		// Format: hostname[:port]
		if strings.Contains(flagValue, ":") {
			parts := strings.Split(flagValue, ":")
			if len(parts) >= 2 {
				return map[string]interface{}{
					"host": parts[0],
					"port": parts[1],
				}
			}
		}
		return map[string]interface{}{
			"host": flagValue,
		}
	case "IEM":
		// Email address
		return flagValue
	case "IP":
		// IP address
		return flagValue
	default:
		return flagValue
	}
}

// updateContext updates parsing context based on node type
func (ap *AdvancedParser) updateContext(nodeType string, nodeNum int) {
	switch strings.ToLower(nodeType) {
	case "zone":
		ap.Context.CurrentZone = nodeNum
		ap.Context.CurrentNet = nodeNum
	case "region":
		region := nodeNum
		ap.Context.CurrentRegion = &region
		ap.Context.CurrentNet = nodeNum
	case "host":
		ap.Context.CurrentNet = nodeNum
	}
}

// Enhanced parseFlags with full categorization
func (ap *AdvancedParser) parseAdvancedFlags(flagsStr string) ([]string, []string, []string, []int, []string, []string) {
	if flagsStr == "" {
		return []string{}, []string{}, []string{}, []int{}, []string{}, []string{}
	}
	
	var allFlags []string
	var internetProtocols []string
	var internetHostnames []string
	var internetPorts []int
	var internetEmails []string
	var modemFlags []string
	
	// Split flags by comma and handle quoted strings
	flags := ap.splitFlags(flagsStr)
	
	for _, flag := range flags {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			continue
		}
		
		// Handle legacy format conversion
		if ap.DetectedFormat == Format1986 {
			if modernFlag, exists := ap.LegacyFlagMap[flag]; exists {
				flag = modernFlag
			}
		}
		
		// Parse complex flags with parameters
		flagName, flagValue := ap.parseFlagValue(flag)
		
		// Categorize flag
		if flagInfo, exists := ap.ModernFlagMap[flagName]; exists {
			switch flagInfo.Category {
			case "internet":
				internetProtocols = append(internetProtocols, flagName)
				if flagValue != "" {
					switch flagName {
					case "IBN", "ITN", "IFC", "IFT":
						// Parse hostname:port
						if strings.Contains(flagValue, ":") {
							parts := strings.Split(flagValue, ":")
							if len(parts) >= 2 {
								internetHostnames = append(internetHostnames, parts[0])
								if port, err := strconv.Atoi(parts[1]); err == nil {
									internetPorts = append(internetPorts, port)
								}
							}
						} else {
							internetHostnames = append(internetHostnames, flagValue)
						}
					case "IEM":
						internetEmails = append(internetEmails, flagValue)
					}
				}
			case "modem":
				modemFlags = append(modemFlags, flagName)
			default:
				allFlags = append(allFlags, flagName)
			}
		} else {
			// Unknown flag - store as-is
			allFlags = append(allFlags, flag)
		}
	}
	
	return allFlags, internetProtocols, internetHostnames, internetPorts, internetEmails, modemFlags
}

// parseAdvancedLine parses a single nodelist entry line with all advanced features
func (ap *AdvancedParser) parseAdvancedLine(line string, nodelistDate time.Time, dayNumber int, filePath string, fileCRC uint16) (*database.Node, error) {
	// Sanitize UTF-8 for database storage
	line = sanitizeUTF8(line)
	
	// Handle different line formats - FidoNet standard is comma-separated
	fields := strings.Split(line, ",")
	if len(fields) < 7 {
		return nil, fmt.Errorf("insufficient fields: expected at least 7, got %d. Line: %s", len(fields), line)
	}

	// Extract basic fields
	nodeTypeStr := strings.TrimSpace(fields[0])
	nodeNumStr := strings.TrimSpace(fields[1])
	
	// Strip leading "-" from node numbers (legacy format)
	if strings.HasPrefix(nodeNumStr, "-") {
		nodeNumStr = nodeNumStr[1:]
	}

	systemName := sanitizeUTF8(strings.TrimSpace(fields[2]))
	location := sanitizeUTF8(strings.TrimSpace(fields[3]))
	sysopName := sanitizeUTF8(strings.TrimSpace(fields[4]))
	phone := strings.TrimSpace(fields[5])
	maxSpeed := strings.TrimSpace(fields[6])

	// Parse flags (field 7 and beyond)
	var flagsStr string
	if len(fields) > 7 {
		flagsStr = strings.Join(fields[7:], ",")
	}

	// Determine node type and update context
	var nodeType string
	var zone, net, node int
	var region *int

	if nodeTypeStr == "" {
		// Empty first field = normal node
		nodeType = "Node"
		zone = ap.Context.CurrentZone
		net = ap.Context.CurrentNet
		if nodeNum, err := strconv.Atoi(nodeNumStr); err == nil {
			node = nodeNum
		} else {
			return nil, fmt.Errorf("invalid node number: %s", nodeNumStr)
		}
	} else {
		// Handle special node types with proper context updating
		switch strings.Title(strings.ToLower(nodeTypeStr)) {
		case "Zone":
			nodeType = "Zone"
			if z, err := strconv.Atoi(nodeNumStr); err == nil {
				ap.updateContext("zone", z)  // Update context FIRST
				zone = z
				net = z  // Zone nodes have net = zone
				node = 0 // Zone coordinator
			} else {
				return nil, fmt.Errorf("invalid zone number: %s", nodeNumStr)
			}
		case "Region":
			nodeType = "Region"
			if r, err := strconv.Atoi(nodeNumStr); err == nil {
				ap.updateContext("region", r)  // Update context FIRST
				regionNum := r
				region = &regionNum
				zone = ap.Context.CurrentZone
				net = r
				node = 0 // Regional coordinator
			} else {
				return nil, fmt.Errorf("invalid region number: %s", nodeNumStr)
			}
		case "Host":
			nodeType = "Host"
			if n, err := strconv.Atoi(nodeNumStr); err == nil {
				ap.updateContext("host", n)  // Update context FIRST
				zone = ap.Context.CurrentZone
				net = n
				node = 0 // Host coordinator
			} else {
				return nil, fmt.Errorf("invalid host number: %s", nodeNumStr)
			}
		case "Hub":
			nodeType = "Hub"
			zone = ap.Context.CurrentZone
			net = ap.Context.CurrentNet
			if h, err := strconv.Atoi(nodeNumStr); err == nil {
				node = h
			} else {
				return nil, fmt.Errorf("invalid hub number: %s", nodeNumStr)
			}
		case "Pvt":
			nodeType = "Pvt"
			zone = ap.Context.CurrentZone
			net = ap.Context.CurrentNet
			if n, err := strconv.Atoi(nodeNumStr); err == nil {
				node = n
			} else {
				return nil, fmt.Errorf("invalid pvt node number: %s", nodeNumStr)
			}
		case "Down":
			nodeType = "Down"
			zone = ap.Context.CurrentZone
			net = ap.Context.CurrentNet
			if n, err := strconv.Atoi(nodeNumStr); err == nil {
				node = n
			} else {
				return nil, fmt.Errorf("invalid down node number: %s", nodeNumStr)
			}
		case "Hold":
			nodeType = "Hold"
			zone = ap.Context.CurrentZone
			net = ap.Context.CurrentNet
			if n, err := strconv.Atoi(nodeNumStr); err == nil {
				node = n
			} else {
				return nil, fmt.Errorf("invalid hold node number: %s", nodeNumStr)
			}
		default:
			return nil, fmt.Errorf("unknown node type: %s", nodeTypeStr)
		}
	}

	// Copy context region if available
	if ap.Context.CurrentRegion != nil && region == nil {
		region = ap.Context.CurrentRegion
	}

	// Parse flags into structured format with full categorization
	flags, internetProtocols, internetHostnames, internetPorts, internetEmails, modemFlags := ap.parseAdvancedFlags(flagsStr)
	
	// Compute boolean flags based on comprehensive flag analysis
	isCM := ap.hasFlag(flags, "CM")
	isMO := ap.hasFlag(flags, "MO") 
	hasBinkp := len(internetProtocols) > 0 && (ap.hasProtocol(internetProtocols, "IBN") || ap.hasProtocol(internetProtocols, "BND"))
	hasTelnet := len(internetProtocols) > 0 && (ap.hasProtocol(internetProtocols, "ITN") || ap.hasProtocol(internetProtocols, "TEL"))
	isDown := nodeType == "Down"
	isHold := nodeType == "Hold"
	isPvt := nodeType == "Pvt"
	isActive := !isDown && !isHold

	// Create node with all enhanced data
	now := time.Now()
	dbNode := database.Node{
		Zone:         zone,
		Net:          net,
		Node:         node,
		NodelistDate: nodelistDate,
		DayNumber:    dayNumber,
		SystemName:   systemName,
		Location:     location,
		SysopName:    sysopName,
		Phone:        phone,
		NodeType:     nodeType,
		Region:       region,
		MaxSpeed:     maxSpeed,
		IsCM:         isCM,
		IsMO:         isMO,
		HasBinkp:     hasBinkp,
		HasTelnet:    hasTelnet,
		IsDown:       isDown,
		IsHold:       isHold,
		IsPvt:        isPvt,
		IsActive:     isActive,
		Flags:        flags,
		ModemFlags:   modemFlags,
		InternetProtocols: internetProtocols,
		InternetHostnames: internetHostnames,
		InternetPorts:     internetPorts,
		InternetEmails:    internetEmails,
		RawLine:      line,
		FilePath:     filePath,
		FileCRC:      int(fileCRC), // Use actual CRC from header
		FirstSeen:    now,
		LastSeen:     now,
		ConflictSequence: 0,    // Default to 0 (original entry)
		HasConflict:      false, // Default to false
	}

	return &dbNode, nil
}

// Helper methods for flag checking
func (ap *AdvancedParser) hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

func (ap *AdvancedParser) hasProtocol(protocols []string, protocol string) bool {
	for _, p := range protocols {
		if strings.EqualFold(p, protocol) {
			return true
		}
	}
	return false
}

// extractDateFromFile extracts nodelist date from filename and file content
func (ap *AdvancedParser) extractDateFromFile(filePath string) (time.Time, int, error) {
	filename := filepath.Base(filePath)
	
	// Pattern 1: nodelist.NNN (where NNN is day of year)
	if match := regexp.MustCompile(`nodelist\.(\d{3})$`).FindStringSubmatch(strings.ToLower(filename)); match != nil {
		dayNum, err := strconv.Atoi(match[1])
		if err != nil {
			return time.Time{}, 0, err
		}
		
		// Extract year from directory path
		year := ap.extractYearFromPath(filePath)
		if year == 0 {
			year = 1986 // Default fallback
		}
		
		// Convert day number to date
		date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
		return date, dayNum, nil
	}

	// Pattern 2: Try to extract from file header
	if date, day, err := ap.extractDateFromHeader(filePath); err == nil {
		return date, day, nil
	}

	// Fallback: use file modification time
	info, err := os.Stat(filePath)
	if err != nil {
		return time.Time{}, 0, err
	}

	modTime := info.ModTime()
	dayOfYear := modTime.YearDay()
	
	return modTime.Truncate(24 * time.Hour), dayOfYear, nil
}

// extractYearFromPath tries to extract year from directory path
func (ap *AdvancedParser) extractYearFromPath(filePath string) int {
	parts := strings.Split(filePath, string(filepath.Separator))
	for _, part := range parts {
		if year, err := strconv.Atoi(part); err == nil {
			if year >= 1980 && year <= time.Now().Year()+1 {
				return year
			}
		}
	}
	return 0
}

// extractDateFromHeader tries to extract date from file header
func (ap *AdvancedParser) extractDateFromHeader(filePath string) (time.Time, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	
	for scanner.Scan() && lineCount < 10 {
		line := strings.TrimSpace(scanner.Text())
		lineCount++
		
		// Look for date patterns in comments
		if strings.HasPrefix(line, ";") {
			// Parse various date formats from comments
			if date, day, err := ap.parseDateFromComment(line); err == nil {
				return date, day, nil
			}
		}
	}
	
	return time.Time{}, 0, fmt.Errorf("no date found in header")
}

// parseDateFromComment extracts date from comment lines
func (ap *AdvancedParser) parseDateFromComment(comment string) (time.Time, int, error) {
	comment = strings.TrimPrefix(comment, ";")
	comment = strings.TrimSpace(comment)
	
	// Try various date formats
	formats := []string{
		"Day number 276 : 03 Oct 86",
		"Day number 276 : 03 Oct 1986",
		"03 Oct 86",
		"03 Oct 1986",
		"1986-10-03",
		"86-10-03",
	}
	
	for _ = range formats {
		// This is a simplified parser - in production, use proper date parsing
		if strings.Contains(strings.ToLower(comment), "day number") {
			// Extract day number
			re := regexp.MustCompile(`day number (\d+)`)
			if matches := re.FindStringSubmatch(strings.ToLower(comment)); len(matches) > 1 {
				if dayNum, err := strconv.Atoi(matches[1]); err == nil {
					// Try to extract year from same line
					year := 1986 // Default
					if yearMatch := regexp.MustCompile(`(\d{4})`).FindStringSubmatch(comment); len(yearMatch) > 1 {
						if y, err := strconv.Atoi(yearMatch[1]); err == nil && y > 1980 {
							year = y
						}
					}
					
					date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
					return date, dayNum, nil
				}
			}
		}
	}
	
	return time.Time{}, 0, fmt.Errorf("no date pattern found")
}

// ParseFile parses a single nodelist file and returns the nodes
func (ap *AdvancedParser) ParseFile(filePath string) ([]database.Node, error) {
	if ap.verbose {
		fmt.Printf("Parsing file: %s\n", filepath.Base(filePath))
	}

	// Read file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	var nodes []database.Node
	scanner := bufio.NewScanner(file)
	lineNum := 0
	var nodelistDate time.Time
	var dayNumber int
	var fileCRC uint16
	var firstNodeLine string
	headerParsed := false

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines  
		if line == "" {
			continue
		}

		// Handle header line with advanced parsing
		if strings.HasPrefix(line, ";A ") && !headerParsed {
			var err error
			nodelistDate, dayNumber, fileCRC, err = ap.parseHeaderLine(line)
			if err != nil {
				if ap.verbose {
					fmt.Printf("  Header parsing failed for %s: %v\n", filepath.Base(filePath), err)
					fmt.Printf("  Falling back to filename extraction\n")
				}
				// Fallback to date extraction
				nodelistDate, dayNumber, err = ap.extractDateFromFile(filePath)
				if err != nil {
					return nil, fmt.Errorf("failed to parse header and extract date from %s: %w", filePath, err)
				}
				if ap.verbose {
					fmt.Printf("  Filename extraction result: %s (Day %d)\n", nodelistDate.Format("2006-01-02"), dayNumber)
				}
			} else {
				if ap.verbose {
					fmt.Printf("  Header parsing successful: %s (Day %d, CRC %d)\n", nodelistDate.Format("2006-01-02"), dayNumber, fileCRC)
				}
			}
			headerParsed = true
			continue
		}

		// Skip other comment lines
		if strings.HasPrefix(line, ";") {
			continue
		}

		// Detect format on first node line
		if firstNodeLine == "" && !strings.HasPrefix(line, ";") {
			firstNodeLine = line
			ap.DetectedFormat = ap.detectFormat(line, firstNodeLine)
			if ap.verbose {
				fmt.Printf("Detected format: %v\n", ap.DetectedFormat)
			}
		}

		// Parse node line with advanced features
		node, err := ap.parseAdvancedLine(line, nodelistDate, dayNumber, filePath, fileCRC)
		if err != nil {
			if ap.verbose {
				fmt.Printf("Warning: Failed to parse line %d in %s (Full path: %s): %v\n", lineNum, filepath.Base(filePath), filePath, err)
			}
			continue // Skip malformed lines
		}

		if node != nil {
			nodes = append(nodes, *node)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	if ap.verbose {
		fmt.Printf("Parsed %d nodes from %s (Format: %v)\n", len(nodes), filepath.Base(filePath), ap.DetectedFormat)
		fmt.Printf("  File: %s\n", filePath)
		fmt.Printf("  Date: %s (Day %d)\n", nodelistDate.Format("2006-01-02"), dayNumber)
	}

	return nodes, nil
}

// ParseNodelistFile is a convenience function to parse a nodelist file using the advanced parser
func ParseNodelistFile(filePath string, verbose bool) ([]database.Node, error) {
	parser := NewAdvanced(verbose)
	return parser.ParseFile(filePath)
}