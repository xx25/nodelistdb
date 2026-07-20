package daemon

import (
	"testing"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

func TestClassifyWhoisResult(t *testing.T) {
	exp := time.Now().AddDate(1, 0, 0)

	tests := []struct {
		name        string
		result      *models.WhoisResult
		wantPersist bool
		wantSeen    bool
	}{
		{
			name:        "success with registrar",
			result:      &models.WhoisResult{Registrar: "Porkbun"},
			wantPersist: true,
			wantSeen:    true,
		},
		{
			name:        "success with expiry only",
			result:      &models.WhoisResult{ExpirationDate: &exp},
			wantPersist: true,
			wantSeen:    true,
		},
		{
			name:        "success with status only (.nl-style)",
			result:      &models.WhoisResult{Status: "active"},
			wantPersist: true,
			wantSeen:    true,
		},
		{
			name:        "success but empty stub",
			result:      &models.WhoisResult{},
			wantPersist: false,
			wantSeen:    true,
		},
		{
			name:        "domain not found",
			result:      &models.WhoisResult{Error: "domain not found"},
			wantPersist: true,
			wantSeen:    true,
		},
		{
			name:        "no whois server (RDAP-only TLD)",
			result:      &models.WhoisResult{Error: models.WhoisNoServerError},
			wantPersist: false,
			wantSeen:    true,
		},
		{
			name:        "transient error",
			result:      &models.WhoisResult{Error: "WHOIS lookup failed: i/o timeout"},
			wantPersist: false,
			wantSeen:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			persist, seen := classifyWhoisResult(tt.result)
			if persist != tt.wantPersist {
				t.Errorf("persist = %v, want %v", persist, tt.wantPersist)
			}
			if seen != tt.wantSeen {
				t.Errorf("markSeen = %v, want %v", seen, tt.wantSeen)
			}
		})
	}
}
