package protocols

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
)

// errVMPNoLocalPort reports that we could not open the listener the answering
// node must dial back to. That is a fault on this side, not evidence about the
// node, so it must never be mistaken for "this peer is not a VMODEM".
var errVMPNoLocalPort = errors.New("vmp: no free local port for the data channel")

// vmpDialConfig tunes an outgoing VMP call.
//
// A VMP call cannot be placed from a host that only makes outbound
// connections: the answering node dials the DATA channel back to us, so the
// tester must be reachable inbound on the port it advertises. PortMin/PortMax
// exist so that range can be opened in a firewall; leaving both at zero uses an
// ephemeral port, which is correct when the whole ephemeral range is reachable.
type vmpDialConfig struct {
	ConnectTimeout time.Duration // TCP connect and control-frame read timeout
	RingTimeout    time.Duration // how long to let the remote ring its mailer
	ListenHost     string        // bind address for the data channel ("" = all interfaces)
	PreferredPort  int           // tried first; 0 to skip straight to the range
	PortMin        int           // low bound of the data-channel port range (0 = ephemeral)
	PortMax        int           // high bound of the data-channel port range
	Debug          bool
}

func (c vmpDialConfig) connectTimeout() time.Duration {
	if c.ConnectTimeout > 0 {
		return c.ConnectTimeout
	}
	return 15 * time.Second
}

func (c vmpDialConfig) ringTimeout() time.Duration {
	if c.RingTimeout > 0 {
		return c.RingTimeout
	}
	return 45 * time.Second
}

// hasFallbackRange reports whether a usable port range was configured behind
// the preferred port.
func (c vmpDialConfig) hasFallbackRange() bool {
	return c.PortMin > 0 && c.PortMax >= c.PortMin
}

// singlePort reports whether this configuration can only ever bind one port, in
// which case calls must be serialized rather than racing for it.
func (c vmpDialConfig) singlePort() bool {
	if c.PreferredPort > 0 {
		return !c.hasFallbackRange()
	}
	return c.PortMin > 0 && c.PortMin == c.PortMax
}

// vmpFirstFrameWait bounds how long a peer has to react to the connect frame
// before we conclude it is not a VMODEM. A real one answers in well under a
// second — either RINGING or a disconnect naming why it cannot call back.
const vmpFirstFrameWait = 10 * time.Second

// vmpOutcome says how a VMP call ended.
type vmpOutcome int

const (
	vmpOutcomeNotVMP       vmpOutcome = iota // the peer is not speaking VMP
	vmpOutcomeConnected                      // the mailer answered; the data channel is live
	vmpOutcomeBusy                           // the remote has no free virtual COM port
	vmpOutcomeNoAnswer                       // the remote rang but its mailer never picked up
	vmpOutcomeDisconnected                   // the remote hung up; Reason says why
)

// vmpCall is the result of placing an outgoing VMP call. When Outcome is
// vmpOutcomeConnected, Data carries the live binary session and the caller owns
// it until Close.
type vmpCall struct {
	Outcome  vmpOutcome
	Reason   int    // disconnect reason, when Outcome is vmpOutcomeDisconnected
	Rings    int    // RINGING frames observed
	Greeting []byte // bytes the peer sent instead of VMP framing, when Outcome is vmpOutcomeNotVMP
	DataFrom string // set when the data channel came from a different address than the control channel

	Data net.Conn // the reverse binary data channel (nil unless connected)

	ctrl     net.Conn
	listener net.Listener
	debug    bool

	mu       sync.Mutex
	closed   bool
	accepted chan net.Conn // hand-off from the accept goroutine
}

// IsVMODEM reports whether the peer proved itself a VMODEM, regardless of
// whether a mailer ultimately answered.
func (c *vmpCall) IsVMODEM() bool {
	return c.Outcome != vmpOutcomeNotVMP
}

// SilentPeer reports that the peer swallowed our hello and connect request
// without sending a single byte back.
//
// This is deliberately not treated as VMODEM, because a wedged or ignoring TCP
// service looks exactly the same. But it is not proof of the opposite either: a
// genuine VMODEM whose reverse connection to our advertised port is being
// dropped sits blocked in connect() and says nothing at all for as long as its
// operating system takes to give up — far longer than we wait. The two are
// indistinguishable from here, so record the ambiguity rather than assert
// either side of it.
func (c *vmpCall) SilentPeer() bool {
	return c.Outcome == vmpOutcomeNotVMP && len(c.Greeting) == 0
}

