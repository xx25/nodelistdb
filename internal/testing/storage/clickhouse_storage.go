package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/nodelistdb/internal/testing/models"
)

// ClickHouseConfig holds ClickHouse connection configuration
type ClickHouseConfig struct {
	MaxOpenConns  int
	MaxIdleConns  int
	DialTimeout   time.Duration
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	Compression   string
	BatchSize     int
	FlushInterval time.Duration
}

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
	return NewClickHouseStorageWithConfig(host, port, database, username, password, nil)
}

// NewClickHouseStorageWithConfig creates a new ClickHouse storage with custom config
func NewClickHouseStorageWithConfig(host string, port int, database, username, password string, cfg *ClickHouseConfig) (*ClickHouseStorage, error) {
	// Set defaults
	maxOpenConns := 10
	maxIdleConns := 5
	dialTimeout := 10 * time.Second
	readTimeout := 5 * time.Minute
	compression := clickhouse.CompressionLZ4
	batchSize := 1000
	flushInterval := 30 * time.Second
	
	// Override with config if provided
	if cfg != nil {
		if cfg.MaxOpenConns > 0 {
			maxOpenConns = cfg.MaxOpenConns
		}
		if cfg.MaxIdleConns > 0 {
			maxIdleConns = cfg.MaxIdleConns
		}
		if cfg.DialTimeout > 0 {
			dialTimeout = cfg.DialTimeout
		}
		if cfg.ReadTimeout > 0 {
			readTimeout = cfg.ReadTimeout
		}
		if cfg.BatchSize > 0 {
			batchSize = cfg.BatchSize
		}
		if cfg.FlushInterval > 0 {
			flushInterval = cfg.FlushInterval
		}
		// Handle compression
		switch strings.ToLower(cfg.Compression) {
		case "lz4":
			compression = clickhouse.CompressionLZ4
		case "zstd":
			compression = clickhouse.CompressionZSTD
		case "gzip":
			compression = clickhouse.CompressionGZIP
		case "none", "":
			compression = clickhouse.CompressionNone
		}
	}

	// Create connection options
	// IMPORTANT: Do NOT set MaxOpenConns/MaxIdleConns in Options due to driver bug
	// They must be set on the sql.DB after OpenDB
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
			Method: compression,
		},
		DialTimeout: dialTimeout,
		ReadTimeout: readTimeout,
		// DO NOT SET MaxOpenConns/MaxIdleConns here - driver bug!
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
	
	// CRITICAL: Set pool settings AFTER OpenDB due to driver bug
	// If MaxOpenConns/MaxIdleConns are in Options, the driver fails with "invalid settings"
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)
	
	// Note: We don't ping the SQL DB here as it can cause issues.
	// The native connection ping above is sufficient.
	
	storage := &ClickHouseStorage{
		conn:          conn,
		db:            sqlDB,
		batchSize:     batchSize,
		resultsBatch:  make([]*models.TestResult, 0, batchSize),
		lastFlush:     time.Now(),
		flushInterval: flushInterval,
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
			if(JSONExtractString(toString(internet_config), 'protocols', 'IBN', 'address') != '',
				JSONExtractString(toString(internet_config), 'protocols', 'IBN', 'address'),
				if(JSONExtractString(toString(internet_config), 'protocols', 'IFC', 'address') != '',
					JSONExtractString(toString(internet_config), 'protocols', 'IFC', 'address'),
					if(JSONExtractString(toString(internet_config), 'protocols', 'ITN', 'address') != '',
						JSONExtractString(toString(internet_config), 'protocols', 'ITN', 'address'),
						if(JSONExtractString(toString(internet_config), 'protocols', 'IVM', 'address') != '',
							JSONExtractString(toString(internet_config), 'protocols', 'IVM', 'address'),
							if(JSONExtractString(toString(internet_config), 'protocols', 'IFT', 'address') != '',
								JSONExtractString(toString(internet_config), 'protocols', 'IFT', 'address'),
								JSONExtractString(toString(internet_config), 'defaults', 'INA')
							)
						)
					)
				)
			) as internet_hostnames,
			arrayStringConcat(JSONExtractKeys(toString(internet_config), 'protocols'), ',') as internet_protocols,
			has_inet,
			toString(internet_config) as config_json
		FROM nodes
		WHERE has_inet = true
			AND JSONLength(toString(internet_config), 'protocols') > 0
			AND nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		ORDER BY zone, net, node
	`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	// Use native connection to avoid SQL DB issues
	rows, err := s.conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	return s.scanNodesNative(rows)
}

// GetNodesByZone retrieves nodes from a specific zone
func (s *ClickHouseStorage) GetNodesByZone(ctx context.Context, zone int) ([]*models.Node, error) {
	query := `
		SELECT
			zone, net, node, system_name, sysop_name, location,
			if(JSONExtractString(toString(internet_config), 'protocols', 'IBN', 'address') != '',
				JSONExtractString(toString(internet_config), 'protocols', 'IBN', 'address'),
				if(JSONExtractString(toString(internet_config), 'protocols', 'IFC', 'address') != '',
					JSONExtractString(toString(internet_config), 'protocols', 'IFC', 'address'),
					if(JSONExtractString(toString(internet_config), 'protocols', 'ITN', 'address') != '',
						JSONExtractString(toString(internet_config), 'protocols', 'ITN', 'address'),
						if(JSONExtractString(toString(internet_config), 'protocols', 'IVM', 'address') != '',
							JSONExtractString(toString(internet_config), 'protocols', 'IVM', 'address'),
							if(JSONExtractString(toString(internet_config), 'protocols', 'IFT', 'address') != '',
								JSONExtractString(toString(internet_config), 'protocols', 'IFT', 'address'),
								JSONExtractString(toString(internet_config), 'defaults', 'INA')
							)
						)
					)
				)
			) as internet_hostnames,
			arrayStringConcat(JSONExtractKeys(toString(internet_config), 'protocols'), ',') as internet_protocols,
			has_inet,
			toString(internet_config) as config_json
		FROM nodes
		WHERE zone = ? AND has_inet = true
			AND JSONLength(toString(internet_config), 'protocols') > 0
			AND nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		ORDER BY net, node
	`

	// Use native connection with positional parameters
	rows, err := s.conn.Query(ctx, query, zone)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes by zone: %w", err)
	}
	defer rows.Close()

	return s.scanNodesNative(rows)
}

// GetNodesByProtocol retrieves nodes that support a specific protocol
func (s *ClickHouseStorage) GetNodesByProtocol(ctx context.Context, protocol string, limit int) ([]*models.Node, error) {
	query := `
		SELECT
			zone, net, node, system_name, sysop_name, location,
			if(JSONExtractString(toString(internet_config), 'protocols', 'IBN', 'address') != '',
				JSONExtractString(toString(internet_config), 'protocols', 'IBN', 'address'),
				if(JSONExtractString(toString(internet_config), 'protocols', 'IFC', 'address') != '',
					JSONExtractString(toString(internet_config), 'protocols', 'IFC', 'address'),
					if(JSONExtractString(toString(internet_config), 'protocols', 'ITN', 'address') != '',
						JSONExtractString(toString(internet_config), 'protocols', 'ITN', 'address'),
						if(JSONExtractString(toString(internet_config), 'protocols', 'IVM', 'address') != '',
							JSONExtractString(toString(internet_config), 'protocols', 'IVM', 'address'),
							if(JSONExtractString(toString(internet_config), 'protocols', 'IFT', 'address') != '',
								JSONExtractString(toString(internet_config), 'protocols', 'IFT', 'address'),
								JSONExtractString(toString(internet_config), 'defaults', 'INA')
							)
						)
					)
				)
			) as internet_hostnames,
			arrayStringConcat(JSONExtractKeys(toString(internet_config), 'protocols'), ',') as internet_protocols,
			has_inet,
			toString(internet_config) as config_json
		FROM nodes
		WHERE has_inet = true
			AND JSONHas(toString(internet_config), 'protocols', ?)
			AND nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		ORDER BY zone, net, node
	`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	// Use native connection with positional parameters
	rows, err := s.conn.Query(ctx, query, protocol)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes by protocol: %w", err)
	}
	defer rows.Close()

	return s.scanNodesNative(rows)
}

// GetLatestNodelistDate returns the most recent nodelist date in the database
func (s *ClickHouseStorage) GetLatestNodelistDate(ctx context.Context) (time.Time, error) {
	query := `SELECT MAX(nodelist_date) FROM nodes`
	
	var maxDate time.Time
	row := s.conn.QueryRow(ctx, query)
	err := row.Scan(&maxDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get latest nodelist date: %w", err)
	}
	
	return maxDate, nil
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
	
	// Always flush to ensure data is persisted
	// This is called after each batch from the daemon, so we want to persist immediately
	return s.flushBatch(ctx)
}

// flushBatch flushes the current batch to ClickHouse
func (s *ClickHouseStorage) flushBatch(ctx context.Context) error {
	if len(s.resultsBatch) == 0 {
		return nil
	}
	
	batch, err := s.conn.PrepareBatch(ctx, `INSERT INTO node_test_results (
		test_time, test_date, zone, net, node, address,
		hostname, resolved_ipv4, resolved_ipv6, dns_error,
		country, country_code, city, region, latitude, longitude, isp, org, asn,
		binkp_tested, binkp_success, binkp_response_ms,
		binkp_system_name, binkp_sysop, binkp_location, binkp_version,
		binkp_addresses, binkp_capabilities, binkp_error,
		ifcico_tested, ifcico_success, ifcico_response_ms,
		ifcico_mailer_info, ifcico_system_name, ifcico_addresses,
		ifcico_response_type, ifcico_error,
		telnet_tested, telnet_success, telnet_response_ms, telnet_error,
		ftp_tested, ftp_success, ftp_response_ms, ftp_error,
		vmodem_tested, vmodem_success, vmodem_response_ms, vmodem_error,
		is_operational, has_connectivity_issues, address_validated,
		binkp_ipv4_tested, binkp_ipv4_success, binkp_ipv4_response_ms, binkp_ipv4_address, binkp_ipv4_error,
		binkp_ipv6_tested, binkp_ipv6_success, binkp_ipv6_response_ms, binkp_ipv6_address, binkp_ipv6_error,
		ifcico_ipv4_tested, ifcico_ipv4_success, ifcico_ipv4_response_ms, ifcico_ipv4_address, ifcico_ipv4_error,
		ifcico_ipv6_tested, ifcico_ipv6_success, ifcico_ipv6_response_ms, ifcico_ipv6_address, ifcico_ipv6_error,
		telnet_ipv4_tested, telnet_ipv4_success, telnet_ipv4_response_ms, telnet_ipv4_address, telnet_ipv4_error,
		telnet_ipv6_tested, telnet_ipv6_success, telnet_ipv6_response_ms, telnet_ipv6_address, telnet_ipv6_error
	)`)
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
	// Use batch insert for proper Map type handling
	batch, err := s.conn.PrepareBatch(ctx, `INSERT INTO node_test_daily_stats`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	// Convert nil maps to empty maps to avoid ClickHouse parsing errors
	countries := stats.Countries
	if countries == nil {
		countries = make(map[string]uint32)
	}
	
	isps := stats.ISPs
	if isps == nil {
		isps = make(map[string]uint32)
	}
	
	protocolStats := stats.ProtocolStats
	if protocolStats == nil {
		protocolStats = make(map[string]uint32)
	}
	
	errorTypes := stats.ErrorTypes
	if errorTypes == nil {
		errorTypes = make(map[string]uint32)
	}

	// Append the row to batch
	err = batch.Append(
		stats.Date,
		stats.TotalNodesTested,
		stats.NodesWithBinkP,
		stats.NodesWithIfcico,
		stats.NodesOperational,
		stats.NodesWithIssues,
		stats.NodesDNSFailed,
		stats.AvgBinkPResponseMs,
		stats.AvgIfcicoResponseMs,
		countries,
		isps,
		protocolStats,
		errorTypes,
	)
	if err != nil {
		return fmt.Errorf("failed to append to batch: %w", err)
	}

	// Send the batch
	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	return nil
}

// GetLatestTestResults retrieves the most recent test results
func (s *ClickHouseStorage) GetLatestTestResults(ctx context.Context, limit int) ([]*models.TestResult, error) {
	query := `
		SELECT 
			test_time, test_date,
			zone, net, node, address,
			hostname, resolved_ipv4, resolved_ipv6, dns_error,
			country, country_code, city, region, 
			latitude, longitude, isp, org, asn,
			binkp_tested, binkp_success, binkp_response_ms,
			binkp_system_name, binkp_sysop, binkp_location, 
			binkp_version, binkp_addresses, binkp_capabilities, binkp_error,
			ifcico_tested, ifcico_success, ifcico_response_ms,
			ifcico_mailer_info, ifcico_system_name, ifcico_addresses, 
			ifcico_response_type, ifcico_error,
			telnet_tested, telnet_success, telnet_response_ms, telnet_error,
			ftp_tested, ftp_success, ftp_response_ms, ftp_error,
			vmodem_tested, vmodem_success, vmodem_response_ms, vmodem_error,
			is_operational, has_connectivity_issues, address_validated
		FROM node_test_results
		ORDER BY test_time DESC
		LIMIT ?
	`
	
	rows, err := s.conn.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest test results: %w", err)
	}
	defer rows.Close()
	
	var results []*models.TestResult
	for rows.Next() {
		r := &models.TestResult{}
		var testDate time.Time
		var zone, net, node uint16  // ClickHouse uses UInt16 for these columns
		var dnsError, binkpError, ifcicoError, telnetError, ftpError, vmodemError string
		var binkpTested, binkpSuccess, ifcicoTested, ifcicoSuccess bool
		var telnetTested, telnetSuccess, ftpTested, ftpSuccess bool 
		var vmodemTested, vmodemSuccess bool
		var binkpResponseMs, ifcicoResponseMs, telnetResponseMs uint32
		var ftpResponseMs, vmodemResponseMs uint32
		var binkpSystemName, binkpSysop, binkpLocation, binkpVersion string
		var binkpAddresses, binkpCapabilities []string
		var ifcicoMailerInfo, ifcicoSystemName, ifcicoResponseType string
		var ifcicoAddresses []string
		
		err := rows.Scan(
			&r.TestTime, &testDate,
			&zone, &net, &node, &r.Address,
			&r.Hostname, &r.ResolvedIPv4, &r.ResolvedIPv6, &dnsError,
			&r.Country, &r.CountryCode, &r.City, &r.Region,
			&r.Latitude, &r.Longitude, &r.ISP, &r.Org, &r.ASN,
			&binkpTested, &binkpSuccess, &binkpResponseMs,
			&binkpSystemName, &binkpSysop, &binkpLocation,
			&binkpVersion, &binkpAddresses, &binkpCapabilities, &binkpError,
			&ifcicoTested, &ifcicoSuccess, &ifcicoResponseMs,
			&ifcicoMailerInfo, &ifcicoSystemName, &ifcicoAddresses,
			&ifcicoResponseType, &ifcicoError,
			&telnetTested, &telnetSuccess, &telnetResponseMs, &telnetError,
			&ftpTested, &ftpSuccess, &ftpResponseMs, &ftpError,
			&vmodemTested, &vmodemSuccess, &vmodemResponseMs, &vmodemError,
			&r.IsOperational, &r.HasConnectivityIssues, &r.AddressValidated,
		)
		if err != nil {
			// Log error but continue to process remaining rows
			fmt.Printf("Warning: failed to scan row in GetLatestTestResults: %v\n", err)
			continue
		}
		
		// Convert UInt16 to int
		r.Zone = int(zone)
		r.Net = int(net)
		r.Node = int(node)
		r.TestDate = testDate
		r.DNSError = dnsError
		
		// Populate BinkP result if tested
		if binkpTested {
			r.BinkPResult = &models.ProtocolTestResult{
				Tested:     true,
				Success:    binkpSuccess,
				ResponseMs: binkpResponseMs,
				Error:      binkpError,
				Details:    make(map[string]interface{}),
			}
			if binkpSystemName != "" || binkpSysop != "" {
				r.BinkPResult.Details["system_name"] = binkpSystemName
				r.BinkPResult.Details["sysop"] = binkpSysop
				r.BinkPResult.Details["location"] = binkpLocation
				r.BinkPResult.Details["version"] = binkpVersion
				r.BinkPResult.Details["addresses"] = binkpAddresses
				r.BinkPResult.Details["capabilities"] = binkpCapabilities
			}
		}
		
		// Populate IFCICO result if tested
		if ifcicoTested {
			r.IfcicoResult = &models.ProtocolTestResult{
				Tested:     true,
				Success:    ifcicoSuccess,
				ResponseMs: ifcicoResponseMs,
				Error:      ifcicoError,
				Details:    make(map[string]interface{}),
			}
			if ifcicoSystemName != "" || ifcicoMailerInfo != "" {
				r.IfcicoResult.Details["system_name"] = ifcicoSystemName
				r.IfcicoResult.Details["mailer_info"] = ifcicoMailerInfo
				r.IfcicoResult.Details["addresses"] = ifcicoAddresses
				r.IfcicoResult.Details["response_type"] = ifcicoResponseType
			}
		}
		
		// Populate Telnet result if tested
		if telnetTested {
			r.TelnetResult = &models.ProtocolTestResult{
				Tested:     true,
				Success:    telnetSuccess,
				ResponseMs: telnetResponseMs,
				Error:      telnetError,
				Details:    make(map[string]interface{}),
			}
		}
		
		// Populate FTP result if tested
		if ftpTested {
			r.FTPResult = &models.ProtocolTestResult{
				Tested:     true,
				Success:    ftpSuccess,
				ResponseMs: ftpResponseMs,
				Error:      ftpError,
				Details:    make(map[string]interface{}),
			}
		}
		
		// Populate VModem result if tested
		if vmodemTested {
			r.VModemResult = &models.ProtocolTestResult{
				Tested:     true,
				Success:    vmodemSuccess,
				ResponseMs: vmodemResponseMs,
				Error:      vmodemError,
				Details:    make(map[string]interface{}),
			}
		}
		
		results = append(results, r)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating test results: %w", err)
	}
	
	return results, nil
}

// GetCurrentNodeStatus gets the latest status for each node from test results
func (s *ClickHouseStorage) GetCurrentNodeStatus(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			zone, net, node, address,
			argMax(test_time, test_time) as last_test_time,
			argMax(is_operational, test_time) as is_operational,
			argMax(binkp_success, test_time) as binkp_works,
			argMax(country, test_time) as country,
			argMax(isp, test_time) as isp
		FROM node_test_results
		WHERE test_time > now() - INTERVAL 7 DAY
		GROUP BY zone, net, node, address
		ORDER BY zone, net, node
	`
	
	rows, err := s.conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get current node status: %w", err)
	}
	defer rows.Close()
	
	var results []map[string]interface{}
	for rows.Next() {
		var zone, net, node int32
		var address, country, isp string
		var lastTestTime time.Time
		var isOperational, binkpWorks bool
		
		err := rows.Scan(&zone, &net, &node, &address, &lastTestTime, 
			&isOperational, &binkpWorks, &country, &isp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		
		results = append(results, map[string]interface{}{
			"zone":           zone,
			"net":            net,
			"node":           node,
			"address":        address,
			"last_test_time": lastTestTime,
			"is_operational": isOperational,
			"binkp_works":    binkpWorks,
			"country":        country,
			"isp":            isp,
		})
	}
	
	return results, nil
}

