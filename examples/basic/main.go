package main

import (
	"context"
	"fmt"
	"time"

	goratelimit "github.com/krishna-kudari/ratelimit"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Token Bucket (capacity=5, refill=2/s) ===")
	tb, _ := goratelimit.NewTokenBucket(5, 2)
	for i := 1; i <= 8; i++ {
		r, _ := tb.Allow(ctx, "user:1")
		fmt.Printf("  req %d: allowed=%-5v remaining=%d\n", i, r.Allowed, r.Remaining)
	}

	fmt.Println("\n=== Fixed Window (max=3, window=5s) ===")
	fw, _ := goratelimit.NewFixedWindow(3, 5)
	for i := 1; i <= 5; i++ {
		r, _ := fw.Allow(ctx, "user:1")
		fmt.Printf("  req %d: allowed=%-5v remaining=%d\n", i, r.Allowed, r.Remaining)
	}

	fmt.Println("\n=== Leaky Bucket — Policing (capacity=3, leak=1/s) ===")
	lb, _ := goratelimit.NewLeakyBucket(3, 1, goratelimit.Policing)
	for i := 1; i <= 5; i++ {
		r, _ := lb.Allow(ctx, "user:1")
		fmt.Printf("  req %d: allowed=%-5v remaining=%d\n", i, r.Allowed, r.Remaining)
	}

	fmt.Println("\n=== GCRA (rate=2/s, burst=4) ===")
	gcra, _ := goratelimit.NewGCRA(2, 4)
	for i := 1; i <= 6; i++ {
		r, _ := gcra.Allow(ctx, "user:1")
		fmt.Printf("  req %d: allowed=%-5v remaining=%d\n", i, r.Allowed, r.Remaining)
	}

	fmt.Println("\n=== Builder API ===")
	limiter, _ := goratelimit.NewBuilder().
		SlidingWindowCounter(10, 30*time.Second).
		Build()
	r, _ := limiter.Allow(ctx, "user:1")
	fmt.Printf("  allowed=%v remaining=%d limit=%d\n", r.Allowed, r.Remaining, r.Limit)
}
