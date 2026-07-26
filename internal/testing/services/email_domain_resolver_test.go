package services

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/nodelistdb/internal/emailflags"
)

func TestIsNullMX(t *testing.T) {
	tests := []struct {
		name    string
		records []*net.MX
		want    bool
	}{
		{
			name:    "RFC 7505 null MX",
			records: []*net.MX{{Pref: 0, Host: "."}},
			want:    true,
		},
		{
			name:    "null MX with an empty host",
			records: []*net.MX{{Pref: 0, Host: ""}},
			want:    true,
		},
		{
			name:    "ordinary single MX",
			records: []*net.MX{{Pref: 10, Host: "mx.example.net."}},
			want:    false,
		},
		{
			// Only a lone preference-0 root target is a null MX. A root
			// target alongside real hosts is a misconfiguration, not a
			// declaration that the domain refuses mail.
			name:    "root target alongside a real host is not a null MX",
			records: []*net.MX{{Pref: 0, Host: "."}, {Pref: 10, Host: "mx.example.net."}},
			want:    false,
		},
		{
			name:    "preference 0 pointing at a real host",
			records: []*net.MX{{Pref: 0, Host: "mx.example.net."}},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNullMX(tt.records); got != tt.want {
				t.Errorf("isNullMX() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMailDomain(t *testing.T) {
	tests := []struct{ address, want string }{
		{"user@example.net", "example.net"},
		{"User@EXAMPLE.NET", "example.net"},
		{"first.last+tag@mail.example.co.uk", "mail.example.co.uk"},
		{"weird@user@example.net", "example.net"},
		{"no-at-sign", ""},
		{"@example.net", ""},
		{"user@", ""},
		{"", ""},
	}

	for _, tt := range tests {
		if got := emailflags.MailDomain(tt.address); got != tt.want {
			t.Errorf("MailDomain(%q) = %q, want %q", tt.address, got, tt.want)
		}
	}
}

func TestDomainStatusClassification(t *testing.T) {
	routable := []string{
		emailflags.DomainStatusOK,
		emailflags.DomainStatusImplicitMX,
		emailflags.DomainStatusDegraded,
	}
	for _, status := range routable {
		if !emailflags.DomainStatusRoutable(status) {
			t.Errorf("%s should count as routable", status)
		}
	}

	notRoutable := []string{
		emailflags.DomainStatusNoDomain,
		emailflags.DomainStatusNullMX,
		emailflags.DomainStatusNoMX,
		emailflags.DomainStatusMXUnresolvable,
		emailflags.DomainStatusInvalid,
		emailflags.DomainStatusError,
	}
	for _, status := range notRoutable {
		if emailflags.DomainStatusRoutable(status) {
			t.Errorf("%s should not count as routable", status)
		}
	}

	// Only a transient error is unstable; everything else may be persisted.
	if emailflags.DomainStatusStable(emailflags.DomainStatusError) {
		t.Error("a transient error must never overwrite a stored verdict")
	}
	if emailflags.DomainStatusStable("") {
		t.Error("an empty status is not a verdict")
	}
	for _, status := range append(routable, emailflags.DomainStatusNoDomain, emailflags.DomainStatusNullMX) {
		if !emailflags.DomainStatusStable(status) {
			t.Errorf("%s should be persistable", status)
		}
	}
}

func TestCheckAddressRejectsMalformed(t *testing.T) {
	r := NewEmailDomainResolver(EmailDomainResolverConfig{})

	for _, address := range []string{"", "not-an-address", "@example.net", "user@"} {
		got := r.CheckAddress(context.Background(), address)
		if got.Status != emailflags.DomainStatusInvalid {
			t.Errorf("CheckAddress(%q) = %s, want %s", address, got.Status, emailflags.DomainStatusInvalid)
		}
	}
}

func TestCheckDomainCachesResults(t *testing.T) {
	r := NewEmailDomainResolver(EmailDomainResolverConfig{})

	// Seed the cache directly so the test needs no network.
	seeded := EmailDomainResult{
		Domain:    "cached.example",
		Status:    emailflags.DomainStatusOK,
		Detail:    "seeded",
		CheckTime: time.Now(),
	}
	r.store("cached.example", seeded)

	got := r.CheckDomain(context.Background(), "Cached.Example.")
	if got.Detail != "seeded" {
		t.Errorf("expected the cached verdict, got %+v", got)
	}

	r.InvalidateCache("cached.example")
	if _, ok := r.cached("cached.example"); ok {
		t.Error("InvalidateCache did not drop the entry")
	}
}

func TestTransientResultsExpireSooner(t *testing.T) {
	r := NewEmailDomainResolver(EmailDomainResolverConfig{CacheTTL: 10 * time.Hour})

	r.store("stable.example", EmailDomainResult{Status: emailflags.DomainStatusOK})
	r.store("flaky.example", EmailDomainResult{Status: emailflags.DomainStatusError})

	r.mu.RLock()
	stable := r.cache["stable.example"].expiresAt
	flaky := r.cache["flaky.example"].expiresAt
	r.mu.RUnlock()

	if !flaky.Before(stable) {
		t.Errorf("a transient failure should be retried sooner than a settled verdict (flaky %v, stable %v)", flaky, stable)
	}
}

// TestCheckDomainLive exercises the ladder against real DNS. It is skipped by
// `go test -short`, which is what `make test-short` runs.
func TestCheckDomainLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live DNS test in short mode")
	}

	r := NewEmailDomainResolver(EmailDomainResolverConfig{Timeout: 8 * time.Second})
	ctx := context.Background()

	tests := []struct {
		name   string
		domain string
		want   string
	}{
		{
			// example.com publishes "0 ." exactly to say it takes no mail.
			name:   "null MX",
			domain: "example.com",
			want:   emailflags.DomainStatusNullMX,
		},
		{
			name:   "healthy domain with several MX hosts",
			domain: "gmail.com",
			want:   emailflags.DomainStatusOK,
		},
		{
			// .invalid is reserved by RFC 2606 and never resolves.
			name:   "domain that does not exist",
			domain: "nodelistdb-no-such-domain.invalid",
			want:   emailflags.DomainStatusNoDomain,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.CheckDomain(ctx, tt.domain)
			if got.Status == emailflags.DomainStatusError {
				t.Skipf("DNS unavailable: %s", got.Error)
			}
			if got.Status != tt.want {
				t.Errorf("CheckDomain(%q) = %s (%s), want %s", tt.domain, got.Status, got.Detail, tt.want)
			}
		})
	}
}

// TestCheckDomainPunycodes confirms an internationalised domain is converted
// before it is queried, rather than being reported as dead.
func TestCheckDomainPunycodes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live DNS test in short mode")
	}

	r := NewEmailDomainResolver(EmailDomainResolverConfig{Timeout: 8 * time.Second})
	got := r.CheckDomain(context.Background(), "пример.рф")

	if got.Status == emailflags.DomainStatusInvalid {
		t.Fatalf("an internationalised domain was rejected outright: %+v", got)
	}
	if got.ASCIIDomain != "xn--e1afmkfd.xn--p1ai" {
		t.Errorf("ASCIIDomain = %q, want the punycode form xn--e1afmkfd.xn--p1ai", got.ASCIIDomain)
	}
}

