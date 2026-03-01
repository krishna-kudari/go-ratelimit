package goratelimit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krishna-kudari/ratelimit"
	"github.com/redis/go-redis/v9"
)

func TestNewGCRA(t *testing.T) {
	tests := []struct {
		name           string
		rate           int64
		burst          int64
		expectError    bool
		errorSubstring string
	}{
		{
			name:        "valid parameters",
			rate:        10,
			burst:       20,
			expectError: false,
		},
		{
			name:           "zero rate",
			rate:           0,
			burst:          20,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative rate",
			rate:           -1,
			burst:          20,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "zero burst",
			rate:           10,
			burst:          0,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:           "negative burst",
			rate:           10,
			burst:          -1,
			expectError:    true,
			errorSubstring: "must be positive",
		},
		{
			name:        "burst equals rate",
			rate:        10,
			burst:       10,
			expectError: false,
		},
		{
			name:        "burst greater than rate",
			rate:        10,
			burst:       30,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := goratelimit.NewGCRA(tt.rate, tt.burst)
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

func TestGCRA_Allow(t *testing.T) {
	ctx := context.Background()
	key := "test"

	t.Run("allows requests within burst", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 5) // 10 per second, burst of 5
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
			if res.Remaining < 0 {
				t.Errorf("remaining should be non-negative, got %d", res.Remaining)
			}
			if res.RetryAfter != 0 {
				t.Errorf("retryAfter should be 0 when allowed, got %v", res.RetryAfter)
			}
		}
	})

	t.Run("rejects requests exceeding burst", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 3) // 10 per second, burst of 3
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
			t.Errorf("remaining should be 0 when rejected, got %d", res.Remaining)
		}
		if res.RetryAfter <= 0 {
			t.Errorf("retryAfter should be positive when rejected, got %v", res.RetryAfter)
		}
	})

	t.Run("allows requests after rate limit period", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(2, 2) // 2 per second, burst of 2
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		res, _ := limiter.Allow(ctx, key)
		if !res.Allowed {
			t.Error("first request should be allowed")
		}
		res, _ = limiter.Allow(ctx, key)
		if !res.Allowed {
			t.Error("second request should be allowed")
		}
		res, _ = limiter.Allow(ctx, key)
		if res.Allowed {
			t.Error("third request should be rejected")
		}

		time.Sleep(600 * time.Millisecond)

		res, _ = limiter.Allow(ctx, key)
		if !res.Allowed {
			t.Error("request after rate limit period should be allowed")
		}
	})

	t.Run("allows steady rate of requests", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 10) // 10 per second, burst of 10
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 10; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Allowed {
				t.Errorf("request %d should be allowed at steady rate", i+1)
			}
			time.Sleep(100 * time.Millisecond)
		}
	})

	t.Run("remaining count decreases as requests are made", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 5) // 10 per second, burst of 5
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		prevRemaining := int64(5)
		for i := 0; i < 5; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Remaining >= prevRemaining {
				t.Errorf("remaining should decrease, got %d (previous: %d)", res.Remaining, prevRemaining)
			}
			prevRemaining = res.Remaining
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(100, 50) // 100 per second, burst of 50
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

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

		if count > 50 {
			t.Errorf("expected at most 50 allowed requests (burst), got %d", count)
		}
		if count < 50 {
			t.Errorf("expected at least 50 allowed requests (burst), got %d", count)
		}
	})

	t.Run("allows burst after waiting", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(5, 3) // 5 per second, burst of 3
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		initialAllowed := 0
		for i := 0; i < 5; i++ {
			res, _ := limiter.Allow(ctx, key)
			if res.Allowed {
				initialAllowed++
			} else {
				break
			}
		}

		if initialAllowed < 1 {
			t.Fatalf("expected at least 1 request to be allowed initially, got %d", initialAllowed)
		}

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
		if !res.Allowed {
			t.Error("request should be allowed after waiting emission interval")
		}
	})

	t.Run("retryAfter is calculated correctly", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 2) // 10 per second, burst of 2
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, _ = limiter.Allow(ctx, key)
		_, _ = limiter.Allow(ctx, key)

		res, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RetryAfter <= 0 {
			t.Errorf("retryAfter should be positive, got %v", res.RetryAfter)
		}
		retrySec := res.RetryAfter.Seconds()
		if retrySec < 0.1 || retrySec > 2.0 {
			t.Errorf("retryAfter should be between 0.1 and 2.0 seconds (rounded up), got %v", res.RetryAfter)
		}
	})
}

