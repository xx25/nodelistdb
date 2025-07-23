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

	"nodelistdb/internal/database"
)

// Parser handles FidoNet nodelist file parsing
type Parser struct {
	// Configuration
	verbose bool
	// Context tracking for Zone/Net hierarchy
	currentZone int
	currentNet  int
}

// New creates a new parser instance
func New(verbose bool) *Parser {
	return &Parser{
		verbose:     verbose,
		currentZone: 1, // Default zone (will be updated based on year in ParseFile)
		currentNet:  1, // Default net
	}
}

// ParseFile parses a single nodelist file and returns the nodes (DEPRECATED: Use NewAdvanced().ParseFile() instead)
func (p *Parser) ParseFile(filePath string) ([]database.Node, error) {
	// Delegate to advanced parser for consistency
	advParser := NewAdvanced(p.verbose)
	return advParser.ParseFile(filePath)
}

// extractDateFromFile extracts nodelist date from filename and file content
func (p *Parser) extractDateFromFile(filePath string) (time.Time, int, error) {
	filename := filepath.Base(filePath)
	
	// Pattern 1: nodelist.NNN (where NNN is day of year)
	if match := regexp.MustCompile(`nodelist\.(\d{3})$`).FindStringSubmatch(strings.ToLower(filename)); match != nil {
		dayNum, err := strconv.Atoi(match[1])
		if err != nil {
			return time.Time{}, 0, err
		}
		
		// Extract year from directory path
		year := p.extractYearFromPath(filePath)
		if year == 0 {
			year = 1986 // Default fallback
		}
		
		// Convert day number to date
		date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayNum-1)
		return date, dayNum, nil
	}

	// Pattern 2: Try to extract from file header
	if date, day, err := p.extractDateFromHeader(filePath); err == nil {
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
func (p *Parser) extractYearFromPath(filePath string) int {
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
func (p *Parser) extractDateFromHeader(filePath string) (time.Time, int, error) {
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
			if date, day, err := p.parseDateFromComment(line); err == nil {
				return date, day, nil
			}
		}
	}
	
	return time.Time{}, 0, fmt.Errorf("no date found in header")
}

// parseDateFromComment extracts date from comment lines
func (p *Parser) parseDateFromComment(comment string) (time.Time, int, error) {
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

// parseLine parses a single nodelist entry line
func (p *Parser) parseLine(line string, nodelistDate time.Time, dayNumber int, filePath string) (*database.Node, error) {
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

	// Determine node type and update context
	var nodeType string
	var zone, net, node int
	var region *int

	if nodeTypeStr == "" {
		// Empty first field = normal node
		nodeType = "Node"
		zone = p.currentZone
		net = p.currentNet
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
				p.currentZone = z // Update context
				p.currentNet = z
			} else {
				return nil, fmt.Errorf("invalid zone number: %s", nodeNumStr)
			}
		case "Region":
			nodeType = "Region"
			if r, err := strconv.Atoi(nodeNumStr); err == nil {
				regionNum := r
				region = &regionNum
				zone = p.currentZone
				net = r
				node = 0 // Regional coordinator
				p.currentNet = r // Update context
			} else {
				return nil, fmt.Errorf("invalid region number: %s", nodeNumStr)
			}
		case "Host":
			nodeType = "Host"
			if n, err := strconv.Atoi(nodeNumStr); err == nil {
				zone = p.currentZone
				net = n
				node = 0 // Host coordinator
				p.currentNet = n // Update context
			} else {
				return nil, fmt.Errorf("invalid host number: %s", nodeNumStr)
			}
		case "Hub":
			nodeType = "Hub"
			zone = p.currentZone
			net = p.currentNet
			if h, err := strconv.Atoi(nodeNumStr); err == nil {
				node = h
			} else {
				return nil, fmt.Errorf("invalid hub number: %s", nodeNumStr)
			}
		case "Pvt":
			nodeType = "Pvt"
			zone = p.currentZone
			net = p.currentNet
			if n, err := strconv.Atoi(nodeNumStr); err == nil {
				node = n
			} else {
				return nil, fmt.Errorf("invalid pvt node number: %s", nodeNumStr)
			}
		case "Down":
			nodeType = "Down"
			zone = p.currentZone
			net = p.currentNet
			if n, err := strconv.Atoi(nodeNumStr); err == nil {
				node = n
			} else {
				return nil, fmt.Errorf("invalid down node number: %s", nodeNumStr)
			}
		case "Hold":
			nodeType = "Hold"
			zone = p.currentZone
			net = p.currentNet
			if n, err := strconv.Atoi(nodeNumStr); err == nil {
				node = n
			} else {
				return nil, fmt.Errorf("invalid hold node number: %s", nodeNumStr)
			}
		default:
			return nil, fmt.Errorf("unknown node type: %s", nodeTypeStr)
		}
	}

	// Parse flags into structured format
	flags, internetProtocols, internetHostnames, internetPorts, internetEmails := p.parseFlags(flagsStr)
	
	// Compute boolean flags
	isCM := p.hasFlag(flags, "CM")
	isMO := p.hasFlag(flags, "MO") 
	hasBinkp := p.hasProtocol(internetProtocols, "IBN") || p.hasProtocol(internetProtocols, "BND") || p.hasFlag(flags, "IBN") || p.hasFlag(flags, "BND")
	hasTelnet := p.hasProtocol(internetProtocols, "ITN") || p.hasProtocol(internetProtocols, "TEL") || p.hasFlag(flags, "ITN") || p.hasFlag(flags, "TEL")
	isDown := nodeType == "Down"
	isHold := nodeType == "Hold"
	isPvt := nodeType == "Pvt"
	isActive := !isDown && !isHold

	// Create node
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
		ModemFlags:   []string{}, // TODO: Parse from flags
		InternetProtocols: internetProtocols,
		InternetHostnames: internetHostnames,
		InternetPorts:     internetPorts,
		InternetEmails:    internetEmails,
	}

	return &dbNode, nil
}

