package middleware

// Echo Adapter
//
// Echo supports net/http middleware via echo.WrapMiddleware. Use the core
// RateLimit middleware directly:
//
//	import (
//	    "github.com/labstack/echo/v4"
//	    goratelimit "github.com/krishna-kudari/ratelimit"
//	    "github.com/krishna-kudari/ratelimit/middleware"
//	)
//
//	func main() {
//	    limiter, _ := goratelimit.NewGCRA(1000, 50, goratelimit.WithRedis(redisClient))
//
//	    e := echo.New()
//
//	    // Option 1: Wrap the net/http middleware
//	    e.Use(echo.WrapMiddleware(middleware.RateLimit(limiter, middleware.KeyByIP)))
//
//	    // Option 2: Use a custom Echo middleware for more control
//	    e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
//	        return func(c echo.Context) error {
//	            key := c.RealIP() // or c.Request().Header.Get("X-API-Key")
//	            result, err := limiter.Allow(c.Request().Context(), key)
//	            if err != nil {
//	                return next(c)
//	            }
//
//	            c.Response().Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", result.Limit))
//	            c.Response().Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
//
//	            if !result.Allowed {
//	                c.Response().Header().Set("Retry-After", fmt.Sprintf("%d", int(result.RetryAfter.Seconds()+0.5)))
//	                return c.JSON(429, map[string]string{"error": "rate limit exceeded"})
//	            }
//	            return next(c)
//	        }
//	    })
//
//	    e.GET("/api/data", handler)
//	    e.Start(":8080")
//	}
