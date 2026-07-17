package daemon

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
)

// AkaEquivalence is an in-memory index linking schedule entries across FTN
// networks that belong to one physical host. Two entries are linked when
// evidence shows they are the same system: identical hostname sets in the
// current nodelists, or one entry's test announced the other entry's address
// as an AKA (with overlapping hostnames).
//
// The index lets a test cycle collapse a group to a single representative so
// one direct test covers the whole host, with the other entries filled in via
// derived results.
type AkaEquivalence struct {
	mu    sync.RWMutex
	links map[string]map[string]struct{} // node key -> directly linked node keys
}

// NewAkaEquivalence creates an empty equivalence index.
func NewAkaEquivalence() *AkaEquivalence {
	return &AkaEquivalence{
		links: make(map[string]map[string]struct{}),
	}
}

// Link records that two domain-qualified node keys belong to one physical host.
func (e *AkaEquivalence) Link(a, b string) {
	if a == b || a == "" || b == "" {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.links[a] == nil {
		e.links[a] = make(map[string]struct{})
	}
	if e.links[b] == nil {
		e.links[b] = make(map[string]struct{})
	}
	e.links[a][b] = struct{}{}
	e.links[b][a] = struct{}{}
}

// Group returns the connected component containing key (including key itself
// when it has any links; nil when the key is unknown).
func (e *AkaEquivalence) Group(key string) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if len(e.links[key]) == 0 {
		return nil
	}

	seen := map[string]struct{}{key: {}}
	stack := []string{key}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for linked := range e.links[current] {
			if _, ok := seen[linked]; !ok {
				seen[linked] = struct{}{}
				stack = append(stack, linked)
			}
		}
	}

	group := make([]string, 0, len(seen))
	for k := range seen {
		group = append(group, k)
	}
	sort.Strings(group)
	return group
}

// Size returns the number of keys that participate in at least one link.
func (e *AkaEquivalence) Size() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.links)
}

// hostnameSet returns the node's hostnames lowercased as a set.
func hostnameSet(node *models.Node) map[string]struct{} {
	set := make(map[string]struct{}, len(node.InternetHostnames))
	for _, h := range node.InternetHostnames {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			set[h] = struct{}{}
		}
	}
	return set
}

// hostnamesOverlap reports whether two nodes share at least one hostname
// (case-insensitive). A dead per-network hostname must not look operational,
// so entries with disjoint hostname sets are never treated as one host.
func hostnamesOverlap(a, b *models.Node) bool {
	setA := hostnameSet(a)
	if len(setA) == 0 {
		return false
	}
	for _, h := range b.InternetHostnames {
		if _, ok := setA[strings.ToLower(strings.TrimSpace(h))]; ok {
			return true
		}
	}
	return false
}

// seedAkaEquivalence builds the initial equivalence index from two sources:
// identical hostname sets across networks in the current nodelists (covers a
// cold start with no history) and AKA lists announced during recent tests.
func (d *Daemon) seedAkaEquivalence(ctx context.Context) {
	if d.akaEquiv == nil || d.scheduler == nil {
		return
	}

	scheduled := d.scheduler.AllScheduledNodes()
	d.akaEquiv.SeedFromNodes(scheduled)

	// Link entries whose addresses were announced as AKAs in recent tests,
	// applying the hostname-overlap gate between the two schedule entries
	records, err := d.storage.GetRecentAnnouncedAKAs(ctx, 7)
	if err != nil {
		logging.Warnf("Failed to seed AKA equivalence from test history: %v", err)
	} else {
		byKey := make(map[string]*models.Node, len(scheduled))
		for _, n := range scheduled {
			byKey[n.Key()] = n
		}
		for _, rec := range records {
			tested := &models.Node{Zone: rec.Zone, Net: rec.Net, Node: rec.Node, Domain: rec.Domain}
			testedNode, ok := byKey[tested.Key()]
			if !ok {
				continue
			}
			for _, raw := range rec.Announced {
				aka, ok := parseAKA(raw)
				if !ok {
					continue
				}
				for _, candidate := range d.scheduler.SchedulesFor3D(aka.Zone, aka.Net, aka.Node) {
					if candidate.EffectiveDomain() == testedNode.EffectiveDomain() {
						continue
					}
					if aka.Domain != "" && aka.Domain != candidate.EffectiveDomain() {
						continue
					}
					if hostnamesOverlap(testedNode, candidate) {
						d.akaEquiv.Link(testedNode.Key(), candidate.Key())
					}
				}
			}
		}
	}

	if size := d.akaEquiv.Size(); size > 0 {
		logging.Infof("AKA equivalence index seeded: %d linked entries", size)
	}
}

// SeedFromNodes links entries in different networks that advertise identical
// hostname sets. This covers cold start, when no test history exists yet.
func (e *AkaEquivalence) SeedFromNodes(nodes []*models.Node) {
	bySet := make(map[string][]*models.Node)
	for _, node := range nodes {
		set := hostnameSet(node)
		if len(set) == 0 {
			continue
		}
		hosts := make([]string, 0, len(set))
		for h := range set {
			hosts = append(hosts, h)
		}
		sort.Strings(hosts)
		key := strings.Join(hosts, ",")
		bySet[key] = append(bySet[key], node)
	}

	for _, group := range bySet {
		if len(group) < 2 {
			continue
		}
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				if group[i].EffectiveDomain() != group[j].EffectiveDomain() {
					e.Link(group[i].Key(), group[j].Key())
				}
			}
		}
	}
}