func TestGCRA_Reset(t *testing.T) {
	ctx := context.Background()
	key := "test-reset"

	t.Run("reset clears state and allows burst again", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 3; i++ {
			res, _ := limiter.Allow(ctx, key)
			if !res.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}
		res, _ := limiter.Allow(ctx, key)
		if res.Allowed {
			t.Error("4th request should be rejected")
		}

		if err := limiter.Reset(ctx, key); err != nil {
			t.Fatalf("unexpected reset error: %v", err)
		}

		for i := 0; i < 3; i++ {
			res, _ := limiter.Allow(ctx, key)
			if !res.Allowed {
				t.Errorf("after reset: request %d should be allowed", i+1)
			}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if limiter == nil {
			t.Error("expected limiter to be non-nil")
		}
	})

	t.Run("invalid parameters - zero rate", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(0, 20, goratelimit.WithRedis(client))
		if err == nil {
			t.Error("expected error for zero rate")
		}
		if limiter != nil {
			t.Error("expected limiter to be nil on error")
		}
	})

	t.Run("invalid parameters - zero burst", func(t *testing.T) {
		limiter, err := goratelimit.NewGCRA(10, 0, goratelimit.WithRedis(client))
		if err == nil {
			t.Error("expected error for zero burst")
		}
		if limiter != nil {
			t.Error("expected limiter to be nil on error")
		}
	})
}

func TestGCRA_Redis_Allow(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	limiter, err := goratelimit.NewGCRA(10, 20, goratelimit.WithRedis(client))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("allows requests within burst", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-1-%d", time.Now().UnixNano())
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

	t.Run("rejects requests exceeding burst", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(10, 3, goratelimit.WithRedis(client))
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
		if res.RetryAfter <= 0 {
			t.Errorf("retryAfter should be positive, got %v", res.RetryAfter)
		}
	})

	t.Run("allows requests after rate limit period", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-3-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(2, 2, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		limiter.Allow(ctx, key)
		limiter.Allow(ctx, key)

		res, _ := limiter.Allow(ctx, key)
		if res.Allowed {
			t.Error("third request should be rejected")
		}

		time.Sleep(600 * time.Millisecond)

		res, _ = limiter.Allow(ctx, key)
		if !res.Allowed {
			t.Error("request after rate limit period should be allowed")
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-gcra-user-4-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-gcra-user-5-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(10, 2, goratelimit.WithRedis(client))
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

	t.Run("allows steady rate of requests", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-6-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(10, 10, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 10; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Allowed {
				t.Errorf("request %d should be allowed at steady rate", i+1)
			}
			time.Sleep(100 * time.Millisecond)
		}
	})

	t.Run("remaining count decreases as requests are made", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-7-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(10, 5, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		prevRemaining := int64(5)
		for i := 0; i < 5; i++ {
			res, err := limiter.Allow(ctx, key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Remaining >= prevRemaining {
				t.Errorf("remaining should decrease, got %d (previous: %d)", res.Remaining, prevRemaining)
			}
			prevRemaining = res.Remaining
		}
	})

	t.Run("allows burst after waiting", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-8-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(5, 3, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		limiter.Allow(ctx, key)
		limiter.Allow(ctx, key)
		limiter.Allow(ctx, key)

		res, _ := limiter.Allow(ctx, key)
		if res.Allowed {
			t.Error("request should be rejected after burst")
		}

		time.Sleep(250 * time.Millisecond)

		res, _ = limiter.Allow(ctx, key)
		if !res.Allowed {
			t.Error("request should be allowed after waiting emission interval")
		}
	})

	t.Run("retryAfter is calculated correctly", func(t *testing.T) {
		key := fmt.Sprintf("test-gcra-user-9-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewGCRA(10, 2, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		limiter.Allow(ctx, key)
		limiter.Allow(ctx, key)

		res, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.RetryAfter <= 0 {
			t.Errorf("retryAfter should be positive, got %v", res.RetryAfter)
		}
		if res.RetryAfter > time.Second {
			t.Errorf("retryAfter should be approximately 1 second (rounded up), got %v", res.RetryAfter)
		}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < 3; i++ {
			res, _ := limiter.Allow(ctx, key)
			if !res.Allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}
		res, _ := limiter.Allow(ctx, key)
		if res.Allowed {
			t.Error("4th request should be rejected")
		}

		if err := limiter.Reset(ctx, key); err != nil {
			t.Fatalf("unexpected reset error: %v", err)
		}

		for i := 0; i < 3; i++ {
			res, _ := limiter.Allow(ctx, key)
			if !res.Allowed {
				t.Errorf("after reset: request %d should be allowed", i+1)
			}
		}
	})
}
