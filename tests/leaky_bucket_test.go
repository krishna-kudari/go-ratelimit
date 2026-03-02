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

func TestNewLeakyBucket(t *testing.T) {
	tests := []struct {
		name           string
		capacity       int64
		leakRate       int64
		mode           goratelimit.LeakyBucketMode
		expectError    bool
		errorSubstring string
	}{
		{name: "valid parameters - policing mode", capacity: 10, leakRate: 60, mode: goratelimit.Policing, expectError: false},
		{name: "valid parameters - shaping mode", capacity: 10, leakRate: 60, mode: goratelimit.Shaping, expectError: false},
		{name: "zero capacity", capacity: 0, leakRate: 60, mode: goratelimit.Policing, expectError: true, errorSubstring: "must be positive"},
		{name: "negative capacity", capacity: -1, leakRate: 60, mode: goratelimit.Policing, expectError: true, errorSubstring: "must be positive"},
		{name: "zero leak rate", capacity: 10, leakRate: 0, mode: goratelimit.Policing, expectError: true, errorSubstring: "must be positive"},
		{name: "negative leak rate", capacity: 10, leakRate: -1, mode: goratelimit.Policing, expectError: true, errorSubstring: "must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := goratelimit.NewLeakyBucket(tt.capacity, tt.leakRate, tt.mode)
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

func TestLeakyBucket_Allow_Policing(t *testing.T) {
	ctx := context.Background()
	key := "test"

	t.Run("allows requests within capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 60, goratelimit.Policing)
		require.NoError(t, err)

		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "request %d should be allowed", i+1)
			assert.GreaterOrEqual(t, result.Remaining, int64(0), "remaining should be non-negative")
			assert.Zero(t, result.RetryAfter, "retryAfter should be 0 when allowed")
		}
	})

	t.Run("rejects requests when bucket is full", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(3, 60, goratelimit.Policing)
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "request %d should be allowed", i+1)
		}

		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "4th request should be rejected")
		assert.Zero(t, result.Remaining, "remaining should be 0 when rejected")
		assert.Greater(t, result.RetryAfter, time.Duration(0), "retryAfter should be positive when rejected")
	})

	t.Run("leaks tokens over time", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(2, 2, goratelimit.Policing)
		require.NoError(t, err)

		result, _ := limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "first request should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "second request should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.False(t, result.Allowed, "third request should be rejected")

		time.Sleep(1100 * time.Millisecond)

		result, _ = limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "request after leak should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "second request after leak should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.False(t, result.Allowed, "third request after leak should be rejected")
	})

	t.Run("gradual leak allows steady request flow", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(10, 10, goratelimit.Policing)
		require.NoError(t, err)

		for i := 0; i < 10; i++ {
			result, _ := limiter.Allow(ctx, key)
			assert.True(t, result.Allowed, "request %d should be allowed", i+1)
		}
		result, _ := limiter.Allow(ctx, key)
		assert.False(t, result.Allowed, "11th request should be rejected")

		time.Sleep(110 * time.Millisecond)

		result, _ = limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "request after partial leak should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.False(t, result.Allowed, "next request should be rejected (only 1 token leaked)")
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(100, 60, goratelimit.Policing)
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

	t.Run("remaining count decreases as bucket fills", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 60, goratelimit.Policing)
		require.NoError(t, err)

		prevRemaining := int64(5)
		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.Less(t, result.Remaining, prevRemaining, "remaining should decrease")
			prevRemaining = result.Remaining
		}
	})

	t.Run("level never exceeds capacity", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 100, goratelimit.Policing)
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

