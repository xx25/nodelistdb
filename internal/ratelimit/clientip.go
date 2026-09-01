// Package ratelimit throttles requests per client IP.
//
// The point it exists to solve is measured, not theoretical: bot traffic on
// the expensive archive pages keeps both vCPUs of the public front end busy
// around the clock, and neither the storage cache (13% hit rate against a
// crawler walking unique URLs) nor a robots.txt a bot may ignore bounds it.
package ratelimit

import (
	"net"
	"net/http"
	"strings"
)

// defaultTrustedProxies is the fallback trusted set: the loopback addresses.
//
// That is exactly right for the deployed shape - Caddy terminates TLS on the
// same host and proxies to localhost:8081 - and it is the only default that
// is safe when the server is exposed directly, because then no forwarding
// header is believed at all.
var defaultTrustedProxies = []string{"127.0.0.0/8", "::1/128"}

// ClientIPResolver turns a request into the address to rate-limit it under.
//
// It exists because cmd/server's clientIP() must not be used here. That
// function reads X-Real-IP and X-Forwarded-For unconditionally, which is fine
// for a log line and useless for a limiter: any client can send
// "X-Real-IP: <random>" on every request and be treated as a new caller. The
// production log shows this is not hypothetical - it carries entries from
// 127.184.17.99, an address inside 127.0.0.0/8 that cannot have dialled in
// from the internet, so something upstream is already forging the header.
//
// The rule here is the standard one: believe a forwarding header only when the
// connection itself came from an address configured as a proxy, and then walk
// the X-Forwarded-For chain from the RIGHT, discarding trusted hops, because
// the right-hand entries are the ones our own proxies appended. Anything the
// client invented sits further left and is never reached.
type ClientIPResolver struct {
	trusted []*net.IPNet
}

// NewClientIPResolver compiles the trusted-proxy CIDRs. An empty list means
// the loopback default; each entry may be a CIDR ("10.0.0.0/8") or a bare
// address ("192.0.2.1"), which is taken as a single-host network.
func NewClientIPResolver(cidrs []string) (*ClientIPResolver, error) {
	if len(cidrs) == 0 {
		cidrs = defaultTrustedProxies
	}
	r := &ClientIPResolver{}
	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, netw, err := net.ParseCIDR(raw); err == nil {
			r.trusted = append(r.trusted, netw)
			continue
		}
		ip := net.ParseIP(raw)
		if ip == nil {
			return nil, &net.ParseError{Type: "trusted proxy CIDR or IP", Text: raw}
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		r.trusted = append(r.trusted, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return r, nil
}

// trustedAddr reports whether ip is one of the configured proxies.
func (r *ClientIPResolver) trustedAddr(ip net.IP) bool {
	for _, n := range r.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientIP returns the address to attribute this request to.
//
// When the peer is not a trusted proxy its own address is returned and every
// forwarding header is ignored - a direct caller does not get to name itself.
func (r *ClientIPResolver) ClientIP(req *http.Request) net.IP {
	peer := remoteIP(req.RemoteAddr)
	if peer == nil || !r.trustedAddr(peer) {
		return peer
	}

	// Walk X-Forwarded-For right to left, skipping our own proxies. The first
	// non-trusted hop is the furthest-out address we have any reason to
	// believe; everything left of it is client-supplied.
	for _, hdr := range req.Header.Values("X-Forwarded-For") {
		parts := strings.Split(hdr, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(parts[i]))
			if ip == nil {
				continue
			}
			if !r.trustedAddr(ip) {
				return ip
			}
		}
	}

	// A trusted proxy that sends X-Real-IP and no usable X-Forwarded-For.
	if ip := net.ParseIP(strings.TrimSpace(req.Header.Get("X-Real-IP"))); ip != nil {
		return ip
	}
	return peer
}

// remoteIP parses r.RemoteAddr, which normally carries a port but need not.
func remoteIP(addr string) net.IP {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(addr)
}

// Key collapses an address to the bucket it shares with its neighbours.
//
// IPv4 is limited per address. IPv6 is limited per /64, which is the smallest
// block a subscriber is normally assigned: limiting a single v6 address would
// be free to evade, since a host with a routed prefix can source every request
// from a different one. The production log already shows a caller on a /64
// with a random-looking suffix.
func Key(ip net.IP) string {
	if ip == nil {
		return "unknown"
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}
