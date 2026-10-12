//go:build integration

package prepareddeck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
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
	source, book, analysisHandle, firstReading := seedAnalyzedReading(t, ctx, store, owner, "service-book")
	require.NoError(t, err)
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), client, nil, BatchConfig{}, PreparedDeckConfig{}, false)
	service := &Service{pool: store.Pool(), client: &unconfirmedRiverClient{client: client}, store: store}
	unconfiguredPreparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisHandle.RunID, SnapshotID: firstReading.SnapshotID, Filename: "unconfigured.apkg", DeckName: "Unconfigured", ContentHash: source.ContentHash})
	require.NoError(t, err)
	unconfiguredWorker := &Worker{Store: store, Coordinator: NewDurableCoordinator(store, client, NewPreparedDeckPlanner(NewInputAssembler(store), cardexport.NewPresentation(nil), nil, false, BatchConfig{}, PreparedDeckConfig{}))}
	unconfiguredArgs := JobArgs{PreparationID: unconfiguredPreparation.ID, OwnerID: owner.ID, SourceMaterialID: source.ID, ContentHash: source.ContentHash, AnalysisRunID: analysisHandle.RunID}
	require.NoError(t, unconfiguredWorker.Work(ctx, &river.Job[JobArgs]{Args: unconfiguredArgs}))
	unconfiguredStatus, err := service.Get(ctx, owner.ID, unconfiguredPreparation.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, unconfiguredStatus.State)
	assert.Contains(t, unconfiguredStatus.Error, "configured translation provider")
	assert.Empty(t, unconfiguredStatus.Artifact, "missing provider must not publish a local-only deck")
	retryAfterConfiguration, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationQueued, retryAfterConfiguration.Preparation.State)
	retryJob, err := client.JobGet(ctx, retryAfterConfiguration.JobID)
	require.NoError(t, err)
	assert.NotContains(t, string(retryJob.EncodedArgs), "external_translation_consent", "new retry jobs carry no per-submission consent choice")
	handle, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
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
	repeated, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, handle.Preparation.ID, repeated.Preparation.ID)
	assert.Equal(t, handle.JobID, repeated.JobID)
	exact, err := service.GetForAnalysis(ctx, owner.ID, source.ID, analysisHandle.RunID)
	require.NoError(t, err)
	assert.Equal(t, handle.Preparation.ID, exact.ID)
	assert.Equal(t, analysisHandle.RunID, exact.AnalysisRunID)
	t.Run("cross-owner exact preparation lookup is not found", func(t *testing.T) {
		_, err := service.GetForAnalysis(ctx, other.ID, source.ID, analysisHandle.RunID)
		require.ErrorIs(t, err, persistence.ErrNotFound)
	})
	t.Run("cross-book exact preparation lookup is not found", func(t *testing.T) {
		_, err := service.GetForAnalysis(ctx, owner.ID, other.ID, analysisHandle.RunID)
		require.ErrorIs(t, err, persistence.ErrNotFound)
	})
	var submissions sync.WaitGroup
	results := make(chan Handle, 8)
	errorsCh := make(chan error, 8)
	for range 8 {
		submissions.Go(func() {
			result, submitErr := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
			if submitErr != nil {
				errorsCh <- submitErr
				return
			}
			results <- result
		})
	}
	submissions.Wait()
	close(results)
	close(errorsCh)
	requireConcurrentSubmissions(t, results, errorsCh, handle.Preparation.ID, handle.JobID, "concurrent submit")
	var jobCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'preparation_id'=$2`, (JobArgs{}).Kind(), handle.Preparation.ID).Scan(&jobCount))
	assert.Equal(t, 1, jobCount, "duplicate jobs")
	t.Run("cross-owner get is not found", func(t *testing.T) {
		_, err := service.Get(ctx, other.ID, handle.Preparation.ID)
		require.ErrorIs(t, err, persistence.ErrNotFound)
	})
	cancelled, err := service.Cancel(ctx, owner.ID, handle.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationCancelled, cancelled.State)
	retried, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationQueued, retried.Preparation.State)
	assert.NotEqual(t, handle.JobID, retried.JobID)
	var retries sync.WaitGroup
	retryResults := make(chan Handle, 8)
	retryErrors := make(chan error, 8)
	for range 8 {
		retries.Go(func() {
			result, retryErr := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
			if retryErr != nil {
				retryErrors <- retryErr
				return
			}
			retryResults <- result
		})
	}
	retries.Wait()
	close(retryResults)
	close(retryErrors)
	requireConcurrentSubmissions(t, retryResults, retryErrors, handle.Preparation.ID, retried.JobID, "concurrent retry")
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, retried.Preparation.ID)
	require.NoError(t, err)
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, retried.Preparation.ID, domain.DeckPreparation{Artifact: []byte("old-artifact"), Filename: "old.apkg", DeckName: "Old", TotalCards: 1})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET error=$1 WHERE owner_id=$2 AND id=$3`, domain.DeckPreparationRequiresRepreparationError, owner.ID, retried.Preparation.ID)
	require.NoError(t, err)
	reprepared, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
	require.NoError(t, err)
	assert.NotEqual(t, retried.Preparation.ID, reprepared.Preparation.ID, "re-preparation creates a new specification")
	assert.Equal(t, domain.DeckPreparationQueued, reprepared.Preparation.State)
	oldArtifact, err := store.DownloadDeckPreparation(ctx, owner.ID, retried.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("old-artifact"), oldArtifact.Artifact)
	repeatedReprepare, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
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
	refreshed, err := service.Reprepare(ctx, owner.ID, reprepared.Preparation.ID, firstReading.SnapshotID)
	require.NoError(t, err)
	assert.NotEqual(t, reprepared.Preparation.ID, refreshed.Preparation.ID, "explicit re-preparation creates a distinct generation")
	assert.Equal(t, analysisHandle.RunID, refreshed.Preparation.AnalysisRunID, "re-preparation preserves exact analysis identity")
	assert.Equal(t, reprepared.Preparation.SnapshotID, refreshed.Preparation.SnapshotID, "re-preparation preserves exact Reading snapshot identity")
	assert.Equal(t, domain.DeckPreparationQueued, refreshed.Preparation.State)
	oldGeneration, err := service.Download(ctx, owner.ID, reprepared.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("new-artifact"), oldGeneration.Artifact, "the previous artifact remains owner-downloadable")
	_, err = service.Reprepare(ctx, owner.ID, reprepared.Preparation.ID, firstReading.SnapshotID)
	require.ErrorIs(t, err, domain.ErrDeckPreparationInvalidTransition, "a retired generation is refused, not resolved to its replacement")
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, refreshed.Preparation.ID)
	require.NoError(t, err)
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, refreshed.Preparation.ID, domain.DeckPreparation{Artifact: []byte("refreshed-meaning-artifact"), Filename: "A Book.apkg", DeckName: "Mouseion::de::A Book", TotalCards: 1})
	require.NoError(t, err)
	currentArtifact, err := service.Download(ctx, owner.ID, refreshed.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("refreshed-meaning-artifact"), currentArtifact.Artifact)
	_, err = service.Reprepare(ctx, owner.ID, reprepared.Preparation.ID, firstReading.SnapshotID)
	require.ErrorIs(t, err, domain.ErrDeckPreparationInvalidTransition, "a retired generation stays refused after its replacement is ready")
	_, err = service.Get(ctx, other.ID, reprepared.Preparation.ID)
	require.ErrorIs(t, err, persistence.ErrNotFound, "retired artifact remains owner-scoped")

	oldReading := firstReading
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, firstReading.SnapshotID))
	oldReadingPreparation, err := store.GetDeckPreparation(ctx, owner.ID, refreshed.Preparation.ID)
	require.NoError(t, err)
	assert.Nil(t, oldReadingPreparation.RetiredAt, "releasing Reading leaves submitted work available to finish")

	firstReading, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	firstReadingPreparation, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
	require.NoError(t, err)
	require.Equal(t, domain.DeckPreparationQueued, firstReadingPreparation.Preparation.State)
	require.NotZero(t, firstReadingPreparation.JobID)
	assert.Equal(t, firstReading.SnapshotID, firstReadingPreparation.Preparation.SnapshotID)
	var readingSubmissions sync.WaitGroup
	readingResults := make(chan Handle, 8)
	readingErrors := make(chan error, 8)
	startReadingSubmissions := make(chan struct{})
	for range 8 {
		readingSubmissions.Go(func() {
			<-startReadingSubmissions
			result, submitErr := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
			if submitErr != nil {
				readingErrors <- submitErr
				return
			}
			readingResults <- result
		})
	}
	close(startReadingSubmissions)
	readingSubmissions.Wait()
	close(readingResults)
	close(readingErrors)
	requireConcurrentSubmissions(t, readingResults, readingErrors, firstReadingPreparation.Preparation.ID, firstReadingPreparation.JobID, "concurrent Reading submission")
	require.NotEmpty(t, firstReadingPreparation.Preparation.ID)
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, firstReadingPreparation.Preparation.ID)
	require.NoError(t, err)
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, firstReadingPreparation.Preparation.ID, domain.DeckPreparation{Artifact: []byte("reading-old-artifact"), Filename: "A Book.apkg", DeckName: "Mouseion::de::A Book", TotalCards: 1})
	require.NoError(t, err)
	readingRefresh, err := service.Reprepare(ctx, owner.ID, firstReadingPreparation.Preparation.ID, firstReading.SnapshotID)
	require.NoError(t, err)
	assert.NotEqual(t, firstReadingPreparation.Preparation.ID, readingRefresh.Preparation.ID)
	assert.Equal(t, firstReadingPreparation.Preparation.AnalysisRunID, readingRefresh.Preparation.AnalysisRunID)
	assert.Equal(t, firstReading.SnapshotID, readingRefresh.Preparation.SnapshotID, "Reading re-preparation preserves exact snapshot identity")
	readingHistory, err := service.Download(ctx, owner.ID, firstReadingPreparation.Preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("reading-old-artifact"), readingHistory.Artifact)
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, firstReading.SnapshotID))
	retiredReadingPreparation, err := store.GetDeckPreparation(ctx, owner.ID, readingRefresh.Preparation.ID)
	require.NoError(t, err)
	assert.Nil(t, retiredReadingPreparation.RetiredAt, "releasing a Reading snapshot leaves its preparation available to finish")
	secondReading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	blockingClient := &blockingRiverClient{client: service.client, inserted: make(chan struct{}), release: make(chan struct{})}
	service.client = blockingClient
	submitted := make(chan Handle, 1)
	submitErrors := make(chan error, 1)
	go func() {
		result, submitErr := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, secondReading.SnapshotID)
		submitted <- result
		submitErrors <- submitErr
	}()
	select {
	case <-blockingClient.inserted:
	case <-ctx.Done():
		require.FailNow(t, "Reading submission did not reach River insertion", ctx.Err())
	}
	cleared := make(chan error, 1)
	go func() {
		cleared <- store.EndCurrentReading(ctx, owner.ID, "de", book.ID, secondReading.SnapshotID)
	}()
	select {
	case clearErr := <-cleared:
		require.FailNow(t, "Reading clear did not wait for snapshot-bound submission", clearErr)
	case <-time.After(100 * time.Millisecond):
	}
	close(blockingClient.release)
	secondReadingPreparation := <-submitted
	require.NoError(t, <-submitErrors)
	require.NoError(t, <-cleared)
	retiredSecondReadingPreparation, err := store.GetDeckPreparation(ctx, owner.ID, secondReadingPreparation.Preparation.ID)
	require.NoError(t, err)
	assert.Nil(t, retiredSecondReadingPreparation.RetiredAt, "Reading clear preserves the submitted preparation so its job can complete")
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, secondReadingPreparation.Preparation.ID)
	require.NoError(t, err, "a job submitted before Reading clear remains claimable afterwards")
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, secondReadingPreparation.Preparation.ID, domain.DeckPreparation{Artifact: []byte("after-reading-clear"), Filename: "after-clear.apkg", DeckName: "After clear", TotalCards: 1})
	require.NoError(t, err)

	_, err = service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
	require.ErrorIs(t, err, domain.ErrDeckPreparationNotCurrentReading, "a released snapshot cannot be retried into a new generation")
	_, err = service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, firstReading.SnapshotID)
	require.ErrorIs(t, err, domain.ErrDeckPreparationNotCurrentReading, "a submitted job cannot be retried after Reading clear")
	_, err = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.NotEmpty(t, oldReading.SnapshotID, "the first Reading snapshot remains historical")
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

