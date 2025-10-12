#!/bin/bash

# NodelistDB Release Script
# Creates a git tag and pushes it to trigger the release workflow

set -e

VERSION=${1:-"v0.1.9"}

echo "Creating release for NodelistDB version: $VERSION"

# Ensure we're on the main branch
if [[ $(git branch --show-current) != "main" ]]; then
    echo "Warning: Not on main branch. Current branch: $(git branch --show-current)"
    read -p "Continue anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Ensure working directory is clean
if [[ -n $(git status --porcelain) ]]; then
    echo "Error: Working directory is not clean. Please commit or stash changes first."
    git status --short
    exit 1
fi

# Pull latest changes
echo "Pulling latest changes..."
git pull origin main

# Run tests to ensure everything works
echo "Running tests..."
go test ./cmd/... ./internal/...

# Build to ensure everything compiles
echo "Building binaries..."
make build

echo "All checks passed!"

# Create and push the tag
echo "Creating git tag: $VERSION"
git tag -a "$VERSION" -m "Release $VERSION

Features:
- FidoNet nodelist parsing and storage system
- ClickHouse-based high-performance storage engine
- REST API for node search and statistics
- Web interface for browsing nodelist data
- Command-line tools for parsing and serving
- Multi-platform support (Linux, Windows, macOS, FreeBSD)

Architecture:
- Component-based storage layer with improved security
- Parameterized queries prevent SQL injection
- Thread-safe operations with optimized batch processing
- Comprehensive node change tracking and history"

echo "Pushing tag to trigger release workflow..."
git push origin "$VERSION"

echo ""
echo "✅ Release $VERSION has been created!"
echo "📦 GitHub Actions will now build and publish the release artifacts."
echo "🔗 Check the progress at: https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^.]*\).*/\1/')/actions"
echo ""
echo "The release will include:"
echo "  - linux-amd64.tar.gz"
echo "  - linux-arm64.tar.gz" 
echo "  - windows-amd64.zip"
echo "  - windows-arm64.zip"
echo "  - darwin-amd64.tar.gz"
echo "  - darwin-arm64.tar.gz"
echo "  - freebsd-amd64.tar.gz"
echo "  - SHA256SUMS and MD5SUMS"