// GetNodeTestHistory retrieves test history for a specific node
func (s *ClickHouseStorage) GetNodeTestHistory(ctx context.Context, zone, net, node int, limit int) ([]*models.TestResult, error) {
	// Debug logging disabled for performance
	// fmt.Printf("DEBUG GetNodeTestHistory: Querying history for %d:%d/%d (limit=%d)\n", zone, net, node, limit)
	
	query := `
		SELECT 
			test_time, test_date,
			zone, net, node, address,
			hostname, resolved_ipv4, resolved_ipv6, dns_error,
			country, country_code, city, region, 
			latitude, longitude, isp, org, asn,
			binkp_tested, binkp_success, binkp_response_ms,
			binkp_system_name, binkp_sysop, binkp_location, 
			binkp_version, binkp_addresses, binkp_capabilities, binkp_error,
			ifcico_tested, ifcico_success, ifcico_response_ms,
			ifcico_mailer_info, ifcico_system_name, ifcico_addresses, 
			ifcico_response_type, ifcico_error,
			telnet_tested, telnet_success, telnet_response_ms, telnet_error,
			ftp_tested, ftp_success, ftp_response_ms, ftp_error,
			vmodem_tested, vmodem_success, vmodem_response_ms, vmodem_error,
			is_operational, has_connectivity_issues, address_validated
		FROM node_test_results
		WHERE zone = ? AND net = ? AND node = ?
		ORDER BY test_time DESC`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	
	rows, err := s.conn.Query(ctx, query, zone, net, node)
	if err != nil {
		// Log the error for debugging
		fmt.Printf("DEBUG GetNodeTestHistory ERROR: Failed to query history for %d:%d/%d - %v\n", zone, net, node, err)
		// Return empty result if no history found (not an error for scheduler)
		return []*models.TestResult{}, nil
	}
	defer rows.Close()
	
	var results []*models.TestResult
	for rows.Next() {
		r := &models.TestResult{}
		var testDate time.Time
		var zone, net, node uint16  // ClickHouse uses UInt16 for these columns
		var dnsError, binkpError, ifcicoError, telnetError, ftpError, vmodemError string
		var binkpTested, binkpSuccess, ifcicoTested, ifcicoSuccess bool
		var telnetTested, telnetSuccess, ftpTested, ftpSuccess bool 
		var vmodemTested, vmodemSuccess bool
		var binkpResponseMs, ifcicoResponseMs, telnetResponseMs uint32
		var ftpResponseMs, vmodemResponseMs uint32
		var binkpSystemName, binkpSysop, binkpLocation, binkpVersion string
		var binkpAddresses, binkpCapabilities []string
		var ifcicoMailerInfo, ifcicoSystemName, ifcicoResponseType string
		var ifcicoAddresses []string
		
		err := rows.Scan(
			&r.TestTime, &testDate,
			&zone, &net, &node, &r.Address,
			&r.Hostname, &r.ResolvedIPv4, &r.ResolvedIPv6, &dnsError,
			&r.Country, &r.CountryCode, &r.City, &r.Region,
			&r.Latitude, &r.Longitude, &r.ISP, &r.Org, &r.ASN,
			&binkpTested, &binkpSuccess, &binkpResponseMs,
			&binkpSystemName, &binkpSysop, &binkpLocation,
			&binkpVersion, &binkpAddresses, &binkpCapabilities, &binkpError,
			&ifcicoTested, &ifcicoSuccess, &ifcicoResponseMs,
			&ifcicoMailerInfo, &ifcicoSystemName, &ifcicoAddresses,
			&ifcicoResponseType, &ifcicoError,
			&telnetTested, &telnetSuccess, &telnetResponseMs, &telnetError,
			&ftpTested, &ftpSuccess, &ftpResponseMs, &ftpError,
			&vmodemTested, &vmodemSuccess, &vmodemResponseMs, &vmodemError,
			&r.IsOperational, &r.HasConnectivityIssues, &r.AddressValidated,
		)
		if err != nil {
			// Log error but continue
			continue
		}
		
		// Convert UInt16 to int
		r.Zone = int(zone)
		r.Net = int(net)
		r.Node = int(node)
		r.TestDate = testDate
		r.DNSError = dnsError
		
		// Populate BinkP result if tested
		if binkpTested {
			r.BinkPResult = &models.ProtocolTestResult{
				Tested:     true,
				Success:    binkpSuccess,
				ResponseMs: binkpResponseMs,
				Error:      binkpError,
				Details:    make(map[string]interface{}),
			}
			if binkpSystemName != "" {
				r.BinkPResult.Details["system_name"] = binkpSystemName
				r.BinkPResult.Details["sysop"] = binkpSysop
				r.BinkPResult.Details["location"] = binkpLocation
				r.BinkPResult.Details["version"] = binkpVersion
				r.BinkPResult.Details["addresses"] = binkpAddresses
				r.BinkPResult.Details["capabilities"] = binkpCapabilities
			}
		}
		
		// Populate IFCICO result if tested
		if ifcicoTested {
			r.IfcicoResult = &models.ProtocolTestResult{
				Tested:     true,
				Success:    ifcicoSuccess,
				ResponseMs: ifcicoResponseMs,
				Error:      ifcicoError,
				Details:    make(map[string]interface{}),
			}
			if ifcicoSystemName != "" || ifcicoMailerInfo != "" {
				r.IfcicoResult.Details["system_name"] = ifcicoSystemName
				r.IfcicoResult.Details["mailer_info"] = ifcicoMailerInfo
				r.IfcicoResult.Details["addresses"] = ifcicoAddresses
				r.IfcicoResult.Details["response_type"] = ifcicoResponseType
			}
		}
		
		// Populate Telnet result if tested
		if telnetTested {
			r.TelnetResult = &models.ProtocolTestResult{
				Tested:     true,
				Success:    telnetSuccess,
				ResponseMs: telnetResponseMs,
				Error:      telnetError,
				Details:    make(map[string]interface{}),
			}
		}
		
		// Populate FTP result if tested
		if ftpTested {
			r.FTPResult = &models.ProtocolTestResult{
				Tested:     true,
				Success:    ftpSuccess,
				ResponseMs: ftpResponseMs,
				Error:      ftpError,
				Details:    make(map[string]interface{}),
			}
		}
		
		// Populate VModem result if tested
		if vmodemTested {
			r.VModemResult = &models.ProtocolTestResult{
				Tested:     true,
				Success:    vmodemSuccess,
				ResponseMs: vmodemResponseMs,
				Error:      vmodemError,
				Details:    make(map[string]interface{}),
			}
		}
		
		results = append(results, r)
	}
	
	if err := rows.Err(); err != nil {
		// Return what we got so far
		return results, nil
	}
	
	// Debug logging disabled for performance
	// fmt.Printf("DEBUG GetNodeTestHistory: Found %d results for %d:%d/%d\n", len(results), zone, net, node)
	
	return results, nil
}

