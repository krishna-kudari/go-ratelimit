package goratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFakeClock_Advance(t *testing.T) {
	clock := NewFakeClock()
	start := clock.Now()
	assert.True(t, start.Equal(time.Unix(0, 0)), "starts at epoch")

	clock.Advance(61 * time.Second)
	now := clock.Now()
	assert.Equal(t, int64(61), now.Unix(), "advanced 61 seconds")
}

func TestFixedWindow_WithClock_NoSleep(t *testing.T) {
	clock := NewFakeClock()
	limiter, err := NewFixedWindow(2, 60, WithClock(clock))
	require.NoError(t, err)

	ctx := context.Background()
	key := "user"

	// Use up limit
	r1, err := limiter.Allow(ctx, key)
	require.NoError(t, err)
	assert.True(t, r1.Allowed, "first allowed")
	r2, err := limiter.Allow(ctx, key)
	require.NoError(t, err)
	assert.True(t, r2.Allowed, "second allowed")

	// Denied
	r3, err := limiter.Allow(ctx, key)
	require.NoError(t, err)
	assert.False(t, r3.Allowed, "third denied")

	// Advance past window — no time.Sleep
	clock.Advance(61 * time.Second)

	// Allowed again
	r4, err := limiter.Allow(ctx, key)
	require.NoError(t, err)
	assert.True(t, r4.Allowed, "after advance: allowed")
	assert.Equal(t, int64(1), r4.Remaining)
}

func TestNewInMemory_WithClock(t *testing.T) {
	clock := NewFakeClock()
	limiter, err := NewInMemory(PerMinute(3), WithClock(clock))
	require.NoError(t, err)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		r, err := limiter.Allow(ctx, "k")
		require.NoError(t, err)
		assert.True(t, r.Allowed)
	}
	r, err := limiter.Allow(ctx, "k")
	require.NoError(t, err)
	assert.False(t, r.Allowed)

	clock.Advance(61 * time.Second)
	r, err = limiter.Allow(ctx, "k")
	require.NoError(t, err)
	assert.True(t, r.Allowed)
}
