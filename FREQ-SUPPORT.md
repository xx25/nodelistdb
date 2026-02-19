# Plan: Minimal File Request (FREQ) Testing for modem-test

## Overview

Add optional WaZOO file request capability to the modem-test tool. After a successful EMSI handshake over modem, the tool can send a .REQ file via ZModem and receive the requested files. This is opt-in via `-freq` CLI flag.

**ZModem**: Will be provided as an external library later. This plan defines the interface it must satisfy and builds everything around it.

**Future scope** (not this task): Reuse WaZOO package in the test daemon for ifcico/binkp FREQ over TCP/IP.

---

## Step 1: ZModem Interface Definition

**Create:** `internal/testing/protocols/zmodem/interface.go`

Define the interface that the ZModem library must implement. This decouples the WaZOO session from any specific ZModem implementation.

```go
package zmodem

import (
    "bufio"
    "net"
)

// ReceivedFile represents a file received via ZModem
type ReceivedFile struct {
    Name string // Original filename from sender
    Size int64  // File size in bytes
    Path string // Local path where file was saved
}

// Sender sends files via ZModem protocol
type Sender interface {
    // SendFile sends a single file. name is the filename to announce,
    // data is the file content. Blocks until transfer completes or fails.
    SendFile(name string, data []byte) error
    // Finish sends ZFIN and completes the sender session.
    Finish() error
}

// Receiver receives files via ZModem protocol
type Receiver interface {
    // ReceiveFiles receives all files until the remote sends ZFIN.
    // Files are saved to the configured output directory.
    ReceiveFiles() ([]ReceivedFile, error)
}

// DebugFunc is a callback for routing debug output.
type DebugFunc func(format string, args ...any)

// Config holds ZModem session configuration
type Config struct {
    Conn    net.Conn
    Reader  *bufio.Reader // May have buffered data from EMSI
    Debug   bool
    DbgFunc DebugFunc
}

// SenderConfig extends Config for sending
type SenderConfig struct {
    Config
}

// ReceiverConfig extends Config for receiving
type ReceiverConfig struct {
    Config
    OutputDir string // Directory to save received files
}

// NewSender creates a ZModem sender (placeholder - will be implemented by library)
// var NewSender func(cfg SenderConfig) Sender

// NewReceiver creates a ZModem receiver (placeholder - will be implemented by library)
// var NewReceiver func(cfg ReceiverConfig) Receiver
```

---

## Step 2: WaZOO Session Layer

**Create:** `internal/testing/protocols/wazoo/session.go`

Orchestrates the post-EMSI WaZOO file exchange per FTS-0006.

```go
type Config struct {
    RequestFile   string        // Filename to request (e.g., "FLAGS")
    Password      string        // Optional FREQ password
    OutputDir     string        // Directory to save received files
    Timeout       time.Duration // Overall session timeout
    Debug         bool
    DebugFunc     func(string, ...any)

    // ZModem factory functions (injected - allows swapping implementations)
    NewSender   func(zmodem.SenderConfig) zmodem.Sender
    NewReceiver func(zmodem.ReceiverConfig) zmodem.Receiver
}

type Result struct {
    RequestSent   bool
    FilesReceived []zmodem.ReceivedFile
    Error         error
}

type Session struct {
    conn   net.Conn
    reader *bufio.Reader
    config Config
}

func NewSession(conn net.Conn, reader *bufio.Reader, cfg Config) *Session
func (s *Session) Run() *Result
```

### WaZOO Session Flow (FTS-0006, we are the caller)

1. **Build .REQ content**: `"FLAGS\r\n"` (or specified file, with optional `!password`)
2. **Build .REQ filename**: `NNNNNNNN.REQ` from remote's net/node in hex (per FTS-0006)
   - Encode as 4-hex-digit net + 4-hex-digit node, e.g., `2:5001/100` -> `13890064.REQ`
   - Accept remote address as parameter from EMSI info
