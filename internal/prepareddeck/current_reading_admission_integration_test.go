//go:build integration

package prepareddeck

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCurrentReadingAdmissionPrecedence pins the refusal order against Postgres.
// The fixtures adapter has a table with the same case names; keep them in step.
func TestCurrentReadingAdmissionPrecedence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "admission-precedence-owner", false)
	require.NoError(t, err)
	_, book, _, reading := seedAnalyzedReading(t, ctx, store, owner, "admission-precedence-book")
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), client, nil, BatchConfig{}, PreparedDeckConfig{}, false)
	service := NewService(store, client)

	t.Run("stale expected snapshot is refused before anything is queued", func(t *testing.T) {
		_, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, "stale-snapshot")
		require.ErrorIs(t, err, domain.ErrDeckPreparationStale)
	})

	t.Run("invalid transition: reprepare of a queued preparation", func(t *testing.T) {
		handle, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, reading.SnapshotID)
		require.NoError(t, err)
		assert.Equal(t, domain.DeckPreparationQueued, handle.Preparation.State)
		_, err = service.Reprepare(ctx, owner.ID, handle.Preparation.ID, reading.SnapshotID)
		require.ErrorIs(t, err, domain.ErrDeckPreparationInvalidTransition)
	})

	t.Run("not the current reading after the reading ends", func(t *testing.T) {
		require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, reading.SnapshotID))
		_, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, reading.SnapshotID)
		require.ErrorIs(t, err, domain.ErrDeckPreparationNotCurrentReading)
	})
}
