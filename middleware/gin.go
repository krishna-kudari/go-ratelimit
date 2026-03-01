package middleware

// Gin Adapter
//
// Gin supports net/http middleware via gin.WrapH. Use the core RateLimit
// middleware directly:
//
//	import (
//	    "github.com/gin-gonic/gin"
//	    goratelimit "github.com/krishna-kudari/ratelimit"
//	    "github.com/krishna-kudari/ratelimit/middleware"
//	)
//
//	func main() {
//	    limiter, _ := goratelimit.NewGCRA(1000, 50, goratelimit.WithRedis(redisClient))
//
//	    r := gin.Default()
//
//	    // Option 1: Use the built-in net/http middleware wrapper
//	    r.Use(gin.WrapH(middleware.RateLimit(limiter, middleware.KeyByIP)(http.DefaultServeMux)))
//
//	    // Option 2: Use a custom Gin middleware for more control
//	    r.Use(func(c *gin.Context) {
//	        key := c.ClientIP() // or c.GetHeader("X-API-Key")
//	        result, err := limiter.Allow(c.Request.Context(), key)
//	        if err != nil {
//	            c.Next()
//	            return
//	        }
//
//	        // Set rate limit headers
//	        c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", result.Limit))
//	        c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
//
//	        if !result.Allowed {
//	            c.Header("Retry-After", fmt.Sprintf("%d", int(result.RetryAfter.Seconds()+0.5)))
//	            c.AbortWithStatusJSON(429, gin.H{"error": "rate limit exceeded"})
//	            return
//	        }
//	        c.Next()
//	    })
//
//	    r.GET("/api/data", handler)
//	    r.Run(":8080")
//	}
