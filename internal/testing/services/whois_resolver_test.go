package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

// stubWhoisClient is a scriptable whoisClient for tests. ianaFn answers bare-TLD
// (IANA) queries; domainFn answers domain queries routed to an explicit server.
type stubWhoisClient struct {
	ianaFn    func(tld string) (string, error)
	domainFn  func(domain, server string) (string, error)
	ianaCalls int
}

func (s *stubWhoisClient) Whois(domain string, servers ...string) (string, error) {
	if len(servers) == 0 {
		s.ianaCalls++
		if s.ianaFn != nil {
			return s.ianaFn(domain)
		}
		return "whois:        whois.example-registry.test\n", nil
	}
	if s.domainFn != nil {
		return s.domainFn(domain, servers[0])
	}
	return "", nil
}

func newTestResolver(client whoisClient) *WhoisResolver {
	r := &WhoisResolver{
		timeout:         time.Second,
		client:          client,
		tldServerTTL:    time.Hour,
		rateLimiter:     make(chan struct{}, 1),
		rateLimiterDone: make(chan struct{}),
	}
	r.rateLimiter <- struct{}{}
	return r
}

const cannedPorkbunResponse = `Domain Name: SOG.ONE
Registrar WHOIS Server: https://porkbun.com/whois
Updated Date: 2026-04-15T01:35:58Z
Creation Date: 2018-05-26T13:34:50Z
Registry Expiry Date: 2028-05-26T13:34:50Z
Registrar: Porkbun
Registrar IANA ID: 1861
Domain Status: clientTransferProhibited https://icann.org/epp#clientTransferProhibited
`

// Phase 1: a referral-failure response (registry data + non-nil error) must be
// rescued — registrar and expiry parsed, Error cleared.
func TestLookupWhois_RescueOnReferralFailure(t *testing.T) {
	referralErr := errors.New("whois: connect to whois server failed: connection refused")
	client := &stubWhoisClient{
		ianaFn:   func(tld string) (string, error) { return "whois:        whois.nic.one\n", nil },
		domainFn: func(domain, server string) (string, error) { return cannedPorkbunResponse, referralErr },
	}
	r := newTestResolver(client)

	got := r.lookupWhois("sog.one")

	if got.Error != "" {
		t.Fatalf("expected rescued success, got Error=%q", got.Error)
	}
	if got.Registrar != "Porkbun" {
		t.Errorf("registrar = %q, want Porkbun", got.Registrar)
	}
	if got.ExpirationDate == nil {
		t.Errorf("expiration date not parsed")
	}
}

// A total primary-query failure (empty body + error) must stay a transient error.
func TestLookupWhois_TotalFailureIsTransient(t *testing.T) {
	client := &stubWhoisClient{
		ianaFn:   func(tld string) (string, error) { return "whois:        whois.nic.one\n", nil },
		domainFn: func(domain, server string) (string, error) { return "", errors.New("i/o timeout") },
	}
	r := newTestResolver(client)

	got := r.lookupWhois("sog.one")

	if got.Error == "" || got.Error == models.WhoisNoServerError || got.Error == "domain not found" {
		t.Fatalf("expected transient error, got Error=%q", got.Error)
	}
	if !strings.HasPrefix(got.Error, "WHOIS lookup failed") {
		t.Errorf("Error = %q, want WHOIS lookup failed prefix", got.Error)
	}
}

// Phase 2a/2b: when IANA reports no WHOIS server for the TLD, the result carries
// the stable WhoisNoServerError marker.
func TestLookupWhois_NoWhoisServer(t *testing.T) {
	client := &stubWhoisClient{
		ianaFn: func(tld string) (string, error) { return "whois:        \n", nil }, // empty value = RDAP-only
	}
	r := newTestResolver(client)

	got := r.lookupWhois("jacobcat.app")

	if got.Error != models.WhoisNoServerError {
		t.Fatalf("Error = %q, want %q", got.Error, models.WhoisNoServerError)
	}
}

