package goratelimit

import (
	"context"
	"testing"
	"time"
)

func TestBuilder_NoAlgorithm(t *testing.T) {
	_, err := NewBuilder().Build()
	if err == nil {
		t.Fatal("expected error when no algorithm selected")
	}
}

func TestBuilder_FixedWindow(t *testing.T) {
	l, err := NewBuilder().
		FixedWindow(10, 60*time.Second).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	res, err := l.Allow(context.Background(), "k")
	if err != nil || !res.Allowed {
		t.Fatalf("expected allowed, got %+v, err=%v", res, err)
	}
}

func TestBuilder_SlidingWindow(t *testing.T) {
	l, err := NewBuilder().
		SlidingWindow(5, 30*time.Second).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	res, _ := l.Allow(context.Background(), "k")
	if !res.Allowed || res.Limit != 5 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestBuilder_SlidingWindowCounter(t *testing.T) {
	l, err := NewBuilder().
		SlidingWindowCounter(100, time.Minute).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	res, _ := l.Allow(context.Background(), "k")
	if !res.Allowed || res.Limit != 100 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestBuilder_TokenBucket(t *testing.T) {
	l, err := NewBuilder().
		TokenBucket(20, 5).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	res, _ := l.Allow(context.Background(), "k")
	if !res.Allowed || res.Limit != 20 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestBuilder_LeakyBucket_Policing(t *testing.T) {
	l, err := NewBuilder().
		LeakyBucket(10, 2, Policing).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	res, _ := l.Allow(context.Background(), "k")
	if !res.Allowed || res.Limit != 10 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestBuilder_LeakyBucket_Shaping(t *testing.T) {
	l, err := NewBuilder().
		LeakyBucket(10, 2, Shaping).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	res, _ := l.Allow(context.Background(), "k")
	if !res.Allowed {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestBuilder_GCRA(t *testing.T) {
	l, err := NewBuilder().
		GCRA(10, 5).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	res, _ := l.Allow(context.Background(), "k")
	if !res.Allowed || res.Limit != 5 {
		t.Fatalf("unexpected result: %+v", res)
	}
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
			if err == nil {
				t.Error("expected error for invalid params")
			}
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
	if err != nil {
		t.Fatal(err)
	}
	res, _ := l.Allow(context.Background(), "k")
	if !res.Allowed || res.Limit != 50 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestBuilder_AlgorithmOverride(t *testing.T) {
	l, err := NewBuilder().
		FixedWindow(10, time.Second).
		TokenBucket(20, 5).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	res, _ := l.Allow(context.Background(), "k")
	if res.Limit != 20 {
		t.Fatalf("expected TokenBucket limit 20, got %d", res.Limit)
	}
}
