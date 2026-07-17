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
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// Pointlist source formats (FTS-5002)
const (
	PointlistFormatAuto    = "auto"
	PointlistFormatBoss    = "boss"    // §2.1: Boss, lines + point lines with empty first field
	PointlistFormatPoss    = "poss"    // §2.3: Point, in field 1 under Boss, lines
	PointlistFormatPvt     = "pvt"     // §2.1 variant: Pvt in field 1 under Boss, lines
	PointlistFormatPoint   = "point"   // §2.2: combined/V7 full nodelist + Point, lines (Phase 3)
	PointlistFormatFakenet = "fakenet" // §2.4: fakenet-shaped lists (Phase 3)
)

// Source priority tiers: lower wins when the same point appears in
// overlapping sources at query time.
const (
	SourcePriorityNet      uint8 = 0  // net-level segment
	SourcePriorityRegional uint8 = 10 // regional list (r24, r50, ...)
	SourcePriorityZone     uint8 = 20 // zone rollup (z2)
)

// PointlistParseResult contains the parsed points and file metadata
type PointlistParseResult struct {
	Points        []database.Point
	FilePath      string
	PointlistDate time.Time
	DayNumber     int
	BossCount     int
	SkippedLines  int      // non-conforming lines (logged, not fatal)
	SkippedBosses int      // boss blocks skipped (unresolvable boss address)
	Warnings      []string // non-fatal anomalies (non-Friday date, day mismatch, ...)
	SourceFormat  string   // detected/selected format actually used
}

// PointlistParser parses FTN pointlist files (FTS-5002 formats).
// It shares the flag machinery with the nodelist Parser but is a separate
// type: pointlist parsing must never disturb nodelist parsing.
type PointlistParser struct {
	verbose bool
	domain  string

	// Import parameters
	ListSource     string // 'r24', 'z2', ... (stamped on every point)
	SourcePriority uint8
	Format         string // one of the PointlistFormat* constants
	Year           int    // year the 3-digit filename/header day belongs to (0 = derive from path)
	DefaultZone    int    // zone assumed for boss addresses without an explicit zone
	Charset        string // cp437 (default), cp850, cp866, latin1, utf8

	// helper reuses the nodelist Parser's flag parsing and date helpers
	helper *Parser

	// duplicate tracking within one file, key "z:n/n.p"
	pointTracker map[string][]int
}

// NewPointlistParser creates a pointlist parser with defaults suitable for
// the historical corpus (zone 2, CP437, autodetected format).
func NewPointlistParser(verbose bool) *PointlistParser {
	return &PointlistParser{
		verbose:      verbose,
		Format:       PointlistFormatAuto,
		DefaultZone:  2,
		Charset:      "cp437",
		helper:       New(verbose),
		pointTracker: make(map[string][]int, 4096),
	}
}

// SetDomain sets the FTN network name stamped on every parsed point.
func (pp *PointlistParser) SetDomain(domain string) {
	pp.domain = domain
}

// charsetDecoder returns the text decoder for the configured charset,
// or nil when the input is already UTF-8.
func (pp *PointlistParser) charsetDecoder() (*encoding.Decoder, error) {
	switch strings.ToLower(pp.Charset) {
	case "", "utf8", "utf-8":
		return nil, nil
	case "cp437":
		return charmap.CodePage437.NewDecoder(), nil
	case "cp850":
		return charmap.CodePage850.NewDecoder(), nil
	case "cp866":
		return charmap.CodePage866.NewDecoder(), nil
	case "latin1", "iso8859-1":
		return charmap.ISO8859_1.NewDecoder(), nil
	default:
		return nil, fmt.Errorf("unsupported charset: %s (use cp437, cp850, cp866, latin1 or utf8)", pp.Charset)
	}
}

// bossAddrPattern matches a boss address: "2:240/2188" or "240/2188",
// tolerating a 4D ".0" suffix ("Boss,2:465/101.0" exists in the corpus)
var bossAddrPattern = regexp.MustCompile(`^(?:(\d+):)?(\d+)/(\d+)(?:\.\d+)?$`)

// pointlistDayPattern matches a trailing 3-digit day-of-year extension
// (R24PNT.005, pnt46reg.012, Z2PNT.201), optionally gzipped
var pointlistDayPattern = regexp.MustCompile(`\.(\d{3})(?:\.gz)?$`)

// dayNumberHeaderPattern matches "Day number 005" in header comments
var dayNumberHeaderPattern = regexp.MustCompile(`(?i)day\s+number\s+(\d{1,3})`)

