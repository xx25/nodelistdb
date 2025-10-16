package storage

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
)

// GeoAnalyticsOperations handles geographic analytics queries
type GeoAnalyticsOperations struct {
	db database.DatabaseInterface
	mu sync.RWMutex
}

// NewGeoAnalyticsOperations creates a new GeoAnalyticsOperations instance
func NewGeoAnalyticsOperations(db database.DatabaseInterface) *GeoAnalyticsOperations {
	return &GeoAnalyticsOperations{
		db: db,
	}
}

// GetGeoHostingDistribution returns geographic hosting distribution statistics
func (gao *GeoAnalyticsOperations) GetGeoHostingDistribution(days int) (*GeoHostingDistribution, error) {
	gao.mu.RLock()
	defer gao.mu.RUnlock()

	// This feature is only available for ClickHouse
	if _, isClickHouse := gao.db.(*database.ClickHouseDB); !isClickHouse {
		return &GeoHostingDistribution{
			TotalNodes:           0,
			CountryDistribution:  []CountryStats{},
			ProviderDistribution: []ProviderStats{},
			TopCountries:         []CountryStats{},
			TopProviders:         []ProviderStats{},
			LastUpdated:          time.Now(),
		}, nil
	}

	conn := gao.db.Conn()

	// Get country distribution
	countryQuery := `
		SELECT
			country,
			country_code,
			COUNT(*) as count
		FROM (
			SELECT
				zone, net, node,
				argMax(country, test_time) as country,
				argMax(country_code, test_time) as country_code
			FROM node_test_results
			WHERE is_operational = true
				AND test_date >= today() - ?
			GROUP BY zone, net, node
			HAVING country <> ''
		) AS latest_operational_nodes
		GROUP BY country, country_code
		ORDER BY count DESC
	`

	rows, err := conn.Query(countryQuery, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query country distribution: %w", err)
	}
	defer rows.Close()

	countryMap := make(map[string]*CountryStats)
	total := 0

	for rows.Next() {
		var country, countryCode string
		var count int

		if err := rows.Scan(&country, &countryCode, &count); err != nil {
			continue
		}

		total += count
		countryMap[country] = &CountryStats{
			Country:     country,
			CountryCode: countryCode,
			NodeCount:   count,
		}
	}

	// Get provider distribution with countries
	providerQuery := `
		SELECT
			isp,
			org,
			asn,
			groupArray(DISTINCT country_code) as countries,
			COUNT(*) as count
		FROM (
			SELECT
				zone, net, node,
				argMax(isp, test_time) as isp,
				argMax(org, test_time) as org,
				argMax(asn, test_time) as asn,
				argMax(country_code, test_time) as country_code
			FROM node_test_results
			WHERE is_operational = true
				AND test_date >= today() - ?
			GROUP BY zone, net, node
			HAVING isp <> ''
		) AS latest_operational_nodes
		GROUP BY isp, org, asn
		ORDER BY count DESC
	`

	rows2, err := conn.Query(providerQuery, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query provider distribution: %w", err)
	}
	defer rows2.Close()

	providerList := []ProviderStats{}

	for rows2.Next() {
		var isp, org string
		var asn uint32
		var countries []string
		var count int

		if err := rows2.Scan(&isp, &org, &asn, &countries, &count); err != nil {
			continue
		}

		providerList = append(providerList, ProviderStats{
			Provider:     isp,
			Organization: org,
			ASN:          asn,
			NodeCount:    count,
			Countries:    countries,
		})
	}

	// Calculate percentages for countries
	countryDistribution := []CountryStats{}
	for _, stats := range countryMap {
		if total > 0 {
			stats.Percentage = float64(stats.NodeCount) * 100.0 / float64(total)
		}
		countryDistribution = append(countryDistribution, *stats)
	}

	// Sort by count descending
	sort.Slice(countryDistribution, func(i, j int) bool {
		return countryDistribution[i].NodeCount > countryDistribution[j].NodeCount
	})

	// Calculate percentages for providers
	for i := range providerList {
		if total > 0 {
			providerList[i].Percentage = float64(providerList[i].NodeCount) * 100.0 / float64(total)
		}
	}

	// Get top 20 countries
	topCountries := countryDistribution
	if len(topCountries) > 20 {
		topCountries = topCountries[:20]
	}

	// Get top 20 providers
	topProviders := providerList
	if len(topProviders) > 20 {
		topProviders = topProviders[:20]
	}

	return &GeoHostingDistribution{
		TotalNodes:           total,
		CountryDistribution:  countryDistribution,
		ProviderDistribution: providerList,
		TopCountries:         topCountries,
		TopProviders:         topProviders,
		LastUpdated:          time.Now(),
	}, nil
}
