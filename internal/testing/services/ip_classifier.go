package services

import (
	"net"
	"strings"
)

// IPClassifier classifies IP addresses
type IPClassifier struct{}

// NewIPClassifier creates a new IP classifier
func NewIPClassifier() *IPClassifier {
	return &IPClassifier{}
}

// IPClassification represents the classification of an IP address
type IPClassification struct {
	IP           string
	Version      int    // 4 or 6
	Type         string // public, private, loopback, link-local, multicast, reserved
	IsPublic     bool
	IsPrivate    bool
	IsLoopback   bool
	IsLinkLocal  bool
	IsMulticast  bool
	IsReserved   bool
	IsRoutable   bool
}

// Classify classifies an IP address
func (c *IPClassifier) Classify(ipStr string) *IPClassification {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil
	}
	
	classification := &IPClassification{
		IP: ipStr,
	}
	
	// Determine IP version
	if ip.To4() != nil {
		classification.Version = 4
		c.classifyIPv4(ip, classification)
	} else if ip.To16() != nil {
		classification.Version = 6
		c.classifyIPv6(ip, classification)
	}
	
	// Determine primary type
	classification.determineType()
	
	return classification
}

// classifyIPv4 classifies an IPv4 address
func (c *IPClassifier) classifyIPv4(ip net.IP, class *IPClassification) {
	ipv4 := ip.To4()
	if ipv4 == nil {
		return
	}
	
	// Check for loopback (127.0.0.0/8)
	if ipv4[0] == 127 {
		class.IsLoopback = true
		return
	}
	
	// Check for private ranges (RFC 1918)
	// 10.0.0.0/8
	if ipv4[0] == 10 {
		class.IsPrivate = true
		return
	}
	
	// 172.16.0.0/12
	if ipv4[0] == 172 && ipv4[1] >= 16 && ipv4[1] <= 31 {
		class.IsPrivate = true
		return
	}
	
	// 192.168.0.0/16
	if ipv4[0] == 192 && ipv4[1] == 168 {
		class.IsPrivate = true
		return
	}
	
	// Check for link-local (169.254.0.0/16)
	if ipv4[0] == 169 && ipv4[1] == 254 {
		class.IsLinkLocal = true
		return
	}
	
	// Check for multicast (224.0.0.0/4)
	if ipv4[0] >= 224 && ipv4[0] <= 239 {
		class.IsMulticast = true
		return
	}
	
	// Check for reserved ranges
	// 0.0.0.0/8 - Current network
	if ipv4[0] == 0 {
		class.IsReserved = true
		return
	}
	
	// 240.0.0.0/4 - Reserved for future use
	if ipv4[0] >= 240 {
		class.IsReserved = true
		return
	}
	
	// 100.64.0.0/10 - Carrier-grade NAT (RFC 6598)
	if ipv4[0] == 100 && ipv4[1] >= 64 && ipv4[1] <= 127 {
		class.IsPrivate = true
		return
	}
	
	// If none of the above, it's public
	class.IsPublic = true
	class.IsRoutable = true
}

// classifyIPv6 classifies an IPv6 address
func (c *IPClassifier) classifyIPv6(ip net.IP, class *IPClassification) {
	// Check for loopback (::1/128)
	if ip.IsLoopback() {
		class.IsLoopback = true
		return
	}
	
	// Check for link-local (fe80::/10)
	if ip.IsLinkLocalUnicast() {
		class.IsLinkLocal = true
		return
	}
	
	// Check for multicast (ff00::/8)
	if ip.IsMulticast() {
		class.IsMulticast = true
		return
	}
	
	// Check for unique local (fc00::/7) - similar to IPv4 private
	if len(ip) >= 1 && (ip[0] == 0xfc || ip[0] == 0xfd) {
		class.IsPrivate = true
		return
	}
	
	// Check for unspecified address (::)
	if ip.IsUnspecified() {
		class.IsReserved = true
		return
	}
	
	// Check for IPv4-mapped IPv6 (::ffff:0:0/96)
	if strings.HasPrefix(ip.String(), "::ffff:") {
		class.IsReserved = true
		return
	}
	
	// Documentation prefix (2001:db8::/32)
	if len(ip) >= 2 && ip[0] == 0x20 && ip[1] == 0x01 && 
	   len(ip) >= 4 && ip[2] == 0x0d && ip[3] == 0xb8 {
		class.IsReserved = true
		return
	}
	
	// Global unicast (2000::/3)
	if len(ip) >= 1 && (ip[0]&0xe0) == 0x20 {
		class.IsPublic = true
		class.IsRoutable = true
		return
	}
	
	// Default to reserved if not classified
	class.IsReserved = true
}

// determineType sets the primary type based on classification flags
func (class *IPClassification) determineType() {
	switch {
	case class.IsLoopback:
		class.Type = "loopback"
	case class.IsPrivate:
		class.Type = "private"
	case class.IsLinkLocal:
		class.Type = "link-local"
	case class.IsMulticast:
		class.Type = "multicast"
	case class.IsPublic:
		class.Type = "public"
	case class.IsReserved:
		class.Type = "reserved"
	default:
		class.Type = "unknown"
	}
}

// ClassifyBatch classifies multiple IP addresses
func (c *IPClassifier) ClassifyBatch(ips []string) map[string]*IPClassification {
	results := make(map[string]*IPClassification)
	for _, ip := range ips {
		if classification := c.Classify(ip); classification != nil {
			results[ip] = classification
		}
	}
	return results
}

// IsRoutable checks if an IP address is routable on the public internet
func (c *IPClassifier) IsRoutable(ipStr string) bool {
	classification := c.Classify(ipStr)
	if classification == nil {
		return false
	}
	return classification.IsRoutable
}

// GetIPVersion returns the IP version (4 or 6) or 0 if invalid
func (c *IPClassifier) GetIPVersion(ipStr string) int {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return 0
	}
	if ip.To4() != nil {
		return 4
	}
	if ip.To16() != nil {
		return 6
	}
	return 0
}