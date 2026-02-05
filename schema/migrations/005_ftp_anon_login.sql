-- Migration 005: Add FTP anonymous login success column
-- Tracks whether anonymous FTP login was successful
-- NULL = not attempted (banner check failed), true = success, false = rejected

ALTER TABLE nodelistdb.node_test_results ADD COLUMN IF NOT EXISTS `ftp_anon_success` Nullable(Bool) DEFAULT NULL;
