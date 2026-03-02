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

func TestNewGCRA(t *testing.T) {
	tests := []struct {
		name           string
		rate           int64
		burst          int64
		expectError    bool
		errorSubstring string
	}{
		{"valid parameters", 10, 20, false, ""},
		{"zero rate", 0, 20, true, "must be positive"},
		{"negative rate", -1, 20, true, "must be positive"},
		{"zero burst", 10, 0, true, "must be positive"},
		{"negative burst", 10, -1, true, "must be positive"},
		{"burst equals rate", 10, 10, false, ""},
		{"burst greater than rate", 10, 30, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := goratelimit.NewGCRA(tt.rate, tt.burst)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorSubstring)
				assert.Nil(t, limiter)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, limiter)
			}
		})
	}
}

func TestGCRA_Allow(t *testing.T) {
	ctx := context.Background()
	key := "test"

	t.Run("allows requests within burst", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 5)
		require.NoError(t, err)

		for i := 0; i < 5; i++ {
			res, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, res.Allowed, "request %d should be allowed", i+1)
			assert.GreaterOrEqual(t, res.Remaining, int64(0))
			assert.Equal(t, time.Duration(0), res.RetryAfter, "retryAfter should be 0 when allowed")
		}
	})

	t.Run("rejects requests exceeding burst", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 3)
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			res, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, res.Allowed, "request %d should be allowed", i+1)
		}

		res, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.False(t, res.Allowed, "4th request should be rejected")
		assert.Equal(t, int64(0), res.Remaining, "remaining should be 0 when rejected")
		assert.Greater(t, res.RetryAfter, time.Duration(0), "retryAfter should be positive when rejected")
	})

	t.Run("allows requests after rate limit period", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(2, 2)
		require.NoError(t, err)

		res, _ := limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "first request should be allowed")
		res, _ = limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "second request should be allowed")
		res, _ = limiter.Allow(ctx, key)
		assert.False(t, res.Allowed, "third request should be rejected")

		time.Sleep(600 * time.Millisecond)

		res, _ = limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "request after rate limit period should be allowed")
	})

	t.Run("allows steady rate of requests", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 10)
		require.NoError(t, err)

		for i := 0; i < 10; i++ {
			res, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, res.Allowed, "request %d should be allowed at steady rate", i+1)
			time.Sleep(100 * time.Millisecond)
		}
	})

	t.Run("remaining count decreases as requests are made", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 5)
		require.NoError(t, err)

		prevRemaining := int64(5)
		for i := 0; i < 5; i++ {
			res, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.Less(t, res.Remaining, prevRemaining, "remaining should decrease")
			prevRemaining = res.Remaining
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(100, 50)
		require.NoError(t, err)

		allowed := make(chan bool, 100)
		for i := 0; i < 100; i++ {
			go func() {
				res, _ := limiter.Allow(ctx, key)
				allowed <- res.Allowed
			}()
		}

		count := 0
		for i := 0; i < 100; i++ {
			if <-allowed {
				count++
			}
		}

		assert.Equal(t, 50, count, "expected exactly 50 allowed requests (burst)")
	})

	t.Run("allows burst after waiting", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(5, 3)
		require.NoError(t, err)

		initialAllowed := 0
		for i := 0; i < 5; i++ {
			res, _ := limiter.Allow(ctx, key)
			if res.Allowed {
				initialAllowed++
			} else {
				break
			}
		}

		require.GreaterOrEqual(t, initialAllowed, 1, "expected at least 1 request to be allowed initially")

		var retryAfter time.Duration
		gotRejection := false
		for i := 0; i < 3; i++ {
			res, _ := limiter.Allow(ctx, key)
			if !res.Allowed && res.RetryAfter > 0 {
				retryAfter = res.RetryAfter
				gotRejection = true
				break
			}
		}

		if gotRejection {
			waitTime := retryAfter
			if waitTime < 200*time.Millisecond {
				waitTime = 200 * time.Millisecond
			}
			time.Sleep(waitTime + 50*time.Millisecond)
		} else {
			time.Sleep(250 * time.Millisecond)
		}

		res, _ := limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "request should be allowed after waiting emission interval")
	})

	t.Run("retryAfter is calculated correctly", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 2)
		require.NoError(t, err)

		_, _ = limiter.Allow(ctx, key)
		_, _ = limiter.Allow(ctx, key)

		res, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.Greater(t, res.RetryAfter, time.Duration(0), "retryAfter should be positive")
		retrySec := res.RetryAfter.Seconds()
		assert.GreaterOrEqual(t, retrySec, 0.1, "retryAfter should be at least 0.1 seconds")
		assert.LessOrEqual(t, retrySec, 2.0, "retryAfter should be at most 2.0 seconds")
	})
}

func TestGCRA_Reset(t *testing.T) {
	ctx := context.Background()
	key := "test-reset"

	t.Run("reset clears state and allows burst again", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 3)
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			res, _ := limiter.Allow(ctx, key)
			assert.True(t, res.Allowed, "request %d should be allowed", i+1)
		}
		res, _ := limiter.Allow(ctx, key)
		assert.False(t, res.Allowed, "4th request should be rejected")

		require.NoError(t, limiter.Reset(ctx, key))

		for i := 0; i < 3; i++ {
			res, _ := limiter.Allow(ctx, key)
			assert.True(t, res.Allowed, "after reset: request %d should be allowed", i+1)
		}
	})
}

