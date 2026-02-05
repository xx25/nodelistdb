-- NodelistDB ClickHouse Schema
-- This file contains the complete schema for the NodelistDB database
-- Use this to initialize a new ClickHouse installation

-- Create database
CREATE DATABASE IF NOT EXISTS nodelistdb;

-- Nodes table - stores parsed nodelist data
CREATE TABLE IF NOT EXISTS nodelistdb.nodes
(
    `zone` Int32,
    `net` Int32,
    `node` Int32,
    `nodelist_date` Date,
    `day_number` Int32,
    `system_name` String,
    `location` String,
    `sysop_name` String,
    `phone` String,
    `node_type` LowCardinality(String),
    `region` Nullable(Int32),
    `max_speed` UInt32 DEFAULT 0,
    `is_cm` Bool DEFAULT false,
    `is_mo` Bool DEFAULT false,
    `flags` Array(String) DEFAULT [],
    `modem_flags` Array(String) DEFAULT [],
    `has_inet` Bool DEFAULT false,
    `internet_config` JSON DEFAULT '{}',
    `conflict_sequence` Int32 DEFAULT 0,
    `has_conflict` Bool DEFAULT false,
    `fts_id` String,
    `raw_line` String DEFAULT '',
    `year` UInt16 MATERIALIZED toYear(nodelist_date),
    `json_protocols` Array(String) MATERIALIZED extractAll(toString(internet_config), '"([A-Z]{3})"'),
    INDEX idx_nodes_date nodelist_date TYPE minmax GRANULARITY 1,
    INDEX idx_nodes_system system_name TYPE bloom_filter GRANULARITY 1,
    INDEX idx_nodes_location location TYPE bloom_filter GRANULARITY 1,
    INDEX idx_nodes_sysop sysop_name TYPE bloom_filter GRANULARITY 1,
    INDEX idx_nodes_type node_type TYPE set(100) GRANULARITY 1,
    INDEX idx_nodes_fts_id fts_id TYPE bloom_filter GRANULARITY 1,
    INDEX idx_fts_location location TYPE bloom_filter GRANULARITY 1,
    INDEX idx_fts_sysop sysop_name TYPE bloom_filter GRANULARITY 1,
    INDEX idx_fts_system system_name TYPE bloom_filter GRANULARITY 1,
    INDEX idx_year year TYPE minmax GRANULARITY 1,
    INDEX idx_flags_bloom flags TYPE bloom_filter GRANULARITY 1,
    INDEX idx_modem_flags_bloom modem_flags TYPE bloom_filter GRANULARITY 1,
    INDEX idx_json_protocols_bloom json_protocols TYPE bloom_filter GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(nodelist_date)
ORDER BY (zone, net, node, nodelist_date, conflict_sequence)
SETTINGS index_granularity = 8192;

-- Node test results table - stores connectivity test results
CREATE TABLE IF NOT EXISTS nodelistdb.node_test_results
(
    `test_time` DateTime,
    `test_date` Date DEFAULT toDate(test_time),
    `zone` UInt16,
    `net` UInt16,
    `node` UInt16,
    `address` String,
    `hostname` String,
    `resolved_ipv4` Array(String),
    `resolved_ipv6` Array(String),
    `dns_error` String,
    `country` String,
    `country_code` String,
    `city` String,
    `region` String,
    `latitude` Float32,
    `longitude` Float32,
    `isp` String,
    `org` String,
    `asn` UInt32,
    `binkp_tested` Bool,
    `binkp_success` Bool,
    `binkp_response_ms` UInt32,
    `binkp_system_name` String,
    `binkp_sysop` String,
    `binkp_location` String,
    `binkp_version` String,
    `binkp_addresses` Array(String),
    `binkp_capabilities` Array(String),
    `binkp_error` String,
    `ifcico_tested` Bool,
    `ifcico_success` Bool,
    `ifcico_response_ms` UInt32,
    `ifcico_mailer_info` String,
    `ifcico_system_name` String,
    `ifcico_addresses` Array(String),
    `ifcico_response_type` String,
    `ifcico_error` String,
    `telnet_tested` Bool,
    `telnet_success` Bool,
    `telnet_response_ms` UInt32,
    `telnet_error` String,
    `ftp_tested` Bool,
    `ftp_success` Bool,
    `ftp_response_ms` UInt32,
    `ftp_error` String,
    `ftp_anon_success` Nullable(Bool) DEFAULT NULL,
    `vmodem_tested` Bool,
    `vmodem_success` Bool,
    `vmodem_response_ms` UInt32,
    `vmodem_error` String,
    `is_operational` Bool,
    `has_connectivity_issues` Bool,
    `address_validated` Bool,
    `binkp_ipv4_tested` Bool DEFAULT 0,
    `binkp_ipv4_success` Bool DEFAULT 0,
    `binkp_ipv4_response_ms` UInt32 DEFAULT 0,
    `binkp_ipv4_address` String DEFAULT '',
    `binkp_ipv4_error` String DEFAULT '',
    `binkp_ipv6_tested` Bool DEFAULT 0,
    `binkp_ipv6_success` Bool DEFAULT 0,
    `binkp_ipv6_response_ms` UInt32 DEFAULT 0,
    `binkp_ipv6_address` String DEFAULT '',
    `binkp_ipv6_error` String DEFAULT '',
    `ifcico_ipv4_tested` Bool DEFAULT 0,
    `ifcico_ipv4_success` Bool DEFAULT 0,
    `ifcico_ipv4_response_ms` UInt32 DEFAULT 0,
    `ifcico_ipv4_address` String DEFAULT '',
    `ifcico_ipv4_error` String DEFAULT '',
    `ifcico_ipv6_tested` Bool DEFAULT 0,
    `ifcico_ipv6_success` Bool DEFAULT 0,
    `ifcico_ipv6_response_ms` UInt32 DEFAULT 0,
    `ifcico_ipv6_address` String DEFAULT '',
    `ifcico_ipv6_error` String DEFAULT '',
    `telnet_ipv4_tested` Bool DEFAULT 0,
    `telnet_ipv4_success` Bool DEFAULT 0,
    `telnet_ipv4_response_ms` UInt32 DEFAULT 0,
    `telnet_ipv4_address` String DEFAULT '',
    `telnet_ipv4_error` String DEFAULT '',
    `telnet_ipv6_tested` Bool DEFAULT 0,
    `telnet_ipv6_success` Bool DEFAULT 0,
    `telnet_ipv6_response_ms` UInt32 DEFAULT 0,
    `telnet_ipv6_address` String DEFAULT '',
    `telnet_ipv6_error` String DEFAULT '',
    `tested_hostname` String DEFAULT '',
    `hostname_index` Int32 DEFAULT -1,
    `is_aggregated` Bool DEFAULT false,
    `total_hostnames` Int32 DEFAULT 1,
    `hostnames_tested` Int32 DEFAULT 1,
    `hostnames_operational` Int32 DEFAULT 0,
    `ipv4_skipped` Bool DEFAULT false,
    `ftp_ipv4_tested` Bool DEFAULT false,
    `ftp_ipv4_success` Bool DEFAULT false,
    `ftp_ipv4_response_ms` UInt32 DEFAULT 0,
    `ftp_ipv4_address` String DEFAULT '',
    `ftp_ipv4_error` String DEFAULT '',
    `ftp_ipv6_tested` Bool DEFAULT false,
    `ftp_ipv6_success` Bool DEFAULT false,
    `ftp_ipv6_response_ms` UInt32 DEFAULT 0,
    `ftp_ipv6_address` String DEFAULT '',
    `ftp_ipv6_error` String DEFAULT '',
    `vmodem_ipv4_tested` Bool DEFAULT false,
    `vmodem_ipv4_success` Bool DEFAULT false,
    `vmodem_ipv4_response_ms` UInt32 DEFAULT 0,
    `vmodem_ipv4_address` String DEFAULT '',
    `vmodem_ipv4_error` String DEFAULT '',
    `vmodem_ipv6_tested` Bool DEFAULT false,
    `vmodem_ipv6_success` Bool DEFAULT false,
    `vmodem_ipv6_response_ms` UInt32 DEFAULT 0,
    `vmodem_ipv6_address` String DEFAULT '',
    `vmodem_ipv6_error` String DEFAULT '',
    -- Test source identification (daemon, cli, manual)
    `test_source` LowCardinality(String) DEFAULT 'daemon',

    -- Modem (PSTN) test results
    `modem_tested` Bool DEFAULT false,
    `modem_success` Bool DEFAULT false,
    `modem_response_ms` UInt32 DEFAULT 0,
    `modem_system_name` String DEFAULT '',
    `modem_mailer_info` String DEFAULT '',
    `modem_addresses` Array(String) DEFAULT [],
    `modem_address_valid` Bool DEFAULT false,
    `modem_response_type` String DEFAULT '',
    `modem_software_source` String DEFAULT '',
    `modem_error` String DEFAULT '',
    `modem_connect_speed` UInt32 DEFAULT 0,
    `modem_protocol` String DEFAULT '',
    `modem_caller_id` String DEFAULT '',
    `modem_phone_dialed` String DEFAULT '',
    `modem_ring_count` UInt8 DEFAULT 0,
    `modem_carrier_time_ms` UInt32 DEFAULT 0,
    `modem_used` String DEFAULT '',
    `modem_match_reason` String DEFAULT '',
    `modem_line_stats` String DEFAULT '',

    -- Modem operator/routing info (for carrier comparison)
    `modem_operator_name` String DEFAULT '',
    `modem_operator_prefix` String DEFAULT '',
    `modem_dial_time_ms` UInt32 DEFAULT 0,
    `modem_emsi_time_ms` UInt32 DEFAULT 0,
    `modem_connect_string` String DEFAULT '',

    -- Modem line statistics (parsed from AT commands)
    `modem_tx_speed` UInt32 DEFAULT 0,
    `modem_rx_speed` UInt32 DEFAULT 0,
    `modem_compression` String DEFAULT '',
    `modem_modulation` String DEFAULT '',
    `modem_line_quality` UInt8 DEFAULT 0,
    `modem_snr` Float32 DEFAULT 0,
    `modem_rx_level` Int16 DEFAULT 0,
    `modem_tx_power` Int16 DEFAULT 0,
    `modem_round_trip_delay` UInt16 DEFAULT 0,
    `modem_local_retrains` UInt8 DEFAULT 0,
    `modem_remote_retrains` UInt8 DEFAULT 0,
    `modem_termination_reason` String DEFAULT '',
    `modem_stats_notes` String DEFAULT '',

    -- Modem EMSI remote system details
    `modem_remote_location` String DEFAULT '',
    `modem_remote_sysop` String DEFAULT '',

    -- AudioCodes VoIP CDR quality metrics
    `modem_cdr_session_id` String DEFAULT '',
    `modem_cdr_codec` String DEFAULT '',
    `modem_cdr_rtp_jitter_ms` UInt16 DEFAULT 0,
    `modem_cdr_rtp_delay_ms` UInt16 DEFAULT 0,
    `modem_cdr_packet_loss` UInt8 DEFAULT 0,
    `modem_cdr_remote_packet_loss` UInt8 DEFAULT 0,
    `modem_cdr_local_mos` UInt8 DEFAULT 0,
    `modem_cdr_remote_mos` UInt8 DEFAULT 0,
    `modem_cdr_local_r_factor` UInt8 DEFAULT 0,
    `modem_cdr_remote_r_factor` UInt8 DEFAULT 0,
    `modem_cdr_term_reason` String DEFAULT '',
    `modem_cdr_term_category` String DEFAULT '',

    -- Asterisk CDR call routing info
    `modem_ast_disposition` String DEFAULT '',
    `modem_ast_peer` String DEFAULT '',
    `modem_ast_duration` UInt16 DEFAULT 0,
    `modem_ast_billsec` UInt16 DEFAULT 0,
    `modem_ast_hangup_cause` UInt8 DEFAULT 0,
    `modem_ast_hangup_source` String DEFAULT '',
    `modem_ast_early_media` Bool DEFAULT false,
    INDEX idx_date test_date TYPE minmax GRANULARITY 1,
    INDEX idx_zone_net (zone, net) TYPE minmax GRANULARITY 1,
    INDEX idx_operational is_operational TYPE minmax GRANULARITY 1,
    INDEX idx_binkp_ipv6 (binkp_ipv6_tested, binkp_ipv6_success) TYPE minmax GRANULARITY 1,
    INDEX idx_ifcico_ipv6 (ifcico_ipv6_tested, ifcico_ipv6_success) TYPE minmax GRANULARITY 1,
    INDEX idx_telnet_ipv6 (telnet_ipv6_tested, telnet_ipv6_success) TYPE minmax GRANULARITY 1,
    INDEX idx_hostname_test (test_date, zone, net, node, tested_hostname) TYPE minmax GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(test_date)
ORDER BY (test_date, zone, net, node)
SETTINGS index_granularity = 8192;

-- Daily statistics table - aggregated test statistics per day
CREATE TABLE IF NOT EXISTS nodelistdb.node_test_daily_stats
(
    `date` Date,
    `total_nodes_tested` UInt32,
    `nodes_with_binkp` UInt32,
    `nodes_with_ifcico` UInt32,
    `nodes_operational` UInt32,
    `nodes_with_issues` UInt32,
    `nodes_dns_failed` UInt32,
    `avg_binkp_response_ms` Float32,
    `avg_ifcico_response_ms` Float32,
    `countries` Map(String, UInt32),
    `isps` Map(String, UInt32),
    `protocol_stats` Map(String, UInt32),
    `error_types` Map(String, UInt32)
)
ENGINE = SummingMergeTree
ORDER BY date
SETTINGS index_granularity = 8192;

-- Flag statistics table - tracks flag usage across nodelists
CREATE TABLE IF NOT EXISTS nodelistdb.flag_statistics
(
    `flag` String,
    `year` UInt16,
    `nodelist_date` Date,
    `unique_nodes` UInt32,
    `first_zone` Int32,
    `first_net` Int32,
    `first_node` Int32,
    `first_nodelist_date` Date,
    `first_day_number` Int32,
    `first_system_name` String,
    `first_location` String,
    `first_sysop_name` String,
    `first_phone` String,
    `first_node_type` String,
    `first_region` Nullable(Int32),
    `first_max_speed` UInt32,
    `first_is_cm` Bool,
    `first_is_mo` Bool,
    `first_has_inet` Bool,
    `first_raw_line` String,
    `total_nodes_in_year` UInt32 DEFAULT 0
)
ENGINE = ReplacingMergeTree(nodelist_date)
PARTITION BY year
ORDER BY (flag, year, nodelist_date)
SETTINGS index_granularity = 8192;

-- Modem caller runtime status table
-- Tracks heartbeats and runtime statistics for modem daemons
-- Configuration (prefixes, API keys) is stored in config.yaml, not here
CREATE TABLE IF NOT EXISTS nodelistdb.modem_caller_status
(
    `caller_id` String,                                         -- References config.yaml caller_id
    `last_heartbeat` DateTime,
    `status` Enum8('active' = 1, 'inactive' = 2, 'maintenance' = 3),
    `modems_available` UInt8 DEFAULT 0,
    `modems_in_use` UInt8 DEFAULT 0,
    `tests_completed` UInt32 DEFAULT 0,
    `tests_failed` UInt32 DEFAULT 0,
    `last_test_time` DateTime DEFAULT toDateTime(0),
    `updated_at` DateTime
)
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY caller_id
SETTINGS index_granularity = 8192;

-- Modem test queue table
-- Nodes are pre-assigned to specific daemons on queue entry
CREATE TABLE IF NOT EXISTS nodelistdb.modem_test_queue
(
    -- Node identification
    `zone` UInt16,
    `net` UInt16,
    `node` UInt16,
    `conflict_sequence` UInt8 DEFAULT 0,                        -- Matches nodelist duplicate handling
    `phone` String,
    `phone_normalized` String,                                  -- "+74951234567" for prefix matching
    `modem_flags` Array(String),                                -- [V34, V32B, ZYX] - passed to daemon
    `flags` Array(String),                                      -- General node flags [CM, XA, TAN]

    -- Time availability (denormalized from flags for query efficiency)
    `is_cm` Bool DEFAULT false,                                 -- Has CM flag (24/7 available)
    `time_flags` Array(String),                                 -- T-flags like [T, A, N]

    -- Pre-assignment (replaces claiming)
    `assigned_to` String,                                       -- caller_id of assigned daemon
    `assigned_at` DateTime,

    -- Scheduling
    `priority` UInt8,
    `retry_count` UInt8,
    `next_attempt_after` DateTime,                              -- For retry backoff

    -- Status
    `status` Enum8('pending' = 1, 'in_progress' = 2, 'completed' = 3, 'failed' = 4),
    `in_progress_since` DateTime DEFAULT toDateTime(0),         -- For stale detection
    `last_tested_at` DateTime DEFAULT toDateTime(0),
    `last_error` String DEFAULT '',

    -- Metadata
    `created_at` DateTime,
    `updated_at` DateTime,

    -- Secondary index for daemon queries (WHERE assigned_to = ? AND status = ?)
    INDEX idx_assigned_status (assigned_to, status) TYPE set(10) GRANULARITY 4
)
ENGINE = MergeTree()
ORDER BY (zone, net, node, conflict_sequence)
SETTINGS index_granularity = 8192;