// Close hangs up: it sends a normal-hangup disconnect frame on the control
// channel (reason 2, what VMODEM itself sends when DTR drops) and closes both
// connections and the listener.
func (c *vmpCall) Close() {
	if c.ctrl != nil {
		// Only a peer that has shown itself to be VMODEM gets a VMP frame; to
		// anything else this would just be noise on the way out.
		if c.Outcome != vmpOutcomeNotVMP {
			_ = c.ctrl.SetWriteDeadline(time.Now().Add(2 * time.Second))
			_, _ = c.ctrl.Write(vmpCommandFrame(vmpCmdDisconnect, vmpReasonLocalHangup))
		}
		_ = c.ctrl.Close()
		c.ctrl = nil
	}
	if c.Data != nil {
		_ = c.Data.Close()
		c.Data = nil
	}
	if c.listener != nil {
		_ = c.listener.Close()
		c.listener = nil
	}

	// The answerer opens the data channel BEFORE it rings, so an un-answered or
	// busy call still leaves one accepted and unclaimed. Closing it releases the
	// remote's virtual modem port instead of waiting on a GC finalizer.
	c.mu.Lock()
	c.closed = true
	accepted := c.accepted
	c.accepted = nil
	c.mu.Unlock()
	if accepted != nil {
		for {
			select {
			case conn := <-accepted:
				if conn != nil {
					_ = conn.Close()
				}
			default:
				return
			}
		}
	}
}

// takeDataChannel claims the accepted data connection, or reports that the
// accept goroutine should close whatever it gets.
func (c *vmpCall) offerDataChannel(conn net.Conn) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.accepted == nil {
		return false
	}
	select {
	case c.accepted <- conn:
		return true
	default:
		return false
	}
}

// OutcomeToken renders the outcome as a short, stable token for storage.
// Describe's prose is for humans and its wording will drift; this is what
// analytics group by, so the vocabulary is fixed: "connected", "busy",
// "no-answer", "not-vmp", and one token per disconnect reason.
func (c *vmpCall) OutcomeToken() string {
	switch c.Outcome {
	case vmpOutcomeConnected:
		return "connected"
	case vmpOutcomeBusy:
		return "busy"
	case vmpOutcomeNoAnswer:
		return "no-answer"
	case vmpOutcomeDisconnected:
		switch c.Reason {
		case vmpReasonRingAborted:
			return "ring-aborted"
		case vmpReasonLocalHangup:
			return "remote-hangup"
		case vmpReasonBadState:
			return "bad-state"
		case vmpReasonNoDataChannel:
			return "no-data-channel"
		case vmpReasonAborted:
			return "remote-aborted"
		case vmpReasonShortConnect:
			return "short-connect"
		case vmpReasonLoopback:
			return "loopback"
		case vmpReasonNotVModem:
			return "rejected-our-hello"
		}
		return fmt.Sprintf("disconnect-%d", c.Reason)
	default:
		return "not-vmp"
	}
}

// Describe renders the outcome for logs and the stored test detail.
func (c *vmpCall) Describe() string {
	switch c.Outcome {
	case vmpOutcomeConnected:
		return "VMP call established"
	case vmpOutcomeBusy:
		return "VMP responder busy: no free virtual modem port"
	case vmpOutcomeNoAnswer:
		return fmt.Sprintf("VMP call placed, remote mailer did not answer (%d rings)", c.Rings)
	case vmpOutcomeDisconnected:
		if c.Reason == vmpReasonNoDataChannel {
			return "VMP responder could not reach our data channel — the tester must be reachable inbound"
		}
		return "VMP call rejected: " + vmpDisconnectReasonText(c.Reason)
	default:
		return "not a VMP responder"
	}
}

