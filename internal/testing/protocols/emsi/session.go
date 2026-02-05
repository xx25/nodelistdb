package emsi

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
)

// CompletionReason indicates how the EMSI handshake finished
type CompletionReason string

const (
	ReasonNone     CompletionReason = ""         // Not completed yet
	ReasonComplete CompletionReason = "COMPLETE" // Normal handshake finished
	ReasonNCP      CompletionReason = "NCP"      // No Compatible Protocols (test mode)
	ReasonTimeout  CompletionReason = "TIMEOUT"  // Timed out waiting for response
	ReasonError    CompletionReason = "ERROR"     // Error during handshake
)

// DebugFunc is a callback for routing debug output to an external logger.
type DebugFunc func(format string, args ...any)

// Session represents an EMSI session
type Session struct {
	conn         net.Conn
	reader       *bufio.Reader
	writer       *bufio.Writer
	localAddress string
	systemName   string // Our system name
	sysop        string // Our sysop name
	location     string // Our location
	remoteInfo   *EMSIData
	config       *Config // EMSI configuration (FSC-0056.001 compliant defaults)
	debug        bool
	debugFunc    DebugFunc // Optional external debug logger
	bannerText   string    // Capture full banner text during handshake

	// Completion info
	completionReason CompletionReason
	handshakeTiming  HandshakeTiming

	// Master deadline for the current handshake (set by Handshake())
	masterDeadline time.Time

	// EMSI-II negotiation state (FSC-0088.001)
	emsi2Negotiated  bool   // Both sides presented EII
	selectedProtocol string // Final negotiated protocol
}

// HandshakeTiming records timing for each handshake phase
type HandshakeTiming struct {
	InitialPhase time.Duration // Time to get first EMSI response
	DATExchange  time.Duration // Time for DAT packet exchange
	Total        time.Duration // Total handshake time
}

// NewSession creates a new EMSI session with FSC-0056.001 defaults
func NewSession(conn net.Conn, localAddress string) *Session {
	return NewSessionWithConfig(conn, localAddress, nil)
}

// NewSessionWithInfo creates a new EMSI session with custom system info (backward compatible)
// Used by existing code like ifcico_tester.go
func NewSessionWithInfo(conn net.Conn, localAddress, systemName, sysop, location string) *Session {
	return NewSessionWithInfoAndConfig(conn, localAddress, systemName, sysop, location, nil)
}

// NewSessionWithConfig creates a session with custom config
func NewSessionWithConfig(conn net.Conn, localAddress string, cfg *Config) *Session {
	return NewSessionWithInfoAndConfig(conn, localAddress, "NodelistDB Test Daemon", "Test Operator", "Test Location", cfg)
}

// NewSessionWithInfoAndConfig creates a session with system info and custom config
// The config is defensively copied to prevent shared mutation.
func NewSessionWithInfoAndConfig(conn net.Conn, localAddress, systemName, sysop, location string, cfg *Config) *Session {
	if cfg == nil {
		cfg = DefaultConfig()
	} else {
		cfg = cfg.Copy() // Defensive copy to prevent shared mutation
	}
	return &Session{
		conn:         conn,
		reader:       bufio.NewReader(conn),
		writer:       bufio.NewWriter(conn),
		localAddress: localAddress,
		systemName:   systemName,
		sysop:        sysop,
		location:     location,
		config:       cfg,
		debug:        false,
	}
}

// SetDebug enables debug logging
func (s *Session) SetDebug(debug bool) {
	s.debug = debug
}

// SetDebugFunc sets an external debug logging function.
// When set, debug output is routed through this function instead of
// the internal testing/logging package. This allows callers (e.g. modem-test)
// to route EMSI debug output through their own logger.
func (s *Session) SetDebugFunc(fn DebugFunc) {
	s.debugFunc = fn
}

// dbg emits a debug message if debug mode is enabled.
// Routes to debugFunc if set, otherwise falls back to internal logger.
func (s *Session) dbg(format string, args ...any) {
	if !s.debug {
		return
	}
	if s.debugFunc != nil {
		s.debugFunc(format, args...)
		return
	}
	logging.Debugf(format, args...)
}

// GetCompletionReason returns how the handshake finished
func (s *Session) GetCompletionReason() CompletionReason {
	return s.completionReason
}

// GetHandshakeTiming returns timing information for the handshake phases
func (s *Session) GetHandshakeTiming() HandshakeTiming {
	return s.handshakeTiming
}

// IsEMSI2Mode returns true if EMSI-II mode was negotiated (both sides presented EII)
func (s *Session) IsEMSI2Mode() bool {
	return s.emsi2Negotiated
}

// GetSelectedProtocol returns the negotiated file transfer protocol
func (s *Session) GetSelectedProtocol() string {
	return s.selectedProtocol
}

// negotiateEMSI2 determines if EMSI-II mode is active based on mutual EII flag presence
func (s *Session) negotiateEMSI2() {
	// EMSI-II mode requires both sides to present EII
	if s.config.EnableEMSI2 && s.remoteInfo != nil && s.remoteInfo.HasEII {
		s.emsi2Negotiated = true
		if s.debug {
			s.dbg("EMSI: EMSI-II mode negotiated (both sides presented EII)")
		}
	} else {
		s.emsi2Negotiated = false
		if s.debug {
			if !s.config.EnableEMSI2 {
				s.dbg("EMSI: EMSI-I mode (local EII disabled)")
			} else if s.remoteInfo == nil {
				s.dbg("EMSI: EMSI-I mode (no remote info)")
			} else {
				s.dbg("EMSI: EMSI-I mode (remote did not present EII)")
			}
		}
	}
}

// selectProtocol implements protocol selection per FSC-0056.001 and FSC-0088.001
// Returns the first protocol from our list that remote also supports
func (s *Session) selectProtocol() string {
	if s.remoteInfo == nil || len(s.remoteInfo.Protocols) == 0 {
		return ""
	}

	// FSC-0088.001: EMSI-II mandates caller-prefs (caller lists in preference order)
	// FSC-0056.001: EMSI-I was ambiguous, but we use caller-prefs as well
	// CallerPrefsMode controls whether we strictly enforce this
	if s.config.CallerPrefsMode || s.emsi2Negotiated {
		// Caller-prefs: use OUR protocol order, find first match in remote's list
		for _, ourProto := range s.config.Protocols {
			for _, theirProto := range s.remoteInfo.Protocols {
				if strings.EqualFold(ourProto, theirProto) {
					s.selectedProtocol = ourProto
					if s.debug {
						s.dbg("EMSI: Selected protocol %s (caller-prefs)", ourProto)
					}
					return ourProto
				}
			}
		}
	} else {
		// Answerer-prefs (legacy EMSI-I): use THEIR protocol order
		for _, theirProto := range s.remoteInfo.Protocols {
			for _, ourProto := range s.config.Protocols {
				if strings.EqualFold(ourProto, theirProto) {
					s.selectedProtocol = ourProto
					if s.debug {
						s.dbg("EMSI: Selected protocol %s (answerer-prefs)", ourProto)
					}
					return ourProto
				}
			}
		}
	}

	if s.debug {
		s.dbg("EMSI: No compatible protocol found (NCP)")
	}
	return "" // NCP - No Compatible Protocol
}

