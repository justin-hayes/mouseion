//go:build integration

package prepareddeck

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceEnqueuesOwnerScopedImmutablePreparationAndConfirmsUnreportedJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "prepared-service-owner", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "prepared-service-other", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "service-book", Title: "A Book", MediaType: "application/epub+zip", Content: []byte("text"), FullText: "text"}, domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "unit"), Order: 0, SpineIndex: 0, ManifestID: "unit", Text: "text", EndOffset: 4}}})
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: source.Title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
	analysisRiver, err := analysis.NewClient(store.Pool(), &analyzertest.Fake{}, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	analysisService := analysis.NewService(store.Pool(), analysisRiver)
	analysisHandle, err := analysisService.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	artifactHash := "sha256:prepared-analysis-artifact"
	_, err = store.Pool().Exec(ctx, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version) VALUES($1,'de','1','casefold','1','fake','1')`, artifactHash)
	require.NoError(t, err)
	var corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash,analysis_run_id,status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count) VALUES($1,$2,$3,$4,'complete',0,0,1,1,0,1,1,0) RETURNING id::text`, owner.ID, source.ID, artifactHash, analysisHandle.RunID).Scan(&corpusID))
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET state='completed',corpus_id=$2,completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$3`, owner.ID, corpusID, analysisHandle.RunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_run_attempts SET state='completed',finalized_at=now() WHERE run_id=$1`, analysisHandle.RunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_jobs SET corpus_id=$2,progress=100,updated_at=now() WHERE owner_id=$1 AND analysis_run_id=$3`, owner.ID, corpusID, analysisHandle.RunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner.ID, book.ID, source.ID, analysisHandle.RunID)
	require.NoError(t, err)
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), client, nil, BatchConfig{}, PreparedDeckConfig{}, false)
	service := &Service{pool: store.Pool(), client: &unconfirmedRiverClient{client: client}, store: store}
	analysisID := strconv.FormatInt(analysisHandle.ID, 10)
	handle, err := service.Submit(ctx, owner.ID, analysisID, true)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationQueued, handle.Preparation.State)
	assert.Equal(t, source.ContentHash, handle.Preparation.ContentHash)
	assert.NotZero(t, handle.JobID)
	var args JobArgs
	job, err := client.JobGet(ctx, handle.JobID)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(job.EncodedArgs, &args))
	assert.Equal(t, owner.ID, args.OwnerID)
	assert.Equal(t, source.ID, args.SourceMaterialID)
	assert.Equal(t, source.ContentHash, args.ContentHash)
	assert.Equal(t, analysisHandle.RunID, args.AnalysisRunID)
	assert.True(t, args.ExternalTranslationConsent)
	repeated, err := service.Submit(ctx, owner.ID, analysisID, false)
	require.NoError(t, err)
	assert.Equal(t, handle.Preparation.ID, repeated.Preparation.ID)
	assert.Equal(t, handle.JobID, repeated.JobID)
	exact, err := service.GetForAnalysis(ctx, owner.ID, source.ID, analysisHandle.RunID)
	require.NoError(t, err)
	assert.Equal(t, handle.Preparation.ID, exact.ID)
	assert.Equal(t, analysisHandle.RunID, exact.AnalysisRunID)
	_, err = service.GetForAnalysis(ctx, other.ID, source.ID, analysisHandle.RunID)
	assert.ErrorIs(t, err, persistence.ErrNotFound, "cross-owner exact preparation lookup") //nolint:testifylint // Cross-owner lookup is independent of the concurrent submission exercise.
	_, err = service.GetForAnalysis(ctx, owner.ID, other.ID, analysisHandle.RunID)
	assert.ErrorIs(t, err, persistence.ErrNotFound, "cross-book exact preparation lookup") //nolint:testifylint // Cross-book lookup is independent of the concurrent submission exercise.
	var submissions sync.WaitGroup
	results := make(chan Handle, 8)
	errorsCh := make(chan error, 8)
	for i := range 8 {
		submissions.Add(1)
		go func(consent bool) {
			defer submissions.Done()
			result, submitErr := service.Submit(ctx, owner.ID, analysisID, consent)
			if submitErr != nil {
				errorsCh <- submitErr
				return
			}
			results <- result
		}(i%2 == 0)
	}
	submissions.Wait()
	close(results)
	close(errorsCh)
	for submitErr := range errorsCh {
		assert.NoError(t, submitErr, "concurrent submit") //nolint:testifylint // Collect every independent concurrent result instead of stopping at the first failure.
	}
	for result := range results {
		assert.Equal(t, handle.Preparation.ID, result.Preparation.ID)
		assert.Equal(t, handle.JobID, result.JobID, "concurrent duplicate")
	}
	var jobCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'preparation_id'=$2`, (JobArgs{}).Kind(), handle.Preparation.ID).Scan(&jobCount))
	assert.Equal(t, 1, jobCount, "duplicate jobs")
	_, err = service.Get(ctx, other.ID, handle.Preparation.ID)
	assert.ErrorIs(t, err, persistence.ErrNotFound, "cross-owner get") //nolint:testifylint // Cross-owner lookup is independent of the cancellation and retry checks.
	cancelled, err := service.Cancel(ctx, owner.ID, handle.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationCancelled, cancelled.State)
	retried, err := service.Retry(ctx, owner.ID, handle.Preparation.ID, false)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationQueued, retried.Preparation.State)
	assert.NotEqual(t, handle.JobID, retried.JobID)
	var retries sync.WaitGroup
	retryResults := make(chan Handle, 8)
	retryErrors := make(chan error, 8)
	for i := range 8 {
		retries.Add(1)
		go func(consent bool) {
			defer retries.Done()
			result, retryErr := service.Retry(ctx, owner.ID, handle.Preparation.ID, consent)
			if retryErr != nil {
				retryErrors <- retryErr
				return
			}
			retryResults <- result
		}(i%2 == 0)
	}
	retries.Wait()
	close(retryResults)
	close(retryErrors)
	for retryErr := range retryErrors {
		assert.NoError(t, retryErr, "concurrent retry") //nolint:testifylint // Collect every independent concurrent result instead of stopping at the first failure.
	}
	for result := range retryResults {
		assert.Equal(t, handle.Preparation.ID, result.Preparation.ID)
		assert.Equal(t, retried.JobID, result.JobID, "concurrent retry")
	}
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, retried.Preparation.ID)
	require.NoError(t, err)
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, retried.Preparation.ID, domain.DeckPreparation{Artifact: []byte("old-artifact"), Filename: "old.apkg", DeckName: "Old", TotalCards: 1})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET error=$1 WHERE owner_id=$2 AND id=$3`, domain.DeckPreparationRequiresRepreparationError, owner.ID, retried.Preparation.ID)
	require.NoError(t, err)
	reprepared, err := service.Retry(ctx, owner.ID, retried.Preparation.ID, false)
	require.NoError(t, err)
	assert.NotEqual(t, retried.Preparation.ID, reprepared.Preparation.ID, "re-preparation creates a new specification")
	assert.Equal(t, domain.DeckPreparationQueued, reprepared.Preparation.State)
	oldArtifact, err := store.DownloadDeckPreparation(ctx, owner.ID, retried.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("old-artifact"), oldArtifact.Artifact)
	repeatedReprepare, err := service.Retry(ctx, owner.ID, retried.Preparation.ID, false)
	require.NoError(t, err)
	assert.Equal(t, reprepared.Preparation.ID, repeatedReprepare.Preparation.ID, "re-preparation is idempotent")
	assert.Equal(t, reprepared.JobID, repeatedReprepare.JobID)
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, reprepared.Preparation.ID)
	require.NoError(t, err)
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, reprepared.Preparation.ID, domain.DeckPreparation{Artifact: []byte("new-artifact"), Filename: "new.apkg", DeckName: "New", TotalCards: 1})
	require.NoError(t, err)
	newArtifact, err := service.Download(ctx, owner.ID, reprepared.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("new-artifact"), newArtifact.Artifact)

	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	var firstGoalPreparation Handle
	firstGoal, err := store.CreatePrimaryGoalWith(ctx, owner.ID, "de", book.ID, func(ctx context.Context, tx pgx.Tx, reading domain.PrimaryGoal) error {
		var queueErr error
		firstGoalPreparation, queueErr = service.SubmitForGoalTx(ctx, tx, owner.ID, analysisID, reading.SnapshotID)
		return queueErr
	})
	require.NoError(t, err)
	require.Equal(t, domain.DeckPreparationQueued, firstGoalPreparation.Preparation.State)
	require.NotZero(t, firstGoalPreparation.JobID)
	assert.Equal(t, firstGoal.SnapshotID, firstGoalPreparation.Preparation.GoalSnapshotID)
	var goalSubmissions sync.WaitGroup
	goalResults := make(chan Handle, 8)
	goalErrors := make(chan error, 8)
	startGoalSubmissions := make(chan struct{})
	for range 8 {
		goalSubmissions.Add(1)
		go func() {
			defer goalSubmissions.Done()
			<-startGoalSubmissions
			result, submitErr := service.SubmitForGoal(ctx, owner.ID, analysisID, firstGoal.SnapshotID)
			if submitErr != nil {
				goalErrors <- submitErr
				return
			}
			goalResults <- result
		}()
	}
	close(startGoalSubmissions)
	goalSubmissions.Wait()
	close(goalResults)
	close(goalErrors)
	for submitErr := range goalErrors {
		assert.NoError(t, submitErr, "concurrent Goal submission") //nolint:testifylint // Collect every independent concurrent result instead of stopping at the first failure.
	}
	for result := range goalResults {
		assert.Equal(t, firstGoalPreparation.Preparation.ID, result.Preparation.ID)
		assert.Equal(t, firstGoalPreparation.JobID, result.JobID)
	}
	require.NotEmpty(t, firstGoalPreparation.Preparation.ID)
	_, err = service.Submit(ctx, owner.ID, analysisID, true)
	require.ErrorIs(t, err, persistence.ErrInvalidTransition, "generic submission must not reuse an active Goal preparation")
	require.NoError(t, store.ClearPrimaryGoal(ctx, owner.ID, "de", book.ID))
	retiredGoalPreparation, err := store.GetDeckPreparation(ctx, owner.ID, firstGoalPreparation.Preparation.ID)
	require.NoError(t, err)
	require.NotNil(t, retiredGoalPreparation.RetiredAt, "releasing a Goal snapshot retires its preparation")
	genericPreparation, err := service.Submit(ctx, owner.ID, analysisID, true)
	require.NoError(t, err)
	assert.Empty(t, genericPreparation.Preparation.GoalSnapshotID, "generic submission reused a released Goal preparation")
	assert.NotEqual(t, firstGoalPreparation.Preparation.ID, genericPreparation.Preparation.ID)
	secondGoal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	blockingClient := &blockingRiverClient{client: service.client, inserted: make(chan struct{}), release: make(chan struct{})}
	service.client = blockingClient
	submitted := make(chan Handle, 1)
	submitErrors := make(chan error, 1)
	go func() {
		result, submitErr := service.SubmitForGoal(ctx, owner.ID, analysisID, secondGoal.SnapshotID)
		submitted <- result
		submitErrors <- submitErr
	}()
	select {
	case <-blockingClient.inserted:
	case <-ctx.Done():
		require.FailNow(t, "Goal submission did not reach River insertion", ctx.Err())
	}
	cleared := make(chan error, 1)
	go func() {
		cleared <- store.ClearPrimaryGoal(ctx, owner.ID, "de", book.ID)
	}()
	select {
	case clearErr := <-cleared:
		require.FailNow(t, "Goal clear did not wait for snapshot-bound submission", clearErr)
	case <-time.After(100 * time.Millisecond):
	}
	close(blockingClient.release)
	secondGoalPreparation := <-submitted
	require.NoError(t, <-submitErrors)
	require.NoError(t, <-cleared)
	retiredSecondGoalPreparation, err := store.GetDeckPreparation(ctx, owner.ID, secondGoalPreparation.Preparation.ID)
	require.NoError(t, err)
	require.NotNil(t, retiredSecondGoalPreparation.RetiredAt, "Goal clear left a concurrent snapshot-bound preparation current")

	_, err = service.Retry(ctx, owner.ID, firstGoalPreparation.Preparation.ID, false)
	assert.ErrorIs(t, err, persistence.ErrInvalidTransition, "a retired preparation must not resolve across Goal snapshots")
}

func TestServiceReconcilesOrphanedPreparationStates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "reconcile-owner", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "reconcile-book", Title: "A Book", MediaType: "text/plain", ContentHash: "reconcile-hash", Content: []byte("text"), FullText: "text"})
	require.NoError(t, err)
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), client, nil, BatchConfig{}, PreparedDeckConfig{}, false)
	service := NewService(store, client)
	queued, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: "queued.apkg", DeckName: "queued", ContentHash: "queued-hash"})
	require.NoError(t, err)
	got, err := service.Get(ctx, owner.ID, queued.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationQueued, got.State)
	var queuedJobs int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'preparation_id'=$2 AND state IN ('available','pending','running','retryable','scheduled')`, (JobArgs{}).Kind(), queued.ID).Scan(&queuedJobs))
	assert.Equal(t, 1, queuedJobs)
	preparing, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: "preparing.apkg", DeckName: "preparing", ContentHash: "preparing-hash"})
	require.NoError(t, err)
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, preparing.ID)
	require.NoError(t, err)
	got, err = service.Get(ctx, owner.ID, preparing.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, got.State)
	assert.Equal(t, orphanedPreparationError, got.Error)
	retired, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: "retired.apkg", DeckName: "retired", ContentHash: "retired-hash"})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET retired_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, retired.ID)
	require.NoError(t, err)
	got, err = service.Get(ctx, owner.ID, retired.ID)
	require.NoError(t, err)
	require.NotNil(t, got.RetiredAt)
	var retiredJobs int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'preparation_id'=$2`, (JobArgs{}).Kind(), retired.ID).Scan(&retiredJobs))
	assert.Zero(t, retiredJobs, "reconciliation enqueued a retired preparation")
	var historyCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='prepared_deck' AND details->>'preparation_id'=$2`, owner.ID, preparing.ID).Scan(&historyCount))
	assert.NotZero(t, historyCount, "reconciliation history")
}

