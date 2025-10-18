package protocols

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/protocols/emsi"
)

// IfcicoTester implements IFCICO/EMSI protocol testing
type IfcicoTester struct {
	timeout      time.Duration
	ourAddress   string
	systemName   string
	sysop        string
	location     string
	defaultPort  int
	debug        bool
}

// NewIfcicoTester creates a new IFCICO tester
func NewIfcicoTester(timeout time.Duration, ourAddress string) *IfcicoTester {
	// Use a default address if none provided
	if ourAddress == "" {
		ourAddress = "2:5001/5001"
	}
	
	return &IfcicoTester{
		timeout:     timeout,
		ourAddress:  ourAddress,
		systemName:  "NodelistDB Test Daemon",
		sysop:       "Test Operator",
		location:    "Test Location",
		defaultPort: 60179,
		debug:       false, // Debug mode disabled by default
	}
}

// NewIfcicoTesterWithInfo creates a new IFCICO tester with custom system info
func NewIfcicoTesterWithInfo(timeout time.Duration, ourAddress, systemName, sysop, location string) *IfcicoTester {
	// Use a default address if none provided
	if ourAddress == "" {
		ourAddress = "2:5001/5001"
	}
	
	return &IfcicoTester{
		timeout:     timeout,
		ourAddress:  ourAddress,
		systemName:  systemName,
		sysop:       sysop,
		location:    location,
		defaultPort: 60179,
		debug:       false, // Debug mode disabled by default
	}
}

// GetProtocolName returns the protocol name
func (t *IfcicoTester) GetProtocolName() string {
	return "IFCICO"
}

// Test performs an IFCICO/EMSI connectivity test
func (t *IfcicoTester) Test(ctx context.Context, host string, port int, expectedAddress string) TestResult {
	startTime := time.Now()
	
	// Use default port if not specified
	if port == 0 {
		port = t.defaultPort
	}
	
	// Always log for debugging
	logging.Debugf("IFCICO: Testing %s:%d (expected address: %s) [debug=%v]", host, port, expectedAddress, t.debug)

	if t.debug {
		logging.Debugf("IFCICO: Debug mode is ON")
		logging.Debugf("IFCICO: Our system info: Address=%s, System='%s', Sysop='%s', Location='%s'",
			t.ourAddress, t.systemName, t.sysop, t.location)
		logging.Debugf("IFCICO: Connection timeout: %v", t.timeout)
	}
	
	// Create connection with timeout
	dialer := net.Dialer{
		Timeout: t.timeout,
	}
	
	if t.debug {
		logging.Debugf("IFCICO: Attempting TCP connection to %s:%d...", host, port)
	}
	
	connStart := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	connDuration := time.Since(connStart)
	
	if err != nil {
		if t.debug {
			logging.Debugf("IFCICO: Connection failed after %v: %v", connDuration, err)
		}
		return &IfcicoTestResult{
			BaseTestResult: BaseTestResult{
				Success:    false,
				Error:      fmt.Sprintf("connection failed after %v: %v", connDuration, err),
				ResponseMs: uint32(time.Since(startTime).Milliseconds()),
				TestTime:   startTime,
			},
		}
	}
	defer conn.Close()
	
	if t.debug {
		logging.Debugf("IFCICO: TCP connection established in %v", connDuration)
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			localAddr := tcpConn.LocalAddr()
			remoteAddr := tcpConn.RemoteAddr()
			logging.Debugf("IFCICO: Local: %s -> Remote: %s", localAddr, remoteAddr)
		}
	}
	
	// Create EMSI session with custom system info
	if t.debug {
		logging.Debugf("IFCICO: Creating EMSI session...")
	}
	session := emsi.NewSessionWithInfo(conn, t.ourAddress, t.systemName, t.sysop, t.location)
	session.SetTimeout(t.timeout)
	session.SetDebug(t.debug)

	// Perform EMSI handshake
	if t.debug {
		logging.Debugf("IFCICO: Starting EMSI handshake...")
	}
	
	handshakeStart := time.Now()
	err = session.Handshake()
	handshakeDuration := time.Since(handshakeStart)
	
	if err != nil {
		if t.debug {
			logging.Debugf("IFCICO: Handshake failed after %v: %v", handshakeDuration, err)
		}
		return &IfcicoTestResult{
			BaseTestResult: BaseTestResult{
				Success:    false,
				Error:      fmt.Sprintf("handshake failed after %v: %v", handshakeDuration, err),
				ResponseMs: uint32(time.Since(startTime).Milliseconds()),
				TestTime:   startTime,
			},
		}
	}
	
	if t.debug {
		logging.Debugf("IFCICO: Handshake completed successfully in %v", handshakeDuration)
	}
	
	// Get remote node information
	remoteInfo := session.GetRemoteInfo()
	
	// Build result
	result := &IfcicoTestResult{
		BaseTestResult: BaseTestResult{
			Success:    true,
			Error:      "",
			ResponseMs: uint32(time.Since(startTime).Milliseconds()),
			TestTime:   startTime,
		},
		ResponseType: "EMSI",
		AddressValid: false, // Initialize to false
	}
	
	if remoteInfo != nil {
		result.SystemName = remoteInfo.SystemName
		result.MailerInfo = fmt.Sprintf("%s %s", remoteInfo.MailerName, remoteInfo.MailerVersion)
		result.Addresses = remoteInfo.Addresses
		
		// Additional info - don't duplicate location if it's already in system name
		if remoteInfo.Location != "" && !strings.Contains(result.SystemName, remoteInfo.Location) {
			result.SystemName = fmt.Sprintf("%s (%s)", result.SystemName, remoteInfo.Location)
		}
		
		// Debug log the details we got
		if t.debug || (result.SystemName == "" && result.MailerInfo == " ") {
			logging.Debugf("IFCICO remote info for %s: SystemName=%s, MailerName=%s, MailerVersion=%s, Location=%s, Addresses=%v",
				expectedAddress, remoteInfo.SystemName, remoteInfo.MailerName, remoteInfo.MailerVersion,
				remoteInfo.Location, remoteInfo.Addresses)
		}

		// Validate address if expected
		if expectedAddress != "" {
			result.AddressValid = session.ValidateAddress(expectedAddress)
			if t.debug {
				logging.Debugf("IFCICO: Address validation: expected=%s, received=%v, valid=%v",
					expectedAddress, remoteInfo.Addresses, result.AddressValid)
			}
		}
	} else {
		if t.debug {
			logging.Debugf("IFCICO: WARNING: Handshake completed but no remote info received for %s", expectedAddress)
			logging.Debugf("IFCICO: This may indicate the remote system didn't send valid EMSI_DAT")
		}
		// Mark as partial success - connection worked but no data exchanged
		result.SystemName = "[No EMSI data received]"
		result.MailerInfo = "[Unknown]"
	}
	
	// Close session gracefully
	if t.debug {
		logging.Debugf("IFCICO: Closing session gracefully...")
	}
	session.Close()

	if t.debug {
		totalDuration := time.Since(startTime)
		logging.Debugf("IFCICO: Test completed in %v, Success=%v", totalDuration, result.Success)
		if result.Success {
			logging.Debugf("IFCICO: Result: System='%s', Mailer='%s', Addresses=%v",
				result.SystemName, result.MailerInfo, result.Addresses)
		}
	}
	
	return result
}

// SetDebug enables or disables debug mode
func (t *IfcicoTester) SetDebug(enabled bool) {
	t.debug = enabled
}

