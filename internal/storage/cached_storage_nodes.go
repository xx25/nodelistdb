package storage

import (
	"fmt"
	"time"

	"github.com/nodelistdb/internal/database"
)

// Node-related caching operations

// GetNodes with caching
func (cs *CachedStorage) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	return cachedFetch(cs, cs.keyGen.SearchKey(filter), cs.config.SearchTTL, func() ([]database.Node, error) {
		return cs.Storage.NodeOps().GetNodes(filter)
	})
}

// GetNodeHistory with caching
func (cs *CachedStorage) GetNodeHistory(zone, net, node int, domain string) ([]database.Node, error) {
	return cachedFetch(cs, cs.keyGen.NodeHistoryKey(zone, net, node)+":"+domain, cs.config.NodeTTL, func() ([]database.Node, error) {
		return cs.Storage.NodeOps().GetNodeHistory(zone, net, node, domain)
	})
}

// GetNodeDomains with caching. The answer changes only when a nodelist import
// adds the address to another network, so it rides the node TTL.
func (cs *CachedStorage) GetNodeDomains(zone, net, node int) ([]string, error) {
	key := fmt.Sprintf("%s:node:domains:%d:%d:%d", cs.keyGen.Prefix, zone, net, node)
	return cachedFetch(cs, key, cs.config.NodeTTL, func() ([]string, error) {
		return cs.Storage.NodeOps().GetNodeDomains(zone, net, node)
	})
}

// GetNodeChanges with caching
func (cs *CachedStorage) GetNodeChanges(zone, net, node int, domain string) ([]database.NodeChange, error) {
	return cachedFetch(cs, cs.keyGen.NodeChangesKey(zone, net, node, domain), cs.config.NodeTTL, func() ([]database.NodeChange, error) {
		return cs.Storage.SearchOps().GetNodeChanges(zone, net, node, domain)
	})
}

// GetUniqueSysops with caching
func (cs *CachedStorage) GetUniqueSysops(nameFilter string, limit, offset int) ([]SysopInfo, error) {
	return cachedFetch(cs, cs.keyGen.UniqueSysopsKey(nameFilter, limit, offset), cs.config.SearchTTL, func() ([]SysopInfo, error) {
		return cs.Storage.SearchOps().GetUniqueSysops(nameFilter, limit, offset)
	})
}

// GetNodesBySysop with caching
func (cs *CachedStorage) GetNodesBySysop(sysopName string, limit int) ([]database.Node, error) {
	return cachedFetch(cs, cs.keyGen.NodesBySysopKey(sysopName, limit), cs.config.SearchTTL, func() ([]database.Node, error) {
		return cs.Storage.SearchOps().GetNodesBySysop(sysopName, limit)
	})
}

// Pass-through methods (not cached)

// GetNodeDateRange returns the first and last date a node appears in nodelists
func (cs *CachedStorage) GetNodeDateRange(zone, net, node int, domain string) (firstDate, lastDate time.Time, err error) {
	// Not cached as this is rarely called
	return cs.Storage.NodeOps().GetNodeDateRange(zone, net, node, domain)
}

// SearchNodesBySysop searches for nodes by sysop name
func (cs *CachedStorage) SearchNodesBySysop(sysopName string, limit int, domain string) ([]NodeSummary, error) {
	// Not cached as this overlaps with GetNodesBySysop
	return cs.Storage.SearchOps().SearchNodesBySysop(sysopName, limit, domain)
}

// SearchNodesWithLifetime searches for nodes with lifetime information
func (cs *CachedStorage) SearchNodesWithLifetime(filter database.NodeFilter) ([]NodeSummary, error) {
	// Not cached as this is similar to GetNodes but with extra processing
	return cs.Storage.SearchOps().SearchNodesWithLifetime(filter)
}

// InsertNodes inserts a batch of nodes into the database
func (cs *CachedStorage) InsertNodes(nodes []database.Node) error {
	// Invalidate relevant caches after insertion
	err := cs.Storage.NodeOps().InsertNodes(nodes)
	if err == nil && len(nodes) > 0 {
		// Get the nodelist date from the first node
		nodelistDate := nodes[0].NodelistDate
		// Invalidate caches affected by the new data
		_ = cs.InvalidateAfterImport(nodelistDate, false)
	}
	return err
}
