//go:build integration

package knownvocab

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiverImportLifecycleResultsRetrySafetyAndOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "known vocabulary store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, store.Pool()))
	alice, err := store.CreateUser(ctx, "known-job-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "known-job-bob", false)
	require.NoError(t, err)
	_, err = store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "German library book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	workers := river.NewWorkers()
	AddWorker(workers, store.Pool())
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "known vocabulary client", client.Stop)
	service := NewJobService(store.Pool(), client)
	handle, err := service.Submit(ctx, alice.ID, "de", "Daß\nHaus\nbad\tNOPE\n")
	require.NoError(t, err)
	_, err = service.Get(ctx, bob.ID, handle.ID)
	assert.ErrorIs(t, err, ErrJobNotFound, "cross-owner status") //nolint:testifylint // Independent owner-isolation check; the owner-scoped job is polled next.
	status := waitKnownVocabJob(t, ctx, service, alice.ID, handle.ID)
	assert.Equal(t, rivertype.JobStateCompleted, status.State)
	assert.Equal(t, 2, status.Imported)
	assert.Zero(t, status.AlreadyKnown)
	require.Len(t, status.Rejected, 1)
	assert.Equal(t, 3, status.Processed)
	assert.Equal(t, 3, status.Total)
	duplicate, err := service.Submit(ctx, alice.ID, "de", "Daß\nHaus\n")
	require.NoError(t, err)
	status = waitKnownVocabJob(t, ctx, service, alice.ID, duplicate.ID)
	assert.Zero(t, status.Imported)
	assert.Equal(t, 2, status.AlreadyKnown)
	var knownRows int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&knownRows))
	assert.Equal(t, 2, knownRows)
	_, err = store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Italian library book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "it"})
	require.NoError(t, err)
	italian, err := service.Submit(ctx, alice.ID, "it", "casa\n")
	require.NoError(t, err)
	status = waitKnownVocabJob(t, ctx, service, alice.ID, italian.ID)
	assert.Equal(t, rivertype.JobStateCompleted, status.State)
	assert.Equal(t, 1, status.Imported)
	assert.Len(t, status.Rejected, 0)
	// A forged retry without its owner-scoped history handle is rejected before writes.
	forged := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 999999}, Args: JobArgs{OwnerID: alice.ID, Language: "de", FileContents: "neu"}}
	err = (&Worker{Pool: store.Pool()}).Work(ctx, forged)
	assert.ErrorIs(t, err, ErrJobNotFound, "forged work") //nolint:testifylint // Independent forged-job check; the following query verifies no writes.
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&knownRows))
	assert.Equal(t, 3, knownRows)
}

func waitKnownVocabJob(t *testing.T, ctx context.Context, service *JobService, owner string, id int64) Status {
	t.Helper()
	var status Status
	testutil.Eventually(t, testutil.DefaultWait, fmt.Sprintf("known vocabulary job %d", id), func() (bool, string) {
		current, err := service.Get(ctx, owner, id)
		require.NoError(t, err)
		status = current
		done := current.State == rivertype.JobStateCompleted || current.State == rivertype.JobStateDiscarded
		return done, fmt.Sprintf("state %s", current.State)
	})
	return status
}