type failingRiverClient struct{ err error }

func (c *failingRiverClient) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, c.err
}

func (*failingRiverClient) JobCancel(context.Context, int64) (*rivertype.JobRow, error) {
	return nil, errors.New("unexpected job cancellation")
}

type unconfirmedRiverClient struct{ client *river.Client[pgx.Tx] }

func (c *unconfirmedRiverClient) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	_, err := c.client.InsertTx(ctx, tx, args, opts)
	if err != nil {
		return nil, err
	}
	return &rivertype.JobInsertResult{}, nil
}

func (c *unconfirmedRiverClient) JobCancel(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	return c.client.JobCancel(ctx, id)
}

type blockingRiverClient struct {
	client   riverClient
	inserted chan struct{}
	release  chan struct{}
}

func (c *blockingRiverClient) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	result, err := c.client.InsertTx(ctx, tx, args, opts)
	close(c.inserted)
	select {
	case <-c.release:
		return result, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *blockingRiverClient) JobCancel(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	return c.client.JobCancel(ctx, id)
}

func TestServiceEnqueueFailureDoesNotLeaveWaitingPreparation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "enqueue-failure-owner", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "enqueue-failure-book", Title: "A Book", MediaType: "text/plain", ContentHash: "enqueue-failure-hash", Content: []byte("text"), FullText: "text"})
	require.NoError(t, err)
	failing := &failingRiverClient{err: errors.New("River unavailable")}
	service := &Service{pool: store.Pool(), client: failing, store: store}
	_, err = service.Submit(ctx, owner.ID, source.ID, false)
	assert.Error(t, err, "expected initial enqueue failure") //nolint:testifylint // Error classification and the rollback query are independent expectations.
	var count int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparations WHERE owner_id=$1`, owner.ID).Scan(&count))
	assert.Zero(t, count, "initial enqueue left preparations")
	p, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: "retry.apkg", DeckName: "retry", ContentHash: "retry-hash"})
	require.NoError(t, err)
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, p.ID)
	require.NoError(t, err)
	_, err = store.FailDeckPreparation(ctx, owner.ID, p.ID, "previous attempt failed")
	require.NoError(t, err)
	retried, err := service.Retry(ctx, owner.ID, p.ID, false)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, retried.Preparation.State)
	assert.Contains(t, retried.Preparation.Error, "River unavailable")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='prepared_deck' AND details->>'preparation_id'=$2`, owner.ID, p.ID).Scan(&count))
	assert.GreaterOrEqual(t, count, 3, "retry history")
}
