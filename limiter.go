package goratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/krishna-kudari/ratelimit/store"
)

// Limiter is the core interface for all rate limiting algorithms.
// All implementations (in-memory and Redis-backed) satisfy this interface,
// making algorithms swappable without changing caller code.
type Limiter interface {
	// Allow checks whether a single request identified by key should be allowed.
	Allow(ctx context.Context, key string) (*Result, error)

	// AllowN checks whether n requests identified by key should be allowed.
	AllowN(ctx context.Context, key string, n int) (*Result, error)

	// Reset clears all rate limit state for the given key.
	Reset(ctx context.Context, key string) error
}

// Result holds the outcome of a rate limit check.
type Result struct {
	Allowed    bool
	Remaining  int64
	Limit      int64
	ResetAt    time.Time
	RetryAfter time.Duration
}

// Options configures behavior shared across all algorithm implementations.
type Options struct {
	// Store is the pluggable backend for rate limit state.
	// Takes precedence over RedisClient if both are set.
	Store store.Store

	// RedisClient is a Redis connection for distributed rate limiting.
	// Accepts *redis.Client, *redis.ClusterClient, *redis.Ring, or any
	// redis.UniversalClient implementation.
	RedisClient redis.UniversalClient

	// KeyPrefix is prepended to all storage keys.
	// Default: "ratelimit".
	KeyPrefix string

	// FailOpen controls behavior when the backend is unreachable.
	// If true (default), requests are allowed on errors.
	// If false, requests are denied on errors.
	FailOpen bool

	// HashTag enables Redis Cluster hash-tag wrapping of user keys.
	// When true, keys are formatted as "prefix:{key}" instead of "prefix:key",
	// ensuring all keys for the same logical entity route to the same slot.
	// This is required for Sliding Window Counter (multi-key) and recommended
	// for any Redis Cluster deployment.
	HashTag bool

	// LimitFunc dynamically resolves the rate limit for each key.
	// Returns the effective limit (maxRequests / capacity / burst) for the key.
	// Returning <= 0 falls back to the construction-time default.
	LimitFunc func(key string) int64
}

// Option is a functional option for configuring a Limiter.
type Option func(*Options)

// WithStore configures the limiter to use a custom store.Store backend.
// This takes precedence over WithRedis if both are set.
func WithStore(s store.Store) Option {
	return func(o *Options) { o.Store = s }
}

// WithRedis configures the limiter to use Redis as its backing store.
// Accepts any redis.UniversalClient: *redis.Client (standalone),
// *redis.ClusterClient (cluster), *redis.Ring (ring), or sentinel.
// When set, the limiter operates in distributed mode.
func WithRedis(client redis.UniversalClient) Option {
	return func(o *Options) { o.RedisClient = client }
}

// WithKeyPrefix sets the prefix prepended to all storage keys.
// Default: "ratelimit".
func WithKeyPrefix(prefix string) Option {
	return func(o *Options) { o.KeyPrefix = prefix }
}

// WithFailOpen controls behavior when the backend is unreachable.
// If true (default), requests are allowed on errors.
// If false, requests are denied on errors.
func WithFailOpen(failOpen bool) Option {
	return func(o *Options) { o.FailOpen = failOpen }
}

// WithHashTag enables Redis Cluster hash-tag wrapping.
// Keys become "prefix:{key}" so all keys for a given user route
// to the same Redis Cluster slot. Required for multi-key algorithms
// (Sliding Window Counter) in Cluster mode.
func WithHashTag() Option {
	return func(o *Options) { o.HashTag = true }
}

// WithLimitFunc sets a dynamic limit resolver. The function is called on
// every Allow/AllowN with the request key and returns the effective limit
// (maxRequests for window algorithms, capacity for buckets, burst for GCRA).
// Returning <= 0 falls back to the construction-time default.
func WithLimitFunc(fn func(key string) int64) Option {
	return func(o *Options) { o.LimitFunc = fn }
}

func defaultOptions() *Options {
	return &Options{
		KeyPrefix: "ratelimit",
		FailOpen:  true,
	}
}

func applyOptions(opts []Option) *Options {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// resolveLimit returns the dynamic limit for key, or defaultLimit when
// LimitFunc is nil or returns <= 0.
func (o *Options) resolveLimit(key string, defaultLimit int64) int64 {
	if o.LimitFunc != nil {
		if v := o.LimitFunc(key); v > 0 {
			return v
		}
	}
	return defaultLimit
}

// FormatKey builds a storage key. With HashTag enabled the user key is
// wrapped in {}: "prefix:{key}" so all derived keys for the same user
// land on the same Redis Cluster slot.
func (o *Options) FormatKey(key string) string {
	if o.HashTag {
		return o.KeyPrefix + ":{" + key + "}"
	}
	return o.KeyPrefix + ":" + key
}

// FormatKeySuffix builds a storage key with an additional suffix.
// "prefix:{key}:suffix" (hash-tag) or "prefix:key:suffix" (plain).
func (o *Options) FormatKeySuffix(key, suffix string) string {
	if o.HashTag {
		return o.KeyPrefix + ":{" + key + "}:" + suffix
	}
	return o.KeyPrefix + ":" + key + ":" + suffix
}

// redisClient returns the effective redis.UniversalClient from Options,
// checking Store (if it's a RedisStore) then falling back to RedisClient.
func (o *Options) redisClient() redis.UniversalClient {
	return o.RedisClient
}

// isRedis returns true if a Redis backend is configured.
func (o *Options) isRedis() bool {
	return o.RedisClient != nil
}
