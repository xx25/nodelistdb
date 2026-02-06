package services

import (
	"context"
	"errors"
	"fmt"
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
	Get(domain string) (*models.WhoisResult, error)
}

// WhoisResolver handles WHOIS lookups with in-memory caching, singleflight dedup, and rate limiting
type WhoisResolver struct {
	timeout         time.Duration
	client          *whois.Client
	cache           sync.Map // domain → *whoisCacheEntry
	sfGroup         singleflight.Group
	rateLimiter     chan struct{}
	rateLimiterDone chan struct{} // closed to stop the rate limiter goroutine
	persistentCache PersistentWhoisCache
}

type whoisCacheEntry struct {
	result *models.WhoisResult
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

// Close stops the rate limiter goroutine
func (r *WhoisResolver) Close() {
	close(r.rateLimiterDone)
}

// SetPersistentCache sets an optional persistent cache for WHOIS results (read-only)
func (r *WhoisResolver) SetPersistentCache(cache PersistentWhoisCache) {
	r.persistentCache = cache
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

// doResolve performs the actual WHOIS lookup (within singleflight)
func (r *WhoisResolver) doResolve(ctx context.Context, domain string) *models.WhoisResult {
	// Check persistent cache (read-only)
	if r.persistentCache != nil {
		if cached, err := r.persistentCache.Get(domain); err == nil && cached != nil {
			// Use shorter freshness for errors vs successes
			if cached.Error != "" && cached.Error != "domain not found" {
				// Don't trust persistent transient errors — do a fresh lookup
			} else {
				// Store in in-memory cache too
				r.cacheResult(domain, cached)
				cached.Cached = true
				return cached
			}
		}
	}

	// Wait for rate limiter token
	select {
	case <-r.rateLimiter:
		// Got token, proceed
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

	// Cache the result in memory only (worker handles persistent storage)
	r.cacheResult(domain, result)

	return result
}

// lookupWhois performs the raw WHOIS lookup and parsing
func (r *WhoisResolver) lookupWhois(domain string) *models.WhoisResult {
	result := &models.WhoisResult{
		Domain: domain,
	}

	// Perform WHOIS query using configured client with timeout
	rawWhois, err := r.client.Whois(domain)
	if err != nil {
		result.Error = fmt.Sprintf("WHOIS lookup failed: %v", err)
		return result
	}

	// Parse WHOIS response
	parsed, err := whoisparser.Parse(rawWhois)
	if err != nil {
		if errors.Is(err, whoisparser.ErrNotFoundDomain) {
			result.Error = "domain not found"
		} else {
			result.Error = fmt.Sprintf("WHOIS parse failed: %v", err)
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
	result.Registrar = parsed.Registrar.Name
	if len(parsed.Domain.Status) > 0 {
		result.Status = strings.Join(parsed.Domain.Status, ", ")
	}

	return result
}

// cacheResult stores a result in the in-memory cache with appropriate TTL
func (r *WhoisResolver) cacheResult(domain string, result *models.WhoisResult) {
	ttl := 24 * time.Hour
	if result.Error != "" {
		if result.Error == "domain not found" {
			ttl = 24 * time.Hour // Not found is stable
		} else {
			ttl = 1 * time.Hour // Transient errors — retry sooner
		}
	}

	r.cache.Store(domain, &whoisCacheEntry{
		result: result,
		expiry: time.Now().Add(ttl),
	})
}

// parseFlexibleDate tries multiple date formats common in WHOIS responses
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
