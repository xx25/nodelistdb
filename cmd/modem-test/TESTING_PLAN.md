# Unit Testing Plan for Operator Failover

## Overview

This plan outlines unit tests needed for the operator failover implementation in `cmd/modem-test/`. Tests follow the codebase's established patterns: table-driven tests, mock objects, and builder patterns.

## Test Files to Create

### 1. `cmd/modem-test/operator_cache_test.go`

Tests for the BadgerDB-based operator cache.

```go
// Test Categories:

// 1. Cache Operations
func TestOperatorCache_GetSet(t *testing.T)
    - Set and retrieve an operator for a phone
    - Get returns (nil, false) for non-existent key
    - Set overwrites existing entry
    - Delete removes entry
    - Get after Delete returns (nil, false)

// 2. Cache TTL Behavior
func TestOperatorCache_TTL(t *testing.T)
    - Entry expires after TTL (requires time manipulation or short TTL)
    - Entry accessible before TTL expires

// 3. Cache Path Handling
func TestOperatorCache_PathExpansion(t *testing.T)
    - Expands "~/" to home directory
    - Creates directory if doesn't exist
    - Uses default path when empty

// 4. Disabled Cache
func TestOperatorCache_Disabled(t *testing.T)
    - NewOperatorCache returns nil when Enabled=false
    - Operations on nil cache are safe (no panics)

// 5. isUserBusy Function
func TestIsUserBusy(t *testing.T)
    tests := []struct {
        name     string
        cdr      *AsteriskCDRData
        expected bool
    }{
        {"nil CDR", nil, false},
        {"HangupCause 17", &AsteriskCDRData{HangupCause: 17}, true},
        {"HangupCause 16 (normal)", &AsteriskCDRData{HangupCause: 16}, false},
        {"BUSY disposition without cause 17", &AsteriskCDRData{Disposition: "BUSY", BillSec: 0, HangupCause: 21}, false},
        {"HangupCause 17 with ANSWERED", &AsteriskCDRData{HangupCause: 17, Disposition: "ANSWERED"}, true},
    }

// 6. Operator Reordering
func TestReorderOperatorsWithCached(t *testing.T)
    - Cached operator moved to first position
    - Other operators maintain relative order
    - Non-existent cached operator returns original order
    - Empty operators list returns empty
    - nil cached returns original order

// 7. FindOperatorByName
func TestFindOperatorByName(t *testing.T)
    - Finds operator by exact name match
    - Returns (_, -1, false) when not found
    - Returns correct index
```

### 2. `cmd/modem-test/config_test.go`

Tests for configuration validation.

```go
// 1. Operator Name Validation
func TestConfig_ValidateOperatorNames(t *testing.T)
    tests := []struct {
        name      string
        operators []OperatorConfig
        wantErr   bool
        errMsg    string
    }{
        {"single operator without name is valid", []OperatorConfig{{Prefix: "01"}}, false, ""},
        {"multiple operators require names", []OperatorConfig{{Prefix: "01"}, {Prefix: "02"}}, true, "name is required"},
        {"duplicate names rejected", []OperatorConfig{{Name: "A", Prefix: "01"}, {Name: "A", Prefix: "02"}}, true, "duplicate"},
        {"unique names valid", []OperatorConfig{{Name: "A", Prefix: "01"}, {Name: "B", Prefix: "02"}}, false, ""},
        {"empty operators list valid", []OperatorConfig{}, false, ""},
    }

// 2. OperatorCacheConfig Defaults
func TestConfig_OperatorCacheDefaults(t *testing.T)
    - Verify default TTL is 30 days
    - Verify default path is empty (uses ~/.modem-test/operator_cache)
    - Verify Enabled defaults to true

// 3. GetOperators
func TestConfig_GetOperators(t *testing.T)
    - Returns empty slice when not configured
    - Returns configured operators
```

### 3. `cmd/modem-test/worker_test.go`

Tests for worker and failover logic.

```go
// 1. phoneJob Structure
func TestPhoneJob_OperatorsField(t *testing.T)
    - Verify operators field accepts multiple operators
    - Verify backward compatibility with operatorName/operatorPrefix

// 2. RetryAttemptCallback Signature
func TestRetryAttemptCallback(t *testing.T)
    - Verify callback receives operator info
    - Verify callback can be nil (no panic)

// 3. FailoverResult Structure
func TestFailoverResult_Fields(t *testing.T)
    - Success case has SuccessOperator set
    - Failure case has LastOperator set
    - UserBusy case has correct flags
    - AllOperatorsFail case has correct flags
```

### 4. `cmd/modem-test/failover_test.go` (New Integration-style Tests)

Tests for the complete failover flow (may require mocking modem).

```go
// Mock modem/CDR for testing runTestWithFailover

type MockModemWorker struct {
    testResults []testResult  // Results to return in sequence
    callCount   int
}

// 1. Failover Sequence
func TestRunTestWithFailover_TriesOperatorsInOrder(t *testing.T)
    - First operator fails -> tries second
    - Second operator succeeds -> returns success with correct operator
    - Verifies TriedOperators count

// 2. Cache Integration
func TestRunTestWithFailover_UsesCachedOperator(t *testing.T)
    - With cached operator, tries it first
    - On success, doesn't try other operators
    - On failure, falls back to others

// 3. User Busy Handling
func TestRunTestWithFailover_UserBusyStopsFailover(t *testing.T)
    - When CDR shows cause 17, stops trying operators
    - Returns UserBusy=true in result
    - Does not clear cache

// 4. All Operators Fail
func TestRunTestWithFailover_AllFailClearsCache(t *testing.T)
    - All operators fail -> clears cache
    - Returns AllOperatorsFail=true
    - LastOperator is set to last tried

// 5. Empty Operators List
func TestRunTestWithFailover_EmptyOperators(t *testing.T)
    - Calls runTest directly without prefix
    - Returns result without operator info

// 6. Cache Stale Entry
func TestRunTestWithFailover_StaleCacheEntry(t *testing.T)
    - Cached operator not in config -> ignored
    - Uses config order instead
    - Logs appropriate message
```

