-- 018_ping_reply_delivery.sql
--
-- Record on the ping row HOW its answer reached us.
--
-- ping_replies already carries the receipt provenance of every reply
-- (migration 017). This denormalises the answering reply's verdict onto
-- the ping itself, beside reply_from_name / robot_pid, which are there for
-- the same reason: the report renders one row per node from ping_tests and
-- must not join ping_replies per row to say what it shows.
--
-- What it shows is worth showing. An answer on tier B or C was handed to
-- us by a peer we hold no link with -- in practice the answering node
-- dialling this system directly -- rather than relayed back down our
-- uplink with the rest of the mail. That is a property of the ANSWER's
-- delivery, independent of the path the ping took getting there, and it is
-- not otherwise visible: both look identical in the report.
--
-- Purely additive. Rows written before this carry '' / 0 = "not
-- reported", deliberately distinct from "unsecure".

ALTER TABLE nodelistdb.ping_tests
    ADD COLUMN IF NOT EXISTS `reply_inbound_auth` LowCardinality(String) DEFAULT '';

ALTER TABLE nodelistdb.ping_tests
    ADD COLUMN IF NOT EXISTS `reply_inbound_tier` UInt8 DEFAULT 0;
