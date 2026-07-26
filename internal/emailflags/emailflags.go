// Package emailflags interprets the FidoNet nodelist flags that advertise mail
// transport over Internet email.
//
// The authority is FTS-5001 rev 4 (10 March 2013), section "Email Flags",
// which defines exactly five: IEM, ITX, IUC, IMI and ISE. EMA and EVY are
// recognised here as well because they occur in real nodelists, but they are
// defined by no FTSC document and are reported as non-standard.
//
// Three properties of the standard drive the design:
//
//   - "No flag implies another. Each capability MUST be specifically" listed
//     (FTS-5001 rev 4 line 372). Nothing here infers one flag from another.
//   - IEM "sets the default email address for other flags (similar to INA)"
//     (line 515), while INA itself "sets the default Internet address used for
//     any non-email based flag" (lines 484-486). INA is therefore never
//     consulted for an email address.
//   - "The e-mail flags do not carry a port number" (line 524), so a flag is
//     split on its first colon only and the whole remainder is the address.
package emailflags

import (
	"sort"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// Method classifies the transport a flag advertises.
type Method int

const (
	// MethodUnspecified is IEM: email transport whose wire format the
	// nodelist does not identify.
	MethodUnspecified Method = iota
	MethodUUEncode           // IUC
	MethodMIME               // IMI
	MethodTransX             // ITX
	MethodSEAT               // ISE
	MethodVoyager            // EVY, non-standard
	MethodOther              // EMA, non-standard
)

// Source records which rule supplied a capability's address.
type Source int

const (
	// SourceUnresolved means no address could be found for the capability.
	SourceUnresolved Source = iota
	// SourceExplicit means the address was attached to this flag.
	SourceExplicit
	// SourceIEMDefault means the address came from IEM, per FTS-5001's
	// default-email-address rule.
	SourceIEMDefault
	// SourceOtherEmailFlag means the address was borrowed from a different
	// email flag, per "Be prepared to look for addresses under any flag of
	// the same type" (FTS-5001 rev 4 lines 422-424).
	SourceOtherEmailFlag
	// SourceLocationField and SourceSystemName mean the address was recovered
	// from a non-flag nodelist field, per FSP-1012 section 2.3.4. Both are
	// heuristic and are only used when explicitly enabled.
	SourceLocationField
	SourceSystemName
)

func (s Source) String() string {
	switch s {
	case SourceExplicit:
		return "explicit"
	case SourceIEMDefault:
		return "IEM default"
	case SourceOtherEmailFlag:
		return "other email flag"
	case SourceLocationField:
		return "location field"
	case SourceSystemName:
		return "system name"
	default:
		return "unresolved"
	}
}

// spec describes one recognised email flag.
type spec struct {
	method Method
	// standard is true for the five flags FTS-5001 defines.
	standard bool
	// receiptRequired marks the flags FTS-5001 rev 4 lines 506-508 require to
	// have receipts enabled and answered within 24 hours.
	receiptRequired bool
	// wireProtocolSpecified is true only where a complete FTSC wire
	// specification exists. ISE maps to SEAT (FTS-1025); every other flag
	// names an encoding or a product, not an interoperable message format.
	wireProtocolSpecified bool
}

// specs is keyed by the canonical (upper-case) flag name.
var specs = map[string]spec{
	"IEM": {method: MethodUnspecified, standard: true},
	"IUC": {method: MethodUUEncode, standard: true},
	"IMI": {method: MethodMIME, standard: true},
	"ITX": {method: MethodTransX, standard: true, receiptRequired: true},
	"ISE": {method: MethodSEAT, standard: true, receiptRequired: true, wireProtocolSpecified: true},
	"EMA": {method: MethodOther},
	"EVY": {method: MethodVoyager},
}

// order fixes the presentation sequence: IEM first as the address-bearing
// default, then the standard methods, then the non-standard ones.
var order = []string{"IEM", "IMI", "ITX", "ISE", "IUC", "EMA", "EVY"}

var orderIndex = func() map[string]int {
	m := make(map[string]int, len(order))
	for i, f := range order {
		m[f] = i
	}
	return m
}()

// IsEmailFlag reports whether name is a recognised email transport flag.
// name may carry an address ("IMI:user@example.org").
func IsEmailFlag(name string) bool {
	flag, _, _ := splitFlag(name)
	_, ok := specs[flag]
	return ok
}

// Capability is one email transport a node advertises.
//
// A flag repeated for a multihomed system (FTS-5001 rev 4 lines 426-434)
// collapses into a single Capability carrying every advertised address, since
// the repetition denotes alternative endpoints for one capability rather than
// distinct capabilities.
type Capability struct {
	// Flag is the canonical upper-case flag name.
	Flag   string
	Method Method
	// Standard is true when FTS-5001 defines the flag.
	Standard bool
	// ReceiptRequired is true for ITX and ISE.
	ReceiptRequired bool
	// WireProtocolSpecified is true only for ISE (SEAT, FTS-1025).
	WireProtocolSpecified bool

	// Addresses holds every address resolved for this capability, in the
	// order discovered, deduplicated case-insensitively.
	Addresses []string
	// Source says which rule supplied Addresses. Resolution stops at the
	// first tier that yields anything, so all addresses share one source.
	Source Source
	// Occurrences counts how many times the flag appeared on the line.
	Occurrences int
	// Malformed lists advertised values that do not look like email
	// addresses. They are reported rather than discarded so bad nodelist
	// data stays visible.
	Malformed []string
}

// Resolved reports whether the capability has at least one usable address.
func (c Capability) Resolved() bool { return len(c.Addresses) > 0 }

// Options tunes address recovery from non-flag nodelist fields.
type Options struct {
	// Location and SystemName are the nodelist fields of the same name.
	Location   string
	SystemName string
	// UseFieldFallback enables recovering an address from Location and then
	// SystemName when no flag carries one (FSP-1012 section 2.3.4). It is
	// off by default: FTS-5001 rev 4 lines 441-450 warn that these fields are
	// easily mistaken for Internet data, so the recovery is a guess.
	UseFieldFallback bool
}

// Extract returns the email capabilities a node advertises.
//
// It reads both the raw flags slice and the parsed internet configuration,
// because the two have historically disagreed about where email flags land:
// rows written before the parser was made consistent kept a bare IEM and any
// addressed IUC/EMA/EVY verbatim in flags, while everything else went to
// internet_config. Reading only one source undercounts. ic may be nil.
func Extract(flags []string, ic *database.InternetConfiguration, opts Options) []Capability {
	// occurrence counts and raw advertised values, keyed by canonical flag.
	counts := make(map[string]int, len(specs))
	values := make(map[string][]string, len(specs))

	note := func(flag, value string) {
		counts[flag]++
		if value != "" {
			values[flag] = append(values[flag], value)
		}
	}

	// Source 1: the raw flags slice (pre-fix rows, and anything the parser
	// did not recognise).
	for _, raw := range flags {
		flag, value, _ := splitFlag(raw)
		if _, ok := specs[flag]; !ok {
			continue
		}
		note(flag, value)
	}

	// Source 2: the parsed internet configuration.
	if ic != nil {
		for flag, details := range ic.EmailProtocols {
			canonical := strings.ToUpper(strings.TrimSpace(flag))
			if _, ok := specs[canonical]; !ok {
				continue
			}
			for _, d := range details {
				note(canonical, strings.TrimSpace(d.Email))
			}
		}
		// An addressed IEM is stored among the defaults rather than under
		// email_protocols, because that is where the resolver looks for the
		// node's default email address.
		for _, addr := range ic.Defaults["IEM"] {
			note("IEM", strings.TrimSpace(addr))
		}
	}

	if len(counts) == 0 {
		return nil
	}

	// Partition advertised values into usable addresses and malformed ones.
	addresses := make(map[string][]string, len(counts))
	malformed := make(map[string][]string, len(counts))
	for flag, vals := range values {
		for _, v := range vals {
			if looksLikeEmail(v) {
				addresses[flag] = appendUnique(addresses[flag], v)
			} else {
				malformed[flag] = appendUnique(malformed[flag], v)
			}
		}
	}

	// Fallback tiers, computed once.
	iemDefaults := addresses["IEM"]
	var anyEmailAddress []string
	for _, flag := range order {
		for _, a := range addresses[flag] {
			anyEmailAddress = appendUnique(anyEmailAddress, a)
		}
	}

	result := make([]Capability, 0, len(counts))
	for flag, count := range counts {
		sp := specs[flag]
		cap := Capability{
			Flag:                  flag,
			Method:                sp.method,
			Standard:              sp.standard,
			ReceiptRequired:       sp.receiptRequired,
			WireProtocolSpecified: sp.wireProtocolSpecified,
			Occurrences:           count,
			Malformed:             malformed[flag],
		}
		cap.Addresses, cap.Source = resolve(flag, addresses[flag], iemDefaults, anyEmailAddress, opts)
		result = append(result, cap)
	}

	sort.Slice(result, func(i, j int) bool {
		return orderIndex[result[i].Flag] < orderIndex[result[j].Flag]
	})
	return result
}

// resolve applies the address-resolution tiers for one flag.
//
// FTS-5001 states the IEM default-address mechanism (line 515) and the
// look-under-any-flag-of-the-same-type guidance (lines 422-424) without
// ranking them against each other. Preferring an explicit address, then IEM,
// then any other email flag is this implementation's reading, chosen because
// it moves from most specific to least. Flag order on the line is never
// consulted, which the standard does make explicit.
func resolve(flag string, explicit, iemDefaults, anyEmail []string, opts Options) ([]string, Source) {
	if len(explicit) > 0 {
		return explicit, SourceExplicit
	}
	if flag != "IEM" && len(iemDefaults) > 0 {
		return iemDefaults, SourceIEMDefault
	}
	if len(anyEmail) > 0 {
		return anyEmail, SourceOtherEmailFlag
	}
	if opts.UseFieldFallback {
		if addr := fieldAddress(opts.Location); addr != "" {
			return []string{addr}, SourceLocationField
		}
		if addr := fieldAddress(opts.SystemName); addr != "" {
			return []string{addr}, SourceSystemName
		}
	}
	return nil, SourceUnresolved
}

// splitFlag separates a nodelist flag from its parameter at the first colon.
// The email flags carry no port, so the entire remainder is the value.
func splitFlag(raw string) (flag, value string, hasValue bool) {
	raw = strings.TrimSpace(raw)
	name, param, found := strings.Cut(raw, ":")
	return strings.ToUpper(strings.TrimSpace(name)), strings.TrimSpace(param), found
}

// looksLikeEmail applies a deliberately permissive check: it rejects what
// cannot be an address rather than enforcing a narrow historical pattern, so
// that unusual but published addresses survive into the report. The nodelist
// is 7-bit ASCII with comma-separated fields, so an address can contain
// neither a comma nor a space.
func looksLikeEmail(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t,") {
		return false
	}
	at := strings.LastIndex(s, "@")
	if at <= 0 || at == len(s)-1 {
		return false
	}
	domain := s[at+1:]
	// A bare host with no dot cannot be a public mail domain, and rejecting
	// it keeps the Location-field heuristic from matching prose.
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	// The domain is checked strictly, unlike the local part. A stray colon
	// here is the common malformation: the email flags carry no port
	// (FTS-5001 rev 4 line 524), so "user@host:something" is bad data rather
	// than an address with a port, and must not be silently accepted.
	for _, r := range domain {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-':
		default:
			return false
		}
	}
	return true
}

// fieldAddress recovers an address from a free-text nodelist field. Nodelist
// fields use underscores where the source text had spaces, so the field is
// split on both before each token is tested.
func fieldAddress(field string) string {
	if field == "" {
		return ""
	}
	for _, token := range strings.FieldsFunc(field, func(r rune) bool {
		return r == '_' || r == ' ' || r == '\t'
	}) {
		token = strings.Trim(token, "()<>[]\"';")
		if looksLikeEmail(token) {
			return token
		}
	}
	return ""
}

func appendUnique(list []string, value string) []string {
	for _, existing := range list {
		if strings.EqualFold(existing, value) {
			return list
		}
	}
	return append(list, value)
}
