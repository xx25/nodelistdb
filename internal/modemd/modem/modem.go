package modem

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mfkenney/go-serial/v2"
)

// Config contains modem configuration
type Config struct {
	Device       string        // Serial port device (e.g., /dev/ttyUSB0, /dev/ttyACM0)
	BaudRate     int           // Serial port baud rate (e.g., 115200)
	InitString   string        // Modem init command (e.g., "ATZ" or "AT&F") - used if InitCommands is empty
	InitCommands []string      // Ordered list of init commands (overrides InitString if non-empty)
	DialPrefix   string        // Dial command prefix (e.g., "ATDT" or "ATD")
	HangupMethod string        // "dtr" (drop DTR) or "escape" (+++ ATH)
	LineStatsCommand string    // AT command for line stats (default AT&V1)
	Debug        bool          // Enable debug logging of all modem I/O

	// Timeouts
	DialTimeout      time.Duration // Timeout for dial (waiting for CONNECT/error), default 200s
	CarrierTimeout   time.Duration // Timeout for DCD after CONNECT, default 5s
	ATCommandTimeout time.Duration // Timeout for AT command responses
	ReadTimeout      time.Duration // Default read timeout for serial port
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() Config {
	return Config{
		BaudRate:         115200,
		InitString:       ATZ,
		DialPrefix:       ATDT,
		HangupMethod:     "dtr",
		LineStatsCommand: ATLineStats,
		DialTimeout:      200 * time.Second,
		CarrierTimeout:   5 * time.Second,
		ATCommandTimeout: 5 * time.Second,
		ReadTimeout:      1 * time.Second,
	}
}

// ModemStatus represents the current state of modem control lines
type ModemStatus struct {
	DCD bool // Data Carrier Detect - true when connected
	DSR bool // Data Set Ready - modem is powered and ready
	CTS bool // Clear To Send - flow control
	RI  bool // Ring Indicator - incoming call
}

// Modem represents a single modem connected via serial port
type Modem struct {
	config Config
	port   *serial.Port

	mu          sync.Mutex
	initialized bool
	inDataMode  bool // true after CONNECT, false after hangup/reset
}

// New creates a new Modem instance with the given configuration.
// The modem is not opened until Open() is called.
func New(cfg Config) (*Modem, error) {
	if cfg.Device == "" {
		return nil, errors.New("modem device path is required")
	}

	// Apply defaults for zero values
	if cfg.BaudRate == 0 {
		cfg.BaudRate = 115200
	}
	if cfg.InitString == "" {
		cfg.InitString = ATZ
	}
	if cfg.DialPrefix == "" {
		cfg.DialPrefix = ATDT
	}
	if cfg.HangupMethod == "" {
		cfg.HangupMethod = "dtr"
	}
	if cfg.LineStatsCommand == "" {
		cfg.LineStatsCommand = ATLineStats
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 200 * time.Second
	}
	if cfg.CarrierTimeout == 0 {
		cfg.CarrierTimeout = 5 * time.Second
	}
	if cfg.ATCommandTimeout == 0 {
		cfg.ATCommandTimeout = 5 * time.Second
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 1 * time.Second
	}

	return &Modem{
		config: cfg,
	}, nil
}

// debugTimestamp returns current time formatted for debug output
func debugTimestamp() string {
	return time.Now().Format("15:04:05.000")
}

// debugLog prints debug message if debug mode is enabled (uses stderr for unbuffered output)
func (m *Modem) debugLog(format string, args ...interface{}) {
	if m.config.Debug {
		fmt.Fprintf(os.Stderr, "[%s MODEM] "+format+"\n", append([]interface{}{debugTimestamp()}, args...)...)
	}
}

// debugLogTX logs data being sent to modem
func (m *Modem) debugLogTX(data []byte) {
	if m.config.Debug {
		fmt.Fprintf(os.Stderr, "[%s TX] %q\n", debugTimestamp(), string(data))
	}
}

// debugLogRX logs data received from modem
func (m *Modem) debugLogRX(data []byte) {
	if m.config.Debug {
		fmt.Fprintf(os.Stderr, "[%s RX] %q\n", debugTimestamp(), string(data))
	}
}

// debugLogStatus logs RS232 control line status
func (m *Modem) debugLogStatus(label string) {
	if !m.config.Debug {
		return
	}
	status, err := m.getStatusLocked()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s RS232] %s: error: %v\n", debugTimestamp(), label, err)
		return
	}
	fmt.Fprintf(os.Stderr, "[%s RS232] %s: DCD=%v DSR=%v CTS=%v RI=%v\n",
		debugTimestamp(), label, status.DCD, status.DSR, status.CTS, status.RI)
}

