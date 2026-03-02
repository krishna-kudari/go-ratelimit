package goratelimit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		{name: "valid parameters", maxCapacity: 10, refillRate: 60, expectError: false},
		{name: "zero max capacity", maxCapacity: 0, refillRate: 60, expectError: true, errorSubstring: "must be positive"},
		{name: "negative max capacity", maxCapacity: -1, refillRate: 60, expectError: true, errorSubstring: "must be positive"},
		{name: "zero refill rate", maxCapacity: 10, refillRate: 0, expectError: true, errorSubstring: "must be positive"},
		{name: "negative refill rate", maxCapacity: 10, refillRate: -1, expectError: true, errorSubstring: "must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := goratelimit.NewTokenBucket(tt.maxCapacity, tt.refillRate)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorSubstring != "" {
					assert.Contains(t, err.Error(), tt.errorSubstring)
				}
				assert.Nil(t, limiter, "expected limiter to be nil on error")
			} else {
				require.NoError(t, err)
				assert.NotNil(t, limiter, "expected limiter to be non-nil")
			}
		})
	}
}

func TestTokenBucket_Allow_InMemory(t *testing.T) {
	ctx := context.Background()
	key := "test"

	t.Run("allows requests within capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(5, 60)
		require.NoError(t, err)

		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "request %d should be allowed", i+1)
		}
	})

	t.Run("rejects requests when tokens exhausted", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(3, 60)
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "request %d should be allowed", i+1)
		}

		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "4th request should be rejected")
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(2, 2)
		require.NoError(t, err)

		result, _ := limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "first request should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "second request should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.False(t, result.Allowed, "third request should be rejected")

		time.Sleep(1100 * time.Millisecond)

		result, _ = limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "request after refill should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "second request after refill should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.False(t, result.Allowed, "third request after refill should be rejected")
	})

	t.Run("gradual refill allows steady request flow", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(10, 10)
		require.NoError(t, err)

		for i := 0; i < 10; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "request %d should be allowed", i+1)
		}
		result, _ := limiter.Allow(ctx, key)
		assert.False(t, result.Allowed, "11th request should be rejected")

		time.Sleep(110 * time.Millisecond)

		result, _ = limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "request after partial refill should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.False(t, result.Allowed, "next request should be rejected (only 1 token refilled)")
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(100, 60)
		require.NoError(t, err)

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

		assert.Equal(t, 100, count, "expected exactly 100 allowed requests")
	})

	t.Run("tokens never exceed capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(5, 100)
		require.NoError(t, err)

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

		assert.Equal(t, 5, allowedCount, "expected exactly 5 allowed requests (capacity)")
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
		require.NoError(t, err)
		assert.NotNil(t, limiter, "expected limiter to be non-nil")
	})

	t.Run("invalid parameters", func(t *testing.T) {
		limiter, err := goratelimit.NewTokenBucket(0, 60, goratelimit.WithRedis(client))
		require.Error(t, err, "expected error for zero capacity")
		assert.Nil(t, limiter, "expected limiter to be nil on error")
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
		require.NoError(t, err)

		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "first request should be allowed")
		assert.True(t, result.Remaining >= 0 && result.Remaining <= result.Limit, "remaining should be between 0 and %d, got %d", result.Limit, result.Remaining)
		assert.Zero(t, result.RetryAfter, "retryAfter should be 0 when allowed")
	})

	t.Run("rejects requests exceeding limit", func(t *testing.T) {
		key := fmt.Sprintf("test-token-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewTokenBucket(3, 60, goratelimit.WithRedis(client))
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "request %d should be allowed", i+1)
		}

		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "4th request should be rejected")
		assert.Zero(t, result.Remaining, "remaining should be 0")
		assert.Greater(t, result.RetryAfter, time.Duration(0), "retryAfter should be positive")
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		key := fmt.Sprintf("test-token-user-3-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewTokenBucket(2, 2, goratelimit.WithRedis(client))
		require.NoError(t, err)

		limiter.Allow(ctx, key)
		limiter.Allow(ctx, key)

		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "third request should be rejected")

		time.Sleep(1100 * time.Millisecond)

		result, err = limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "request after refill should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "second request after refill should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.False(t, result.Allowed, "third request after refill should be rejected")
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-token-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-token-user-5-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewTokenBucket(2, 60, goratelimit.WithRedis(client))
		require.NoError(t, err)

		result, _ := limiter.Allow(ctx, user1)
		assert.True(t, result.Allowed, "user1 first request should be allowed")
		result, _ = limiter.Allow(ctx, user1)
		assert.True(t, result.Allowed, "user1 second request should be allowed")

		result, _ = limiter.Allow(ctx, user1)
		assert.False(t, result.Allowed, "user1 should be rate limited")

		result, _ = limiter.Allow(ctx, user2)
		assert.True(t, result.Allowed, "user2 should not be rate limited")
	})

	t.Run("tokens never exceed capacity", func(t *testing.T) {
		key := fmt.Sprintf("test-token-user-6-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewTokenBucket(5, 100, goratelimit.WithRedis(client))
		require.NoError(t, err)

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

		assert.Equal(t, 5, allowedCount, "expected exactly 5 allowed requests (capacity)")
	})

	t.Run("fail open on Redis error", func(t *testing.T) {
		t.Skip("requires Redis mocking to test fail-open behavior")
	})
}
