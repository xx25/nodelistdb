package emsi

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
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
	bannerText   string // Capture full banner text during handshake
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
		logging.Debugf("EMSI: === Starting EMSI Handshake ===")
		logging.Debugf("EMSI: Session timeout: %v", s.timeout)
		logging.Debugf("EMSI: Our address: %s", s.localAddress)
		logging.Debugf("EMSI: Sending initial CR to trigger remote EMSI")
	}
	_ = s.conn.SetWriteDeadline(time.Now().Add(s.timeout))
	if _, err := s.writer.WriteString("\r"); err != nil {
		if s.debug {
			logging.Debugf("EMSI: ERROR sending initial CR: %v", err)
		}
		return fmt.Errorf("failed to send initial CR: %w", err)
	}
	if err := s.writer.Flush(); err != nil {
		if s.debug {
			logging.Debugf("EMSI: ERROR flushing initial CR: %v", err)
		}
		return fmt.Errorf("failed to flush initial CR: %w", err)
	}
	
	if s.debug {
		logging.Debugf("EMSI: Initial CR sent, waiting for response...")
	}
	
	// Wait for response (should be EMSI_REQ from remote)
	response, responseType, err := s.readEMSIResponse()
	if err != nil {
		// If no EMSI response after CR, try sending EMSI_INQ as fallback
		if s.debug {
			logging.Debugf("EMSI: No response to CR (error: %v), trying EMSI_INQ as fallback", err)
		}
		if err := s.sendEMSI_INQ(); err != nil {
			if s.debug {
				logging.Debugf("EMSI: ERROR sending EMSI_INQ: %v", err)
			}
			return fmt.Errorf("failed to send EMSI_INQ: %w", err)
		}
		
		// Try reading response again
		if s.debug {
			logging.Debugf("EMSI: EMSI_INQ sent, waiting for response...")
		}
		response, responseType, err = s.readEMSIResponse()
		if err != nil {
			if s.debug {
				logging.Debugf("EMSI: ERROR reading response after INQ: %v", err)
			}
			return fmt.Errorf("failed to read EMSI response: %w", err)
		}
	}
	
	if s.debug {
		logging.Debugf("EMSI: Received response type: %s", responseType)
		if len(response) > 0 && len(response) < 500 {
			logging.Debugf("EMSI: Response data (first 500 chars): %q", response)
		} else if len(response) > 0 {
			logging.Debugf("EMSI: Response data (first 500 chars): %q...", response[:500])
		}
	}
	
	switch responseType {
	case "REQ":
		// Remote wants our EMSI_DAT
		if s.debug {
			logging.Debugf("EMSI: Remote requested our data (EMSI_REQ), sending EMSI_DAT...")
		}
		if err := s.sendEMSI_DAT(); err != nil {
			if s.debug {
				logging.Debugf("EMSI: ERROR sending EMSI_DAT: %v", err)
			}
			return fmt.Errorf("failed to send EMSI_DAT: %w", err)
		}
		
		// Now we need to wait for their EMSI_DAT
		// They might send multiple REQs or other messages before sending DAT
		maxAttempts := 5
		if s.debug {
			logging.Debugf("EMSI: Waiting for remote's EMSI_DAT (max %d attempts)...", maxAttempts)
		}
		for i := 0; i < maxAttempts; i++ {
			if s.debug {
				logging.Debugf("EMSI: Reading response attempt %d/%d...", i+1, maxAttempts)
			}
			response, responseType, err = s.readEMSIResponse()
			if err != nil {
				if s.debug {
					logging.Debugf("EMSI: ERROR reading response (attempt %d): %v", i+1, err)
				}
				return fmt.Errorf("failed to read EMSI_DAT response (attempt %d): %w", i+1, err)
			}
			
			if s.debug {
				logging.Debugf("EMSI: After sending DAT, received response type: %s (attempt %d)", responseType, i+1)
			}
			
			if responseType == "DAT" {
				// Parse remote's EMSI_DAT
				if s.debug {
					logging.Debugf("EMSI: Received remote's EMSI_DAT, parsing...")
				}
				var parseErr error
				s.remoteInfo, parseErr = ParseEMSI_DAT(response)
				if s.debug {
					if parseErr != nil {
						logging.Debugf("EMSI: ERROR parsing remote data: %v", parseErr)
					} else {
						logging.Debugf("EMSI: Successfully parsed remote data:")
						logging.Debugf("EMSI:   System: %s", s.remoteInfo.SystemName)
						logging.Debugf("EMSI:   Location: %s", s.remoteInfo.Location)
						logging.Debugf("EMSI:   Sysop: %s", s.remoteInfo.Sysop)
						logging.Debugf("EMSI:   Mailer: %s %s", s.remoteInfo.MailerName, s.remoteInfo.MailerVersion)
						logging.Debugf("EMSI:   Addresses: %v", s.remoteInfo.Addresses)
					}
				}
				// Send ACK
				if err := s.sendEMSI_ACK(); err != nil {
					if s.debug {
						logging.Debugf("EMSI: WARNING: Failed to send ACK: %v", err)
					}
				} else if s.debug {
					logging.Debugf("EMSI: Sent ACK for remote's DAT")
				}
				break
			} else if responseType == "REQ" {
				// They're still requesting, send our DAT again
				if s.debug {
					logging.Debugf("EMSI: Remote sent another REQ, resending our DAT")
				}
				if err := s.sendEMSI_DAT(); err != nil {
					return fmt.Errorf("failed to resend EMSI_DAT: %w", err)
				}
			} else if responseType == "ACK" {
				// They acknowledged but didn't send their DAT yet
				if s.debug {
					logging.Debugf("EMSI: Remote sent ACK, waiting for DAT")
				}
				continue
			} else if responseType == "NAK" {
				// Remote is rejecting our DAT - there might be a CRC or format issue
				if s.debug {
					logging.Debugf("EMSI: Remote sent NAK, our EMSI_DAT might be invalid")
				}
				// Some implementations might still work after NAK, keep trying
				continue
			}
		}

		if s.remoteInfo == nil {
			if s.debug {
				logging.Debugf("EMSI: WARNING: Failed to receive remote DAT after %d attempts", maxAttempts)
				logging.Debugf("EMSI: Attempting banner text extraction as fallback...")
			}

			// Try to extract software from banner text
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
					logging.Debugf("EMSI: Successfully extracted software from banner: %s %s", software.Name, software.Version)
				}
			} else if s.debug {
				logging.Debugf("EMSI: Handshake incomplete - no remote data received and banner extraction failed")
			}
		} else if s.debug {
			logging.Debugf("EMSI: Handshake completed successfully after REQ exchange")
		}
		
	case "DAT":
		// Remote sent their EMSI_DAT directly
		if s.debug {
			logging.Debugf("EMSI: Remote sent EMSI_DAT directly, parsing...")
		}
		var parseErr error
		s.remoteInfo, parseErr = ParseEMSI_DAT(response)
		if s.debug {
			if parseErr != nil {
				logging.Debugf("EMSI: ERROR parsing remote data: %v", parseErr)
			} else {
				logging.Debugf("EMSI: Successfully parsed remote data:")
				logging.Debugf("EMSI:   System: %s", s.remoteInfo.SystemName)
				logging.Debugf("EMSI:   Location: %s", s.remoteInfo.Location)
				logging.Debugf("EMSI:   Sysop: %s", s.remoteInfo.Sysop)
				logging.Debugf("EMSI:   Mailer: %s %s", s.remoteInfo.MailerName, s.remoteInfo.MailerVersion)
				logging.Debugf("EMSI:   Addresses: %v", s.remoteInfo.Addresses)
			}
		}
		
		// Send our EMSI_DAT in response
		if s.debug {
			logging.Debugf("EMSI: Sending our EMSI_DAT in response...")
		}
		if err := s.sendEMSI_DAT(); err != nil {
			if s.debug {
				logging.Debugf("EMSI: ERROR sending EMSI_DAT: %v", err)
			}
			return fmt.Errorf("failed to send EMSI_DAT: %w", err)
		}
		
		// Send ACK
		if err := s.sendEMSI_ACK(); err != nil {
			if s.debug {
				logging.Debugf("EMSI: WARNING: Failed to send ACK: %v", err)
			}
		} else if s.debug {
			logging.Debugf("EMSI: Sent ACK for remote's DAT")
			logging.Debugf("EMSI: Handshake completed successfully after DAT exchange")
		}
		
	case "NAK":
		return fmt.Errorf("remote sent EMSI_NAK")
		
	case "CLI":
		return fmt.Errorf("remote sent EMSI_CLI (calling system only)")
		
	case "HBT":
		return fmt.Errorf("remote sent EMSI_HBT (heartbeat)")
		
	default:
		if s.debug {
			logging.Debugf("EMSI: ERROR: Unexpected response type: %s", responseType)
		}
		return fmt.Errorf("unexpected response type: %s", responseType)
	}
	
	if s.debug {
		logging.Debugf("EMSI: === Handshake Complete ===")
	}
	
	return nil
}

