package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/likexian/whois"
	whoisparser "github.com/likexian/whois-parser"
	"golang.org/x/sync/singleflight"

	"github.com/nodelistdb/internal/testing/models"
)

// PersistentWhoisCache is a read-only callback interface for persistent WHOIS cache.
// The worker is responsible for writing results to persistent storage.
type PersistentWhoisCache interface {
	GetWithContext(ctx context.Context, domain string) (*models.WhoisResult, error)
}

// whoisClient is the subset of *whois.Client the resolver uses. Extracting it as an
// interface lets tests stub referral-failure and total-failure responses.
type whoisClient interface {
	Whois(domain string, servers ...string) (string, error)
}

// rdapFallback is the subset of *RDAPResolver the resolver uses (nil-able, for tests).
type rdapFallback interface {
	Lookup(ctx context.Context, domain string) *models.WhoisResult
}

// errNoWhoisServer is returned by queryWhois when IANA publishes no port-43 WHOIS
// server for the domain's TLD. Distinct from a transient IANA failure.
var errNoWhoisServer = errors.New(models.WhoisNoServerError)

// WhoisResolver handles WHOIS lookups with in-memory caching, singleflight dedup,
// rate limiting, a per-TLD server cache, and an optional RDAP fallback.
type WhoisResolver struct {
	timeout         time.Duration
	client          whoisClient
	cache           sync.Map // domain → *whoisCacheEntry
	sfGroup         singleflight.Group
	rateLimiter     chan struct{}
	rateLimiterDone chan struct{} // closed to stop the rate limiter goroutine
	closeOnce       sync.Once
	persistentCache PersistentWhoisCache
	rdap            rdapFallback

	tldServers   sync.Map // tld → *tldServerEntry (per-TLD WHOIS server cache)
	tldServerTTL time.Duration
}

type whoisCacheEntry struct {
	result *models.WhoisResult
	expiry time.Time
}

// tldServerEntry caches the WHOIS server for a TLD. An empty server means IANA
// authoritatively reported no WHOIS server (RDAP-only TLD).
type tldServerEntry struct {
	server string
	expiry time.Time
}

// NewWhoisResolver creates a new WHOIS resolver with rate limiting
func NewWhoisResolver(timeout time.Duration) *WhoisResolver {
	client := whois.NewClient()
	client.SetTimeout(timeout)

	r := &WhoisResolver{
		timeout:         timeout,
		client:          client,
		rateLimiter:     make(chan struct{}, 1),
		rateLimiterDone: make(chan struct{}),
		tldServerTTL:    24 * time.Hour,
	}

	// Start rate limiter: allows one lookup per second
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
	// Seed the first token immediately
	r.rateLimiter <- struct{}{}

	return r
}

// Close stops the rate limiter goroutine. Safe to call multiple times.
func (r *WhoisResolver) Close() {
	r.closeOnce.Do(func() {
		close(r.rateLimiterDone)
	})
}

// SetPersistentCache sets an optional persistent cache for WHOIS results (read-only)
func (r *WhoisResolver) SetPersistentCache(cache PersistentWhoisCache) {
	r.persistentCache = cache
}

// SetRDAPResolver sets an optional RDAP fallback used when WHOIS yields no server
// or unusable stub data. Pass nil to disable RDAP.
func (r *WhoisResolver) SetRDAPResolver(rdap *RDAPResolver) {
	if rdap == nil {
		r.rdap = nil
		return
	}
	r.rdap = rdap
}

// Resolve looks up WHOIS data for a domain with caching and dedup
func (r *WhoisResolver) Resolve(ctx context.Context, domain string) *models.WhoisResult {
	// 1. Check in-memory cache
	if entry, ok := r.cache.Load(domain); ok {
		ce := entry.(*whoisCacheEntry)
		if time.Now().Before(ce.expiry) {
			result := *ce.result
			result.Cached = true
			return &result
		}
		// Expired — remove
		r.cache.Delete(domain)
	}

	// 2. Singleflight dedup — only one concurrent lookup per domain
	val, _, _ := r.sfGroup.Do(domain, func() (interface{}, error) {
		return r.doResolve(ctx, domain), nil
	})

	result := val.(*models.WhoisResult)
	return result
}

