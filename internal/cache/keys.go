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

// Pattern generation for bulk invalidation
func (kg *KeyGenerator) NodePattern(zone, net, node int) string {
	return fmt.Sprintf("%s:*:%d:%d:%d*", kg.Prefix, zone, net, node)
}

func (kg *KeyGenerator) AllPattern() string {
	return fmt.Sprintf("%s:*", kg.Prefix)
}

func (kg *KeyGenerator) StatsPattern() string {
	return fmt.Sprintf("%s:stats:*", kg.Prefix)
}

func (kg *KeyGenerator) SearchPattern() string {
	return fmt.Sprintf("%s:search:*", kg.Prefix)
}

func (kg *KeyGenerator) SysopsPattern() string {
	return fmt.Sprintf("%s:sysops:*", kg.Prefix)
}

func (kg *KeyGenerator) DatesPattern() string {
	return fmt.Sprintf("%s:dates:*", kg.Prefix)
}

// Analytics keys
func (kg *KeyGenerator) FlagFirstAppearanceKey(flagName string) string {
	return fmt.Sprintf("%s:analytics:flag:first:%s", kg.Prefix, flagName)
}

func (kg *KeyGenerator) FlagUsageByYearKey(flagName string) string {
	return fmt.Sprintf("%s:analytics:flag:usage:%s", kg.Prefix, flagName)
}

func (kg *KeyGenerator) NetworkHistoryKey(zone, net int) string {
	return fmt.Sprintf("%s:analytics:network:%d:%d", kg.Prefix, zone, net)
}

// Test result analytics keys
func (kg *KeyGenerator) IPv6EnabledNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:ipv6:enabled:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) IPv6NonWorkingNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:ipv6:nonworking:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) IPv6AdvertisedIPv4OnlyNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:ipv6:ipv4only:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) BinkPEnabledNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:binkp:enabled:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) IfcicoEnabledNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:ifcico:enabled:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) TelnetEnabledNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:telnet:enabled:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) VModemEnabledNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:vmodem:enabled:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) FTPEnabledNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:ftp:enabled:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) BinkPSoftwareDistributionKey(days int) string {
	return fmt.Sprintf("%s:analytics:binkp:software:%d", kg.Prefix, days)
}

func (kg *KeyGenerator) IFCICOSoftwareDistributionKey(days int) string {
	return fmt.Sprintf("%s:analytics:ifcico:software:%d", kg.Prefix, days)
}

func (kg *KeyGenerator) BinkdDetailedStatsKey(days int) string {
	return fmt.Sprintf("%s:analytics:binkd:stats:%d", kg.Prefix, days)
}

func (kg *KeyGenerator) IPv6WeeklyNewsKey(limit int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:ipv6:weeklynews:%d:%t", kg.Prefix, limit, includeZeroNodes)
}

func (kg *KeyGenerator) IPv6OnlyNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:ipv6:only:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) PureIPv6OnlyNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:ipv6:pureonly:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) IPv6NodeListKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:ipv6:nodelist:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) GeoHostingDistributionKey(days int) string {
	return fmt.Sprintf("%s:analytics:geo:hosting:%d", kg.Prefix, days)
}

func (kg *KeyGenerator) NodesByCountryKey(countryCode string, days int) string {
	return fmt.Sprintf("%s:analytics:geo:country:%s:%d", kg.Prefix, countryCode, days)
}

func (kg *KeyGenerator) NodesByProviderKey(provider string, days int) string {
	hash := md5.Sum([]byte(provider))
	return fmt.Sprintf("%s:analytics:geo:provider:%s:%d", kg.Prefix, hex.EncodeToString(hash[:8]), days)
}

func (kg *KeyGenerator) OnThisDayNodesKey(month, day, limit int, activeOnly bool) string {
	return fmt.Sprintf("%s:analytics:onthisday:%d:%d:%d:%t", kg.Prefix, month, day, limit, activeOnly)
}

func (kg *KeyGenerator) PioneersByRegionKey(zone, region, limit int) string {
	return fmt.Sprintf("%s:analytics:pioneers:%d:%d:%d", kg.Prefix, zone, region, limit)
}

func (kg *KeyGenerator) PSTNCMNodesKey(limit int) string {
	return fmt.Sprintf("%s:analytics:pstn:cm:%d", kg.Prefix, limit)
}

func (kg *KeyGenerator) PSTNNodesKey(limit, zone int) string {
	return fmt.Sprintf("%s:analytics:pstn:nodes:%d:%d", kg.Prefix, limit, zone)
}

func (kg *KeyGenerator) FileRequestNodesKey(limit int) string {
	return fmt.Sprintf("%s:analytics:filerequest:%d", kg.Prefix, limit)
}

func (kg *KeyGenerator) AKAMismatchNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:aka:mismatch:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) ModemAccessibleNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:modem:accessible:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) ModemNoAnswerNodesKey(limit, days int, includeZeroNodes bool) string {
	return fmt.Sprintf("%s:analytics:modem:noanswer:%d:%d:%t", kg.Prefix, limit, days, includeZeroNodes)
}

func (kg *KeyGenerator) ModemTestDetailKey(zone, net, node int, testTime string) string {
	return fmt.Sprintf("%s:analytics:modem:detail:%d:%d:%d:%s", kg.Prefix, zone, net, node, testTime)
}

func (kg *KeyGenerator) ReachabilityTrendsKey(days int) string {
	return fmt.Sprintf("%s:reachability:trends:%d", kg.Prefix, days)
}

func (kg *KeyGenerator) SearchNodesByReachabilityKey(operational bool, limit, days int) string {
	return fmt.Sprintf("%s:reachability:search:%t:%d:%d", kg.Prefix, operational, limit, days)
}

func (kg *KeyGenerator) NodeTestHistoryKey(zone, net, node, days int) string {
	return fmt.Sprintf("%s:reachability:history:%d:%d:%d:%d", kg.Prefix, zone, net, node, days)
}

func (kg *KeyGenerator) NodeReachabilityStatsKey(zone, net, node, days int) string {
	return fmt.Sprintf("%s:reachability:stats:%d:%d:%d:%d", kg.Prefix, zone, net, node, days)
}

func (kg *KeyGenerator) DetailedTestResultKey(zone, net, node int, testTime string) string {
	return fmt.Sprintf("%s:reachability:detail:%d:%d:%d:%s", kg.Prefix, zone, net, node, testTime)
}

func (kg *KeyGenerator) WhoisResultsKey() string {
	return fmt.Sprintf("%s:analytics:whois:results", kg.Prefix)
}

func (kg *KeyGenerator) AnalyticsPattern() string {
	return fmt.Sprintf("%s:analytics:*", kg.Prefix)
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