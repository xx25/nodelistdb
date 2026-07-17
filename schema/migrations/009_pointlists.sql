-- Migration 009: Pointlist (FTS-5002) support
-- Adds two new tables; nothing existing is touched, so this migration is
-- additive and safe to run at any time (no import pause needed).
--
-- points: one row per point entry per pointlist file. Overlapping sources
-- (z2 aggregates the regional lists) are stored verbatim and resolved at
-- query time: readers pick the latest issue per source within a staleness
-- window, then LIMIT 1 BY address ordered by source_priority.
--
-- pointlist_files: the import gate. A file is "imported" only when its row
-- exists here; the row is registered after all chunks inserted successfully,
-- so a crash mid-import never looks like a completed file.

CREATE TABLE IF NOT EXISTS nodelistdb.points
(
    `domain`            LowCardinality(String) DEFAULT 'fidonet',
    `zone`              Int32,
    `net`               Int32,
    `node`              Int32,              -- boss node number
    `point`             Int32,
    `pointlist_date`    Date,
    `day_number`        Int32,
    `list_source`       LowCardinality(String),   -- 'r24','z2','r50','net244','nodelist',...
    `source_priority`   UInt8 DEFAULT 10,          -- 0 net-level, 10 regional, 20 zone rollup
    `source_format`     LowCardinality(String),    -- 'boss','poss','pvt','point','fakenet'
    `system_name`       String DEFAULT '',
    `location`          String DEFAULT '',
    `sysop_name`        String DEFAULT '',
    `phone`             String DEFAULT '',
    `max_speed`         UInt32 DEFAULT 0,
    `is_cm`             Bool DEFAULT false,
    `is_mo`             Bool DEFAULT false,
    `has_inet`          Bool DEFAULT false,
    `flags`             Array(String) DEFAULT [],
    `modem_flags`       Array(String) DEFAULT [],
    `internet_config`   String DEFAULT '',
    `conflict_sequence` UInt16 DEFAULT 0,          -- same 4D address twice in ONE file
    `has_conflict`      Bool DEFAULT false,
    `fts_id`            String,                    -- z:n/n.p@date#seq[@domain]
    `raw_line`          String DEFAULT '',

    INDEX idx_pts_sysop  sysop_name     TYPE bloom_filter GRANULARITY 1,
    INDEX idx_pts_system system_name    TYPE bloom_filter GRANULARITY 1,
    INDEX idx_pts_loc    location       TYPE bloom_filter GRANULARITY 1,
    INDEX idx_pts_fts    fts_id         TYPE bloom_filter GRANULARITY 1,
    INDEX idx_pts_date   pointlist_date TYPE minmax GRANULARITY 1
)
ENGINE = MergeTree()
PARTITION BY zone
-- domain IS in the sort key (unlike nodes, which was already populated when
-- multi-network support arrived and cannot change its ORDER BY)
ORDER BY (domain, zone, net, node, point, pointlist_date, conflict_sequence)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS nodelistdb.pointlist_files
(
    `domain`          LowCardinality(String),
    `list_source`     LowCardinality(String),
    `pointlist_date`  Date,
    `day_number`      Int32,
    `filename`        String,
    `source_format`   LowCardinality(String),
    `points_count`    UInt32,
    `bosses_count`    UInt32,
    `imported_at`     DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(imported_at)
ORDER BY (domain, list_source, pointlist_date)
SETTINGS index_granularity = 8192;
