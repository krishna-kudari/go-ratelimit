package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/krishna-kudari/ratelimit/store"
	"github.com/krishna-kudari/ratelimit/store/memory"
)

func TestMemoryStore_GetSetDel(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	// Get non-existent key
	_, err := s.Get(ctx, "missing")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if _, ok := err.(*store.ErrKeyNotFound); !ok {
		t.Fatalf("expected ErrKeyNotFound, got %T", err)
	}

	// Set and Get
	if err := s.Set(ctx, "k1", "v1", 0); err != nil {
		t.Fatal(err)
	}
	val, err := s.Get(ctx, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if val != "v1" {
		t.Errorf("expected v1, got %q", val)
	}

	// Del
	if err := s.Del(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(ctx, "k1")
	if _, ok := err.(*store.ErrKeyNotFound); !ok {
		t.Error("expected ErrKeyNotFound after Del")
	}
}

func TestMemoryStore_SetWithTTL(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	if err := s.Set(ctx, "ttl-key", "val", 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	val, err := s.Get(ctx, "ttl-key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "val" {
		t.Error("expected val before expiry")
	}

	time.Sleep(150 * time.Millisecond)

	_, err = s.Get(ctx, "ttl-key")
	if _, ok := err.(*store.ErrKeyNotFound); !ok {
		t.Error("expected key to be expired")
	}
}

func TestMemoryStore_IncrBy(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	val, err := s.IncrBy(ctx, "counter", 5)
	if err != nil {
		t.Fatal(err)
	}
	if val != 5 {
		t.Errorf("expected 5, got %d", val)
	}

	val, err = s.IncrBy(ctx, "counter", 3)
	if err != nil {
		t.Fatal(err)
	}
	if val != 8 {
		t.Errorf("expected 8, got %d", val)
	}
}

func TestMemoryStore_Expire(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	s.Set(ctx, "exp-key", "val", 0)
	s.Expire(ctx, "exp-key", 100*time.Millisecond)

	ttl, _ := s.TTL(ctx, "exp-key")
	if ttl <= 0 {
		t.Errorf("expected positive TTL, got %v", ttl)
	}

	time.Sleep(150 * time.Millisecond)

	_, err := s.Get(ctx, "exp-key")
	if _, ok := err.(*store.ErrKeyNotFound); !ok {
		t.Error("expected key to be expired after Expire()")
	}
}

func TestMemoryStore_TTL(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	// Non-existent key
	ttl, _ := s.TTL(ctx, "nope")
	if ttl != -2*time.Second {
		t.Errorf("expected -2s for missing key, got %v", ttl)
	}

	// Key with no TTL
	s.Set(ctx, "no-ttl", "val", 0)
	ttl, _ = s.TTL(ctx, "no-ttl")
	if ttl != -1*time.Second {
		t.Errorf("expected -1s for no TTL, got %v", ttl)
	}

	// Key with TTL
	s.Set(ctx, "with-ttl", "val", 10*time.Second)
	ttl, _ = s.TTL(ctx, "with-ttl")
	if ttl < 9*time.Second || ttl > 11*time.Second {
		t.Errorf("expected ~10s TTL, got %v", ttl)
	}
}

func TestMemoryStore_SortedSet(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	// ZAdd and ZCard
	s.ZAdd(ctx, "zset", 1.0, "a")
	s.ZAdd(ctx, "zset", 2.0, "b")
	s.ZAdd(ctx, "zset", 3.0, "c")

	count, _ := s.ZCard(ctx, "zset")
	if count != 3 {
		t.Errorf("expected 3 members, got %d", count)
	}

	// ZRangeWithScores
	entries, _ := s.ZRangeWithScores(ctx, "zset", 0, 0)
	if len(entries) != 1 || entries[0].Member != "a" {
		t.Errorf("expected first entry to be 'a', got %v", entries)
	}

	entries, _ = s.ZRangeWithScores(ctx, "zset", 0, -1)
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// ZRemRangeByScore
	s.ZRemRangeByScore(ctx, "zset", "0", "1.5")
	count, _ = s.ZCard(ctx, "zset")
	if count != 2 {
		t.Errorf("expected 2 members after remove, got %d", count)
	}
}

func TestMemoryStore_Pipeline(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	pipe := s.Pipeline()
	pipe.ZAdd(ctx, "pipe-zset", 1.0, "x")
	pipe.ZAdd(ctx, "pipe-zset", 2.0, "y")
	pipe.Expire(ctx, "pipe-zset", 10*time.Second)

	if err := pipe.Exec(ctx); err != nil {
		t.Fatal(err)
	}

	count, _ := s.ZCard(ctx, "pipe-zset")
	if count != 2 {
		t.Errorf("expected 2 members after pipeline, got %d", count)
	}
}

func TestMemoryStore_EvalReturnsError(t *testing.T) {
	s := memory.New()
	defer s.Close()
	ctx := context.Background()

	_, err := s.Eval(ctx, "return 1", nil)
	if _, ok := err.(*store.ErrScriptNotSupported); !ok {
		t.Errorf("expected ErrScriptNotSupported, got %T: %v", err, err)
	}
}

func TestMemoryStore_InterfaceCompliance(t *testing.T) {
	var _ store.Store = (*memory.Store)(nil)
}
