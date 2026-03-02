package goratelimit

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Rate specifies a request limit over a time window.
// Create one with [PerSecond], [PerMinute], or [PerHour].
type Rate struct {
	maxRequests   int64
	windowSeconds int64
}

// PerSecond returns a Rate allowing n requests per second.
func PerSecond(n int64) Rate {
	return Rate{maxRequests: n, windowSeconds: 1}
}

// PerMinute returns a Rate allowing n requests per minute.
func PerMinute(n int64) Rate {
	return Rate{maxRequests: n, windowSeconds: 60}
}

// PerHour returns a Rate allowing n requests per hour.
func PerHour(n int64) Rate {
	return Rate{maxRequests: n, windowSeconds: 3600}
}

// New creates a rate limiter with sensible defaults (Fixed Window algorithm).
// Pass an empty redisURL for in-memory mode, or a Redis URL
// (e.g. "redis://localhost:6379/0") for distributed mode.
//
//	limiter, err := goratelimit.New("", goratelimit.PerMinute(100))
//
//	limiter, err := goratelimit.New("redis://localhost:6379", goratelimit.PerMinute(100))
func New(redisURL string, rate Rate, opts ...Option) (Limiter, error) {
	if rate.maxRequests <= 0 || rate.windowSeconds <= 0 {
		return nil, fmt.Errorf("goratelimit: rate must have positive limit and window")
	}

	if redisURL != "" {
		ropts, err := redis.ParseURL(redisURL)
		if err != nil {
			return nil, fmt.Errorf("goratelimit: invalid redis URL %q: %w", redisURL, err)
		}
		client := redis.NewClient(ropts)
		opts = append([]Option{WithRedis(client)}, opts...)
	}

	return NewFixedWindow(rate.maxRequests, rate.windowSeconds, opts...)
}

// NewInMemory creates an in-memory rate limiter — ideal for tests and
// single-process deployments.
//
//	limiter, err := goratelimit.NewInMemory(goratelimit.PerMinute(100))
func NewInMemory(rate Rate, opts ...Option) (Limiter, error) {
	return New("", rate, opts...)
}