// vmpListen opens the listener the answering node will connect back to.
func vmpListen(cfg vmpDialConfig) (net.Listener, int, error) {
	bind := func(port int) (net.Listener, error) {
		return net.Listen("tcp", net.JoinHostPort(cfg.ListenHost, strconv.Itoa(port)))
	}

	// VMODEM's own caller always tries 14592 first, so nodes firewalled "for
	// VMODEM" tend to permit that port and little else — 2:371/52 refuses a
	// callback to 50090 outright but completes the call on 14592.
	if cfg.PreferredPort > 0 {
		if ln, err := bind(cfg.PreferredPort); err == nil {
			return ln, cfg.PreferredPort, nil
		}
		if !cfg.hasFallbackRange() {
			// Never quietly fall back to an arbitrary port here. If the
			// operator named one port, every other port is probably firewalled,
			// and a callback to one would come back as reason 4 — which reads
			// as "the node cannot reach us" when the truth is that we asked it
			// to call a port nobody can reach.
			return nil, 0, fmt.Errorf("%w: preferred port %d is busy and no fallback range is configured",
				errVMPNoLocalPort, cfg.PreferredPort)
		}
	}

	if !cfg.hasFallbackRange() {
		ln, err := bind(0)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: %v", errVMPNoLocalPort, err)
		}
		return ln, ln.Addr().(*net.TCPAddr).Port, nil
	}

	// Fixed range: probe it in a random order so concurrent tests rarely
	// collide, and so a wedged port doesn't block every call.
	span := cfg.PortMax - cfg.PortMin + 1
	start := rand.Intn(span)
	for i := 0; i < span; i++ {
		port := cfg.PortMin + (start+i)%span
		if ln, err := bind(port); err == nil {
			return ln, port, nil
		}
	}
	return nil, 0, fmt.Errorf("%w: none free in %d-%d", errVMPNoLocalPort, cfg.PortMin, cfg.PortMax)
}

// vmpDial places an outgoing VMP call to host:port and drives it as far as it
// will go. It returns a non-nil call for every outcome including "not VMP", so
// the caller can always inspect what happened; err is reserved for local
// failures (no listener, TCP connect refused).
func vmpDial(ctx context.Context, host string, port int, cfg vmpDialConfig) (*vmpCall, error) {
	dialer := net.Dialer{Timeout: cfg.connectTimeout()}
	ctrl, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	return vmpDialOn(ctx, ctrl, cfg)
}