// ParseFile parses a single pointlist file.
func (pp *PointlistParser) ParseFile(filePath string) (*PointlistParseResult, error) {
	for k := range pp.pointTracker {
		delete(pp.pointTracker, k)
	}

	reader, closeFunc, err := pp.openReader(filePath)
	if err != nil {
		return nil, err
	}
	defer closeFunc()

	result := &PointlistParseResult{FilePath: filePath}

	// Read all lines up-front (decoded to UTF-8): pointlist files are small
	// (< 5 MB even for Z2PNT) and autodetect needs a preview pass anyway.
	lines, err := pp.readLines(reader, filePath)
	if err != nil {
		return nil, err
	}

	// Resolve the issue date before parsing lines: filename day beats header
	// (r46 headers are stale), header is the fallback.
	pp.resolveDate(filePath, lines, result)

	// Resolve format
	format := pp.Format
	if format == "" || format == PointlistFormatAuto {
		format, err = detectPointlistFormat(lines)
		if err != nil {
			return nil, NewFileError(filePath, "detect", "cannot autodetect pointlist format", err)
		}
	}
	result.SourceFormat = format

	switch format {
	case PointlistFormatBoss, PointlistFormatPoss, PointlistFormatPvt:
		err = pp.parseBossFamily(lines, filePath, result)
	case PointlistFormatPoint:
		err = pp.parseCombinedFormat(lines, filePath, result)
	case PointlistFormatFakenet:
		err = pp.parseFakenetFormat(lines, filePath, result)
	default:
		return nil, NewFileError(filePath, "parse", fmt.Sprintf("unknown pointlist format %q", format), nil)
	}
	if err != nil {
		return nil, err
	}

	// Stamp domain, source and identity on every point
	domain := pp.domain
	if domain == "" {
		domain = database.DefaultDomain
	}
	for i := range result.Points {
		p := &result.Points[i]
		p.Domain = domain
		p.PointlistDate = result.PointlistDate
		p.DayNumber = result.DayNumber
		p.ListSource = pp.ListSource
		p.SourcePriority = pp.SourcePriority
		p.SourceFormat = format
		p.ComputeFtsId()
	}

	if pp.verbose {
		fmt.Printf("Parsed %d points under %d bosses from %s (format %s, date %s, day %d)\n",
			len(result.Points), result.BossCount, filepath.Base(filePath),
			format, result.PointlistDate.Format("2006-01-02"), result.DayNumber)
		for _, w := range result.Warnings {
			fmt.Printf("  WARNING: %s\n", w)
		}
	}

	return result, nil
}

// openReader opens the file with gzip and charset decoding applied.
func (pp *PointlistParser) openReader(filePath string) (io.Reader, func(), error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, NewFileError(filePath, "open", "failed to open file", err)
	}

	var reader io.Reader = file
	closeFunc := func() { file.Close() }

	if strings.HasSuffix(strings.ToLower(filePath), ".gz") {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			file.Close()
			return nil, nil, NewFileError(filePath, "gzip", "failed to create gzip reader", err)
		}
		reader = io.LimitReader(gzipReader, MaxDecompressedSize)
		closeFunc = func() {
			gzipReader.Close()
			file.Close()
		}
	}

	decoder, err := pp.charsetDecoder()
	if err != nil {
		closeFunc()
		return nil, nil, err
	}
	if decoder != nil {
		reader = transform.NewReader(reader, decoder)
	}

	return reader, closeFunc, nil
}

// readLines reads all lines, stopping at an EOF marker (Ctrl+Z).
func (pp *PointlistParser) readLines(reader io.Reader, filePath string) ([]string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lines []string
	for scanner.Scan() {
		raw := scanner.Text()
		if idx := strings.IndexByte(raw, 0x1a); idx >= 0 {
			raw = raw[:idx]
			if strings.TrimSpace(raw) != "" {
				lines = append(lines, raw)
			}
			break
		}
		lines = append(lines, raw)
	}
	if err := scanner.Err(); err != nil {
		return nil, NewFileError(filePath, "read", "error reading file", err)
	}
	return lines, nil
}

