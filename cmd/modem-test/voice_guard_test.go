// Package main tests the voice-answer protection: failure report parsing, the
// do-not-call list and the recording of a voice answer.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xx25/fidomail/pkg/modem"
)

func TestParseFailureReport(t *testing.T) {
	patterns := defaultVoiceAnswerPatterns
	tests := []struct {
		name       string
		response   string
		patterns   []string
		dialError  string
		wantDetail string
		wantVoice  bool
	}{
		{
			name:       "voice answered",
			response:   "End: NO ANSWER phase=training step=V.8/wait-ANSam\nDisconnect: local reason=voice answered\n\nOK\n",
			patterns:   patterns,
			dialError:  "NO ANSWER",
			wantDetail: "voice answered",
			wantVoice:  true,
		},
		{
			// A recorded announcement BEFORE answer is a dead number, not a
			// person: detail only.
			name:       "announcement before answer is not a person",
			response:   "End: NO ANSWER phase=dial\r\nDisconnect: local reason=whole call attempt expired after a voice announcement\r\nOK\r\n",
			patterns:   patterns,
			dialError:  "NO ANSWER",
			wantDetail: "whole call attempt expired after a voice announcement",
		},
		{
			name:       "ordinary timeout",
			response:   "End: NO ANSWER phase=training\nDisconnect: local reason=answer signal timeout\nOK\n",
			patterns:   patterns,
			dialError:  "NO ANSWER",
			wantDetail: "answer signal timeout",
		},
		{
			name:       "remote reason",
			response:   "Disconnect: remote reason=remote SIP call ended\nOK\n",
			patterns:   patterns,
			dialError:  "NO CARRIER",
			wantDetail: "remote SIP call ended",
		},
		{
			name:       "pattern is case-insensitive",
			response:   "DISCONNECT: local REASON=Voice Answered\n",
			patterns:   []string{"VOICE answered"},
			dialError:  "NO ANSWER",
			wantDetail: "Voice Answered",
			wantVoice:  true,
		},
		{
			name:       "configured pattern outside the Disconnect line",
			response:   "Last call: human detected\nDisconnect: local reason=aborted\n",
			patterns:   []string{"human detected"},
			dialError:  "NO CARRIER",
			wantDetail: "aborted",
			wantVoice:  true,
		},
		{
			name:       "blank patterns match nothing",
			response:   "Disconnect: local reason=answer signal timeout\n",
			patterns:   []string{"", "  "},
			dialError:  "NO ANSWER",
			wantDetail: "answer signal timeout",
		},
		{
			name:       "VOICE result code without a report",
			patterns:   patterns,
			dialError:  "VOICE",
			wantDetail: "modem result code VOICE",
			wantVoice:  true,
		},
		{
			// pkg/modem has no VOICE terminal: this is how it arrives.
			name:       "VOICE as an unknown dial response",
			patterns:   patterns,
			dialError:  "unknown response: \r\nVOICE\r\n",
			wantDetail: "modem result code VOICE",
			wantVoice:  true,
		},
		{
			name:      "no report, ordinary failure",
			patterns:  patterns,
			dialError: "BUSY",
		},
		{
			name:      "report without a Disconnect line",
			response:  "ERROR\n",
			patterns:  patterns,
			dialError: "NO CARRIER",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFailureReport(tt.response, tt.patterns, tt.dialError)
			if got.detail != tt.wantDetail {
				t.Errorf("detail = %q, want %q", got.detail, tt.wantDetail)
			}
			if got.voiceAnswered != tt.wantVoice {
				t.Errorf("voiceAnswered = %v, want %v", got.voiceAnswered, tt.wantVoice)
			}
		})
	}
}

func TestDialErrorIsVoice(t *testing.T) {
	for dialError, want := range map[string]bool{
		"VOICE":                         true,
		"unknown response: VOICE":       true,
		"unknown response: RING\nVOICE": true,
		"NO ANSWER":                     false,
		"unknown response: VOICEMAIL":   false,
		"":                              false,
	} {
		if got := dialErrorIsVoice(dialError); got != want {
			t.Errorf("dialErrorIsVoice(%q) = %v, want %v", dialError, got, want)
		}
	}
}

// fakeReportModem records how the failure report was requested.
type fakeReportModem struct {
	captures []modem.StatsCapture
	err      error
	commands []string
	resets   int
}

