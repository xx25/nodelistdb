-- Migration 008: Multi-network (FTN domain) support
-- Adds a `domain` dimension so several FTN networks (fidonet, fsxnet, ...) can
-- coexist in one database. Existing rows default to 'fidonet'.
--
-- ALTER ADD COLUMN ... DEFAULT is metadata-only on MergeTree, so the nodes and
-- node_test_results changes are safe on production-size tables and old binaries
-- keep working (they use explicit insert column lists; new columns get defaults).

-- Nodes: which network's nodelist this row was parsed from
ALTER TABLE nodelistdb.nodes ADD COLUMN IF NOT EXISTS `domain` LowCardinality(String) DEFAULT 'fidonet';

-- Test results: domain of the tested node identity, plus provenance for results
-- derived from another node's test via announced AKAs (empty = direct test)
ALTER TABLE nodelistdb.node_test_results ADD COLUMN IF NOT EXISTS `domain` LowCardinality(String) DEFAULT 'fidonet';
ALTER TABLE nodelistdb.node_test_results ADD COLUMN IF NOT EXISTS `derived_from_address` String DEFAULT '';

-- flag_statistics is ReplacingMergeTree: `domain` MUST be part of ORDER BY,
-- otherwise background merges keep only one network's row per (flag, year, date).
-- ClickHouse cannot append a defaulted column to a sort key, so recreate + swap.
--
-- IMPORTANT: the copy + rename below is not atomic. Writes landing between the
-- INSERT snapshot and the RENAME go to the old table and are silently lost.
-- Pause parser imports (cron / sync_nodelists.sh) while running this migration.
CREATE TABLE IF NOT EXISTS nodelistdb.flag_statistics_new
(
    `flag` String,
    `year` UInt16,
    `nodelist_date` Date,
    `domain` LowCardinality(String) DEFAULT 'fidonet',
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
ORDER BY (flag, year, nodelist_date, domain)
SETTINGS index_granularity = 8192;

INSERT INTO nodelistdb.flag_statistics_new
    (flag, year, nodelist_date, domain, unique_nodes,
     first_zone, first_net, first_node, first_nodelist_date, first_day_number,
     first_system_name, first_location, first_sysop_name, first_phone,
     first_node_type, first_region, first_max_speed, first_is_cm, first_is_mo,
     first_has_inet, first_raw_line, total_nodes_in_year)
SELECT
    flag, year, nodelist_date, 'fidonet', unique_nodes,
    first_zone, first_net, first_node, first_nodelist_date, first_day_number,
    first_system_name, first_location, first_sysop_name, first_phone,
    first_node_type, first_region, first_max_speed, first_is_cm, first_is_mo,
    first_has_inet, first_raw_line, total_nodes_in_year
FROM nodelistdb.flag_statistics;

RENAME TABLE nodelistdb.flag_statistics TO nodelistdb.flag_statistics_old,
             nodelistdb.flag_statistics_new TO nodelistdb.flag_statistics;

-- After verifying the swap:
-- DROP TABLE nodelistdb.flag_statistics_old;
