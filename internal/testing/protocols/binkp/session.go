package binkp

import (
	"fmt"
	"github.com/nodelistdb/internal/testing/logging"
	"io"
	"net"
	"strings"
	"time"
)

// NodeInfo contains information about a remote BinkP node
type NodeInfo struct {
	SystemName   string   // SYS - System name
	Sysop        string   // ZYZ - Sysop name
	Location     string   // LOC - Location
	Phone        string   // PHN - Phone number
	Flags        string   // FLG - Node flags
	Version      string   // VER - Software version
	Time         string   // TIME - Node time
	Addresses    []string // From M_ADR frame
	Capabilities []string // OPT - Protocol capabilities
	NDL          string   // NDL - Network data
	Password     string   // From M_PWD frame (if any)
}

// Session represents a BinkP session
type Session struct {
	conn           net.Conn
	localAddress   string // Our FidoNet address
	systemName     string // Our system name (SYS)
	sysop          string // Our sysop name (ZYZ)
	location       string // Our location (LOC)
	remoteInfo     NodeInfo
	timeout        time.Duration
	debug          bool
	localEOBSent   bool      // Track if we sent M_EOB
	remoteEOBRecvd bool      // Track if we received M_EOB from remote
	closeBy        time.Time // wall-clock bound on the whole close-out, once endSession runs
}

// writeDeadline is the session timeout, cut to the close-out bound when one
// is in force, so no write can outlive the budget endSession set.
func (s *Session) writeDeadline() time.Time {
	d := time.Now().Add(s.timeout)
	if !s.closeBy.IsZero() && s.closeBy.Before(d) {
		return s.closeBy
	}
	return d
}

// NewSession creates a new BinkP session
func NewSession(conn net.Conn, localAddress string) *Session {
	return &Session{
		conn:         conn,
		localAddress: localAddress,
		systemName:   "NodelistDB Test Daemon",
		sysop:        "Test Operator",
		location:     "Test Location",
		timeout:      30 * time.Second,
		debug:        false,
	}
}

// NewSessionWithInfo creates a new BinkP session with custom system info
func NewSessionWithInfo(conn net.Conn, localAddress, systemName, sysop, location string) *Session {
	return &Session{
		conn:         conn,
		localAddress: localAddress,
		systemName:   systemName,
		sysop:        sysop,
		location:     location,
		timeout:      30 * time.Second,
		debug:        false,
	}
}

// SetDebug enables debug logging
func (s *Session) SetDebug(debug bool) {
	s.debug = debug
}

// SetTimeout sets the timeout for operations
func (s *Session) SetTimeout(timeout time.Duration) {
	s.timeout = timeout
}

// Handshake performs the BinkP handshake
func (s *Session) Handshake() error {
	// Set write deadline for outgoing frames
	_ = s.conn.SetWriteDeadline(time.Now().Add(s.timeout))

	// Send our M_NUL frames first
	if err := s.sendOurInfo(); err != nil {
		return fmt.Errorf("failed to send our info: %w", err)
	}

	// Send our address
	if err := s.sendOurAddress(); err != nil {
		return fmt.Errorf("failed to send our address: %w", err)
	}

	// Send password (we send "-" for no password in testing)
	if err := s.sendPassword("-"); err != nil {
		return fmt.Errorf("failed to send password: %w", err)
	}

	// Receive remote frames
	if err := s.receiveRemoteInfo(); err != nil {
		return fmt.Errorf("failed to receive remote info: %w", err)
	}

	return nil
}

// sendOurInfo sends our M_NUL frames
func (s *Session) sendOurInfo() error {
	// System info
	frames := []*Frame{
		CreateM_NUL("SYS", s.systemName),
		CreateM_NUL("ZYZ", s.sysop),
		CreateM_NUL("LOC", s.location),
		CreateM_NUL("VER", "NodelistDB/1.0 binkp/1.1"),
		CreateM_NUL("TIME", time.Now().Format(time.RFC822)),
		// Don't advertise CRAM-MD5 since we're not implementing it for testing
		// CreateM_NUL("OPT", "CRAM-MD5"),
	}

	for _, frame := range frames {
		if s.debug {
			logging.Debugf("BinkP: Sending %s", frame)
		}
		if err := WriteFrame(s.conn, frame); err != nil {
			return err
		}
	}

	return nil
}

