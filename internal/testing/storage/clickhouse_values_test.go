package storage

import (
	"testing"

	"github.com/nodelistdb/internal/testing/models"
)

// resultToValuesColumns is the number of columns in the INSERT statement in
// flushBatch. resultToValues must return exactly this many values in the same
// order, or ClickHouse batch appends fail at runtime. If you add or remove a
// column, update the INSERT list, resultToValues, AND this constant together.
const resultToValuesColumns = 123

func TestResultToValuesColumnCount(t *testing.T) {
	s := &ClickHouseStorage{}
	vals := s.resultToValues(&models.TestResult{})
	if len(vals) != resultToValuesColumns {
		t.Fatalf("resultToValues returned %d values, want %d (must match the flushBatch INSERT column list)", len(vals), resultToValuesColumns)
	}
}
