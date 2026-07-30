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

    # Get version info (strip 'v' prefix since templates add it)
    VERSION=$(git describe --tags --always 2>/dev/null | sed 's/^v//' || echo dev)
    COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
    BUILD_TIME=$(date -u +"%Y-%m-%d %H:%M:%S UTC")
    LDFLAGS="-X 'github.com/nodelistdb/internal/version.Version=${VERSION}' -X 'github.com/nodelistdb/internal/version.GitCommit=${COMMIT}' -X 'github.com/nodelistdb/internal/version.BuildTime=${BUILD_TIME}'"

    # Build for x86_64 (oracle-vm1 and nodelist.5001.ru — both run the server)
    echo "  Building for x86_64..."
    GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$BIN_DIR/server-amd64" ./cmd/server
    GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$BIN_DIR/testdaemon-amd64" ./cmd/testdaemon

    # Build for ARM64 (oracle-main.thodin.net — parser + testdaemon only).
    # No server-arm64: the web server moved to vm1 on 2026-07-30 and
    # nodelistdb.service is disabled on oracle-main. Building a binary nothing
    # deploys is how the old topology got baked into this script.
    echo "  Building for ARM64..."
    GOOS=linux GOARCH=arm64 go build -ldflags "$LDFLAGS" -o "$BIN_DIR/parser-arm64" ./cmd/parser
    GOOS=linux GOARCH=arm64 go build -ldflags "$LDFLAGS" -o "$BIN_DIR/testdaemon-arm64" ./cmd/testdaemon

    echo -e "  ${GREEN}✓ Build complete${NC}"
    echo ""
}

# Server 1: oracle-main.thodin.net (ARM64, parser + testdaemon)
#
# This host keeps ClickHouse, the parser, sync_nodelists.sh and the testdaemon.
# It does NOT run the web server any more: that moved to oracle-vm1 on
# 2026-07-30 and `nodelistdb.service` here is disabled. Until then this
# function stopped, reinstalled and STARTED that unit, which would have
# resurrected a second web server on this host — and the is-active check
# afterwards would have called it a success.
deploy_oracle_main() {
    echo -e "${YELLOW}[1/3] Deploying to oracle-main.thodin.net (ARM64, parser + testdaemon)...${NC}"

    HOST="dp@oracle-main.thodin.net"
    REMOTE_PATH="/opt/nodelistdb"

    # Check binaries exist
    if [[ ! -f "$BIN_DIR/parser-arm64" ]] || [[ ! -f "$BIN_DIR/testdaemon-arm64" ]]; then
        echo -e "${RED}Error: ARM64 binaries not found. Run with --build flag.${NC}"
        exit 1
    fi

    # Copy binaries
    echo "  Copying parser..."
    scp -q "$BIN_DIR/parser-arm64" "$HOST:/tmp/parser-new"

    echo "  Copying testdaemon..."
    scp -q "$BIN_DIR/testdaemon-arm64" "$HOST:/tmp/testdaemon-new"

    # Only the testdaemon runs here, so only it needs stopping. The parser is a
    # CLI invoked by sync_nodelists.sh; copy over it while nothing is running.
    echo "  Stopping testdaemon..."
    ssh "$HOST" "sudo systemctl stop nodelistdb-testdaemon"

    echo "  Installing binaries..."
    ssh "$HOST" "sudo cp /tmp/parser-new $REMOTE_PATH/parser && sudo cp /tmp/testdaemon-new $REMOTE_PATH/testdaemon"

    echo "  Starting testdaemon..."
    ssh "$HOST" "sudo systemctl start nodelistdb-testdaemon"

    echo "  Cleaning up..."
    ssh "$HOST" "rm -f /tmp/parser-new /tmp/testdaemon-new"

    # Verify
    echo "  Verifying..."
    sleep 2
    if ssh "$HOST" "systemctl is-active --quiet nodelistdb-testdaemon"; then
        echo -e "  ${GREEN}✓ oracle-main.thodin.net deployed successfully${NC}"
    else
        echo -e "  ${RED}✗ Service verification failed!${NC}"
        ssh "$HOST" "systemctl status nodelistdb-testdaemon --no-pager -n 15"
        exit 1
    fi
    echo ""
}

