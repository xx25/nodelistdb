package binkp

import (
	"fmt"
	"io"
	"github.com/nodelistdb/internal/testing/logging"
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
	conn         net.Conn
	localAddress string // Our FidoNet address
	systemName   string // Our system name (SYS)
	sysop        string // Our sysop name (ZYZ)
	location     string // Our location (LOC)
	remoteInfo   NodeInfo
	timeout      time.Duration
	debug        bool
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
				logging.Debugf("BinkP: Remote sent M_OK")
			}
			// Don't send anything in response - just note it
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
			if s.debug {
				logging.Debugf("BinkP: Remote sent M_EOB (no files)")
			}
			// DON'T send M_EOB in response here - the remote is already done
			// Just acknowledge we received it
			return nil
			
		default:
			// Unknown or file transfer frame - skip for testing
			if s.debug {
				logging.Debugf("BinkP: Ignoring frame type 0x%02X", frame.Type)
			}
		}
		
		// If we have received address and basic info, we have enough info for testing
		if receivedADR && s.remoteInfo.SystemName != "" {
			// Don't send M_OK here - it confuses the remote
			// Just return to indicate we have the info we need
			return nil
		}
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

// Close closes the session gracefully
func (s *Session) Close() error {
	if s.conn != nil {
		// Send M_EOB to indicate we're done
		if err := WriteFrame(s.conn, &Frame{
			Type:    M_EOB,
			Command: true,
			Data:    nil,
		}); err != nil {
			if s.debug {
				logging.Debugf("BinkP: Failed to send M_EOB: %v", err)
			}
		} else if s.debug {
			logging.Debugf("BinkP: Sent M_EOB")
		}
		
		// Wait briefly for remote's M_EOB (best effort)
		_ = s.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		frame, err := ReadFrame(s.conn)
		if err == nil && frame.Type == M_EOB {
			if s.debug {
				logging.Debugf("BinkP: Received remote M_EOB")
			}
		}
		
		return s.conn.Close()
	}
	return nil
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
	
	return addr
}