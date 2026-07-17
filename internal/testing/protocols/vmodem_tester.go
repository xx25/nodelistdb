package protocols

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/xx25/fidomail/pkg/emsi"
)

// VModemTester probes a node's announced IVM ("Internet VMODEM") port and
// identifies what protocol actually runs there. The IVM flag announces Ray
// Gwinn's binary Virtual Modem Protocol (VMP), but most IVM ports in practice
// run an EMSI mailer session over telnet-binary or raw TCP, and some run other
// things entirely. The tester confirms genuine VMP where present and otherwise
// reports the actual protocol and software.
type VModemTester struct {
	timeout    time.Duration
	ourAddress string
	systemName string
	sysop      string
	location   string
	debug      bool
	configMgr  *emsi.ConfigManager
}

// NewVModemTester creates a VModem tester with neutral defaults.
func NewVModemTester(timeout time.Duration) *VModemTester {
	return NewVModemTesterWithInfo(timeout, "", "", "", "")
}

// NewVModemTesterWithInfo creates a VModem tester that advertises the given
// identity when it falls through to an EMSI handshake.
func NewVModemTesterWithInfo(timeout time.Duration, ourAddress, systemName, sysop, location string) *VModemTester {
	if ourAddress == "" {
		ourAddress = "2:5001/5001"
	}
	return &VModemTester{
		timeout:    timeout,
		ourAddress: ourAddress,
		systemName: systemName,
		sysop:      sysop,
		location:   location,
	}
}

// GetProtocolName returns the protocol name.
func (t *VModemTester) GetProtocolName() string { return "VModem" }

// SetDebug implements DebugSetter.
func (t *VModemTester) SetDebug(enabled bool) { t.debug = enabled }

// SetEMSIConfigManager implements EMSIConfigSetter.
func (t *VModemTester) SetEMSIConfigManager(mgr *emsi.ConfigManager) { t.configMgr = mgr }

func (t *VModemTester) classifyTimeout() time.Duration {
	// At least as generous as the EMSI first-step timeout so a slow-but-valid
	// telnet/EMSI mailer isn't misclassified as silent before it greets.
	if d := emsi.DefaultConfig().FirstStepTimeout; d > 0 {
		return d
	}
	return 8 * time.Second
}

// Test probes the IVM port and returns a *VModemTestResult.
func (t *VModemTester) Test(ctx context.Context, host string, port int, expectedAddress string) TestResult {
	start := time.Now()
	if port == 0 {
		port = 3141
	}
	res := &VModemTestResult{
		BaseTestResult: BaseTestResult{TestTime: start},
	}

	dialer := net.Dialer{Timeout: t.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		res.Success = false
		res.Variant = "down"
		res.Error = fmt.Sprintf("connection failed: %v", err)
		res.ResponseMs = uint32(time.Since(start).Milliseconds())
		return res
	}

	app, sawTelnet, nudged := t.sniff(conn)
	_ = conn.Close() // classification done; EMSI identity uses a fresh connection

	res.ResponseMs = uint32(time.Since(start).Milliseconds())
	t.classify(ctx, host, port, expectedAddress, app, sawTelnet, nudged, res)
	if t.debug {
		logging.Debugf("VModem %s:%d -> variant=%s conformant=%v software=%q", host, port, res.Variant, res.Conformant, res.Software)
	}
	return res
}

