package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/krishna-kudari/ratelimit/store"
	"github.com/krishna-kudari/ratelimit/store/memory"
)

func TestMemoryStore_GetSetDel(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	// Get non-existent key
	_, err := s.Get(ctx, "missing")
	require.Error(t, err, "expected error for missing key")
	require.IsType(t, &store.ErrKeyNotFound{}, err)

	// Set and Get
	err = s.Set(ctx, "k1", "v1", 0)
	require.NoError(t, err)
	val, err := s.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, "v1", val)

	// Del
	err = s.Del(ctx, "k1")
	require.NoError(t, err)
	_, err = s.Get(ctx, "k1")
	assert.IsType(t, &store.ErrKeyNotFound{}, err, "expected ErrKeyNotFound after Del")
}

func TestMemoryStore_SetWithTTL(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	err := s.Set(ctx, "ttl-key", "val", 100*time.Millisecond)
	require.NoError(t, err)

	val, err := s.Get(ctx, "ttl-key")
	require.NoError(t, err)
	assert.Equal(t, "val", val, "expected val before expiry")

	time.Sleep(150 * time.Millisecond)

	_, err = s.Get(ctx, "ttl-key")
	assert.IsType(t, &store.ErrKeyNotFound{}, err, "expected key to be expired")
}

func TestMemoryStore_IncrBy(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	val, err := s.IncrBy(ctx, "counter", 5)
	require.NoError(t, err)
	assert.Equal(t, int64(5), val)

	val, err = s.IncrBy(ctx, "counter", 3)
	require.NoError(t, err)
	assert.Equal(t, int64(8), val)
}

func TestMemoryStore_Expire(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	_ = s.Set(ctx, "exp-key", "val", 0)
	_ = s.Expire(ctx, "exp-key", 100*time.Millisecond)

	ttl, _ := s.TTL(ctx, "exp-key")
	assert.Greater(t, ttl, time.Duration(0), "expected positive TTL")

	time.Sleep(150 * time.Millisecond)

	_, err := s.Get(ctx, "exp-key")
	assert.IsType(t, &store.ErrKeyNotFound{}, err, "expected key to be expired after Expire()")
}

func TestMemoryStore_TTL(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	// Non-existent key
	ttl, _ := s.TTL(ctx, "nope")
	assert.Equal(t, -2*time.Second, ttl, "expected -2s for missing key")

	// Key with no TTL
	_ = s.Set(ctx, "no-ttl", "val", 0)
	ttl, _ = s.TTL(ctx, "no-ttl")
	assert.Equal(t, -1*time.Second, ttl, "expected -1s for no TTL")

	// Key with TTL
	_ = s.Set(ctx, "with-ttl", "val", 10*time.Second)
	ttl, _ = s.TTL(ctx, "with-ttl")
	assert.True(t, ttl >= 9*time.Second && ttl <= 11*time.Second, "expected ~10s TTL, got %v", ttl)
}

func TestMemoryStore_SortedSet(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	// ZAdd and ZCard
	_ = s.ZAdd(ctx, "zset", 1.0, "a")
	_ = s.ZAdd(ctx, "zset", 2.0, "b")
	_ = s.ZAdd(ctx, "zset", 3.0, "c")

	count, _ := s.ZCard(ctx, "zset")
	assert.Equal(t, int64(3), count)

	// ZRangeWithScores
	entries, _ := s.ZRangeWithScores(ctx, "zset", 0, 0)
	require.Len(t, entries, 1)
	assert.Equal(t, "a", entries[0].Member)

	entries, _ = s.ZRangeWithScores(ctx, "zset", 0, -1)
	assert.Len(t, entries, 3)

	// ZRemRangeByScore
	_ = s.ZRemRangeByScore(ctx, "zset", "0", "1.5")
	count, _ = s.ZCard(ctx, "zset")
	assert.Equal(t, int64(2), count)
}

func TestMemoryStore_Pipeline(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	pipe := s.Pipeline()
	pipe.ZAdd(ctx, "pipe-zset", 1.0, "x")
	pipe.ZAdd(ctx, "pipe-zset", 2.0, "y")
	pipe.Expire(ctx, "pipe-zset", 10*time.Second)

	err := pipe.Exec(ctx)
	require.NoError(t, err)

	count, _ := s.ZCard(ctx, "pipe-zset")
	assert.Equal(t, int64(2), count)
}

func TestMemoryStore_EvalReturnsError(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	_, err := s.Eval(ctx, "return 1", nil)
	assert.IsType(t, &store.ErrScriptNotSupported{}, err)
}

func TestMemoryStore_InterfaceCompliance(t *testing.T) {
	var _ store.Store = (*memory.Store)(nil)
}
