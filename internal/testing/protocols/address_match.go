package protocols

import "strings"

// announcedAddressMatches reports whether a remote announced the FTN address we
// expected to reach.
//
// This lived in fidomail as emsi.Session.ValidateAddress until that package
// dropped it (ab5eb06) for having no callers inside fidomail and for being the
// second of two competing wire-address normalizations there. It did have a
// caller - this package - and the normalization it performs is the one whose
// results are already recorded in a year of address_validated history, so it
// moves here rather than being re-expressed through ftn.ParseFTNAddress:
// changing what counts as a match would silently reinterpret that column, and
// now also dns_fallback_address_validated, which exists precisely to tell the
// real node apart from whoever holds its old IP.
func announcedAddressMatches(announced []string, expected string) bool {
	if len(announced) == 0 || expected == "" {
		return false
	}

	want := normalizeFTNAddress(expected)
	for _, addr := range announced {
		if normalizeFTNAddress(addr) == want {
			return true
		}
	}
	return false
}

// normalizeFTNAddress canonicalizes an address as announced on the wire, where
// case, surrounding space, an @domain suffix and an explicit point 0 are all
// noise: 2:5030/0.0 and " 2:5030/0@FidoNet " are the same node as 2:5030/0.
func normalizeFTNAddress(addr string) string {
	addr = strings.TrimSpace(strings.ToLower(addr))

	if idx := strings.Index(addr, "@"); idx != -1 {
		addr = addr[:idx]
	}

	// Point 0 is implicit, so a node announcing it is still announcing itself.
	addr = strings.TrimSuffix(addr, ".0")

	return addr
}
