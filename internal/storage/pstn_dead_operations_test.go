package storage

import (
	"context"
	"strings"
	"testing"
)

// TestPSTNDeadNodeAppliesTo pins the phone-match rule: a mark made for one
// number covers the node only while it still lists that number, and a mark
// without a phone (every row written before marks carried one) covers it
// whatever it lists.
func TestPSTNDeadNodeAppliesTo(t *testing.T) {
	cases := []struct {
		name      string
		markPhone string
		nodePhone string
		want      bool
	}{
		{"no phone on the mark applies to any number", "", "+7-495-123-45-67", true},
		{"no phone on the mark applies to an unpublished number", "", "-Unpublished-", true},
		{"same number, nodelist formatting", "74951234567", "7-495-123-45-67", true},
		{"same number, normalized with plus", "74951234567", "+74951234567", true},
		{"the node lists a new number", "74951234567", "7-495-765-43-21", false},
		{"the node no longer publishes a number", "74951234567", "-Unpublished-", false},
		{"a longer number that merely contains it", "74951234567", "7-495-123-45-678", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mark := PSTNDeadNode{Zone: 2, Net: 5020, Node: 100, Kind: PSTNDeadKindVoice, Phone: tc.markPhone}
			if got := mark.AppliesTo(tc.nodePhone); got != tc.want {
				t.Errorf("mark for %q AppliesTo(%q) = %v, want %v", tc.markPhone, tc.nodePhone, got, tc.want)
			}
		})
	}
}

func TestPhoneDigits(t *testing.T) {
	for in, want := range map[string]string{
		"+7-495-123-45-67":  "74951234567",
		"7 (495) 123 45 67": "74951234567",
		"74951234567":       "74951234567",
		"-Unpublished-":     "",
		"":                  "",
	} {
		if got := PhoneDigits(in); got != want {
			t.Errorf("PhoneDigits(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidPSTNDeadKind(t *testing.T) {
	for kind, want := range map[string]bool{
		PSTNDeadKindDead:  true,
		PSTNDeadKindVoice: true,
		"":                false, // callers default it before asking
		"Voice":           false,
		"fax":             false,
	} {
		if got := ValidPSTNDeadKind(kind); got != want {
			t.Errorf("ValidPSTNDeadKind(%q) = %v, want %v", kind, got, want)
		}
	}
}

// TestMarkDeadRefusesUnknownKind: the storage layer is the last place an
// unknown kind can be stopped before it becomes a row every reader would have
// to guess the meaning of. The nil database proves the refusal comes before
// any query.
func TestMarkDeadRefusesUnknownKind(t *testing.T) {
	ops := NewPSTNDeadOperations(nil)
	err := ops.MarkDead(context.Background(), PSTNDeadNode{Zone: 2, Net: 5020, Node: 100, Kind: "fax"})
	if err == nil || !strings.Contains(err.Error(), "fax") {
		t.Errorf("MarkDead(kind fax) error = %v, want a refusal naming the kind", err)
	}
}
