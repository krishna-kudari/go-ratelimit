package goratelimit_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	goratelimit "github.com/krishna-kudari/ratelimit"
)

func TestNewLeakyBucket(t *testing.T) {
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
			limiter, err := goratelimit.NewLeakyBucket(tt.capacity, tt.leakRate, tt.mode)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorSubstring != "" && !strings.Contains(err.Error(), tt.errorSubstring) {
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

func TestLeakyBucket_Allow_Policing(t *testing.T) {
	ctx := context.Background()
	key := "test"

	t.Run("allows requests within capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 60, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
			if result.Remaining < 0 {
				t.Errorf("remaining should be non-negative, got %d", result.Remaining)
			}
			if result.RetryAfter != 0 {
				t.Errorf("retryAfter should be 0 when allowed, got %v", result.RetryAfter)
			}
		}
	})

	t.Run("rejects requests when bucket is full", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(3, 60, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		result, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Allowed {
			t.Error("4th request should be rejected")
		}
		if result.Remaining != 0 {
			t.Errorf("remaining should be 0 when rejected, got %d", result.Remaining)
		}
		if result.RetryAfter <= 0 {
			t.Errorf("retryAfter should be positive when rejected, got %v", result.RetryAfter)
		}
	})

	t.Run("leaks tokens over time", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(2, 2, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result, _ := limiter.Allow(ctx, key); !result.Allowed {
			t.Error("first request should be allowed")
		}
		if result, _ := limiter.Allow(ctx, key); !result.Allowed {
			t.Error("second request should be allowed")
		}
		if result, _ := limiter.Allow(ctx, key); result.Allowed {
			t.Error("third request should be rejected")
		}

		time.Sleep(1100 * time.Millisecond)

		if result, _ := limiter.Allow(ctx, key); !result.Allowed {
			t.Error("request after leak should be allowed")
		}
		if result, _ := limiter.Allow(ctx, key); !result.Allowed {
			t.Error("second request after leak should be allowed")
		}
		if result, _ := limiter.Allow(ctx, key); result.Allowed {
			t.Error("third request after leak should be rejected")
		}
	})

	t.Run("gradual leak allows steady request flow", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(10, 10, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 10; i++ {
			if result, _ := limiter.Allow(ctx, key); !result.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}
		if result, _ := limiter.Allow(ctx, key); result.Allowed {
			t.Error("11th request should be rejected")
		}

		time.Sleep(110 * time.Millisecond)

		if result, _ := limiter.Allow(ctx, key); !result.Allowed {
			t.Error("request after partial leak should be allowed")
		}
		if result, _ := limiter.Allow(ctx, key); result.Allowed {
			t.Error("next request should be rejected (only 1 token leaked)")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(100, 60, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		allowed := make(chan bool, 200)
		for i := 0; i < 200; i++ {
			go func() {
				result, _ := limiter.Allow(ctx, key)
				allowed <- result.Allowed
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

	t.Run("remaining count decreases as bucket fills", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 60, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		prevRemaining := int64(5)
		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Remaining >= prevRemaining {
				t.Errorf("remaining should decrease, got %d (previous: %d)", result.Remaining, prevRemaining)
			}
			prevRemaining = result.Remaining
		}
	})

	t.Run("level never exceeds capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 100, goratelimit.Policing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 5; i++ {
			limiter.Allow(ctx, key)
		}

		time.Sleep(200 * time.Millisecond)

		allowedCount := 0
		for i := 0; i < 10; i++ {
			result, _ := limiter.Allow(ctx, key)
			if result.Allowed {
				allowedCount++
			}
		}

		if allowedCount != 5 {
			t.Errorf("expected exactly 5 allowed requests (capacity), got %d", allowedCount)
		}
	})
}

func TestLeakyBucket_Allow_Shaping(t *testing.T) {
	ctx := context.Background()
	key := "test"

	t.Run("allows requests within capacity with delay", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 5, goratelimit.Shaping)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("first request should be allowed")
		}
		if result.RetryAfter != 0 {
			t.Errorf("first request should have no delay, got %v", result.RetryAfter)
		}
		if result.Remaining < 0 {
			t.Errorf("remaining should be non-negative, got %d", result.Remaining)
		}

		prevDelay := result.RetryAfter
		for i := 0; i < 4; i++ {
			result, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Allowed {
				t.Errorf("request %d should be allowed", i+2)
			}
			if result.RetryAfter <= prevDelay {
				t.Errorf("delay should increase, got %v (previous: %v)", result.RetryAfter, prevDelay)
			}
			if result.Remaining < 0 {
				t.Errorf("remaining should be non-negative, got %d", result.Remaining)
			}
			prevDelay = result.RetryAfter
		}
	})

	t.Run("rejects requests when queue is full", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(3, 60, goratelimit.Shaping)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		result, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Allowed {
			t.Error("4th request should be rejected")
		}
		if result.Remaining != 0 {
			t.Errorf("remaining should be 0 when rejected, got %d", result.Remaining)
		}
		if result.RetryAfter != 0 {
			t.Errorf("retryAfter should be 0 when rejected, got %v", result.RetryAfter)
		}
	})

	t.Run("delays requests based on queue depth", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 5, goratelimit.Shaping)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result1, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result1.RetryAfter != 0 {
			t.Errorf("first request should have no delay, got %v", result1.RetryAfter)
		}

		result2, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedDelay := 1.0 / 5.0
		delay2Sec := result2.RetryAfter.Seconds()
		if delay2Sec < expectedDelay*0.9 || delay2Sec > expectedDelay*1.1 {
			t.Errorf("expected delay ~%f, got %f", expectedDelay, delay2Sec)
		}

		result3, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedDelay = 2.0 / 5.0
		delay3Sec := result3.RetryAfter.Seconds()
		if delay3Sec < expectedDelay*0.9 || delay3Sec > expectedDelay*1.1 {
			t.Errorf("expected delay ~%f, got %f", expectedDelay, delay3Sec)
		}
	})

	t.Run("queue drains over time", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(2, 2, goratelimit.Shaping)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result, _ := limiter.Allow(ctx, key); !result.Allowed {
			t.Error("first request should be allowed")
		}
		if result, _ := limiter.Allow(ctx, key); !result.Allowed {
			t.Error("second request should be allowed")
		}
		if result, _ := limiter.Allow(ctx, key); result.Allowed {
			t.Error("third request should be rejected")
		}

		time.Sleep(1100 * time.Millisecond)

		result, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("request after drain should be allowed")
		}
		if result.RetryAfter != 0 {
			t.Errorf("delay should be 0 after drain, got %v", result.RetryAfter)
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(100, 60, goratelimit.Shaping)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		allowed := make(chan bool, 200)
		for i := 0; i < 200; i++ {
			go func() {
				result, _ := limiter.Allow(ctx, key)
				allowed <- result.Allowed
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

	t.Run("remaining count decreases as queue fills", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 60, goratelimit.Shaping)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		prevRemaining := int64(5)
		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Remaining >= prevRemaining {
				t.Errorf("remaining should decrease, got %d (previous: %d)", result.Remaining, prevRemaining)
			}
			prevRemaining = result.Remaining
		}
	})
}

