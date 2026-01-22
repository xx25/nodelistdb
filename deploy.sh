#!/bin/bash
# NodelistDB Deployment Script
# Deploys server and testdaemon to production servers

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}NodelistDB Deployment Script${NC}"
echo "=============================="
echo ""

# Build for both architectures
build_binaries() {
    echo -e "${YELLOW}Building binaries...${NC}"

    # Build for x86_64 (nodelist.5001.ru)
    echo "  Building for x86_64..."
    GOOS=linux GOARCH=amd64 go build -ldflags "-X 'main.version=$(git describe --tags --always 2>/dev/null || echo dev)'" -o "$BIN_DIR/server-amd64" ./cmd/server
    GOOS=linux GOARCH=amd64 go build -ldflags "-X 'main.version=$(git describe --tags --always 2>/dev/null || echo dev)'" -o "$BIN_DIR/testdaemon-amd64" ./cmd/testdaemon

    # Build for ARM64 (oracle-main.thodin.net)
    echo "  Building for ARM64..."
    GOOS=linux GOARCH=arm64 go build -ldflags "-X 'main.version=$(git describe --tags --always 2>/dev/null || echo dev)'" -o "$BIN_DIR/parser-arm64" ./cmd/parser
    GOOS=linux GOARCH=arm64 go build -ldflags "-X 'main.version=$(git describe --tags --always 2>/dev/null || echo dev)'" -o "$BIN_DIR/server-arm64" ./cmd/server
    GOOS=linux GOARCH=arm64 go build -ldflags "-X 'main.version=$(git describe --tags --always 2>/dev/null || echo dev)'" -o "$BIN_DIR/testdaemon-arm64" ./cmd/testdaemon

    echo -e "  ${GREEN}✓ Build complete${NC}"
    echo ""
}

# Server 1: oracle-main.thodin.net (ARM64, parser + server + testdaemon)
deploy_oracle_main() {
    echo -e "${YELLOW}[1/2] Deploying to oracle-main.thodin.net (ARM64)...${NC}"

    HOST="dp@oracle-main.thodin.net"
    REMOTE_PATH="/opt/nodelistdb"

    # Check binaries exist
    if [[ ! -f "$BIN_DIR/parser-arm64" ]] || [[ ! -f "$BIN_DIR/server-arm64" ]] || [[ ! -f "$BIN_DIR/testdaemon-arm64" ]]; then
        echo -e "${RED}Error: ARM64 binaries not found. Run with --build flag.${NC}"
        exit 1
    fi

    # Copy binaries
    echo "  Copying parser..."
    scp -q "$BIN_DIR/parser-arm64" "$HOST:/tmp/parser-new"

    echo "  Copying server..."
    scp -q "$BIN_DIR/server-arm64" "$HOST:/tmp/server-new"

    echo "  Copying testdaemon..."
    scp -q "$BIN_DIR/testdaemon-arm64" "$HOST:/tmp/testdaemon-new"

    # Deploy and restart services
    echo "  Stopping services..."
    ssh "$HOST" "sudo systemctl stop nodelistdb nodelistdb-testdaemon"

    echo "  Installing binaries..."
    ssh "$HOST" "sudo cp /tmp/parser-new $REMOTE_PATH/parser && sudo cp /tmp/server-new $REMOTE_PATH/server && sudo cp /tmp/testdaemon-new $REMOTE_PATH/testdaemon"

    echo "  Starting services..."
    ssh "$HOST" "sudo systemctl start nodelistdb nodelistdb-testdaemon"

    echo "  Cleaning up..."
    ssh "$HOST" "rm -f /tmp/parser-new /tmp/server-new /tmp/testdaemon-new"

    # Verify services are running
    echo "  Verifying..."
    sleep 2
    if ssh "$HOST" "systemctl is-active --quiet nodelistdb && systemctl is-active --quiet nodelistdb-testdaemon"; then
        echo -e "  ${GREEN}✓ oracle-main.thodin.net deployed successfully${NC}"
    else
        echo -e "  ${RED}✗ Service verification failed!${NC}"
        ssh "$HOST" "systemctl status nodelistdb nodelistdb-testdaemon"
        exit 1
    fi
    echo ""
}

