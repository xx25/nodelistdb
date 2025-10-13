package storage

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/nodelistdb/internal/database"
)

// Statistics and dates caching operations

// GetStats with caching
func (cs *CachedStorage) GetStats(date time.Time) (*database.NetworkStats, error) {
	if !cs.config.Enabled {
		return cs.Storage.StatsOps().GetStats(date)
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
	stats, err := cs.Storage.StatsOps().GetStats(date)
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
		return cs.Storage.StatsOps().GetLatestStatsDate()
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
	date, err := cs.Storage.StatsOps().GetLatestStatsDate()
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
		return cs.Storage.StatsOps().GetAvailableDates()
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
	dates, err := cs.Storage.StatsOps().GetAvailableDates()
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
		return cs.Storage.StatsOps().GetNearestAvailableDate(targetDate)
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
	date, err := cs.Storage.StatsOps().GetNearestAvailableDate(targetDate)
	if err != nil {
		return time.Time{}, err
	}

	// Cache result
	if data, err := json.Marshal(date); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, cs.config.StatsTTL)
	}

	return date, nil
}

// Pass-through methods (not cached)

// IsNodelistProcessed checks if a nodelist for a specific date has been processed
func (cs *CachedStorage) IsNodelistProcessed(nodelistDate time.Time) (bool, error) {
	// Not cached as this is used during import operations
	return cs.Storage.NodeOps().IsNodelistProcessed(nodelistDate)
}

// FindConflictingNode checks if a node with the same address exists on a given date
func (cs *CachedStorage) FindConflictingNode(zone, net, node int, date time.Time) (bool, error) {
	// Not cached as this is used during import operations
	return cs.Storage.NodeOps().FindConflictingNode(zone, net, node, date)
}

// GetMaxNodelistDate returns the maximum nodelist date in the database
func (cs *CachedStorage) GetMaxNodelistDate() (time.Time, error) {
	// Could be cached but usually called alongside GetLatestStatsDate
	return cs.Storage.NodeOps().GetMaxNodelistDate()
}

// IsLatestNodelist checks if a date is the latest nodelist
func (cs *CachedStorage) IsLatestNodelist(date time.Time) (bool, error) {
	latestDate, err := cs.GetLatestStatsDate()
	if err != nil {
		return false, err
	}
	return date.Equal(latestDate) || date.After(latestDate), nil
}
