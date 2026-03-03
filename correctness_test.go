package goratelimit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCorrectness_ExactlyNRequestsAllowed(t *testing.T) {
	testCorrectnessExactlyN(t)
}

func testCorrectnessExactlyN(t *testing.T) {
	const (
		limit      = 100
		goroutines = 500 // 5x the limit, to ensure pressure
	)

	limiter, err := NewInMemory(PerHour(limit))
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	ctx := context.Background()

	var (
		allowed atomic.Int64
		denied  atomic.Int64
		wg      sync.WaitGroup
	)

	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, _ := limiter.Allow(ctx, "test-key")
			if result.Allowed {
				allowed.Add(1)
			} else {
				denied.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	if allowed.Load() != limit {
		t.Errorf("expected exactly %d allowed, got %d allowed and %d denied",
			limit, allowed.Load(), denied.Load())
	}
}

func TestCorrectness_NeverFlaps(t *testing.T) {
	for i := 0; i < 100; i++ {
		t.Run(fmt.Sprintf("run_%d", i), testCorrectnessExactlyN)
	}
}
