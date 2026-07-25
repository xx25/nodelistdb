package protocols

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestVMPEncodeFrameStuffsDLE(t *testing.T) {
	// Payload length 0x10 forces a DLE in the length word, and the payload
	// itself carries one; both must be doubled, the marker must not.
	payload := make([]byte, 0x10)
	payload[0] = 0x00
	payload[1] = 0x02
	payload[4] = vmpDLE

	got := vmpEncodeFrame(payload)
	if !bytes.HasPrefix(got, []byte{vmpDLE, vmpSTX}) {
		t.Fatalf("frame does not start with the DLE STX marker: % x", got[:2])
	}
	// length 0x0010 -> high byte 0x00, low byte 0x10 (stuffed)
	if !bytes.HasPrefix(got[2:], []byte{0x00, vmpDLE, vmpDLE}) {
		t.Errorf("length word not DLE-stuffed: % x", got[2:6])
	}
	if bytes.Count(got, []byte{vmpDLE, vmpDLE}) != 2 {
		t.Errorf("expected two doubled DLEs (length + payload), got % x", got)
	}
}

func TestVMPHelloFrameMatchesVMODEMValidator(t *testing.T) {
	// VMODEM.EXE's validator (0x240b) reads exactly 10 bytes and checks the
	// marker, a big-endian command word of 6, a little-endian protocol level
	// >= 1, and the requested port. This is the exact byte string a real
	// VMODEM accepted on the wire.
	want := []byte{0x10, 0x02, 0x00, 0x06, 0x00, 0x06, 0x01, 0x00, 0xff, 0xff}
	if got := vmpHelloFrame(vmpAnyPort); !bytes.Equal(got, want) {
		t.Errorf("hello frame = % x, want % x", got, want)
	}
}

func TestVMPConnectFrameCarriesPortBigEndian(t *testing.T) {
	want := []byte{0x10, 0x02, 0x00, 0x08, 0x00, 0x00, 0x30, 0x39, 0, 0, 0, 0}
	if got := vmpConnectFrame(12345); !bytes.Equal(got, want) {
		t.Errorf("connect frame = % x, want % x", got, want)
	}
}

func TestVMPFrameReaderRoundTrip(t *testing.T) {
	var stream bytes.Buffer
	stream.Write([]byte("garbage the parser must discard"))
	stream.Write(vmpCommandFrame(vmpCmdRinging))
	stream.Write([]byte{0x10, 0x99}) // a DLE that isn't a frame start
	stream.Write(vmpCommandFrame(vmpCmdDisconnect, vmpReasonNotVModem))

	fr := newVMPFrameReader(&stream)

	f1, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("first frame: %v", err)
	}
	if f1.Command != vmpCmdRinging {
		t.Errorf("first command = %d, want %d", f1.Command, vmpCmdRinging)
	}

	f2, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("second frame: %v", err)
	}
	if f2.Command != vmpCmdDisconnect || f2.Arg(0) != vmpReasonNotVModem {
		t.Errorf("second frame = cmd %d arg %d, want cmd %d arg %d",
			f2.Command, f2.Arg(0), vmpCmdDisconnect, vmpReasonNotVModem)
	}

	if _, err := fr.ReadFrame(); !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF after the last frame, got %v", err)
	}
}

func TestVMPFrameReaderRecoversFromTruncatedFrame(t *testing.T) {
	var stream bytes.Buffer
	// A frame header claiming 6 payload bytes but carrying an unpaired DLE,
	// which cannot occur inside a real frame body: the reader must resync
	// rather than swallow the following good frame.
	stream.Write([]byte{0x10, 0x02, 0x00, 0x06, 0x00, 0x02, 0x10, 0x41})
	stream.Write(vmpCommandFrame(vmpCmdBusy))

	fr0 := newVMPFrameReader(&stream)
	if f, err := fr0.ReadFrame(); err != nil || f.Command != vmpCmdBusy {
		t.Fatalf("ReadFrame after unpaired DLE = cmd %d err %v, want busy", f.Command, err)
	}

	// The harder case: the truncated body runs straight into the next frame's
	// marker with no separating byte, so the resync must not eat the marker.
	stream.Reset()
	stream.Write([]byte{0x10, 0x02, 0x00, 0x06, 0x00, 0x02})
	stream.Write(vmpCommandFrame(vmpCmdBusy))

	fr := newVMPFrameReader(&stream)
	f, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if f.Command != vmpCmdBusy {
		t.Errorf("command = %d, want %d (busy)", f.Command, vmpCmdBusy)
	}
}

