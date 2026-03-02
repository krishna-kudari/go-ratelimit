package goratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_NoAlgorithm(t *testing.T) {
	_, err := NewBuilder().Build()
	require.Error(t, err, "expected error when no algorithm selected")
}

func TestBuilder_FixedWindow(t *testing.T) {
	l, err := NewBuilder().
		FixedWindow(10, 60*time.Second).
		Build()
	require.NoError(t, err)
	res, err := l.Allow(context.Background(), "k")
	require.NoError(t, err)
	require.True(t, res.Allowed)
}

func TestBuilder_SlidingWindow(t *testing.T) {
	l, err := NewBuilder().
		SlidingWindow(5, 30*time.Second).
		Build()
	require.NoError(t, err)
	res, _ := l.Allow(context.Background(), "k")
	require.True(t, res.Allowed)
	assert.Equal(t, int64(5), res.Limit)
}

func TestBuilder_SlidingWindowCounter(t *testing.T) {
	l, err := NewBuilder().
		SlidingWindowCounter(100, time.Minute).
		Build()
	require.NoError(t, err)
	res, _ := l.Allow(context.Background(), "k")
	require.True(t, res.Allowed)
	assert.Equal(t, int64(100), res.Limit)
}

func TestBuilder_TokenBucket(t *testing.T) {
	l, err := NewBuilder().
		TokenBucket(20, 5).
		Build()
	require.NoError(t, err)
	res, _ := l.Allow(context.Background(), "k")
	require.True(t, res.Allowed)
	assert.Equal(t, int64(20), res.Limit)
}

func TestBuilder_LeakyBucket_Policing(t *testing.T) {
	l, err := NewBuilder().
		LeakyBucket(10, 2, Policing).
		Build()
	require.NoError(t, err)
	res, _ := l.Allow(context.Background(), "k")
	require.True(t, res.Allowed)
	assert.Equal(t, int64(10), res.Limit)
}

func TestBuilder_LeakyBucket_Shaping(t *testing.T) {
	l, err := NewBuilder().
		LeakyBucket(10, 2, Shaping).
		Build()
	require.NoError(t, err)
	res, _ := l.Allow(context.Background(), "k")
	require.True(t, res.Allowed)
}

func TestBuilder_GCRA(t *testing.T) {
	l, err := NewBuilder().
		GCRA(10, 5).
		Build()
	require.NoError(t, err)
	res, _ := l.Allow(context.Background(), "k")
	require.True(t, res.Allowed)
	assert.Equal(t, int64(5), res.Limit)
}

func TestBuilder_InvalidParams(t *testing.T) {
	tests := []struct {
		name string
		fn   func() (Limiter, error)
	}{
		{"FixedWindow zero", func() (Limiter, error) {
			return NewBuilder().FixedWindow(0, time.Second).Build()
		}},
		{"SlidingWindow negative", func() (Limiter, error) {
			return NewBuilder().SlidingWindow(-1, time.Second).Build()
		}},
		{"TokenBucket zero", func() (Limiter, error) {
			return NewBuilder().TokenBucket(0, 10).Build()
		}},
		{"LeakyBucket zero", func() (Limiter, error) {
			return NewBuilder().LeakyBucket(0, 0, Policing).Build()
		}},
		{"GCRA zero", func() (Limiter, error) {
			return NewBuilder().GCRA(0, 5).Build()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.fn()
			assert.Error(t, err, "expected error for invalid params")
		})
	}
}

func TestBuilder_OptionChaining(t *testing.T) {
	l, err := NewBuilder().
		FixedWindow(50, 30*time.Second).
		KeyPrefix("myapp").
		HashTag().
		FailOpen(false).
		Build()
	require.NoError(t, err)
	res, _ := l.Allow(context.Background(), "k")
	require.True(t, res.Allowed)
	assert.Equal(t, int64(50), res.Limit)
}

func TestBuilder_AlgorithmOverride(t *testing.T) {
	l, err := NewBuilder().
		FixedWindow(10, time.Second).
		TokenBucket(20, 5).
		Build()
	require.NoError(t, err)
	res, _ := l.Allow(context.Background(), "k")
	assert.Equal(t, int64(20), res.Limit)
}
