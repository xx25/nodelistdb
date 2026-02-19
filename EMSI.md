# EMSI Handshake Architecture Rewrite

## Problem

The current EMSI handshake in `session.go` has accumulated ad-hoc fixes but the architecture doesn't match how real FidoNet mailers work. Key issues:

1. **Monolithic `readEMSIResponseWithTimeout`** reads 4KB chunks, scans buffer for EMSI substrings. Doesn't know what packet type it's looking for.
2. **No proper state machine** — ad-hoc if/else/switch in `Handshake()` with 3 separate strategy methods.
3. **Truncated EMSI_DAT** — detects "EMSI_DAT" substring before full packet arrives (current `isEMSI_DATComplete` is a bolt-on fix).
4. **String-based carrier detection** — scans entire buffer for "NO CARRIER" substring.
5. **Single-tier timeouts** — everything derived from one `SetTimeout()` value.

## Reference: How Real Mailers Do It

All 4 studied mailers (bforce, ifcico, qico, FTNd) share:
- **Character-at-a-time I/O** via `GETCHAR(timeout)` with per-byte timeout
- **Length-driven EMSI_DAT reading** — parse 4 hex length digits, read exactly that many bytes
- **Two-tier timeouts** — master (60s total) + step (20s per attempt) + character (1-8s per byte)
- **Proper FSM** with named states and explicit transitions
- **I/O-layer carrier detection** (DCD/hangup signals, not string scanning)

## Approach

Rewrite `session.go` with proper character-at-a-time I/O and a finite state machine. Keep all other files unchanged. Keep all public API methods identical.

---

## Files Changed

| File | Action |
|------|--------|
| `internal/testing/protocols/emsi/session.go` | **Rewrite** |
| `internal/testing/protocols/emsi/session_test.go` | **New** — FSM and charReader tests |
| `internal/testing/protocols/emsi/config.go` | Unchanged |
| `internal/testing/protocols/emsi/protocol.go` | Unchanged |
| `internal/testing/protocols/emsi/banner_parser.go` | Unchanged |
| `internal/testing/protocols/emsi/config_test.go` | Unchanged |
| `internal/testing/protocols/emsi/protocol_test.go` | Unchanged |

---

## Code Reuse — No Duplication

Both consumers (testdaemon's `ifcico_tester.go` and modem-test's `worker.go`) use Session through its public API only — no protocol logic is duplicated outside the emsi package. The rewrite stays entirely within `internal/testing/protocols/emsi/`.

Existing functions to **reuse** (not reimplement):
- `CalculateCRC16()` from `protocol.go` — used by `readEMSI_DAT()` for CRC validation
- `ParseEMSI_DAT()` from `protocol.go` — used by RX phase to parse received DAT packet
- `CreateEMSI_DATWithConfig()` from `protocol.go` — used by `sendEMSI_DAT()` (unchanged)
- `extractSoftwareFromBanner()` from `banner_parser.go` — uses banner text collected by charReader
- `ConfigManager` from `config.go` — per-node config resolution (unchanged)
- `sendEMSI_INQ()`, `sendEMSI_ACK()`, `sendEMSI_DAT()` — existing wire-write methods (unchanged)

## Step 1: charReader — Single-byte I/O Layer

Replace 4KB chunk reads with character-at-a-time reading. Add at bottom of `session.go` or as new `charreader.go`.

```go
type charReader struct {
    conn       net.Conn
    reader     *bufio.Reader
    banner     strings.Builder   // accumulated non-EMSI text
    debug      bool
    dbgFunc    func(string, ...any)
    // Line-based carrier detection
    lineBuf    [32]byte
    linePos    int
    carrierLost bool
}
```

### Key methods:

**`getchar(timeout time.Duration) (byte, error)`**
- Set `conn.SetReadDeadline(now + timeout)`
- Call `reader.ReadByte()` — returns instantly if bufio has buffered data, blocks on conn if not
- Feed byte to `feedCarrierDetect()` for line-boundary "NO CARRIER" matching
- Accumulate in banner builder
- Return byte or sentinel errors: `errCharTimeout`, `errCarrierLost`

