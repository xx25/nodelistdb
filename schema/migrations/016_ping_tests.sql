-- Migration 016: FTS-4010 netmail PING/TRACE measurements
--
-- Backs the /analytics/pingtrace report. The testdaemon sends a netmail
-- addressed to user "PING" to every node flying the PING flag (via the
-- fidomail control API on the same host) and later reads the robots'
-- answers back out of fidomail's inbox. One row per ping sent lives in
-- ping_tests and is REPLACED as its state advances (queued -> sent ->
-- pong | ndr | timeout); every inbound reply the poller saw -- pong,
-- trace notice, NDR, or unmatched -- is kept verbatim in ping_replies as
-- the evidence behind the summary.
--
-- Purely additive: two new tables, nothing existing touched. Safe to run
-- before or after deploying new binaries.
--
-- Both engines are ReplacingMergeTree versioned by updated_at, so readers
-- must use FINAL (the tables are small: at most a few hundred rows a
-- fortnight). A DateTime of 0 (1970-01-01) means "not yet" on the
-- optional timestamps; the Go side never writes NULL.
--
-- Paths are stored as parallel arrays, one element per FTS-4009 Via line:
-- out_* is the outbound chain the destination robot quoted back (our node
-- first, the destination last), back_* is the reply's own Via chain (the
-- return path). An element with an empty address is a Via line that
-- carried none (some gateways write only a program name); the raw line
-- is always kept alongside.

CREATE TABLE IF NOT EXISTS nodelistdb.ping_tests
(
    `domain` LowCardinality(String),            -- FTN network of the pinged node
    `zone` UInt16,
    `net` UInt16,
    `node` UInt16,
    `address` String,                           -- "zone:net/node"
    `mode` LowCardinality(String),              -- routed | direct
    `sent_time` DateTime,                       -- when the testdaemon asked fidomail to send it
    `token` String,                             -- per-ping tag carried in the subject
    `msgid` String,                             -- FTS-0009 MSGID of the ping
    `fidomail_message_id` UInt64,               -- fidomail's row id, for delivery-state polling
    `first_hop` String DEFAULT '',              -- the link fidomail queued it to
    `route_source` String DEFAULT '',           -- how that link was chosen (default-route, rule, direct-nodelist, ...)
    `status` LowCardinality(String),            -- queued | sent | failed | pong | ndr | timeout
    `dispatched_time` DateTime DEFAULT toDateTime(0), -- when the first hop took the packet
    `reply_time` DateTime DEFAULT toDateTime(0),      -- when fidomail stored the pong
    `rtt_seconds` UInt32 DEFAULT 0,             -- reply_time - sent_time
    `reply_message_id` UInt64 DEFAULT 0,        -- fidomail id of the pong
    `reply_msgid` String DEFAULT '',
    `reply_from_name` String DEFAULT '',
    `reply_from_addr` String DEFAULT '',        -- the AKA the robot answered from
    `robot_pid` String DEFAULT '',              -- PID kludge of the pong
    `robot_tearline` String DEFAULT '',
    `out_hops` Array(String) DEFAULT [],        -- outbound path, as quoted by the robot
    `out_hop_times` Array(DateTime) DEFAULT [],
    `out_hop_software` Array(String) DEFAULT [],
    `out_vias_raw` Array(String) DEFAULT [],
    `back_hops` Array(String) DEFAULT [],       -- return path, the pong's own Via chain
    `back_hop_times` Array(DateTime) DEFAULT [],
    `back_hop_software` Array(String) DEFAULT [],
    `back_vias_raw` Array(String) DEFAULT [],
    `trace_count` UInt32 DEFAULT 0,             -- TRACE notices received for this ping
    `error` String DEFAULT '',
    `updated_at` DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (domain, zone, net, node, mode, sent_time)
TTL sent_time + INTERVAL 730 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS nodelistdb.ping_replies
(
    `fidomail_message_id` UInt64,               -- fidomail's row id: the dedup key
    `kind` LowCardinality(String),              -- pong | trace | ndr | unmatched
    `ping_domain` LowCardinality(String),       -- the ping it answers (zero values when unmatched)
    `ping_zone` UInt16 DEFAULT 0,
    `ping_net` UInt16 DEFAULT 0,
    `ping_node` UInt16 DEFAULT 0,
    `ping_sent_time` DateTime DEFAULT toDateTime(0),
    `ping_msgid` String DEFAULT '',
    `msgid` String DEFAULT '',
    `reply_id` String DEFAULT '',               -- REPLY kludge
    `from_name` String,
    `from_addr` String,                         -- "zone:net/node[.point]"
    `to_name` String,
    `subject` String,
    `body` String,                              -- capped at 16 KiB by the sender API
    `date` DateTime,                            -- the message's own date
    `received_at` DateTime,                     -- when fidomail stored it
    `pid` String DEFAULT '',
    `tearline` String DEFAULT '',
    `vias` Array(String) DEFAULT [],            -- the reply's own Via chain, raw
    `hops` Array(String) DEFAULT [],            -- the path quoted in the body (trace notices carry it too)
    `hop_times` Array(DateTime) DEFAULT [],
    `hop_software` Array(String) DEFAULT [],
    `updated_at` DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY fidomail_message_id
TTL received_at + INTERVAL 730 DAY
SETTINGS index_granularity = 8192;
