package storage

// GetAllWhoisResults returns all WHOIS results with node counts (cached),
// scoped to the given FTN network ("" = all networks). The domain is part of
// the cache key so each network keeps its own entry.
func (cs *CachedStorage) GetAllWhoisResults(domain string) ([]DomainWhoisResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("whois:results:v4", domain), cs.config.LongAnalyticsTTL, func() ([]DomainWhoisResult, error) {
		return cs.Storage.GetAllWhoisResults(domain)
	})
}

// GetNodesByDomain returns nodes for a specific domain (cached)
func (cs *CachedStorage) GetNodesByDomain(domain string, days int) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("whois:domain:v2", cs.keyGen.ShortHash(domain), days), cs.config.AnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetNodesByDomain(domain, days)
	})
}
