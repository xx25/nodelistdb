package modem

import (
	"net"
	"time"
)

// ModemConn wraps a modem's serial port as net.Conn for protocol compatibility.
//
// LIFECYCLE CONTRACT:
//   - ModemConn is a thin wrapper that does NOT own the connection
//   - Close() is a NO-OP - it does not hang up or close the serial port
//   - Caller MUST call Modem.Hangup() separately after using ModemConn
//   - This allows the caller to handle hangup errors and perform recovery if needed
type ModemConn struct {
	modem         *Modem
	readDeadline  time.Time
	writeDeadline time.Time
}

// Ensure ModemConn implements net.Conn
var _ net.Conn = (*ModemConn)(nil)

// GetConn returns a net.Conn-compatible wrapper for the modem's serial port.
// The modem must be in data mode (after successful Dial).
//
// IMPORTANT: The returned conn.Close() is a NO-OP.
// Caller must call Modem.Hangup() separately to terminate the call.
func (m *Modem) GetConn() *ModemConn {
	return &ModemConn{
		modem: m,
	}
}

// Read reads data from the modem serial port.
// Uses short polling intervals (50ms) to return data promptly when available,
// rather than blocking for the full read deadline. This matches the proven
// pattern used by readResponseLocked() in modem.go.
func (c *ModemConn) Read(b []byte) (n int, err error) {
	if c.modem.port == nil {
		return 0, net.ErrClosed
	}

	// No deadline set - single read with default timeout
	if c.readDeadline.IsZero() {
		return c.modem.port.Read(b)
	}

	// Poll with short timeout until data arrives or deadline expires
	const pollTimeoutMs = 50 // milliseconds - same as readResponseLocked

	for {
		remaining := time.Until(c.readDeadline)
		if remaining <= 0 {
			return 0, &net.OpError{Op: "read", Net: "serial", Err: timeoutError{}}
		}

		// Use the shorter of poll timeout or remaining time
		timeout := pollTimeoutMs
		if remainMs := int(remaining.Milliseconds()); remainMs < timeout {
			timeout = remainMs
			if timeout < 10 {
				timeout = 10 // minimum 10ms
			}
		}
		_ = c.modem.port.SetReadTimeout(timeout)

		n, err = c.modem.port.Read(b)
		if n > 0 || err != nil {
			return n, err
		}
		// No data yet, continue polling until deadline
	}
}

// Write writes data to the modem serial port
func (c *ModemConn) Write(b []byte) (n int, err error) {
	if c.modem.port == nil {
		return 0, net.ErrClosed
	}

	// Note: go-serial doesn't have per-call write deadline,
	// but writes to serial ports are typically non-blocking
	n, err = c.modem.port.Write(b)
	return n, err
}

// Close is a NO-OP by design.
// Caller must call Modem.Hangup() to terminate the call.
// This design allows the caller to handle hangup errors and perform recovery.
func (c *ModemConn) Close() error {
	// NO-OP by design - see contract in type documentation
	return nil
}

// LocalAddr returns a placeholder address for the local side
func (c *ModemConn) LocalAddr() net.Addr {
	return &modemAddr{device: c.modem.config.Device}
}

// RemoteAddr returns a placeholder address for the remote side
func (c *ModemConn) RemoteAddr() net.Addr {
	return &modemAddr{device: "remote"}
}

// SetDeadline sets both read and write deadlines
func (c *ModemConn) SetDeadline(t time.Time) error {
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

// SetReadDeadline sets the read deadline
func (c *ModemConn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

// SetWriteDeadline sets the write deadline
func (c *ModemConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}

// modemAddr implements net.Addr for modem connections
type modemAddr struct {
	device string
}

func (a *modemAddr) Network() string {
	return "serial"
}

func (a *modemAddr) String() string {
	return a.device
}

// timeoutError implements the net.Error interface for timeout errors
type timeoutError struct{}

func (e timeoutError) Error() string   { return "i/o timeout" }
func (e timeoutError) Timeout() bool   { return true }
func (e timeoutError) Temporary() bool { return true }