// doResolve performs the actual lookup (WHOIS, then RDAP fallback) within singleflight
func (r *WhoisResolver) doResolve(ctx context.Context, domain string) *models.WhoisResult {
	// Check persistent cache (read-only, respects context for cancellation)
	if r.persistentCache != nil {
		if cached, err := r.persistentCache.GetWithContext(ctx, domain); err == nil && cached != nil {
			// Only trust successes and stable "not found" from persistent cache.
			// Transient errors fall through to a fresh lookup.
			if cached.Error == "" || cached.Error == "domain not found" {
				r.cacheResult(domain, cached)
				cached.Cached = true
				return cached
			}
		}
	}

	// Wait for rate limiter token (also unblock if resolver is closed)
	select {
	case <-r.rateLimiter:
		// Got token, proceed
	case <-r.rateLimiterDone:
		return &models.WhoisResult{
			Domain: domain,
			Error:  "resolver closed while waiting for rate limiter",
		}
	case <-ctx.Done():
		return &models.WhoisResult{
			Domain: domain,
			Error:  "context cancelled while waiting for rate limiter",
		}
	}

	// Perform WHOIS lookup with timeout
	start := time.Now()
	result := r.lookupWhois(domain)
	result.LookupTimeMs = time.Since(start).Milliseconds()

	// RDAP fallback: only when WHOIS produced nothing usable and did not
	// authoritatively resolve the domain as not-found.
	if r.rdap != nil && shouldTryRDAP(result) {
		if merged := r.tryRDAP(ctx, domain, result); merged != nil {
			result = merged
		}
	}

	// Cache the result in memory only (worker handles persistent storage)
	r.cacheResult(domain, result)

	return result
}

// shouldTryRDAP reports whether RDAP should be attempted given a WHOIS result.
func shouldTryRDAP(r *models.WhoisResult) bool {
	if r.Error == "domain not found" {
		return false // WHOIS authoritatively resolved it
	}
	return !r.HasUsableData()
}

// tryRDAP runs the RDAP fallback and reconciles it with the WHOIS result. Returns
// the result to use, or nil to keep the WHOIS result unchanged.
func (r *WhoisResolver) tryRDAP(ctx context.Context, domain string, whoisResult *models.WhoisResult) *models.WhoisResult {
	rd := r.rdap.Lookup(ctx, domain)
	if rd == nil {
		return nil // RDAP not applicable for this TLD
	}
	switch {
	case rd.Error == "":
		// RDAP responded authoritatively. Return it as-is (data or not) and let the
		// worker classify: usable data → persist+seen; empty (e.g. GDPR-redacted) →
		// seen only. This mirrors the WHOIS-only empty-response path — two sources
		// agreeing "no data" must not be treated as LESS stable than one.
		rd.LookupTimeMs += whoisResult.LookupTimeMs
		return rd
	case rd.Error == "domain not found":
		return &models.WhoisResult{Domain: domain, Error: "domain not found", LookupTimeMs: whoisResult.LookupTimeMs + rd.LookupTimeMs}
	default:
		// RDAP failed transiently. If WHOIS had no stable answer either, surface a
		// transient error so we retry next cycle rather than marking the domain seen
		// for a full day on a fluke.
		if whoisResult.Error == "" || whoisResult.Error == models.WhoisNoServerError {
			return &models.WhoisResult{
				Domain:       domain,
				Error:        "whois/rdap lookup failed: " + rd.Error,
				LookupTimeMs: whoisResult.LookupTimeMs + rd.LookupTimeMs,
			}
		}
		return nil // keep WHOIS's own transient error
	}
}

