package parser

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
)

// parseNodeFields extracts basic node fields from the comma-separated line.
// Returns: nodeType, nodeNum, systemName, location, sysopName, phone, maxSpeed, flagsStr, error
func (p *Parser) parseNodeFields(line string) (string, string, string, string, string, string, uint32, string, error) {
	// Handle different line formats - FidoNet standard is comma-separated
	fields := strings.Split(line, ",")
	if len(fields) < 7 {
		return "", "", "", "", "", "", 0, "", fmt.Errorf("insufficient fields: expected at least 7, got %d", len(fields))
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
	}

	// Parse flags (field 7 and beyond)
	var flagsStr string
	if len(fields) > 7 {
		flagsStr = strings.Join(fields[7:], ",")
	}

	return nodeTypeStr, nodeNumStr, systemName, location, sysopName, phone, maxSpeed, flagsStr, nil
}

// parseNodeType determines the node type and address from the type string and context.
// Returns: nodeType, zone, net, node, region, error
func (p *Parser) parseNodeType(nodeTypeStr, nodeNumStr string) (string, int, int, int, *int, error) {
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
			return "", 0, 0, 0, nil, err
		}
	} else {
		// Handle special node types
		var err error
		nodeType, zone, net, node, region, err = p.parseSpecialNodeType(nodeTypeStr, nodeNumStr)
		if err != nil {
			return "", 0, 0, 0, nil, err
		}
	}

	// Copy context region if available
	if p.Context.CurrentRegion != nil && region == nil {
		region = p.Context.CurrentRegion
	}

	return nodeType, zone, net, node, region, nil
}

// parseSpecialNodeType handles special node types like Zone, Region, Host, Hub, etc.
func (p *Parser) parseSpecialNodeType(nodeTypeStr, nodeNumStr string) (string, int, int, int, *int, error) {
	var nodeType string
	var zone, net, node int
	var region *int

	switch strings.Title(strings.ToLower(nodeTypeStr)) {
	case "Zone":
		nodeType = "Zone"
		if z, err := ParseInt("zone", nodeNumStr); err == nil {
			zone = z
			net = z  // Zone nodes have net = zone
			node = 0 // Zone coordinator
			p.Context.CurrentZone = z
			p.Context.CurrentNet = z
		} else {
			return "", 0, 0, 0, nil, err
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
			region = &regionNum
		} else {
			return "", 0, 0, 0, nil, err
		}

	case "Host":
		nodeType = "Host"
		zone = p.Context.CurrentZone
		if n, err := ParseInt("net", nodeNumStr); err == nil {
			net = n
			node = 0 // Host = Net/0
			p.Context.CurrentNet = n
		} else {
			return "", 0, 0, 0, nil, err
		}

	case "Hub":
		nodeType = "Hub"
		zone = p.Context.CurrentZone
		net = p.Context.CurrentNet
		if nodeNum, err := strconv.Atoi(nodeNumStr); err == nil {
			node = nodeNum
		} else {
			return "", 0, 0, 0, nil, fmt.Errorf("invalid hub node number: %s", nodeNumStr)
		}

	case "Pvt", "Hold", "Down":
		nodeType = strings.Title(strings.ToLower(nodeTypeStr))
		zone = p.Context.CurrentZone
		net = p.Context.CurrentNet
		if nodeNum, err := strconv.Atoi(nodeNumStr); err == nil {
			node = nodeNum
		} else {
			return "", 0, 0, 0, nil, fmt.Errorf("invalid %s node number: %s", strings.ToLower(nodeTypeStr), nodeNumStr)
		}

	default:
		return "", 0, 0, 0, nil, fmt.Errorf("unknown node type: %s", nodeTypeStr)
	}

	return nodeType, zone, net, node, region, nil
}

// parseLine parses a single nodelist entry line.
// This is the main entry point for parsing a nodelist line into a database.Node structure.
func (p *Parser) parseLine(line string, nodelistDate time.Time, dayNumber int, filePath string) (*database.Node, error) {
	// Store the original raw line before any processing
	rawLine := line

	// Sanitize UTF-8 for database storage
	line = sanitizeUTF8(line)

	// Extract basic fields
	nodeTypeStr, nodeNumStr, systemName, location, sysopName, phone, maxSpeed, flagsStr, err := p.parseNodeFields(line)
	if err != nil {
		return nil, err
	}

	// Apply legacy flag conversions if needed
	if p.DetectedFormat == Format1986 {
		flagsStr = p.convertLegacyFlags(flagsStr)
	}

	// Determine node type and address
	nodeType, zone, net, node, region, err := p.parseNodeType(nodeTypeStr, nodeNumStr)
	if err != nil {
		return nil, err
	}

	// Parse flags into structured format with full categorization AND build JSON config
	flags, internetConfig := p.parseFlagsWithConfig(flagsStr)

	// Get modem flags separately (not included in parseFlagsWithConfig)
	_, _, _, _, _, modemFlags := p.parseAdvancedFlags(flagsStr)

	// Compute boolean flags based on comprehensive flag analysis
	isCM := p.hasFlag(flags, "CM")
	isMO := p.hasFlag(flags, "MO")

	// Determine internet connectivity from JSON config
	hasInet := len(internetConfig) > 0 && string(internetConfig) != "null"

	// Create node with all enhanced data
	dbNode := database.Node{
		Zone:             zone,
		Net:              net,
		Node:             node,
		NodelistDate:     nodelistDate,
		DayNumber:        dayNumber,
		SystemName:       systemName,
		Location:         location,
		SysopName:        sysopName,
		Phone:            phone,
		NodeType:         nodeType,
		Region:           region,
		MaxSpeed:         maxSpeed,
		IsCM:             isCM,
		IsMO:             isMO,
		HasInet:          hasInet,
		Flags:            flags,
		ModemFlags:       modemFlags,
		InternetConfig:   internetConfig,
		ConflictSequence: 0,
		HasConflict:      false,
		RawLine:          rawLine,
	}

	return &dbNode, nil
}
