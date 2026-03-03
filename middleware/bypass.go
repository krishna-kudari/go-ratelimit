package middleware

import (
	"net"
	"net/http"
	"strings"
)

// BypassFunc returns true if the request should skip rate limiting.
type BypassFunc func(r *http.Request) bool

// BypassByAllowlist returns a BypassFunc that bypasses when the client IP
// is in any of the given CIDR blocks (e.g. "10.0.0.0/8", "127.0.0.1/32").
// Client IP is resolved via X-Forwarded-For, X-Real-IP, then RemoteAddr.
func BypassByAllowlist(cidrs []string) BypassFunc {
	if len(cidrs) == 0 {
		return nil
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, s := range cidrs {
		_, n, err := net.ParseCIDR(strings.TrimSpace(s))
		if err != nil {
			continue
		}
		nets = append(nets, n)
	}
	if len(nets) == 0 {
		return nil
	}
	return func(r *http.Request) bool {
		return IPInAllowlist(KeyByIP(r), nets)
	}
}

// BypassByHeader returns a BypassFunc that bypasses when the request has
// the given header set to the given value (e.g. internal service secret).
// If value is empty, bypasses when the header is present.
func BypassByHeader(name, value string) BypassFunc {
	return func(r *http.Request) bool {
		v := r.Header.Get(name)
		if value == "" {
			return v != ""
		}
		return v == value
	}
}

// IPInAllowlist reports whether ipStr (e.g. "192.168.1.1") is contained
// in any of the pre-parsed CIDR networks. Exported for use by framework
// middleware (gin, echo, fiber) that resolve client IP themselves.
func IPInAllowlist(ipStr string, nets []*net.IPNet) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ParseAllowlistCIDRs parses a slice of CIDR strings into *net.IPNet.
// Invalid entries are skipped. Use with IPInAllowlist when client IP
// is obtained outside net/http (e.g. Gin's ClientIP(), Echo's RealIP()).
func ParseAllowlistCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, s := range cidrs {
		_, n, err := net.ParseCIDR(strings.TrimSpace(s))
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}
