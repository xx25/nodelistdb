package storage

import (
	"context"
	"encoding/json"
	"log"
	"sync/atomic"
	"time"

	"github.com/nodelistdb/internal/cache"
	"github.com/nodelistdb/internal/database"
)

type CachedStorage struct {
	*Storage
	cache    cache.Cache
	keyGen   *cache.KeyGenerator
	config   *CacheStorageConfig
}

type CacheStorageConfig struct {
	Enabled           bool
	DefaultTTL        time.Duration
	NodeTTL           time.Duration
	StatsTTL          time.Duration
	SearchTTL         time.Duration
	MaxSearchResults  int
	WarmupOnStart     bool
}

func NewCachedStorage(storage *Storage, cacheImpl cache.Cache, config *CacheStorageConfig) *CachedStorage {
	if config == nil {
		config = &CacheStorageConfig{
			Enabled:          true,
			DefaultTTL:       5 * time.Minute,
			NodeTTL:          15 * time.Minute,
			StatsTTL:         1 * time.Hour,
			SearchTTL:        5 * time.Minute,
			MaxSearchResults: 500,
		}
	}

	cs := &CachedStorage{
		Storage: storage,
		cache:   cacheImpl,
		keyGen:  cache.NewKeyGenerator("ndb"),
		config:  config,
	}

	if config.WarmupOnStart {
		go cs.warmupCache()
	}

	return cs
}

// GetNodes with caching
func (cs *CachedStorage) GetNodes(filter database.NodeFilter) ([]database.Node, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetNodes(filter)
	}

	// Skip cache for large result sets
	if filter.Limit > cs.config.MaxSearchResults {
		return cs.Storage.GetNodes(filter)
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
		log.Printf("Warning: Failed to unmarshal cached data for key %s: %v", key, err)
	}
	
	// Cache miss - need to fetch from database
	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)
	
	// Fall back to database
	nodes, err := cs.Storage.GetNodes(filter)
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
		return cs.Storage.GetNodeHistory(zone, net, node)
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
	history, err := cs.Storage.GetNodeHistory(zone, net, node)
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
		return cs.Storage.GetNodeChanges(zone, net, node)
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
	changes, err := cs.Storage.GetNodeChanges(zone, net, node)
	if err != nil {
		return nil, err
	}

	// Cache result
	if data, err := json.Marshal(changes); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.NodeTTL)
	}
	
	return changes, nil
}

// GetStats with caching
func (cs *CachedStorage) GetStats(date time.Time) (*database.NetworkStats, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetStats(date)
	}

	key := cs.keyGen.StatsKey(date)
	
	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var stats database.NetworkStats
		if err := json.Unmarshal(data, &stats); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return &stats, nil
		}
	}
	
	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)
	
	// Fall back to database
	stats, err := cs.Storage.GetStats(date)
	if err != nil {
		return nil, err
	}
	
	// Cache result
	if data, err := json.Marshal(stats); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.StatsTTL)
	}
	
	return stats, nil
}

// GetLatestStatsDate with caching
func (cs *CachedStorage) GetLatestStatsDate() (time.Time, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetLatestStatsDate()
	}

	key := cs.keyGen.LatestStatsDateKey()
	
	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var date time.Time
		if err := json.Unmarshal(data, &date); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return date, nil
		}
	}
	
	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)
	
	// Fall back to database
	date, err := cs.Storage.GetLatestStatsDate()
	if err != nil {
		return time.Time{}, err
	}
	
	// Cache result
	if data, err := json.Marshal(date); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.StatsTTL)
	}
	
	return date, nil
}

// GetAvailableDates with caching
func (cs *CachedStorage) GetAvailableDates() ([]time.Time, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetAvailableDates()
	}

	key := cs.keyGen.AvailableDatesKey()
	
	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var dates []time.Time
		if err := json.Unmarshal(data, &dates); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return dates, nil
		}
	}
	
	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)
	
	// Fall back to database
	dates, err := cs.Storage.GetAvailableDates()
	if err != nil {
		return nil, err
	}
	
	// Cache result
	if data, err := json.Marshal(dates); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.StatsTTL)
	}
	
	return dates, nil
}

