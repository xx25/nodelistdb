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
func (cs *CachedStorage) GetIPv6EnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("ipv6:enabled", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetIPv6EnabledNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetIPv6NonWorkingNodes returns nodes with IPv6 but non-working services (cached)
func (cs *CachedStorage) GetIPv6NonWorkingNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("ipv6:nonworking", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetIPv6NonWorkingNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetIPv6AdvertisedIPv4OnlyNodes returns nodes advertising IPv6 but only accessible via IPv4 (cached)
func (cs *CachedStorage) GetIPv6AdvertisedIPv4OnlyNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("ipv6:ipv4only", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetIPv6AdvertisedIPv4OnlyNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetBinkPEnabledNodes returns nodes with working BinkP protocol (cached)
func (cs *CachedStorage) GetBinkPEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("binkp:enabled", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetBinkPEnabledNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetIfcicoEnabledNodes returns nodes with working IFCICO protocol (cached)
func (cs *CachedStorage) GetIfcicoEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("ifcico:enabled", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetIfcicoEnabledNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetTelnetEnabledNodes returns nodes with working Telnet protocol (cached)
func (cs *CachedStorage) GetTelnetEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("telnet:enabled", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetTelnetEnabledNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetVModemEnabledNodes returns nodes with working VModem protocol (cached)
func (cs *CachedStorage) GetVModemEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("vmodem:enabled", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetVModemEnabledNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetVModemUnconfirmedNodes returns nodes whose VModem probe did not confirm a genuine VMP responder (cached)
func (cs *CachedStorage) GetVModemUnconfirmedNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("vmodem:unconfirmed", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetVModemUnconfirmedNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetFTPEnabledNodes returns nodes with working FTP protocol (cached)
func (cs *CachedStorage) GetFTPEnabledNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("ftp:enabled", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetFTPEnabledNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetBinkPSoftwareDistribution returns BinkP software distribution statistics (cached)
func (cs *CachedStorage) GetBinkPSoftwareDistribution(days int, domain string) (*SoftwareDistribution, error) {
	return cachedFetch(cs, cs.analyticsKey("binkp:software", days, domain), cs.config.AnalyticsTTL, func() (*SoftwareDistribution, error) {
		return cs.Storage.GetBinkPSoftwareDistribution(days, domain)
	})
}

// GetIFCICOSoftwareDistribution returns IFCICO software distribution statistics (cached)
func (cs *CachedStorage) GetIFCICOSoftwareDistribution(days int, domain string) (*SoftwareDistribution, error) {
	return cachedFetch(cs, cs.analyticsKey("ifcico:software", days, domain), cs.config.AnalyticsTTL, func() (*SoftwareDistribution, error) {
		return cs.Storage.GetIFCICOSoftwareDistribution(days, domain)
	})
}

// GetBinkdDetailedStats returns detailed Binkd statistics (cached)
func (cs *CachedStorage) GetBinkdDetailedStats(days int, domain string) (*SoftwareDistribution, error) {
	return cachedFetch(cs, cs.analyticsKey("binkd:stats", days, domain), cs.config.AnalyticsTTL, func() (*SoftwareDistribution, error) {
		return cs.Storage.GetBinkdDetailedStats(days, domain)
	})
}

// GetIPv6WeeklyNews returns weekly IPv6 connectivity changes (cached)
// This is accessed via TestOps().GetIPv6WeeklyNews(domain) in handlers,
// but we provide a direct cached wrapper for it
func (cs *CachedStorage) GetIPv6WeeklyNews(limit int, includeZeroNodes bool, domain string) (*IPv6WeeklyNews, error) {
	return cachedFetch(cs, cs.analyticsKey("ipv6:weeklynews", limit, includeZeroNodes, domain), cs.config.LongAnalyticsTTL, func() (*IPv6WeeklyNews, error) {
		return cs.Storage.TestOps().GetIPv6WeeklyNews(limit, includeZeroNodes, domain)
	})
}

// GetIPv6OnlyNodes returns nodes with working IPv6 but no working IPv4 (cached)
func (cs *CachedStorage) GetIPv6OnlyNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("ipv6:only", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetIPv6OnlyNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetPureIPv6OnlyNodes returns nodes that only advertise IPv6 addresses (cached)
func (cs *CachedStorage) GetPureIPv6OnlyNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("ipv6:pureonly", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetPureIPv6OnlyNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetIPv6NodeList returns verified working IPv6 nodes for the node list report (cached)
func (cs *CachedStorage) GetIPv6NodeList(limit int, days int, includeZeroNodes bool, domain string) ([]IPv6NodeListEntry, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("ipv6:nodelist", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]IPv6NodeListEntry, error) {
		return cs.Storage.GetIPv6NodeList(limit, days, includeZeroNodes, domain)
	})
}

// GetGeoHostingDistribution returns geographic hosting distribution (cached)
func (cs *CachedStorage) GetGeoHostingDistribution(days int, domain string) (*GeoHostingDistribution, error) {
	return cachedFetch(cs, cs.analyticsKey("geo:hosting", days, domain), cs.config.LongAnalyticsTTL, func() (*GeoHostingDistribution, error) {
		return cs.Storage.GetGeoHostingDistribution(days, domain)
	})
}

// GetNodesByCountry returns nodes for a specific country (cached)
func (cs *CachedStorage) GetNodesByCountry(countryCode string, days int, domain string) ([]NodeTestResult, error) {
	return cachedGeoDrilldown(cs, cs.analyticsKey("geo:country:v2", countryCode, days, domain), func() ([]NodeTestResult, error) {
		return cs.Storage.GetNodesByCountry(countryCode, days, domain)
	})
}

// GetNodesByProvider returns nodes for a specific provider (cached)
func (cs *CachedStorage) GetNodesByProvider(provider string, days int, domain string) ([]NodeTestResult, error) {
	return cachedGeoDrilldown(cs, cs.analyticsKey("geo:provider:v2", cs.keyGen.ShortHash(provider), days, domain), func() ([]NodeTestResult, error) {
		return cs.Storage.GetNodesByProvider(provider, days, domain)
	})
}

// cachedGeoDrilldown is cachedFetchSlice for the two geo drill-downs, which
// cache an empty result rather than skipping it - but for less time.
//
// Their cache keys are the only analytics keys containing a caller-supplied
// string, so they are the only ones where a crawler can walk keys that resolve
// to nothing. Declining to store an empty result, which is what every other
// analytics reader does, would mean those keys never cache and re-run their
// query on every request. The shorter TTL for an empty answer bounds how long
// manufactured keys occupy the (unbounded, in-memory) cache.
func cachedGeoDrilldown(cs *CachedStorage, key string, fetch func() ([]NodeTestResult, error)) ([]NodeTestResult, error) {
	if !cs.config.Enabled {
		return fetch()
	}

	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []NodeTestResult
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	results, err := fetch()
	if err != nil {
		return nil, err
	}

	ttl := cs.config.AnalyticsTTL
	if len(results) == 0 {
		ttl = geoDrilldownEmptyTTL
	}
	if data, err := json.Marshal(results); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, ttl)
	}
	return results, nil
}

// geoDrilldownEmptyTTL is how long an empty geo drill-down is remembered.
const geoDrilldownEmptyTTL = 5 * time.Minute

// GetOnThisDayNodes returns nodes first added on this day in previous years (cached)
func (cs *CachedStorage) GetOnThisDayNodes(month, day, limit int, activeOnly bool, domain string) ([]OnThisDayNode, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("onthisday", month, day, limit, activeOnly, domain), cs.config.LongAnalyticsTTL, func() ([]OnThisDayNode, error) {
		return cs.Storage.GetOnThisDayNodes(month, day, limit, activeOnly, domain)
	})
}

// GetPioneersByRegion returns first sysops in a FidoNet region (cached)
func (cs *CachedStorage) GetPioneersByRegion(zone, region, limit int, domain string) ([]PioneerNode, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("pioneers", zone, region, limit, domain), cs.config.LongAnalyticsTTL, func() ([]PioneerNode, error) {
		return cs.Storage.GetPioneersByRegion(zone, region, limit, domain)
	})
}

// GetOtherNetworksSummary returns a summary of non-FidoNet networks found in AKAs (cached)
func (cs *CachedStorage) GetOtherNetworksSummary(days int, domain string) ([]OtherNetworkSummary, error) {
	return cachedFetchSlice(cs, fmt.Sprintf("%s:analytics:other_networks_summary:%d:%s", cs.keyGen.Prefix, days, domain), cs.config.LongAnalyticsTTL, func() ([]OtherNetworkSummary, error) {
		return cs.Storage.GetOtherNetworksSummary(days, domain)
	})
}

// GetNodesInNetwork returns nodes that announce AKAs in a specific network (cached)
func (cs *CachedStorage) GetIPv6IncorrectIPv4CorrectNodes(limit int, days int, includeZeroNodes bool, domain string) ([]AKAIPVersionMismatchNode, error) {
	return cachedFetchSlice(cs, fmt.Sprintf("%s:analytics:ipv6_incorrect_ipv4_correct:%d:%d:%v:%s", cs.keyGen.Prefix, limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]AKAIPVersionMismatchNode, error) {
		return cs.Storage.GetIPv6IncorrectIPv4CorrectNodes(limit, days, includeZeroNodes, domain)
	})
}

func (cs *CachedStorage) GetIPv4IncorrectIPv6CorrectNodes(limit int, days int, includeZeroNodes bool, domain string) ([]AKAIPVersionMismatchNode, error) {
	return cachedFetchSlice(cs, fmt.Sprintf("%s:analytics:ipv4_incorrect_ipv6_correct:%d:%d:%v:%s", cs.keyGen.Prefix, limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]AKAIPVersionMismatchNode, error) {
		return cs.Storage.GetIPv4IncorrectIPv6CorrectNodes(limit, days, includeZeroNodes, domain)
	})
}

func (cs *CachedStorage) GetNodesInNetwork(networkName string, limit int, days int, domain string) ([]OtherNetworkNode, error) {
	return cachedFetchSlice(cs, fmt.Sprintf("%s:analytics:nodes_in_network:%s:%d:%d:%s", cs.keyGen.Prefix, networkName, limit, days, domain), cs.config.LongAnalyticsTTL, func() ([]OtherNetworkNode, error) {
		return cs.Storage.GetNodesInNetwork(networkName, limit, days, domain)
	})
}

// GetModemAccessibleNodes returns nodes successfully reached via modem tests (cached)
func (cs *CachedStorage) GetModemAccessibleNodes(limit int, days int, includeZeroNodes bool, domain string) ([]ModemAccessibleNode, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("modem:accessible", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]ModemAccessibleNode, error) {
		return cs.Storage.GetModemAccessibleNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetModemNoAnswerNodes returns nodes tested via modem that never answered (cached)
func (cs *CachedStorage) GetModemNoAnswerNodes(limit int, days int, includeZeroNodes bool, domain string) ([]ModemNoAnswerNode, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("modem:noanswer", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]ModemNoAnswerNode, error) {
		return cs.Storage.GetModemNoAnswerNodes(limit, days, includeZeroNodes, domain)
	})
}

// GetRecentModemSuccessPhones returns phone numbers successfully tested via modem (pass-through, no cache)
func (cs *CachedStorage) GetRecentModemSuccessPhones(days int) ([]string, error) {
	return cs.Storage.GetRecentModemSuccessPhones(days)
}

// GetDetailedModemTestResult returns detailed modem test data (cached)
func (cs *CachedStorage) GetDetailedModemTestResult(zone, net, node int, testTime string) (*ModemTestDetail, error) {
	return cachedFetch(cs, cs.analyticsKey("modem:detail", zone, net, node, testTime), cs.config.TestAnalyticsTTL, func() (*ModemTestDetail, error) {
		return cs.Storage.GetDetailedModemTestResult(zone, net, node, testTime)
	})
}

// GetPSTNCMNodes returns PSTN CM nodes (cached)
func (cs *CachedStorage) GetPSTNCMNodes(limit int) ([]PSTNNode, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("pstn:cm", limit), cs.config.LongAnalyticsTTL, func() ([]PSTNNode, error) {
		return cs.Storage.GetPSTNCMNodes(limit)
	})
}

// GetPSTNNodes returns PSTN nodes (cached)
func (cs *CachedStorage) GetPSTNNodes(limit int, zone int, domain string) ([]PSTNNode, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("pstn:nodes", limit, zone, domain), cs.config.LongAnalyticsTTL, func() ([]PSTNNode, error) {
		return cs.Storage.GetPSTNNodes(limit, zone, domain)
	})
}

