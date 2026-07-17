package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// Combined/V7 ("point", FTS-5002 §2.2) and fakenet (§2.4) formats. Both are
// nodelist-shaped; the keyword vocabulary means the OPPOSITE of the boss
// family: an empty first field is a node (context) line in the combined
// format and a point (leaf) line in a fakenet, never a point line keyed
// "Point". This is why the three families get three code paths.

// parseCombinedFormat parses the combined/V7 format: a full nodelist shape
// where Zone/Region/Host/Hub and plain node lines only update the "current
// boss" context (they are NOT imported — the boss is a node and lives in the
// nodes table), and "Point,N,..." lines emit points under that boss. V7 lists
// carry no Zone line, so the zone starts at DefaultZone.
func (pp *PointlistParser) parseCombinedFormat(lines []string, filePath string, result *PointlistParseResult) error {
	zone := pp.DefaultZone
	curNet, curNode := 0, 0
	haveContext := false
	bossesWithPoints := make(map[string]bool)

	for lineNum, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		line = sanitizeUTF8(line)

		fields := strings.Split(line, ",")
		keyword := strings.TrimSpace(fields[0])

		// Context lines carry their number in field 2, like every nodelist line.
		contextNum := func() (int, bool) {
			if len(fields) < 2 {
				return 0, false
			}
			n, err := strconv.Atoi(strings.TrimSpace(fields[1]))
			if err != nil || n < 0 {
				return 0, false
			}
			return n, true
		}

		switch {
		case strings.EqualFold(keyword, "Zone"):
			if n, ok := contextNum(); ok {
				zone, curNet, curNode = n, n, 0
				haveContext = true
			} else {
				pp.skipLine(result, filePath, lineNum+1, "Zone line without a number")
				haveContext = false
			}
		case strings.EqualFold(keyword, "Region"), strings.EqualFold(keyword, "Host"):
			if n, ok := contextNum(); ok {
				curNet, curNode = n, 0
				haveContext = true
			} else {
				pp.skipLine(result, filePath, lineNum+1, fmt.Sprintf("%s line without a number", keyword))
				haveContext = false
			}
		case keyword == "",
			strings.EqualFold(keyword, "Hub"),
			strings.EqualFold(keyword, "Pvt"),
			strings.EqualFold(keyword, "Down"),
			strings.EqualFold(keyword, "Hold"):
			// Node line: context only, never imported here.
			if n, ok := contextNum(); ok && haveContext {
				curNode = n
			} else {
				pp.skipLine(result, filePath, lineNum+1, "node line without a number or before any context")
				haveContext = false
			}
		case strings.EqualFold(keyword, "Point"):
			if !haveContext {
				pp.skipLine(result, filePath, lineNum+1, "Point line before any node context")
				continue
			}
			point, err := pp.parsePointLine(fields, line)
			if err != nil {
				pp.skipLine(result, filePath, lineNum+1, err.Error())
				continue
			}
			point.Zone = zone
			point.Net = curNet
			point.Node = curNode
			point.RawLine = rawLine
			bossesWithPoints[fmt.Sprintf("%d:%d/%d", zone, curNet, curNode)] = true
			pp.emitPoint(point, result)
		default:
			pp.skipLine(result, filePath, lineNum+1, fmt.Sprintf("unknown line type %q", keyword))
		}
	}

	result.BossCount = len(bossesWithPoints)
	return nil
}

