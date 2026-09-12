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

The REST API is available at `/api` when the server is running. The OpenAPI
document at `/api/openapi.yaml` (Swagger UI at `/api/docs`) is the contract:
three tests in `internal/api` hold it against the router, the handlers'
response keys and the Go types they encode, so it cannot drift silently.
The web server's `/api/help` page is the short tour.

Conventions: the database holds several FTN networks and most endpoints take
`?domain=` (endpoints about one address resolve it from the address when
omitted; listings default to `fidonet` or to all networks as documented).
Searches require at least one constraint. Errors are `{error, status, time}`;
rate limiting answers `429` with `Retry-After`, an over-budget query `503`.

### Endpoints

**Networks and statistics:**
- `GET /api/networks` - FTN networks in the database with their latest nodelist date
- `GET /api/stats` - Network statistics for one date (`?date=`, nearest available), wrapped with the date actually used
- `GET /api/stats/dates` - Available nodelist dates
- `GET /api/flags` - FidoNet flag documentation (`?category=`, `?flag=`)
- `GET /api/nodelist/latest` - Newest nodelist file and its download URL

**Nodes:**
- `GET /api/nodes` - Search nodes
  - Query params: `domain`, `zone`, `net`, `node`, `system_name`, `location`, `sysop_name`, `node_type`, `is_cm`, `is_mo`, `has_inet`, `has_binkp`, `date_from`, `date_to`, `latest_only`, `limit` (max 500), `offset`
- `GET /api/nodes/{zone}/{net}/{node}` - Most recent entry of one address
- `GET /api/nodes/{zone}/{net}/{node}/history` - Every entry, with first and last dates
- `GET /api/nodes/{zone}/{net}/{node}/changes` - Change log
- `GET /api/nodes/{zone}/{net}/{node}/timeline` - Active/removed events for a chart
- `GET /api/sysops` - List sysops (`?name=`, `?limit=` max 200, `?offset=`)
- `GET /api/sysops/{name}/nodes` - A sysop's nodes

**Points (FTS-5002 pointlists):**
- `GET /api/nodes/{zone}/{net}/{node}/points` - Pointlist snapshot under a boss (`?date=`)
- `GET /api/points` - Search point entries
- `GET /api/points/{zone}/{net}/{node}/{point}` - One point; `/history` for every stored entry
- `GET /api/pointlists/dates` - Imported pointlist issues (`?source=`)
- `GET /api/pointlists/sources` - Imported pointlist series

**Reachability (testdaemon results):**
- `GET /api/nodes/{zone}/{net}/{node}/tests` - One node's test results in the window (`?days=`, 1-365) with per-protocol statistics
- `GET /api/nodes/{zone}/{net}/{node}/tests/detail?time=` - One test result in full, by its `test_time`
- `GET /api/reachability/nodes` - Each node's newest result, filtered by `status` (operational/failed) and `protocol` (binkp/ifcico/telnet/ftp/vmodem)
- `GET /api/reachability/trends` - Tested and operational counts per day (`?days=`, omit for all time)
- `GET /api/nodes/{zone}/{net}/{node}/ping` - Netmail PING history of one node with the paths mail walked
- `GET /api/analytics/pingtrace` - PING/TRACE summary
- `GET /api/software/binkp`, `/ifcico`, `/binkd` - Mailer software, version and OS distributions
- `GET /api/analytics/geo-hosting` - Hosting by country and provider
- Analytics endpoints take `?days=` (default 365) and `?domain=` (default all networks)

**PSTN / modem testing:**
- `GET /api/nodes/pstn` - Nodes with a dialable phone number
- `GET /api/nodes/pstn/dead`, `GET /api/nodes/pstn/recent-success` - Dead marks and recently reached numbers
- `POST /api/modem/results/direct`, `POST`/`DELETE /api/modem/pstn-dead` - Writes for the modem test caller (bearer API key; see "Modem-Test Result Submission" in CLAUDE.md)

**Operations and documentation:**
- `GET /api/health` - Health report
- `GET /api/cache/stats`, `/api/ratelimit/stats`, `/api/ftp/stats` - Counters, registered when the feature is on
- `GET /api/openapi.yaml` - OpenAPI specification
- `GET /api/docs` - Interactive Swagger UI documentation

### Example API Usage

```bash
# Networks in the database
curl "http://localhost:8080/api/networks"

# Nodes with internet connectivity, one entry per node
curl "http://localhost:8080/api/nodes?has_inet=true&latest_only=true&limit=10"

# One node in another network
curl "http://localhost:8080/api/nodes/21/1/100?domain=fsxnet"

# What the daemon found when it called a node this week
curl "http://localhost:8080/api/nodes/2/5001/100/tests?days=7"

# Nodes whose newest test failed today
curl "http://localhost:8080/api/reachability/nodes?status=failed&days=1"

# BinkP software distribution over the last year
curl "http://localhost:8080/api/software/binkp?days=365"
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

## Production Deployment

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

## License

MIT

## Support

For issues and feature requests, please use the GitHub issue tracker.