// sniff reads the peer's opening bytes (stripping telnet if present), nudging
// once with an EMSI_INQ if the peer stays silent. It returns the decoded
// application bytes, whether telnet was seen, and whether we sent the nudge.
func (t *VModemTester) sniff(conn net.Conn) (app []byte, sawTelnet, nudged bool) {
	deadline := t.classifyTimeout()

	_ = conn.SetReadDeadline(time.Now().Add(deadline))
	first := readSome(conn, 512)
	if len(first) == 0 {
		// Silent peer: could be VMP (waits for input) or a quiet EMSI answerer.
		_, _ = conn.Write([]byte("**EMSI_INQ\r"))
		nudged = true
		_ = conn.SetReadDeadline(time.Now().Add(deadline))
		first = readSome(conn, 512)
		if len(first) == 0 {
			return nil, false, nudged
		}
	}

	if first[0] == tnIAC {
		// Telnet layer: run the opening bytes + a little more through the shim.
		tn := newTelnetBinaryConn(newPrefixConn(conn, first))
		app = readMore(tn, 2048, 2500*time.Millisecond)
		// Many telnet-binary EMSI mailers stay quiet after negotiation until the
		// caller speaks; nudge once like a calling mailer and read again.
		if !containsEMSIReply(string(app)) {
			_, _ = tn.Write([]byte("**EMSI_INQ\r"))
			nudged = true
			app = append(app, readMore(tn, 2048, 3500*time.Millisecond)...)
		}
		return app, true, nudged
	}

	app = first
	app = append(app, readMore(conn, 2048-len(app), 1500*time.Millisecond)...)
	return app, false, nudged
}

// classify inspects the sniffed bytes and fills in the result.
func (t *VModemTester) classify(ctx context.Context, host string, port int, expectedAddress string, app []byte, sawTelnet, nudged bool, res *VModemTestResult) {
	text := string(app)

	// 1. Genuine VMODEM (VMP) — the conformant case.
	if ok, cmd := looksLikeVMP(app); ok {
		res.Success = true
		res.Variant = "vmp"
		res.Conformant = true
		res.Software = "VMODEM (Gwinn VMP)"
		res.Detail = fmt.Sprintf("genuine VMODEM/VMP responder (frame command %d)", cmd)
		return
	}

	// 2. EMSI mailer (over telnet-binary or raw). EMSI_INQ is what we send, so
	// don't count it as a peer marker unless we never nudged.
	if hasEMSIMarker(text, nudged) {
		res.Success = true
		if sawTelnet {
			res.Variant = "emsi-telnet"
		} else {
			res.Variant = "emsi-raw"
		}
		t.emsiIdentity(ctx, host, port, sawTelnet, expectedAddress, res)
		if res.Software == "" {
			// Handshake didn't complete; recover the product name from the
			// banner the mailer printed ahead of its EMSI reply.
			res.Software = sniffSoftware(text)
		}
		res.Detail = describeMismatch(res)
		return
	}

	// 3. binkp (some IVM ports actually run binkd).
	if sw, sys, ok := parseBinkpGreeting(app); ok {
		res.Success = true
		res.Variant = "binkp"
		res.Software = sw
		res.SystemName = sys
		res.Detail = describeMismatch(res)
		return
	}

	// 4. A telnet endpoint whose banner names a known mailer is an EMSI mailer,
	// even when the EMSI reply itself slipped past the sniff window (some
	// mailers, e.g. FrontDoor, are slow and finicky about the opening exchange).
	if sawTelnet {
		if sw := sniffSoftware(text); sw != "" {
			res.Success = true
			res.Variant = "emsi-telnet"
			res.Software = sw
			t.emsiIdentity(ctx, host, port, true, expectedAddress, res)
			if res.Software == "" {
				res.Software = sw
			}
			res.Detail = describeMismatch(res)
			return
		}
	}

	// 5. Named text banners (SSH/HTTP/SMTP/FTP/human telnet login).
	if name, banner := identifyBanner(text); name != "" {
		res.Success = true
		res.Variant = name
		res.Banner = banner
		res.Detail = describeMismatch(res)
		return
	}
	if sawTelnet {
		res.Success = true
		res.Variant = "telnet-login"
		res.Banner = cleanBanner(text)
		res.Detail = describeMismatch(res)
		return
	}

	// 6. Reachable but nothing recognizable.
	res.Success = false
	res.Variant = "unknown"
	if len(app) > 0 {
		res.Banner = cleanBanner(text)
		res.Detail = "connected but protocol not recognized"
	} else {
		res.Detail = "connected but peer sent nothing"
	}
}

