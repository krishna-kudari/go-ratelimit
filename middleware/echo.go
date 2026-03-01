// This file is kept for backward-compatibility documentation.
// The concrete Echo middleware implementation lives in the echomw sub-package
// to avoid pulling github.com/labstack/echo into projects that only need HTTP middleware.
//
// Import:
//
//	import "github.com/krishna-kudari/ratelimit/middleware/echomw"
//
// Usage:
//
//	limiter, _ := goratelimit.NewGCRA(1000, 50, goratelimit.WithRedis(redisClient))
//	e := echo.New()
//	e.Use(echomw.RateLimit(limiter, echomw.KeyByRealIP))
//
// Key extractors:
//
//	echomw.KeyByRealIP            — Echo's RealIP() with proxy support
//	echomw.KeyByHeader("X-API-Key") — value from request header
//	echomw.KeyByParam("id")      — value from path parameter
//	echomw.KeyByPathAndIP        — path + real IP for per-endpoint limits
//
// Full config:
//
//	echomw.RateLimitWithConfig(echomw.Config{
//	    Limiter:      limiter,
//	    KeyFunc:      echomw.KeyByRealIP,
//	    ExcludePaths: map[string]bool{"/health": true},
//	    DeniedHandler: customHandler,
//	})
//
// See package github.com/krishna-kudari/ratelimit/middleware/echomw for full API.
package middleware