func newRedisLeakyBucket(t *testing.T, capacity, leakRate int64, mode goratelimit.LeakyBucketMode) goratelimit.Limiter {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	limiter, err := goratelimit.NewLeakyBucket(capacity, leakRate, mode, goratelimit.WithRedis(client))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return limiter
}

func TestNewLeakyBucket_Redis(t *testing.T) {
	t.Run("valid parameters - policing mode", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 10, 60, goratelimit.Policing)
		if limiter == nil {
			t.Error("expected limiter to be non-nil")
		}
	})

	t.Run("valid parameters - shaping mode", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 10, 60, goratelimit.Shaping)
		if limiter == nil {
			t.Error("expected limiter to be non-nil")
		}
	})

	t.Run("invalid parameters - zero capacity", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		limiter, err := goratelimit.NewLeakyBucket(0, 60, goratelimit.Policing, goratelimit.WithRedis(client))
		if err == nil {
			t.Error("expected error for zero capacity")
		}
		if limiter != nil {
			t.Error("expected limiter to be nil on error")
		}
	})

	t.Run("invalid parameters - zero leak rate", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		limiter, err := goratelimit.NewLeakyBucket(10, 0, goratelimit.Policing, goratelimit.WithRedis(client))
		if err == nil {
			t.Error("expected error for zero leak rate")
		}
		if limiter != nil {
			t.Error("expected limiter to be nil on error")
		}
	})
}