// SetTimeout is a legacy API for backward compatibility.
// Sets MasterTimeout and scales other timeouts proportionally:
// - If timeout > MasterTimeout: scales up all timeouts proportionally
// - If timeout < MasterTimeout: caps all timeouts at the new value
// Used by existing code that expects a single timeout value.
func (s *Session) SetTimeout(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	if s.config == nil {
		s.config = DefaultConfig()
	}

	oldMaster := s.config.MasterTimeout
	if oldMaster <= 0 {
		oldMaster = 60 * time.Second // FSC default
	}

	// Scale factor for proportional adjustment
	scale := float64(timeout) / float64(oldMaster)

	// Scale helper: adjust timeout proportionally, but cap at new master timeout
	scaleTimeout := func(d time.Duration) time.Duration {
		if d <= 0 {
			return timeout
		}
		scaled := time.Duration(float64(d) * scale)
		if scaled > timeout {
			return timeout
		}
		return scaled
	}

	s.config.MasterTimeout = timeout
	s.config.StepTimeout = scaleTimeout(s.config.StepTimeout)
	s.config.FirstStepTimeout = scaleTimeout(s.config.FirstStepTimeout)
	s.config.CharacterTimeout = scaleTimeout(s.config.CharacterTimeout)
	s.config.InitialCRTimeout = scaleTimeout(s.config.InitialCRTimeout)
	s.config.PreventiveINQTimeout = scaleTimeout(s.config.PreventiveINQTimeout)
}

// ---------------------------------------------------------------------------
// Token types for the FSM
// ---------------------------------------------------------------------------

// emsiToken represents the type of EMSI sequence detected by readToken.
type emsiToken int

const (
	tokenNone    emsiToken = iota
	tokenINQ                // **EMSI_INQC816
	tokenREQ                // **EMSI_REQA77E
	tokenACK                // **EMSI_ACKA490
	tokenNAK                // **EMSI_NAKEEC3
	tokenCLI                // **EMSI_CLIFA8C
	tokenHBT                // **EMSI_HBTEAEE
	tokenDAT                // **EMSI_DAT (header only; caller must read payload via readEMSI_DAT)
	tokenTimeout            // Step or master timeout expired
	tokenCarrier            // Carrier lost (NO CARRIER / BUSY / etc.)
	tokenError              // I/O or other unrecoverable error
)

func (t emsiToken) String() string {
	switch t {
	case tokenNone:
		return "NONE"
	case tokenINQ:
		return "INQ"
	case tokenREQ:
		return "REQ"
	case tokenACK:
		return "ACK"
	case tokenNAK:
		return "NAK"
	case tokenCLI:
		return "CLI"
	case tokenHBT:
		return "HBT"
	case tokenDAT:
		return "DAT"
	case tokenTimeout:
		return "TIMEOUT"
	case tokenCarrier:
		return "CARRIER"
	case tokenError:
		return "ERROR"
	default:
		return fmt.Sprintf("TOKEN(%d)", int(t))
	}
}

// Sentinel errors returned by charReader.getchar.
var (
	errCharTimeout = errors.New("character timeout")
	errCarrierLost = errors.New("carrier lost")
)

// ---------------------------------------------------------------------------
// charReader — single-byte I/O layer
// ---------------------------------------------------------------------------

// charReader provides character-at-a-time reading with line-based carrier
// detection and banner text accumulation.
type charReader struct {
	conn    net.Conn
	reader  *bufio.Reader
	banner  strings.Builder // accumulated non-EMSI text
	debug   bool
	dbgFunc func(string, ...any)

	// Line-based carrier detection
	lineBuf     [64]byte
	linePos     int
	carrierLost bool
}

// newCharReader creates a charReader for the given session.
func newCharReader(s *Session) *charReader {
	return &charReader{
		conn:    s.conn,
		reader:  s.reader,
		debug:   s.debug,
		dbgFunc: s.dbg,
	}
}

// getchar reads a single byte with the given per-character timeout.
// Returns errCharTimeout on deadline expiry and errCarrierLost if a modem
// disconnect signal was detected on a line boundary.
func (cr *charReader) getchar(timeout time.Duration) (byte, error) {
	if cr.carrierLost {
		return 0, errCarrierLost
	}

	_ = cr.conn.SetReadDeadline(time.Now().Add(timeout))
	b, err := cr.reader.ReadByte()
	if err != nil {
		if isTimeoutError(err) {
			return 0, errCharTimeout
		}
		// EOF / connection reset → treat as carrier lost
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			cr.carrierLost = true
			return 0, errCarrierLost
		}
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			cr.carrierLost = true
			return 0, errCarrierLost
		}
		return 0, err
	}

	// Feed to carrier detector
	cr.feedCarrierDetect(b)
	if cr.carrierLost {
		return 0, errCarrierLost
	}

	// Accumulate into banner (printable + whitespace only)
	if b >= 0x20 || b == '\r' || b == '\n' || b == '\t' {
		cr.banner.WriteByte(b)
	}

	return b, nil
}

// getcharFiltered reads a single byte, skipping XON (0x11), XOFF (0x13), and
// NUL (0x00) bytes per FSC-0056. These flow-control bytes can appear anywhere
// in the stream and must be transparently stripped before processing.
// Uses a deadline derived from timeout to prevent a stream of only filtered
// bytes from extending past the intended timeout.
func (cr *charReader) getcharFiltered(timeout time.Duration) (byte, error) {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, errCharTimeout
		}
		b, err := cr.getchar(remaining)
		if err != nil {
			return 0, err
		}
		if b == 0x00 || b == 0x11 || b == 0x13 {
			continue
		}
		return b, nil
	}
}

// feedCarrierDetect accumulates bytes into a line buffer and checks complete
// lines for modem disconnect signals. Only matches at line boundaries to avoid
// false positives from banner/EMSI data containing these words mid-line.
func (cr *charReader) feedCarrierDetect(b byte) {
	// Skip flow-control bytes that would corrupt line matching
	if b == 0x00 || b == 0x11 || b == 0x13 {
		return
	}
	if b == '\r' || b == '\n' {
		if cr.linePos > 0 {
			line := strings.TrimSpace(string(cr.lineBuf[:cr.linePos]))
			switch line {
			case "NO CARRIER", "BUSY", "NO DIALTONE", "NO DIAL TONE", "NO ANSWER":
				cr.carrierLost = true
			}
			cr.linePos = 0
		}
		return
	}
	if cr.linePos < len(cr.lineBuf) {
		cr.lineBuf[cr.linePos] = b
		cr.linePos++
	}
}

