package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

// rdapBootstrapURL is the IANA-published RDAP bootstrap file mapping TLDs to the
// authoritative RDAP base URLs. See RFC 9224.
const rdapBootstrapURL = "https://data.iana.org/rdap/dns.json"

// RDAPResolver resolves domain registration data over RDAP (RFC 9083), used as a
// fallback when port-43 WHOIS yields no server or an unusable stub. It maintains an
// in-memory cache of the IANA bootstrap (TLD → RDAP base URLs) and its own 1/sec
// rate limiter so it does not contend with the WHOIS limiter.
type RDAPResolver struct {
	client       *http.Client
	timeout      time.Duration
	bootstrapURL string // overridable in tests

	bootstrapMu       sync.Mutex
	bootstrap         map[string]string // tld → base URL (trailing slash trimmed)
	bootstrapNextTry  time.Time         // do not attempt a (re)fetch before this time
	bootstrapTTL      time.Duration
	bootstrapCooldown time.Duration // wait between attempts after a failed fetch
	bootstrapFetched  bool

	rateLimiter     chan struct{}
	rateLimiterDone chan struct{}
	closeOnce       sync.Once
}

// NewRDAPResolver creates an RDAP resolver with the given per-request timeout.
func NewRDAPResolver(timeout time.Duration) *RDAPResolver {
	r := &RDAPResolver{
		client:            &http.Client{Timeout: timeout},
		timeout:           timeout,
		bootstrapURL:      rdapBootstrapURL,
		bootstrapTTL:      24 * time.Hour,
		bootstrapCooldown: 5 * time.Minute,
		rateLimiter:       make(chan struct{}, 1),
		rateLimiterDone:   make(chan struct{}),
	}

	// Rate limiter: at most one RDAP request per second.
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.rateLimiterDone:
				return
			case <-ticker.C:
				select {
				case r.rateLimiter <- struct{}{}:
				default:
				}
			}
		}
	}()
	r.rateLimiter <- struct{}{} // seed the first token immediately

	return r
}

// Close stops the rate limiter goroutine. Safe to call multiple times.
func (r *RDAPResolver) Close() {
	r.closeOnce.Do(func() {
		close(r.rateLimiterDone)
	})
}

// Lookup resolves a domain over RDAP. It returns:
//   - nil when RDAP is not applicable (no RDAP base URL known for the TLD),
//   - a result with Error == "" and usable data on success,
//   - a result with Error == "domain not found" on an authoritative 404,
//   - a result with a transient Error otherwise (bootstrap/network/parse failure).
func (r *RDAPResolver) Lookup(ctx context.Context, domain string) *models.WhoisResult {
	tld := whoisExtension(domain)
	base, err := r.baseURL(ctx, tld)
	if err != nil {
		return &models.WhoisResult{Domain: domain, Error: "rdap bootstrap: " + err.Error()}
	}
	if base == "" {
		return nil // no RDAP endpoint for this TLD
	}

	// Wait for a rate limiter token (or bail on cancellation/close).
	select {
	case <-r.rateLimiter:
	case <-r.rateLimiterDone:
		return &models.WhoisResult{Domain: domain, Error: "rdap: resolver closed"}
	case <-ctx.Done():
		return &models.WhoisResult{Domain: domain, Error: "rdap: " + ctx.Err().Error()}
	}

	start := time.Now()
	result := r.query(ctx, base, domain)
	result.LookupTimeMs = time.Since(start).Milliseconds()
	return result
}

// query performs the RDAP HTTP GET and parses the response.
func (r *RDAPResolver) query(ctx context.Context, base, domain string) *models.WhoisResult {
	result := &models.WhoisResult{Domain: domain}

	url := base + "/domain/" + domain
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		result.Error = "rdap: " + err.Error()
		return result
	}
	req.Header.Set("Accept", "application/rdap+json")

	resp, err := r.client.Do(req)
	if err != nil {
		result.Error = "rdap: " + err.Error()
		return result
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		result.Error = "domain not found"
		return result
	case resp.StatusCode != http.StatusOK:
		result.Error = fmt.Sprintf("rdap: unexpected status %d", resp.StatusCode)
		return result
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		result.Error = "rdap: " + err.Error()
		return result
	}

	var parsed rdapDomain
	if err := json.Unmarshal(body, &parsed); err != nil {
		result.Error = "rdap parse: " + err.Error()
		return result
	}

	result.Registrar = parsed.registrarName()
	if t := parsed.eventDate("expiration"); t != nil {
		result.ExpirationDate = t
	}
	if t := parsed.eventDate("registration"); t != nil {
		result.CreationDate = t
	}
	if len(parsed.Status) > 0 {
		result.Status = strings.Join(parsed.Status, ", ")
	}

	return result
}