// emsiIdentity performs a full EMSI handshake on a fresh connection to extract
// the remote's system name, software and addresses. Failure leaves the variant
// intact (the protocol was already recognized during sniff) with identity blank.
func (t *VModemTester) emsiIdentity(ctx context.Context, host string, port int, sawTelnet bool, expectedAddress string, res *VModemTestResult) {
	dialer := net.Dialer{Timeout: t.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return
	}
	defer conn.Close()

	var c net.Conn = conn
	if sawTelnet {
		c = newTelnetBinaryConn(conn)
	}

	var cfg *emsi.Config
	if t.configMgr != nil {
		cfg = t.configMgr.GetConfigForNode(expectedAddress)
	} else {
		cfg = emsi.DefaultConfig()
	}
	cfg.MailerName = "NodelistDB"
	cfg.MailerVersion = mailerVersion

	session := emsi.NewSessionWithInfoAndConfig(c, t.ourAddress, t.systemName, t.sysop, t.location, cfg)
	if t.configMgr == nil {
		session.SetTimeout(t.timeout)
	}
	session.SetDebug(t.debug)

	if err := session.Handshake(); err != nil {
		if t.debug {
			logging.Debugf("VModem EMSI handshake %s:%d failed: %v", host, port, err)
		}
		return
	}
	defer session.Close()

	if info := session.GetRemoteInfo(); info != nil {
		res.SystemName = info.SystemName
		res.Addresses = info.Addresses
		mailer := strings.TrimSpace(info.MailerName + " " + info.MailerVersion)
		if mailer != "" {
			res.Software = mailer
		}
		if info.Location != "" && res.SystemName != "" && !strings.Contains(res.SystemName, info.Location) {
			res.SystemName = fmt.Sprintf("%s (%s)", res.SystemName, info.Location)
		}
		if expectedAddress != "" {
			res.AddressValid = session.ValidateAddress(expectedAddress)
		}
	}
}

// describeMismatch produces the human note reported for a non-VMP IVM port.
func describeMismatch(res *VModemTestResult) string {
	if res.Conformant {
		return ""
	}
	msg := fmt.Sprintf("IVM announced, actual: %s", res.Variant)
	if res.Software != "" {
		msg += fmt.Sprintf(" (%s)", res.Software)
	}
	return msg
}

// hasEMSIMarker reports whether the text contains a peer-originated EMSI marker.
func hasEMSIMarker(text string, nudged bool) bool {
	for _, m := range []string{"EMSI_REQ", "EMSI_DAT", "EMSI_ACK", "EMSI_NAK", "EMSI_HBT"} {
		if strings.Contains(text, m) {
			return true
		}
	}
	// EMSI_INQ is what we send when nudging; only trust it as a peer signal
	// when we never nudged.
	return !nudged && strings.Contains(text, "EMSI_INQ")
}

// parseBinkpGreeting extracts software (VER) and system name (SYS) from a binkp
// M_NUL greeting frame, if the bytes look like binkp.
func parseBinkpGreeting(b []byte) (software, systemName string, ok bool) {
	i := 0
	for i+2 <= len(b) {
		hdr := int(b[i])<<8 | int(b[i+1])
		i += 2
		cmd := hdr&0x8000 != 0
		ln := hdr & 0x7fff
		if ln == 0 || i+ln > len(b) {
			break
		}
		body := b[i : i+ln]
		i += ln
		if !cmd || len(body) < 1 || body[0] != 0 { // M_NUL == 0
			continue
		}
		line := string(body[1:])
		if strings.HasPrefix(line, "VER ") {
			software = strings.TrimSpace(line[4:])
			ok = true
		} else if strings.HasPrefix(line, "SYS ") {
			systemName = strings.TrimSpace(line[4:])
			ok = true
		} else if strings.HasPrefix(line, "OPT ") || strings.HasPrefix(line, "TIME ") || strings.HasPrefix(line, "ZYZ ") {
			ok = true
		}
	}
	return software, systemName, ok
}

