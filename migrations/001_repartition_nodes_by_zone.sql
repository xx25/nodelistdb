-- Migration 001: Repartition nodes table from toYYYYMM(nodelist_date) to zone
--
-- Why: Point lookups (WHERE zone=? AND net=? AND node=?) currently open all
-- ~438 monthly partitions. With zone partitioning (7 partitions), they touch 1.
-- Date-only queries (WHERE nodelist_date=?) go from 1 to 7 partitions, but
-- those are cached and the set() skip index compensates.
--
-- Prerequisites:
--   1. Stop the parser (no writes during migration)
--   2. Verify disk space: need ~900MB temporary for nodes_v2
--
-- Estimated time: 2-5 minutes for 31M rows

-- Step 1: Create the new table with zone partitioning
CREATE TABLE IF NOT EXISTS nodes_v2 (
    zone Int32,
    net Int32,
    node Int32,
    nodelist_date Date,
    day_number Int32,
    system_name String,
    location String,
    sysop_name String,
    phone String,
    node_type LowCardinality(String),
    region Nullable(Int32),
    max_speed UInt32 DEFAULT 0,

    is_cm Bool DEFAULT false,
    is_mo Bool DEFAULT false,

    flags Array(String) DEFAULT [],
    modem_flags Array(String) DEFAULT [],

    has_inet Bool DEFAULT false,
    internet_config JSON DEFAULT '{}',

    conflict_sequence Int32 DEFAULT 0,
    has_conflict Bool DEFAULT false,

    fts_id String,
    raw_line String DEFAULT '',

    -- Materialized columns
    year UInt16 MATERIALIZED toYear(nodelist_date),
    json_protocols Array(String) MATERIALIZED extractAll(toString(internet_config), '"([A-Z]{3})"'),

    -- Inline indexes
    INDEX idx_year year TYPE minmax GRANULARITY 1,
    INDEX idx_flags_bloom flags TYPE bloom_filter GRANULARITY 1,
    INDEX idx_modem_flags_bloom modem_flags TYPE bloom_filter GRANULARITY 1,
    INDEX idx_json_protocols_bloom json_protocols TYPE bloom_filter GRANULARITY 1,
    INDEX idx_nodelist_date nodelist_date TYPE set(100) GRANULARITY 4
) ENGINE = MergeTree()
ORDER BY (zone, net, node, nodelist_date, conflict_sequence)
PARTITION BY zone
SETTINGS index_granularity = 8192;

-- Step 2: Copy all data
INSERT INTO nodes_v2
SELECT
    zone, net, node, nodelist_date, day_number,
    system_name, location, sysop_name, phone, node_type, region, max_speed,
    is_cm, is_mo,
    flags, modem_flags,
    has_inet, internet_config,
    conflict_sequence, has_conflict,
    fts_id, raw_line
FROM nodes;

-- Step 3: Verify row counts match
-- Run manually and compare before proceeding:
--   SELECT 'old' AS tbl, count() FROM nodes UNION ALL SELECT 'new', count() FROM nodes_v2;

-- Step 4: Re-create secondary indexes
CREATE INDEX IF NOT EXISTS idx_nodes_date ON nodes_v2(nodelist_date) TYPE minmax GRANULARITY 1;
CREATE INDEX IF NOT EXISTS idx_nodes_date_set ON nodes_v2(nodelist_date) TYPE set(100) GRANULARITY 4;
CREATE INDEX IF NOT EXISTS idx_nodes_system ON nodes_v2(system_name) TYPE bloom_filter GRANULARITY 1;
CREATE INDEX IF NOT EXISTS idx_nodes_location ON nodes_v2(location) TYPE bloom_filter GRANULARITY 1;
CREATE INDEX IF NOT EXISTS idx_nodes_sysop ON nodes_v2(sysop_name) TYPE bloom_filter GRANULARITY 1;
CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes_v2(node_type) TYPE set(100) GRANULARITY 1;
CREATE INDEX IF NOT EXISTS idx_nodes_fts_id ON nodes_v2(fts_id) TYPE bloom_filter GRANULARITY 1;

-- Step 5: Atomic swap
RENAME TABLE nodes TO nodes_old, nodes_v2 TO nodes;

-- Step 6: Verify point lookup performance
-- Run manually:
--   SELECT * FROM nodes WHERE zone = 2 AND net = 5080 AND node = 31 ORDER BY nodelist_date DESC LIMIT 5;

-- Step 7: Drop old table (after confirming everything works)
-- DROP TABLE nodes_old;
