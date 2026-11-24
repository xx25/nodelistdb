//go:build linux

// Package main tests the dial loops' handling of a voice answer against a
// scripted modem on a pseudo-terminal: the real pkg/modem talks to it, so the
// retry decisions are exercised exactly as they run in production. Nothing
// here can reach a telephone line.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/xx25/fidomail/pkg/modem"
)

// scriptedModem answers AT commands on the master side of a pty.
type scriptedModem struct {
	master *os.File

	mu         sync.Mutex
	dialResult string   // result code for every ATD
	report     string   // body of the reply to reportCommand
	dials      []string // dial strings received
	reports    int      // report commands received
}

const scriptedReportCommand = "ATI11"

// newScriptedModem opens a pty, starts the scripted modem on its master side
// and returns it with the slave's device path.
func newScriptedModem(t *testing.T, dialResult, report string) (*scriptedModem, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping pty modem test in short mode")
	}

	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pty available: %v", err)
	}
	t.Cleanup(func() { master.Close() })

	var unlock int32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		t.Skipf("cannot unlock pty: %v", errno)
	}
	var num uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&num))); errno != 0 {
		t.Skipf("cannot name pty: %v", errno)
	}

	sm := &scriptedModem{master: master, dialResult: dialResult, report: report}
	go sm.serve()
	return sm, fmt.Sprintf("/dev/pts/%d", num)
}

// serve reads CR-terminated commands and answers them until the pty closes.
func (sm *scriptedModem) serve() {
	var line []byte
	buf := make([]byte, 256)
	for {
		n, err := sm.master.Read(buf)
		if err != nil {
			return
		}
		for _, b := range buf[:n] {
			if b != '\r' {
				if b != '\n' {
					line = append(line, b)
				}
				continue
			}
			cmd := strings.ToUpper(strings.TrimSpace(string(line)))
			line = line[:0]
			if cmd == "" {
				continue
			}
			_, _ = sm.master.Write([]byte(sm.reply(cmd)))
		}
	}
}

func (sm *scriptedModem) reply(cmd string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	switch {
	case strings.HasPrefix(cmd, "ATD"):
		sm.dials = append(sm.dials, cmd)
		return "\r\n" + sm.dialResult + "\r\n"
	case cmd == scriptedReportCommand:
		sm.reports++
		return "\r\n" + sm.report + "\r\n\r\nOK\r\n"
	}
	return "\r\nOK\r\n"
}

func (sm *scriptedModem) dialCount() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return len(sm.dials)
}

// scriptedModemConfig is the modem-test config for a scripted modem.
func scriptedModemConfig(device string) ModemConfig {
	return ModemConfig{
		Device:               device,
		BaudRate:             115200,
		DialPrefix:           "ATDT",
		HangupMethod:         "escape",
		DialTimeout:          Duration(5 * time.Second),
		CarrierTimeout:       Duration(time.Second),
		ATCommandTimeout:     Duration(2 * time.Second),
		ReadTimeout:          Duration(time.Second),
		InitCommands:         []string{"ATZ"},
		FailureReportCommand: scriptedReportCommand,
	}
}