// getBannerText returns the accumulated banner text.
func (cr *charReader) getBannerText() string {
	return cr.banner.String()
}

// isTimeoutError checks whether err is a net timeout.
func isTimeoutError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// readToken — sliding-window EMSI sequence detector
// ---------------------------------------------------------------------------

// emsiTokenDef maps a suffix (after "EMSI_") to a token type plus expected CRC.
type emsiTokenDef struct {
	suffix   string    // e.g. "INQ"
	token    emsiToken
	fullCRC  string    // full 14-char constant including ** prefix, e.g. "**EMSI_INQC816"
}

var fixedTokenDefs = []emsiTokenDef{
	{"INQ", tokenINQ, EMSI_INQ},
	{"REQ", tokenREQ, EMSI_REQ},
	{"ACK", tokenACK, EMSI_ACK},
	{"NAK", tokenNAK, EMSI_NAK},
	{"CLI", tokenCLI, EMSI_CLI},
	{"HBT", tokenHBT, EMSI_HBT},
}

// readToken reads bytes one-at-a-time and uses a sliding window to detect
// EMSI sequences. It respects both the step timeout (overall for this call)
// and the character timeout (per-byte).
//
// For fixed-length tokens (INQ, REQ, ACK, NAK, CLI, HBT) it detects the
// "**EMSI_XXX" or "EMSI_XXX" prefix, then reads the 4 CRC bytes and
// validates (logging a warning on mismatch but still returning the token).
//
// For DAT it detects "**EMSI_DAT" or "EMSI_DAT" and returns tokenDAT
// immediately without consuming the length bytes — the caller must use
// readEMSI_DAT() next.
//
// XON (0x11), XOFF (0x13), and NUL (0x00) are silently stripped via getcharFiltered.
func (cr *charReader) readToken(stepTimeout, charTimeout time.Duration, masterDeadline time.Time) emsiToken {
	deadline := time.Now().Add(stepTimeout)
	if !masterDeadline.IsZero() && masterDeadline.Before(deadline) {
		deadline = masterDeadline
	}

	// Sliding window: we look for "EMSI_" (5 chars) then the 3-char type.
	// The window is big enough for "**EMSI_DAT" (10 chars) or "**EMSI_XXXCCCC" (14 chars).
	var window [14]byte
	wpos := 0 // number of valid bytes in window

	for {
		if time.Now().After(deadline) {
			return tokenTimeout
		}

		// Cap character timeout at remaining step time
		remaining := time.Until(deadline)
		ct := min(charTimeout, remaining)
		if ct <= 0 {
			return tokenTimeout
		}

		b, err := cr.getcharFiltered(ct)
		if err != nil {
			if errors.Is(err, errCharTimeout) {
				continue // re-check step deadline at top of loop
			}
			if errors.Is(err, errCarrierLost) {
				return tokenCarrier
			}
			return tokenError
		}

		// High-bit strip for window matching (FSC-0056 §7-bit)
		stripped := b & 0x7F

		// Shift window left and append
		if wpos < len(window) {
			window[wpos] = stripped
			wpos++
		} else {
			copy(window[:], window[1:])
			window[len(window)-1] = stripped
		}

		// Check for "EMSI_" in the window at various positions
		// We look for both "**EMSI_" and bare "EMSI_" for non-compliant mailers
		tok := cr.matchToken(window[:wpos])
		if tok != tokenNone {
			return tok
		}
	}
}

// matchToken checks the current window for an EMSI sequence.
// Returns tokenNone if nothing matches yet.
// Fixed-length tokens are validated against their expected CRC; mismatches
// are rejected to prevent banner data from driving the FSM.
func (cr *charReader) matchToken(window []byte) emsiToken {
	w := string(window)

	// Look for "EMSI_DAT" with optional "**" prefix (no CRC on header — payload has its own)
	if idx := strings.Index(w, "EMSI_DAT"); idx >= 0 {
		return tokenDAT
	}

	// Look for fixed-length tokens: "EMSI_XXX" (8 chars) optionally preceded by "**"
	for _, def := range fixedTokenDefs {
		target := "EMSI_" + def.suffix // 8 chars
		idx := strings.Index(w, target)
		if idx < 0 {
			continue
		}
		afterPrefix := idx + len(target)
		crcCharsAvail := len(w) - afterPrefix
		if crcCharsAvail >= 4 {
			crcGot := w[afterPrefix : afterPrefix+4]
			crcExpected := def.fullCRC[len(def.fullCRC)-4:]

			if !isHexString(crcGot) {
				// Not valid hex after prefix — not a real EMSI token
				return tokenNone
			}

			if !strings.EqualFold(crcGot, crcExpected) {
				if cr.debug {
					cr.dbgFunc("EMSI: readToken: CRC mismatch for %s: got %q, expected %q",
						def.suffix, crcGot, crcExpected)
				}
				return tokenNone
			}
			return def.token
		}
		// Have the prefix but not all 4 CRC chars yet — wait for more bytes.
		if afterPrefix == len(w) || crcCharsAvail < 4 {
			return tokenNone
		}
	}

	return tokenNone
}

