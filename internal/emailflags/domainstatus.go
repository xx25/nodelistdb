package emailflags

import (
	"strings"
	"time"
)

// MXHost is one mail exchanger published by a domain.
type MXHost struct {
	Preference uint16 `json:"preference"`
	Host       string `json:"host"`
	// Resolved is true when the host has at least one A or AAAA record.
	Resolved bool `json:"resolved"`
	// Addresses counts the IPs the host resolves to.
	Addresses int `json:"addresses"`
}

// DomainResult is the DNS verdict for one mail domain.
//
// It lives here rather than beside the resolver because the testdaemon
// produces it and the storage layer persists it, and those two packages
// cannot import each other.
type DomainResult struct {
	// Domain is the lower-cased domain as published.
	Domain string `json:"domain"`
	// ASCIIDomain is the punycode form actually queried. It differs from
	// Domain only for internationalised names.
	ASCIIDomain string    `json:"ascii_domain,omitempty"`
	Status      string    `json:"status"`
	Detail      string    `json:"detail"`
	MXHosts     []MXHost  `json:"mx_hosts,omitempty"`
	CheckTime   time.Time `json:"check_time"`
	// Error carries the transient failure text, if any.
	Error string `json:"error,omitempty"`
}

// Stable reports whether the verdict may overwrite a stored result.
func (r DomainResult) Stable() bool { return DomainStatusStable(r.Status) }

// Routable reports whether mail can currently be delivered to the domain.
func (r DomainResult) Routable() bool { return DomainStatusRoutable(r.Status) }

// Verdicts produced by DNS verification of a published mail domain.
//
// They are stored as strings so the set can grow without a schema change, and
// they live in this dependency-light package because the testdaemon writes
// them and the web layer reads them.
const (
	// DomainStatusOK means the domain publishes at least one MX host and
	// every one of them resolves to an address.
	DomainStatusOK = "ok"
	// DomainStatusImplicitMX means the domain publishes no MX but does
	// resolve, so mail routes to it directly (RFC 5321 section 5.1).
	DomainStatusImplicitMX = "implicit_mx"
	// DomainStatusDegraded means some MX hosts resolve and others do not.
	// Mail still routes, but the configuration is decaying.
	DomainStatusDegraded = "degraded"
	// DomainStatusNoDomain means the domain does not exist, or exists with
	// no usable records. Go's resolver reports NXDOMAIN and NODATA
	// identically, and for this report both mean the same thing: there is
	// nowhere to deliver.
	DomainStatusNoDomain = "no_domain"
	// DomainStatusNullMX means the domain publishes RFC 7505 null MX
	// ("0 ."), explicitly declaring that it accepts no mail.
	DomainStatusNullMX = "null_mx"
	// DomainStatusNoMX means the domain exists but has neither an MX nor an
	// address record.
	DomainStatusNoMX = "no_mx"
	// DomainStatusMXUnresolvable means every published MX host fails to
	// resolve, a common decay mode for dynamic-DNS mail hosts.
	DomainStatusMXUnresolvable = "mx_unresolvable"
	// DomainStatusInvalid means the published address is not usable as an
	// email address, so no lookup was attempted.
	DomainStatusInvalid = "invalid"
	// DomainStatusError means the lookup failed transiently (SERVFAIL,
	// timeout). It is never persisted over a previous good verdict.
	DomainStatusError = "error"
)

// DomainStatusRoutable reports whether a verdict means mail can currently be
// delivered to the domain.
func DomainStatusRoutable(status string) bool {
	switch status {
	case DomainStatusOK, DomainStatusImplicitMX, DomainStatusDegraded:
		return true
	default:
		return false
	}
}

// DomainStatusStable reports whether a verdict is a settled fact about the
// domain rather than a transient lookup failure. Only stable verdicts may
// overwrite a previously stored result.
func DomainStatusStable(status string) bool {
	return status != "" && status != DomainStatusError
}

// MailDomain returns the lower-cased domain part of an email address, or ""
// if the address has no usable domain.
func MailDomain(address string) string {
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(address[at+1:]))
}

// RedactLocalPart replaces everything before the last "@" with an ellipsis,
// so a value can be shown for diagnosis without publishing a mailbox.
//
// It exists for values the report shows verbatim because they are malformed:
// "user@example.net:extra" is not a usable address, but it still contains one,
// and the report deliberately does not put mailboxes on a web page.
func RedactLocalPart(value string) string {
	at := strings.LastIndex(value, "@")
	if at <= 0 {
		return value
	}
	return "…" + value[at:]
}
