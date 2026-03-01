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

// NewSlidingWindowCounter creates a Sliding Window Counter rate limiter.
// This uses the weighted-counter approximation (~1% error) with O(1) memory per key.
// maxRequests is the maximum requests allowed per window.
// windowSeconds is the window duration in seconds.
// Pass WithRedis for distributed mode; omit for in-memory.
func NewSlidingWindowCounter(maxRequests, windowSeconds int64, opts ...Option) (Limiter, error) {
	if maxRequests <= 0 || windowSeconds <= 0 {
		return nil, fmt.Errorf("goratelimit: maxRequests and windowSeconds must be positive")
	}
	o := applyOptions(opts)

	if o.RedisClient != nil {
		return &slidingWindowCounterRedis{
			redis:         o.RedisClient,
			maxRequests:   maxRequests,
			windowSeconds: windowSeconds,
			opts:          o,
		}, nil
	}
	return &slidingWindowCounterMemory{
		states:        make(map[string]*slidingWindowCounterState),
		maxRequests:   maxRequests,
		windowSeconds: windowSeconds,
		opts:          o,
	}, nil
}

// ─── In-Memory ───────────────────────────────────────────────────────────────

type slidingWindowCounterState struct {
	windowStart   time.Time
	previousCount int64
	currentCount  int64
}

type slidingWindowCounterMemory struct {
	mu            sync.Mutex
	states        map[string]*slidingWindowCounterState
	maxRequests   int64
	windowSeconds int64
	opts          *Options
}

func (s *slidingWindowCounterMemory) Allow(ctx context.Context, key string) (*Result, error) {
	return s.AllowN(ctx, key, 1)
}

func (s *slidingWindowCounterMemory) AllowN(ctx context.Context, key string, n int) (*Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.states[key]
	if !ok {
		state = &slidingWindowCounterState{windowStart: time.Now()}
		s.states[key] = state
	}

	now := time.Now()
	windowDuration := time.Duration(s.windowSeconds) * time.Second

	for now.Sub(state.windowStart) >= windowDuration {
		state.previousCount = state.currentCount
		state.currentCount = 0
		state.windowStart = state.windowStart.Add(windowDuration)
	}

	elapsedFraction := now.Sub(state.windowStart).Seconds() / float64(s.windowSeconds)
	prevWeight := float64(state.previousCount) * (1 - elapsedFraction)
	estimatedCount := prevWeight + float64(state.currentCount)

	cost := float64(n)
	if estimatedCount+cost <= float64(s.maxRequests) {
		state.currentCount += int64(n)
		newEstimate := prevWeight + float64(state.currentCount)
		remaining := int64(math.Max(0, math.Floor(float64(s.maxRequests)-newEstimate)))
		return &Result{
			Allowed:   true,
			Remaining: remaining,
			Limit:     s.maxRequests,
		}, nil
	}

	retryAfter := time.Duration(math.Ceil(float64(s.windowSeconds)*(1-elapsedFraction))) * time.Second
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return &Result{
		Allowed:    false,
		Remaining:  0,
		Limit:      s.maxRequests,
		RetryAfter: retryAfter,
	}, nil
}

func (s *slidingWindowCounterMemory) Reset(ctx context.Context, key string) error {
	s.mu.Lock()
	delete(s.states, key)
	s.mu.Unlock()
	return nil
}

// ─── Redis ────────────────────────────────────────────────────────────────────

type slidingWindowCounterRedis struct {
	redis         *redis.Client
	maxRequests   int64
	windowSeconds int64
	opts          *Options
}

func (s *slidingWindowCounterRedis) Allow(ctx context.Context, key string) (*Result, error) {
	return s.AllowN(ctx, key, 1)
}

func (s *slidingWindowCounterRedis) AllowN(ctx context.Context, key string, n int) (*Result, error) {
	now := time.Now().Unix()
	currentWindow := now / s.windowSeconds
	previousWindow := currentWindow - 1
	elapsed := float64(now%s.windowSeconds) / float64(s.windowSeconds)

	prefix := s.opts.KeyPrefix
	currentKey := fmt.Sprintf("%s:%s:%d", prefix, key, currentWindow)
	previousKey := fmt.Sprintf("%s:%s:%d", prefix, key, previousWindow)

	prevStr, err := s.redis.Get(ctx, previousKey).Result()
	if err != nil && err != redis.Nil {
		return s.failResult(err)
	}
	prevCount, _ := strconv.ParseFloat(prevStr, 64)
	weightedPrev := prevCount * (1 - elapsed)

	currStr, err := s.redis.Get(ctx, currentKey).Result()
	if err != nil && err != redis.Nil {
		return s.failResult(err)
	}
	currentCount, _ := strconv.ParseFloat(currStr, 64)

	estimatedCount := weightedPrev + currentCount
	cost := float64(n)

	if estimatedCount+cost > float64(s.maxRequests) {
		retryAfter := int64(math.Ceil(float64(s.windowSeconds) * (1 - elapsed)))
		if retryAfter < 1 {
			retryAfter = 1
		}
		if retryAfter > s.windowSeconds {
			retryAfter = s.windowSeconds
		}
		return &Result{
			Allowed:    false,
			Remaining:  0,
			Limit:      s.maxRequests,
			RetryAfter: time.Duration(retryAfter) * time.Second,
		}, nil
	}

	newCount, err := s.redis.IncrBy(ctx, currentKey, int64(n)).Result()
	if err != nil {
		return s.failResult(err)
	}
	if newCount == int64(n) {
		s.redis.Expire(ctx, currentKey, time.Duration(s.windowSeconds*2)*time.Second)
	}

	newEstimate := weightedPrev + float64(newCount)
	remaining := int64(math.Max(0, math.Floor(float64(s.maxRequests)-newEstimate)))

	return &Result{
		Allowed:   true,
		Remaining: remaining,
		Limit:     s.maxRequests,
	}, nil
}

func (s *slidingWindowCounterRedis) Reset(ctx context.Context, key string) error {
	now := time.Now().Unix()
	currentWindow := now / s.windowSeconds
	previousWindow := currentWindow - 1
	prefix := s.opts.KeyPrefix
	currentKey := fmt.Sprintf("%s:%s:%d", prefix, key, currentWindow)
	previousKey := fmt.Sprintf("%s:%s:%d", prefix, key, previousWindow)
	return s.redis.Del(ctx, currentKey, previousKey).Err()
}

func (s *slidingWindowCounterRedis) failResult(err error) (*Result, error) {
	if s.opts.FailOpen {
		return &Result{Allowed: true, Remaining: s.maxRequests - 1, Limit: s.maxRequests}, nil
	}
	return &Result{Allowed: false, Remaining: 0, Limit: s.maxRequests}, fmt.Errorf("goratelimit: redis error: %w", err)
}
