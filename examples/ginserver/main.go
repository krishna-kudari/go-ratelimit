// Complete Gin server with rate limiting middleware.
// Run: go run ./examples/ginserver/
// Test: curl -i http://localhost:8080/api/hello
package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	goratelimit "github.com/krishna-kudari/ratelimit"
	"github.com/krishna-kudari/ratelimit/middleware/ginmw"
)

func main() {
	limiter, _ := goratelimit.NewTokenBucket(5, 1)

	r := gin.Default()
	r.Use(ginmw.RateLimit(limiter, ginmw.KeyByClientIP))

	r.GET("/api/hello", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "hello"})
	})

	log.Println("listening on :8080")
	r.Run(":8080")
}
