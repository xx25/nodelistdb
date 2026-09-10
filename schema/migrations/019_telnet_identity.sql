-- Migration 019: keep what an ITN mailer announced
--
-- Purely additive. The telnet test used to be a TCP connect and stored only
-- pass/fail. Since 2026-09-10 it is an EMSI mail session over the telnet
-- option layer (FTS-5001 ITN means a mailer, not a BBS login), and the
-- mailer's EMSI_DAT is the evidence of what answered: the same identity
-- binkp_*/ifcico_* already carry for their protocols. telnet_banner keeps
-- whatever the port printed before EMSI began, which for a failed handshake
-- is the only clue to what was there.
--
-- Run BEFORE deploying the testdaemon: the new binary names these columns in
-- its INSERT and every batch fails without them. Old binaries are unaffected
-- by the new columns (defaults, never named).
ALTER TABLE nodelistdb.node_test_results
    ADD COLUMN IF NOT EXISTS telnet_mailer_info String DEFAULT '',
    ADD COLUMN IF NOT EXISTS telnet_system_name String DEFAULT '',
    ADD COLUMN IF NOT EXISTS telnet_addresses Array(String) DEFAULT [],
    ADD COLUMN IF NOT EXISTS telnet_banner String DEFAULT '';
