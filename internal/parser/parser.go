package parser

import (
	"bufio"
	"encoding/json"
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

// ParseResult contains the parsed nodes and metadata
type ParseResult struct {
	Nodes         []database.Node
	FilePath      string
	NodelistDate  time.Time
	DayNumber     int
	FileCRC       uint16
	ProcessedDate time.Time
}

// Context tracks the current parsing context
type Context struct {
	CurrentZone   int
	CurrentNet    int  
	CurrentRegion *int
}

// Parser handles FidoNet nodelist file parsing with all advanced features
type Parser struct {
	// Configuration
	verbose bool
	
	// Format detection
	DetectedFormat NodelistFormat
	LegacyFlagMap  map[string]string
	ModernFlagMap  map[string]FlagInfo
	
	// Header parsing patterns
	HeaderPattern  *regexp.Regexp
	LinePattern    *regexp.Regexp
	
	// Context tracking
	Context        Context
}

// FlagInfo contains metadata about flag types
type FlagInfo struct {
	Category    string // modem, internet, capability, schedule, user
	HasValue    bool   // whether flag takes a parameter
	Description string
}

// New creates a new parser instance
func New(verbose bool) *Parser {
	return &Parser{
		verbose: verbose,
		Context: Context{
			CurrentZone: 1, // Default to Zone 1
			CurrentNet:  1,
		},
		HeaderPattern: regexp.MustCompile(`^;[AST]\s+(.+)$`),
		LinePattern:   regexp.MustCompile(`^([^,]*),([^,]+),(.+)$`),
		LegacyFlagMap: map[string]string{
			"XP:": "XA",  // Extended addressing
			"MO:": "MO",  // Mail Only
			"LO:": "LO",  // Local Only
			"CM:": "CM",  // Continuous Mail
		},
		ModernFlagMap: map[string]FlagInfo{
			// Modem flags
			"V21":  {Category: "modem", HasValue: false, Description: "ITU-T V.21 (300 bps, full-duplex, FSK modulation)"},
			"V22":  {Category: "modem", HasValue: false, Description: "ITU-T V.22 (1200 bps, full-duplex, QAM modulation)"},
			"V29":  {Category: "modem", HasValue: false, Description: "ITU-T V.29 (9600 bps, half-duplex, used for fax and data)"},
			"V32":  {Category: "modem", HasValue: false, Description: "ITU-T V.32 (9600 bps, full-duplex, QAM modulation)"},
			"V32B": {Category: "modem", HasValue: false, Description: "ITU-T V.32bis (14400 bps, full-duplex, QAM modulation)"},
			"V33":  {Category: "modem", HasValue: false, Description: "ITU-T V.33 (14400 bps, half-duplex, data/fax transmission)"},
			"V34":  {Category: "modem", HasValue: false, Description: "ITU-T V.34 (up to 28800 bps, full-duplex, advanced QAM modulation)"},
			"V42":  {Category: "modem", HasValue: false, Description: "ITU-T V.42 (LAPM error correction protocol)"},
			"V42B": {Category: "modem", HasValue: false, Description: "ITU-T V.42bis (data compression, up to 4:1 ratio)"},
			"V90C": {Category: "modem", HasValue: false, Description: "ITU-T V.90 (56 kbps, client side, analog download)"},
			"V90S": {Category: "modem", HasValue: false, Description: "ITU-T V.90 (56 kbps, server side, digital upload)"},
			"X75":  {Category: "modem", HasValue: false, Description: "ITU-T X.75 (ISDN B-channel protocol, 64 kbps)"},
			"HST":  {Category: "modem", HasValue: false, Description: "USRobotics HST (High-Speed Transfer, proprietary, 9600-14400 bps)"},
			"H96":  {Category: "modem", HasValue: false, Description: "USRobotics HST 9600 (early HST modem, 9600 bps)"},
			"H14":  {Category: "modem", HasValue: false, Description: "USRobotics HST 14400 (improved speed variant, 14400 bps)"},
			"H16":  {Category: "modem", HasValue: false, Description: "USRobotics HST 16800 (advanced speed variant, 16800 bps)"},
			"MAX":  {Category: "modem", HasValue: false, Description: "Microcom AX/96xx series (proprietary modulation, 9600 bps+)"},
			"PEP":  {Category: "modem", HasValue: false, Description: "Packet Ensemble Protocol (proprietary error correction and modulation)"},
			"CSP":  {Category: "modem", HasValue: false, Description: "Compucom SpeedModem (CSP, proprietary protocol)"},
			"ZYX":  {Category: "modem", HasValue: false, Description: "ZyXEL modem (supports proprietary and standard protocols)"},
			"VFC":  {Category: "modem", HasValue: false, Description: "V.Fast Class (V.FC, pre-V.34 28800 bps, Rockwell standard)"},
			
			// Internet flags
			"IBN":  {Category: "internet", HasValue: true, Description: "BinkP"},
			"IFC":  {Category: "internet", HasValue: true, Description: "File transfer"},
			"ITN":  {Category: "internet", HasValue: true, Description: "Telnet"},
			"IVM":  {Category: "internet", HasValue: true, Description: "VModem"},
			"IFT":  {Category: "internet", HasValue: true, Description: "FTP"},
			"INA":  {Category: "internet", HasValue: true, Description: "Internet address"},
			"IP":   {Category: "internet", HasValue: true, Description: "General IP"},
			
			// Email protocols
			"IEM":  {Category: "internet", HasValue: true, Description: "Email"},
			"IMI":  {Category: "internet", HasValue: true, Description: "Mail interface"},
			"ITX":  {Category: "internet", HasValue: true, Description: "TransX"},
			"IUC":  {Category: "internet", HasValue: true, Description: "UUencoded"},
			"ISE":  {Category: "internet", HasValue: true, Description: "SendEmail"},
			
			// Capability flags
			"CM":   {Category: "capability", HasValue: false, Description: "Continuous Mail"},
			"MO":   {Category: "capability", HasValue: false, Description: "Mail Only"},
			"LO":   {Category: "capability", HasValue: false, Description: "Local Only"},
			"XA":   {Category: "capability", HasValue: false, Description: "Extended addressing"},
			"XB":   {Category: "capability", HasValue: false, Description: "Bark requests"},
			"XC":   {Category: "capability", HasValue: false, Description: "Compressed mail"},
			"XP":   {Category: "capability", HasValue: false, Description: "Extended protocol"},
			"XR":   {Category: "capability", HasValue: false, Description: "Accepts file requests"},
			"XW":   {Category: "capability", HasValue: false, Description: "X.75 windowing"},
			"XX":   {Category: "capability", HasValue: false, Description: "No file/update requests"},
			
			// Schedule flags
			"U":    {Category: "schedule", HasValue: true, Description: "Availability"},
			"T":    {Category: "schedule", HasValue: true, Description: "Time zone"},
			
			// User flags
			"ENC":  {Category: "user", HasValue: false, Description: "Encrypted"},
			"NC":   {Category: "user", HasValue: false, Description: "Network Coordinator"},
			"NEC":  {Category: "user", HasValue: false, Description: "Net Echomail Coordinator"},
			"REC":  {Category: "user", HasValue: false, Description: "Region Echomail Coordinator"},
			"ZEC":  {Category: "user", HasValue: false, Description: "Zone Echomail Coordinator"},
			"PING": {Category: "user", HasValue: false, Description: "Ping OK"},
			"RPK":  {Category: "user", HasValue: false, Description: "Regional Pointlist Keeper"},
		},
	}
}

// NewAdvanced creates a new parser (kept for compatibility, just returns New)
func NewAdvanced(verbose bool) *Parser {
	return New(verbose)
}

// ParseFile parses a single nodelist file and returns nodes
func (p *Parser) ParseFile(filePath string) ([]database.Node, error) {
	result, err := p.ParseFileWithCRC(filePath)
	if err != nil {
		return nil, err
	}
	return result.Nodes, nil
}

// ParseFileWithCRC parses a single nodelist file and returns nodes with file CRC
func (p *Parser) ParseFileWithCRC(filePath string) (*ParseResult, error) {
	if p.verbose {
		fmt.Printf("Parsing file: %s\n", filepath.Base(filePath))
	}

	// Extract year from path to determine default zone
	year := p.extractYearFromPath(filePath)
	if year >= 1987 {
		// For 1987+ nodelists, default to zone 2 if no explicit zone is found
		p.Context.CurrentZone = 2
		p.Context.CurrentNet = 2
		if p.verbose {
			fmt.Printf("  Year %d detected: defaulting to Zone 2 for nodelists without explicit zone declaration\n", year)
		}
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
	
	// Track duplicates within this file
	nodeTracker := make(map[string][]int) // key: "zone:net/node", value: slice of indices in nodes array
	duplicateStats := struct {
		totalDuplicates int
		conflictGroups  int
	}{}

	for scanner.Scan() {
		lineNum++
		rawLine := scanner.Text()
		line := strings.TrimSpace(rawLine)

		// Check for EOF markers (^Z, Ctrl+Z)
		if strings.Contains(rawLine, "\x1a") || strings.Contains(rawLine, "\u001a") {
			// Stop processing at EOF marker
			break
		}

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse header comments
		if strings.HasPrefix(line, ";A") || strings.HasPrefix(line, ";S") {
			if !headerParsed {
				date, dayNum, err := p.extractDateFromLine(line)
				if err == nil {
					nodelistDate = date
					dayNumber = dayNum
					headerParsed = true
					if p.verbose {
						fmt.Printf("  Header parsing successful: %s (Day %d, CRC %d)\n", 
							nodelistDate.Format("2006-01-02"), dayNumber, fileCRC)
					}
				}
			}
			// Extract CRC if present
			if crcMatch := regexp.MustCompile(`CRC-?(\w+)`).FindStringSubmatch(line); len(crcMatch) > 1 {
				if crc, err := strconv.ParseUint(crcMatch[1], 16, 16); err == nil {
					fileCRC = uint16(crc)
				}
			}
			continue
		}

		// Skip other comment lines
		if strings.HasPrefix(line, ";") {
			continue
		}

		// Detect format on first node line
		if firstNodeLine == "" && !strings.HasPrefix(line, ";") {
			firstNodeLine = line
			p.DetectedFormat = p.detectFormat(line, firstNodeLine)
			if p.verbose {
				fmt.Printf("Detected format: %v\n", p.DetectedFormat)
			}
		}

		// Parse node line with advanced features
		node, err := p.parseLine(line, nodelistDate, dayNumber, filePath)
		if err != nil {
			if p.verbose {
				fmt.Printf("Warning: Failed to parse line %d in %s (Full path: %s): %v\n", lineNum, filepath.Base(filePath), filePath, err)
			}
			continue // Skip malformed lines
		}

		if node != nil {
			// Check for duplicates within this file
			nodeKey := fmt.Sprintf("%d:%d/%d", node.Zone, node.Net, node.Node)
			
			if existingIndices, exists := nodeTracker[nodeKey]; exists {
				// This is a duplicate - handle conflict tracking
				if p.verbose {
					fmt.Printf("  DUPLICATE DETECTED: Node %s appears multiple times in %s (line %d)\n", 
						nodeKey, filePath, lineNum)
					fmt.Printf("    Previous occurrences at indices: %v\n", existingIndices)
					fmt.Printf("    System Name: '%s', Location: '%s'\n", node.SystemName, node.Location)
				}
				
				// Set conflict sequence for this duplicate
				node.ConflictSequence = len(existingIndices)
				node.HasConflict = true
				
				// Mark all previous occurrences as having conflicts
				for _, idx := range existingIndices {
					nodes[idx].HasConflict = true
				}
				
				duplicateStats.totalDuplicates++
				if len(existingIndices) == 1 {
					// First duplicate for this node
					duplicateStats.conflictGroups++
				}
				
				// Add current index to tracker
				nodeTracker[nodeKey] = append(existingIndices, len(nodes))
			} else {
				// First occurrence - add to tracker
				nodeTracker[nodeKey] = []int{len(nodes)}
			}
			
			nodes = append(nodes, *node)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	// Fallback: extract date from filename if not found in header
	if nodelistDate.IsZero() {
		nodelistDate, dayNumber, _ = p.extractDateFromFile(filePath)
	}

	if p.verbose {
		fmt.Printf("Parsed %d nodes from %s (Format: %v)\n", len(nodes), filepath.Base(filePath), p.DetectedFormat)
		fmt.Printf("  File: %s\n", filePath)
		fmt.Printf("  Date: %s (Day %d)\n", nodelistDate.Format("2006-01-02"), dayNumber)
		
		if duplicateStats.totalDuplicates > 0 {
			fmt.Printf("  ⚠️  DUPLICATES FOUND: %d duplicate entries across %d nodes\n", 
				duplicateStats.totalDuplicates, duplicateStats.conflictGroups)
			fmt.Printf("     These duplicates have been preserved with conflict tracking\n")
		} else {
			fmt.Printf("  ✓ No duplicate node addresses found in this file\n")
		}
	}

	return &ParseResult{
		Nodes:         nodes,
		FilePath:      filePath,
		NodelistDate:  nodelistDate,
		DayNumber:     dayNumber,
		FileCRC:       fileCRC,
		ProcessedDate: time.Now(),
	}, nil
}

// detectFormat analyzes line patterns to determine nodelist format
func (p *Parser) detectFormat(line string, firstLine string) NodelistFormat {
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

// parseLine parses a single nodelist entry line
func (p *Parser) parseLine(line string, nodelistDate time.Time, dayNumber int, filePath string) (*database.Node, error) {
	
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

	systemName := strings.TrimSpace(fields[2])
	location := strings.TrimSpace(fields[3])
	sysopName := strings.TrimSpace(fields[4])
	phone := strings.TrimSpace(fields[5])
	maxSpeed := strings.TrimSpace(fields[6])

	// Parse flags (field 7 and beyond)
	var flagsStr string
	if len(fields) > 7 {
		flagsStr = strings.Join(fields[7:], ",")
	}

	// Apply legacy flag conversions if needed
	if p.DetectedFormat == Format1986 {
		flagsStr = p.convertLegacyFlags(flagsStr)
	}

	// Determine node type and update context
	var nodeType string
	var zone, net, node int
	var region *int

	if nodeTypeStr == "" {
		// Empty first field = normal node
		nodeType = "Node"
		zone = p.Context.CurrentZone
		net = p.Context.CurrentNet
		if nodeNum, err := strconv.Atoi(nodeNumStr); err == nil {
			node = nodeNum
		} else {
			return nil, fmt.Errorf("invalid node number: %s", nodeNumStr)
		}
	} else {
		// Handle special node types
		switch strings.Title(strings.ToLower(nodeTypeStr)) {
		case "Zone":
			nodeType = "Zone"
			if z, err := strconv.Atoi(nodeNumStr); err == nil {
				zone = z
				net = z  // Zone nodes have net = zone
				node = 0 // Zone coordinator
				p.Context.CurrentZone = z // Update context
				p.Context.CurrentNet = z
			} else {
				return nil, fmt.Errorf("invalid zone number: %s", nodeNumStr)
			}
		case "Region":
			nodeType = "Region"
			zone = p.Context.CurrentZone
			if r, err := strconv.Atoi(nodeNumStr); err == nil {
				net = r
				node = 0 // Region coordinator
				p.Context.CurrentNet = r
				regionNum := r
				p.Context.CurrentRegion = &regionNum
			} else {
				return nil, fmt.Errorf("invalid region number: %s", nodeNumStr)
			}
		case "Host":
			nodeType = "Host"
			zone = p.Context.CurrentZone
			if n, err := strconv.Atoi(nodeNumStr); err == nil {
				net = n
				node = 0 // Host = Net/0
				p.Context.CurrentNet = n
			} else {
				return nil, fmt.Errorf("invalid host net number: %s", nodeNumStr)
			}
		case "Hub":
			nodeType = "Hub"
			zone = p.Context.CurrentZone
			net = p.Context.CurrentNet
			if nodeNum, err := strconv.Atoi(nodeNumStr); err == nil {
				node = nodeNum
			} else {
				return nil, fmt.Errorf("invalid hub node number: %s", nodeNumStr)
			}
		case "Pvt", "Hold", "Down":
			nodeType = strings.Title(strings.ToLower(nodeTypeStr))
			zone = p.Context.CurrentZone
			net = p.Context.CurrentNet
			if nodeNum, err := strconv.Atoi(nodeNumStr); err == nil {
				node = nodeNum
			} else {
				return nil, fmt.Errorf("invalid %s node number: %s", strings.ToLower(nodeTypeStr), nodeNumStr)
			}
		default:
			return nil, fmt.Errorf("unknown node type: %s", nodeTypeStr)
		}
	}

	// Copy context region if available
	if p.Context.CurrentRegion != nil && region == nil {
		region = p.Context.CurrentRegion
	}

	// Parse flags into structured format with full categorization AND build JSON config
	flags, internetProtocols, internetHostnames, internetPorts, internetEmails, internetConfig := p.parseFlagsWithConfig(flagsStr)
	
	// Get modem flags separately (not included in parseFlagsWithConfig yet)
	_, _, _, _, _, modemFlags := p.parseAdvancedFlags(flagsStr)
	
	
	// Compute boolean flags based on comprehensive flag analysis
	isCM := p.hasFlag(flags, "CM")
	isMO := p.hasFlag(flags, "MO") 
	hasBinkp := p.hasProtocol(internetProtocols, "IBN") || p.hasProtocol(internetProtocols, "BND") || p.hasFlag(flags, "IBN") || p.hasFlag(flags, "BND")
	hasTelnet := p.hasProtocol(internetProtocols, "ITN") || p.hasProtocol(internetProtocols, "TEL") || p.hasFlag(flags, "ITN") || p.hasFlag(flags, "TEL")
	hasInet := len(internetProtocols) > 0 || len(internetEmails) > 0 || hasBinkp || hasTelnet
	isDown := nodeType == "Down"
	isHold := nodeType == "Hold"
	isPvt := nodeType == "Pvt"
	isActive := !isDown && !isHold

	// Create node with all enhanced data
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
		HasInet:      hasInet,
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
		InternetConfig:    internetConfig,
		ConflictSequence: 0,    // Default to 0 (original entry)
		HasConflict:      false, // Default to false
	}

	return &dbNode, nil
}

// parseProtocolValue determines if a value is an address, port, or both
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
		// Make sure it's not part of IPv6 without brackets
		if strings.Count(value[:lastColon], ":") == 1 {
			// Standard host:port
			possiblePort := value[lastColon+1:]
			if p, err := strconv.Atoi(possiblePort); err == nil && p > 0 && p < 65536 {
				return value[:lastColon], p
			}
		}
	}
	
	// It's just an address (hostname, IPv4, or unbracketed IPv6)
	return value, 0
}

// buildInternetConfig builds the JSON configuration from parsed flag data
func (p *Parser) buildInternetConfig(protocols map[string]database.InternetProtocolDetail, 
	defaults map[string]string, 
	emailProtocols map[string]database.EmailProtocolDetail,
	infoFlags []string) []byte {
	
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

// parseFlagsWithConfig extracts flags and builds structured internet configuration
func (p *Parser) parseFlagsWithConfig(flagsStr string) ([]string, []string, []string, []int, []string, []byte) {
	var flags []string
	var internetProtocols []string
	var internetHostnames []string
	var internetPorts []int
	var internetEmails []string
	
	// For building structured config
	protocols := make(map[string]database.InternetProtocolDetail)
	defaults := make(map[string]string)
	emailProtocols := make(map[string]database.EmailProtocolDetail)
	var infoFlags []string
	
	// Default ports for protocols
	defaultPorts := map[string]int{
		"IBN": 24554,  // BinkP
		"ITN": 23,     // Telnet
		"IFC": 60179,  // EMSI over TCP
		"IFT": 21,     // FTP
	}

	if flagsStr == "" {
		return flags, internetProtocols, internetHostnames, internetPorts, internetEmails, nil
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
			flagName := strings.TrimSpace(part[:colonIndex])
			flagValue := strings.TrimSpace(part[colonIndex+1:])
			
			switch flagName {
			// Connection protocols
			case "IBN", "IFC", "ITN", "IVM", "IFT":
				internetProtocols = append(internetProtocols, flagName)
				
				detail := database.InternetProtocolDetail{}
				if flagValue != "" {
					addr, port := p.parseProtocolValue(flagValue)
					if addr != "" {
						detail.Address = addr
						internetHostnames = append(internetHostnames, addr)
					}
					if port > 0 {
						detail.Port = port
						internetPorts = append(internetPorts, port)
					} else if defaultPort, ok := defaultPorts[flagName]; ok && addr != "" {
						detail.Port = defaultPort
					}
				}
				if detail.Address != "" || detail.Port > 0 {
					protocols[flagName] = detail
				}
				
			// Default internet address
			case "INA":
				internetProtocols = append(internetProtocols, flagName)
				if flagValue != "" {
					defaults["INA"] = flagValue
					internetHostnames = append(internetHostnames, flagValue)
				}
				
			// Email protocols
			case "IEM":
				if flagValue != "" {
					defaults["IEM"] = flagValue // Default email
					internetEmails = append(internetEmails, flagValue)
				}
				
			case "IMI", "ITX", "ISE":
				emailDetail := database.EmailProtocolDetail{}
				if flagValue != "" {
					emailDetail.Email = flagValue
					internetEmails = append(internetEmails, flagValue)
				}
				emailProtocols[flagName] = emailDetail
				
			// General IP flag
			case "IP":
				internetProtocols = append(internetProtocols, flagName)
				if flagValue != "" {
					addr, port := p.parseProtocolValue(flagValue)
					detail := database.InternetProtocolDetail{}
					if addr != "" {
						detail.Address = addr
						internetHostnames = append(internetHostnames, addr)
					}
					if port > 0 {
						detail.Port = port
						internetPorts = append(internetPorts, port)
					}
					if detail.Address != "" || detail.Port > 0 {
						protocols[flagName] = detail
					}
				}
				
			// User flags with values (U:time, T:zone)
			case "U", "T", "Tyz":
				flags = append(flags, part)
				
			default:
				// Unknown flag with parameter - store as-is
				flags = append(flags, part)
			}
		} else {
			// Simple flags without parameters
			switch part {
			// Connection protocol flags without values
			case "IBN", "IFC", "ITN", "IVM", "IFT", "INA", "IP":
				internetProtocols = append(internetProtocols, part)
				// Add to protocols map with default port if applicable
				if defaultPort, ok := defaultPorts[part]; ok {
					protocols[part] = database.InternetProtocolDetail{Port: defaultPort}
				} else {
					protocols[part] = database.InternetProtocolDetail{}
				}
				
			// Email protocol flags without values
			case "IMI", "ITX", "ISE", "IUC", "EMA", "EVY":
				emailProtocols[part] = database.EmailProtocolDetail{}
				
			// Information flags
			case "INO4", "INO6", "ICM":
				infoFlags = append(infoFlags, part)
				
			// Alternative protocol names
			case "BND": // Alternative name for IBN
				internetProtocols = append(internetProtocols, "IBN")
				if _, exists := protocols["IBN"]; !exists {
					protocols["IBN"] = database.InternetProtocolDetail{Port: 24554}
				}
				
			case "TEL": // Alternative name for ITN
				internetProtocols = append(internetProtocols, "ITN")
				if _, exists := protocols["ITN"]; !exists {
					protocols["ITN"] = database.InternetProtocolDetail{Port: 23}
				}
				
			default:
				// Regular flag
				flags = append(flags, part)
			}
		}
	}

	// Build JSON config
	internetConfig := p.buildInternetConfig(protocols, defaults, emailProtocols, infoFlags)
	
	return flags, internetProtocols, internetHostnames, internetPorts, internetEmails, internetConfig
}

// parseAdvancedFlags parses flags with full categorization (kept for modem flags)
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
		if info, exists := p.ModernFlagMap[strings.Split(part, ":")[0]]; exists {
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

// convertLegacyFlags converts 1986-style flags to modern equivalents
func (p *Parser) convertLegacyFlags(flagsStr string) string {
	for old, new := range p.LegacyFlagMap {
		flagsStr = strings.ReplaceAll(flagsStr, old, new)
	}
	return flagsStr
}

// Helper methods for flag checking
func (p *Parser) hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

func (p *Parser) hasProtocol(protocols []string, protocol string) bool {
	for _, p := range protocols {
		if strings.EqualFold(p, protocol) {
			return true
		}
	}
	return false
}

// Date extraction methods
func (p *Parser) extractDateFromLine(line string) (time.Time, int, error) {
	// Try various date patterns in header comments
	patterns := []struct {
		regex   *regexp.Regexp
		handler func([]string) (time.Time, int, error)
	}{
		// Modern format with 4-digit year: "Friday, 1 July 2022 -- Day number 182"
		{
			regexp.MustCompile(`(\w+),?\s+(\d{1,2})\s+(\w+)\s+(\d{4})\s+--\s+Day\s+number\s+(\d+)`),
			func(matches []string) (time.Time, int, error) {
				year, _ := strconv.Atoi(matches[4])
				day, _ := strconv.Atoi(matches[2])
				dayNum, _ := strconv.Atoi(matches[5])
				month := p.parseMonth(matches[3])
				if month == 0 {
					return time.Time{}, 0, fmt.Errorf("invalid month: %s", matches[3])
				}
				return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), dayNum, nil
			},
		},
		// Format with year in comment: "Day number 002 : Friday, January 2, 1998"
		{
			regexp.MustCompile(`Day\s+number\s+(\d+)\s*:\s*\w+,?\s+(\w+)\s+(\d{1,2}),?\s+(\d{4})`),
			func(matches []string) (time.Time, int, error) {
				dayNum, _ := strconv.Atoi(matches[1])
				day, _ := strconv.Atoi(matches[3])
				year, _ := strconv.Atoi(matches[4])
				month := p.parseMonth(matches[2])
				if month == 0 {
					return time.Time{}, 0, fmt.Errorf("invalid month: %s", matches[2])
				}
				return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), dayNum, nil
			},
		},
		// Old format without year: "Friday, 4 August 1989 -- Day number 216"
		{
			regexp.MustCompile(`(\w+),?\s+(\d{1,2})\s+(\w+)\s+--\s+Day\s+number\s+(\d+)`),
			func(matches []string) (time.Time, int, error) {
				day, _ := strconv.Atoi(matches[2])
				dayNum, _ := strconv.Atoi(matches[4])
				month := p.parseMonth(matches[3])
				if month == 0 {
					return time.Time{}, 0, fmt.Errorf("invalid month: %s", matches[3])
				}
				
				// Extract year from comment or filename context
				year := 1989 // Default for old nodelists
				if yearMatch := regexp.MustCompile(`(\d{4})`).FindStringSubmatch(line); len(yearMatch) > 1 {
					if y, err := strconv.Atoi(yearMatch[1]); err == nil && y > 1980 && y < 2100 {
						year = y
					}
				}
				
				return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), dayNum, nil
			},
		},
	}

	for _, pattern := range patterns {
		if matches := pattern.regex.FindStringSubmatch(line); len(matches) > 0 {
			return pattern.handler(matches)
		}
	}

	// Fallback: look for day number only
	if matches := regexp.MustCompile(`[Dd]ay\s+(?:number\s+)?(\d+)`).FindStringSubmatch(line); len(matches) > 1 {
		dayNum, _ := strconv.Atoi(matches[1])
		
		// Try to find year in the line
		year := 1989 // Default
		if yearMatch := regexp.MustCompile(`(\d{4})`).FindStringSubmatch(line); len(yearMatch) > 1 {
			if y, err := strconv.Atoi(yearMatch[1]); err == nil && y > 1980 && y < 2100 {
				year = y
			}
		}
		
		// Calculate date from day number
		date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
		return date, dayNum, nil
	}

	return time.Time{}, 0, fmt.Errorf("no date pattern found in line")
}

