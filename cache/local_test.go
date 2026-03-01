package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goratelimit "github.com/krishna-kudari/ratelimit"
)

// mockLimiter records calls and returns configurable results.
type mockLimiter struct {
	mu       sync.Mutex
	calls    int
	allowN   func(ctx context.Context, key string, n int) (*goratelimit.Result, error)
	resetErr error
	resets   int
}

func (m *mockLimiter) Allow(ctx context.Context, key string) (*goratelimit.Result, error) {
	return m.AllowN(ctx, key, 1)
}

func (m *mockLimiter) AllowN(ctx context.Context, key string, n int) (*goratelimit.Result, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return m.allowN(ctx, key, n)
}

func (m *mockLimiter) Reset(ctx context.Context, key string) error {
	m.mu.Lock()
	m.resets++
	m.mu.Unlock()
	return m.resetErr
}

func (m *mockLimiter) getCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestLocalCache_CacheHit(t *testing.T) {
	mock := &mockLimiter{
		allowN: func(_ context.Context, _ string, _ int) (*goratelimit.Result, error) {
			return &goratelimit.Result{
				Allowed:   true,
				Remaining: 10,
				Limit:     10,
				ResetAt:   time.Now().Add(time.Minute),
			}, nil
		},
	}

	lc := New(mock, WithTTL(500*time.Millisecond))
	defer lc.Close()

	ctx := context.Background()

	// First call — cache miss, hits backend
	r, err := lc.Allow(ctx, "k1")
	if err != nil || !r.Allowed {
		t.Fatalf("expected allowed, got err=%v allowed=%v", err, r.Allowed)
	}
	if mock.getCalls() != 1 {
		t.Fatalf("expected 1 backend call, got %d", mock.getCalls())
	}

	// Next calls should be served from cache
	for i := 0; i < 5; i++ {
		r, err = lc.Allow(ctx, "k1")
		if err != nil || !r.Allowed {
			t.Fatalf("call %d: expected allowed, got err=%v allowed=%v", i, err, r.Allowed)
		}
	}
	if mock.getCalls() != 1 {
		t.Fatalf("expected still 1 backend call after cache hits, got %d", mock.getCalls())
	}
}

func TestLocalCache_RemainingDecreases(t *testing.T) {
	mock := &mockLimiter{
		allowN: func(_ context.Context, _ string, _ int) (*goratelimit.Result, error) {
			return &goratelimit.Result{
				Allowed:   true,
				Remaining: 5,
				Limit:     5,
				ResetAt:   time.Now().Add(time.Minute),
			}, nil
		},
	}

	lc := New(mock, WithTTL(time.Second))
	defer lc.Close()

	ctx := context.Background()

	// First call is cache miss → backend already counted this request,
	// returns Remaining=5 as-is. localUsed starts at 0.
	r, _ := lc.Allow(ctx, "k1")
	if r.Remaining != 5 {
		t.Fatalf("expected remaining=5 from backend, got %d", r.Remaining)
	}

	// Second call: cache hit → localUsed=1, remaining = 5-1 = 4
	r, _ = lc.Allow(ctx, "k1")
	if r.Remaining != 4 {
		t.Fatalf("expected remaining=4, got %d", r.Remaining)
	}

	// Third call: cache hit → localUsed=2, remaining = 5-2 = 3
	r, _ = lc.Allow(ctx, "k1")
	if r.Remaining != 3 {
		t.Fatalf("expected remaining=3, got %d", r.Remaining)
	}
}

func TestLocalCache_ExhaustedLocalQuota_SyncsBackend(t *testing.T) {
	var callCount atomic.Int64
	mock := &mockLimiter{
		allowN: func(_ context.Context, _ string, _ int) (*goratelimit.Result, error) {
			callCount.Add(1)
			return &goratelimit.Result{
				Allowed:   true,
				Remaining: 2,
				Limit:     3,
				ResetAt:   time.Now().Add(time.Minute),
			}, nil
		},
	}

	lc := New(mock, WithTTL(5*time.Second))
	defer lc.Close()

	ctx := context.Background()

	// Call 1: cache miss → backend (call 1), returns remaining=2, localUsed=0
	lc.Allow(ctx, "k1")
	if callCount.Load() != 1 {
		t.Fatalf("expected 1 backend call, got %d", callCount.Load())
	}

	// Call 2: cache hit → remaining=2, localUsed becomes 1, 2-0>=1 true → serves locally
	lc.Allow(ctx, "k1")
	if callCount.Load() != 1 {
		t.Fatalf("expected still 1 backend call, got %d", callCount.Load())
	}

	// Call 3: cache hit → remaining=2, localUsed=1, 2-1>=1 true → serves locally
	lc.Allow(ctx, "k1")
	if callCount.Load() != 1 {
		t.Fatalf("expected still 1 backend call after call 3, got %d", callCount.Load())
	}

	// Call 4: cache hit → remaining=2, localUsed=2, 2-2=0 < 1 → exhausted, syncs backend (call 2)
	lc.Allow(ctx, "k1")
	if callCount.Load() != 2 {
		t.Fatalf("expected 2 backend calls after local exhaustion, got %d", callCount.Load())
	}
}

