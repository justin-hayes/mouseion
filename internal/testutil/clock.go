package testutil

import (
	"sync"
	"time"
)

// ManualClock is a controllable clock for tests. Times are kept in UTC so a
// test does not depend on the host's zone.
type ManualClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewManualClock returns a clock set to start.
func NewManualClock(start time.Time) *ManualClock {
	return &ManualClock{now: start.UTC()}
}

// Now returns the clock's current time.
func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Set moves the clock to t.
func (c *ManualClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t.UTC()
}

// Func returns the clock as a function value for webapp.Services.Clock. The
// function reads the clock's shared state, so Advance and Set are seen by
// every holder.
func (c *ManualClock) Func() func() time.Time { return c.Now }
