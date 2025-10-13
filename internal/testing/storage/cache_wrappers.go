package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/cache"
)

// DNSCache wraps unified cache for DNS-specific operations
type DNSCache struct {
	cache cache.Cache
	ttl   time.Duration
}

// NewDNSCache creates a new DNS cache using unified cache interface
func NewDNSCache(c cache.Cache) *DNSCache {
	return &DNSCache{
		cache: c,
		ttl:   24 * time.Hour, // Default 24 hour TTL
	}
}

// Set stores a DNS result
func (d *DNSCache) Set(hostname string, result interface{}) error {
	if d.cache == nil {
		return nil // Cache disabled
	}

	key := fmt.Sprintf("dns:%s", hostname)
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal DNS result: %w", err)
	}

	return d.cache.Set(context.Background(), key, data, d.ttl)
}

// Get retrieves a DNS result
func (d *DNSCache) Get(hostname string, dest interface{}) error {
	if d.cache == nil {
		return fmt.Errorf("cache disabled")
	}

	key := fmt.Sprintf("dns:%s", hostname)
	data, err := d.cache.Get(context.Background(), key)
	if err != nil {
		if cache.IsKeyNotFound(err) {
			return fmt.Errorf("key not found: %s", hostname)
		}
		return err
	}

	return json.Unmarshal(data, dest)
}

// GeolocationCache wraps unified cache for geolocation-specific operations
type GeolocationCache struct {
	cache cache.Cache
	ttl   time.Duration
}

// NewGeolocationCache creates a new geolocation cache using unified cache interface
func NewGeolocationCache(c cache.Cache) *GeolocationCache {
	return &GeolocationCache{
		cache: c,
		ttl:   7 * 24 * time.Hour, // Default 7 day TTL
	}
}

// Set stores a geolocation result
func (g *GeolocationCache) Set(ip string, result interface{}) error {
	if g.cache == nil {
		return nil // Cache disabled
	}

	key := fmt.Sprintf("geo:%s", ip)
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal geolocation result: %w", err)
	}

	return g.cache.Set(context.Background(), key, data, g.ttl)
}

// Get retrieves a geolocation result
func (g *GeolocationCache) Get(ip string, dest interface{}) error {
	if g.cache == nil {
		return fmt.Errorf("cache disabled")
	}

	key := fmt.Sprintf("geo:%s", ip)
	data, err := g.cache.Get(context.Background(), key)
	if err != nil {
		if cache.IsKeyNotFound(err) {
			return fmt.Errorf("key not found: %s", ip)
		}
		return err
	}

	return json.Unmarshal(data, dest)
}
