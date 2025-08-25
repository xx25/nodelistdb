package parser

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
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
	ModernFlagMap  map[string]flags.ParserFlagInfo

	// Header parsing patterns
	HeaderPattern *regexp.Regexp
	LinePattern   *regexp.Regexp

	// Context tracking
	Context Context

	// Reusable maps to reduce allocations (performance optimization)
	nodeTracker       map[string][]int // key: "zone:net/node", value: slice of indices
	
	// Pre-compiled regex patterns for common operations
	crcPattern *regexp.Regexp
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
			"XP:": "XA", // Extended addressing
			"MO:": "MO", // Mail Only
			"LO:": "LO", // Local Only
			"CM:": "CM", // Continuous Mail
		},
		ModernFlagMap: flags.GetParserFlagMap(),
		
		// Initialize reusable maps with reasonable starting capacity
		nodeTracker:       make(map[string][]int, 1000),
		
		// Pre-compile commonly used regex patterns
		crcPattern: regexp.MustCompile(`CRC-?(\w+)`),
	}
}

// NewAdvanced creates a new parser (kept for compatibility, just returns New)
func NewAdvanced(verbose bool) *Parser {
	return New(verbose)
}

// clearReusableMaps resets all reusable maps for the next parsing operation
// This prevents memory allocations by reusing existing map capacity
func (p *Parser) clearReusableMaps() {
	// Clear nodeTracker map but keep capacity
	for k := range p.nodeTracker {
		delete(p.nodeTracker, k)
	}
}

