package storage

import (
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
)

// StoreEmailDomainCheck appends mx_resolved as a []uint8. Go cannot distinguish
// []uint8 from []byte, and clickhouse-go does special-case []byte elsewhere as a
// binary string, so an Array(UInt8) column could plausibly receive the whole
// slice as one opaque value instead of three numbers. It does not: array_gen.go
// dispatches on `case []uint8` and appends element-wise, and bare []byte is not
// a case at all (only [][]byte, for Array(String)).
//
// That behaviour is a property of the driver, not of our code, so it is worth
// pinning: a clickhouse-go upgrade that reordered or removed the []uint8 branch
// would silently corrupt every stored MX resolution flag, and the column type
// would still read back as Array(UInt8). The values are deliberately mixed --
// [1 0 1] as raw bytes is "\x01\x00\x01", which is exactly what a []byte mangle
// would produce.
func TestUint8SliceEncodesAsNumericArray(t *testing.T) {
	col, err := column.Type("Array(UInt8)").Column("mx_resolved", nil)
	if err != nil {
		t.Fatalf("build Array(UInt8) column: %v", err)
	}

	if err := col.AppendRow([]uint8{1, 0, 1}); err != nil {
		t.Fatalf("append []uint8: %v", err)
	}

	got, ok := col.Row(0, false).([]uint8)
	if !ok {
		t.Fatalf("row read back as %T, want []uint8 -- the driver treated the slice as an opaque value", col.Row(0, false))
	}

	want := []uint8{1, 0, 1}
	if len(got) != len(want) {
		t.Fatalf("row has %d elements (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("element %d = %d, want %d (full row %v, want %v)", i, got[i], want[i], got, want)
		}
	}
}

// The other two array columns written by StoreEmailDomainCheck, for the same
// reason: they are what the Append call actually passes.
func TestEmailDomainArrayColumnsRoundTrip(t *testing.T) {
	t.Run("mx_preferences", func(t *testing.T) {
		col, err := column.Type("Array(UInt16)").Column("mx_preferences", nil)
		if err != nil {
			t.Fatalf("build column: %v", err)
		}
		if err := col.AppendRow([]uint16{10, 20, 30}); err != nil {
			t.Fatalf("append []uint16: %v", err)
		}
		got, ok := col.Row(0, false).([]uint16)
		if !ok || len(got) != 3 || got[0] != 10 || got[1] != 20 || got[2] != 30 {
			t.Fatalf("read back %v (%T), want [10 20 30]", col.Row(0, false), col.Row(0, false))
		}
	})

	t.Run("mx_hosts", func(t *testing.T) {
		col, err := column.Type("Array(String)").Column("mx_hosts", nil)
		if err != nil {
			t.Fatalf("build column: %v", err)
		}
		if err := col.AppendRow([]string{"mx1.example", "mx2.example"}); err != nil {
			t.Fatalf("append []string: %v", err)
		}
		got, ok := col.Row(0, false).([]string)
		if !ok || len(got) != 2 || got[0] != "mx1.example" || got[1] != "mx2.example" {
			t.Fatalf("read back %v (%T), want [mx1.example mx2.example]", col.Row(0, false), col.Row(0, false))
		}
	})
}
