package modem

import (
	"bufio"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mfkenney/go-serial/v2"
)

// Config contains modem configuration
type Config struct {
	Device       string        // Serial port device (e.g., /dev/ttyUSB0, /dev/ttyACM0)
	BaudRate     int           // Serial port baud rate (e.g., 115200)
	InitString   string        // Modem init command (e.g., "ATZ" or "AT&F")
	DialPrefix   string        // Dial command prefix (e.g., "ATDT" or "ATD")
	HangupMethod string        // "dtr" (drop DTR) or "escape" (+++ ATH)

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
	reader *bufio.Reader

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
	m.reader = bufio.NewReader(port)

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
	m.reader = nil
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

// sendATLocked sends an AT command (caller must hold mutex)
func (m *Modem) sendATLocked(cmd string, timeout time.Duration) (string, error) {
	// Flush input buffer before sending AND recreate bufio.Reader
	// to discard any stale buffered data
	_ = m.port.ResetInputBuffer()
	_ = m.port.ResetOutputBuffer()
	m.reader = bufio.NewReader(m.port)

	// Write command with CR
	cmdBytes := []byte(cmd + "\r")
	if _, err := m.port.Write(cmdBytes); err != nil {
		return "", fmt.Errorf("write failed: %w", err)
	}

	// Small delay for modem to process and respond
	time.Sleep(100 * time.Millisecond)

	// Read response until we get OK, ERROR, or a terminal result
	return m.readResponseLocked(timeout)
}

// readResponseLocked reads modem response until terminal result or timeout
func (m *Modem) readResponseLocked(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var response []byte
	buf := make([]byte, 256)

	for time.Now().Before(deadline) {
		// Set read deadline
		remaining := time.Until(deadline)
		if remaining < 100*time.Millisecond {
			remaining = 100 * time.Millisecond
		}
		_ = m.port.SetReadTimeout(int(remaining.Milliseconds()))

		// Read raw bytes (modems may not terminate with '\n' promptly)
		n, err := m.reader.Read(buf)
		if n > 0 {
			response = append(response, buf[:n]...)
			resp := string(response)

			// Check for terminal responses
			if IsSuccessResponse(resp) || IsConnectResponse(resp) {
				return normalizeResponseLineEndings(resp), nil
			}
			if failed, _ := IsFailureResponse(resp); failed {
				return normalizeResponseLineEndings(resp), nil
			}
		}

		if err != nil {
			if len(response) > 0 {
				resp := string(response)
				if IsSuccessResponse(resp) || IsConnectResponse(resp) {
					return normalizeResponseLineEndings(resp), nil
				}
				if failed, _ := IsFailureResponse(resp); failed {
					return normalizeResponseLineEndings(resp), nil
				}
			}
			// Continue trying until deadline
			time.Sleep(50 * time.Millisecond)
			continue
		}

		if n == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}

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

	// Recreate reader after buffer flush
	m.reader = bufio.NewReader(m.port)

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
	m.reader = bufio.NewReader(m.port)

	return nil
}

// GetStatus returns current modem control line status
func (m *Modem) GetStatus() (*ModemStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

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

// FlushBuffers flushes serial port buffers and recreates reader
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

	m.reader = bufio.NewReader(m.port)
	return nil
}
