-- Migration 007: Add PSTN dead nodes table
-- Stores nodes with permanently disconnected/invalid phone numbers
-- Used by modem testers to skip dead nodes via the API

CREATE TABLE IF NOT EXISTS nodelistdb.pstn_dead_nodes
(
    `zone` Int32,
    `net` Int32,
    `node` Int32,
    `reason` String DEFAULT '',
    `marked_by` String DEFAULT '',
    `marked_at` DateTime DEFAULT now(),
    `is_active` Bool DEFAULT true,
    `updated_at` DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (zone, net, node)
SETTINGS index_granularity = 8192;
