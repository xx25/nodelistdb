package storage

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Software info structure for parsing
type softwareInfo struct {
	Software string
	Version  string
	OS       string
	Protocol string
}

// SoftwareAnalyticsOperations handles software analytics queries
type SoftwareAnalyticsOperations struct {
	db database.DatabaseInterface
	mu sync.RWMutex
}

// NewSoftwareAnalyticsOperations creates a new SoftwareAnalyticsOperations instance
func NewSoftwareAnalyticsOperations(db database.DatabaseInterface) *SoftwareAnalyticsOperations {
	return &SoftwareAnalyticsOperations{
		db: db,
	}
}

// GetBinkPSoftwareDistribution returns BinkP software distribution statistics
func (sao *SoftwareAnalyticsOperations) GetBinkPSoftwareDistribution(days int) (*SoftwareDistribution, error) {
	sao.mu.RLock()
	defer sao.mu.RUnlock()

	// This feature is only available for ClickHouse
	if _, isClickHouse := sao.db.(*database.ClickHouseDB); !isClickHouse {
		return &SoftwareDistribution{
			Protocol:         "BinkP",
			TotalNodes:       0,
			SoftwareTypes:    []SoftwareTypeStats{},
			VersionBreakdown: []SoftwareVersionStats{},
			OSDistribution:   []OSStats{},
			LastUpdated:      time.Now(),
		}, nil
	}

	conn := sao.db.Conn()

	// Get latest test result per node, then count software distribution
	query := `
		SELECT
			binkp_version,
			COUNT(*) as count
		FROM (
			SELECT
				zone, net, node,
				argMax(binkp_version, test_time) as binkp_version
			FROM node_test_results
			WHERE binkp_tested = true
				AND binkp_success = true
				AND test_date >= today() - ?
				AND is_aggregated = true
			GROUP BY zone, net, node
			HAVING binkp_version <> ''
		) AS latest_tests
		GROUP BY binkp_version
		ORDER BY count DESC
	`

	rows, err := conn.Query(query, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query binkp versions: %w", err)
	}
	defer rows.Close()

	dist := &SoftwareDistribution{
		Protocol:         "BinkP",
		SoftwareTypes:    []SoftwareTypeStats{},
		VersionBreakdown: []SoftwareVersionStats{},
		OSDistribution:   []OSStats{},
		LastUpdated:      time.Now(),
	}

	softwareMap := make(map[string]int)
	versionMap := make(map[string]int)
	osMap := make(map[string]int)
	total := 0

	for rows.Next() {
		var version string
		var count int

		if err := rows.Scan(&version, &count); err != nil {
			continue
		}

		// Parse the version string to extract software and OS info
		info := parseBinkPVersion(version)
		if info == nil {
			continue
		}

		total += count

		// Count by software type
		softwareMap[info.Software] += count

		// Count by software + version
		if info.Version != "" {
			versionKey := fmt.Sprintf("%s %s", info.Software, info.Version)
			versionMap[versionKey] += count
		}

		// Count by OS
		if info.OS != "" && info.OS != "Unknown" {
			osMap[info.OS] += count
		}
	}

	dist.TotalNodes = total

	// Convert maps to sorted slices with percentages
	dist.SoftwareTypes = mapToSoftwareTypeStats(softwareMap, total)
	dist.VersionBreakdown = mapToVersionStats(versionMap, total)
	dist.OSDistribution = mapToOSStats(osMap, total)

	return dist, nil
}