// mailerBannerRE matches product names FidoNet mailers commonly print in the
// banner ahead of their EMSI reply, used when a full handshake didn't complete.
var mailerBannerRE = regexp.MustCompile(`(?i)(FrontDoor(?:/\d)?[\w/. ]*?\d[\w/.]*|Platinum Xpress[\w/. ]*?[\d][\w/.\-]*|WINServer[\w/. ]*|BinkleyTerm[\w/. ]*|Mystic[\w/. ]*|Argus[\w/. ]*|Radius[\w/. ]*|T-Mail[\w/. ]*|Taurus[\w/. ]*|McMail[\w/. ]*|Internet Rex[\w/. ]*|Synchronet[\w/. ]*|ifcico[\w/.\-]*|qico[\w/.\-]*)`)

// sniffSoftware extracts a mailer product name from a banner, best-effort.
func sniffSoftware(text string) string {
	if m := mailerBannerRE.FindString(text); m != "" {
		return strings.TrimSpace(m)
	}
	return ""
}

// identifyBanner names a well-known text banner protocol, if recognizable.
func identifyBanner(text string) (name, banner string) {
	trimmed := strings.TrimSpace(text)
	switch {
	case strings.HasPrefix(trimmed, "SSH-"):
		return "ssh", cleanBanner(trimmed)
	case strings.HasPrefix(trimmed, "HTTP/") || strings.HasPrefix(trimmed, "GET ") || strings.Contains(trimmed, "<html"):
		return "http", cleanBanner(trimmed)
	case strings.HasPrefix(trimmed, "220 ") && strings.Contains(strings.ToUpper(trimmed), "ESMTP"):
		return "smtp", cleanBanner(trimmed)
	case strings.HasPrefix(trimmed, "220 ") || strings.HasPrefix(trimmed, "220-"):
		return "ftp", cleanBanner(trimmed)
	}
	return "", ""
}

// cleanBanner strips control bytes and truncates a banner for storage.
func cleanBanner(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || (r >= 32 && r < 127) {
			b.WriteRune(r)
		} else if r == '\r' {
			continue
		} else {
			b.WriteRune(' ')
		}
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 300 {
		out = out[:300] + "..."
	}
	return out
}

// containsEMSIReply reports whether text holds an answerer-originated EMSI reply
// (i.e. enough to decide it's a mailer). EMSI_INQ is excluded — that's what we
// send.
func containsEMSIReply(text string) bool {
	return strings.Contains(text, "EMSI_REQ") || strings.Contains(text, "EMSI_DAT") ||
		strings.Contains(text, "EMSI_ACK") || strings.Contains(text, "EMSI_NAK") ||
		strings.Contains(text, "EMSI_MD5")
}

// readChunk does one bounded read. done is true on EOF/hard error (not timeout).
func readChunk(conn net.Conn, max int) (data []byte, done bool) {
	buf := make([]byte, max)
	n, err := conn.Read(buf)
	if n > 0 {
		data = buf[:n]
	}
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return data, false
		}
		return data, true
	}
	return data, false
}

// readSome does one bounded read, ignoring the error.
func readSome(conn net.Conn, max int) []byte {
	data, _ := readChunk(conn, max)
	return data
}

// readMore accumulates bytes up to max within window, stopping early once an
// EMSI reply marker is present or the peer closes. It keeps waiting through
// individual read timeouts so a slow-to-greet mailer isn't cut off.
func readMore(conn net.Conn, max int, window time.Duration) []byte {
	var out []byte
	end := time.Now().Add(window)
	for len(out) < max {
		remaining := time.Until(end)
		if remaining <= 0 {
			break
		}
		rd := 400 * time.Millisecond
		if remaining < rd {
			rd = remaining
		}
		_ = conn.SetReadDeadline(time.Now().Add(rd))
		chunk, done := readChunk(conn, max-len(out))
		if len(chunk) > 0 {
			out = append(out, chunk...)
			if containsEMSIReply(string(out)) {
				break
			}
		}
		if done {
			break
		}
	}
	return out
}
