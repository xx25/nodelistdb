package storage

import (
	"fmt"
	"strings"
	"time"

	"nodelistdb/internal/database"
)

// GetV34ModemReport analyzes V.34 modem adoption in FidoNet
func (s *Storage) GetV34ModemReport() (*database.V34ModemReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	// Find first V.34 modem appearance
	firstQuery := `
		SELECT nodelist_date, zone, net, node, system_name, sysop_name, location
		FROM nodes 
		WHERE array_contains(modem_flags, 'V34')
		ORDER BY nodelist_date ASC, zone ASC, net ASC, node ASC 
		LIMIT 1
	`

	var firstDate time.Time
	var firstNode database.NodeInfo
	err := conn.QueryRow(firstQuery).Scan(
		&firstDate, &firstNode.Zone, &firstNode.Net, &firstNode.Node,
		&firstNode.SystemName, &firstNode.SysopName, &firstNode.Location,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find first V.34 modem: %w", err)
	}

	firstNode.Address = fmt.Sprintf("%d:%d/%d", firstNode.Zone, firstNode.Net, firstNode.Node)

	// Get adoption by year
	yearlyQuery := `
		SELECT EXTRACT(YEAR FROM nodelist_date) as year, COUNT(DISTINCT zone||':'||net||'/'||node) as count
		FROM nodes 
		WHERE array_contains(modem_flags, 'V34')
		GROUP BY EXTRACT(YEAR FROM nodelist_date)
		ORDER BY year ASC
	`

	rows, err2 := conn.Query(yearlyQuery)
	if err2 != nil {
		return nil, fmt.Errorf("failed to get V.34 adoption by year: %w", err2)
	}
	defer rows.Close()

	var adoptionByYear []database.YearlyCount
	var chartPoints []database.ChartPoint
	var totalCountV34 int

	for rows.Next() {
		var year, count int
		if err := rows.Scan(&year, &count); err != nil {
			return nil, fmt.Errorf("failed to scan yearly adoption: %w", err)
		}
		adoptionByYear = append(adoptionByYear, database.YearlyCount{Year: year, Count: count})
		chartPoints = append(chartPoints, database.ChartPoint{
			X: year,
			Y: count,
			Label: fmt.Sprintf("%d nodes", count),
		})
		totalCountV34 += count
	}

	// Create chart data for visualization
	chartData := &database.ChartData{
		Type:       "line",
		Title:      "V.34 Modem Adoption Over Time",
		XAxisLabel: "Year",
		YAxisLabel: "Number of Nodes",
		Series: []database.ChartSeries{
			{
				Name: "V.34 Nodes",
				Data: chartPoints,
			},
		},
	}

	return &database.V34ModemReport{
		FirstAppearance: firstDate,
		FirstNode:       firstNode,
		TotalV34Nodes:   totalCountV34,
		AdoptionByYear:  adoptionByYear,
		ChartData:       chartData,
	}, nil
}

// GetBinkpReport analyzes Binkp protocol introduction
func (s *Storage) GetBinkpReport() (*database.BinkpReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	// Find first Binkp appearance
	firstQuery := `
		SELECT nodelist_date, zone, net, node, system_name, sysop_name, location
		FROM nodes 
		WHERE has_binkp = true 
		   OR array_contains(internet_protocols, 'binkp')
		ORDER BY nodelist_date ASC, zone ASC, net ASC, node ASC 
		LIMIT 1
	`

	var firstDate time.Time
	var firstNode database.NodeInfo
	err := conn.QueryRow(firstQuery).Scan(
		&firstDate, &firstNode.Zone, &firstNode.Net, &firstNode.Node,
		&firstNode.SystemName, &firstNode.SysopName, &firstNode.Location,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find first Binkp node: %w", err)
	}

	firstNode.Address = fmt.Sprintf("%d:%d/%d", firstNode.Zone, firstNode.Net, firstNode.Node)

	// Get adoption by year
	yearlyQuery := `
		SELECT EXTRACT(YEAR FROM nodelist_date) as year, COUNT(DISTINCT zone||':'||net||'/'||node) as count
		FROM nodes 
		WHERE has_binkp = true 
		   OR array_contains(internet_protocols, 'binkp')
		GROUP BY EXTRACT(YEAR FROM nodelist_date)
		ORDER BY year ASC
	`

	rows, err := conn.Query(yearlyQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get Binkp adoption by year: %w", err)
	}
	defer rows.Close()

	var adoptionByYear []database.YearlyCount
	var chartPoints []database.ChartPoint
	var totalBinkpNodes int

	for rows.Next() {
		var year, count int
		if err := rows.Scan(&year, &count); err != nil {
			return nil, fmt.Errorf("failed to scan Binkp yearly adoption: %w", err)
		}
		adoptionByYear = append(adoptionByYear, database.YearlyCount{Year: year, Count: count})
		chartPoints = append(chartPoints, database.ChartPoint{
			X: year,
			Y: count,
			Label: fmt.Sprintf("%d nodes", count),
		})
		totalBinkpNodes += count
	}

	// Create chart data
	chartData := &database.ChartData{
		Type:       "line",
		Title:      "Binkp Protocol Adoption Over Time",
		XAxisLabel: "Year",
		YAxisLabel: "Number of Nodes",
		Series: []database.ChartSeries{
			{
				Name: "Binkp Nodes",
				Data: chartPoints,
			},
		},
	}

	return &database.BinkpReport{
		FirstAppearance: firstDate,
		FirstNode:       firstNode,
		TotalBinkpNodes: totalBinkpNodes,
		AdoptionByYear:  adoptionByYear,
		ChartData:       chartData,
	}, nil
}

