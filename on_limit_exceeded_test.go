package goratelimit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnLimitExceeded_CalledWhenDenied(t *testing.T) {
	ctx := context.Background()
	var gotCtx context.Context
	var gotKey string
	var gotResult *Result
	l, err := NewFixedWindow(2, 60, WithOnLimitExceeded(func(ctx context.Context, key string, result *Result) {
		gotCtx = ctx
		gotKey = key
		gotResult = result
	}))
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		_, _ = l.Allow(ctx, "user")
	}
	res, err := l.Allow(ctx, "user")
	require.NoError(t, err)
	assert.False(t, res.Allowed)
	assert.NotNil(t, gotCtx)
	assert.Equal(t, "user", gotKey)
	require.NotNil(t, gotResult)
	assert.False(t, gotResult.Allowed)
	assert.Equal(t, int64(2), gotResult.Limit)
	assert.Equal(t, int64(0), gotResult.Remaining)
}

func TestOnLimitExceeded_NotCalledWhenAllowed(t *testing.T) {
	ctx := context.Background()
	called := false
	l, err := NewFixedWindow(2, 60, WithOnLimitExceeded(func(context.Context, string, *Result) {
		called = true
	}))
	require.NoError(t, err)
	_, _ = l.Allow(ctx, "user")
	assert.False(t, called)
}

func TestOnLimitExceeded_NotCalledWhenNil(t *testing.T) {
	ctx := context.Background()
	l, err := NewFixedWindow(1, 60)
	require.NoError(t, err)
	_, _ = l.Allow(ctx, "user")
	res, _ := l.Allow(ctx, "user")
	assert.False(t, res.Allowed)
	// no panic, callback was nil
}

func TestOnLimitExceeded_NotCalledWhenDryRun(t *testing.T) {
	ctx := context.Background()
	called := false
	l, err := NewFixedWindow(2, 60,
		WithDryRun(true),
		WithOnLimitExceeded(func(context.Context, string, *Result) {
			called = true
		}),
	)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		_, _ = l.Allow(ctx, "user")
	}
	assert.False(t, called, "OnLimitExceeded should not be called when DryRun is true")
}