func (f *fakeReportModem) CaptureStats(cmds []string) ([]modem.StatsCapture, error) {
	f.commands = append(f.commands, cmds...)
	return f.captures, f.err
}

func (f *fakeReportModem) Reset() error {
	f.resets++
	return nil
}

func TestCollectFailureReport(t *testing.T) {
	voice := []modem.StatsCapture{{Response: "End: NO ANSWER phase=training\nDisconnect: local reason=voice answered\nOK\n"}}

	t.Run("no command configured sends nothing", func(t *testing.T) {
		m := &fakeReportModem{captures: voice}
		report := collectFailureReport(m, &ModemConfig{}, "NO ANSWER", discardLogger())
		if len(m.commands) != 0 {
			t.Errorf("commands sent with no failure_report_command: %v", m.commands)
		}
		if report.voiceAnswered || report.detail != "" {
			t.Errorf("report = %+v, want empty", report)
		}
	})

	t.Run("VOICE result code needs no command", func(t *testing.T) {
		m := &fakeReportModem{}
		report := collectFailureReport(m, &ModemConfig{}, "VOICE", discardLogger())
		if !report.voiceAnswered {
			t.Error("voiceAnswered = false for a VOICE result code")
		}
	})

	t.Run("configured command is sent and parsed", func(t *testing.T) {
		m := &fakeReportModem{captures: voice}
		report := collectFailureReport(m, &ModemConfig{FailureReportCommand: " ATI11 "}, "NO ANSWER", discardLogger())
		if len(m.commands) != 1 || m.commands[0] != "ATI11" {
			t.Errorf("commands = %v, want [ATI11]", m.commands)
		}
		if !report.voiceAnswered || report.detail != "voice answered" {
			t.Errorf("report = %+v, want voice answered", report)
		}
		if m.resets != 0 {
			t.Errorf("modem reset %d times after a clean report", m.resets)
		}
	})

	t.Run("custom patterns replace the default", func(t *testing.T) {
		m := &fakeReportModem{captures: voice}
		cfg := &ModemConfig{FailureReportCommand: "ATI11", VoiceAnswerPatterns: []string{"human"}}
		if report := collectFailureReport(m, cfg, "NO ANSWER", discardLogger()); report.voiceAnswered {
			t.Error("default pattern matched although voice_answer_patterns was overridden")
		}
	})

	t.Run("a report that wedges the modem is still used, and the modem reset", func(t *testing.T) {
		m := &fakeReportModem{captures: voice, err: modem.ErrStatsResyncFailed}
		report := collectFailureReport(m, &ModemConfig{FailureReportCommand: "ATI11"}, "NO ANSWER", discardLogger())
		if !report.voiceAnswered {
			t.Error("voice answer lost because the modem did not resync")
		}
		if m.resets != 1 {
			t.Errorf("modem reset %d times, want 1", m.resets)
		}
	})
}

func TestPhoneDigits(t *testing.T) {
	for in, want := range map[string]string{
		"+7-495-123-45-67":  "74951234567",
		"74951234567":       "74951234567",
		"+7 (495) 123 4567": "74951234567",
		"-Unpublished-":     "",
		"":                  "",
	} {
		if got := phoneDigits(in); got != want {
			t.Errorf("phoneDigits(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadDoNotCallList(t *testing.T) {
	t.Run("missing file is an empty list", func(t *testing.T) {
		list, err := LoadDoNotCallList(filepath.Join(t.TempDir(), "absent.txt"))
		if err != nil {
			t.Fatalf("LoadDoNotCallList() error = %v", err)
		}
		if list.Len() != 0 {
			t.Errorf("Len() = %d, want 0", list.Len())
		}
	})

	t.Run("entries, comments and hand-written lines", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "do-not-call.txt")
		content := "# comment\n" +
			"\n" +
			"74951234567\t2:5020/100\t2026-11-23T22:15:04+03:00\tvoice answered\n" +
			"  # indented comment\n" +
			"+7-812-555-00-11\t\t\tadded by hand\r\n" +
			"4930123456\n" +
			"74951234567\t2:5020/200\t2026-11-24T10:00:00Z\tduplicate, first one wins\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		list, err := LoadDoNotCallList(path)
		if err != nil {
			t.Fatalf("LoadDoNotCallList() error = %v", err)
		}
		if list.Len() != 3 {
			t.Fatalf("Len() = %d, want 3", list.Len())
		}

		entry, ok := list.Lookup("74951234567")
		if !ok {
			t.Fatal("74951234567 not found")
		}
		if entry.NodeAddress != "2:5020/100" || entry.Reason != "voice answered" {
			t.Errorf("entry = %+v, want node 2:5020/100, reason voice answered", entry)
		}
		if want := time.Date(2026, 11, 23, 19, 15, 4, 0, time.UTC); !entry.Time.Equal(want) {
			t.Errorf("entry.Time = %v, want %v", entry.Time, want)
		}

		if entry, ok := list.Lookup("78125550011"); !ok || entry.NodeAddress != "" || entry.Reason != "added by hand" {
			t.Errorf("hand-written entry = %+v (found %v)", entry, ok)
		}
		if _, ok := list.Lookup("4930123456"); !ok {
			t.Error("phone-only line not listed")
		}
	})

	t.Run("a line without a number is an error, not a silent gap", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "do-not-call.txt")
		if err := os.WriteFile(path, []byte("74951234567\nnot a phone\n"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadDoNotCallList(path)
		if err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Errorf("LoadDoNotCallList() error = %v, want one naming line 2", err)
		}
	})

	t.Run("an unreadable list is an error", func(t *testing.T) {
		// A directory in place of the file: exists, cannot be read as a list.
		if _, err := LoadDoNotCallList(t.TempDir()); err == nil {
			t.Error("LoadDoNotCallList(directory) error = nil, want a failure")
		}
	})
}

