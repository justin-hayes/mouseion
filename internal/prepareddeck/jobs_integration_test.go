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
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	firstGoal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), client, nil, BatchConfig{}, PreparedDeckConfig{}, false)
	service := &Service{pool: store.Pool(), client: &unconfirmedRiverClient{client: client}, store: store}
	unconfiguredPreparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisHandle.RunID, GoalSnapshotID: firstGoal.SnapshotID, Filename: "unconfigured.apkg", DeckName: "Unconfigured", ContentHash: source.ContentHash})
	require.NoError(t, err)
	unconfiguredWorker := &Worker{Store: store, Coordinator: NewDurableCoordinator(store, client, NewPreparedDeckPlanner(NewInputAssembler(store), cardexport.NewPresentation(nil), nil, false, BatchConfig{}, PreparedDeckConfig{}))}
	unconfiguredArgs := JobArgs{PreparationID: unconfiguredPreparation.ID, OwnerID: owner.ID, SourceMaterialID: source.ID, ContentHash: source.ContentHash, AnalysisRunID: analysisHandle.RunID}
	require.NoError(t, unconfiguredWorker.Work(ctx, &river.Job[JobArgs]{Args: unconfiguredArgs}))
	unconfiguredStatus, err := service.Get(ctx, owner.ID, unconfiguredPreparation.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, unconfiguredStatus.State)
	assert.Contains(t, unconfiguredStatus.Error, "configured translation provider")
	assert.Empty(t, unconfiguredStatus.Artifact, "missing provider must not publish a local-only deck")
	retryAfterConfiguration, err := service.Retry(ctx, owner.ID, unconfiguredPreparation.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationQueued, retryAfterConfiguration.Preparation.State)
	retryJob, err := client.JobGet(ctx, retryAfterConfiguration.JobID)
	require.NoError(t, err)
	assert.NotContains(t, string(retryJob.EncodedArgs), "external_translation_consent", "new retry jobs carry no per-submission consent choice")
	analysisID := strconv.FormatInt(analysisHandle.ID, 10)
	handle, err := service.SubmitForGoal(ctx, owner.ID, analysisID, firstGoal.SnapshotID)
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
	assert.NotContains(t, string(job.EncodedArgs), "external_translation_consent", "new submission jobs carry no per-submission consent choice")
	repeated, err := service.SubmitForGoal(ctx, owner.ID, analysisID, firstGoal.SnapshotID)
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
	for range 8 {
		submissions.Add(1)
		go func() {
			defer submissions.Done()
			result, submitErr := service.SubmitForGoal(ctx, owner.ID, analysisID, firstGoal.SnapshotID)
			if submitErr != nil {
				errorsCh <- submitErr
				return
			}
			results <- result
		}()
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
	retried, err := service.Retry(ctx, owner.ID, handle.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationQueued, retried.Preparation.State)
	assert.NotEqual(t, handle.JobID, retried.JobID)
	var retries sync.WaitGroup
	retryResults := make(chan Handle, 8)
	retryErrors := make(chan error, 8)
	for range 8 {
		retries.Add(1)
		go func() {
			defer retries.Done()
			result, retryErr := service.Retry(ctx, owner.ID, handle.Preparation.ID)
			if retryErr != nil {
				retryErrors <- retryErr
				return
			}
			retryResults <- result
		}()
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
	reprepared, err := service.Retry(ctx, owner.ID, retried.Preparation.ID)
	require.NoError(t, err)
	assert.NotEqual(t, retried.Preparation.ID, reprepared.Preparation.ID, "re-preparation creates a new specification")
	assert.Equal(t, domain.DeckPreparationQueued, reprepared.Preparation.State)
	oldArtifact, err := store.DownloadDeckPreparation(ctx, owner.ID, retried.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("old-artifact"), oldArtifact.Artifact)
	repeatedReprepare, err := service.Retry(ctx, owner.ID, retried.Preparation.ID)
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
	refreshed, err := service.Reprepare(ctx, owner.ID, reprepared.Preparation.ID)
	require.NoError(t, err)
	assert.NotEqual(t, reprepared.Preparation.ID, refreshed.Preparation.ID, "explicit re-preparation creates a distinct generation")
	assert.Equal(t, analysisHandle.RunID, refreshed.Preparation.AnalysisRunID, "re-preparation preserves exact analysis identity")
	assert.Equal(t, reprepared.Preparation.GoalSnapshotID, refreshed.Preparation.GoalSnapshotID, "re-preparation preserves exact Goal snapshot identity")
	assert.Equal(t, domain.DeckPreparationQueued, refreshed.Preparation.State)
	oldGeneration, err := service.Download(ctx, owner.ID, reprepared.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("new-artifact"), oldGeneration.Artifact, "the previous artifact remains owner-downloadable")
	repeatedRefresh, err := service.Reprepare(ctx, owner.ID, reprepared.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, refreshed.Preparation.ID, repeatedRefresh.Preparation.ID, "retries against a retired generation resolve to the current generation")
	assert.Equal(t, refreshed.JobID, repeatedRefresh.JobID)
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, refreshed.Preparation.ID)
	require.NoError(t, err)
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, refreshed.Preparation.ID, domain.DeckPreparation{Artifact: []byte("refreshed-meaning-artifact"), Filename: "A Book.apkg", DeckName: "Mouseion::de::A Book", TotalCards: 1})
	require.NoError(t, err)
	currentArtifact, err := service.Download(ctx, owner.ID, refreshed.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("refreshed-meaning-artifact"), currentArtifact.Artifact)
	completedRefresh, err := service.Reprepare(ctx, owner.ID, reprepared.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, refreshed.Preparation.ID, completedRefresh.Preparation.ID, "a retired generation remains idempotent after its replacement is ready")
	_, err = service.Get(ctx, other.ID, reprepared.Preparation.ID)
	require.ErrorIs(t, err, persistence.ErrNotFound, "retired artifact remains owner-scoped")

	oldGoal := firstGoal
	require.NoError(t, store.ClearPrimaryGoal(ctx, owner.ID, "de", book.ID))
	oldGoalPreparation, err := store.GetDeckPreparation(ctx, owner.ID, refreshed.Preparation.ID)
	require.NoError(t, err)
	assert.Nil(t, oldGoalPreparation.RetiredAt, "releasing Reading leaves submitted work available to finish")
	_, err = service.Submit(ctx, owner.ID, analysisID)
	require.ErrorIs(t, err, persistence.ErrInvalidTransition, "the former generic Book submission remains unavailable after Reading ends")

	var firstGoalPreparation Handle
	firstGoal, err = store.CreatePrimaryGoalWith(ctx, owner.ID, "de", book.ID, func(ctx context.Context, tx pgx.Tx, reading domain.PrimaryGoal) error {
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
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, firstGoalPreparation.Preparation.ID)
	require.NoError(t, err)
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, firstGoalPreparation.Preparation.ID, domain.DeckPreparation{Artifact: []byte("goal-old-artifact"), Filename: "A Book.apkg", DeckName: "Mouseion::de::A Book", TotalCards: 1})
	require.NoError(t, err)
	goalRefresh, err := service.Reprepare(ctx, owner.ID, firstGoalPreparation.Preparation.ID)
	require.NoError(t, err)
	assert.NotEqual(t, firstGoalPreparation.Preparation.ID, goalRefresh.Preparation.ID)
	assert.Equal(t, firstGoalPreparation.Preparation.AnalysisRunID, goalRefresh.Preparation.AnalysisRunID)
	assert.Equal(t, firstGoal.SnapshotID, goalRefresh.Preparation.GoalSnapshotID, "Goal re-preparation preserves exact snapshot identity")
	goalHistory, err := service.Download(ctx, owner.ID, firstGoalPreparation.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("goal-old-artifact"), goalHistory.Artifact)
	_, err = service.Submit(ctx, owner.ID, analysisID)
	require.ErrorIs(t, err, persistence.ErrInvalidTransition, "generic submission must not reuse an active Goal preparation")
	require.NoError(t, store.ClearPrimaryGoal(ctx, owner.ID, "de", book.ID))
	retiredGoalPreparation, err := store.GetDeckPreparation(ctx, owner.ID, goalRefresh.Preparation.ID)
	require.NoError(t, err)
	assert.Nil(t, retiredGoalPreparation.RetiredAt, "releasing a Goal snapshot leaves its preparation available to finish")
	_, err = service.Submit(ctx, owner.ID, analysisID)
	require.ErrorIs(t, err, persistence.ErrInvalidTransition, "the former generic Book submission remains unavailable after Reading ends")
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
	assert.Nil(t, retiredSecondGoalPreparation.RetiredAt, "Goal clear preserves the submitted preparation so its job can complete")
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, secondGoalPreparation.Preparation.ID)
	require.NoError(t, err, "a job submitted before Goal clear remains claimable afterwards")
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, secondGoalPreparation.Preparation.ID, domain.DeckPreparation{Artifact: []byte("after-goal-clear"), Filename: "after-clear.apkg", DeckName: "After clear", TotalCards: 1})
	require.NoError(t, err)

	_, err = service.Retry(ctx, owner.ID, firstGoalPreparation.Preparation.ID)
	require.ErrorIs(t, err, persistence.ErrInvalidTransition, "a released snapshot cannot be retried into a new generation")
	_, err = service.Retry(ctx, owner.ID, secondGoalPreparation.Preparation.ID)
	require.ErrorIs(t, err, persistence.ErrInvalidTransition, "a submitted job cannot be retried after Goal clear")
	_, err = store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.NotEmpty(t, oldGoal.SnapshotID, "the first Reading snapshot remains historical")
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
	testutil.StopOnCleanup(t, "River client", client.Stop)
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
	_, err = service.Submit(ctx, owner.ID, source.ID)
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
	retried, err := service.Retry(ctx, owner.ID, p.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, retried.Preparation.State)
	assert.Contains(t, retried.Preparation.Error, "River unavailable")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='prepared_deck' AND details->>'preparation_id'=$2`, owner.ID, p.ID).Scan(&count))
	assert.GreaterOrEqual(t, count, 3, "retry history")
}