func TestLeakyBucket_Allow_Shaping(t *testing.T) {
	ctx := context.Background()
	key := "test"

	t.Run("allows requests within capacity with delay", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 5, goratelimit.Shaping)
		require.NoError(t, err)

		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "first request should be allowed")
		assert.Zero(t, result.RetryAfter, "first request should have no delay")
		assert.GreaterOrEqual(t, result.Remaining, int64(0), "remaining should be non-negative")

		prevDelay := result.RetryAfter
		for i := 0; i < 4; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "request %d should be allowed", i+2)
			assert.Greater(t, result.RetryAfter, prevDelay, "delay should increase")
			assert.GreaterOrEqual(t, result.Remaining, int64(0), "remaining should be non-negative")
			prevDelay = result.RetryAfter
		}
	})

	t.Run("rejects requests when queue is full", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(3, 60, goratelimit.Shaping)
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "request %d should be allowed", i+1)
		}

		result, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "4th request should be rejected")
		assert.Zero(t, result.Remaining, "remaining should be 0 when rejected")
		assert.Zero(t, result.RetryAfter, "retryAfter should be 0 when rejected")
	})

	t.Run("delays requests based on queue depth", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 5, goratelimit.Shaping)
		require.NoError(t, err)

		result1, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.Zero(t, result1.RetryAfter, "first request should have no delay")

		result2, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		expectedDelay := 1.0 / 5.0
		delay2Sec := result2.RetryAfter.Seconds()
		assert.InDelta(t, expectedDelay, delay2Sec, expectedDelay*0.1, "expected delay ~%f, got %f", expectedDelay, delay2Sec)

		result3, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		expectedDelay = 2.0 / 5.0
		delay3Sec := result3.RetryAfter.Seconds()
		assert.InDelta(t, expectedDelay, delay3Sec, expectedDelay*0.1, "expected delay ~%f, got %f", expectedDelay, delay3Sec)
	})

	t.Run("queue drains over time", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(2, 2, goratelimit.Shaping)
		require.NoError(t, err)

		result, _ := limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "first request should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.True(t, result.Allowed, "second request should be allowed")
		result, _ = limiter.Allow(ctx, key)
		assert.False(t, result.Allowed, "third request should be rejected")

		time.Sleep(1100 * time.Millisecond)

		result, err = limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "request after drain should be allowed")
		assert.Zero(t, result.RetryAfter, "delay should be 0 after drain")
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(100, 60, goratelimit.Shaping)
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

	t.Run("remaining count decreases as queue fills", func(t *testing.T) {
		limiter, err := goratelimit.NewLeakyBucket(5, 60, goratelimit.Shaping)
		require.NoError(t, err)

		prevRemaining := int64(5)
		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.Less(t, result.Remaining, prevRemaining, "remaining should decrease")
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
	require.NoError(t, err)
	return limiter
}

func TestNewLeakyBucket_Redis(t *testing.T) {
	t.Run("valid parameters - policing mode", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 10, 60, goratelimit.Policing)
		assert.NotNil(t, limiter, "expected limiter to be non-nil")
	})

	t.Run("valid parameters - shaping mode", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 10, 60, goratelimit.Shaping)
		assert.NotNil(t, limiter, "expected limiter to be non-nil")
	})

	t.Run("invalid parameters - zero capacity", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		limiter, err := goratelimit.NewLeakyBucket(0, 60, goratelimit.Policing, goratelimit.WithRedis(client))
		require.Error(t, err, "expected error for zero capacity")
		assert.Nil(t, limiter, "expected limiter to be nil on error")
	})

	t.Run("invalid parameters - zero leak rate", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		limiter, err := goratelimit.NewLeakyBucket(10, 0, goratelimit.Policing, goratelimit.WithRedis(client))
		require.Error(t, err, "expected error for zero leak rate")
		assert.Nil(t, limiter, "expected limiter to be nil on error")
	})
}

func TestLeakyBucket_Redis_Allow_Policing(t *testing.T) {
	ctx := context.Background()

	t.Run("allows requests within limit", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 10, 60, goratelimit.Policing)
		userID := fmt.Sprintf("test-leaky-policing-user-1-%d", time.Now().UnixNano())

		result, err := limiter.Allow(ctx, userID)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "first request should be allowed")
		assert.True(t, result.Remaining >= 0 && result.Remaining <= result.Limit, "remaining should be between 0 and %d, got %d", result.Limit, result.Remaining)
		assert.Zero(t, result.RetryAfter, "retryAfter should be 0 when allowed")
	})

	t.Run("rejects requests exceeding limit", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 3, 60, goratelimit.Policing)
		userID := fmt.Sprintf("test-leaky-policing-user-2-%d", time.Now().UnixNano())

		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, userID)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "request %d should be allowed", i+1)
		}

		result, err := limiter.Allow(ctx, userID)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "4th request should be rejected")
		assert.Zero(t, result.Remaining, "remaining should be 0")
		assert.Greater(t, result.RetryAfter, time.Duration(0), "retryAfter should be positive")
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 2, 2, goratelimit.Policing)
		userID := fmt.Sprintf("test-leaky-policing-user-3-%d", time.Now().UnixNano())

		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)

		result, _ := limiter.Allow(ctx, userID)
		assert.False(t, result.Allowed, "third request should be rejected")

		time.Sleep(1100 * time.Millisecond)

		result, _ = limiter.Allow(ctx, userID)
		assert.True(t, result.Allowed, "request after leak should be allowed")
		result, _ = limiter.Allow(ctx, userID)
		assert.True(t, result.Allowed, "second request after leak should be allowed")
		result, _ = limiter.Allow(ctx, userID)
		assert.False(t, result.Allowed, "third request after leak should be rejected")
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 2, 60, goratelimit.Policing)
		user1 := fmt.Sprintf("test-leaky-policing-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-leaky-policing-user-5-%d", time.Now().UnixNano())

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

		assert.Equal(t, 5, allowedCount, "expected exactly 5 allowed requests (capacity)")
	})
}

