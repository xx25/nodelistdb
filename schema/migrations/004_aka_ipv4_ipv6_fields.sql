-- Migration: Add per-IPv4/IPv6 AKA address fields for mismatch detection
-- Run this on existing ClickHouse installations to add new columns

-- Per-IP-version announced AKA addresses (from BinkP/IFCICO handshakes)
ALTER TABLE nodelistdb.node_test_results ADD COLUMN IF NOT EXISTS `binkp_ipv4_addresses` Array(String) DEFAULT [];
ALTER TABLE nodelistdb.node_test_results ADD COLUMN IF NOT EXISTS `binkp_ipv6_addresses` Array(String) DEFAULT [];
ALTER TABLE nodelistdb.node_test_results ADD COLUMN IF NOT EXISTS `ifcico_ipv4_addresses` Array(String) DEFAULT [];
ALTER TABLE nodelistdb.node_test_results ADD COLUMN IF NOT EXISTS `ifcico_ipv6_addresses` Array(String) DEFAULT [];

-- Per-IP-version address validation flags
ALTER TABLE nodelistdb.node_test_results ADD COLUMN IF NOT EXISTS `address_validated_ipv4` Bool DEFAULT false;
ALTER TABLE nodelistdb.node_test_results ADD COLUMN IF NOT EXISTS `address_validated_ipv6` Bool DEFAULT false;
