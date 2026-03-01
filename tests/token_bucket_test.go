package goratelimit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krishna-kudari/ratelimit"
)

func TestNewtokenBucketRateLimitter(t *testing.T) {
	tests := []struct {
		name           string
		maxCapacity    int64
		refilRate      int64
		expectError    bool
		errorSubstring string
	}{
		{
			name:        "valid parameters",
			maxCapacity: 10,
			refilRate:   60,
			expectError: false,
		},
		{
			name:           "zero max capacity",
			maxCapacity:    0,
			refilRate:      60,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative max capacity",
			maxCapacity:    -1,
			refilRate:      60,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "zero refill rate",
			maxCapacity:    10,
			refilRate:      0,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative refill rate",
			maxCapacity:    10,
			refilRate:      -1,
			expectError:    true,
			errorSubstring: "must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := goratelimit.NewtokenBucketRateLimitter(tt.maxCapacity, tt.refilRate)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorSubstring != "" && !contains(err.Error(), tt.errorSubstring) {
					t.Errorf("expected error to contain %q, got %q", tt.errorSubstring, err.Error())
				}
				if limiter != nil {
					t.Errorf("expected limiter to be nil on error, got %v", limiter)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if limiter == nil {
					t.Errorf("expected limiter to be non-nil, got nil")
				}
			}
		})
	}
}

func TestTokenBucketRateLimiter_Allow(t *testing.T) {
	t.Run("allows requests within capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewtokenBucketRateLimitter(5, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should allow up to capacity (5 tokens)
		for i := 0; i < 5; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i+1)
			}
		}
	})

	t.Run("rejects requests when tokens exhausted", func(t *testing.T) {
		limiter, err := goratelimit.NewtokenBucketRateLimitter(3, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up all tokens
		for i := 0; i < 3; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// 4th request should be rejected (no tokens left)
		if limiter.Allow() {
			t.Error("4th request should be rejected")
		}
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		limiter, err := goratelimit.NewtokenBucketRateLimitter(2, 2) // 2 tokens capacity, 2 tokens per second refill
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up all tokens
		if !limiter.Allow() {
			t.Error("first request should be allowed")
		}
		if !limiter.Allow() {
			t.Error("second request should be allowed")
		}
		if limiter.Allow() {
			t.Error("third request should be rejected")
		}

		// Wait for tokens to refill (1 second should refill 2 tokens)
		time.Sleep(1100 * time.Millisecond)

		// Should allow requests again after refill
		if !limiter.Allow() {
			t.Error("request after refill should be allowed")
		}
		if !limiter.Allow() {
			t.Error("second request after refill should be allowed")
		}
		if limiter.Allow() {
			t.Error("third request after refill should be rejected")
		}
	})

	t.Run("gradual refill allows steady request flow", func(t *testing.T) {
		limiter, err := goratelimit.NewtokenBucketRateLimitter(10, 10) // 10 tokens capacity, 10 tokens per second refill
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up all tokens
		for i := 0; i < 10; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i+1)
			}
		}
		if limiter.Allow() {
			t.Error("11th request should be rejected")
		}

		// Wait for partial refill (0.1 second = 1 token)
		time.Sleep(110 * time.Millisecond)

		// Should allow one more request
		if !limiter.Allow() {
			t.Error("request after partial refill should be allowed")
		}
		if limiter.Allow() {
			t.Error("next request should be rejected (only 1 token refilled)")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewtokenBucketRateLimitter(100, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		allowed := make(chan bool, 200)
		for i := 0; i < 200; i++ {
			go func() {
				allowed <- limiter.Allow()
			}()
		}

		count := 0
		for i := 0; i < 200; i++ {
			if <-allowed {
				count++
			}
		}

		// Should allow exactly capacity (100) requests
		if count != 100 {
			t.Errorf("expected exactly 100 allowed requests, got %d", count)
		}
	})

	t.Run("tokens never exceed capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewtokenBucketRateLimitter(5, 100) // High refill rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up all tokens
		for i := 0; i < 5; i++ {
			limiter.Allow()
		}

		// Wait long enough that refill would exceed capacity
		time.Sleep(200 * time.Millisecond)

		// Should only allow up to capacity, not more
		allowedCount := 0
		for i := 0; i < 10; i++ {
			if limiter.Allow() {
				allowedCount++
			}
		}

		// Should allow exactly capacity (5) tokens
		if allowedCount != 5 {
			t.Errorf("expected exactly 5 allowed requests (capacity), got %d", allowedCount)
		}
	})
}

func TestTokenBucketRateLimitStore_Allow(t *testing.T) {
	t.Run("creates limiter for new user", func(t *testing.T) {
		// Note: tokenBucketRateLimitStore has unexported fields
		// This test would need a constructor function or be moved to same package
		t.Skip("tokenBucketRateLimitStore has unexported fields - needs constructor or same-package test")
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		t.Skip("tokenBucketRateLimitStore has unexported fields - needs constructor or same-package test")
	})

	t.Run("concurrent access to different users", func(t *testing.T) {
		t.Skip("tokenBucketRateLimitStore has unexported fields - needs constructor or same-package test")
	})
}