// A transient IANA failure must NOT be cached as a negative and must be transient.
func TestQueryWhois_TransientIANAFailureNotCached(t *testing.T) {
	client := &stubWhoisClient{
		ianaFn: func(tld string) (string, error) { return "", errors.New("iana timeout") },
	}
	r := newTestResolver(client)

	if _, err := r.queryWhois("example.one"); err == nil || errors.Is(err, errNoWhoisServer) {
		t.Fatalf("expected transient (non-no-server) error, got %v", err)
	}
	if _, ok := r.loadTLDServer("one"); ok {
		t.Errorf("transient IANA failure must not populate the TLD cache")
	}
}

// Critical: a PARTIAL IANA body returned alongside an error must NOT poison the TLD
// cache with a negative entry — the whole TLD would go dark for 24h otherwise.
func TestQueryWhois_PartialIANABodyWithErrorNotCached(t *testing.T) {
	client := &stubWhoisClient{
		// Body cut off before the whois: line, plus a read error.
		ianaFn: func(tld string) (string, error) {
			return "domain:       ONE\norganisation: nic\n", errors.New("i/o timeout")
		},
	}
	r := newTestResolver(client)

	_, err := r.queryWhois("example.one")
	if err == nil || errors.Is(err, errNoWhoisServer) {
		t.Fatalf("partial IANA body + error must be transient, got %v", err)
	}
	if _, ok := r.loadTLDServer("one"); ok {
		t.Errorf("partial IANA read must not populate the TLD cache")
	}
}

// InvalidateCache drops the in-memory entry so a failed persist can be retried.
func TestInvalidateCache(t *testing.T) {
	r := newTestResolver(&stubWhoisClient{})
	r.cacheResult("example.one", &models.WhoisResult{Domain: "example.one", Registrar: "X"})
	if _, ok := r.cache.Load("example.one"); !ok {
		t.Fatal("precondition: entry should be cached")
	}
	r.InvalidateCache("example.one")
	if _, ok := r.cache.Load("example.one"); ok {
		t.Errorf("InvalidateCache did not remove the entry")
	}
}

// safeWhois converts a panicking client (the vendored getServer slice-bounds bug)
// into a transient error instead of crashing the daemon.
func TestSafeWhois_RecoversPanic(t *testing.T) {
	panicky := &stubWhoisClient{
		ianaFn:   func(tld string) (string, error) { return "whois:        whois.nic.one\n", nil },
		domainFn: func(domain, server string) (string, error) { panic("slice bounds out of range") },
	}
	got := newTestResolver(panicky).lookupWhois("boom.one")
	if got.Error == "" || got.Registrar != "" {
		t.Fatalf("expected transient error from recovered panic, got %+v", got)
	}
}

// stubRDAP is a scriptable rdapFallback for tests.
type stubRDAP struct {
	fn func(domain string) *models.WhoisResult
}

func (s *stubRDAP) Lookup(_ context.Context, domain string) *models.WhoisResult {
	return s.fn(domain)
}

// Issue #4: when WHOIS finds no server and RDAP returns an authoritative but EMPTY
// (e.g. GDPR-redacted) success, the final result must be Error=="" so the worker
// marks it seen — not a synthetic transient error that retries forever.
func TestResolve_RDAPEmptySuccessIsStable(t *testing.T) {
	client := &stubWhoisClient{
		ianaFn: func(tld string) (string, error) { return "whois:        \n", nil }, // no server
	}
	r := newTestResolver(client)
	r.rdap = &stubRDAP{fn: func(domain string) *models.WhoisResult {
		return &models.WhoisResult{Domain: domain} // 200 OK, no usable data
	}}

	got := r.doResolve(context.Background(), "redacted.app")
	if got.Error != "" {
		t.Errorf("RDAP empty success must yield a stable (Error==\"\") result, got Error=%q", got.Error)
	}
	if got.HasUsableData() {
		t.Errorf("expected no usable data")
	}
}

// Issue #4 (continued): a genuine RDAP transient failure after a no-server WHOIS
// stays transient so it is retried.
func TestResolve_RDAPTransientFailureRetries(t *testing.T) {
	client := &stubWhoisClient{
		ianaFn: func(tld string) (string, error) { return "whois:        \n", nil },
	}
	r := newTestResolver(client)
	r.rdap = &stubRDAP{fn: func(domain string) *models.WhoisResult {
		return &models.WhoisResult{Domain: domain, Error: "rdap: connection refused"}
	}}

	got := r.doResolve(context.Background(), "flaky.app")
	if got.Error == "" || got.Error == models.WhoisNoServerError || got.Error == "domain not found" {
		t.Errorf("RDAP transient failure must stay transient, got Error=%q", got.Error)
	}
}

