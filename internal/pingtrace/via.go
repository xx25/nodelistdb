// Package pingtrace holds the pure, side-effect-free pieces of the FTS-4010
// PING/TRACE measurement: parsing the FTS-4009 ^AVia lines a reply carries or
// quotes into an ordered list of hops, and correlating a reply with the ping it
// answers. Both the testdaemon (which sends pings and reads replies) and the
// web server (which renders paths) use it, so it must depend on nothing but
// the standard library.
//
// Why the parser is lenient: FTS-4010 only says a robot must "clearly quote
// all the original via lines" -- it prescribes no format for the quote, and
// the Via lines themselves come in the FTS-4009 §2 canonical form, the
// deprecated §4 comma form, and a long tail of mailer-specific spellings
// (some without any address at all, e.g. "ZoneGate V8.1 by ..., id : 493C").
// The parser therefore extracts what it can recognise -- an FTN address, a
// timestamp, the software text -- and always keeps the raw line, so nothing a
// hop wrote is lost even when nothing in it could be parsed.
package pingtrace

import (
	"regexp"
	"strings"
	"time"
)

// Hop is one node a message passed through, as recorded by that node's
// Via line.
type Hop struct {
	// Address is the FTN address the line names, without any @domain
	// suffix ("2:5020/469", "2:5020/469.12"). Empty when the line carries
	// no recognisable address.
	Address string
	// Time is when the hop processed the message. Zero when the line has
	// no recognisable timestamp. FTS-4009 §2 timestamps carry an explicit
	// "UTC" marker only optionally; TimeIsUTC says whether one was present.
	Time      time.Time
	TimeIsUTC bool
	// Software is the free text left after the address and timestamp are
	// removed: normally the program name and version.
	Software string
	// Raw is the line as found, with the kludge/quote prefix stripped.
	Raw string
}

var (
	// ftnAddrRe matches a 3D/4D FTN address with an optional @domain.
	// The domain is captured separately so it can be dropped: paths are
	// compared against nodelist addresses, which carry the domain as a
	// column, not as a suffix.
	ftnAddrRe = regexp.MustCompile(`(?:^|[^0-9A-Za-z])(\d{1,5}:\d{1,5}/\d{1,5}(?:\.\d{1,5})?)(@[A-Za-z0-9_.-]+)?`)

	// canonicalTimeRe is FTS-4009 §2: "@YYYYMMDD.HHMMSS[.Wxyz][.UTC]", where
	// several mailers write HHMM rather than HHMMSS and some omit the "@".
	canonicalTimeRe = regexp.MustCompile(`@?\b(\d{8})[.\s](\d{6}|\d{4})(?:\.W[A-Za-z0-9]{0,4})?(\.UTC)?\b`)

	// clockTimeRe is the deprecated FTS-4009 §4 spelling "YYYYMMDD HH:MM[:SS]".
	clockTimeRe = regexp.MustCompile(`\b(\d{8})\s+(\d{2}):(\d{2})(?::(\d{2}))?\b`)

	// isoTimeRe is the "YYYY-MM-DD HH:MM[:SS]" / "YYYY.MM.DD HH:MM:SS" spelling.
	isoTimeRe = regexp.MustCompile(`\b(\d{4})[.-](\d{2})[.-](\d{2})[ T](\d{2}):(\d{2})(?::(\d{2}))?\b`)

	// viaPrefixRe strips the visible or wire spelling of the kludge tag:
	// "\x01Via", "^AVia", "@Via", "Via", optionally followed by ":".
	viaPrefixRe = regexp.MustCompile(`(?i)^(?:\x01|\^A|@)?Via:?\s+`)

	// quotePrefixRe strips reply-quote furniture a robot may put in front
	// of a quoted line (" > ", ">>", leading blanks).
	quotePrefixRe = regexp.MustCompile(`^[\s>]+`)

	// bareViaRe recognises a canonical FTS-4009 line quoted without any
	// tag at all ("2:5020/715 @20260903.123000 hpt/lnx 1.9.0"): an address
	// followed by the @-timestamp is unambiguous enough to count as a hop
	// even with no "Via" in front of it.
	bareViaRe = regexp.MustCompile(`^\d{1,5}:\d{1,5}/\d{1,5}(?:\.\d{1,5})?(?:@[A-Za-z0-9_.-]+)?\s+@?\d{8}[.\s]\d{4,6}\b`)

	spaceRe = regexp.MustCompile(`\s+`)
)

