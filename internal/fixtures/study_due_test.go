package fixtures_test

import (
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeededDueReviewsAreCounted(t *testing.T) {
	store := fixtures.NewStore()
	now := time.Date(2026, time.November, 26, 9, 0, 0, 0, time.UTC)
	due, err := store.CountStudyReviewsDue(t.Context(), fixtures.OwnerID, "de", now)
	require.NoError(t, err)
	assert.Zero(t, due)

	store.SeedDueStudyReviews(fixtures.OwnerID, 3, now)
	due, err = store.CountStudyReviewsDue(t.Context(), fixtures.OwnerID, "de", now)
	require.NoError(t, err)
	assert.Equal(t, 3, due)
	due, err = store.CountStudyReviewsDue(t.Context(), fixtures.OwnerID, "it", now)
	require.NoError(t, err)
	assert.Zero(t, due, "the count is the language's")
}
