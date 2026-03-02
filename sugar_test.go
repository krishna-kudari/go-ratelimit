package goratelimit

import (
	"context"
	"testing"
)

func TestRateHelpers(t *testing.T) {
	tests := []struct {
		name          string
		rate          Rate
		wantRequests  int64
		wantWindowSec int64
	}{
		{"PerSecond", PerSecond(50), 50, 1},
		{"PerMinute", PerMinute(100), 100, 60},
		{"PerHour", PerHour(1000), 1000, 3600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.rate.maxRequests != tt.wantRequests {
				t.Errorf("maxRequests = %d, want %d", tt.rate.maxRequests, tt.wantRequests)
			}
			if tt.rate.windowSeconds != tt.wantWindowSec {
				t.Errorf("windowSeconds = %d, want %d", tt.rate.windowSeconds, tt.wantWindowSec)
			}
		})
	}
}

func TestNew_InMemory(t *testing.T) {
	limiter, err := New("", PerMinute(100))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := limiter.Allow(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if !result.Allowed {
		t.Error("expected request to be allowed")
	}
	if result.Remaining != 99 {
		t.Errorf("remaining = %d, want 99", result.Remaining)
	}
	if result.Limit != 100 {
		t.Errorf("limit = %d, want 100", result.Limit)
	}
}

func TestNew_InvalidRate(t *testing.T) {
	tests := []struct {
		name string
		rate Rate
	}{
		{"zero requests", Rate{maxRequests: 0, windowSeconds: 60}},
		{"negative requests", Rate{maxRequests: -1, windowSeconds: 60}},
		{"zero window", Rate{maxRequests: 100, windowSeconds: 0}},
		{"negative window", Rate{maxRequests: 100, windowSeconds: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New("", tt.rate)
			if err == nil {
				t.Error("expected error for invalid rate")
			}
		})
	}
}

func TestNew_InvalidRedisURL(t *testing.T) {
	_, err := New("not-a-url", PerMinute(100))
	if err == nil {
		t.Error("expected error for invalid redis URL")
	}
}

func TestNew_WithOptions(t *testing.T) {
	limiter, err := New("", PerSecond(10),
		WithKeyPrefix("myapp"),
		WithFailOpen(false),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := limiter.Allow(context.Background(), "key")
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if !result.Allowed {
		t.Error("expected request to be allowed")
	}
}

func TestNewInMemory(t *testing.T) {
	limiter, err := NewInMemory(PerHour(500))
	if err != nil {
		t.Fatalf("NewInMemory() error = %v", err)
	}

	result, err := limiter.Allow(context.Background(), "user:1")
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if result.Limit != 500 {
		t.Errorf("limit = %d, want 500", result.Limit)
	}
}