// Helper methods

// scanNodesNative scans rows from native ClickHouse driver
func (s *ClickHouseStorage) scanNodesNative(rows driver.Rows) ([]*models.Node, error) {
	var nodes []*models.Node
	for rows.Next() {
		node := &models.Node{}
		var hostnames, protocols string
		var zone, net, nodeNum int32
		var configJSON string
		
		err := rows.Scan(
			&zone,
			&net,
			&nodeNum,
			&node.SystemName,
			&node.SysopName,
			&node.Location,
			&hostnames,
			&protocols,
			&node.HasInet,
			&configJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		
		// Convert int32 values to int
		node.Zone = int(zone)
		node.Net = int(net)
		node.Node = int(nodeNum)
		
		// Convert hostname string to array and handle addresses with embedded ports
		if hostnames != "" {
			// Check if the hostname contains a port (e.g., "185.22.236.179:2030" for IVM)
			if strings.Contains(hostnames, ":") {
				// Split hostname and port
				parts := strings.SplitN(hostnames, ":", 2)
				if len(parts) == 2 {
					node.InternetHostnames = []string{parts[0]}
					// Store the port for IVM protocol if it's the only protocol
					if len(node.InternetProtocols) == 1 && node.InternetProtocols[0] == "IVM" {
						if port, err := strconv.Atoi(parts[1]); err == nil {
							node.ProtocolPorts["IVM"] = port
						}
					}
				} else {
					node.InternetHostnames = []string{hostnames}
				}
			} else {
				node.InternetHostnames = []string{hostnames}
			}
		} else {
			node.InternetHostnames = []string{}
		}
		
		// Parse protocols from comma-separated string
		if protocols != "" {
			node.InternetProtocols = strings.Split(protocols, ",")
			// Trim spaces from each protocol
			for i := range node.InternetProtocols {
				node.InternetProtocols[i] = strings.TrimSpace(node.InternetProtocols[i])
			}
		} else {
			node.InternetProtocols = []string{}
		}
		
		// Parse internet_config JSON to extract custom ports
		node.ProtocolPorts = make(map[string]int)
		if configJSON != "" && configJSON != "{}" {
			var config map[string]interface{}
			if err := json.Unmarshal([]byte(configJSON), &config); err == nil {
				// Extract protocol ports from the JSON structure
				if protocols, ok := config["protocols"].(map[string]interface{}); ok {
					for proto, protoData := range protocols {
						if protoMap, ok := protoData.(map[string]interface{}); ok {
							// Check for port field
							if portStr, ok := protoMap["port"].(string); ok {
								if port, err := strconv.Atoi(portStr); err == nil {
									node.ProtocolPorts[proto] = port
								}
							} else if portFloat, ok := protoMap["port"].(float64); ok {
								// Sometimes port comes as a number
								node.ProtocolPorts[proto] = int(portFloat)
							}

							// Special handling for IVM protocol with embedded port in address
							if proto == "IVM" {
								if addr, ok := protoMap["address"].(string); ok && strings.Contains(addr, ":") {
									parts := strings.SplitN(addr, ":", 2)
									if len(parts) == 2 {
										if port, err := strconv.Atoi(parts[1]); err == nil {
											node.ProtocolPorts[proto] = port
										}
									}
								}
							}
						}
					}
				}
			}
		}
		
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (s *ClickHouseStorage) scanNodes(rows *sql.Rows) ([]*models.Node, error) {
	var nodes []*models.Node
	for rows.Next() {
		node := &models.Node{}
		var hostnames, protocols string
		
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
		
		// Convert hostname string to array and handle addresses with embedded ports
		if hostnames != "" {
			// Check if the hostname contains a port (e.g., "185.22.236.179:2030" for IVM)
			if strings.Contains(hostnames, ":") {
				// Split hostname and port
				parts := strings.SplitN(hostnames, ":", 2)
				if len(parts) == 2 {
					node.InternetHostnames = []string{parts[0]}
					// Store the port for IVM protocol if it's the only protocol
					if len(node.InternetProtocols) == 1 && node.InternetProtocols[0] == "IVM" {
						if port, err := strconv.Atoi(parts[1]); err == nil {
							node.ProtocolPorts["IVM"] = port
						}
					}
				} else {
					node.InternetHostnames = []string{hostnames}
				}
			} else {
				node.InternetHostnames = []string{hostnames}
			}
		} else {
			node.InternetHostnames = []string{}
		}
		
		// Parse protocols from comma-separated string
		if protocols != "" {
			node.InternetProtocols = strings.Split(protocols, ",")
			// Trim spaces from each protocol
			for i := range node.InternetProtocols {
				node.InternetProtocols[i] = strings.TrimSpace(node.InternetProtocols[i])
			}
		} else {
			node.InternetProtocols = []string{}
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
	
	// Extract IPv4/IPv6 specific results for each protocol
	var binkpIPv4Tested, binkpIPv4Success, binkpIPv6Tested, binkpIPv6Success bool
	var binkpIPv4ResponseMs, binkpIPv6ResponseMs uint32
	var binkpIPv4Address, binkpIPv4Error, binkpIPv6Address, binkpIPv6Error string

	if r.BinkPResult != nil {
		binkpIPv4Tested = r.BinkPResult.IPv4Tested
		binkpIPv4Success = r.BinkPResult.IPv4Success
		binkpIPv4ResponseMs = r.BinkPResult.IPv4ResponseMs
		binkpIPv4Address = r.BinkPResult.IPv4Address
		binkpIPv4Error = r.BinkPResult.IPv4Error

		binkpIPv6Tested = r.BinkPResult.IPv6Tested
		binkpIPv6Success = r.BinkPResult.IPv6Success
		binkpIPv6ResponseMs = r.BinkPResult.IPv6ResponseMs
		binkpIPv6Address = r.BinkPResult.IPv6Address
		binkpIPv6Error = r.BinkPResult.IPv6Error
	}

	var ifcicoIPv4Tested, ifcicoIPv4Success, ifcicoIPv6Tested, ifcicoIPv6Success bool
	var ifcicoIPv4ResponseMs, ifcicoIPv6ResponseMs uint32
	var ifcicoIPv4Address, ifcicoIPv4Error, ifcicoIPv6Address, ifcicoIPv6Error string

	if r.IfcicoResult != nil {
		ifcicoIPv4Tested = r.IfcicoResult.IPv4Tested
		ifcicoIPv4Success = r.IfcicoResult.IPv4Success
		ifcicoIPv4ResponseMs = r.IfcicoResult.IPv4ResponseMs
		ifcicoIPv4Address = r.IfcicoResult.IPv4Address
		ifcicoIPv4Error = r.IfcicoResult.IPv4Error

		ifcicoIPv6Tested = r.IfcicoResult.IPv6Tested
		ifcicoIPv6Success = r.IfcicoResult.IPv6Success
		ifcicoIPv6ResponseMs = r.IfcicoResult.IPv6ResponseMs
		ifcicoIPv6Address = r.IfcicoResult.IPv6Address
		ifcicoIPv6Error = r.IfcicoResult.IPv6Error
	}

	var telnetIPv4Tested, telnetIPv4Success, telnetIPv6Tested, telnetIPv6Success bool
	var telnetIPv4ResponseMs, telnetIPv6ResponseMs uint32
	var telnetIPv4Address, telnetIPv4Error, telnetIPv6Address, telnetIPv6Error string

	if r.TelnetResult != nil {
		telnetIPv4Tested = r.TelnetResult.IPv4Tested
		telnetIPv4Success = r.TelnetResult.IPv4Success
		telnetIPv4ResponseMs = r.TelnetResult.IPv4ResponseMs
		telnetIPv4Address = r.TelnetResult.IPv4Address
		telnetIPv4Error = r.TelnetResult.IPv4Error

		telnetIPv6Tested = r.TelnetResult.IPv6Tested
		telnetIPv6Success = r.TelnetResult.IPv6Success
		telnetIPv6ResponseMs = r.TelnetResult.IPv6ResponseMs
		telnetIPv6Address = r.TelnetResult.IPv6Address
		telnetIPv6Error = r.TelnetResult.IPv6Error
	}

	return []interface{}{
		// First 52 fields (original)
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
		// IPv4/IPv6 specific fields (53-82)
		binkpIPv4Tested, binkpIPv4Success, binkpIPv4ResponseMs, binkpIPv4Address, binkpIPv4Error,
		binkpIPv6Tested, binkpIPv6Success, binkpIPv6ResponseMs, binkpIPv6Address, binkpIPv6Error,
		ifcicoIPv4Tested, ifcicoIPv4Success, ifcicoIPv4ResponseMs, ifcicoIPv4Address, ifcicoIPv4Error,
		ifcicoIPv6Tested, ifcicoIPv6Success, ifcicoIPv6ResponseMs, ifcicoIPv6Address, ifcicoIPv6Error,
		telnetIPv4Tested, telnetIPv4Success, telnetIPv4ResponseMs, telnetIPv4Address, telnetIPv4Error,
		telnetIPv6Tested, telnetIPv6Success, telnetIPv6ResponseMs, telnetIPv6Address, telnetIPv6Error,
	}
}