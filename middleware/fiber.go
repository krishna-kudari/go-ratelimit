// This file is kept for backward-compatibility documentation.
// The concrete Fiber middleware implementation lives in the fibermw sub-package
// to avoid pulling github.com/gofiber/fiber into projects that only need HTTP middleware.
// Fiber uses fasthttp (not net/http) so a dedicated adapter is required.
//
// Import:
//
//	import "github.com/krishna-kudari/ratelimit/middleware/fibermw"
//
// Usage:
//
//	limiter, _ := goratelimit.NewGCRA(1000, 50, goratelimit.WithRedis(redisClient))
//	app := fiber.New()
//	app.Use(fibermw.RateLimit(limiter, fibermw.KeyByIP))
//
// Key extractors:
//
//	fibermw.KeyByIP               — Fiber's IP() with proxy support
//	fibermw.KeyByHeader("X-API-Key") — value from request header
//	fibermw.KeyByParam("id")     — value from route parameter
//	fibermw.KeyByPathAndIP       — path + IP for per-endpoint limits
//
// Full config:
//
//	fibermw.RateLimitWithConfig(fibermw.Config{
//	    Limiter:      limiter,
//	    KeyFunc:      fibermw.KeyByIP,
//	    ExcludePaths: map[string]bool{"/health": true},
//	    DeniedHandler: customHandler,
//	})
//
// See package github.com/krishna-kudari/ratelimit/middleware/fibermw for full API.
package middleware
