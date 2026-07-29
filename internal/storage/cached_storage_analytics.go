package storage

// Analytics-related caching operations (flags, networks, historical data)

// GetFlagFirstAppearance returns when a flag first appeared in the nodelist
func (cs *CachedStorage) GetFlagFirstAppearance(flagName string, domain string) (*FlagFirstAppearance, error) {
	return cachedFetch(cs, cs.analyticsKey("flag:first", flagName)+":"+domain, cs.config.HistoricalTTL, func() (*FlagFirstAppearance, error) {
		return cs.Storage.AnalyticsOps().GetFlagFirstAppearance(flagName, domain)
	})
}

// GetFlagUsageByYear returns flag usage statistics by year
func (cs *CachedStorage) GetFlagUsageByYear(flagName string, domain string) ([]FlagUsageByYear, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("flag:usage", flagName)+":"+domain, cs.config.HistoricalTTL, func() ([]FlagUsageByYear, error) {
		return cs.Storage.AnalyticsOps().GetFlagUsageByYear(flagName, domain)
	})
}

// GetNetworkHistory returns historical network statistics
func (cs *CachedStorage) GetNetworkHistory(zone, net int, domain string) (*NetworkHistory, error) {
	return cachedFetch(cs, cs.analyticsKey("network", zone, net)+":"+domain, cs.config.HistoricalTTL, func() (*NetworkHistory, error) {
		return cs.Storage.AnalyticsOps().GetNetworkHistory(zone, net, domain)
	})
}