# Server 2: nodelist.5001.ru (x86_64, server only)
deploy_5001() {
    echo -e "${YELLOW}[2/2] Deploying to nodelist.5001.ru (x86_64)...${NC}"

    HOST="root@nodelist.5001.ru"
    REMOTE_PATH="/opt/nodelistdb"

    # Check binary exists
    if [[ ! -f "$BIN_DIR/server-amd64" ]]; then
        echo -e "${RED}Error: x86_64 binary not found. Run with --build flag.${NC}"
        exit 1
    fi

    # Copy binary
    echo "  Copying server..."
    scp -q "$BIN_DIR/server-amd64" "$HOST:/tmp/server-new"

    # Deploy and restart service
    echo "  Stopping service..."
    ssh "$HOST" "systemctl stop nodelistdb"

    echo "  Installing binary..."
    ssh "$HOST" "cp /tmp/server-new $REMOTE_PATH/server"

    echo "  Starting service..."
    ssh "$HOST" "systemctl start nodelistdb"

    echo "  Cleaning up..."
    ssh "$HOST" "rm -f /tmp/server-new"

    # Verify service is running
    echo "  Verifying..."
    sleep 2
    if ssh "$HOST" "systemctl is-active --quiet nodelistdb"; then
        echo -e "  ${GREEN}✓ nodelist.5001.ru deployed successfully${NC}"
    else
        echo -e "  ${RED}✗ Service verification failed!${NC}"
        ssh "$HOST" "systemctl status nodelistdb"
        exit 1
    fi
    echo ""
}

# Parse arguments
DEPLOY_ALL=true
DEPLOY_ORACLE=""
DEPLOY_5001=""
DO_BUILD=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --oracle-main)
            DEPLOY_ALL=false
            DEPLOY_ORACLE=true
            shift
            ;;
        --5001)
            DEPLOY_ALL=false
            DEPLOY_5001=true
            shift
            ;;
        --build)
            DO_BUILD=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --build        Build binaries before deploying"
            echo "  --oracle-main  Deploy only to oracle-main.thodin.net (ARM64)"
            echo "  --5001         Deploy only to nodelist.5001.ru (x86_64)"
            echo "  (no options)   Deploy to all servers (requires pre-built binaries)"
            echo ""
            echo "Servers:"
            echo "  oracle-main.thodin.net  ARM64, parser + server + testdaemon"
            echo "  nodelist.5001.ru        x86_64, server only"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Build if requested
if [[ "$DO_BUILD" == "true" ]]; then
    build_binaries
fi

# Show binary info
echo "Binaries to deploy:"
if [[ -f "$BIN_DIR/server-arm64" ]]; then
    echo "  - parser-arm64:     $(ls -lh "$BIN_DIR/parser-arm64" | awk '{print $5}')"
    echo "  - server-arm64:     $(ls -lh "$BIN_DIR/server-arm64" | awk '{print $5}')"
    echo "  - testdaemon-arm64: $(ls -lh "$BIN_DIR/testdaemon-arm64" | awk '{print $5}')"
else
    echo "  - ARM64 binaries:   NOT FOUND (use --build)"
fi
if [[ -f "$BIN_DIR/server-amd64" ]]; then
    echo "  - server-amd64:     $(ls -lh "$BIN_DIR/server-amd64" | awk '{print $5}')"
else
    echo "  - x86_64 binaries:  NOT FOUND (use --build)"
fi
echo ""

# Deploy
if [[ "$DEPLOY_ALL" == "true" ]] || [[ "$DEPLOY_ORACLE" == "true" ]]; then
    deploy_oracle_main
fi

if [[ "$DEPLOY_ALL" == "true" ]] || [[ "$DEPLOY_5001" == "true" ]]; then
    deploy_5001
fi

echo -e "${GREEN}=============================="
echo -e "Deployment complete!${NC}"
