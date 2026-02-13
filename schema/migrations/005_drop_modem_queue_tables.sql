-- Migration 005: Drop unused modem queue tables
-- The server-controlled queue has been replaced by direct CLI submission
-- via POST /api/modem/results/direct. These tables are no longer used.

DROP TABLE IF EXISTS nodelistdb.modem_test_queue;
DROP TABLE IF EXISTS nodelistdb.modem_caller_status;
