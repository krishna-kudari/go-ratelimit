package goratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type fixedWindowRateLimiter struct {
	mu             sync.Mutex
	max_requests   int64
	window_seconds int64
	requests       int64
	window_start   time.Time
}

func (r *fixedWindowRateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if now.Sub(r.window_start) >= time.Duration(r.window_seconds)*time.Second {
		r.window_start = time.Now()
		r.requests = 0
	}

	if r.requests < r.max_requests {
		r.requests++
		return true
	}
	return false
}

func NewFixedWindowRateLimitter(max_requests int64, window_seconds int64) (*fixedWindowRateLimiter, error) {
	if max_requests <= 0 || window_seconds <= 0 {
		return nil, fmt.Errorf("goratelimit.NewFixedWindowRateLimitter: max_requests and window_seconds must be positive and greater than zero")
	}
	return &fixedWindowRateLimiter{
		max_requests:   max_requests,
		window_seconds: window_seconds,
		requests:       0,
		window_start:   time.Now(),
	}, nil
}

type RateLimitStore struct {
	mu       sync.Mutex
	limiters map[string]*fixedWindowRateLimiter
}

func (r *RateLimitStore) Allow(ctx context.Context, useId string) bool {

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.limiters[useId]; !ok {
		r.limiters[useId], _ = NewFixedWindowRateLimitter(100, 60)
	}
	return r.limiters[useId].Allow()
}

type RedisRateLimiter struct {
	redis         *redis.Client
	MaxRequests   int64
	WindowSeconds int64
}

func NewRedisRateLimiter(ctx context.Context, max_requests int64, window_seconds int64) (*RedisRateLimiter, error) {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
		Protocol: 2,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}
	return &RedisRateLimiter{
		redis:         redisClient,
		MaxRequests:   max_requests,
		WindowSeconds: window_seconds,
	}, nil
}

func (r *RedisRateLimiter) Allow(ctx context.Context, userID string) (allowed bool, remaining int64, limit int64, retryAfter int64) {
	key := fmt.Sprintf("ratelimit:%s", userID)
	max_requests, window_seconds := r.MaxRequests, r.WindowSeconds
	allowed = true
	limit = max_requests

	count, err := r.redis.Incr(ctx, key).Result()
	if err != nil {
		// false open approach
		return allowed, max_requests - 1, max_requests, 0
	}
	if count == 1 {
		_, err = r.redis.Expire(ctx, key, time.Duration(window_seconds)*time.Second).Result()
		if err != nil {
			return allowed, max_requests - 1, max_requests, 0
		}
	}

	allowed = count <= max_requests
	remaining = max(max_requests-count, 0)
	if !allowed {
		TTL, err := r.redis.TTL(ctx, key).Result()
		if err != nil {
			retryAfter = window_seconds
			return
		}
		if TTL.Seconds() > 0 {
			retryAfter = int64(TTL.Seconds())
		} else {
			retryAfter = window_seconds
		}
	}
	return
}
