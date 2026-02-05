package emsi

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
)

// CompletionReason indicates how the EMSI handshake finished
type CompletionReason string

const (
	ReasonNone      CompletionReason = ""           // Not completed yet
	ReasonComplete  CompletionReason = "COMPLETE"   // Normal handshake finished
	ReasonNCP       CompletionReason = "NCP"        // No Compatible Protocols (test mode)
	ReasonTimeout   CompletionReason = "TIMEOUT"    // Timed out waiting for response
	ReasonError     CompletionReason = "ERROR"      // Error during handshake
)

// DebugFunc is a callback for routing debug output to an external logger.
type DebugFunc func(format string, args ...interface{})

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
	debugFunc    DebugFunc // Optional external debug logger
	bannerText   string    // Capture full banner text during handshake

	// Completion info
	completionReason CompletionReason
	handshakeTiming  HandshakeTiming

	// EMSI-II negotiation state (FSC-0088.001)
	emsi2Negotiated  bool   // Both sides presented EII
	selectedProtocol string // Final negotiated protocol
}

// HandshakeTiming records timing for each handshake phase
type HandshakeTiming struct {
	InitialPhase  time.Duration // Time to get first EMSI response
	DATExchange   time.Duration // Time for DAT packet exchange
	Total         time.Duration // Total handshake time
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

// SetDebugFunc sets an external debug logging function.
// When set, debug output is routed through this function instead of
// the internal testing/logging package. This allows callers (e.g. modem-test)
// to route EMSI debug output through their own logger.
func (s *Session) SetDebugFunc(fn DebugFunc) {
	s.debugFunc = fn
}

// dbg emits a debug message if debug mode is enabled.
// Routes to debugFunc if set, otherwise falls back to internal logger.
func (s *Session) dbg(format string, args ...interface{}) {
	if !s.debug {
		return
	}
	if s.debugFunc != nil {
		s.debugFunc(format, args...)
		return
	}
	logging.Debugf(format, args...)
}

// GetCompletionReason returns how the handshake finished
func (s *Session) GetCompletionReason() CompletionReason {
	return s.completionReason
}

// GetHandshakeTiming returns timing information for the handshake phases
func (s *Session) GetHandshakeTiming() HandshakeTiming {
	return s.handshakeTiming
}

// IsEMSI2Mode returns true if EMSI-II mode was negotiated (both sides presented EII)
func (s *Session) IsEMSI2Mode() bool {
	return s.emsi2Negotiated
}

// GetSelectedProtocol returns the negotiated file transfer protocol
func (s *Session) GetSelectedProtocol() string {
	return s.selectedProtocol
}

// negotiateEMSI2 determines if EMSI-II mode is active based on mutual EII flag presence
func (s *Session) negotiateEMSI2() {
	// EMSI-II mode requires both sides to present EII
	if s.config.EnableEMSI2 && s.remoteInfo != nil && s.remoteInfo.HasEII {
		s.emsi2Negotiated = true
		if s.debug {
			s.dbg("EMSI: EMSI-II mode negotiated (both sides presented EII)")
		}
	} else {
		s.emsi2Negotiated = false
		if s.debug {
			if !s.config.EnableEMSI2 {
				s.dbg("EMSI: EMSI-I mode (local EII disabled)")
			} else if s.remoteInfo == nil {
				s.dbg("EMSI: EMSI-I mode (no remote info)")
			} else {
				s.dbg("EMSI: EMSI-I mode (remote did not present EII)")
			}
		}
	}
}

// selectProtocol implements protocol selection per FSC-0056.001 and FSC-0088.001
// Returns the first protocol from our list that remote also supports
func (s *Session) selectProtocol() string {
	if s.remoteInfo == nil || len(s.remoteInfo.Protocols) == 0 {
		return ""
	}

	// FSC-0088.001: EMSI-II mandates caller-prefs (caller lists in preference order)
	// FSC-0056.001: EMSI-I was ambiguous, but we use caller-prefs as well
	// CallerPrefsMode controls whether we strictly enforce this
	if s.config.CallerPrefsMode || s.emsi2Negotiated {
		// Caller-prefs: use OUR protocol order, find first match in remote's list
		for _, ourProto := range s.config.Protocols {
			for _, theirProto := range s.remoteInfo.Protocols {
				if strings.EqualFold(ourProto, theirProto) {
					s.selectedProtocol = ourProto
					if s.debug {
						s.dbg("EMSI: Selected protocol %s (caller-prefs)", ourProto)
					}
					return ourProto
				}
			}
		}
	} else {
		// Answerer-prefs (legacy EMSI-I): use THEIR protocol order
		for _, theirProto := range s.remoteInfo.Protocols {
			for _, ourProto := range s.config.Protocols {
				if strings.EqualFold(ourProto, theirProto) {
					s.selectedProtocol = ourProto
					if s.debug {
						s.dbg("EMSI: Selected protocol %s (answerer-prefs)", ourProto)
					}
					return ourProto
				}
			}
		}
	}

	if s.debug {
		s.dbg("EMSI: No compatible protocol found (NCP)")
	}
	return "" // NCP - No Compatible Protocol
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
	handshakeStart := time.Now()
	s.completionReason = ReasonNone

	if s.debug {
		s.dbg("EMSI: === Starting EMSI Handshake ===")
		s.dbg("EMSI: Strategy: %s, PreventiveINQ: %v", s.config.InitialStrategy, s.config.PreventiveINQ)
		s.dbg("EMSI: Master timeout: %v, Step timeout: %v", s.config.MasterTimeout, s.config.StepTimeout)
		s.dbg("EMSI: Our address: %s", s.localAddress)
	}

	var response string
	var responseType string
	var err error

	initialPhaseStart := time.Now()

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
		s.completionReason = ReasonError
		s.handshakeTiming.Total = time.Since(handshakeStart)
		return err
	}

	// If we got banner but no EMSI and PreventiveINQ is enabled,
	// send EMSI_INQ to speed up handshake
	if (responseType == "BANNER" || responseType == "UNKNOWN") && s.config.PreventiveINQ {
		response, responseType, err = s.sendPreventiveINQ()
		if err != nil {
			s.completionReason = ReasonError
			s.handshakeTiming.Total = time.Since(handshakeStart)
			return err
		}
	}

	s.handshakeTiming.InitialPhase = time.Since(initialPhaseStart)
	datExchangeStart := time.Now()

	if s.debug {
		s.dbg("EMSI: Initial phase completed in %v", s.handshakeTiming.InitialPhase)
		s.dbg("EMSI: Received response type: %s", responseType)
		if len(response) > 0 && len(response) < 500 {
			s.dbg("EMSI: Response data (first 500 chars): %q", response)
		} else if len(response) > 0 {
			s.dbg("EMSI: Response data (first 500 chars): %q...", response[:500])
		}
	}

	switch responseType {
	case "REQ":
		// Remote wants our EMSI_DAT
		if s.debug {
			s.dbg("EMSI: Remote requested our data (EMSI_REQ), sending EMSI_DAT...")
		}
		if err := s.sendEMSI_DAT(); err != nil {
			if s.debug {
				s.dbg("EMSI: ERROR sending EMSI_DAT: %v", err)
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
			s.dbg("EMSI: Waiting for remote's EMSI_DAT (max %d attempts)...", maxAttempts)
		}
		for i := 0; i < maxAttempts; i++ {
			if s.debug {
				s.dbg("EMSI: Reading response attempt %d/%d...", i+1, maxAttempts)
			}
			response, responseType, err = s.readEMSIResponseWithTimeout(s.config.StepTimeout)
			if err != nil {
				if s.debug {
					s.dbg("EMSI: ERROR reading response (attempt %d): %v", i+1, err)
				}
				return fmt.Errorf("failed to read EMSI_DAT response (attempt %d): %w", i+1, err)
			}
			
			if s.debug {
				s.dbg("EMSI: After sending DAT, received response type: %s (attempt %d)", responseType, i+1)
			}
			
			if responseType == "DAT" {
				// Parse remote's EMSI_DAT
				if s.debug {
					s.dbg("EMSI: Received remote's EMSI_DAT, parsing...")
				}
				var parseErr error
				s.remoteInfo, parseErr = ParseEMSI_DAT(response)
				if s.debug {
					if parseErr != nil {
						s.dbg("EMSI: ERROR parsing remote data: %v", parseErr)
					} else {
						s.dbg("EMSI: Successfully parsed remote data:")
						s.dbg("EMSI:   System: %s", s.remoteInfo.SystemName)
						s.dbg("EMSI:   Location: %s", s.remoteInfo.Location)
						s.dbg("EMSI:   Sysop: %s", s.remoteInfo.Sysop)
						s.dbg("EMSI:   Mailer: %s %s", s.remoteInfo.MailerName, s.remoteInfo.MailerVersion)
						s.dbg("EMSI:   Addresses: %v", s.remoteInfo.Addresses)
					}
				}
				// Send ACK
				if err := s.sendEMSI_ACK(); err != nil {
					if s.debug {
						s.dbg("EMSI: WARNING: Failed to send ACK: %v", err)
					}
				} else if s.debug {
					s.dbg("EMSI: Sent ACK for remote's DAT")
				}
				break
			} else if responseType == "REQ" {
				// They're still requesting, send our DAT again
				if s.debug {
					s.dbg("EMSI: Remote sent another REQ, resending our DAT")
				}
				if err := s.sendEMSI_DAT(); err != nil {
					return fmt.Errorf("failed to resend EMSI_DAT: %w", err)
				}
			} else if responseType == "ACK" {
				// They acknowledged but didn't send their DAT yet
				if s.debug {
					s.dbg("EMSI: Remote sent ACK, waiting for DAT")
				}
				continue
			} else if responseType == "NAK" {
				// Remote is rejecting our DAT - there might be a CRC or format issue
				if s.debug {
					s.dbg("EMSI: Remote sent NAK, our EMSI_DAT might be invalid")
				}
				// Some implementations might still work after NAK, keep trying
				continue
			}
		}

		if s.remoteInfo == nil {
			if s.debug {
				s.dbg("EMSI: WARNING: Failed to receive remote DAT after %d attempts", maxAttempts)
				s.dbg("EMSI: Attempting banner text extraction as fallback...")
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
					s.dbg("EMSI: Successfully extracted software from banner: %s %s", software.Name, software.Version)
				}
			} else if s.debug {
				s.dbg("EMSI: Handshake incomplete - no remote data received and banner extraction failed")
			}
		} else if s.debug {
			s.dbg("EMSI: Handshake completed successfully after REQ exchange")
		}
		
	case "DAT":
		// Remote sent their EMSI_DAT directly
		if s.debug {
			s.dbg("EMSI: Remote sent EMSI_DAT directly, parsing...")
		}
		var parseErr error
		s.remoteInfo, parseErr = ParseEMSI_DAT(response)
		if s.debug {
			if parseErr != nil {
				s.dbg("EMSI: ERROR parsing remote data: %v", parseErr)
			} else {
				s.dbg("EMSI: Successfully parsed remote data:")
				s.dbg("EMSI:   System: %s", s.remoteInfo.SystemName)
				s.dbg("EMSI:   Location: %s", s.remoteInfo.Location)
				s.dbg("EMSI:   Sysop: %s", s.remoteInfo.Sysop)
				s.dbg("EMSI:   Mailer: %s %s", s.remoteInfo.MailerName, s.remoteInfo.MailerVersion)
				s.dbg("EMSI:   Addresses: %v", s.remoteInfo.Addresses)
			}
		}
		
		// Send our EMSI_DAT in response
		if s.debug {
			s.dbg("EMSI: Sending our EMSI_DAT in response...")
		}
		if err := s.sendEMSI_DAT(); err != nil {
			if s.debug {
				s.dbg("EMSI: ERROR sending EMSI_DAT: %v", err)
			}
			return fmt.Errorf("failed to send EMSI_DAT: %w", err)
		}
		
		// Send ACK
		if err := s.sendEMSI_ACK(); err != nil {
			if s.debug {
				s.dbg("EMSI: WARNING: Failed to send ACK: %v", err)
			}
		} else if s.debug {
			s.dbg("EMSI: Sent ACK for remote's DAT")
			s.dbg("EMSI: Handshake completed successfully after DAT exchange")
		}
		
	case "NAK":
		s.completionReason = ReasonError
		s.handshakeTiming.Total = time.Since(handshakeStart)
		return fmt.Errorf("remote sent EMSI_NAK")

	case "CLI":
		s.completionReason = ReasonError
		s.handshakeTiming.Total = time.Since(handshakeStart)
		return fmt.Errorf("remote sent EMSI_CLI (calling system only)")

	case "HBT":
		s.completionReason = ReasonError
		s.handshakeTiming.Total = time.Since(handshakeStart)
		return fmt.Errorf("remote sent EMSI_HBT (heartbeat)")

	default:
		if s.debug {
			s.dbg("EMSI: ERROR: Unexpected response type: %s", responseType)
		}
		s.completionReason = ReasonTimeout
		s.handshakeTiming.Total = time.Since(handshakeStart)
		// Include the actual data received for BANNER/UNKNOWN to help diagnose
		if (responseType == "BANNER" || responseType == "UNKNOWN") && len(response) > 0 {
			preview := formatResponsePreview(response, 200)
			return fmt.Errorf("unexpected response type: %s\nReceived %d bytes: %s", responseType, len(response), preview)
		}
		return fmt.Errorf("unexpected response type: %s", responseType)
	}

	// Record timing
	s.handshakeTiming.DATExchange = time.Since(datExchangeStart)
	s.handshakeTiming.Total = time.Since(handshakeStart)

	// EMSI-II negotiation (FSC-0088.001)
	s.negotiateEMSI2()
	s.selectProtocol()

	// Determine completion reason based on protocol negotiation
	if len(s.config.Protocols) == 0 {
		s.completionReason = ReasonNCP // No Compatible Protocols (test mode)
	} else if s.selectedProtocol == "" && len(s.config.Protocols) > 0 {
		// We have protocols but couldn't negotiate one
		s.completionReason = ReasonNCP
	} else {
		s.completionReason = ReasonComplete
	}

	if s.debug {
		s.dbg("EMSI: === Handshake Complete (%s) ===", s.completionReason)
		s.dbg("EMSI: Timing: Initial=%v, DATExchange=%v, Total=%v",
			s.handshakeTiming.InitialPhase, s.handshakeTiming.DATExchange, s.handshakeTiming.Total)
	}

	return nil
}