func TestLooksLikeVMPRejectsCallerOnlyCommands(t *testing.T) {
	// Commands 0 (connect) and 6 (hello) only ever travel caller->answerer, so
	// seeing one in a reply means the peer is not an answering VMODEM.
	for _, cmd := range []int{vmpCmdConnect, vmpCmdHello, 9, 0x14} {
		if ok, _ := looksLikeVMP(vmpCommandFrame(cmd, 0)); ok {
			t.Errorf("command %d accepted as an answerer frame", cmd)
		}
	}
	for _, cmd := range []int{vmpCmdDisconnect, vmpCmdRinging, vmpCmdConnected, vmpCmdNAK, vmpCmdBusy} {
		ok, got := looksLikeVMP(vmpCommandFrame(cmd, 0))
		if !ok || got != cmd {
			t.Errorf("command %d rejected (ok=%v got=%d)", cmd, ok, got)
		}
	}
}

// fakeVMODEM answers one VMP call the way VMODEM.EXE does, driven by a script.
type fakeVMODEM struct {
	t *testing.T
	// reply decides what to do after a valid connect frame arrives. It is
	// given the data-channel port the caller advertised.
	reply func(ctrl net.Conn, dataPort int)
	// greeting, when set, is sent instead of speaking VMP at all.
	greeting []byte

	ln       net.Listener
	helloRaw []byte
}

func startFakeVMODEM(t *testing.T, f *fakeVMODEM) *fakeVMODEM {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f.t = t
	f.ln = ln
	go f.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return f
}

func (f *fakeVMODEM) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeVMODEM) serve() {
	conn, err := f.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	if f.greeting != nil {
		_, _ = conn.Write(f.greeting)
		time.Sleep(200 * time.Millisecond)
		return
	}

	// VMODEM reads exactly 10 bytes for the opening frame.
	hello := make([]byte, 10)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, hello); err != nil {
		return
	}
	f.helloRaw = hello
	if hello[0] != vmpDLE || hello[1] != vmpSTX ||
		int(hello[4])<<8|int(hello[5]) != vmpCmdHello ||
		int(hello[7])<<8|int(hello[6]) < 1 {
		_, _ = conn.Write(vmpCommandFrame(vmpCmdDisconnect, vmpReasonNotVModem))
		return
	}

	fr := newVMPFrameReader(conn)
	frame, err := fr.ReadFrame()
	if err != nil || frame.Command != vmpCmdConnect {
		return
	}
	f.reply(conn, frame.Arg(0))
}

// dialBack mimics the answerer opening the reverse data channel.
func dialBack(t *testing.T, port int) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", itoa(port)), 3*time.Second)
	if err != nil {
		t.Fatalf("data channel dial back: %v", err)
	}
	return c
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [8]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}

func testDialConfig() vmpDialConfig {
	return vmpDialConfig{
		ConnectTimeout: 3 * time.Second,
		RingTimeout:    6 * time.Second,
		ListenHost:     "127.0.0.1",
	}
}

func TestVMPDialCompletesCallAndCarriesData(t *testing.T) {
	fake := startFakeVMODEM(t, &fakeVMODEM{
		reply: func(ctrl net.Conn, dataPort int) {
			data := dialBack(t, dataPort)
			defer data.Close()
			_, _ = ctrl.Write(vmpCommandFrame(vmpCmdRinging))
			_, _ = ctrl.Write(vmpCommandFrame(vmpCmdRinging))
			_, _ = ctrl.Write(vmpCommandFrame(vmpCmdConnected))
			_, _ = data.Write([]byte("**EMSI_REQA77E\r"))
			time.Sleep(500 * time.Millisecond)
		},
	})

	call, err := vmpDial(context.Background(), "127.0.0.1", fake.port(), testDialConfig())
	if err != nil {
		t.Fatalf("vmpDial: %v", err)
	}
	defer call.Close()

	if call.Outcome != vmpOutcomeConnected {
		t.Fatalf("outcome = %v (%s), want connected", call.Outcome, call.Describe())
	}
	if call.Rings != 2 {
		t.Errorf("rings = %d, want 2", call.Rings)
	}
	if call.Data == nil {
		t.Fatal("connected without a data channel")
	}

	_ = call.Data.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 32)
	n, err := call.Data.Read(buf)
	if err != nil {
		t.Fatalf("read from data channel: %v", err)
	}
	if got := string(buf[:n]); got != "**EMSI_REQA77E\r" {
		t.Errorf("data channel carried %q", got)
	}
}