// ParseViaLine parses the body of one Via line (with or without its
// "^AVia"/"@Via" prefix). It returns ok=false only for an empty line; a
// line without any recognisable field still yields a Hop carrying Raw.
func ParseViaLine(line string) (Hop, bool) {
	line = strings.TrimSpace(strings.TrimRight(line, "\r\n"))
	line = viaPrefixRe.ReplaceAllString(line, "")
	line = strings.TrimSpace(line)
	if line == "" {
		return Hop{}, false
	}
	h := Hop{Raw: line}
	rest := line

	if m := ftnAddrRe.FindStringSubmatchIndex(rest); m != nil {
		h.Address = rest[m[2]:m[3]]
		// Remove the whole address token including the @domain suffix.
		end := m[3]
		if m[4] >= 0 {
			end = m[5]
		}
		rest = rest[:m[2]] + " " + rest[end:]
	}

	if m := canonicalTimeRe.FindStringSubmatchIndex(rest); m != nil {
		date := rest[m[2]:m[3]]
		clock := rest[m[4]:m[5]]
		if len(clock) == 4 {
			clock += "00"
		}
		if t, err := time.Parse("20060102150405", date+clock); err == nil {
			h.Time = t
			h.TimeIsUTC = m[6] >= 0
		}
		rest = rest[:m[0]] + " " + rest[m[1]:]
	} else if m := clockTimeRe.FindStringSubmatchIndex(rest); m != nil {
		sub := clockTimeRe.FindStringSubmatch(rest)
		sec := sub[4]
		if sec == "" {
			sec = "00"
		}
		if t, err := time.Parse("20060102150405", sub[1]+sub[2]+sub[3]+sec); err == nil {
			h.Time = t
		}
		rest = rest[:m[0]] + " " + rest[m[1]:]
	} else if m := isoTimeRe.FindStringSubmatchIndex(rest); m != nil {
		sub := isoTimeRe.FindStringSubmatch(rest)
		sec := sub[6]
		if sec == "" {
			sec = "00"
		}
		if t, err := time.Parse("2006-01-02 15:04:05",
			sub[1]+"-"+sub[2]+"-"+sub[3]+" "+sub[4]+":"+sub[5]+":"+sec); err == nil {
			h.Time = t
		}
		rest = rest[:m[0]] + " " + rest[m[1]:]
	}

	// What is left is the software: tidy separators the removed tokens
	// left behind ("FidoMail 1.0, , " -> "FidoMail 1.0").
	rest = strings.ReplaceAll(rest, ",", " ")
	rest = spaceRe.ReplaceAllString(rest, " ")
	h.Software = strings.Trim(rest, " -;")
	return h, true
}

// ParseVias parses a message's own Via chain (the kludge values, in wire
// order) into hops. Every non-empty element becomes a hop.
func ParseVias(vias []string) []Hop {
	hops := make([]Hop, 0, len(vias))
	for _, v := range vias {
		if h, ok := ParseViaLine(v); ok {
			hops = append(hops, h)
		}
	}
	return hops
}

// ExtractPath finds the Via lines a robot quoted in a reply body and parses
// them in order. Only lines that start with a Via tag (after any quote
// furniture) count, so ordinary text mentioning an address is never
// mistaken for a hop. Consecutive duplicates are collapsed: some robots
// quote the chain twice (once as kludges, once as text).
func ExtractPath(body string) []Hop {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	var hops []Hop
	for _, line := range strings.Split(body, "\n") {
		line = quotePrefixRe.ReplaceAllString(line, "")
		if !viaPrefixRe.MatchString(line) && !bareViaRe.MatchString(line) {
			continue
		}
		h, ok := ParseViaLine(line)
		if !ok {
			continue
		}
		if n := len(hops); n > 0 && hops[n-1].Raw == h.Raw {
			continue
		}
		hops = append(hops, h)
	}
	return hops
}

// Addresses returns the non-empty addresses of hops, in order.
func Addresses(hops []Hop) []string {
	out := make([]string, 0, len(hops))
	for _, h := range hops {
		if h.Address != "" {
			out = append(out, h.Address)
		}
	}
	return out
}

// TimeIsUTCInVia reports whether a stored raw Via line carried the optional
// FTS-4009 "UTC" marker. Hop.TimeIsUTC has no column of its own in the
// ping tables -- the raw line is kept verbatim, so the flag is re-derived
// from it on read rather than migrated in.
func TimeIsUTCInVia(raw string) bool {
	h, ok := ParseViaLine(raw)
	return ok && h.TimeIsUTC
}

// Node3D reduces an address to its zone:net/node form (a point's Via is
// attributed to its boss for path purposes).
func Node3D(addr string) string {
	if i := strings.IndexByte(addr, '.'); i >= 0 {
		return addr[:i]
	}
	return addr
}
