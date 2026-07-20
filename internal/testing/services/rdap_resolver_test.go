package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVcardFN(t *testing.T) {
	raw := json.RawMessage(`["vcard",[["version",{},"text","4.0"],["fn",{},"text","Porkbun LLC"]]]`)
	if got := vcardFN(raw); got != "Porkbun LLC" {
		t.Errorf("vcardFN = %q, want Porkbun LLC", got)
	}
	if got := vcardFN(json.RawMessage(`[]`)); got != "" {
		t.Errorf("vcardFN(empty) = %q, want empty", got)
	}
	if got := vcardFN(json.RawMessage(`not json`)); got != "" {
		t.Errorf("vcardFN(garbage) = %q, want empty", got)
	}
}

func TestRegistrarFromEntities(t *testing.T) {
	var doc rdapDomain
	body := `{"entities":[
		{"roles":["technical"],"vcardArray":["vcard",[["fn",{},"text","Tech Person"]]]},
		{"roles":["registrar"],"vcardArray":["vcard",[["fn",{},"text","Gandi SAS"]]]}
	]}`
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.registrarName(); got != "Gandi SAS" {
		t.Errorf("registrarName = %q, want Gandi SAS", got)
	}
}

func TestRegistrarFromEntities_Nested(t *testing.T) {
	var doc rdapDomain
	body := `{"entities":[
		{"roles":["registrant"],"entities":[
			{"roles":["registrar"],"vcardArray":["vcard",[["fn",{},"text","Nested Registrar"]]]}
		]}
	]}`
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.registrarName(); got != "Nested Registrar" {
		t.Errorf("registrarName(nested) = %q, want Nested Registrar", got)
	}
}

func TestEventDate(t *testing.T) {
	var doc rdapDomain
	body := `{"events":[
		{"eventAction":"registration","eventDate":"2018-05-26T13:34:50Z"},
		{"eventAction":"expiration","eventDate":"2028-05-26T13:34:50Z"}
	]}`
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	exp := doc.eventDate("expiration")
	if exp == nil || exp.Year() != 2028 {
		t.Errorf("expiration = %v, want 2028", exp)
	}
	reg := doc.eventDate("registration")
	if reg == nil || reg.Year() != 2018 {
		t.Errorf("registration = %v, want 2018", reg)
	}
	if doc.eventDate("transfer") != nil {
		t.Errorf("absent event should return nil")
	}
}

func TestPreferredRDAPBase(t *testing.T) {
	if got := preferredRDAPBase([]string{"http://a.test/", "https://b.test/"}); got != "https://b.test" {
		t.Errorf("preferredRDAPBase prefers https, got %q", got)
	}
	if got := preferredRDAPBase([]string{"http://a.test/"}); got != "http://a.test" {
		t.Errorf("preferredRDAPBase = %q", got)
	}
}

// rdapTestServer serves both the bootstrap file and RDAP domain queries.
func rdapTestServer(t *testing.T) (*httptest.Server, *RDAPResolver) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)

	mux.HandleFunc("/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"services": [][]any{
				{[]string{"one", "app"}, []string{srv.URL + "/rdap/"}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/rdap/domain/", func(w http.ResponseWriter, r *http.Request) {
		domain := strings.TrimPrefix(r.URL.Path, "/rdap/domain/")
		if domain == "gone.one" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/rdap+json")
		_, _ = w.Write([]byte(`{
			"entities":[{"roles":["registrar"],"vcardArray":["vcard",[["fn",{},"text","Porkbun LLC"]]]}],
			"events":[{"eventAction":"expiration","eventDate":"2028-05-26T13:34:50Z"}],
			"status":["client transfer prohibited"]
		}`))
	})

	r := NewRDAPResolver(5 * time.Second)
	r.bootstrapURL = srv.URL + "/bootstrap"
	return srv, r
}

