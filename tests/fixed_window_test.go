package goratelimit_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krishna-kudari/ratelimit"
	"github.com/redis/go-redis/v9"
)

func TestNewFixedWindow(t *testing.T) {
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
			limiter, err := goratelimit.NewFixedWindow(tt.maxRequests, tt.windowSeconds)
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

func TestFixedWindow_Allow(t *testing.T) {
	ctx := context.Background()
	key := "test-key"

	t.Run("allows requests within limit", func(t *testing.T) {
		limiter, err := goratelimit.NewFixedWindow(5, 60)
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
		limiter, err := goratelimit.NewFixedWindow(3, 60)
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

	t.Run("resets window after time expires", func(t *testing.T) {
		limiter, err := goratelimit.NewFixedWindow(2, 1)
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

		time.Sleep(1100 * time.Millisecond)

		res, _ = limiter.Allow(ctx, key)
		if !res.Allowed {
			t.Error("request after window expiry should be allowed")
		}
		res, _ = limiter.Allow(ctx, key)
		if !res.Allowed {
			t.Error("second request after window expiry should be allowed")
		}
		res, _ = limiter.Allow(ctx, key)
		if res.Allowed {
			t.Error("third request after window expiry should be rejected")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		limiter, err := goratelimit.NewFixedWindow(100, 60)
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
}

func TestFixedWindow_Allow_Redis(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	limiter, err := goratelimit.NewFixedWindow(10, 60, goratelimit.WithRedis(client))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("allows requests within limit", func(t *testing.T) {
		key := fmt.Sprintf("test-user-1-%d", time.Now().UnixNano())
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
		key := fmt.Sprintf("test-user-2-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewFixedWindow(3, 60, goratelimit.WithRedis(client))
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
		if res.RetryAfter > 60*time.Second {
			t.Errorf("retryAfter should not exceed limit, got %v", res.RetryAfter)
		}
	})

	t.Run("tracks separate limits per user", func(t *testing.T) {
		user1 := fmt.Sprintf("test-user-3-%d", time.Now().UnixNano())
		user2 := fmt.Sprintf("test-user-4-%d", time.Now().UnixNano())
		limiter, err := goratelimit.NewFixedWindow(2, 60, goratelimit.WithRedis(client))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, _ = limiter.Allow(ctx, user1)
		_, _ = limiter.Allow(ctx, user1)

		res1, _ := limiter.Allow(ctx, user1)
		if res1.Allowed {
			t.Error("user1 should be rate limited")
		}

		res2, _ := limiter.Allow(ctx, user2)
		if !res2.Allowed {
			t.Error("user2 should not be rate limited")
		}
	})
}

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
