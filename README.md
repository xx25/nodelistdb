# NodelistDB

FidoNet nodelist parser and storage system built with Go and ClickHouse. Provides CLI tools, REST API, and web interface for managing and analyzing FidoNet node data.

## Features

- **High-Performance Storage**: ClickHouse-backed database optimized for time-series analytics
- **FidoNet Nodelist Parsing**: Parse and import FidoNet nodelist files with concurrent processing
- **Web Interface**: Search and browse node data through a modern web UI
- **REST API**: Programmatic access to node data and statistics
- **FTP Server**: Optional FTP server for nodelist distribution (anonymous read-only access)
- **Node Testing**: Automated connectivity testing for Binkp, IFCico, Telnet, and FTP protocols
- **Analytics**: Geographic analysis, protocol statistics, and historical trends

## Quick Start

### Prerequisites

- Go 1.21 or higher
- ClickHouse server (local or remote)
- Git

### Installation

1. **Clone the repository:**
```bash
git clone https://github.com/yourusername/nodelistdb.git
cd nodelistdb
```

2. **Install dependencies:**
```bash
make deps
```

3. **Configure ClickHouse connection:**

Create `config.yaml` from the example:
```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml` with your ClickHouse connection details:
```yaml
database:
  type: clickhouse
  clickhouse:
    host: localhost
    port: 9000
    database: nodelistdb
    username: default
    password: ""
```

4. **Build the binaries:**
```bash
make build
```

### Usage

#### Parse and Import Nodelist Files

```bash
# Import a single nodelist file
./bin/parser -config config.yaml -path /path/to/nodelist.365

# Import all nodelists in a directory (recursively)
./bin/parser -config config.yaml -path /path/to/nodelists -recursive

# Enable concurrent processing for faster imports
./bin/parser -config config.yaml -path /path/to/nodelists -concurrent -workers 8

# Verbose output for debugging
./bin/parser -config config.yaml -path /path/to/nodelists -verbose
```

#### Run the Web Server

```bash
# Start the web server
./bin/server -config config.yaml -host localhost -port 8080

# Access the web interface at http://localhost:8080
# Access the REST API at http://localhost:8080/api
```

#### Run Node Testing Daemon

```bash
# Test node connectivity
./bin/testdaemon -config config.yaml test ifcico 2:5001/100

# Run as a daemon for continuous testing
./bin/testdaemon -config config.yaml daemon
```

## Configuration

NodelistDB uses YAML configuration files. See `config.example.yaml` for all available options.

### Basic Configuration

```yaml
database:
  type: clickhouse
  clickhouse:
    host: localhost
    port: 9000
    database: nodelistdb
    username: default
    password: ""

server:
  host: localhost
  port: 8080

daemon:
  interval: 24h
  batch_size: 100
  workers: 10
  timeout: 30s
```

### FTP Server Configuration (Optional)

To enable the FTP server for nodelist distribution:

```yaml
ftp:
  enabled: true
  host: "0.0.0.0"
  port: 2121
  nodelist_path: /path/to/nodelists
  max_connections: 10
  passive_port_min: 50000
  passive_port_max: 50100
  idle_timeout: 300s
```

## CLI Reference

### Parser Options

- `-config <path>`: Configuration file path (default: config.yaml)
- `-path <path>`: Nodelist file or directory to parse (required)
- `-recursive`: Scan directories recursively
- `-concurrent`: Enable concurrent processing
- `-workers <n>`: Number of worker threads (default: 4)
- `-batch <n>`: Batch size for inserts (default: 1000)
- `-verbose`: Enable verbose logging
- `-create-fts`: Create full-text search indexes (default: true)
- `-rebuild-fts`: Rebuild FTS indexes only

### Server Options

- `-config <path>`: Configuration file path (default: config.yaml)
- `-host <addr>`: Server host (default: localhost)
- `-port <n>`: Server port (default: 8080)

### TestDaemon Options

- `-config <path>`: Configuration file path (default: config.yaml)
- `test <protocol> <address>`: Test a single node
- `daemon`: Run as a continuous testing daemon

## REST API

The REST API is available at `/api` when the server is running.

### Endpoints

**Node Operations:**
- `GET /api/nodes` - Search nodes with filtering
  - Query params: `zone`, `net`, `node`, `system_name`, `location`, `sysop_name`, `node_type`, `is_cm`, `date_from`, `date_to`, `limit`, `offset`
- `GET /api/nodes/{zone}/{net}/{node}` - Get specific node details
- `GET /api/nodes/{zone}/{net}/{node}/history` - Get complete node history
- `GET /api/nodes/{zone}/{net}/{node}/changes` - Get node change log
- `GET /api/nodes/{zone}/{net}/{node}/timeline` - Get node timeline visualization

**Sysop Operations:**
- `GET /api/sysops` - List sysops with filtering
- `GET /api/sysops/{name}/nodes` - Get all nodes for a specific sysop

**Statistics:**
- `GET /api/stats` - Get network statistics
- `GET /api/stats/dates` - Get available nodelist dates

**Software Analytics:**
- `GET /api/software/binkp` - BinkP software distribution
- `GET /api/software/ifcico` - IFCico software distribution
- `GET /api/software/binkd` - Detailed Binkd statistics
- `GET /api/software/trends` - Software usage trends (stub)