// Open opens the serial port and initializes the modem
func (m *Modem) Open() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.port != nil {
		return errors.New("modem already open")
	}

	// Open serial port with modem-appropriate settings
	port, err := serial.Open(m.config.Device,
		serial.WithBaudrate(m.config.BaudRate),
		serial.WithDataBits(8),
		serial.WithParity(serial.NoParity),
		serial.WithStopBits(serial.OneStopBit),
		serial.WithReadTimeout(int(m.config.ReadTimeout.Milliseconds())),
		serial.WithHUPCL(true), // Drop DTR on close
	)
	if err != nil {
		return fmt.Errorf("failed to open serial port %s: %w", m.config.Device, err)
	}

	m.port = port

	// Ensure DTR is high (modem ready)
	if err := port.SetDTR(true); err != nil {
		port.Close()
		m.port = nil
		return fmt.Errorf("failed to set DTR: %w", err)
	}

	// Small delay for modem to stabilize
	time.Sleep(100 * time.Millisecond)

	// Initialize modem with init string
	if err := m.initModem(); err != nil {
		port.Close()
		m.port = nil
		return fmt.Errorf("failed to initialize modem: %w", err)
	}

	m.initialized = true
	m.inDataMode = false

	return nil
}

// initModem sends initialization commands to the modem
func (m *Modem) initModem() error {
	// Flush any pending data
	_ = m.port.ResetInputBuffer()
	_ = m.port.ResetOutputBuffer()

	// If InitCommands is provided, use it instead of default sequence
	if len(m.config.InitCommands) > 0 {
		return m.initModemWithCommands(m.config.InitCommands)
	}

	// Default initialization sequence (backward compatible)
	// Send init string (usually ATZ)
	response, err := m.sendATLocked(m.config.InitString, m.config.ATCommandTimeout)
	if err != nil {
		return fmt.Errorf("init command failed: %w", err)
	}
	if !IsSuccessResponse(response) {
		return fmt.Errorf("init command returned: %s", response)
	}

	// Disable echo for cleaner responses
	response, err = m.sendATLocked(ATE0, m.config.ATCommandTimeout)
	if err != nil {
		return fmt.Errorf("ATE0 failed: %w", err)
	}
	if !IsSuccessResponse(response) {
		return fmt.Errorf("ATE0 returned: %s", response)
	}

	// Enable verbose result codes
	response, err = m.sendATLocked(ATV1, m.config.ATCommandTimeout)
	if err != nil {
		return fmt.Errorf("ATV1 failed: %w", err)
	}

	// Enable extended result codes (BUSY, NO DIALTONE, etc.)
	response, err = m.sendATLocked(ATX4, m.config.ATCommandTimeout)
	if err != nil {
		// Not all modems support X4, continue anyway
		_ = response
	}

	// Disable auto-answer
	_, _ = m.sendATLocked(ATS0, m.config.ATCommandTimeout)

	return nil
}

// initModemWithCommands executes a custom list of init commands
func (m *Modem) initModemWithCommands(commands []string) error {
	for i, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}

		m.debugLog("init command %d/%d: %s", i+1, len(commands), cmd)

		response, err := m.sendATLocked(cmd, m.config.ATCommandTimeout)
		if err != nil {
			return fmt.Errorf("init command %q failed: %w", cmd, err)
		}

		// Check for failure responses
		if failed, reason := IsFailureResponse(response); failed {
			// Some commands like ATX4 may fail on certain modems - log but continue
			m.debugLog("init command %q returned %s (continuing)", cmd, reason)
			continue
		}

		// For most commands, we expect OK
		if !IsSuccessResponse(response) {
			m.debugLog("init command %q returned unexpected: %s", cmd, response)
		}
	}

	return nil
}

