package storage

import (
	"context"
	"log"
	"time"
)

// Cache invalidation operations

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
