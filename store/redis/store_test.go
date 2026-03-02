package redis_test

import (
	"context"
	"testing"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/krishna-kudari/ratelimit/store"
	redisstore "github.com/krishna-kudari/ratelimit/store/redis"
)

func newTestStore(t *testing.T) *redisstore.Store {
	t.Helper()
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	return redisstore.New(client)
}

func TestRedisStore_InterfaceCompliance(t *testing.T) {
	var _ store.Store = (*redisstore.Store)(nil)
}

func TestRedisStore_GetSetDel(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	// Get non-existent
	_, err := s.Get(ctx, "test:missing:key")
	require.IsType(t, &store.ErrKeyNotFound{}, err)

	// Set and Get
	err = s.Set(ctx, "test:store:k1", "hello", 0)
	require.NoError(t, err)
	defer func() { _ = s.Del(ctx, "test:store:k1") }()

	val, err := s.Get(ctx, "test:store:k1")
	require.NoError(t, err)
	assert.Equal(t, "hello", val)

	// Del
	err = s.Del(ctx, "test:store:k1")
	require.NoError(t, err)
	_, err = s.Get(ctx, "test:store:k1")
	assert.IsType(t, &store.ErrKeyNotFound{}, err, "expected ErrKeyNotFound after Del")
}

func TestRedisStore_IncrBy(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	key := "test:store:incr"
	defer func() { _ = s.Del(ctx, key) }()

	val, err := s.IncrBy(ctx, key, 5)
	require.NoError(t, err)
	assert.Equal(t, int64(5), val)

	val, err = s.IncrBy(ctx, key, 3)
	require.NoError(t, err)
	assert.Equal(t, int64(8), val)
}

func TestRedisStore_Eval(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	result, err := s.Eval(ctx, "return 42", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(42), result)
}

func TestRedisStore_SortedSet(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	key := "test:store:zset"
	defer func() { _ = s.Del(ctx, key) }()

	_ = s.ZAdd(ctx, key, 1.0, "a")
	_ = s.ZAdd(ctx, key, 2.0, "b")
	_ = s.ZAdd(ctx, key, 3.0, "c")

	count, _ := s.ZCard(ctx, key)
	assert.Equal(t, int64(3), count)

	entries, _ := s.ZRangeWithScores(ctx, key, 0, 0)
	require.Len(t, entries, 1)
	assert.Equal(t, "a", entries[0].Member)

	_ = s.ZRemRangeByScore(ctx, key, "0", "1.5")
	count, _ = s.ZCard(ctx, key)
	assert.Equal(t, int64(2), count)
}

func TestRedisStore_Client(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	assert.NotNil(t, s.Client(), "Client() should not return nil")
}
