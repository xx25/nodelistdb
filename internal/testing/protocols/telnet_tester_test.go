package protocols

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/xx25/fidomail/pkg/emsi"
	"github.com/xx25/fidomail/pkg/xfer"
)

// fakeEMSIMailer answers on a loopback port the way a mailer does: telnet
// option negotiation when asked, the FSC-0056 answering side of EMSI, then
// the negotiated transfer protocol as answerer with nothing to send. It is
// fidomail's own answerer, so what it reports about the session's end is
// what a real mailer would log.
type fakeEMSIMailer struct {
	addr       string
	overTelnet bool
	banner     []byte // printed before EMSI begins, as many mailers do
	done       chan struct{}
	xferErr    error // how the transfer phase ended for the mailer
	hsErr      error
	remote     *emsi.EMSIData
}

func startFakeEMSIMailer(t *testing.T, overTelnet bool) *fakeEMSIMailer {
	return startFakeEMSIMailerWithBanner(t, overTelnet, nil)
}

func startFakeEMSIMailerWithBanner(t *testing.T, overTelnet bool, banner []byte) *fakeEMSIMailer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := &fakeEMSIMailer{addr: ln.Addr().String(), overTelnet: overTelnet, banner: banner, done: make(chan struct{})}
	go func() {
		defer close(m.done)
		defer ln.Close()
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		defer raw.Close()
		_ = raw.SetDeadline(time.Now().Add(20 * time.Second))
		var conn net.Conn = raw
		if overTelnet {
			// Open with the option negotiation an mbcico "-t itn" performs.
			_, _ = raw.Write([]byte{tnIAC, tnWILL, tnOptBinary, tnIAC, tnDO, tnOptBinary, tnIAC, tnWILL, tnOptSGA})
			conn = newTelnetBinaryConn(raw)
		}
		if len(m.banner) > 0 {
			_, _ = conn.Write(m.banner)
		}
		cfg := emsi.DefaultConfig()
		cfg.InitialStrategy = "wait"
		cfg.Protocols = []string{"ZAP", "ZMO"}
		cfg.MailerName = "FakeMailer"
		cfg.MailerVersion = "1.0"
		sess := emsi.NewSessionWithInfoAndConfig(conn, "1:320/219", "Phoenix", "A Sysop", "Somewhere", cfg)
		if m.hsErr = sess.HandshakeAnswerer(); m.hsErr != nil {
			return
		}
		m.remote = sess.GetRemoteInfo()
		drv, ok := xfer.ByName(sess.GetSelectedProtocol())
		if !ok {
			m.xferErr = io.ErrUnexpectedEOF
			return
		}
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		_, m.xferErr = drv.Exchange(context.Background(), sess.HandoffConn(), xfer.RoleAnswerer, nothingToSend{}, declineEverything{}, log)
	}()
	return m
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}
	return host, port
}

// The ITN test is a mail session: EMSI through the telnet option layer, and
// an empty transfer phase that leaves the mailer with a completed session.
func TestTelnetTesterCompletesMailSessionOverTelnet(t *testing.T) {
	m := startFakeEMSIMailer(t, true)
	host, port := splitHostPort(t, m.addr)

	tester := NewTelnetTesterWithInfo(10*time.Second, "2:5001/5001", "NodelistDB", "Tester", "London")
	res, ok := tester.Test(context.Background(), host, port, "1:320/219").(*TelnetTestResult)
	if !ok {
		t.Fatal("not a TelnetTestResult")
	}
	<-m.done

	if !res.Success {
		t.Fatalf("test failed: %s", res.Error)
	}
	if res.MailerInfo != "FakeMailer 1.0" {
		t.Errorf("mailer = %q, want the answerer's identity", res.MailerInfo)
	}
	if !res.AddressValid {
		t.Errorf("address not validated; announced %v", res.Addresses)
	}
	if m.hsErr != nil {
		t.Errorf("mailer's handshake failed: %v", m.hsErr)
	}
	if m.xferErr != nil {
		t.Errorf("mailer saw the session end badly: %v", m.xferErr)
	}
	if m.remote == nil || m.remote.MailerName != "NodelistDB" {
		t.Errorf("mailer did not receive our EMSI_DAT: %+v", m.remote)
	}
}