// vmpDialOn runs the caller side of VMP over an already-open control
// connection. It takes ownership of ctrl: the returned call closes it.
//
// Reusing the connection the tester already opened matters, because a VMODEM
// that has accepted a caller sits blocked reading the opening frame — exactly
// what a real VMODEM client finds when it dials.
func vmpDialOn(ctx context.Context, ctrl net.Conn, cfg vmpDialConfig) (*vmpCall, error) {
	host, port := "", 0
	if addr, ok := ctrl.RemoteAddr().(*net.TCPAddr); ok {
		host, port = addr.IP.String(), addr.Port
	}

	call := &vmpCall{ctrl: ctrl, debug: cfg.Debug}

	listener, dataPort, err := vmpListen(cfg)
	if err != nil {
		_ = ctrl.Close()
		call.ctrl = nil
		return nil, err
	}
	call.listener = listener

	// Step 1: the hello. A genuine VMODEM accepts it silently; anything else
	// either greets us in some other protocol or rejects the frame outright.
	_ = ctrl.SetWriteDeadline(time.Now().Add(cfg.connectTimeout()))
	if _, err := ctrl.Write(vmpHelloFrame(vmpAnyPort)); err != nil {
		call.Close()
		return nil, fmt.Errorf("vmp hello: %w", err)
	}

	// VMODEM's own client pauses between the hello and the connect request;
	// mirror that, and use the pause to notice a peer that isn't VMODEM.
	_ = ctrl.SetReadDeadline(time.Now().Add(700 * time.Millisecond))
	probe := readSome(ctrl, 512)
	if len(probe) > 0 {
		if ok, _ := looksLikeVMP(probe); !ok {
			call.Outcome = vmpOutcomeNotVMP
			call.Greeting = probe
			return call, nil
		}
	}

	fr := newVMPFrameReader(newPrefixConn(ctrl, probe))

	// Any frame arriving before we ask to connect can only be a rejection.
	if len(probe) > 0 {
		frame, err := fr.ReadFrame()
		if err == nil && frame.Command == vmpCmdDisconnect {
			call.Outcome = vmpOutcomeDisconnected
			call.Reason = frame.Arg(0)
			return call, nil
		}
	}

	// Step 2: start accepting before asking, because the answerer dials the
	// data channel while handling the connect frame.
	accepted := make(chan net.Conn, 1)
	call.mu.Lock()
	call.accepted = accepted
	call.mu.Unlock()
	go acceptDataChannel(listener, remoteIP(ctrl), call, cfg.Debug)

	_ = ctrl.SetWriteDeadline(time.Now().Add(cfg.connectTimeout()))
	if _, err := ctrl.Write(vmpConnectFrame(dataPort)); err != nil {
		call.Close()
		return nil, fmt.Errorf("vmp connect: %w", err)
	}

	// Step 3: follow the call until it resolves.
	//
	// Two deadlines, because they answer different questions. A VMODEM reacts
	// to the connect frame promptly — it either starts ringing or reports that
	// it cannot reach us — so silence past vmpFirstFrameWait means this was
	// never a VMODEM, and we must give up quickly rather than hold a peer that
	// is merely slow to greet for the whole ring timeout. Once a frame has
	// arrived, the longer ring budget applies.
	ringDeadline := time.Now().Add(cfg.ringTimeout())
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(ringDeadline) {
		ringDeadline = ctxDeadline
	}
	firstFrameDeadline := time.Now().Add(vmpFirstFrameWait)
	if firstFrameDeadline.After(ringDeadline) {
		firstFrameDeadline = ringDeadline
	}

	sawFrame := false
	give_up := func() (*vmpCall, error) {
		if call.Rings > 0 || sawFrame {
			call.Outcome = vmpOutcomeNoAnswer
		} else {
			call.Outcome = vmpOutcomeNotVMP
		}
		return call, nil
	}

	for {
		deadline := ringDeadline
		if !sawFrame {
			deadline = firstFrameDeadline
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return give_up()
		}
		_ = ctrl.SetReadDeadline(deadline)
		frame, err := fr.ReadFrame()
		if err != nil {
			// A peer that took our frames and then went away without ever
			// ringing was not a VMODEM after all.
			return give_up()
		}
		sawFrame = true
		if cfg.Debug {
			logging.Debugf("VMP %s:%d <- command %d args %v", host, port, frame.Command, frame.Payload)
		}

		switch frame.Command {
		case vmpCmdRinging:
			call.Rings++

		case vmpCmdConnected:
			timer := time.NewTimer(2 * time.Second)
			select {
			case data := <-accepted:
				timer.Stop()
				call.Data = data
				call.Outcome = vmpOutcomeConnected
			case <-ctx.Done():
				timer.Stop()
				call.Outcome = vmpOutcomeDisconnected
				call.Reason = vmpReasonAborted
			case <-timer.C:
				// CONNECT without a data channel should not happen — the
				// answerer opens it first — but never report success without one.
				call.Outcome = vmpOutcomeDisconnected
				call.Reason = vmpReasonNoDataChannel
			}
			return call, nil

		case vmpCmdBusy:
			call.Outcome = vmpOutcomeBusy
			return call, nil

		case vmpCmdDisconnect:
			call.Outcome = vmpOutcomeDisconnected
			call.Reason = frame.Arg(0)
			return call, nil

		case vmpCmdNAK:
			call.Outcome = vmpOutcomeDisconnected
			call.Reason = vmpReasonBadState
			return call, nil
		}
	}
}

// acceptDataChannel waits for the answering node to dial back.
//
// The connection is deliberately NOT required to come from the address the
// control channel was answered on. Real VMODEM hosts are multi-homed or NAT'd
// and routinely call back from a different address: 2:5025/2 answers on
// 109.106.139.152 and dials back from 95.32.211.149. Rejecting that mismatch
// silently breaks the call — the node rings its mailer, we hang up on the data
// connection, and the whole thing looks like an unreachable data channel.
//
// This is safe because the listener is opened for one call and closed with it,
// and because identity is established by the EMSI address exchange that
// follows, not by the source address. A mismatch is recorded for diagnostics.
func acceptDataChannel(ln net.Listener, peerIP string, call *vmpCall, debug bool) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	if from := remoteIP(conn); peerIP != "" && from != peerIP {
		call.mu.Lock()
		call.DataFrom = from
		call.mu.Unlock()
		if debug {
			logging.Debugf("VMP data channel came from %s, control channel is %s (multi-homed peer)",
				from, peerIP)
		}
	}
	if !call.offerDataChannel(conn) {
		_ = conn.Close()
	}
}

// remoteIP returns the IP part of a connection's remote address.
func remoteIP(c net.Conn) string {
	host, _, err := net.SplitHostPort(c.RemoteAddr().String())
	if err != nil {
		return ""
	}
	return host
}
