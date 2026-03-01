package goratelimit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krishna-kudari/ratelimit"
)

func TestNewSlidingWindowRateLimitter(t *testing.T) {
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
			limiter, err := goratelimit.NewSlidingWindowRateLimitter(tt.maxRequests, tt.windowSeconds)
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

func TestSlidingWindowRateLimiter_Allow(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindowRateLimitter(5, 60)
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
		limiter, err := goratelimit.NewSlidingWindowRateLimitter(3, 60)
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

	t.Run("sliding window removes old timestamps", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindowRateLimitter(2, 1) // 2 requests per second
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up the limit
		if !limiter.Allow() {
			t.Error("first request should be allowed")
		}
		if !limiter.Allow() {
			t.Error("second request should be allowed")
		}
		if limiter.Allow() {
			t.Error("third request should be rejected")
		}

		// Wait for window to slide (oldest timestamp expires)
		time.Sleep(1100 * time.Millisecond)

		// Should allow requests again as old timestamps are removed
		if !limiter.Allow() {
			t.Error("request after window slide should be allowed")
		}
		if !limiter.Allow() {
			t.Error("second request after window slide should be allowed")
		}
		if limiter.Allow() {
			t.Error("third request after window slide should be rejected")
		}
	})

	t.Run("sliding window allows gradual request flow", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindowRateLimitter(3, 2) // 3 requests per 2 seconds
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Make 3 requests with delays to ensure they expire at different times
		start := time.Now()
		for i := 0; i < 3; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i+1)
			}
			time.Sleep(200 * time.Millisecond) // Delay to ensure different timestamps
		}
		if limiter.Allow() {
			t.Error("4th request should be rejected")
		}

		// Wait for oldest request to expire
		// First request was at start, expires at start+2s
		// Wait until we're past that point
		elapsed := time.Since(start)
		if elapsed < 2100*time.Millisecond {
			time.Sleep(2100*time.Millisecond - elapsed)
		}

		// After oldest expires, should allow one more
		if !limiter.Allow() {
			t.Error("request after oldest expires should be allowed")
		}
		// Should be at limit again now
		if limiter.Allow() {
			t.Error("next request should be rejected (at limit)")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindowRateLimitter(100, 60)
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
}

func TestSlidingWindowRedisRateLimiter_Allow(t *testing.T) {
	// Skip if Redis is not available
	ctx := context.Background()
	limiter, err := goratelimit.NewSlidingWindowRedisRateLimiter(ctx, 10, 60)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("allows requests within limit", func(t *testing.T) {
		userID := "test-sliding-user-1"
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
		limiter, err := goratelimit.NewSlidingWindowRedisRateLimiter(ctx, 3, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Clear any existing data for this user (in case of previous test runs)
		// Note: This assumes we can access redis, but we can't from test package
		// So we'll use a unique userID with timestamp to avoid collisions
		userID := fmt.Sprintf("test-sliding-user-2-%d", time.Now().UnixNano())

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
		if retryAfter > 60 {
			t.Errorf("retryAfter should not exceed window, got %d", retryAfter)
		}
	})

	t.Run("sliding window removes old entries", func(t *testing.T) {
		userID := "test-sliding-user-3"
		limiter, err := goratelimit.NewSlidingWindowRedisRateLimiter(ctx, 2, 2) // 2 requests per 2 seconds
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up limit
		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)

		// Should be rejected
		allowed, _, _, _ := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("third request should be rejected")
		}

		// Wait for window to slide
		time.Sleep(2100 * time.Millisecond)

		// Should allow again as old entries expire
		allowed, _, _, _ = limiter.Allow(ctx, userID)
		if !allowed {
			t.Error("request after window slide should be allowed")
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		// Use unique user IDs to avoid collisions with previous test runs
		user1 := fmt.Sprintf("test-sliding-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-sliding-user-5-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewSlidingWindowRedisRateLimiter(ctx, 2, 60)
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
}
