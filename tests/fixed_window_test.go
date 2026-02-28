package goratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/krishna-kudari/ratelimit"
)

func TestNewFixedWindowRateLimitter(t *testing.T) {
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
			limiter, err := goratelimit.NewFixedWindowRateLimitter(tt.maxRequests, tt.windowSeconds)
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
				// Note: Can't access unexported fields from external test package
				// Verify limiter is not nil instead
				if limiter == nil {
					t.Error("limiter should not be nil")
				}
			}
		})
	}
}

func TestFixedWindowRateLimiter_Allow(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		limiter, err := goratelimit.NewFixedWindowRateLimitter(5, 60)
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
		limiter, err := goratelimit.NewFixedWindowRateLimitter(3, 60)
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

	t.Run("resets window after time expires", func(t *testing.T) {
		limiter, err := goratelimit.NewFixedWindowRateLimitter(2, 1) // 2 requests per second
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

		// Wait for window to expire
		time.Sleep(1100 * time.Millisecond)

		// Should allow requests again
		if !limiter.Allow() {
			t.Error("request after window expiry should be allowed")
		}
		if !limiter.Allow() {
			t.Error("second request after window expiry should be allowed")
		}
		if limiter.Allow() {
			t.Error("third request after window expiry should be rejected")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewFixedWindowRateLimitter(100, 60)
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

func TestRateLimitStore_Allow(t *testing.T) {
	t.Run("creates limiter for new user", func(t *testing.T) {
		// Note: RateLimitStore has unexported fields, so we can't construct it directly
		// This test would need a constructor function or be moved to same package
		t.Skip("RateLimitStore has unexported fields - needs constructor or same-package test")
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		t.Skip("RateLimitStore has unexported fields - needs constructor or same-package test")
		store := &goratelimit.RateLimitStore{
			// limiters field is unexported
		}

		// Use up limit for user1
		for i := 0; i < 100; i++ {
			store.Allow(context.Background(), "user1")
		}

		// user1 should be rate limited
		if store.Allow(context.Background(), "user1") {
			t.Error("user1 should be rate limited")
		}

		// user2 should still have full limit
		if !store.Allow(context.Background(), "user2") {
			t.Error("user2 should not be rate limited")
		}
	})

	t.Run("concurrent access to different users", func(t *testing.T) {
		t.Skip("RateLimitStore has unexported fields - needs constructor or same-package test")
		store := &goratelimit.RateLimitStore{
			// limiters field is unexported
		}

		done := make(chan bool, 2)
		go func() {
			for i := 0; i < 50; i++ {
				store.Allow(context.Background(), "user1")
			}
			done <- true
		}()
		go func() {
			for i := 0; i < 50; i++ {
				store.Allow(context.Background(), "user2")
			}
			done <- true
		}()

		<-done
		<-done

		// Both users should still have remaining requests
		if !store.Allow(context.Background(), "user1") {
			t.Error("user1 should have remaining requests")
		}
		if !store.Allow(context.Background(), "user2") {
			t.Error("user2 should have remaining requests")
		}
	})
}

func TestRedisRateLimiter_Allow(t *testing.T) {
	// Skip if Redis is not available
	ctx := context.Background()
	limiter, err := goratelimit.NewRedisRateLimiter(ctx, 10, 60)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("allows requests within limit", func(t *testing.T) {
		userID := "test-user-1"
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
		userID := "test-user-2"
		limiter, err := goratelimit.NewRedisRateLimiter(ctx, 3, 60)
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
		if retryAfter > 60 {
			t.Errorf("retryAfter should not exceed limit, got %d", retryAfter)
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := "test-user-3"
		user2 := "test-user-4"
		limiter, err := goratelimit.NewRedisRateLimiter(ctx, 2, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up limit for user1
		limiter.Allow(ctx, user1)
		limiter.Allow(ctx, user1)

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

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr))))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
