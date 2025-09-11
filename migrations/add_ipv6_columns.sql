-- Migration: Add IPv4/IPv6 specific columns for dual-stack testing
-- Date: 2025-01-12
-- Purpose: Track IPv4 and IPv6 test results separately for each protocol

-- DuckDB Migration
-- Add IPv4/IPv6 specific columns for BinkP
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_tested BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_success BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_response_ms INTEGER;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_address VARCHAR;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv4_error VARCHAR;

ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_tested BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_success BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_response_ms INTEGER;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_address VARCHAR;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS binkp_ipv6_error VARCHAR;

-- Add IPv4/IPv6 specific columns for IFCICO
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_tested BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_success BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_response_ms INTEGER;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_address VARCHAR;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv4_error VARCHAR;

ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_tested BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_success BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_response_ms INTEGER;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_address VARCHAR;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS ifcico_ipv6_error VARCHAR;

-- Add IPv4/IPv6 specific columns for Telnet
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_tested BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_success BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_response_ms INTEGER;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_address VARCHAR;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv4_error VARCHAR;

ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_tested BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_success BOOLEAN DEFAULT false;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_response_ms INTEGER;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_address VARCHAR;
ALTER TABLE node_test_results ADD COLUMN IF NOT EXISTS telnet_ipv6_error VARCHAR;

-- Create indexes for IPv6 analysis
CREATE INDEX IF NOT EXISTS idx_binkp_ipv6_success ON node_test_results(binkp_ipv6_success) WHERE binkp_ipv6_tested = true;
CREATE INDEX IF NOT EXISTS idx_ifcico_ipv6_success ON node_test_results(ifcico_ipv6_success) WHERE ifcico_ipv6_tested = true;
CREATE INDEX IF NOT EXISTS idx_telnet_ipv6_success ON node_test_results(telnet_ipv6_success) WHERE telnet_ipv6_tested = true;