// resolveDate determines the pointlist issue date.
// Priority: 3-digit day from the filename (+ Year) > "Day number" header >
// full header date. Mismatches and non-Friday dates become warnings.
func (pp *PointlistParser) resolveDate(filePath string, lines []string, result *PointlistParseResult) {
	year := pp.Year
	if year == 0 {
		year = pp.helper.extractYearFromPath(filePath)
	}

	filenameDay := 0
	if m := pointlistDayPattern.FindStringSubmatch(filepath.Base(filePath)); len(m) > 1 {
		filenameDay, _ = strconv.Atoi(m[1])
	}

	headerDay := 0
	var headerDate time.Time
	for i, line := range lines {
		if i >= 50 {
			break
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, ";") {
			break // header comments come before the first data line
		}
		if headerDay == 0 {
			if m := dayNumberHeaderPattern.FindStringSubmatch(trimmed); len(m) > 1 {
				headerDay, _ = strconv.Atoi(m[1])
			}
		}
		if headerDate.IsZero() {
			if date, _, err := pp.helper.extractDateFromLine(trimmed); err == nil {
				headerDate = date
			}
		}
	}

	switch {
	case filenameDay > 0 && year > 0:
		result.DayNumber = filenameDay
		result.PointlistDate = dateFromYearDay(year, filenameDay, result)
		if headerDay > 0 && headerDay != filenameDay {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("filename day %03d differs from header day %03d (filename wins; some series have stale headers)",
					filenameDay, headerDay))
		}
	case headerDay > 0 && year > 0:
		result.DayNumber = headerDay
		result.PointlistDate = dateFromYearDay(year, headerDay, result)
	case !headerDate.IsZero():
		result.PointlistDate = headerDate
		result.DayNumber = headerDate.YearDay()
	}

	if result.PointlistDate.IsZero() {
		result.Warnings = append(result.Warnings,
			"could not determine pointlist date (no usable filename day, -year or header date)")
		return
	}
	if result.PointlistDate.Weekday() != time.Friday {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("pointlist date %s is a %s, not a Friday",
				result.PointlistDate.Format("2006-01-02"), result.PointlistDate.Weekday()))
	}
}

// daysInYear returns 365 or 366.
func daysInYear(year int) int {
	return time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC).YearDay()
}

// dateFromYearDay converts (year, day-of-year) to a date. A day that
// overflows the year (day 366 with a non-leap year) means the year is wrong —
// year-end issues get archived under the NEXT year's directory (real case:
// P28-LIST.366 of leap 2004 sits in r28/2005/). Silently rolling into
// January of the next year would be a year-and-a-day off, so try year-1.
func dateFromYearDay(year, day int, result *PointlistParseResult) time.Time {
	if day > daysInYear(year) && day <= daysInYear(year-1) {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("day %03d does not exist in %d; assuming %d (year-end issue archived under the next year)",
				day, year, year-1))
		year--
	}
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, day-1)
}

// parseBossFamily parses the boss/poss/pvt formats (one shared code path):
// "Boss,<addr>[,ignored nodelist-style extras]" sets the boss context; point
// lines carry "" / "Point" / "Pvt" in field 1 and the point number in field 2.
func (pp *PointlistParser) parseBossFamily(lines []string, filePath string, result *PointlistParseResult) error {
	var bossZone, bossNet, bossNode int
	haveBoss := false

	for lineNum, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		line = sanitizeUTF8(line)

		fields := strings.Split(line, ",")
		keyword := strings.TrimSpace(fields[0])

		if strings.EqualFold(keyword, "Boss") {
			if len(fields) < 2 {
				pp.skipLine(result, filePath, lineNum+1, "Boss line without address")
				haveBoss = false
				continue
			}
			zone, net, node, err := pp.parseBossAddress(strings.TrimSpace(fields[1]))
			if err != nil {
				pp.skipLine(result, filePath, lineNum+1, fmt.Sprintf("unparseable boss address %q", fields[1]))
				result.SkippedBosses++
				haveBoss = false
				continue
			}
			bossZone, bossNet, bossNode = zone, net, node
			haveBoss = true
			result.BossCount++
			// Nodelist-style extras on the Boss line (z2 rollup carries full
			// system/flags data for the boss itself) are intentionally ignored:
			// the boss is a node, and nodes live in the nodes table.
			continue
		}

		if isPointLineKeyword(keyword) {
			if !haveBoss {
				pp.skipLine(result, filePath, lineNum+1, "point line before any valid Boss line")
				continue
			}
			point, err := pp.parsePointLine(fields, line)
			if err != nil {
				pp.skipLine(result, filePath, lineNum+1, err.Error())
				continue
			}
			point.Zone = bossZone
			point.Net = bossNet
			point.Node = bossNode
			point.RawLine = rawLine
			pp.trackPointDuplicates(point, &result.Points)
			result.Points = append(result.Points, *point)
		} else {
			// Unknown keyword (nodelist-style Host/Region lines in a boss-family
			// file would land here) — skip, count, keep going.
			pp.skipLine(result, filePath, lineNum+1, fmt.Sprintf("unknown line type %q", keyword))
		}
	}

	return nil
}