// sendEMSI_INQ sends EMSI inquiry
func (s *Session) sendEMSI_INQ() error {
	if s.debug {
		logging.Debugf("EMSI: sendEMSI_INQ: Sending EMSI_INQ")
	}
	
	deadline := time.Now().Add(s.timeout)
	_ = s.conn.SetWriteDeadline(deadline)

	if _, err := s.writer.WriteString(EMSI_INQ + "\r"); err != nil {
		if s.debug {
			logging.Debugf("EMSI: sendEMSI_INQ: ERROR writing: %v", err)
		}
		return err
	}
	
	if err := s.writer.Flush(); err != nil {
		if s.debug {
			logging.Debugf("EMSI: sendEMSI_INQ: ERROR flushing: %v", err)
		}
		return err
	}
	
	if s.debug {
		logging.Debugf("EMSI: sendEMSI_INQ: Successfully sent EMSI_INQ")
	}
	return nil
}

// sendEMSI_ACK sends EMSI acknowledgment
func (s *Session) sendEMSI_ACK() error {
	if s.debug {
		logging.Debugf("EMSI: Sending EMSI_ACK")
	}

	_ = s.conn.SetWriteDeadline(time.Now().Add(s.timeout))
	
	if _, err := s.writer.WriteString(EMSI_ACK + "\r"); err != nil {
		return err
	}
	return s.writer.Flush()
}