### 5. `cmd/modem-test/nodelist_source_test.go`

Tests for ScheduleNodes job generation.

```go
// 1. Job Generation
func TestScheduleNodes_SingleJobPerNode(t *testing.T)
    - Emits one job per node (not per operator)
    - Job contains full operators list
    - Job has correct node info

// 2. Empty Operators
func TestScheduleNodes_EmptyOperators(t *testing.T)
    - Works with empty operators list
    - Job.operators is empty slice

// 3. Context Cancellation
func TestScheduleNodes_Cancellation(t *testing.T)
    - Stops emitting when context cancelled
    - Channel is closed
```

## Mock Objects Needed

### MockAsteriskCDRData Builder

```go
type AsteriskCDRBuilder struct {
    cdr AsteriskCDRData
}

func NewAsteriskCDRBuilder() *AsteriskCDRBuilder {
    return &AsteriskCDRBuilder{
        cdr: AsteriskCDRData{
            Disposition: "ANSWERED",
            HangupCause: 16,
            BillSec:     60,
        },
    }
}

func (b *AsteriskCDRBuilder) WithUserBusy() *AsteriskCDRBuilder {
    b.cdr.HangupCause = 17
    b.cdr.Disposition = "BUSY"
    b.cdr.BillSec = 0
    return b
}

func (b *AsteriskCDRBuilder) WithNoAnswer() *AsteriskCDRBuilder {
    b.cdr.Disposition = "NO ANSWER"
    b.cdr.HangupCause = 19
    b.cdr.BillSec = 0
    return b
}

func (b *AsteriskCDRBuilder) Build() *AsteriskCDRData {
    cdr := b.cdr
    return &cdr
}
```

### MockOperatorCache

```go
type MockOperatorCache struct {
    entries map[string]*CachedOperator
    setCalls []struct{ phone string; op OperatorConfig }
    deleteCalls []string
}

func (m *MockOperatorCache) Get(phone string) (*CachedOperator, bool) {
    if m.entries == nil {
        return nil, false
    }
    cached, ok := m.entries[phone]
    return cached, ok
}

func (m *MockOperatorCache) Set(phone string, op OperatorConfig) error {
    m.setCalls = append(m.setCalls, struct{phone string; op OperatorConfig}{phone, op})
    if m.entries == nil {
        m.entries = make(map[string]*CachedOperator)
    }
    m.entries[phone] = &CachedOperator{
        OperatorName:   op.Name,
        OperatorPrefix: op.Prefix,
        LastSuccess:    time.Now(),
    }
    return nil
}

func (m *MockOperatorCache) Delete(phone string) error {
    m.deleteCalls = append(m.deleteCalls, phone)
    delete(m.entries, phone)
    return nil
}
```

## Test Data / Fixtures

### Standard Operators

```go
var testOperators = []OperatorConfig{
    {Name: "Primary", Prefix: "01"},
    {Name: "Secondary", Prefix: "02"},
    {Name: "Tertiary", Prefix: "03"},
}
```

### Standard Phone Jobs

```go
func newTestPhoneJob(phone string, operators []OperatorConfig) phoneJob {
    return phoneJob{
        phone:          phone,
        operators:      operators,
        testNum:        1,
        nodeAddress:    "2:5001/100",
        nodeSystemName: "Test BBS",
        nodeLocation:   "Test City",
        nodeSysop:      "Test Sysop",
    }
}
```

## Implementation Priority

1. **High Priority** (Core functionality)
   - `TestIsUserBusy` - Critical for failover decision
   - `TestConfig_ValidateOperatorNames` - Config validation
   - `TestReorderOperatorsWithCached` - Cache reordering logic
   - `TestFindOperatorByName` - Cache lookup

2. **Medium Priority** (Integration)
   - `TestOperatorCache_GetSet` - Cache operations
   - `TestScheduleNodes_SingleJobPerNode` - Job generation
   - `TestRunTestWithFailover_*` - Failover flow (requires more setup)

3. **Lower Priority** (Edge cases)
   - `TestOperatorCache_TTL` - Time-dependent
   - `TestOperatorCache_PathExpansion` - OS-dependent
   - Context cancellation tests

## Running Tests

```bash
# Run all modem-test tests
go test ./cmd/modem-test/... -v

# Run specific test file
go test ./cmd/modem-test/... -v -run TestIsUserBusy

# Run with coverage
go test ./cmd/modem-test/... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Run short tests only (skip integration)
go test ./cmd/modem-test/... -short
```

## Notes

- BadgerDB tests should use `t.TempDir()` for isolation
- Mock the modem for `runTestWithFailover` tests (don't dial actual numbers)
- Use `testing.Short()` to skip slow tests
- Follow existing patterns in `internal/storage/software_parsers_test.go`
