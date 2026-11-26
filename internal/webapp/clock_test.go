package webapp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHandlerUsesInjectedClockAndDefaultsToRealClock(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "clock-test-secret-0123456789-abcdefghij")
	current := time.Date(2026, 3, 1, 4, 0, 0, 0, time.UTC)
	injected := New(Services{Clock: func() time.Time { return current }})
	assert.Equal(t, current, injected.now())
	current = current.Add(time.Hour)
	assert.Equal(t, current, injected.now())

	before := time.Now()
	defaulted := New(Services{})
	assert.False(t, defaulted.now().Before(before))
}
