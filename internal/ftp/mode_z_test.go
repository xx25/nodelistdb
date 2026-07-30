package ftp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestModeZNotAdvertised pins MODE Z off.
//
// ftpserverlib compresses with compress/flate, which emits a bare DEFLATE
// stream (RFC 1951). Every real client treats MODE Z data as a zlib stream
// (RFC 1950) and so fails on the missing 2-byte header — lftp reports
// "zlib inflate error: incorrect header check" for every listing and transfer.
// v0.29.0 forced DeflateCompressionLevel to 5 when unset, which is how this
// server shipped a MODE Z that no client could read; v0.30.0 made it opt-in.
//
// This test fails if someone sets DeflateCompressionLevel or if a future
// library version starts advertising MODE Z again by default.
func TestModeZNotAdvertised(t *testing.T) {
	ctl, cleanup := startTestServer(t)
	defer cleanup()

	feat := ctl.multiline(t, "FEAT")
	if strings.Contains(strings.ToUpper(feat), "MODE Z") {
		t.Errorf("FEAT advertises MODE Z, so clients will negotiate a mode this\n"+
			"server cannot actually speak. FEAT was:\n%s", feat)
	}

	// Refusing it is what makes a client fall back to stream mode.
	if resp := ctl.cmd(t, "MODE Z"); !strings.HasPrefix(resp, "504") {
		t.Errorf("MODE Z answered %q, want a 504 refusal", resp)
	}
	if resp := ctl.cmd(t, "MODE S"); !strings.HasPrefix(resp, "200") {
		t.Errorf("MODE S answered %q, want 200", resp)
	}
}

// TestStreamModeListingIsPlaintext is the other half of the guarantee: with
// MODE Z refused, a listing arrives uncompressed. If this ever returns deflate
// bytes, clients that never sent MODE Z would break.
func TestStreamModeListingIsPlaintext(t *testing.T) {
	ctl, cleanup := startTestServer(t)
	defer cleanup()

	ctl.cmd(t, "TYPE I")
	port := ctl.passivePort(t)

	data, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second)
	if err != nil {
		t.Fatalf("data connection: %v", err)
	}
	defer data.Close()

	ctl.cmd(t, "LIST /nodelists")
	raw, err := io.ReadAll(data)
	if err != nil {
		t.Fatalf("reading listing: %v", err)
	}

	// The directory name appearing literally is proof the bytes were not
	// compressed: a deflate stream of this listing would not contain it.
	if !strings.Contains(string(raw), "2026") {
		t.Errorf("listing is not plaintext — compression leaked into stream mode.\n"+
			"%d bytes, first 4 = % x, content: %q",
			len(raw), firstBytes(raw, 4), truncate(raw))
	}
}

// --- helpers ---

type testCtl struct {
	conn net.Conn
	br   *bufio.Reader
}

func startTestServer(t *testing.T) (*testCtl, func()) {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "2026"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "2026", "nodelist.001"), []byte(";A test\r\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Port 0 lets the kernel pick a free port, but the server needs to report
	// it back; ftpserverlib exposes the resolved address after Listen, so bind
	// a probe listener first to find a free port and hand that number over.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	srv, err := New(&Config{
		Enabled:        true,
		Host:           "127.0.0.1",
		Port:           port,
		Mounts:         []MountConfig{{VirtualPath: "/nodelists", RealPath: root}},
		MaxConnections: 4,
		IdleTimeout:    30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	go func() { _ = srv.Start() }()

	var ctl *testCtl
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			ctl = &testCtl{conn: conn, br: bufio.NewReader(conn)}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ctl == nil {
		_ = srv.Stop()
		t.Fatalf("FTP server did not come up on port %d", port)
	}

	ctl.readResponse(t)          // 220 greeting
	ctl.cmd(t, "USER anonymous") // 331
	ctl.cmd(t, "PASS test@test.invalid")

	return ctl, func() {
		ctl.conn.Close()
		_ = srv.Stop()
	}
}

// readResponse reads one complete reply, skipping continuation lines.
func (c *testCtl) readResponse(t *testing.T) string {
	t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		line, err := c.br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading reply: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if len(line) >= 4 && line[3] == ' ' {
			return line
		}
	}
}

func (c *testCtl) cmd(t *testing.T, cmd string) string {
	t.Helper()
	if _, err := fmt.Fprintf(c.conn, "%s\r\n", cmd); err != nil {
		t.Fatalf("sending %q: %v", cmd, err)
	}
	return c.readResponse(t)
}

// multiline returns every line of a multi-line reply (FEAT).
func (c *testCtl) multiline(t *testing.T, cmd string) string {
	t.Helper()
	if _, err := fmt.Fprintf(c.conn, "%s\r\n", cmd); err != nil {
		t.Fatalf("sending %q: %v", cmd, err)
	}
	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var sb strings.Builder
	for {
		line, err := c.br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading %s reply: %v", cmd, err)
		}
		sb.WriteString(line)
		trimmed := strings.TrimRight(line, "\r\n")
		if len(trimmed) >= 4 && trimmed[3] == ' ' && strings.HasPrefix(trimmed, "211") {
			return sb.String()
		}
	}
}

// passivePort issues EPSV and returns the port from "229 ... (|||PORT|)".
func (c *testCtl) passivePort(t *testing.T) int {
	t.Helper()
	resp := c.cmd(t, "EPSV")
	open := strings.Index(resp, "(|||")
	if open < 0 {
		t.Fatalf("unexpected EPSV reply: %q", resp)
	}
	rest := resp[open+4:]
	end := strings.Index(rest, "|")
	if end < 0 {
		t.Fatalf("unexpected EPSV reply: %q", resp)
	}
	var port int
	if _, err := fmt.Sscanf(rest[:end], "%d", &port); err != nil {
		t.Fatalf("parsing EPSV port from %q: %v", resp, err)
	}
	return port
}

func firstBytes(b []byte, n int) []byte {
	if len(b) < n {
		return b
	}
	return b[:n]
}

func truncate(b []byte) string {
	if len(b) > 120 {
		return string(b[:120]) + "..."
	}
	return string(b)
}