// isHexString returns true if s is non-empty and contains only hex digits.
func isHexString(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// readEMSI_DAT — length-driven DAT payload reader
// ---------------------------------------------------------------------------

// readEMSI_DAT reads an EMSI_DAT payload after the "EMSI_DAT" header has
// already been detected and consumed by readToken. It reads:
//   - 4 hex chars → payload length
//   - payloadLen bytes (high-bit stripped)
//   - 4 hex chars → CRC
//
// Returns the reconstructed full packet string suitable for ParseEMSI_DAT.
func (cr *charReader) readEMSI_DAT(charTimeout time.Duration, masterDeadline time.Time, acceptFDLen bool) (string, error) {
	// Read 4 hex length chars
	lenHex := make([]byte, 4)
	for i := range 4 {
		ct := charTimeout
		if !masterDeadline.IsZero() {
			remaining := time.Until(masterDeadline)
			if remaining < ct {
				ct = remaining
			}
			if ct <= 0 {
				return "", fmt.Errorf("master timeout reading DAT length")
			}
		}
		b, err := cr.getcharFiltered(ct)
		if err != nil {
			return "", fmt.Errorf("reading DAT length byte %d: %w", i, err)
		}
		lenHex[i] = b & 0x7F
	}

	payloadLen, err := strconv.ParseInt(string(lenHex), 16, 32)
	if err != nil {
		return "", fmt.Errorf("invalid DAT length hex %q: %w", string(lenHex), err)
	}
	if payloadLen < 0 || payloadLen > 8192 {
		return "", fmt.Errorf("DAT length %d out of range", payloadLen)
	}

	// Read exactly payloadLen bytes with high-bit strip
	payload := make([]byte, payloadLen)
	for i := range int(payloadLen) {
		ct := charTimeout
		if !masterDeadline.IsZero() {
			remaining := time.Until(masterDeadline)
			if remaining < ct {
				ct = remaining
			}
			if ct <= 0 {
				return "", fmt.Errorf("master timeout reading DAT payload at byte %d/%d", i, payloadLen)
			}
		}
		b, err := cr.getcharFiltered(ct)
		if err != nil {
			return "", fmt.Errorf("reading DAT payload byte %d/%d: %w", i, payloadLen, err)
		}
		payload[i] = b & 0x7F
	}

	// Read 4 hex CRC chars
	crcHex := make([]byte, 4)
	for i := range 4 {
		ct := charTimeout
		if !masterDeadline.IsZero() {
			remaining := time.Until(masterDeadline)
			if remaining < ct {
				ct = remaining
			}
			if ct <= 0 {
				return "", fmt.Errorf("master timeout reading DAT CRC")
			}
		}
		b, err := cr.getcharFiltered(ct)
		if err != nil {
			return "", fmt.Errorf("reading DAT CRC byte %d: %w", i, err)
		}
		crcHex[i] = b & 0x7F
	}

	remoteCRC, err := strconv.ParseUint(string(crcHex), 16, 16)
	if err != nil {
		return "", fmt.Errorf("invalid DAT CRC hex %q: %w", string(crcHex), err)
	}

	// CRC is computed over "EMSI_DAT" + lenHex + payload (without ** prefix)
	crcData := append([]byte("EMSI_DAT"), lenHex...)
	crcData = append(crcData, payload...)
	localCRC := CalculateCRC16(crcData)

	if uint16(remoteCRC) != localCRC {
		// FrontDoor length bug: last byte is CR, retry with payload[:len-1]
		if acceptFDLen && payloadLen > 0 && payload[payloadLen-1] == '\r' {
			trimmed := payload[:payloadLen-1]
			crcData2 := append([]byte("EMSI_DAT"), lenHex...)
			crcData2 = append(crcData2, trimmed...)
			if CalculateCRC16(crcData2) == uint16(remoteCRC) {
				if cr.debug {
					cr.dbgFunc("EMSI: readEMSI_DAT: FrontDoor length bug workaround applied")
				}
				payload = trimmed
			} else {
				return "", fmt.Errorf("DAT CRC mismatch: local=%04X remote=%04X", localCRC, remoteCRC)
			}
		} else {
			return "", fmt.Errorf("DAT CRC mismatch: local=%04X remote=%04X", localCRC, remoteCRC)
		}
	}

	// Reconstruct full packet for ParseEMSI_DAT
	packet := fmt.Sprintf("**EMSI_DAT%s%s%s", string(lenHex), string(payload), string(crcHex))
	return packet, nil
}

// ---------------------------------------------------------------------------
// FSM handshake phases
// ---------------------------------------------------------------------------

// Handshake performs the EMSI handshake as caller (we initiate).
// Strategy is selected based on config.InitialStrategy:
//   - "wait": FSC-0056.001 default - wait for remote EMSI_INQ/REQ
//   - "send_cr": Send CRs to wake remote, then wait for EMSI
//   - "send_inq": Immediately send EMSI_INQ
func (s *Session) Handshake() error {
	handshakeStart := time.Now()
	s.completionReason = ReasonNone
	masterDeadline := handshakeStart.Add(s.config.MasterTimeout)
	s.masterDeadline = masterDeadline

	if s.debug {
		s.dbg("EMSI: === Starting EMSI Handshake (FSM) ===")
		s.dbg("EMSI: Strategy: %s, PreventiveINQ: %v", s.config.InitialStrategy, s.config.PreventiveINQ)
		s.dbg("EMSI: Master timeout: %v, Step timeout: %v, Char timeout: %v",
			s.config.MasterTimeout, s.config.StepTimeout, s.config.CharacterTimeout)
		s.dbg("EMSI: Our address: %s", s.localAddress)
	}

	cr := newCharReader(s)

	// --- Phase 1: Initial Contact ---
	initialPhaseStart := time.Now()
	initTok, initDAT, err := s.runInitialPhase(cr, masterDeadline)
	s.handshakeTiming.InitialPhase = time.Since(initialPhaseStart)

	// Capture banner text for fallback extraction
	s.bannerText = cr.getBannerText()

	if err != nil {
		s.completionReason = ReasonError
		s.handshakeTiming.Total = time.Since(handshakeStart)
		return err
	}

	if s.debug {
		s.dbg("EMSI: Initial phase completed in %v, token=%s", s.handshakeTiming.InitialPhase, initTok)
	}

	datExchangeStart := time.Now()

	// --- Phase 2 & 3: DAT exchange ---
	switch initTok {
	case tokenREQ:
		// Remote wants our DAT first, then we get theirs
		if err := s.runTXPhase(cr, masterDeadline); err != nil {
			s.completionReason = ReasonError
			s.handshakeTiming.Total = time.Since(handshakeStart)
			return fmt.Errorf("TX phase: %w", err)
		}
		if err := s.runRXPhase(cr, masterDeadline, false); err != nil {
			s.completionReason = ReasonError
			s.handshakeTiming.Total = time.Since(handshakeStart)
			return fmt.Errorf("RX phase: %w", err)
		}

	case tokenINQ:
		// Remote sent INQ — we already sent REQ in initial phase
		if err := s.runRXPhase(cr, masterDeadline, true); err != nil {
			s.completionReason = ReasonError
			s.handshakeTiming.Total = time.Since(handshakeStart)
			return fmt.Errorf("RX phase: %w", err)
		}
		if err := s.runTXPhase(cr, masterDeadline); err != nil {
			s.completionReason = ReasonError
			s.handshakeTiming.Total = time.Since(handshakeStart)
			return fmt.Errorf("TX phase: %w", err)
		}

	case tokenDAT:
		// Remote sent DAT directly — FSC-0056.001: ACK before processing
		if err := s.sendEMSI_ACK(); err != nil {
			if s.debug {
				s.dbg("EMSI: WARNING: Failed to send ACK: %v", err)
			}
		}
		if s.config.SendACKTwice {
			_ = s.sendEMSI_ACK()
		}
		info, parseErr := ParseEMSI_DAT(initDAT)
		if parseErr != nil {
			if s.debug {
				s.dbg("EMSI: DAT-first parse failed after ACK: %v (proceeding without remote info)", parseErr)
			}
		} else {
			s.remoteInfo = info
			s.logRemoteInfo()
		}
		// Send our DAT
		if err := s.runTXPhase(cr, masterDeadline); err != nil {
			s.completionReason = ReasonError
			s.handshakeTiming.Total = time.Since(handshakeStart)
			return fmt.Errorf("TX phase: %w", err)
		}

	default:
		// No EMSI token received — try banner extraction fallback
		s.bannerText = cr.getBannerText()
		if software := s.extractSoftwareFromBanner(); software != nil {
			s.remoteInfo = &EMSIData{
				MailerName:    software.Name,
				MailerVersion: software.Version,
				SystemName:    "[Extracted from banner]",
			}
			if software.Platform != "" {
				s.remoteInfo.MailerSerial = software.Platform
			}
			if s.debug {
				s.dbg("EMSI: Extracted software from banner: %s %s", software.Name, software.Version)
			}
		}

		s.completionReason = ReasonTimeout
		s.handshakeTiming.Total = time.Since(handshakeStart)
		bannerText := cr.getBannerText()
		if len(bannerText) > 0 {
			preview := formatResponsePreview(bannerText, 200)
			return fmt.Errorf("no EMSI token received\nReceived %d bytes: %s", len(bannerText), preview)
		}
		return fmt.Errorf("no EMSI token received (timeout)")
	}

	// Record timing
	s.handshakeTiming.DATExchange = time.Since(datExchangeStart)
	s.handshakeTiming.Total = time.Since(handshakeStart)

	// Capture final banner text
	s.bannerText = cr.getBannerText()

	// EMSI-II negotiation (FSC-0088.001)
	s.negotiateEMSI2()
	s.selectProtocol()

	// Determine completion reason based on protocol negotiation
	if len(s.config.Protocols) == 0 {
		s.completionReason = ReasonNCP // No Compatible Protocols (test mode)
	} else if s.selectedProtocol == "" && len(s.config.Protocols) > 0 {
		s.completionReason = ReasonNCP
	} else {
		s.completionReason = ReasonComplete
	}

	if s.debug {
		s.dbg("EMSI: === Handshake Complete (%s) ===", s.completionReason)
		s.dbg("EMSI: Timing: Initial=%v, DATExchange=%v, Total=%v",
			s.handshakeTiming.InitialPhase, s.handshakeTiming.DATExchange, s.handshakeTiming.Total)
	}

	return nil
}

// runInitialPhase implements Phase 1: Initial Contact.
// Returns the token that ended the phase plus any DAT packet data.
func (s *Session) runInitialPhase(cr *charReader, masterDeadline time.Time) (emsiToken, string, error) {
	// Execute strategy-specific preamble
	switch s.config.InitialStrategy {
	case "send_cr":
		if err := s.initialSendCRs(cr, masterDeadline); err != nil {
			return tokenError, "", err
		}
	case "send_inq":
		if err := s.sendEMSI_INQ(); err != nil {
			return tokenError, "", fmt.Errorf("send_inq strategy: failed to send INQ: %w", err)
		}
	}
	// "wait" strategy: do nothing, just read

	// First read attempt uses FirstStepTimeout
	stepTimeout := s.config.FirstStepTimeout
	if stepTimeout <= 0 {
		stepTimeout = s.config.StepTimeout
	}

	tok := cr.readToken(stepTimeout, s.config.CharacterTimeout, masterDeadline)

	if tok == tokenDAT {
		// Read the full DAT packet
		dat, err := cr.readEMSI_DAT(s.config.CharacterTimeout, masterDeadline, s.config.AcceptFDLenWithCR)
		if err != nil {
			if s.debug {
				s.dbg("EMSI: Initial phase: DAT header detected but read failed: %v", err)
			}
			// Fall through to retry/preventive INQ logic
			tok = tokenTimeout
		} else {
			return tokenDAT, dat, nil
		}
	}

	// Handle INQ: send REQ in response, then return INQ so Handshake knows the flow
	if tok == tokenINQ {
		if s.debug {
			s.dbg("EMSI: Received INQ, sending REQ")
		}
		if err := s.sendEMSI_REQ(); err != nil {
			return tokenError, "", fmt.Errorf("failed to send REQ after INQ: %w", err)
		}
		return tokenINQ, "", nil
	}

	// REQ = remote wants our DAT → return to Handshake
	if tok == tokenREQ {
		return tok, "", nil
	}

	// ACK/NAK/CLI are noise during initial phase (stale from previous session
	// or non-compliant mailer). Treat like HBT: retry with step timeout.
	// HBT = keepalive, also restart step timer.
	if tok == tokenHBT || tok == tokenACK || tok == tokenNAK || tok == tokenCLI {
		if s.debug && tok != tokenHBT {
			s.dbg("EMSI: Initial phase: ignoring unexpected %s, retrying", tok)
		}
		return s.initialRetry(cr, masterDeadline, s.config.StepTimeout)
	}

	// Timeout or carrier with no EMSI — try PreventiveINQ if enabled
	if tok == tokenCarrier {
		return tokenCarrier, "", fmt.Errorf("carrier lost during initial phase")
	}
	if tok == tokenError {
		return tokenError, "", fmt.Errorf("I/O error during initial phase")
	}

	// tokenTimeout — try preventive INQ or send_inq retry
	if s.config.PreventiveINQ || s.config.InitialStrategy == "send_inq" {
		if s.debug {
			s.dbg("EMSI: Initial phase timeout, sending INQ")
		}
		if err := s.sendEMSI_INQ(); err != nil {
			return tokenError, "", fmt.Errorf("failed to send INQ: %w", err)
		}

		// Second attempt — use PreventiveINQTimeout if set, otherwise StepTimeout
		retryTimeout := s.config.StepTimeout
		if s.config.PreventiveINQ && s.config.PreventiveINQTimeout > 0 {
			retryTimeout = s.config.PreventiveINQTimeout
		}
		return s.initialRetry(cr, masterDeadline, retryTimeout)
	}

	// If SendINQTwice is set for send_inq strategy, this was already the second attempt
	return tokenTimeout, "", fmt.Errorf("timeout waiting for EMSI response in initial phase")
}

// initialRetry reads another token after sending INQ or receiving HBT.
func (s *Session) initialRetry(cr *charReader, masterDeadline time.Time, stepTimeout time.Duration) (emsiToken, string, error) {
	tok := cr.readToken(stepTimeout, s.config.CharacterTimeout, masterDeadline)

	if tok == tokenDAT {
		dat, err := cr.readEMSI_DAT(s.config.CharacterTimeout, masterDeadline, s.config.AcceptFDLenWithCR)
		if err != nil {
			return tokenTimeout, "", fmt.Errorf("incomplete DAT in initial retry: %w", err)
		}
		return tokenDAT, dat, nil
	}
	if tok == tokenINQ {
		if s.debug {
			s.dbg("EMSI: Received INQ in retry, sending REQ")
		}
		if err := s.sendEMSI_REQ(); err != nil {
			return tokenError, "", fmt.Errorf("failed to send REQ: %w", err)
		}
		return tokenINQ, "", nil
	}
	if tok == tokenREQ {
		return tokenREQ, "", nil
	}
	if tok == tokenHBT {
		// HBT again — restart, but check master deadline
		if time.Now().After(masterDeadline) {
			return tokenTimeout, "", fmt.Errorf("master timeout after HBT in initial phase")
		}
		return s.initialRetry(cr, masterDeadline, stepTimeout)
	}
	if tok == tokenCarrier {
		return tokenCarrier, "", fmt.Errorf("carrier lost during initial phase")
	}
	if tok == tokenTimeout {
		return tokenTimeout, "", fmt.Errorf("timeout in initial phase retry")
	}
	return tok, "", fmt.Errorf("unexpected token %s in initial phase", tok)
}

// initialSendCRs sends CRs at InitialCRInterval to wake a remote BBS.
func (s *Session) initialSendCRs(cr *charReader, masterDeadline time.Time) error {
	if s.debug {
		s.dbg("EMSI: Strategy=send_cr: Sending CRs to trigger remote EMSI...")
	}

	deadline := time.Now().Add(s.config.InitialCRTimeout)
	if masterDeadline.Before(deadline) {
		deadline = masterDeadline
	}

	for time.Now().Before(deadline) {
		_ = s.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if _, err := s.writer.WriteString("\r"); err != nil {
			return fmt.Errorf("failed to send CR: %w", err)
		}
		if err := s.writer.Flush(); err != nil {
			return fmt.Errorf("failed to flush CR: %w", err)
		}

		time.Sleep(s.config.InitialCRInterval)

		// Check if we have data available
		_ = s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		if _, err := cr.reader.Peek(1); err == nil {
			if s.debug {
				s.dbg("EMSI: Strategy=send_cr: Got initial data from remote")
			}
			break
		}
	}
	return nil
}

// runTXPhase implements Phase 2: Send our EMSI_DAT and wait for ACK.
func (s *Session) runTXPhase(cr *charReader, masterDeadline time.Time) error {
	maxRetries := s.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 6
	}

	for retry := 0; retry < maxRetries; retry++ {
		if time.Now().After(masterDeadline) {
			return fmt.Errorf("master timeout in TX phase")
		}
		if retry > 0 && s.config.RetryDelay > 0 {
			time.Sleep(s.config.RetryDelay)
		}

		if s.debug {
			s.dbg("EMSI: TX: Sending EMSI_DAT (attempt %d/%d)", retry+1, maxRetries)
		}
		if err := s.sendEMSI_DAT(); err != nil {
			return fmt.Errorf("failed to send EMSI_DAT: %w", err)
		}

		// Wait for ACK, skipping HBTs (FSC-0056: HBT resets step timer only)
		tok := cr.readToken(s.config.StepTimeout, s.config.CharacterTimeout, masterDeadline)
		for tok == tokenHBT {
			if s.debug {
				s.dbg("EMSI: TX: Received HBT, restarting step timer")
			}
			tok = cr.readToken(s.config.StepTimeout, s.config.CharacterTimeout, masterDeadline)
		}

		switch tok {
		case tokenACK:
			if s.debug {
				s.dbg("EMSI: TX: Received ACK")
			}
			return nil

		case tokenNAK:
			if s.debug {
				s.dbg("EMSI: TX: Received NAK, retrying")
			}
			continue

		case tokenREQ:
			if s.debug {
				s.dbg("EMSI: TX: Received REQ, resending DAT")
			}
			continue

		case tokenINQ:
			if s.debug {
				s.dbg("EMSI: TX: Received INQ, remote restarting")
			}
			continue

		case tokenDAT:
			// Remote sent DAT before ACKing ours
			dat, err := cr.readEMSI_DAT(s.config.CharacterTimeout, masterDeadline, s.config.AcceptFDLenWithCR)
			if err != nil {
				if s.debug {
					s.dbg("EMSI: TX: Received DAT but read failed: %v", err)
				}
				continue
			}
			// FSC-0056.001: send ACK before processing
			if ackErr := s.sendEMSI_ACK(); ackErr != nil {
				if s.debug {
					s.dbg("EMSI: TX: WARNING: Failed to send ACK: %v", ackErr)
				}
			}
			if s.config.SendACKTwice {
				_ = s.sendEMSI_ACK()
			}
			info, parseErr := ParseEMSI_DAT(dat)
			if parseErr != nil {
				if s.debug {
					s.dbg("EMSI: TX: DAT parse failed after ACK: %v (proceeding)", parseErr)
				}
			} else {
				s.remoteInfo = info
				s.logRemoteInfo()
			}
			return nil

		case tokenTimeout:
			if s.debug {
				s.dbg("EMSI: TX: Step timeout, retrying")
			}
			continue

		case tokenCarrier:
			return fmt.Errorf("carrier lost in TX phase")

		default:
			if s.debug {
				s.dbg("EMSI: TX: Unexpected token %s", tok)
			}
			continue
		}
	}

	return fmt.Errorf("TX phase: max retries (%d) exceeded", maxRetries)
}