**`readToken(stepTimeout, charTimeout time.Duration) (emsiToken, string)`**
- Read bytes one-at-a-time via `getchar`
- Maintain 14-byte sliding window to detect EMSI sequences
- Fixed-length tokens (INQ, REQ, ACK, NAK, CLI, HBT): matched against full 14-char constants including CRC
- DAT: matched on first 10 chars (`**EMSI_DAT`), returns `tokenDAT` immediately
- Respects both step timeout (overall) and character timeout (per-byte)
- Returns `tokenTimeout`, `tokenCarrier`, `tokenError` on failures

**`readEMSI_DAT(charTimeout time.Duration) (string, error)`**
- Called AFTER `readToken` returned `tokenDAT` (header already consumed)
- Read 4 hex chars → parse payload length (cap at 8192)
- Read exactly `payloadLen` bytes, applying high-bit strip (`b & 0x7F`) per FSC-0056 §7-bit
- Read 4 hex chars → CRC
- Validate CRC-16 over `EMSI_DAT` + lenHex + data (without `**` prefix), all bytes high-bit stripped
- Handle FrontDoor length bug (`AcceptFDLenWithCR`): if CRC fails, retry with len-1
- Return reconstructed full packet string for `ParseEMSI_DAT()`

**`feedCarrierDetect(b byte)`**
- Line-boundary matching: accumulate bytes until `\r`/`\n`, then check complete line
- Exact match: `"NO CARRIER"`, `"BUSY"`, `"NO DIALTONE"`, `"NO ANSWER"`
- Sets `carrierLost = true`, checked by next `getchar()` call
- No false positives from banner/EMSI data containing these words mid-line

## Step 2: Token Type

```go
type emsiToken int
const (
    tokenNone emsiToken = iota
    tokenINQ    // **EMSI_INQC816
    tokenREQ    // **EMSI_REQA77E
    tokenACK    // **EMSI_ACKA490
    tokenNAK    // **EMSI_NAKEEC3
    tokenCLI    // **EMSI_CLIFA8C
    tokenHBT    // **EMSI_HBTEAEE
    tokenDAT    // **EMSI_DATxxxx... (header detected, use readEMSI_DAT for rest)
    tokenTimeout
    tokenCarrier
    tokenError
)
```

## Step 3: FSM-based Handshake

Replace `Handshake()` body with three phases. Keep all public API identical.

### Phase 1: Initial Contact (`runInitialPhase`)

Replaces `handshakeInitialWait`, `handshakeInitialSendCR`, `handshakeInitialSendINQ`, `sendPreventiveINQ`.

```
strategy="wait":     wait for remote to send first
strategy="send_cr":  send CRs at InitialCRInterval, then wait
strategy="send_inq": send EMSI_INQ, then wait

WAIT: readToken(FirstStepTimeout / StepTimeout, CharacterTimeout) →
  REQ     → return (remote wants our DAT first)
  INQ     → send EMSI_REQ, return (remote will send DAT next)
  DAT     → readEMSI_DAT, return (remote sent DAT directly)
  HBT     → restart step timer, continue WAIT
  timeout → if PreventiveINQ: send EMSI_INQ, retry once; else error
  carrier → error
```

Returns: token type + optional received DAT packet.

### Phase 2: TX Phase — Send Our DAT (`runTXPhase`)

```
TX_INIT: retries=0
TX_SEND_DAT: send our EMSI_DAT packet
TX_WAIT: readToken(StepTimeout, CharacterTimeout) →
  ACK     → SUCCESS
  NAK     → retries++, if < MaxRetries → TX_SEND_DAT, else ERROR
  REQ     → restart timer, stay in TX_WAIT
  HBT     → restart timer, stay in TX_WAIT
  DAT     → save for RX phase, SUCCESS (remote sent DAT before ACKing)
  timeout → retries++, if < MaxRetries → TX_SEND_DAT, else ERROR
  carrier → ERROR
  master expired → ERROR
```

