# Modem Daemon Implementation Plan

## Overview

This document describes the implementation plan for the modem daemon (`cmd/modem-daemon/`) - a standalone Go binary that runs on remote servers with physical modem hardware to test FidoNet nodes via PSTN.

**Simplified Architecture:** This daemon uses a single modem. Tests are executed sequentially, one node at a time.

## Dependencies

- **go-serial** library from `/home/dp/src/go-serial` (github.com/mfkenney/go-serial/v2)
- Reuse existing **EMSI protocol** implementation from `internal/testing/protocols/emsi/`
- Reuse existing **logging** package from `internal/logging/`

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         MODEM DAEMON BINARY                              │
│                                                                          │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────────┐  │
│  │   API Client    │    │   Single Modem  │    │   EMSI Session      │  │
│  │                 │    │                 │    │   (reused from      │  │
│  │ - GetNodes      │    │ - Serial port   │    │   internal/testing) │  │
│  │ - MarkInProgress│    │ - AT commands   │    │                     │  │
│  │ - SubmitResults │    │ - Dial/Hangup   │    │ - Handshake         │  │
│  │ - Heartbeat     │    │ - State machine │    │ - Parse EMSI_DAT    │  │
│  └────────┬────────┘    └────────┬────────┘    └──────────┬──────────┘  │
│           │                      │                        │              │
│           │              ┌───────┴───────┐                │              │
│           │              │               │                │              │
│           ▼              ▼               ▼                ▼              │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                        Test Executor                              │   │
│  │                                                                   │   │
│  │  1. Get nodes from API                                           │   │
│  │  2. Mark in_progress                                             │   │
│  │  3. For each node (sequential):                                  │   │
│  │     a. Dial phone number                                         │   │
│  │     b. Wait for carrier (DCD)                                    │   │
│  │     c. Perform EMSI handshake                                    │   │
│  │     d. Collect results                                           │   │
│  │     e. Hangup + Reset if needed                                  │   │
│  │     f. Wait inter-call delay                                     │   │
│  │  4. Submit results to API                                        │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                      Heartbeat Goroutine                          │   │
│  │  - Runs every 60s                                                 │   │
│  │  - Reports status, modem state, test counts                       │   │
│  └──────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ HTTPS
                                    ▼
                          ┌─────────────────────┐
                          │   NodelistDB Server │
                          │   /api/modem/*      │
                          └─────────────────────┘
```

## Directory Structure

```
cmd/modem-daemon/
├── main.go              # Entry point, CLI flags, signal handling
├── config.go            # Configuration loading and validation
├── daemon.go            # Main daemon loop and orchestration
└── README.md            # Usage documentation

internal/modemd/
├── api/
│   ├── client.go        # HTTP client for server API
│   └── types.go         # API request/response types
├── modem/
│   ├── modem.go         # Single modem abstraction with state machine
│   ├── at_commands.go   # AT command handling
│   ├── dial.go          # Dialing and carrier detection
│   └── conn.go          # net.Conn wrapper for serial port
└── executor/
    └── executor.go      # Test execution logic
```

## Data Types

### API Types (`internal/modemd/api/types.go`)

```go
// NodeID uniquely identifies a node for the modem testing API
type NodeID struct {
    Zone int `json:"zone"`
    Net  int `json:"net"`
    Node int `json:"node"`
}

// String returns FidoNet address format "zone:net/node"
func (n NodeID) String() string {
    return fmt.Sprintf("%d:%d/%d", n.Zone, n.Net, n.Node)
}

// Node represents a node to be tested via modem
type Node struct {
    ID          NodeID   `json:"id"`
    Phone       string   `json:"phone"`        // Phone number to dial
    Flags       []string `json:"flags"`        // Node flags (for logging)
    SystemName  string   `json:"system_name"`  // Expected system name (optional)
    LastTested  *time.Time `json:"last_tested"` // When last tested (optional)
}

// TestResult contains the outcome of testing a single node
type TestResult struct {
    NodeID        NodeID    `json:"node_id"`
    Success       bool      `json:"success"`
    Error         string    `json:"error,omitempty"`

    // Connection info
    ConnectSpeed  int       `json:"connect_speed,omitempty"`  // 33600, 28800, etc.
    ModemProtocol string    `json:"modem_protocol,omitempty"` // V.34, V.32bis, etc.
    RingCount     int       `json:"ring_count,omitempty"`
    DialTimeMs    int       `json:"dial_time_ms"`
    CarrierTimeMs int       `json:"carrier_time_ms,omitempty"`

    // EMSI results (if handshake succeeded)
    SystemName    string    `json:"system_name,omitempty"`
    MailerInfo    string    `json:"mailer_info,omitempty"`
    Addresses     []string  `json:"addresses,omitempty"`
    AddressValid  bool      `json:"address_valid,omitempty"`

    // Metadata
    TestedAt      time.Time `json:"tested_at"`
}

// HeartbeatStatus reports daemon health to the server
type HeartbeatStatus struct {
    Status         string    `json:"status"`           // "active", "idle", "error"
    ModemReady     bool      `json:"modem_ready"`
    TestsCompleted int       `json:"tests_completed"`
    TestsFailed    int       `json:"tests_failed"`
    LastTestTime   time.Time `json:"last_test_time,omitempty"`
    ErrorMessage   string    `json:"error_message,omitempty"`
}

// Helper function to extract NodeIDs from a slice of Nodes
func ExtractNodeIDs(nodes []Node) []NodeID {
    ids := make([]NodeID, len(nodes))
    for i, n := range nodes {
        ids[i] = n.ID
    }
    return ids
}
```

### Modem Types (`internal/modemd/modem/`)

```go
// ModemStatus represents the current state of modem control lines
type ModemStatus struct {
    DCD bool // Data Carrier Detect - true when connected
    DSR bool // Data Set Ready - modem is powered and ready
    CTS bool // Clear To Send - flow control
    RI  bool // Ring Indicator - incoming call (not used for outbound)
}

// ModemInfo contains modem identification from ATI response
type ModemInfo struct {
    Manufacturer string // e.g., "USRobotics", "ZyXEL"
    Model        string // e.g., "Courier", "U-1496"
    Firmware     string // Firmware version if available
    RawResponse  string // Full ATI response for debugging
}
```

## Design Contracts & Invariants

These contracts MUST be maintained during implementation:

### 1. Dial Always Returns Non-Nil Result
```go
// Modem.Dial ALWAYS returns a non-nil *DialResult, even on error.
// This allows callers to safely access DialResult.Error and DialResult.DialTime
// without nil checks after the error path.
func (m *Modem) Dial(...) (*DialResult, error)  // DialResult is NEVER nil
```

### 2. Node Lifecycle: No Orphaned In-Progress Nodes
```
GetNodes → MarkInProgress → TestNodes → SubmitResults → clearInProgress
                ↓                            ↓
           (on failure)               (on failure)
                ↓                            ↓
           ReleaseNodes ──(success)──→ clearInProgress
                ↓
           (failure)
                ↓
           moveToUnreleased ──→ [unreleasedNodes queue]
                                       ↓
                              retryUnreleasedNodes (each cycle)
                                       ↓
                              Stop() releases all remaining
```
**Two-tier tracking:**
- `inProgressNodes`: Current cycle's nodes (cleared at end of cycle)
- `unreleasedNodes`: Accumulated failed releases (persists across cycles, retried each cycle)

**Guarantees:**
- Every node marked `in_progress` MUST eventually be either submitted or released
- On release failure: nodes move to `unreleasedNodes` queue (not lost on next cycle)
- Each cycle starts by retrying `unreleasedNodes` before fetching new work
- On daemon shutdown: release both `inProgressNodes` + `unreleasedNodes` with retry (3 attempts)

### 3. Serial Port State Machine
```
[Command Mode] ──CONNECT──→ [Data Mode] ──Hangup──→ [Command Mode]
                    │                         │
                    ├─ Flush buffers          ├─ DTR drop: no OK expected
                    └─ Set inDataMode=true    ├─ Escape seq: wait for OK
                                              ├─ Flush buffers
                                              └─ Set inDataMode=false
```
- AT commands only valid in Command Mode
- EMSI data transfer only valid in Data Mode
- Buffer flush required on every transition
- Two hangup methods: DTR drop (fast, no OK) or escape sequence (slower, waits for OK)

### 4. Modem Recovery After Failed Test
```
Test Failed (dial error, EMSI error, carrier lost)
    ↓
Hangup() ──(success)──→ Wait inter-call delay → Next test
    ↓
 (failure)
    ↓
Reset() ──(success)──→ Wait inter-call delay → Next test
    ↓
 (failure)
    ↓
Log error, continue (modem may recover on next dial)
```
- Always attempt Hangup() first (faster)
- If Hangup() fails, try Reset() (ATZ)
- Never skip inter-call delay even after failures

### 5. Daemon Stop is Idempotent
```go
daemon.Stop()  // First call: closes stop channel, waits, cleans up
daemon.Stop()  // Second call: no-op (sync.Once)
daemon.Stop()  // Third call: no-op
```

### 6. ModemConn Lifecycle
```go
// ModemConn is a thin wrapper - it does NOT own the connection lifecycle
// - Read/Write: delegate to serial port with timeouts
// - Close(): NO-OP - does not hang up or close serial port
// - Caller must call Modem.Hangup() separately after EMSI session
```
This allows the caller to handle hangup explicitly and perform recovery if needed.

## Implementation Phases

### Phase 1: Core Infrastructure

#### 1.1 Configuration (`cmd/modem-daemon/config.go`)

```go
type Config struct {
    // API connection
    API struct {
        URL    string `yaml:"url"`     // https://nodelistdb.example.com/api/modem
        Key    string `yaml:"key"`     // API key (plain text, hashed on server)
    } `yaml:"api"`

    // EMSI identity
    Identity struct {
        Address    string `yaml:"address"`     // 2:5001/100
        SystemName string `yaml:"system_name"` // NodelistDB Modem Tester
        Sysop      string `yaml:"sysop"`       // Test Operator
        Location   string `yaml:"location"`    // Moscow, Russia
    } `yaml:"identity"`

    // Single modem configuration
    Modem ModemConfig `yaml:"modem"`

    // Timeouts
    Timeouts struct {
        Dial      time.Duration `yaml:"dial"`       // 200s - wait for CONNECT (modem S7 register controls actual timeout)
        Carrier   time.Duration `yaml:"carrier"`    // 5s - wait for DCD after CONNECT
        EMSI      time.Duration `yaml:"emsi"`       // 30s - EMSI handshake
        ATCommand time.Duration `yaml:"at_command"` // 5s - AT command response
    } `yaml:"timeouts"`

    // Polling
    Polling struct {
        Interval      time.Duration `yaml:"interval"`        // 30s - between cycles
        BatchSize     int           `yaml:"batch_size"`      // 10 (sequential, keep small)
        InterCallDelay time.Duration `yaml:"inter_call_delay"` // 3s - between calls
    } `yaml:"polling"`

    // Heartbeat
    Heartbeat struct {
        Interval time.Duration `yaml:"interval"` // 60s
    } `yaml:"heartbeat"`

    // Logging
    Logging struct {
        Level   string `yaml:"level"`   // debug, info, warn, error
        File    string `yaml:"file"`    // optional log file
        Console bool   `yaml:"console"` // log to console
    } `yaml:"logging"`
}

type ModemConfig struct {
    Device       string   `yaml:"device"`        // /dev/ttyUSB0
    InitString   string   `yaml:"init"`          // ATZ or AT&F
    BaudRate     int      `yaml:"baud_rate"`     // 115200 (serial port speed)
    Protocols    []string `yaml:"protocols"`     // [V34, V32B, ZYX] - for info only
    MaxSpeed     int      `yaml:"max_speed"`     // 33600 - for info only
    DialPrefix   string   `yaml:"dial_prefix"`   // Optional: ATDT (default) or ATD
    HangupMethod string   `yaml:"hangup_method"` // "dtr" (default) or "escape"
}

// Validate checks configuration and returns error if invalid
func (c *Config) Validate() error {
    var errs []string

    // API validation
    if c.API.URL == "" {
        errs = append(errs, "api.url is required")
    }
    if c.API.Key == "" {
        errs = append(errs, "api.key is required")
    }

    // Identity validation
    if c.Identity.Address == "" {
        errs = append(errs, "identity.address is required")
    }

    // Modem validation
    if c.Modem.Device == "" {
        errs = append(errs, "modem.device is required")
    }
    validBaudRates := map[int]bool{9600: true, 19200: true, 38400: true, 57600: true, 115200: true}
    if !validBaudRates[c.Modem.BaudRate] {
        errs = append(errs, "modem.baud_rate must be 9600, 19200, 38400, 57600, or 115200")
    }
    if c.Modem.HangupMethod != "" && c.Modem.HangupMethod != "dtr" && c.Modem.HangupMethod != "escape" {
        errs = append(errs, "modem.hangup_method must be 'dtr' or 'escape'")
    }

    // Timeout defaults and validation
    if c.Timeouts.Dial == 0 {
        c.Timeouts.Dial = 200 * time.Second
    }
    if c.Timeouts.Carrier == 0 {
        c.Timeouts.Carrier = 5 * time.Second
    }
    if c.Timeouts.EMSI == 0 {
        c.Timeouts.EMSI = 30 * time.Second
    }
    if c.Timeouts.ATCommand == 0 {
        c.Timeouts.ATCommand = 5 * time.Second
    }

    // Polling defaults
    if c.Polling.Interval == 0 {
        c.Polling.Interval = 30 * time.Second
    }
    if c.Polling.BatchSize == 0 {
        c.Polling.BatchSize = 10
    }
    if c.Polling.InterCallDelay == 0 {
        c.Polling.InterCallDelay = 3 * time.Second
    }

    // Heartbeat default
    if c.Heartbeat.Interval == 0 {
        c.Heartbeat.Interval = 60 * time.Second
    }

    // Modem defaults
    if c.Modem.DialPrefix == "" {
        c.Modem.DialPrefix = "ATDT"
    }
    if c.Modem.HangupMethod == "" {
        c.Modem.HangupMethod = "dtr"
    }
    if c.Modem.InitString == "" {
        c.Modem.InitString = "ATZ"
    }

    if len(errs) > 0 {
        return fmt.Errorf("config validation failed: %s", strings.Join(errs, "; "))
    }
    return nil
}
```

#### 1.2 API Client (`internal/modemd/api/client.go`)

```go
type Client struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}

func NewClient(baseURL, apiKey string) *Client

// GetNodes fetches assigned nodes from server
func (c *Client) GetNodes(ctx context.Context, limit int, onlyCallable bool) ([]Node, int, error)

// MarkInProgress marks nodes as being tested
func (c *Client) MarkInProgress(ctx context.Context, nodeIDs []NodeID) (int, error)

// SubmitResults submits test results
func (c *Client) SubmitResults(ctx context.Context, results []TestResult) error

// Heartbeat sends daemon status to server
func (c *Client) Heartbeat(ctx context.Context, status HeartbeatStatus) error

// ReleaseNodes releases nodes back to queue
func (c *Client) ReleaseNodes(ctx context.Context, nodeIDs []NodeID, reason string) error
```

### Phase 2: Modem Abstraction

#### 2.1 Single Modem (`internal/modemd/modem/modem.go`)

```go
type Modem struct {
    config       ModemConfig
    port         *serial.Port
    mu           sync.Mutex
    initialized  bool
    inDataMode   bool          // true after CONNECT, false after hangup/reset
}

// ModemMode represents the current modem operating mode
type ModemMode int

const (
    ModeCommand ModemMode = iota // AT command mode (before dial or after hangup)
    ModeData                     // Data mode (after CONNECT, for EMSI/data transfer)
)

// State transition diagram:
//   [Command Mode] --CONNECT--> [Data Mode] --Hangup--> [Command Mode]
//
// CRITICAL: Before transitioning to Data Mode after CONNECT:
//   1. Fully consume the CONNECT response line (e.g., "CONNECT 33600/ARQ/V34")
//   2. Flush serial read buffer to discard any trailing characters
//   3. Set inDataMode = true
//   4. Only then pass serial port to EMSI session
//
// Hangup methods (choose one based on config):
//
// Method A: DTR Drop (preferred, faster)
//   1. Drop DTR (SetDTR(false))
//   2. Wait 500ms for modem to detect
//   3. Raise DTR (SetDTR(true))
//   4. Wait 100ms for modem to stabilize
//   5. Set inDataMode = false
//   6. Flush buffers (modem may emit garbage during hangup)
//   7. Send ATZ to ensure known state (optional but recommended)
//   NOTE: No OK response expected from DTR drop itself
//
// Method B: Escape Sequence (fallback if DTR not reliable)
//   1. Wait 1000ms guard time (no data sent)
//   2. Send "+++" (escape sequence)
//   3. Wait 1000ms guard time
//   4. Wait for "OK" response (timeout: 2s)
//   5. Send "ATH" (hangup command)
//   6. Wait for "OK" response (timeout: 2s)
//   7. Set inDataMode = false
//   8. Flush buffers
//
// On carrier loss (DCD drops unexpectedly):
//   1. Modem automatically returns to command mode
//   2. Set inDataMode = false
//   3. Flush buffers
//   4. Send ATZ to ensure known state

func NewModem(cfg ModemConfig) (*Modem, error)

// Open opens serial port and initializes modem
func (m *Modem) Open() error

// Close closes serial port
func (m *Modem) Close() error

// Reset resets modem to known state (ATZ), can be called in any mode
// Use this for recovery after failed Hangup or stuck modem
func (m *Modem) Reset() error

// Dial dials a phone number, returns connection info
// Uses both dial timeout and carrier timeout from config
func (m *Modem) Dial(phone string, dialTimeout, carrierTimeout time.Duration) (*DialResult, error)

// Hangup terminates call, returns error if failed
// Caller should call Reset() if Hangup fails
func (m *Modem) Hangup() error

// GetConn returns net.Conn-compatible wrapper for EMSI session
// IMPORTANT: The returned conn.Close() is a NO-OP - caller must call Hangup() separately
func (m *Modem) GetConn() net.Conn

// SendAT sends AT command and waits for response
func (m *Modem) SendAT(cmd string, timeout time.Duration) (string, error)

// GetModemStatus returns current modem status (DCD, DSR, CTS, RI)
func (m *Modem) GetModemStatus() (*ModemStatus, error)

// IsReady returns true if modem is initialized and in command mode
func (m *Modem) IsReady() bool

// InDataMode returns true if modem is in data mode (after CONNECT)
func (m *Modem) InDataMode() bool
```

#### 2.2 AT Command Handler (`internal/modemd/modem/at_commands.go`)

```go
// Common AT commands
const (
    ATZ     = "ATZ"          // Reset to profile 0
    ATI     = "ATI"          // Modem identification
    ATDT    = "ATDT"         // Dial tone
    ATDP    = "ATDP"         // Dial pulse
    ATH     = "ATH"          // Hangup
    ATH0    = "ATH0"         // Hangup (explicit)
    ATO     = "ATO"          // Return to online mode
    ATE0    = "ATE0"         // Echo off
    ATE1    = "ATE1"         // Echo on
    ATQ0    = "ATQ0"         // Result codes on
    ATV1    = "ATV1"         // Verbose result codes
    ATX4    = "ATX4"         // Extended result codes
    ATS0    = "ATS0=0"       // Auto-answer off
)

// Response codes
const (
    OK          = "OK"
    CONNECT     = "CONNECT"
    RING        = "RING"
    NO_CARRIER  = "NO CARRIER"
    ERROR       = "ERROR"
    NO_DIALTONE = "NO DIALTONE"
    BUSY        = "BUSY"
    NO_ANSWER   = "NO ANSWER"
)

// ParseConnectSpeed extracts speed and protocol from "CONNECT 33600/ARQ/V34" response
func ParseConnectSpeed(response string) (speed int, protocol string, err error)

// ParseModemInfo extracts modem model from ATI response
func ParseModemInfo(response string) *ModemInfo
```

#### 2.3 Dial Logic (`internal/modemd/modem/dial.go`)

```go
type DialResult struct {
    Success      bool
    ConnectSpeed int           // 33600, 28800, etc.
    Protocol     string        // V.34, V.32bis, etc.
    Error        string        // NO CARRIER, BUSY, etc. (modem response)
    RingCount    int           // Number of rings before answer
    DialTime     time.Duration // Time from dial to CONNECT/failure
    CarrierTime  time.Duration // Time from CONNECT to stable DCD
}

// Dial dials a phone number and returns connection info.
// IMPORTANT: Always returns a non-nil DialResult, even on error.
// On error, DialResult.Success will be false and DialResult.Error will be set.
// This contract ensures callers can safely access DialResult fields.
func (m *Modem) Dial(phone string, dialTimeout, carrierTimeout time.Duration) (*DialResult, error) {
    startTime := time.Now()
    result := &DialResult{} // Always initialize non-nil

    // 1. Validate modem state
    if m.inDataMode {
        result.Error = "modem in data mode"
        result.DialTime = time.Since(startTime)
        return result, fmt.Errorf("modem in data mode, call Hangup() first")
    }

    // 2. Format phone number (remove non-digits except + and ,)
    phone = formatPhoneNumber(phone)

    // 3. Send ATDT<phone>
    cmd := m.config.DialPrefix + phone
    if _, err := m.port.Write([]byte(cmd + "\r")); err != nil {
        result.Error = "write error"
        result.DialTime = time.Since(startTime)
        return result, err
    }

    // 4. Wait for response: CONNECT, BUSY, NO CARRIER, NO ANSWER, NO DIALTONE
    response, err := m.readUntilResult(dialTimeout)
    result.DialTime = time.Since(startTime)

    if err != nil {
        result.Error = "timeout"
        return result, err
    }

    // 5. Parse response
    if strings.HasPrefix(response, CONNECT) {
        // Parse speed/protocol from CONNECT message
        speed, protocol, _ := ParseConnectSpeed(response)
        result.ConnectSpeed = speed
        result.Protocol = protocol

        // Flush any remaining data from serial buffer
        m.flushBuffers()

        // Wait for DCD to stabilize
        connectTime := time.Now()
        if err := m.waitForDCD(carrierTimeout); err != nil {
            result.Error = "no carrier detect"
            return result, err
        }
        result.CarrierTime = time.Since(connectTime)

        // Mark modem as in data mode
        m.inDataMode = true
        result.Success = true
        return result, nil
    }

    // 6. Handle failure responses
    result.Error = response // BUSY, NO CARRIER, NO ANSWER, NO DIALTONE, ERROR
    return result, nil // Not a Go error, just unsuccessful dial
}
```

#### 2.4 net.Conn Wrapper (`internal/modemd/modem/conn.go`)

Wraps serial port as `net.Conn` for EMSI session compatibility:

```go
// ModemConn wraps a modem's serial port as net.Conn for EMSI compatibility.
//
// LIFECYCLE CONTRACT:
// - ModemConn is a thin wrapper that does NOT own the connection
// - Close() is a NO-OP - it does not hang up or close the serial port
// - Caller MUST call Modem.Hangup() separately after using ModemConn
// - This allows caller to handle errors and perform recovery if needed
type ModemConn struct {
    modem        *Modem
    readDeadline time.Time
    writeDeadline time.Time
}

// Read reads data from the modem serial port
func (c *ModemConn) Read(b []byte) (n int, err error)

// Write writes data to the modem serial port
func (c *ModemConn) Write(b []byte) (n int, err error)

// Close is a NO-OP. Caller must call Modem.Hangup() to terminate the call.
// This is intentional - it allows the caller to handle hangup errors and
// perform recovery (like calling Reset()) if needed.
func (c *ModemConn) Close() error {
    return nil // NO-OP by design
}

func (c *ModemConn) LocalAddr() net.Addr
func (c *ModemConn) RemoteAddr() net.Addr
func (c *ModemConn) SetDeadline(t time.Time) error
func (c *ModemConn) SetReadDeadline(t time.Time) error
func (c *ModemConn) SetWriteDeadline(t time.Time) error
```

### Phase 3: Test Executor

#### 3.1 Executor (`internal/modemd/executor/executor.go`)

```go
type Executor struct {
    modem       *modem.Modem    // Single modem
    config      *Config
    identity    Identity

    // Stats
    mu             sync.Mutex
    testsCompleted int
    testsFailed    int
    lastTestTime   time.Time
}

type Identity struct {
    Address    string
    SystemName string
    Sysop      string
    Location   string
}

func NewExecutor(
    modem *modem.Modem,
    config *Config,
    identity Identity,
) *Executor

// TestNodes tests a batch of nodes sequentially
func (e *Executor) TestNodes(ctx context.Context, nodes []api.Node) []api.TestResult {
    results := make([]api.TestResult, 0, len(nodes))

    for i, node := range nodes {
        // Check for cancellation between tests
        if ctx.Err() != nil {
            break
        }

        result := e.testSingleNode(ctx, node)
        results = append(results, result)

        e.mu.Lock()
        if result.Success {
            e.testsCompleted++
        } else {
            e.testsFailed++
        }
        e.lastTestTime = time.Now()
        e.mu.Unlock()

        // Wait inter-call delay before next test (unless last node)
        if i < len(nodes)-1 {
            select {
            case <-ctx.Done():
                break
            case <-time.After(e.config.Polling.InterCallDelay):
            }
        }
    }

    return results
}

// testSingleNode tests one node with proper error handling and recovery
func (e *Executor) testSingleNode(ctx context.Context, node api.Node) api.TestResult {
    result := api.TestResult{
        NodeID:   node.ID,
        TestedAt: time.Now(),
    }

    // 1. Dial phone number
    dialResult, err := e.modem.Dial(
        node.Phone,
        e.config.Timeouts.Dial,
        e.config.Timeouts.Carrier,
    )

    // Always record dial time
    result.DialTimeMs = int(dialResult.DialTime.Milliseconds())

    if err != nil {
        // Go error (I/O error, timeout, etc.)
        result.Error = err.Error()
        e.recoverModem() // Try to recover
        return result
    }

    if !dialResult.Success {
        // Modem error (BUSY, NO CARRIER, etc.)
        result.Error = dialResult.Error
        // No recovery needed - modem should be in command mode
        return result
    }

    // Record connection info
    result.ConnectSpeed = dialResult.ConnectSpeed
    result.ModemProtocol = dialResult.Protocol
    result.RingCount = dialResult.RingCount
    result.CarrierTimeMs = int(dialResult.CarrierTime.Milliseconds())

    // 2. Create EMSI session using modem as net.Conn
    conn := e.modem.GetConn()
    session := emsi.NewSessionWithInfo(
        conn,
        e.identity.Address,
        e.identity.SystemName,
        e.identity.Sysop,
        e.identity.Location,
    )
    session.SetTimeout(e.config.Timeouts.EMSI)

    // 3. Perform EMSI handshake
    if err := session.Handshake(); err != nil {
        result.Error = fmt.Sprintf("EMSI handshake failed: %v", err)
        e.hangupWithRecovery() // Hangup with recovery on failure
        return result
    }

    // 4. Extract results
    info := session.GetRemoteInfo()
    result.Success = true
    result.SystemName = info.SystemName
    result.MailerInfo = fmt.Sprintf("%s %s", info.MailerName, info.MailerVersion)
    result.Addresses = info.Addresses
    result.AddressValid = session.ValidateAddress(node.ID.String())

    // 5. Clean hangup
    e.hangupWithRecovery()

    return result
}

// hangupWithRecovery attempts hangup, falls back to reset if it fails
func (e *Executor) hangupWithRecovery() {
    if err := e.modem.Hangup(); err != nil {
        logging.Warn("hangup failed, attempting reset", "error", err)
        e.recoverModem()
    }
}

// recoverModem attempts to reset the modem to a known state
func (e *Executor) recoverModem() {
    if err := e.modem.Reset(); err != nil {
        logging.Error("modem reset failed", "error", err)
        // Continue anyway - modem might recover on next dial
    }
}

// Stats accessors
func (e *Executor) TestsCompleted() int {
    e.mu.Lock()
    defer e.mu.Unlock()
    return e.testsCompleted
}

func (e *Executor) TestsFailed() int {
    e.mu.Lock()
    defer e.mu.Unlock()
    return e.testsFailed
}

func (e *Executor) LastTestTime() time.Time {
    e.mu.Lock()
    defer e.mu.Unlock()
    return e.lastTestTime
}
```

### Phase 4: Main Daemon

#### 4.1 Daemon Loop (`cmd/modem-daemon/daemon.go`)

```go
type Daemon struct {
    config      *Config
    apiClient   *api.Client
    modem       *modem.Modem
    executor    *executor.Executor

    stop        chan struct{}
    stopOnce    sync.Once       // Ensures Stop() is idempotent
    wg          sync.WaitGroup

    // Track in-progress nodes for cleanup on shutdown
    mu              sync.Mutex
    inProgressNodes []api.NodeID   // Current cycle's nodes
    unreleasedNodes []api.NodeID   // Accumulated failed releases
}

func NewDaemon(config *Config) (*Daemon, error)

func (d *Daemon) Run(ctx context.Context) error {
    // Start heartbeat goroutine
    d.wg.Add(1)
    go d.heartbeatLoop(ctx)

    // Main test loop
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-d.stop:
            return nil
        default:
            // Try to release any accumulated unreleased nodes first
            d.retryUnreleasedNodes(ctx)

            if err := d.runTestCycle(ctx); err != nil {
                logging.Error("test cycle failed", "error", err)
            }

            // Wait before next cycle
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-d.stop:
                return nil
            case <-time.After(d.config.Polling.Interval):
            }
        }
    }
}

// retryUnreleasedNodes attempts to release nodes that failed in previous cycles
func (d *Daemon) retryUnreleasedNodes(ctx context.Context) {
    d.mu.Lock()
    if len(d.unreleasedNodes) == 0 {
        d.mu.Unlock()
        return
    }
    nodesToRetry := d.unreleasedNodes
    d.unreleasedNodes = nil // Clear optimistically
    d.mu.Unlock()

    logging.Info("retrying release of previously failed nodes", "count", len(nodesToRetry))

    releaseCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    if err := d.apiClient.ReleaseNodes(releaseCtx, nodesToRetry, "retry from previous failure"); err != nil {
        logging.Error("retry release failed, will try again next cycle", "error", err, "count", len(nodesToRetry))
        d.mu.Lock()
        d.unreleasedNodes = append(d.unreleasedNodes, nodesToRetry...)
        d.mu.Unlock()
    } else {
        logging.Info("successfully released previously failed nodes", "count", len(nodesToRetry))
    }
}

func (d *Daemon) runTestCycle(ctx context.Context) error {
    // 1. Get assigned nodes
    nodes, remaining, err := d.apiClient.GetNodes(ctx, d.config.Polling.BatchSize, true)
    if err != nil {
        return err
    }

    if len(nodes) == 0 {
        return nil // No work
    }

    logging.Info("got nodes to test", "count", len(nodes), "remaining", remaining)

    // 2. Mark as in_progress
    nodeIDs := api.ExtractNodeIDs(nodes)
    if _, err := d.apiClient.MarkInProgress(ctx, nodeIDs); err != nil {
        return err
    }

    // Track in-progress nodes for cleanup on shutdown
    d.mu.Lock()
    d.inProgressNodes = nodeIDs
    d.mu.Unlock()

    // Helper to clear current cycle tracking
    clearInProgress := func() {
        d.mu.Lock()
        d.inProgressNodes = nil
        d.mu.Unlock()
    }

    // Helper to move nodes to unreleased queue
    moveToUnreleased := func(nodes []api.NodeID) {
        d.mu.Lock()
        d.inProgressNodes = nil
        d.unreleasedNodes = append(d.unreleasedNodes, nodes...)
        d.mu.Unlock()
    }

    // 3. Check context before testing
    if ctx.Err() != nil {
        releaseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        if releaseErr := d.apiClient.ReleaseNodes(releaseCtx, nodeIDs, "context cancelled"); releaseErr != nil {
            logging.Error("failed to release nodes on cancellation", "error", releaseErr)
            moveToUnreleased(nodeIDs)
            return ctx.Err()
        }
        clearInProgress()
        return ctx.Err()
    }

    // 4. Test nodes
    results := d.executor.TestNodes(ctx, nodes)

    // 5. Submit results
    if err := d.apiClient.SubmitResults(ctx, results); err != nil {
        logging.Error("failed to submit results, releasing nodes", "error", err)

        releaseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        if releaseErr := d.apiClient.ReleaseNodes(releaseCtx, nodeIDs, fmt.Sprintf("submit failed: %v", err)); releaseErr != nil {
            logging.Error("failed to release nodes after submit failure", "error", releaseErr)
            moveToUnreleased(nodeIDs)
            return err
        }
        clearInProgress()
        return err
    }

    clearInProgress()
    return nil
}

func (d *Daemon) heartbeatLoop(ctx context.Context) {
    defer d.wg.Done()

    ticker := time.NewTicker(d.config.Heartbeat.Interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-d.stop:
            return
        case <-ticker.C:
            status := api.HeartbeatStatus{
                Status:         "active",
                ModemReady:     d.modem.IsReady(),
                TestsCompleted: d.executor.TestsCompleted(),
                TestsFailed:    d.executor.TestsFailed(),
                LastTestTime:   d.executor.LastTestTime(),
            }
            if err := d.apiClient.Heartbeat(ctx, status); err != nil {
                logging.Error("heartbeat failed", "error", err)
            }
        }
    }
}

// Stop gracefully shuts down the daemon (idempotent via sync.Once)
func (d *Daemon) Stop() {
    d.stopOnce.Do(func() {
        close(d.stop)
    })

    d.wg.Wait()

    // Collect all nodes that need releasing
    d.mu.Lock()
    var nodesToRelease []api.NodeID
    nodesToRelease = append(nodesToRelease, d.inProgressNodes...)
    nodesToRelease = append(nodesToRelease, d.unreleasedNodes...)
    d.mu.Unlock()

    if len(nodesToRelease) > 0 {
        logging.Info("releasing nodes on shutdown",
            "in_progress", len(d.inProgressNodes),
            "unreleased_backlog", len(d.unreleasedNodes),
            "total", len(nodesToRelease))

        // Retry release up to 3 times with exponential backoff
        var lastErr error
        for attempt := 1; attempt <= 3; attempt++ {
            releaseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            err := d.apiClient.ReleaseNodes(releaseCtx, nodesToRelease, "daemon shutdown")
            cancel()

            if err == nil {
                d.mu.Lock()
                d.inProgressNodes = nil
                d.unreleasedNodes = nil
                d.mu.Unlock()
                logging.Info("successfully released all nodes on shutdown")
                break
            }

            lastErr = err
            logging.Error("failed to release nodes on shutdown",
                "error", err, "attempt", attempt, "max_attempts", 3)

            if attempt < 3 {
                time.Sleep(time.Duration(attempt) * 2 * time.Second)
            }
        }

        if lastErr != nil {
            logging.Error("CRITICAL: could not release nodes after retries",
                "count", len(nodesToRelease), "error", lastErr)
        }
    }

    d.modem.Close()
}
```

#### 4.2 Main Entry Point (`cmd/modem-daemon/main.go`)

```go
func main() {
    var (
        configPath  = flag.String("config", "modem-daemon.yaml", "Path to config file")
        showVersion = flag.Bool("version", false, "Show version")
        testModem   = flag.Bool("test-modem", false, "Test modem interactively")
        testDial    = flag.String("test-dial", "", "Test dial phone number")
    )
    flag.Parse()

    if *showVersion {
        fmt.Printf("NodelistDB Modem Daemon %s\n", version.GetFullVersionInfo())
        os.Exit(0)
    }

    // Load config
    config, err := LoadConfig(*configPath)
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Validate config
    if err := config.Validate(); err != nil {
        log.Fatalf("Invalid config: %v", err)
    }

    // Initialize logging
    logging.Initialize(config.Logging)

    // Test modem mode
    if *testModem {
        testModemInteractive(config, *testDial)
        return
    }

    // Create and run daemon
    daemon, err := NewDaemon(config)
    if err != nil {
        log.Fatalf("Failed to create daemon: %v", err)
    }

    // Handle signals
    ctx, cancel := context.WithCancel(context.Background())
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigChan
        logging.Info("Shutting down...")
        cancel()
    }()

    // Run daemon
    if err := daemon.Run(ctx); err != nil && err != context.Canceled {
        log.Fatalf("Daemon error: %v", err)
    }

    daemon.Stop()
    logging.Info("Daemon stopped")
}

// testModemInteractive runs interactive modem testing
func testModemInteractive(config *Config, dialNumber string) {
    fmt.Println("=== Modem Test Mode ===")

    // Create modem
    m, err := modem.NewModem(config.Modem)
    if err != nil {
        log.Fatalf("Failed to create modem: %v", err)
    }

    // Open and initialize
    fmt.Printf("Opening %s at %d baud...\n", config.Modem.Device, config.Modem.BaudRate)
    if err := m.Open(); err != nil {
        log.Fatalf("Failed to open modem: %v", err)
    }
    defer m.Close()

    fmt.Println("Modem opened successfully")

    // Get modem info
    fmt.Println("\nSending ATI...")
    resp, err := m.SendAT("ATI", 5*time.Second)
    if err != nil {
        fmt.Printf("ATI failed: %v\n", err)
    } else {
        fmt.Printf("Response: %s\n", resp)
    }

    // Test dial if requested
    if dialNumber != "" {
        fmt.Printf("\nDialing %s...\n", dialNumber)
        result, err := m.Dial(dialNumber, config.Timeouts.Dial, config.Timeouts.Carrier)
        if err != nil {
            fmt.Printf("Dial error: %v\n", err)
        } else if result.Success {
            fmt.Printf("CONNECT %d %s\n", result.ConnectSpeed, result.Protocol)
            fmt.Printf("Dial time: %v, Carrier time: %v\n", result.DialTime, result.CarrierTime)

            // Wait a moment then hangup
            time.Sleep(2 * time.Second)
            fmt.Println("Hanging up...")
            if err := m.Hangup(); err != nil {
                fmt.Printf("Hangup error: %v\n", err)
            }
        } else {
            fmt.Printf("Dial failed: %s (took %v)\n", result.Error, result.DialTime)
        }
    }

    fmt.Println("\nTest complete")
}
```

## Testing Utilities

### Standalone Modem Test CLI (`cmd/modem-test`)

A standalone CLI tool for testing modem hardware without the full daemon:

```bash
# Build the test tool
go build -o bin/modem-test ./cmd/modem-test/

# Test modem AT commands interactively
./bin/modem-test -device /dev/ttyACM0 -interactive

# Dial a number (without EMSI)
./bin/modem-test -device /dev/ttyACM0 -dial 900

# Dial with EMSI handshake
./bin/modem-test -device /dev/ttyACM0 -dial 900 -emsi

# Dial with debug output
./bin/modem-test -device /dev/ttyACM0 -dial 900 -emsi -debug

# Full options
./bin/modem-test -help
```

**CLI Options:**
- `-device` - Serial port device (default: /dev/ttyACM0)
- `-baud` - Baud rate (default: 115200)
- `-dial` - Phone number to dial
- `-emsi` - Perform EMSI handshake after connect
- `-debug` - Enable debug output
- `-interactive` - Interactive AT command mode
- `-dial-timeout` - Dial timeout (default: 200s)
- `-carrier-timeout` - Carrier detect timeout (default: 5s)
- `-emsi-timeout` - EMSI handshake timeout (default: 30s)
- `-addr` - Our FidoNet address for EMSI (default: 2:5001/5001)
- `-system` - Our system name for EMSI
- `-init` - Modem init string (default: ATZ)
- `-hangup` - Hangup method: dtr or escape (default: dtr)

### Modem Test Mode (Future)

Built-in testing in the daemon binary:

```bash
# Test modem AT commands interactively
./modem-daemon -test-modem

# Test dial (without EMSI)
./modem-daemon -test-modem -test-dial "+7-495-123-4567"
```

### Example Config

```yaml
# modem-daemon.yaml
api:
  url: "https://nodelistdb.example.com/api/modem"
  key: "your-api-key-here"

identity:
  address: "2:5001/5001"
  system_name: "NodelistDB Modem Tester"
  sysop: "Test Operator"
  location: "Moscow, Russia"

# Single modem configuration
modem:
  device: "/dev/ttyACM0"
  init: "ATZ"                                # Or "AT&F S7=120" for custom init
  baud_rate: 115200
  protocols: ["V34", "V32B", "ZYX", "Z19"]  # For info only
  max_speed: 33600                           # For info only
  dial_prefix: "ATDT"                        # Optional, default: ATDT
  hangup_method: "dtr"                       # "dtr" (default) or "escape"

timeouts:
  dial: 200s          # Wait for CONNECT (modem S7 register controls actual timeout)
  carrier: 5s         # Wait for DCD after CONNECT
  emsi: 30s
  at_command: 5s

polling:
  interval: 30s
  batch_size: 10
  inter_call_delay: 3s  # Wait between calls

heartbeat:
  interval: 60s

logging:
  level: info
  console: true
```

### Tested Configuration

Successfully tested with:
- **Modem:** USB modem on /dev/ttyACM0 (56000 baud capable)
- **Target:** 2:5020/2021 (Airoport BBS, Moscow)
- **Phone:** 900 (internal test number)
- **Connection:** CONNECT 115200, LAP-M
- **EMSI:** binkleyforce 0.27/linux-gnu

## Implementation Checklist

### Phase 1: Core Infrastructure
- [ ] Configuration loading (`config.go`)
- [ ] Configuration validation with defaults
- [ ] API types (`internal/modemd/api/types.go`)
- [ ] API client (`internal/modemd/api/client.go`)
- [ ] API client tests

### Phase 2: Modem Abstraction ✅ COMPLETED
- [x] Single modem with state machine (`internal/modemd/modem/modem.go`)
  - [x] inDataMode flag tracking (Contract #3)
  - [x] Buffer flush on mode transitions
  - [x] Reset() for recovery (Contract #4)
- [x] AT command handling (`internal/modemd/modem/at_commands.go`)
- [x] Dial logic (`internal/modemd/modem/dial.go`)
  - [x] Always return non-nil DialResult (Contract #1)
  - [x] Use both dial and carrier timeouts (configurable, default 200s/5s)
  - [x] Buffer flush after CONNECT
  - [x] Raw byte reads for reliable response parsing
- [x] net.Conn wrapper (`internal/modemd/modem/conn.go`)
  - [x] Close() is NO-OP (Contract #6)
- [x] Modem test CLI (`cmd/modem-test/main.go`)
- [ ] Modem tests with mock serial port

### Phase 3: Test Executor
- [ ] Executor (`internal/modemd/executor/executor.go`)
  - [ ] Sequential node testing
  - [ ] Inter-call delay
  - [ ] hangupWithRecovery() (Contract #4)
  - [ ] Stats tracking
- [x] Integration with existing EMSI session (verified working)
- [ ] Executor tests

### Phase 4: Main Daemon
- [ ] Daemon loop (`daemon.go`)
  - [ ] Two-tier node tracking (Contract #2)
  - [ ] retryUnreleasedNodes() at cycle start
  - [ ] moveToUnreleased() on release failure
  - [ ] ReleaseNodes on submit failure
  - [ ] ReleaseNodes on context cancellation
- [ ] Idempotent Stop with sync.Once (Contract #5)
  - [ ] Releases both tiers
  - [ ] Retry with exponential backoff
- [ ] Main entry point (`main.go`)
- [ ] Signal handling
- [ ] testModemInteractive()
- [ ] README documentation

### Phase 5: Testing & Documentation
- [x] Integration tests with real modem (tested with 2:5020/2021)
- [ ] Mock modem for CI testing
- [ ] Contract verification tests
- [ ] MockAPIClient test helper
- [ ] User documentation
- [ ] Deployment guide

## Contract Verification Test Plan

### Test Suite: Two-Tier Node Tracking (Contract #2)

#### Test 1: Basic Submit Success
```go
func TestNodeTracking_SubmitSuccess(t *testing.T) {
    // Setup: Mock API that succeeds on all calls
    // Action: Run one test cycle
    // Verify:
    //   - inProgressNodes is empty after cycle
    //   - unreleasedNodes is empty
    //   - SubmitResults was called with correct results
}
```

#### Test 2: Submit Failure → Release Success
```go
func TestNodeTracking_SubmitFailure_ReleaseSuccess(t *testing.T) {
    // Setup: Mock API where SubmitResults fails, ReleaseNodes succeeds
    // Action: Run one test cycle
    // Verify:
    //   - inProgressNodes is empty after cycle
    //   - unreleasedNodes is empty (release succeeded)
    //   - ReleaseNodes was called with correct node IDs
}
```

#### Test 3: Submit Failure → Release Failure → Accumulate
```go
func TestNodeTracking_SubmitFailure_ReleaseFailure_Accumulates(t *testing.T) {
    // Setup: Mock API where both SubmitResults and ReleaseNodes fail
    // Action: Run one test cycle
    // Verify:
    //   - inProgressNodes is empty (cleared regardless)
    //   - unreleasedNodes contains the failed nodes
    //   - Nodes are NOT lost
}
```

#### Test 4: Unreleased Nodes Persist Across Cycles
```go
func TestNodeTracking_UnreleasedPersistsAcrossCycles(t *testing.T) {
    // Setup: Mock API where ReleaseNodes fails on cycle 1, succeeds on cycle 2
    // Action: Run two cycles
    // Verify:
    //   - Cycle 1's nodes were NOT overwritten by cycle 2
    //   - retryUnreleasedNodes was called at start of cycle 2
    //   - All nodes eventually released
}
```

#### Test 5: Shutdown Releases Both Tiers
```go
func TestNodeTracking_ShutdownReleasesBothTiers(t *testing.T) {
    // Setup: Both inProgressNodes and unreleasedNodes have nodes
    // Action: Call Stop()
    // Verify:
    //   - ReleaseNodes called with union of both lists
    //   - Both lists cleared on success
}
```

#### Test 6: Shutdown Retry on Failure
```go
func TestNodeTracking_ShutdownRetriesOnFailure(t *testing.T) {
    // Setup: Mock API where ReleaseNodes fails twice, succeeds on third
    // Action: Call Stop() with nodes in tracking
    // Verify:
    //   - ReleaseNodes called exactly 3 times
    //   - Exponential backoff timing
    //   - Nodes cleared after success
}
```

### Test Helpers

```go
// MockAPIClient with configurable failure injection
type MockAPIClient struct {
    GetNodesFunc       func() ([]Node, int, error)
    MarkInProgressFunc func([]NodeID) (int, error)
    SubmitResultsFunc  func([]TestResult) error
    ReleaseNodesFunc   func([]NodeID, string) error

    // Track calls for verification
    ReleaseNodesCalls []struct {
        NodeIDs []NodeID
        Reason  string
    }
}

// FailNTimes returns a function that fails n times then succeeds
func FailNTimes(n int, err error) func() error {
    count := 0
    return func() error {
        count++
        if count <= n {
            return err
        }
        return nil
    }
}
```

## Serial Port Settings

For modem communication via go-serial:

```go
port, err := serial.Open(device,
    serial.WithBaudrate(115200),
    serial.WithDataBits(8),
    serial.WithParity(serial.NoParity),
    serial.WithStopBits(serial.OneStopBit),
    serial.WithReadTimeout(1000),
    serial.WithHUPCL(true),         // Hangup on close (drops DTR)
)
```

**Note:** The serial port baud rate (115200) is the speed between computer and modem. The modem-to-modem speed is negotiated during the call and reported in the CONNECT message.

## Modem Control Lines Usage

- **DTR (Data Terminal Ready)**: Drop to hangup call
- **DCD (Data Carrier Detect)**: Monitor for carrier loss
- **DSR (Data Set Ready)**: Modem is powered and ready
- **RI (Ring Indicator)**: Incoming call detection (not used)
- **RTS/CTS**: Hardware flow control (auto-managed)

```go
// Hangup by dropping DTR
port.SetDTR(false)
time.Sleep(500 * time.Millisecond)
port.SetDTR(true)

// Check carrier detect
status, _ := port.GetModemStatusBits()
if !status.DCD {
    // Carrier lost
}
```
