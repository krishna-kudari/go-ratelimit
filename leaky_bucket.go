package goratelimit

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type LeakyBucketMode string

const (
	Policing LeakyBucketMode = "policing"
	Shaping  LeakyBucketMode = "shaping"
)

// ─── In-Memory ───────────────────────────────────────────────────────────────

type leakyBucketRateLimiter struct {
	mu       sync.Mutex
	mode     LeakyBucketMode
	capacity float64
	leakRate float64

	// policing state
	level    float64
	lastLeak time.Time

	// shaping state
	nextFree time.Time
}

func NewLeakyBucketRateLimiter(capacity int64, leakRate int64, mode LeakyBucketMode) (*leakyBucketRateLimiter, error) {
	if capacity <= 0 || leakRate <= 0 {
		return nil, fmt.Errorf("goratelimit: capacity and leakRate must be positive and greater than zero")
	}
	now := time.Now()
	return &leakyBucketRateLimiter{
		mode:     mode,
		capacity: float64(capacity),
		leakRate: float64(leakRate),
		lastLeak: now,
		nextFree: now,
	}, nil
}

// Allow returns (allowed, remaining, delaySeconds)
func (r *leakyBucketRateLimiter) Allow() (allowed bool, remaining int64, delay float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.mode == Policing {
		return r.allowPolicing()
	}
	return r.allowShaping()
}

func (r *leakyBucketRateLimiter) allowPolicing() (allowed bool, remaining int64, delay float64) {
	now := time.Now()

	elapsed := now.Sub(r.lastLeak).Seconds()
	leaked := elapsed * r.leakRate
	r.level = math.Max(0, r.level-leaked)
	r.lastLeak = now

	remaining = int64(math.Max(0, math.Floor(r.capacity-r.level)))

	if r.level+1 <= r.capacity {
		r.level++
		remaining = int64(math.Max(0, math.Floor(r.capacity-r.level)))
		return true, remaining, 0
	}
	return false, 0, math.Ceil(1 / r.leakRate)
}

func (r *leakyBucketRateLimiter) allowShaping() (allowed bool, remaining int64, delay float64) {
	now := time.Now()

	if r.nextFree.Before(now) {
		r.nextFree = now
	}

	delayDuration := r.nextFree.Sub(now).Seconds()
	queueDepth := delayDuration * r.leakRate
	remaining = int64(math.Max(0, math.Floor(r.capacity-queueDepth)))

	if queueDepth+1 <= r.capacity {
		delay = delayDuration
		r.nextFree = r.nextFree.Add(time.Duration(float64(time.Second) / r.leakRate))
		queueDepth++
		remaining = int64(math.Max(0, math.Floor(r.capacity-queueDepth)))
		return true, remaining, delay
	}
	return false, 0, 0
}

// ─── Redis ────────────────────────────────────────────────────────────────────

var luaPolicing = redis.NewScript(`
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local leak_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local data = redis.call('HGETALL', key)
local level = 0
local last_leak = now

if #data > 0 then
  local fields = {}
  for i = 1, #data, 2 do
    fields[data[i]] = data[i + 1]
  end
  level = tonumber(fields['level']) or 0
  last_leak = tonumber(fields['last_leak']) or now
end

local elapsed = now - last_leak
local leaked = elapsed * leak_rate
level = math.max(0, level - leaked)

local allowed = 0
local remaining = math.max(0, math.floor(capacity - level))

if level + 1 <= capacity then
  level = level + 1
  remaining = math.max(0, math.floor(capacity - level))
  allowed = 1
end

redis.call('HSET', key, 'level', tostring(level), 'last_leak', tostring(now))
redis.call('EXPIRE', key, math.ceil(capacity / leak_rate) + 1)

return { allowed, remaining, 0 }
`)

var luaShaping = redis.NewScript(`
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local leak_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local data = redis.call('HGETALL', key)
local next_free = now

if #data > 0 then
  local fields = {}
  for i = 1, #data, 2 do
    fields[data[i]] = data[i + 1]
  end
  next_free = tonumber(fields['next_free']) or now
end

if next_free < now then
  next_free = now
end

local delay = next_free - now
local queue_depth = delay * leak_rate

local allowed = 0
local remaining = math.max(0, math.floor(capacity - queue_depth))
local delay_ms = 0

if queue_depth + 1 <= capacity then
  delay_ms = math.floor(delay * 1000)
  next_free = next_free + (1 / leak_rate)
  allowed = 1
  queue_depth = queue_depth + 1
  remaining = math.max(0, math.floor(capacity - queue_depth))
end

redis.call('HSET', key, 'next_free', tostring(next_free))
redis.call('EXPIRE', key, math.ceil(capacity / leak_rate) + 1)

return { allowed, remaining, delay_ms }
`)

type leakyBucketRedisRateLimiter struct {
	redis    *redis.Client
	capacity int64
	leakRate int64
	mode     LeakyBucketMode
}

func NewLeakyBucketRedisRateLimiter(ctx context.Context, capacity int64, leakRate int64, mode LeakyBucketMode) (*leakyBucketRedisRateLimiter, error) {
	if capacity <= 0 || leakRate <= 0 {
		return nil, fmt.Errorf("goratelimit: capacity and leakRate must be positive and greater than zero")
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
	return &leakyBucketRedisRateLimiter{
		redis:    redisClient,
		capacity: capacity,
		leakRate: leakRate,
		mode:     mode,
	}, nil
}

// Allow returns (allowed, remaining, limit, retryAfter, delaySeconds)
func (r *leakyBucketRedisRateLimiter) Allow(ctx context.Context, userID string) (allowed bool, remaining int64, limit int64, retryAfter int64, delay float64) {
	capacity, leakRate := r.capacity, r.leakRate
	limit = capacity

	key := fmt.Sprintf("ratelimit:%s", userID)
	now := float64(time.Now().UnixNano()) / 1e9

	script := luaPolicing
	if r.mode == Shaping {
		script = luaShaping
	}

	result, err := script.Run(ctx, r.redis, []string{key},
		capacity,
		leakRate,
		now,
	).Int64Slice()
	if err != nil {
		return true, limit - 1, limit, 0, 0 // fail open
	}

	allowed = result[0] == 1
	remaining = result[1]
	delayMs := result[2]

	if !allowed && r.mode == Policing {
		retryAfter = int64(math.Ceil(1.0 / float64(leakRate)))
		if retryAfter < 1 {
			retryAfter = 1
		}
	}

	if delayMs > 0 {
		delay = float64(delayMs) / 1000
	}

	return allowed, remaining, limit, retryAfter, delay
}
