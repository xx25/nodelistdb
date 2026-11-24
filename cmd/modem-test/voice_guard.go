// Package main keeps the tester from calling people. A listed number can stop
// being a modem line; when a call is answered by a voice the modem says so,
// and that number must never be dialled again: it goes on a local do-not-call
// list that every dial is checked against, and is marked on the NodelistDB
// server so other testers skip it too.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/xx25/fidomail/pkg/modem"
)

// defaultDoNotCallFile is used when do_not_call_file is not configured.
const defaultDoNotCallFile = "do-not-call.txt"

// defaultVoiceAnswerPatterns are matched against the failure report when
// voice_answer_patterns is not configured.
var defaultVoiceAnswerPatterns = []string{"voice answered"}

// dialErrorVoice is the dial result code some hardware modems return when a
// person answers.
const dialErrorVoice = "VOICE"

// doNotCallHeader starts a newly created list file.
const doNotCallHeader = "# modem-test do-not-call list: numbers a person answered. Never dialled again.\n" +
	"# phone (digits)<TAB>node address<TAB>time (RFC3339)<TAB>reason\n"

// phoneDigits reduces a phone number to its digits, so that "+7-495-123-45-67",
// "+74951234567" and the "74951234567" the dial path uses after stripPhonePlus
// all compare equal. Operator dial prefixes are not part of a number: callers
// pass the node's phone, never the prefixed dial string.
func phoneDigits(phone string) string {
	var b strings.Builder
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// doNotCallEntry is one line of the do-not-call file.
type doNotCallEntry struct {
	Phone       string // digits only
	NodeAddress string // may be empty (number dialled without a node)
	Time        time.Time
	Reason      string
}

// DoNotCallList is the local list of numbers that must not be dialled. It is
// shared by all modem workers.
type DoNotCallList struct {
	mu      sync.Mutex
	path    string
	entries map[string]doNotCallEntry
}

// LoadDoNotCallList reads the list at path. A missing file is an empty list;
// it is created by the first Add. Any other read failure is an error, because
// dialling without knowing the list is exactly what the list exists to stop.
func LoadDoNotCallList(path string) (*DoNotCallList, error) {
	l := &DoNotCallList{path: path, entries: make(map[string]doNotCallEntry)}

	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read do-not-call list: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for lineNum := 1; scanner.Scan(); lineNum++ {
		line := strings.TrimRight(scanner.Text(), "\r")
		if trimmed := strings.TrimSpace(line); trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		phone := phoneDigits(fields[0])
		if phone == "" {
			// A line someone meant as an entry but that blocks nothing must
			// not pass silently.
			return nil, fmt.Errorf("do-not-call list %s line %d: no phone number in %q", path, lineNum, line)
		}
		entry := doNotCallEntry{Phone: phone}
		if len(fields) > 1 {
			entry.NodeAddress = strings.TrimSpace(fields[1])
		}
		if len(fields) > 2 {
			// A hand-added line may carry no usable time; the number still counts.
			entry.Time, _ = time.Parse(time.RFC3339, strings.TrimSpace(fields[2]))
		}
		if len(fields) > 3 {
			entry.Reason = strings.TrimSpace(strings.Join(fields[3:], " "))
		}
		if _, dup := l.entries[phone]; !dup {
			l.entries[phone] = entry
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read do-not-call list: %w", err)
	}
	return l, nil
}

// Path returns the file backing the list.
func (l *DoNotCallList) Path() string {
	return l.path
}

// Len returns the number of listed numbers.
func (l *DoNotCallList) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

// Lookup reports whether phone is listed, in any formatting.
func (l *DoNotCallList) Lookup(phone string) (doNotCallEntry, bool) {
	digits := phoneDigits(phone)
	if digits == "" {
		return doNotCallEntry{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[digits]
	return entry, ok
}

// Add lists phone and appends it to the file. added is false when the number
// was already listed (nothing is written). The number is listed in memory
// even when the file cannot be written, so the running session still honours
// it; the error tells the operator the entry will not survive a restart.
func (l *DoNotCallList) Add(phone, nodeAddress, reason string, now time.Time) (added bool, err error) {
	digits := phoneDigits(phone)
	if digits == "" {
		return false, fmt.Errorf("no digits in phone %q", phone)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, dup := l.entries[digits]; dup {
		return false, nil
	}
	entry := doNotCallEntry{
		Phone:       digits,
		NodeAddress: singleField(nodeAddress),
		Time:        now,
		Reason:      singleField(reason),
	}
	l.entries[digits] = entry

	return true, l.appendEntry(entry)
}

// appendEntry writes one entry to the end of the file, creating it if needed.
func (l *DoNotCallList) appendEntry(entry doNotCallEntry) error {
	var prefix string
	existing, err := os.ReadFile(l.path)
	switch {
	case errors.Is(err, os.ErrNotExist) || (err == nil && len(existing) == 0):
		prefix = doNotCallHeader
	case err != nil:
		return fmt.Errorf("failed to read do-not-call list: %w", err)
	case existing[len(existing)-1] != '\n':
		// A hand-edited file without a final newline would otherwise have the
		// new entry glued onto its last line.
		prefix = "\n"
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open do-not-call list: %w", err)
	}
	line := fmt.Sprintf("%s%s\t%s\t%s\t%s\n", prefix, entry.Phone, entry.NodeAddress, entry.Time.Format(time.RFC3339), entry.Reason)
	if _, err := f.WriteString(line); err != nil {
		f.Close()
		return fmt.Errorf("failed to write do-not-call list: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write do-not-call list: %w", err)
	}
	return nil
}

// singleField makes s safe to store as one tab-separated field.
func singleField(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(s))
}

// failureReport is what the modem said about a failed dial.
type failureReport struct {
	detail        string // text after reason= on the Disconnect: line, if any
	voiceAnswered bool   // a person answered the call
}

// failureReportModem is the part of the modem a failure report needs.
type failureReportModem interface {
	CaptureStats(cmds []string) ([]modem.StatsCapture, error)
	Reset() error
}

// collectFailureReport asks the modem why the dial that just failed did so.
// It must run before any retry decision: a voice answer looks like an
// ordinary NO ANSWER in the dial result, and only the report tells them apart.
//
// The report is read with CaptureStats, not SendAT. SendAT ends its read at
// the first result-code substring anywhere in the output, and the report
// names the call's own result code ("End: NO ANSWER phase=..."): the read
// would stop on that line, before the "Disconnect: ... reason=" line this is
// all about. CaptureStats only ends on a result code that is a line of its
// own; it also drains pending output first and pages a long report.
func collectFailureReport(m failureReportModem, cfg *ModemConfig, dialError string, log *TestLogger) failureReport {
	var response string
	if cmd := strings.TrimSpace(cfg.FailureReportCommand); cmd != "" {
		captures, err := m.CaptureStats([]string{cmd})
		for _, c := range captures {
			response += c.Response + "\n"
			if c.Err != "" {
				log.Warn("Failure report command %s failed: %s", cmd, c.Err)
			}
		}
		for _, line := range reportLines(response) {
			log.Info("%s: %s", cmd, line)
		}
		if err != nil {
			// The modem did not come back to command mode after the report.
			// Whatever arrived is still parsed below: a report that already
			// names a voice answer must not be thrown away.
			log.Warn("Failure report command %s left the modem unresponsive (%v), resetting...", cmd, err)
			if resetErr := m.Reset(); resetErr != nil {
				log.Warn("Modem reset failed: %v", resetErr)
			}
		}
	}
	return parseFailureReport(response, cfg.GetVoiceAnswerPatterns(), dialError)
}

// parseFailureReport extracts the failure detail from a last-call report and
// decides whether a person answered: the report contains one of patterns
// (case-insensitive), or the modem returned the VOICE result code.
func parseFailureReport(response string, patterns []string, dialError string) failureReport {
	report := failureReport{detail: disconnectReason(response)}

	lower := strings.ToLower(response)
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" && strings.Contains(lower, p) {
			report.voiceAnswered = true
			break
		}
	}
	if dialErrorIsVoice(dialError) {
		report.voiceAnswered = true
		if report.detail == "" {
			report.detail = "modem result code VOICE"
		}
	}
	return report
}

// disconnectReason returns the text after "reason=" on the report's
// "Disconnect:" line, or "" when the report has none.
func disconnectReason(response string) string {
	for _, line := range reportLines(response) {
		if !strings.HasPrefix(strings.ToLower(line), "disconnect:") {
			continue
		}
		if idx := strings.Index(strings.ToLower(line), "reason="); idx >= 0 {
			return strings.TrimSpace(line[idx+len("reason="):])
		}
	}
	return ""
}

// dialErrorIsVoice recognises the VOICE result code in a dial error. pkg/modem
// does not know VOICE as a terminal result, so it surfaces as
// "unknown response: VOICE" rather than as the bare code.
func dialErrorIsVoice(dialError string) bool {
	dialError = strings.TrimPrefix(dialError, "unknown response:")
	for _, line := range reportLines(dialError) {
		if line == dialErrorVoice {
			return true
		}
	}
	return false
}

// reportLines splits modem output into trimmed, non-empty lines.
func reportLines(s string) []string {
	var lines []string
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\r' || r == '\n' }) {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// callGuard is the pre-dial gate and the recorder of voice answers. A nil
// guard blocks nothing and records nothing.
type callGuard struct {
	list *DoNotCallList

	// NodelistDB API for marking the node on the server; the mark is skipped
	// when either is empty.
	apiURL string
	apiKey string
}

// blocked reports whether phone is on the do-not-call list.
func (g *callGuard) blocked(phone string) (doNotCallEntry, bool) {
	if g == nil || g.list == nil {
		return doNotCallEntry{}, false
	}
	return g.list.Lookup(phone)
}

// recordVoiceAnswer puts phone on the do-not-call list and, when the API is
// configured and the node is known, marks the node on the server. Neither
// failure stops the run, but a list that could not be saved is logged loudly:
// until someone adds the line by hand, a restart would dial the number again.
func (g *callGuard) recordVoiceAnswer(log *TestLogger, phone, nodeAddress, detail string) {
	if g == nil {
		return
	}
	reason := detail
	if reason == "" {
		reason = "voice answered"
	}

	if g.list != nil {
		added, err := g.list.Add(phone, nodeAddress, reason, time.Now())
		switch {
		case err != nil:
			log.Error("*** DO-NOT-CALL LIST NOT SAVED: %v", err)
			log.Error("*** Add %s to %s by hand, or it will be dialled again after a restart", phoneDigits(phone), g.list.Path())
		case added:
			log.Warn("Added %s to do-not-call list %s", phoneDigits(phone), g.list.Path())
		}
	}

	if g.apiURL == "" || g.apiKey == "" || nodeAddress == "" {
		return
	}
	zone, net, node, err := ParseNodeAddress(nodeAddress)
	if err != nil {
		log.Warn("Cannot mark %q as a voice line on the server: %v", nodeAddress, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = sendPSTNMark(ctx, g.apiURL, g.apiKey, "POST", pstnMarkRequest{
		Zone:   zone,
		Net:    net,
		Node:   node,
		Reason: reason,
		Kind:   pstnMarkKindVoice,
		Phone:  phoneDigits(phone),
	})
	if err != nil {
		log.Error("Failed to mark %s as a voice line on the server: %v", nodeAddress, err)
		return
	}
	log.Info("Marked %s as a voice line on the server", nodeAddress)
}

// doNotCallResult is the result of a test that was not dialled because the
// number is on the do-not-call list.
func doNotCallResult(log *TestLogger, label, phone string, entry doNotCallEntry) testResult {
	why := entry.Reason
	if why == "" {
		why = "listed"
	}
	if !entry.Time.IsZero() {
		why += ", " + entry.Time.Format("2006-01-02")
	}
	log.Warn("NOT DIALLING %s: on the do-not-call list (%s)", phone, why)
	return testResult{
		doNotCall: true,
		message:   fmt.Sprintf("%s: SKIPPED - on do-not-call list (%s)", label, why),
	}
}

// dropDoNotCallNodes removes nodes whose phone is listed, logging each one.
func dropDoNotCallNodes(nodes []NodeTarget, guard *callGuard, log *TestLogger) []NodeTarget {
	kept := make([]NodeTarget, 0, len(nodes))
	for _, n := range nodes {
		if entry, blocked := guard.blocked(n.Phone); blocked {
			log.Warn("Skipping %s (%s): on the do-not-call list (%s)", n.Address(), n.Phone, entry.Reason)
			continue
		}
		kept = append(kept, n)
	}
	return kept
}

// dropDoNotCallPhones removes listed numbers from a plain phone list, logging
// each one.
func dropDoNotCallPhones(phones []string, guard *callGuard, log *TestLogger) []string {
	kept := make([]string, 0, len(phones))
	for _, p := range phones {
		if entry, blocked := guard.blocked(p); blocked {
			log.Warn("Skipping %s: on the do-not-call list (%s)", p, entry.Reason)
			continue
		}
		kept = append(kept, p)
	}
	return kept
}

// dialFailureText is the reason shown for a failed dial: the modem's result
// code, with the failure report's detail when there is one.
func dialFailureText(dialError string, report failureReport) string {
	switch {
	case report.voiceAnswered && report.detail != "":
		return fmt.Sprintf("VOICE ANSWERED (%s: %s)", dialError, report.detail)
	case report.voiceAnswered:
		return fmt.Sprintf("VOICE ANSWERED (%s)", dialError)
	case report.detail != "":
		return fmt.Sprintf("%s [%s]", dialError, report.detail)
	}
	return dialError
}