// TestDegradedDomainSurvivesADeadBackupMX pins the owlserver.de case: a domain
// whose primary MX resolves but whose backups sit on defunct dynamic-DNS
// providers is deliverable, and must be reported degraded rather than
// abandoned as unchecked at the first SERVFAIL.
func TestDegradedDomainSurvivesADeadBackupMX(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live DNS test in short mode")
	}

	r := NewEmailDomainResolver(EmailDomainResolverConfig{Timeout: 8 * time.Second})
	got := r.CheckDomain(context.Background(), "owlserver.de")

	if !emailflags.DomainStatusRoutable(got.Status) {
		t.Fatalf("owlserver.de reported unroutable (%s: %s); its preference-10 MX resolves",
			got.Status, got.Detail)
	}
	if got.Status != emailflags.DomainStatusDegraded {
		t.Errorf("status = %s, want %s (%s)", got.Status, emailflags.DomainStatusDegraded, got.Detail)
	}
	if got.Error != "" {
		t.Errorf("a deliverable domain must not carry an error that marks it stale: %q", got.Error)
	}

	var resolved int
	for _, h := range got.MXHosts {
		if h.Resolved {
			resolved++
		}
	}
	if resolved == 0 || resolved == len(got.MXHosts) {
		t.Errorf("expected some but not all MX hosts to resolve, got %d of %d", resolved, len(got.MXHosts))
	}
}
