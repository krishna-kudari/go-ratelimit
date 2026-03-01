package goratelimit

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type tokenBucketRateLimiter struct {
	mu            sync.Mutex
	tokens        int64
	capacity      int64
	lastRefilTime time.Time
	refilRate     int64
}

func (r *tokenBucketRateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	tokens := now.Sub(r.lastRefilTime).Seconds() * float64(r.refilRate)
	r.tokens = min(r.capacity, r.tokens+int64(tokens))
	r.lastRefilTime = now
	if r.tokens > 0 {
		r.tokens--
		return true
	}
	return false
}

func NewtokenBucketRateLimitter(maxCapacity int64, refilRate int64) (*tokenBucketRateLimiter, error) {
	if maxCapacity <= 0 || refilRate <= 0 {
		return nil, fmt.Errorf("goratelimit.NewFixedWindowRateLimitter: max_requests and window_seconds must be positive and greater than zero")
	}
	return &tokenBucketRateLimiter{
		capacity:      maxCapacity,
		tokens:        maxCapacity,
		refilRate:     refilRate,
		lastRefilTime: time.Now(),
	}, nil
}

type tokenBucketRateLimitStore struct {
	mu       sync.Mutex
	limiters map[string]*tokenBucketRateLimiter
}

func (r *tokenBucketRateLimitStore) Allow(ctx context.Context, useId string) bool {

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.limiters[useId]; !ok {
		r.limiters[useId], _ = NewtokenBucketRateLimitter(100, 60)
	}
	return r.limiters[useId].Allow()
}

type tokenBucketRedisRateLimiter struct {
	redis         *redis.Client
	capacity   int64
	refilRate int64
}

func NewtokenBucketRedisRateLimiter(ctx context.Context, capacity int64, refilRate int64) (*tokenBucketRedisRateLimiter, error) {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
		Protocol: 2,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}
	return &tokenBucketRedisRateLimiter{
		redis:         redisClient,
		capacity:   capacity,
		refilRate: refilRate,
	}, nil
}

var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local max_tokens = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local data = redis.call('HGETALL', key)
local tokens = max_tokens
local last_refill = now

if #data > 0 then
  local fields = {}
  for i = 1, #data, 2 do
    fields[data[i]] = data[i + 1]
  end
  tokens = tonumber(fields['tokens']) or max_tokens
  last_refill = tonumber(fields['last_refill']) or now
end

local elapsed = now - last_refill
tokens = math.min(max_tokens, tokens + elapsed * refill_rate)

local allowed = 0
local remaining = tokens

if tokens >= 1 then
  tokens = tokens - 1
  remaining = tokens
  allowed = 1
end

redis.call('HSET', key, 'tokens', tostring(tokens), 'last_refill', tostring(now))
redis.call('EXPIRE', key, math.ceil(max_tokens / refill_rate) + 1)

return { allowed, math.floor(remaining) }
`)
func (r *tokenBucketRedisRateLimiter) Allow(ctx context.Context, userID string) (allowed bool, remaining int64, limit int64, retryAfter int64) {
	capacity, refilRate := r.capacity, r.refilRate
	limit = capacity

	key := fmt.Sprintf("ratelimit:%s", userID)
	now := float64(time.Now().UnixNano()) / 1e9 // fractional seconds

	result, err := tokenBucketScript.Run(ctx, r.redis, []string{key},
		capacity,
		refilRate,
		now,
	).Int64Slice()
	if err != nil {
		return true, limit - 1, limit, 0 // fail open
	}

	allowed = result[0] == 1
	remaining = result[1]

	if !allowed {
		retryAfter = int64(math.Ceil(1.0 / float64(refilRate)))
		if retryAfter < 1 {
			retryAfter = 1
		}
	}

	return allowed, remaining, limit, retryAfter
}
