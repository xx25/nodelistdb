package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/database"
)

// Node-related caching operations

// GetNodes with caching, except for result sets above max_search_results.
//
// That bypass is an operator-set ceiling on what may occupy the cache: the
// search endpoints are unauthenticated and their own limit caps at 500, so
// without it a handful of wide searches can fill the store with entries
// nothing will ask for again. SearchPoints and SearchPointsWithLifetime carry
// the same guard.
func (cs *CachedStorage) GetNodes(ctx context.Context, filter database.NodeFilter) ([]database.Node, error) {
	if filter.Limit > cs.config.MaxSearchResults {
		return cs.Storage.NodeOps().GetNodes(ctx, filter)
	}
	return cachedFetch(cs, cs.keyGen.SearchKey(filter), cs.config.SearchTTL, func() ([]database.Node, error) {
		return cs.Storage.NodeOps().GetNodes(ctx, filter)
	})
}

// GetNodeHistory with caching
func (cs *CachedStorage) GetNodeHistory(ctx context.Context, zone, net, node int, domain string) ([]database.Node, error) {
	return cachedFetch(cs, cs.keyGen.NodeHistoryKey(zone, net, node)+":"+domain, cs.config.NodeTTL, func() ([]database.Node, error) {
		return cs.Storage.NodeOps().GetNodeHistory(ctx, zone, net, node, domain)
	})
}

// GetNodeDomains with caching. The answer changes only when a nodelist import
// adds the address to another network, so it rides the node TTL.
func (cs *CachedStorage) GetNodeDomains(ctx context.Context, zone, net, node int) ([]string, error) {
	key := fmt.Sprintf("%s:node:domains:%d:%d:%d", cs.keyGen.Prefix, zone, net, node)
	return cachedFetch(cs, key, cs.config.NodeTTL, func() ([]string, error) {
		return cs.Storage.NodeOps().GetNodeDomains(ctx, zone, net, node)
	})
}

// GetNodeChanges with caching
func (cs *CachedStorage) GetNodeChanges(ctx context.Context, zone, net, node int, domain string) ([]database.NodeChange, error) {
	return cachedFetch(cs, cs.keyGen.NodeChangesKey(zone, net, node, domain), cs.config.NodeTTL, func() ([]database.NodeChange, error) {
		return cs.Storage.SearchOps().GetNodeChanges(ctx, zone, net, node, domain)
	})
}

// GetUniqueSysops with caching
func (cs *CachedStorage) GetUniqueSysops(ctx context.Context, nameFilter string, limit, offset int) ([]SysopInfo, error) {
	return cachedFetch(cs, cs.keyGen.UniqueSysopsKey(nameFilter, limit, offset), cs.config.SearchTTL, func() ([]SysopInfo, error) {
		return cs.Storage.SearchOps().GetUniqueSysops(ctx, nameFilter, limit, offset)
	})
}

// GetNodesBySysop with caching
func (cs *CachedStorage) GetNodesBySysop(ctx context.Context, sysopName string, limit int) ([]database.Node, error) {
	return cachedFetch(cs, cs.keyGen.NodesBySysopKey(sysopName, limit), cs.config.SearchTTL, func() ([]database.Node, error) {
		return cs.Storage.SearchOps().GetNodesBySysop(ctx, sysopName, limit)
	})
}

// Pass-through methods (not cached)

// GetNodeDateRange returns the first and last date a node appears in nodelists
func (cs *CachedStorage) GetNodeDateRange(ctx context.Context, zone, net, node int, domain string) (firstDate, lastDate time.Time, err error) {
	// Not cached as this is rarely called
	return cs.Storage.NodeOps().GetNodeDateRange(ctx, zone, net, node, domain)
}

// SearchNodesBySysop searches for nodes by sysop name
func (cs *CachedStorage) SearchNodesBySysop(ctx context.Context, sysopName string, limit int, domain string) ([]NodeSummary, error) {
	// Not cached as this overlaps with GetNodesBySysop
	return cs.Storage.SearchOps().SearchNodesBySysop(ctx, sysopName, limit, domain)
}

// SearchNodesWithLifetime searches for nodes with lifetime information
func (cs *CachedStorage) SearchNodesWithLifetime(ctx context.Context, filter database.NodeFilter) ([]NodeSummary, error) {
	// Not cached as this is similar to GetNodes but with extra processing
	return cs.Storage.SearchOps().SearchNodesWithLifetime(ctx, filter)
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