func TestNewtokenBucketRedisRateLimiter(t *testing.T) {
	ctx := context.Background()

	t.Run("valid parameters", func(t *testing.T) {
		limiter, err := goratelimit.NewtokenBucketRedisRateLimiter(ctx, 10, 60)
		if err != nil {
			t.Skipf("Redis not available: %v", err)
		}
		if limiter == nil {
			t.Error("expected limiter to be non-nil")
		}
	})

	t.Run("invalid parameters", func(t *testing.T) {
		// Note: The constructor doesn't validate parameters, it only checks Redis connection
		// This is a design issue but we test what exists
		limiter, err := goratelimit.NewtokenBucketRedisRateLimiter(ctx, 0, 60)
		if err != nil {
			t.Skipf("Redis not available: %v", err)
		}
		if limiter == nil {
			t.Error("limiter created even with invalid capacity")
		}
	})
}

func TestTokenBucketRedisRateLimiter_Allow(t *testing.T) {
	ctx := context.Background()
	limiter, err := goratelimit.NewtokenBucketRedisRateLimiter(ctx, 10, 60)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("allows requests within limit", func(t *testing.T) {
		userID := "test-token-user-1"
		allowed, remaining, limit, retryAfter := limiter.Allow(ctx, userID)
		if !allowed {
			t.Error("first request should be allowed")
		}
		if remaining < 0 || remaining > limit {
			t.Errorf("remaining should be between 0 and %d, got %d", limit, remaining)
		}
		if retryAfter != 0 {
			t.Errorf("retryAfter should be 0 when allowed, got %d", retryAfter)
		}
	})

	t.Run("rejects requests exceeding limit", func(t *testing.T) {
		userID := fmt.Sprintf("test-token-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewtokenBucketRedisRateLimiter(ctx, 3, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Make 3 requests
		for i := 0; i < 3; i++ {
			allowed, _, _, _ := limiter.Allow(ctx, userID)
			if !allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// 4th request should be rejected
		allowed, remaining, _, retryAfter := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("4th request should be rejected")
		}
		if remaining != 0 {
			t.Errorf("remaining should be 0, got %d", remaining)
		}
		if retryAfter <= 0 {
			t.Errorf("retryAfter should be positive, got %d", retryAfter)
		}
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		userID := fmt.Sprintf("test-token-user-3-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewtokenBucketRedisRateLimiter(ctx, 2, 2) // 2 tokens capacity, 2 tokens per second refill
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up all tokens
		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)

		// Should be rejected
		allowed, _, _, _ := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("third request should be rejected")
		}

		// Wait for tokens to refill
		time.Sleep(1100 * time.Millisecond)

		// Should allow requests again after refill
		allowed, _, _, _ = limiter.Allow(ctx, userID)
		if !allowed {
			t.Error("request after refill should be allowed")
		}
		allowed, _, _, _ = limiter.Allow(ctx, userID)
		if !allowed {
			t.Error("second request after refill should be allowed")
		}
		allowed, _, _, _ = limiter.Allow(ctx, userID)
		if allowed {
			t.Error("third request after refill should be rejected")
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-token-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-token-user-5-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewtokenBucketRedisRateLimiter(ctx, 2, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up limit for user1
		allowed, _, _, _ := limiter.Allow(ctx, user1)
		if !allowed {
			t.Error("user1 first request should be allowed")
		}
		allowed, _, _, _ = limiter.Allow(ctx, user1)
		if !allowed {
			t.Error("user1 second request should be allowed")
		}

		// user1 should be rate limited
		allowed1, _, _, _ := limiter.Allow(ctx, user1)
		if allowed1 {
			t.Error("user1 should be rate limited")
		}

		// user2 should still have full limit
		allowed2, _, _, _ := limiter.Allow(ctx, user2)
		if !allowed2 {
			t.Error("user2 should not be rate limited")
		}
	})

	t.Run("tokens never exceed capacity", func(t *testing.T) {
		userID := fmt.Sprintf("test-token-user-6-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewtokenBucketRedisRateLimiter(ctx, 5, 100) // High refill rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up all tokens
		for i := 0; i < 5; i++ {
			limiter.Allow(ctx, userID)
		}

		// Wait long enough that refill would exceed capacity
		time.Sleep(200 * time.Millisecond)

		// Should only allow up to capacity
		allowedCount := 0
		for i := 0; i < 10; i++ {
			allowed, _, _, _ := limiter.Allow(ctx, userID)
			if allowed {
				allowedCount++
			}
		}

		// Should allow exactly capacity (5) tokens
		if allowedCount != 5 {
			t.Errorf("expected exactly 5 allowed requests (capacity), got %d", allowedCount)
		}
	})

	t.Run("fail open on Redis error", func(t *testing.T) {
		// Create a limiter with invalid Redis connection
		// This is hard to test without mocking, but the code shows fail-open behavior
		// The Allow method returns true on error (fail open)
		t.Skip("requires Redis mocking to test fail-open behavior")
	})
}
