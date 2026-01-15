package storage

import (
	"context"
	"fmt"
	"strings"
)

// initModemSchema creates modem-related tables and adds columns to existing tables
func (s *ClickHouseStorage) initModemSchema(ctx context.Context) error {
	// Create modem tables
	tables := []string{
		// Modem caller runtime status table
		`CREATE TABLE IF NOT EXISTS modem_caller_status (
			caller_id String,
			last_heartbeat DateTime,
			status Enum8('active' = 1, 'inactive' = 2, 'maintenance' = 3),
			modems_available UInt8 DEFAULT 0,
			modems_in_use UInt8 DEFAULT 0,
			tests_completed UInt32 DEFAULT 0,
			tests_failed UInt32 DEFAULT 0,
			last_test_time DateTime DEFAULT toDateTime(0),
			updated_at DateTime
		) ENGINE = ReplacingMergeTree(updated_at)
		ORDER BY caller_id`,

		// Modem test queue table
		`CREATE TABLE IF NOT EXISTS modem_test_queue (
			zone UInt16,
			net UInt16,
			node UInt16,
			conflict_sequence UInt8 DEFAULT 0,
			phone String,
			phone_normalized String,
			modem_flags Array(String),
			flags Array(String),
			is_cm Bool DEFAULT false,
			time_flags Array(String),
			assigned_to String,
			assigned_at DateTime,
			priority UInt8,
			retry_count UInt8,
			next_attempt_after DateTime,
			status Enum8('pending' = 1, 'in_progress' = 2, 'completed' = 3, 'failed' = 4),
			in_progress_since DateTime DEFAULT toDateTime(0),
			last_tested_at DateTime DEFAULT toDateTime(0),
			last_error String DEFAULT '',
			created_at DateTime,
			updated_at DateTime,
			INDEX idx_assigned_status (assigned_to, status) TYPE set(10) GRANULARITY 4
		) ENGINE = MergeTree()
		ORDER BY (zone, net, node, conflict_sequence)`,
	}

	for _, table := range tables {
		if err := s.conn.Exec(ctx, table); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("failed to create modem table: %w", err)
			}
		}
	}

	// Add modem columns to node_test_results table (for existing deployments)
	// These are ALTER TABLE ADD COLUMN statements that will be skipped if columns exist
	modemColumns := []struct {
		name       string
		definition string
	}{
		{"modem_tested", "Bool DEFAULT false"},
		{"modem_success", "Bool DEFAULT false"},
		{"modem_response_ms", "UInt32 DEFAULT 0"},
		{"modem_system_name", "String DEFAULT ''"},
		{"modem_mailer_info", "String DEFAULT ''"},
		{"modem_addresses", "Array(String) DEFAULT []"},
		{"modem_address_valid", "Bool DEFAULT false"},
		{"modem_response_type", "String DEFAULT ''"},
		{"modem_software_source", "String DEFAULT ''"},
		{"modem_error", "String DEFAULT ''"},
		{"modem_connect_speed", "UInt32 DEFAULT 0"},
		{"modem_protocol", "String DEFAULT ''"},
		{"modem_caller_id", "String DEFAULT ''"},
		{"modem_phone_dialed", "String DEFAULT ''"},
		{"modem_ring_count", "UInt8 DEFAULT 0"},
		{"modem_carrier_time_ms", "UInt32 DEFAULT 0"},
		{"modem_used", "String DEFAULT ''"},
		{"modem_match_reason", "String DEFAULT ''"},
	}

	for _, col := range modemColumns {
		query := fmt.Sprintf("ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS %s %s", col.name, col.definition)
		if err := s.conn.Exec(ctx, query); err != nil {
			// Ignore "already exists" errors
			if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "duplicate column name") {
				return fmt.Errorf("failed to add modem column %s: %w", col.name, err)
			}
		}
	}

	return nil
}
