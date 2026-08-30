package daemon

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/testing/models"
)

// classifyDNSError maps a resolver error onto a stable vocabulary.
//
// The Go resolver reports NXDOMAIN and NODATA through the same IsNotFound flag
// and otherwise hands back prose ("lookup example.org on 127.0.0.53:53: no such
// host"), so every past analysis of this data had to regex those strings. The
// distinction matters here: an NXDOMAIN usually means the name is gone, while a
// timeout means the nameserver could not be reached and the host behind it may
// be perfectly fine - which is the case a fallback address is meant to cover.
func classifyDNSError(err error) string {
	if err == nil {
		return ""
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		switch {
		case dnsErr.IsTimeout:
			return models.DNSErrorTimeout
		case dnsErr.IsNotFound:
			return models.DNSErrorNXDomain
		}
	}

	// Fall back to the message for resolver errors that are not net.DNSError,
	// and for the server-side conditions Go does not model as flags.
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "no such host"):
		return models.DNSErrorNXDomain
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "i/o timeout"):
		return models.DNSErrorTimeout
	case strings.Contains(msg, "server misbehaving"), strings.Contains(msg, "servfail"):
		return models.DNSErrorServfail
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "refused"):
		return models.DNSErrorRefused
	case strings.Contains(msg, "no answer"), strings.Contains(msg, "not found"):
		return models.DNSErrorNoAnswer
	default:
		return models.DNSErrorOther
	}
}

// literalIPsFor returns the IP literals a node publishes in INA, split by family.
//
// These are the addresses a sysop chose to put in the nodelist alongside a
// hostname - the mechanism under discussion in FIDONEWS/echo threads about
// nodelist static IPs. When the hostname fails, they are the first thing to try,
// and unlike a remembered address they carry the sysop's explicit claim that the
// address is stable.
func literalIPsFor(node *models.Node) (v4 []string, v6 []string) {
	for _, h := range node.InternetHostnames {
		ip := net.ParseIP(strings.Trim(h, "[]"))
		if ip == nil {
			continue
		}
		if ip.To4() != nil {
			v4 = append(v4, ip.String())
		} else {
			v6 = append(v6, ip.String())
		}
	}
	return v4, v6
}

