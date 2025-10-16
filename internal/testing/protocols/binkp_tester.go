package protocols

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/nodelistdb/internal/testing/protocols/binkp"
)

// BinkPTester tests BinkP protocol connectivity
type BinkPTester struct {
	timeout     time.Duration
	ourAddress  string
	systemName  string
	sysop       string
	location    string
	defaultPort int
	debug       bool
}

// GetProtocolName returns the protocol name
func (t *BinkPTester) GetProtocolName() string {
	return "BinkP"
}

// NewBinkPTester creates a new BinkP tester
func NewBinkPTester(timeout time.Duration, ourAddress string) *BinkPTester {
	// Enable debug mode if DEBUG_BINKP env var is set
	debug := os.Getenv("DEBUG_BINKP") != ""
	
	return &BinkPTester{
		timeout:     timeout,
		ourAddress:  ourAddress,
		systemName:  "NodelistDB Test Daemon",
		sysop:       "Test Operator",
		location:    "Test Location",
		defaultPort: 24554,
		debug:       debug,
	}
}

// NewBinkPTesterWithInfo creates a new BinkP tester with custom system info
func NewBinkPTesterWithInfo(timeout time.Duration, ourAddress, systemName, sysop, location string) *BinkPTester {
	// Enable debug mode if DEBUG_BINKP env var is set
	debug := os.Getenv("DEBUG_BINKP") != ""
	
	return &BinkPTester{
		timeout:     timeout,
		ourAddress:  ourAddress,
		systemName:  systemName,
		sysop:       sysop,
		location:    location,
		defaultPort: 24554,
		debug:       debug,
	}
}

// Test performs a BinkP connectivity test
func (t *BinkPTester) Test(ctx context.Context, host string, port int, expectedAddress string) TestResult {
	startTime := time.Now()
	
	// Use default port if not specified
	if port == 0 {
		port = t.defaultPort
	}
	
	// Parse hostname:port if port is in the hostname
	// But skip this for IPv6 addresses (which contain colons)
	if strings.Contains(host, ":") && !strings.Contains(host, "::") && strings.Count(host, ":") == 1 {
		// This looks like hostname:port, not an IPv6 address
		parts := strings.SplitN(host, ":", 2)
		host = parts[0]
		// Port in hostname overrides the port parameter
		if len(parts) > 1 {
			var p int
			_, _ = fmt.Sscanf(parts[1], "%d", &p)
			if p > 0 {
				port = p
			}
		}
	}
	
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	
	if t.debug {
		log.Printf("BinkP: Testing %s (expected address: %s)", address, expectedAddress)
	}
	
	// Create connection with timeout
	dialer := net.Dialer{
		Timeout: t.timeout,
	}
	
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return &BinkPTestResult{
			BaseTestResult: BaseTestResult{
				Success:    false,
				Error:      fmt.Sprintf("connection failed: %v", err),
				ResponseMs: uint32(time.Since(startTime).Milliseconds()),
				TestTime:   startTime,
			},
		}
	}
	defer conn.Close()
	
	// Create BinkP session with custom system info
	session := binkp.NewSessionWithInfo(conn, t.ourAddress, t.systemName, t.sysop, t.location)
	session.SetTimeout(t.timeout)
	session.SetDebug(t.debug)
	
	// Perform handshake
	err = session.Handshake()
	if err != nil {
		return &BinkPTestResult{
			BaseTestResult: BaseTestResult{
				Success:    false,
				Error:      fmt.Sprintf("handshake failed: %v", err),
				ResponseMs: uint32(time.Since(startTime).Milliseconds()),
				TestTime:   startTime,
			},
		}
	}
	
	// Get remote node information
	nodeInfo := session.GetNodeInfo()
	
	// Validate address if expected
	addressValid := false
	if expectedAddress != "" {
		addressValid = session.ValidateAddress(expectedAddress)
		
		if t.debug {
			log.Printf("BinkP: Address validation: expected=%s, received=%v, valid=%v",
				expectedAddress, nodeInfo.Addresses, addressValid)
		}
	}
	
	// Close session gracefully
	session.Close()
	
	// Build capabilities list
	capabilities := nodeInfo.Capabilities
	if nodeInfo.Flags != "" {
		// Add flags as capabilities for compatibility
		capabilities = append(capabilities, strings.Fields(nodeInfo.Flags)...)
	}
	
	return &BinkPTestResult{
		BaseTestResult: BaseTestResult{
			Success:    true,
			Error:      "",
			ResponseMs: uint32(time.Since(startTime).Milliseconds()),
			TestTime:   startTime,
		},
		SystemName:      nodeInfo.SystemName,
		Sysop:           nodeInfo.Sysop,
		Location:        nodeInfo.Location,
		Version:         nodeInfo.Version,
		Addresses:       nodeInfo.Addresses,
		Capabilities:    capabilities,
		AddressValid:    addressValid,
		Port:            port,
	}
}

// SetDebug enables or disables debug mode
func (t *BinkPTester) SetDebug(enabled bool) {
	t.debug = enabled
}

