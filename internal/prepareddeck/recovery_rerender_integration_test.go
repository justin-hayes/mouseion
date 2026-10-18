//go:build integration

package prepareddeck

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
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
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "recovery-rerender-owner", false)
	require.NoError(t, err)

	for _, name := range []string{"stale-one", "stale-two"} {
		createCompletedStaleDeck(t, ctx, store, owner.ID, name)
	}
	workers := river.NewWorkers()
	AddRerenderWorker(workers, nil)
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
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

func TestRecoveryWorkerSkipsRerenderedDeckWithOlderFrozenInputs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	store, err := persistence.Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "recovery-rerendered-owner", false)
	require.NoError(t, err)
	preparation := createCompletedStaleDeck(t, ctx, store, owner.ID, "rerendered")
	// The state a rerender leaves behind: the artifact is current, while the
	// run keeps the older version its inputs were frozen at.
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET presentation_version=$3, render_input_version=$4 WHERE owner_id=$1 AND id=$2`, owner.ID, preparation.ID, cardexport.PresentationVersion, cardexport.RenderInputVersion)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET presentation_version=$3, render_input_version=$4 WHERE owner_id=$1 AND preparation_id=$2`, owner.ID, preparation.ID, cardexport.PresentationVersion, cardexport.RenderInputVersion-1)
	require.NoError(t, err)
	workers := river.NewWorkers()
	AddRerenderWorker(workers, nil)
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	worker := &RecoveryWorker{Store: store, Client: client, Interval: time.Hour, Limit: 100}

	err = worker.Work(ctx, nil)
	var snooze *river.JobSnoozeError
	require.ErrorAs(t, err, &snooze)

	var jobs int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1`, (RerenderJobArgs{}).Kind()).Scan(&jobs))
	assert.Zero(t, jobs, "a deck already rerendered at the current versions is not queued again")
}

func TestRecoveryWorkerSnoozesAtDefaultIntervalWhenUnset(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	store, err := persistence.Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	worker := &RecoveryWorker{Store: store, Client: jobLessRiverClient{}}
	err = worker.Work(ctx, nil)

	var snooze *river.JobSnoozeError
	require.ErrorAs(t, err, &snooze)
	assert.Equal(t, DefaultBatchPollInterval, snooze.Duration, "an unset interval falls back to the default poll interval")
}

func TestRecoveryWorkerSurfacesStoreReadFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	store, err := persistence.Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	worker := &RecoveryWorker{Store: store, Client: jobLessRiverClient{}}
	require.NoError(t, store.Close())

	err = worker.Work(ctx, nil)

	require.Error(t, err, "a failed recovery read is returned, never silently snoozed")
}

func TestRecoveryRepairSkipsBatchOutcomeWithoutInsertingTranslationWork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, _ := newBatchIntegrationRun(t, ctx)
	require.Equal(t, domain.PreparedDeckExecutionBatch, run.run.ExecutionMode)
	worker := &RecoveryWorker{Store: run.store, Client: jobLessRiverClient{}}

	err := worker.repair(ctx, domain.PreparedDeckRecoveryWork{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0, Kind: "outcome"})

	require.NoError(t, err, "Batch outcomes are repaired by their durable chunk jobs, not the standard recovery path")
}

func TestRecoveryRepairReconstructsEachDurableJobKind(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client := newBatchIntegrationRun(t, ctx)
	chunks, err := run.store.ListPreparedDeckBatchChunks(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	worker := &RecoveryWorker{Store: run.store, Client: client}

	cases := map[string]struct {
		work    domain.PreparedDeckRecoveryWork
		wantErr error
	}{
		"batch submission":     {work: domain.PreparedDeckRecoveryWork{Kind: "batch_submission", ChunkID: chunks[0].ID, Generation: 1}},
		"batch reconciliation": {work: domain.PreparedDeckRecoveryWork{Kind: "batch_reconciliation", ChunkID: chunks[0].ID, Generation: 1}, wantErr: persistence.ErrPreparedDeckClaimLost},
		"finalizer":            {work: domain.PreparedDeckRecoveryWork{Kind: "finalizer", Generation: 0}, wantErr: persistence.ErrPreparedDeckClaimLost},
		"batch cleanup":        {work: domain.PreparedDeckRecoveryWork{Kind: "batch_cleanup", ChunkID: chunks[0].ID, Generation: 0}},
		"unknown kind":         {work: domain.PreparedDeckRecoveryWork{Kind: "no_such_kind"}},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			work := test.work
			work.OwnerID, work.PreparationID, work.RunID = run.owner, run.prep.ID, run.run.ID
			err := worker.repair(ctx, work)
			if test.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestRecoveryRepairAdvancesTranslationCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, _, _ := newStandardIntegrationRun(t, ctx, 1, 1)
	worker := &RecoveryWorker{Store: run.store, Client: jobLessRiverClient{}}

	err := worker.repair(ctx, domain.PreparedDeckRecoveryWork{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Kind: "translation_completion"})

	require.NoError(t, err, "a run with pending outcomes has nothing to advance yet")
}

// newBatchIntegrationRun freezes a real Batch run so recovery can observe a
// non-standard execution mode.
func newBatchIntegrationRun(t *testing.T, ctx context.Context) (standardIntegrationRun, *river.Client[pgx.Tx]) {
	t.Helper()
	store, err := persistence.Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "batch-recovery-"+uuid.NewString(), false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: "Batch Recovery", MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text"})
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: source.Title, ContentHash: source.ContentHash})
	require.NoError(t, err)
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "recovery-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	entries := []cardexport.Entry{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus steht heute dort.", TargetWord: "Haus", SourceDocument: source.Title, FirstEncounter: 1}}
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, entries, testutil.PresentationProvider{Name: codec.ProviderName(), Version: codec.ContextualGlossProviderVersion(), TargetLanguage: "en", RequireContextualGloss: true})
	require.NoError(t, err)
	runID := uuid.NewString()
	chunks, err := PlanBatchChunks(codec, runID, 1, codec.Model(), enrichment.OpenAIChatCompletionsEndpoint, deck.WorkProjection(), BatchChunkLimits{MaxRequests: 10})
	require.NoError(t, err)
	config := persistence.PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ExecutionMode: string(domain.PreparedDeckExecutionBatch), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext), Provider: codec.ProviderName(), ProviderVersion: codec.ContextualGlossProviderVersion(), Endpoint: enrichment.OpenAIChatCompletionsEndpoint, Model: codec.Model(), BatchMaxRequests: 10, BatchMaxBytes: persistence.DefaultBatchMaxBytes}
	planner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: runID, Projection: deck.StorageProjection(), Config: config, Chunks: chunks}}
	workers := river.NewWorkers()
	river.AddWorker(workers, &BatchSubmitWorker{})
	river.AddWorker(workers, &BatchPollWorker{})
	river.AddWorker(workers, &BatchCleanupWorker{})
	river.AddWorker(workers, &integrationFinalizeWorker{})
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	result, err := NewDurableCoordinator(store, client, planner).Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: preparation.ID})
	require.NoError(t, err)
	return standardIntegrationRun{store: store, owner: owner.ID, prep: preparation, run: result.Run}, client
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