// subsetOf reports whether every address in got also appears in want. An empty
// got is a subset of anything, which is what makes the IPv4-only and IPv6-only
// cases fall out correctly.
func subsetOf(got, want []string) bool {
	for _, g := range got {
		found := false
		for _, w := range want {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// buildFallbackProbe decides what address to probe after a DNS failure.
//
// Published literals win over a remembered address: the sysop asserted those,
// they cannot silently age past the point of being wrong the way a cached
// observation can, and measuring them is the point of the exercise. Only if the
// node publishes none do we fall back to what the hostname last resolved to,
// which approximates what a mailer with a warm DNS cache would still hold.
//
// Returns nil when there is nothing worth probing.
func (d *Daemon) buildFallbackProbe(ctx context.Context, node *models.Node, hostname string) *models.DNSFallbackProbe {
	cfg := d.config.Services.DNS.FallbackProbe
	if !cfg.IsEnabled() {
		return nil
	}

	litV4, litV6 := literalIPsFor(node)

	// A node that publishes an IP literal alongside a hostname has that literal
	// tested as its own entry in the same cycle (testMultipleHostnameNode walks
	// every entry, and Go's resolver short-circuits IP literals), so probing it
	// again here would send a second handshake to the same host for no new
	// information. The measurement is not lost: the literal's own row and the
	// failing hostname's row share a test cycle and pair up in analysis.
	litTestedSeparately := len(node.InternetHostnames) > 1

	if !litTestedSeparately && (len(litV4) > 0 || len(litV6) > 0) {
		return &models.DNSFallbackProbe{
			Source: models.FallbackSourceNodelistLiteral,
			IPv4:   litV4,
			IPv6:   litV6,
		}
	}

	if d.storage == nil || hostname == "" {
		return nil
	}

	last, err := d.storage.GetLastKnownIPs(ctx, node.Zone, node.Net, node.Node, node.EffectiveDomain(), hostname, cfg.MaxAge)
	if err != nil {
		logging.Debugf("[%s] last-known IP lookup failed for %s: %v", node.Address(), hostname, err)
		return nil
	}
	if last == nil || (len(last.IPv4) == 0 && len(last.IPv6) == 0) {
		return nil
	}

	// Same duplicate-handshake guard: a remembered address that is also a
	// published literal is already being dialled by that literal's own entry.
	if litTestedSeparately && subsetOf(last.IPv4, litV4) && subsetOf(last.IPv6, litV6) {
		return nil
	}

	age := time.Since(last.ObservedAt)
	if age < 0 {
		age = 0
	}

	return &models.DNSFallbackProbe{
		Source:   models.FallbackSourceLastKnown,
		IPv4:     last.IPv4,
		IPv6:     last.IPv6,
		AgeHours: uint32(age / time.Hour),
	}
}

// runFallbackProbe tests a node at its fallback address after DNS failed and
// records the outcome on result.DNSFallback.
//
// The probe runs against a scratch TestResult rather than the node's own. The
// protocol testers take their addresses from a result and write their verdict
// back into it - including IsOperational - so sharing the caller's result would
// silently promote a node to "operational" on the strength of an address DNS no
// longer confirms, and would leave probe addresses sitting in resolved_ipv4.
// Every existing reachability query reads those fields; none of them may change
// meaning because this probe was added.
func (d *Daemon) runFallbackProbe(ctx context.Context, node *models.Node, result *models.TestResult, probe *models.DNSFallbackProbe) {
	if probe == nil {
		return
	}

	scratch := &models.TestResult{
		Zone:         result.Zone,
		Net:          result.Net,
		Node:         result.Node,
		Address:      result.Address,
		Domain:       result.Domain,
		ResolvedIPv4: probe.IPv4,
		ResolvedIPv6: probe.IPv6,
	}

	logging.Debugf("[%s] DNS failed for %s; probing %s fallback %v%v",
		node.Address(), result.TestedHostname, probe.Source, probe.IPv4, probe.IPv6)

	// Only the mailer protocols are probed. Telnet, FTP and VModem answer from
	// plenty of hosts that are not FidoNet nodes, so they cannot tell us whether
	// this node is still there - whereas a BinkP or EMSI handshake announces an
	// address we can check, which is the entire value of the measurement.
	dialed := false
	if node.HasProtocol("IBN") && d.binkpTester != nil {
		d.testBinkP(ctx, node, scratch)
		dialed = true
	}
	if node.HasProtocol("IFC") && d.ifcicoTester != nil {
		d.testIfcico(ctx, node, scratch)
		dialed = true
	}

	// Nothing was actually contacted - the node advertises neither IBN nor IFC,
	// or those testers are disabled. Recording a probe here would write
	// dns_fallback_attempted = true with dns_fallback_success = false, which
	// reads as "we tried the address and nothing answered" when in truth no
	// packet was ever sent. That distinction is the entire point of the column.
	if !dialed {
		return
	}

	if r := scratch.BinkPResult; r != nil {
		if r.Success {
			probe.Success = true
			probe.Protocols = append(probe.Protocols, "binkp")
		} else if r.Error != "" {
			probe.Error = r.Error
		}
	}
	if r := scratch.IfcicoResult; r != nil {
		if r.Success {
			probe.Success = true
			probe.Protocols = append(probe.Protocols, "ifcico")
		} else if probe.Error == "" && r.Error != "" {
			probe.Error = r.Error
		}
	}
	probe.AddressValidated = scratch.AddressValidated

	result.DNSFallback = probe

	if probe.Success {
		logging.Infof("[%s] DNS failed but node answered at %s address %v%v (age %dh, protocols %v, address validated: %t)",
			node.Address(), probe.Source, probe.IPv4, probe.IPv6, probe.AgeHours, probe.Protocols, probe.AddressValidated)
	}
}
