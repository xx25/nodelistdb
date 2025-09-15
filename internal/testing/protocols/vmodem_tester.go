package protocols

import (
	"context"
	"fmt"
	"net"
	"time"
)

// VModemTester implements VModem protocol testing
type VModemTester struct {
	timeout time.Duration
}

// NewVModemTester creates a new VModem tester
func NewVModemTester(timeout time.Duration) *VModemTester {
	return &VModemTester{
		timeout: timeout,
	}
}

// GetProtocolName returns the protocol name
func (t *VModemTester) GetProtocolName() string {
	return "VModem"
}

// Test performs a VModem connectivity test
func (t *VModemTester) Test(ctx context.Context, host string, port int, expectedAddress string) TestResult {
	startTime := time.Now()
	
	// Default VModem port is 3141
	if port == 0 {
		port = 3141
	}
	
	// Create connection with timeout
	dialer := net.Dialer{
		Timeout: t.timeout,
	}
	
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return &VModemTestResult{
			BaseTestResult: BaseTestResult{
				Success:    false,
				Error:      fmt.Sprintf("connection failed: %v", err),
				ResponseMs: uint32(time.Since(startTime).Milliseconds()),
				TestTime:   startTime,
			},
		}
	}
	defer conn.Close()
	
	// VModem is a simple TCP tunnel for virtual modem connections
	// Just checking if connection is successful is enough for basic testing
	// More advanced testing would involve sending AT commands
	
	result := &VModemTestResult{
		BaseTestResult: BaseTestResult{
			Success:    true,
			ResponseMs: uint32(time.Since(startTime).Milliseconds()),
			TestTime:   startTime,
		},
	}
	
	// Optionally try to read any initial response
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buffer := make([]byte, 256)
	n, err := conn.Read(buffer)
	if err == nil && n > 0 {
		result.Banner = string(buffer[:n])
	}
	
	return result
}