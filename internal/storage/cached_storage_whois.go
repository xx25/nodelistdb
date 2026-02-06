package storage

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"
)

// GetAllWhoisResults returns all WHOIS results with node counts (cached)
func (cs *CachedStorage) GetAllWhoisResults() ([]DomainWhoisResult, error) {
	if !cs.config.Enabled {
		return cs.Storage.GetAllWhoisResults()
	}

	key := cs.keyGen.WhoisResultsKey()

	// Try cache
	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var results []DomainWhoisResult
		if err := json.Unmarshal(data, &results); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return results, nil
		}
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	// Fall back to database
	results, err := cs.Storage.GetAllWhoisResults()
	if err != nil {
		return nil, err
	}

	// Cache result with 1 hour TTL (WHOIS data changes infrequently)
	if len(results) > 0 {
		if data, err := json.Marshal(results); err == nil {
			_ = cs.cache.Set(context.Background(), key, data, 1*time.Hour)
		}
	}

	return results, nil
}
