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

func TestNewSlidingWindow(t *testing.T) {
	tests := []struct {
		name           string
		maxRequests    int64
		windowSeconds  int64
		expectError    bool
		errorSubstring string
	}{
		{"valid parameters", 10, 60, false, ""},
		{"zero max requests", 0, 60, true, "must be positive"},
		{"negative max requests", -1, 60, true, "must be positive"},
		{"zero window seconds", 10, 0, true, "must be positive"},
		{"negative window seconds", 10, -1, true, "must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := goratelimit.NewSlidingWindow(tt.maxRequests, tt.windowSeconds)
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

func TestSlidingWindow_Allow(t *testing.T) {
	ctx := context.Background()
	key := "test-key"

	t.Run("allows requests within limit", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindow(5, 60)
		require.NoError(t, err)

		for i := 0; i < 5; i++ {
			res, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, res.Allowed, "request %d should be allowed", i+1)
		}
	})

	t.Run("rejects requests exceeding limit", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindow(3, 60)
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			res, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, res.Allowed, "request %d should be allowed", i+1)
		}

		res, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.False(t, res.Allowed, "4th request should be rejected")
	})

	t.Run("sliding window removes old timestamps", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindow(2, 1)
		require.NoError(t, err)

		res, _ := limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "first request should be allowed")
		res, _ = limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "second request should be allowed")
		res, _ = limiter.Allow(ctx, key)
		assert.False(t, res.Allowed, "third request should be rejected")

		time.Sleep(1100 * time.Millisecond)

		res, _ = limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "request after window slide should be allowed")
		res, _ = limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "second request after window slide should be allowed")
		res, _ = limiter.Allow(ctx, key)
		assert.False(t, res.Allowed, "third request after window slide should be rejected")
	})

	t.Run("sliding window allows gradual request flow", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindow(3, 2)
		require.NoError(t, err)

		start := time.Now()
		for i := 0; i < 3; i++ {
			res, err := limiter.Allow(ctx, key)
			require.NoError(t, err)
			assert.True(t, res.Allowed, "request %d should be allowed", i+1)
			time.Sleep(200 * time.Millisecond)
		}
		res, _ := limiter.Allow(ctx, key)
		assert.False(t, res.Allowed, "4th request should be rejected")

		elapsed := time.Since(start)
		if elapsed < 2100*time.Millisecond {
			time.Sleep(2100*time.Millisecond - elapsed)
		}

		res, _ = limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "request after oldest expires should be allowed")
		res, _ = limiter.Allow(ctx, key)
		assert.False(t, res.Allowed, "next request should be rejected (at limit)")
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindow(100, 60)
		require.NoError(t, err)

		allowed := make(chan bool, 200)
		for i := 0; i < 200; i++ {
			go func() {
				res, _ := limiter.Allow(ctx, key)
				allowed <- res.Allowed
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
}

func TestSlidingWindow_Allow_Redis(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	limiter, err := goratelimit.NewSlidingWindow(10, 60, goratelimit.WithRedis(client))
	require.NoError(t, err)

	t.Run("allows requests within limit", func(t *testing.T) {
		key := fmt.Sprintf("test-sliding-user-1-%d", time.Now().UnixNano())
		res, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, res.Allowed, "first request should be allowed")
		assert.GreaterOrEqual(t, res.Remaining, int64(0))
		assert.LessOrEqual(t, res.Remaining, res.Limit)
		assert.Equal(t, time.Duration(0), res.RetryAfter, "retryAfter should be 0 when allowed")
	})

	t.Run("rejects requests exceeding limit", func(t *testing.T) {
		key := fmt.Sprintf("test-sliding-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewSlidingWindow(3, 60, goratelimit.WithRedis(client))
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
		assert.LessOrEqual(t, res.RetryAfter, 60*time.Second, "retryAfter should not exceed window")
	})

	t.Run("sliding window removes old entries", func(t *testing.T) {
		key := fmt.Sprintf("test-sliding-user-3-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewSlidingWindow(2, 2, goratelimit.WithRedis(client))
		require.NoError(t, err)

		limiter.Allow(ctx, key)
		limiter.Allow(ctx, key)

		res, _ := limiter.Allow(ctx, key)
		assert.False(t, res.Allowed, "third request should be rejected")

		time.Sleep(2100 * time.Millisecond)

		res, _ = limiter.Allow(ctx, key)
		assert.True(t, res.Allowed, "request after window slide should be allowed")
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-sliding-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-sliding-user-5-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewSlidingWindow(2, 60, goratelimit.WithRedis(client))
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
}
