package goratelimit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krishna-kudari/ratelimit"
)

func TestNewGCRARateLimiter(t *testing.T) {
	tests := []struct {
		name           string
		rate           int64
		burst          int64
		expectError    bool
		errorSubstring string
	}{
		{
			name:        "valid parameters",
			rate:        10,
			burst:       20,
			expectError: false,
		},
		{
			name:           "zero rate",
			rate:           0,
			burst:          20,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative rate",
			rate:           -1,
			burst:          20,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "zero burst",
			rate:           10,
			burst:          0,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative burst",
			rate:           10,
			burst:          -1,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:        "burst equals rate",
			rate:        10,
			burst:       10,
			expectError: false,
		},
		{
			name:        "burst greater than rate",
			rate:        10,
			burst:       30,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := goratelimit.NewGCRARateLimiter(tt.rate, tt.burst)
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

func TestGCRARateLimiter_Allow(t *testing.T) {
	t.Run("allows requests within burst", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRARateLimiter(10, 5) // 10 per second, burst of 5
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should allow up to burst (5 requests)
		for i := 0; i < 5; i++ {
			allowed, remaining, retryAfter := limiter.Allow()
			if !allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
			if remaining < 0 {
				t.Errorf("remaining should be non-negative, got %d", remaining)
			}
			if retryAfter != 0 {
				t.Errorf("retryAfter should be 0 when allowed, got %f", retryAfter)
			}
		}
	})

	t.Run("rejects requests exceeding burst", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRARateLimiter(10, 3) // 10 per second, burst of 3
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up the burst
		for i := 0; i < 3; i++ {
			allowed, _, _ := limiter.Allow()
			if !allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// 4th request should be rejected
		allowed, remaining, retryAfter := limiter.Allow()
		if allowed {
			t.Error("4th request should be rejected")
		}
		if remaining != 0 {
			t.Errorf("remaining should be 0 when rejected, got %d", remaining)
		}
		if retryAfter <= 0 {
			t.Errorf("retryAfter should be positive when rejected, got %f", retryAfter)
		}
	})

	t.Run("allows requests after rate limit period", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRARateLimiter(2, 2) // 2 per second, burst of 2
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up the burst
		if allowed, _, _ := limiter.Allow(); !allowed {
			t.Error("first request should be allowed")
		}
		if allowed, _, _ := limiter.Allow(); !allowed {
			t.Error("second request should be allowed")
		}
		if allowed, _, _ := limiter.Allow(); allowed {
			t.Error("third request should be rejected")
		}

		// Wait for rate limit period (0.5 seconds = 1/rate)
		time.Sleep(600 * time.Millisecond)

		// Should allow requests again after rate limit period
		allowed, _, _ := limiter.Allow()
		if !allowed {
			t.Error("request after rate limit period should be allowed")
		}
	})

	t.Run("allows steady rate of requests", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRARateLimiter(10, 10) // 10 per second, burst of 10
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Make requests at steady rate (should allow all)
		for i := 0; i < 10; i++ {
			allowed, _, _ := limiter.Allow()
			if !allowed {
				t.Errorf("request %d should be allowed at steady rate", i+1)
			}
			time.Sleep(100 * time.Millisecond) // 100ms = 0.1s, which is less than 1/10s = 0.1s
		}
	})

	t.Run("remaining count decreases as requests are made", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRARateLimiter(10, 5) // 10 per second, burst of 5
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		prevRemaining := int64(5)
		for i := 0; i < 5; i++ {
			_, remaining, _ := limiter.Allow()
			if remaining >= prevRemaining {
				t.Errorf("remaining should decrease, got %d (previous: %d)", remaining, prevRemaining)
			}
			prevRemaining = remaining
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRARateLimiter(100, 50) // 100 per second, burst of 50
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		allowed := make(chan bool, 100)
		for i := 0; i < 100; i++ {
			go func() {
				allowedVal, _, _ := limiter.Allow()
				allowed <- allowedVal
			}()
		}

		count := 0
		for i := 0; i < 100; i++ {
			if <-allowed {
				count++
			}
		}

		// Should allow up to burst (50) requests
		if count > 50 {
			t.Errorf("expected at most 50 allowed requests (burst), got %d", count)
		}
		if count < 50 {
			t.Errorf("expected at least 50 allowed requests (burst), got %d", count)
		}
	})

	t.Run("allows burst after waiting", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRARateLimiter(5, 3) // 5 per second, burst of 3
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Make initial requests - should allow at least some
		initialAllowed := 0
		for i := 0; i < 5; i++ {
			allowed, _, _ := limiter.Allow()
			if allowed {
				initialAllowed++
			} else {
				break // Stop when we hit the limit
			}
		}
		
		// Should allow at least 1 request
		if initialAllowed < 1 {
			t.Fatalf("expected at least 1 request to be allowed initially, got %d", initialAllowed)
		}

		// Try to get a rejection by making more requests
		var retryAfter float64
		gotRejection := false
		for i := 0; i < 3; i++ {
			allowed, _, retryAfterVal := limiter.Allow()
			if !allowed && retryAfterVal > 0 {
				retryAfter = retryAfterVal
				gotRejection = true
				break
			}
		}

		if gotRejection {
			// If rejected, wait for retryAfter
			waitTime := time.Duration(retryAfter*1000) * time.Millisecond
			if waitTime < 200*time.Millisecond {
				waitTime = 200 * time.Millisecond // Minimum wait for emission interval
			}
			time.Sleep(waitTime + 50*time.Millisecond)
		} else {
			// If no rejection yet, wait for emission interval anyway
			time.Sleep(250 * time.Millisecond)
		}

		// Should allow requests after waiting
		allowed, _, _ := limiter.Allow()
		if !allowed {
			t.Error("request should be allowed after waiting emission interval")
		}
	})

	t.Run("retryAfter is calculated correctly", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRARateLimiter(10, 2) // 10 per second, burst of 2
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up the burst
		limiter.Allow()
		limiter.Allow()

		// Next request should be rejected with retryAfter
		_, _, retryAfter := limiter.Allow()
		if retryAfter <= 0 {
			t.Errorf("retryAfter should be positive, got %f", retryAfter)
		}
		// retryAfter uses math.Ceil, so 0.2 seconds becomes 1.0 seconds
		// The actual wait time needed is approximately 0.1 seconds (1/rate),
		// but retryAfter is rounded up to the nearest integer second
		if retryAfter < 0.1 || retryAfter > 2.0 {
			t.Errorf("retryAfter should be between 0.1 and 2.0 seconds (rounded up), got %f", retryAfter)
		}
	})
}

