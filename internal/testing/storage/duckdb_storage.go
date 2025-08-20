package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/marcboeker/go-duckdb"
	"github.com/nodelistdb/internal/testing/models"
)

// DuckDBStorage implements Storage interface for DuckDB
type DuckDBStorage struct {
	nodesDB   *sql.DB // Read-only connection to nodes database
	resultsDB *sql.DB // Read-write connection to results database
}

// NewDuckDBStorage creates a new DuckDB storage
func NewDuckDBStorage(nodesPath, resultsPath string) (*DuckDBStorage, error) {
	// Open nodes database in read-only mode
	nodesConnStr := fmt.Sprintf("%s?access_mode=read_only", nodesPath)
	nodesDB, err := sql.Open("duckdb", nodesConnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open nodes DuckDB: %w", err)
	}

	// Test nodes connection
	if err := nodesDB.Ping(); err != nil {
		nodesDB.Close()
		return nil, fmt.Errorf("failed to ping nodes DuckDB: %w", err)
	}

	// Open results database in read-write mode
	resultsDB, err := sql.Open("duckdb", resultsPath)
	if err != nil {
		nodesDB.Close()
		return nil, fmt.Errorf("failed to open results DuckDB: %w", err)
	}

	// Test results connection
	if err := resultsDB.Ping(); err != nil {
		nodesDB.Close()
		resultsDB.Close()
		return nil, fmt.Errorf("failed to ping results DuckDB: %w", err)
	}

	storage := &DuckDBStorage{
		nodesDB:   nodesDB,
		resultsDB: resultsDB,
	}

	// Initialize results schema
	if err := storage.initResultsSchema(); err != nil {
		storage.Close()
		return nil, fmt.Errorf("failed to initialize results schema: %w", err)
	}

	return storage, nil
}

