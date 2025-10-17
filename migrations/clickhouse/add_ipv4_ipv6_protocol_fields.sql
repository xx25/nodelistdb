-- Migration: Add IPv4/IPv6 Protocol-Specific Fields to node_test_results
-- Date: 2025-10-17
-- Description: Adds separate IPv4 and IPv6 test result fields for BinkP, IFCICO, and Telnet protocols

-- ============================================================================
-- IPv4-SPECIFIC PROTOCOL FIELDS
-- ============================================================================

-- BinkP IPv4 Fields
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_tested Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_success Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_error String DEFAULT '';

-- IFCICO IPv4 Fields
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_tested Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_success Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_error String DEFAULT '';

-- Telnet IPv4 Fields
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_tested Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_success Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_error String DEFAULT '';

-- ============================================================================
-- IPv6-SPECIFIC PROTOCOL FIELDS
-- ============================================================================

-- BinkP IPv6 Fields
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_tested Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_success Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_error String DEFAULT '';

-- IFCICO IPv6 Fields
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_tested Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_success Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_error String DEFAULT '';

-- Telnet IPv6 Fields
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_tested Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_success Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_error String DEFAULT '';

-- FTP IPv4 Fields
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ftp_ipv4_tested Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ftp_ipv4_success Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ftp_ipv4_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ftp_ipv4_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ftp_ipv4_error String DEFAULT '';

-- FTP IPv6 Fields
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ftp_ipv6_tested Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ftp_ipv6_success Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ftp_ipv6_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ftp_ipv6_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ftp_ipv6_error String DEFAULT '';

-- VModem IPv4 Fields
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS vmodem_ipv4_tested Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS vmodem_ipv4_success Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS vmodem_ipv4_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS vmodem_ipv4_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS vmodem_ipv4_error String DEFAULT '';

-- VModem IPv6 Fields
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS vmodem_ipv6_tested Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS vmodem_ipv6_success Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS vmodem_ipv6_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS vmodem_ipv6_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS vmodem_ipv6_error String DEFAULT '';

-- ============================================================================
-- MULTI-HOSTNAME TESTING FIELDS
-- ============================================================================

ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS tested_hostname String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS hostname_index Int32 DEFAULT -1;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS is_aggregated Bool DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS total_hostnames Int32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS hostnames_tested Int32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS hostnames_operational Int32 DEFAULT 0;