# Server 2: oracle-vm1 (x86_64, server only) — the public web/FTP front end
# since 2026-07-30. Reached as dp@nodelist.fidonet.cc: there is no oracle-vm1
# DNS name, and the A/AAAA record for the site points here.
deploy_vm1() {
    echo -e "${YELLOW}[2/3] Deploying to oracle-vm1 / nodelist.fidonet.cc (x86_64, server)...${NC}"

    HOST="dp@nodelist.fidonet.cc"
    REMOTE_PATH="/opt/nodelistdb"

    local ssh_opts=(-o ConnectTimeout=20 -o ServerAliveInterval=10 -o ServerAliveCountMax=6)

    if [[ ! -f "$BIN_DIR/server-amd64" ]]; then
        echo -e "${RED}Error: x86_64 binary not found. Run with --build flag.${NC}"
        exit 1
    fi

    echo "  Copying server..."
    scp "${ssh_opts[@]}" -q "$BIN_DIR/server-amd64" "$HOST:/tmp/server-new"

    # Verify the upload before touching the running service.
    echo "  Verifying checksum..."
    local lsha rsha
    lsha=$(sha256sum "$BIN_DIR/server-amd64" | awk '{print $1}')
    rsha=$(ssh "${ssh_opts[@]}" "$HOST" "sha256sum /tmp/server-new | awk '{print \$1}'")
    if [[ "$lsha" != "$rsha" ]]; then
        echo -e "  ${RED}✗ Checksum mismatch (local=$lsha remote=$rsha) — aborting${NC}"
        exit 1
    fi

    # Stage then atomic rename: an in-place cp over the running binary fails
    # with "Text file busy". The unit runs as User=daemon, so sudo is needed
    # for the install and the restart.
    echo "  Installing binary and restarting..."
    ssh "${ssh_opts[@]}" "$HOST" "sudo cp /tmp/server-new $REMOTE_PATH/server.new && sudo chmod 755 $REMOTE_PATH/server.new && sudo mv -f $REMOTE_PATH/server.new $REMOTE_PATH/server && sudo systemctl restart nodelistdb && rm -f /tmp/server-new"

    # ClickHouse is one hop away over the private VCN, so startup is quick —
    # but check the HTTP surface too, not just the unit state: this host serves
    # the public site and a bad config exits after systemd calls it active.
    echo "  Verifying..."
    local ok=""
    for i in $(seq 1 8); do
        sleep 3
        if ssh "${ssh_opts[@]}" "$HOST" "systemctl is-active --quiet nodelistdb && curl -sf -o /dev/null http://127.0.0.1:8081/api/health"; then
            ok=1; break
        fi
    done
    if [[ -n "$ok" ]]; then
        echo -e "  ${GREEN}✓ oracle-vm1 deployed successfully${NC}"
    else
        echo -e "  ${RED}✗ Service verification failed!${NC}"
        ssh "${ssh_opts[@]}" "$HOST" "systemctl status nodelistdb --no-pager -n 20"
        exit 1
    fi
    echo ""
}

