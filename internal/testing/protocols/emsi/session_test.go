package emsi

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// charReader unit tests
// ============================================================================

func TestGetchar_BasicRead(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	go func() {
		_, _ = s.Write([]byte("A"))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	b, err := cr.getchar(time.Second)
	if err != nil {
		t.Fatalf("getchar failed: %v", err)
	}
	if b != 'A' {
		t.Errorf("expected 'A', got %q", b)
	}
}

func TestGetchar_Timeout(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	_ = s

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	_, err := cr.getchar(50 * time.Millisecond)
	if err != errCharTimeout {
		t.Errorf("expected errCharTimeout, got %v", err)
	}
}

func TestGetchar_CarrierDetect(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	go func() {
		_, _ = s.Write([]byte("OK\r\nNO CARRIER\r\n"))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	// Read until carrier lost
	var lastErr error
	for range 20 {
		_, lastErr = cr.getchar(time.Second)
		if lastErr != nil {
			break
		}
	}
	if lastErr != errCarrierLost {
		t.Errorf("expected errCarrierLost, got %v", lastErr)
	}
}

func TestGetchar_CarrierNoFalsePositive(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	// "NO CARRIER" embedded in middle of a longer line — not a standalone signal
	go func() {
		_, _ = s.Write([]byte("Status: NO CARRIER detected previously\r\nDONE\r\n"))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	// Should read all bytes without triggering carrier lost
	var bytes []byte
	for range 50 {
		b, err := cr.getchar(time.Second)
		if err != nil {
			t.Fatalf("unexpected error at byte %d: %v", len(bytes), err)
		}
		bytes = append(bytes, b)
		if b == '\n' && len(bytes) > 5 && string(bytes[len(bytes)-6:]) == "DONE\r\n" {
			break
		}
	}
	if cr.carrierLost {
		t.Error("false positive: carrier lost should not be set for longer line")
	}
}

func TestGetchar_EOF(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()

	go func() {
		_, _ = s.Write([]byte("AB"))
		s.Close()
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	// Read 2 bytes successfully
	b1, err := cr.getchar(time.Second)
	if err != nil || b1 != 'A' {
		t.Fatalf("first byte: got %q err %v", b1, err)
	}
	b2, err := cr.getchar(time.Second)
	if err != nil || b2 != 'B' {
		t.Fatalf("second byte: got %q err %v", b2, err)
	}
	// Third read should get carrier lost (EOF)
	_, err = cr.getchar(time.Second)
	if err != errCarrierLost {
		t.Errorf("expected errCarrierLost on EOF, got %v", err)
	}
}

func TestGetchar_BannerAccumulation(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	go func() {
		_, _ = s.Write([]byte("Hello World\r\n"))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	for range 13 { // "Hello World\r\n" = 13 chars
		_, err := cr.getchar(time.Second)
		if err != nil {
			t.Fatalf("getchar error: %v", err)
		}
	}

	banner := cr.getBannerText()
	if banner != "Hello World\r\n" {
		t.Errorf("banner = %q, want %q", banner, "Hello World\r\n")
	}
}

// ============================================================================
// readToken unit tests
// ============================================================================

func TestReadToken_INQ(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	go func() {
		_, _ = s.Write([]byte(EMSI_INQ + "\r"))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	tok := cr.readToken(2*time.Second, time.Second, time.Time{})
	if tok != tokenINQ {
		t.Errorf("expected tokenINQ, got %s", tok)
	}
}

func TestReadToken_REQ(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	go func() {
		_, _ = s.Write([]byte(EMSI_REQ + "\r"))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	tok := cr.readToken(2*time.Second, time.Second, time.Time{})
	if tok != tokenREQ {
		t.Errorf("expected tokenREQ, got %s", tok)
	}
}

func TestReadToken_ACK(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	go func() {
		_, _ = s.Write([]byte(EMSI_ACK + "\r"))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	tok := cr.readToken(2*time.Second, time.Second, time.Time{})
	if tok != tokenACK {
		t.Errorf("expected tokenACK, got %s", tok)
	}
}

func TestReadToken_NAK(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	go func() {
		_, _ = s.Write([]byte(EMSI_NAK + "\r"))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	tok := cr.readToken(2*time.Second, time.Second, time.Time{})
	if tok != tokenNAK {
		t.Errorf("expected tokenNAK, got %s", tok)
	}
}

func TestReadToken_DAT(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	// Send just the DAT header — readToken should detect it without reading the length
	go func() {
		_, _ = s.Write([]byte("**EMSI_DAT"))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	tok := cr.readToken(2*time.Second, time.Second, time.Time{})
	if tok != tokenDAT {
		t.Errorf("expected tokenDAT, got %s", tok)
	}
}

func TestReadToken_BannerThenEMSI(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	go func() {
		// Simulate a BBS banner followed by EMSI_REQ
		_, _ = s.Write([]byte("Welcome to FidoNet BBS!\r\nPlease wait...\r\n" + EMSI_REQ + "\r"))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	tok := cr.readToken(2*time.Second, time.Second, time.Time{})
	if tok != tokenREQ {
		t.Errorf("expected tokenREQ after banner, got %s", tok)
	}

	// Banner should contain the welcome text
	banner := cr.getBannerText()
	if !strings.Contains(banner, "Welcome to FidoNet BBS!") {
		t.Errorf("banner should contain welcome text, got %q", banner)
	}
}

func TestReadToken_StepTimeout(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	_ = s

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	tok := cr.readToken(100*time.Millisecond, 50*time.Millisecond, time.Time{})
	if tok != tokenTimeout {
		t.Errorf("expected tokenTimeout, got %s", tok)
	}
}

func TestReadToken_XONXOFFStripped(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	go func() {
		// Insert XON (0x11) and XOFF (0x13) inside the token
		data := []byte{'*', '*', 0x11, 'E', 'M', 'S', 'I', 0x13, '_', 'R', 'E', 'Q', 'A', '7', '7', 'E'}
		_, _ = s.Write(data)
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	tok := cr.readToken(2*time.Second, time.Second, time.Time{})
	if tok != tokenREQ {
		t.Errorf("expected tokenREQ with XON/XOFF stripped, got %s", tok)
	}
}

func TestReadToken_MasterDeadline(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	_ = s

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	// Master deadline already passed
	past := time.Now().Add(-time.Second)
	tok := cr.readToken(10*time.Second, time.Second, past)
	if tok != tokenTimeout {
		t.Errorf("expected tokenTimeout with expired master deadline, got %s", tok)
	}
}

func TestReadToken_BareEMSI(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	// Some non-compliant mailers omit the ** prefix
	go func() {
		_, _ = s.Write([]byte("EMSI_REQA77E\r"))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	tok := cr.readToken(2*time.Second, time.Second, time.Time{})
	if tok != tokenREQ {
		t.Errorf("expected tokenREQ for bare EMSI_REQ, got %s", tok)
	}
}

// ============================================================================
// readEMSI_DAT unit tests
// ============================================================================

// buildTestDATPacket builds a complete EMSI_DAT packet with correct CRC.
// Returns the full packet and the portion after "EMSI_DAT" (lenHex+data+crc).
func buildTestDATPacket() (string, string) {
	data := &EMSIData{
		SystemName:    "Test System",
		Location:      "Test City",
		Sysop:         "Test Op",
		Phone:         "-Unpublished-",
		Speed:         "9600",
		Flags:         "CM,XA",
		MailerName:    "TestMailer",
		MailerVersion: "1.0",
		MailerSerial:  "TST",
		Addresses:     []string{"2:5001/100"},
		Protocols:     []string{"ZMO", "ZAP"},
	}
	packet := CreateEMSI_DAT(data)
	return packet, packet[len("**EMSI_DAT"):] // everything after the "**EMSI_DAT" prefix
}

func TestReadEMSI_DAT_ValidPacket(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	fullPacket, afterHeader := buildTestDATPacket()

	go func() {
		// Send the part after "EMSI_DAT" (lenHex + data + CRC)
		_, _ = s.Write([]byte(afterHeader))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	result, err := cr.readEMSI_DAT(time.Second, time.Time{}, true)
	if err != nil {
		t.Fatalf("readEMSI_DAT failed: %v", err)
	}

	// The result should be a valid packet that ParseEMSI_DAT can handle
	info, err := ParseEMSI_DAT(result)
	if err != nil {
		t.Fatalf("ParseEMSI_DAT failed: %v", err)
	}
	if info.SystemName != "Test System" {
		t.Errorf("SystemName = %q, want %q", info.SystemName, "Test System")
	}
	if info.MailerName != "TestMailer" {
		t.Errorf("MailerName = %q, want %q", info.MailerName, "TestMailer")
	}

	// Result should equal the original packet
	if result != fullPacket {
		t.Errorf("reconstructed packet differs from original:\ngot:  %q\nwant: %q", result, fullPacket)
	}
}

func TestReadEMSI_DAT_CRCMismatch(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	_, afterHeader := buildTestDATPacket()

	// Corrupt the CRC (last 4 chars)
	corrupted := afterHeader[:len(afterHeader)-4] + "0000"

	go func() {
		_, _ = s.Write([]byte(corrupted))
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	_, err := cr.readEMSI_DAT(time.Second, time.Time{}, false)
	if err == nil {
		t.Fatal("expected CRC mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "CRC mismatch") {
		t.Errorf("expected CRC mismatch error, got: %v", err)
	}
}

func TestReadEMSI_DAT_Timeout(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	// Only send 2 of the 4 length hex chars
	go func() {
		_, _ = s.Write([]byte("00"))
		// Don't send more
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	_, err := cr.readEMSI_DAT(100*time.Millisecond, time.Time{}, true)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestReadEMSI_DAT_InvalidLength(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	go func() {
		_, _ = s.Write([]byte("ZZZZ")) // Invalid hex
	}()

	cr := &charReader{
		conn:   c,
		reader: bufio.NewReader(c),
	}

	_, err := cr.readEMSI_DAT(time.Second, time.Time{}, true)
	if err == nil {
		t.Fatal("expected invalid length error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid DAT length") {
		t.Errorf("expected invalid length error, got: %v", err)
	}
}

// ============================================================================
// feedCarrierDetect unit tests
// ============================================================================

func TestFeedCarrierDetect_Signals(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLost bool
	}{
		{"NO CARRIER", "\r\nNO CARRIER\r\n", true},
		{"BUSY", "\r\nBUSY\r\n", true},
		{"NO DIALTONE", "\r\nNO DIALTONE\r\n", true},
		{"NO ANSWER", "\r\nNO ANSWER\r\n", true},
		{"normal text", "\r\nHello World\r\n", false},
		{"NO CARRIER in longer line", "text NO CARRIER here\r\n", false}, // not standalone
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &charReader{}
			for _, b := range []byte(tt.input) {
				cr.feedCarrierDetect(b)
			}
			if cr.carrierLost != tt.wantLost {
				t.Errorf("carrierLost = %v, want %v", cr.carrierLost, tt.wantLost)
			}
		})
	}
}

func TestFeedCarrierDetect_NoFalsePositive(t *testing.T) {
	// "NO CARRIER" as part of a longer line should NOT trigger
	cr := &charReader{}
	for _, b := range []byte("Status: NO CARRIER detected previously\r\n") {
		cr.feedCarrierDetect(b)
	}
	if cr.carrierLost {
		t.Error("false positive: line with extra text should not trigger carrier lost")
	}
}

// ============================================================================
// Token String() test
// ============================================================================

func TestTokenString(t *testing.T) {
	tests := []struct {
		tok  emsiToken
		want string
	}{
		{tokenNone, "NONE"},
		{tokenINQ, "INQ"},
		{tokenREQ, "REQ"},
		{tokenACK, "ACK"},
		{tokenNAK, "NAK"},
		{tokenCLI, "CLI"},
		{tokenHBT, "HBT"},
		{tokenDAT, "DAT"},
		{tokenTimeout, "TIMEOUT"},
		{tokenCarrier, "CARRIER"},
		{tokenError, "ERROR"},
		{emsiToken(99), "TOKEN(99)"},
	}
	for _, tt := range tests {
		if got := tt.tok.String(); got != tt.want {
			t.Errorf("%d.String() = %q, want %q", int(tt.tok), got, tt.want)
		}
	}
}

// ============================================================================
// Full handshake integration tests
// ============================================================================

// drainReader continuously reads from a connection, discarding all data.
// Used by remote goroutines to prevent write blocking on net.Pipe.
// Returns a channel that receives each chunk of data read.
func drainReader(conn net.Conn) <-chan []byte {
	ch := make(chan []byte, 100)
	go func() {
		defer close(ch)
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				ch <- data
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

// waitForData reads from the drain channel until we've accumulated at least
// a specified substring, or timeout.
func waitForData(ch <-chan []byte, contains string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	var buf strings.Builder
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return false
			}
			buf.Write(data)
			if strings.Contains(buf.String(), contains) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func setupHandshakeTest(t *testing.T) (sess *Session, remote net.Conn, cleanup func()) {
	t.Helper()
	local, remote := net.Pipe()

	cfg := DefaultConfig()
	cfg.MasterTimeout = 5 * time.Second
	cfg.StepTimeout = 2 * time.Second
	cfg.FirstStepTimeout = 1 * time.Second
	cfg.CharacterTimeout = 500 * time.Millisecond
	cfg.RetryDelay = 0 // No delay in tests
	cfg.SendINQTwice = false // Single sends for test simplicity
	cfg.SendREQTwice = false
	cfg.Protocols = []string{} // NCP mode for test

	sess = NewSessionWithInfoAndConfig(local, "2:5001/0", "TestLocal", "Tester", "Test", cfg)

	cleanup = func() {
		local.Close()
		remote.Close()
	}

	return sess, remote, cleanup
}

func TestHandshake_REQFlow(t *testing.T) {
	sess, remote, cleanup := setupHandshakeTest(t)
	defer cleanup()

	go func() {
		defer remote.Close()

		// Start draining reads from session side continuously
		ch := drainReader(remote)

		// Send EMSI_REQ — remote requests our DAT
		_, _ = remote.Write([]byte(EMSI_REQ + "\r"))

		// Wait for our DAT to arrive
		if !waitForData(ch, "EMSI_DAT", 5*time.Second) {
			return
		}

		// Send ACK for our DAT
		_, _ = remote.Write([]byte(EMSI_ACK + "\r"))

		// Send their DAT (with SkipFirstRXReq=true, session waits without sending REQ first)
		datPacket := buildRemoteDATPacket("Remote System", "2:5001/100")
		_, _ = remote.Write([]byte(datPacket))

		// Wait for remaining data (our ACK) then close
		waitForData(ch, "EMSI_ACK", 5*time.Second)
	}()

	err := sess.Handshake()
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	info := sess.GetRemoteInfo()
	if info == nil {
		t.Fatal("remote info is nil")
	}
	if info.SystemName != "Remote System" {
		t.Errorf("SystemName = %q, want %q", info.SystemName, "Remote System")
	}
	if sess.GetCompletionReason() != ReasonNCP {
		t.Errorf("completion reason = %s, want NCP", sess.GetCompletionReason())
	}
}

func TestHandshake_DATFlow(t *testing.T) {
	sess, remote, cleanup := setupHandshakeTest(t)
	defer cleanup()

	go func() {
		defer remote.Close()

		// Send their DAT directly
		datPacket := buildRemoteDATPacket("Direct DAT System", "2:5001/200")
		_, _ = remote.Write([]byte(datPacket))

		// Drain and respond: we'll get ACK + our DAT
		reader := bufio.NewReader(remote)
		buf := make([]byte, 4096)

		// Read until we see our DAT
		for {
			n, err := reader.Read(buf)
			if err != nil {
				return
			}
			if strings.Contains(string(buf[:n]), "EMSI_DAT") {
				break
			}
		}

		// Send ACK for our DAT
		_, _ = remote.Write([]byte(EMSI_ACK + "\r"))

		// Drain remaining
		_, _ = io.Copy(io.Discard, reader)
	}()

	err := sess.Handshake()
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	info := sess.GetRemoteInfo()
	if info == nil {
		t.Fatal("remote info is nil")
	}
	if info.SystemName != "Direct DAT System" {
		t.Errorf("SystemName = %q, want %q", info.SystemName, "Direct DAT System")
	}
}

func TestHandshake_INQFlow(t *testing.T) {
	sess, remote, cleanup := setupHandshakeTest(t)
	defer cleanup()

	go func() {
		defer remote.Close()

		// Send INQ
		_, _ = remote.Write([]byte(EMSI_INQ + "\r"))

		// Continuously drain from local side while responding
		ch := drainReader(remote)

		// Wait for our REQ
		waitForData(ch, "EMSI_REQ", 3*time.Second)

		// Send their DAT
		datPacket := buildRemoteDATPacket("INQ System", "2:5001/300")
		_, _ = remote.Write([]byte(datPacket))

		// Wait for our ACK + DAT
		waitForData(ch, "EMSI_DAT", 3*time.Second)

		// Send ACK for our DAT
		_, _ = remote.Write([]byte(EMSI_ACK + "\r"))

		// Let drain finish
		time.Sleep(100 * time.Millisecond)
	}()

	err := sess.Handshake()
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	info := sess.GetRemoteInfo()
	if info == nil {
		t.Fatal("remote info is nil")
	}
	if info.SystemName != "INQ System" {
		t.Errorf("SystemName = %q, want %q", info.SystemName, "INQ System")
	}
}

func TestHandshake_BannerThenREQ(t *testing.T) {
	sess, remote, cleanup := setupHandshakeTest(t)
	defer cleanup()

	go func() {
		defer remote.Close()

		// Send long banner first
		_, _ = remote.Write([]byte("Welcome to FidoNet BBS!\r\n"))
		time.Sleep(50 * time.Millisecond)
		_, _ = remote.Write([]byte("Running qico v0.57.1xe\r\n"))
		time.Sleep(50 * time.Millisecond)

		// Then EMSI_REQ
		_, _ = remote.Write([]byte(EMSI_REQ + "\r"))

		ch := drainReader(remote)

		// Wait for our DAT
		waitForData(ch, "EMSI_DAT", 3*time.Second)

		// Send ACK
		_, _ = remote.Write([]byte(EMSI_ACK + "\r"))

		// Send their DAT
		datPacket := buildRemoteDATPacket("Banner System", "2:5001/400")
		_, _ = remote.Write([]byte(datPacket))

		// Let drain finish
		time.Sleep(100 * time.Millisecond)
	}()

	err := sess.Handshake()
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	info := sess.GetRemoteInfo()
	if info == nil {
		t.Fatal("remote info is nil")
	}
	if info.SystemName != "Banner System" {
		t.Errorf("SystemName = %q, want %q", info.SystemName, "Banner System")
	}
}

func TestHandshake_MasterTimeout(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	cfg := DefaultConfig()
	cfg.MasterTimeout = 300 * time.Millisecond
	cfg.StepTimeout = 200 * time.Millisecond
	cfg.FirstStepTimeout = 100 * time.Millisecond
	cfg.CharacterTimeout = 50 * time.Millisecond
	cfg.PreventiveINQ = false

	sess := NewSessionWithConfig(local, "2:5001/0", cfg)

	// Drain remote side (to prevent write blocks from our INQ retries)
	go func() { _, _ = io.Copy(io.Discard, remote) }()

	err := sess.Handshake()
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestHandshake_CarrierLoss(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()

	cfg := DefaultConfig()
	cfg.MasterTimeout = 5 * time.Second
	cfg.StepTimeout = 2 * time.Second
	cfg.FirstStepTimeout = 1 * time.Second
	cfg.CharacterTimeout = 500 * time.Millisecond

	sess := NewSessionWithConfig(local, "2:5001/0", cfg)

	go func() {
		// Send NO CARRIER on its own line
		_, _ = remote.Write([]byte("\r\nNO CARRIER\r\n"))
		remote.Close()
	}()

	err := sess.Handshake()
	if err == nil {
		t.Fatal("expected carrier lost error, got nil")
	}
}

func TestHandshake_RetryOnNAK(t *testing.T) {
	sess, remote, cleanup := setupHandshakeTest(t)
	defer cleanup()

	go func() {
		defer remote.Close()

		// Send REQ
		_, _ = remote.Write([]byte(EMSI_REQ + "\r"))

		ch := drainReader(remote)

		// Wait for our first DAT
		waitForData(ch, "EMSI_DAT", 3*time.Second)

		// Send NAK (reject first DAT)
		_, _ = remote.Write([]byte(EMSI_NAK + "\r"))

		// Wait for retry DAT
		waitForData(ch, "EMSI_DAT", 3*time.Second)

		// Send ACK this time
		_, _ = remote.Write([]byte(EMSI_ACK + "\r"))

		// Wait for REQ from RX phase
		waitForData(ch, "EMSI_REQ", 3*time.Second)

		// Send their DAT
		datPacket := buildRemoteDATPacket("NAK Retry System", "2:5001/500")
		_, _ = remote.Write([]byte(datPacket))

		// Let drain finish
		time.Sleep(100 * time.Millisecond)
	}()

	err := sess.Handshake()
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	info := sess.GetRemoteInfo()
	if info == nil {
		t.Fatal("remote info is nil")
	}
	if info.SystemName != "NAK Retry System" {
		t.Errorf("SystemName = %q, want %q", info.SystemName, "NAK Retry System")
	}
}

func TestHandshake_RetryOnTimeout(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	cfg := DefaultConfig()
	cfg.MasterTimeout = 10 * time.Second
	cfg.StepTimeout = 500 * time.Millisecond
	cfg.FirstStepTimeout = 300 * time.Millisecond
	cfg.CharacterTimeout = 100 * time.Millisecond
	cfg.Protocols = []string{}

	sess := NewSessionWithInfoAndConfig(local, "2:5001/0", "TestLocal", "Tester", "Test", cfg)

	go func() {
		defer remote.Close()

		// Send REQ
		_, _ = remote.Write([]byte(EMSI_REQ + "\r"))

		ch := drainReader(remote)

		// Wait for first DAT
		waitForData(ch, "EMSI_DAT", 3*time.Second)

		// Wait for step timeout to expire (don't respond)
		time.Sleep(600 * time.Millisecond)

		// Wait for retry DAT
		waitForData(ch, "EMSI_DAT", 3*time.Second)

		// Now send ACK
		_, _ = remote.Write([]byte(EMSI_ACK + "\r"))

		// Wait for REQ
		waitForData(ch, "EMSI_REQ", 3*time.Second)

		// Send their DAT
		datPacket := buildRemoteDATPacket("Timeout Retry System", "2:5001/600")
		_, _ = remote.Write([]byte(datPacket))

		// Let drain finish
		time.Sleep(100 * time.Millisecond)
	}()

	err := sess.Handshake()
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	info := sess.GetRemoteInfo()
	if info == nil {
		t.Fatal("remote info is nil")
	}
	if info.SystemName != "Timeout Retry System" {
		t.Errorf("SystemName = %q, want %q", info.SystemName, "Timeout Retry System")
	}
}

func TestHandshake_SendINQStrategy(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	cfg := DefaultConfig()
	cfg.MasterTimeout = 5 * time.Second
	cfg.StepTimeout = 2 * time.Second
	cfg.FirstStepTimeout = 1 * time.Second
	cfg.CharacterTimeout = 500 * time.Millisecond
	cfg.InitialStrategy = "send_inq"
	cfg.Protocols = []string{}

	sess := NewSessionWithInfoAndConfig(local, "2:5001/0", "TestLocal", "Tester", "Test", cfg)

	go func() {
		defer remote.Close()

		ch := drainReader(remote)

		// Wait for our INQ
		waitForData(ch, "EMSI_INQ", 3*time.Second)

		// Send REQ in response
		_, _ = remote.Write([]byte(EMSI_REQ + "\r"))

		// Wait for our DAT
		waitForData(ch, "EMSI_DAT", 3*time.Second)

		// Send ACK
		_, _ = remote.Write([]byte(EMSI_ACK + "\r"))

		// Wait for REQ
		waitForData(ch, "EMSI_REQ", 3*time.Second)

		// Send their DAT
		datPacket := buildRemoteDATPacket("INQ Strategy System", "2:5001/700")
		_, _ = remote.Write([]byte(datPacket))

		// Let drain finish
		time.Sleep(100 * time.Millisecond)
	}()

	err := sess.Handshake()
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	info := sess.GetRemoteInfo()
	if info == nil {
		t.Fatal("remote info is nil")
	}
	if info.SystemName != "INQ Strategy System" {
		t.Errorf("SystemName = %q, want %q", info.SystemName, "INQ Strategy System")
	}
}

func TestHandshake_HBTKeepalive(t *testing.T) {
	sess, remote, cleanup := setupHandshakeTest(t)
	defer cleanup()

	go func() {
		defer remote.Close()

		// Send HBT first (keepalive)
		_, _ = remote.Write([]byte(EMSI_HBT + "\r"))
		time.Sleep(50 * time.Millisecond)

		// Then send REQ
		_, _ = remote.Write([]byte(EMSI_REQ + "\r"))

		ch := drainReader(remote)

		// Wait for our DAT
		waitForData(ch, "EMSI_DAT", 3*time.Second)

		// Send ACK
		_, _ = remote.Write([]byte(EMSI_ACK + "\r"))

		// Wait for REQ
		waitForData(ch, "EMSI_REQ", 3*time.Second)

		// Send their DAT
		datPacket := buildRemoteDATPacket("HBT System", "2:5001/800")
		_, _ = remote.Write([]byte(datPacket))

		// Let drain finish
		time.Sleep(100 * time.Millisecond)
	}()

	err := sess.Handshake()
	if err != nil {
		t.Fatalf("Handshake failed (HBT should be keepalive): %v", err)
	}

	info := sess.GetRemoteInfo()
	if info == nil {
		t.Fatal("remote info is nil after HBT keepalive")
	}
}

func TestHandshake_PreventiveINQ(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	cfg := DefaultConfig()
	cfg.MasterTimeout = 5 * time.Second
	cfg.StepTimeout = 2 * time.Second
	cfg.FirstStepTimeout = 300 * time.Millisecond // Short first timeout
	cfg.CharacterTimeout = 100 * time.Millisecond
	cfg.PreventiveINQ = true
	cfg.Protocols = []string{}

	sess := NewSessionWithInfoAndConfig(local, "2:5001/0", "TestLocal", "Tester", "Test", cfg)

	go func() {
		defer remote.Close()

		ch := drainReader(remote)

		// Wait for our INQ (sent after first timeout due to PreventiveINQ)
		waitForData(ch, "EMSI_INQ", 3*time.Second)

		// Respond with REQ
		_, _ = remote.Write([]byte(EMSI_REQ + "\r"))

		// Wait for our DAT
		waitForData(ch, "EMSI_DAT", 3*time.Second)

		// ACK
		_, _ = remote.Write([]byte(EMSI_ACK + "\r"))

		// Wait for REQ
		waitForData(ch, "EMSI_REQ", 3*time.Second)

		// Their DAT
		datPacket := buildRemoteDATPacket("Preventive INQ System", "2:5001/900")
		_, _ = remote.Write([]byte(datPacket))

		// Let drain finish
		time.Sleep(100 * time.Millisecond)
	}()

	err := sess.Handshake()
	if err != nil {
		t.Fatalf("Handshake with PreventiveINQ failed: %v", err)
	}

	info := sess.GetRemoteInfo()
	if info == nil {
		t.Fatal("remote info is nil")
	}
	if info.SystemName != "Preventive INQ System" {
		t.Errorf("SystemName = %q, want %q", info.SystemName, "Preventive INQ System")
	}
}

// ============================================================================
// Test utilities
// ============================================================================

// buildRemoteDATPacket creates a complete EMSI_DAT packet for a simulated remote.
func buildRemoteDATPacket(systemName, address string) string {
	data := &EMSIData{
		SystemName:    systemName,
		Location:      "Test Location",
		Sysop:         "Remote Sysop",
		Phone:         "-Unpublished-",
		Speed:         "9600",
		Flags:         "CM,XA",
		MailerName:    "TestRemote",
		MailerVersion: "2.0",
		MailerSerial:  "TST",
		Addresses:     []string{address},
		Protocols:     []string{"ZMO", "ZAP"},
	}
	return CreateEMSI_DAT(data) + "\r"
}

// ============================================================================
// isTimeoutError test
// ============================================================================

func TestIsTimeoutError(t *testing.T) {
	if isTimeoutError(nil) {
		t.Error("nil should not be a timeout error")
	}
	if isTimeoutError(fmt.Errorf("random error")) {
		t.Error("random error should not be a timeout error")
	}
}
