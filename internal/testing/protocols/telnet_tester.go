package protocols

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/xx25/fidomail/pkg/emsi"
)

// TelnetTester tests the ITN flag: an FTN mailer reachable over telnet.
//
// ITN (FTS-5001) announces a mail session over a telnet transport, not a
// BBS login, so the test is an EMSI handshake through the telnet option
// layer, exactly what a calling mailer does on that port. This used to be a
// TCP connect that read a banner for two seconds and reported success
// whatever it saw, which the sysop of 1:320/219 spotted: his mbcico logged
// two "Session setup error" telnet sessions while the site showed ITN
// working on both address families. A port that answers with something
// other than a mailer is still reported, in the error text, so that the row
// says what was there.
type TelnetTester struct {
	timeout    time.Duration
	ourAddress string
	systemName string
	sysop      string
	location   string
	debug      bool
	configMgr  *emsi.ConfigManager
}

// telnetGreetWindow is how long a mailer gets to speak first. Answerers greet
// at once, with a banner and EMSI_REQ; the EMSI session then replays what
// was read and answers it. What is collected here is also the evidence for
// the error text when there turns out to be no mailer.
const telnetGreetWindow = 1500 * time.Millisecond

// NewTelnetTesterWithInfo creates a Telnet tester that announces the given
// identity in EMSI_DAT. ourAddress comes from configuration only, for the
// reason given on NewIfcicoTesterWithInfo.
func NewTelnetTesterWithInfo(timeout time.Duration, ourAddress, systemName, sysop, location string) *TelnetTester {
	return &TelnetTester{
		timeout:    timeout,
		ourAddress: ourAddress,
		systemName: systemName,
		sysop:      sysop,
		location:   location,
	}
}

// GetProtocolName returns the protocol name
func (t *TelnetTester) GetProtocolName() string {
	return "Telnet"
}

// SetDebug enables or disables debug logging
func (t *TelnetTester) SetDebug(enabled bool) { t.debug = enabled }

// SetEMSIConfigManager applies per-node EMSI handshake settings
func (t *TelnetTester) SetEMSIConfigManager(mgr *emsi.ConfigManager) { t.configMgr = mgr }

