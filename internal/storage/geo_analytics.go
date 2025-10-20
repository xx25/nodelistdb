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

// GetNodesByCountry returns all operational nodes for a specific country
func (gao *GeoAnalyticsOperations) GetNodesByCountry(countryCode string, days int) ([]NodeTestResult, error) {
	gao.mu.RLock()
	defer gao.mu.RUnlock()

	// This feature is only available for ClickHouse
	if _, isClickHouse := gao.db.(*database.ClickHouseDB); !isClickHouse {
		return []NodeTestResult{}, nil
	}

	conn := gao.db.Conn()

	query := `
		WITH latest_nodes AS (
			SELECT
				zone, net, node,
				argMax(system_name, nodelist_date) as system_name,
				argMax(sysop_name, nodelist_date) as sysop_name
			FROM nodes
			GROUP BY zone, net, node
		)
		SELECT
			r.zone, r.net, r.node,
			COALESCE(n.system_name, r.binkp_system_name, r.ifcico_system_name) as binkp_system_name,
			COALESCE(n.sysop_name, r.binkp_sysop) as binkp_sysop,
			r.binkp_location,
			r.country, r.country_code, r.city, r.isp, r.org, r.asn,
			r.resolved_ipv4, r.resolved_ipv6,
			r.binkp_success, r.binkp_ipv6_success,
			r.ifcico_success, r.ifcico_ipv6_success,
			r.telnet_success, r.telnet_ipv6_success
		FROM (
			SELECT
				zone, net, node,
				argMax(binkp_system_name, test_time) as binkp_system_name,
				argMax(ifcico_system_name, test_time) as ifcico_system_name,
				argMax(binkp_sysop, test_time) as binkp_sysop,
				argMax(binkp_location, test_time) as binkp_location,
				argMax(country, test_time) as country,
				argMax(country_code, test_time) as country_code,
				argMax(city, test_time) as city,
				argMax(isp, test_time) as isp,
				argMax(org, test_time) as org,
				argMax(asn, test_time) as asn,
				argMax(resolved_ipv4, test_time) as resolved_ipv4,
				argMax(resolved_ipv6, test_time) as resolved_ipv6,
				argMax(binkp_success, test_time) as binkp_success,
				argMax(binkp_ipv6_success, test_time) as binkp_ipv6_success,
				argMax(ifcico_success, test_time) as ifcico_success,
				argMax(ifcico_ipv6_success, test_time) as ifcico_ipv6_success,
				argMax(telnet_success, test_time) as telnet_success,
				argMax(telnet_ipv6_success, test_time) as telnet_ipv6_success
			FROM node_test_results
			WHERE is_operational = true
				AND test_date >= today() - ?
			GROUP BY zone, net, node
		) AS r
		LEFT JOIN latest_nodes n ON r.zone = n.zone AND r.net = n.net AND r.node = n.node
		WHERE r.country_code = ?
		ORDER BY r.zone, r.net, r.node
	`

	rows, err := conn.Query(query, days, countryCode)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes by country: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	for rows.Next() {
		var result NodeTestResult
		var resolvedIPv4, resolvedIPv6 []string

		err := rows.Scan(
			&result.Zone, &result.Net, &result.Node,
			&result.BinkPSystemName, &result.BinkPSysop, &result.BinkPLocation,
			&result.Country, &result.CountryCode, &result.City,
			&result.ISP, &result.Org, &result.ASN,
			&resolvedIPv4, &resolvedIPv6,
			&result.BinkPSuccess, &result.BinkPIPv6Success,
			&result.IfcicoSuccess, &result.IfcicoIPv6Success,
			&result.TelnetSuccess, &result.TelnetIPv6Success,
		)
		if err != nil {
			continue
		}

		result.ResolvedIPv4 = resolvedIPv4
		result.ResolvedIPv6 = resolvedIPv6
		results = append(results, result)
	}

	return results, nil
}

// GetNodesByProvider returns all operational nodes for a specific provider
func (gao *GeoAnalyticsOperations) GetNodesByProvider(isp string, days int) ([]NodeTestResult, error) {
	gao.mu.RLock()
	defer gao.mu.RUnlock()

	// This feature is only available for ClickHouse
	if _, isClickHouse := gao.db.(*database.ClickHouseDB); !isClickHouse {
		return []NodeTestResult{}, nil
	}

	conn := gao.db.Conn()

	query := `
		WITH latest_nodes AS (
			SELECT
				zone, net, node,
				argMax(system_name, nodelist_date) as system_name,
				argMax(sysop_name, nodelist_date) as sysop_name
			FROM nodes
			GROUP BY zone, net, node
		)
		SELECT
			r.zone, r.net, r.node,
			COALESCE(n.system_name, r.binkp_system_name, r.ifcico_system_name) as binkp_system_name,
			COALESCE(n.sysop_name, r.binkp_sysop) as binkp_sysop,
			r.binkp_location,
			r.country, r.country_code, r.city, r.isp, r.org, r.asn,
			r.resolved_ipv4, r.resolved_ipv6,
			r.binkp_success, r.binkp_ipv6_success,
			r.ifcico_success, r.ifcico_ipv6_success,
			r.telnet_success, r.telnet_ipv6_success
		FROM (
			SELECT
				zone, net, node,
				argMax(binkp_system_name, test_time) as binkp_system_name,
				argMax(ifcico_system_name, test_time) as ifcico_system_name,
				argMax(binkp_sysop, test_time) as binkp_sysop,
				argMax(binkp_location, test_time) as binkp_location,
				argMax(country, test_time) as country,
				argMax(country_code, test_time) as country_code,
				argMax(city, test_time) as city,
				argMax(isp, test_time) as isp,
				argMax(org, test_time) as org,
				argMax(asn, test_time) as asn,
				argMax(resolved_ipv4, test_time) as resolved_ipv4,
				argMax(resolved_ipv6, test_time) as resolved_ipv6,
				argMax(binkp_success, test_time) as binkp_success,
				argMax(binkp_ipv6_success, test_time) as binkp_ipv6_success,
				argMax(ifcico_success, test_time) as ifcico_success,
				argMax(ifcico_ipv6_success, test_time) as ifcico_ipv6_success,
				argMax(telnet_success, test_time) as telnet_success,
				argMax(telnet_ipv6_success, test_time) as telnet_ipv6_success
			FROM node_test_results
			WHERE is_operational = true
				AND test_date >= today() - ?
			GROUP BY zone, net, node
		) AS r
		LEFT JOIN latest_nodes n ON r.zone = n.zone AND r.net = n.net AND r.node = n.node
		WHERE r.isp = ?
		ORDER BY r.zone, r.net, r.node
	`

	rows, err := conn.Query(query, days, isp)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes by provider: %w", err)
	}
	defer rows.Close()

	var results []NodeTestResult
	for rows.Next() {
		var result NodeTestResult
		var resolvedIPv4, resolvedIPv6 []string

		err := rows.Scan(
			&result.Zone, &result.Net, &result.Node,
			&result.BinkPSystemName, &result.BinkPSysop, &result.BinkPLocation,
			&result.Country, &result.CountryCode, &result.City,
			&result.ISP, &result.Org, &result.ASN,
			&resolvedIPv4, &resolvedIPv6,
			&result.BinkPSuccess, &result.BinkPIPv6Success,
			&result.IfcicoSuccess, &result.IfcicoIPv6Success,
			&result.TelnetSuccess, &result.TelnetIPv6Success,
		)
		if err != nil {
			continue
		}

		result.ResolvedIPv4 = resolvedIPv4
		result.ResolvedIPv6 = resolvedIPv6
		results = append(results, result)
	}

	return results, nil
}