func TestNewGCRARedisRateLimiter(t *testing.T) {
	ctx := context.Background()

	t.Run("valid parameters", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRARedisRateLimiter(ctx, 10, 20)
		if err != nil {
			t.Skipf("Redis not available: %v", err)
		}
		if limiter == nil {
			t.Error("expected limiter to be non-nil")
		}
	})

	t.Run("invalid parameters - zero rate", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRARedisRateLimiter(ctx, 0, 20)
		if err == nil {
			t.Error("expected error for zero rate")
		}
		if limiter != nil {
			t.Error("expected limiter to be nil on error")
		}
	})

	t.Run("invalid parameters - zero burst", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRARedisRateLimiter(ctx, 10, 0)
		if err == nil {
			t.Error("expected error for zero burst")
		}
		if limiter != nil {
			t.Error("expected limiter to be nil on error")
		}
	})
}

func TestGCRARedisRateLimiter_Allow(t *testing.T) {
	ctx := context.Background()
	limiter, err := goratelimit.NewGCRARedisRateLimiter(ctx, 10, 20)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("allows requests within burst", func(t *testing.T) {
		userID := fmt.Sprintf("test-gcra-user-1-%d", time.Now().UnixNano())
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

	t.Run("rejects requests exceeding burst", func(t *testing.T) {
		userID := fmt.Sprintf("test-gcra-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRARedisRateLimiter(ctx, 10, 3) // 10 per second, burst of 3
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up the burst
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

	t.Run("allows requests after rate limit period", func(t *testing.T) {
		userID := fmt.Sprintf("test-gcra-user-3-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRARedisRateLimiter(ctx, 2, 2) // 2 per second, burst of 2
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up the burst
		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)

		// Should be rejected
		allowed, _, _, _ := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("third request should be rejected")
		}

		// Wait for rate limit period (0.5 seconds = 1/rate)
		time.Sleep(600 * time.Millisecond)

		// Should allow requests again after rate limit period
		allowed, _, _, _ = limiter.Allow(ctx, userID)
		if !allowed {
			t.Error("request after rate limit period should be allowed")
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-gcra-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-gcra-user-5-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRARedisRateLimiter(ctx, 10, 2) // 10 per second, burst of 2
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

	t.Run("allows steady rate of requests", func(t *testing.T) {
		userID := fmt.Sprintf("test-gcra-user-6-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRARedisRateLimiter(ctx, 10, 10) // 10 per second, burst of 10
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Make requests at steady rate (should allow all)
		for i := 0; i < 10; i++ {
			allowed, _, _, _ := limiter.Allow(ctx, userID)
			if !allowed {
				t.Errorf("request %d should be allowed at steady rate", i+1)
			}
			time.Sleep(100 * time.Millisecond) // 100ms spacing
		}
	})

	t.Run("remaining count decreases as requests are made", func(t *testing.T) {
		userID := fmt.Sprintf("test-gcra-user-7-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRARedisRateLimiter(ctx, 10, 5) // 10 per second, burst of 5
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		prevRemaining := int64(5)
		for i := 0; i < 5; i++ {
			_, remaining, _, _ := limiter.Allow(ctx, userID)
			if remaining >= prevRemaining {
				t.Errorf("remaining should decrease, got %d (previous: %d)", remaining, prevRemaining)
			}
			prevRemaining = remaining
		}
	})

	t.Run("allows burst after waiting", func(t *testing.T) {
		userID := fmt.Sprintf("test-gcra-user-8-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRARedisRateLimiter(ctx, 5, 3) // 5 per second, burst of 3
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up the burst
		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)

		// Should be rejected
		allowed, _, _, _ := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("request should be rejected after burst")
		}

		// Wait for emission interval (1/5 = 0.2 seconds)
		time.Sleep(250 * time.Millisecond)

		// Should allow one more request
		allowed, _, _, _ = limiter.Allow(ctx, userID)
		if !allowed {
			t.Error("request should be allowed after waiting emission interval")
		}
	})

	t.Run("retryAfter is calculated correctly", func(t *testing.T) {
		userID := fmt.Sprintf("test-gcra-user-9-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRARedisRateLimiter(ctx, 10, 2) // 10 per second, burst of 2
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up the burst
		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)

		// Next request should be rejected with retryAfter
		_, _, _, retryAfter := limiter.Allow(ctx, userID)
		if retryAfter <= 0 {
			t.Errorf("retryAfter should be positive, got %d", retryAfter)
		}
		// retryAfter should be approximately 0.1 seconds (1/rate), but as integer seconds
		if retryAfter > 1 {
			t.Errorf("retryAfter should be approximately 1 second (rounded up), got %d", retryAfter)
		}
	})

	t.Run("fail open on Redis error", func(t *testing.T) {
		// The code shows fail-open behavior (returns true on error)
		// This is hard to test without mocking Redis connection
		t.Skip("requires Redis mocking to test fail-open behavior")
	})
}