// GetIFCICOSoftwareDistribution returns IFCICO software distribution statistics
func (sao *SoftwareAnalyticsOperations) GetIFCICOSoftwareDistribution(days int) (*SoftwareDistribution, error) {
	sao.mu.RLock()
	defer sao.mu.RUnlock()

	// This feature is only available for ClickHouse
	if _, isClickHouse := sao.db.(*database.ClickHouseDB); !isClickHouse {
		return &SoftwareDistribution{
			Protocol:         "IFCICO/EMSI",
			TotalNodes:       0,
			SoftwareTypes:    []SoftwareTypeStats{},
			VersionBreakdown: []SoftwareVersionStats{},
			OSDistribution:   []OSStats{},
			LastUpdated:      time.Now(),
		}, nil
	}

	conn := sao.db.Conn()

	// Get latest test result per node, then count software distribution
	query := `
		SELECT
			ifcico_mailer_info,
			COUNT(*) as count
		FROM (
			SELECT
				zone, net, node,
				argMax(ifcico_mailer_info, test_time) as ifcico_mailer_info
			FROM node_test_results
			WHERE ifcico_tested = true
				AND ifcico_success = true
				AND test_date >= today() - ?
				AND is_aggregated = true
			GROUP BY zone, net, node
			HAVING ifcico_mailer_info <> ''
		) AS latest_tests
		GROUP BY ifcico_mailer_info
		ORDER BY count DESC
	`

	rows, err := conn.Query(query, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query ifcico versions: %w", err)
	}
	defer rows.Close()

	dist := &SoftwareDistribution{
		Protocol:         "IFCICO/EMSI",
		SoftwareTypes:    []SoftwareTypeStats{},
		VersionBreakdown: []SoftwareVersionStats{},
		OSDistribution:   []OSStats{},
		LastUpdated:      time.Now(),
	}

	softwareMap := make(map[string]int)
	versionMap := make(map[string]int)
	total := 0

	for rows.Next() {
		var mailerInfo string
		var count int

		if err := rows.Scan(&mailerInfo, &count); err != nil {
			continue
		}

		info := parseIFCICOMailerInfo(mailerInfo)
		if info == nil {
			continue
		}

		total += count

		// Count by software type
		softwareMap[info.Software] += count

		// Count by software + version
		if info.Version != "" {
			versionKey := fmt.Sprintf("%s %s", info.Software, info.Version)
			versionMap[versionKey] += count
		}
	}

	dist.TotalNodes = total

	// Convert maps to sorted slices
	dist.SoftwareTypes = mapToSoftwareTypeStats(softwareMap, total)
	dist.VersionBreakdown = mapToVersionStats(versionMap, total)
	// IFCICO doesn't have OS info in mailer string
	dist.OSDistribution = []OSStats{}

	return dist, nil
}

// GetBinkdDetailedStats returns detailed binkd statistics
func (sao *SoftwareAnalyticsOperations) GetBinkdDetailedStats(days int) (*SoftwareDistribution, error) {
	sao.mu.RLock()
	defer sao.mu.RUnlock()

	// This feature is only available for ClickHouse
	if _, isClickHouse := sao.db.(*database.ClickHouseDB); !isClickHouse {
		return &SoftwareDistribution{
			Protocol:         "BinkP (binkd only)",
			TotalNodes:       0,
			SoftwareTypes:    []SoftwareTypeStats{},
			VersionBreakdown: []SoftwareVersionStats{},
			OSDistribution:   []OSStats{},
			LastUpdated:      time.Now(),
		}, nil
	}

	conn := sao.db.Conn()

	// Get latest test result per node, then count software distribution
	query := `
		SELECT
			binkp_version,
			COUNT(*) as count
		FROM (
			SELECT
				zone, net, node,
				argMax(binkp_version, test_time) as binkp_version
			FROM node_test_results
			WHERE binkp_tested = true
				AND binkp_success = true
				AND test_date >= today() - ?
				AND is_aggregated = true
			GROUP BY zone, net, node
			HAVING binkp_version LIKE 'binkd/%'
		) AS latest_tests
		GROUP BY binkp_version
		ORDER BY count DESC
	`

	rows, err := conn.Query(query, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query binkd versions: %w", err)
	}
	defer rows.Close()

	dist := &SoftwareDistribution{
		Protocol:         "BinkP (binkd only)",
		SoftwareTypes:    []SoftwareTypeStats{},
		VersionBreakdown: []SoftwareVersionStats{},
		OSDistribution:   []OSStats{},
		LastUpdated:      time.Now(),
	}

	versionMap := make(map[string]int)
	osMap := make(map[string]int)
	total := 0

	for rows.Next() {
		var version string
		var count int

		if err := rows.Scan(&version, &count); err != nil {
			continue
		}

		info := parseBinkPVersion(version)
		if info == nil || info.Software != "binkd" {
			continue
		}

		total += count

		// Count by version
		if info.Version != "" {
			versionMap[info.Version] += count
		}

		// Count by OS
		if info.OS != "" && info.OS != "Unknown" {
			osMap[info.OS] += count
		}
	}

	dist.TotalNodes = total

	// For binkd-only stats, show version distribution
	dist.SoftwareTypes = mapToSoftwareTypeStats(map[string]int{"binkd": total}, total)
	// Convert version map directly without the software prefix
	dist.VersionBreakdown = mapToBinkdVersionStats(versionMap, total)
	dist.OSDistribution = mapToOSStats(osMap, total)

	return dist, nil
}