// lookupWhois performs the raw WHOIS lookup and parsing, rescuing usable registry
// data even when the referral leg fails.
func (r *WhoisResolver) lookupWhois(domain string) *models.WhoisResult {
	result := &models.WhoisResult{
		Domain: domain,
	}

	rawWhois, lookupErr := r.queryWhois(domain)

	// A non-nil error with an empty body is a real failure of the primary query.
	// A non-nil error WITH a body means the referral (or a mid-stream close) failed
	// but the registry section is present — parse it, then gate on completeness.
	if lookupErr != nil && strings.TrimSpace(rawWhois) == "" {
		if errors.Is(lookupErr, errNoWhoisServer) {
			result.Error = models.WhoisNoServerError // stable: no WHOIS server for TLD
		} else {
			result.Error = fmt.Sprintf("WHOIS lookup failed: %v", lookupErr)
		}
		return result
	}

	parsed, parseErr := whoisparser.Parse(rawWhois)
	if parseErr != nil {
		// Do not trust a "not found" classification when the lookup itself errored:
		// a rate-limit or referral stub can trip whois-parser's not-found heuristics.
		if lookupErr == nil && errors.Is(parseErr, whoisparser.ErrNotFoundDomain) {
			result.Error = "domain not found"
		} else {
			result.Error = fmt.Sprintf("WHOIS parse failed: %v", parseErr)
		}
		return result
	}

	// Extract expiration date
	if parsed.Domain.ExpirationDateInTime != nil {
		t := *parsed.Domain.ExpirationDateInTime
		result.ExpirationDate = &t
	} else if parsed.Domain.ExpirationDate != "" {
		if t, err := parseFlexibleDate(parsed.Domain.ExpirationDate); err == nil {
			result.ExpirationDate = &t
		}
	}

	// Extract creation date
	if parsed.Domain.CreatedDateInTime != nil {
		t := *parsed.Domain.CreatedDateInTime
		result.CreationDate = &t
	} else if parsed.Domain.CreatedDate != "" {
		if t, err := parseFlexibleDate(parsed.Domain.CreatedDate); err == nil {
			result.CreationDate = &t
		}
	}

	// Registrar and status
	if parsed.Registrar != nil {
		result.Registrar = parsed.Registrar.Name
	}
	if len(parsed.Domain.Status) > 0 {
		result.Status = strings.Join(parsed.Domain.Status, ", ")
	}

	// Rescue gate: if the lookup errored (referral/short-read failure), only accept
	// the parsed data when it is genuinely complete — a registrar name or an
	// expiration date. A status-only rescue is treated as transient so we don't
	// persist a thin stub in place of a real answer.
	if lookupErr != nil && result.Registrar == "" && result.ExpirationDate == nil {
		return &models.WhoisResult{
			Domain: domain,
			Error:  fmt.Sprintf("WHOIS lookup failed: %v", lookupErr),
		}
	}

	return result
}

// queryWhois performs the WHOIS network query, using a per-TLD server cache to avoid
// re-querying IANA for every domain. Returns errNoWhoisServer (stable) when IANA has
// no WHOIS server for the TLD, or a transient error when IANA itself fails.
func (r *WhoisResolver) queryWhois(domain string) (string, error) {
	tld := whoisExtension(domain)
	if tld == "" {
		return safeWhois(r.client, domain) // fall back to library default routing
	}

	// Fast path: cached TLD server (positive or negative).
	if entry, ok := r.loadTLDServer(tld); ok {
		if entry.server == "" {
			return "", errNoWhoisServer
		}
		return safeWhois(r.client, domain, entry.server)
	}

	// Resolve the TLD's WHOIS server via IANA once, then cache it. A bare-TLD query
	// hits IANA directly (no referral leg), so ANY non-nil error — even one returned
	// alongside a partial body — is a failed read: treat it as transient and do NOT
	// cache a negative, or we would poison the whole TLD for 24h on a fluke.
	ianaRecord, ianaErr := safeWhois(r.client, tld)
	if ianaErr != nil {
		return "", fmt.Errorf("whois: query for TLD server failed: %w", ianaErr)
	}

	server := extractWhoisServer(ianaRecord)
	r.storeTLDServer(tld, server)
	if server == "" {
		return "", errNoWhoisServer
	}
	return safeWhois(r.client, domain, server)
}

