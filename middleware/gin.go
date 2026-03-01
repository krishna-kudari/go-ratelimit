// This file is kept for backward-compatibility documentation.
// The concrete Gin middleware implementation lives in the ginmw sub-package
// to avoid pulling github.com/gin-gonic/gin into projects that only need HTTP middleware.
//
// Import:
//
//	import "github.com/krishna-kudari/ratelimit/middleware/ginmw"
//
// Usage:
//
//	limiter, _ := goratelimit.NewGCRA(1000, 50, goratelimit.WithRedis(redisClient))
//	r := gin.Default()
//	r.Use(ginmw.RateLimit(limiter, ginmw.KeyByClientIP))
//
// Key extractors:
//
//	ginmw.KeyByClientIP          — Gin's ClientIP() with trusted proxy support
//	ginmw.KeyByHeader("X-API-Key") — value from request header
//	ginmw.KeyByParam(":id")     — value from URL parameter
//	ginmw.KeyByPathAndIP        — path + client IP for per-endpoint limits
//
// Full config:
//
//	ginmw.RateLimitWithConfig(ginmw.Config{
//	    Limiter:      limiter,
//	    KeyFunc:      ginmw.KeyByClientIP,
//	    ExcludePaths: map[string]bool{"/health": true},
//	    DeniedHandler: customHandler,
//	})
//
// See package github.com/krishna-kudari/ratelimit/middleware/ginmw for full API.
package middleware
