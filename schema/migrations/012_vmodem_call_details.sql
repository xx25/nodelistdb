-- Migration 012: record who answered a VModem/IVM test and why it came out that way
--
-- The IVM tester already stored WHAT it found on the announced port
-- (vmodem_variant, vmodem_conformant, vmodem_software, vmodem_system_name,
-- vmodem_addresses). Two things were missing.
--
-- The rest of the identity. A completed VMP call runs an EMSI handshake over
-- the reverse data channel and reads back the same identity every other
-- protocol tester collects; sysop and location were captured and then dropped
-- at the database boundary, unlike their binkp_sysop/binkp_location
-- counterparts:
--
--   vmodem_sysop        remote sysop name (EMSI)
--   vmodem_location     remote location (EMSI)
--
-- And WHY, which is the whole diagnostic value of a VMP call: whether the
-- remote rang its mailer, whether it could open the reverse data channel back
-- to us, whether it hung up and with which reason, and whether the EMSI
-- handshake produced an identity:
--
--   vmodem_detail       the human-readable note behind the variant, e.g.
--                       "VMP call established, mailer answered: handshake
--                       failed (TIMEOUT)" or "IVM announced, actual:
--                       emsi-telnet (FrontDoor 2.26)"
--   vmodem_call_outcome the groupable form of the same thing for a VMP call
--                       that was actually placed. Fixed vocabulary, safe to
--                       GROUP BY: connected | no-answer | busy | no-reply |
--                       no-data-channel | ring-aborted | remote-hangup |
--                       bad-state | remote-aborted | short-connect | loopback |
--                       rejected-our-hello | disconnect-<n> | no-local-port.
--                       ("no-reply" is a peer that swallowed a VMP hello and a
--                       connect request without a byte back — either not a
--                       VMODEM, or one stuck dialling a data-channel port of
--                       ours it cannot reach; the two look identical from here.)
--                       Empty when no call was placed, which is the normal case
--                       for every variant other than vmp.
--   vmodem_banner       the raw greeting, kept so a peer classified only as
--                       "unknown" can still be identified later.
--
-- All five are additive String columns with an empty default: existing rows
-- read back as '' and old binaries keep inserting without them. This is a
-- metadata-only ALTER — no data is rewritten.
--
-- Run on production ClickHouse BEFORE deploying the new testdaemon/server
-- binaries (the new INSERT names these columns; the new SELECTs read them).

ALTER TABLE node_test_results
    ADD COLUMN IF NOT EXISTS `vmodem_sysop` String DEFAULT '' AFTER `vmodem_system_name`,
    ADD COLUMN IF NOT EXISTS `vmodem_location` String DEFAULT '' AFTER `vmodem_sysop`,
    ADD COLUMN IF NOT EXISTS `vmodem_detail` String DEFAULT '' AFTER `vmodem_addresses`,
    ADD COLUMN IF NOT EXISTS `vmodem_call_outcome` String DEFAULT '' AFTER `vmodem_detail`,
    ADD COLUMN IF NOT EXISTS `vmodem_banner` String DEFAULT '' AFTER `vmodem_call_outcome`;
