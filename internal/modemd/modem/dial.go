package modem

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// DialResult contains the outcome of a dial attempt
type DialResult struct {
	Success       bool          // True if CONNECT was received
	ConnectSpeed  int           // Connection speed (e.g., 33600)
	ConnectString string        // Raw CONNECT string from modem (e.g., "CONNECT 33600/ARQ/V34/LAPM")
	Protocol      string        // Protocol info (e.g., "V34/LAPM/V42BIS")
	Error         string        // Error message if failed (e.g., "BUSY", "NO CARRIER")
	RingCount     int           // Number of RINGs detected before answer
	DialTime      time.Duration // Time from dial to CONNECT/failure
	CarrierTime   time.Duration // Time from CONNECT to stable DCD
}

// DialNumber dials a phone number using config timeouts.
// Returns a non-nil DialResult even on error, containing timing and error info.
func (m *Modem) DialNumber(phone string) (*DialResult, error) {
	return m.Dial(phone, m.config.DialTimeout, m.config.CarrierTimeout)
}

// Dial dials a phone number and waits for connection.
// Returns a non-nil DialResult even on error, containing timing and error info.
//
// The dialTimeout is how long to wait for CONNECT/BUSY/NO CARRIER response.
// The carrierTimeout is how long to wait for DCD to stabilize after CONNECT.
func (m *Modem) Dial(phone string, dialTimeout, carrierTimeout time.Duration) (*DialResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	startTime := time.Now()
	result := &DialResult{} // Always non-nil per contract

	// Validate modem state
	if m.port == nil {
		result.Error = "modem not open"
		result.DialTime = time.Since(startTime)
		return result, errors.New("modem not open")
	}

	if m.inDataMode {
		result.Error = "modem in data mode"
		result.DialTime = time.Since(startTime)
		return result, errors.New("modem in data mode, call Hangup() first")
	}

	// Format phone number
	phone = FormatPhoneNumber(phone)
	if phone == "" {
		result.Error = "empty phone number"
		result.DialTime = time.Since(startTime)
		return result, errors.New("empty phone number after formatting")
	}

	// Flush buffers before dialing
	_ = m.port.ResetInputBuffer()

	// Build dial command
	dialCmd := m.config.DialPrefix + phone + "\r"

	// Send dial command
	if _, err := m.port.Write([]byte(dialCmd)); err != nil {
		result.Error = "write error"
		result.DialTime = time.Since(startTime)
		return result, fmt.Errorf("failed to send dial command: %w", err)
	}

	// Wait for response
	response, err := m.readDialResponseLocked(dialTimeout)
	result.DialTime = time.Since(startTime)

	if err != nil {
		result.Error = "timeout"
		return result, fmt.Errorf("dial timeout: %w", err)
	}

	// Count rings in response
	result.RingCount = strings.Count(response, ResponseRing)

	// Check for CONNECT
	if IsConnectResponse(response) {
		// Extract the CONNECT line
		result.ConnectString = extractConnectLine(response)

		// Parse speed and protocol
		speed, protocol, _ := ParseConnectSpeed(response)
		result.ConnectSpeed = speed
		result.Protocol = protocol

		// Flush any remaining data after CONNECT line
		_ = m.port.ResetInputBuffer()

		// Wait for DCD to stabilize
		connectTime := time.Now()
		if err := m.waitForDCDLocked(carrierTimeout); err != nil {
			result.Error = "no carrier detect"
			// Note: we got CONNECT but DCD didn't come up
			return result, fmt.Errorf("carrier detect timeout: %w", err)
		}
		result.CarrierTime = time.Since(connectTime)

		// Mark modem as in data mode
		m.inDataMode = true
		result.Success = true
		return result, nil
	}

	// Check for failure responses
	if failed, reason := IsFailureResponse(response); failed {
		result.Error = reason
		return result, nil // Not a Go error, just unsuccessful dial
	}

	// Unknown response
	result.Error = "unknown response: " + strings.TrimSpace(response)
	return result, nil
}

// readDialResponseLocked reads dial response, handling RING counts
func (m *Modem) readDialResponseLocked(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var response []byte
	buf := make([]byte, 256)

	for time.Now().Before(deadline) {
		// Set read timeout - use shorter intervals to be responsive
		remaining := time.Until(deadline)
		if remaining < 100*time.Millisecond {
			remaining = 100 * time.Millisecond
		}
		readTimeout := 2000 // 2 second read chunks
		if int(remaining.Milliseconds()) < readTimeout {
			readTimeout = int(remaining.Milliseconds())
		}
		_ = m.port.SetReadTimeout(readTimeout)

		// Read raw bytes
		n, err := m.port.Read(buf)
		if n > 0 {
			response = append(response, buf[:n]...)
			resp := string(response)

			// Check for terminal responses
			if IsConnectResponse(resp) {
				return resp, nil
			}
			if failed, _ := IsFailureResponse(resp); failed {
				return resp, nil
			}
			// RING means still ringing - continue waiting
		}

		if err != nil {
			// Check accumulated response
			if len(response) > 0 {
				resp := string(response)
				if IsConnectResponse(resp) {
					return resp, nil
				}
				if failed, _ := IsFailureResponse(resp); failed {
					return resp, nil
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	if len(response) > 0 {
		return string(response), nil
	}
	return "", errors.New("timeout waiting for dial response")
}

// extractConnectLine extracts the CONNECT line from modem response
func extractConnectLine(response string) string {
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "CONNECT") {
			return line
		}
	}
	// Try with \r as line separator
	lines = strings.Split(response, "\r")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "CONNECT") {
			return line
		}
	}
	return ""
}

// waitForDCDLocked waits for DCD (Data Carrier Detect) to go high
func (m *Modem) waitForDCDLocked(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, err := m.port.GetModemStatusBits()
		if err != nil {
			return fmt.Errorf("failed to read modem status: %w", err)
		}

		if status.DCD {
			return nil // Carrier detected
		}

		time.Sleep(50 * time.Millisecond)
	}

	return errors.New("DCD timeout")
}

// DialWithRetry dials with automatic retry on busy/no answer
func (m *Modem) DialWithRetry(phone string, dialTimeout, carrierTimeout time.Duration, maxRetries int, retryDelay time.Duration) (*DialResult, error) {
	var lastResult *DialResult
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		result, err := m.Dial(phone, dialTimeout, carrierTimeout)
		lastResult = result
		lastErr = err

		if err != nil {
			// Go-level error (I/O, timeout, etc.) - might want to retry
			continue
		}

		if result.Success {
			return result, nil
		}

		// Check if error is retryable
		switch result.Error {
		case ResponseBusy, ResponseNoAnswer:
			// Retryable
			continue
		case ResponseNoCarrier, ResponseNoDialtone, ResponseError:
			// Not retryable
			return result, nil
		default:
			continue
		}
	}

	return lastResult, lastErr
}
