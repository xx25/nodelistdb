package protocols

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// FTPTester implements FTP protocol testing
type FTPTester struct {
	timeout time.Duration
}

// NewFTPTester creates a new FTP tester
func NewFTPTester(timeout time.Duration) *FTPTester {
	return &FTPTester{
		timeout: timeout,
	}
}

// GetProtocolName returns the protocol name
func (t *FTPTester) GetProtocolName() string {
	return "FTP"
}

// Test performs an FTP connectivity test
func (t *FTPTester) Test(ctx context.Context, host string, port int, expectedAddress string) TestResult {
	startTime := time.Now()
	
	// Default FTP port is 21
	if port == 0 {
		port = 21
	}
	
	// Create connection with timeout
	dialer := net.Dialer{
		Timeout: t.timeout,
	}
	
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return &FTPTestResult{
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
	conn.SetReadDeadline(time.Now().Add(t.timeout))
	
	// Read FTP banner (220 response)
	reader := bufio.NewReader(conn)
	banner, responseCode := t.readFTPResponse(reader)
	
	result := &FTPTestResult{
		BaseTestResult: BaseTestResult{
			Success:    false,
			ResponseMs: uint32(time.Since(startTime).Milliseconds()),
			TestTime:   startTime,
		},
		Banner: banner,
	}
	
	// Check if we got a valid FTP response
	if responseCode == "220" {
		result.Success = true
		
		// Try to send QUIT command for clean disconnect
		conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		fmt.Fprintf(conn, "QUIT\r\n")
	} else if responseCode != "" {
		result.Error = fmt.Sprintf("unexpected FTP response code: %s", responseCode)
	} else {
		result.Error = "no FTP response received"
	}
	
	return result
}

// readFTPResponse reads an FTP response and returns the banner and response code
func (t *FTPTester) readFTPResponse(reader *bufio.Reader) (string, string) {
	// FTP responses format: "NNN message\r\n" where NNN is the response code
	
	// Try to read response with timeout
	responseChan := make(chan string, 1)
	go func() {
		line, err := reader.ReadString('\n')
		if err == nil {
			responseChan <- line
		} else {
			responseChan <- ""
		}
	}()
	
	select {
	case response := <-responseChan:
		if response == "" {
			return "", ""
		}
		
		// Clean up response
		response = strings.TrimSpace(response)
		
		// Extract response code (first 3 characters)
		responseCode := ""
		if len(response) >= 3 {
			responseCode = response[:3]
			
			// Validate that it's a numeric code
			for _, c := range responseCode {
				if c < '0' || c > '9' {
					responseCode = ""
					break
				}
			}
		}
		
		// Extract banner message (after code and space)
		banner := response
		if len(response) > 4 && response[3] == ' ' {
			banner = response[4:]
		}
		
		// Clean up banner
		banner = strings.TrimSpace(banner)
		
		// Limit banner length
		if len(banner) > 500 {
			banner = banner[:500] + "..."
		}
		
		return banner, responseCode
		
	case <-time.After(t.timeout):
		return "", ""
	}
}