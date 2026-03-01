// Complete Fiber server with rate limiting middleware.
// Run: go run ./examples/fiberserver/
// Test: curl -i http://localhost:8080/api/hello
package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	goratelimit "github.com/krishna-kudari/ratelimit"
	"github.com/krishna-kudari/ratelimit/middleware/fibermw"
)

func main() {
	limiter, _ := goratelimit.NewTokenBucket(5, 1)

	app := fiber.New()
	app.Use(fibermw.RateLimit(limiter, fibermw.KeyByIP))

	app.Get("/api/hello", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "hello"})
	})

	log.Fatal(app.Listen(":8080"))
}