func TestDoNotCallList_LookupMatchesAnyFormatting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "do-not-call.txt")
	list, err := LoadDoNotCallList(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := list.Add("+7-495-123-45-67", "2:5020/100", "voice answered", time.Now()); err != nil {
		t.Fatal(err)
	}

	for _, phone := range []string{"74951234567", "+74951234567", "+7-495-123-45-67", "7 495 123 45 67"} {
		if _, ok := list.Lookup(phone); !ok {
			t.Errorf("Lookup(%q) = not listed, want listed", phone)
		}
	}
	// A dial string with an operator prefix is a different digit string:
	// callers must pass the node's number.
	for _, phone := range []string{"74951234568", "0074951234567", "4951234567", ""} {
		if _, ok := list.Lookup(phone); ok {
			t.Errorf("Lookup(%q) = listed, want not listed", phone)
		}
	}
}

func TestDoNotCallList_AddCreatesAppendsAndDedupes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "do-not-call.txt")
	list, err := LoadDoNotCallList(path)
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 11, 23, 22, 15, 4, 0, time.UTC)

	added, err := list.Add("+7-495-123-45-67", "2:5020/100", "voice\tanswered\n", when)
	if err != nil || !added {
		t.Fatalf("first Add() = (%v, %v), want (true, nil)", added, err)
	}
	added, err = list.Add("74951234567", "2:5020/999", "again", when)
	if err != nil || added {
		t.Fatalf("duplicate Add() = (%v, %v), want (false, nil)", added, err)
	}
	added, err = list.Add("78125550011", "", "voice answered", when)
	if err != nil || !added {
		t.Fatalf("second Add() = (%v, %v), want (true, nil)", added, err)
	}
	if _, err := list.Add("-Unpublished-", "", "x", when); err == nil {
		t.Error("Add() of a phone without digits succeeded")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := doNotCallHeader +
		"74951234567\t2:5020/100\t2026-11-23T22:15:04Z\tvoice answered\n" +
		"78125550011\t\t2026-11-23T22:15:04Z\tvoice answered\n"
	if string(data) != want {
		t.Errorf("file content:\n%q\nwant:\n%q", data, want)
	}

	// What was written must load back as the same list.
	reloaded, err := LoadDoNotCallList(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Len() != 2 {
		t.Errorf("reloaded Len() = %d, want 2", reloaded.Len())
	}
	if entry, _ := reloaded.Lookup("78125550011"); entry.NodeAddress != "" || !entry.Time.Equal(when) {
		t.Errorf("reloaded entry = %+v", entry)
	}
}

func TestDoNotCallList_AddAfterFileWithoutFinalNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "do-not-call.txt")
	if err := os.WriteFile(path, []byte("74951234567"), 0644); err != nil {
		t.Fatal(err)
	}
	list, err := LoadDoNotCallList(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := list.Add("78125550011", "", "voice answered", time.Now()); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadDoNotCallList(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	for _, phone := range []string{"74951234567", "78125550011"} {
		if _, ok := reloaded.Lookup(phone); !ok {
			t.Errorf("%s lost: the new entry was glued onto the last line", phone)
		}
	}
}

func TestDoNotCallList_UnwritableFileStillBlocksThisRun(t *testing.T) {
	// The parent directory does not exist, so the append must fail.
	list, err := LoadDoNotCallList(filepath.Join(t.TempDir(), "missing-dir", "do-not-call.txt"))
	if err != nil {
		t.Fatal(err)
	}
	added, err := list.Add("74951234567", "", "voice answered", time.Now())
	if err == nil {
		t.Fatal("Add() error = nil, want a write failure")
	}
	if !added {
		t.Error("added = false, want true: the number is new")
	}
	if _, ok := list.Lookup("74951234567"); !ok {
		t.Error("number not listed in memory after a failed write; it would be dialled again this run")
	}
}

func TestDoNotCallList_ConcurrentAdds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "do-not-call.txt")
	list, err := LoadDoNotCallList(path)
	if err != nil {
		t.Fatal(err)
	}

	// Several workers reporting the same number at once must list it once.
	var wg sync.WaitGroup
	var mu sync.Mutex
	addedCount := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			added, err := list.Add("74951234567", "2:5020/100", "voice answered", time.Now())
			if err != nil {
				t.Errorf("Add() error = %v", err)
			}
			if added {
				mu.Lock()
				addedCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if addedCount != 1 {
		t.Errorf("number added %d times, want 1", addedCount)
	}
	data, _ := os.ReadFile(path)
	if got := strings.Count(string(data), "74951234567"); got != 1 {
		t.Errorf("number appears %d times in the file, want 1", got)
	}
}

