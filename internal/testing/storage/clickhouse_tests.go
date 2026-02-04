package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

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

// StoreBatchTestResults stores multiple test results as a batch (for per-hostname testing)
func (s *ClickHouseStorage) StoreBatchTestResults(ctx context.Context, results []*models.TestResult) error {
	// For batch inserts, ensure legacy compatibility
	for _, result := range results {
		if result.HostnameIndex == 0 && !result.IsAggregated && result.TestedHostname == "" {
			// Mark as legacy if not explicitly set
			result.HostnameIndex = -1
			result.IsAggregated = true
		}
	}

	// Use existing batch storage mechanism
	return s.StoreTestResults(ctx, results)
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
		ipv4_skipped,
		binkp_ipv4_tested, binkp_ipv4_success, binkp_ipv4_response_ms, binkp_ipv4_address, binkp_ipv4_error,
		binkp_ipv6_tested, binkp_ipv6_success, binkp_ipv6_response_ms, binkp_ipv6_address, binkp_ipv6_error,
		ifcico_ipv4_tested, ifcico_ipv4_success, ifcico_ipv4_response_ms, ifcico_ipv4_address, ifcico_ipv4_error,
		ifcico_ipv6_tested, ifcico_ipv6_success, ifcico_ipv6_response_ms, ifcico_ipv6_address, ifcico_ipv6_error,
		telnet_ipv4_tested, telnet_ipv4_success, telnet_ipv4_response_ms, telnet_ipv4_address, telnet_ipv4_error,
		telnet_ipv6_tested, telnet_ipv6_success, telnet_ipv6_response_ms, telnet_ipv6_address, telnet_ipv6_error,
		ftp_ipv4_tested, ftp_ipv4_success, ftp_ipv4_response_ms, ftp_ipv4_address, ftp_ipv4_error,
		ftp_ipv6_tested, ftp_ipv6_success, ftp_ipv6_response_ms, ftp_ipv6_address, ftp_ipv6_error,
		vmodem_ipv4_tested, vmodem_ipv4_success, vmodem_ipv4_response_ms, vmodem_ipv4_address, vmodem_ipv4_error,
		vmodem_ipv6_tested, vmodem_ipv6_success, vmodem_ipv6_response_ms, vmodem_ipv6_address, vmodem_ipv6_error,
		tested_hostname, hostname_index, is_aggregated, total_hostnames, hostnames_tested, hostnames_operational,
		binkp_ipv4_addresses, binkp_ipv6_addresses, ifcico_ipv4_addresses, ifcico_ipv6_addresses,
		address_validated_ipv4, address_validated_ipv6
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
		var zone, net, node uint16 // ClickHouse uses UInt16 for these columns
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
			is_operational, has_connectivity_issues, address_validated,
			tested_hostname, hostname_index, is_aggregated,
			total_hostnames, hostnames_tested, hostnames_operational
		FROM node_test_results
		WHERE zone = ? AND net = ? AND node = ?
		ORDER BY test_time DESC, hostname_index`

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
		var zone, net, node uint16 // ClickHouse uses UInt16 for these columns
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
			&r.TestedHostname, &r.HostnameIndex, &r.IsAggregated,
			&r.TotalHostnames, &r.HostnamesTested, &r.HostnamesOperational,
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

// resultToValues converts TestResult to values for batch insert
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
		// First try to get from IPv6 or IPv4 structured details
		if details, ok := r.BinkPResult.Details["ipv6"].(*models.BinkPTestDetails); ok {
			binkpSystemName = details.SystemName
			binkpSysop = details.Sysop
			binkpLocation = details.Location
			binkpVersion = details.Version
			binkpAddresses = details.Addresses
			binkpCapabilities = details.Capabilities
		} else if details, ok := r.BinkPResult.Details["ipv4"].(*models.BinkPTestDetails); ok {
			binkpSystemName = details.SystemName
			binkpSysop = details.Sysop
			binkpLocation = details.Location
			binkpVersion = details.Version
			binkpAddresses = details.Addresses
			binkpCapabilities = details.Capabilities
		} else {
			// Fall back to flat string extraction for backward compatibility
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

		// First try to get from IPv6 or IPv4 structured details
		if details, ok := r.IfcicoResult.Details["ipv6"].(*models.IfcicoTestDetails); ok {
			ifcicoMailerInfo = details.MailerInfo
			ifcicoSystemName = details.SystemName
			ifcicoAddresses = details.Addresses
			ifcicoResponseType = details.ResponseType
		} else if details, ok := r.IfcicoResult.Details["ipv4"].(*models.IfcicoTestDetails); ok {
			ifcicoMailerInfo = details.MailerInfo
			ifcicoSystemName = details.SystemName
			ifcicoAddresses = details.Addresses
			ifcicoResponseType = details.ResponseType
		} else {
			// Fall back to flat string extraction for backward compatibility
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

	var ftpIPv4Tested, ftpIPv4Success, ftpIPv6Tested, ftpIPv6Success bool
	var ftpIPv4ResponseMs, ftpIPv6ResponseMs uint32
	var ftpIPv4Address, ftpIPv4Error, ftpIPv6Address, ftpIPv6Error string

	if r.FTPResult != nil {
		ftpIPv4Tested = r.FTPResult.IPv4Tested
		ftpIPv4Success = r.FTPResult.IPv4Success
		ftpIPv4ResponseMs = r.FTPResult.IPv4ResponseMs
		ftpIPv4Address = r.FTPResult.IPv4Address
		ftpIPv4Error = r.FTPResult.IPv4Error

		ftpIPv6Tested = r.FTPResult.IPv6Tested
		ftpIPv6Success = r.FTPResult.IPv6Success
		ftpIPv6ResponseMs = r.FTPResult.IPv6ResponseMs
		ftpIPv6Address = r.FTPResult.IPv6Address
		ftpIPv6Error = r.FTPResult.IPv6Error
	}

	var vmodemIPv4Tested, vmodemIPv4Success, vmodemIPv6Tested, vmodemIPv6Success bool
	var vmodemIPv4ResponseMs, vmodemIPv6ResponseMs uint32
	var vmodemIPv4Address, vmodemIPv4Error, vmodemIPv6Address, vmodemIPv6Error string

	if r.VModemResult != nil {
		vmodemIPv4Tested = r.VModemResult.IPv4Tested
		vmodemIPv4Success = r.VModemResult.IPv4Success
		vmodemIPv4ResponseMs = r.VModemResult.IPv4ResponseMs
		vmodemIPv4Address = r.VModemResult.IPv4Address
		vmodemIPv4Error = r.VModemResult.IPv4Error

		vmodemIPv6Tested = r.VModemResult.IPv6Tested
		vmodemIPv6Success = r.VModemResult.IPv6Success
		vmodemIPv6ResponseMs = r.VModemResult.IPv6ResponseMs
		vmodemIPv6Address = r.VModemResult.IPv6Address
		vmodemIPv6Error = r.VModemResult.IPv6Error
	}

	// Extract per-IP-version announced addresses for IPv4/IPv6 AKA mismatch detection
	var binkpIPv4Addrs, binkpIPv6Addrs []string
	if r.BinkPResult != nil {
		if details, ok := r.BinkPResult.Details["ipv6"].(*models.BinkPTestDetails); ok {
			binkpIPv6Addrs = details.Addresses
		}
		if details, ok := r.BinkPResult.Details["ipv4"].(*models.BinkPTestDetails); ok {
			binkpIPv4Addrs = details.Addresses
		}
	}

	var ifcicoIPv4Addrs, ifcicoIPv6Addrs []string
	if r.IfcicoResult != nil {
		if details, ok := r.IfcicoResult.Details["ipv6"].(*models.IfcicoTestDetails); ok {
			ifcicoIPv6Addrs = details.Addresses
		}
		if details, ok := r.IfcicoResult.Details["ipv4"].(*models.IfcicoTestDetails); ok {
			ifcicoIPv4Addrs = details.Addresses
		}
	}

	// Handle legacy compatibility: set defaults if hostname_index not set
	testedHostname := r.TestedHostname
	if testedHostname == "" {
		testedHostname = r.Hostname
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
		// INO4 flag (FTS-1038)
		r.IPv4Skipped,
		// IPv4/IPv6 specific fields (54-83)
		binkpIPv4Tested, binkpIPv4Success, binkpIPv4ResponseMs, binkpIPv4Address, binkpIPv4Error,
		binkpIPv6Tested, binkpIPv6Success, binkpIPv6ResponseMs, binkpIPv6Address, binkpIPv6Error,
		ifcicoIPv4Tested, ifcicoIPv4Success, ifcicoIPv4ResponseMs, ifcicoIPv4Address, ifcicoIPv4Error,
		ifcicoIPv6Tested, ifcicoIPv6Success, ifcicoIPv6ResponseMs, ifcicoIPv6Address, ifcicoIPv6Error,
		telnetIPv4Tested, telnetIPv4Success, telnetIPv4ResponseMs, telnetIPv4Address, telnetIPv4Error,
		telnetIPv6Tested, telnetIPv6Success, telnetIPv6ResponseMs, telnetIPv6Address, telnetIPv6Error,
		ftpIPv4Tested, ftpIPv4Success, ftpIPv4ResponseMs, ftpIPv4Address, ftpIPv4Error,
		ftpIPv6Tested, ftpIPv6Success, ftpIPv6ResponseMs, ftpIPv6Address, ftpIPv6Error,
		vmodemIPv4Tested, vmodemIPv4Success, vmodemIPv4ResponseMs, vmodemIPv4Address, vmodemIPv4Error,
		vmodemIPv6Tested, vmodemIPv6Success, vmodemIPv6ResponseMs, vmodemIPv6Address, vmodemIPv6Error,
		// New per-hostname testing fields (85-90)
		testedHostname, r.HostnameIndex, r.IsAggregated,
		r.TotalHostnames, r.HostnamesTested, r.HostnamesOperational,
		// Per-IP-version AKA addresses and validation flags
		binkpIPv4Addrs, binkpIPv6Addrs, ifcicoIPv4Addrs, ifcicoIPv6Addrs,
		r.AddressValidatedIPv4, r.AddressValidatedIPv6,
	}
}