// runRXPhase implements Phase 3: Receive remote's EMSI_DAT.
// reqAlreadySent indicates whether REQ was already sent (e.g. in initial phase
// after receiving INQ), preventing a redundant REQ on the first try.
func (s *Session) runRXPhase(cr *charReader, masterDeadline time.Time, reqAlreadySent bool) error {
	// If we already have remoteInfo (from TX phase receiving DAT), skip RX
	if s.remoteInfo != nil {
		if s.debug {
			s.dbg("EMSI: RX: Remote info already available, skipping RX phase")
		}
		return nil
	}

	maxRetries := s.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 6
	}

	firstTry := true
	for retry := 0; retry < maxRetries; retry++ {
		if time.Now().After(masterDeadline) {
			return fmt.Errorf("master timeout in RX phase")
		}
		if retry > 0 && s.config.RetryDelay > 0 {
			time.Sleep(s.config.RetryDelay)
		}

		// Send REQ (or NAK on retry)
		if firstTry && (s.config.SkipFirstRXReq || reqAlreadySent) {
			if s.debug {
				s.dbg("EMSI: RX: Skipping first REQ")
			}
		} else if retry > 0 && s.config.SendNAKOnRetry {
			if s.debug {
				s.dbg("EMSI: RX: Sending NAK (retry %d)", retry)
			}
			s.sendEMSI_NAK()
		} else {
			if s.debug {
				s.dbg("EMSI: RX: Sending REQ (attempt %d/%d)", retry+1, maxRetries)
			}
			if err := s.sendEMSI_REQ(); err != nil {
				return fmt.Errorf("failed to send REQ: %w", err)
			}
		}
		firstTry = false

		// Wait for DAT, skipping HBTs and stale ACKs (FSC-0056: reset timer only)
		tok := cr.readToken(s.config.StepTimeout, s.config.CharacterTimeout, masterDeadline)
		for tok == tokenHBT || tok == tokenACK {
			if s.debug {
				if tok == tokenHBT {
					s.dbg("EMSI: RX: Received HBT, restarting step timer")
				} else {
					s.dbg("EMSI: RX: Received stale ACK, ignoring")
				}
			}
			tok = cr.readToken(s.config.StepTimeout, s.config.CharacterTimeout, masterDeadline)
		}

		switch tok {
		case tokenDAT:
			dat, err := cr.readEMSI_DAT(s.config.CharacterTimeout, masterDeadline, s.config.AcceptFDLenWithCR)
			if err != nil {
				if s.debug {
					s.dbg("EMSI: RX: DAT read failed: %v, retrying", err)
				}
				continue
			}
			return s.rxValidate(dat)

		case tokenINQ:
			if s.debug {
				s.dbg("EMSI: RX: Received INQ, remote restarting")
			}
			continue

		case tokenREQ:
			if s.debug {
				s.dbg("EMSI: RX: Received REQ, resending our DAT")
			}
			_ = s.sendEMSI_DAT()
			continue

		case tokenNAK:
			if s.debug {
				s.dbg("EMSI: RX: Received NAK, resending our DAT")
			}
			_ = s.sendEMSI_DAT()
			continue

		case tokenTimeout:
			if s.debug {
				s.dbg("EMSI: RX: Step timeout, retrying")
			}
			continue

		case tokenCarrier:
			return fmt.Errorf("carrier lost in RX phase")

		default:
			if s.debug {
				s.dbg("EMSI: RX: Unexpected token %s", tok)
			}
			continue
		}
	}

	// Exhausted retries — try banner extraction fallback
	s.bannerText = cr.getBannerText()
	if software := s.extractSoftwareFromBanner(); software != nil {
		s.remoteInfo = &EMSIData{
			MailerName:    software.Name,
			MailerVersion: software.Version,
			SystemName:    "[Extracted from banner]",
		}
		if software.Platform != "" {
			s.remoteInfo.MailerSerial = software.Platform
		}
		if s.debug {
			s.dbg("EMSI: RX: Extracted software from banner: %s %s", software.Name, software.Version)
		}
		return nil
	}

	return fmt.Errorf("RX phase: max retries (%d) exceeded, no remote DAT received", maxRetries)
}

