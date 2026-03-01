package goratelimit

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type slidingWindowCounterRateLimiter struct {
	mu            sync.Mutex
	maxRequests   int64
	windowSeconds int64
	windowStart   time.Time
	previousCount int64
	currentCount  int64
}

func (r *slidingWindowCounterRateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	windowDuration := time.Duration(r.windowSeconds) * time.Second

	// Use for loop to handle multiple elapsed windows
	for now.Sub(r.windowStart) >= windowDuration {
		r.previousCount = r.currentCount
		r.currentCount = 0
		r.windowStart = r.windowStart.Add(windowDuration) // ← key fix
	}

	elapsedFraction := now.Sub(r.windowStart).Seconds() / float64(r.windowSeconds)
	prevCount := float64(r.previousCount) * (1 - elapsedFraction)
	count := prevCount + float64(r.currentCount)

	if count < float64(r.maxRequests) {
		r.currentCount++
		return true
	}
	return false
}

func NewslidingWindowCounterRateLimitter(max_requests int64, window_seconds int64) (*slidingWindowCounterRateLimiter, error) {
	if max_requests <= 0 || window_seconds <= 0 {
		return nil, fmt.Errorf("goratelimit.NewFixedWindowRateLimitter: max_requests and window_seconds must be positive and greater than zero")
	}
	return &slidingWindowCounterRateLimiter{
		maxRequests:   max_requests,
		windowSeconds: window_seconds,
		windowStart:   time.Now(),
		previousCount: 0,
		currentCount:  0,
	}, nil
}

type slidingWindowCounterRateLimitStore struct {
	mu       sync.Mutex
	limiters map[string]*slidingWindowCounterRateLimiter
}

func (r *slidingWindowCounterRateLimitStore) Allow(ctx context.Context, useId string) bool {

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.limiters[useId]; !ok {
		r.limiters[useId], _ = NewslidingWindowCounterRateLimitter(100, 60)
	}
	return r.limiters[useId].Allow()
}

type slidingWindowCounterRedisRateLimiter struct {
	redis         *redis.Client
	MaxRequests   int64
	WindowSeconds int64
}

func NewslidingWindowCounterRedisRateLimiter(ctx context.Context, max_requests int64, window_seconds int64) (*slidingWindowCounterRedisRateLimiter, error) {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
		Protocol: 2,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}
	return &slidingWindowCounterRedisRateLimiter{
		redis:         redisClient,
		MaxRequests:   max_requests,
		WindowSeconds: window_seconds,
	}, nil
}

func (r *slidingWindowCounterRedisRateLimiter) Allow(ctx context.Context, userID string) (allowed bool, remaining int64, limit int64, retryAfter int64) {
	maxRequests, windowSeconds := r.MaxRequests, r.WindowSeconds
	limit = maxRequests

	now := time.Now().Unix()
	currentWindow := now / windowSeconds
	previousWindow := currentWindow - 1

	currentKey := fmt.Sprintf("ratelimit:%s:%d", userID, currentWindow)
	previousKey := fmt.Sprintf("ratelimit:%s:%d", userID, previousWindow)

	elapsed := float64(now%windowSeconds) / float64(windowSeconds)

	prevStr, err := r.redis.Get(ctx, previousKey).Result()
	if err != nil && err != redis.Nil {
		return true, maxRequests - 1, maxRequests, 0 // fail open
	}
	prevCount, _ := strconv.ParseFloat(prevStr, 64)

	weightedPrev := prevCount * (1 - elapsed)

	currStr, err := r.redis.Get(ctx, currentKey).Result()
	if err != nil && err != redis.Nil {
		return true, maxRequests - 1, maxRequests, 0 // fail open
	}
	currentCount, _ := strconv.ParseFloat(currStr, 64)

	estimatedCount := weightedPrev + currentCount

	if estimatedCount >= float64(maxRequests) {
		retryAfter = int64(math.Ceil(float64(windowSeconds) * (1 - elapsed)))
		if retryAfter < 1 {
			retryAfter = 1
		}
		if retryAfter > windowSeconds {
			retryAfter = windowSeconds
		}
		return false, 0, maxRequests, retryAfter
	}

	newCount, err := r.redis.Incr(ctx, currentKey).Result()
	if err != nil {
		return true, maxRequests - 1, maxRequests, 0
	}
	if newCount == 1 {
		r.redis.Expire(ctx, currentKey, time.Duration(windowSeconds*2)*time.Second)
	}

	newEstimate := weightedPrev + float64(newCount)
	remaining = int64(math.Max(0, math.Floor(float64(maxRequests)-newEstimate)))

	return true, remaining, maxRequests, 0
}
