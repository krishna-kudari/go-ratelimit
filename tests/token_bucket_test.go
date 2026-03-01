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

func TestNewTokenBucket(t *testing.T) {
	tests := []struct {
		name           string
		maxCapacity    int64
		refillRate     int64
		expectError    bool
		errorSubstring string
	}{
		{
			name:        "valid parameters",
			maxCapacity: 10,
			refillRate:  60,
			expectError: false,
		},
		{
			name:           "zero max capacity",
			maxCapacity:    0,
			refillRate:     60,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative max capacity",
			maxCapacity:    -1,
			refillRate:     60,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "zero refill rate",
			maxCapacity:    10,
			refillRate:     0,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative refill rate",
			maxCapacity:    10,
			refillRate:     -1,
			expectError:    true,
			errorSubstring: "must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := goratelimit.NewTokenBucket(tt.maxCapacity, tt.refillRate)
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

func TestTokenBucket_Allow_InMemory(t *testing.T) {
	ctx := context.Background()
	key := "test"

	t.Run("allows requests within capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(5, 60)
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
		}
	})

	t.Run("rejects requests when tokens exhausted", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(3, 60)
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
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(2, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result, _ := limiter.Allow(ctx, key)
		if !result.Allowed {
			t.Error("first request should be allowed")
		}
		result, _ = limiter.Allow(ctx, key)
		if !result.Allowed {
			t.Error("second request should be allowed")
		}
		result, _ = limiter.Allow(ctx, key)
		if result.Allowed {
			t.Error("third request should be rejected")
		}

		time.Sleep(1100 * time.Millisecond)

		result, _ = limiter.Allow(ctx, key)
		if !result.Allowed {
			t.Error("request after refill should be allowed")
		}
		result, _ = limiter.Allow(ctx, key)
		if !result.Allowed {
			t.Error("second request after refill should be allowed")
		}
		result, _ = limiter.Allow(ctx, key)
		if result.Allowed {
			t.Error("third request after refill should be rejected")
		}
	})

	t.Run("gradual refill allows steady request flow", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(10, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 10; i++ {
			result, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}
		result, _ := limiter.Allow(ctx, key)
		if result.Allowed {
			t.Error("11th request should be rejected")
		}

		time.Sleep(110 * time.Millisecond)

		result, _ = limiter.Allow(ctx, key)
		if !result.Allowed {
			t.Error("request after partial refill should be allowed")
		}
		result, _ = limiter.Allow(ctx, key)
		if result.Allowed {
			t.Error("next request should be rejected (only 1 token refilled)")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(100, 60)
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

	t.Run("tokens never exceed capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(5, 100)
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

func TestNewTokenBucket_Redis(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("valid parameters", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(10, 60, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if limiter == nil {
			t.Error("expected limiter to be non-nil")
		}
	})

	t.Run("invalid parameters", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(0, 60, goratelimit.WithRedis(client))
		if err == nil {
			t.Error("expected error for zero capacity")
		}
		if limiter != nil {
			t.Error("expected limiter to be nil on error")
		}
	})
}

func TestTokenBucket_Allow_Redis(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("allows requests within limit", func(t *testing.T) {
		key := "test-token-user-1"
		limiter, err := goratelimit.NewTokenBucket(10, 60, goratelimit.WithRedis(client))
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
		if result.Remaining < 0 || result.Remaining > result.Limit {
			t.Errorf("remaining should be between 0 and %d, got %d", result.Limit, result.Remaining)
		}
		if result.RetryAfter != 0 {
			t.Errorf("retryAfter should be 0 when allowed, got %v", result.RetryAfter)
		}
	})

	t.Run("rejects requests exceeding limit", func(t *testing.T) {
		key := fmt.Sprintf("test-token-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewTokenBucket(3, 60, goratelimit.WithRedis(client))
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
			t.Errorf("remaining should be 0, got %d", result.Remaining)
		}
		if result.RetryAfter <= 0 {
			t.Errorf("retryAfter should be positive, got %v", result.RetryAfter)
		}
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		key := fmt.Sprintf("test-token-user-3-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewTokenBucket(2, 2, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		limiter.Allow(ctx, key)
		limiter.Allow(ctx, key)

		result, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Allowed {
			t.Error("third request should be rejected")
		}

		time.Sleep(1100 * time.Millisecond)

		result, err = limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("request after refill should be allowed")
		}
		result, _ = limiter.Allow(ctx, key)
		if !result.Allowed {
			t.Error("second request after refill should be allowed")
		}
		result, _ = limiter.Allow(ctx, key)
		if result.Allowed {
			t.Error("third request after refill should be rejected")
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-token-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-token-user-5-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewTokenBucket(2, 60, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result, _ := limiter.Allow(ctx, user1)
		if !result.Allowed {
			t.Error("user1 first request should be allowed")
		}
		result, _ = limiter.Allow(ctx, user1)
		if !result.Allowed {
			t.Error("user1 second request should be allowed")
		}

		result, _ = limiter.Allow(ctx, user1)
		if result.Allowed {
			t.Error("user1 should be rate limited")
		}

		result, _ = limiter.Allow(ctx, user2)
		if !result.Allowed {
			t.Error("user2 should not be rate limited")
		}
	})

	t.Run("tokens never exceed capacity", func(t *testing.T) {
		key := fmt.Sprintf("test-token-user-6-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewTokenBucket(5, 100, goratelimit.WithRedis(client))
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

	t.Run("fail open on Redis error", func(t *testing.T) {
		t.Skip("requires Redis mocking to test fail-open behavior")
	})
}