func TestVMPDialReportsBusy(t *testing.T) {
	fake := startFakeVMODEM(t, &fakeVMODEM{
		reply: func(ctrl net.Conn, _ int) {
			_, _ = ctrl.Write(vmpCommandFrame(vmpCmdBusy))
		},
	})

	call, err := vmpDial(context.Background(), "127.0.0.1", fake.port(), testDialConfig())
	if err != nil {
		t.Fatalf("vmpDial: %v", err)
	}
	defer call.Close()

	if call.Outcome != vmpOutcomeBusy {
		t.Errorf("outcome = %v (%s), want busy", call.Outcome, call.Describe())
	}
	if !call.IsVMODEM() {
		t.Error("a busy responder is still a VMODEM")
	}
}

func TestVMPDialReportsUnreachableDataChannel(t *testing.T) {
	// The answerer cannot reach us, so it reports reason 4 — the signal that
	// the tester host is not inbound-reachable.
	fake := startFakeVMODEM(t, &fakeVMODEM{
		reply: func(ctrl net.Conn, _ int) {
			_, _ = ctrl.Write(vmpCommandFrame(vmpCmdDisconnect, vmpReasonNoDataChannel))
		},
	})

	call, err := vmpDial(context.Background(), "127.0.0.1", fake.port(), testDialConfig())
	if err != nil {
		t.Fatalf("vmpDial: %v", err)
	}
	defer call.Close()

	if call.Outcome != vmpOutcomeDisconnected || call.Reason != vmpReasonNoDataChannel {
		t.Fatalf("outcome = %v reason = %d, want disconnected/%d", call.Outcome, call.Reason, vmpReasonNoDataChannel)
	}
	if !call.IsVMODEM() {
		t.Error("a node that answered our frames is still a VMODEM")
	}
	if got := call.Describe(); got == "" || !bytes.Contains([]byte(got), []byte("reachable inbound")) {
		t.Errorf("Describe() = %q, want it to name the inbound-reachability problem", got)
	}
}

func TestVMPDialRejectsNonVMODEM(t *testing.T) {
	fake := startFakeVMODEM(t, &fakeVMODEM{
		greeting: []byte("Welcome to Some BBS\r\nlogin: "),
	})

	call, err := vmpDial(context.Background(), "127.0.0.1", fake.port(), testDialConfig())
	if err != nil {
		t.Fatalf("vmpDial: %v", err)
	}
	defer call.Close()

	if call.Outcome != vmpOutcomeNotVMP {
		t.Errorf("outcome = %v (%s), want not-VMP", call.Outcome, call.Describe())
	}
	if !bytes.Contains(call.Greeting, []byte("login:")) {
		t.Errorf("greeting not captured: %q", call.Greeting)
	}
}

func TestVMPCloseReleasesUnclaimedDataChannel(t *testing.T) {
	// The answerer opens the data channel BEFORE it rings, so a call that ends
	// without an answer still leaves one accepted. Close must release it, or
	// the remote's virtual modem port stays tied up.
	dataReady := make(chan net.Conn, 1)
	fake := startFakeVMODEM(t, &fakeVMODEM{
		reply: func(ctrl net.Conn, dataPort int) {
			data := dialBack(t, dataPort)
			dataReady <- data
			_, _ = ctrl.Write(vmpCommandFrame(vmpCmdRinging))
			time.Sleep(3 * time.Second) // ring, but never answer
		},
	})

	cfg := testDialConfig()
	cfg.RingTimeout = 1500 * time.Millisecond
	call, err := vmpDial(context.Background(), "127.0.0.1", fake.port(), cfg)
	if err != nil {
		t.Fatalf("vmpDial: %v", err)
	}
	if call.Outcome != vmpOutcomeNoAnswer {
		t.Fatalf("outcome = %v (%s), want no-answer", call.Outcome, call.Describe())
	}
	call.Close()

	// The answerer's end must now see the connection go away.
	answererSide := <-dataReady
	defer answererSide.Close()
	_ = answererSide.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := answererSide.Read(make([]byte, 1)); err == nil {
		t.Fatal("data channel still readable after Close")
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("data channel was leaked: Close did not shut it down")
	}
}

func TestVMPListenRefusesToSubstituteAnUnreachablePort(t *testing.T) {
	// With one named port and no fallback range, a busy port must be an error.
	// Binding some other port instead would ask the node to call back somewhere
	// the firewall does not permit, and come back as reason 4 — which reads as
	// "the node cannot reach us" when in fact we gave it a bad port.
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer held.Close()
	busy := held.Addr().(*net.TCPAddr).Port

	cfg := vmpDialConfig{ListenHost: "127.0.0.1", PreferredPort: busy}
	if _, _, err := vmpListen(cfg); !errors.Is(err, errVMPNoLocalPort) {
		t.Fatalf("vmpListen err = %v, want errVMPNoLocalPort", err)
	}

	// With a fallback range it may substitute, because the operator has said
	// those ports are reachable too.
	cfg.PortMin, cfg.PortMax = 0, 0
	cfg.PreferredPort = busy
	cfg2 := cfg
	cfg2.PortMin, cfg2.PortMax = 41000, 41010
	ln, port, err := vmpListen(cfg2)
	if err != nil {
		t.Fatalf("vmpListen with fallback range: %v", err)
	}
	defer ln.Close()
	if port < 41000 || port > 41010 {
		t.Errorf("fallback port = %d, want it inside 41000-41010", port)
	}
}