// The rescue gate rejects thin data on referral failure: no registrar and no
// expiration means the result stays transient rather than persisting a stub.
func TestLookupWhois_RescueGateRejectsThinData(t *testing.T) {
	referralErr := errors.New("whois: connect to whois server failed: connection refused")
	client := &stubWhoisClient{
		ianaFn:   func(tld string) (string, error) { return "whois:        whois.nic.test\n", nil },
		domainFn: func(domain, server string) (string, error) { return "Domain Status: ok\n", referralErr },
	}
	r := newTestResolver(client)

	got := r.lookupWhois("thin.test")

	if got.Registrar != "" || got.ExpirationDate != nil {
		t.Fatalf("expected no usable data, got registrar=%q expiry=%v", got.Registrar, got.ExpirationDate)
	}
	if got.Error == "" || got.Error == models.WhoisNoServerError || got.Error == "domain not found" {
		t.Errorf("expected transient error, got Error=%q", got.Error)
	}
}

// A clean primary query that whois-parser flags as not-found is classified
// "domain not found"; the same body WITH a lookup error is NOT (stays transient).
func TestLookupWhois_NotFoundClassification(t *testing.T) {
	const notFound = "No match for \"NOTHERE.ONE\".\n"

	cleanClient := &stubWhoisClient{
		ianaFn:   func(tld string) (string, error) { return "whois:        whois.nic.one\n", nil },
		domainFn: func(domain, server string) (string, error) { return notFound, nil },
	}
	if got := newTestResolver(cleanClient).lookupWhois("nothere.one"); got.Error != "domain not found" {
		t.Fatalf("clean not-found: Error = %q, want domain not found", got.Error)
	}

	erroredClient := &stubWhoisClient{
		ianaFn:   func(tld string) (string, error) { return "whois:        whois.nic.one\n", nil },
		domainFn: func(domain, server string) (string, error) { return notFound, errors.New("connection refused") },
	}
	if got := newTestResolver(erroredClient).lookupWhois("nothere.one"); got.Error == "domain not found" {
		t.Errorf("errored not-found stub must not be classified as domain not found")
	}
}

// Phase 2a: two domains under the same TLD trigger exactly one IANA query.
func TestQueryWhois_TLDServerCachedOnce(t *testing.T) {
	client := &stubWhoisClient{
		ianaFn:   func(tld string) (string, error) { return "whois:        whois.nic.one\n", nil },
		domainFn: func(domain, server string) (string, error) { return cannedPorkbunResponse, nil },
	}
	r := newTestResolver(client)

	for _, d := range []string{"a.one", "b.one"} {
		if _, err := r.queryWhois(d); err != nil {
			t.Fatalf("queryWhois(%s): %v", d, err)
		}
	}
	if client.ianaCalls != 1 {
		t.Errorf("IANA queried %d times, want 1 (TLD server should be cached)", client.ianaCalls)
	}
}

func TestWhoisExtension(t *testing.T) {
	tests := map[string]string{
		"sog.one":            "one",
		"mail.example.co.uk": "uk",
		"bbs.fido.net:24554": "net",
		"EXAMPLE.COM":        "com",
		"example.com.":       "com",
		"1.2.3.4":            "",
		"":                   "",
	}
	for in, want := range tests {
		if got := whoisExtension(in); got != want {
			t.Errorf("whoisExtension(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractWhoisServer(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"present", "domain:       COM\nwhois:        whois.verisign-grs.com\n", "whois.verisign-grs.com"},
		{"empty value", "domain:       APP\nwhois:        \n", ""},
		{"missing", "domain:       EXAMPLE\nstatus:       ACTIVE\n", ""},
		{"no trailing newline", "whois:        whois.nic.uk", "whois.nic.uk"},
		{"case insensitive", "WHOIS:        whois.nic.one\n", "whois.nic.one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractWhoisServer(tt.in); got != tt.want {
				t.Errorf("extractWhoisServer() = %q, want %q", got, tt.want)
			}
		})
	}
}
