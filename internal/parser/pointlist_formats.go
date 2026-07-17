package parser

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// fakenetHostSignalRe matches an address-shaped system-name field on a Host
// or Hub line ("Host,10251,240/2188,..." — the POINTS24 fakenet convention;
// early issues put the boss on Hub lines instead).
var fakenetHostSignalRe = regexp.MustCompile(`^(?:\d+:)?\d+/\d+(?:\.\d+)?$`)

// fakenetLeaf4DRe matches a full 4-D address in a leaf line's system-name
// field (",1101,241/1.1101,..." — the earliest POINTS24 convention carries
// the real point address per leaf).
var fakenetLeaf4DRe = regexp.MustCompile(`^(?:\d+:)?\d+/\d+\.\d+$`)

// detectPointlistFormat inspects every non-comment line and decides which
// FTS-5002 format the file uses. The keyword vocabulary is format-relative
// (an empty field-1 is a point line in the boss family but a node context
// line in combined/fakenet), so misclassification would silently discard
// points — when the nodelist-shaped formats cannot be told apart by
// structural signals, this errors out instead of guessing, and the caller
// demands an explicit -format.
func detectPointlistFormat(lines []string) (string, error) {
	bossLines := 0
	emptyFieldPoints := 0
	possPoints := 0
	pvtPoints := 0
	hostLines := 0
	nodeContextLines := 0 // Zone/Region/Hub lines — combined (point) or fakenet shape
	fakenetSignals := 0   // Host lines resolvable to a real boss (sysname/UBOSS)

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		fields := strings.SplitN(line, ",", 4)
		keyword := strings.TrimSpace(fields[0])
		switch {
		case strings.EqualFold(keyword, "Boss"):
			bossLines++
		case keyword == "":
			emptyFieldPoints++
		case strings.EqualFold(keyword, "Point"):
			possPoints++
		case strings.EqualFold(keyword, "Pvt"):
			pvtPoints++
		case strings.EqualFold(keyword, "Host"), strings.EqualFold(keyword, "Hub"):
			if strings.EqualFold(keyword, "Host") {
				hostLines++
			} else {
				nodeContextLines++
			}
			if len(fields) > 2 && fakenetHostSignalRe.MatchString(strings.TrimSpace(fields[2])) {
				fakenetSignals++
			} else if strings.Contains(strings.ToUpper(line), "UBOSS:") {
				fakenetSignals++
			}
		case strings.EqualFold(keyword, "Zone"),
			strings.EqualFold(keyword, "Region"):
			nodeContextLines++
		}

		// Leaf lines carrying a full 4-D address as system name are the
		// earliest fakenet convention.
		if (keyword == "" || strings.EqualFold(keyword, "Pvt")) &&
			len(fields) > 2 && fakenetLeaf4DRe.MatchString(strings.TrimSpace(fields[2])) {
			fakenetSignals++
		}
	}

	if bossLines > 0 {
		// Boss family. The three variants share one code path; the label
		// records the dominant point-line flavour for provenance.
		switch {
		case emptyFieldPoints > 0:
			return PointlistFormatBoss, nil
		case possPoints > 0:
			return PointlistFormatPoss, nil
		case pvtPoints > 0:
			return PointlistFormatPvt, nil
		default:
			// Boss lines but no point lines — still boss family
			return PointlistFormatBoss, nil
		}
	}

	if hostLines > 0 || nodeContextLines > 0 {
		// Fakenet markers are structural (boss-resolvable Host lines) and
		// beat a stray Point line; a combined file is identified by Point
		// lines with NO fakenet markers. Anything else is ambiguous.
		if fakenetSignals > 0 {
			return PointlistFormatFakenet, nil
		}
		if possPoints > 0 {
			return PointlistFormatPoint, nil
		}
		return "", fmt.Errorf("nodelist-shaped file has neither Point lines nor fakenet boss markers (address sysname / UBOSS flag); pass -format explicitly")
	}

	return "", fmt.Errorf("no Boss/Host/Point lines found; pass -format explicitly")
}

// pointlistSourceFamily maps a known pointlist filename family to its
// list_source and source priority. list_source derives from the FILENAME,
// never the directory: the corpus contains strays (R50PNT.293 sitting in
// R56/BOSS/) and the filename family is the reliable signal.
type pointlistSourceFamily struct {
	pattern  *regexp.Regexp
	source   string // empty = use first capture group as region number ("r" + m[1])
	priority uint8
}

var pointlistSourceFamilies = []pointlistSourceFamily{
	// Zone rollup
	{regexp.MustCompile(`(?i)^z2pnt\.`), "z2", SourcePriorityZone},
	// Generic regional series: R24PNT.* boss lists, R29PNT_B.* boss flavour,
	// R29PNT_P.* combined/V7 flavour. Same region → same list_source, so the
	// import gate automatically gap-fills: whichever flavour is imported first
	// for a date wins and the other is skipped.
	{regexp.MustCompile(`(?i)^r(\d+)pnt(?:_[bp])?\.`), "", SourcePriorityRegional},
	// Named regional series that don't follow the r##pnt scheme
	{regexp.MustCompile(`(?i)^pnt46reg\.`), "r46", SourcePriorityRegional},
	{regexp.MustCompile(`(?i)^p28-list\.`), "r28", SourcePriorityRegional},
	{regexp.MustCompile(`(?i)^r45point\.`), "r45", SourcePriorityRegional},
	{regexp.MustCompile(`(?i)^point_48\.`), "r48", SourcePriorityRegional},
	{regexp.MustCompile(`(?i)^(?:pointr34|ptlstr34)\.`), "r34", SourcePriorityRegional},
	// Fakenet series (§2.4). The r23/r24 boss lists of the early years were
	// reconstructed FROM these, sharing list_source keeps the gate as the
	// gap-filler here too.
	{regexp.MustCompile(`(?i)^points24\.`), "r24", SourcePriorityRegional},
	{regexp.MustCompile(`(?i)^dk-point\.`), "r23", SourcePriorityRegional},
}

// pointlistDiffPattern matches diff series (R24PNT_D.*, PR24DIFF.*,
// DKP-DIFF.*, POINTS24.D##) which must never be imported as full lists.
// The .D## extension form matters: diffs extracted from a series' archives
// share the series' base name, so the family patterns would otherwise
// claim them.
var pointlistDiffPattern = regexp.MustCompile(`(?i)(?:_d\.|diff|\.d[0-9]{2,3}$)`)

// DerivePointlistSource derives (list_source, source_priority) from a
// pointlist filename. ok is false for diffs and unknown families — the
// caller must then supply -list-source explicitly.
func DerivePointlistSource(filename string) (string, uint8, bool) {
	base := filepath.Base(filename)
	base = strings.TrimSuffix(strings.ToLower(base), ".gz")

	if pointlistDiffPattern.MatchString(base) {
		return "", 0, false
	}

	for _, family := range pointlistSourceFamilies {
		m := family.pattern.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		source := family.source
		if source == "" && len(m) > 1 {
			source = "r" + m[1]
		}
		return source, family.priority, true
	}

	return "", 0, false
}
