package domain

import (
	"net"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// ExtractRegistrableDomain extracts the registrable domain from a hostname.
// For example: "mail.example.co.uk" → "example.co.uk", "bbs.fido.net:24554" → "fido.net".
// Returns empty string for IP addresses, invalid hostnames, and private PSL suffixes.
func ExtractRegistrableDomain(hostname string) string {
	if hostname == "" {
		return ""
	}

	host := hostname

	// Strip port (handles both "host:port" and "[::1]:port")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Remove brackets from IPv6 addresses like "[::1]"
	host = strings.Trim(host, "[]")

	// Skip bare IP addresses
	if net.ParseIP(host) != nil {
		return ""
	}

	// Normalize: lowercase and trim trailing dot
	host = strings.ToLower(strings.TrimRight(host, "."))

	if host == "" {
		return ""
	}

	// Check if the suffix is ICANN-managed (not a private suffix like github.io)
	_, icann := publicsuffix.PublicSuffix(host)
	if !icann {
		return ""
	}

	// Extract registrable domain (eTLD+1)
	d, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return ""
	}

	return d
}

// ExtractUniqueDomains extracts unique registrable domains from a list of hostnames.
// Returns a map of domain → count of hostnames mapping to it.
func ExtractUniqueDomains(hostnames []string) map[string]int {
	domains := make(map[string]int)
	for _, hostname := range hostnames {
		d := ExtractRegistrableDomain(hostname)
		if d != "" {
			domains[d]++
		}
	}
	return domains
}