// GetFileRequestNodes returns file request capable nodes (cached)
func (cs *CachedStorage) GetFileRequestNodes(limit int, domain string) ([]FileRequestNode, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("filerequest", limit, domain), cs.config.LongAnalyticsTTL, func() ([]FileRequestNode, error) {
		return cs.Storage.GetFileRequestNodes(limit, domain)
	})
}

// GetEmailCapableNodes returns nodes advertising email transport (cached)
func (cs *CachedStorage) GetEmailCapableNodes(limit int, useFieldFallback bool, domain string) ([]EmailCapableNode, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("email:nodes", limit, useFieldFallback, domain), cs.config.LongAnalyticsTTL, func() ([]EmailCapableNode, error) {
		return cs.Storage.GetEmailCapableNodes(limit, useFieldFallback, domain)
	})
}

// GetEmailFlagTrend returns per-year email flag counts (cached).
// The trend only changes when a new nodelist lands, so it is held longer.
func (cs *CachedStorage) GetEmailFlagTrend(domain string) ([]EmailFlagTrendPoint, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("email:trend", domain), cs.config.HistoricalTTL, func() ([]EmailFlagTrendPoint, error) {
		return cs.Storage.GetEmailFlagTrend(domain)
	})
}

// GetAKAMismatchNodes returns AKA mismatch nodes (cached)
func (cs *CachedStorage) GetAKAMismatchNodes(limit int, days int, includeZeroNodes bool, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("aka:mismatch", limit, days, includeZeroNodes, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetAKAMismatchNodes(limit, days, includeZeroNodes, domain)
	})
}