// parseFakenetFormat parses fakenet-style pointlists (§2.4): nodelist-shaped
// files where each "Host,<fakenet>,..." block maps to a real boss node and
// the leaf node lines under it are that boss's points. The real boss address
// comes from (a) the host's system-name field when it is address-shaped
// ("240/2188" — the POINTS24 convention), (b) a "UBOSS:230/26" flag (the
// DK-POINT convention), (c) the same conventions on a Hub line
// ("Hub,1100,241/1" — early POINTS24 puts bosses on hubs), or (d) a full
// 4-D address in the LEAF line's own system-name field (",1101,241/1.1101"
// — the earliest convention, resolved per leaf). Host blocks with no
// resolvable boss are skipped whole (unless (d) rescues their leaves),
// counted in SkippedBosses.
func (pp *PointlistParser) parseFakenetFormat(lines []string, filePath string, result *PointlistParseResult) error {
	zone := pp.DefaultZone
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

		switch {
		case strings.EqualFold(keyword, "Zone"):
			// Real zone context (DK-POINT starts with "Zone,2,...") — scopes
			// zoneless boss addresses. Not a boss; leaf lines under it are not
			// points of anything.
			if len(fields) > 1 {
				if n, err := strconv.Atoi(strings.TrimSpace(fields[1])); err == nil && n > 0 {
					zone = n
				}
			}
			haveBoss = false
		case strings.EqualFold(keyword, "Region"):
			// Fake-net grouping context — ignore, and detach any current boss.
			haveBoss = false
		case strings.EqualFold(keyword, "Hub"):
			// Early POINTS24 issues put the boss on Hub lines under a
			// descriptive Host; a Hub without a boss address is plain
			// grouping context and detaches the current boss.
			if z, n, nd, ok := pp.resolveFakenetBoss(fields, zone); ok {
				bossZone, bossNet, bossNode = z, n, nd
				haveBoss = true
				result.BossCount++
			} else {
				haveBoss = false
			}
		case strings.EqualFold(keyword, "Host"):
			z, n, nd, ok := pp.resolveFakenetBoss(fields, zone)
			if !ok {
				pp.skipLine(result, filePath, lineNum+1,
					fmt.Sprintf("Host block %q has no resolvable boss (no address-shaped sysname, no UBOSS flag)", strings.TrimSpace(strings.Join(fields[:min(3, len(fields))], ","))))
				result.SkippedBosses++
				haveBoss = false
				continue
			}
			bossZone, bossNet, bossNode = z, n, nd
			haveBoss = true
			result.BossCount++
		case isPointLineKeyword(keyword):
			point, err := pp.parsePointLine(fields, line)
			if err != nil {
				pp.skipLine(result, filePath, lineNum+1, err.Error())
				continue
			}
			// A full 4-D address in the leaf's system-name field resolves the
			// boss (and point number) directly — independent of the block
			// context, rescuing leaves under unresolvable Host headers.
			if z, n, nd, p, ok := pp.parseLeaf4DSysname(fields, zone); ok {
				point.Zone, point.Net, point.Node, point.PointNum = z, n, nd, p
			} else if haveBoss {
				point.Zone = bossZone
				point.Net = bossNet
				point.Node = bossNode
			} else {
				pp.skipLine(result, filePath, lineNum+1, "leaf line outside a resolvable Host block")
				continue
			}
			point.RawLine = rawLine
			pp.emitPoint(point, result)
		default:
			pp.skipLine(result, filePath, lineNum+1, fmt.Sprintf("unknown line type %q", keyword))
		}
	}

	return nil
}

// resolveFakenetBoss extracts the real boss address of a fakenet Host block.
func (pp *PointlistParser) resolveFakenetBoss(fields []string, zone int) (int, int, int, bool) {
	parse := func(addr string) (int, int, int, bool) {
		m := bossAddrPattern.FindStringSubmatch(addr)
		if m == nil {
			return 0, 0, 0, false
		}
		z := zone
		if m[1] != "" {
			z, _ = strconv.Atoi(m[1])
		}
		net, err1 := strconv.Atoi(m[2])
		node, err2 := strconv.Atoi(m[3])
		if err1 != nil || err2 != nil {
			return 0, 0, 0, false
		}
		return z, net, node, true
	}

	// (a) address-shaped system name: Host,10251,240/2188,...
	if len(fields) > 2 {
		if z, n, nd, ok := parse(strings.TrimSpace(fields[2])); ok {
			return z, n, nd, true
		}
	}

	// (b) UBOSS:230/26 flag anywhere in the flag fields
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if len(f) > 6 && strings.EqualFold(f[:6], "UBOSS:") {
			if z, n, nd, ok := parse(f[6:]); ok {
				return z, n, nd, true
			}
		}
	}

	return 0, 0, 0, false
}

// parseLeaf4DSysname extracts a full 4-D address from a leaf line's
// system-name field (",1101,241/1.1101,..." — earliest POINTS24 convention).
func (pp *PointlistParser) parseLeaf4DSysname(fields []string, zone int) (int, int, int, int, bool) {
	if len(fields) < 3 {
		return 0, 0, 0, 0, false
	}
	sysname := strings.TrimSpace(fields[2])
	if !fakenetLeaf4DRe.MatchString(sysname) {
		return 0, 0, 0, 0, false
	}

	addr, pointStr, _ := strings.Cut(sysname, ".")
	m := bossAddrPattern.FindStringSubmatch(addr)
	if m == nil {
		return 0, 0, 0, 0, false
	}
	z := zone
	if m[1] != "" {
		z, _ = strconv.Atoi(m[1])
	}
	net, err1 := strconv.Atoi(m[2])
	node, err2 := strconv.Atoi(m[3])
	point, err3 := strconv.Atoi(pointStr)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, 0, false
	}
	return z, net, node, point, true
}

// emitPoint applies duplicate tracking and appends the point.
func (pp *PointlistParser) emitPoint(point *database.Point, result *PointlistParseResult) {
	pp.trackPointDuplicates(point, &result.Points)
	result.Points = append(result.Points, *point)
}