// GetNearestAvailableDate with caching
func (cs *CachedStorage) GetNearestAvailableDate(targetDate time.Time) (time.Time, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetNearestAvailableDate(targetDate)
	}

	key := cs.keyGen.NearestDateKey(targetDate)
	
	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var date time.Time
		if err := json.Unmarshal(data, &date); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return date, nil
		}
	}
	
	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)
	
	// Fall back to database
	date, err := cs.Storage.GetNearestAvailableDate(targetDate)
	if err != nil {
		return time.Time{}, err
	}
	
	// Cache result
	if data, err := json.Marshal(date); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.StatsTTL)
	}
	
	return date, nil
}

// GetUniqueSysops with caching
func (cs *CachedStorage) GetUniqueSysops(nameFilter string, limit, offset int) ([]SysopInfo, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetUniqueSysops(nameFilter, limit, offset)
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
	sysops, err := cs.Storage.GetUniqueSysops(nameFilter, limit, offset)
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
		return cs.Storage.GetNodesBySysop(sysopName, limit)
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
	nodes, err := cs.Storage.GetNodesBySysop(sysopName, limit)
	if err != nil {
		return nil, err
	}
	
	// Cache result
	if data, err := json.Marshal(nodes); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.SearchTTL)
	}
	
	return nodes, nil
}

// Cache invalidation methods

// InvalidateNode clears cache for a specific node
func (cs *CachedStorage) InvalidateNode(zone, net, node int) error {
	if !cs.config.Enabled {
		return nil
	}
	pattern := cs.keyGen.NodePattern(zone, net, node)
	return cs.cache.DeleteByPattern(context.Background(), pattern)
}

// InvalidateAll clears entire cache (used after nodelist import)
func (cs *CachedStorage) InvalidateAll() error {
	if !cs.config.Enabled {
		return nil
	}
	log.Println("Invalidating entire cache...")
	return cs.cache.DeleteByPattern(context.Background(), cs.keyGen.AllPattern())
}

// InvalidateStats clears all statistics cache entries
func (cs *CachedStorage) InvalidateStats() error {
	if !cs.config.Enabled {
		return nil
	}
	log.Println("Invalidating statistics cache...")
	return cs.cache.DeleteByPattern(context.Background(), cs.keyGen.StatsPattern())
}

// InvalidateSearches clears all search result cache entries
func (cs *CachedStorage) InvalidateSearches() error {
	if !cs.config.Enabled {
		return nil
	}
	log.Println("Invalidating search cache...")
	return cs.cache.DeleteByPattern(context.Background(), cs.keyGen.SearchPattern())
}

// InvalidateDates clears all date-related cache entries
func (cs *CachedStorage) InvalidateDates() error {
	if !cs.config.Enabled {
		return nil
	}
	log.Println("Invalidating dates cache...")
	return cs.cache.DeleteByPattern(context.Background(), cs.keyGen.DatesPattern())
}

// InvalidateSysops clears all sysop-related cache entries
func (cs *CachedStorage) InvalidateSysops() error {
	if !cs.config.Enabled {
		return nil
	}
	log.Println("Invalidating sysops cache...")
	return cs.cache.DeleteByPattern(context.Background(), cs.keyGen.SysopsPattern())
}

// InvalidateAnalytics clears all analytics-related cache entries
func (cs *CachedStorage) InvalidateAnalytics() error {
	if !cs.config.Enabled {
		return nil
	}
	log.Println("Invalidating analytics cache...")
	return cs.cache.DeleteByPattern(context.Background(), cs.keyGen.AnalyticsPattern())
}

