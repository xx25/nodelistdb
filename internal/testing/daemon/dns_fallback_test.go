package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/nodelistdb/internal/testing/models"
	"github.com/nodelistdb/internal/testing/protocols"
	"github.com/nodelistdb/internal/testing/storage"
)

func TestClassifyDNSError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{
			// The overwhelmingly common shape in production: a name that no
			// longer exists. 82 hostnames in the archive have looked like this
			// on every test for a year.
			"nxdomain via flag",
			&net.DNSError{Err: "no such host", Name: "bbs.example.org", IsNotFound: true},
			models.DNSErrorNXDomain,
		},
		{
			// The interesting one for a fallback address: the nameserver could
			// not be reached, which says nothing about whether the node is up.
			"timeout via flag",
			&net.DNSError{Err: "i/o timeout", Name: "f0.s0t.ru", IsTimeout: true},
			models.DNSErrorTimeout,
		},
		{"servfail from message", errors.New("lookup XStation on 127.0.0.53:53: server misbehaving"), models.DNSErrorServfail},
		{"nxdomain from message", errors.New("lookup gone.example on 127.0.0.53:53: no such host"), models.DNSErrorNXDomain},
		{"timeout from message", errors.New("lookup slow.example: i/o timeout"), models.DNSErrorTimeout},
		{"refused from message", errors.New("lookup x.example: connection refused"), models.DNSErrorRefused},
		{"unknown", errors.New("something else entirely"), models.DNSErrorOther},
		{
			// Errors reach us wrapped in places; the flags must still win over
			// the prose, since a wrapped timeout reads as "other" otherwise.
			"wrapped timeout",
			fmt.Errorf("resolving: %w", &net.DNSError{Err: "i/o timeout", IsTimeout: true}),
			models.DNSErrorTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyDNSError(tt.err); got != tt.want {
				t.Errorf("classifyDNSError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestLiteralIPsFor(t *testing.T) {
	// Exactly the shape 2:5030/0 publishes since 2026-08-28: a hostname and an
	// IP literal side by side in INA.
	node := &models.Node{
		Zone: 2, Net: 5030, Node: 0,
		InternetHostnames: []string{"f0.s0t.ru", "217.71.231.2", "[2001:db8::1]"},
	}

	v4, v6 := literalIPsFor(node)
	if len(v4) != 1 || v4[0] != "217.71.231.2" {
		t.Errorf("IPv4 literals = %v, want [217.71.231.2]", v4)
	}
	if len(v6) != 1 || v6[0] != "2001:db8::1" {
		t.Errorf("IPv6 literals = %v, want [2001:db8::1]", v6)
	}

	// A node with names only must yield nothing, or every DNS failure would
	// trigger a pointless probe.
	nameOnly := &models.Node{InternetHostnames: []string{"bbs.example.org"}}
	if v4, v6 := literalIPsFor(nameOnly); len(v4) != 0 || len(v6) != 0 {
		t.Errorf("hostname-only node yielded literals: %v %v", v4, v6)
	}
}

// fakeFallbackStorage records what was asked for and returns a fixed answer.
// The Storage interface is embedded rather than implemented: only
// GetLastKnownIPs is reachable from this code path, and a nil embedded
// interface makes any other call panic loudly instead of passing silently.
type fakeFallbackStorage struct {
	storage.Storage

	result     *storage.LastKnownIPs
	err        error
	gotAddress string
	gotHost    string
	gotMaxAge  time.Duration
	calls      int
}

func (f *fakeFallbackStorage) GetLastKnownIPs(_ context.Context, zone, net, node int, domain, hostname string, maxAge time.Duration) (*storage.LastKnownIPs, error) {
	f.calls++
	f.gotAddress = fmt.Sprintf("%d:%d/%d@%s", zone, net, node, domain)
	f.gotHost, f.gotMaxAge = hostname, maxAge
	return f.result, f.err
}

// newFallbackTestDaemon builds the smallest Daemon that buildFallbackProbe and
// runFallbackProbe need: config plus storage, no services and no testers.
func newFallbackTestDaemon(store storage.Storage, enabled bool, maxAge time.Duration) *Daemon {
	return &Daemon{
		config: &Config{
			Services: ServicesConfig{
				DNS: DNSConfig{
					FallbackProbe: DNSFallbackProbeConfig{
						Enabled: &enabled,
						MaxAge:  maxAge,
					},
				},
			},
		},
		storage: store,
	}
}

func TestBuildFallbackProbePrefersPublishedLiteral(t *testing.T) {
	store := &fakeFallbackStorage{
		result: &storage.LastKnownIPs{IPv4: []string{"10.0.0.9"}, ObservedAt: time.Now().Add(-time.Hour)},
	}
	d := newFallbackTestDaemon(store, true, 30*24*time.Hour)

	// A single entry, so nothing else in this cycle already dials the literal.
	node := &models.Node{
		Zone: 2, Net: 5030, Node: 0,
		InternetHostnames: []string{"217.71.231.2"},
	}

	probe := d.buildFallbackProbe(context.Background(), node, "f0.s0t.ru")
	if probe == nil {
		t.Fatal("expected a probe for a node publishing an IP literal")
	}
	if probe.Source != models.FallbackSourceNodelistLiteral {
		t.Errorf("source = %q, want %q", probe.Source, models.FallbackSourceNodelistLiteral)
	}
	if len(probe.IPv4) != 1 || probe.IPv4[0] != "217.71.231.2" {
		t.Errorf("probe IPv4 = %v, want the published literal", probe.IPv4)
	}
	if probe.AgeHours != 0 {
		t.Errorf("age = %d, want 0: a published address is asserted, not observed", probe.AgeHours)
	}
	if store.calls != 0 {
		t.Errorf("storage was queried %d times; a published literal needs no history lookup", store.calls)
	}
}

func TestBuildFallbackProbeFallsBackToLastKnown(t *testing.T) {
	observed := time.Now().Add(-50 * time.Hour)
	store := &fakeFallbackStorage{
		result: &storage.LastKnownIPs{IPv4: []string{"192.0.2.7"}, ObservedAt: observed},
	}
	d := newFallbackTestDaemon(store, true, 30*24*time.Hour)

	node := &models.Node{Zone: 1, Net: 123, Node: 755, InternetHostnames: []string{"bbs.example.org"}}

	probe := d.buildFallbackProbe(context.Background(), node, "bbs.example.org")
	if probe == nil {
		t.Fatal("expected a probe from the remembered address")
	}
	if probe.Source != models.FallbackSourceLastKnown {
		t.Errorf("source = %q, want %q", probe.Source, models.FallbackSourceLastKnown)
	}
	if probe.AgeHours != 50 {
		t.Errorf("age = %dh, want 50h: the age is the measurement, not a detail", probe.AgeHours)
	}
	// The lookup must carry the FTN network: zone numbers are reused across
	// networks, so 21:1/100@fsxnet and 21:1/100@fidonet are different nodes and
	// must not inherit each other's address history.
	if store.gotAddress != "1:123/755@fidonet" || store.gotHost != "bbs.example.org" {
		t.Errorf("looked up (%q, %q), want the tested node, its domain, and hostname", store.gotAddress, store.gotHost)
	}
}

func TestBuildFallbackProbeDisabledAndEmptyCases(t *testing.T) {
	node := &models.Node{Zone: 1, Net: 1, Node: 1, InternetHostnames: []string{"bbs.example.org"}}

	t.Run("disabled", func(t *testing.T) {
		store := &fakeFallbackStorage{result: &storage.LastKnownIPs{IPv4: []string{"192.0.2.7"}}}
		d := newFallbackTestDaemon(store, false, time.Hour)
		if probe := d.buildFallbackProbe(context.Background(), node, "bbs.example.org"); probe != nil {
			t.Error("probe built while disabled")
		}
		if store.calls != 0 {
			t.Error("storage queried while disabled")
		}
	})

	t.Run("no history", func(t *testing.T) {
		d := newFallbackTestDaemon(&fakeFallbackStorage{result: nil}, true, time.Hour)
		if probe := d.buildFallbackProbe(context.Background(), node, "bbs.example.org"); probe != nil {
			t.Error("probe built with no remembered address")
		}
	})

	t.Run("lookup error is not fatal", func(t *testing.T) {
		d := newFallbackTestDaemon(&fakeFallbackStorage{err: errors.New("clickhouse down")}, true, time.Hour)
		if probe := d.buildFallbackProbe(context.Background(), node, "bbs.example.org"); probe != nil {
			t.Error("probe built despite lookup failure")
		}
	})
}

// TestRunFallbackProbeLeavesResolutionFieldsEmpty pins the property that makes
// the whole feature safe to add: a row where DNS failed must still look like a
// DNS failure to every existing query. If the probe left its addresses in
// resolved_ipv4, reachability and geo analytics would silently start counting
// nodes that never resolved.
func TestRunFallbackProbeLeavesResolutionFieldsEmpty(t *testing.T) {
	d := newFallbackTestDaemon(&fakeFallbackStorage{}, true, time.Hour)
	d.binkpTester = &stubTester{name: "binkp", success: false}

	node := &models.Node{
		Zone: 2, Net: 5030, Node: 0,
		InternetHostnames: []string{"f0.s0t.ru"},
		InternetProtocols: []string{"IBN"},
	}
	result := &models.TestResult{
		DNSError:     "lookup f0.s0t.ru: i/o timeout",
		DNSErrorKind: models.DNSErrorTimeout,
	}
	probe := &models.DNSFallbackProbe{
		Source: models.FallbackSourceLastKnown,
		IPv4:   []string{"217.71.231.2"},
	}

	d.runFallbackProbe(context.Background(), node, result, probe)

	if len(result.ResolvedIPv4) != 0 || len(result.ResolvedIPv6) != 0 {
		t.Errorf("probe leaked addresses into resolved fields: %v %v", result.ResolvedIPv4, result.ResolvedIPv6)
	}
	if result.IsOperational {
		t.Error("fallback probe marked the node operational; it is unreachable to a DNS-only mailer")
	}
	if result.DNSFallback == nil {
		t.Fatal("probe outcome not recorded")
	}
	if result.DNSFallback.Success {
		t.Error("failed probe recorded as a success")
	}
	if result.DNSError == "" {
		t.Error("DNSError was cleared; the row must still read as a DNS failure")
	}
}

// TestRunFallbackProbeRecordsNothingWhenNothingDialled guards the meaning of
// dns_fallback_attempted. A node that advertises neither IBN nor IFC has no
// probeable protocol, so no packet is sent; writing a row that says "attempted,
// did not succeed" would make every future count of failed fallbacks wrong.
func TestRunFallbackProbeRecordsNothingWhenNothingDialled(t *testing.T) {
	d := newFallbackTestDaemon(&fakeFallbackStorage{}, true, time.Hour)
	d.binkpTester = &stubTester{name: "binkp", success: true}

	// Telnet only: nothing the probe is willing to dial.
	node := &models.Node{
		Zone: 1, Net: 1, Node: 1,
		InternetHostnames: []string{"bbs.example.org"},
		InternetProtocols: []string{"ITN"},
	}
	result := &models.TestResult{DNSError: "lookup bbs.example.org: no such host"}

	d.runFallbackProbe(context.Background(), node, result,
		&models.DNSFallbackProbe{Source: models.FallbackSourceLastKnown, IPv4: []string{"192.0.2.7"}})

	if result.DNSFallback != nil {
		t.Error("recorded a probe attempt when no protocol was dialled")
	}
}

// stubTester answers every probe with a fixed verdict.
type stubTester struct {
	name         string
	success      bool
	addressValid bool
	gotHosts     []string
}

func (s *stubTester) GetProtocolName() string { return s.name }

func (s *stubTester) Test(_ context.Context, host string, _ int, _ string) protocols.TestResult {
	s.gotHosts = append(s.gotHosts, host)
	return &protocols.BinkPTestResult{
		BaseTestResult: protocols.BaseTestResult{Success: s.success},
		SystemName:     "Test System",
		AddressValid:   s.addressValid,
	}
}

// TestRunFallbackProbeSuccessDoesNotPromoteNode is the important one.
//
// The protocol testers write IsOperational straight into whatever result they
// are handed, so a probe that shares the node's result would report a node as
// operational on the strength of an address DNS no longer confirms. To a mailer
// that only has the hostname, such a node is unreachable, and every existing
// reachability figure depends on it being counted that way.
func TestRunFallbackProbeSuccessDoesNotPromoteNode(t *testing.T) {
	d := newFallbackTestDaemon(&fakeFallbackStorage{}, true, time.Hour)
	tester := &stubTester{name: "binkp", success: true, addressValid: true}
	d.binkpTester = tester

	node := &models.Node{
		Zone: 2, Net: 5030, Node: 0,
		InternetHostnames: []string{"f0.s0t.ru", "217.71.231.2"},
		InternetProtocols: []string{"IBN"},
	}
	result := &models.TestResult{
		Address:      "2:5030/0",
		DNSError:     "lookup f0.s0t.ru: i/o timeout",
		DNSErrorKind: models.DNSErrorTimeout,
	}

	d.runFallbackProbe(context.Background(), node, result,
		&models.DNSFallbackProbe{Source: models.FallbackSourceNodelistLiteral, IPv4: []string{"217.71.231.2"}})

	if len(tester.gotHosts) != 1 || tester.gotHosts[0] != "217.71.231.2" {
		t.Errorf("probed %v, want the published literal once", tester.gotHosts)
	}

	fb := result.DNSFallback
	if fb == nil || !fb.Success {
		t.Fatal("successful probe not recorded")
	}
	if len(fb.Protocols) != 1 || fb.Protocols[0] != "binkp" {
		t.Errorf("probe protocols = %v, want [binkp]", fb.Protocols)
	}
	if !fb.AddressValidated {
		t.Error("address validation not carried across; a recycled IP must be distinguishable from the real node")
	}

	// The node's own record must be untouched by all of that.
	if result.IsOperational {
		t.Error("node marked operational by a fallback probe")
	}
	if result.AddressValidated {
		t.Error("node's AddressValidated set by a fallback probe")
	}
	if result.BinkPResult != nil {
		t.Error("probe's protocol result leaked into the node's own BinkP result")
	}
	if len(result.ResolvedIPv4) != 0 {
		t.Errorf("probe addresses leaked into resolved_ipv4: %v", result.ResolvedIPv4)
	}
	if result.DNSError == "" {
		t.Error("row no longer reads as a DNS failure")
	}
}

// TestBuildFallbackProbeScopesLookupToDomain pins that a node's address history
// is fetched per FTN network. Zones are reused between networks, so an fsxnet
// node must never be probed at the address a same-numbered fidonet node used.
func TestBuildFallbackProbeScopesLookupToDomain(t *testing.T) {
	store := &fakeFallbackStorage{
		result: &storage.LastKnownIPs{IPv4: []string{"192.0.2.7"}, ObservedAt: time.Now()},
	}
	d := newFallbackTestDaemon(store, true, time.Hour)

	node := &models.Node{
		Zone: 21, Net: 1, Node: 100, Domain: "fsxnet",
		InternetHostnames: []string{"bbs.example.org"},
	}

	if probe := d.buildFallbackProbe(context.Background(), node, "bbs.example.org"); probe == nil {
		t.Fatal("expected a probe")
	}
	if store.gotAddress != "21:1/100@fsxnet" {
		t.Errorf("looked up %q, want the fsxnet identity", store.gotAddress)
	}
}

// TestBuildFallbackProbeSkipsSeparatelyTestedLiteral covers the shape 2:5030/0
// publishes: a hostname and an IP literal side by side. The daemon tests every
// hostname entry, and Go's resolver short-circuits IP literals, so that literal
// already gets its own handshake this cycle. Probing it again on the name's DNS
// failure would send a second handshake to the same host for nothing.
func TestBuildFallbackProbeSkipsSeparatelyTestedLiteral(t *testing.T) {
	node := &models.Node{
		Zone: 2, Net: 5030, Node: 0,
		InternetHostnames: []string{"f0.s0t.ru", "217.71.231.2"},
	}

	t.Run("literal source not used", func(t *testing.T) {
		d := newFallbackTestDaemon(&fakeFallbackStorage{}, true, time.Hour)
		if probe := d.buildFallbackProbe(context.Background(), node, "f0.s0t.ru"); probe != nil {
			t.Errorf("probe built for a literal already tested on its own: %+v", probe)
		}
	})

	t.Run("remembered address equal to the literal is skipped too", func(t *testing.T) {
		store := &fakeFallbackStorage{
			result: &storage.LastKnownIPs{IPv4: []string{"217.71.231.2"}, ObservedAt: time.Now()},
		}
		d := newFallbackTestDaemon(store, true, time.Hour)
		if probe := d.buildFallbackProbe(context.Background(), node, "f0.s0t.ru"); probe != nil {
			t.Errorf("probe built for an address covered by a separately tested literal: %+v", probe)
		}
	})

	t.Run("a different remembered address is still probed", func(t *testing.T) {
		store := &fakeFallbackStorage{
			result: &storage.LastKnownIPs{IPv4: []string{"198.51.100.4"}, ObservedAt: time.Now()},
		}
		d := newFallbackTestDaemon(store, true, time.Hour)
		probe := d.buildFallbackProbe(context.Background(), node, "f0.s0t.ru")
		if probe == nil {
			t.Fatal("an address no other entry covers must still be probed")
		}
		if probe.Source != models.FallbackSourceLastKnown {
			t.Errorf("source = %q", probe.Source)
		}
	})
}
