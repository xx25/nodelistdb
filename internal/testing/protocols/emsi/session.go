package emsi

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

// Session represents an EMSI session
type Session struct {
	conn         net.Conn
	reader       *bufio.Reader
	writer       *bufio.Writer
	localAddress string
	remoteInfo   *EMSIData
	timeout      time.Duration
	debug        bool
}

// NewSession creates a new EMSI session
func NewSession(conn net.Conn, localAddress string) *Session {
	return &Session{
		conn:         conn,
		reader:       bufio.NewReader(conn),
		writer:       bufio.NewWriter(conn),
		localAddress: localAddress,
		timeout:      30 * time.Second,
		debug:        os.Getenv("DEBUG_EMSI") != "",
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

// Handshake performs the EMSI handshake as caller (we initiate)
func (s *Session) Handshake() error {
	// Send EMSI_INQ to start handshake
	if err := s.sendEMSI_INQ(); err != nil {
		return fmt.Errorf("failed to send EMSI_INQ: %w", err)
	}
	
	// Wait for response
	response, responseType, err := s.readEMSIResponse()
	if err != nil {
		return fmt.Errorf("failed to read EMSI response: %w", err)
	}
	
	if s.debug {
		log.Printf("EMSI: Received response type: %s", responseType)
	}
	
	switch responseType {
	case "REQ":
		// Remote wants our EMSI_DAT
		if err := s.sendEMSI_DAT(); err != nil {
			return fmt.Errorf("failed to send EMSI_DAT: %w", err)
		}
		
		// Read their EMSI_DAT or ACK
		response, responseType, err = s.readEMSIResponse()
		if err != nil {
			return fmt.Errorf("failed to read EMSI_DAT response: %w", err)
		}
		
		if responseType == "DAT" {
			// Parse remote's EMSI_DAT
			s.remoteInfo, _ = ParseEMSI_DAT(response)
			// Send ACK
			if err := s.sendEMSI_ACK(); err != nil {
				if s.debug {
					log.Printf("EMSI: Failed to send ACK: %v", err)
				}
			}
		}
		
	case "DAT":
		// Remote sent their EMSI_DAT directly
		s.remoteInfo, _ = ParseEMSI_DAT(response)
		
		// Send our EMSI_DAT in response
		if err := s.sendEMSI_DAT(); err != nil {
			return fmt.Errorf("failed to send EMSI_DAT: %w", err)
		}
		
		// Send ACK
		if err := s.sendEMSI_ACK(); err != nil {
			if s.debug {
				log.Printf("EMSI: Failed to send ACK: %v", err)
			}
		}
		
	case "NAK":
		return fmt.Errorf("remote sent EMSI_NAK")
		
	case "CLI":
		return fmt.Errorf("remote sent EMSI_CLI (calling system only)")
		
	case "HBT":
		return fmt.Errorf("remote sent EMSI_HBT (heartbeat)")
		
	default:
		return fmt.Errorf("unexpected response type: %s", responseType)
	}
	
	return nil
}

// sendEMSI_INQ sends EMSI inquiry
func (s *Session) sendEMSI_INQ() error {
	if s.debug {
		log.Printf("EMSI: Sending EMSI_INQ")
	}
	
	s.conn.SetWriteDeadline(time.Now().Add(s.timeout))
	
	if _, err := s.writer.WriteString(EMSI_INQ + "\r"); err != nil {
		return err
	}
	return s.writer.Flush()
}

// sendEMSI_REQ sends EMSI request
func (s *Session) sendEMSI_REQ() error {
	if s.debug {
		log.Printf("EMSI: Sending EMSI_REQ")
	}
	
	s.conn.SetWriteDeadline(time.Now().Add(s.timeout))
	
	if _, err := s.writer.WriteString(EMSI_REQ + "\r"); err != nil {
		return err
	}
	return s.writer.Flush()
}

// sendEMSI_ACK sends EMSI acknowledgment
func (s *Session) sendEMSI_ACK() error {
	if s.debug {
		log.Printf("EMSI: Sending EMSI_ACK")
	}
	
	s.conn.SetWriteDeadline(time.Now().Add(s.timeout))
	
	if _, err := s.writer.WriteString(EMSI_ACK + "\r"); err != nil {
		return err
	}
	return s.writer.Flush()
}

// sendEMSI_DAT sends our EMSI data packet
func (s *Session) sendEMSI_DAT() error {
	// Create our EMSI data
	data := &EMSIData{
		SystemName:    "NodelistDB Test Daemon",
		Location:      "Test Location",
		Sysop:         "Test Operator",
		Phone:         "-Unpublished-",
		Speed:         "TCP/IP",
		Flags:         "XA",  // Mail only
		MailerName:    "NodelistDB",
		MailerVersion: "1.0",
		MailerSerial:  "TEST",
		Addresses:     []string{s.localAddress},
		Protocols:     []string{"NCP"}, // No Compatible Protocols (we don't transfer files)
		Password:      "-",
	}
	
	packet := CreateEMSI_DAT(data)
	
	if s.debug {
		log.Printf("EMSI: Sending EMSI_DAT (%d bytes)", len(packet))
	}
	
	s.conn.SetWriteDeadline(time.Now().Add(s.timeout))
	
	// Send the packet with CR
	if _, err := s.writer.WriteString(packet + "\r"); err != nil {
		return err
	}
	
	// Send additional CR (some mailers expect double CR)
	if _, err := s.writer.WriteString("\r"); err != nil {
		return err
	}
	
	return s.writer.Flush()
}

// readEMSIResponse reads and identifies EMSI response
func (s *Session) readEMSIResponse() (string, string, error) {
	s.conn.SetReadDeadline(time.Now().Add(s.timeout))
	
	// Read up to 16KB (max EMSI_DAT size)
	data := make([]byte, 16384)
	n, err := s.reader.Read(data)
	if err != nil {
		return "", "", err
	}
	
	response := string(data[:n])
	
	if s.debug {
		log.Printf("EMSI: Received %d bytes", n)
		if n < 200 {
			log.Printf("EMSI: Data: %q", response)
		}
	}
	
	// Identify response type
	if strings.Contains(response, "EMSI_REQ") {
		return response, "REQ", nil
	}
	if strings.Contains(response, "EMSI_DAT") {
		return response, "DAT", nil
	}
	if strings.Contains(response, "EMSI_ACK") {
		return response, "ACK", nil
	}
	if strings.Contains(response, "EMSI_NAK") {
		return response, "NAK", nil
	}
	if strings.Contains(response, "EMSI_CLI") {
		return response, "CLI", nil
	}
	if strings.Contains(response, "EMSI_HBT") {
		return response, "HBT", nil
	}
	if strings.Contains(response, "EMSI_INQ") {
		return response, "INQ", nil
	}
	
	// Check if it's a banner or other text
	if len(response) > 0 {
		return response, "BANNER", nil
	}
	
	return response, "UNKNOWN", nil
}

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

// normalizeAddress normalizes a FidoNet address for comparison
func normalizeAddress(addr string) string {
	// Remove spaces and convert to lowercase
	addr = strings.TrimSpace(strings.ToLower(addr))
	
	// Remove @domain if present
	if idx := strings.Index(addr, "@"); idx != -1 {
		addr = addr[:idx]
	}
	
	return addr
}