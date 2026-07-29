package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/database"
)

// Point-related caching operations. Pointlists update weekly, so the stats
// TTL is a comfortable fit for snapshot aggregates.

// pointAsOfKey renders the optional as-of anchor for cache keys.
func pointAsOfKey(asOf *time.Time) string {
	if asOf == nil || asOf.IsZero() {
		return "latest"
	}
	return asOf.Format("2006-01-02")
}

// GetPointsByBoss with caching
func (cs *CachedStorage) GetPointsByBoss(ctx context.Context, domain string, zone, net, node int, asOf *time.Time) ([]database.Point, error) {
	key := fmt.Sprintf("%s:points:boss:%s:%d:%d:%d:%s", cs.keyGen.Prefix, domain, zone, net, node, pointAsOfKey(asOf))
	return cachedFetch(cs, key, cs.config.NodeTTL, func() ([]database.Point, error) {
		return cs.Storage.PointOps().GetPointsByBoss(ctx, domain, zone, net, node, asOf)
	})
}

// GetPointHistory with caching
func (cs *CachedStorage) GetPointHistory(ctx context.Context, domain string, zone, net, node, point int) ([]database.Point, error) {
	key := fmt.Sprintf("%s:points:history:%s:%d:%d:%d:%d", cs.keyGen.Prefix, domain, zone, net, node, point)
	return cachedFetch(cs, key, cs.config.NodeTTL, func() ([]database.Point, error) {
		return cs.Storage.PointOps().GetPointHistory(ctx, domain, zone, net, node, point)
	})
}

// SearchPoints with caching
func (cs *CachedStorage) SearchPoints(ctx context.Context, filter database.PointFilter) ([]database.Point, error) {
	if filter.Limit > cs.config.MaxSearchResults {
		return cs.Storage.PointOps().SearchPoints(ctx, filter)
	}
	key := cs.keyGen.SearchKey(filter) + ":points"
	return cachedFetch(cs, key, cs.config.SearchTTL, func() ([]database.Point, error) {
		return cs.Storage.PointOps().SearchPoints(ctx, filter)
	})
}

// SearchPointsWithLifetime with caching
func (cs *CachedStorage) SearchPointsWithLifetime(ctx context.Context, filter database.PointFilter) ([]PointSummary, error) {
	if filter.Limit > cs.config.MaxSearchResults {
		return cs.Storage.PointOps().SearchPointsWithLifetime(ctx, filter)
	}
	key := cs.keyGen.SearchKey(filter) + ":pointsum"
	return cachedFetch(cs, key, cs.config.SearchTTL, func() ([]PointSummary, error) {
		return cs.Storage.PointOps().SearchPointsWithLifetime(ctx, filter)
	})
}

// GetPointStats with caching
func (cs *CachedStorage) GetPointStats(ctx context.Context, domain string, asOf *time.Time) (*PointStats, error) {
	key := fmt.Sprintf("%s:points:stats:%s:%s", cs.keyGen.Prefix, domain, pointAsOfKey(asOf))
	return cachedFetch(cs, key, cs.config.StatsTTL, func() (*PointStats, error) {
		return cs.Storage.PointOps().GetPointStats(ctx, domain, asOf)
	})
}

// GetPointCountsByNet with caching
func (cs *CachedStorage) GetPointCountsByNet(ctx context.Context, domain string, zone, net int, asOf *time.Time) (map[int]uint64, error) {
	key := fmt.Sprintf("%s:points:netcounts:%s:%d:%d:%s", cs.keyGen.Prefix, domain, zone, net, pointAsOfKey(asOf))
	return cachedFetch(cs, key, cs.config.StatsTTL, func() (map[int]uint64, error) {
		return cs.Storage.PointOps().GetPointCountsByNet(ctx, domain, zone, net, asOf)
	})
}

// GetPointlistDates with caching
func (cs *CachedStorage) GetPointlistDates(ctx context.Context, domain, listSource string) ([]database.PointlistFile, error) {
	key := fmt.Sprintf("%s:points:dates:%s:%s", cs.keyGen.Prefix, domain, listSource)
	return cachedFetch(cs, key, cs.config.StatsTTL, func() ([]database.PointlistFile, error) {
		return cs.Storage.PointOps().GetPointlistDates(ctx, domain, listSource)
	})
}

// GetPointlistSources with caching
func (cs *CachedStorage) GetPointlistSources(ctx context.Context, domain string) ([]PointlistSourceInfo, error) {
	key := fmt.Sprintf("%s:points:sources:%s", cs.keyGen.Prefix, domain)
	return cachedFetch(cs, key, cs.config.StatsTTL, func() ([]PointlistSourceInfo, error) {
		return cs.Storage.PointOps().GetPointlistSources(ctx, domain)
	})
}

// GetPointDomains with caching — resolvePointDomain hits this on every
// point-related API/web request.
func (cs *CachedStorage) GetPointDomains(ctx context.Context, zone, net, node int, point *int) ([]string, error) {
	pointKey := -1
	if point != nil {
		pointKey = *point
	}
	key := fmt.Sprintf("%s:points:domains:%d:%d:%d:%d", cs.keyGen.Prefix, zone, net, node, pointKey)
	return cachedFetch(cs, key, cs.config.NodeTTL, func() ([]string, error) {
		return cs.Storage.PointOps().GetPointDomains(ctx, zone, net, node, point)
	})
}

// latestPointlistDateEntry makes the (date, found) pair JSON-cacheable.
type latestPointlistDateEntry struct {
	Date  time.Time `json:"date"`
	Found bool      `json:"found"`
}

// LatestPointlistDate with caching
func (cs *CachedStorage) LatestPointlistDate(ctx context.Context, domain string) (time.Time, bool, error) {
	key := fmt.Sprintf("%s:points:latest:%s", cs.keyGen.Prefix, domain)
	entry, err := cachedFetch(cs, key, cs.config.StatsTTL, func() (latestPointlistDateEntry, error) {
		date, found, err := cs.Storage.PointOps().LatestPointlistDate(ctx, domain)
		return latestPointlistDateEntry{Date: date, Found: found}, err
	})
	return entry.Date, entry.Found, err
}
