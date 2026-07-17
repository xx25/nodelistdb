-- Migration 002: Add VModem/IVM protocol-identification columns
--
-- Why: The IVM ("Internet VMODEM") nodelist flag announces Ray Gwinn's binary
-- Virtual Modem Protocol (VMP), but most IVM ports actually run an EMSI mailer
-- over telnet-binary/raw or something else. The tester now identifies what
-- protocol is really there and whether it is genuine VMP; these columns persist
-- that verdict alongside the existing vmodem_tested/success/error fields.
--
--   vmodem_variant     protocol observed: vmp | emsi-telnet | emsi-raw |
--                      binkp | telnet-login | ssh | http | ... | unknown | down
--   vmodem_conformant  true only when a genuine VMODEM (VMP) responder confirmed
--   vmodem_software    detected mailer/software, when identifiable
--   vmodem_system_name remote system name (EMSI), when identifiable
--   vmodem_addresses   remote FTN addresses (EMSI), when identifiable
--
-- Prerequisites: none (additive, backfills defaults on existing rows).
-- Estimated time: seconds (metadata-only ADD COLUMN).

ALTER TABLE node_test_results
    ADD COLUMN IF NOT EXISTS `vmodem_variant`     String        DEFAULT '' AFTER `vmodem_error`,
    ADD COLUMN IF NOT EXISTS `vmodem_conformant`  Bool          DEFAULT false AFTER `vmodem_variant`,
    ADD COLUMN IF NOT EXISTS `vmodem_software`    String        DEFAULT '' AFTER `vmodem_conformant`,
    ADD COLUMN IF NOT EXISTS `vmodem_system_name` String        DEFAULT '' AFTER `vmodem_software`,
    ADD COLUMN IF NOT EXISTS `vmodem_addresses`   Array(String) DEFAULT [] AFTER `vmodem_system_name`;
