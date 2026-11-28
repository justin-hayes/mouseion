package main

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedDueReviewsIfRequested(t *testing.T) {
	now := time.Now()
	t.Run("seeds the number of reviews the browser spec asks for", func(t *testing.T) {
		t.Setenv("MOUSEION_FIXTURE_STUDY_DUE", "3")
		store := fixtures.NewStore()
		seedDueReviewsIfRequested(store)
		due, err := store.CountStudyReviewsDue(context.Background(), fixtures.OwnerID, "de", now)
		require.NoError(t, err)
		assert.Equal(t, 3, due)
	})
	t.Run("seeds nothing otherwise", func(t *testing.T) {
		t.Setenv("MOUSEION_FIXTURE_STUDY_DUE", "")
		store := fixtures.NewStore()
		seedDueReviewsIfRequested(store)
		due, err := store.CountStudyReviewsDue(context.Background(), fixtures.OwnerID, "de", now)
		require.NoError(t, err)
		assert.Zero(t, due)
	})
}
