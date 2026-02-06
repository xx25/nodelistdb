-- Migration 006: Add domain WHOIS cache table
-- Stores WHOIS lookup results for domains used by FidoNet nodes
-- Used by testdaemon (writes) and server analytics page (reads)

CREATE TABLE IF NOT EXISTS nodelistdb.domain_whois_cache
(
    `domain` String,                        -- Registrable domain (e.g., "example.com")
    `expiration_date` Nullable(DateTime),   -- Domain expiration date (NULL if unknown/unparseable)
    `creation_date` Nullable(DateTime),     -- Domain creation date (NULL if unknown)
    `registrar` String DEFAULT '',          -- Domain registrar name
    `whois_status` String DEFAULT '',       -- Raw WHOIS status string
    `check_time` DateTime DEFAULT now(),    -- When this WHOIS lookup was performed
    `check_error` String DEFAULT ''         -- Error message if lookup failed
)
ENGINE = ReplacingMergeTree(check_time)
ORDER BY domain
TTL check_time + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;
