package storage

import (
	"testing"

	"github.com/nodelistdb/internal/testing/models"
)

// resultToValuesColumns is the number of columns in the INSERT statement in
// flushBatchLocked. resultToValues must return exactly this many values in the same
// order, or ClickHouse batch appends fail at runtime. If you add or remove a
// column, update the INSERT list, resultToValues, AND this constant together.
const resultToValuesColumns = 138

func TestResultToValuesColumnCount(t *testing.T) {
	s := &ClickHouseStorage{}
	vals := s.resultToValues(&models.TestResult{})
	if len(vals) != resultToValuesColumns {
		t.Fatalf("resultToValues returned %d values, want %d (must match the flushBatchLocked INSERT column list)", len(vals), resultToValuesColumns)
	}
}

// TestResultToValuesDNSFallback pins the encoding of the fallback-probe columns.
//
// The array columns must be non-nil even when no probe ran: the ClickHouse
// driver rejects a nil slice for Array(String), which is how the email domain
// check writer broke once already.
func TestResultToValuesDNSFallback(t *testing.T) {
	s := &ClickHouseStorage{}

	t.Run("no probe", func(t *testing.T) {
		vals := s.resultToValues(&models.TestResult{})
		fb := vals[len(vals)-10:]

		if attempted, ok := fb[1].(bool); !ok || attempted {
			t.Errorf("dns_fallback_attempted = %v, want false", fb[1])
		}
		for i, name := range map[int]string{2: "dns_fallback_ipv4", 3: "dns_fallback_ipv6", 7: "dns_fallback_protocols"} {
			arr, ok := fb[i].([]string)
			if !ok {
				t.Fatalf("%s is %T, want []string", name, fb[i])
			}
			if arr == nil {
				t.Errorf("%s is a nil slice; the driver rejects nil for Array(String)", name)
			}
		}
	})

	t.Run("probe recorded", func(t *testing.T) {
		vals := s.resultToValues(&models.TestResult{
			DNSErrorKind: models.DNSErrorTimeout,
			DNSFallback: &models.DNSFallbackProbe{
				Source:           models.FallbackSourceNodelistLiteral,
				IPv4:             []string{"217.71.231.2"},
				AgeHours:         0,
				Success:          true,
				Protocols:        []string{"binkp"},
				AddressValidated: true,
			},
		})
		fb := vals[len(vals)-10:]

		if fb[0] != models.DNSErrorTimeout {
			t.Errorf("dns_error_kind = %v", fb[0])
		}
		if attempted, _ := fb[1].(bool); !attempted {
			t.Error("dns_fallback_attempted = false, want true")
		}
		if ipv4, _ := fb[2].([]string); len(ipv4) != 1 || ipv4[0] != "217.71.231.2" {
			t.Errorf("dns_fallback_ipv4 = %v", fb[2])
		}
		if fb[4] != models.FallbackSourceNodelistLiteral {
			t.Errorf("dns_fallback_source = %v", fb[4])
		}
		if success, _ := fb[6].(bool); !success {
			t.Error("dns_fallback_success = false, want true")
		}
		if validated, _ := fb[8].(bool); !validated {
			t.Error("dns_fallback_address_validated = false, want true")
		}
	})
}
