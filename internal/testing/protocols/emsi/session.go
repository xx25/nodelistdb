package emsi

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

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
		systemName:   "NodelistDB Test Daemon",
		sysop:        "Test Operator",
		location:     "Test Location",
		timeout:      30 * time.Second,
		debug:        false, // Debug mode disabled by default
	}
}

// NewSessionWithInfo creates a new EMSI session with custom system info
func NewSessionWithInfo(conn net.Conn, localAddress, systemName, sysop, location string) *Session {
	return &Session{
		conn:         conn,
		reader:       bufio.NewReader(conn),
		writer:       bufio.NewWriter(conn),
		localAddress: localAddress,
		systemName:   systemName,
		sysop:        sysop,
		location:     location,
		timeout:      30 * time.Second,
		debug:        false, // Debug mode disabled by default
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
	// Send CR first to trigger remote's EMSI announcement (like bforce does)
	if s.debug {
		log.Printf("EMSI: === Starting EMSI Handshake ===")
		log.Printf("EMSI: Session timeout: %v", s.timeout)
		log.Printf("EMSI: Our address: %s", s.localAddress)
		log.Printf("EMSI: Sending initial CR to trigger remote EMSI")
	}
	s.conn.SetWriteDeadline(time.Now().Add(s.timeout))
	if _, err := s.writer.WriteString("\r"); err != nil {
		if s.debug {
			log.Printf("EMSI: ERROR sending initial CR: %v", err)
		}
		return fmt.Errorf("failed to send initial CR: %w", err)
	}
	if err := s.writer.Flush(); err != nil {
		if s.debug {
			log.Printf("EMSI: ERROR flushing initial CR: %v", err)
		}
		return fmt.Errorf("failed to flush initial CR: %w", err)
	}
	
	if s.debug {
		log.Printf("EMSI: Initial CR sent, waiting for response...")
	}
	
	// Wait for response (should be EMSI_REQ from remote)
	response, responseType, err := s.readEMSIResponse()
	if err != nil {
		// If no EMSI response after CR, try sending EMSI_INQ as fallback
		if s.debug {
			log.Printf("EMSI: No response to CR (error: %v), trying EMSI_INQ as fallback", err)
		}
		if err := s.sendEMSI_INQ(); err != nil {
			if s.debug {
				log.Printf("EMSI: ERROR sending EMSI_INQ: %v", err)
			}
			return fmt.Errorf("failed to send EMSI_INQ: %w", err)
		}
		
		// Try reading response again
		if s.debug {
			log.Printf("EMSI: EMSI_INQ sent, waiting for response...")
		}
		response, responseType, err = s.readEMSIResponse()
		if err != nil {
			if s.debug {
				log.Printf("EMSI: ERROR reading response after INQ: %v", err)
			}
			return fmt.Errorf("failed to read EMSI response: %w", err)
		}
	}
	
	if s.debug {
		log.Printf("EMSI: Received response type: %s", responseType)
		if len(response) > 0 && len(response) < 500 {
			log.Printf("EMSI: Response data (first 500 chars): %q", response)
		} else if len(response) > 0 {
			log.Printf("EMSI: Response data (first 500 chars): %q...", response[:500])
		}
	}
	
	switch responseType {
	case "REQ":
		// Remote wants our EMSI_DAT
		if s.debug {
			log.Printf("EMSI: Remote requested our data (EMSI_REQ), sending EMSI_DAT...")
		}
		if err := s.sendEMSI_DAT(); err != nil {
			if s.debug {
				log.Printf("EMSI: ERROR sending EMSI_DAT: %v", err)
			}
			return fmt.Errorf("failed to send EMSI_DAT: %w", err)
		}
		
		// Now we need to wait for their EMSI_DAT
		// They might send multiple REQs or other messages before sending DAT
		maxAttempts := 5
		if s.debug {
			log.Printf("EMSI: Waiting for remote's EMSI_DAT (max %d attempts)...", maxAttempts)
		}
		for i := 0; i < maxAttempts; i++ {
			if s.debug {
				log.Printf("EMSI: Reading response attempt %d/%d...", i+1, maxAttempts)
			}
			response, responseType, err = s.readEMSIResponse()
			if err != nil {
				if s.debug {
					log.Printf("EMSI: ERROR reading response (attempt %d): %v", i+1, err)
				}
				return fmt.Errorf("failed to read EMSI_DAT response (attempt %d): %w", i+1, err)
			}
			
			if s.debug {
				log.Printf("EMSI: After sending DAT, received response type: %s (attempt %d)", responseType, i+1)
			}
			
			if responseType == "DAT" {
				// Parse remote's EMSI_DAT
				if s.debug {
					log.Printf("EMSI: Received remote's EMSI_DAT, parsing...")
				}
				var parseErr error
				s.remoteInfo, parseErr = ParseEMSI_DAT(response)
				if s.debug {
					if parseErr != nil {
						log.Printf("EMSI: ERROR parsing remote data: %v", parseErr)
					} else {
						log.Printf("EMSI: Successfully parsed remote data:")
						log.Printf("EMSI:   System: %s", s.remoteInfo.SystemName)
						log.Printf("EMSI:   Location: %s", s.remoteInfo.Location)
						log.Printf("EMSI:   Sysop: %s", s.remoteInfo.Sysop)
						log.Printf("EMSI:   Mailer: %s %s", s.remoteInfo.MailerName, s.remoteInfo.MailerVersion)
						log.Printf("EMSI:   Addresses: %v", s.remoteInfo.Addresses)
					}
				}
				// Send ACK
				if err := s.sendEMSI_ACK(); err != nil {
					if s.debug {
						log.Printf("EMSI: WARNING: Failed to send ACK: %v", err)
					}
				} else if s.debug {
					log.Printf("EMSI: Sent ACK for remote's DAT")
				}
				break
			} else if responseType == "REQ" {
				// They're still requesting, send our DAT again
				if s.debug {
					log.Printf("EMSI: Remote sent another REQ, resending our DAT")
				}
				if err := s.sendEMSI_DAT(); err != nil {
					return fmt.Errorf("failed to resend EMSI_DAT: %w", err)
				}
			} else if responseType == "ACK" {
				// They acknowledged but didn't send their DAT yet
				if s.debug {
					log.Printf("EMSI: Remote sent ACK, waiting for DAT")
				}
				continue
			} else if responseType == "NAK" {
				// Remote is rejecting our DAT - there might be a CRC or format issue
				if s.debug {
					log.Printf("EMSI: Remote sent NAK, our EMSI_DAT might be invalid")
				}
				// Some implementations might still work after NAK, keep trying
				continue
			}
		}
		
		if s.remoteInfo == nil {
			if s.debug {
				log.Printf("EMSI: WARNING: Failed to receive remote DAT after %d attempts", maxAttempts)
				log.Printf("EMSI: Handshake incomplete - no remote data received")
			}
		} else if s.debug {
			log.Printf("EMSI: Handshake completed successfully after REQ exchange")
		}
		
	case "DAT":
		// Remote sent their EMSI_DAT directly
		if s.debug {
			log.Printf("EMSI: Remote sent EMSI_DAT directly, parsing...")
		}
		var parseErr error
		s.remoteInfo, parseErr = ParseEMSI_DAT(response)
		if s.debug {
			if parseErr != nil {
				log.Printf("EMSI: ERROR parsing remote data: %v", parseErr)
			} else {
				log.Printf("EMSI: Successfully parsed remote data:")
				log.Printf("EMSI:   System: %s", s.remoteInfo.SystemName)
				log.Printf("EMSI:   Location: %s", s.remoteInfo.Location)
				log.Printf("EMSI:   Sysop: %s", s.remoteInfo.Sysop)
				log.Printf("EMSI:   Mailer: %s %s", s.remoteInfo.MailerName, s.remoteInfo.MailerVersion)
				log.Printf("EMSI:   Addresses: %v", s.remoteInfo.Addresses)
			}
		}
		
		// Send our EMSI_DAT in response
		if s.debug {
			log.Printf("EMSI: Sending our EMSI_DAT in response...")
		}
		if err := s.sendEMSI_DAT(); err != nil {
			if s.debug {
				log.Printf("EMSI: ERROR sending EMSI_DAT: %v", err)
			}
			return fmt.Errorf("failed to send EMSI_DAT: %w", err)
		}
		
		// Send ACK
		if err := s.sendEMSI_ACK(); err != nil {
			if s.debug {
				log.Printf("EMSI: WARNING: Failed to send ACK: %v", err)
			}
		} else if s.debug {
			log.Printf("EMSI: Sent ACK for remote's DAT")
			log.Printf("EMSI: Handshake completed successfully after DAT exchange")
		}
		
	case "NAK":
		return fmt.Errorf("remote sent EMSI_NAK")
		
	case "CLI":
		return fmt.Errorf("remote sent EMSI_CLI (calling system only)")
		
	case "HBT":
		return fmt.Errorf("remote sent EMSI_HBT (heartbeat)")
		
	default:
		if s.debug {
			log.Printf("EMSI: ERROR: Unexpected response type: %s", responseType)
		}
		return fmt.Errorf("unexpected response type: %s", responseType)
	}
	
	if s.debug {
		log.Printf("EMSI: === Handshake Complete ===")
	}
	
	return nil
}

// sendEMSI_INQ sends EMSI inquiry
func (s *Session) sendEMSI_INQ() error {
	if s.debug {
		log.Printf("EMSI: sendEMSI_INQ: Sending EMSI_INQ")
	}
	
	deadline := time.Now().Add(s.timeout)
	s.conn.SetWriteDeadline(deadline)
	
	if _, err := s.writer.WriteString(EMSI_INQ + "\r"); err != nil {
		if s.debug {
			log.Printf("EMSI: sendEMSI_INQ: ERROR writing: %v", err)
		}
		return err
	}
	
	if err := s.writer.Flush(); err != nil {
		if s.debug {
			log.Printf("EMSI: sendEMSI_INQ: ERROR flushing: %v", err)
		}
		return err
	}
	
	if s.debug {
		log.Printf("EMSI: sendEMSI_INQ: Successfully sent EMSI_INQ")
	}
	return nil}

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
	if s.debug {
		log.Printf("EMSI: sendEMSI_DAT: Preparing EMSI_DAT packet...")
	}
	
	// Create our EMSI data
	data := &EMSIData{
		SystemName:    s.systemName,
		Location:      s.location,
		Sysop:         s.sysop,
		Phone:         "-Unpublished-",
		Speed:         "TCP/IP",
		Flags:         "XA",  // Mail only
		MailerName:    "NodelistDB",
		MailerVersion: "1.0",
		MailerSerial:  "TEST",
		Addresses:     []string{s.localAddress + "@fidonet"},
		Protocols:     []string{"ZMO", "ZAP"}, // Zmodem protocols (even though we don't transfer files)
		Password:      "",  // Empty password
	}
	
	packet := CreateEMSI_DAT(data)
	
	if s.debug {
		log.Printf("EMSI: sendEMSI_DAT: Created EMSI_DAT packet (%d bytes)", len(packet))
		// Log first 200 chars of packet for debugging
		if len(packet) > 200 {
			log.Printf("EMSI: sendEMSI_DAT: DAT packet (first 200): %q", packet[:200])
		} else {
			log.Printf("EMSI: sendEMSI_DAT: DAT packet: %q", packet)
		}
		// Also log what we're actually writing including CR and XON
		fullPacket := packet + "\r\x11\r"
		log.Printf("EMSI: sendEMSI_DAT: Full packet with terminators (%d bytes)", len(fullPacket))
	}
	
	deadline := time.Now().Add(s.timeout)
	s.conn.SetWriteDeadline(deadline)
	
	if s.debug {
		log.Printf("EMSI: sendEMSI_DAT: Sending packet with deadline %v...", deadline.Format("15:04:05.000"))
	}
	
	// Send the packet with CR and XON (like ifmail does)
	if _, err := s.writer.WriteString(packet + "\r\x11"); err != nil {
		if s.debug {
			log.Printf("EMSI: sendEMSI_DAT: ERROR writing packet: %v", err)
		}
		return fmt.Errorf("failed to write EMSI_DAT: %w", err)
	}
	
	// Send additional CR (some mailers expect double CR)
	if _, err := s.writer.WriteString("\r"); err != nil {
		if s.debug {
			log.Printf("EMSI: sendEMSI_DAT: ERROR writing additional CR: %v", err)
		}
		return fmt.Errorf("failed to write additional CR: %w", err)
	}
	
	if err := s.writer.Flush(); err != nil {
		if s.debug {
			log.Printf("EMSI: sendEMSI_DAT: ERROR flushing buffer: %v", err)
		}
		return fmt.Errorf("failed to flush EMSI_DAT: %w", err)
	}
	
	if s.debug {
		log.Printf("EMSI: sendEMSI_DAT: Successfully sent EMSI_DAT packet")
	}
	
	return nil}