func TestLeakyBucket_Redis_Allow_Shaping(t *testing.T) {
	ctx := context.Background()

	t.Run("allows requests within limit with delay", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 10, 60, goratelimit.Shaping)
		userID := fmt.Sprintf("test-leaky-shaping-user-1-%d", time.Now().UnixNano())

		result, err := limiter.Allow(ctx, userID)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "first request should be allowed")
		assert.True(t, result.Remaining >= 0 && result.Remaining <= result.Limit, "remaining should be between 0 and %d, got %d", result.Limit, result.Remaining)
		assert.GreaterOrEqual(t, result.RetryAfter, time.Duration(0), "retryAfter should be non-negative when allowed")
	})

	t.Run("rejects requests when queue is full", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 3, 60, goratelimit.Shaping)
		userID := fmt.Sprintf("test-leaky-shaping-user-2-%d", time.Now().UnixNano())

		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, userID)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "request %d should be allowed", i+1)
		}

		result, err := limiter.Allow(ctx, userID)
		require.NoError(t, err)
		assert.False(t, result.Allowed, "4th request should be rejected")
		assert.Zero(t, result.Remaining, "remaining should be 0")
		assert.Zero(t, result.RetryAfter, "retryAfter should be 0 for shaping mode when rejected")
	})

	t.Run("delays requests based on queue depth", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 5, 5, goratelimit.Shaping)
		userID := fmt.Sprintf("test-leaky-shaping-user-3-%d", time.Now().UnixNano())

		result1, err := limiter.Allow(ctx, userID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, result1.RetryAfter, time.Duration(0), "delay should be non-negative")

		result2, err := limiter.Allow(ctx, userID)
		require.NoError(t, err)
		assert.Greater(t, result2.RetryAfter, result1.RetryAfter, "delay should increase")
	})

	t.Run("queue drains over time", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 2, 2, goratelimit.Shaping)
		userID := fmt.Sprintf("test-leaky-shaping-user-4-%d", time.Now().UnixNano())

		limiter.Allow(ctx, userID)
		limiter.Allow(ctx, userID)

		result, _ := limiter.Allow(ctx, userID)
		assert.False(t, result.Allowed, "third request should be rejected")

		time.Sleep(1100 * time.Millisecond)

		result, err := limiter.Allow(ctx, userID)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "request after drain should be allowed")
		assert.GreaterOrEqual(t, result.RetryAfter, time.Duration(0), "delay should be non-negative")
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		limiter := newRedisLeakyBucket(t, 2, 60, goratelimit.Shaping)
		user1 := fmt.Sprintf("test-leaky-shaping-user-5-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-leaky-shaping-user-6-%d", time.Now().UnixNano())

		result, _ := limiter.Allow(ctx, user1)
		assert.True(t, result.Allowed, "user1 first request should be allowed")
		result, _ = limiter.Allow(ctx, user1)
		assert.True(t, result.Allowed, "user1 second request should be allowed")

		result, _ = limiter.Allow(ctx, user1)
		assert.False(t, result.Allowed, "user1 should be rate limited")

		result, _ = limiter.Allow(ctx, user2)
		assert.True(t, result.Allowed, "user2 should not be rate limited")
	})

	t.Run("fail open on Redis error", func(t *testing.T) {
		t.Skip("requires Redis mocking to test fail-open behavior")
	})
}