func (p *Parser) extractDateFromFile(filePath string) (time.Time, int, error) {
	filename := filepath.Base(filePath)
	
	// Try various filename patterns
	patterns := []struct {
		regex   *regexp.Regexp
		handler func([]string, string) (time.Time, int, error)
	}{
		// NODELIST.nnn format (where nnn is day of year)
		{
			regexp.MustCompile(`(?i)nodelist\.(\d{3})`),
			func(matches []string, path string) (time.Time, int, error) {
				dayNum, _ := strconv.Atoi(matches[1])
				year := p.extractYearFromPath(path)
				if year == 0 {
					year = 1989 // Default for old nodelists
				}
				date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
				return date, dayNum, nil
			},
		},
		// z1-nnn.yy format (zone-day.year)
		{
			regexp.MustCompile(`(?i)z\d+-(\d{3})\.(\d{2})`),
			func(matches []string, path string) (time.Time, int, error) {
				dayNum, _ := strconv.Atoi(matches[1])
				year, _ := strconv.Atoi(matches[2])
				if year < 50 {
					year += 2000
				} else {
					year += 1900
				}
				date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
				return date, dayNum, nil
			},
		},
		// nodelist_yyyy_ddd format
		{
			regexp.MustCompile(`(?i)nodelist[_-](\d{4})[_-](\d{3})`),
			func(matches []string, path string) (time.Time, int, error) {
				year, _ := strconv.Atoi(matches[1])
				dayNum, _ := strconv.Atoi(matches[2])
				date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
				return date, dayNum, nil
			},
		},
	}

	for _, pattern := range patterns {
		if matches := pattern.regex.FindStringSubmatch(filename); len(matches) > 0 {
			return pattern.handler(matches, filePath)
		}
	}

	// Fallback: read file header
	file, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() && lineCount < 20 {
		lineCount++
		line := scanner.Text()
		if strings.HasPrefix(line, ";A") || strings.HasPrefix(line, ";S") {
			if date, dayNum, err := p.extractDateFromLine(line); err == nil {
				return date, dayNum, nil
			}
		}
	}

	return time.Time{}, 0, fmt.Errorf("no date found in filename or header")
}