### Phase 3: RX Phase — Receive Remote's DAT (`runRXPhase`)

```
RX_INIT: retries=0; if DAT already received → RX_VALIDATE
RX_FIRST: if SkipFirstRXReq (spec-compliant caller mode): go to RX_WAIT directly
RX_SEND_REQ: send EMSI_REQ (or NAK if retrying with SendNAKOnRetry)
RX_WAIT: readToken(StepTimeout, CharacterTimeout) →
  DAT     → readEMSI_DAT → RX_VALIDATE
  HBT     → restart timer, stay in RX_WAIT
  INQ     → RX_SEND_REQ (remote restarting)
  timeout → retries++, if < MaxRetries → RX_SEND_REQ, else ERROR
  carrier → ERROR
  master expired → ERROR

RX_VALIDATE:
  ParseEMSI_DAT → success → send EMSI_ACK (twice if SendACKTwice) → SUCCESS
  parse error → retries++, RX_SEND_REQ
```

### Complete Flow (Handshake body)

```
create charReader
masterDeadline = now + MasterTimeout

token, datPacket = runInitialPhase(cr)

switch token:
  REQ → runTXPhase(cr) → runRXPhase(cr)
  INQ → runRXPhase(cr) → runTXPhase(cr)
  DAT → parse datPacket, send ACK → runTXPhase(cr)
  error → try banner extraction fallback, return error

negotiateEMSI2()
selectProtocol()
set completionReason + timing
return nil
```

## Step 4: Timeout Architecture

Three tiers matching reference mailers:

| Timer | Default | Config Field | Used By |
|-------|---------|-------------|---------|
| Master | 60s | `MasterTimeout` | `Handshake()` — absolute deadline |
| Step | 20s | `StepTimeout` | `readToken()` — per-attempt |
| Character | 5s | `CharacterTimeout` | `getchar()` — per-byte |

- `readToken` caps step timeout at `min(StepTimeout, time.Until(masterDeadline))`
- Legacy `SetTimeout()` continues to work: sets MasterTimeout and scales others proportionally (existing logic unchanged)
- `FirstStepTimeout` (10s) used only for initial phase first attempt

