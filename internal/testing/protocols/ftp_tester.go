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

// Test performs an FTP connectivity test with anonymous login attempt
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
	_ = conn.SetReadDeadline(time.Now().Add(t.timeout))

	// Read FTP banner (220 response), handling multi-line banners
	reader := bufio.NewReader(conn)
	banner, responseCode := t.readFullFTPResponse(reader)

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

		// Attempt anonymous login
		t.tryAnonymousLogin(conn, reader, result)
	} else if responseCode != "" {
		result.Error = fmt.Sprintf("unexpected FTP response code: %s", responseCode)
	} else {
		result.Error = "no FTP response received"
	}

	// Send QUIT for clean disconnect
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, _ = fmt.Fprintf(conn, "QUIT\r\n")

	return result
}

// tryAnonymousLogin attempts anonymous FTP login after a successful banner
func (t *FTPTester) tryAnonymousLogin(conn net.Conn, reader *bufio.Reader, result *FTPTestResult) {
	// Send USER anonymous
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := fmt.Fprintf(conn, "USER anonymous\r\n")
	if err != nil {
		return
	}

	// Read USER response
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, userCode := t.readFullFTPResponse(reader)

	if userCode == "" {
		return
	}

	result.AnonTested = true

	switch userCode {
	case "230":
		// Login accepted without password
		result.AnonLogin = true
	case "331":
		// Password required, send PASS
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err := fmt.Fprintf(conn, "PASS anonymous@\r\n")
		if err != nil {
			return
		}

		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, passCode := t.readFullFTPResponse(reader)

		if passCode == "230" {
			result.AnonLogin = true
		}
		// 530 = login rejected, 332 = need account, etc. - AnonLogin stays false
	}
	// 530, 421, other codes - AnonLogin stays false
}

// readFullFTPResponse reads a complete FTP response, handling multi-line responses.
// Multi-line FTP responses use "NNN-text" for continuation lines and "NNN text" for the final line.
// Returns the text of the final line and the response code.
func (t *FTPTester) readFullFTPResponse(reader *bufio.Reader) (string, string) {
	var lastText string
	var responseCode string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// Return whatever we have so far
			return lastText, responseCode
		}

		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}

		// Extract response code (first 3 characters)
		code := line[:3]
		isNumeric := true
		for _, c := range code {
			if c < '0' || c > '9' {
				isNumeric = false
				break
			}
		}
		if !isNumeric {
			continue
		}

		responseCode = code

		// Extract text after code
		text := line
		if len(line) > 4 {
			text = line[4:]
		} else if len(line) > 3 {
			text = ""
		}

		text = strings.TrimSpace(text)
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		lastText = text

		// Check if this is the final line: "NNN " (space after code, not dash)
		if len(line) == 3 || (len(line) > 3 && line[3] == ' ') {
			return lastText, responseCode
		}
		// If line[3] == '-', it's a continuation line, keep reading
	}
}
