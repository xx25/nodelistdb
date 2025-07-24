# NodelistDB Release Guide

This document describes how to create releases for NodelistDB.

## Quick Release (Recommended)

Use the provided release script:

```bash
./release.sh v0.1.0
```

This will:
1. Run tests and build checks
2. Create a git tag
3. Push the tag to trigger the GitHub Actions release workflow
4. Automatically build and publish multi-platform binaries

## Manual Release Process

If you prefer to create releases manually:

### 1. Prepare the Release

```bash
# Ensure you're on main branch
git checkout main
git pull origin main

# Update version if needed
echo "0.1.0" > VERSION

# Run tests
go test ./cmd/... ./internal/...

# Build locally to verify
make build
```

### 2. Create Git Tag

```bash
git tag -a v0.1.0 -m "Release v0.1.0

Features:
- FidoNet nodelist parsing and storage system
- DuckDB-based high-performance storage engine
- REST API for node search and statistics
- Web interface for browsing nodelist data
- Command-line tools for parsing and serving
- Multi-platform support

Architecture:
- Component-based storage layer with improved security
- Parameterized queries prevent SQL injection
- Thread-safe operations with optimized batch processing
- Comprehensive node change tracking and history"
```

### 3. Push Tag

```bash
git push origin v0.1.0
```

### 4. Monitor Release

The GitHub Actions workflow will automatically:
- Build binaries for all supported platforms
- Create release packages (tar.gz/zip)
- Generate checksums
- Publish the release with generated release notes

## Release Artifacts

Each release includes:

### Binaries
- `linux-amd64.tar.gz` - Linux x86-64
- `linux-arm64.tar.gz` - Linux ARM64
- `windows-amd64.zip` - Windows x86-64
- `windows-arm64.zip` - Windows ARM64
- `darwin-amd64.tar.gz` - macOS Intel
- `darwin-arm64.tar.gz` - macOS Apple Silicon
- `freebsd-amd64.tar.gz` - FreeBSD x86-64

### Checksums
- `SHA256SUMS` - SHA256 hashes of all artifacts
- `MD5SUMS` - MD5 hashes of all artifacts

### Contents of Each Package
- `parser` / `parser.exe` - Nodelist parser tool
- `server` / `server.exe` - Web server application
- `USAGE.txt` - Usage instructions
- `USAGE.md` - Detailed usage guide (if CLAUDE.md exists)
- `README.md` - Project README (if exists)
- `LICENSE` - License file (if exists)

## Supported Platforms

The release workflow builds for:
- **Linux**: amd64, arm64 (with CGO for DuckDB)
- **Windows**: amd64 (with CGO), arm64 (without CGO)
- **macOS**: amd64, arm64 (without CGO for cross-compilation)
- **FreeBSD**: amd64 (without CGO)

## Workflow Triggers

The release workflow can be triggered in two ways:

1. **Tag Push** (Recommended):
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

2. **Manual Dispatch**:
   - Go to GitHub Actions tab
   - Select "Release" workflow
   - Click "Run workflow"
   - Enter version (e.g., v0.1.0)

## Version Numbering

Follow semantic versioning (semver):
- `v0.1.0` - Initial release
- `v0.1.1` - Patch release (bug fixes)
- `v0.2.0` - Minor release (new features, backward compatible)
- `v1.0.0` - Major release (breaking changes or first stable)

## Pre-release Testing

Before creating a release:

1. **Run full test suite**:
   ```bash
   go test ./cmd/... ./internal/...
   ```

2. **Test building**:
   ```bash
   make build
   ```

3. **Test basic functionality**:
   ```bash
   # Test parser
   ./bin/parser -path test_nodelists/ -db test.duckdb -verbose
   
   # Test server
   ./bin/server -db test.duckdb -port 8080
   ```

4. **Verify cross-compilation** (optional):
   ```bash
   GOOS=linux GOARCH=amd64 go build ./cmd/parser
   GOOS=windows GOARCH=amd64 go build ./cmd/server
   ```

## Troubleshooting

### Build Failures
- Check Go version compatibility (requires Go 1.23+)
- Verify CGO dependencies are available
- Check for import errors after refactoring

### Release Workflow Failures
- Verify GitHub Actions has necessary permissions
- Check that secrets are configured if needed
- Ensure git tags follow the expected format (`v*`)

### Missing Artifacts
- Check workflow logs for cross-compilation errors
- Some platforms may build without CGO if cross-compilation tools aren't available
- Verify upload permissions and GitHub token scope