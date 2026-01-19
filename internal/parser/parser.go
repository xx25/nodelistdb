package parser

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

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

// Context tracks the current parsing context (zone, net, region)
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
	nodeTracker map[string][]int // key: "zone:net/node", value: slice of indices

	// Pre-compiled regex patterns for common operations
	crcPattern *regexp.Regexp
}

// New creates a new parser instance with default configuration.
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
		nodeTracker: make(map[string][]int, 1000),

		// Pre-compile commonly used regex patterns
		crcPattern: regexp.MustCompile(`CRC-?(\w+)`),
	}
}

// NewAdvanced creates a new parser (kept for backward compatibility).
func NewAdvanced(verbose bool) *Parser {
	return New(verbose)
}

// clearReusableMaps resets all reusable maps for the next parsing operation.
// This prevents memory allocations by reusing existing map capacity.
func (p *Parser) clearReusableMaps() {
	// Clear nodeTracker map but keep capacity
	for k := range p.nodeTracker {
		delete(p.nodeTracker, k)
	}
}

// estimateNodeCount estimates the number of nodes in a file for slice pre-allocation.
// This reduces memory reallocations during parsing by providing a reasonable capacity estimate.
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

// ParseFile parses a single nodelist file and returns nodes.
// This is a convenience wrapper around ParseFileWithCRC that discards CRC information.
func (p *Parser) ParseFile(filePath string) ([]database.Node, error) {
	result, err := p.ParseFileWithCRC(filePath)
	if err != nil {
		return nil, err
	}
	return result.Nodes, nil
}

// ParseFileWithCRC parses a single nodelist file and returns nodes with file CRC.
// This is the main parsing entry point that handles the entire file parsing workflow.
func (p *Parser) ParseFileWithCRC(filePath string) (*ParseResult, error) {
	// Clear reusable maps at start of parsing to reuse capacity
	p.clearReusableMaps()

	// Reset context to defaults for clean parsing state between files
	// This prevents state leakage where zone/net/region from a previous file
	// could affect parsing of subsequent files
	p.Context = Context{
		CurrentZone: 1,
		CurrentNet:  1,
		// CurrentRegion intentionally nil
	}

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

	// Open file and create reader (with gzip support)
	reader, closeFunc, err := p.openFileReader(filePath)
	if err != nil {
		return nil, err
	}
	defer closeFunc()

	// Parse the file content
	nodes, nodelistDate, dayNumber, fileCRC, err := p.parseFileContent(reader, filePath, estimatedNodes)
	if err != nil {
		return nil, err
	}

	// Fallback: extract date from filename if not found in header
	if nodelistDate.IsZero() {
		nodelistDate, dayNumber, _ = p.extractDateFromFile(filePath)
	}

	if p.verbose {
		p.printParseResults(filePath, len(nodes), nodelistDate, dayNumber)
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

// MaxDecompressedSize is the maximum size of decompressed data to prevent gzip bombs.
// 500MB should be sufficient for any legitimate FidoNet nodelist file.
const MaxDecompressedSize = 500 * 1024 * 1024

// openFileReader opens a file and returns a reader that handles both regular and gzipped files.
// For gzipped files, decompression is limited to MaxDecompressedSize to prevent gzip bombs.
func (p *Parser) openFileReader(filePath string) (io.Reader, func(), error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, NewFileError(filePath, "open", "failed to open file", err)
	}

	// Create reader that handles both regular and gzipped files
	var reader io.Reader = file
	closeFunc := func() { file.Close() }

	if strings.HasSuffix(strings.ToLower(filePath), ".gz") {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			file.Close()
			return nil, nil, NewFileError(filePath, "gzip", "failed to create gzip reader", err)
		}
		// Wrap with LimitReader to prevent gzip bombs (small compressed files
		// that decompress to enormous sizes, causing memory exhaustion)
		reader = io.LimitReader(gzipReader, MaxDecompressedSize)
		closeFunc = func() {
			gzipReader.Close()
			file.Close()
		}
	}

	return reader, closeFunc, nil
}

// parseFileContent reads and parses the content of a nodelist file.
func (p *Parser) parseFileContent(reader io.Reader, filePath string, estimatedNodes int) ([]database.Node, time.Time, int, uint16, error) {
	// Pre-allocate nodes slice with estimated capacity for better performance
	nodes := make([]database.Node, 0, estimatedNodes)
	scanner := bufio.NewScanner(reader)
	lineNum := 0
	var nodelistDate time.Time
	var dayNumber int
	var fileCRC uint16
	var firstNodeLine string
	headerParsed := false

	// Track duplicates within this file
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
			p.DetectedFormat = detectFormat(line, firstNodeLine)
			if p.verbose {
				fmt.Printf("Detected format: %v\n", p.DetectedFormat)
			}
		}

		// Parse node line
		node, err := p.parseLine(line, nodelistDate, dayNumber, filePath)
		if err != nil {
			if p.verbose {
				fmt.Printf("Warning: Failed to parse line %d in %s: %v\n", lineNum, filepath.Base(filePath), err)
			}
			continue // Skip malformed lines
		}

		if node != nil {
			p.trackDuplicates(node, &nodes, &duplicateStats, lineNum, filePath)
			nodes = append(nodes, *node)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, time.Time{}, 0, 0, NewFileError(filePath, "read", "error reading file", err)
	}

	return nodes, nodelistDate, dayNumber, fileCRC, nil
}

// trackDuplicates checks for and tracks duplicate node entries within a file.
func (p *Parser) trackDuplicates(node *database.Node, nodes *[]database.Node, stats *struct {
	totalDuplicates int
	conflictGroups  int
}, lineNum int, filePath string) {
	nodeKey := fmt.Sprintf("%d:%d/%d", node.Zone, node.Net, node.Node)

	if existingIndices, exists := p.nodeTracker[nodeKey]; exists {
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
			(*nodes)[idx].HasConflict = true
		}

		stats.totalDuplicates++
		if len(existingIndices) == 1 {
			// First duplicate for this node
			stats.conflictGroups++
		}

		// Add current index to tracker
		p.nodeTracker[nodeKey] = append(existingIndices, len(*nodes))
	} else {
		// First occurrence - add to tracker
		p.nodeTracker[nodeKey] = []int{len(*nodes)}
	}
}

// printParseResults prints a summary of the parsing results.
func (p *Parser) printParseResults(filePath string, nodeCount int, nodelistDate time.Time, dayNumber int) {
	fmt.Printf("Parsed %d nodes from %s (Format: %v)\n", nodeCount, filepath.Base(filePath), p.DetectedFormat)
	fmt.Printf("  File: %s\n", filePath)
	fmt.Printf("  Date: %s (Day %d)\n", nodelistDate.Format("2006-01-02"), dayNumber)

	// Count conflicts
	conflictCount := 0
	duplicateGroups := make(map[string]bool)
	for key, indices := range p.nodeTracker {
		if len(indices) > 1 {
			duplicateGroups[key] = true
			conflictCount += len(indices) - 1
		}
	}

	if conflictCount > 0 {
		fmt.Printf("  ⚠️  DUPLICATES FOUND: %d duplicate entries across %d nodes\n",
			conflictCount, len(duplicateGroups))
		fmt.Printf("     These duplicates have been preserved with conflict tracking\n")
	} else {
		fmt.Printf("  ✓ No duplicate node addresses found in this file\n")
	}
}
