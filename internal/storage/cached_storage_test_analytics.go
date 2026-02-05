package storage

import (
	"context"
	"encoding/json"
	"fmt"
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

// GetIPv6OnlyNodes returns nodes with working IPv6 but no working IPv4 (cached)
func (cs *CachedStorage) GetIPv6OnlyNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetIPv6OnlyNodes(limit, days, includeZeroNodes)
	}

	key := cs.keyGen.IPv6OnlyNodesKey(limit, days, includeZeroNodes)

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
	results, err := cs.Storage.GetIPv6OnlyNodes(limit, days, includeZeroNodes)
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

// GetPureIPv6OnlyNodes returns nodes that only advertise IPv6 addresses (cached)
func (cs *CachedStorage) GetPureIPv6OnlyNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetPureIPv6OnlyNodes(limit, days, includeZeroNodes)
	}

	key := cs.keyGen.PureIPv6OnlyNodesKey(limit, days, includeZeroNodes)

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
	results, err := cs.Storage.GetPureIPv6OnlyNodes(limit, days, includeZeroNodes)
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

// GetIPv6NodeList returns verified working IPv6 nodes for the node list report (cached)
func (cs *CachedStorage) GetIPv6NodeList(limit int, days int, includeZeroNodes bool) ([]IPv6NodeListEntry, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetIPv6NodeList(limit, days, includeZeroNodes)
	}

	key := cs.keyGen.IPv6NodeListKey(limit, days, includeZeroNodes)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []IPv6NodeListEntry
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetIPv6NodeList(limit, days, includeZeroNodes)
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

// GetGeoHostingDistribution returns geographic hosting distribution (cached)
func (cs *CachedStorage) GetGeoHostingDistribution(days int) (*GeoHostingDistribution, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetGeoHostingDistribution(days)
	}

	key := cs.keyGen.GeoHostingDistributionKey(days)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var dist GeoHostingDistribution
		if err := json.Unmarshal(data, &dist); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return &dist, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	dist, err := cs.Storage.GetGeoHostingDistribution(days)
	if err != nil {
		return nil, err
	}

	// Cache result with 1 hour TTL (geo data changes slowly)
	if dist != nil {
		if data, err := json.Marshal(dist); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 1*time.Hour)
		}
	}

	return dist, nil
}

// GetNodesByCountry returns nodes for a specific country (cached)
func (cs *CachedStorage) GetNodesByCountry(countryCode string, days int) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetNodesByCountry(countryCode, days)
	}

	key := cs.keyGen.NodesByCountryKey(countryCode, days)

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
	results, err := cs.Storage.GetNodesByCountry(countryCode, days)
	if err != nil {
		return nil, err
	}

	// Cache result with 30 minute TTL
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 30*time.Minute)
		}
	}

	return results, nil
}

// GetNodesByProvider returns nodes for a specific provider (cached)
func (cs *CachedStorage) GetNodesByProvider(provider string, days int) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetNodesByProvider(provider, days)
	}

	key := cs.keyGen.NodesByProviderKey(provider, days)

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
	results, err := cs.Storage.GetNodesByProvider(provider, days)
	if err != nil {
		return nil, err
	}

	// Cache result with 30 minute TTL
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 30*time.Minute)
		}
	}

	return results, nil
}

// GetOnThisDayNodes returns nodes first added on this day in previous years (cached)
func (cs *CachedStorage) GetOnThisDayNodes(month, day, limit int, activeOnly bool) ([]OnThisDayNode, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetOnThisDayNodes(month, day, limit, activeOnly)
	}

	key := cs.keyGen.OnThisDayNodesKey(month, day, limit, activeOnly)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []OnThisDayNode
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetOnThisDayNodes(month, day, limit, activeOnly)
	if err != nil {
		return nil, err
	}

	// Cache result with 1 hour TTL (historical data doesn't change often)
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 1*time.Hour)
		}
	}

	return results, nil
}

// GetPioneersByRegion returns first sysops in a FidoNet region (cached)
func (cs *CachedStorage) GetPioneersByRegion(zone, region, limit int) ([]PioneerNode, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetPioneersByRegion(zone, region, limit)
	}

	key := cs.keyGen.PioneersByRegionKey(zone, region, limit)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []PioneerNode
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetPioneersByRegion(zone, region, limit)
	if err != nil {
		return nil, err
	}

	// Cache result with 1 hour TTL (historical data doesn't change often)
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 1*time.Hour)
		}
	}

	return results, nil
}

// GetOtherNetworksSummary returns a summary of non-FidoNet networks found in AKAs (cached)
func (cs *CachedStorage) GetOtherNetworksSummary(days int) ([]OtherNetworkSummary, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetOtherNetworksSummary(days)
	}

	key := fmt.Sprintf("other_networks_summary:%d", days)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []OtherNetworkSummary
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetOtherNetworksSummary(days)
	if err != nil {
		return nil, err
	}

	// Cache result with 1 hour TTL
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 1*time.Hour)
		}
	}

	return results, nil
}

// GetNodesInNetwork returns nodes that announce AKAs in a specific network (cached)
func (cs *CachedStorage) GetIPv6IncorrectIPv4CorrectNodes(limit int, days int, includeZeroNodes bool) ([]AKAIPVersionMismatchNode, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetIPv6IncorrectIPv4CorrectNodes(limit, days, includeZeroNodes)
	}

	key := fmt.Sprintf("ipv6_incorrect_ipv4_correct:%d:%d:%v", limit, days, includeZeroNodes)

	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []AKAIPVersionMismatchNode
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	results, err := cs.Storage.GetIPv6IncorrectIPv4CorrectNodes(limit, days, includeZeroNodes)
	if err != nil {
		return nil, err
	}

	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 15*time.Minute)
		}
	}

	return results, nil
}

func (cs *CachedStorage) GetIPv4IncorrectIPv6CorrectNodes(limit int, days int, includeZeroNodes bool) ([]AKAIPVersionMismatchNode, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetIPv4IncorrectIPv6CorrectNodes(limit, days, includeZeroNodes)
	}

	key := fmt.Sprintf("ipv4_incorrect_ipv6_correct:%d:%d:%v", limit, days, includeZeroNodes)

	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []AKAIPVersionMismatchNode
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	results, err := cs.Storage.GetIPv4IncorrectIPv6CorrectNodes(limit, days, includeZeroNodes)
	if err != nil {
		return nil, err
	}

	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 15*time.Minute)
		}
	}

	return results, nil
}

func (cs *CachedStorage) GetNodesInNetwork(networkName string, limit int, days int) ([]OtherNetworkNode, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetNodesInNetwork(networkName, limit, days)
	}

	key := fmt.Sprintf("nodes_in_network:%s:%d:%d", networkName, limit, days)

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []OtherNetworkNode
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetNodesInNetwork(networkName, limit, days)
	if err != nil {
		return nil, err
	}

	// Cache result with 1 hour TTL
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 1*time.Hour)
		}
	}

	return results, nil
}

// GetModemAccessibleNodes returns nodes successfully reached via modem tests (pass-through, no caching)
func (cs *CachedStorage) GetModemAccessibleNodes(limit int, days int, includeZeroNodes bool) ([]ModemAccessibleNode, error) {
	return cs.Storage.GetModemAccessibleNodes(limit, days, includeZeroNodes)
}