// isPointLineKeyword reports whether an FTS-5000 keyword field marks a point
// entry line in the boss family. Down and Hold are real point states in the
// corpus (same keyword vocabulary as node lines), matched case-insensitively.
func isPointLineKeyword(keyword string) bool {
	switch {
	case keyword == "":
		return true
	case strings.EqualFold(keyword, "Point"),
		strings.EqualFold(keyword, "Pvt"),
		strings.EqualFold(keyword, "Down"),
		strings.EqualFold(keyword, "Hold"):
		return true
	}
	return false
}

// parseBossAddress parses "z:net/node" or "net/node" (DefaultZone applied).
func (pp *PointlistParser) parseBossAddress(addr string) (int, int, int, error) {
	m := bossAddrPattern.FindStringSubmatch(addr)
	if m == nil {
		return 0, 0, 0, fmt.Errorf("invalid boss address: %s", addr)
	}
	zone := pp.DefaultZone
	if m[1] != "" {
		zone, _ = strconv.Atoi(m[1])
	}
	net, err := ParseInt("net", m[2])
	if err != nil {
		return 0, 0, 0, err
	}
	node, err := ParseInt("node", m[3])
	if err != nil {
		return 0, 0, 0, err
	}
	return zone, net, node, nil
}

// parsePointLine parses one point entry line:
// <""|Point|Pvt>,<point>,<system>,<location>,<sysop>,<phone>,<speed>[,flags...]
func (pp *PointlistParser) parsePointLine(fields []string, line string) (*database.Point, error) {
	return pp.helper.parsePointFields(fields)
}

// parsePointFields parses the fields of a point entry line. It lives on the
// nodelist Parser so inline "Point," lines inside nodelists parse identically
// to pointlist point lines.
func (p *Parser) parsePointFields(fields []string) (*database.Point, error) {
	if len(fields) < 7 {
		return nil, fmt.Errorf("insufficient fields: expected at least 7, got %d", len(fields))
	}

	pointNum, err := ParseInt("point", strings.TrimSpace(fields[1]))
	if err != nil {
		return nil, fmt.Errorf("invalid point number %q", fields[1])
	}

	var maxSpeed uint32
	if speedStr := strings.TrimSpace(fields[6]); speedStr != "" {
		if speed, err := strconv.ParseUint(speedStr, 10, 32); err == nil {
			maxSpeed = uint32(speed)
		}
	}

	var flagsStr string
	if len(fields) > 7 {
		flagsStr = strings.Join(fields[7:], ",")
	}

	// Reuse the nodelist flag machinery: points carry real connectivity
	// flags (CM, MO, XA, IBN:host, INA:host, ...)
	flags, internetConfig := p.parseFlagsWithConfig(flagsStr)
	_, _, _, _, _, modemFlags := p.parseAdvancedFlags(flagsStr)

	point := &database.Point{
		PointNum:       pointNum,
		SystemName:     strings.TrimSpace(fields[2]),
		Location:       strings.TrimSpace(fields[3]),
		SysopName:      strings.TrimSpace(fields[4]),
		Phone:          strings.TrimSpace(fields[5]),
		MaxSpeed:       maxSpeed,
		IsCM:           p.hasFlag(flags, "CM"),
		IsMO:           p.hasFlag(flags, "MO"),
		HasInet:        len(internetConfig) > 0 && string(internetConfig) != "null",
		Flags:          flags,
		ModemFlags:     modemFlags,
		InternetConfig: internetConfig,
	}
	return point, nil
}

// trackPointDuplicates assigns conflict sequences to repeated 4D addresses
// within one file (mirrors the nodelist trackDuplicates behaviour).
func (pp *PointlistParser) trackPointDuplicates(point *database.Point, points *[]database.Point) {
	key := fmt.Sprintf("%d:%d/%d.%d", point.Zone, point.Net, point.Node, point.PointNum)

	if existing, ok := pp.pointTracker[key]; ok {
		point.ConflictSequence = len(existing)
		point.HasConflict = true
		for _, idx := range existing {
			(*points)[idx].HasConflict = true
		}
		pp.pointTracker[key] = append(existing, len(*points))
	} else {
		pp.pointTracker[key] = []int{len(*points)}
	}
}

// skipLine records one skipped non-conforming line.
func (pp *PointlistParser) skipLine(result *PointlistParseResult, filePath string, lineNum int, reason string) {
	result.SkippedLines++
	if pp.verbose {
		fmt.Printf("  Skipping line %d in %s: %s\n", lineNum, filepath.Base(filePath), reason)
	}
}
