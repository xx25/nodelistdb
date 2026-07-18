package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/logging"
)

// Point-related caching operations. Pointlists update weekly, so the stats
// TTL is a comfortable fit for snapshot aggregates.

// cachedPointFetch wraps the standard cache-aside pattern used across the
// cached_storage_* files for the point readers.
func cachedPointFetch[T any](cs *CachedStorage, key string, ttl time.Duration, fetch func() (T, error)) (T, error) {
	if !cs.config.Enabled {
		return fetch()
	}

	if data, err := cs.cache.Get(context.Background(), key); err == nil {
		var result T
		if err := json.Unmarshal(data, &result); err == nil {
			atomic.AddUint64(&cs.cache.GetMetrics().Hits, 1)
			return result, nil
		}
		logging.Warn("Failed to unmarshal cached data", slog.String("key", key), slog.Any("error", err))
	}

	atomic.AddUint64(&cs.cache.GetMetrics().Misses, 1)

	result, err := fetch()
	if err != nil {
		return result, err
	}

	if data, err := json.Marshal(result); err == nil {
		_ = cs.cache.Set(context.Background(), key, data, ttl)
	}
	return result, nil
}

// pointAsOfKey renders the optional as-of anchor for cache keys.
func pointAsOfKey(asOf *time.Time) string {
	if asOf == nil || asOf.IsZero() {
		return "latest"
	}
	return asOf.Format("2006-01-02")
}

// GetPointsByBoss with caching
func (cs *CachedStorage) GetPointsByBoss(domain string, zone, net, node int, asOf *time.Time) ([]database.Point, error) {
	key := fmt.Sprintf("%s:points:boss:%s:%d:%d:%d:%s", cs.keyGen.Prefix, domain, zone, net, node, pointAsOfKey(asOf))
	return cachedPointFetch(cs, key, cs.config.NodeTTL, func() ([]database.Point, error) {
		return cs.Storage.PointOps().GetPointsByBoss(domain, zone, net, node, asOf)
	})
}

// GetPointHistory with caching
func (cs *CachedStorage) GetPointHistory(domain string, zone, net, node, point int) ([]database.Point, error) {
	key := fmt.Sprintf("%s:points:history:%s:%d:%d:%d:%d", cs.keyGen.Prefix, domain, zone, net, node, point)
	return cachedPointFetch(cs, key, cs.config.NodeTTL, func() ([]database.Point, error) {
		return cs.Storage.PointOps().GetPointHistory(domain, zone, net, node, point)
	})
}

// SearchPoints with caching
func (cs *CachedStorage) SearchPoints(filter database.PointFilter) ([]database.Point, error) {
	if filter.Limit > cs.config.MaxSearchResults {
		return cs.Storage.PointOps().SearchPoints(filter)
	}
	key := cs.keyGen.SearchKey(filter) + ":points"
	return cachedPointFetch(cs, key, cs.config.SearchTTL, func() ([]database.Point, error) {
		return cs.Storage.PointOps().SearchPoints(filter)
	})
}

// SearchPointsWithLifetime with caching
func (cs *CachedStorage) SearchPointsWithLifetime(ctx context.Context, filter database.PointFilter) ([]PointSummary, error) {
	if filter.Limit > cs.config.MaxSearchResults {
		return cs.Storage.PointOps().SearchPointsWithLifetime(ctx, filter)
	}
	key := cs.keyGen.SearchKey(filter) + ":pointsum"
	return cachedPointFetch(cs, key, cs.config.SearchTTL, func() ([]PointSummary, error) {
		return cs.Storage.PointOps().SearchPointsWithLifetime(ctx, filter)
	})
}

// GetPointStats with caching
func (cs *CachedStorage) GetPointStats(domain string, asOf *time.Time) (*PointStats, error) {
	key := fmt.Sprintf("%s:points:stats:%s:%s", cs.keyGen.Prefix, domain, pointAsOfKey(asOf))
	return cachedPointFetch(cs, key, cs.config.StatsTTL, func() (*PointStats, error) {
		return cs.Storage.PointOps().GetPointStats(domain, asOf)
	})
}

// GetPointCountsByNet with caching
func (cs *CachedStorage) GetPointCountsByNet(domain string, zone, net int, asOf *time.Time) (map[int]uint64, error) {
	key := fmt.Sprintf("%s:points:netcounts:%s:%d:%d:%s", cs.keyGen.Prefix, domain, zone, net, pointAsOfKey(asOf))
	return cachedPointFetch(cs, key, cs.config.StatsTTL, func() (map[int]uint64, error) {
		return cs.Storage.PointOps().GetPointCountsByNet(domain, zone, net, asOf)
	})
}

// GetPointlistDates with caching
func (cs *CachedStorage) GetPointlistDates(domain, listSource string) ([]database.PointlistFile, error) {
	key := fmt.Sprintf("%s:points:dates:%s:%s", cs.keyGen.Prefix, domain, listSource)
	return cachedPointFetch(cs, key, cs.config.StatsTTL, func() ([]database.PointlistFile, error) {
		return cs.Storage.PointOps().GetPointlistDates(domain, listSource)
	})
}

// GetPointlistSources with caching
func (cs *CachedStorage) GetPointlistSources(domain string) ([]PointlistSourceInfo, error) {
	key := fmt.Sprintf("%s:points:sources:%s", cs.keyGen.Prefix, domain)
	return cachedPointFetch(cs, key, cs.config.StatsTTL, func() ([]PointlistSourceInfo, error) {
		return cs.Storage.PointOps().GetPointlistSources(domain)
	})
}

// GetPointDomains with caching — resolvePointDomain hits this on every
// point-related API/web request.
func (cs *CachedStorage) GetPointDomains(zone, net, node int, point *int) ([]string, error) {
	pointKey := -1
	if point != nil {
		pointKey = *point
	}
	key := fmt.Sprintf("%s:points:domains:%d:%d:%d:%d", cs.keyGen.Prefix, zone, net, node, pointKey)
	return cachedPointFetch(cs, key, cs.config.NodeTTL, func() ([]string, error) {
		return cs.Storage.PointOps().GetPointDomains(zone, net, node, point)
	})
}

// latestPointlistDateEntry makes the (date, found) pair JSON-cacheable.
type latestPointlistDateEntry struct {
	Date  time.Time `json:"date"`
	Found bool      `json:"found"`
}

// LatestPointlistDate with caching
func (cs *CachedStorage) LatestPointlistDate(domain string) (time.Time, bool, error) {
	key := fmt.Sprintf("%s:points:latest:%s", cs.keyGen.Prefix, domain)
	entry, err := cachedPointFetch(cs, key, cs.config.StatsTTL, func() (latestPointlistDateEntry, error) {
		date, found, err := cs.Storage.PointOps().LatestPointlistDate(domain)
		return latestPointlistDateEntry{Date: date, Found: found}, err
	})
	return entry.Date, entry.Found, err
}
