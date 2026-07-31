package storage

import (
	"strings"
	"testing"
)

// whoisInsertColumns is the number of columns in whoisInsertSQL. StoreWhoisResult
// must Append exactly this many values in the same order, or the batch fails at
// runtime. Adding or removing a column means updating the SQL, the Append call,
// AND this constant together.
const whoisInsertColumns = 7

// Reverting whoisInsertSQL to the Exec form -- `VALUES (?, ?, ...)` -- still
// stores rows correctly, so no test that only checks stored data would catch it.
// The damage is silent: clickhouse-go renders the time.Time argument as
// toDateTime('<unix>'), which the server's Values fast-path parser rejects
// before recovering via the expression interpreter, bumping
// CANNOT_PARSE_INPUT_ASSERTION_FAILED in system.errors once per call. Pin the
// batch shape, since that is the only observable difference.
func TestWhoisInsertIsBatchShaped(t *testing.T) {
	if strings.Contains(whoisInsertSQL, "?") {
		t.Errorf("whoisInsertSQL carries bind placeholders, so it would take the text Exec path: %s", whoisInsertSQL)
	}
	if strings.Contains(strings.ToUpper(whoisInsertSQL), "VALUES") {
		t.Errorf("whoisInsertSQL carries a VALUES clause; PrepareBatch takes the column list only: %s", whoisInsertSQL)
	}
}

func TestWhoisInsertColumnCount(t *testing.T) {
	open := strings.Index(whoisInsertSQL, "(")
	closeIdx := strings.LastIndex(whoisInsertSQL, ")")
	if open < 0 || closeIdx < open {
		t.Fatalf("whoisInsertSQL has no column list: %s", whoisInsertSQL)
	}

	cols := strings.Split(whoisInsertSQL[open+1:closeIdx], ",")
	if len(cols) != whoisInsertColumns {
		t.Fatalf("whoisInsertSQL lists %d columns, want %d (must match the StoreWhoisResult Append call)", len(cols), whoisInsertColumns)
	}
}