// rxValidate sends ACK then parses a received DAT packet.
// FSC-0056.001: transmit ACK before processing (CRC already validated by readEMSI_DAT).
func (s *Session) rxValidate(datPacket string) error {
	// Send ACK before processing per FSC-0056.001
	if err := s.sendEMSI_ACK(); err != nil {
		if s.debug {
			s.dbg("EMSI: RX: WARNING: Failed to send ACK: %v", err)
		}
	}
	if s.config.SendACKTwice {
		_ = s.sendEMSI_ACK()
	}

	info, err := ParseEMSI_DAT(datPacket)
	if err != nil {
		if s.debug {
			s.dbg("EMSI: RX: DAT parse failed after ACK: %v", err)
		}
		return fmt.Errorf("RX phase: DAT parse failed: %w", err)
	}

	s.remoteInfo = info
	s.logRemoteInfo()
	return nil
}

// logRemoteInfo logs parsed remote info at debug level.
func (s *Session) logRemoteInfo() {
	if !s.debug || s.remoteInfo == nil {
		return
	}
	s.dbg("EMSI: Parsed remote data:")
	s.dbg("EMSI:   System: %s", s.remoteInfo.SystemName)
	s.dbg("EMSI:   Location: %s", s.remoteInfo.Location)
	s.dbg("EMSI:   Sysop: %s", s.remoteInfo.Sysop)
	s.dbg("EMSI:   Mailer: %s %s", s.remoteInfo.MailerName, s.remoteInfo.MailerVersion)
	s.dbg("EMSI:   Addresses: %v", s.remoteInfo.Addresses)
}

