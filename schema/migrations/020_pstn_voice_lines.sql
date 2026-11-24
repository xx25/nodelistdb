-- Migration 020: tell a voice line from a dead number
--
-- Purely additive. A modem test call to a node's listed number was answered
-- by a person: the number is a voice line now. That is not the "dead number"
-- pstn_dead_nodes was built for, and it is the one case where calling again
-- is not merely useless but a nuisance to somebody.
--
-- pstn_dead_nodes gains:
--
--   kind    'dead' (the number does not answer; every existing row) or
--           'voice' (a person answers: voice line, do not call). Fixed
--           vocabulary; the API refuses anything else.
--   phone   the number the mark was made for, digits only. A mark with a
--           phone applies only while the node's current number still equals
--           it, so a node that publishes a new number is tested again. Empty
--           (every existing row) means the mark applies whatever the number.
--
-- node_test_results gains what the modem said about a failed dial:
--
--   modem_voice_answered  the call was answered by a person
--   modem_failure_detail  the modem's own reason for the failure, e.g.
--                         "voice answered", "answer signal timeout",
--                         "whole call attempt expired after a voice
--                         announcement" (a recorded announcement before
--                         answer: a dead number, not a person)
--
-- All four have defaults: existing rows read back as 'dead' / '' / false and
-- old binaries keep inserting without naming them. Metadata-only ALTERs.
--
-- Run on production ClickHouse BEFORE deploying the new server binary: its
-- INSERTs into both tables name these columns and its SELECTs read them.

ALTER TABLE nodelistdb.pstn_dead_nodes
    ADD COLUMN IF NOT EXISTS `kind` LowCardinality(String) DEFAULT 'dead' AFTER `reason`,
    ADD COLUMN IF NOT EXISTS `phone` String DEFAULT '' AFTER `kind`;

ALTER TABLE nodelistdb.node_test_results
    ADD COLUMN IF NOT EXISTS `modem_voice_answered` Bool DEFAULT false AFTER `modem_ast_early_media`,
    ADD COLUMN IF NOT EXISTS `modem_failure_detail` String DEFAULT '' AFTER `modem_voice_answered`;
