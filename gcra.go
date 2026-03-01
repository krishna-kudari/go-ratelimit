package goratelimit

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ─── In-Memory ───────────────────────────────────────────────────────────────

type gcraRateLimiter struct {
	mu                 sync.Mutex
	emissionInterval   float64 // 1 / rate
	burstAllowance     float64 // (burst - 1) * emissionInterval
	tat                float64 // theoretical arrival time (unix seconds)
}

func NewGCRARateLimiter(rate int64, burst int64) (*gcraRateLimiter, error) {
	if rate <= 0 || burst <= 0 {
		return nil, fmt.Errorf("goratelimit: rate and burst must be positive and greater than zero")
	}
	emissionInterval := 1.0 / float64(rate)
	return &gcraRateLimiter{
		emissionInterval: emissionInterval,
		burstAllowance:   float64(burst-1) * emissionInterval,
		tat:              0,
	}, nil
}

// Allow returns (allowed, remaining, retryAfter)
func (r *gcraRateLimiter) Allow() (allowed bool, remaining int64, retryAfter float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := float64(time.Now().UnixNano()) / 1e9

	// TAT can't be in the past
	tat := math.Max(r.tat, now)
	newTAT := tat + r.emissionInterval
	diff := newTAT - now

	if diff <= r.burstAllowance+r.emissionInterval {
		r.tat = newTAT
		remaining = int64(math.Floor((r.burstAllowance - diff + r.emissionInterval) / r.emissionInterval))
		return true, remaining, 0
	}

	retryAfter = math.Ceil(diff - r.burstAllowance)
	return false, 0, retryAfter
}

// ─── Redis ────────────────────────────────────────────────────────────────────

var gcraScript = redis.NewScript(`
local key = KEYS[1]
local emission_interval = tonumber(ARGV[1])
local burst_allowance = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local tat = tonumber(redis.call('GET', key)) or now
tat = math.max(tat, now)

local new_tat = tat + emission_interval
local diff = new_tat - now

if diff <= burst_allowance + emission_interval then
    redis.call('SET', key, tostring(new_tat))
    redis.call('EXPIRE', key, math.ceil((burst_allowance + emission_interval) + 1))
    local remaining = math.floor((burst_allowance - diff + emission_interval) / emission_interval)
    return { 1, remaining, 0 }
else
    local retry_after = math.ceil(diff - burst_allowance)
    return { 0, 0, retry_after }
end
`)

type gcraRedisRateLimiter struct {
	redis            *redis.Client
	emissionInterval float64
	burstAllowance   float64
	burst            int64
}

func NewGCRARedisRateLimiter(ctx context.Context, rate int64, burst int64) (*gcraRedisRateLimiter, error) {
	if rate <= 0 || burst <= 0 {
		return nil, fmt.Errorf("goratelimit: rate and burst must be positive and greater than zero")
	}
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
		Protocol: 2,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("goratelimit: redis connection failed: %w", err)
	}
	emissionInterval := 1.0 / float64(rate)
	return &gcraRedisRateLimiter{
		redis:            redisClient,
		emissionInterval: emissionInterval,
		burstAllowance:   float64(burst-1) * emissionInterval,
		burst:            burst,
	}, nil
}

// Allow returns (allowed, remaining, limit, retryAfter)
func (r *gcraRedisRateLimiter) Allow(ctx context.Context, userID string) (allowed bool, remaining int64, limit int64, retryAfter int64) {
	limit = r.burst
	key := fmt.Sprintf("ratelimit:%s", userID)
	now := float64(time.Now().UnixNano()) / 1e9

	result, err := gcraScript.Run(ctx, r.redis, []string{key},
		r.emissionInterval,
		r.burstAllowance,
		now,
	).Int64Slice()
	if err != nil {
		return true, limit - 1, limit, 0 // fail open
	}

	allowed = result[0] == 1
	remaining = result[1]
	retryAfter = result[2]

	return allowed, remaining, limit, retryAfter
}