// sendOurAddress sends our M_ADR frame
func (s *Session) sendOurAddress() error {
	frame := CreateM_ADR(s.localAddress)
	if s.debug {
		logging.Debugf("BinkP: Sending M_ADR %s", s.localAddress)
	}
	return WriteFrame(s.conn, frame)
}

// sendPassword sends M_PWD frame
func (s *Session) sendPassword(password string) error {
	frame := CreateM_PWD(password)
	if s.debug {
		logging.Debugf("BinkP: Sending M_PWD")
	}
	return WriteFrame(s.conn, frame)
}

// receiveRemoteInfo receives and parses remote node information
func (s *Session) receiveRemoteInfo() error {
	// Set overall timeout for receiving all frames
	_ = s.conn.SetReadDeadline(time.Now().Add(s.timeout))

	receivedADR := false
	frameCount := 0
	maxFrames := 50 // Prevent infinite loop

	for frameCount < maxFrames {
		frame, err := ReadFrame(s.conn)
		if err != nil {
			if err == io.EOF && receivedADR {
				// Connection closed after we got address - might be OK
				break
			}
			return fmt.Errorf("failed to read frame: %w", err)
		}

		frameCount++

		if s.debug {
			logging.Debugf("BinkP: Received %s", frame)
		}

		switch frame.Type {
		case M_NUL:
			// Parse M_NUL frame
			key, value := ParseM_NUL(frame.Data)
			if s.debug {
				logging.Debugf("BinkP: M_NUL parsed: key=[%s] value=[%s] (raw=%q)", key, value, frame.Data)
			}
			s.parseM_NUL(key, value)

		case M_ADR:
			// Parse addresses
			s.remoteInfo.Addresses = ParseAddresses(frame.Data)
			receivedADR = true
			if s.debug {
				logging.Debugf("BinkP: Remote addresses: %v", s.remoteInfo.Addresses)
			}

		case M_PWD:
			// Remote sent password
			s.remoteInfo.Password = string(frame.Data)

		case M_OK:
			// Remote accepts our handshake
			if s.debug {
				logging.Debugf("BinkP: Remote sent M_OK - handshake complete")
			}
			// Handshake complete - authentication accepted
			return nil

		case M_ERR:
			// Remote reported error
			errMsg := string(frame.Data)
			return fmt.Errorf("remote error: %s", errMsg)

		case M_BSY:
			// Remote is busy
			return fmt.Errorf("remote is busy")

		case M_EOB:
			// End of batch - remote has no files for us
			s.remoteEOBRecvd = true

			// Require M_ADR before accepting M_EOB as successful handshake
			if !receivedADR {
				return fmt.Errorf("received M_EOB before M_ADR - invalid handshake")
			}

			if s.debug {
				logging.Debugf("BinkP: Remote sent M_EOB - sending our M_EOB in response")
			}
			// Send our M_EOB immediately to complete the exchange
			if !s.localEOBSent {
				if err := s.sendEOB(); err != nil {
					if s.debug {
						logging.Debugf("BinkP: Failed to send M_EOB response: %v", err)
					}
				}
			}
			// Handshake complete - no files to transfer
			return nil

		default:
			// Unknown or file transfer frame - skip for testing
			if s.debug {
				logging.Debugf("BinkP: Ignoring frame type 0x%02X", frame.Type)
			}
		}
	}

	// If we hit maxFrames limit but got valid data, accept it
	// This handles non-compliant implementations
	if receivedADR && s.remoteInfo.SystemName != "" {
		if s.debug {
			logging.Debugf("BinkP: maxFrames limit reached, but have valid data - accepting")
		}
		return nil
	}

	if !receivedADR {
		return fmt.Errorf("no M_ADR received from remote")
	}

	return nil
}