// ===== Reachability Operations (cached) =====

// GetReachabilityTrends returns reachability trend data (cached)
func (cs *CachedStorage) GetReachabilityTrends(days int, domain string) ([]ReachabilityTrend, error) {
	return cachedFetchSlice(cs, cs.keyGen.ReachabilityTrendsKey(days, domain), cs.config.TestAnalyticsTTL, func() ([]ReachabilityTrend, error) {
		return cs.Storage.GetReachabilityTrends(days, domain)
	})
}

// GetReachabilityTrendsAllTime returns all-time reachability trend data (cached)
func (cs *CachedStorage) GetReachabilityTrendsAllTime(domain string) ([]ReachabilityTrend, error) {
	return cachedFetchSlice(cs, cs.keyGen.ReachabilityTrendsKey(0, domain), cs.config.TestAnalyticsTTL, func() ([]ReachabilityTrend, error) {
		return cs.Storage.GetReachabilityTrendsAllTime(domain)
	})
}

// SearchNodesByReachability returns nodes filtered by reachability status (cached)
func (cs *CachedStorage) SearchNodesByReachability(operational bool, limit int, days int, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.keyGen.SearchNodesByReachabilityKey(operational, limit, days, domain), cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.SearchNodesByReachability(operational, limit, days, domain)
	})
}

