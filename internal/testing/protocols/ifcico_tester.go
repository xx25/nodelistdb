package protocols

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"
	
	"github.com/nodelistdb/internal/testing/protocols/emsi"
)

// IfcicoTester implements IFCICO/EMSI protocol testing
type IfcicoTester struct {
	timeout      time.Duration
	ourAddress   string
	defaultPort  int
	debug        bool
}

// NewIfcicoTester creates a new IFCICO tester
func NewIfcicoTester(timeout time.Duration, ourAddress string) *IfcicoTester {
	// Enable debug mode if DEBUG_EMSI env var is set
	debug := os.Getenv("DEBUG_EMSI") != ""
	
	// Use a default address if none provided
	if ourAddress == "" {
		ourAddress = "2:5001/5001"
	}
	
	return &IfcicoTester{
		timeout:     timeout,
		ourAddress:  ourAddress,
		defaultPort: 60179,
		debug:       debug,
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
	
	if t.debug {
		log.Printf("IFCICO: Testing %s:%d (expected address: %s)", host, port, expectedAddress)
	}
	
	// Create connection with timeout
	dialer := net.Dialer{
		Timeout: t.timeout,
	}
	
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return &IfcicoTestResult{
			BaseTestResult: BaseTestResult{
				Success:    false,
				Error:      fmt.Sprintf("connection failed: %v", err),
				ResponseMs: uint32(time.Since(startTime).Milliseconds()),
				TestTime:   startTime,
			},
		}
	}
	defer conn.Close()
	
	// Create EMSI session
	session := emsi.NewSession(conn, t.ourAddress)
	session.SetTimeout(t.timeout)
	session.SetDebug(t.debug)
	
	// Perform EMSI handshake
	err = session.Handshake()
	if err != nil {
		return &IfcicoTestResult{
			BaseTestResult: BaseTestResult{
				Success:    false,
				Error:      fmt.Sprintf("handshake failed: %v", err),
				ResponseMs: uint32(time.Since(startTime).Milliseconds()),
				TestTime:   startTime,
			},
		}
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
	}
	
	if remoteInfo != nil {
		result.SystemName = remoteInfo.SystemName
		result.MailerInfo = fmt.Sprintf("%s %s", remoteInfo.MailerName, remoteInfo.MailerVersion)
		result.Addresses = remoteInfo.Addresses
		
		// Additional info
		if remoteInfo.Location != "" {
			result.SystemName = fmt.Sprintf("%s (%s)", result.SystemName, remoteInfo.Location)
		}
		
		// Validate address if expected
		if expectedAddress != "" {
			addressValid := session.ValidateAddress(expectedAddress)
			if t.debug {
				log.Printf("IFCICO: Address validation: expected=%s, received=%v, valid=%v",
					expectedAddress, remoteInfo.Addresses, addressValid)
			}
		}
	}
	
	// Close session gracefully
	session.Close()
	
	return result
}