// Helper functions for parsing and converting

func parseBinkPVersion(version string) *softwareInfo {
	if version == "" {
		return nil
	}

	info := &softwareInfo{}

	// Common patterns for BinkP software
	patterns := []struct {
		regex    *regexp.Regexp
		software string
		groups   []string // ["version", "os", "protocol"]
	}{
		{
			regex:    regexp.MustCompile(`binkd/([0-9.a-zA-Z-]+)/(\w+)\s+binkp/([0-9.]+)`),
			software: "binkd",
			groups:   []string{"version", "os", "protocol"},
		},
		{
			regex:    regexp.MustCompile(`BinkIT/([0-9.]+),JSBinkP/([0-9.]+),sbbs([0-9.a-z]+)/(\w+)\s+binkp/([0-9.]+)`),
			software: "BinkIT/Synchronet",
			groups:   []string{"binkit_ver", "jsbinkp_ver", "sbbs_ver", "os", "protocol"},
		},
		{
			regex:    regexp.MustCompile(`Mystic/([0-9.A-Za-z]+)\s+binkp/([0-9.]+)`),
			software: "Mystic",
			groups:   []string{"version", "protocol"},
		},
		{
			regex:    regexp.MustCompile(`mbcico/([0-9.a-z-]+)/([^/\s]+)\s+binkp/([0-9.]+)`),
			software: "mbcico",
			groups:   []string{"version", "os", "protocol"},
		},
		{
			regex:    regexp.MustCompile(`Argus/([0-9.]+)/\s*binkp/([0-9.]+)`),
			software: "Argus",
			groups:   []string{"version", "protocol"},
		},
		{
			regex:    regexp.MustCompile(`InterMail/([0-9.]+)/\s*binkp/([0-9.]+)`),
			software: "InterMail",
			groups:   []string{"version", "protocol"},
		},
	}

	for _, pattern := range patterns {
		matches := pattern.regex.FindStringSubmatch(version)
		if matches != nil && len(matches) > 1 {
			info.Software = pattern.software

			// Extract values based on group names
			for i, groupName := range pattern.groups {
				if i+1 < len(matches) {
					switch groupName {
					case "version":
						info.Version = matches[i+1]
					case "sbbs_ver":
						info.Version = "sbbs" + matches[i+1]
					case "binkit_ver":
						info.Version = matches[i+1]
					case "os":
						info.OS = normalizeOS(matches[i+1])
					case "protocol":
						info.Protocol = "binkp/" + matches[i+1]
					}
				}
			}

			// Special handling for BinkIT/Synchronet
			if pattern.software == "BinkIT/Synchronet" && len(matches) > 3 {
				info.Version = "sbbs" + matches[3] + "/BinkIT" + matches[1]
			}

			return info
		}
	}

	// If no pattern matches, try to extract basic info
	info.Software = "Unknown"
	if strings.Contains(strings.ToLower(version), "binkp") {
		info.Protocol = "binkp"
	}

	return info
}