// GetNetworkLifecycleReport analyzes network creation and deletion
func (s *Storage) GetNetworkLifecycleReport(zone, net int) (*database.NetworkLifecycleReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	// Get network lifecycle data
	lifecycleQuery := `
		SELECT 
			MIN(nodelist_date) as first_seen,
			MAX(nodelist_date) as last_seen,
			MAX(system_name) FILTER (WHERE node = 0 AND node_type = 'Host') as host_name
		FROM nodes 
		WHERE zone = ? AND net = ?
	`

	var firstSeen, lastSeen time.Time
	var hostName *string
	err := conn.QueryRow(lifecycleQuery, zone, net).Scan(&firstSeen, &lastSeen, &hostName)
	if err != nil {
		return nil, fmt.Errorf("failed to get network lifecycle: %w", err)
	}

	// Get node count history
	historyQuery := `
		SELECT nodelist_date, COUNT(*) as node_count
		FROM nodes 
		WHERE zone = ? AND net = ? AND node_type IN ('Node', 'Hub', 'Pvt', 'Hold', 'Down')
		GROUP BY nodelist_date
		ORDER BY nodelist_date ASC
	`

	rows, err := conn.Query(historyQuery, zone, net)
	if err != nil {
		return nil, fmt.Errorf("failed to get network history: %w", err)
	}
	defer rows.Close()

	var nodeHistory []database.NetworkHistory
	var chartPoints []database.ChartPoint
	maxNodes := 0

	for rows.Next() {
		var date time.Time
		var count int
		if err := rows.Scan(&date, &count); err != nil {
			return nil, fmt.Errorf("failed to scan network history: %w", err)
		}
		nodeHistory = append(nodeHistory, database.NetworkHistory{Date: date, NodeCount: count})
		chartPoints = append(chartPoints, database.ChartPoint{
			X: date,
			Y: count,
			Label: fmt.Sprintf("%d nodes", count),
		})
		if count > maxNodes {
			maxNodes = count
		}
	}

	// Check if network is still active (recent activity)
	recentDate := time.Now().AddDate(0, -6, 0) // 6 months ago
	isActive := lastSeen.After(recentDate)

	// Calculate duration
	duration := lastSeen.Sub(firstSeen)
	years := int(duration.Hours() / 24 / 365)
	durationStr := fmt.Sprintf("%d years", years)
	if years == 0 {
		days := int(duration.Hours() / 24)
		durationStr = fmt.Sprintf("%d days", days)
	}

	hostNameStr := ""
	if hostName != nil {
		hostNameStr = *hostName
	}

	// Create chart data
	chartData := &database.ChartData{
		Type:       "area",
		Title:      fmt.Sprintf("Node Count History for %d:%d", zone, net),
		XAxisLabel: "Date",
		YAxisLabel: "Number of Nodes",
		Series: []database.ChartSeries{
			{
				Name: "Active Nodes",
				Data: chartPoints,
			},
		},
	}

	return &database.NetworkLifecycleReport{
		Zone:        zone,
		Net:         net,
		HostName:    hostNameStr,
		FirstSeen:   firstSeen,
		LastSeen:    lastSeen,
		IsActive:    isActive,
		Duration:    durationStr,
		MaxNodes:    maxNodes,
		NodeHistory: nodeHistory,
		ChartData:   chartData,
	}, nil
}

