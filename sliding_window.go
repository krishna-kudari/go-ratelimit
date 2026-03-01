package goratelimit

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type slidingWindowRateLimiter struct {
	mu             sync.Mutex
	max_requests   int64
	window_seconds int64
	timestamps []time.Time
}

func (r *slidingWindowRateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for len(r.timestamps) > 0 && now.Sub(r.timestamps[0])*time.Second > time.Duration(r.window_seconds)*time.Second {
		r.timestamps = r.timestamps[1:]
 	}

	if len(r.timestamps) < int(r.max_requests) {
		r.timestamps = append(r.timestamps, time.Now())
		return true
	}
	return false
}

func NewSlidingWindowRateLimitter(max_requests int64, window_seconds int64) (*slidingWindowRateLimiter, error) {
	if max_requests <= 0 || window_seconds <= 0 {
		return nil, fmt.Errorf("goratelimit.NewFixedWindowRateLimitter: max_requests and window_seconds must be positive and greater than zero")
	}
	return &slidingWindowRateLimiter{
		max_requests:   max_requests,
		window_seconds: window_seconds,
		timestamps: []time.Time{},
	}, nil
}

type slidingWindowRateLimitStore struct {
	mu       sync.Mutex
	limiters map[string]*slidingWindowRateLimiter
}

func (r *slidingWindowRateLimitStore) Allow(ctx context.Context, useId string) bool {

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.limiters[useId]; !ok {
		r.limiters[useId], _ = NewSlidingWindowRateLimitter(100, 60)
	}
	return r.limiters[useId].Allow()
}

type SlidingWindowRedisRateLimiter struct {
	redis         *redis.Client
	MaxRequests   int64
	WindowSeconds int64
}

func NewSlidingWindowRedisRateLimiter(ctx context.Context, max_requests int64, window_seconds int64) (*RedisRateLimiter, error) {
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

func (r *SlidingWindowRedisRateLimiter) Allow(ctx context.Context, userID string) (allowed bool, remaining int64, limit int64, retryAfter int64) {
	key := fmt.Sprintf("ratelimit:%s", userID)
	maxRequests, windowSeconds := r.MaxRequests, r.WindowSeconds
	allowed = true
	limit = maxRequests

	now := time.Now().UnixMilli()
	window_start := now - windowSeconds*1000

	err := r.redis.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", window_start)).Err()
	if err != nil {
		return true, maxRequests - 1, maxRequests, 0
	}

	count, err := r.redis.ZCard(ctx, key).Result()
	if err != nil {
		return true, maxRequests -1, maxRequests, 0
	}
	if count < maxRequests {

		member := fmt.Sprintf("%d:%d", now, rand.Int63())
		err := r.redis.ZAdd(ctx, key, redis.Z{
			Score: float64(now),
			Member: member,
		}).Err()
		if err != nil {
			return true, maxRequests - 1, maxRequests, 0
		}

		r.redis.Expire(ctx, key, time.Duration(windowSeconds)*time.Second)
		return true, maxRequests - count - 1, maxRequests, 0
	}

	oldest, err := r.redis.ZRangeWithScores(ctx, key, 0, 0).Result()
	retryAfter = windowSeconds
	if err == nil && len(oldest) > 0 {
		oldestScore := int64(oldest[0].Score)
		retryAfter = (oldestScore+windowSeconds*1000-now)/1000 + 1
		if retryAfter < 1 {
			retryAfter = 1
		}
	}
	return false, 0, maxRequests, retryAfter
}
