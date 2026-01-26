package modem

import (
	"errors"
	"fmt"
	"io"
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
	DebugWriter  io.Writer     // Output writer for debug logs (default: os.Stderr)
	Name         string        // Optional name for identifying modem in debug logs

	// Timeouts
	DialTimeout      time.Duration // Timeout for dial (waiting for CONNECT/error), default 200s
	CarrierTimeout   time.Duration // Timeout for DCD after CONNECT, default 5s
	ATCommandTimeout time.Duration // Timeout for AT command responses
	ReadTimeout      time.Duration // Default read timeout for serial port

	// DTR Hangup timing
	DTRHoldTime      time.Duration // How long to hold DTR low initially, default 500ms
	DTRWaitInterval  time.Duration // Interval between DCD checks while waiting, default 150ms
	DTRMaxWaitTime   time.Duration // Max time to wait for DCD drop after initial hold, default 1500ms
	DTRStabilizeTime time.Duration // Time to wait after raising DTR, default 200ms
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
		// DTR hangup timing defaults
		DTRHoldTime:      500 * time.Millisecond,
		DTRWaitInterval:  150 * time.Millisecond,
		DTRMaxWaitTime:   1500 * time.Millisecond,
		DTRStabilizeTime: 200 * time.Millisecond,
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

	// USB device info (for USB reset capability)
	usbVendor  string
	usbProduct string
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
	if cfg.DebugWriter == nil {
		cfg.DebugWriter = os.Stderr
	}

	return &Modem{
		config: cfg,
	}, nil
}

// debugTimestamp returns current time formatted for debug output
func debugTimestamp() string {
	return time.Now().Format("15:04:05.000")
}

// debugPrefix returns the modem identifier for log messages (name or device)
func (m *Modem) debugPrefix() string {
	if m.config.Name != "" {
		return m.config.Name
	}
	return m.config.Device
}

// debugLog prints debug message if debug mode is enabled
func (m *Modem) debugLog(format string, args ...interface{}) {
	if m.config.Debug {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(m.config.DebugWriter, "[%s] [%s] MODEM  %s\n", debugTimestamp(), m.debugPrefix(), msg)
	}
}

// debugLogTX logs data being sent to modem
func (m *Modem) debugLogTX(data []byte) {
	if m.config.Debug {
		fmt.Fprintf(m.config.DebugWriter, "[%s] [%s] TX     %q\n", debugTimestamp(), m.debugPrefix(), string(data))
	}
}

// debugLogRX logs data received from modem
func (m *Modem) debugLogRX(data []byte) {
	if m.config.Debug {
		fmt.Fprintf(m.config.DebugWriter, "[%s] [%s] RX     %q\n", debugTimestamp(), m.debugPrefix(), string(data))
	}
}

// debugLogStatus logs RS232 control line status
func (m *Modem) debugLogStatus(label string) {
	if !m.config.Debug {
		return
	}
	status, err := m.getStatusLocked()
	if err != nil {
		fmt.Fprintf(m.config.DebugWriter, "[%s] [%s] RS232  %s: error: %v\n", debugTimestamp(), m.debugPrefix(), label, err)
		return
	}
	fmt.Fprintf(m.config.DebugWriter, "[%s] [%s] RS232  %s: DCD=%s DSR=%s CTS=%s RI=%s\n",
		debugTimestamp(), m.debugPrefix(), label,
		boolTo01(status.DCD), boolTo01(status.DSR), boolTo01(status.CTS), boolTo01(status.RI))
}