// parseFlags extracts individual flags from flag string and categorizes them
func (p *Parser) parseFlags(flagsStr string) ([]string, []string, []string, []int, []string) {
	if flagsStr == "" {
		return []string{}, []string{}, []string{}, []int{}, []string{}
	}
	
	var flags []string
	var internetProtocols []string
	var internetHostnames []string
	var internetPorts []int
	var internetEmails []string
	
	// Split by comma and handle complex flags like IBN:hostname:port
	parts := strings.Split(flagsStr, ",")
	
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		
		// Handle internet flags with parameters
		if strings.Contains(part, ":") {
			flagParts := strings.Split(part, ":")
			flagName := strings.TrimSpace(flagParts[0])
			
			switch flagName {
			case "IBN", "BND": // Binkp
				internetProtocols = append(internetProtocols, flagName)
				if len(flagParts) > 1 {
					hostname := strings.TrimSpace(flagParts[1])
					if hostname != "" {
						internetHostnames = append(internetHostnames, hostname)
					}
				}
				if len(flagParts) > 2 {
					if port, err := strconv.Atoi(strings.TrimSpace(flagParts[2])); err == nil {
						internetPorts = append(internetPorts, port)
					}
				}
			case "ITN", "TEL": // Telnet
				internetProtocols = append(internetProtocols, flagName)
				if len(flagParts) > 1 {
					hostname := strings.TrimSpace(flagParts[1])
					if hostname != "" {
						internetHostnames = append(internetHostnames, hostname)
					}
				}
				if len(flagParts) > 2 {
					if port, err := strconv.Atoi(strings.TrimSpace(flagParts[2])); err == nil {
						internetPorts = append(internetPorts, port)
					}
				}
			case "INA": // Internet Address (hostname)
				internetProtocols = append(internetProtocols, flagName)
				if len(flagParts) > 1 {
					hostname := strings.TrimSpace(flagParts[1])
					if hostname != "" {
						internetHostnames = append(internetHostnames, hostname)
					}
				}
			case "IEM": // Email
				if len(flagParts) > 1 {
					email := strings.TrimSpace(flagParts[1])
					if email != "" {
						internetEmails = append(internetEmails, email)
					}
				}
			case "IMI": // Mail via Internet (email)
				if len(flagParts) > 1 {
					email := strings.TrimSpace(flagParts[1])
					if email != "" {
						internetEmails = append(internetEmails, email)
					}
				}
			case "IFC": // Raw FidoNet over Internet
				internetProtocols = append(internetProtocols, flagName)
				if len(flagParts) > 1 {
					hostname := strings.TrimSpace(flagParts[1])
					if hostname != "" {
						internetHostnames = append(internetHostnames, hostname)
					}
				}
				if len(flagParts) > 2 {
					if port, err := strconv.Atoi(strings.TrimSpace(flagParts[2])); err == nil {
						internetPorts = append(internetPorts, port)
					}
				}
			case "IFT": // Telnet to FidoNet
				internetProtocols = append(internetProtocols, flagName)
				if len(flagParts) > 1 {
					hostname := strings.TrimSpace(flagParts[1])
					if hostname != "" {
						internetHostnames = append(internetHostnames, hostname)
					}
				}
				if len(flagParts) > 2 {
					if port, err := strconv.Atoi(strings.TrimSpace(flagParts[2])); err == nil {
						internetPorts = append(internetPorts, port)
					}
				}
			case "IVM": // VModem over Internet
				internetProtocols = append(internetProtocols, flagName)
				if len(flagParts) > 1 {
					hostname := strings.TrimSpace(flagParts[1])
					if hostname != "" {
						internetHostnames = append(internetHostnames, hostname)
					}
				}
				if len(flagParts) > 2 {
					if port, err := strconv.Atoi(strings.TrimSpace(flagParts[2])); err == nil {
						internetPorts = append(internetPorts, port)
					}
				}
			case "ITX": // Txy over Internet  
				internetProtocols = append(internetProtocols, flagName)
				if len(flagParts) > 1 {
					hostname := strings.TrimSpace(flagParts[1])
					if hostname != "" {
						internetHostnames = append(internetHostnames, hostname)
					}
				}
				if len(flagParts) > 2 {
					if port, err := strconv.Atoi(strings.TrimSpace(flagParts[2])); err == nil {
						internetPorts = append(internetPorts, port)
					}
				}
			default:
				// Other flags with parameters
				flags = append(flags, part)
			}
		} else {
			// Simple flags without parameters
			// Check if it's an internet protocol flag and add to both arrays
			switch part {
			case "IBN", "BND", "ITN", "TEL", "IFC", "IFT", "IVM", "ITX", "INA":
				internetProtocols = append(internetProtocols, part)
				flags = append(flags, part)
			default:
				flags = append(flags, part)
			}
		}
	}
	
	return flags, internetProtocols, internetHostnames, internetPorts, internetEmails
}

// hasFlag checks if a flag exists in the flags array
func (p *Parser) hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

// hasProtocol checks if a protocol exists in the protocols array
func (p *Parser) hasProtocol(protocols []string, protocol string) bool {
	for _, p := range protocols {
		if strings.EqualFold(p, protocol) {
			return true
		}
	}
	return false
}

