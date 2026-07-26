package protocols

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/xx25/fidomail/pkg/emsi"
	"github.com/xx25/fidomail/pkg/vmp"
)

// VModemTester probes a node's announced IVM ("Internet VMODEM") port and
// identifies what protocol actually runs there. The IVM flag announces Ray
// Gwinn's binary Virtual Modem Protocol (VMP), but most IVM ports in practice
// run an EMSI mailer session over telnet-binary or raw TCP, and some run other
// things entirely.
//
// Against a genuine VMP responder the tester places a real outgoing call: it
// completes the VMP handshake, lets the remote ring its mailer, and runs an
// EMSI handshake over the resulting data channel to read back the remote's
// address, system name and sysop — the same identity the other protocol testers
// collect. Everything else on an IVM port is classified without placing a call.
//
// The protocol itself lives in fidomail/pkg/vmp, shared with the mailer that
// speaks it in both directions; what stays here is the classification — what
// is actually running on a port the nodelist flagged IVM.
type VModemTester struct {
	timeout    time.Duration
	ourAddress string
	systemName string
	sysop      string
	location   string
	debug      bool
	configMgr  *emsi.ConfigManager
	vmp        vmp.DialConfig
	vmpEnabled bool
	vmpSlot    chan struct{} // non-nil when calls must be serialized on one port
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

// EnableVMPCalls turns on real outgoing VMP calls and configures the listener
// the answering node dials back to. A VMP session needs that reverse
// connection, so calls only work from a host reachable inbound on the port
// range given (leave the range at 0/0 to use an ephemeral port).
func (t *VModemTester) EnableVMPCalls(listenHost string, preferredPort, portMin, portMax int, ringTimeout time.Duration) {
	t.vmpEnabled = true
	t.vmp = vmp.DialConfig{
		ConnectTimeout: t.timeout,
		RingTimeout:    ringTimeout,
		ListenHost:     listenHost,
		PreferredPort:  preferredPort,
		PortMin:        portMin,
		PortMax:        portMax,
		Logger:         vmpDebugLogger(t.debug),
	}
	// One port means one call at a time. Serialize rather than let concurrent
	// workers race for the bind, since the loser cannot fall back to another
	// port without asking a node to call back somewhere unreachable.
	if t.vmp.SinglePort() {
		t.vmpSlot = make(chan struct{}, 1)
	} else {
		t.vmpSlot = nil
	}
}

func (t *VModemTester) classifyTimeout() time.Duration {
	// At least as generous as the EMSI first-step timeout so a slow-but-valid
	// telnet/EMSI mailer isn't misclassified as silent before it greets.
	if d := emsi.DefaultConfig().FirstStepTimeout; d > 0 {
		return d
	}
	return 8 * time.Second
}

// Test probes the IVM port and returns a *VModemTestResult. Against a genuine
// VMODEM this places a real call, which rings the remote sysop's mailer.
func (t *VModemTester) Test(ctx context.Context, host string, port int, expectedAddress string) TestResult {
	return t.test(ctx, host, port, expectedAddress, true)
}

// TestWithoutCalling probes the IVM port but never places a VMP call. Use it
// for the second and later addresses of a node whose mailer has already been
// rung this cycle: it still records reachability per address family without
// ringing the same sysop again.
func (t *VModemTester) TestWithoutCalling(ctx context.Context, host string, port int, expectedAddress string) TestResult {
	return t.test(ctx, host, port, expectedAddress, false)
}

func (t *VModemTester) test(ctx context.Context, host string, port int, expectedAddress string, mayCall bool) TestResult {
	start := time.Now()
	if port == 0 {
		port = vmp.DefaultPort
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

	// A genuine VMODEM greets nobody — it blocks reading the caller's opening
	// frame — so silence is the cue to place a real VMP call on this same
	// connection, which is exactly what a real VMODEM client finds when it
	// dials. Anything that speaks first is running some other protocol on the
	// IVM port and gets classified without troubling a mailer.
	//
	// The wait before calling is short: mailers that greet do so at once, and
	// the slower ones are still given the full classification window on the
	// fall-back connection below.
	var first []byte
	var silentToVMP bool
	if t.vmpEnabled && mayCall {
		_ = conn.SetReadDeadline(time.Now().Add(vmpGreetWindow))
		first = readSome(conn, 512)
		if len(first) == 0 {
			var isVMODEM bool
			isVMODEM, silentToVMP = t.placeVMPCall(ctx, conn, expectedAddress, res)
			if isVMODEM {
				res.ResponseMs = uint32(time.Since(start).Milliseconds())
				if t.debug {
					logging.Debugf("VModem %s:%d -> variant=%s conformant=%v software=%q detail=%q",
						host, port, res.Variant, res.Conformant, res.Software, res.Detail)
				}
				return res
			}
			// Not VMODEM after all. Classify on a fresh connection, since this
			// one has our handshake bytes buffered, and let sniff spend the full
			// classification window on a peer that may just be slow to greet.
			conn, err = dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
			if err != nil {
				res.Success = false
				res.Variant = "down"
				res.Error = fmt.Sprintf("connection failed: %v", err)
				res.ResponseMs = uint32(time.Since(start).Milliseconds())
				return res
			}
		}
	}

	app, sawTelnet, nudged := t.sniff(conn, first)
	_ = conn.Close() // classification done; EMSI identity uses a fresh connection

	res.ResponseMs = uint32(time.Since(start).Milliseconds())
	t.classify(ctx, host, port, expectedAddress, app, sawTelnet, nudged, res)
	if silentToVMP && res.Variant == "unknown" {
		// The peer took a VMP hello and a connect request, then a fresh
		// connection and an EMSI nudge, and never sent a byte to any of it.
		// Report that as what it is rather than as an unrecognized protocol:
		// the other reading is a VMODEM stuck dialling a data-channel port it
		// cannot reach, which is a fault on our side of the call, not theirs.
		res.CallOutcome = "no-reply"
		res.Detail = "no reply to a VMP hello or connect request: either not a VMODEM, " +
			"or a VMODEM whose callback to our data-channel port never got through"
	}
	if t.debug {
		logging.Debugf("VModem %s:%d -> variant=%s conformant=%v software=%q", host, port, res.Variant, res.Conformant, res.Software)
	}
	return res
}

// vmpGreetWindow is how long a peer gets to greet us before we treat its
// silence as "this is a VMODEM waiting for an opening frame" and call it.
const vmpGreetWindow = 2 * time.Second

// emsiCallerStrategy makes the EMSI session announce itself with EMSI_INQ at
// connect instead of waiting to be spoken to.
//
// This is only the preamble; the calling role itself is pinned by
// HandshakeCaller (see emsiOver), which is what makes a spec-strict answerer
// respond at all. The announce is kept because it is what our field results
// were measured with — it saves a REQ round-trip against mailers that greet
// first, and it is what gained identity from 2:371/52 (Xenia/2 Mailer) and
// 2:420/33 (FrontDoor) where waiting to be spoken to did not.
const emsiCallerStrategy = "send_inq"

// emsiINQNudge is the sequence sniff sends to coax a greeting out of a silent
// peer. It must be the spec form — FSC-0056 spells EMSI_INQ with its fixed
// C816 CRC suffix, and a mailer's reader matches the whole 14-character token,
// so a bare "**EMSI_INQ" is not an INQ to anything and gets no answer. A
// genuine VMODEM rejects either form the same way (its opening-frame check
// fails on the first ten bytes and it hangs up with DISCONNECT reason 8),
// which is exactly the frame that classifies it.
var emsiINQNudge = []byte(emsi.EMSI_INQ + "\r")

// placeVMPCall runs the real Virtual Modem Protocol over an already-open
// connection and, if the remote mailer answers, runs an EMSI handshake over the
// data channel to read back the remote's identity.
//
// It reports whether the peer proved itself a VMODEM; when it did not, res is
// left untouched and conn has been closed, so the caller must reconnect to
// classify. The second return says the peer answered our frames with complete
// silence, which the caller uses to describe a peer nothing else identifies
// either — see vmp.Call.SilentPeer.
func (t *VModemTester) placeVMPCall(ctx context.Context, conn net.Conn, expectedAddress string, res *VModemTestResult) (isVMODEM, silent bool) {
	cfg := t.vmp
	cfg.Logger = vmpDebugLogger(t.debug)
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = t.timeout
	}
	peer := conn.RemoteAddr().String()

	// Wait our turn when there is only one data-channel port to go around.
	// Registered before the call's own cleanup so the slot is released last.
	if t.vmpSlot != nil {
		select {
		case t.vmpSlot <- struct{}{}:
			defer func() { <-t.vmpSlot }()
		case <-ctx.Done():
			return false, false
		}
	}

	call, err := vmp.DialOn(ctx, conn, cfg)
	if err != nil {
		if t.debug {
			logging.Debugf("VModem %s VMP call failed: %v", peer, err)
		}
		if errors.Is(err, vmp.ErrNoLocalPort) {
			// Our own listener, not the node: say so instead of letting a
			// healthy VMODEM be reported as an unrecognized peer.
			res.Success = false
			res.Variant = "vmp"
			res.Error = err.Error()
			res.Detail = "cannot place VMP calls: " + err.Error()
			res.CallOutcome = "no-local-port"
			return true, false
		}
		return false, false
	}
	defer call.Close()

	if !call.IsVMODEM() {
		return false, call.SilentPeer()
	}

	res.Variant = "vmp"
	res.Conformant = true
	res.Software = "VMODEM (Gwinn VMP)"
	res.Detail = call.Describe()
	res.CallOutcome = call.OutcomeToken()

	if call.Outcome != vmp.OutcomeConnected {
		// The responder is genuine even when no mailer picked up, so the node
		// is reachable and correctly flagged; there is just no identity to read.
		res.Success = true
		return true, false
	}

	res.Success = true
	res.Detail = "VMP call established, mailer answered"
	if why := t.emsiOver(call.Data, expectedAddress, res); why != "" {
		res.Detail += ": " + why
	}
	return true, false
}

// sniff reads the peer's opening bytes (stripping telnet if present), nudging
// once with an EMSI_INQ if the peer stays silent. `first` carries any opening
// bytes the caller already read, so the peer isn't waited on twice. It returns
// the decoded application bytes, whether telnet was seen, and whether we sent
// the nudge.
func (t *VModemTester) sniff(conn net.Conn, first []byte) (app []byte, sawTelnet, nudged bool) {
	deadline := t.classifyTimeout()

	if len(first) == 0 {
		_ = conn.SetReadDeadline(time.Now().Add(deadline))
		first = readSome(conn, 512)
	}
	if len(first) == 0 {
		// Silent peer: could be VMP (waits for input) or a quiet EMSI answerer.
		_, _ = conn.Write(emsiINQNudge)
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
			_, _ = tn.Write(emsiINQNudge)
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
	if ok, cmd := vmp.LooksLike(app); ok {
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
	_ = t.emsiOver(c, expectedAddress, res)
}

// emsiOver runs an EMSI handshake as the calling side over an already-open
// transport and copies whatever the remote announced into res. It returns a
// short note on the outcome, empty only when a full EMSI identity was read
// cleanly. Everything else names the fault, because those faults are not the
// same thing: a handshake that never completed, one that completed while the
// peer supplied nothing usable, and one that produced only a product name
// guessed from the mailer's banner are three different states of the far side.
func (t *VModemTester) emsiOver(c net.Conn, expectedAddress string, res *VModemTestResult) string {
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

	session := emsi.NewSessionWithInfoAndConfig(c, t.ourAddress, t.systemName, t.sysop, t.location, cfg)
	if t.configMgr == nil {
		session.SetTimeout(t.timeout)
	}
	session.SetDebug(t.debug)

	// We dialed, so pin the calling role: the peer's EMSI_REQ is answered with
	// EMSI_INQ before our EMSI_DAT, which is what a strict answerer waits for.
	err := session.HandshakeCaller()
	defer session.Close()

	info := session.GetRemoteInfo()
	reason := session.GetCompletionReason()

	// What the identity is worth is decided by where it came from, not by the
	// handshake error. Two asymmetries make that distinction necessary:
	//
	//   - An error does not mean nothing was learned. On the INQ-first path the
	//     remote's DAT is read and parsed before ours goes out, so a TX-phase
	//     failure afterwards still leaves a genuine identity behind.
	//   - No error does not mean an EMSI identity. When the RX phase runs out
	//     of retries the library falls back to reading the product name out of
	//     the mailer's banner and returns success, marking that guess with a
	//     placeholder system name. Software so identified is weaker evidence
	//     and the placeholder is never a system name.
	bannerOnly := info != nil && info.SystemName == emsiBannerPlaceholder

	if mailer := remoteMailer(info); mailer != "" {
		res.Software = mailer
	}
	if info != nil && !bannerOnly {
		res.SystemName = info.SystemName
		res.Sysop = info.Sysop
		res.Location = info.Location
		res.Addresses = info.Addresses
		if info.Location != "" && res.SystemName != "" && !strings.Contains(res.SystemName, info.Location) {
			res.SystemName = fmt.Sprintf("%s (%s)", res.SystemName, info.Location)
		}
		if expectedAddress != "" {
			res.AddressValid = session.ValidateAddress(expectedAddress)
		}
	}

	var note string
	switch {
	case info != nil && !bannerOnly:
		if err != nil {
			note = fmt.Sprintf("identity read, handshake then failed (%s)", reason)
		}
	case bannerOnly:
		note = fmt.Sprintf("no EMSI identity, software read from the mailer's banner (%s)", reason)
	case err != nil:
		note = fmt.Sprintf("handshake failed (%s)", reason)
	default:
		// Completed, but the peer supplied nothing usable — e.g. its EMSI_DAT
		// failed to parse after we ACKed it.
		note = "handshake completed, no identity sent"
	}
	if note != "" && t.debug {
		if err != nil {
			logging.Debugf("VModem EMSI: %s: %v", note, err)
		} else {
			logging.Debugf("VModem EMSI: %s", note)
		}
	}
	return note
}

// emsiBannerPlaceholder is the system name pkg/emsi substitutes when it gave up
// on the handshake and read the product name out of the mailer's banner
// instead. It marks a guess, not an announced identity.
const emsiBannerPlaceholder = "[Extracted from banner]"

// remoteMailer renders the product name a remote announced, if any.
func remoteMailer(info *emsi.EMSIData) string {
	if info == nil {
		return ""
	}
	return strings.TrimSpace(info.MailerName + " " + info.MailerVersion)
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
