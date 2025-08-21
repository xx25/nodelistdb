package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/nodelistdb/internal/testing/models"
)

// ClickHouseStorage implements Storage interface for ClickHouse
type ClickHouseStorage struct {
	conn      driver.Conn
	db        *sql.DB
	batchSize int
	
	// Batch accumulator
	resultsBatch []*models.TestResult
	lastFlush    time.Time
	flushInterval time.Duration
}

// NewClickHouseStorage creates a new ClickHouse storage
func NewClickHouseStorage(host string, port int, database, username, password string) (*ClickHouseStorage, error) {
	// Create connection options
	options := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", host, port)},
		Auth: clickhouse.Auth{
			Database: database,
			Username: username,
			Password: password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
	}

	// Create native connection
	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("failed to open ClickHouse connection: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	// Create SQL DB for compatibility
	sqlDB := clickhouse.OpenDB(options)
	
	storage := &ClickHouseStorage{
		conn:          conn,
		db:            sqlDB,
		batchSize:     1000,
		resultsBatch:  make([]*models.TestResult, 0, 1000),
		lastFlush:     time.Now(),
		flushInterval: 30 * time.Second,
	}

	// Initialize schema
	if err := storage.initSchema(context.Background()); err != nil {
		storage.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return storage, nil
}

// initSchema creates tables if they don't exist
func (s *ClickHouseStorage) initSchema(ctx context.Context) error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS node_test_results (
			test_time DateTime,
			test_date Date DEFAULT toDate(test_time),
			zone UInt16,
			net UInt16,
			node UInt16,
			address String,
			
			hostname String,
			resolved_ipv4 Array(String),
			resolved_ipv6 Array(String),
			dns_error String,
			
			country String,
			country_code String,
			city String,
			region String,
			latitude Float32,
			longitude Float32,
			isp String,
			org String,
			asn UInt32,
			
			binkp_tested Bool,
			binkp_success Bool,
			binkp_response_ms UInt32,
			binkp_system_name String,
			binkp_sysop String,
			binkp_location String,
			binkp_version String,
			binkp_addresses Array(String),
			binkp_capabilities Array(String),
			binkp_error String,
			
			ifcico_tested Bool,
			ifcico_success Bool,
			ifcico_response_ms UInt32,
			ifcico_mailer_info String,
			ifcico_system_name String,
			ifcico_addresses Array(String),
			ifcico_response_type String,
			ifcico_error String,
			
			telnet_tested Bool,
			telnet_success Bool,
			telnet_response_ms UInt32,
			telnet_error String,
			
			ftp_tested Bool,
			ftp_success Bool,
			ftp_response_ms UInt32,
			ftp_error String,
			
			vmodem_tested Bool,
			vmodem_success Bool,
			vmodem_response_ms UInt32,
			vmodem_error String,
			
			is_operational Bool,
			has_connectivity_issues Bool,
			address_validated Bool,
			
			INDEX idx_date test_date TYPE minmax GRANULARITY 1,
			INDEX idx_zone_net (zone, net) TYPE minmax GRANULARITY 1,
			INDEX idx_operational is_operational TYPE minmax GRANULARITY 1
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(test_date)
		ORDER BY (test_date, zone, net, node)`,
		
		`CREATE TABLE IF NOT EXISTS node_test_daily_stats (
			date Date,
			total_nodes_tested UInt32,
			nodes_with_binkp UInt32,
			nodes_with_ifcico UInt32,
			nodes_operational UInt32,
			nodes_with_issues UInt32,
			nodes_dns_failed UInt32,
			avg_binkp_response_ms Float32,
			avg_ifcico_response_ms Float32,
			countries Map(String, UInt32),
			isps Map(String, UInt32),
			protocol_stats Map(String, UInt32),
			error_types Map(String, UInt32)
		) ENGINE = SummingMergeTree()
		ORDER BY date`,
		
		`CREATE MATERIALIZED VIEW IF NOT EXISTS node_current_status
		ENGINE = ReplacingMergeTree()
		ORDER BY (zone, net, node)
		AS SELECT
			zone, net, node, address,
			argMax(test_time, test_time) as last_test_time,
			argMax(is_operational, test_time) as is_operational,
			argMax(binkp_success, test_time) as binkp_works,
			argMax(country, test_time) as country,
			argMax(isp, test_time) as isp
		FROM node_test_results
		GROUP BY zone, net, node, address`,
	}

	for _, schema := range schemas {
		if err := s.conn.Exec(ctx, schema); err != nil {
			// Ignore "already exists" errors for views
			if !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("failed to create schema: %w", err)
			}
		}
	}

	return nil
}

// Close closes the database connection
func (s *ClickHouseStorage) Close() error {
	// Flush any pending results
	if len(s.resultsBatch) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.flushBatch(ctx)
	}
	
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			return err
		}
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// GetNodesWithInternet retrieves nodes from ClickHouse nodes table
func (s *ClickHouseStorage) GetNodesWithInternet(ctx context.Context, limit int) ([]*models.Node, error) {
	query := `
		SELECT 
			zone, net, node, system_name, sysop_name, location,
			internet_hostnames, internet_protocols, has_inet
		FROM nodes
		WHERE has_inet = 1
			AND length(internet_protocols) > 0
		ORDER BY zone, net, node
	`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	return s.scanNodes(rows)
}

// GetNodesByZone retrieves nodes from a specific zone
func (s *ClickHouseStorage) GetNodesByZone(ctx context.Context, zone int) ([]*models.Node, error) {
	query := `
		SELECT 
			zone, net, node, system_name, sysop_name, location,
			internet_hostnames, internet_protocols, has_inet
		FROM nodes
		WHERE zone = ? AND has_inet = 1
			AND length(internet_protocols) > 0
		ORDER BY net, node
	`

	rows, err := s.db.QueryContext(ctx, query, zone)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes by zone: %w", err)
	}
	defer rows.Close()

	return s.scanNodes(rows)
}

// GetNodesByProtocol retrieves nodes that support a specific protocol
func (s *ClickHouseStorage) GetNodesByProtocol(ctx context.Context, protocol string, limit int) ([]*models.Node, error) {
	query := `
		SELECT 
			zone, net, node, system_name, sysop_name, location,
			internet_hostnames, internet_protocols, has_inet
		FROM nodes
		WHERE has_inet = 1
			AND has(internet_protocols, ?)
		ORDER BY zone, net, node
	`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.QueryContext(ctx, query, protocol)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes by protocol: %w", err)
	}
	defer rows.Close()

	return s.scanNodes(rows)
}

// GetStatistics returns basic statistics about nodes
func (s *ClickHouseStorage) GetStatistics(ctx context.Context) (map[string]int, error) {
	query := `
		SELECT 
			count(*) as total_nodes,
			countIf(has_inet = 1) as nodes_with_inet,
			countIf(has(internet_protocols, 'IBN')) as nodes_with_binkp,
			countIf(has(internet_protocols, 'IFC')) as nodes_with_ifcico,
			countIf(has(internet_protocols, 'ITN')) as nodes_with_telnet,
			countIf(has(internet_protocols, 'IFT')) as nodes_with_ftp
		FROM nodes
	`

	var stats struct {
		Total      int
		WithInet   int
		WithBinkP  int
		WithIfcico int
		WithTelnet int
		WithFTP    int
	}

	row := s.db.QueryRowContext(ctx, query)
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
func (s *ClickHouseStorage) StoreTestResult(ctx context.Context, result *models.TestResult) error {
	// Add to batch
	s.resultsBatch = append(s.resultsBatch, result)
	
	// Check if we should flush
	if len(s.resultsBatch) >= s.batchSize || time.Since(s.lastFlush) > s.flushInterval {
		return s.flushBatch(ctx)
	}
	
	return nil
}

// StoreTestResults stores multiple test results
func (s *ClickHouseStorage) StoreTestResults(ctx context.Context, results []*models.TestResult) error {
	// Add all to batch
	s.resultsBatch = append(s.resultsBatch, results...)
	
	// Flush if batch is large enough
	if len(s.resultsBatch) >= s.batchSize {
		return s.flushBatch(ctx)
	}
	
	return nil
}

// flushBatch flushes the current batch to ClickHouse
func (s *ClickHouseStorage) flushBatch(ctx context.Context) error {
	if len(s.resultsBatch) == 0 {
		return nil
	}
	
	batch, err := s.conn.PrepareBatch(ctx, `INSERT INTO node_test_results`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}
	
	for _, r := range s.resultsBatch {
		err := batch.Append(s.resultToValues(r)...)
		if err != nil {
			return fmt.Errorf("failed to append to batch: %w", err)
		}
	}
	
	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}
	
	// Clear batch
	s.resultsBatch = s.resultsBatch[:0]
	s.lastFlush = time.Now()
	
	return nil
}

// StoreDailyStats stores daily statistics
func (s *ClickHouseStorage) StoreDailyStats(ctx context.Context, stats *models.TestStatistics) error {
	query := `
		INSERT INTO node_test_daily_stats (
			date, total_nodes_tested, nodes_with_binkp, nodes_with_ifcico,
			nodes_operational, nodes_with_issues, nodes_dns_failed,
			avg_binkp_response_ms, avg_ifcico_response_ms,
			countries, isps, protocol_stats, error_types
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	return s.conn.Exec(ctx, query,
		stats.Date,
		stats.TotalNodesTested,
		stats.NodesWithBinkP,
		stats.NodesWithIfcico,
		stats.NodesOperational,
		stats.NodesWithIssues,
		stats.NodesDNSFailed,
		stats.AvgBinkPResponseMs,
		stats.AvgIfcicoResponseMs,
		stats.Countries,
		stats.ISPs,
		stats.ProtocolStats,
		stats.ErrorTypes,
	)
}

// GetLatestTestResults retrieves the most recent test results
func (s *ClickHouseStorage) GetLatestTestResults(ctx context.Context, limit int) ([]*models.TestResult, error) {
	_ = `
		SELECT * FROM node_test_results
		ORDER BY test_time DESC
		LIMIT ?
	`
	// Implementation would parse results back into TestResult structs
	// Simplified for brevity
	return nil, fmt.Errorf("not fully implemented yet")
}

// GetNodeTestHistory retrieves test history for a specific node
func (s *ClickHouseStorage) GetNodeTestHistory(ctx context.Context, zone, net, node int, days int) ([]*models.TestResult, error) {
	_ = `
		SELECT * FROM node_test_results
		WHERE zone = ? AND net = ? AND node = ?
			AND test_date >= today() - INTERVAL ? DAY
		ORDER BY test_time DESC
	`
	// Implementation would parse results back into TestResult structs
	// Simplified for brevity
	return nil, fmt.Errorf("not fully implemented yet")
}

// Helper methods

func (s *ClickHouseStorage) scanNodes(rows *sql.Rows) ([]*models.Node, error) {
	var nodes []*models.Node
	for rows.Next() {
		node := &models.Node{}
		err := rows.Scan(
			&node.Zone,
			&node.Net,
			&node.Node,
			&node.SystemName,
			&node.SysopName,
			&node.Location,
			&node.InternetHostnames,
			&node.InternetProtocols,
			&node.HasInet,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (s *ClickHouseStorage) resultToValues(r *models.TestResult) []interface{} {
	// Extract protocol test details
	var binkpTested, binkpSuccess bool
	var binkpResponseMs uint32
	var binkpSystemName, binkpSysop, binkpLocation, binkpVersion, binkpError string
	var binkpAddresses, binkpCapabilities []string
	
	if r.BinkPResult != nil {
		binkpTested = r.BinkPResult.Tested
		binkpSuccess = r.BinkPResult.Success
		binkpResponseMs = r.BinkPResult.ResponseMs
		binkpError = r.BinkPResult.Error
		
		// Extract details from map
		if sysName, ok := r.BinkPResult.Details["system_name"].(string); ok {
			binkpSystemName = sysName
		}
		if sysop, ok := r.BinkPResult.Details["sysop"].(string); ok {
			binkpSysop = sysop
		}
		if loc, ok := r.BinkPResult.Details["location"].(string); ok {
			binkpLocation = loc
		}
		if ver, ok := r.BinkPResult.Details["version"].(string); ok {
			binkpVersion = ver
		}
		if addrs, ok := r.BinkPResult.Details["addresses"].([]string); ok {
			binkpAddresses = addrs
		}
		if caps, ok := r.BinkPResult.Details["capabilities"].([]string); ok {
			binkpCapabilities = caps
		}
	}
	
	// Similar extraction for other protocols
	var ifcicoTested, ifcicoSuccess bool
	var ifcicoResponseMs uint32
	var ifcicoMailerInfo, ifcicoSystemName, ifcicoResponseType, ifcicoError string
	var ifcicoAddresses []string
	
	if r.IfcicoResult != nil {
		ifcicoTested = r.IfcicoResult.Tested
		ifcicoSuccess = r.IfcicoResult.Success
		ifcicoResponseMs = r.IfcicoResult.ResponseMs
		ifcicoError = r.IfcicoResult.Error
		if mailer, ok := r.IfcicoResult.Details["mailer_info"].(string); ok {
			ifcicoMailerInfo = mailer
		}
		if sysName, ok := r.IfcicoResult.Details["system_name"].(string); ok {
			ifcicoSystemName = sysName
		}
		if addrs, ok := r.IfcicoResult.Details["addresses"].([]string); ok {
			ifcicoAddresses = addrs
		}
		if respType, ok := r.IfcicoResult.Details["response_type"].(string); ok {
			ifcicoResponseType = respType
		}
	}
	
	// Default values for other protocols
	var telnetTested, telnetSuccess, ftpTested, ftpSuccess, vmodemTested, vmodemSuccess bool
	var telnetResponseMs, ftpResponseMs, vmodemResponseMs uint32
	var telnetError, ftpError, vmodemError string
	
	if r.TelnetResult != nil {
		telnetTested = r.TelnetResult.Tested
		telnetSuccess = r.TelnetResult.Success
		telnetResponseMs = r.TelnetResult.ResponseMs
		telnetError = r.TelnetResult.Error
	}
	
	if r.FTPResult != nil {
		ftpTested = r.FTPResult.Tested
		ftpSuccess = r.FTPResult.Success
		ftpResponseMs = r.FTPResult.ResponseMs
		ftpError = r.FTPResult.Error
	}
	
	if r.VModemResult != nil {
		vmodemTested = r.VModemResult.Tested
		vmodemSuccess = r.VModemResult.Success
		vmodemResponseMs = r.VModemResult.ResponseMs
		vmodemError = r.VModemResult.Error
	}
	
	return []interface{}{
		r.TestTime, r.TestDate, r.Zone, r.Net, r.Node, r.Address,
		r.Hostname, r.ResolvedIPv4, r.ResolvedIPv6, r.DNSError,
		r.Country, r.CountryCode, r.City, r.Region, r.Latitude, r.Longitude, r.ISP, r.Org, r.ASN,
		binkpTested, binkpSuccess, binkpResponseMs, binkpSystemName,
		binkpSysop, binkpLocation, binkpVersion, binkpAddresses,
		binkpCapabilities, binkpError,
		ifcicoTested, ifcicoSuccess, ifcicoResponseMs, ifcicoMailerInfo, 
		ifcicoSystemName, ifcicoAddresses, ifcicoResponseType, ifcicoError,
		telnetTested, telnetSuccess, telnetResponseMs, telnetError,
		ftpTested, ftpSuccess, ftpResponseMs, ftpError,
		vmodemTested, vmodemSuccess, vmodemResponseMs, vmodemError,
		r.IsOperational, r.HasConnectivityIssues, r.AddressValidated,
	}
}