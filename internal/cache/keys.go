package cache

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type KeyGenerator struct {
	Prefix string
}

// NewKeyGenerator creates a new key generator with the given prefix
func NewKeyGenerator(prefix string) *KeyGenerator {
	if prefix == "" {
		prefix = "ndb"
	}
	return &KeyGenerator{Prefix: prefix}
}

// Node-specific keys
func (kg *KeyGenerator) NodeKey(zone, net, node int) string {
	return fmt.Sprintf("%s:node:%d:%d:%d", kg.Prefix, zone, net, node)
}

func (kg *KeyGenerator) NodeHistoryKey(zone, net, node int) string {
	return fmt.Sprintf("%s:history:%d:%d:%d", kg.Prefix, zone, net, node)
}

func (kg *KeyGenerator) NodeChangesKey(zone, net, node int, filterHash string) string {
	return fmt.Sprintf("%s:changes:%d:%d:%d:%s", kg.Prefix, zone, net, node, filterHash)
}

// Search keys with hash of filter
func (kg *KeyGenerator) SearchKey(filter interface{}) string {
	// Use JSON marshaling for consistent serialization
	// This handles pointers properly and ensures deterministic output
	jsonBytes, err := json.Marshal(filter)
	if err != nil {
		// Fallback to string representation if JSON fails
		filterStr := fmt.Sprintf("%+v", filter)
		hash := md5.Sum([]byte(filterStr))
		return fmt.Sprintf("%s:search:%s", kg.Prefix, hex.EncodeToString(hash[:]))
	}
	hash := md5.Sum(jsonBytes)
	return fmt.Sprintf("%s:search:%s", kg.Prefix, hex.EncodeToString(hash[:]))
}

// Stats keys
func (kg *KeyGenerator) StatsKey(date time.Time) string {
	return fmt.Sprintf("%s:stats:%s", kg.Prefix, date.Format("2006-01-02"))
}

func (kg *KeyGenerator) LatestStatsDateKey() string {
	return fmt.Sprintf("%s:stats:latest", kg.Prefix)
}

// Sysop keys
func (kg *KeyGenerator) UniqueSysopsKey(nameFilter string, limit, offset int) string {
	filterHash := ""
	if nameFilter != "" {
		hash := md5.Sum([]byte(nameFilter))
		filterHash = hex.EncodeToString(hash[:8]) // Use shorter hash for readability
	}
	return fmt.Sprintf("%s:sysops:%s:%d:%d", kg.Prefix, filterHash, limit, offset)
}

func (kg *KeyGenerator) NodesBySysopKey(sysopName string, limit int) string {
	hash := md5.Sum([]byte(sysopName))
	return fmt.Sprintf("%s:bysysop:%s:%d", kg.Prefix, hex.EncodeToString(hash[:8]), limit)
}

// Date keys
func (kg *KeyGenerator) AvailableDatesKey() string {
	return fmt.Sprintf("%s:dates:available", kg.Prefix)
}

func (kg *KeyGenerator) NearestDateKey(date time.Time) string {
	return fmt.Sprintf("%s:dates:nearest:%s", kg.Prefix, date.Format("2006-01-02"))
}

// Prefix generation for bulk invalidation. Each one is a literal key prefix
// for DeleteByPrefix, terminated on the ":" delimiter so it cannot reach into
// a neighbouring namespace.
//
// There is deliberately no per-node prefix. Node-scoped entries are spread
// across three namespaces that do not share one ("node:"/"history:"/"changes:"
// here, plus "analytics:modem:detail:" and several "reachability:" families),
// so no single prefix covers a node. See the note on InvalidateAll in
// internal/storage/cached_storage_invalidation.go.
func (kg *KeyGenerator) AllPrefix() string {
	return fmt.Sprintf("%s:", kg.Prefix)
}

func (kg *KeyGenerator) StatsPrefix() string {
	return fmt.Sprintf("%s:stats:", kg.Prefix)
}

func (kg *KeyGenerator) SearchPrefix() string {
	return fmt.Sprintf("%s:search:", kg.Prefix)
}

func (kg *KeyGenerator) SysopsPrefix() string {
	return fmt.Sprintf("%s:sysops:", kg.Prefix)
}

func (kg *KeyGenerator) DatesPrefix() string {
	return fmt.Sprintf("%s:dates:", kg.Prefix)
}

func (kg *KeyGenerator) ReachabilityTrendsKey(days int, domain string) string {
	return fmt.Sprintf("%s:reachability:trends:%d:%s", kg.Prefix, days, domain)
}

func (kg *KeyGenerator) SearchNodesByReachabilityKey(operational bool, limit, days int, domain string) string {
	return fmt.Sprintf("%s:reachability:search:%t:%d:%d:%s", kg.Prefix, operational, limit, days, domain)
}

func (kg *KeyGenerator) NodeTestHistoryKey(zone, net, node, days int) string {
	return fmt.Sprintf("%s:reachability:history:%d:%d:%d:%d", kg.Prefix, zone, net, node, days)
}

func (kg *KeyGenerator) NodeReachabilityStatsKey(zone, net, node, days int) string {
	// v2: the counters are per test cycle rather than per stored row. Cached v1
	// entries hold the old inflated totals and would outlive the fix by their
	// whole TTL, on the very page the change was made for.
	return fmt.Sprintf("%s:reachability:stats:v2:%d:%d:%d:%d", kg.Prefix, zone, net, node, days)
}

func (kg *KeyGenerator) DetailedTestResultKey(zone, net, node int, testTime string) string {
	return fmt.Sprintf("%s:reachability:detail:%d:%d:%d:%s", kg.Prefix, zone, net, node, testTime)
}

func (kg *KeyGenerator) AnalyticsPrefix() string {
	return fmt.Sprintf("%s:analytics:", kg.Prefix)
}

// Helper function to create a filter hash
func (kg *KeyGenerator) HashFilter(filter interface{}) string {
	// Use JSON marshaling for consistent serialization
	jsonBytes, err := json.Marshal(filter)
	if err != nil {
		// Fallback to string representation if JSON fails
		filterStr := fmt.Sprintf("%+v", filter)
		hash := md5.Sum([]byte(filterStr))
		return hex.EncodeToString(hash[:])
	}
	hash := md5.Sum(jsonBytes)
	return hex.EncodeToString(hash[:])
}

// Helper function to create a short hash (for readable keys)
func (kg *KeyGenerator) ShortHash(data string) string {
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// ValidateKey checks if a key follows the expected format
func (kg *KeyGenerator) ValidateKey(key string) bool {
	return strings.HasPrefix(key, kg.Prefix+":")
}

// ExtractNodeAddress extracts zone, net, node from a node-related key
func (kg *KeyGenerator) ExtractNodeAddress(key string) (zone, net, node int, ok bool) {
	parts := strings.Split(key, ":")
	if len(parts) < 5 {
		return 0, 0, 0, false
	}

	// Try to parse the address components
	_, err1 := fmt.Sscanf(parts[2], "%d", &zone)
	_, err2 := fmt.Sscanf(parts[3], "%d", &net)
	_, err3 := fmt.Sscanf(parts[4], "%d", &node)

	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}

	return zone, net, node, true
}