// InvalidateAfterImport performs smart cache invalidation after nodelist import
func (cs *CachedStorage) InvalidateAfterImport(nodelistDate time.Time, clearAll bool) error {
	if !cs.config.Enabled {
		return nil
	}

	log.Printf("Invalidating cache after nodelist import for date %s", nodelistDate.Format("2006-01-02"))

	// Strategy 1: Clear everything (simple but aggressive)
	if clearAll {
		return cs.InvalidateAll()
	}

	// Strategy 2: Selective invalidation (more efficient)
	// Clear stats cache since new data affects statistics
	if err := cs.InvalidateStats(); err != nil {
		log.Printf("Failed to invalidate stats cache: %v", err)
	}

	// Clear search results cache since results may have changed
	if err := cs.InvalidateSearches(); err != nil {
		log.Printf("Failed to invalidate search cache: %v", err)
	}

	// Clear dates cache since we have new dates available
	if err := cs.InvalidateDates(); err != nil {
		log.Printf("Failed to invalidate dates cache: %v", err)
	}

	// Clear analytics cache since new data affects flag/network statistics
	if err := cs.InvalidateAnalytics(); err != nil {
		log.Printf("Failed to invalidate analytics cache: %v", err)
	}

	// Keep node-specific caches if they're for older dates
	// Only invalidate if the imported date is recent (within last 7 days)
	if nodelistDate.After(time.Now().AddDate(0, 0, -7)) {
		// For recent imports, also clear sysop caches
		if err := cs.InvalidateSysops(); err != nil {
			log.Printf("Failed to invalidate sysops cache: %v", err)
		}
	}

	return nil
}

// GetCacheMetrics returns cache performance metrics
func (cs *CachedStorage) GetCacheMetrics() *cache.Metrics {
	if cs.cache == nil {
		return nil
	}
	return cs.cache.GetMetrics()
}

// warmupCache pre-populates cache with frequently accessed data
func (cs *CachedStorage) warmupCache() {
	log.Println("Starting cache warmup...")

	// Pre-cache latest stats
	if date, err := cs.Storage.GetLatestStatsDate(); err == nil {
		_, _ = cs.GetStats(date)
	}

	// Pre-cache available dates
	_, _ = cs.GetAvailableDates()

	// Pre-cache some popular nodes (example addresses)
	popularNodes := []struct{ Zone, Net, Node int }{
		{2, 450, 1024},
		{1, 1, 1},
		{2, 2, 20},
	}

	for _, node := range popularNodes {
		_, _ = cs.GetNodeHistory(node.Zone, node.Net, node.Node)
	}

	log.Println("Cache warmup completed")
}

// Close closes the cache
func (cs *CachedStorage) Close() error {
	if cs.cache != nil {
		return cs.cache.Close()
	}
	return nil
}

// IsLatestNodelist checks if a date is the latest nodelist
func (cs *CachedStorage) IsLatestNodelist(date time.Time) (bool, error) {
	latestDate, err := cs.GetLatestStatsDate()
	if err != nil {
		return false, err
	}
	return date.Equal(latestDate) || date.After(latestDate), nil
}

// SetTemporaryTTL temporarily reduces TTL for cache entries (used after imports)
func (cs *CachedStorage) SetTemporaryTTL(ttl time.Duration) {
	// This would require a more complex implementation to track
	// and restore the original TTL values
	// For now, we'll just log the intent
	log.Printf("Would set temporary TTL to %v (not implemented)", ttl)
}

// GetNodeDateRange returns the first and last date a node appears in nodelists
func (cs *CachedStorage) GetNodeDateRange(zone, net, node int) (firstDate, lastDate time.Time, err error) {
	// Not cached as this is rarely called
	return cs.Storage.GetNodeDateRange(zone, net, node)
}

// SearchNodesBySysop searches for nodes by sysop name
func (cs *CachedStorage) SearchNodesBySysop(sysopName string, limit int) ([]NodeSummary, error) {
	// Not cached as this overlaps with GetNodesBySysop
	return cs.Storage.SearchNodesBySysop(sysopName, limit)
}

