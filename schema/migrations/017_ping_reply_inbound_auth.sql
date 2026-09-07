-- 017_ping_reply_inbound_auth.sql
--
-- Record HOW a PING/TRACE reply reached us, not just what it said.
--
-- fidomail classifies every inbound session (domain.Tier): A is a
-- configured link that passed its session password, B is a peer in the
-- nodelist with no configured link, C is a peer unknown to the nodelist.
-- B and C are what its web UI files under "Unsecure": the peer was never
-- authenticated, so the From address on such a netmail is an unverified
-- claim. That matters here because the whole output of this measurement is
-- "node X answered" -- 4 of the first 26 replies arrived that way.
--
-- Tier A is NOT proof of the originator either: it says a contracted link
-- relayed the mail to us. Neither value verifies who wrote the message,
-- only how it arrived, which is why this is stored as evidence beside the
-- reply and changes no classification.
--
-- Purely additive. Rows written before this (and rows from a fidomail
-- older than the API field) carry tier 0 / auth '' = "not reported", which
-- is deliberately distinct from "unsecure": absent provenance is not
-- evidence of a stranger.

ALTER TABLE nodelistdb.ping_replies
    ADD COLUMN IF NOT EXISTS `inbound_tier` UInt8 DEFAULT 0;

ALTER TABLE nodelistdb.ping_replies
    ADD COLUMN IF NOT EXISTS `inbound_auth` LowCardinality(String) DEFAULT '';
