package services

import (
	"context"
	"net"
	"sync"
	"time"
	
	"github.com/nodelistdb/internal/testing/models"
	"github.com/nodelistdb/internal/testing/storage"
)

// DNSResolver handles DNS resolution with caching
type DNSResolver struct {
	workers        int
	timeout        time.Duration
	cache          *DNSCache
	persistentCache *storage.DNSCache // Optional persistent cache
}

// DNSCache stores DNS resolution results
type DNSCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	ttl     time.Duration
}

// CacheEntry represents a cached DNS result
type CacheEntry struct {
	Result    *models.DNSResult
	Timestamp time.Time
}

// NewDNSResolver creates a new DNS resolver
func NewDNSResolver(workers int, timeout time.Duration) *DNSResolver {
	return &DNSResolver{
		workers: workers,
		timeout: timeout,
		cache: &DNSCache{
			entries: make(map[string]*CacheEntry),
			ttl:     24 * time.Hour, // Default 24 hour TTL
		},
	}
}

// SetPersistentCache sets an optional persistent cache
func (r *DNSResolver) SetPersistentCache(cache *storage.DNSCache) {
	r.persistentCache = cache
}

// NewDNSResolverWithTTL creates a new DNS resolver with custom TTL
func NewDNSResolverWithTTL(workers int, timeout time.Duration, cacheTTL time.Duration) *DNSResolver {
	return &DNSResolver{
		workers: workers,
		timeout: timeout,
		cache: &DNSCache{
			entries: make(map[string]*CacheEntry),
			ttl:     cacheTTL,
		},
	}
}

// Resolve resolves a hostname to IP addresses
func (r *DNSResolver) Resolve(ctx context.Context, hostname string) *models.DNSResult {
	// Check in-memory cache first
	if cached := r.cache.Get(hostname); cached != nil {
		return cached
	}
	
	// Check persistent cache if available
	if r.persistentCache != nil {
		var result models.DNSResult
		if err := r.persistentCache.Get(hostname, &result); err == nil {
			// Update in-memory cache and return
			r.cache.Set(hostname, &result)
			return &result
		}
	}
	
	startTime := time.Now()
	result := &models.DNSResult{
		Hostname: hostname,
	}
	
	// Set timeout for resolution
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	
	// Resolve IPv4
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		result.Error = err
		result.ResolutionMs = time.Since(startTime).Milliseconds()
		return result
	}
	
	// Separate IPv4 and IPv6 addresses
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			result.IPv4Addresses = append(result.IPv4Addresses, ip.IP.String())
		} else if ip.IP.To16() != nil {
			result.IPv6Addresses = append(result.IPv6Addresses, ip.IP.String())
		}
	}
	
	result.ResolutionMs = time.Since(startTime).Milliseconds()
	
	// Cache the result in memory
	r.cache.Set(hostname, result)
	
	// Cache in persistent storage if available
	if r.persistentCache != nil {
		r.persistentCache.Set(hostname, result)
	}
	
	return result
}

// ResolveBatch resolves multiple hostnames concurrently
func (r *DNSResolver) ResolveBatch(ctx context.Context, hostnames []string) map[string]*models.DNSResult {
	results := make(map[string]*models.DNSResult)
	var mu sync.Mutex
	
	// Create worker pool
	work := make(chan string, len(hostnames))
	var wg sync.WaitGroup
	
	// Start workers
	for i := 0; i < r.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for hostname := range work {
				result := r.Resolve(ctx, hostname)
				mu.Lock()
				results[hostname] = result
				mu.Unlock()
			}
		}()
	}
	
	// Submit work
	for _, hostname := range hostnames {
		work <- hostname
	}
	close(work)
	
	// Wait for completion
	wg.Wait()
	
	return results
}

// Get retrieves a cached DNS result
func (c *DNSCache) Get(hostname string) *models.DNSResult {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.entries[hostname]
	if !exists {
		return nil
	}
	
	// Check if entry is expired
	if time.Since(entry.Timestamp) > c.ttl {
		return nil
	}
	
	return entry.Result
}

// Set stores a DNS result in cache
func (c *DNSCache) Set(hostname string, result *models.DNSResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.entries[hostname] = &CacheEntry{
		Result:    result,
		Timestamp: time.Now(),
	}
}

// Clean removes expired entries from cache
func (c *DNSCache) Clean() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	for hostname, entry := range c.entries {
		if now.Sub(entry.Timestamp) > c.ttl {
			delete(c.entries, hostname)
		}
	}
}