package storage

import (
	"context"
	"fmt"
	"strings"
)

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
			ftp_anon_success Nullable(Bool) DEFAULT NULL,

			vmodem_tested Bool,
			vmodem_success Bool,
			vmodem_response_ms UInt32,
			vmodem_error String,

			is_operational Bool,
			has_connectivity_issues Bool,
			address_validated Bool,

			ipv4_skipped Bool DEFAULT false,

			binkp_ipv4_addresses Array(String),
			binkp_ipv6_addresses Array(String),
			ifcico_ipv4_addresses Array(String),
			ifcico_ipv6_addresses Array(String),
			address_validated_ipv4 Bool,
			address_validated_ipv6 Bool,

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