// initResultsSchema creates tables for test results if they don't exist
func (s *DuckDBStorage) initResultsSchema() error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS node_test_results (
			test_time TIMESTAMP,
			test_date DATE,
			zone INTEGER,
			net INTEGER,
			node INTEGER,
			address VARCHAR,
			
			-- DNS Resolution
			hostname VARCHAR,
			resolved_ipv4 VARCHAR[],
			resolved_ipv6 VARCHAR[],
			dns_error VARCHAR,
			
			-- Geolocation
			country VARCHAR,
			country_code VARCHAR,
			city VARCHAR,
			region VARCHAR,
			latitude REAL,
			longitude REAL,
			isp VARCHAR,
			org VARCHAR,
			asn INTEGER,
			
			-- BinkP Test
			binkp_tested BOOLEAN,
			binkp_success BOOLEAN,
			binkp_response_ms INTEGER,
			binkp_system_name VARCHAR,
			binkp_sysop VARCHAR,
			binkp_location VARCHAR,
			binkp_version VARCHAR,
			binkp_addresses VARCHAR[],
			binkp_capabilities VARCHAR[],
			binkp_error VARCHAR,
			
			-- IFCICO Test
			ifcico_tested BOOLEAN,
			ifcico_success BOOLEAN,
			ifcico_response_ms INTEGER,
			ifcico_mailer_info VARCHAR,
			ifcico_error VARCHAR,
			
			-- Telnet Test
			telnet_tested BOOLEAN,
			telnet_success BOOLEAN,
			telnet_response_ms INTEGER,
			telnet_error VARCHAR,
			
			-- FTP Test
			ftp_tested BOOLEAN,
			ftp_success BOOLEAN,
			ftp_response_ms INTEGER,
			ftp_error VARCHAR,
			
			-- VModem Test
			vmodem_tested BOOLEAN,
			vmodem_success BOOLEAN,
			vmodem_response_ms INTEGER,
			vmodem_error VARCHAR,
			
			-- Summary flags
			is_operational BOOLEAN,
			has_connectivity_issues BOOLEAN,
			address_validated BOOLEAN
		)`,
		
		`CREATE TABLE IF NOT EXISTS node_test_daily_stats (
			date DATE PRIMARY KEY,
			total_nodes_tested INTEGER,
			nodes_with_binkp INTEGER,
			nodes_with_ifcico INTEGER,
			nodes_operational INTEGER,
			nodes_with_issues INTEGER,
			nodes_dns_failed INTEGER,
			avg_binkp_response_ms REAL,
			avg_ifcico_response_ms REAL
		)`,
		
		// Create indexes
		`CREATE INDEX IF NOT EXISTS idx_test_date ON node_test_results(test_date)`,
		`CREATE INDEX IF NOT EXISTS idx_zone_net ON node_test_results(zone, net, node)`,
		`CREATE INDEX IF NOT EXISTS idx_operational ON node_test_results(is_operational)`,
	}

	for _, schema := range schemas {
		if _, err := s.resultsDB.Exec(schema); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}
	}

	return nil
}

// Close closes both database connections
func (s *DuckDBStorage) Close() error {
	var errs []error
	if s.nodesDB != nil {
		if err := s.nodesDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close nodes DB: %w", err))
		}
	}
	if s.resultsDB != nil {
		if err := s.resultsDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close results DB: %w", err))
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// GetNodesWithInternet retrieves all nodes with internet connectivity
func (s *DuckDBStorage) GetNodesWithInternet(ctx context.Context, limit int) ([]*models.Node, error) {
	query := `
		SELECT DISTINCT
			zone, 
			net, 
			node,
			system_name,
			sysop_name,
			location,
			internet_hostnames,
			internet_protocols,
			has_inet
		FROM nodes
		WHERE has_inet = true
			AND array_length(internet_protocols) > 0
			AND nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		ORDER BY zone, net, node
	`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	return s.queryNodes(ctx, query)
}

// GetNodesByZone retrieves nodes from a specific zone
func (s *DuckDBStorage) GetNodesByZone(ctx context.Context, zone int) ([]*models.Node, error) {
	query := `
		SELECT DISTINCT
			zone, 
			net, 
			node,
			system_name,
			sysop_name,
			location,
			internet_hostnames,
			internet_protocols,
			has_inet
		FROM nodes
		WHERE zone = ?
			AND has_inet = true
			AND array_length(internet_protocols) > 0
			AND nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		ORDER BY net, node
	`

	return s.queryNodesWithArgs(ctx, query, zone)
}

// GetNodesByProtocol retrieves nodes that support a specific protocol
func (s *DuckDBStorage) GetNodesByProtocol(ctx context.Context, protocol string, limit int) ([]*models.Node, error) {
	query := `
		SELECT DISTINCT
			zone, 
			net, 
			node,
			system_name,
			sysop_name,
			location,
			internet_hostnames,
			internet_protocols,
			has_inet
		FROM nodes
		WHERE has_inet = true
			AND list_contains(internet_protocols, ?)
			AND nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		ORDER BY zone, net, node
	`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	return s.queryNodesWithArgs(ctx, query, protocol)
}

// GetStatistics returns basic statistics about nodes
func (s *DuckDBStorage) GetStatistics(ctx context.Context) (map[string]int, error) {
	query := `
		SELECT 
			COUNT(*) as total_nodes,
			COUNT(CASE WHEN has_inet THEN 1 END) as nodes_with_inet,
			COUNT(CASE WHEN list_contains(internet_protocols, 'IBN') THEN 1 END) as nodes_with_binkp,
			COUNT(CASE WHEN list_contains(internet_protocols, 'IFC') THEN 1 END) as nodes_with_ifcico,
			COUNT(CASE WHEN list_contains(internet_protocols, 'ITN') THEN 1 END) as nodes_with_telnet,
			COUNT(CASE WHEN list_contains(internet_protocols, 'IFT') THEN 1 END) as nodes_with_ftp
		FROM nodes
		WHERE nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
	`

	var stats struct {
		Total      int
		WithInet   int
		WithBinkP  int
		WithIfcico int
		WithTelnet int
		WithFTP    int
	}

	row := s.nodesDB.QueryRowContext(ctx, query)
	err := row.Scan(
		&stats.Total,
		&stats.WithInet,
		&stats.WithBinkP,
		&stats.WithIfcico,
		&stats.WithTelnet,
		&stats.WithFTP,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get statistics: %w", err)
	}

	return map[string]int{
		"total_nodes":       stats.Total,
		"nodes_with_inet":   stats.WithInet,
		"nodes_with_binkp":  stats.WithBinkP,
		"nodes_with_ifcico": stats.WithIfcico,
		"nodes_with_telnet": stats.WithTelnet,
		"nodes_with_ftp":    stats.WithFTP,
	}, nil
}

// StoreTestResult stores a single test result
func (s *DuckDBStorage) StoreTestResult(ctx context.Context, result *models.TestResult) error {
	return s.StoreTestResults(ctx, []*models.TestResult{result})
}

// StoreTestResults stores multiple test results
func (s *DuckDBStorage) StoreTestResults(ctx context.Context, results []*models.TestResult) error {
	if len(results) == 0 {
		return nil
	}

	tx, err := s.resultsDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO node_test_results (
			test_time, test_date, zone, net, node, address,
			hostname, resolved_ipv4, resolved_ipv6, dns_error,
			country, country_code, city, region, latitude, longitude, isp, org, asn,
			binkp_tested, binkp_success, binkp_response_ms, binkp_system_name,
			binkp_sysop, binkp_location, binkp_version, binkp_addresses,
			binkp_capabilities, binkp_error,
			ifcico_tested, ifcico_success, ifcico_response_ms, ifcico_mailer_info, ifcico_error,
			telnet_tested, telnet_success, telnet_response_ms, telnet_error,
			ftp_tested, ftp_success, ftp_response_ms, ftp_error,
			vmodem_tested, vmodem_success, vmodem_response_ms, vmodem_error,
			is_operational, has_connectivity_issues, address_validated
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
				  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
				  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, r := range results {
		args := s.resultToArgs(r)
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("failed to insert result: %w", err)
		}
	}

	return tx.Commit()
}

// StoreDailyStats stores daily statistics
func (s *DuckDBStorage) StoreDailyStats(ctx context.Context, stats *models.TestStatistics) error {
	query := `
		INSERT OR REPLACE INTO node_test_daily_stats (
			date, total_nodes_tested, nodes_with_binkp, nodes_with_ifcico,
			nodes_operational, nodes_with_issues, nodes_dns_failed,
			avg_binkp_response_ms, avg_ifcico_response_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.resultsDB.ExecContext(ctx, query,
		stats.Date,
		stats.TotalNodesTested,
		stats.NodesWithBinkP,
		stats.NodesWithIfcico,
		stats.NodesOperational,
		stats.NodesWithIssues,
		stats.NodesDNSFailed,
		stats.AvgBinkPResponseMs,
		stats.AvgIfcicoResponseMs,
	)

	return err
}