func TestRDAPLookup_Success(t *testing.T) {
	srv, r := rdapTestServer(t)
	defer srv.Close()
	defer r.Close()

	got := r.Lookup(context.Background(), "sog.one")
	if got == nil {
		t.Fatal("expected a result, got nil")
	}
	if got.Error != "" {
		t.Fatalf("Error = %q, want empty", got.Error)
	}
	if got.Registrar != "Porkbun LLC" {
		t.Errorf("registrar = %q, want Porkbun LLC", got.Registrar)
	}
	if got.ExpirationDate == nil || got.ExpirationDate.Year() != 2028 {
		t.Errorf("expiration = %v, want 2028", got.ExpirationDate)
	}
	if got.Status == "" {
		t.Errorf("status not populated")
	}
}

func TestRDAPLookup_NotFound(t *testing.T) {
	srv, r := rdapTestServer(t)
	defer srv.Close()
	defer r.Close()

	got := r.Lookup(context.Background(), "gone.one")
	if got == nil || got.Error != "domain not found" {
		t.Fatalf("Error = %v, want domain not found", got)
	}
}

// Issue #3: a failing bootstrap fetch must back off — at most one attempt per
// cooldown window — instead of re-fetching (and paying a full timeout) on every call.
func TestRDAPBootstrap_BackoffOnFailure(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewRDAPResolver(2 * time.Second)
	defer r.Close()
	r.bootstrapURL = srv.URL
	r.bootstrapCooldown = time.Hour // long enough that all calls fall inside it

	// Every cold-start call within the cooldown must still surface a TRANSIENT error
	// (not nil), so a WhoisNoServerError domain is retried rather than silently
	// suppressed for 24h. Only the first call should actually hit the network.
	for i := 0; i < 3; i++ {
		got := r.Lookup(context.Background(), "example.one")
		if got == nil || got.Error == "" {
			t.Fatalf("call %d: expected a transient error result, got %+v", i, got)
		}
	}
	if hits != 1 {
		t.Errorf("bootstrap fetched %d times, want 1 (cooldown should suppress refetch)", hits)
	}
}

// Issue #3 (regression lock): once the bootstrap has loaded successfully, a later
// failed REFRESH must keep serving the stale map — the cold-start guard must not
// over-tighten and start erroring on the warm path.
func TestRDAPBootstrap_WarmStaleFallback(t *testing.T) {
	var bootstrapHits int
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		bootstrapHits++
		if bootstrapHits == 1 {
			resp := map[string]any{"services": [][]any{{[]string{"one"}, []string{srv.URL + "/rdap/"}}}}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusInternalServerError) // refreshes fail
	})
	mux.HandleFunc("/rdap/domain/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"entities":[{"roles":["registrar"],"vcardArray":["vcard",[["fn",{},"text","Reg"]]]}]}`))
	})

	r := NewRDAPResolver(2 * time.Second)
	defer r.Close()
	r.bootstrapURL = srv.URL + "/bootstrap"

	// Prime a successful bootstrap + lookup.
	if got := r.Lookup(context.Background(), "a.one"); got == nil || got.Error != "" || got.Registrar != "Reg" {
		t.Fatalf("priming lookup failed: %+v", got)
	}
	// Force a refresh; it fails, but the stale map must still resolve the TLD.
	r.bootstrapNextTry = time.Now().Add(-time.Hour)
	got := r.Lookup(context.Background(), "a.one")
	if got == nil || got.Error != "" || got.Registrar != "Reg" {
		t.Fatalf("warm stale fallback failed after refresh error: %+v", got)
	}
}

func TestRDAPLookup_NoEndpoint(t *testing.T) {
	srv, r := rdapTestServer(t)
	defer srv.Close()
	defer r.Close()

	// .net is not in the test bootstrap → RDAP not applicable → nil.
	if got := r.Lookup(context.Background(), "example.net"); got != nil {
		t.Errorf("expected nil for TLD with no RDAP endpoint, got %+v", got)
	}
}
