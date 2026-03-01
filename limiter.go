package goratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
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
	RedisClient *redis.Client
	KeyPrefix   string
	FailOpen    bool
}

// Option is a functional option for configuring a Limiter.
type Option func(*Options)

// WithRedis configures the limiter to use Redis as its backing store.
// When set, the limiter operates in distributed mode. When nil (default),
// it uses an in-memory store.
func WithRedis(client *redis.Client) Option {
	return func(o *Options) { o.RedisClient = client }
}

// WithKeyPrefix sets the prefix prepended to all Redis keys.
// Default: "ratelimit".
func WithKeyPrefix(prefix string) Option {
	return func(o *Options) { o.KeyPrefix = prefix }
}

// WithFailOpen controls behavior when Redis is unreachable.
// If true (default), requests are allowed on Redis errors.
// If false, requests are denied on Redis errors.
func WithFailOpen(failOpen bool) Option {
	return func(o *Options) { o.FailOpen = failOpen }
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
