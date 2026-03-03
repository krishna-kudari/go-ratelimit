package goratelimit

import (
	"sync"
	"time"
)

// Clock provides the current time. Use in production (nil = time.Now) or inject
// a fake clock in tests to advance time without time.Sleep.
type Clock interface {
	Now() time.Time
}

// FakeClock is a deterministic clock for testing. Advance time with Advance
// instead of sleeping.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock returns a fake clock starting at the Unix epoch.
// Use Advance to move time forward in tests.
func NewFakeClock() *FakeClock {
	return &FakeClock{now: time.Unix(0, 0)}
}

// NewFakeClockAt returns a fake clock starting at the given time.
func NewFakeClockAt(t time.Time) *FakeClock {
	return &FakeClock{now: t}
}

// Now returns the current fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d. Use in tests to simulate elapsed time
// without time.Sleep, e.g. clock.Advance(61 * time.Second) to expire a 60s window.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