// handshakeInitialWait implements FSC-0056.001 "wait" strategy
// Wait for remote to send EMSI_INQ or EMSI_REQ first
func (s *Session) handshakeInitialWait() (string, string, error) {
	if s.debug {
		s.dbg("EMSI: Strategy=wait: Waiting for remote EMSI (timeout=%v)...", s.config.StepTimeout)
	}

	response, responseType, err := s.readEMSIResponseWithTimeout(s.config.StepTimeout)
	if err != nil {
		if s.debug {
			s.dbg("EMSI: Strategy=wait: No response, error: %v", err)
		}
		return "", "", fmt.Errorf("wait strategy: %w", err)
	}

	return response, responseType, nil
}

// handshakeInitialSendCR implements "send_cr" strategy
// Send CRs to wake remote BBS, then wait for EMSI
func (s *Session) handshakeInitialSendCR() (string, string, error) {
	if s.debug {
		s.dbg("EMSI: Strategy=send_cr: Sending CRs to trigger remote EMSI...")
	}

	initialWait := s.config.InitialCRTimeout
	deadline := time.Now().Add(initialWait)
	gotData := false

	for time.Now().Before(deadline) {
		_ = s.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if _, err := s.writer.WriteString("\r"); err != nil {
			if s.debug {
				s.dbg("EMSI: Strategy=send_cr: ERROR sending CR: %v", err)
			}
			return "", "", fmt.Errorf("failed to send CR: %w", err)
		}
		if err := s.writer.Flush(); err != nil {
			if s.debug {
				s.dbg("EMSI: Strategy=send_cr: ERROR flushing CR: %v", err)
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
				s.dbg("EMSI: Strategy=send_cr: Got initial data from remote")
			}
			break
		}
	}

	if !gotData {
		if s.debug {
			s.dbg("EMSI: Strategy=send_cr: No data received after %v", initialWait)
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
		s.dbg("EMSI: Strategy=send_inq: Sending immediate EMSI_INQ...")
	}

	if err := s.sendEMSI_INQ(); err != nil {
		return "", "", fmt.Errorf("send_inq strategy: failed to send INQ: %w", err)
	}

	// Wait with StepTimeout (T1)
	response, responseType, err := s.readEMSIResponseWithTimeout(s.config.StepTimeout)

	// If SendINQTwice is enabled and no EMSI, send second INQ
	if (err != nil || responseType == "BANNER" || responseType == "UNKNOWN") && s.config.SendINQTwice {
		if s.debug {
			s.dbg("EMSI: Strategy=send_inq: Sending second EMSI_INQ...")
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
		s.dbg("EMSI: Sending preventive EMSI_INQ (PreventiveINQ enabled)...")
	}

	if err := s.sendEMSI_INQ(); err != nil {
		if s.debug {
			s.dbg("EMSI: ERROR sending preventive EMSI_INQ: %v", err)
		}
		return "", "", fmt.Errorf("failed to send preventive EMSI_INQ: %w", err)
	}

	// Wait with PreventiveINQTimeout (typically shorter than StepTimeout)
	timeout := s.config.PreventiveINQTimeout
	if timeout <= 0 {
		timeout = s.config.StepTimeout
	}

	if s.debug {
		s.dbg("EMSI: Preventive EMSI_INQ sent, waiting for response (timeout=%v)...", timeout)
	}

	response, responseType, err := s.readEMSIResponseWithTimeout(timeout)

	// If SendINQTwice is enabled and still no EMSI, send second INQ
	if (err != nil || responseType == "BANNER" || responseType == "UNKNOWN") && s.config.SendINQTwice {
		if s.debug {
			s.dbg("EMSI: Still no EMSI after preventive INQ, sending second EMSI_INQ...")
		}
		time.Sleep(s.config.INQInterval)
		if err := s.sendEMSI_INQ(); err != nil {
			return "", "", fmt.Errorf("failed to send second EMSI_INQ: %w", err)
		}
		response, responseType, err = s.readEMSIResponseWithTimeout(s.config.StepTimeout)
	}

	if err != nil {
		if s.debug {
			s.dbg("EMSI: ERROR reading response after preventive INQ: %v", err)
		}
		return "", "", fmt.Errorf("failed to read EMSI response: %w", err)
	}

	return response, responseType, nil
}

// sendEMSI_INQ sends EMSI inquiry
func (s *Session) sendEMSI_INQ() error {
	if s.debug {
		s.dbg("EMSI: sendEMSI_INQ: Sending EMSI_INQ")
	}

	deadline := time.Now().Add(s.config.MasterTimeout)
	_ = s.conn.SetWriteDeadline(deadline)

	if _, err := s.writer.WriteString(EMSI_INQ + "\r"); err != nil {
		if s.debug {
			s.dbg("EMSI: sendEMSI_INQ: ERROR writing: %v", err)
		}
		return err
	}
	
	if err := s.writer.Flush(); err != nil {
		if s.debug {
			s.dbg("EMSI: sendEMSI_INQ: ERROR flushing: %v", err)
		}
		return err
	}
	
	if s.debug {
		s.dbg("EMSI: sendEMSI_INQ: Successfully sent EMSI_INQ")
	}
	return nil
}

// sendEMSI_ACK sends EMSI acknowledgment
func (s *Session) sendEMSI_ACK() error {
	if s.debug {
		s.dbg("EMSI: Sending EMSI_ACK")
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
		s.dbg("EMSI: sendEMSI_DAT: Preparing EMSI_DAT packet...")
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
		Speed:         "9600",                   // Numeric baud for compatibility
		Flags:         "CM,IFC,XA",              // Traditional flags: Continuous Mail, IFCICO, Mail only
		MailerName:    "NodelistDB",
		MailerVersion: "1.0",
		MailerSerial:  "LNX",                    // Traditional OS identifier
		Addresses:     []string{s.localAddress}, // Bare address without @fidonet suffix
		Protocols:     protocols,
		Password:      "",                       // Empty password
	}

	// Use config-aware packet builder for EMSI-II support
	packet := CreateEMSI_DATWithConfig(data, s.config)
	
	if s.debug {
		s.dbg("EMSI: sendEMSI_DAT: Created EMSI_DAT packet (%d bytes)", len(packet))
		// Log first 200 chars of packet for debugging
		if len(packet) > 200 {
			s.dbg("EMSI: sendEMSI_DAT: DAT packet (first 200): %q", packet[:200])
		} else {
			s.dbg("EMSI: sendEMSI_DAT: DAT packet: %q", packet)
		}
		// Also log what we're actually writing including CRs
		fullPacket := packet + "\r\r"
		s.dbg("EMSI: sendEMSI_DAT: Full packet with terminators (%d bytes)", len(fullPacket))
	}
	
	deadline := time.Now().Add(s.config.MasterTimeout)
	_ = s.conn.SetWriteDeadline(deadline)

	if s.debug {
		s.dbg("EMSI: sendEMSI_DAT: Sending packet with deadline %v...", deadline.Format("15:04:05.000"))
	}
	
	// Send the packet with CR (binkleyforce-compatible, ifcico drops XON anyway)
	if _, err := s.writer.WriteString(packet + "\r"); err != nil {
		if s.debug {
			s.dbg("EMSI: sendEMSI_DAT: ERROR writing packet: %v", err)
		}
		return fmt.Errorf("failed to write EMSI_DAT: %w", err)
	}

	// Send additional CR (some mailers expect double CR)
	if _, err := s.writer.WriteString("\r"); err != nil {
		if s.debug {
			s.dbg("EMSI: sendEMSI_DAT: ERROR writing additional CR: %v", err)
		}
		return fmt.Errorf("failed to write additional CR: %w", err)
	}
	
	if err := s.writer.Flush(); err != nil {
		if s.debug {
			s.dbg("EMSI: sendEMSI_DAT: ERROR flushing buffer: %v", err)
		}
		return fmt.Errorf("failed to flush EMSI_DAT: %w", err)
	}
	
	if s.debug {
		s.dbg("EMSI: sendEMSI_DAT: Successfully sent EMSI_DAT packet")
	}

	return nil
}

// readEMSIResponseWithTimeout reads and identifies EMSI response with a specific timeout
// We read continuously until we find an EMSI sequence or timeout expires
func (s *Session) readEMSIResponseWithTimeout(timeout time.Duration) (string, string, error) {
	deadline := time.Now().Add(timeout)
	_ = s.conn.SetReadDeadline(deadline)

	if s.debug {
		s.dbg("EMSI: readEMSIResponse: Starting read with timeout %v (deadline: %v)", timeout, deadline.Format("15:04:05.000"))
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
			s.dbg("EMSI: readEMSIResponse: Still waiting for data after %v...", elapsed)
		}

		n, err := s.reader.Read(buffer)

		if err != nil {
			elapsed := time.Since(startTime)
			if s.debug {
				s.dbg("EMSI: readEMSIResponse: Read error after %v: %v (bytes so far: %d)", elapsed, err, totalBytesRead)
			}
			// If we have accumulated data, check it before giving up
			if response.Len() > 0 {
				responseStr := response.String()
				if signal := detectModemDisconnect(responseStr); signal != "" {
					return responseStr, "", fmt.Errorf("modem disconnect: %s", signal)
				}
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
				s.dbg("EMSI: readEMSIResponse: Received %d bytes (total: %d, elapsed: %v)",
					n, totalBytesRead, elapsed)
				if n < 200 {
					s.dbg("EMSI: readEMSIResponse: Data: %q", chunk)
				} else {
					s.dbg("EMSI: readEMSIResponse: Data (first 200): %q...", chunk[:200])
				}
			}

			responseStr := response.String()

			// Always update banner text with accumulated response to capture initial banner
			if len(responseStr) > len(s.bannerText) {
				s.bannerText = responseStr
			}

			// Check for modem disconnect signals before EMSI detection.
			// NO CARRIER can arrive in the same read as EMSI data when the
			// remote drops carrier immediately after sending EMSI_REQ.
			if signal := detectModemDisconnect(responseStr); signal != "" {
				if s.debug {
					s.dbg("EMSI: readEMSIResponse: Modem disconnect (%s) detected after %v", signal, elapsed)
				}
				return responseStr, "", fmt.Errorf("modem disconnect: %s", signal)
			}

			// Check if we have EMSI sequences
			if emsiType := s.detectEMSIType(responseStr); emsiType != "" {
				// For EMSI_DAT, wait until the full packet arrives (length field tells us how much)
				if emsiType == "DAT" && !isEMSI_DATComplete(responseStr) {
					if s.debug {
						s.dbg("EMSI: readEMSIResponse: EMSI_DAT detected but incomplete, continuing read...")
					}
					continue
				}
				if s.debug {
					s.dbg("EMSI: readEMSIResponse: Found EMSI_%s in response after %v", emsiType, elapsed)
				}
				return responseStr, emsiType, nil
			}

			if s.debug {
				s.dbg("EMSI: readEMSIResponse: No EMSI sequence found yet, continuing...")
			}
		}
	}

	// Timeout expired, check what we got
	responseStr := response.String()
	finalElapsed := time.Since(startTime)

	if len(responseStr) > 0 {
		// Got some data but no EMSI, treat as banner
		if s.debug {
			s.dbg("EMSI: readEMSIResponse: Timeout after %v, treating %d bytes as BANNER", finalElapsed, len(responseStr))
			if len(responseStr) < 500 {
				s.dbg("EMSI: readEMSIResponse: Banner content: %q", responseStr)
			} else {
				s.dbg("EMSI: readEMSIResponse: Banner content (first 500): %q...", responseStr[:500])
			}
		}
		return responseStr, "BANNER", nil
	}

	if s.debug {
		s.dbg("EMSI: readEMSIResponse: No data received after %v, returning timeout error", finalElapsed)
	}
	return "", "", fmt.Errorf("timeout waiting for EMSI response after %v", finalElapsed)
}

// detectModemDisconnect checks if the response contains a modem disconnect signal.
// Returns the signal string (e.g. "NO CARRIER") or empty string if none found.
func detectModemDisconnect(responseStr string) string {
	for _, signal := range []string{"NO CARRIER", "NO DIALTONE", "NO DIAL TONE", "BUSY", "NO ANSWER"} {
		if strings.Contains(responseStr, signal) {
			return signal
		}
	}
	return ""
}

// isEMSI_DATComplete checks if the buffer contains a complete EMSI_DAT packet.
// EMSI_DAT format: **EMSI_DATxxxx<data><CRC4>\r  where xxxx is hex length of <data>.
// Total expected: len up to "**EMSI_DAT" + 4 (len field) + dataLen + 4 (CRC) + 1 (\r)
func isEMSI_DATComplete(responseStr string) bool {
	idx := strings.Index(responseStr, "EMSI_DAT")
	if idx < 0 {
		return false
	}
	// Need at least EMSI_DAT + 4 hex digits for the length field
	hdrEnd := idx + len("EMSI_DAT") + 4 // position after length field
	if len(responseStr) < hdrEnd {
		return false
	}
	lenHex := responseStr[idx+len("EMSI_DAT") : hdrEnd]
	dataLen, err := strconv.ParseInt(lenHex, 16, 32)
	if err != nil {
		return true // can't parse length, return what we have
	}
	// Full packet: up to hdrEnd + dataLen + 4 (CRC hex)
	needed := hdrEnd + int(dataLen) + 4
	return len(responseStr) >= needed
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

	// Remove .0 point suffix (point 0 is implicit)
	// e.g., 2:222/0.0 -> 2:222/0
	addr = strings.TrimSuffix(addr, ".0")

	return addr
}
// formatResponsePreview formats response data for error messages.
// Returns text if data is 7-bit ASCII printable, otherwise hex dump.
func formatResponsePreview(data string, maxLen int) string {
	if len(data) == 0 {
		return "(empty)"
	}

	// Check if data is 7-bit ASCII printable (or common control chars)
	is7BitASCII := true
	for _, b := range []byte(data) {
		// Allow printable ASCII (32-126), tab, CR, LF
		if b < 32 && b != '\t' && b != '\r' && b != '\n' {
			is7BitASCII = false
			break
		}
		if b > 126 {
			is7BitASCII = false
			break
		}
	}

	if is7BitASCII {
		// Show as text, truncate if needed
		preview := data
		if len(preview) > maxLen {
			preview = preview[:maxLen] + "..."
		}
		// Replace control chars for display
		preview = strings.ReplaceAll(preview, "\r\n", "\\r\\n\n")
		preview = strings.ReplaceAll(preview, "\r", "\\r\n")
		preview = strings.ReplaceAll(preview, "\n", "\\n\n")
		return preview
	}

	// Show as hex dump
	hexLen := maxLen / 3 // Each byte takes ~3 chars in hex
	if hexLen > len(data) {
		hexLen = len(data)
	}

	var hex strings.Builder
	for i := 0; i < hexLen; i++ {
		if i > 0 {
			hex.WriteByte(' ')
		}
		fmt.Fprintf(&hex, "%02X", data[i])
	}
	if hexLen < len(data) {
		hex.WriteString("...")
	}
	return hex.String()
}