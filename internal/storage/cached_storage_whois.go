package storage

import "context"

// GetAllWhoisResults returns all WHOIS results with node counts (cached),
// scoped to the given FTN network ("" = all networks). The domain is part of
// the cache key so each network keeps its own entry.
func (cs *CachedStorage) GetAllWhoisResults(ctx context.Context, domain string) ([]DomainWhoisResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("whois:results:v4", whoisDomainKey(domain)), cs.config.LongAnalyticsTTL, func() ([]DomainWhoisResult, error) {
		return cs.Storage.GetAllWhoisResults(ctx, domain)
	})
}

// GetNodesByDomain returns nodes for a specific domain (cached)
func (cs *CachedStorage) GetNodesByDomain(ctx context.Context, domain string, days int) ([]NodeTestResult, error) {
	return cachedFetchSlice(cs, cs.analyticsKey("whois:domain:v2", cs.keyGen.ShortHash(domain), days), cs.config.AnalyticsTTL, func() ([]NodeTestResult, error) {
		return cs.Storage.GetNodesByDomain(ctx, domain, days)
	})
}

// whoisDomainKey renders the FTN network for the WHOIS results key. The
// all-networks case is "*", not an empty segment: "*" is outside the valid
// network-name charset ([a-z0-9_-]) so it cannot collide with a network
// literally named "all", and keeping the spelling means entries written before
// this key moved to analyticsKey are still found.
func whoisDomainKey(domain string) string {
	if domain == "" {
		return "*"
	}
	return domain
}