// IsNodelistProcessed checks if a nodelist for a specific date has been processed
func (cs *CachedStorage) IsNodelistProcessed(nodelistDate time.Time) (bool, error) {
	// Not cached as this is used during import operations
	return cs.Storage.IsNodelistProcessed(nodelistDate)
}

// FindConflictingNode checks if a node with the same address exists on a given date
func (cs *CachedStorage) FindConflictingNode(zone, net, node int, date time.Time) (bool, error) {
	// Not cached as this is used during import operations
	return cs.Storage.FindConflictingNode(zone, net, node, date)
}

// GetMaxNodelistDate returns the maximum nodelist date in the database
func (cs *CachedStorage) GetMaxNodelistDate() (time.Time, error) {
	// Could be cached but usually called alongside GetLatestStatsDate
	return cs.Storage.GetMaxNodelistDate()
}

// InsertNodes inserts a batch of nodes into the database
func (cs *CachedStorage) InsertNodes(nodes []database.Node) error {
	// Invalidate relevant caches after insertion
	err := cs.Storage.InsertNodes(nodes)
	if err == nil && len(nodes) > 0 {
		// Get the nodelist date from the first node
		nodelistDate := nodes[0].NodelistDate
		// Invalidate caches affected by the new data
		_ = cs.InvalidateAfterImport(nodelistDate, false)
	}
	return err
}

// SearchNodesWithLifetime searches for nodes with lifetime information
func (cs *CachedStorage) SearchNodesWithLifetime(filter database.NodeFilter) ([]NodeSummary, error) {
	// Not cached as this is similar to GetNodes but with extra processing
	return cs.Storage.SearchNodesWithLifetime(filter)
}

// GetFlagFirstAppearance returns when a flag first appeared in the nodelist
func (cs *CachedStorage) GetFlagFirstAppearance(flagName string) (*FlagFirstAppearance, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetFlagFirstAppearance(flagName)
	}

	key := cs.keyGen.FlagFirstAppearanceKey(flagName)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var fa FlagFirstAppearance
		if err := json.Unmarshal(data, &fa); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return &fa, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	fa, err := cs.Storage.GetFlagFirstAppearance(flagName)
	if err != nil {
		return nil, err
	}

	// Cache result (long TTL since historical data doesn't change)
	if fa != nil {
		if data, err := json.Marshal(fa); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 24*time.Hour)
		}
	}

	return fa, nil
}

// GetFlagUsageByYear returns flag usage statistics by year
func (cs *CachedStorage) GetFlagUsageByYear(flagName string) ([]FlagUsageByYear, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetFlagUsageByYear(flagName)
	}

	key := cs.keyGen.FlagUsageByYearKey(flagName)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var usage []FlagUsageByYear
		if err := json.Unmarshal(data, &usage); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return usage, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	usage, err := cs.Storage.GetFlagUsageByYear(flagName)
	if err != nil {
		return nil, err
	}

	// Cache result (long TTL since historical data doesn't change much)
	if len(usage) > 0 {
		if data, err := json.Marshal(usage); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 24*time.Hour)
		}
	}

	return usage, nil
}

// GetNetworkHistory returns historical network statistics
func (cs *CachedStorage) GetNetworkHistory(zone, net int) (*NetworkHistory, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetNetworkHistory(zone, net)
	}

	key := cs.keyGen.NetworkHistoryKey(zone, net)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var history NetworkHistory
		if err := json.Unmarshal(data, &history); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return &history, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	history, err := cs.Storage.GetNetworkHistory(zone, net)
	if err != nil {
		return nil, err
	}

	// Cache result (long TTL since historical data doesn't change much)
	if history != nil {
		if data, err := json.Marshal(history); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 24*time.Hour)
		}
	}

	return history, nil
}