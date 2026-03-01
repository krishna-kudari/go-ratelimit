package goratelimit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krishna-kudari/ratelimit"
)

func TestNewSlidingWindowCounterRateLimitter(t *testing.T) {
	tests := []struct {
		name           string
		maxRequests    int64
		windowSeconds  int64
		expectError    bool
		errorSubstring string
	}{
		{
			name:          "valid parameters",
			maxRequests:   10,
			windowSeconds: 60,
			expectError:   false,
		},
		{
			name:           "zero max requests",
			maxRequests:    0,
			windowSeconds:  60,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative max requests",
			maxRequests:    -1,
			windowSeconds:  60,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "zero window seconds",
			maxRequests:    10,
			windowSeconds:  0,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative window seconds",
			maxRequests:    10,
			windowSeconds:  -1,
			expectError:    true,
			errorSubstring: "must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := goratelimit.NewslidingWindowCounterRateLimitter(tt.maxRequests, tt.windowSeconds)
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

func TestSlidingWindowCounterRateLimiter_Allow(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		limiter, err := goratelimit.NewslidingWindowCounterRateLimitter(5, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 5; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i+1)
			}
		}
	})

	t.Run("rejects requests exceeding limit", func(t *testing.T) {
		limiter, err := goratelimit.NewslidingWindowCounterRateLimitter(3, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Allow 3 requests
		for i := 0; i < 3; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// 4th request should be rejected
		if limiter.Allow() {
			t.Error("4th request should be rejected")
		}
	})

	t.Run("sliding window counter weights previous window", func(t *testing.T) {
		limiter, err := goratelimit.NewslidingWindowCounterRateLimitter(10, 2) // 10 requests per 2 seconds
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up most of the limit in current window
		for i := 0; i < 8; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// Wait for window to slide (2 seconds)
		time.Sleep(2100 * time.Millisecond)

		// Previous window had 8 requests, current window starts fresh
		// Should be able to make requests in new window
		for i := 0; i < 2; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d in new window should be allowed", i+1)
			}
		}
	})

	t.Run("sliding window counter gradually allows requests as previous window expires", func(t *testing.T) {
		limiter, err := goratelimit.NewslidingWindowCounterRateLimitter(10, 2) // 10 requests per 2 seconds
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fill up the limit
		for i := 0; i < 10; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// Should be rejected
		if limiter.Allow() {
			t.Error("11th request should be rejected")
		}

		// Wait for window to slide (2 seconds)
		// After slide: previousCount = 10, currentCount = 0, windowStart = now
		time.Sleep(2100 * time.Millisecond)
		
		// Immediately after slide: elapsedFraction = 0, prevCount = 10 * 1 = 10
		// count = 10 + 0 = 10, which is at limit, so will reject
		// Wait a small amount to let elapsedFraction increase
		time.Sleep(300 * time.Millisecond)
		
		// Now elapsedFraction ≈ 0.15, prevCount = 10 * 0.85 = 8.5
		// count = 8.5 + 0 = 8.5, so we can allow some requests (8.5 < 10)
		allowed := 0
		for i := 0; i < 3; i++ {
			if limiter.Allow() {
				allowed++
			} else {
				break
			}
		}
		if allowed == 0 {
			t.Error("should allow some requests as previous window weight decreases")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewslidingWindowCounterRateLimitter(100, 60)
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

		if count != 100 {
			t.Errorf("expected exactly 100 allowed requests, got %d", count)
		}
	})

	t.Run("window reset behavior", func(t *testing.T) {
		limiter, err := goratelimit.NewslidingWindowCounterRateLimitter(5, 1) // 5 requests per second
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up limit
		for i := 0; i < 5; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i+1)
			}
		}
		if limiter.Allow() {
			t.Error("6th request should be rejected")
		}

		// Wait for window to slide (1 second)
		// After slide: previousCount = 5, currentCount = 0, windowStart = now
		time.Sleep(1100 * time.Millisecond)

		// At start of new window: elapsedFraction = 0, prevCount = 5 * 1 = 5
		// count = 5 + 0 = 5, which is at limit (5 < 5 is false), so should reject
		// Wait a small amount to let elapsedFraction increase
		time.Sleep(200 * time.Millisecond)

		// Now elapsedFraction ≈ 0.2, prevCount = 5 * (1 - 0.2) = 4.0
		// count = 4.0 + 0 = 4.0, so should allow (4.0 < 5 is true)
		if !limiter.Allow() {
			t.Error("request after previous window weight decreases should be allowed")
		}
	})
}