// sendEMSI_DAT sends our EMSI data packet
func (s *Session) sendEMSI_DAT() error {
	if s.debug {
		logging.Debugf("EMSI: sendEMSI_DAT: Preparing EMSI_DAT packet...")
	}
	
	// Create our EMSI data
	data := &EMSIData{
		SystemName:    s.systemName,
		Location:      s.location,
		Sysop:         s.sysop,
		Phone:         "-Unpublished-",
		Speed:         "9600",  // Numeric baud for compatibility
		Flags:         "CM,IFC,XA",  // Traditional flags: Continuous Mail, IFCICO, Mail only
		MailerName:    "NodelistDB",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",  // Traditional OS identifier
		Addresses:     []string{s.localAddress},  // Bare address without @fidonet suffix
		Protocols:     []string{"ZMO", "ZAP"}, // Zmodem protocols (even though we don't transfer files)
		Password:      "",  // Empty password
	}
	
	packet := CreateEMSI_DAT(data)
	
	if s.debug {
		logging.Debugf("EMSI: sendEMSI_DAT: Created EMSI_DAT packet (%d bytes)", len(packet))
		// Log first 200 chars of packet for debugging
		if len(packet) > 200 {
			logging.Debugf("EMSI: sendEMSI_DAT: DAT packet (first 200): %q", packet[:200])
		} else {
			logging.Debugf("EMSI: sendEMSI_DAT: DAT packet: %q", packet)
		}
		// Also log what we're actually writing including CRs
		fullPacket := packet + "\r\r"
		logging.Debugf("EMSI: sendEMSI_DAT: Full packet with terminators (%d bytes)", len(fullPacket))
	}
	
	deadline := time.Now().Add(s.timeout)
	_ = s.conn.SetWriteDeadline(deadline)

	if s.debug {
		logging.Debugf("EMSI: sendEMSI_DAT: Sending packet with deadline %v...", deadline.Format("15:04:05.000"))
	}
	
	// Send the packet with CR (binkleyforce-compatible, ifcico drops XON anyway)
	if _, err := s.writer.WriteString(packet + "\r"); err != nil {
		if s.debug {
			logging.Debugf("EMSI: sendEMSI_DAT: ERROR writing packet: %v", err)
		}
		return fmt.Errorf("failed to write EMSI_DAT: %w", err)
	}

	// Send additional CR (some mailers expect double CR)
	if _, err := s.writer.WriteString("\r"); err != nil {
		if s.debug {
			logging.Debugf("EMSI: sendEMSI_DAT: ERROR writing additional CR: %v", err)
		}
		return fmt.Errorf("failed to write additional CR: %w", err)
	}
	
	if err := s.writer.Flush(); err != nil {
		if s.debug {
			logging.Debugf("EMSI: sendEMSI_DAT: ERROR flushing buffer: %v", err)
		}
		return fmt.Errorf("failed to flush EMSI_DAT: %w", err)
	}
	
	if s.debug {
		logging.Debugf("EMSI: sendEMSI_DAT: Successfully sent EMSI_DAT packet")
	}
	
	return nil}