// Close closes the serial port
func (m *Modem) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.port == nil {
		return nil
	}

	// Try to hangup if we're in data mode
	if m.inDataMode {
		// Just drop DTR - we're closing anyway
		_ = m.port.SetDTR(false)
		time.Sleep(100 * time.Millisecond)
	}

	err := m.port.Close()
	m.port = nil
	m.initialized = false
	m.inDataMode = false

	return err
}

// Reset resets the modem to a known state.
// This can be called in any mode (command or data) and will attempt to
// recover the modem to command mode.
func (m *Modem) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.port == nil {
		return errors.New("modem not open")
	}

	// First, try dropping DTR to force hangup
	if err := m.port.SetDTR(false); err != nil {
		return fmt.Errorf("failed to drop DTR: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	if err := m.port.SetDTR(true); err != nil {
		return fmt.Errorf("failed to raise DTR: %w", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Flush buffers
	_ = m.port.ResetInputBuffer()
	_ = m.port.ResetOutputBuffer()

	// Mark as not in data mode
	m.inDataMode = false

	// Send ATZ to reset modem
	response, err := m.sendATLocked(ATZ, m.config.ATCommandTimeout)
	if err != nil {
		return fmt.Errorf("ATZ failed: %w", err)
	}
	if !IsSuccessResponse(response) {
		return fmt.Errorf("ATZ returned: %s", response)
	}

	// Re-initialize
	return m.initModem()
}

// SendAT sends an AT command and waits for response.
// This should only be called when modem is in command mode.
func (m *Modem) SendAT(cmd string, timeout time.Duration) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.port == nil {
		return "", errors.New("modem not open")
	}
	if m.inDataMode {
		return "", errors.New("modem in data mode, cannot send AT commands")
	}

	return m.sendATLocked(cmd, timeout)
}

// GetLineStats sends the configured line stats command and returns raw response.
func (m *Modem) GetLineStats() (string, error) {
	cmd := strings.TrimSpace(m.config.LineStatsCommand)
	if cmd == "" {
		cmd = ATLineStats
	}

	// Small delay after hangup for modem to stabilize.
	time.Sleep(500 * time.Millisecond)

	resp, err := m.SendAT(cmd, m.config.ATCommandTimeout)
	if err != nil {
		return "", err
	}
	if failed, reason := IsFailureResponse(resp); failed {
		return "", fmt.Errorf("line stats command failed: %s", reason)
	}
	return resp, nil
}

// sendATLocked sends an AT command (caller must hold mutex)
func (m *Modem) sendATLocked(cmd string, timeout time.Duration) (string, error) {
	if m.config.Debug {
		fmt.Fprintf(os.Stderr, "[%s MODEM] sendAT: cmd=%q timeout=%v\n", debugTimestamp(), cmd, timeout)
	}
	// Flush input/output buffers before sending
	_ = m.port.ResetInputBuffer()
	_ = m.port.ResetOutputBuffer()

	m.debugLogStatus("before TX")

	// Write command with CR
	cmdBytes := []byte(cmd + "\r")
	m.debugLogTX(cmdBytes)
	if _, err := m.port.Write(cmdBytes); err != nil {
		return "", fmt.Errorf("write failed: %w", err)
	}

	// Small delay for modem to process and respond
	time.Sleep(100 * time.Millisecond)

	// Read response until we get OK, ERROR, or a terminal result
	return m.readResponseLocked(timeout)
}

// readResponseLocked reads modem response until terminal result or timeout.
// Uses short polling intervals to avoid blocking on slow serial reads.
func (m *Modem) readResponseLocked(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var response []byte
	buf := make([]byte, 64) // Small buffer for quick reads

	// Use short read timeout for polling (50ms for fast responsiveness)
	const defaultPollTimeout = 50 // milliseconds

	// Restore original read timeout when done
	defer func() {
		_ = m.port.SetReadTimeout(int(m.config.ReadTimeout.Milliseconds()))
	}()

	// Set initial poll timeout
	currentTimeout := defaultPollTimeout
	_ = m.port.SetReadTimeout(currentTimeout)

	for time.Now().Before(deadline) {
		// Adjust timeout if remaining time is less than poll timeout
		remaining := time.Until(deadline)
		if remaining < time.Duration(defaultPollTimeout)*time.Millisecond {
			newTimeout := int(remaining.Milliseconds())
			if newTimeout < 10 {
				newTimeout = 10 // minimum 10ms
			}
			if newTimeout != currentTimeout {
				currentTimeout = newTimeout
				_ = m.port.SetReadTimeout(currentTimeout)
			}
		}

		// Read directly from port (bypass bufio to avoid 4KB buffer fill wait)
		n, err := m.port.Read(buf)
		if n > 0 {
			m.debugLogRX(buf[:n])
			response = append(response, buf[:n]...)
			resp := string(response)

			// Check for terminal responses
			if IsSuccessResponse(resp) || IsConnectResponse(resp) {
				m.debugLogStatus("after RX (terminal)")
				return normalizeResponseLineEndings(resp), nil
			}
			if failed, _ := IsFailureResponse(resp); failed {
				m.debugLogStatus("after RX (failure)")
				return normalizeResponseLineEndings(resp), nil
			}
		}

		if err != nil {
			// Check for fatal errors (not just timeout)
			errStr := err.Error()
			if strings.Contains(errStr, "closed") || strings.Contains(errStr, "denied") ||
				strings.Contains(errStr, "no such") || strings.Contains(errStr, "not open") {
				m.debugLogStatus("after RX (fatal error)")
				return "", fmt.Errorf("port error: %w", err)
			}

			// On timeout error, check if we have a complete response
			if len(response) > 0 {
				resp := string(response)
				if IsSuccessResponse(resp) || IsConnectResponse(resp) {
					m.debugLogStatus("after RX (terminal)")
					return normalizeResponseLineEndings(resp), nil
				}
				if failed, _ := IsFailureResponse(resp); failed {
					m.debugLogStatus("after RX (failure)")
					return normalizeResponseLineEndings(resp), nil
				}
			}
			// Timeout with no/incomplete data - continue polling
			continue
		}

		if n == 0 {
			// No data available, brief pause before next poll
			time.Sleep(10 * time.Millisecond)
		}
	}

	m.debugLogStatus("after RX (timeout)")
	if len(response) > 0 {
		return normalizeResponseLineEndings(string(response)), nil
	}
	return "", fmt.Errorf("timeout waiting for response")
}

func normalizeResponseLineEndings(response string) string {
	if !strings.Contains(response, "\r") {
		return response
	}
	response = strings.ReplaceAll(response, "\r\n", "\n")
	response = strings.ReplaceAll(response, "\r", "\n")
	return response
}

// Hangup terminates the current call.
// Uses either DTR drop or escape sequence method based on configuration.
func (m *Modem) Hangup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.port == nil {
		return errors.New("modem not open")
	}

	if !m.inDataMode {
		// Already in command mode, nothing to do
		return nil
	}

	var err error
	if m.config.HangupMethod == "escape" {
		err = m.hangupEscapeLocked()
	} else {
		err = m.hangupDTRLocked()
	}

	if err == nil {
		m.inDataMode = false
	}

	return err
}