// boolTo01 converts bool to "1" or "0" for compact display
func boolTo01(b bool) string {
	if b {
		return "1"
	}
	return "0"
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

	// Discover USB device IDs (for USB reset capability)
	// This is non-fatal - device may not be USB-based
	if vendor, product, err := GetUSBDeviceID(m.config.Device); err == nil {
		m.usbVendor = vendor
		m.usbProduct = product
		m.debugLog("USB device detected: %s:%s", vendor, product)
	}

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
	_, err = m.sendATLocked(ATV1, m.config.ATCommandTimeout)
	if err != nil {
		return fmt.Errorf("ATV1 failed: %w", err)
	}

	// Enable extended result codes (BUSY, NO DIALTONE, etc.)
	// Not all modems support X4, ignore errors
	_, _ = m.sendATLocked(ATX4, m.config.ATCommandTimeout)

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

// USBReset performs a hardware-level USB reset of the modem.
// This is a last-resort recovery when software reset fails.
// Requires sudo permissions for usbreset command.
func (m *Modem) USBReset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.usbVendor == "" || m.usbProduct == "" {
		return errors.New("not a USB device or USB IDs unknown")
	}

	m.debugLog("Attempting USB reset for %s:%s", m.usbVendor, m.usbProduct)

	// Close serial port first
	if m.port != nil {
		_ = m.port.Close()
		m.port = nil
	}
	m.initialized = false
	m.inDataMode = false

	// Reset USB device
	if err := ResetUSBDevice(m.usbVendor, m.usbProduct); err != nil {
		return fmt.Errorf("USB reset failed: %w", err)
	}

	// Wait for device to reappear
	if err := WaitForDevice(m.config.Device, 10*time.Second); err != nil {
		return fmt.Errorf("device did not reappear after USB reset: %w", err)
	}

	m.debugLog("USB reset complete, reopening modem")

	// Reopen the modem
	return m.openLocked()
}

// openLocked opens the serial port (caller must hold mutex)
func (m *Modem) openLocked() error {
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
		serial.WithHUPCL(true),
	)
	if err != nil {
		return fmt.Errorf("failed to open serial port %s: %w", m.config.Device, err)
	}

	m.port = port

	// Re-discover USB device IDs (may have changed after reset)
	if vendor, product, err := GetUSBDeviceID(m.config.Device); err == nil {
		m.usbVendor = vendor
		m.usbProduct = product
	}

	// Ensure DTR is high
	if err := port.SetDTR(true); err != nil {
		port.Close()
		m.port = nil
		return fmt.Errorf("failed to set DTR: %w", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Initialize modem
	if err := m.initModem(); err != nil {
		port.Close()
		m.port = nil
		return fmt.Errorf("failed to initialize modem: %w", err)
	}

	m.initialized = true
	m.inDataMode = false

	return nil
}

// IsUSBDevice returns true if this modem is connected via USB
func (m *Modem) IsUSBDevice() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.usbVendor != "" && m.usbProduct != ""
}

// GetUSBID returns the USB vendor:product ID string, or empty if not USB
func (m *Modem) GetUSBID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.usbVendor == "" || m.usbProduct == "" {
		return ""
	}
	return m.usbVendor + ":" + m.usbProduct
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
		fmt.Fprintf(m.config.DebugWriter, "[%s] [%s] MODEM  sendAT: cmd=%q timeout=%v\n", debugTimestamp(), m.debugPrefix(), cmd, timeout)
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
	// Get timing config with defaults
	holdTime := m.config.DTRHoldTime
	if holdTime == 0 {
		holdTime = 500 * time.Millisecond
	}
	waitInterval := m.config.DTRWaitInterval
	if waitInterval == 0 {
		waitInterval = 150 * time.Millisecond
	}
	maxWaitTime := m.config.DTRMaxWaitTime
	if maxWaitTime == 0 {
		maxWaitTime = 1500 * time.Millisecond
	}
	stabilizeTime := m.config.DTRStabilizeTime
	if stabilizeTime == 0 {
		stabilizeTime = 200 * time.Millisecond
	}

	// Drop DTR
	if err := m.port.SetDTR(false); err != nil {
		return fmt.Errorf("failed to drop DTR: %w", err)
	}

	// Hold DTR low for configured time to ensure modem recognizes the drop
	// (S25 register typically controls DTR recognition time, default ~50ms,
	// but we need extra time for the modem to actually disconnect)
	time.Sleep(holdTime)

	// Check if DCD dropped
	dcdDropped := false
	status, err := m.getStatusLocked()
	if err == nil && !status.DCD {
		dcdDropped = true
	}

	// If DCD still high, wait longer (up to maxWaitTime)
	if !dcdDropped {
		iterations := int(maxWaitTime / waitInterval)
		if iterations < 1 {
			iterations = 1
		}
		for i := 0; i < iterations; i++ {
			time.Sleep(waitInterval)
			status, err = m.getStatusLocked()
			if err == nil && !status.DCD {
				dcdDropped = true
				break
			}
		}
	}

	// Now raise DTR (after confirming disconnect or timeout)
	if err := m.port.SetDTR(true); err != nil {
		return fmt.Errorf("failed to raise DTR: %w", err)
	}

	// Wait for modem to stabilize after DTR raised
	time.Sleep(stabilizeTime)

	// If DCD never dropped, try escape sequence as fallback
	if !dcdDropped {
		// Re-check DCD one more time after raising DTR
		status, err = m.getStatusLocked()
		if err == nil && status.DCD {
			if m.config.Debug {
				totalWait := holdTime + maxWaitTime
				fmt.Fprintf(m.config.DebugWriter, "[%s] [%s] MODEM  DTR hangup failed (DCD still high after %v), trying escape sequence\n", debugTimestamp(), m.debugPrefix(), totalWait)
			}
			// DTR method failed, try escape sequence as fallback
			return m.hangupEscapeLocked()
		}
	}

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
	_, err = m.readResponseLocked(2 * time.Second)
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