func TestCallGuard_NilBlocksAndRecordsNothing(t *testing.T) {
	var guard *callGuard
	if _, blocked := guard.blocked("74951234567"); blocked {
		t.Error("nil guard blocked a number")
	}
	guard.recordVoiceAnswer(discardLogger(), "74951234567", "2:5020/100", "voice answered") // must not panic
}

func TestCallGuard_RecordVoiceAnswer(t *testing.T) {
	type received struct {
		method, path, auth string
		body               pstnMarkRequest
	}
	var mu sync.Mutex
	var got []received
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body pstnMarkRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		got = append(got, received{r.Method, r.URL.Path, r.Header.Get("Authorization"), body})
		code := status
		mu.Unlock()
		w.WriteHeader(code)
	}))
	defer server.Close()

	newGuard := func(t *testing.T, apiURL, apiKey string) *callGuard {
		list, err := LoadDoNotCallList(filepath.Join(t.TempDir(), "do-not-call.txt"))
		if err != nil {
			t.Fatal(err)
		}
		return &callGuard{list: list, apiURL: apiURL, apiKey: apiKey}
	}

	t.Run("lists the number and marks the node as a voice line", func(t *testing.T) {
		got = nil
		guard := newGuard(t, server.URL+"/", "secret")
		guard.recordVoiceAnswer(discardLogger(), "+7-495-123-45-67", "2:5020/100", "voice answered")

		if _, blocked := guard.blocked("74951234567"); !blocked {
			t.Error("number not on the do-not-call list")
		}
		if len(got) != 1 {
			t.Fatalf("API received %d requests, want 1", len(got))
		}
		req := got[0]
		if req.method != http.MethodPost || req.path != "/api/modem/pstn-dead" || req.auth != "Bearer secret" {
			t.Errorf("request = %s %s auth %q", req.method, req.path, req.auth)
		}
		want := pstnMarkRequest{Zone: 2, Net: 5020, Node: 100, Reason: "voice answered", Kind: "voice", Phone: "74951234567"}
		if req.body != want {
			t.Errorf("body = %+v, want %+v", req.body, want)
		}
	})

	t.Run("no node address: list only", func(t *testing.T) {
		got = nil
		guard := newGuard(t, server.URL, "secret")
		guard.recordVoiceAnswer(discardLogger(), "74951234567", "", "")

		if entry, blocked := guard.blocked("74951234567"); !blocked || entry.Reason != "voice answered" {
			t.Errorf("entry = %+v (blocked %v), want reason voice answered", entry, blocked)
		}
		if len(got) != 0 {
			t.Errorf("API received %d requests for a number without a node", len(got))
		}
	})

	t.Run("no API key: list only", func(t *testing.T) {
		got = nil
		guard := newGuard(t, server.URL, "")
		guard.recordVoiceAnswer(discardLogger(), "74951234567", "2:5020/100", "voice answered")

		if _, blocked := guard.blocked("74951234567"); !blocked {
			t.Error("number not on the do-not-call list")
		}
		if len(got) != 0 {
			t.Errorf("API received %d requests without an API key", len(got))
		}
	})

	t.Run("API failure does not undo the list entry", func(t *testing.T) {
		got = nil
		mu.Lock()
		status = http.StatusInternalServerError
		mu.Unlock()
		defer func() { mu.Lock(); status = http.StatusOK; mu.Unlock() }()

		guard := newGuard(t, server.URL, "secret")
		guard.recordVoiceAnswer(discardLogger(), "74951234567", "2:5020/100", "voice answered")

		if _, blocked := guard.blocked("74951234567"); !blocked {
			t.Error("number not on the do-not-call list after an API failure")
		}
	})
}

