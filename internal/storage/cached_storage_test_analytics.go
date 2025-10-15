package storage

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"
)

// Test result analytics caching operations (IPv6, protocols, weekly news)

// GetIPv6EnabledNodes returns nodes that have been successfully tested with IPv6 (cached)
func (cs *CachedStorage) GetIPv6EnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetIPv6EnabledNodes(limit, days, includeZeroNodes)
	}

	key := cs.keyGen.IPv6EnabledNodesKey(limit, days, includeZeroNodes)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []NodeTestResult
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetIPv6EnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		return nil, err
	}

	// Cache result with 15 minute TTL (test results change frequently)
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 15*time.Minute)
		}
	}

	return results, nil
}

// GetIPv6NonWorkingNodes returns nodes with IPv6 but non-working services (cached)
func (cs *CachedStorage) GetIPv6NonWorkingNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetIPv6NonWorkingNodes(limit, days, includeZeroNodes)
	}

	key := cs.keyGen.IPv6NonWorkingNodesKey(limit, days, includeZeroNodes)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []NodeTestResult
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetIPv6NonWorkingNodes(limit, days, includeZeroNodes)
	if err != nil {
		return nil, err
	}

	// Cache result with 15 minute TTL
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 15*time.Minute)
		}
	}

	return results, nil
}

// GetIPv6AdvertisedIPv4OnlyNodes returns nodes advertising IPv6 but only accessible via IPv4 (cached)
func (cs *CachedStorage) GetIPv6AdvertisedIPv4OnlyNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetIPv6AdvertisedIPv4OnlyNodes(limit, days, includeZeroNodes)
	}

	key := cs.keyGen.IPv6AdvertisedIPv4OnlyNodesKey(limit, days, includeZeroNodes)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []NodeTestResult
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetIPv6AdvertisedIPv4OnlyNodes(limit, days, includeZeroNodes)
	if err != nil {
		return nil, err
	}

	// Cache result with 15 minute TTL
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 15*time.Minute)
		}
	}

	return results, nil
}

// GetBinkPEnabledNodes returns nodes with working BinkP protocol (cached)
func (cs *CachedStorage) GetBinkPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetBinkPEnabledNodes(limit, days, includeZeroNodes)
	}

	key := cs.keyGen.BinkPEnabledNodesKey(limit, days, includeZeroNodes)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []NodeTestResult
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetBinkPEnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		return nil, err
	}

	// Cache result with 15 minute TTL
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 15*time.Minute)
		}
	}

	return results, nil
}

// GetIfcicoEnabledNodes returns nodes with working IFCICO protocol (cached)
func (cs *CachedStorage) GetIfcicoEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetIfcicoEnabledNodes(limit, days, includeZeroNodes)
	}

	key := cs.keyGen.IfcicoEnabledNodesKey(limit, days, includeZeroNodes)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []NodeTestResult
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetIfcicoEnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		return nil, err
	}

	// Cache result with 15 minute TTL
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 15*time.Minute)
		}
	}

	return results, nil
}

// GetTelnetEnabledNodes returns nodes with working Telnet protocol (cached)
func (cs *CachedStorage) GetTelnetEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetTelnetEnabledNodes(limit, days, includeZeroNodes)
	}

	key := cs.keyGen.TelnetEnabledNodesKey(limit, days, includeZeroNodes)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []NodeTestResult
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetTelnetEnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		return nil, err
	}

	// Cache result with 15 minute TTL
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 15*time.Minute)
		}
	}

	return results, nil
}

// GetVModemEnabledNodes returns nodes with working VModem protocol (cached)
func (cs *CachedStorage) GetVModemEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetVModemEnabledNodes(limit, days, includeZeroNodes)
	}

	key := cs.keyGen.VModemEnabledNodesKey(limit, days, includeZeroNodes)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []NodeTestResult
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetVModemEnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		return nil, err
	}

	// Cache result with 15 minute TTL
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 15*time.Minute)
		}
	}

	return results, nil
}

