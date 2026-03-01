package fibermw_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	goratelimit "github.com/krishna-kudari/ratelimit"
	"github.com/krishna-kudari/ratelimit/middleware/fibermw"
)

func newApp(mw fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Use(mw)
	app.Get("/api/data", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendString("ok") })
	return app
}

func doReq(app *fiber.App, method, path string, headers map[string]string) *http.Response {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, _ := app.Test(req, -1)
	return resp
}

func TestRateLimit_AllowsWithinLimit(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(5, 60))
	app := newApp(fibermw.RateLimit(limiter, fibermw.KeyByIP))

	for i := 0; i < 5; i++ {
		resp := doReq(app, "GET", "/api/data", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, resp.StatusCode)
		}
		if resp.Header.Get("X-RateLimit-Limit") != "5" {
			t.Errorf("request %d: expected limit=5, got %s", i+1, resp.Header.Get("X-RateLimit-Limit"))
		}
	}
}

func TestRateLimit_DeniesExceedingLimit(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(2, 60))
	app := newApp(fibermw.RateLimit(limiter, fibermw.KeyByIP))

	for i := 0; i < 2; i++ {
		doReq(app, "GET", "/api/data", nil)
	}

	resp := doReq(app, "GET", "/api/data", nil)
	if resp.StatusCode != 429 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 429, got %d, body: %s", resp.StatusCode, body)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestRateLimit_ExcludePaths(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(1, 60))
	app := newApp(fibermw.RateLimitWithConfig(fibermw.Config{
		Limiter:      limiter,
		KeyFunc:      fibermw.KeyByIP,
		ExcludePaths: map[string]bool{"/health": true},
	}))

	doReq(app, "GET", "/api/data", nil)

	resp := doReq(app, "GET", "/health", nil)
	if resp.StatusCode != 200 {
		t.Errorf("health should bypass, got %d", resp.StatusCode)
	}
}

func TestRateLimit_CustomDeniedHandler(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(1, 60))
	customCalled := false
	app := newApp(fibermw.RateLimitWithConfig(fibermw.Config{
		Limiter: limiter,
		KeyFunc: fibermw.KeyByIP,
		DeniedHandler: func(c *fiber.Ctx, _ *goratelimit.Result) error {
			customCalled = true
			return c.Status(429).JSON(fiber.Map{"custom": true})
		},
	}))

	doReq(app, "GET", "/api/data", nil)
	doReq(app, "GET", "/api/data", nil)

	if !customCalled {
		t.Error("custom denied handler should be called")
	}
}

func TestRateLimit_HeadersDisabled(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(5, 60))
	noHeaders := false
	app := newApp(fibermw.RateLimitWithConfig(fibermw.Config{
		Limiter: limiter,
		KeyFunc: fibermw.KeyByIP,
		Headers: &noHeaders,
	}))

	resp := doReq(app, "GET", "/api/data", nil)
	if resp.Header.Get("X-RateLimit-Limit") != "" {
		t.Error("headers should not be set")
	}
}

func TestKeyByHeader(t *testing.T) {
	limiter := must(goratelimit.NewFixedWindow(1, 60))
	app := newApp(fibermw.RateLimit(limiter, fibermw.KeyByHeader("X-API-Key")))

	resp := doReq(app, "GET", "/api/data", map[string]string{"X-API-Key": "key-A"})
	if resp.StatusCode != 200 {
		t.Fatal("key-A should be allowed")
	}

	resp = doReq(app, "GET", "/api/data", map[string]string{"X-API-Key": "key-A"})
	if resp.StatusCode != 429 {
		t.Fatal("key-A should be denied")
	}

	resp = doReq(app, "GET", "/api/data", map[string]string{"X-API-Key": "key-B"})
	if resp.StatusCode != 200 {
		t.Fatal("key-B should be allowed")
	}
}

func must(l goratelimit.Limiter, err error) goratelimit.Limiter {
	if err != nil {
		panic(err)
	}
	return l
}
