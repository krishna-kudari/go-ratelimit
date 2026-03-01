package goratelimit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krishna-kudari/ratelimit"
)

func TestNewLeakyBucketRateLimiter(t *testing.T) {
	tests := []struct {
		name           string
		capacity       int64
		leakRate       int64
		mode           goratelimit.LeakyBucketMode
		expectError    bool
		errorSubstring string
	}{
		{
			name:        "valid parameters - policing mode",
			capacity:    10,
			leakRate:    60,
			mode:        goratelimit.Policing,
			expectError: false,
		},
		{
			name:        "valid parameters - shaping mode",
			capacity:    10,
			leakRate:    60,
			mode:        goratelimit.Shaping,
			expectError: false,
		},
		{
			name:           "zero capacity",
			capacity:       0,
			leakRate:       60,
			mode:           goratelimit.Policing,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative capacity",
			capacity:       -1,
			leakRate:       60,
			mode:           goratelimit.Policing,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "zero leak rate",
			capacity:       10,
			leakRate:       0,
			mode:           goratelimit.Policing,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative leak rate",
			capacity:       10,
			leakRate:       -1,
			mode:           goratelimit.Policing,
			expectError:    true,
			errorSubstring: "must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := goratelimit.NewLeakyBucketRateLimiter(tt.capacity, tt.leakRate, tt.mode)
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

func TestLeakyBucketRateLimiter_Allow_Policing(t *testing.T) {
	t.Run("allows requests within capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(5, 60, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 5; i++ {
			allowed, remaining, delay := limiter.Allow()
			if !allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
			if remaining < 0 {
				t.Errorf("remaining should be non-negative, got %d", remaining)
			}
			if delay != 0 {
				t.Errorf("delay should be 0 when allowed, got %f", delay)
			}
		}
	})

	t.Run("rejects requests when bucket is full", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(3, 60, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fill up the bucket
		for i := 0; i < 3; i++ {
			allowed, _, _ := limiter.Allow()
			if !allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// 4th request should be rejected
		allowed, remaining, delay := limiter.Allow()
		if allowed {
			t.Error("4th request should be rejected")
		}
		if remaining != 0 {
			t.Errorf("remaining should be 0 when rejected, got %d", remaining)
		}
		if delay <= 0 {
			t.Errorf("delay should be positive when rejected, got %f", delay)
		}
	})

	t.Run("leaks tokens over time", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(2, 2, goratelimit.Policing) // 2 capacity, 2 per second leak rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fill up the bucket
		if allowed, _, _ := limiter.Allow(); !allowed {
			t.Error("first request should be allowed")
		}
		if allowed, _, _ := limiter.Allow(); !allowed {
			t.Error("second request should be allowed")
		}
		if allowed, _, _ := limiter.Allow(); allowed {
			t.Error("third request should be rejected")
		}

		// Wait for tokens to leak (1 second should leak 2 tokens)
		time.Sleep(1100 * time.Millisecond)

		// Should allow requests again after leak
		allowed, _, _ := limiter.Allow()
		if !allowed {
			t.Error("request after leak should be allowed")
		}
		allowed, _, _ = limiter.Allow()
		if !allowed {
			t.Error("second request after leak should be allowed")
		}
		allowed, _, _ = limiter.Allow()
		if allowed {
			t.Error("third request after leak should be rejected")
		}
	})

	t.Run("gradual leak allows steady request flow", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(10, 10, goratelimit.Policing) // 10 capacity, 10 per second leak rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fill up the bucket
		for i := 0; i < 10; i++ {
			if allowed, _, _ := limiter.Allow(); !allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}
		if allowed, _, _ := limiter.Allow(); allowed {
			t.Error("11th request should be rejected")
		}

		// Wait for partial leak (0.1 second = 1 token)
		time.Sleep(110 * time.Millisecond)

		// Should allow one more request
		allowed, _, _ := limiter.Allow()
		if !allowed {
			t.Error("request after partial leak should be allowed")
		}
		allowed, _, _ = limiter.Allow()
		if allowed {
			t.Error("next request should be rejected (only 1 token leaked)")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(100, 60, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		allowed := make(chan bool, 200)
		for i := 0; i < 200; i++ {
			go func() {
				allowedVal, _, _ := limiter.Allow()
				allowed <- allowedVal
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

	t.Run("remaining count decreases as bucket fills", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(5, 60, goratelimit.Policing)
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

	t.Run("level never exceeds capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(5, 100, goratelimit.Policing) // High leak rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fill up the bucket
		for i := 0; i < 5; i++ {
			limiter.Allow()
		}

		// Wait long enough that leak would exceed capacity
		time.Sleep(200 * time.Millisecond)

		// Should only allow up to capacity, not more
		allowedCount := 0
		for i := 0; i < 10; i++ {
			allowed, _, _ := limiter.Allow()
			if allowed {
				allowedCount++
			}
		}

		// Should allow exactly capacity (5) tokens
		if allowedCount != 5 {
			t.Errorf("expected exactly 5 allowed requests (capacity), got %d", allowedCount)
		}
	})
}

func TestLeakyBucketRateLimiter_Allow_Shaping(t *testing.T) {
	t.Run("allows requests within capacity with delay", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(5, 5, goratelimit.Shaping) // 5 capacity, 5 per second leak rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// First request should have no delay
		allowed, remaining, delay := limiter.Allow()
		if !allowed {
			t.Error("first request should be allowed")
		}
		if delay != 0 {
			t.Errorf("first request should have no delay, got %f", delay)
		}
		if remaining < 0 {
			t.Errorf("remaining should be non-negative, got %d", remaining)
		}

		// Subsequent requests should have increasing delays
		prevDelay := delay
		for i := 0; i < 4; i++ {
			allowed, remaining, delay := limiter.Allow()
			if !allowed {
				t.Errorf("request %d should be allowed", i+2)
			}
			if delay <= prevDelay {
				t.Errorf("delay should increase, got %f (previous: %f)", delay, prevDelay)
			}
			if remaining < 0 {
				t.Errorf("remaining should be non-negative, got %d", remaining)
			}
			prevDelay = delay
		}
	})

	t.Run("rejects requests when queue is full", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(3, 60, goratelimit.Shaping)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fill up the queue
		for i := 0; i < 3; i++ {
			allowed, _, _ := limiter.Allow()
			if !allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// 4th request should be rejected
		allowed, remaining, delay := limiter.Allow()
		if allowed {
			t.Error("4th request should be rejected")
		}
		if remaining != 0 {
			t.Errorf("remaining should be 0 when rejected, got %d", remaining)
		}
		if delay != 0 {
			t.Errorf("delay should be 0 when rejected, got %f", delay)
		}
	})

	t.Run("delays requests based on queue depth", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(5, 5, goratelimit.Shaping) // 5 capacity, 5 per second leak rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// First request: no delay
		_, _, delay1 := limiter.Allow()
		if delay1 != 0 {
			t.Errorf("first request should have no delay, got %f", delay1)
		}

		// Second request: should have delay of ~0.2 seconds (1/5)
		_, _, delay2 := limiter.Allow()
		expectedDelay := 1.0 / 5.0
		if delay2 < expectedDelay*0.9 || delay2 > expectedDelay*1.1 {
			t.Errorf("expected delay ~%f, got %f", expectedDelay, delay2)
		}

		// Third request: should have delay of ~0.4 seconds (2/5)
		_, _, delay3 := limiter.Allow()
		expectedDelay = 2.0 / 5.0
		if delay3 < expectedDelay*0.9 || delay3 > expectedDelay*1.1 {
			t.Errorf("expected delay ~%f, got %f", expectedDelay, delay3)
		}
	})

	t.Run("queue drains over time", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(2, 2, goratelimit.Shaping) // 2 capacity, 2 per second leak rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fill up the queue
		if allowed, _, _ := limiter.Allow(); !allowed {
			t.Error("first request should be allowed")
		}
		if allowed, _, _ := limiter.Allow(); !allowed {
			t.Error("second request should be allowed")
		}
		if allowed, _, _ := limiter.Allow(); allowed {
			t.Error("third request should be rejected")
		}

		// Wait for queue to drain (1 second should drain 2 requests)
		time.Sleep(1100 * time.Millisecond)

		// Should allow requests again after drain
		allowed, _, delay := limiter.Allow()
		if !allowed {
			t.Error("request after drain should be allowed")
		}
		if delay != 0 {
			t.Errorf("delay should be 0 after drain, got %f", delay)
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(100, 60, goratelimit.Shaping)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		allowed := make(chan bool, 200)
		for i := 0; i < 200; i++ {
			go func() {
				allowedVal, _, _ := limiter.Allow()
				allowed <- allowedVal
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

	t.Run("remaining count decreases as queue fills", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRateLimiter(5, 60, goratelimit.Shaping)
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
}

func TestNewLeakyBucketRedisRateLimiter(t *testing.T) {
	ctx := context.Background()

	t.Run("valid parameters - policing mode", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 10, 60, goratelimit.Policing)
		if err != nil {
			t.Skipf("Redis not available: %v", err)
		}
		if limiter == nil {
			t.Error("expected limiter to be non-nil")
		}
	})

	t.Run("valid parameters - shaping mode", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 10, 60, goratelimit.Shaping)
		if err != nil {
			t.Skipf("Redis not available: %v", err)
		}
		if limiter == nil {
			t.Error("expected limiter to be non-nil")
		}
	})

	t.Run("invalid parameters - zero capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 0, 60, goratelimit.Policing)
		if err == nil {
			t.Error("expected error for zero capacity")
		}
		if limiter != nil {
			t.Error("expected limiter to be nil on error")
		}
	})

	t.Run("invalid parameters - zero leak rate", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 10, 0, goratelimit.Policing)
		if err == nil {
			t.Error("expected error for zero leak rate")
		}
		if limiter != nil {
			t.Error("expected limiter to be nil on error")
		}
	})
}

func TestLeakyBucketRedisRateLimiter_Allow_Policing(t *testing.T) {
	ctx := context.Background()
	limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 10, 60, goratelimit.Policing)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("allows requests within limit", func(t *testing.T) {
		userID := fmt.Sprintf("test-leaky-policing-user-1-%d", time.Now().UnixNano())
		allowed, remaining, limit, retryAfter, delay := limiter.Allow(ctx, userID)
		if !allowed {
			t.Error("first request should be allowed")
		}
		if remaining < 0 || remaining > limit {
			t.Errorf("remaining should be between 0 and %d, got %d", limit, remaining)
		}
		if retryAfter != 0 {
			t.Errorf("retryAfter should be 0 when allowed, got %d", retryAfter)
		}
		if delay != 0 {
			t.Errorf("delay should be 0 when allowed, got %f", delay)
		}
	})

	t.Run("rejects requests exceeding limit", func(t *testing.T) {
		userID := fmt.Sprintf("test-leaky-policing-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 3, 60, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Make 3 requests
		for i := 0; i < 3; i++ {
			allowed, _, _, _, _ := limiter.Allow(ctx, userID)
			if !allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// 4th request should be rejected
		allowed, remaining, _, retryAfter, delay := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("4th request should be rejected")
		}
		if remaining != 0 {
			t.Errorf("remaining should be 0, got %d", remaining)
		}
		if retryAfter <= 0 {
			t.Errorf("retryAfter should be positive, got %d", retryAfter)
		}
		if delay != 0 {
			t.Errorf("delay should be 0 for policing mode when rejected, got %f", delay)
		}
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		userID := fmt.Sprintf("test-leaky-policing-user-3-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 2, 2, goratelimit.Policing) // 2 capacity, 2 per second leak rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up all tokens
		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)

		// Should be rejected
		allowed, _, _, _, _ := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("third request should be rejected")
		}

		// Wait for tokens to leak
		time.Sleep(1100 * time.Millisecond)

		// Should allow requests again after leak
		allowed, _, _, _, _ = limiter.Allow(ctx, userID)
		if !allowed {
			t.Error("request after leak should be allowed")
		}
		allowed, _, _, _, _ = limiter.Allow(ctx, userID)
		if !allowed {
			t.Error("second request after leak should be allowed")
		}
		allowed, _, _, _, _ = limiter.Allow(ctx, userID)
		if allowed {
			t.Error("third request after leak should be rejected")
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-leaky-policing-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-leaky-policing-user-5-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 2, 60, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up limit for user1
		allowed, _, _, _, _ := limiter.Allow(ctx, user1)
		if !allowed {
			t.Error("user1 first request should be allowed")
		}
		allowed, _, _, _, _ = limiter.Allow(ctx, user1)
		if !allowed {
			t.Error("user1 second request should be allowed")
		}

		// user1 should be rate limited
		allowed1, _, _, _, _ := limiter.Allow(ctx, user1)
		if allowed1 {
			t.Error("user1 should be rate limited")
		}

		// user2 should still have full limit
		allowed2, _, _, _, _ := limiter.Allow(ctx, user2)
		if !allowed2 {
			t.Error("user2 should not be rate limited")
		}
	})

	t.Run("tokens never exceed capacity", func(t *testing.T) {
		userID := fmt.Sprintf("test-leaky-policing-user-6-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 5, 100, goratelimit.Policing) // High leak rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up all tokens
		for i := 0; i < 5; i++ {
			limiter.Allow(ctx, userID)
		}

		// Wait long enough that leak would exceed capacity
		time.Sleep(200 * time.Millisecond)

		// Should only allow up to capacity
		allowedCount := 0
		for i := 0; i < 10; i++ {
			allowed, _, _, _, _ := limiter.Allow(ctx, userID)
			if allowed {
				allowedCount++
			}
		}

		// Should allow exactly capacity (5) tokens
		if allowedCount != 5 {
			t.Errorf("expected exactly 5 allowed requests (capacity), got %d", allowedCount)
		}
	})
}

func TestLeakyBucketRedisRateLimiter_Allow_Shaping(t *testing.T) {
	ctx := context.Background()
	limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 10, 60, goratelimit.Shaping)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("allows requests within limit with delay", func(t *testing.T) {
		userID := fmt.Sprintf("test-leaky-shaping-user-1-%d", time.Now().UnixNano())
		allowed, remaining, limit, retryAfter, delay := limiter.Allow(ctx, userID)
		if !allowed {
			t.Error("first request should be allowed")
		}
		if remaining < 0 || remaining > limit {
			t.Errorf("remaining should be between 0 and %d, got %d", limit, remaining)
		}
		if retryAfter != 0 {
			t.Errorf("retryAfter should be 0 when allowed, got %d", retryAfter)
		}
		// First request may have no delay or minimal delay
		if delay < 0 {
			t.Errorf("delay should be non-negative, got %f", delay)
		}
	})

	t.Run("rejects requests when queue is full", func(t *testing.T) {
		userID := fmt.Sprintf("test-leaky-shaping-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 3, 60, goratelimit.Shaping)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fill up the queue
		for i := 0; i < 3; i++ {
			allowed, _, _, _, _ := limiter.Allow(ctx, userID)
			if !allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// 4th request should be rejected
		allowed, remaining, _, retryAfter, delay := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("4th request should be rejected")
		}
		if remaining != 0 {
			t.Errorf("remaining should be 0, got %d", remaining)
		}
		if retryAfter != 0 {
			t.Errorf("retryAfter should be 0 for shaping mode when rejected, got %d", retryAfter)
		}
		if delay != 0 {
			t.Errorf("delay should be 0 when rejected, got %f", delay)
		}
	})

	t.Run("delays requests based on queue depth", func(t *testing.T) {
		userID := fmt.Sprintf("test-leaky-shaping-user-3-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 5, 5, goratelimit.Shaping) // 5 capacity, 5 per second leak rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// First request: should have minimal or no delay
		_, _, _, _, delay1 := limiter.Allow(ctx, userID)
		if delay1 < 0 {
			t.Errorf("delay should be non-negative, got %f", delay1)
		}

		// Second request: should have delay
		_, _, _, _, delay2 := limiter.Allow(ctx, userID)
		if delay2 <= delay1 {
			t.Errorf("delay should increase, got %f (previous: %f)", delay2, delay1)
		}
	})

	t.Run("queue drains over time", func(t *testing.T) {
		userID := fmt.Sprintf("test-leaky-shaping-user-4-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 2, 2, goratelimit.Shaping) // 2 capacity, 2 per second leak rate
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fill up the queue
		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)

		// Should be rejected
		allowed, _, _, _, _ := limiter.Allow(ctx, userID)
		if allowed {
			t.Error("third request should be rejected")
		}

		// Wait for queue to drain
		time.Sleep(1100 * time.Millisecond)

		// Should allow requests again after drain
		allowed, _, _, _, delay := limiter.Allow(ctx, userID)
		if !allowed {
			t.Error("request after drain should be allowed")
		}
		if delay < 0 {
			t.Errorf("delay should be non-negative, got %f", delay)
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-leaky-shaping-user-5-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-leaky-shaping-user-6-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewLeakyBucketRedisRateLimiter(ctx, 2, 60, goratelimit.Shaping)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Use up limit for user1
		allowed, _, _, _, _ := limiter.Allow(ctx, user1)
		if !allowed {
			t.Error("user1 first request should be allowed")
		}
		allowed, _, _, _, _ = limiter.Allow(ctx, user1)
		if !allowed {
			t.Error("user1 second request should be allowed")
		}

		// user1 should be rate limited
		allowed1, _, _, _, _ := limiter.Allow(ctx, user1)
		if allowed1 {
			t.Error("user1 should be rate limited")
		}

		// user2 should still have full limit
		allowed2, _, _, _, _ := limiter.Allow(ctx, user2)
		if !allowed2 {
			t.Error("user2 should not be rate limited")
		}
	})

	t.Run("fail open on Redis error", func(t *testing.T) {
		// The code shows fail-open behavior (returns true on error)
		// This is hard to test without mocking Redis connection
		t.Skip("requires Redis mocking to test fail-open behavior")
	})
}
