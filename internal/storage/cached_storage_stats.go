package storage

import (
	"time"

	"github.com/nodelistdb/internal/database"
)

// Statistics and dates caching operations

// GetStats with caching
func (cs *CachedStorage) GetStats(date time.Time, domain string) (*database.NetworkStats, error) {
	return cachedFetch(cs, cs.keyGen.StatsKey(date)+":"+domain, cs.config.StatsTTL, func() (*database.NetworkStats, error) {
		return cs.Storage.StatsOps().GetStats(date, domain)
	})
}

// GetLatestStatsDate with caching
func (cs *CachedStorage) GetLatestStatsDate(domain string) (time.Time, error) {
	return cachedFetch(cs, cs.keyGen.LatestStatsDateKey()+":"+domain, cs.config.StatsTTL, func() (time.Time, error) {
		return cs.Storage.StatsOps().GetLatestStatsDate(domain)
	})
}

// GetAvailableDates with caching
func (cs *CachedStorage) GetAvailableDates(domain string) ([]time.Time, error) {
	return cachedFetch(cs, cs.keyGen.AvailableDatesKey()+":"+domain, cs.config.StatsTTL, func() ([]time.Time, error) {
		return cs.Storage.StatsOps().GetAvailableDates(domain)
	})
}

// GetNearestAvailableDate with caching
func (cs *CachedStorage) GetNearestAvailableDate(targetDate time.Time, domain string) (time.Time, error) {
	return cachedFetch(cs, cs.keyGen.NearestDateKey(targetDate)+":"+domain, cs.config.StatsTTL, func() (time.Time, error) {
		return cs.Storage.StatsOps().GetNearestAvailableDate(targetDate, domain)
	})
}

// GetNodeCountHistory returns total node count per nodelist date (cached).
// The zero time stands in for "no single date" in the stats key namespace.
func (cs *CachedStorage) GetNodeCountHistory(domain string) ([]NodeCountByDate, error) {
	key := cs.keyGen.StatsKey(time.Time{}) + ":history:" + domain
	return cachedFetchSlice(cs, key, cs.config.StatsTTL, func() ([]NodeCountByDate, error) {
		return cs.Storage.GetNodeCountHistory(domain)
	})
}

// Pass-through methods (not cached)

// IsNodelistProcessed checks if a nodelist for a specific date has been processed
func (cs *CachedStorage) IsNodelistProcessed(nodelistDate time.Time, domain string) (bool, error) {
	// Not cached as this is used during import operations
	return cs.Storage.NodeOps().IsNodelistProcessed(nodelistDate, domain)
}

// FindConflictingNode checks if a node with the same address exists on a given date
func (cs *CachedStorage) FindConflictingNode(zone, net, node int, date time.Time, domain string) (bool, error) {
	// Not cached as this is used during import operations
	return cs.Storage.NodeOps().FindConflictingNode(zone, net, node, date, domain)
}

// GetMaxNodelistDate returns the maximum nodelist date in the database
func (cs *CachedStorage) GetMaxNodelistDate(domain string) (time.Time, error) {
	// Could be cached but usually called alongside GetLatestStatsDate
	return cs.Storage.NodeOps().GetMaxNodelistDate(domain)
}

// GetDomains lists the FTN networks in the database (not cached; cheap query)
func (cs *CachedStorage) GetDomains() ([]DomainInfo, error) {
	return cs.Storage.NodeOps().GetDomains()
}

// IsLatestNodelist checks if a date is the latest nodelist
func (cs *CachedStorage) IsLatestNodelist(date time.Time) (bool, error) {
	latestDate, err := cs.GetLatestStatsDate("")
	if err != nil {
		return false, err
	}
	return date.Equal(latestDate) || date.After(latestDate), nil
}
