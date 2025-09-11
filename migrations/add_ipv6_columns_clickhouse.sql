-- Migration: Add IPv4/IPv6 specific columns for dual-stack testing
-- Date: 2025-01-12
-- Purpose: Track IPv4 and IPv6 test results separately for each protocol
-- Database: ClickHouse

-- Add IPv4/IPv6 specific columns for BinkP
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_tested Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_success Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_error String DEFAULT '';

ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_tested Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_success Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_error String DEFAULT '';

-- Add IPv4/IPv6 specific columns for IFCICO
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_tested Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_success Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_error String DEFAULT '';

ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_tested Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_success Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_error String DEFAULT '';

-- Add IPv4/IPv6 specific columns for Telnet
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_tested Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_success Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_error String DEFAULT '';

ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_tested Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_success Bool DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_response_ms UInt32 DEFAULT 0;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_address String DEFAULT '';
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_error String DEFAULT '';

-- Add data skipping indexes for IPv6 analysis (ClickHouse specific)
ALTER TABLE node_test_results ADD INDEX IF NOT EXISTS idx_binkp_ipv6 (binkp_ipv6_tested, binkp_ipv6_success) TYPE minmax GRANULARITY 1;
ALTER TABLE node_test_results ADD INDEX IF NOT EXISTS idx_ifcico_ipv6 (ifcico_ipv6_tested, ifcico_ipv6_success) TYPE minmax GRANULARITY 1;
ALTER TABLE node_test_results ADD INDEX IF NOT EXISTS idx_telnet_ipv6 (telnet_ipv6_tested, telnet_ipv6_success) TYPE minmax GRANULARITY 1;