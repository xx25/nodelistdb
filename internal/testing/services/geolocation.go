package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
	
	"github.com/nodelistdb/internal/testing/models"
	"github.com/nodelistdb/internal/testing/storage"
)

// Geolocation service for IP geolocation
type Geolocation struct {
	provider        string
	apiKey          string
	cache           *GeoCache
	persistentCache *storage.GeolocationCache // Optional persistent cache
	rateLimit       *RateLimiter
	client          *http.Client
}

// GeoCache stores geolocation results
type GeoCache struct {
	mu      sync.RWMutex
	entries map[string]*GeoCacheEntry
	ttl     time.Duration
}

// GeoCacheEntry represents a cached geolocation result
type GeoCacheEntry struct {
	Result    *models.GeolocationResult
	Timestamp time.Time
}

// RateLimiter implements basic rate limiting
type RateLimiter struct {
	mu           sync.Mutex
	requestTimes []time.Time
	maxRequests  int
	window       time.Duration
}

// NewGeolocation creates a new geolocation service
func NewGeolocation(provider, apiKey string) *Geolocation {
	return &Geolocation{
		provider: provider,
		apiKey:   apiKey,
		cache: &GeoCache{
			entries: make(map[string]*GeoCacheEntry),
			ttl:     7 * 24 * time.Hour, // 7 days default TTL
		},
		rateLimit: &RateLimiter{
			maxRequests:  150,
			window:       time.Minute,
			requestTimes: make([]time.Time, 0),
		},
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetPersistentCache sets an optional persistent cache
func (g *Geolocation) SetPersistentCache(cache *storage.GeolocationCache) {
	g.persistentCache = cache
}

// NewGeolocationWithConfig creates a new geolocation service with custom config
func NewGeolocationWithConfig(provider, apiKey string, cacheTTL time.Duration, rateLimit int) *Geolocation {
	return &Geolocation{
		provider: provider,
		apiKey:   apiKey,
		cache: &GeoCache{
			entries: make(map[string]*GeoCacheEntry),
			ttl:     cacheTTL,
		},
		rateLimit: &RateLimiter{
			maxRequests:  rateLimit,
			window:       time.Minute,
			requestTimes: make([]time.Time, 0),
		},
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetLocation gets geolocation for an IP address
func (g *Geolocation) GetLocation(ctx context.Context, ip string) *models.GeolocationResult {
	// Check in-memory cache first
	if cached := g.cache.Get(ip); cached != nil {
		return cached
	}
	
	// Check persistent cache if available
	if g.persistentCache != nil {
		var result models.GeolocationResult
		if err := g.persistentCache.Get(ip, &result); err == nil {
			// Update in-memory cache and return
			g.cache.Set(ip, &result)
			return &result
		}
	}
	
	// Check rate limit
	if !g.rateLimit.Allow() {
		return nil
	}
	
	var result *models.GeolocationResult
	var err error
	
	switch g.provider {
	case "ip-api":
		result, err = g.getFromIPAPI(ctx, ip)
	case "ipinfo":
		result, err = g.getFromIPInfo(ctx, ip)
	case "ipgeolocation":
		result, err = g.getFromIPGeolocation(ctx, ip)
	default:
		result, err = g.getFromIPAPI(ctx, ip) // Default to ip-api
	}
	
	if err != nil {
		return nil
	}
	
	// Cache the result in memory
	g.cache.Set(ip, result)
	
	// Cache in persistent storage if available
	if g.persistentCache != nil {
		g.persistentCache.Set(ip, result)
	}
	
	return result
}

// getFromIPAPI gets geolocation from ip-api.com
func (g *Geolocation) getFromIPAPI(ctx context.Context, ip string) (*models.GeolocationResult, error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,countryCode,region,city,lat,lon,timezone,isp,org,as,hosting,proxy", ip)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var data struct {
		Status      string  `json:"status"`
		Country     string  `json:"country"`
		CountryCode string  `json:"countryCode"`
		Region      string  `json:"region"`
		City        string  `json:"city"`
		Lat         float32 `json:"lat"`
		Lon         float32 `json:"lon"`
		Timezone    string  `json:"timezone"`
		ISP         string  `json:"isp"`
		Org         string  `json:"org"`
		AS          string  `json:"as"`
		Hosting     bool    `json:"hosting"`
		Proxy       bool    `json:"proxy"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	
	if data.Status != "success" {
		return nil, fmt.Errorf("geolocation failed for %s", ip)
	}
	
	// Extract ASN number from AS field (format: "AS12345 Provider Name")
	var asn uint32
	if data.AS != "" {
		fmt.Sscanf(data.AS, "AS%d", &asn)
	}
	
	return &models.GeolocationResult{
		IP:          ip,
		Country:     data.Country,
		CountryCode: data.CountryCode,
		City:        data.City,
		Region:      data.Region,
		Latitude:    data.Lat,
		Longitude:   data.Lon,
		ISP:         data.ISP,
		Org:         data.Org,
		ASN:         asn,
		Timezone:    data.Timezone,
		Source:      "ip-api",
	}, nil
}

// getFromIPInfo gets geolocation from ipinfo.io
func (g *Geolocation) getFromIPInfo(ctx context.Context, ip string) (*models.GeolocationResult, error) {
	url := fmt.Sprintf("https://ipinfo.io/%s/json", ip)
	if g.apiKey != "" {
		url += "?token=" + g.apiKey
	}
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var data struct {
		IP       string `json:"ip"`
		City     string `json:"city"`
		Region   string `json:"region"`
		Country  string `json:"country"`
		Loc      string `json:"loc"`
		Org      string `json:"org"`
		Timezone string `json:"timezone"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	
	// Parse location coordinates
	var lat, lon float32
	fmt.Sscanf(data.Loc, "%f,%f", &lat, &lon)
	
	return &models.GeolocationResult{
		IP:          ip,
		Country:     data.Country,
		CountryCode: data.Country,
		City:        data.City,
		Region:      data.Region,
		Latitude:    lat,
		Longitude:   lon,
		Org:         data.Org,
		Timezone:    data.Timezone,
		Source:      "ipinfo",
	}, nil
}

// getFromIPGeolocation gets geolocation from ipgeolocation.io
func (g *Geolocation) getFromIPGeolocation(ctx context.Context, ip string) (*models.GeolocationResult, error) {
	url := fmt.Sprintf("https://api.ipgeolocation.io/ipgeo?apiKey=%s&ip=%s", g.apiKey, ip)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var data struct {
		IP           string  `json:"ip"`
		CountryName  string  `json:"country_name"`
		CountryCode2 string  `json:"country_code2"`
		StateProv    string  `json:"state_prov"`
		City         string  `json:"city"`
		Latitude     string  `json:"latitude"`
		Longitude    string  `json:"longitude"`
		ISP          string  `json:"isp"`
		Organization string  `json:"organization"`
		TimeZone     struct {
			Name string `json:"name"`
		} `json:"time_zone"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	
	// Convert string coordinates to float32
	var lat, lon float32
	fmt.Sscanf(data.Latitude, "%f", &lat)
	fmt.Sscanf(data.Longitude, "%f", &lon)
	
	return &models.GeolocationResult{
		IP:          ip,
		Country:     data.CountryName,
		CountryCode: data.CountryCode2,
		City:        data.City,
		Region:      data.StateProv,
		Latitude:    lat,
		Longitude:   lon,
		ISP:         data.ISP,
		Org:         data.Organization,
		Timezone:    data.TimeZone.Name,
		Source:      "ipgeolocation",
	}, nil
}

// Get retrieves a cached geolocation result
func (c *GeoCache) Get(ip string) *models.GeolocationResult {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.entries[ip]
	if !exists {
		return nil
	}
	
	// Check if entry is expired
	if time.Since(entry.Timestamp) > c.ttl {
		return nil
	}
	
	return entry.Result
}

// Set stores a geolocation result in cache
func (c *GeoCache) Set(ip string, result *models.GeolocationResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.entries[ip] = &GeoCacheEntry{
		Result:    result,
		Timestamp: time.Now(),
	}
}

// Allow checks if a request is allowed based on rate limit
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	now := time.Now()
	
	// Remove old requests outside the window
	cutoff := now.Add(-r.window)
	i := 0
	for i < len(r.requestTimes) && r.requestTimes[i].Before(cutoff) {
		i++
	}
	r.requestTimes = r.requestTimes[i:]
	
	// Check if we're at the limit
	if len(r.requestTimes) >= r.maxRequests {
		return false
	}
	
	// Add current request
	r.requestTimes = append(r.requestTimes, now)
	return true
}