-- Migration 015: record what happens when a node's hostname fails to resolve
--
-- Purely additive. Before this, a DNS failure ended the test (test_executor.go
-- returned immediately), so the database could say "DNS broke" but never
-- "...and the node was still answering at the address it had yesterday".
-- That distinction is exactly what the nodelist static-IP-fallback question
-- turns on, and it was unobservable.
--
-- Fallback results are stored in their own columns and deliberately do NOT
-- feed is_operational: to a DNS-only mailer the node is unreachable, and every
-- existing reachability query must keep reporting it that way.

ALTER TABLE nodelistdb.node_test_results
    -- Classification of dns_error, so analysis stops depending on regex over
    -- the Go resolver's error strings: nxdomain | timeout | servfail | refused |
    -- no_answer | other. Empty when DNS succeeded.
    ADD COLUMN IF NOT EXISTS dns_error_kind LowCardinality(String) DEFAULT '',

    -- Was a fallback probe attempted after DNS failed, and against what.
    ADD COLUMN IF NOT EXISTS dns_fallback_attempted Bool DEFAULT false,
    ADD COLUMN IF NOT EXISTS dns_fallback_ipv4 Array(String) DEFAULT [],
    ADD COLUMN IF NOT EXISTS dns_fallback_ipv6 Array(String) DEFAULT [],

    -- Where the fallback address came from:
    --   'nodelist_literal' - an IP literal the sysop published in INA. This is
    --                       the proposed mechanism, measured as actually used.
    --   'last_known'       - the most recent address this hostname resolved to,
    --                       i.e. what a caching mailer would still have.
    ADD COLUMN IF NOT EXISTS dns_fallback_source LowCardinality(String) DEFAULT '',

    -- Age of the fallback address at probe time. The whole staleness question is
    -- "how correct is an address N hours old", so N has to be recorded per probe.
    -- 0 for nodelist_literal (published, not observed).
    ADD COLUMN IF NOT EXISTS dns_fallback_age_hours UInt32 DEFAULT 0,

    -- Did anything answer at the fallback address, and what.
    ADD COLUMN IF NOT EXISTS dns_fallback_success Bool DEFAULT false,
    ADD COLUMN IF NOT EXISTS dns_fallback_protocols Array(String) DEFAULT [],
    -- Whether the host that answered proved it was the right node by announcing
    -- the expected FTN address. Without this a reassigned IP that happens to run
    -- a mailer would count as a success, which is the exact failure mode that
    -- makes a stale fallback dangerous rather than merely useless.
    ADD COLUMN IF NOT EXISTS dns_fallback_address_validated Bool DEFAULT false,
    ADD COLUMN IF NOT EXISTS dns_fallback_error String DEFAULT '';