// readEMSIResponse reads and identifies EMSI response
func (s *Session) readEMSIResponse() (string, string, error) {
	deadline := time.Now().Add(s.timeout)
	s.conn.SetReadDeadline(deadline)
	
	if s.debug {
		log.Printf("EMSI: readEMSIResponse: Starting read with timeout %v (deadline: %v)", s.timeout, deadline.Format("15:04:05.000"))
	}
	
	// Accumulate response data, some systems send banner first
	var response strings.Builder
	buffer := make([]byte, 4096)
	attempts := 0
	maxAttempts := 5
	totalBytesRead := 0
	startTime := time.Now()
	
	for attempts < maxAttempts {
		if s.debug {
			log.Printf("EMSI: readEMSIResponse: Read attempt %d/%d...", attempts+1, maxAttempts)
		}
		
		n, err := s.reader.Read(buffer)
		elapsed := time.Since(startTime)
		
		if err != nil {
			if s.debug {
				log.Printf("EMSI: readEMSIResponse: Read error after %v: %v (bytes so far: %d)", elapsed, err, totalBytesRead)
			}
			if attempts > 0 && response.Len() > 0 {
				// We have some data, check what we got
				if s.debug {
					log.Printf("EMSI: readEMSIResponse: Have partial data (%d bytes), processing...", response.Len())
				}
				break
			}
			return "", "", fmt.Errorf("read failed after %v: %w", elapsed, err)
		}
		
		chunk := string(buffer[:n])
		response.WriteString(chunk)
		totalBytesRead += n
		
		if s.debug {
			log.Printf("EMSI: readEMSIResponse: Received %d bytes (attempt %d, total: %d, elapsed: %v)", 
				n, attempts+1, totalBytesRead, elapsed)
			if n < 200 {
				log.Printf("EMSI: readEMSIResponse: Data: %q", chunk)
			} else {
				log.Printf("EMSI: readEMSIResponse: Data (first 200): %q...", chunk[:200])
			}
		}
		
		responseStr := response.String()
		
		// Check if we have EMSI sequences
		if strings.Contains(responseStr, "EMSI_NAK") {
			if s.debug {
				log.Printf("EMSI: readEMSIResponse: Found EMSI_NAK in response")
			}
			return responseStr, "NAK", nil
		}
		if strings.Contains(responseStr, "EMSI_DAT") {
			if s.debug {
				log.Printf("EMSI: readEMSIResponse: Found EMSI_DAT in response")
			}
			return responseStr, "DAT", nil
		}
		if strings.Contains(responseStr, "EMSI_ACK") {
			if s.debug {
				log.Printf("EMSI: readEMSIResponse: Found EMSI_ACK in response")
			}
			return responseStr, "ACK", nil
		}
		if strings.Contains(responseStr, "EMSI_REQ") {
			if s.debug {
				log.Printf("EMSI: readEMSIResponse: Found EMSI_REQ in response")
			}
			return responseStr, "REQ", nil
		}
		if strings.Contains(responseStr, "EMSI_CLI") {
			if s.debug {
				log.Printf("EMSI: readEMSIResponse: Found EMSI_CLI in response")
			}
			return responseStr, "CLI", nil
		}
		if strings.Contains(responseStr, "EMSI_HBT") {
			if s.debug {
				log.Printf("EMSI: readEMSIResponse: Found EMSI_HBT in response")
			}
			return responseStr, "HBT", nil
		}
		if strings.Contains(responseStr, "EMSI_INQ") {
			if s.debug {
				log.Printf("EMSI: readEMSIResponse: Found EMSI_INQ in response")
			}
			return responseStr, "INQ", nil
		}
		
		// Continue reading if we haven't found EMSI yet
		if s.debug {
			log.Printf("EMSI: readEMSIResponse: No EMSI sequence found yet, continuing...")
		}
		attempts++
		
		// Small delay between reads
		time.Sleep(100 * time.Millisecond)
	}
	
	// No EMSI found, check what we got
	responseStr := response.String()
	finalElapsed := time.Since(startTime)
	
	if len(responseStr) > 0 {
		// Got some data but no EMSI, treat as banner
		if s.debug {
			log.Printf("EMSI: readEMSIResponse: No EMSI sequence found after %v, treating %d bytes as BANNER", finalElapsed, len(responseStr))
			if len(responseStr) < 500 {
				log.Printf("EMSI: readEMSIResponse: Banner content: %q", responseStr)
			} else {
				log.Printf("EMSI: readEMSIResponse: Banner content (first 500): %q...", responseStr[:500])
			}
		}
		return responseStr, "BANNER", nil
	}
	
	if s.debug {
		log.Printf("EMSI: readEMSIResponse: No data received after %v, returning UNKNOWN", finalElapsed)
	}
	return "", "UNKNOWN", nil
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