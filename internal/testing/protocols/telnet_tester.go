package protocols

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// TelnetTester implements Telnet protocol testing
type TelnetTester struct {
	timeout time.Duration
}

// NewTelnetTester creates a new Telnet tester
func NewTelnetTester(timeout time.Duration) *TelnetTester {
	return &TelnetTester{
		timeout: timeout,
	}
}

// GetProtocolName returns the protocol name
func (t *TelnetTester) GetProtocolName() string {
	return "Telnet"
}

// Test performs a Telnet connectivity test
func (t *TelnetTester) Test(ctx context.Context, host string, port int, expectedAddress string) TestResult {
	startTime := time.Now()
	
	// Default Telnet port is 23
	if port == 0 {
		port = 23
	}
	
	// Create connection with timeout
	dialer := net.Dialer{
		Timeout: t.timeout,
	}
	
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return &TelnetTestResult{
			BaseTestResult: BaseTestResult{
				Success:    false,
				Error:      fmt.Sprintf("connection failed: %v", err),
				ResponseMs: uint32(time.Since(startTime).Milliseconds()),
				TestTime:   startTime,
			},
		}
	}
	defer conn.Close()
	
	// Set read deadline
	_ = conn.SetReadDeadline(time.Now().Add(t.timeout))
	
	// Try to read banner/welcome message
	reader := bufio.NewReader(conn)
	banner := ""
	
	// Read initial response (with timeout)
	bannerChan := make(chan string, 1)
	go func() {
		data := make([]byte, 1024)
		n, err := reader.Read(data)
		if err == nil && n > 0 {
			bannerChan <- string(data[:n])
		} else {
			bannerChan <- ""
		}
	}()
	
	select {
	case b := <-bannerChan:
		banner = b
	case <-time.After(2 * time.Second):
		// No banner within 2 seconds, but connection successful
		banner = ""
	}
	
	// Clean up banner (remove telnet negotiation bytes and control characters)
	banner = t.cleanBanner(banner)
	
	result := &TelnetTestResult{
		BaseTestResult: BaseTestResult{
			Success:    true,
			ResponseMs: uint32(time.Since(startTime).Milliseconds()),
			TestTime:   startTime,
		},
		Banner: banner,
	}
	
	// Check if banner contains BBS/FidoNet indicators
	if banner != "" {
		lowerBanner := strings.ToLower(banner)
		if strings.Contains(lowerBanner, "bbs") ||
			strings.Contains(lowerBanner, "fidonet") ||
			strings.Contains(lowerBanner, "node") ||
			strings.Contains(lowerBanner, "sysop") {
			// Likely a BBS system
			result.Success = true
		}
	}
	
	return result
}

// cleanBanner removes telnet negotiation and control characters
func (t *TelnetTester) cleanBanner(banner string) string {
	if banner == "" {
		return ""
	}
	
	// Remove common telnet IAC (Interpret As Command) sequences
	// IAC = 255, followed by command bytes
	cleaned := ""
	skip := 0
	for i := 0; i < len(banner); i++ {
		if skip > 0 {
			skip--
			continue
		}
		
		b := banner[i]
		
		// Skip IAC sequences (255 followed by 2 bytes)
		if b == 255 && i+2 < len(banner) {
			skip = 2
			continue
		}
		
		// Skip other control characters except newline and tab
		if b < 32 && b != 10 && b != 13 && b != 9 {
			continue
		}
		
		// Skip high control characters
		if b > 126 && b < 255 {
			continue
		}
		
		cleaned += string(b)
	}
	
	// Trim whitespace
	cleaned = strings.TrimSpace(cleaned)
	
	// Limit banner length
	if len(cleaned) > 500 {
		cleaned = cleaned[:500] + "..."
	}
	
	return cleaned
}