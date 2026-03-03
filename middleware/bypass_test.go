package middleware_test

import (
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/krishna-kudari/ratelimit/middleware"
)

func TestBypassByAllowlist(t *testing.T) {
	bypass := middleware.BypassByAllowlist([]string{"10.0.0.0/8", "192.168.0.0/16"})
	require.NotNil(t, bypass)

	req := func(remote string) *http.Request {
		r, _ := http.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		return r
	}

	assert.True(t, bypass(req("10.1.2.3:80")))
	assert.True(t, bypass(req("192.168.1.1:443")))
	assert.False(t, bypass(req("172.16.0.1:80")))
	assert.False(t, bypass(req("8.8.8.8:53")))
}

func TestBypassByAllowlist_EmptyOrInvalid(t *testing.T) {
	assert.Nil(t, middleware.BypassByAllowlist(nil))
	assert.Nil(t, middleware.BypassByAllowlist([]string{}))
	assert.Nil(t, middleware.BypassByAllowlist([]string{"not-a-cidr"}))
}

func TestBypassByHeader(t *testing.T) {
	bypassValue := middleware.BypassByHeader("X-Internal", "secret")
	r, _ := http.NewRequest("GET", "/", nil)
	assert.False(t, bypassValue(r))
	r.Header.Set("X-Internal", "other")
	assert.False(t, bypassValue(r))
	r.Header.Set("X-Internal", "secret")
	assert.True(t, bypassValue(r))

	bypassPresence := middleware.BypassByHeader("X-Present", "")
	r2, _ := http.NewRequest("GET", "/", nil)
	assert.False(t, bypassPresence(r2))
	r2.Header.Set("X-Present", "anything")
	assert.True(t, bypassPresence(r2))
}

func TestIPInAllowlist(t *testing.T) {
	nets := middleware.ParseAllowlistCIDRs([]string{"10.0.0.0/8", "::1/128"})
	require.Len(t, nets, 2)

	assert.True(t, middleware.IPInAllowlist("10.0.0.1", nets))
	assert.True(t, middleware.IPInAllowlist("10.255.255.255", nets))
	assert.False(t, middleware.IPInAllowlist("11.0.0.1", nets))
	assert.True(t, middleware.IPInAllowlist("::1", nets))
	assert.False(t, middleware.IPInAllowlist("invalid", nets))
	assert.False(t, middleware.IPInAllowlist("", nets))
}

func TestParseAllowlistCIDRs(t *testing.T) {
	nets := middleware.ParseAllowlistCIDRs([]string{" 10.0.0.0/8 ", "bad", "192.168.0.0/16"})
	require.Len(t, nets, 2)
	var a, b *net.IPNet
	for _, n := range nets {
		if n.IP.String() == "10.0.0.0" {
			a = n
		} else {
			b = n
		}
	}
	require.NotNil(t, a)
	require.NotNil(t, b)
	assert.True(t, a.Contains(net.ParseIP("10.1.0.0")))
	assert.True(t, b.Contains(net.ParseIP("192.168.1.1")))
}