func TestSlidingWindowCounterRedisRateLimiter_Allow(t *testing.T) {
	// Skip if Redis is not available
	ctx := context.Background()
	limiter, err := goratelimit.NewslidingWindowCounterRedisRateLimiter(ctx, 10, 60)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("allows requests within limit", func(t *testing.T) {
		userID := fmt.Sprintf("test-counter-user-1-%d", time.Now().UnixNano())
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
		userID := fmt.Sprintf("test-counter-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewslidingWindowCounterRedisRateLimiter(ctx, 3, 60)
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
		// Note: The sliding window counter uses estimated count which includes weighted previous window
		// If previous window had 0 and current window has 3, estimatedCount = 0 + 3 = 3
		// Since 3 >= 3, it should be rejected
		allowed, remaining, _, retryAfter := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("4th request should be rejected")
		}
		if remaining != 0 {
			t.Errorf("remaining should be 0, got %d", remaining)
		}
		// retryAfter might be 0 if we're at the start of a new window, so allow 0 or positive
		if retryAfter < 0 {
			t.Errorf("retryAfter should be non-negative, got %d", retryAfter)
		}
		if retryAfter > 60 {
			t.Errorf("retryAfter should not exceed window, got %d", retryAfter)
		}
	})

	t.Run("sliding window counter weights previous window", func(t *testing.T) {
		userID := fmt.Sprintf("test-counter-user-3-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewslidingWindowCounterRedisRateLimiter(ctx, 10, 2) // 10 requests per 2 seconds
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fill up the limit
		for i := 0; i < 10; i++ {
			allowed, _, _, _ := limiter.Allow(ctx, userID)
			if !allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// Should be rejected (estimatedCount = 0 + 10 = 10, which >= 10)
		allowed, _, _, _ := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("11th request should be rejected")
		}

		// Wait for window to slide (2 seconds) plus additional time to be well into the new window
		// We need elapsed > 0 so that weightedPrev < 10
		// The weighted previous count decreases as we progress through the new window
		time.Sleep(2100 * time.Millisecond) // Wait for window to slide

		// Try multiple times with increasing wait periods
		// As elapsed increases, weightedPrev decreases: weightedPrev = 10 * (1 - elapsed)
		// We need elapsed such that weightedPrev < 10, i.e., elapsed > 0
		maxAttempts := 5
		allowed = false
		for i := 0; i < maxAttempts && !allowed; i++ {
			time.Sleep(300 * time.Millisecond) // Wait a bit more each iteration
			allowed, _, _, _ = limiter.Allow(ctx, userID)
		}
		if !allowed {
			t.Error("request in new window should eventually be allowed as previous window weight decreases")
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-counter-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-counter-user-5-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewslidingWindowCounterRedisRateLimiter(ctx, 2, 60)
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

		// user1 should be rate limited (estimatedCount = 0 + 2 = 2, which >= 2)
		allowed1, _, _, _ := limiter.Allow(ctx, user1)
		if allowed1 {
			t.Error("user1 should be rate limited")
		}

		// user2 should still have full limit (different key)
		allowed2, _, _, _ := limiter.Allow(ctx, user2)
		if !allowed2 {
			t.Error("user2 should not be rate limited")
		}
	})

	t.Run("gradual request allowance as window slides", func(t *testing.T) {
		userID := fmt.Sprintf("test-counter-user-6-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewslidingWindowCounterRedisRateLimiter(ctx, 10, 2) // 10 requests per 2 seconds
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fill up the limit
		for i := 0; i < 10; i++ {
			limiter.Allow(ctx, userID)
		}

		// Should be rejected
		allowed, _, _, _ := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("11th request should be rejected")
		}

		// Wait for window to slide first (2 seconds)
		time.Sleep(2100 * time.Millisecond)

		// Now wait 1 second into the new window (halfway through)
		// Previous window had 10, current window has 0
		// elapsed = 0.5, weightedPrev = 10 * (1 - 0.5) = 5
		// estimatedCount = 5 + 0 = 5, so we can allow requests (5 < 10)
		time.Sleep(1100 * time.Millisecond)

		// Should allow some requests as previous window weight decreases
		allowedCount := 0
		for i := 0; i < 6; i++ {
			allowed, _, _, _ := limiter.Allow(ctx, userID)
			if allowed {
				allowedCount++
			} else {
				break // Stop if we hit the limit
			}
		}
		if allowedCount == 0 {
			t.Error("should allow some requests as previous window weight decreases")
		}
		if allowedCount < 1 {
			t.Errorf("should allow at least 1 request, got %d", allowedCount)
		}
	})
}