func TestSendPSTNMark_ReportsServerRefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"kind must be \"dead\" or \"voice\""}`, http.StatusBadRequest)
	}))
	defer server.Close()

	err := sendPSTNMark(t.Context(), server.URL, "k", http.MethodPost, pstnMarkRequest{Zone: 2, Net: 5020, Node: 100, Kind: "nope"})
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("sendPSTNMark() error = %v, want one naming status 400", err)
	}
}

func TestDropDoNotCall(t *testing.T) {
	list, err := LoadDoNotCallList(filepath.Join(t.TempDir(), "do-not-call.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := list.Add("74951234567", "2:5020/100", "voice answered", time.Now()); err != nil {
		t.Fatal(err)
	}
	guard := &callGuard{list: list}

	nodes := []NodeTarget{
		{Zone: 2, Net: 5020, Node: 100, Phone: "74951234567"},
		{Zone: 2, Net: 5020, Node: 200, Phone: "74957654321"},
		// A second node on the same voice line goes too.
		{Zone: 2, Net: 5020, Node: 300, Phone: "74951234567"},
	}
	kept := dropDoNotCallNodes(nodes, guard, discardLogger())
	if len(kept) != 1 || kept[0].Node != 200 {
		t.Errorf("kept nodes = %+v, want only 2:5020/200", kept)
	}

	phones := dropDoNotCallPhones([]string{"+74951234567", "917", "74957654321"}, guard, discardLogger())
	if len(phones) != 2 || phones[0] != "917" || phones[1] != "74957654321" {
		t.Errorf("kept phones = %v, want [917 74957654321]", phones)
	}

	// Without a guard nothing is dropped.
	if got := dropDoNotCallPhones([]string{"74951234567"}, nil, discardLogger()); len(got) != 1 {
		t.Errorf("nil guard dropped phones: %v", got)
	}
}

func TestDialFailureText(t *testing.T) {
	tests := []struct {
		dialError string
		report    failureReport
		want      string
	}{
		{"NO ANSWER", failureReport{}, "NO ANSWER"},
		{"NO ANSWER", failureReport{detail: "answer signal timeout"}, "NO ANSWER [answer signal timeout]"},
		{"NO ANSWER", failureReport{detail: "voice answered", voiceAnswered: true}, "VOICE ANSWERED (NO ANSWER: voice answered)"},
		{"VOICE", failureReport{voiceAnswered: true}, "VOICE ANSWERED (VOICE)"},
	}
	for _, tt := range tests {
		if got := dialFailureText(tt.dialError, tt.report); got != tt.want {
			t.Errorf("dialFailureText(%q, %+v) = %q, want %q", tt.dialError, tt.report, got, tt.want)
		}
	}
}

func TestDoNotCallResult(t *testing.T) {
	entry := doNotCallEntry{Phone: "74951234567", Reason: "voice answered", Time: time.Date(2026, 11, 23, 22, 0, 0, 0, time.UTC)}
	result := doNotCallResult(discardLogger(), "Test 3 [74951234567]", "74951234567", entry)
	if !result.doNotCall || result.success || result.voiceAnswered || result.windowClosed {
		t.Errorf("result flags = %+v, want only doNotCall", result)
	}
	if want := "Test 3 [74951234567]: SKIPPED - on do-not-call list (voice answered, 2026-11-23)"; result.message != want {
		t.Errorf("message = %q, want %q", result.message, want)
	}
}

// The dial loops pass the real modem; keep the interface in step with it.
var _ failureReportModem = (*modem.Modem)(nil)