func TestVMPSinglePortSerializesCalls(t *testing.T) {
	one := vmpDialConfig{PreferredPort: 14592}
	if !one.singlePort() {
		t.Error("a preferred port with no fallback range must serialize")
	}
	if one.hasFallbackRange() {
		t.Error("no range was configured")
	}

	both := vmpDialConfig{PreferredPort: 14592, PortMin: 50000, PortMax: 50100}
	if both.singlePort() {
		t.Error("a fallback range allows concurrent calls")
	}

	pinned := vmpDialConfig{PortMin: 50000, PortMax: 50000}
	if !pinned.singlePort() {
		t.Error("a one-port range must serialize")
	}
}

func TestVMPDialGivesUpQuicklyOnSilentNonVMODEM(t *testing.T) {
	// A peer that swallows the handshake without ever answering must not hold
	// us for the whole ring budget — that window is for mailers that are
	// actually ringing.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		time.Sleep(30 * time.Second) // silent, never replies
	}()

	cfg := testDialConfig()
	cfg.RingTimeout = 25 * time.Second

	start := time.Now()
	call, err := vmpDial(context.Background(), "127.0.0.1", ln.Addr().(*net.TCPAddr).Port, cfg)
	if err != nil {
		t.Fatalf("vmpDial: %v", err)
	}
	defer call.Close()
	elapsed := time.Since(start)

	if call.Outcome != vmpOutcomeNotVMP {
		t.Errorf("outcome = %v (%s), want not-VMP", call.Outcome, call.Describe())
	}
	// Silence is not proof either way: a VMODEM blocked dialling a data-channel
	// port it cannot reach looks exactly like this, so the call has to say so.
	if !call.SilentPeer() {
		t.Error("SilentPeer() = false for a peer that never sent a byte")
	}
	if elapsed > vmpFirstFrameWait+3*time.Second {
		t.Errorf("waited %v for a silent peer; should give up near %v", elapsed, vmpFirstFrameWait)
	}
}

func TestVMPSilentPeerIsNotClaimedForATalkingPeer(t *testing.T) {
	// A peer that answers with something other than VMP framing is identified,
	// not ambiguous — SilentPeer must not swallow that distinction.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = c.Write([]byte("**EMSI_REQA77E\r"))
		time.Sleep(time.Second)
	}()

	call, err := vmpDial(context.Background(), "127.0.0.1", ln.Addr().(*net.TCPAddr).Port, testDialConfig())
	if err != nil {
		t.Fatalf("vmpDial: %v", err)
	}
	defer call.Close()

	if call.Outcome != vmpOutcomeNotVMP {
		t.Errorf("outcome = %v (%s), want not-VMP", call.Outcome, call.Describe())
	}
	if call.SilentPeer() {
		t.Error("SilentPeer() = true for a peer that greeted us")
	}
}

func TestVMPDialAcceptsDataChannelFromMultiHomedPeer(t *testing.T) {
	// Real VMODEM hosts answer on one address and dial back from another
	// (2:5025/2 answers on 109.106.139.152, calls back from 95.32.211.149).
	// Refusing the mismatch strands the call after the node has already rung
	// its mailer, so the connection must be taken and the mismatch recorded.
	fake := startFakeVMODEM(t, &fakeVMODEM{
		reply: func(ctrl net.Conn, dataPort int) {
			d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.2")}}
			data, err := d.Dial("tcp", net.JoinHostPort("127.0.0.1", itoa(dataPort)))
			if err != nil {
				return
			}
			defer data.Close()
			_, _ = ctrl.Write(vmpCommandFrame(vmpCmdConnected))
			_, _ = data.Write([]byte("**EMSI_REQA77E\r"))
			time.Sleep(500 * time.Millisecond)
		},
	})

	call, err := vmpDial(context.Background(), "127.0.0.1", fake.port(), testDialConfig())
	if err != nil {
		t.Fatalf("vmpDial: %v", err)
	}
	defer call.Close()

	if call.Outcome != vmpOutcomeConnected {
		t.Fatalf("outcome = %v (%s), want connected", call.Outcome, call.Describe())
	}
	if call.DataFrom != "127.0.0.2" {
		t.Errorf("DataFrom = %q, want the mismatched address to be recorded", call.DataFrom)
	}
}