// GetSysopNameReportByYear analyzes most common sysop names by year
func (s *Storage) GetSysopNameReportByYear(year int) (*database.SysopNameReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	// Get top sysop names for the year
	query := `
		SELECT 
			TRIM(UPPER(sysop_name)) as name, 
			COUNT(DISTINCT zone||':'||net||'/'||node) as count
		FROM nodes 
		WHERE EXTRACT(YEAR FROM nodelist_date) = ? 
		  AND sysop_name IS NOT NULL 
		  AND TRIM(sysop_name) != ''
		  AND TRIM(sysop_name) NOT ILIKE 'unknown%'
		  AND TRIM(sysop_name) NOT ILIKE 'n/a%'
		GROUP BY TRIM(UPPER(sysop_name))
		HAVING COUNT(DISTINCT zone||':'||net||'/'||node) > 1
		ORDER BY count DESC
		LIMIT 20
	`

	rows, err := conn.Query(query, year)
	if err != nil {
		return nil, fmt.Errorf("failed to get sysop names for year %d: %w", year, err)
	}
	defer rows.Close()

	var topNames []database.NameCount
	var chartPoints []database.ChartPoint

	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return nil, fmt.Errorf("failed to scan sysop name: %w", err)
		}
		topNames = append(topNames, database.NameCount{Name: name, Count: count})
		chartPoints = append(chartPoints, database.ChartPoint{
			X: name,
			Y: count,
			Label: fmt.Sprintf("%d nodes", count),
		})
	}

	// Get totals
	totalQuery := `
		SELECT 
			COUNT(DISTINCT TRIM(UPPER(sysop_name))) as unique_names,
			COUNT(DISTINCT zone||':'||net||'/'||node) as total_nodes
		FROM nodes 
		WHERE EXTRACT(YEAR FROM nodelist_date) = ? 
		  AND sysop_name IS NOT NULL 
		  AND TRIM(sysop_name) != ''
	`

	var totalUnique, totalNodes int
	err = conn.QueryRow(totalQuery, year).Scan(&totalUnique, &totalNodes)
	if err != nil {
		return nil, fmt.Errorf("failed to get totals for year %d: %w", year, err)
	}

	// Create chart data
	chartData := &database.ChartData{
		Type:       "bar",
		Title:      fmt.Sprintf("Most Common Sysop Names in %d", year),
		XAxisLabel: "Sysop Name",
		YAxisLabel: "Number of Nodes",
		Series: []database.ChartSeries{
			{
				Name: "Frequency",
				Data: chartPoints,
			},
		},
	}

	return &database.SysopNameReport{
		Year:        year,
		TopNames:    topNames,
		TotalUnique: totalUnique,
		TotalNodes:  totalNodes,
		ChartData:   chartData,
	}, nil
}

// GetProtocolAdoptionTrend analyzes protocol adoption over time
func (s *Storage) GetProtocolAdoptionTrend(protocol string) (*database.ProtocolAdoptionReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn := s.db.Conn()

	var whereClause string
	switch strings.ToUpper(protocol) {
	case "V34", "V.34":
		whereClause = "array_contains(modem_flags, 'V34') OR max_speed ILIKE '%V34%' OR max_speed ILIKE '%V.34%'"
	case "BINKP":
		whereClause = "has_binkp = true OR array_contains(internet_protocols, 'binkp')"
	case "TELNET":
		whereClause = "has_telnet = true OR array_contains(internet_protocols, 'telnet')"
	case "ISDN":
		whereClause = "array_contains(modem_flags, 'ISDN') OR max_speed ILIKE '%ISDN%'"
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	query := fmt.Sprintf(`
		SELECT nodelist_date, COUNT(DISTINCT zone||':'||net||'/'||node) as count
		FROM nodes 
		WHERE %s
		GROUP BY nodelist_date
		ORDER BY nodelist_date ASC
	`, whereClause)

	rows, err := conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get protocol adoption trend: %w", err)
	}
	defer rows.Close()

	var timeline []database.TrendPoint
	var chartPoints []database.ChartPoint
	var peakAdoption database.TrendPoint
	var firstSeen time.Time

	for rows.Next() {
		var date time.Time
		var count int
		if err := rows.Scan(&date, &count); err != nil {
			return nil, fmt.Errorf("failed to scan protocol trend: %w", err)
		}

		point := database.TrendPoint{Date: date, Value: count}
		timeline = append(timeline, point)
		chartPoints = append(chartPoints, database.ChartPoint{
			X: date,
			Y: count,
			Label: fmt.Sprintf("%d nodes", count),
		})

		if firstSeen.IsZero() {
			firstSeen = date
		}
		if count > peakAdoption.Value {
			peakAdoption = point
		}
	}

	// Create chart data
	chartData := &database.ChartData{
		Type:       "line",
		Title:      fmt.Sprintf("%s Protocol Adoption Over Time", strings.ToUpper(protocol)),
		XAxisLabel: "Date",
		YAxisLabel: "Number of Nodes",
		Series: []database.ChartSeries{
			{
				Name: fmt.Sprintf("%s Nodes", strings.ToUpper(protocol)),
				Data: chartPoints,
			},
		},
	}

	return &database.ProtocolAdoptionReport{
		Protocol:     strings.ToUpper(protocol),
		Timeline:     timeline,
		PeakAdoption: peakAdoption,
		FirstSeen:    firstSeen,
		ChartData:    chartData,
	}, nil
}