// GetLatestTestResults retrieves the most recent test results
func (s *DuckDBStorage) GetLatestTestResults(ctx context.Context, limit int) ([]*models.TestResult, error) {
	_ = `
		SELECT * FROM node_test_results
		ORDER BY test_time DESC
		LIMIT ?
	`
	// Implementation would parse results back into TestResult structs
	// Simplified for brevity
	return nil, fmt.Errorf("not implemented yet")
}

// GetNodeTestHistory retrieves test history for a specific node
func (s *DuckDBStorage) GetNodeTestHistory(ctx context.Context, zone, net, node int, days int) ([]*models.TestResult, error) {
	_ = `
		SELECT * FROM node_test_results
		WHERE zone = ? AND net = ? AND node = ?
			AND test_date >= CURRENT_DATE - INTERVAL ? DAY
		ORDER BY test_time DESC
	`
	// Implementation would parse results back into TestResult structs
	// Simplified for brevity
	return nil, fmt.Errorf("not implemented yet")
}

// Helper methods

func (s *DuckDBStorage) queryNodes(ctx context.Context, query string) ([]*models.Node, error) {
	rows, err := s.nodesDB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	return s.scanNodes(rows)
}

func (s *DuckDBStorage) queryNodesWithArgs(ctx context.Context, query string, args ...interface{}) ([]*models.Node, error) {
	rows, err := s.nodesDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	return s.scanNodes(rows)
}

