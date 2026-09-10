package binkp

import (
	"io"
	"net"
	"testing"
	"time"
)

// fakeMailer is the answering side of a BinkP session, scripted per test.
type fakeMailer struct {
	t      *testing.T
	conn   net.Conn
	frames []*Frame // every command frame read after the handshake
}

func startFakeMailer(t *testing.T, script func(m *fakeMailer)) (addr string, done chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done = make(chan struct{})
	go func() {
		defer close(done)
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		m := &fakeMailer{t: t, conn: conn}
		m.handshake()
		script(m)
	}()
	return ln.Addr().String(), done
}

// handshake plays the answerer: announce, read the caller's frames up to
// M_PWD, then M_OK.
func (m *fakeMailer) handshake() {
	for _, f := range []*Frame{
		CreateM_NUL("SYS", "Fake Mailer"),
		CreateM_NUL("VER", "fake/1.0 binkp/1.1"),
		CreateM_ADR("1:320/219@fidonet"),
	} {
		if err := WriteFrame(m.conn, f); err != nil {
			m.t.Errorf("fake mailer write: %v", err)
			return
		}
	}
	for {
		f, err := ReadFrame(m.conn)
		if err != nil {
			m.t.Errorf("fake mailer read during handshake: %v", err)
			return
		}
		if f.Command && f.Type == M_PWD {
			break
		}
	}
	if err := WriteFrame(m.conn, CreateM_OK()); err != nil {
		m.t.Errorf("fake mailer M_OK: %v", err)
	}
}

func (m *fakeMailer) send(f *Frame) {
	if err := WriteFrame(m.conn, f); err != nil {
		m.t.Errorf("fake mailer write: %v", err)
	}
}

// expect reads frames until one of the given type arrives, or fails.
func (m *fakeMailer) expect(typ uint8) *Frame {
	for {
		f, err := ReadFrame(m.conn)
		if err != nil {
			m.t.Errorf("fake mailer: waiting for %s: %v", frameTypeNames[typ], err)
			return nil
		}
		if !f.Command {
			continue
		}
		m.frames = append(m.frames, f)
		if f.Type == typ {
			return f
		}
	}
}

// expectClose reads until the caller closes the connection.
func (m *fakeMailer) expectClose() {
	buf := make([]byte, 64)
	for {
		if _, err := m.conn.Read(buf); err != nil {
			if err != io.EOF {
				m.t.Errorf("fake mailer: expected EOF, got %v", err)
			}
			return
		}
	}
}

func runCaller(t *testing.T, addr string) *Session {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSession(conn, "2:5001/5001")
	s.SetTimeout(5 * time.Second)
	if err := s.Handshake(); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	return s
}

func countEOB(frames []*Frame) int {
	n := 0
	for _, f := range frames {
		if f.Type == M_EOB {
			n++
		}
	}
	return n
}

// MBSE's mbcico counts three command frames in the first empty batch (its
// TRF, its M_EOB, our M_EOB), takes that as "something happened", and opens
// a second batch by sending M_EOB again. The caller has to answer that one
// too, and only then does mbcico end the session and close.
func TestCloseAnswersMBSESecondBatch(t *testing.T) {
	var m2 *fakeMailer
	addr, done := startFakeMailer(t, func(m *fakeMailer) {
		m2 = m
		m.send(CreateM_NUL("TRF", "0 0"))
		m.send(&Frame{Type: M_EOB, Command: true})
		if m.expect(M_EOB) == nil {
			return
		}
		// "Binkp: receiver starts batch 2"
		m.send(&Frame{Type: M_EOB, Command: true})
		if m.expect(M_EOB) == nil {
			return
		}
		// Batch 2 saw fewer than three frames: session complete, we close.
	})
	s := runCaller(t, addr)
	start := time.Now()
	_ = s.Close()
	<-done
	if got := countEOB(m2.frames); got != 2 {
		t.Fatalf("mailer received %d M_EOB, want 2 (one per batch)", got)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("close took %v; should return as soon as the mailer hangs up", time.Since(start))
	}
}

// binkd closes as soon as both sides have sent M_EOB in an empty session.
func TestCloseAcceptsRemoteHangupAfterFirstEOB(t *testing.T) {
	var m2 *fakeMailer
	addr, done := startFakeMailer(t, func(m *fakeMailer) {
		m2 = m
		m.send(&Frame{Type: M_EOB, Command: true})
		m.expect(M_EOB)
	})
	s := runCaller(t, addr)
	_ = s.Close()
	<-done
	if got := countEOB(m2.frames); got != 1 {
		t.Fatalf("mailer received %d M_EOB, want 1", got)
	}
}

