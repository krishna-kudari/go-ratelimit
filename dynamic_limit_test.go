package goratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func limitByKey(key string) int64 {
	switch key {
	case "premium":
		return 1000
	case "free":
		return 2
	default:
		return 0 // fallback to default
	}
}

func TestDynamicLimit_FixedWindow(t *testing.T) {
	ctx := context.Background()
	l, err := NewFixedWindow(10, 60, WithLimitFunc(limitByKey))
	require.NoError(t, err)

	res, _ := l.Allow(ctx, "premium")
	assert.Equal(t, int64(1000), res.Limit)

	res, _ = l.Allow(ctx, "free")
	assert.Equal(t, int64(2), res.Limit)

	// Exhaust "free" limit
	res, _ = l.Allow(ctx, "free")
	require.True(t, res.Allowed, "second free request should be allowed (limit=2)")
	res, _ = l.Allow(ctx, "free")
	require.False(t, res.Allowed, "third free request should be denied (limit=2)")

	// "unknown" falls back to default of 10
	res, _ = l.Allow(ctx, "unknown")
	assert.Equal(t, int64(10), res.Limit)
}

func TestDynamicLimit_SlidingWindow(t *testing.T) {
	ctx := context.Background()
	l, err := NewSlidingWindow(10, 60, WithLimitFunc(limitByKey))
	require.NoError(t, err)

	res, _ := l.Allow(ctx, "premium")
	assert.Equal(t, int64(1000), res.Limit)

	res, _ = l.Allow(ctx, "free")
	assert.Equal(t, int64(2), res.Limit)
}

func TestDynamicLimit_SlidingWindowCounter(t *testing.T) {
	ctx := context.Background()
	l, err := NewSlidingWindowCounter(10, 60, WithLimitFunc(limitByKey))
	require.NoError(t, err)

	res, _ := l.Allow(ctx, "premium")
	assert.Equal(t, int64(1000), res.Limit)

	res, _ = l.Allow(ctx, "free")
	assert.Equal(t, int64(2), res.Limit)
}

func TestDynamicLimit_TokenBucket(t *testing.T) {
	ctx := context.Background()
	l, err := NewTokenBucket(10, 5, WithLimitFunc(limitByKey))
	require.NoError(t, err)

	res, _ := l.Allow(ctx, "premium")
	assert.Equal(t, int64(1000), res.Limit)

	res, _ = l.Allow(ctx, "free")
	assert.Equal(t, int64(2), res.Limit)

	// free: capacity=2, one used, one remaining
	res, _ = l.Allow(ctx, "free")
	require.True(t, res.Allowed, "second free request should be allowed (capacity=2)")
	res, _ = l.Allow(ctx, "free")
	require.False(t, res.Allowed, "third free request should be denied (capacity=2)")
}

func TestDynamicLimit_LeakyBucket(t *testing.T) {
	ctx := context.Background()
	l, err := NewLeakyBucket(10, 2, Policing, WithLimitFunc(limitByKey))
	require.NoError(t, err)

	res, _ := l.Allow(ctx, "premium")
	assert.Equal(t, int64(1000), res.Limit)

	res, _ = l.Allow(ctx, "free")
	assert.Equal(t, int64(2), res.Limit)
}

func TestDynamicLimit_GCRA(t *testing.T) {
	ctx := context.Background()
	l, err := NewGCRA(10, 5, WithLimitFunc(limitByKey))
	require.NoError(t, err)

	res, _ := l.Allow(ctx, "premium")
	assert.Equal(t, int64(1000), res.Limit)

	res, _ = l.Allow(ctx, "free")
	assert.Equal(t, int64(2), res.Limit)

	// free burst=2: one used, one remaining
	res, _ = l.Allow(ctx, "free")
	require.True(t, res.Allowed, "second free request should be allowed (burst=2)")
	res, _ = l.Allow(ctx, "free")
	require.False(t, res.Allowed, "third free request should be denied (burst=2)")
}

func TestDynamicLimit_FallbackToDefault(t *testing.T) {
	ctx := context.Background()
	fn := func(key string) int64 {
		if key == "custom" {
			return 50
		}
		return 0 // <= 0 means fallback
	}

	l, _ := NewFixedWindow(10, 60, WithLimitFunc(fn))

	res, _ := l.Allow(ctx, "custom")
	assert.Equal(t, int64(50), res.Limit)

	res, _ = l.Allow(ctx, "other")
	assert.Equal(t, int64(10), res.Limit)
}

func TestDynamicLimit_NegativeReturnFallback(t *testing.T) {
	ctx := context.Background()
	fn := func(key string) int64 { return -1 }

	l, _ := NewTokenBucket(20, 5, WithLimitFunc(fn))

	res, _ := l.Allow(ctx, "any")
	assert.Equal(t, int64(20), res.Limit)
}

func TestDynamicLimit_Builder(t *testing.T) {
	ctx := context.Background()
	l, err := NewBuilder().
		FixedWindow(10, 60*time.Second).
		LimitFunc(func(key string) int64 {
			if key == "vip" {
				return 500
			}
			return 0
		}).
		Build()
	require.NoError(t, err)

	res, _ := l.Allow(ctx, "vip")
	assert.Equal(t, int64(500), res.Limit)

	res, _ = l.Allow(ctx, "regular")
	assert.Equal(t, int64(10), res.Limit)
}

func TestDynamicLimit_NilFunc(t *testing.T) {
	ctx := context.Background()
	l, _ := NewFixedWindow(10, 60)

	res, _ := l.Allow(ctx, "key")
	assert.Equal(t, int64(10), res.Limit)
}
