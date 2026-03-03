package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	rl "github.com/krishna-kudari/ratelimit"
)

type okResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

type errResponse struct {
	Error      string `json:"error"`
	RetryAfter string `json:"retry_after"`
	Remaining  int64  `json:"remaining"`
}

func rateLimitMiddleware(limiter rl.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				key = r.RemoteAddr
			}

			result, err := limiter.Allow(r.Context(), key)
			if err != nil {
				log.Printf("[ERROR] rate limiter error: %v", err)
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", result.Limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetAt.Unix()))

			if !result.Allowed {
				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", result.RetryAfter.Seconds()))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(errResponse{
					Error:      "rate_limit_exceeded",
					RetryAfter: result.RetryAfter.String(),
					Remaining:  result.Remaining,
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(okResponse{
		Status:    "ok",
		Timestamp: time.Now(),
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	var limiter rl.Limiter
	var err error

	mode := os.Getenv("LIMITER_MODE")
	limit := int64(1000)
	window := int64(60)
	// Align all algorithms to "limit per window" (e.g. 1000 per 60s).
	// GCRA/TokenBucket use per-second rate, so ratePerSec = limit/window.
	ratePerSec := limit / window
	if ratePerSec < 1 {
		ratePerSec = 1
	}

	switch mode {
	case "fixed":
		log.Println("[server] using FixedWindow limiter")
		limiter, err = rl.NewFixedWindow(limit, window)
	case "token":
		log.Println("[server] using TokenBucket limiter")
		limiter, err = rl.NewTokenBucket(ratePerSec*2, ratePerSec)
	case "cms":
		log.Println("[server] using CMS limiter")
		limiter, err = rl.NewCMS(limit, window, 0.01, 0.001)
	case "prefilter":
		log.Println("[server] using PreFilter (CMS+GCRA) limiter")
		local, _ := rl.NewCMS(limit, window, 0.01, 0.001)
		precise, _ := rl.NewGCRA(ratePerSec, ratePerSec*2)
		limiter = rl.NewPreFilter(local, precise)
	default:
		mode = "gcra"
		log.Println("[server] using GCRA limiter (default)")
		limiter, err = rl.NewGCRA(ratePerSec, ratePerSec*2)
	}

	if err != nil {
		log.Fatalf("failed to create limiter: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.Handle("/api/test", rateLimitMiddleware(limiter)(http.HandlerFunc(apiHandler)))

	addr := ":" + port
	log.Printf("[server] listening on %s mode=%s limit=%d/per %ds window", addr, mode, limit, window)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