// hangupDTRLocked hangs up by dropping DTR (caller must hold mutex)
func (m *Modem) hangupDTRLocked() error {
	// Drop DTR
	if err := m.port.SetDTR(false); err != nil {
		return fmt.Errorf("failed to drop DTR: %w", err)
	}

	// Wait for modem to detect DTR drop
	time.Sleep(500 * time.Millisecond)

	// Raise DTR
	if err := m.port.SetDTR(true); err != nil {
		return fmt.Errorf("failed to raise DTR: %w", err)
	}

	// Wait for modem to stabilize
	time.Sleep(100 * time.Millisecond)

	// Flush buffers (modem may emit garbage during hangup)
	_ = m.port.ResetInputBuffer()
	_ = m.port.ResetOutputBuffer()

	return nil
}

// hangupEscapeLocked hangs up using escape sequence (caller must hold mutex)
func (m *Modem) hangupEscapeLocked() error {
	// Guard time before escape sequence
	time.Sleep(time.Duration(EscapeGuardTime) * time.Millisecond)

	// Send escape sequence
	if _, err := m.port.Write([]byte(EscapeSequence)); err != nil {
		return fmt.Errorf("failed to send escape sequence: %w", err)
	}

	// Guard time after escape sequence
	time.Sleep(time.Duration(EscapeGuardTime) * time.Millisecond)

	// Wait for OK
	response, err := m.readResponseLocked(2 * time.Second)
	if err != nil {
		return fmt.Errorf("no response to escape sequence: %w", err)
	}
	if !IsSuccessResponse(response) {
		return fmt.Errorf("escape sequence returned: %s", response)
	}

	// Send hangup command
	if _, err := m.port.Write([]byte(ATH + "\r")); err != nil {
		return fmt.Errorf("failed to send ATH: %w", err)
	}

	// Wait for OK
	response, err = m.readResponseLocked(2 * time.Second)
	if err != nil {
		return fmt.Errorf("ATH failed: %w", err)
	}

	// Flush buffers
	_ = m.port.ResetInputBuffer()

	return nil
}