func (s *DuckDBStorage) scanNodes(rows *sql.Rows) ([]*models.Node, error) {
	var nodes []*models.Node
	for rows.Next() {
		node := &models.Node{}
		var hostnames, protocols sql.NullString

		err := rows.Scan(
			&node.Zone,
			&node.Net,
			&node.Node,
			&node.SystemName,
			&node.SysopName,
			&node.Location,
			&hostnames,
			&protocols,
			&node.HasInet,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Parse arrays (DuckDB returns as strings like ['host1', 'host2'])
		node.InternetHostnames = parseArray(hostnames.String)
		node.InternetProtocols = parseArray(protocols.String)

		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return nodes, nil
}

func (s *DuckDBStorage) resultToArgs(r *models.TestResult) []interface{} {
	// Extract BinkP details
	var binkpTested, binkpSuccess bool
	var binkpResponseMs uint32
	var binkpSystemName, binkpSysop, binkpLocation, binkpVersion, binkpError string
	var binkpAddresses, binkpCapabilities []string
	
	if r.BinkPResult != nil {
		binkpTested = r.BinkPResult.Tested
		binkpSuccess = r.BinkPResult.Success
		binkpResponseMs = r.BinkPResult.ResponseMs
		binkpError = r.BinkPResult.Error
		
		if details, ok := r.BinkPResult.Details["system_name"].(string); ok {
			binkpSystemName = details
		}
		if details, ok := r.BinkPResult.Details["sysop"].(string); ok {
			binkpSysop = details
		}
		if details, ok := r.BinkPResult.Details["location"].(string); ok {
			binkpLocation = details
		}
		if details, ok := r.BinkPResult.Details["version"].(string); ok {
			binkpVersion = details
		}
		if details, ok := r.BinkPResult.Details["addresses"].([]string); ok {
			binkpAddresses = details
		}
		if details, ok := r.BinkPResult.Details["capabilities"].([]string); ok {
			binkpCapabilities = details
		}
	}

	// Similar extraction for other protocols (simplified)
	var ifcicoTested, ifcicoSuccess bool
	var ifcicoResponseMs uint32
	var ifcicoMailerInfo, ifcicoError string
	
	if r.IfcicoResult != nil {
		ifcicoTested = r.IfcicoResult.Tested
		ifcicoSuccess = r.IfcicoResult.Success
		ifcicoResponseMs = r.IfcicoResult.ResponseMs
		ifcicoError = r.IfcicoResult.Error
		if details, ok := r.IfcicoResult.Details["mailer_info"].(string); ok {
			ifcicoMailerInfo = details
		}
	}

	// Default values for other protocols
	var telnetTested, telnetSuccess, ftpTested, ftpSuccess, vmodemTested, vmodemSuccess bool
	var telnetResponseMs, ftpResponseMs, vmodemResponseMs uint32
	var telnetError, ftpError, vmodemError string

	return []interface{}{
		r.TestTime, r.TestDate, r.Zone, r.Net, r.Node, r.Address,
		r.Hostname, r.ResolvedIPv4, r.ResolvedIPv6, r.DNSError,
		r.Country, r.CountryCode, r.City, r.Region, r.Latitude, r.Longitude, r.ISP, r.Org, r.ASN,
		binkpTested, binkpSuccess, binkpResponseMs, binkpSystemName,
		binkpSysop, binkpLocation, binkpVersion, binkpAddresses,
		binkpCapabilities, binkpError,
		ifcicoTested, ifcicoSuccess, ifcicoResponseMs, ifcicoMailerInfo, ifcicoError,
		telnetTested, telnetSuccess, telnetResponseMs, telnetError,
		ftpTested, ftpSuccess, ftpResponseMs, ftpError,
		vmodemTested, vmodemSuccess, vmodemResponseMs, vmodemError,
		r.IsOperational, r.HasConnectivityIssues, r.AddressValidated,
	}
}