// openScriptedModem opens the real modem driver on the pty slave.
func openScriptedModem(t *testing.T, cfg ModemConfig) *modem.Modem {
	t.Helper()
	m, err := modem.New(modem.Config{
		Device:           cfg.Device,
		BaudRate:         cfg.BaudRate,
		FlowControl:      modem.FlowNone,
		InitCommands:     cfg.InitCommands,
		DialPrefix:       cfg.DialPrefix,
		HangupMethod:     cfg.HangupMethod,
		DialTimeout:      cfg.DialTimeout.Duration(),
		CarrierTimeout:   cfg.CarrierTimeout.Duration(),
		ATCommandTimeout: cfg.ATCommandTimeout.Duration(),
		ReadTimeout:      cfg.ReadTimeout.Duration(),
	})
	if err != nil {
		t.Fatalf("modem.New: %v", err)
	}
	if err := m.Open(); err != nil {
		t.Skipf("modem driver cannot open a pty here: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

// The reports name the call's result code mid-line ("End: NO ANSWER ..."),
// as the software modem's ATI11 does. A reader that stops at the first
// result-code substring never reaches the Disconnect: line below it.
const (
	voiceReport = "End: NO ANSWER phase=training step=V.8/wait-ANSam\r\nDisconnect: local reason=voice answered"
	otherReport = "End: BUSY phase=dial\r\nDisconnect: remote reason=remote SIP call ended"
)

// dialLoop runs one test through either flow's dial loop.
type dialLoop func(t *testing.T, m *modem.Modem, cfg ModemConfig, guard *callGuard, retries int) testResult

var dialLoops = map[string]dialLoop{
	"runSingleTest": func(t *testing.T, m *modem.Modem, mc ModemConfig, guard *callGuard, retries int) testResult {
		cfg := DefaultConfig()
		cfg.Modem = mc
		cfg.Test.RetryCount = retries
		cfg.Test.Pause = Duration(10 * time.Millisecond)
		return runSingleTest(context.Background(), m, cfg, discardLogger(), nil, nil, guard, 1, "74951234567", "74951234567", nil)
	},
	"ModemWorker.runTest": func(t *testing.T, m *modem.Modem, mc ModemConfig, guard *callGuard, retries int) testResult {
		w := &ModemWorker{
			name:           "m1",
			modem:          m,
			config:         ModemInstanceConfig{ModemConfig: mc, Name: "m1"},
			retryCount:     retries,
			retryDelay:     10 * time.Millisecond,
			cdrLookupDelay: 10 * time.Millisecond,
			log:            discardLogger(),
			guard:          guard,
		}
		// The dial string carries an operator prefix; the list and the
		// result are about the bare number.
		return w.runTest(context.Background(), 1, "00"+"74951234567", "74951234567", nil, nil)
	},
}

// A voice answer must end the test after one call even when the dial result
// alone (BUSY) would be retried.
func TestDialLoop_VoiceAnswerIsNeverRetried(t *testing.T) {
	for name, run := range dialLoops {
		t.Run(name, func(t *testing.T) {
			sm, device := newScriptedModem(t, "BUSY", voiceReport)
			cfg := scriptedModemConfig(device)
			m := openScriptedModem(t, cfg)

			result := run(t, m, cfg, nil, 3)

			if got := sm.dialCount(); got != 1 {
				t.Errorf("number was dialled %d times, want exactly 1", got)
			}
			if !result.voiceAnswered {
				t.Error("voiceAnswered = false, want true")
			}
			if result.failureDetail != "voice answered" {
				t.Errorf("failureDetail = %q, want %q", result.failureDetail, "voice answered")
			}
			if result.success {
				t.Error("success = true for a voice-answered call")
			}
			if !strings.Contains(result.message, "VOICE ANSWERED") {
				t.Errorf("message %q does not say VOICE ANSWERED", result.message)
			}
		})
	}
}

// Control for the test above: the same BUSY with an ordinary failure reason
// is retried, so the single dial there is the voice rule and not an accident
// of the rig.
func TestDialLoop_BusyWithoutVoiceIsRetried(t *testing.T) {
	for name, run := range dialLoops {
		t.Run(name, func(t *testing.T) {
			sm, device := newScriptedModem(t, "BUSY", otherReport)
			cfg := scriptedModemConfig(device)
			m := openScriptedModem(t, cfg)

			result := run(t, m, cfg, nil, 2)

			if got := sm.dialCount(); got != 3 {
				t.Errorf("number was dialled %d times, want 3 (1 + 2 retries)", got)
			}
			if result.voiceAnswered {
				t.Error("voiceAnswered = true for a plain BUSY")
			}
			if result.failureDetail != "remote SIP call ended" {
				t.Errorf("failureDetail = %q, want %q", result.failureDetail, "remote SIP call ended")
			}
		})
	}
}

// A listed number must not produce a single ATD, in either flow, and with an
// operator prefix on the dial string.
func TestDialLoop_DoNotCallNumberIsNeverDialled(t *testing.T) {
	for name, run := range dialLoops {
		t.Run(name, func(t *testing.T) {
			sm, device := newScriptedModem(t, "BUSY", otherReport)
			cfg := scriptedModemConfig(device)
			m := openScriptedModem(t, cfg)

			list, err := LoadDoNotCallList(t.TempDir() + "/do-not-call.txt")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := list.Add("+7-495-123-45-67", "2:5020/100", "voice answered", time.Now()); err != nil {
				t.Fatal(err)
			}

			result := run(t, m, cfg, &callGuard{list: list}, 3)

			if got := sm.dialCount(); got != 0 {
				t.Errorf("a do-not-call number was dialled %d times", got)
			}
			if !result.doNotCall {
				t.Error("doNotCall = false, want true")
			}
			if result.success || result.voiceAnswered {
				t.Errorf("unexpected result flags: %+v", result)
			}
		})
	}
}