// A mailer that has sent M_EOB and then neither closes nor speaks gets the
// idle window, after which we hang up ourselves.
func TestCloseGivesUpOnSilentRemote(t *testing.T) {
	addr, done := startFakeMailer(t, func(m *fakeMailer) {
		m.send(&Frame{Type: M_EOB, Command: true})
		m.expect(M_EOB)
		m.expectClose()
	})
	s := runCaller(t, addr)
	start := time.Now()
	_ = s.Close()
	elapsed := time.Since(start)
	<-done
	if elapsed < eobIdleWindow || elapsed > eobIdleWindow+2*time.Second {
		t.Fatalf("close took %v; want about the idle window (%v)", elapsed, eobIdleWindow)
	}
}

// A file offered to us is declined with M_SKIP, so the mailer can move on to
// its own M_EOB instead of waiting on a receiver that does not exist.
func TestCloseDeclinesOfferedFile(t *testing.T) {
	var m2 *fakeMailer
	addr, done := startFakeMailer(t, func(m *fakeMailer) {
		m2 = m
		m.send(&Frame{Type: M_FILE, Command: true, Data: []byte("00000001.pkt 123 1700000000 0")})
		if m.expect(M_SKIP) == nil {
			return
		}
		// The caller's own M_EOB went out before it read our offer, so by
		// now it has been read; after ours the session is complete.
		m.send(&Frame{Type: M_EOB, Command: true})
		m.expectClose()
	})
	s := runCaller(t, addr)
	_ = s.Close()
	<-done
	var skip *Frame
	for _, f := range m2.frames {
		if f.Type == M_SKIP {
			skip = f
		}
	}
	if skip == nil {
		t.Fatal("mailer never received M_SKIP for its file")
	}
	if string(skip.Data) != "00000001.pkt 123 1700000000 0" {
		t.Fatalf("M_SKIP carried %q, want the M_FILE arguments", skip.Data)
	}
	if got := countEOB(m2.frames); got != 1 {
		t.Fatalf("mailer received %d M_EOB, want 1", got)
	}
}

// Chatter between the first EOB exchange and the reopening M_EOB does not
// use up the batch budget: the reopen is still answered.
func TestCloseAnswersReopenAfterChatter(t *testing.T) {
	var m2 *fakeMailer
	addr, done := startFakeMailer(t, func(m *fakeMailer) {
		m2 = m
		m.send(&Frame{Type: M_EOB, Command: true})
		if m.expect(M_EOB) == nil {
			return
		}
		for i := 0; i < endOfSessionRounds+2; i++ {
			m.send(CreateM_NUL("NOP", "chatter"))
		}
		m.send(&Frame{Type: M_EOB, Command: true})
		m.expect(M_EOB)
	})
	s := runCaller(t, addr)
	_ = s.Close()
	<-done
	if got := countEOB(m2.frames); got != 2 {
		t.Fatalf("mailer received %d M_EOB, want 2: the reopen after chatter was not answered", got)
	}
}

// A peer that never sends M_EOB but keeps talking is cut off by the frame
// budget rather than held to the session timeout.
func TestCloseCapsFramesFromChatteringPeer(t *testing.T) {
	addr, done := startFakeMailer(t, func(m *fakeMailer) {
		for i := 0; i < endOfSessionFrames*3; i++ {
			if err := WriteFrame(m.conn, CreateM_NUL("NOP", "chatter")); err != nil {
				return
			}
		}
		m.expectClose()
	})
	s := runCaller(t, addr)
	s.SetTimeout(30 * time.Second)
	start := time.Now()
	_ = s.Close()
	<-done
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("close took %v against a chattering peer; the frame cap should end it at once", elapsed)
	}
}

// A mailer that keeps opening batches is not followed forever.
func TestCloseBoundsBatchRounds(t *testing.T) {
	var m2 *fakeMailer
	addr, done := startFakeMailer(t, func(m *fakeMailer) {
		m2 = m
		for i := 0; i < endOfSessionRounds+3; i++ {
			m.send(&Frame{Type: M_EOB, Command: true})
			f, err := ReadFrame(m.conn)
			if err != nil {
				return
			}
			m.frames = append(m.frames, f)
		}
	})
	s := runCaller(t, addr)
	_ = s.Close()
	<-done
	if got := countEOB(m2.frames); got > endOfSessionRounds {
		t.Fatalf("answered %d batches, want at most %d", got, endOfSessionRounds)
	}
}