3. **Send batch** (caller's turn):
   - Create ZModem sender via `NewSender()`
   - Send .REQ file: `sender.SendFile("13890064.REQ", reqContent)`
   - Finish sender: `sender.Finish()`
4. **Receive batch** (called system's turn):
   - Create ZModem receiver via `NewReceiver()`
   - Receive response files: `receiver.ReceiveFiles()`
   - Files are saved to `OutputDir`
5. Return `Result` with list of received files

### .REQ File Format (FTS-0006)
```
filename[SPACE!password][SPACE+/-time]CR
```
Examples:
- `FLAGS\r\n`
- `NODELIST !secret\r\n`
- `NODELIST.* +599634000\r\n`

### Helper: Address to REQ Filename
```go
// AddressToREQName converts a FidoNet address to a .REQ filename per FTS-0006
// Example: "2:5001/100" -> "13890064.REQ" (net=0x1389, node=0x0064)
func AddressToREQName(address string) (string, error)
```

---

## Step 3: EMSI Session Extension

**Modify:** `internal/testing/protocols/emsi/session.go`

Add methods to expose the buffered reader and connection for continued use after handshake. The `bufio.Reader` may have consumed bytes from the stream during EMSI that the next protocol layer (ZModem) needs.

```go
// GetReader returns the buffered reader for post-handshake protocol use.
// The reader may contain bytes consumed during EMSI detection but not
// processed. Essential for ZModem or other protocols that follow EMSI.
func (s *Session) GetReader() *bufio.Reader {
    return s.reader
}

// GetConn returns the underlying net.Conn for direct I/O after handshake.
func (s *Session) GetConn() net.Conn {
    return s.conn
}
```

---

## Step 4: modem-test CLI Integration

### 4a. Config Changes

**Modify:** `cmd/modem-test/config.go`

Add FREQ config section:

```go
type FREQConfig struct {
    File      string `yaml:"file"`       // File to request (e.g., "FLAGS")
    Password  string `yaml:"password"`   // FREQ password (optional)
    OutputDir string `yaml:"output_dir"` // Directory for received files (default: "./received")
}
```

Add to `Config` struct:
```go
FREQ FREQConfig `yaml:"freq"`
```

### 4b. CLI Flags

**Modify:** `cmd/modem-test/main.go`

Add flags:
- `-freq string` - File to request (enables FREQ mode; default: empty = no FREQ)
- `-freq-dir string` - Output directory for received files (default: `./received`)
- `-freq-password string` - Optional FREQ password

CLI flags override YAML config values when both are specified.

### 4c. EMSI Protocol Auto-Config

**Modify:** `cmd/modem-test/main.go` (config validation section)

When FREQ is active, EMSI must negotiate a real file transfer protocol (not NCP):

```go
// If FREQ is requested, ensure protocols include ZMO
if cfg.FREQ.File != "" && len(cfg.EMSI.Protocols) == 0 {
    cfg.EMSI.Protocols = []string{"ZMO", "ZAP"}
}
```

### 4d. Post-EMSI FREQ Logic

**Modify:** `cmd/modem-test/worker.go` (after EMSI handshake ~line 657-695)

After successful EMSI handshake, before hangup:

```go
// After EMSI success, optionally run FREQ
if freqFile != "" {
    if info != nil && info.HasNRQ {
        w.log.FREQ("Remote advertises NRQ (no file requests) - skipping")
    } else {
        w.log.FREQ("Requesting file '%s'...", freqFile)

        wazooResult := wazoo.NewSession(
            session.GetConn(),
            session.GetReader(),
            wazoo.Config{
                RequestFile: freqFile,
                Password:    freqPassword,
                OutputDir:   freqDir,
                Timeout:     120 * time.Second,
                Debug:       w.logConfig.Debug,
                DebugFunc:   func(f string, a ...any) { w.log.FREQ(f, a...) },
                NewSender:   zmodemImpl.NewSender,   // Injected ZModem impl
                NewReceiver: zmodemImpl.NewReceiver,  // Injected ZModem impl
            },
        ).Run()

        if wazooResult.Error != nil {
            w.log.Fail("FREQ failed: %v", wazooResult.Error)
            testRes.freqError = wazooResult.Error
        } else {
            for _, f := range wazooResult.FilesReceived {
                w.log.OK("FREQ: Received %s (%d bytes) -> %s", f.Name, f.Size, f.Path)
            }
            testRes.freqFiles = wazooResult.FilesReceived
        }
    }
}
```

Same logic also applies in `main.go` single-modem mode (~line 994-1060).

### 4e. Logger Extension

**Modify:** `cmd/modem-test/logger.go`

Add FREQ log method (same pattern as existing `EMSI()` method):
```go
func (l *Logger) FREQ(format string, args ...interface{}) { ... }
```

### 4f. Test Result Extension

**Modify:** `cmd/modem-test/main.go` (or wherever `testResult` is defined)

Add FREQ fields to `testResult` struct:
```go
freqFiles []zmodem.ReceivedFile
freqError error
```

---

## Implementation Order

1. ZModem interface definition (`zmodem/interface.go`)
2. WaZOO session + .REQ builder + address-to-filename helper (`wazoo/session.go`)
3. EMSI session extension (`GetReader()`, `GetConn()`)
4. modem-test config + CLI flags
5. modem-test worker integration
6. Logger extension
7. Build and verify compilation
8. **Wait for ZModem library**, wire it in, then end-to-end test

---

## Files Summary

### New Files
| File | Description |
|------|-------------|
| `internal/testing/protocols/zmodem/interface.go` | ZModem sender/receiver interfaces + config types |
| `internal/testing/protocols/wazoo/session.go` | WaZOO session: .REQ building, send/receive orchestration |
| `internal/testing/protocols/wazoo/wazoo_test.go` | Unit tests: .REQ format, address-to-filename conversion |

### Modified Files
| File | Changes |
|------|---------|
| `internal/testing/protocols/emsi/session.go` | Add `GetReader()`, `GetConn()` methods |
| `cmd/modem-test/config.go` | Add `FREQConfig` struct, add to Config |
| `cmd/modem-test/main.go` | Add `-freq` flags, EMSI protocol auto-config, FREQ logic for single-modem |
| `cmd/modem-test/worker.go` | Add post-EMSI FREQ execution logic for multi-modem |
| `cmd/modem-test/logger.go` | Add `FREQ()` log method |

---

## Testing Strategy

1. **Unit tests** for .REQ file content generation (various combinations of file, password, update time)
2. **Unit tests** for `AddressToREQName()` address-to-hex conversion
3. **Build test**: `make build` succeeds with all new code (FREQ is opt-in, doesn't break existing flow)
4. **Dry run** (no ZModem impl yet): Run modem-test without `-freq` flag, verify existing behavior unchanged
5. **End-to-end** (after ZModem library): `./bin/modem-test -phone 917 -freq FLAGS -freq-dir /tmp/freq-test -batch -count 1`

---

## Key Design Decisions

- **ZModem as interface**: Decouples WaZOO session from ZModem implementation. Factory functions injected via config.
- **FREQ is opt-in**: Empty `-freq` flag = existing behavior (EMSI-only test). No FREQ code runs unless explicitly requested.
- **EMSI reader handoff**: `GetReader()` passes the `bufio.Reader` to ZModem so buffered bytes aren't lost between protocol layers.
- **FTS-0006 .REQ naming**: Proper hex encoding of remote address for the .REQ filename. Some systems are strict about this.
- **NRQ check**: Respects remote's "no file requests" flag from EMSI_DAT before attempting FREQ.

---

## Reference Specs

- **FTS-0006.002** - YOOHOO/2U2: WaZOO methods, .REQ file format, session flow
- **FSC-0056.001** - EMSI/IEMSI: Handshake protocol, link codes (NRQ, HRQ, FRQ)
- **FSC-0088.001** - EMSI-II: Compatibility extensions, file type control
- **FTS-0008** - Bark file requests (deprecated, but defines .REQ format origin)
- **FSC-0086** - SRIF: Standard Request Information File (server-side processor interface)

## Reference Implementations Studied

- **ifmail** (`/home/dp/FIDO/src/Mailers/ifmail-3.03/ifcico/`) - EMSI + ZModem session flow
- **bforce** (`/home/dp/FIDO/src/Mailers/bforce/source/`) - BinkP + EMSI + ZModem, session orchestration
- **FTNd** (`/home/dp/FIDO/src/Mailers/FTNd/ftncico/`) - ZModem + BinkP + EMSI
- **mfreq** (`/home/dp/FIDO/src/Freqs/mfreq/`) - SRIF-based file request processor (server-side, .REQ parsing)