// The same session on a raw socket is what the IFCICO tester runs; the
// mailer's view of the ending is the point of the assertion.
func TestIfcicoTesterLeavesMailerWithCompletedSession(t *testing.T) {
	m := startFakeEMSIMailer(t, false)
	host, port := splitHostPort(t, m.addr)

	tester := NewIfcicoTesterWithInfo(10*time.Second, "2:5001/5001", "NodelistDB", "Tester", "London")
	res, ok := tester.Test(context.Background(), host, port, "1:320/219").(*IfcicoTestResult)
	if !ok {
		t.Fatal("not an IfcicoTestResult")
	}
	<-m.done

	if !res.Success {
		t.Fatalf("test failed: %s", res.Error)
	}
	if m.xferErr != nil {
		t.Errorf("mailer saw the session end badly: %v", m.xferErr)
	}
}

// A BBS login prompt on the ITN port is reachable, but it is not a mailer,
// and the row has to say which.
func TestTelnetTesterRejectsBBSLoginPrompt(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte{tnIAC, tnWILL, 1, tnIAC, tnDO, 3})
		_, _ = conn.Write([]byte("Welcome to Phoenix BBS\r\nEnter your name: "))
		_, _ = io.Copy(io.Discard, conn) // swallow the EMSI_INQ until the tester gives up
	}()
	host, port := splitHostPort(t, ln.Addr().String())

	tester := NewTelnetTesterWithInfo(3*time.Second, "2:5001/5001", "NodelistDB", "Tester", "London")
	res := tester.Test(context.Background(), host, port, "1:320/219").(*TelnetTestResult)
	if res.Success {
		t.Fatal("a login prompt was reported as a working mailer")
	}
	if !strings.Contains(res.Error, "no mailer") || !strings.Contains(res.Error, "Phoenix BBS") {
		t.Errorf("error = %q; want it to say there was no mailer and quote the banner", res.Error)
	}
	if !strings.Contains(res.Banner, "Phoenix BBS") {
		t.Errorf("banner = %q; the greeting must be kept on a failed handshake", res.Banner)
	}
}

// EMSI_HBT is a legal first word from an answerer; a mailer that greets with
// it behind a 220 banner is a mailer.
func TestTelnetTesterHeartbeatGreetingIsAMailer(t *testing.T) {
	m := startFakeEMSIMailerWithBanner(t, true, []byte("220-Phoenix mailer ready\r\n"+emsi.EMSI_HBT+"\r"))
	host, port := splitHostPort(t, m.addr)

	tester := NewTelnetTesterWithInfo(10*time.Second, "2:5001/5001", "NodelistDB", "Tester", "London")
	res := tester.Test(context.Background(), host, port, "1:320/219").(*TelnetTestResult)
	<-m.done
	if !res.Success {
		t.Fatalf("mailer greeting with EMSI_HBT was rejected: %s", res.Error)
	}
}

// A port that accepts the connection and says nothing is not a success.
func TestTelnetTesterRejectsSilentPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(io.Discard, conn)
	}()
	host, port := splitHostPort(t, ln.Addr().String())

	tester := NewTelnetTesterWithInfo(3*time.Second, "2:5001/5001", "NodelistDB", "Tester", "London")
	res := tester.Test(context.Background(), host, port, "1:320/219").(*TelnetTestResult)
	if res.Success {
		t.Fatal("a silent socket was reported as a working mailer")
	}
	if !strings.Contains(res.Error, "sent nothing") {
		t.Errorf("error = %q; want it to say the peer sent nothing", res.Error)
	}
}

// A mailer whose greeting happens to look like an FTP or SMTP banner ("220 ")
// is still a mailer when EMSI follows; the early non-mailer exit must not
// fire on the banner alone.
func TestTelnetTesterBannerThatLooksLikeFTPStillGetsHandshake(t *testing.T) {
	m := startFakeEMSIMailerWithBanner(t, true, []byte("220-Phoenix mailer ready\r\n"+emsi.EMSI_REQ+"\r"))
	host, port := splitHostPort(t, m.addr)

	tester := NewTelnetTesterWithInfo(10*time.Second, "2:5001/5001", "NodelistDB", "Tester", "London")
	res := tester.Test(context.Background(), host, port, "1:320/219").(*TelnetTestResult)
	<-m.done
	if !res.Success {
		t.Fatalf("mailer behind a 220 banner was rejected: %s", res.Error)
	}
	if res.MailerInfo != "FakeMailer 1.0" {
		t.Errorf("mailer = %q", res.MailerInfo)
	}
	if m.xferErr != nil {
		t.Errorf("mailer saw the session end badly: %v", m.xferErr)
	}
}
