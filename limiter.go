package goratelimit

import (
	"context"
	"time"
)

type Limiter interface {
	Allow(ctx context.Context, key string) (*Result, error)

	AllowN(ctx context.Context, key string, n int ) (*Result, error)

	Reset(ctx context.Context, key string) error
}


type Result struct {
	Alllowed bool
	Remaining int64
	ResetAt time.Time
	RetryAfter time.Duration
}