func (p *Parser) extractYearFromPath(filePath string) int {
	// Look for 4-digit year in path
	yearRe := regexp.MustCompile(`\b(19[8-9]\d|20[0-5]\d)\b`)
	if matches := yearRe.FindStringSubmatch(filePath); len(matches) > 1 {
		year, _ := strconv.Atoi(matches[1])
		return year
	}
	
	// Look for 2-digit year in path
	year2Re := regexp.MustCompile(`\b([89]\d|[0-5]\d)\b`)
	if matches := year2Re.FindStringSubmatch(filepath.Base(filePath)); len(matches) > 1 {
		year, _ := strconv.Atoi(matches[1])
		if year < 50 {
			return 2000 + year
		}
		return 1900 + year
	}
	
	return 0
}

func (p *Parser) parseMonth(monthStr string) int {
	monthStr = strings.ToLower(monthStr)
	months := map[string]int{
		"january": 1, "jan": 1,
		"february": 2, "feb": 2,
		"march": 3, "mar": 3,
		"april": 4, "apr": 4,
		"may": 5,
		"june": 6, "jun": 6,
		"july": 7, "jul": 7,
		"august": 8, "aug": 8,
		"september": 9, "sep": 9,
		"october": 10, "oct": 10,
		"november": 11, "nov": 11,
		"december": 12, "dec": 12,
	}
	return months[monthStr]
}

// sanitizeUTF8 ensures string is valid UTF-8 for database storage
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