// parseM_NUL parses M_NUL key-value pairs
func (s *Session) parseM_NUL(key, value string) {
	// Convert to uppercase for comparison
	keyUpper := strings.ToUpper(key)

	switch keyUpper {
	case "SYS":
		s.remoteInfo.SystemName = value
	case "ZYZ":
		s.remoteInfo.Sysop = value
	case "LOC":
		s.remoteInfo.Location = value
	case "PHN":
		s.remoteInfo.Phone = value
	case "FLG":
		s.remoteInfo.Flags = value
	case "VER":
		s.remoteInfo.Version = value
	case "TIME":
		s.remoteInfo.Time = value
	case "OPT":
		// Parse capabilities (space-separated)
		s.remoteInfo.Capabilities = strings.Fields(value)
	case "NDL":
		s.remoteInfo.NDL = value
	default:
		// Unknown key - ignore
		if s.debug {
			logging.Debugf("BinkP: Unknown M_NUL key: %s = %s", key, value)
		}
	}
}

// GetNodeInfo returns the collected remote node information
func (s *Session) GetNodeInfo() NodeInfo {
	return s.remoteInfo
}

// sendEOB sends M_EOB once; later calls are no-ops.
func (s *Session) sendEOB() error {
	if s.localEOBSent {
		return nil // Already sent
	}
	return s.writeEOB()
}

// endOfSessionRounds bounds how many empty batches we will close out. A
// binkp/1.1 peer that keeps opening batches after nothing was sent is broken;
// two rounds is what a correct one needs (see endSession).
const endOfSessionRounds = 4

// eobIdleWindow is how long we wait, after both sides have sent M_EOB, for
// the remote to either close the connection or open another batch.
const eobIdleWindow = 2 * time.Second

// endOfSessionFrames caps the frames absorbed while closing out, so a peer
// that chatters without ever sending M_EOB cannot hold a worker here.
const endOfSessionFrames = 200

// endSession runs the end of an empty session the way the remote expects it.
//
// After the handshake nothing is transferred, so both sides send M_EOB and
// the session is over — in binkp/1.0. binkp/1.1 (which we announce) allows
// several batches per session, and a peer decides whether a batch "did
// anything" by its own count of frames: MBSE's mbcico counts its M_NUL TRF,
// its M_EOB and our M_EOB, reaches its threshold of three, and opens a second
// batch by sending M_EOB again. Closing after the first exchange left it
// reading a dead socket ("tty_read: error flag", rc=108) and logging the
// session as failed to its sysop. So: answer every batch the remote opens
// with an M_EOB of our own, and only leave once the remote has closed or has
// said nothing for eobIdleWindow. A file offered to us in the meantime is
// declined with M_SKIP; the remote then carries on to its own M_EOB.
func (s *Session) endSession() {
	localEOB := s.localEOBSent
	remoteEOB := s.remoteEOBRecvd
	// The whole close-out is bounded by frames read and by wall clock, on top
	// of the per-batch round bound; none of the three depends on the peer
	// behaving.
	frames := 0
	deadline := time.Now().Add(s.timeout + time.Duration(endOfSessionRounds)*eobIdleWindow)
	s.closeBy = deadline
	read := func(within time.Duration) (*Frame, error) {
		frames++
		if frames > endOfSessionFrames || time.Now().After(deadline) {
			return nil, fmt.Errorf("end of session budget exhausted")
		}
		if remaining := time.Until(deadline); remaining < within {
			within = remaining
		}
		return s.readFrameWithin(within)
	}
	for round := 0; round < endOfSessionRounds; round++ {
		if !localEOB {
			if err := s.writeEOB(); err != nil {
				return
			}
			localEOB = true
		}
		for !remoteEOB {
			frame, err := read(s.timeout)
			if err != nil {
				return
			}
			switch s.absorbFrame(frame) {
			case M_EOB:
				remoteEOB = true
			case M_ERR, M_BSY:
				return
			}
		}
		// Both sides are in EOB state. A binkp/1.1 peer that considers the
		// batch non-empty opens the next one by sending M_EOB again; a peer
		// that is done closes the connection, which reads as an error here.
		// Anything else it sends in the meantime (M_NUL chatter) is absorbed
		// without counting as a batch.
		reopened := false
		for !reopened {
			frame, err := read(eobIdleWindow)
			if err != nil {
				return
			}
			switch s.absorbFrame(frame) {
			case M_EOB:
				reopened = true
			case M_ERR, M_BSY:
				return
			}
		}
		localEOB, remoteEOB = false, true
	}
}