// ---------------------------------------------------------------------------
// Wire-write methods
// ---------------------------------------------------------------------------

// writeDeadline returns a write deadline capped at the handshake's master
// deadline to prevent writes from extending beyond the overall timeout.
func (s *Session) writeDeadline() time.Time {
	deadline := time.Now().Add(s.config.MasterTimeout)
	if !s.masterDeadline.IsZero() && s.masterDeadline.Before(deadline) {
		deadline = s.masterDeadline
	}
	return deadline
}

// sendEMSI_INQ sends EMSI inquiry. If SendINQTwice is configured, sends
// twice with INQInterval delay between sends (per FSC-0056.001 step 1).
func (s *Session) sendEMSI_INQ() error {
	if s.debug {
		s.dbg("EMSI: sendEMSI_INQ: Sending EMSI_INQ")
	}

	_ = s.conn.SetWriteDeadline(s.writeDeadline())

	if _, err := s.writer.WriteString(EMSI_INQ + "\r"); err != nil {
		if s.debug {
			s.dbg("EMSI: sendEMSI_INQ: ERROR writing: %v", err)
		}
		return err
	}

	if err := s.writer.Flush(); err != nil {
		if s.debug {
			s.dbg("EMSI: sendEMSI_INQ: ERROR flushing: %v", err)
		}
		return err
	}

	// FSC-0056 step 1: "Transmit EMSI_INQ twice"
	if s.config.SendINQTwice {
		if s.config.INQInterval > 0 {
			time.Sleep(s.config.INQInterval)
		}
		_ = s.conn.SetWriteDeadline(s.writeDeadline())
		if _, err := s.writer.WriteString(EMSI_INQ + "\r"); err != nil {
			if s.debug {
				s.dbg("EMSI: sendEMSI_INQ: ERROR writing second INQ: %v", err)
			}
			return err
		}
		if err := s.writer.Flush(); err != nil {
			return err
		}
	}

	if s.debug {
		s.dbg("EMSI: sendEMSI_INQ: Successfully sent EMSI_INQ")
	}
	return nil
}

// sendEMSI_ACK sends EMSI acknowledgment
func (s *Session) sendEMSI_ACK() error {
	if s.debug {
		s.dbg("EMSI: Sending EMSI_ACK")
	}

	_ = s.conn.SetWriteDeadline(s.writeDeadline())

	if _, err := s.writer.WriteString(EMSI_ACK + "\r"); err != nil {
		return err
	}
	return s.writer.Flush()
}