**Reference & Documentation:**
- `GET /api/flags` - Get FidoNet flag documentation
- `GET /api/nodelist/latest` - Get latest nodelist information
- `GET /api/openapi.yaml` - OpenAPI specification
- `GET /api/docs` - Interactive Swagger UI documentation

### Example API Usage

```bash
# Search for nodes with internet connectivity
curl "http://localhost:8080/api/nodes?has_inet=true&limit=10"

# Get specific node details
curl "http://localhost:8080/api/nodes/2/5001/100"

# Get node history
curl "http://localhost:8080/api/nodes/2/5001/100/history"

# Get network statistics
curl "http://localhost:8080/api/stats"

# Get BinkP software distribution
curl "http://localhost:8080/api/software/binkp?days=365"

# Get all nodes for a sysop
curl "http://localhost:8080/api/sysops/John_Doe/nodes"
```

## Architecture

NodelistDB uses a layered architecture:

```
┌─────────────────────────────────────┐
│  CLI Tools & Web Server             │
│  (cmd/parser, cmd/server)           │
└─────────────┬───────────────────────┘
              │
┌─────────────▼───────────────────────┐
│  Application Layer                  │
│  (API handlers, business logic)     │
└─────────────┬───────────────────────┘
              │
┌─────────────▼───────────────────────┐
│  Storage Layer                      │
│  (internal/storage)                 │
└─────────────┬───────────────────────┘
              │
┌─────────────▼───────────────────────┐
│  Database Layer                     │
│  (internal/database/clickhouse.go)  │
└─────────────┬───────────────────────┘
              │
┌─────────────▼───────────────────────┐
│  ClickHouse Database                │
└─────────────────────────────────────┘
```

### Key Components

- **Parser Layer** (`internal/parser/`): FidoNet nodelist format parsing
- **Storage Layer** (`internal/storage/`): Thread-safe database operations with query builders
- **Database Layer** (`internal/database/`): ClickHouse connection management
- **Testing Layer** (`internal/testing/`): Node connectivity testing and aggregation
- **API Layer** (`internal/api/`): REST API handlers
- **Web Layer** (`internal/web/`): Web interface handlers

## Database Schema

NodelistDB uses an optimized ClickHouse schema:

### Main Tables

- **nodes**: Core nodelist data with MergeTree engine
  - Primary key: `(zone, net, node, nodelist_date, conflict_sequence)`
  - Arrays for flags, phone numbers, IPs, etc.
  - Efficient compression and indexing

- **node_test_results**: Connectivity test results
  - Test timestamps, protocols, success/failure
  - Aggregation support for multi-hostname nodes
  - Geographic data from IP resolution

## Development

### Build Commands

```bash
make build          # Build all binaries
make build-parser   # Build parser only
make build-server   # Build server only
make test           # Run tests
make test-coverage  # Run tests with coverage
make fmt            # Format code
make lint           # Run linter
make clean          # Clean build artifacts
```

### Testing with Sample Data

Sample nodelist files are provided in `test_nodelists/`:

```bash
./bin/parser -config config.yaml -path ./test_nodelists -verbose
```

### Running Tests

```bash
# Run all tests
go test -v ./...

# Run with coverage
make test-coverage

# Run specific package tests
go test -v ./internal/parser/...
```

## Migration from DuckDB

If you're migrating from a previous DuckDB-based installation, see `MIGRATION.md` for detailed instructions.

Key changes:
- Configuration simplified to ClickHouse-only
- Removed `-db` flag from CLI tools (use `-config` instead)
- All data stored in ClickHouse for better performance and scalability

## Production Deployment

### ClickHouse Server

For production deployments, use a dedicated ClickHouse server:

```yaml
database:
  type: clickhouse
  clickhouse:
    host: 10.121.17.211  # Production server
    port: 9000
    database: nodelistdb
    username: nodelistdb_user
    password: "secure_password"
```

### Performance Tuning

- Adjust `workers` and `batch_size` based on your hardware
- Use `concurrent` mode for large nodelist imports
- Configure ClickHouse with appropriate memory limits
- Consider using MergeTree table optimizations

## Analytics

Python-based analytics tools are available in `scripts/analytics/`:

```bash
cd scripts/analytics
pip install -r requirements.txt

# Run IP geolocation analysis
python main.py --analysis ip_geolocation --db-type clickhouse --clickhouse-host localhost

# Generate JSON report
python main.py --analysis ip_geolocation --output json --output-file report.json
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Submit a pull request

## Troubleshooting

### Connection Issues

If you can't connect to ClickHouse:

```bash
# Test connection
clickhouse-client --host localhost --query "SELECT version()"

# Check server logs
journalctl -u clickhouse-server -f
```

### Import Errors

If nodelist import fails:

```bash
# Use verbose mode for debugging
./bin/parser -config config.yaml -path /path/to/nodelists -verbose

# Check nodelist file format
head -20 /path/to/nodelist.365
```

### Performance Issues

If imports are slow:

- Enable concurrent processing: `-concurrent -workers 8`
- Increase batch size: `-batch 5000`
- Check ClickHouse server resources
- Review ClickHouse query logs

## Documentation

- `CLAUDE.md`: Developer guide for working with the codebase
- `MIGRATION_STATUS.md`: DuckDB to ClickHouse migration progress
- `IMPLEMENTATION_STATUS.md`: Feature implementation tracking

## License

MIT

## Support

For issues and feature requests, please use the GitHub issue tracker.