// baseURL returns the RDAP base URL for a TLD (trailing slash trimmed), or "" when
// the TLD has no RDAP endpoint. It lazily loads and caches the IANA bootstrap.
func (r *RDAPResolver) baseURL(ctx context.Context, tld string) (string, error) {
	r.bootstrapMu.Lock()
	defer r.bootstrapMu.Unlock()

	// bootstrapNextTry gates fetch attempts (zero value = fetch immediately). On
	// failure we push it out by the cooldown so a sustained IANA outage costs one
	// timeout per cooldown window, not one per lookup — the worker is a single
	// serial consumer, so repeated full-timeout stalls would throttle everything.
	if time.Now().After(r.bootstrapNextTry) {
		if bs, err := r.fetchBootstrap(ctx); err != nil {
			r.bootstrapNextTry = time.Now().Add(r.bootstrapCooldown)
			// On failure keep using a previously-loaded (now-stale) map; if none was
			// ever loaded we fall through to the transient error below.
		} else {
			r.bootstrap = bs
			r.bootstrapFetched = true
			r.bootstrapNextTry = time.Now().Add(r.bootstrapTTL)
		}
	}

	if !r.bootstrapFetched {
		// The bootstrap has never loaded (cold start, still inside the cooldown after
		// a failure). Surface a TRANSIENT error rather than a nil-map read: returning
		// ("", nil) here would make Lookup report "no RDAP endpoint", silently turning
		// a WhoisNoServerError domain into a falsely-stable 24h-suppressed outcome.
		return "", fmt.Errorf("rdap: bootstrap unavailable")
	}

	return r.bootstrap[tld], nil
}

// fetchBootstrap downloads and parses the IANA RDAP bootstrap file.
func (r *RDAPResolver) fetchBootstrap(ctx context.Context) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.bootstrapURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}

	var doc rdapBootstrap
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}

	m := make(map[string]string)
	for _, svc := range doc.Services {
		if len(svc) < 2 {
			continue
		}
		var tlds, urls []string
		if err := json.Unmarshal(svc[0], &tlds); err != nil {
			continue
		}
		if err := json.Unmarshal(svc[1], &urls); err != nil {
			continue
		}
		base := preferredRDAPBase(urls)
		if base == "" {
			continue
		}
		for _, t := range tlds {
			m[strings.ToLower(t)] = base
		}
	}
	if len(m) == 0 {
		return nil, fmt.Errorf("empty bootstrap")
	}
	return m, nil
}

// preferredRDAPBase picks an HTTPS base URL when available (trailing slash trimmed).
func preferredRDAPBase(urls []string) string {
	var fallback string
	for _, u := range urls {
		u = strings.TrimRight(strings.TrimSpace(u), "/")
		if u == "" {
			continue
		}
		if strings.HasPrefix(u, "https://") {
			return u
		}
		if fallback == "" {
			fallback = u
		}
	}
	return fallback
}

// rdapBootstrap mirrors the top-level structure of the IANA RDAP bootstrap file.
type rdapBootstrap struct {
	Services [][]json.RawMessage `json:"services"`
}

// rdapDomain is the subset of an RDAP domain response we consume.
type rdapDomain struct {
	Entities []rdapEntity `json:"entities"`
	Events   []rdapEvent  `json:"events"`
	Status   []string     `json:"status"`
}

type rdapEntity struct {
	Roles      []string        `json:"roles"`
	VCardArray json.RawMessage `json:"vcardArray"`
	Entities   []rdapEntity    `json:"entities"`
}

type rdapEvent struct {
	EventAction string `json:"eventAction"`
	EventDate   string `json:"eventDate"`
}

// registrarName finds the registrar entity (searching one level of nesting) and
// returns its vCard formatted name.
func (d *rdapDomain) registrarName() string {
	return registrarFromEntities(d.Entities)
}

func registrarFromEntities(entities []rdapEntity) string {
	for _, e := range entities {
		for _, role := range e.Roles {
			if strings.EqualFold(role, "registrar") {
				if name := vcardFN(e.VCardArray); name != "" {
					return name
				}
			}
		}
	}
	// Recurse one level: some registries nest the registrar under another entity.
	for _, e := range entities {
		if name := registrarFromEntities(e.Entities); name != "" {
			return name
		}
	}
	return ""
}

// eventDate returns the parsed date for the given RDAP eventAction, or nil.
func (d *rdapDomain) eventDate(action string) *time.Time {
	for _, ev := range d.Events {
		if strings.EqualFold(ev.EventAction, action) && ev.EventDate != "" {
			if t, err := parseFlexibleDate(ev.EventDate); err == nil {
				return &t
			}
		}
	}
	return nil
}

// vcardFN extracts the "fn" (formatted name) property from a jCard/vCard array as
// used in RDAP. The array is ["vcard", [[name, params, type, value], ...]].
func vcardFN(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) < 2 {
		return ""
	}
	var props [][]json.RawMessage
	if err := json.Unmarshal(arr[1], &props); err != nil {
		return ""
	}
	for _, p := range props {
		if len(p) < 4 {
			continue
		}
		var name string
		if err := json.Unmarshal(p[0], &name); err != nil || !strings.EqualFold(name, "fn") {
			continue
		}
		var val string
		if err := json.Unmarshal(p[3], &val); err == nil {
			return strings.TrimSpace(val)
		}
	}
	return ""
}
