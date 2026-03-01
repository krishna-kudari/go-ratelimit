package ginmw_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	goratelimit "github.com/krishna-kudari/ratelimit"
	"github.com/krishna-kudari/ratelimit/middleware/ginmw"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRouter(mw gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(mw)
	r.GET("/api/data", func(c *gin.Context) { c.String(200, "ok") })
	r.GET("/health", func(c *gin.Context) { c.String(200, "ok") })
	return r
}

func TestRateLimit_AllowsWithinLimit(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(5, 60))
	router := newRouter(ginmw.RateLimit(limiter, ginmw.KeyByClientIP))

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/data", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
		if w.Header().Get("X-RateLimit-Limit") != "5" {
			t.Errorf("request %d: expected limit=5, got %s", i+1, w.Header().Get("X-RateLimit-Limit"))
		}
	}
}

func TestRateLimit_DeniesExceedingLimit(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(2, 60))
	router := newRouter(ginmw.RateLimit(limiter, ginmw.KeyByClientIP))

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/data", nil)
		req.RemoteAddr = "5.6.7.8:1234"
		router.ServeHTTP(w, req)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.RemoteAddr = "5.6.7.8:1234"
	router.ServeHTTP(w, req)

	if w.Code != 429 {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestRateLimit_ExcludePaths(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(1, 60))
	router := newRouter(ginmw.RateLimitWithConfig(ginmw.Config{
		Limiter:      limiter,
		KeyFunc:      ginmw.KeyByClientIP,
		ExcludePaths: map[string]bool{"/health": true},
	}))

	// Exhaust limit
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	router.ServeHTTP(w, req)

	// Health should bypass
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/health", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("health should bypass, got %d", w.Code)
	}
}

func TestRateLimit_CustomDeniedHandler(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(1, 60))
	customCalled := false
	router := newRouter(ginmw.RateLimitWithConfig(ginmw.Config{
		Limiter: limiter,
		KeyFunc: ginmw.KeyByClientIP,
		DeniedHandler: func(c *gin.Context, _ *goratelimit.Result) {
			customCalled = true
			c.AbortWithStatusJSON(429, gin.H{"custom": true})
		},
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.RemoteAddr = "11.0.0.1:1234"
	router.ServeHTTP(w, req)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/data", nil)
	req.RemoteAddr = "11.0.0.1:1234"
	router.ServeHTTP(w, req)

	if !customCalled {
		t.Error("custom denied handler should be called")
	}
}

func TestRateLimit_HeadersDisabled(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(5, 60))
	noHeaders := false
	router := newRouter(ginmw.RateLimitWithConfig(ginmw.Config{
		Limiter: limiter,
		KeyFunc: ginmw.KeyByClientIP,
		Headers: &noHeaders,
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.RemoteAddr = "12.0.0.1:1234"
	router.ServeHTTP(w, req)

	if w.Header().Get("X-RateLimit-Limit") != "" {
		t.Error("headers should not be set")
	}
}

func TestKeyByHeader(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(1, 60))
	router := newRouter(ginmw.RateLimit(limiter, ginmw.KeyByHeader("X-API-Key")))

	// key-A: allowed
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-API-Key", "key-A")
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal("key-A should be allowed")
	}

	// key-A: denied
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-API-Key", "key-A")
	router.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatal("key-A should be denied")
	}

	// key-B: allowed (different key)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-API-Key", "key-B")
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal("key-B should be allowed")
	}
}

func must(l goratelimit.Limiter, err error) goratelimit.Limiter {
	if err != nil {
		panic(err)
	}
	return l
}
