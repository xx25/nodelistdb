package storage

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/logging"
)

// Node-related caching operations

// GetNodes with caching
func (cs *CachedStorage) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	if !cs.config.Enabled {
		return cs.Storage.NodeOps().GetNodes(filter)
	}

	// Skip cache for large result sets
	if filter.Limit > cs.config.MaxSearchResults {
		return cs.Storage.NodeOps().GetNodes(filter)
	}

	// Generate cache key
	key := cs.keyGen.SearchKey(filter)

	// Try cache first
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var nodes []database.Node
		if err := json.Unmarshal(data, &nodes); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return nodes, nil
		}
		// JSON unmarshal failed but cache entry exists - shouldn't happen
		logging.Warn("Failed to unmarshal cached data", slog.String("key", key), slog.Any("error", err))
	}

	// Cache miss - need to fetch from database
	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	nodes, err := cs.Storage.NodeOps().GetNodes(filter)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if data, err := json.Marshal(nodes); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.SearchTTL)
	}

	return nodes, nil
}

// GetNodeHistory with caching
func (cs *CachedStorage) GetNodeHistory(zone, net, node int) ([]database.Node, error) {
	if !cs.config.Enabled {
		return cs.Storage.NodeOps().GetNodeHistory(zone, net, node)
	}

	key := cs.keyGen.NodeHistoryKey(zone, net, node)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var history []database.Node
		if err := json.Unmarshal(data, &history); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return history, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	history, err := cs.Storage.NodeOps().GetNodeHistory(zone, net, node)
	if err != nil {
		return nil, err
	}

	// Cache result
	if data, err := json.Marshal(history); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.NodeTTL)
	}

	return history, nil
}

// GetNodeChanges with caching
func (cs *CachedStorage) GetNodeChanges(zone, net, node int) ([]database.NodeChange, error) {
	if !cs.config.Enabled {
		return cs.Storage.SearchOps().GetNodeChanges(zone, net, node)
	}

	key := cs.keyGen.NodeChangesKey(zone, net, node, "")

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var changes []database.NodeChange
		if err := json.Unmarshal(data, &changes); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return changes, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	changes, err := cs.Storage.SearchOps().GetNodeChanges(zone, net, node)
	if err != nil {
		return nil, err
	}

	// Cache result
	if data, err := json.Marshal(changes); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.NodeTTL)
	}

	return changes, nil
}

// GetUniqueSysops with caching
func (cs *CachedStorage) GetUniqueSysops(nameFilter string, limit, offset int) ([]SysopInfo, error) {
	if !cs.config.Enabled {
		return cs.Storage.SearchOps().GetUniqueSysops(nameFilter, limit, offset)
	}

	key := cs.keyGen.UniqueSysopsKey(nameFilter, limit, offset)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var sysops []SysopInfo
		if err := json.Unmarshal(data, &sysops); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return sysops, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	sysops, err := cs.Storage.SearchOps().GetUniqueSysops(nameFilter, limit, offset)
	if err != nil {
		return nil, err
	}

	// Cache result
	if data, err := json.Marshal(sysops); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.SearchTTL)
	}

	return sysops, nil
}

// GetNodesBySysop with caching
func (cs *CachedStorage) GetNodesBySysop(sysopName string, limit int) ([]database.Node, error) {
	if !cs.config.Enabled {
		return cs.Storage.SearchOps().GetNodesBySysop(sysopName, limit)
	}

	key := cs.keyGen.NodesBySysopKey(sysopName, limit)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var nodes []database.Node
		if err := json.Unmarshal(data, &nodes); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return nodes, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	nodes, err := cs.Storage.SearchOps().GetNodesBySysop(sysopName, limit)
	if err != nil {
		return nil, err
	}

	// Cache result
	if data, err := json.Marshal(nodes); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.SearchTTL)
	}

	return nodes, nil
}

// Pass-through methods (not cached)

// GetNodeDateRange returns the first and last date a node appears in nodelists
func (cs *CachedStorage) GetNodeDateRange(zone, net, node int) (firstDate, lastDate time.Time, err error) {
	// Not cached as this is rarely called
	return cs.Storage.NodeOps().GetNodeDateRange(zone, net, node)
}

// SearchNodesBySysop searches for nodes by sysop name
func (cs *CachedStorage) SearchNodesBySysop(sysopName string, limit int) ([]NodeSummary, error) {
	// Not cached as this overlaps with GetNodesBySysop
	return cs.Storage.SearchOps().SearchNodesBySysop(sysopName, limit)
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