func TestNewGCRA_Redis(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("valid parameters", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 20, goratelimit.WithRedis(client))
		require.NoError(t, err)
		assert.NotNil(t, limiter)
	})

	t.Run("invalid parameters - zero rate", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(0, 20, goratelimit.WithRedis(client))
		require.Error(t, err, "expected error for zero rate")
		assert.Nil(t, limiter)
	})

	t.Run("invalid parameters - zero burst", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 0, goratelimit.WithRedis(client))
		require.Error(t, err, "expected error for zero burst")
		assert.Nil(t, limiter)
	})
}

func TestGCRA_Redis_Allow(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	limiter, err := goratelimit.NewGCRA(10, 20, goratelimit.WithRedis(client))
	require.NoError(t, err)

	t.Run("allows requests within burst", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-1-%d", time.Now().UnixNano())
		res, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, res.Allowed, "first request should be allowed")
		assert.GreaterOrEqual(t, res.Remaining, int64(0))
		assert.LessOrEqual(t, res.Remaining, res.Limit)
		assert.Equal(t, time.Duration(0), res.RetryAfter, "retryAfter should be 0 when allowed")
	})

	t.Run("rejects requests exceeding burst", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(10, 3, goratelimit.WithRedis(client))
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			res, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, res.Allowed, "request %d should be allowed", i+1)
		}

		res, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.False(t, res.Allowed, "4th request should be rejected")
		assert.Equal(t, int64(0), res.Remaining, "remaining should be 0")
		assert.Greater(t, res.RetryAfter, time.Duration(0), "retryAfter should be positive")
	})

	t.Run("allows requests after rate limit period", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-3-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(2, 2, goratelimit.WithRedis(client))
		require.NoError(t, err)

		limiter.Allow(ctx, key)
		limiter.Allow(ctx, key)

		res, _ := limiter.Allow(ctx, key)
		assert.False(t, res.Allowed, "third request should be rejected")

		time.Sleep(600 * time.Millisecond)

		res, _ = limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "request after rate limit period should be allowed")
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-gcra-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-gcra-user-5-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(10, 2, goratelimit.WithRedis(client))
		require.NoError(t, err)

		res, _ := limiter.Allow(ctx, user1)
		assert.True(t, res.Allowed, "user1 first request should be allowed")
		res, _ = limiter.Allow(ctx, user1)
		assert.True(t, res.Allowed, "user1 second request should be allowed")

		res1, _ := limiter.Allow(ctx, user1)
		assert.False(t, res1.Allowed, "user1 should be rate limited")

		res2, _ := limiter.Allow(ctx, user2)
		assert.True(t, res2.Allowed, "user2 should not be rate limited")
	})

	t.Run("allows steady rate of requests", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-6-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(10, 10, goratelimit.WithRedis(client))
		require.NoError(t, err)

		for i := 0; i < 10; i++ {
			res, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, res.Allowed, "request %d should be allowed at steady rate", i+1)
			time.Sleep(100 * time.Millisecond)
		}
	})

	t.Run("remaining count decreases as requests are made", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-7-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(10, 5, goratelimit.WithRedis(client))
		require.NoError(t, err)

		prevRemaining := int64(5)
		for i := 0; i < 5; i++ {
			res, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.Less(t, res.Remaining, prevRemaining, "remaining should decrease")
			prevRemaining = res.Remaining
		}
	})

	t.Run("allows burst after waiting", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-8-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(5, 3, goratelimit.WithRedis(client))
		require.NoError(t, err)

		limiter.Allow(ctx, key)
		limiter.Allow(ctx, key)
		limiter.Allow(ctx, key)

		res, _ := limiter.Allow(ctx, key)
		assert.False(t, res.Allowed, "request should be rejected after burst")

		time.Sleep(250 * time.Millisecond)

		res, _ = limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "request should be allowed after waiting emission interval")
	})

	t.Run("retryAfter is calculated correctly", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-9-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(10, 2, goratelimit.WithRedis(client))
		require.NoError(t, err)

		limiter.Allow(ctx, key)
		limiter.Allow(ctx, key)

		res, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.Greater(t, res.RetryAfter, time.Duration(0), "retryAfter should be positive")
		assert.LessOrEqual(t, res.RetryAfter, time.Second, "retryAfter should be approximately 1 second (rounded up)")
	})

	t.Run("fail open on Redis error", func(t *testing.T) {
		t.Skip("requires Redis mocking to test fail-open behavior")
	})
}

func TestGCRA_Redis_Reset(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	t.Run("reset clears state and allows burst again", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-reset-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(10, 3, goratelimit.WithRedis(client))
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			res, _ := limiter.Allow(ctx, key)
			assert.True(t, res.Allowed, "request %d should be allowed", i+1)
		}
		res, _ := limiter.Allow(ctx, key)
		assert.False(t, res.Allowed, "4th request should be rejected")

		require.NoError(t, limiter.Reset(ctx, key))

		for i := 0; i < 3; i++ {
			res, _ := limiter.Allow(ctx, key)
			assert.True(t, res.Allowed, "after reset: request %d should be allowed", i+1)
		}
	})
}