func TestLeakyBucket_Redis_Allow_Policing(t *testing.T) {
	ctx := context.Background()

	t.Run("allows requests within limit", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 10, 60, goratelimit.Policing)
		userID := fmt.Sprintf("test-leaky-policing-user-1-%d", time.Now().UnixNano())

		result, err := limiter.Allow(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("first request should be allowed")
		}
		if result.Remaining < 0 || result.Remaining > result.Limit {
			t.Errorf("remaining should be between 0 and %d, got %d", result.Limit, result.Remaining)
		}
		if result.RetryAfter != 0 {
			t.Errorf("retryAfter should be 0 when allowed, got %v", result.RetryAfter)
		}
	})

	t.Run("rejects requests exceeding limit", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 3, 60, goratelimit.Policing)
		userID := fmt.Sprintf("test-leaky-policing-user-2-%d", time.Now().UnixNano())

		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, userID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		result, err := limiter.Allow(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Allowed {
			t.Error("4th request should be rejected")
		}
		if result.Remaining != 0 {
			t.Errorf("remaining should be 0, got %d", result.Remaining)
		}
		if result.RetryAfter <= 0 {
			t.Errorf("retryAfter should be positive, got %v", result.RetryAfter)
		}
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 2, 2, goratelimit.Policing)
		userID := fmt.Sprintf("test-leaky-policing-user-3-%d", time.Now().UnixNano())

		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)

		result, _ := limiter.Allow(ctx, userID)
		if result.Allowed {
			t.Error("third request should be rejected")
		}

		time.Sleep(1100 * time.Millisecond)

		if result, _ := limiter.Allow(ctx, userID); !result.Allowed {
			t.Error("request after leak should be allowed")
		}
		if result, _ := limiter.Allow(ctx, userID); !result.Allowed {
			t.Error("second request after leak should be allowed")
		}
		if result, _ := limiter.Allow(ctx, userID); result.Allowed {
			t.Error("third request after leak should be rejected")
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 2, 60, goratelimit.Policing)
		user1 := fmt.Sprintf("test-leaky-policing-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-leaky-policing-user-5-%d", time.Now().UnixNano())

		if result, _ := limiter.Allow(ctx, user1); !result.Allowed {
			t.Error("user1 first request should be allowed")
		}
		if result, _ := limiter.Allow(ctx, user1); !result.Allowed {
			t.Error("user1 second request should be allowed")
		}

		if result, _ := limiter.Allow(ctx, user1); result.Allowed {
			t.Error("user1 should be rate limited")
		}

		if result, _ := limiter.Allow(ctx, user2); !result.Allowed {
			t.Error("user2 should not be rate limited")
		}
	})

	t.Run("tokens never exceed capacity", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 5, 100, goratelimit.Policing)
		userID := fmt.Sprintf("test-leaky-policing-user-6-%d", time.Now().UnixNano())

		for i := 0; i < 5; i++ {
			limiter.Allow(ctx, userID)
		}

		time.Sleep(200 * time.Millisecond)

		allowedCount := 0
		for i := 0; i < 10; i++ {
			result, _ := limiter.Allow(ctx, userID)
			if result.Allowed {
				allowedCount++
			}
		}

		if allowedCount != 5 {
			t.Errorf("expected exactly 5 allowed requests (capacity), got %d", allowedCount)
		}
	})
}

func TestLeakyBucket_Redis_Allow_Shaping(t *testing.T) {
	ctx := context.Background()

	t.Run("allows requests within limit with delay", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 10, 60, goratelimit.Shaping)
		userID := fmt.Sprintf("test-leaky-shaping-user-1-%d", time.Now().UnixNano())

		result, err := limiter.Allow(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("first request should be allowed")
		}
		if result.Remaining < 0 || result.Remaining > result.Limit {
			t.Errorf("remaining should be between 0 and %d, got %d", result.Limit, result.Remaining)
		}
		if result.RetryAfter < 0 {
			t.Errorf("retryAfter should be non-negative when allowed, got %v", result.RetryAfter)
		}
	})

	t.Run("rejects requests when queue is full", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 3, 60, goratelimit.Shaping)
		userID := fmt.Sprintf("test-leaky-shaping-user-2-%d", time.Now().UnixNano())

		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, userID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		result, err := limiter.Allow(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Allowed {
			t.Error("4th request should be rejected")
		}
		if result.Remaining != 0 {
			t.Errorf("remaining should be 0, got %d", result.Remaining)
		}
		if result.RetryAfter != 0 {
			t.Errorf("retryAfter should be 0 for shaping mode when rejected, got %v", result.RetryAfter)
		}
	})

	t.Run("delays requests based on queue depth", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 5, 5, goratelimit.Shaping)
		userID := fmt.Sprintf("test-leaky-shaping-user-3-%d", time.Now().UnixNano())

		result1, err := limiter.Allow(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result1.RetryAfter < 0 {
			t.Errorf("delay should be non-negative, got %v", result1.RetryAfter)
		}

		result2, err := limiter.Allow(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result2.RetryAfter <= result1.RetryAfter {
			t.Errorf("delay should increase, got %v (previous: %v)", result2.RetryAfter, result1.RetryAfter)
		}
	})

	t.Run("queue drains over time", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 2, 2, goratelimit.Shaping)
		userID := fmt.Sprintf("test-leaky-shaping-user-4-%d", time.Now().UnixNano())

		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)

		if result, _ := limiter.Allow(ctx, userID); result.Allowed {
			t.Error("third request should be rejected")
		}

		time.Sleep(1100 * time.Millisecond)

		result, err := limiter.Allow(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("request after drain should be allowed")
		}
		if result.RetryAfter < 0 {
			t.Errorf("delay should be non-negative, got %v", result.RetryAfter)
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 2, 60, goratelimit.Shaping)
		user1 := fmt.Sprintf("test-leaky-shaping-user-5-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-leaky-shaping-user-6-%d", time.Now().UnixNano())

		if result, _ := limiter.Allow(ctx, user1); !result.Allowed {
			t.Error("user1 first request should be allowed")
		}
		if result, _ := limiter.Allow(ctx, user1); !result.Allowed {
			t.Error("user1 second request should be allowed")
		}

		if result, _ := limiter.Allow(ctx, user1); result.Allowed {
			t.Error("user1 should be rate limited")
		}

		if result, _ := limiter.Allow(ctx, user2); !result.Allowed {
			t.Error("user2 should not be rate limited")
		}
	})

	t.Run("fail open on Redis error", func(t *testing.T) {
		t.Skip("requires Redis mocking to test fail-open behavior")
	})
}