func TestLocalCache_DeniedCached(t *testing.T) {
	mock := &mockLimiter{
		allowN: func(_ context.Context, _ string, _ int) (*goratelimit.Result, error) {
			return &goratelimit.Result{
				Allowed:    false,
				Remaining:  0,
				Limit:      10,
				RetryAfter: time.Second,
				ResetAt:    time.Now().Add(time.Second),
			}, nil
		},
	}

	lc := New(mock, WithTTL(time.Second))
	defer lc.Close()

	ctx := context.Background()

	// First call — backend returns denial
	r, _ := lc.Allow(ctx, "k1")
	if r.Allowed {
		t.Fatal("expected denied")
	}

	// Subsequent calls served from cache (denial cached)
	for i := 0; i < 5; i++ {
		r, _ = lc.Allow(ctx, "k1")
		if r.Allowed {
			t.Fatal("expected cached denial")
		}
	}
	if mock.getCalls() != 1 {
		t.Fatalf("expected 1 backend call for cached denial, got %d", mock.getCalls())
	}
}

func TestLocalCache_TTLExpiry(t *testing.T) {
	mock := &mockLimiter{
		allowN: func(_ context.Context, _ string, _ int) (*goratelimit.Result, error) {
			return &goratelimit.Result{
				Allowed:   true,
				Remaining: 100,
				Limit:     100,
				ResetAt:   time.Now().Add(time.Minute),
			}, nil
		},
	}

	lc := New(mock, WithTTL(50*time.Millisecond))
	defer lc.Close()

	ctx := context.Background()

	lc.Allow(ctx, "k1")
	if mock.getCalls() != 1 {
		t.Fatal("expected 1 call")
	}

	// Within TTL — should still be cached
	lc.Allow(ctx, "k1")
	if mock.getCalls() != 1 {
		t.Fatal("expected still 1 call within TTL")
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	lc.Allow(ctx, "k1")
	if mock.getCalls() != 2 {
		t.Fatalf("expected 2 calls after TTL expiry, got %d", mock.getCalls())
	}
}

func TestLocalCache_DenialTTL_UsesRetryAfter(t *testing.T) {
	callCount := 0
	mock := &mockLimiter{
		allowN: func(_ context.Context, _ string, _ int) (*goratelimit.Result, error) {
			callCount++
			return &goratelimit.Result{
				Allowed:    false,
				Remaining:  0,
				Limit:      10,
				RetryAfter: 30 * time.Millisecond,
				ResetAt:    time.Now().Add(30 * time.Millisecond),
			}, nil
		},
	}

	// TTL is 5s, but denied result has RetryAfter=30ms → uses the shorter one
	lc := New(mock, WithTTL(5*time.Second))
	defer lc.Close()

	ctx := context.Background()

	lc.Allow(ctx, "k1")
	if callCount != 1 {
		t.Fatal("expected 1 call")
	}

	time.Sleep(40 * time.Millisecond)

	lc.Allow(ctx, "k1")
	if callCount != 2 {
		t.Fatalf("expected 2 calls after retryAfter expiry, got %d", callCount)
	}
}

func TestLocalCache_AllowN(t *testing.T) {
	mock := &mockLimiter{
		allowN: func(_ context.Context, _ string, _ int) (*goratelimit.Result, error) {
			return &goratelimit.Result{
				Allowed:   true,
				Remaining: 10,
				Limit:     10,
				ResetAt:   time.Now().Add(time.Minute),
			}, nil
		},
	}

	lc := New(mock, WithTTL(time.Second))
	defer lc.Close()

	ctx := context.Background()

	// AllowN(5): cache miss → backend (call 1), returns remaining=10
	r, _ := lc.AllowN(ctx, "k1", 5)
	if !r.Allowed || r.Remaining != 10 {
		t.Fatalf("expected allowed with remaining=10 from backend, got allowed=%v remaining=%d", r.Allowed, r.Remaining)
	}

	// AllowN(5): cache hit → localUsed=5, remaining=10-5=5
	r, _ = lc.AllowN(ctx, "k1", 5)
	if !r.Allowed || r.Remaining != 5 {
		t.Fatalf("expected allowed with remaining=5, got allowed=%v remaining=%d", r.Allowed, r.Remaining)
	}

	// AllowN(5): cache hit → localUsed=10, remaining=10-10=0
	r, _ = lc.AllowN(ctx, "k1", 5)
	if !r.Allowed || r.Remaining != 0 {
		t.Fatalf("expected allowed with remaining=0, got allowed=%v remaining=%d", r.Allowed, r.Remaining)
	}

	// AllowN(1): local quota exhausted (10-10=0 < 1) → syncs backend (call 2)
	lc.AllowN(ctx, "k1", 1)
	if mock.getCalls() != 2 {
		t.Fatalf("expected 2 backend calls, got %d", mock.getCalls())
	}
}

func TestLocalCache_Reset(t *testing.T) {
	mock := &mockLimiter{
		allowN: func(_ context.Context, _ string, _ int) (*goratelimit.Result, error) {
			return &goratelimit.Result{
				Allowed:   true,
				Remaining: 10,
				Limit:     10,
				ResetAt:   time.Now().Add(time.Minute),
			}, nil
		},
	}

	lc := New(mock, WithTTL(5*time.Second))
	defer lc.Close()

	ctx := context.Background()

	// Populate cache
	lc.Allow(ctx, "k1")
	if mock.getCalls() != 1 {
		t.Fatal("expected 1 call")
	}

	// Reset evicts from cache
	if err := lc.Reset(ctx, "k1"); err != nil {
		t.Fatal(err)
	}

	// Next call must hit backend (cache was evicted)
	lc.Allow(ctx, "k1")
	if mock.getCalls() != 2 {
		t.Fatalf("expected 2 backend calls after reset, got %d", mock.getCalls())
	}
}

func TestLocalCache_MultipleKeys(t *testing.T) {
	mock := &mockLimiter{
		allowN: func(_ context.Context, key string, _ int) (*goratelimit.Result, error) {
			return &goratelimit.Result{
				Allowed:   true,
				Remaining: 5,
				Limit:     5,
				ResetAt:   time.Now().Add(time.Minute),
			}, nil
		},
	}

	lc := New(mock, WithTTL(time.Second))
	defer lc.Close()

	ctx := context.Background()

	lc.Allow(ctx, "user:1")
	lc.Allow(ctx, "user:2")
	lc.Allow(ctx, "user:3")

	if mock.getCalls() != 3 {
		t.Fatalf("expected 3 backend calls for 3 different keys, got %d", mock.getCalls())
	}

	// Subsequent calls on same keys → cache hits
	lc.Allow(ctx, "user:1")
	lc.Allow(ctx, "user:2")
	lc.Allow(ctx, "user:3")
	if mock.getCalls() != 3 {
		t.Fatalf("expected still 3 backend calls after cache hits, got %d", mock.getCalls())
	}
}

func TestLocalCache_MaxKeys(t *testing.T) {
	mock := &mockLimiter{
		allowN: func(_ context.Context, _ string, _ int) (*goratelimit.Result, error) {
			return &goratelimit.Result{
				Allowed:   true,
				Remaining: 10,
				Limit:     10,
				ResetAt:   time.Now().Add(time.Minute),
			}, nil
		},
	}

	lc := New(mock, WithTTL(5*time.Second), WithMaxKeys(3))
	defer lc.Close()

	ctx := context.Background()

	// Fill to max
	lc.Allow(ctx, "k1")
	time.Sleep(time.Millisecond)
	lc.Allow(ctx, "k2")
	time.Sleep(time.Millisecond)
	lc.Allow(ctx, "k3")

	stats := lc.Stats()
	if stats.Keys != 3 {
		t.Fatalf("expected 3 keys, got %d", stats.Keys)
	}

	// Adding 4th should evict oldest (k1)
	lc.Allow(ctx, "k4")
	stats = lc.Stats()
	if stats.Keys != 3 {
		t.Fatalf("expected 3 keys after eviction, got %d", stats.Keys)
	}
}

func TestLocalCache_ConcurrentAccess(t *testing.T) {
	mock := &mockLimiter{
		allowN: func(_ context.Context, _ string, _ int) (*goratelimit.Result, error) {
			return &goratelimit.Result{
				Allowed:   true,
				Remaining: 1000,
				Limit:     1000,
				ResetAt:   time.Now().Add(time.Minute),
			}, nil
		},
	}

	lc := New(mock, WithTTL(time.Second))
	defer lc.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, err := lc.Allow(ctx, "concurrent-key")
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	// Should have far fewer than 10000 backend calls due to caching
	if mock.getCalls() > 100 {
		t.Fatalf("expected significantly fewer backend calls with caching, got %d", mock.getCalls())
	}
}

func TestLocalCache_Stats(t *testing.T) {
	mock := &mockLimiter{
		allowN: func(_ context.Context, _ string, _ int) (*goratelimit.Result, error) {
			return &goratelimit.Result{
				Allowed:   true,
				Remaining: 10,
				Limit:     10,
				ResetAt:   time.Now().Add(time.Minute),
			}, nil
		},
	}

	lc := New(mock, WithTTL(time.Second))
	defer lc.Close()

	ctx := context.Background()

	stats := lc.Stats()
	if stats.Keys != 0 {
		t.Fatalf("expected 0 keys initially, got %d", stats.Keys)
	}

	lc.Allow(ctx, "k1")
	lc.Allow(ctx, "k2")

	stats = lc.Stats()
	if stats.Keys != 2 {
		t.Fatalf("expected 2 keys, got %d", stats.Keys)
	}
}