// sendEMSI_REQ sends EMSI request. If SendREQTwice is configured, sends
// twice (per FSC-0056.001).
func (s *Session) sendEMSI_REQ() error {
	if s.debug {
		s.dbg("EMSI: Sending EMSI_REQ")
	}

	_ = s.conn.SetWriteDeadline(s.writeDeadline())

	if _, err := s.writer.WriteString(EMSI_REQ + "\r"); err != nil {
		return err
	}
	if err := s.writer.Flush(); err != nil {
		return err
	}

	if s.config.SendREQTwice {
		_ = s.conn.SetWriteDeadline(s.writeDeadline())
		if _, err := s.writer.WriteString(EMSI_REQ + "\r"); err != nil {
			return err
		}
		return s.writer.Flush()
	}
	return nil
}

// sendEMSI_NAK sends EMSI negative acknowledgment
func (s *Session) sendEMSI_NAK() error {
	if s.debug {
		s.dbg("EMSI: Sending EMSI_NAK")
	}

	_ = s.conn.SetWriteDeadline(s.writeDeadline())

	if _, err := s.writer.WriteString(EMSI_NAK + "\r"); err != nil {
		return err
	}
	return s.writer.Flush()
}

// sendEMSI_DAT sends our EMSI data packet
func (s *Session) sendEMSI_DAT() error {
	if s.debug {
		s.dbg("EMSI: sendEMSI_DAT: Preparing EMSI_DAT packet...")
	}

	// Create our EMSI data using configured protocols
	protocols := s.config.Protocols
	if len(protocols) == 0 {
		protocols = []string{"ZMO", "ZAP"} // Default protocols
	}

	data := &EMSIData{
		SystemName:    s.systemName,
		Location:      s.location,
		Sysop:         s.sysop,
		Phone:         "-Unpublished-",
		Speed:         "9600",                   // Numeric baud for compatibility
		Flags:         "CM,IFC,XA",              // Traditional flags: Continuous Mail, IFCICO, Mail only
		MailerName:    "NodelistDB",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",                    // Traditional OS identifier
		Addresses:     []string{s.localAddress}, // Bare address without @fidonet suffix
		Protocols:     protocols,
		Password:      "", // Empty password
	}

	// Use config-aware packet builder for EMSI-II support
	packet := CreateEMSI_DATWithConfig(data, s.config)

	if s.debug {
		s.dbg("EMSI: sendEMSI_DAT: Created EMSI_DAT packet (%d bytes)", len(packet))
		if len(packet) > 200 {
			s.dbg("EMSI: sendEMSI_DAT: DAT packet (first 200): %q", packet[:200])
		} else {
			s.dbg("EMSI: sendEMSI_DAT: DAT packet: %q", packet)
		}
	}

	_ = s.conn.SetWriteDeadline(s.writeDeadline())

	// Send the packet with CR (binkleyforce-compatible, ifcico drops XON anyway)
	if _, err := s.writer.WriteString(packet + "\r"); err != nil {
		if s.debug {
			s.dbg("EMSI: sendEMSI_DAT: ERROR writing packet: %v", err)
		}
		return fmt.Errorf("failed to write EMSI_DAT: %w", err)
	}

	// Send additional CR (some mailers expect double CR)
	if _, err := s.writer.WriteString("\r"); err != nil {
		if s.debug {
			s.dbg("EMSI: sendEMSI_DAT: ERROR writing additional CR: %v", err)
		}
		return fmt.Errorf("failed to write additional CR: %w", err)
	}

	if err := s.writer.Flush(); err != nil {
		if s.debug {
			s.dbg("EMSI: sendEMSI_DAT: ERROR flushing buffer: %v", err)
		}
		return fmt.Errorf("failed to flush EMSI_DAT: %w", err)
	}

	if s.debug {
		s.dbg("EMSI: sendEMSI_DAT: Successfully sent EMSI_DAT packet")
	}

	return nil
}

// ---------------------------------------------------------------------------
// Public API (unchanged)
// ---------------------------------------------------------------------------

// GetRemoteInfo returns the collected remote node information
func (s *Session) GetRemoteInfo() *EMSIData {
	return s.remoteInfo
}

// Close closes the session
func (s *Session) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// ValidateAddress checks if the remote announced the expected address
func (s *Session) ValidateAddress(expectedAddress string) bool {
	if s.remoteInfo == nil {
		return false
	}

	// Normalize addresses for comparison
	expected := normalizeAddress(expectedAddress)

	for _, addr := range s.remoteInfo.Addresses {
		if normalizeAddress(addr) == expected {
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Utilities (unchanged)
// ---------------------------------------------------------------------------

// normalizeAddress normalizes a FidoNet address for comparison
func normalizeAddress(addr string) string {
	// Remove spaces and convert to lowercase
	addr = strings.TrimSpace(strings.ToLower(addr))

	// Remove @domain if present
	if idx := strings.Index(addr, "@"); idx != -1 {
		addr = addr[:idx]
	}

	// Remove .0 point suffix (point 0 is implicit)
	// e.g., 2:222/0.0 -> 2:222/0
	addr = strings.TrimSuffix(addr, ".0")

	return addr
}

// formatResponsePreview formats response data for error messages.
// Returns text if data is 7-bit ASCII printable, otherwise hex dump.
func formatResponsePreview(data string, maxLen int) string {
	if len(data) == 0 {
		return "(empty)"
	}

	// Check if data is 7-bit ASCII printable (or common control chars)
	is7BitASCII := true
	for _, b := range []byte(data) {
		// Allow printable ASCII (32-126), tab, CR, LF
		if b < 32 && b != '\t' && b != '\r' && b != '\n' {
			is7BitASCII = false
			break
		}
		if b > 126 {
			is7BitASCII = false
			break
		}
	}

	if is7BitASCII {
		// Show as text, truncate if needed
		preview := data
		if len(preview) > maxLen {
			preview = preview[:maxLen] + "..."
		}
		// Replace control chars for display
		preview = strings.ReplaceAll(preview, "\r\n", "\\r\\n\n")
		preview = strings.ReplaceAll(preview, "\r", "\\r\n")
		preview = strings.ReplaceAll(preview, "\n", "\\n\n")
		return preview
	}

	// Show as hex dump
	hexLen := min(maxLen/3, len(data)) // Each byte takes ~3 chars in hex

	var hex strings.Builder
	for i := range hexLen {
		if i > 0 {
			hex.WriteByte(' ')
		}
		fmt.Fprintf(&hex, "%02X", data[i])
	}
	if hexLen < len(data) {
		hex.WriteString("...")
	}
	return hex.String()
}