# Server 2: nodelist.5001.ru (x86_64, server only)
#
# The direct link from here to 5001 is frequently congested (SSH sessions
# stall, large transfers time out), so by default we route through the
# moscow-server jumphost via SSH ProxyJump — a fast, low-latency path to 5001.
# Override the jumphost with DEPLOY_5001_JUMPHOST, or set it empty for a direct
# connection:
#   DEPLOY_5001_JUMPHOST=""            bash deploy.sh --5001   # direct
#   DEPLOY_5001_JUMPHOST=me@host       bash deploy.sh --5001   # custom jump
deploy_5001() {
    echo -e "${YELLOW}[3/3] Deploying to nodelist.5001.ru (x86_64, server)...${NC}"

    HOST="root@nodelist.5001.ru"
    REMOTE_PATH="/opt/nodelistdb"
    # Unset -> default jumphost; explicitly empty -> direct connection.
    local jumphost="${DEPLOY_5001_JUMPHOST-dp@192.168.89.5}"

    # Keepalives so a stalled link fails fast rather than hanging; ProxyJump
    # unless disabled.
    local ssh_opts=(-o ConnectTimeout=20 -o ServerAliveInterval=10 -o ServerAliveCountMax=6)
    if [[ -n "$jumphost" ]]; then
        ssh_opts+=(-J "$jumphost")
        echo "  Routing via jumphost: $jumphost"
    fi

    # Check binary exists
    if [[ ! -f "$BIN_DIR/server-amd64" ]]; then
        echo -e "${RED}Error: x86_64 binary not found. Run with --build flag.${NC}"
        exit 1
    fi

    # Copy binary — prefer rsync (resumes across drops), retry a few times.
    echo "  Copying server..."
    local copied=""
    for i in $(seq 1 5); do
        if command -v rsync >/dev/null 2>&1; then
            if rsync --partial --append-verify -z --timeout=120 \
                -e "ssh ${ssh_opts[*]}" \
                "$BIN_DIR/server-amd64" "$HOST:/tmp/server-new"; then
                copied=1; break
            fi
        else
            if scp "${ssh_opts[@]}" -q "$BIN_DIR/server-amd64" "$HOST:/tmp/server-new"; then
                copied=1; break
            fi
        fi
        echo "  copy attempt $i failed; retrying..."
        sleep 5
    done
    if [[ -z "$copied" ]]; then
        echo -e "  ${RED}✗ Failed to copy binary after retries${NC}"
        exit 1
    fi

    # Verify the upload before touching the running service.
    echo "  Verifying checksum..."
    local lsha rsha
    lsha=$(sha256sum "$BIN_DIR/server-amd64" | awk '{print $1}')
    rsha=$(ssh "${ssh_opts[@]}" "$HOST" "sha256sum /tmp/server-new | awk '{print \$1}'")
    if [[ "$lsha" != "$rsha" ]]; then
        echo -e "  ${RED}✗ Checksum mismatch (local=$lsha remote=$rsha) — aborting${NC}"
        exit 1
    fi

    # Swap without a downtime gap: stage into the target dir then atomic rename
    # (in-place cp over the running binary fails with "Text file busy").
    echo "  Installing binary and restarting..."
    ssh "${ssh_opts[@]}" "$HOST" "cp /tmp/server-new $REMOTE_PATH/server.new && chmod 755 $REMOTE_PATH/server.new && mv -f $REMOTE_PATH/server.new $REMOTE_PATH/server && systemctl restart nodelistdb && rm -f /tmp/server-new"

    # Verify — 5001 talks to a REMOTE ClickHouse, so startup takes ~45s and
    # systemd may log a restart or two before it settles. Poll for up to ~60s.
    echo "  Verifying (allowing for slow ClickHouse startup)..."
    local ok=""
    for i in $(seq 1 12); do
        sleep 5
        if ssh "${ssh_opts[@]}" "$HOST" "systemctl is-active --quiet nodelistdb"; then
            ok=1; break
        fi
    done
    if [[ -n "$ok" ]]; then
        echo -e "  ${GREEN}✓ nodelist.5001.ru deployed successfully${NC}"
    else
        echo -e "  ${RED}✗ Service verification failed!${NC}"
        ssh "${ssh_opts[@]}" "$HOST" "systemctl status nodelistdb --no-pager -n 15"
        exit 1
    fi
    echo ""
}

# Parse arguments
DEPLOY_ALL=true
DEPLOY_ORACLE=""
DEPLOY_VM1=""
DEPLOY_5001=""
DO_BUILD=true  # Build by default

while [[ $# -gt 0 ]]; do
    case $1 in
        --oracle-main)
            DEPLOY_ALL=false
            DEPLOY_ORACLE=true
            shift
            ;;
        --vm1)
            DEPLOY_ALL=false
            DEPLOY_VM1=true
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
        --no-build)
            DO_BUILD=""
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --no-build     Skip building, use existing binaries"
            echo "  --oracle-main  Deploy only to oracle-main.thodin.net (ARM64)"
            echo "  --vm1          Deploy only to oracle-vm1 / nodelist.fidonet.cc"
            echo "  --5001         Deploy only to nodelist.5001.ru (x86_64)"
            echo "  (default)      Build and deploy to all servers"
            echo ""
            echo "Servers:"
            echo "  oracle-main.thodin.net  ARM64, parser + testdaemon (+ ClickHouse, sync)"
            echo "  nodelist.fidonet.cc     x86_64, server only — the public site since 2026-07-30"
            echo "  nodelist.5001.ru        x86_64, server only (via jumphost by default)"
            echo ""
            echo "Environment:"
            echo "  DEPLOY_5001_JUMPHOST    SSH jumphost for 5001 (default: dp@192.168.89.5;"
            echo "                          set empty for a direct connection)"
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
if [[ -f "$BIN_DIR/parser-arm64" ]]; then
    echo "  - parser-arm64:     $(ls -lh "$BIN_DIR/parser-arm64" | awk '{print $5}')"
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

if [[ "$DEPLOY_ALL" == "true" ]] || [[ "$DEPLOY_VM1" == "true" ]]; then
    deploy_vm1
fi

if [[ "$DEPLOY_ALL" == "true" ]] || [[ "$DEPLOY_5001" == "true" ]]; then
    deploy_5001
fi

echo -e "${GREEN}=============================="
echo -e "Deployment complete!${NC}"
