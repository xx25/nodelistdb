package storage

import (
	"strings"
	"testing"
)

// Reverting any of these to the Exec form -- `VALUES (?, ?, ...)` -- still
// stores rows correctly, so no test that only checks stored data would catch
// it. The damage is silent: clickhouse-go renders each time.Time argument as
// toDateTime('<unix>'), which the server's Values fast-path parser rejects
// before recovering via the expression interpreter, bumping
// CANNOT_PARSE_INPUT_ASSERTION_FAILED in system.errors once per rendered
// timestamp. Pin the batch shape, since that is the only observable difference.
//
// The column count guards the other half: PrepareBatch appends positionally, so
// the Append call must supply exactly this many values in this order. Adding or
// removing a column means updating the SQL, the Append call, AND the count here
// together.
var batchShapedInserts = []struct {
	name    string
	sql     string
	columns int
}{
	{"whois", whoisInsertSQL, 7},
	{"emailDomainCheck", emailDomainCheckInsertSQL, 10},
}

func TestInsertsAreBatchShaped(t *testing.T) {
	for _, tc := range batchShapedInserts {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Contains(tc.sql, "?") {
				t.Errorf("carries bind placeholders, so it would take the text Exec path: %s", tc.sql)
			}
			if strings.Contains(strings.ToUpper(tc.sql), "VALUES") {
				t.Errorf("carries a VALUES clause; PrepareBatch takes the column list only: %s", tc.sql)
			}
		})
	}
}

func TestInsertColumnCounts(t *testing.T) {
	for _, tc := range batchShapedInserts {
		t.Run(tc.name, func(t *testing.T) {
			open := strings.Index(tc.sql, "(")
			closeIdx := strings.LastIndex(tc.sql, ")")
			if open < 0 || closeIdx < open {
				t.Fatalf("no column list: %s", tc.sql)
			}

			cols := strings.Split(tc.sql[open+1:closeIdx], ",")
			if len(cols) != tc.columns {
				t.Fatalf("lists %d columns, want %d (must match the Append call)", len(cols), tc.columns)
			}
		})
	}
}
