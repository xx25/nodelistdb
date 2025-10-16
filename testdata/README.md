# Test Data Fixtures

This directory contains test fixtures for NodelistDB testing.

## Directory Structure

```
testdata/
├── nodelists/          # Sample nodelist files
│   ├── valid/          # Valid nodelist files for parsing tests
│   ├── invalid/        # Invalid/malformed files for error handling tests
│   └── edge_cases/     # Edge cases (duplicates, special formats, etc.)
├── json/               # JSON test data
│   ├── nodes/          # Sample node data
│   ├── stats/          # Sample statistics data
│   └── filters/        # Sample filter configurations
├── sql/                # SQL seed data for integration tests
└── configs/            # Test configuration files
```

## Usage

Test fixtures can be loaded using the testutil helpers:

```go
import "github.com/nodelistdb/internal/testutil"

func TestParser(t *testing.T) {
    // Load nodelist fixture
    data := testutil.LoadFixture(t, "nodelists/valid/nodelist.001")

    // Or as string
    content := testutil.LoadFixtureString(t, "nodelists/valid/nodelist.001")
}
```

## Nodelist Files

### Valid Nodelists
- `nodelist.001` - Basic nodelist with various node types (Zone, Region, Hub, Pvt, Hold, Down)
- `nodelist.002` - Updated version showing node changes over time

### Invalid Nodelists
- `malformed.txt` - Contains malformed entries for error handling tests

### Edge Cases
- `duplicate_nodes.txt` - Contains duplicate node addresses to test conflict handling
- `unicode.txt` - Contains unicode characters for encoding tests
- `no_header.txt` - Missing date header

## JSON Data

Sample JSON data for testing serialization and API responses.

## SQL Data

SQL seed files for populating test databases with known data sets.

## Configuration Files

Sample configuration files for testing configuration loading and validation.
