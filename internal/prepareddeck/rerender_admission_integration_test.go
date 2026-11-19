//go:build integration

package prepareddeck

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLearnerRerenderUsesTheSharedFreshnessDecision covers the version
// boundaries the learner's action used to miss: a deck whose presentation is
// current but whose render inputs are behind now queues a rerender, a run input
// already covered by a current preparation still counts as up to date, and an
// up-to-date deck queues none.
func TestLearnerRerenderUsesTheSharedFreshnessDecision(t *testing.T) {
	cases := []struct {
		name                          string
		runRenderInputVersion         int
		preparationRenderInputVersion int
		wantJob                       bool
	}{
		{name: "presentation current and both render inputs behind", runRenderInputVersion: cardexport.RenderInputVersion - 1, preparationRenderInputVersion: cardexport.RenderInputVersion - 1, wantJob: true},
		{name: "run input behind but preparation current", runRenderInputVersion: cardexport.RenderInputVersion - 1, preparationRenderInputVersion: cardexport.RenderInputVersion, wantJob: false},
		{name: "up to date", runRenderInputVersion: cardexport.RenderInputVersion, preparationRenderInputVersion: cardexport.RenderInputVersion, wantJob: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
			store, err := persistence.Open(ctx, url)
			require.NoError(t, err)
			testutil.Cleanup(t, "store", store.Close)
			owner, err := store.CreateUser(ctx, "rerender-admission-"+uuid.NewString(), false)
			require.NoError(t, err)
			source, book, _, reading := seedAnalyzedReading(t, ctx, store, owner, uuid.NewString())
			workers := river.NewWorkers()
			AddRerenderWorker(workers, &DurableRerenderer{Store: store, Renderer: cardexport.NewPresentation(nil)})
			client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
			require.NoError(t, err)
			testutil.StopOnCleanup(t, "River client", client.Stop)
			AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), client, nil, BatchConfig{}, PreparedDeckConfig{}, false)
			service := NewService(store, client)
			prepared, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, reading.SnapshotID)
			require.NoError(t, err)
			preparation := completeReadyPreparedDeck(t, ctx, store, owner.ID, prepared.Preparation, source.Title, tc.runRenderInputVersion, tc.preparationRenderInputVersion)

			handle, err := service.Rerender(ctx, owner.ID, preparation.ID, reading.SnapshotID)
			require.NoError(t, err)
			var jobs int
			require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'preparation_id'=$2 AND state IN ('available','pending','running','retryable','scheduled')`, (RerenderJobArgs{}).Kind(), preparation.ID).Scan(&jobs))
			if tc.wantJob {
				assert.Equal(t, 1, jobs)
				assert.NotZero(t, handle.JobID)
				return
			}
			assert.Zero(t, jobs)
			assert.Zero(t, handle.JobID)
		})
	}
}

// completeReadyPreparedDeck completes the prepared deck's current run with the
// requested render-input versions so the learner's action can be observed at
// each boundary.
func completeReadyPreparedDeck(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner string, preparation domain.DeckPreparation, deckName string, runRenderInputVersion, preparationRenderInputVersion int) domain.DeckPreparation {
	t.Helper()
	deck, err := testutil.FreezePresentationDeck(ctx, owner, deckName, nil, testutil.PresentationProvider{})
	require.NoError(t, err)
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	frozen, err := store.FreezePreparedDeckRunTx(ctx, tx, persistence.FreezePreparedDeckRunParams{
		OwnerID: owner, PreparationID: preparation.ID, Projection: deck.StorageProjection(),
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	claimToken := uuid.NewString()
	_, err = store.ClaimPreparedDeckFinalization(ctx, owner, preparation.ID, frozen.Run.ID, 0, claimToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	ready, err := store.CompletePreparedDeckRun(ctx, owner, preparation.ID, frozen.Run.ID, claimToken, cardexport.Artifact{
		APKG: []byte("artifact"), Filename: preparation.Filename, DeckName: preparation.DeckName,
	})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET render_input_version=$3 WHERE owner_id=$1 AND id=$2`, owner, preparation.ID, preparationRenderInputVersion)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET render_input_version=$4 WHERE owner_id=$1 AND preparation_id=$2 AND id=$3`, owner, preparation.ID, frozen.Run.ID, runRenderInputVersion)
	require.NoError(t, err)
	return ready
}
