package testutil_test

import (
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
)

func TestManualClockAdvancesAndIsSharedByItsFunction(t *testing.T) {
	start := time.Date(2026, 3, 1, 4, 0, 0, 0, time.UTC)
	clock := testutil.NewManualClock(start)
	now := clock.Func()

	assert.Equal(t, start, now())
	clock.Advance(90 * time.Minute)
	assert.Equal(t, start.Add(90*time.Minute), now())
	clock.Set(start.AddDate(0, 0, 3))
	assert.Equal(t, start.AddDate(0, 0, 3), clock.Now())
}

func TestManualClockNormalisesToUTC(t *testing.T) {
	berlin := time.FixedZone("CET", 3600)
	clock := testutil.NewManualClock(time.Date(2026, 3, 1, 5, 0, 0, 0, berlin))
	assert.Equal(t, time.UTC, clock.Now().Location())
	assert.Equal(t, 4, clock.Now().Hour())
}