// Test performs a mail session over telnet: connect, negotiate telnet binary
// mode, complete an EMSI handshake as the caller, and close the session out
// with an empty transfer phase.
func (t *TelnetTester) Test(ctx context.Context, host string, port int, expectedAddress string) TestResult {
	startTime := time.Now()

	if port == 0 {
		port = 23
	}

	// banner is what the port printed before EMSI began. It travels with a
	// failure too: for a handshake that never completed it is the only clue
	// to what was there.
	var banner string
	fail := func(err string) TestResult {
		return &TelnetTestResult{
			BaseTestResult: BaseTestResult{
				Success:    false,
				Error:      err,
				ResponseMs: uint32(time.Since(startTime).Milliseconds()),
				TestTime:   startTime,
			},
			Banner: banner,
		}
	}

	if err := Pace(ctx); err != nil {
		return fail(fmt.Sprintf("cancelled: %v", err))
	}
	startTime = time.Now() // response time measures the session, not the pacing wait

	dialer := net.Dialer{Timeout: t.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return fail(fmt.Sprintf("connection failed: %v", err))
	}
	defer conn.Close()

	// The telnet layer: option negotiation is answered (binary + SGA accepted),
	// IAC sequences are stripped, and 0xFF is doubled on the way out, which a
	// ZMODEM stream needs.
	tn := newTelnetBinaryConn(conn)
	greeting := readMore(tn, 2048, telnetGreetWindow)
	banner = cleanBanner(string(greeting))

	if t.ourAddress == "" {
		// No identity to announce, so no handshake: report what was seen.
		if len(greeting) == 0 {
			return fail("connected but peer sent nothing, and no EMSI identity configured")
		}
		return fail(fmt.Sprintf("no EMSI identity configured; peer greeted with: %s", banner))
	}

	if t.debug {
		logging.Debugf("Telnet: %s:%d greeted with %d byte(s) (telnet=%v): %q", host, port, len(greeting), tn.sawIAC, banner)
	}

	// A port that has already introduced itself as ssh, http, smtp or ftp is
	// not going to answer EMSI; say so now rather than after the handshake
	// budget has run out. Unless something in the same greeting says mailer
	// — an EMSI_REQ, or a known mailer's banner — in which case the banner
	// text is just what that mailer prints first.
	text := string(greeting)
	if !containsEMSIReply(text) && sniffSoftware(text) == "" {
		if name, b := identifyBanner(text); name != "" {
			return fail(fmt.Sprintf("no mailer: %s on the ITN port (%s)", name, b))
		}
	}

	var cfg *emsi.Config
	if t.configMgr != nil {
		cfg = t.configMgr.GetConfigForNode(expectedAddress)
	} else {
		cfg = emsi.DefaultConfig()
	}
	cfg.MailerName = "NodelistDB"
	cfg.MailerVersion = mailerVersion
	if cfg.InitialStrategy == "" || cfg.InitialStrategy == "wait" {
		cfg.InitialStrategy = emsiCallerStrategy
	}

	// Whatever the mailer said while we were listening (banner, EMSI_REQ) is
	// replayed to the handshake, which answers a REQ with INQ as the spec's
	// calling side does.
	session := emsi.NewSessionWithInfoAndConfig(newPrefixConn(tn, greeting), t.ourAddress, t.systemName, t.sysop, t.location, cfg)
	if t.configMgr == nil {
		session.SetTimeout(t.timeout)
	}
	session.SetDebug(t.debug)

	if err := session.HandshakeCaller(); err != nil {
		return fail(describeNonMailer(greeting, tn.sawIAC, err))
	}

	remote := session.GetRemoteInfo()
	result := &TelnetTestResult{
		BaseTestResult: BaseTestResult{
			Success:    true,
			ResponseMs: uint32(time.Since(startTime).Milliseconds()),
			TestTime:   startTime,
		},
		Banner: banner,
	}
	if remote != nil {
		result.SystemName = remote.SystemName
		result.MailerInfo = remoteMailer(remote)
		result.Addresses = remote.Addresses
		if expectedAddress != "" {
			result.AddressValid = announcedAddressMatches(remote.Addresses, expectedAddress)
		}
	}

	if err := finishEMSISession(ctx, session, expectedAddress); err != nil {
		logging.Debugf("Telnet: %s session with %s:%d ended untidily: %v", expectedAddress, host, port, err)
	}

	if t.debug {
		logging.Debugf("Telnet: %s:%d -> mailer=%q system=%q addresses=%v valid=%v",
			host, port, result.MailerInfo, result.SystemName, result.Addresses, result.AddressValid)
	}
	return result
}

// describeNonMailer explains a failed handshake in terms of what the port
// actually answered with, so a BBS login, an ssh daemon or a silent socket
// each read differently in the stored error.
func describeNonMailer(greeting []byte, sawTelnet bool, handshakeErr error) string {
	text := string(greeting)
	if len(greeting) == 0 {
		return fmt.Sprintf("no mailer: peer sent nothing and did not answer EMSI (%v)", handshakeErr)
	}
	if containsEMSIReply(text) || sniffSoftware(text) != "" {
		return fmt.Sprintf("mailer answered but the EMSI handshake failed: %v", handshakeErr)
	}
	if name, banner := identifyBanner(text); name != "" {
		return fmt.Sprintf("no mailer: %s on the ITN port (%s)", name, banner)
	}
	kind := "unrecognized"
	if sawTelnet {
		kind = "telnet login"
	}
	return fmt.Sprintf("no mailer: %s on the ITN port (%s)", kind, cleanBanner(text))
}
