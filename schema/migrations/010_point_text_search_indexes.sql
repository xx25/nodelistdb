-- Migration 010: usable text-search indexes for the points table
--
-- The original idx_pts_{sysop,system,loc} indexes were plain bloom_filter,
-- which ClickHouse only consults for equality/IN — the point search uses
-- case-insensitive substring matching, so those indexes never pruned a
-- single granule. Since the web search now queries points on every request,
-- replace them with ngrambf_v1 indexes over lowerUTF8(<column>); the query
-- side (query_builder_points.go) matches with
-- lowerUTF8(col) LIKE lowerUTF8(pattern), which skip indexes DO accelerate.
-- lowerUTF8 (not lower) preserves the previous ILIKE Unicode case folding
-- for Cyrillic pointlist entries.
--
-- Safe to run at any time: old binaries' ILIKE queries never used the
-- dropped indexes, and the new indexes are simply unused until the new
-- server binary is deployed. Search terms shorter than 3 characters yield
-- no trigrams and fall back to a full scan — correct, just not pruned.

ALTER TABLE nodelistdb.points DROP INDEX IF EXISTS idx_pts_sysop;
ALTER TABLE nodelistdb.points DROP INDEX IF EXISTS idx_pts_system;
ALTER TABLE nodelistdb.points DROP INDEX IF EXISTS idx_pts_loc;

ALTER TABLE nodelistdb.points ADD INDEX IF NOT EXISTS idx_pts_sysop_ngram  lowerUTF8(replaceAll(sysop_name, '_', ' ')) TYPE ngrambf_v1(3, 8192, 3, 0) GRANULARITY 1;
ALTER TABLE nodelistdb.points ADD INDEX IF NOT EXISTS idx_pts_system_ngram lowerUTF8(system_name) TYPE ngrambf_v1(3, 8192, 3, 0) GRANULARITY 1;
ALTER TABLE nodelistdb.points ADD INDEX IF NOT EXISTS idx_pts_loc_ngram    lowerUTF8(location)    TYPE ngrambf_v1(3, 8192, 3, 0) GRANULARITY 1;

-- Build the new indexes for already-existing parts. These are asynchronous
-- mutations: monitor with
--   SELECT command, is_done FROM system.mutations WHERE table = 'points';
ALTER TABLE nodelistdb.points MATERIALIZE INDEX idx_pts_sysop_ngram;
ALTER TABLE nodelistdb.points MATERIALIZE INDEX idx_pts_system_ngram;
ALTER TABLE nodelistdb.points MATERIALIZE INDEX idx_pts_loc_ngram;