// absorbFrame handles a frame read after the handshake and returns its type
// for the caller to act on (0 for data frames and anything not a command).
// M_FILE is answered with M_SKIP so that a remote with mail for us finishes
// its batch instead of waiting for a receiver that is not there.
func (s *Session) absorbFrame(frame *Frame) uint8 {
	if !frame.Command {
		return 0
	}
	switch frame.Type {
	case M_EOB:
		s.remoteEOBRecvd = true
		if s.debug {
			logging.Debugf("BinkP: Received remote M_EOB")
		}
	case M_FILE:
		if s.debug {
			logging.Debugf("BinkP: Remote offered %q; declining with M_SKIP", frame.Data)
		}
		_ = s.conn.SetWriteDeadline(s.writeDeadline())
		_ = WriteFrame(s.conn, &Frame{Type: M_SKIP, Command: true, Data: frame.Data})
	case M_NUL:
		key, value := ParseM_NUL(frame.Data)
		s.parseM_NUL(key, value)
	}
	return frame.Type
}

func (s *Session) readFrameWithin(d time.Duration) (*Frame, error) {
	_ = s.conn.SetReadDeadline(time.Now().Add(d))
	return ReadFrame(s.conn)
}

// writeEOB sends an M_EOB unconditionally; sendEOB is the once-only form the
// handshake uses.
func (s *Session) writeEOB() error {
	_ = s.conn.SetWriteDeadline(s.writeDeadline())
	err := WriteFrame(s.conn, &Frame{Type: M_EOB, Command: true})
	if err == nil {
		s.localEOBSent = true
		if s.debug {
			logging.Debugf("BinkP: Sent M_EOB")
		}
	}
	return err
}

// Close ends the session and closes the connection.
//
// The end of session is played out in full first (endSession), then the
// socket is shut down with FIN rather than RST, as MBSE's closetcp() does, so
// the remote never takes a SIGPIPE from us.
func (s *Session) Close() error {
	if s.conn == nil {
		return nil
	}

	s.endSession()

	if tcpConn, ok := s.conn.(*net.TCPConn); ok {
		// First, close the write side (sends TCP FIN)
		if err := tcpConn.CloseWrite(); err != nil {
			if s.debug {
				logging.Debugf("BinkP: CloseWrite failed: %v", err)
			}
		} else {
			// Give remote a moment to process the FIN and send any final data
			// Then drain any remaining data from the connection
			_ = tcpConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			buf := make([]byte, 1024)
			for {
				_, err := tcpConn.Read(buf)
				if err != nil {
					break // EOF or timeout - expected
				}
			}
		}
	}

	return s.conn.Close()
}

// ValidateAddress checks if the remote announced the expected address
func (s *Session) ValidateAddress(expectedAddress string) bool {
	// Normalize addresses for comparison
	expected := normalizeAddress(expectedAddress)

	for _, addr := range s.remoteInfo.Addresses {
		if normalizeAddress(addr) == expected {
			return true
		}
	}

	return false
}

// normalizeAddress normalizes a FidoNet address for comparison
func normalizeAddress(addr string) string {
	// Remove leading/trailing spaces
	addr = strings.TrimSpace(addr)

	// Convert to lowercase
	addr = strings.ToLower(addr)

	// Remove @domain if present (e.g., @fidonet)
	if idx := strings.Index(addr, "@"); idx != -1 {
		addr = addr[:idx]
	}

	// Remove .0 point suffix (point 0 is implicit)
	// e.g., 2:222/0.0 -> 2:222/0
	addr = strings.TrimSuffix(addr, ".0")

	return addr
}