// readEMSIResponse reads and identifies EMSI response
func (s *Session) readEMSIResponse() (string, string, error) {
	deadline := time.Now().Add(s.timeout)
	_ = s.conn.SetReadDeadline(deadline)

	if s.debug {
		logging.Debugf("EMSI: readEMSIResponse: Starting read with timeout %v (deadline: %v)", s.timeout, deadline.Format("15:04:05.000"))
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
			logging.Debugf("EMSI: readEMSIResponse: Read attempt %d/%d...", attempts+1, maxAttempts)
		}
		
		n, err := s.reader.Read(buffer)
		elapsed := time.Since(startTime)
		
		if err != nil {
			if s.debug {
				logging.Debugf("EMSI: readEMSIResponse: Read error after %v: %v (bytes so far: %d)", elapsed, err, totalBytesRead)
			}
			if attempts > 0 && response.Len() > 0 {
				// We have some data, check what we got
				if s.debug {
					logging.Debugf("EMSI: readEMSIResponse: Have partial data (%d bytes), processing...", response.Len())
				}
				break
			}
			return "", "", fmt.Errorf("read failed after %v: %w", elapsed, err)
		}
		
		chunk := string(buffer[:n])
		response.WriteString(chunk)
		totalBytesRead += n

		if s.debug {
			logging.Debugf("EMSI: readEMSIResponse: Received %d bytes (attempt %d, total: %d, elapsed: %v)",
				n, attempts+1, totalBytesRead, elapsed)
			if n < 200 {
				logging.Debugf("EMSI: readEMSIResponse: Data: %q", chunk)
			} else {
				logging.Debugf("EMSI: readEMSIResponse: Data (first 200): %q...", chunk[:200])
			}
		}

		responseStr := response.String()

		// Always update banner text with accumulated response to capture initial banner
		// The first real response often contains the banner along with EMSI_REQ
		if len(responseStr) > len(s.bannerText) {
			s.bannerText = responseStr
		}
		
		// Check if we have EMSI sequences
		if strings.Contains(responseStr, "EMSI_NAK") {
			if s.debug {
				logging.Debugf("EMSI: readEMSIResponse: Found EMSI_NAK in response")
			}
			return responseStr, "NAK", nil
		}
		if strings.Contains(responseStr, "EMSI_DAT") {
			if s.debug {
				logging.Debugf("EMSI: readEMSIResponse: Found EMSI_DAT in response")
			}
			return responseStr, "DAT", nil
		}
		if strings.Contains(responseStr, "EMSI_ACK") {
			if s.debug {
				logging.Debugf("EMSI: readEMSIResponse: Found EMSI_ACK in response")
			}
			return responseStr, "ACK", nil
		}
		if strings.Contains(responseStr, "EMSI_REQ") {
			if s.debug {
				logging.Debugf("EMSI: readEMSIResponse: Found EMSI_REQ in response")
			}
			return responseStr, "REQ", nil
		}
		if strings.Contains(responseStr, "EMSI_CLI") {
			if s.debug {
				logging.Debugf("EMSI: readEMSIResponse: Found EMSI_CLI in response")
			}
			return responseStr, "CLI", nil
		}
		if strings.Contains(responseStr, "EMSI_HBT") {
			if s.debug {
				logging.Debugf("EMSI: readEMSIResponse: Found EMSI_HBT in response")
			}
			return responseStr, "HBT", nil
		}
		if strings.Contains(responseStr, "EMSI_INQ") {
			if s.debug {
				logging.Debugf("EMSI: readEMSIResponse: Found EMSI_INQ in response")
			}
			return responseStr, "INQ", nil
		}
		
		// Continue reading if we haven't found EMSI yet
		if s.debug {
			logging.Debugf("EMSI: readEMSIResponse: No EMSI sequence found yet, continuing...")
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
			logging.Debugf("EMSI: readEMSIResponse: No EMSI sequence found after %v, treating %d bytes as BANNER", finalElapsed, len(responseStr))
			if len(responseStr) < 500 {
				logging.Debugf("EMSI: readEMSIResponse: Banner content: %q", responseStr)
			} else {
				logging.Debugf("EMSI: readEMSIResponse: Banner content (first 500): %q...", responseStr[:500])
			}
		}
		return responseStr, "BANNER", nil
	}
	
	if s.debug {
		logging.Debugf("EMSI: readEMSIResponse: No data received after %v, returning UNKNOWN", finalElapsed)
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