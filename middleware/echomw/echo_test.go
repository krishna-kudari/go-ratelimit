package echomw_test

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	goratelimit "github.com/krishna-kudari/ratelimit"
	"github.com/krishna-kudari/ratelimit/middleware/echomw"
)

func newEcho(mw echo.MiddlewareFunc) *echo.Echo {
	e := echo.New()
	e.Use(mw)
	e.GET("/api/data", func(c echo.Context) error { return c.String(200, "ok") })
	e.GET("/health", func(c echo.Context) error { return c.String(200, "ok") })
	return e
}

func TestRateLimit_AllowsWithinLimit(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(5, 60))
	e := newEcho(echomw.RateLimit(limiter, echomw.KeyByRealIP))

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/data", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		e.ServeHTTP(w, req)

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
	e := newEcho(echomw.RateLimit(limiter, echomw.KeyByRealIP))

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/data", nil)
		req.RemoteAddr = "5.6.7.8:1234"
		e.ServeHTTP(w, req)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.RemoteAddr = "5.6.7.8:1234"
	e.ServeHTTP(w, req)

	if w.Code != 429 {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestRateLimit_ExcludePaths(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(1, 60))
	e := newEcho(echomw.RateLimitWithConfig(echomw.Config{
		Limiter:      limiter,
		KeyFunc:      echomw.KeyByRealIP,
		ExcludePaths: map[string]bool{"/health": true},
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	e.ServeHTTP(w, req)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/health", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	e.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("health should bypass, got %d", w.Code)
	}
}

func TestRateLimit_CustomDeniedHandler(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(1, 60))
	customCalled := false
	e := newEcho(echomw.RateLimitWithConfig(echomw.Config{
		Limiter: limiter,
		KeyFunc: echomw.KeyByRealIP,
		DeniedHandler: func(c echo.Context, _ *goratelimit.Result) error {
			customCalled = true
			return c.JSON(429, map[string]bool{"custom": true})
		},
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.RemoteAddr = "11.0.0.1:1234"
	e.ServeHTTP(w, req)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/data", nil)
	req.RemoteAddr = "11.0.0.1:1234"
	e.ServeHTTP(w, req)

	if !customCalled {
		t.Error("custom denied handler should be called")
	}
}

func TestRateLimit_HeadersDisabled(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(5, 60))
	noHeaders := false
	e := newEcho(echomw.RateLimitWithConfig(echomw.Config{
		Limiter: limiter,
		KeyFunc: echomw.KeyByRealIP,
		Headers: &noHeaders,
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.RemoteAddr = "12.0.0.1:1234"
	e.ServeHTTP(w, req)

	if w.Header().Get("X-RateLimit-Limit") != "" {
		t.Error("headers should not be set")
	}
}

func TestKeyByHeader(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(1, 60))
	e := newEcho(echomw.RateLimit(limiter, echomw.KeyByHeader("X-API-Key")))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-API-Key", "key-A")
	e.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal("key-A should be allowed")
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-API-Key", "key-A")
	e.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatal("key-A should be denied")
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-API-Key", "key-B")
	e.ServeHTTP(w, req)
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
