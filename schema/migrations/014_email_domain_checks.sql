-- Migration 014: email domain verification cache
--
-- Backs the /analytics/email report. The testdaemon writes one row per mail
-- domain published in a nodelist email flag (IEM, IMI, ITX, ISE, IUC, and the
-- non-standard EMA/EVY); the server reads them to show whether each published
-- address can still receive mail.
--
-- Purely additive: creates one new table, touches nothing existing. Safe to
-- run before or after deploying new binaries.
--
-- Keyed by domain rather than by address because every fact stored here is
-- domain-scoped under DNS-only checking -- two addresses at the same domain
-- always share the same MX state. If SMTP-level (per-mailbox) verification is
-- ever added, it belongs in a separate, address-keyed table.
--
-- The engine version is last_attempt_time, not check_time, so that a row is
-- replaced on every sweep even when the verdict itself has not moved. That is
-- what lets a transient DNS failure carry the previous good verdict forward
-- while still recording that the latest attempt failed: check_time says when
-- the verdict was established, last_attempt_time says when it was last
-- re-tested, and a non-empty check_error means the newest attempt failed and
-- the verdict shown is therefore stale.

CREATE TABLE IF NOT EXISTS nodelistdb.email_domain_checks
(
    `domain` String,                            -- Mail domain, lower-cased (e.g., "example.net")
    `status` LowCardinality(String),            -- ok | implicit_mx | degraded | no_domain | null_mx | no_mx | mx_unresolvable | invalid | error
    `detail` String DEFAULT '',                 -- Short human-readable explanation
    `ascii_domain` String DEFAULT '',           -- Punycode form actually queried, when it differs
    `mx_preferences` Array(UInt16) DEFAULT [],  -- Parallel arrays, ordered by preference
    `mx_hosts` Array(String) DEFAULT [],
    `mx_resolved` Array(UInt8) DEFAULT [],      -- 1 when that MX host has an A/AAAA record
    `check_time` DateTime DEFAULT now(),        -- When the current verdict was established
    `last_attempt_time` DateTime DEFAULT now(), -- When the domain was last re-tested
    `check_error` String DEFAULT ''             -- Non-empty when the latest attempt failed transiently
)
ENGINE = ReplacingMergeTree(last_attempt_time)
ORDER BY domain
TTL last_attempt_time + INTERVAL 180 DAY
SETTINGS index_granularity = 8192;
