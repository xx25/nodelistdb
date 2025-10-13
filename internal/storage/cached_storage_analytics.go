package storage

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"
)

// Analytics-related caching operations (flags, networks, historical data)

// GetFlagFirstAppearance returns when a flag first appeared in the nodelist
func (cs *CachedStorage) GetFlagFirstAppearance(flagName string) (*FlagFirstAppearance, error) {
	if !cs.config.Enabled {
		return cs.Storage.AnalyticsOps().GetFlagFirstAppearance(flagName)
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
	fa, err := cs.Storage.AnalyticsOps().GetFlagFirstAppearance(flagName)
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
		return cs.Storage.AnalyticsOps().GetFlagUsageByYear(flagName)
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
	usage, err := cs.Storage.AnalyticsOps().GetFlagUsageByYear(flagName)
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
		return cs.Storage.AnalyticsOps().GetNetworkHistory(zone, net)
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
	history, err := cs.Storage.AnalyticsOps().GetNetworkHistory(zone, net)
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
