package redis_test

import (
	"context"
	"testing"

	goredis "github.com/redis/go-redis/v9"

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
	if _, ok := err.(*store.ErrKeyNotFound); !ok {
		t.Fatalf("expected ErrKeyNotFound, got %T: %v", err, err)
	}

	// Set and Get
	if err := s.Set(ctx, "test:store:k1", "hello", 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Del(ctx, "test:store:k1") }()

	val, err := s.Get(ctx, "test:store:k1")
	if err != nil {
		t.Fatal(err)
	}
	if val != "hello" {
		t.Errorf("expected hello, got %q", val)
	}

	// Del
	if err := s.Del(ctx, "test:store:k1"); err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(ctx, "test:store:k1")
	if _, ok := err.(*store.ErrKeyNotFound); !ok {
		t.Error("expected ErrKeyNotFound after Del")
	}
}

func TestRedisStore_IncrBy(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	key := "test:store:incr"
	defer func() { _ = s.Del(ctx, key) }()

	val, err := s.IncrBy(ctx, key, 5)
	if err != nil {
		t.Fatal(err)
	}
	if val != 5 {
		t.Errorf("expected 5, got %d", val)
	}

	val, err = s.IncrBy(ctx, key, 3)
	if err != nil {
		t.Fatal(err)
	}
	if val != 8 {
		t.Errorf("expected 8, got %d", val)
	}
}

func TestRedisStore_Eval(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	ctx := context.Background()

	result, err := s.Eval(ctx, "return 42", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.(int64) != 42 {
		t.Errorf("expected 42, got %v", result)
	}
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
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}

	entries, _ := s.ZRangeWithScores(ctx, key, 0, 0)
	if len(entries) != 1 || entries[0].Member != "a" {
		t.Errorf("expected first entry 'a', got %v", entries)
	}

	_ = s.ZRemRangeByScore(ctx, key, "0", "1.5")
	count, _ = s.ZCard(ctx, key)
	if count != 2 {
		t.Errorf("expected 2 after remove, got %d", count)
	}
}

func TestRedisStore_Client(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	if s.Client() == nil {
		t.Error("Client() should not return nil")
	}
}