New config field:
- `SkipFirstRXReq bool` (`skip_first_rx_req`, default: `false`) — when `true`, RX phase skips sending EMSI_REQ on first attempt (strict FSC-0056 caller behavior: wait silently for remote's DAT). When `false` (default), always sends REQ on first RX try, which is what all reference mailers do.

## Step 5: Remove Dead Code

Delete from session.go:
- `readEMSIResponseWithTimeout` — replaced by charReader
- `detectEMSIType` — replaced by readToken window matching
- `isEMSI_DATComplete` — unnecessary with length-driven reads
- `detectModemDisconnect` — replaced by charReader.feedCarrierDetect
- `handshakeInitialWait` — folded into runInitialPhase
- `handshakeInitialSendCR` — folded into runInitialPhase
- `handshakeInitialSendINQ` — folded into runInitialPhase
- `sendPreventiveINQ` — folded into runInitialPhase

Keep unchanged:
- `sendEMSI_INQ()`, `sendEMSI_ACK()`, `sendEMSI_DAT()` — simple wire writes
- `negotiateEMSI2()`, `selectProtocol()` — post-handshake logic
- `normalizeAddress()`, `formatResponsePreview()` — utilities
- `extractSoftwareFromBanner()` — fallback logic (uses banner text from charReader)
- All public constructors: `NewSession*`
- All public getters: `Get*`, `Is*`, `Validate*`

## Step 6: Tests (session_test.go)

Use `net.Pipe()` for all tests:

1. **charReader unit tests**:
   - `TestGetchar_BasicRead` — single byte read
   - `TestGetchar_Timeout` — per-character timeout fires
   - `TestGetchar_CarrierDetect` — "NO CARRIER" line detection
   - `TestGetchar_CarrierNoFalsePositive` — "NO CARRIER" inside EMSI data

2. **readToken unit tests**:
   - `TestReadToken_INQ/REQ/ACK/NAK/DAT` — each token type
   - `TestReadToken_BannerThenEMSI` — banner text before EMSI sequence
   - `TestReadToken_StepTimeout` — timeout with no EMSI data

3. **readEMSI_DAT unit tests**:
   - `TestReadEMSI_DAT_ValidPacket` — full packet with correct CRC
   - `TestReadEMSI_DAT_CRCMismatch` — corrupted CRC
   - `TestReadEMSI_DAT_Timeout` — partial packet then silence
   - `TestReadEMSI_DAT_InvalidLength` — bad hex length

4. **Full handshake integration tests** (simulate remote mailer on net.Pipe):
   - `TestHandshake_REQFlow` — REQ → our DAT → their DAT → ACK
   - `TestHandshake_DATFlow` — remote sends DAT directly
   - `TestHandshake_INQFlow` — INQ → REQ → DAT exchange
   - `TestHandshake_BannerThenREQ` — long banner before EMSI
   - `TestHandshake_MasterTimeout` — no response
   - `TestHandshake_CarrierLoss` — NO CARRIER mid-handshake
   - `TestHandshake_RetryOnNAK` — NAK then success on retry
   - `TestHandshake_RetryOnTimeout` — step timeout then success on retry

---

## Implementation Sequence

1. Add `charReader` type + `getchar()` + `feedCarrierDetect()` + `readToken()` + `readEMSI_DAT()`
2. Write charReader unit tests, verify they pass
3. Add `runInitialPhase()`, `runTXPhase()`, `runRXPhase()`
4. Rewrite `Handshake()` body to use the three phases
5. Write FSM integration tests
6. Delete dead code (old readEMSI*, detect*, handshakeInitial* functions)
7. Run `make test` — all existing config/protocol tests must pass
8. Run `make build` — all binaries must compile
9. Test with real nodes: `testdaemon -test-node "2:5001/100" -test-proto ifcico` and modem-test

## Codex Review Findings (addressed in plan)

### 1. Missing token handling per phase
- **Initial phase**: ACK/NAK/CLI → treat as error (same as current behavior)
- **TX phase**: Add INQ → restart timer (remote restarting); CLI → ERROR
- **RX phase**: Add REQ → resend our DAT (remote confused, wants our data again); NAK → resend our DAT; ACK → ignore (stale ACK from previous exchange)

### 2. Token matching strictness
- `readToken` matches EMSI sequences **permissively**: detect on the `**EMSI_XXX` prefix (10 chars for DAT, 9 chars `**EMSI_` + 3-char type for others), then validate CRC as optional. If CRC is wrong, still return the token type but log a warning. This matches current behavior (substring detection) while being more structured.
- Also accept `EMSI_` without `**` prefix as a fallback for non-compliant mailers.

### 3. readToken must not consume EMSI_DAT length bytes
- `readToken` detects DAT by matching `**EMSI_DAT` (10 chars). The sliding window stops exactly at the `T` of `DAT`. The 4 hex length chars are NOT consumed — `readEMSI_DAT()` reads them next.
- Implementation: when window ends with `**EMSI_DAT`, return `tokenDAT` immediately without reading further.

### 4. FrontDoor length bug handling
- `readEMSI_DAT` always reads the full advertised `payloadLen` bytes from the stream (no desync).
- If CRC fails AND the last byte is `\r`, retry CRC with `data[:len-1]` (don't re-read, just re-validate).

### 5. XON/XOFF and NUL stripping
- `readToken` skips XON (0x11), XOFF (0x13), and NUL (0x00) bytes — they don't enter the sliding window or the banner. Matches bforce/ifcico behavior.

### 6. EOF/conn close handling
- `getchar()` maps `io.EOF`, `io.ErrUnexpectedEOF`, and `net.OpError` to `errCarrierLost` (same as carrier loss). This matches reference mailer behavior where read failure = hangup.

### 7. DAT-first parse failure
- If remote sent DAT directly in initial phase and parsing fails: send EMSI_NAK, fall through to RX phase to request retransmit (same retry path as RX_VALIDATE failure).

### 8. HBT is deliberately treated as keepalive
- Current code treats HBT as error. The new code treats it as keepalive (restart step timer). This is a deliberate behavioral improvement matching FSC-0056.001 and all reference mailers. HBT means "I'm still here, don't timeout."

### 9. Banner text preservation
- `charReader.getBannerText()` is copied to `s.bannerText` at the end of `Handshake()` (both success and failure paths) so `extractSoftwareFromBanner()` continues to work.

## FSC-0056/FSC-0088 Spec Compliance Notes

### Verified against spec:
- **Timeouts**: T1=20s (step), T2=60s (master) per Step 2A/2B. Plan's `StepTimeout=20s`, `MasterTimeout=60s` match.
- **MaxRetries=6**: Spec says "Tries>6? Terminate". Already the default in `config.go`.
- **ACK twice**: Spec says "Transmit EMSI_ACK twice" after successful DAT receipt. Plan's `SendACKTwice` config option matches.
- **CRC16 scope**: Spec text is ambiguous ("CRC of `<data_pkt>`"), but the de facto standard (existing code + all reference mailers bforce/ifcico/qico/FTNd) computes CRC over `EMSI_DAT` + `<len16>` + `<data_pkt>` without `**` prefix. Plan follows de facto standard.
- **DAT length field**: 4 hex chars representing length of `<data_pkt>` only (excluding CRC and CR). Plan matches.
- **Step 2B REQ handling**: Spec says "If EMSI_REQ received, go to step 4" (continue waiting, don't retransmit). Plan: REQ → restart timer, stay in TX_WAIT. Matches.
- **Step 2B "other sequence"**: Spec says retransmit DAT. Plan: NAK → retry (retransmit). Matches.

### Added to plan — high-bit stripping:
- FSC-0056 recommends stripping high-order bit of all received characters before processing and CRC calculation.
- `readEMSI_DAT` must apply `b & 0x7F` to each byte before storing in payload buffer and before CRC validation.
- `readToken` sliding window should also operate on stripped bytes.
- Note: XON/XOFF/NUL stripping (plan item #5) is applied BEFORE high-bit stripping.

### Deliberate spec deviations (improvements matching reference mailers):
- **HBT in TX phase**: Spec Step 2B doesn't mention HBT; only Step 2A does. Plan treats HBT as keepalive in both phases, matching all reference mailers. Harmless: HBT simply resets step timer.
- **RX first-try REQ**: Spec Step 2A says caller skips first send (only answerer sends REQ). Controlled by `SkipFirstRXReq` config option (default: `false` = always send REQ, matching reference mailer behavior). Set to `true` for strict FSC-0056 compliance where caller waits silently on first RX attempt.

### FSC-0088 (EMSI-II) impact:
- FSC-0088 extends EMSI_DAT **field contents only** (new link flags like EII, DFB, FRQ; per-address qualifiers like PUn, HAn; HYD protocol capability).
- **No changes to handshake flow, packet format, or sequence timing.**
- Existing `ParseEMSI_DAT` handles flags generically; no plan changes needed for FSC-0088.

## Verification

- `make test` — all tests pass
- `make build` — all binaries compile (parser, server, testdaemon, modem-test)
- `./bin/testdaemon -test-node "2:5001/100" -test-proto ifcico` — TCP EMSI works
- `./bin/modem-test` against known PSTN node — modem EMSI works
- Debug logs show proper FSM state transitions and per-byte reading