// SendATWithPagination sends an AT command and handles paginated responses.
// Some modems (e.g., MT5634ZBA with ATI11) show multi-page output and wait
// for a keypress at "Press any key to continue" prompts before showing more.
// This method detects such prompts and sends a space to continue.
func (m *Modem) SendATWithPagination(cmd string, timeout time.Duration) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.port == nil {
		return "", errors.New("modem not open")
	}
	if m.inDataMode {
		return "", errors.New("modem in data mode, cannot send AT commands")
	}

	return m.sendATWithPaginationLocked(cmd, timeout)
}

// sendATWithPaginationLocked sends AT command handling pagination (caller must hold mutex)
func (m *Modem) sendATWithPaginationLocked(cmd string, timeout time.Duration) (string, error) {
	if m.config.Debug {
		fmt.Fprintf(m.config.DebugWriter, "[%s] [%s] MODEM  sendATWithPagination: cmd=%q timeout=%v\n", debugTimestamp(), m.debugPrefix(), cmd, timeout)
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

	// Read response with pagination handling
	return m.readResponseWithPaginationLocked(timeout)
}

// readResponseWithPaginationLocked reads modem response, handling pagination prompts.
func (m *Modem) readResponseWithPaginationLocked(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var response []byte
	buf := make([]byte, 64)
	lastPaginationHandledAt := -1 // Track position of last handled pagination prompt

	const defaultPollTimeout = 50 // milliseconds

	// Restore original read timeout when done
	defer func() {
		_ = m.port.SetReadTimeout(int(m.config.ReadTimeout.Milliseconds()))
	}()

	currentTimeout := defaultPollTimeout
	_ = m.port.SetReadTimeout(currentTimeout)

	for time.Now().Before(deadline) {
		// Adjust timeout if remaining time is less than poll timeout
		remaining := time.Until(deadline)
		if remaining < time.Duration(defaultPollTimeout)*time.Millisecond {
			newTimeout := int(remaining.Milliseconds())
			if newTimeout < 10 {
				newTimeout = 10
			}
			if newTimeout != currentTimeout {
				currentTimeout = newTimeout
				_ = m.port.SetReadTimeout(currentTimeout)
			}
		}

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

			// Check for pagination prompt (only if we haven't already handled one at this position)
			if promptPos := m.findPaginationPrompt(resp); promptPos >= 0 && promptPos > lastPaginationHandledAt {
				m.debugLog("detected pagination prompt, sending space to continue")
				lastPaginationHandledAt = promptPos
				// Send space to continue
				if _, err := m.port.Write([]byte(" ")); err != nil {
					m.debugLog("failed to send pagination continue: %v", err)
				}
				// Small delay for modem to process
				time.Sleep(50 * time.Millisecond)
			}
		}

		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "closed") || strings.Contains(errStr, "denied") ||
				strings.Contains(errStr, "no such") || strings.Contains(errStr, "not open") {
				m.debugLogStatus("after RX (fatal error)")
				return "", fmt.Errorf("port error: %w", err)
			}

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
			continue
		}

		if n == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	m.debugLogStatus("after RX (timeout)")
	if len(response) > 0 {
		return normalizeResponseLineEndings(string(response)), nil
	}
	return "", fmt.Errorf("timeout waiting for response")
}

// findPaginationPrompt returns the position of a pagination prompt in response, or -1 if not found.
func (m *Modem) findPaginationPrompt(response string) int {
	lower := strings.ToLower(response)
	// Multi-Tech style: "Press any key to continue; ESC to quit."
	if pos := strings.Index(lower, "press any key to continue"); pos >= 0 {
		return pos
	}
	// Some modems use "-- More --" style prompts
	if pos := strings.Index(lower, "-- more --"); pos >= 0 {
		return pos
	}
	return -1
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