func (r *WhoisResolver) loadTLDServer(tld string) (*tldServerEntry, bool) {
	v, ok := r.tldServers.Load(tld)
	if !ok {
		return nil, false
	}
	entry := v.(*tldServerEntry)
	if time.Now().After(entry.expiry) {
		r.tldServers.Delete(tld)
		return nil, false
	}
	return entry, true
}

func (r *WhoisResolver) storeTLDServer(tld, server string) {
	r.tldServers.Store(tld, &tldServerEntry{
		server: server,
		expiry: time.Now().Add(r.tldServerTTL),
	})
}

// InvalidateCache drops the in-memory cache entry for a domain so the next Resolve
// performs a fresh lookup. The worker calls this when a ClickHouse write fails, so a
// good result is not stranded (unpersisted) behind the resolver's own success cache.
func (r *WhoisResolver) InvalidateCache(domain string) {
	r.cache.Delete(domain)
}

// cacheResult stores a result in the in-memory cache with appropriate TTL
func (r *WhoisResolver) cacheResult(domain string, result *models.WhoisResult) {
	ttl := 24 * time.Hour
	if result.Error != "" {
		switch result.Error {
		case "domain not found", models.WhoisNoServerError:
			ttl = 24 * time.Hour // stable outcomes
		default:
			ttl = 1 * time.Hour // transient errors — retry sooner
		}
	}

	r.cache.Store(domain, &whoisCacheEntry{
		result: result,
		expiry: time.Now().Add(ttl),
	})
}

// safeWhois calls the WHOIS client, converting a panic into an error. The vendored
// library's internal getServer does unchecked slice indexing that can panic on a
// referral line with no trailing newline; nothing else in the worker call chain
// recovers, so a single malformed registry response could otherwise crash the daemon.
func safeWhois(client whoisClient, domain string, servers ...string) (raw string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			raw = ""
			err = fmt.Errorf("whois: recovered panic: %v", rec)
		}
	}()
	return client.Whois(domain, servers...)
}

// whoisExtension returns the TLD used to route a WHOIS query: the last dotted label
// (mirrors github.com/likexian/whois getExtension). Returns "" for IPs/empty input.
func whoisExtension(domain string) string {
	domain = strings.TrimSpace(domain)
	// Strip a trailing :port (inputs are normally clean registrable domains).
	if h, _, err := net.SplitHostPort(domain); err == nil {
		domain = h
	}
	domain = strings.ToLower(strings.Trim(strings.Trim(domain, "[]"), "."))
	if domain == "" || net.ParseIP(domain) != nil {
		return ""
	}
	if i := strings.IndexByte(domain, '/'); i >= 0 {
		domain = domain[:i]
	}
	labels := strings.Split(domain, ".")
	return labels[len(labels)-1]
}

// extractWhoisServer pulls the WHOIS server hostname from an IANA TLD record. It is
// line-based and panic-safe (unlike the vendored library's index-math getServer).
// Returns "" when the record has no (or an empty) whois:/refer: line.
func extractWhoisServer(ianaRecord string) string {
	for _, line := range strings.Split(ianaRecord, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "whois:") {
			if v := strings.TrimSpace(line[len("whois:"):]); v != "" {
				return v
			}
		}
	}
	return ""
}

// parseFlexibleDate tries multiple date formats common in WHOIS/RDAP responses
var flexibleDateFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"02-Jan-2006",
	"January 02 2006",
	"2006/01/02",
}

func parseFlexibleDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, format := range flexibleDateFormats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", s)
}
