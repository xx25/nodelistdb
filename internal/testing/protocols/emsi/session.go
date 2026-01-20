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
	config       *Config // EMSI configuration (FSC-0056.001 compliant defaults)
	debug        bool
	bannerText   string // Capture full banner text during handshake
}

// NewSession creates a new EMSI session with FSC-0056.001 defaults
func NewSession(conn net.Conn, localAddress string) *Session {
	return NewSessionWithConfig(conn, localAddress, nil)
}

// NewSessionWithInfo creates a new EMSI session with custom system info (backward compatible)
// Used by existing code like ifcico_tester.go
func NewSessionWithInfo(conn net.Conn, localAddress, systemName, sysop, location string) *Session {
	return NewSessionWithInfoAndConfig(conn, localAddress, systemName, sysop, location, nil)
}

// NewSessionWithConfig creates a session with custom config
func NewSessionWithConfig(conn net.Conn, localAddress string, cfg *Config) *Session {
	return NewSessionWithInfoAndConfig(conn, localAddress, "NodelistDB Test Daemon", "Test Operator", "Test Location", cfg)
}

// NewSessionWithInfoAndConfig creates a session with system info and custom config
// The config is defensively copied to prevent shared mutation.
func NewSessionWithInfoAndConfig(conn net.Conn, localAddress, systemName, sysop, location string, cfg *Config) *Session {
	if cfg == nil {
		cfg = DefaultConfig()
	} else {
		cfg = cfg.Copy() // Defensive copy to prevent shared mutation
	}
	return &Session{
		conn:         conn,
		reader:       bufio.NewReader(conn),
		writer:       bufio.NewWriter(conn),
		localAddress: localAddress,
		systemName:   systemName,
		sysop:        sysop,
		location:     location,
		config:       cfg,
		debug:        false,
	}
}

// SetDebug enables debug logging
func (s *Session) SetDebug(debug bool) {
	s.debug = debug
}

// SetTimeout is a legacy API for backward compatibility.
// Sets MasterTimeout and scales other timeouts proportionally:
// - If timeout > MasterTimeout: scales up all timeouts proportionally
// - If timeout < MasterTimeout: caps all timeouts at the new value
// Used by existing code that expects a single timeout value.
func (s *Session) SetTimeout(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	if s.config == nil {
		s.config = DefaultConfig()
	}

	oldMaster := s.config.MasterTimeout
	if oldMaster <= 0 {
		oldMaster = 60 * time.Second // FSC default
	}

	// Scale factor for proportional adjustment
	scale := float64(timeout) / float64(oldMaster)

	// Scale helper: adjust timeout proportionally, but cap at new master timeout
	scaleTimeout := func(d time.Duration) time.Duration {
		if d <= 0 {
			return timeout
		}
		scaled := time.Duration(float64(d) * scale)
		if scaled > timeout {
			return timeout
		}
		return scaled
	}

	s.config.MasterTimeout = timeout
	s.config.StepTimeout = scaleTimeout(s.config.StepTimeout)
	s.config.FirstStepTimeout = scaleTimeout(s.config.FirstStepTimeout)
	s.config.CharacterTimeout = scaleTimeout(s.config.CharacterTimeout)
	s.config.InitialCRTimeout = scaleTimeout(s.config.InitialCRTimeout)
	s.config.PreventiveINQTimeout = scaleTimeout(s.config.PreventiveINQTimeout)
}

