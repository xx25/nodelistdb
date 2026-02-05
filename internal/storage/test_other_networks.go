package storage

import (
	"fmt"
	"strings"
	"sync"

	"github.com/nodelistdb/internal/database"
)

// OtherNetworkSummary holds summary info about a non-FidoNet network
type OtherNetworkSummary struct {
	NetworkName string // e.g., "tqwnet", "fsxnet"
	NodeCount   int    // Number of unique nodes announcing this network
}

// OtherNetworkNode holds a node that announces AKAs in other networks
type OtherNetworkNode struct {
	NodeTestResult
	NetworkAddresses []string // Addresses for the specific network being viewed
}

// OtherNetworksOperations handles other network AKA queries
type OtherNetworksOperations struct {
	db database.DatabaseInterface
	mu sync.RWMutex
}

// NewOtherNetworksOperations creates a new other networks operations instance
func NewOtherNetworksOperations(db database.DatabaseInterface) *OtherNetworksOperations {
	return &OtherNetworksOperations{
		db: db,
	}
}

// escapeLikePattern escapes LIKE wildcard characters (% and _) in a string
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// GetOtherNetworksSummary retrieves a summary of all non-FidoNet networks found in AKAs
func (on *OtherNetworksOperations) GetOtherNetworksSummary(days int) ([]OtherNetworkSummary, error) {
	on.mu.RLock()
	defer on.mu.RUnlock()

	conn := on.db.Conn()

	// This query extracts network names from addresses with @ suffix
	// and counts unique nodes for each network
	query := `
		WITH
		-- Get latest test per node with successful BinkP or IFCICO
		latest_tests AS (
			SELECT
				zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				AND is_aggregated = false
				AND is_operational = true
				AND (binkp_success = true OR ifcico_success = true)
			GROUP BY zone, net, node
		),
		-- Get addresses from latest tests, pick one row per node (lowest hostname_index)
		latest_addresses AS (
			SELECT
				r.zone, r.net, r.node,
				arrayConcat(
					if(r.binkp_success, r.binkp_addresses, []),
					if(r.ifcico_success, r.ifcico_addresses, [])
				) as all_addresses
			FROM node_test_results r
			JOIN latest_tests lt ON r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node AND r.test_time = lt.latest_test_time
			WHERE r.is_aggregated = false
				AND (length(r.binkp_addresses) > 0 OR length(r.ifcico_addresses) > 0)
		),
		-- Extract network names from addresses with @ suffix
		network_addresses AS (
			SELECT
				zone, net, node,
				lower(extractAllGroups(addr, '@([a-zA-Z0-9_-]+)')[1][1]) as network_name
			FROM latest_addresses
			ARRAY JOIN all_addresses as addr
			WHERE addr LIKE '%@%'
		)
		SELECT
			network_name,
			count(DISTINCT (zone, net, node)) as node_count
		FROM network_addresses
		WHERE network_name != ''
		GROUP BY network_name
		ORDER BY node_count DESC, network_name ASC
	`

	rows, err := conn.Query(query, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query other networks summary: %w", err)
	}
	defer rows.Close()

	var results []OtherNetworkSummary
	for rows.Next() {
		var s OtherNetworkSummary
		if err := rows.Scan(&s.NetworkName, &s.NodeCount); err != nil {
			return nil, fmt.Errorf("failed to scan network summary: %w", err)
		}
		results = append(results, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating network summary rows: %w", err)
	}

	return results, nil
}

// GetNodesInNetwork retrieves nodes that announce AKAs in a specific network
func (on *OtherNetworksOperations) GetNodesInNetwork(networkName string, limit int, days int) ([]OtherNetworkNode, error) {
	on.mu.RLock()
	defer on.mu.RUnlock()

	conn := on.db.Conn()

	// Escape LIKE wildcards in the network name to prevent pattern injection
	escapedName := escapeLikePattern(networkName)

	query := `
		WITH
		-- Get latest test per node with successful BinkP or IFCICO
		latest_tests AS (
			SELECT
				zone, net, node,
				max(test_time) as latest_test_time
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				AND is_aggregated = false
				AND is_operational = true
				AND (binkp_success = true OR ifcico_success = true)
			GROUP BY zone, net, node
		),
		-- Find nodes with addresses in the target network, one row per node
		-- Pick the row with the lowest hostname_index to avoid duplicates from multi-host nodes
		best_rows AS (
			SELECT
				r.zone, r.net, r.node, r.test_time, r.hostname_index,
				row_number() OVER (
					PARTITION BY r.zone, r.net, r.node
					ORDER BY r.hostname_index ASC
				) as rn
			FROM node_test_results r
			JOIN latest_tests lt ON r.zone = lt.zone AND r.net = lt.net AND r.node = lt.node AND r.test_time = lt.latest_test_time
			WHERE r.is_aggregated = false
				AND (
					arrayExists(addr -> lower(addr) LIKE '%@' || lower(?) || '%', r.binkp_addresses)
					OR arrayExists(addr -> lower(addr) LIKE '%@' || lower(?) || '%', r.ifcico_addresses)
				)
		)
		SELECT
			r.test_time, r.zone, r.net, r.node, r.address, r.hostname,
			r.resolved_ipv4, r.resolved_ipv6, r.dns_error,
			r.country, r.country_code, r.city, r.region, r.latitude, r.longitude, r.isp, r.org, r.asn,
			r.binkp_tested, r.binkp_success, r.binkp_response_ms, r.binkp_system_name,
			r.binkp_sysop, r.binkp_location, r.binkp_version, r.binkp_addresses, r.binkp_capabilities, r.binkp_error,
			r.ifcico_tested, r.ifcico_success, r.ifcico_response_ms, r.ifcico_mailer_info,
			r.ifcico_system_name, r.ifcico_addresses, r.ifcico_response_type, r.ifcico_error,
			r.telnet_tested, r.telnet_success, r.telnet_response_ms, r.telnet_error,
			r.ftp_tested, r.ftp_success, r.ftp_response_ms, r.ftp_error,
			r.vmodem_tested, r.vmodem_success, r.vmodem_response_ms, r.vmodem_error,
			r.binkp_ipv4_tested, r.binkp_ipv4_success, r.binkp_ipv4_response_ms, r.binkp_ipv4_address, r.binkp_ipv4_error,
			r.binkp_ipv6_tested, r.binkp_ipv6_success, r.binkp_ipv6_response_ms, r.binkp_ipv6_address, r.binkp_ipv6_error,
			r.ifcico_ipv4_tested, r.ifcico_ipv4_success, r.ifcico_ipv4_response_ms, r.ifcico_ipv4_address, r.ifcico_ipv4_error,
			r.ifcico_ipv6_tested, r.ifcico_ipv6_success, r.ifcico_ipv6_response_ms, r.ifcico_ipv6_address, r.ifcico_ipv6_error,
			r.telnet_ipv4_tested, r.telnet_ipv4_success, r.telnet_ipv4_response_ms, r.telnet_ipv4_address, r.telnet_ipv4_error,
			r.telnet_ipv6_tested, r.telnet_ipv6_success, r.telnet_ipv6_response_ms, r.telnet_ipv6_address, r.telnet_ipv6_error,
			r.ftp_ipv4_tested, r.ftp_ipv4_success, r.ftp_ipv4_response_ms, r.ftp_ipv4_address, r.ftp_ipv4_error,
			r.ftp_ipv6_tested, r.ftp_ipv6_success, r.ftp_ipv6_response_ms, r.ftp_ipv6_address, r.ftp_ipv6_error,
			r.vmodem_ipv4_tested, r.vmodem_ipv4_success, r.vmodem_ipv4_response_ms, r.vmodem_ipv4_address, r.vmodem_ipv4_error,
			r.vmodem_ipv6_tested, r.vmodem_ipv6_success, r.vmodem_ipv6_response_ms, r.vmodem_ipv6_address, r.vmodem_ipv6_error,
			r.is_operational, r.has_connectivity_issues, r.address_validated,
			r.tested_hostname, r.hostname_index, r.is_aggregated,
			r.total_hostnames, r.hostnames_tested, r.hostnames_operational,
			r.ftp_anon_success,
			-- Extract addresses for this specific network
			arrayFilter(addr -> lower(addr) LIKE '%@' || lower(?) || '%', arrayConcat(r.binkp_addresses, r.ifcico_addresses)) as network_addresses
		FROM node_test_results r
		JOIN best_rows br ON r.zone = br.zone AND r.net = br.net AND r.node = br.node
			AND r.test_time = br.test_time AND r.hostname_index = br.hostname_index AND br.rn = 1
		WHERE r.is_aggregated = false
		ORDER BY r.zone, r.net, r.node
		LIMIT ?
	`

	rows, err := conn.Query(query, days, escapedName, escapedName, escapedName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes in network %s: %w", networkName, err)
	}
	defer rows.Close()

	var results []OtherNetworkNode
	for rows.Next() {
		var n OtherNetworkNode
		err := rows.Scan(
			&n.TestTime, &n.Zone, &n.Net, &n.Node, &n.Address, &n.Hostname,
			&n.ResolvedIPv4, &n.ResolvedIPv6, &n.DNSError,
			&n.Country, &n.CountryCode, &n.City, &n.Region, &n.Latitude, &n.Longitude, &n.ISP, &n.Org, &n.ASN,
			&n.BinkPTested, &n.BinkPSuccess, &n.BinkPResponseMs, &n.BinkPSystemName,
			&n.BinkPSysop, &n.BinkPLocation, &n.BinkPVersion, &n.BinkPAddresses, &n.BinkPCapabilities, &n.BinkPError,
			&n.IfcicoTested, &n.IfcicoSuccess, &n.IfcicoResponseMs, &n.IfcicoMailerInfo,
			&n.IfcicoSystemName, &n.IfcicoAddresses, &n.IfcicoResponseType, &n.IfcicoError,
			&n.TelnetTested, &n.TelnetSuccess, &n.TelnetResponseMs, &n.TelnetError,
			&n.FTPTested, &n.FTPSuccess, &n.FTPResponseMs, &n.FTPError,
			&n.VModemTested, &n.VModemSuccess, &n.VModemResponseMs, &n.VModemError,
			&n.BinkPIPv4Tested, &n.BinkPIPv4Success, &n.BinkPIPv4ResponseMs, &n.BinkPIPv4Address, &n.BinkPIPv4Error,
			&n.BinkPIPv6Tested, &n.BinkPIPv6Success, &n.BinkPIPv6ResponseMs, &n.BinkPIPv6Address, &n.BinkPIPv6Error,
			&n.IfcicoIPv4Tested, &n.IfcicoIPv4Success, &n.IfcicoIPv4ResponseMs, &n.IfcicoIPv4Address, &n.IfcicoIPv4Error,
			&n.IfcicoIPv6Tested, &n.IfcicoIPv6Success, &n.IfcicoIPv6ResponseMs, &n.IfcicoIPv6Address, &n.IfcicoIPv6Error,
			&n.TelnetIPv4Tested, &n.TelnetIPv4Success, &n.TelnetIPv4ResponseMs, &n.TelnetIPv4Address, &n.TelnetIPv4Error,
			&n.TelnetIPv6Tested, &n.TelnetIPv6Success, &n.TelnetIPv6ResponseMs, &n.TelnetIPv6Address, &n.TelnetIPv6Error,
			&n.FTPIPv4Tested, &n.FTPIPv4Success, &n.FTPIPv4ResponseMs, &n.FTPIPv4Address, &n.FTPIPv4Error,
			&n.FTPIPv6Tested, &n.FTPIPv6Success, &n.FTPIPv6ResponseMs, &n.FTPIPv6Address, &n.FTPIPv6Error,
			&n.VModemIPv4Tested, &n.VModemIPv4Success, &n.VModemIPv4ResponseMs, &n.VModemIPv4Address, &n.VModemIPv4Error,
			&n.VModemIPv6Tested, &n.VModemIPv6Success, &n.VModemIPv6ResponseMs, &n.VModemIPv6Address, &n.VModemIPv6Error,
			&n.IsOperational, &n.HasConnectivityIssues, &n.AddressValidated,
			&n.TestedHostname, &n.HostnameIndex, &n.IsAggregated,
			&n.TotalHostnames, &n.HostnamesTested, &n.HostnamesOperational,
			&n.FTPAnonSuccess,
			&n.NetworkAddresses,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan network node: %w", err)
		}
		results = append(results, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating network node rows: %w", err)
	}

	return results, nil
}
