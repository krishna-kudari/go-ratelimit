package middleware

// Fiber Adapter
//
// Fiber uses fasthttp (not net/http), so the core RateLimit middleware cannot
// be wrapped directly. Use the Limiter interface in a custom Fiber middleware:
//
//	import (
//	    "github.com/gofiber/fiber/v2"
//	    goratelimit "github.com/krishna-kudari/ratelimit"
//	)
//
//	func RateLimitFiber(limiter goratelimit.Limiter) fiber.Handler {
//	    return func(c *fiber.Ctx) error {
//	        key := c.IP() // or c.Get("X-API-Key")
//	        result, err := limiter.Allow(c.UserContext(), key)
//	        if err != nil {
//	            return c.Next()
//	        }
//
//	        c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", result.Limit))
//	        c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
//
//	        if !result.Allowed {
//	            c.Set("Retry-After", fmt.Sprintf("%d", int(result.RetryAfter.Seconds()+0.5)))
//	            return c.Status(429).JSON(fiber.Map{"error": "rate limit exceeded"})
//	        }
//	        return c.Next()
//	    }
//	}
//
// Usage:
//
//	limiter, _ := goratelimit.NewGCRA(1000, 50, goratelimit.WithRedis(redisClient))
//	app := fiber.New()
//	app.Use(RateLimitFiber(limiter))
//	app.Get("/api/data", handler)
//	app.Listen(":8080")
