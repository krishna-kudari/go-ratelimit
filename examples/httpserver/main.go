// Complete net/http server with rate limiting middleware.
// Run: go run ./examples/httpserver/
// Test: curl -i http://localhost:8080/api/hello
package main

import (
	"encoding/json"
	"log"
	"net/http"

	goratelimit "github.com/krishna-kudari/ratelimit"
	"github.com/krishna-kudari/ratelimit/middleware"
)

func main() {
	limiter, _ := goratelimit.NewTokenBucket(5, 1)

	mux := http.NewServeMux()

	mux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"message": "hello"})
	})

	// Apply rate limiting — KeyByIP extracts client IP as the key
	handler := middleware.RateLimit(limiter, middleware.KeyByIP)(mux)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