// estimateNodeCount estimates the number of nodes in a file for slice pre-allocation
// This reduces memory reallocations during parsing by providing a reasonable capacity estimate
func (p *Parser) estimateNodeCount(filePath string) int {
	// Get file info for size-based estimation
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		// Default estimate for unknown files
		return 1000
	}
	
	fileSize := fileInfo.Size()
	
	// Handle compressed files (rough estimation)
	if strings.HasSuffix(strings.ToLower(filePath), ".gz") {
		// Assume ~3:1 compression ratio for text
		fileSize *= 3
	}
	
	// Estimate based on average line length
	// Typical nodelist line is ~80-120 characters
	// Account for header lines and comments (~10% overhead)
	avgLineLength := int64(100)
	estimatedLines := fileSize / avgLineLength
	
	// Approximately 90% of lines are actual node entries
	estimatedNodes := int(float64(estimatedLines) * 0.9)
	
	// Reasonable bounds
	if estimatedNodes < 100 {
		estimatedNodes = 100
	} else if estimatedNodes > 100000 {
		estimatedNodes = 100000
	}
	
	if p.verbose {
		fmt.Printf("  Estimated %d nodes (file size: %d bytes)\n", estimatedNodes, fileInfo.Size())
	}
	
	return estimatedNodes
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
	// Clear reusable maps at start of parsing to reuse capacity
	p.clearReusableMaps()
	
	if p.verbose {
		fmt.Printf("Parsing file: %s\n", filepath.Base(filePath))
	}
	
	// Estimate node count for slice pre-allocation
	estimatedNodes := p.estimateNodeCount(filePath)

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

	// Read file (with gzip support)
	file, err := os.Open(filePath)
	if err != nil {
		return nil, NewFileError(filePath, "open", "failed to open file", err)
	}
	defer file.Close()

	// Create reader that handles both regular and gzipped files
	var reader io.Reader = file
	if strings.HasSuffix(strings.ToLower(filePath), ".gz") {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, NewFileError(filePath, "gzip", "failed to create gzip reader", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	// Pre-allocate nodes slice with estimated capacity for better performance
	nodes := make([]database.Node, 0, estimatedNodes)
	scanner := bufio.NewScanner(reader)
	lineNum := 0
	var nodelistDate time.Time
	var dayNumber int
	var fileCRC uint16
	var firstNodeLine string
	headerParsed := false

	// Track duplicates within this file (reusing parser's nodeTracker map)
	nodeTracker := p.nodeTracker
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
			// Extract CRC if present (using pre-compiled regex)
			if crcMatch := p.crcPattern.FindStringSubmatch(line); len(crcMatch) > 1 {
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
		return nil, NewFileError(filePath, "read", "error reading file", err)
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
	maxSpeedStr := strings.TrimSpace(fields[6])
	var maxSpeed uint32
	if maxSpeedStr != "" {
		if speed, err := strconv.ParseUint(maxSpeedStr, 10, 32); err == nil {
			maxSpeed = uint32(speed)
		}
		// If parsing fails, maxSpeed remains 0 (default)
	}

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
		if nodeNum, err := ParseInt("node", nodeNumStr); err == nil {
			node = nodeNum
		} else {
			return nil, err
		}
	} else {
		// Handle special node types
		switch strings.Title(strings.ToLower(nodeTypeStr)) {
		case "Zone":
			nodeType = "Zone"
			if z, err := ParseInt("zone", nodeNumStr); err == nil {
				zone = z
				net = z                   // Zone nodes have net = zone
				node = 0                  // Zone coordinator
				p.Context.CurrentZone = z // Update context
				p.Context.CurrentNet = z
			} else {
				return nil, err
			}
		case "Region":
			nodeType = "Region"
			zone = p.Context.CurrentZone
			if r, err := ParseInt("region", nodeNumStr); err == nil {
				net = r
				node = 0 // Region coordinator
				p.Context.CurrentNet = r
				regionNum := r
				p.Context.CurrentRegion = &regionNum
			} else {
				return nil, err
			}
		case "Host":
			nodeType = "Host"
			zone = p.Context.CurrentZone
			if n, err := ParseInt("net", nodeNumStr); err == nil {
				net = n
				node = 0 // Host = Net/0
				p.Context.CurrentNet = n
			} else {
				return nil, err
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
	flags, internetConfig := p.parseFlagsWithConfig(flagsStr)

	// Get modem flags separately (not included in parseFlagsWithConfig yet)
	_, _, _, _, _, modemFlags := p.parseAdvancedFlags(flagsStr)

	// Compute boolean flags based on comprehensive flag analysis
	isCM := p.hasFlag(flags, "CM")
	isMO := p.hasFlag(flags, "MO")
	
	// Determine internet connectivity from JSON config
	hasInet := len(internetConfig) > 0 && string(internetConfig) != "null"

	// Create node with all enhanced data
	dbNode := database.Node{
		Zone:              zone,
		Net:               net,
		Node:              node,
		NodelistDate:      nodelistDate,
		DayNumber:         dayNumber,
		SystemName:        systemName,
		Location:          location,
		SysopName:         sysopName,
		Phone:             phone,
		NodeType:          nodeType,
		Region:            region,
		MaxSpeed:          maxSpeed,
		IsCM:     isCM,
		IsMO:     isMO,
		HasInet:  hasInet,
		Flags:          flags,
		ModemFlags:     modemFlags,
		InternetConfig: internetConfig,
		ConflictSequence:  0,     // Default to 0 (original entry)
		HasConflict:       false, // Default to false
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
func (p *Parser) parseFlagsWithConfig(flagsStr string) ([]string, []byte) {
	// Pre-allocate flags slice with typical capacity
	flags := make([]string, 0, 10)

	// Create new maps for each node to avoid cross-contamination
	protocols := make(map[string]database.InternetProtocolDetail, 10)
	defaults := make(map[string]string, 5)
	emailProtocols := make(map[string]database.EmailProtocolDetail, 3)
	infoFlags := make([]string, 0, 3) // Pre-allocate with typical capacity

	// Default ports for protocols
	defaultPorts := map[string]int{
		"IBN": 24554, // BinkP
		"ITN": 23,    // Telnet
		"IFC": 60179, // EMSI over TCP
		"IFT": 21,    // FTP
	}

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
			flagName := strings.TrimSpace(part[:colonIndex])
			flagValue := strings.TrimSpace(part[colonIndex+1:])

			switch flagName {
			// Connection protocols
			case "IBN", "IFC", "ITN", "IVM", "IFT":
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
					protocols[flagName] = detail
				}

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
				emailDetail := database.EmailProtocolDetail{}
				if flagValue != "" {
					emailDetail.Email = flagValue
				}
				emailProtocols[flagName] = emailDetail

			// General IP flag
			case "IP":
				if flagValue != "" {
					addr, port := p.parseProtocolValue(flagValue)
					detail := database.InternetProtocolDetail{}
					if addr != "" {
						detail.Address = addr
					}
					if port > 0 {
						detail.Port = port
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
				if _, exists := protocols["IBN"]; !exists {
					protocols["IBN"] = database.InternetProtocolDetail{Port: 24554}
				}

			case "TEL": // Alternative name for ITN
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

	return flags, internetConfig
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

	// Create reader that handles both regular and gzipped files
	var reader io.Reader = file
	if strings.HasSuffix(strings.ToLower(filePath), ".gz") {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return time.Time{}, 0, fmt.Errorf("failed to create gzip reader for %s: %w", filePath, err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	scanner := bufio.NewScanner(reader)
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
		"may":  5,
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