// GetNodeTestHistory returns test history for a specific node (cached)
func (cs *CachedStorage) GetNodeTestHistory(zone, net, node int, days int, domain string) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.keyGen.NodeTestHistoryKey(zone, net, node, days)+":"+domain, cs.config.TestAnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetNodeTestHistory(zone, net, node, days, domain)
	})
}

// GetNodeReachabilityStats returns reachability statistics for a specific node (cached)
func (cs *CachedStorage) GetNodeReachabilityStats(zone, net, node int, days int, domain string) (*NodeReachabilityStats, error) {
	return cachedFetch(cs, cs.keyGen.NodeReachabilityStatsKey(zone, net, node, days)+":"+domain, cs.config.TestAnalyticsTTL, func() (*NodeReachabilityStats, error) {
		return cs.Storage.GetNodeReachabilityStats(zone, net, node, days, domain)
	})
}

// GetDetailedTestResult returns detailed test result for a specific test (cached)
func (cs *CachedStorage) GetDetailedTestResult(zone, net, node int, testTime string, domain string) (*NodeTestResult, error) {
	return cachedFetch(cs, cs.keyGen.DetailedTestResultKey(zone, net, node, testTime)+":"+domain, cs.config.TestAnalyticsTTL, func() (*NodeTestResult, error) {
		return cs.Storage.GetDetailedTestResult(zone, net, node, testTime, domain)
	})
}
