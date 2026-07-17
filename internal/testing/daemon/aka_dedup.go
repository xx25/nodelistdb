package daemon

import (
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
)

// A physical host often holds AKAs in several FTN networks and announces the
// full list during BinkP (M_ADR) and IFCICO (EMSI_DAT) handshakes. One
// successful direct test therefore also proves reachability of the same host's
// entries in other networks — as long as those entries point at the same
// hostnames. This file turns a direct test into "derived" results for such
// entries so they get visible history without being retested.

// akaPattern matches announced AKAs: zone:net/node[.point][@domain]
var akaPattern = regexp.MustCompile(`^(\d+):(\d+)/(\d+)(?:\.(\d+))?(?:@([A-Za-z0-9_.-]+))?$`)

// cycleCoverage tracks which candidate identities already received a derived
// result within one test cycle. Two same-domain siblings of one physical host
// can both be direct-tested in the same cycle and both announce the same
// cross-domain AKA; without this guard each would store its own derived row.
type cycleCoverage struct {
	mu      sync.Mutex
	covered map[string]struct{}
}

func newCycleCoverage() *cycleCoverage {
	return &cycleCoverage{covered: make(map[string]struct{})}
}

// claim marks a candidate key as covered; it returns false when the key was
// already claimed earlier in the cycle.
func (c *cycleCoverage) claim(key string) bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, done := c.covered[key]; done {
		return false
	}
	c.covered[key] = struct{}{}
	return true
}

// parsedAKA is one announced address in canonical form.
type parsedAKA struct {
	Zone, Net, Node int
	Domain          string // lowercase; empty when the AKA carried no @domain
}

// parseAKA parses an announced AKA string. Point addresses are rejected:
// derivation applies to node entries only.
func parseAKA(raw string) (parsedAKA, bool) {
	matches := akaPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if matches == nil {
		return parsedAKA{}, false
	}
	if matches[4] != "" && matches[4] != "0" {
		return parsedAKA{}, false // skip points
	}
	zone, _ := strconv.Atoi(matches[1])
	net, _ := strconv.Atoi(matches[2])
	node, _ := strconv.Atoi(matches[3])
	return parsedAKA{
		Zone:   zone,
		Net:    net,
		Node:   node,
		Domain: strings.ToLower(matches[5]),
	}, true
}

// protocolAddressLists extracts the announced address lists from a protocol
// result: the union plus the per-IP-version lists (used to recompute the
// per-version validation flags for a derived identity).
func protocolAddressLists(pr *models.ProtocolTestResult, ipv4Extract, ipv6Extract func(any) ([]string, bool)) (all, ipv4, ipv6 []string) {
	if pr == nil || pr.Details == nil {
		return nil, nil, nil
	}
	if addrs, ok := ipv4Extract(pr.Details["ipv4"]); ok {
		ipv4 = append(ipv4, addrs...)
		all = append(all, addrs...)
	}
	if addrs, ok := ipv6Extract(pr.Details["ipv6"]); ok {
		ipv6 = append(ipv6, addrs...)
		all = append(all, addrs...)
	}
	// Flat fallback (single-version tests store addresses directly)
	if addrs, ok := pr.Details["addresses"].([]string); ok {
		all = append(all, addrs...)
	}
	return all, ipv4, ipv6
}

func binkpAddrs(v any) ([]string, bool) {
	if details, ok := v.(*models.BinkPTestDetails); ok {
		return details.Addresses, true
	}
	return nil, false
}

func ifcicoAddrs(v any) ([]string, bool) {
	if details, ok := v.(*models.IfcicoTestDetails); ok {
		return details.Addresses, true
	}
	return nil, false
}

