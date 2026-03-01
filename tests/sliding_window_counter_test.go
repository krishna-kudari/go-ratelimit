package goratelimit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krishna-kudari/ratelimit"
	"github.com/redis/go-redis/v9"
)

func TestNewSlidingWindowCounter(t *testing.T) {
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
			limiter, err := goratelimit.NewSlidingWindowCounter(tt.maxRequests, tt.windowSeconds)
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

func TestSlidingWindowCounter_Allow(t *testing.T) {
	ctx := context.Background()
	key := "test-key"

	t.Run("allows requests within limit", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindowCounter(5, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 5; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}
	})

	t.Run("rejects requests exceeding limit", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindowCounter(3, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 3; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		res, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Allowed {
			t.Error("4th request should be rejected")
		}
	})

	t.Run("sliding window counter weights previous window", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindowCounter(10, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 8; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		time.Sleep(2100 * time.Millisecond)

		for i := 0; i < 2; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Allowed {
				t.Errorf("request %d in new window should be allowed", i+1)
			}
		}
	})

	t.Run("sliding window counter gradually allows requests as previous window expires", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindowCounter(10, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 10; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		res, _ := limiter.Allow(ctx, key)
		if res.Allowed {
			t.Error("11th request should be rejected")
		}

		time.Sleep(2100 * time.Millisecond)
		time.Sleep(300 * time.Millisecond)

		allowed := 0
		for i := 0; i < 3; i++ {
			res, _ := limiter.Allow(ctx, key)
			if res.Allowed {
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
		limiter, err := goratelimit.NewSlidingWindowCounter(100, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

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

		if count != 100 {
			t.Errorf("expected exactly 100 allowed requests, got %d", count)
		}
	})

	t.Run("window reset behavior", func(t *testing.T) {
		limiter, err := goratelimit.NewSlidingWindowCounter(5, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 5; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}
		res, _ := limiter.Allow(ctx, key)
		if res.Allowed {
			t.Error("6th request should be rejected")
		}

		time.Sleep(1100 * time.Millisecond)
		time.Sleep(200 * time.Millisecond)

		res, _ = limiter.Allow(ctx, key)
		if !res.Allowed {
			t.Error("request after previous window weight decreases should be allowed")
		}
	})
}

func TestSlidingWindowCounter_Allow_Redis(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	limiter, err := goratelimit.NewSlidingWindowCounter(10, 60, goratelimit.WithRedis(client))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("allows requests within limit", func(t *testing.T) {
		key := fmt.Sprintf("test-counter-user-1-%d", time.Now().UnixNano())
		res, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Allowed {
			t.Error("first request should be allowed")
		}
		if res.Remaining < 0 || res.Remaining > res.Limit {
			t.Errorf("remaining should be between 0 and %d, got %d", res.Limit, res.Remaining)
		}
		if res.RetryAfter != 0 {
			t.Errorf("retryAfter should be 0 when allowed, got %v", res.RetryAfter)
		}
	})

	t.Run("rejects requests exceeding limit", func(t *testing.T) {
		key := fmt.Sprintf("test-counter-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewSlidingWindowCounter(3, 60, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 3; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		res, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Allowed {
			t.Error("4th request should be rejected")
		}
		if res.Remaining != 0 {
			t.Errorf("remaining should be 0, got %d", res.Remaining)
		}
		if res.RetryAfter < 0 {
			t.Errorf("retryAfter should be non-negative, got %v", res.RetryAfter)
		}
		if res.RetryAfter > 60*time.Second {
			t.Errorf("retryAfter should not exceed window, got %v", res.RetryAfter)
		}
	})

	t.Run("sliding window counter weights previous window", func(t *testing.T) {
		key := fmt.Sprintf("test-counter-user-3-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewSlidingWindowCounter(10, 2, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 10; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		res, _ := limiter.Allow(ctx, key)
		if res.Allowed {
			t.Error("11th request should be rejected")
		}

		time.Sleep(2100 * time.Millisecond)

		maxAttempts := 5
		allowed := false
		for i := 0; i < maxAttempts && !allowed; i++ {
			time.Sleep(300 * time.Millisecond)
			res, _ := limiter.Allow(ctx, key)
			allowed = res.Allowed
		}
		if !allowed {
			t.Error("request in new window should eventually be allowed as previous window weight decreases")
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-counter-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-counter-user-5-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewSlidingWindowCounter(2, 60, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		res, _ := limiter.Allow(ctx, user1)
		if !res.Allowed {
			t.Error("user1 first request should be allowed")
		}
		res, _ = limiter.Allow(ctx, user1)
		if !res.Allowed {
			t.Error("user1 second request should be allowed")
		}

		res1, _ := limiter.Allow(ctx, user1)
		if res1.Allowed {
			t.Error("user1 should be rate limited")
		}

		res2, _ := limiter.Allow(ctx, user2)
		if !res2.Allowed {
			t.Error("user2 should not be rate limited")
		}
	})

	t.Run("gradual request allowance as window slides", func(t *testing.T) {
		key := fmt.Sprintf("test-counter-user-6-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewSlidingWindowCounter(10, 2, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 10; i++ {
			limiter.Allow(ctx, key)
		}

		res, _ := limiter.Allow(ctx, key)
		if res.Allowed {
			t.Error("11th request should be rejected")
		}

		time.Sleep(2100 * time.Millisecond)
		time.Sleep(1100 * time.Millisecond)

		allowedCount := 0
		for i := 0; i < 6; i++ {
			res, _ := limiter.Allow(ctx, key)
			if res.Allowed {
				allowedCount++
			} else {
				break
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