// GetStatus returns current modem control line status
func (m *Modem) GetStatus() (*ModemStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getStatusLocked()
}

// getStatusLocked returns modem status (caller must hold mutex)
func (m *Modem) getStatusLocked() (*ModemStatus, error) {
	if m.port == nil {
		return nil, errors.New("modem not open")
	}

	bits, err := m.port.GetModemStatusBits()
	if err != nil {
		return nil, fmt.Errorf("failed to get modem status: %w", err)
	}

	return &ModemStatus{
		DCD: bits.DCD,
		DSR: bits.DSR,
		CTS: bits.CTS,
		RI:  bits.RI,
	}, nil
}

// IsReady returns true if modem is initialized and in command mode
func (m *Modem) IsReady() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.port != nil && m.initialized && !m.inDataMode
}

// InDataMode returns true if modem is in data mode (after CONNECT)
func (m *Modem) InDataMode() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inDataMode
}

// GetInfo returns modem identification info
func (m *Modem) GetInfo() (*ModemInfo, error) {
	response, err := m.SendAT(ATI, m.config.ATCommandTimeout)
	if err != nil {
		return nil, err
	}
	return ParseModemInfo(response), nil
}

// Port returns the underlying serial port for advanced operations.
// Use with caution - direct port access bypasses modem state tracking.
func (m *Modem) Port() *serial.Port {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.port
}

// setInDataMode is used internally to mark modem as in data mode
func (m *Modem) setInDataMode(inData bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inDataMode = inData
}

// FlushBuffers flushes serial port input and output buffers
func (m *Modem) FlushBuffers() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.port == nil {
		return errors.New("modem not open")
	}

	if err := m.port.ResetInputBuffer(); err != nil {
		return fmt.Errorf("failed to flush input buffer: %w", err)
	}
	if err := m.port.ResetOutputBuffer(); err != nil {
		return fmt.Errorf("failed to flush output buffer: %w", err)
	}

	return nil
}

// DrainPendingResponse reads and discards any pending modem output.
// Waits up to timeout for data, returns what was received.
// Useful after hangup to consume NO CARRIER before sending new commands.
func (m *Modem) DrainPendingResponse(timeout time.Duration) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.port == nil {
		return ""
	}

	var response []byte
	buf := make([]byte, 256)
	deadline := time.Now().Add(timeout)
	silenceStart := time.Now()
	const silenceTimeout = 200 * time.Millisecond // Stop after 200ms of silence

	_ = m.port.SetReadTimeout(50) // 50ms read chunks

	for time.Now().Before(deadline) {
		n, _ := m.port.Read(buf)
		if n > 0 {
			if m.config.Debug {
				m.debugLogRX(buf[:n])
			}
			response = append(response, buf[:n]...)
			silenceStart = time.Now() // Reset silence timer

			// Check for terminal responses
			resp := string(response)
			if strings.Contains(resp, "NO CARRIER") ||
				strings.Contains(resp, "OK") ||
				strings.Contains(resp, "ERROR") {
				break
			}
		} else {
			// No data - check silence timeout
			if time.Since(silenceStart) > silenceTimeout {
				break
			}
		}
	}

	// Restore default read timeout
	_ = m.port.SetReadTimeout(int(m.config.ReadTimeout.Milliseconds()))

	return string(response)
}