// announcedAKAs collects the announced address lists across all per-hostname
// results plus the aggregated one. Aggregation keeps only the first successful
// protocol result per protocol, so later hostnames' announcements would be
// lost without the per-hostname union.
func announcedAKAs(results ...*models.TestResult) (all, ipv4, ipv6 []string) {
	seen := make(map[string]struct{})

	for _, r := range results {
		if r == nil {
			continue
		}
		for _, pr := range []struct {
			result  *models.ProtocolTestResult
			extract func(any) ([]string, bool)
		}{
			{r.BinkPResult, binkpAddrs},
			{r.IfcicoResult, ifcicoAddrs},
		} {
			a, v4, v6 := protocolAddressLists(pr.result, pr.extract, pr.extract)
			for _, addr := range a {
				if _, ok := seen["a:"+addr]; !ok {
					seen["a:"+addr] = struct{}{}
					all = append(all, addr)
				}
			}
			for _, addr := range v4 {
				if _, ok := seen["4:"+addr]; !ok {
					seen["4:"+addr] = struct{}{}
					ipv4 = append(ipv4, addr)
				}
			}
			for _, addr := range v6 {
				if _, ok := seen["6:"+addr]; !ok {
					seen["6:"+addr] = struct{}{}
					ipv6 = append(ipv6, addr)
				}
			}
		}
	}
	return all, ipv4, ipv6
}

// akaMatchesIdentity reports whether an announced AKA names the given node
// identity: the 3D address matches and, when the AKA carries an @domain,
// the domain matches too (fidomail's matching rule).
func akaMatchesIdentity(raw string, zone, net, node int, domain string) bool {
	aka, ok := parseAKA(raw)
	if !ok {
		return false
	}
	if aka.Zone != zone || aka.Net != net || aka.Node != node {
		return false
	}
	return aka.Domain == "" || aka.Domain == domain
}

// anyAKAMatches applies akaMatchesIdentity across a list.
func anyAKAMatches(addrs []string, zone, net, node int, domain string) bool {
	for _, a := range addrs {
		if akaMatchesIdentity(a, zone, net, node, domain) {
			return true
		}
	}
	return false
}

// deriveAKAResults turns one successful direct test into derived results for
// the same host's schedule entries in other FTN networks. Rules:
//   - only announced AKAs are considered; the derived entry's hostname set
//     must overlap the tested entry's (a per-network hostname that differs
//     gets tested on its own schedule instead);
//   - derived results carry DerivedFromAddress and never trigger further
//     derivation (no chains);
//   - address validation flags are recomputed for the derived identity.
//
// The returned results are ready for the normal batch storage path. cycle may
// be nil; when set, each candidate identity is derived at most once per cycle.
func (d *Daemon) deriveAKAResults(node *models.Node, aggregated *models.TestResult, partials []*models.TestResult, cycle *cycleCoverage) []*models.TestResult {
	if aggregated == nil || !aggregated.IsOperational || d.scheduler == nil {
		return nil
	}
	if aggregated.DerivedFromAddress != "" {
		return nil // never chain derivations
	}

	results := append([]*models.TestResult{aggregated}, partials...)
	all, ipv4, ipv6 := announcedAKAs(results...)
	if len(all) == 0 {
		return nil
	}

	testedDomain := node.EffectiveDomain()
	covered := make(map[string]struct{}) // candidate keys already derived
	var derived []*models.TestResult

	for _, raw := range all {
		aka, ok := parseAKA(raw)
		if !ok {
			continue
		}
		// The tested identity itself needs no derivation
		if aka.Zone == node.Zone && aka.Net == node.Net && aka.Node == node.Node &&
			(aka.Domain == "" || aka.Domain == testedDomain) {
			continue
		}

		for _, candidate := range d.scheduler.SchedulesFor3D(aka.Zone, aka.Net, aka.Node) {
			candDomain := candidate.EffectiveDomain()
			if candDomain == testedDomain {
				continue // only other networks' entries are derived
			}
			if aka.Domain != "" && aka.Domain != candDomain {
				continue // AKA names a specific network
			}
			if _, done := covered[candidate.Key()]; done {
				continue
			}
			if !hostnamesOverlap(node, candidate) {
				continue // different hostnames must be tested independently
			}
			if !cycle.claim(candidate.Key()) {
				continue // already covered by another direct test this cycle
			}

			result := aggregated.Clone()
			result.Zone = candidate.Zone
			result.Net = candidate.Net
			result.Node = candidate.Node
			result.Address = candidate.Address()
			result.Domain = candDomain
			result.DerivedFromAddress = node.Address() + "@" + testedDomain
			result.AddressValidated = anyAKAMatches(all, candidate.Zone, candidate.Net, candidate.Node, candDomain)
			result.AddressValidatedIPv4 = anyAKAMatches(ipv4, candidate.Zone, candidate.Net, candidate.Node, candDomain)
			result.AddressValidatedIPv6 = anyAKAMatches(ipv6, candidate.Zone, candidate.Net, candidate.Node, candDomain)

			covered[candidate.Key()] = struct{}{}
			derived = append(derived, result)

			// The candidate is now covered: push its next direct test out one
			// interval and remember the equivalence for future cycles
			d.scheduler.MarkDerivedResult(candidate, result)
			if d.akaEquiv != nil {
				d.akaEquiv.Link(node.Key(), candidate.Key())
			}

			logging.Infof("AKA-derived result for %s@%s from direct test of %s@%s",
				candidate.Address(), candDomain, node.Address(), testedDomain)
		}
	}

	return derived
}