// GetFTPEnabledNodes returns nodes with working FTP protocol (cached)
func (cs *CachedStorage) GetFTPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetFTPEnabledNodes(limit, days, includeZeroNodes)
	}

	key := cs.keyGen.FTPEnabledNodesKey(limit, days, includeZeroNodes)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []NodeTestResult
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetFTPEnabledNodes(limit, days, includeZeroNodes)
	if err != nil {
		return nil, err
	}

	// Cache result with 15 minute TTL
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 15*time.Minute)
		}
	}

	return results, nil
}

// GetBinkPSoftwareDistribution returns BinkP software distribution statistics (cached)
func (cs *CachedStorage) GetBinkPSoftwareDistribution(days int) (*SoftwareDistribution, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetBinkPSoftwareDistribution(days)
	}

	key := cs.keyGen.BinkPSoftwareDistributionKey(days)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var distribution SoftwareDistribution
		if err := json.Unmarshal(data, &distribution); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return &distribution, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	distribution, err := cs.Storage.GetBinkPSoftwareDistribution(days)
	if err != nil {
		return nil, err
	}

	// Cache result with 30 minute TTL (software distribution changes slowly)
	if distribution != nil {
		if data, err := json.Marshal(distribution); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 30*time.Minute)
		}
	}

	return distribution, nil
}

// GetIFCICOSoftwareDistribution returns IFCICO software distribution statistics (cached)
func (cs *CachedStorage) GetIFCICOSoftwareDistribution(days int) (*SoftwareDistribution, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetIFCICOSoftwareDistribution(days)
	}

	key := cs.keyGen.IFCICOSoftwareDistributionKey(days)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var distribution SoftwareDistribution
		if err := json.Unmarshal(data, &distribution); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return &distribution, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	distribution, err := cs.Storage.GetIFCICOSoftwareDistribution(days)
	if err != nil {
		return nil, err
	}

	// Cache result with 30 minute TTL
	if distribution != nil {
		if data, err := json.Marshal(distribution); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 30*time.Minute)
		}
	}

	return distribution, nil
}

// GetBinkdDetailedStats returns detailed Binkd statistics (cached)
func (cs *CachedStorage) GetBinkdDetailedStats(days int) (*SoftwareDistribution, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetBinkdDetailedStats(days)
	}

	key := cs.keyGen.BinkdDetailedStatsKey(days)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var stats SoftwareDistribution
		if err := json.Unmarshal(data, &stats); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return &stats, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	stats, err := cs.Storage.GetBinkdDetailedStats(days)
	if err != nil {
		return nil, err
	}

	// Cache result with 30 minute TTL
	if stats != nil {
		if data, err := json.Marshal(stats); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 30*time.Minute)
		}
	}

	return stats, nil
}

// GetIPv6WeeklyNews returns weekly IPv6 connectivity changes (cached)
// This is accessed via TestOps().GetIPv6WeeklyNews() in handlers,
// but we provide a direct cached wrapper for it
func (cs *CachedStorage) GetIPv6WeeklyNews(limit int, includeZeroNodes bool) (*IPv6WeeklyNews, error) {
	if !cs.config.Enabled {
		return cs.Storage.TestOps().GetIPv6WeeklyNews(limit, includeZeroNodes)
	}

	key := cs.keyGen.IPv6WeeklyNewsKey(limit, includeZeroNodes)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var news IPv6WeeklyNews
		if err := json.Unmarshal(data, &news); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return &news, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	news, err := cs.Storage.TestOps().GetIPv6WeeklyNews(limit, includeZeroNodes)
	if err != nil {
		return nil, err
	}

	// Cache result with 1 hour TTL (weekly news changes at most daily)
	if news != nil {
		if data, err := json.Marshal(news); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 1*time.Hour)
		}
	}

	return news, nil
}