// seedAnalyzedReading stores one Book with a completed analysis and starts it
// as the owner's Current reading, returning the exact identities a deck
// preparation is bound to.
func seedAnalyzedReading(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner domain.User, sourceIdentifier string) (domain.SourceMaterial, domain.Book, analysis.Handle, domain.CurrentReading) {
	t.Helper()
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: sourceIdentifier, Title: "A Book", MediaType: "application/epub+zip", Content: []byte("text"), FullText: "text"}, domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "unit"), Order: 0, SpineIndex: 0, ManifestID: "unit", Text: "text", EndOffset: 4}}})
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
	// A frozen snapshot with no vocabulary needs no deck, so seed one identity for it to prepare.
	_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{OwnerID: owner.ID, CorpusID: corpusID, Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3, ObservedForms: []byte(`["haus"]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{"min_occurrences":3}`)})
	require.NoError(t, err)
	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	require.Positive(t, reading.SnapshotSize)
	return source, book, analysisHandle, reading
}

func TestCancelLogsBestEffortRemoteFailuresWithoutChangingOutcome(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "best-effort-cancel-owner", false)
	require.NoError(t, err)
	source, book, _, reading := seedAnalyzedReading(t, ctx, store, owner, "best-effort-cancel-book")
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), client, nil, BatchConfig{}, PreparedDeckConfig{}, false)
	provider := &failingBatchProvider{err: errors.New("provider unavailable")}
	service := &Service{pool: store.Pool(), client: &failingCancelRiverClient{client: client}, store: store, batchCanceller: provider}
	handle, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, reading.SnapshotID)
	require.NoError(t, err)
	require.NotZero(t, handle.JobID)

	// A live batch chunk makes Cancel reach provider cancellation and file cleanup.
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, []cardexport.Entry{{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: source.Title, FirstEncounter: 1}}, testutil.PresentationProvider{Name: "openai", Version: "v1", TargetLanguage: "en"})
	require.NoError(t, err)
	freezeTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	frozen, freezeErr := store.FreezePreparedDeckRunTx(ctx, freezeTx, persistence.FreezePreparedDeckRunParams{OwnerID: owner.ID, PreparationID: handle.Preparation.ID, Projection: deck.StorageProjection(), Config: persistence.PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions", Model: "gpt-test"}, Chunks: []persistence.PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("c", 64), InputBytes: 64, Ordinals: []int{0}}}})
	if freezeErr != nil {
		require.NoError(t, errors.Join(freezeErr, txcleanup.Rollback(ctx, freezeTx)))
	}
	require.NoError(t, freezeTx.Commit(ctx))
	jobTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	chunkID := frozen.Chunks[0].ID
	submitErr := store.SetPreparedDeckBatchSubmissionJobTx(ctx, jobTx, owner.ID, handle.Preparation.ID, frozen.Run.ID, chunkID, 1, 9102)
	if submitErr != nil {
		require.NoError(t, errors.Join(submitErr, txcleanup.Rollback(ctx, jobTx)))
	}
	require.NoError(t, jobTx.Commit(ctx))
	submissionToken := uuid.NewString()
	_, err = store.ClaimPreparedDeckBatchSubmission(ctx, owner.ID, handle.Preparation.ID, frozen.Run.ID, chunkID, 1, submissionToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	_, err = store.RecordPreparedDeckBatchSubmitted(ctx, owner.ID, handle.Preparation.ID, frozen.Run.ID, chunkID, 1, submissionToken, "cancel-file", "cancel-batch", time.Now().UTC())
	require.NoError(t, err)

	// Rejecting the write that records a failed deletion makes the immediate
	// cleanup return an error, the path that durable recovery must cover.
	_, err = store.Pool().Exec(ctx, `CREATE FUNCTION reject_failed_cleanup_write() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'cleanup state write unavailable'; END$$`)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `CREATE TRIGGER reject_failed_cleanup_write BEFORE UPDATE OF input_file_cleanup_state ON deck_preparation_batch_chunks FOR EACH ROW WHEN (NEW.input_file_cleanup_state = 'failed') EXECUTE FUNCTION reject_failed_cleanup_write()`)
	require.NoError(t, err)

	var output bytes.Buffer
	previousWriter, previousFlags := log.Writer(), log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	cancelled, err := service.Cancel(ctx, owner.ID, handle.Preparation.ID)
	require.NoError(t, err, "best-effort remote failures must not fail cancellation")
	assert.Equal(t, domain.DeckPreparationCancelled, cancelled.State)
	logged := output.String()
	assert.Contains(t, logged, "River job cancellation failed")
	assert.Contains(t, logged, "preparation_id="+handle.Preparation.ID)
	assert.Contains(t, logged, "river_job_id="+strconv.FormatInt(handle.JobID, 10))
	assert.Contains(t, logged, "provider batch cancellation failed")
	assert.Contains(t, logged, "provider_batch_id=cancel-batch")
	assert.Contains(t, logged, "provider file deletion failed; durable recovery will retry while attempts remain")
	assert.Contains(t, logged, "chunk_id="+chunkID)
	assert.Contains(t, logged, "file_role=input")
	assert.Contains(t, logged, "immediate cleanup failed; durable recovery will retry")
	assert.Contains(t, logged, "cleanup state write unavailable")
	chunk, err := store.GetPreparedDeckBatchChunk(ctx, owner.ID, handle.Preparation.ID, frozen.Run.ID, chunkID)
	require.NoError(t, err)
	assert.Equal(t, "pending", chunk.InputFileCleanupState, "rejected cleanup write must leave the file pending for recovery")
}

func TestCancelWithoutDurableRunCancelsRiverJobsOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "runless-cancel-owner", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "runless-cancel-other", false)
	require.NoError(t, err)
	_, book, _, reading := seedAnalyzedReading(t, ctx, store, owner, "runless-cancel-book")
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), client, nil, BatchConfig{}, PreparedDeckConfig{}, false)
	canceller := &recordingBatchCanceller{}
	service := &Service{pool: store.Pool(), client: client, store: store, batchCanceller: canceller}
	handle, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, book.ID, reading.SnapshotID)
	require.NoError(t, err)
	require.NotZero(t, handle.JobID)
	require.Empty(t, handle.Preparation.CurrentRunID)

	t.Run("cross-owner cancel is not found", func(t *testing.T) {
		_, cancelErr := service.Cancel(ctx, other.ID, handle.Preparation.ID)
		require.ErrorIs(t, cancelErr, persistence.ErrNotFound)
	})
	jobState := func(t *testing.T) string {
		t.Helper()
		var state string
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT state FROM river_job WHERE id=$1`, handle.JobID).Scan(&state))
		return state
	}
	t.Run("rejected cancel leaves the job live", func(t *testing.T) {
		require.NotEqual(t, "cancelled", jobState(t))
	})

	cancelled, err := service.Cancel(ctx, owner.ID, handle.Preparation.ID)
	require.NoError(t, err)
	t.Run("cancels the preparation", func(t *testing.T) {
		require.Equal(t, domain.DeckPreparationCancelled, cancelled.State)
	})
	t.Run("cancels the River job", func(t *testing.T) {
		require.Equal(t, "cancelled", jobState(t))
	})
	t.Run("does not cancel provider batches", func(t *testing.T) {
		require.Zero(t, canceller.calls)
	})
}

// recordingBatchCanceller counts provider cancellations and cannot delete files.
type recordingBatchCanceller struct{ calls int }

func (c *recordingBatchCanceller) CancelBatch(context.Context, string) (enrichment.Batch, error) {
	c.calls++
	return enrichment.Batch{}, nil
}

// failingCancelRiverClient keeps River insertion real while job cancellation fails.
type failingCancelRiverClient struct{ client *river.Client[pgx.Tx] }

func (c *failingCancelRiverClient) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return c.client.InsertTx(ctx, tx, args, opts)
}

func (*failingCancelRiverClient) JobCancel(context.Context, int64) (*rivertype.JobRow, error) {
	return nil, errors.New("River cancellation unavailable")
}

// failingBatchProvider fails every provider cancellation and file deletion.
type failingBatchProvider struct{ err error }

func (p *failingBatchProvider) CancelBatch(context.Context, string) (enrichment.Batch, error) {
	return enrichment.Batch{}, p.err
}

func (p *failingBatchProvider) DeleteFile(context.Context, string) (enrichment.OpenAIFileDeletion, error) {
	return enrichment.OpenAIFileDeletion{}, p.err
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

// requireConcurrentSubmissions drains every result and error of concurrent
// submissions before asserting anything, so one failure never hides the rest.
// Goroutines never assert; the channels must already be closed.
func requireConcurrentSubmissions(t *testing.T, results <-chan Handle, errs <-chan error, preparationID string, jobID int64, label string) {
	t.Helper()
	var failures []error
	for err := range errs {
		failures = append(failures, err)
	}
	var preparationIDs []string
	var jobIDs []int64
	for result := range results {
		preparationIDs = append(preparationIDs, result.Preparation.ID)
		jobIDs = append(jobIDs, result.JobID)
	}
	require.NoError(t, errors.Join(failures...), label)
	require.Equal(t, slices.Repeat([]string{preparationID}, len(preparationIDs)), preparationIDs, label)
	require.Equal(t, slices.Repeat([]int64{jobID}, len(jobIDs)), jobIDs, label)
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
	source, _, analysisHandle, reading := seedAnalyzedReading(t, ctx, store, owner, "enqueue-failure-book")
	failing := &failingRiverClient{err: errors.New("River unavailable")}
	service := &Service{pool: store.Pool(), client: failing, store: store}
	_, err = service.PrepareCurrentReadingDeck(ctx, owner.ID, reading.BookID, reading.SnapshotID)
	var count int
	t.Run("initial enqueue failure is reported", func(t *testing.T) {
		require.Error(t, err, "expected initial enqueue failure")
	})
	t.Run("failed initial enqueue leaves no preparation", func(t *testing.T) {
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparations WHERE owner_id=$1`, owner.ID).Scan(&count))
		require.Zero(t, count, "initial enqueue left preparations")
	})
	p, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisHandle.RunID, SnapshotID: reading.SnapshotID, Filename: "retry.apkg", DeckName: "retry", ContentHash: "retry-hash"})
	require.NoError(t, err)
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, p.ID)
	require.NoError(t, err)
	_, err = store.FailDeckPreparation(ctx, owner.ID, p.ID, "previous attempt failed")
	require.NoError(t, err)
	retried, err := service.PrepareCurrentReadingDeck(ctx, owner.ID, reading.BookID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, retried.Preparation.State)
	assert.Contains(t, retried.Preparation.Error, "River unavailable")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='prepared_deck' AND details->>'preparation_id'=$2`, owner.ID, p.ID).Scan(&count))
	assert.GreaterOrEqual(t, count, 3, "retry history")
}