// collapseAKAGroups reduces a batch of due nodes so at most one member of each
// AKA equivalence group is direct-tested per cycle. Without this, on cold
// start two linked entries are both due and both get direct-tested before
// either derivation runs. The stalest member is kept; deferred members are
// covered by derivation or picked up next cycle.
func (d *Daemon) collapseAKAGroups(nodes []*models.Node) []*models.Node {
	if d.akaEquiv == nil || d.akaEquiv.Size() == 0 || len(nodes) < 2 {
		return nodes
	}

	byKey := make(map[string]*models.Node, len(nodes))
	for _, n := range nodes {
		byKey[n.Key()] = n
	}

	deferred := make(map[string]struct{})
	kept := make(map[string]struct{})

	for _, n := range nodes {
		key := n.Key()
		if _, isDeferred := deferred[key]; isDeferred {
			continue
		}
		if _, isKept := kept[key]; isKept {
			continue
		}

		group := d.akaEquiv.Group(key)
		if len(group) < 2 {
			kept[key] = struct{}{}
			continue
		}

		// Collect the group's members that are due in this batch and pick the
		// stalest one as representative
		var due []*models.Node
		for _, member := range group {
			if n, isDue := byKey[member]; isDue {
				due = append(due, n)
			}
		}

		representative := byKey[key]
		repLast, _ := d.scheduler.LastTestTime(representative.Key())
		for _, member := range due {
			memberLast, ok := d.scheduler.LastTestTime(member.Key())
			if ok && memberLast.Before(repLast) {
				representative = member
				repLast = memberLast
			}
		}

		// Defer only members in OTHER domains: those are the ones a derived
		// result can cover. Same-domain members (one host, several entries in
		// one network) still need their own direct tests.
		for _, member := range due {
			if member.Key() == representative.Key() {
				continue
			}
			if member.EffectiveDomain() != representative.EffectiveDomain() {
				deferred[member.Key()] = struct{}{}
			} else {
				kept[member.Key()] = struct{}{}
			}
		}
		kept[representative.Key()] = struct{}{}
	}

	if len(deferred) == 0 {
		return nodes
	}

	result := make([]*models.Node, 0, len(nodes)-len(deferred))
	for _, n := range nodes {
		if _, skip := deferred[n.Key()]; skip {
			continue
		}
		result = append(result, n)
	}
	logging.Infof("AKA dedup: deferred %d of %d due nodes covered by equivalent entries", len(deferred), len(nodes))
	return result
}