func parseIFCICOMailerInfo(mailerInfo string) *softwareInfo {
	if mailerInfo == "" {
		return nil
	}

	info := &softwareInfo{
		Protocol: "IFCICO/EMSI",
	}

	patterns := []struct {
		regex    *regexp.Regexp
		software string
		groups   []string
	}{
		{
			regex:    regexp.MustCompile(`mbcico\s+([0-9.a-z-]+)`),
			software: "mbcico",
			groups:   []string{"version"},
		},
		{
			regex:    regexp.MustCompile(`qico\s+([0-9.a-z]+)`),
			software: "qico",
			groups:   []string{"version"},
		},
		{
			regex:    regexp.MustCompile(`BinkleyTerm-ST\s+([0-9.]+)`),
			software: "BinkleyTerm-ST",
			groups:   []string{"version"},
		},
		{
			regex:    regexp.MustCompile(`Argus\s+([0-9.]+)`),
			software: "Argus",
			groups:   []string{"version"},
		},
	}

	for _, pattern := range patterns {
		matches := pattern.regex.FindStringSubmatch(mailerInfo)
		if matches != nil && len(matches) > 1 {
			info.Software = pattern.software
			if len(pattern.groups) > 0 && len(matches) > 1 {
				info.Version = matches[1]
			}
			return info
		}
	}

	// If no pattern matches, use the whole string as software name
	info.Software = mailerInfo
	return info
}

func normalizeOS(os string) string {
	os = strings.ToLower(os)

	// Normalize common OS names
	switch {
	case strings.Contains(os, "linux"):
		return "Linux"
	case strings.Contains(os, "win32"):
		return "Windows 32-bit"
	case strings.Contains(os, "win64"):
		return "Windows 64-bit"
	case strings.Contains(os, "win"):
		return "Windows"
	case strings.Contains(os, "os2") || strings.Contains(os, "os/2"):
		return "OS/2"
	case strings.Contains(os, "freebsd"):
		return "FreeBSD"
	case strings.Contains(os, "darwin") || strings.Contains(os, "mac"):
		return "macOS"
	default:
		if os != "" {
			caser := cases.Title(language.English)
			return caser.String(strings.ToLower(os))
		}
		return "Unknown"
	}
}

func mapToSoftwareTypeStats(m map[string]int, total int) []SoftwareTypeStats {
	var stats []SoftwareTypeStats
	for software, count := range m {
		percentage := 0.0
		if total > 0 {
			percentage = float64(count) * 100.0 / float64(total)
		}
		stats = append(stats, SoftwareTypeStats{
			Software:   software,
			Count:      count,
			Percentage: percentage,
		})
	}

	// Sort by count descending
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	return stats
}

func mapToVersionStats(m map[string]int, total int) []SoftwareVersionStats {
	var stats []SoftwareVersionStats
	for version, count := range m {
		percentage := 0.0
		if total > 0 {
			percentage = float64(count) * 100.0 / float64(total)
		}

		// Split software and version
		software := version
		ver := ""
		// Try to split on first space
		idx := strings.Index(version, " ")
		if idx > 0 {
			software = version[:idx]
			ver = version[idx+1:]
		}

		stats = append(stats, SoftwareVersionStats{
			Software:   software,
			Version:    ver,
			Count:      count,
			Percentage: percentage,
		})
	}

	// Sort by count descending
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	return stats
}

func mapToOSStats(m map[string]int, total int) []OSStats {
	var stats []OSStats
	for os, count := range m {
		percentage := 0.0
		if total > 0 {
			percentage = float64(count) * 100.0 / float64(total)
		}
		stats = append(stats, OSStats{
			OS:         os,
			Count:      count,
			Percentage: percentage,
		})
	}

	// Sort by count descending
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	return stats
}

func mapToBinkdVersionStats(m map[string]int, total int) []SoftwareVersionStats {
	var stats []SoftwareVersionStats
	for version, count := range m {
		percentage := 0.0
		if total > 0 {
			percentage = float64(count) * 100.0 / float64(total)
		}

		stats = append(stats, SoftwareVersionStats{
			Software:   "binkd",
			Version:    version,
			Count:      count,
			Percentage: percentage,
		})
	}

	// Sort by count descending
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	return stats
}