// Handshake performs the EMSI handshake as caller (we initiate)
// Strategy is selected based on config.InitialStrategy:
// - "wait": FSC-0056.001 default - wait for remote EMSI_INQ/REQ
// - "send_cr": Send CRs to wake remote, then wait for EMSI
// - "send_inq": Immediately send EMSI_INQ
func (s *Session) Handshake() error {
	if s.debug {
		logging.Debugf("EMSI: === Starting EMSI Handshake ===")
		logging.Debugf("EMSI: Strategy: %s, PreventiveINQ: %v", s.config.InitialStrategy, s.config.PreventiveINQ)
		logging.Debugf("EMSI: Master timeout: %v, Step timeout: %v", s.config.MasterTimeout, s.config.StepTimeout)
		logging.Debugf("EMSI: Our address: %s", s.localAddress)
	}

	var response string
	var responseType string
	var err error

	// Select initial strategy
	switch s.config.InitialStrategy {
	case "send_cr":
		response, responseType, err = s.handshakeInitialSendCR()
	case "send_inq":
		response, responseType, err = s.handshakeInitialSendINQ()
	default: // "wait" is the FSC-0056.001 default
		response, responseType, err = s.handshakeInitialWait()
	}

	if err != nil {
		return err
	}

	// If we got banner but no EMSI and PreventiveINQ is enabled,
	// send EMSI_INQ to speed up handshake
	if (responseType == "BANNER" || responseType == "UNKNOWN") && s.config.PreventiveINQ {
		response, responseType, err = s.sendPreventiveINQ()
		if err != nil {
			return err
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
		maxAttempts := s.config.MaxRetries
		if maxAttempts <= 0 {
			maxAttempts = 6 // FSC-0056.001 default
		}
		if s.debug {
			logging.Debugf("EMSI: Waiting for remote's EMSI_DAT (max %d attempts)...", maxAttempts)
		}
		for i := 0; i < maxAttempts; i++ {
			if s.debug {
				logging.Debugf("EMSI: Reading response attempt %d/%d...", i+1, maxAttempts)
			}
			response, responseType, err = s.readEMSIResponseWithTimeout(s.config.StepTimeout)
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

// handshakeInitialWait implements FSC-0056.001 "wait" strategy
// Wait for remote to send EMSI_INQ or EMSI_REQ first
func (s *Session) handshakeInitialWait() (string, string, error) {
	if s.debug {
		logging.Debugf("EMSI: Strategy=wait: Waiting for remote EMSI (timeout=%v)...", s.config.StepTimeout)
	}

	response, responseType, err := s.readEMSIResponseWithTimeout(s.config.StepTimeout)
	if err != nil {
		if s.debug {
			logging.Debugf("EMSI: Strategy=wait: No response, error: %v", err)
		}
		return "", "", fmt.Errorf("wait strategy: %w", err)
	}

	return response, responseType, nil
}

// handshakeInitialSendCR implements "send_cr" strategy
// Send CRs to wake remote BBS, then wait for EMSI
func (s *Session) handshakeInitialSendCR() (string, string, error) {
	if s.debug {
		logging.Debugf("EMSI: Strategy=send_cr: Sending CRs to trigger remote EMSI...")
	}

	initialWait := s.config.InitialCRTimeout
	deadline := time.Now().Add(initialWait)
	gotData := false

	for time.Now().Before(deadline) {
		_ = s.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if _, err := s.writer.WriteString("\r"); err != nil {
			if s.debug {
				logging.Debugf("EMSI: Strategy=send_cr: ERROR sending CR: %v", err)
			}
			return "", "", fmt.Errorf("failed to send CR: %w", err)
		}
		if err := s.writer.Flush(); err != nil {
			if s.debug {
				logging.Debugf("EMSI: Strategy=send_cr: ERROR flushing CR: %v", err)
			}
			return "", "", fmt.Errorf("failed to flush CR: %w", err)
		}

		// Wait for any data (using configured CR interval)
		time.Sleep(s.config.InitialCRInterval)

		// Check if we have data available using Peek (non-destructive)
		_ = s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		if _, err := s.reader.Peek(1); err == nil {
			gotData = true
			if s.debug {
				logging.Debugf("EMSI: Strategy=send_cr: Got initial data from remote")
			}
			break
		}
	}

	if !gotData {
		if s.debug {
			logging.Debugf("EMSI: Strategy=send_cr: No data received after %v", initialWait)
		}
	}

	// Read with FirstStepTimeout to check for EMSI in data
	response, responseType, err := s.readEMSIResponseWithTimeout(s.config.FirstStepTimeout)
	if err != nil {
		// Return empty response type so PreventiveINQ can be triggered
		return "", "UNKNOWN", nil
	}

	return response, responseType, nil
}

// handshakeInitialSendINQ implements "send_inq" strategy
// Immediately send EMSI_INQ and wait for response
func (s *Session) handshakeInitialSendINQ() (string, string, error) {
	if s.debug {
		logging.Debugf("EMSI: Strategy=send_inq: Sending immediate EMSI_INQ...")
	}

	if err := s.sendEMSI_INQ(); err != nil {
		return "", "", fmt.Errorf("send_inq strategy: failed to send INQ: %w", err)
	}

	// Wait with StepTimeout (T1)
	response, responseType, err := s.readEMSIResponseWithTimeout(s.config.StepTimeout)

	// If SendINQTwice is enabled and no EMSI, send second INQ
	if (err != nil || responseType == "BANNER" || responseType == "UNKNOWN") && s.config.SendINQTwice {
		if s.debug {
			logging.Debugf("EMSI: Strategy=send_inq: Sending second EMSI_INQ...")
		}
		time.Sleep(s.config.INQInterval)
		if err := s.sendEMSI_INQ(); err != nil {
			return "", "", fmt.Errorf("send_inq strategy: failed to send second INQ: %w", err)
		}
		response, responseType, err = s.readEMSIResponseWithTimeout(s.config.StepTimeout)
	}

	if err != nil {
		return "", "", fmt.Errorf("send_inq strategy: %w", err)
	}

	return response, responseType, nil
}

// sendPreventiveINQ sends EMSI_INQ when we have banner but no EMSI response
// This is a Qico-style optimization to speed up handshake with slow remotes
func (s *Session) sendPreventiveINQ() (string, string, error) {
	if s.debug {
		logging.Debugf("EMSI: Sending preventive EMSI_INQ (PreventiveINQ enabled)...")
	}

	if err := s.sendEMSI_INQ(); err != nil {
		if s.debug {
			logging.Debugf("EMSI: ERROR sending preventive EMSI_INQ: %v", err)
		}
		return "", "", fmt.Errorf("failed to send preventive EMSI_INQ: %w", err)
	}

	// Wait with PreventiveINQTimeout (typically shorter than StepTimeout)
	timeout := s.config.PreventiveINQTimeout
	if timeout <= 0 {
		timeout = s.config.StepTimeout
	}

	if s.debug {
		logging.Debugf("EMSI: Preventive EMSI_INQ sent, waiting for response (timeout=%v)...", timeout)
	}

	response, responseType, err := s.readEMSIResponseWithTimeout(timeout)

	// If SendINQTwice is enabled and still no EMSI, send second INQ
	if (err != nil || responseType == "BANNER" || responseType == "UNKNOWN") && s.config.SendINQTwice {
		if s.debug {
			logging.Debugf("EMSI: Still no EMSI after preventive INQ, sending second EMSI_INQ...")
		}
		time.Sleep(s.config.INQInterval)
		if err := s.sendEMSI_INQ(); err != nil {
			return "", "", fmt.Errorf("failed to send second EMSI_INQ: %w", err)
		}
		response, responseType, err = s.readEMSIResponseWithTimeout(s.config.StepTimeout)
	}

	if err != nil {
		if s.debug {
			logging.Debugf("EMSI: ERROR reading response after preventive INQ: %v", err)
		}
		return "", "", fmt.Errorf("failed to read EMSI response: %w", err)
	}

	return response, responseType, nil
}

// sendEMSI_INQ sends EMSI inquiry
func (s *Session) sendEMSI_INQ() error {
	if s.debug {
		logging.Debugf("EMSI: sendEMSI_INQ: Sending EMSI_INQ")
	}

	deadline := time.Now().Add(s.config.MasterTimeout)
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

	_ = s.conn.SetWriteDeadline(time.Now().Add(s.config.MasterTimeout))
	
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
	
	// Create our EMSI data using configured protocols
	protocols := s.config.Protocols
	if len(protocols) == 0 {
		protocols = []string{"ZMO", "ZAP"} // Default protocols
	}

	data := &EMSIData{
		SystemName:    s.systemName,
		Location:      s.location,
		Sysop:         s.sysop,
		Phone:         "-Unpublished-",
		Speed:         "9600",                    // Numeric baud for compatibility
		Flags:         "CM,IFC,XA",               // Traditional flags: Continuous Mail, IFCICO, Mail only
		MailerName:    "NodelistDB",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",                     // Traditional OS identifier
		Addresses:     []string{s.localAddress},  // Bare address without @fidonet suffix
		Protocols:     protocols,
		Password:      "",                        // Empty password
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
	
	deadline := time.Now().Add(s.config.MasterTimeout)
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

	return nil
}

// readEMSIResponse reads and identifies EMSI response using the default StepTimeout
// Per FSC-0056.001: T1=20s wait for response, T2=60s max handshake time
func (s *Session) readEMSIResponse() (string, string, error) {
	return s.readEMSIResponseWithTimeout(s.config.StepTimeout)
}

// readEMSIResponseWithTimeout reads and identifies EMSI response with a specific timeout
// We read continuously until we find an EMSI sequence or timeout expires
func (s *Session) readEMSIResponseWithTimeout(timeout time.Duration) (string, string, error) {
	deadline := time.Now().Add(timeout)
	_ = s.conn.SetReadDeadline(deadline)

	if s.debug {
		logging.Debugf("EMSI: readEMSIResponse: Starting read with timeout %v (deadline: %v)", timeout, deadline.Format("15:04:05.000"))
	}

	// Accumulate response data, some systems send banner first
	var response strings.Builder
	buffer := make([]byte, 4096)
	totalBytesRead := 0
	startTime := time.Now()

	// Keep reading until timeout - don't limit attempts
	// Remote BBS may send banner for 30+ seconds before EMSI_REQ
	for time.Now().Before(deadline) {
		elapsed := time.Since(startTime)
		if s.debug && totalBytesRead == 0 && elapsed > 5*time.Second && int(elapsed.Seconds())%10 == 0 {
			logging.Debugf("EMSI: readEMSIResponse: Still waiting for data after %v...", elapsed)
		}

		n, err := s.reader.Read(buffer)

		if err != nil {
			elapsed := time.Since(startTime)
			if s.debug {
				logging.Debugf("EMSI: readEMSIResponse: Read error after %v: %v (bytes so far: %d)", elapsed, err, totalBytesRead)
			}
			// If we have accumulated data, check it before giving up
			if response.Len() > 0 {
				responseStr := response.String()
				if emsiType := s.detectEMSIType(responseStr); emsiType != "" {
					return responseStr, emsiType, nil
				}
				// Have data but no EMSI, return as banner
				return responseStr, "BANNER", nil
			}
			return "", "", fmt.Errorf("read failed after %v: %w", elapsed, err)
		}

		if n > 0 {
			chunk := string(buffer[:n])
			response.WriteString(chunk)
			totalBytesRead += n
			elapsed := time.Since(startTime)

			if s.debug {
				logging.Debugf("EMSI: readEMSIResponse: Received %d bytes (total: %d, elapsed: %v)",
					n, totalBytesRead, elapsed)
				if n < 200 {
					logging.Debugf("EMSI: readEMSIResponse: Data: %q", chunk)
				} else {
					logging.Debugf("EMSI: readEMSIResponse: Data (first 200): %q...", chunk[:200])
				}
			}

			responseStr := response.String()

			// Always update banner text with accumulated response to capture initial banner
			if len(responseStr) > len(s.bannerText) {
				s.bannerText = responseStr
			}

			// Check if we have EMSI sequences
			if emsiType := s.detectEMSIType(responseStr); emsiType != "" {
				if s.debug {
					logging.Debugf("EMSI: readEMSIResponse: Found EMSI_%s in response after %v", emsiType, elapsed)
				}
				return responseStr, emsiType, nil
			}

			if s.debug {
				logging.Debugf("EMSI: readEMSIResponse: No EMSI sequence found yet, continuing...")
			}
		}
	}

	// Timeout expired, check what we got
	responseStr := response.String()
	finalElapsed := time.Since(startTime)

	if len(responseStr) > 0 {
		// Got some data but no EMSI, treat as banner
		if s.debug {
			logging.Debugf("EMSI: readEMSIResponse: Timeout after %v, treating %d bytes as BANNER", finalElapsed, len(responseStr))
			if len(responseStr) < 500 {
				logging.Debugf("EMSI: readEMSIResponse: Banner content: %q", responseStr)
			} else {
				logging.Debugf("EMSI: readEMSIResponse: Banner content (first 500): %q...", responseStr[:500])
			}
		}
		return responseStr, "BANNER", nil
	}

	if s.debug {
		logging.Debugf("EMSI: readEMSIResponse: No data received after %v, returning timeout error", finalElapsed)
	}
	return "", "", fmt.Errorf("timeout waiting for EMSI response after %v", finalElapsed)
}

// detectEMSIType checks if the response contains an EMSI sequence and returns its type
func (s *Session) detectEMSIType(responseStr string) string {
	if strings.Contains(responseStr, "EMSI_NAK") {
		return "NAK"
	}
	if strings.Contains(responseStr, "EMSI_DAT") {
		return "DAT"
	}
	if strings.Contains(responseStr, "EMSI_ACK") {
		return "ACK"
	}
	if strings.Contains(responseStr, "EMSI_REQ") {
		return "REQ"
	}
	if strings.Contains(responseStr, "EMSI_CLI") {
		return "CLI"
	}
	if strings.Contains(responseStr, "EMSI_HBT") {
		return "HBT"
	}
	if strings.Contains(responseStr, "EMSI_INQ") {
		return "INQ"
	}
	return ""
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