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

func TestRecoveryWorkerEnqueuesEachStaleDeckOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	store, err := persistence.Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()
	owner, err := store.CreateUser(ctx, "recovery-rerender-owner", false)
	require.NoError(t, err)

	for _, name := range []string{"stale-one", "stale-two"} {
		createCompletedStaleDeck(t, ctx, store, owner.ID, name)
	}
	workers := river.NewWorkers()
	AddRerenderWorker(workers, nil)
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	worker := &RecoveryWorker{Store: store, Client: client, Interval: time.Hour, Limit: 100}

	err = worker.Work(ctx, nil)
	var snooze *river.JobSnoozeError
	require.ErrorAs(t, err, &snooze)
	assert.Equal(t, time.Hour, snooze.Duration)
	err = worker.Work(ctx, nil)
	require.ErrorAs(t, err, &snooze)

	var jobs int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND state IN ('available','pending','running','retryable','scheduled')`, (RerenderJobArgs{}).Kind()).Scan(&jobs))
	assert.Equal(t, 2, jobs)
}

func createCompletedStaleDeck(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, name string) domain.DeckPreparation {
	t.Helper()
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{
		OwnerID: owner, Language: "de", SourceIdentifier: name, Title: name,
		MediaType: "text/plain", ContentHash: name + "-hash", Content: []byte(name), FullText: name,
	})
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner, SourceMaterialID: source.ID, Filename: name + ".apkg", DeckName: cardexport.DeckName("und", name), ContentHash: source.ContentHash,
	})
	require.NoError(t, err)
	deck, err := testutil.FreezePresentationDeck(ctx, owner, name, nil, testutil.PresentationProvider{})
	require.NoError(t, err)
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	result, err := store.FreezePreparedDeckRunTx(ctx, tx, persistence.FreezePreparedDeckRunParams{
		OwnerID: owner, PreparationID: preparation.ID, Projection: deck.StorageProjection(),
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	claimToken := "00000000-0000-0000-0000-000000000001"
	_, err = store.ClaimPreparedDeckFinalization(ctx, owner, preparation.ID, result.Run.ID, 0, claimToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	_, err = store.CompletePreparedDeckRun(ctx, owner, preparation.ID, result.Run.ID, claimToken, cardexport.Artifact{APKG: []byte(name), Filename: preparation.Filename, DeckName: preparation.DeckName})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET presentation_version=0 WHERE owner_id=$1 AND id=$2`, owner, preparation.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET presentation_version=0 WHERE owner_id=$1 AND preparation_id=$2`, owner, preparation.ID)
	require.NoError(t, err)
	return preparation
}

func integrationDatabase(t *testing.T, ctx context.Context) string {
	t.Helper()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	return url
}
