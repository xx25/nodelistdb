#!/bin/bash
# Modem-Test Deployment Script
# Deploys modem-test binary to test server

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="$SCRIPT_DIR/bin"

# Deploy target
HOST="192.168.88.30"
REMOTE_PATH="/opt/modem"
BINARY_NAME="modem-test"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Modem-Test Deployment Script${NC}"
echo "=============================="
echo ""

# Build binary
build_binary() {
    echo -e "${YELLOW}Building modem-test...${NC}"

    # Get version info
    VERSION=$(git describe --tags --always 2>/dev/null | sed 's/^v//' || echo dev)
    COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
    BUILD_TIME=$(date -u +"%Y-%m-%d %H:%M:%S UTC")
    LDFLAGS="-X 'github.com/nodelistdb/internal/version.Version=${VERSION}' -X 'github.com/nodelistdb/internal/version.GitCommit=${COMMIT}' -X 'github.com/nodelistdb/internal/version.BuildTime=${BUILD_TIME}'"

    # Build for x86_64
    echo "  Building for x86_64..."
    GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$BIN_DIR/$BINARY_NAME-amd64" ./cmd/modem-test

    echo -e "  ${GREEN}✓ Build complete${NC}"
    echo ""
}

# Deploy to server
deploy() {
    echo -e "${YELLOW}Deploying to $HOST...${NC}"

    # Check binary exists
    if [[ ! -f "$BIN_DIR/$BINARY_NAME-amd64" ]]; then
        echo -e "${RED}Error: Binary not found at $BIN_DIR/$BINARY_NAME-amd64${NC}"
        echo "Run with --build flag or without --no-build"
        exit 1
    fi

    # Copy binary
    echo "  Copying $BINARY_NAME..."
    scp -q "$BIN_DIR/$BINARY_NAME-amd64" "$HOST:/tmp/$BINARY_NAME-new"

    # Install binary
    echo "  Installing binary..."
    ssh "$HOST" "cp /tmp/$BINARY_NAME-new $REMOTE_PATH/$BINARY_NAME && chmod +x $REMOTE_PATH/$BINARY_NAME"

    # Cleanup
    echo "  Cleaning up..."
    ssh "$HOST" "rm -f /tmp/$BINARY_NAME-new"

    # Verify
    echo "  Verifying..."
    REMOTE_VERSION=$(ssh "$HOST" "$REMOTE_PATH/$BINARY_NAME --version 2>&1 | head -1" || echo "unknown")
    echo "  Remote version: $REMOTE_VERSION"

    echo -e "  ${GREEN}✓ Deployed successfully to $HOST:$REMOTE_PATH/$BINARY_NAME${NC}"
    echo ""
}

# Parse arguments
DO_BUILD=true

while [[ $# -gt 0 ]]; do
    case $1 in
        --build)
            DO_BUILD=true
            shift
            ;;
        --no-build)
            DO_BUILD=""
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --no-build     Skip building, use existing binary"
            echo "  --build        Build before deploying (default)"
            echo ""
            echo "Target: $HOST:$REMOTE_PATH"
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
    build_binary
fi

# Show binary info
echo "Binary to deploy:"
if [[ -f "$BIN_DIR/$BINARY_NAME-amd64" ]]; then
    SIZE=$(ls -lh "$BIN_DIR/$BINARY_NAME-amd64" | awk '{print $5}')
    echo "  - $BINARY_NAME-amd64: $SIZE"
else
    echo -e "  ${RED}- $BINARY_NAME-amd64: NOT FOUND (use --build)${NC}"
    exit 1
fi
echo ""

# Deploy
deploy

echo -e "${GREEN}=============================="
echo -e "Deployment complete!${NC}"
