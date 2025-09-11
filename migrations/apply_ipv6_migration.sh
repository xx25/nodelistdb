#!/bin/bash

# IPv6 Dual-Stack Migration Script
# Apply database schema changes for IPv6/IPv4 dual-stack testing support

set -e

echo "=== IPv6 Dual-Stack Database Migration ==="
echo "This script will add IPv4/IPv6 specific columns to track dual-stack test results"
echo ""

# Detect database type from config
if [ -f "config-clickhouse.yaml" ]; then
    echo "ClickHouse configuration detected"
    DB_TYPE="clickhouse"
elif [ -f "config.yaml" ]; then
    # Check if it's DuckDB config
    if grep -q "type: duckdb" config.yaml 2>/dev/null; then
        echo "DuckDB configuration detected"
        DB_TYPE="duckdb"
    elif grep -q "type: clickhouse" config.yaml 2>/dev/null; then
        echo "ClickHouse configuration detected"
        DB_TYPE="clickhouse"
    else
        echo "Unable to detect database type from config.yaml"
        exit 1
    fi
else
    echo "No configuration file found. Please specify database type:"
    echo "1) ClickHouse"
    echo "2) DuckDB"
    read -p "Enter choice (1 or 2): " choice
    case $choice in
        1) DB_TYPE="clickhouse" ;;
        2) DB_TYPE="duckdb" ;;
        *) echo "Invalid choice"; exit 1 ;;
    esac
fi

if [ "$DB_TYPE" = "clickhouse" ]; then
    echo ""
    echo "Applying ClickHouse migration..."
    
    # Check if clickhouse-client is available
    if ! command -v clickhouse-client &> /dev/null; then
        echo "ERROR: clickhouse-client not found. Please install ClickHouse client."
        exit 1
    fi
    
    # Apply the migration
    clickhouse-client --database nodelistdb --multiquery <<'EOF'
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

-- Add data skipping indexes for IPv6 analysis
ALTER TABLE node_test_results ADD INDEX IF NOT EXISTS idx_binkp_ipv6 (binkp_ipv6_tested, binkp_ipv6_success) TYPE minmax GRANULARITY 1;
ALTER TABLE node_test_results ADD INDEX IF NOT EXISTS idx_ifcico_ipv6 (ifcico_ipv6_tested, ifcico_ipv6_success) TYPE minmax GRANULARITY 1;
ALTER TABLE node_test_results ADD INDEX IF NOT EXISTS idx_telnet_ipv6 (telnet_ipv6_tested, telnet_ipv6_success) TYPE minmax GRANULARITY 1;
EOF
    
    echo "ClickHouse migration completed successfully!"
    
elif [ "$DB_TYPE" = "duckdb" ]; then
    echo ""
    echo "Applying DuckDB migration..."
    
    # Find DuckDB database file
    if [ -f "nodelist.duckdb" ]; then
        DB_FILE="nodelist.duckdb"
    elif [ -f "test_results.duckdb" ]; then
        DB_FILE="test_results.duckdb"
    else
        read -p "Enter path to DuckDB database file: " DB_FILE
        if [ ! -f "$DB_FILE" ]; then
            echo "ERROR: Database file not found: $DB_FILE"
            exit 1
        fi
    fi
    
    echo "Using database: $DB_FILE"
    
    # Apply the migration using duckdb CLI
    duckdb "$DB_FILE" <<'EOF'
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
EOF
    
    echo "DuckDB migration completed successfully!"
fi

echo ""
echo "=== Migration Complete ==="
echo ""
echo "The database now supports IPv4/IPv6 dual-stack tracking with:"
echo "  - Separate IPv4/IPv6 test results for each protocol"
echo "  - Response time tracking for each IP version"
echo "  - Error tracking for failed connections"
echo "  - Indexes for efficient IPv6 adoption queries"
echo ""
echo "Note: The testdaemon binary needs to be rebuilt and deployed to use these new fields."
echo "Run: go build -o bin/testdaemon cmd/testdaemon/main.go"