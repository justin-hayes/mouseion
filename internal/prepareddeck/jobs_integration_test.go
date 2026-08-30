//go:build integration

package prepareddeck

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

func TestServiceEnqueuesOwnerScopedImmutablePreparationAndConfirmsUnreportedJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := store.CreateUser(ctx, "prepared-service-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateUser(ctx, "prepared-service-other", false)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "service-book", Title: "A Book", MediaType: "application/epub+zip", Content: []byte("text"), FullText: "text"}, domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "unit"), Order: 0, SpineIndex: 0, ManifestID: "unit", Text: "text", EndOffset: 4}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := store.CreateEPUBReviewedScope(ctx, domain.EPUBReviewedScopeSnapshot{SchemaVersion: 1, ScopeID: uuid.NewString(), OwnerID: owner.ID, SourceMaterialID: source.ID, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: 1}, Classifier: domain.EPUBClassifierIdentity{Name: "deterministic", Version: "1"}, SelectionMode: domain.EPUBScopeSelectionRecommended, SelectedUnits: []domain.EPUBSelectedUnitReference{{UnitID: domain.EPUBUnitID(0, "unit"), Order: 0}}})
	if err != nil {
		t.Fatal(err)
	}
	analysisRiver, err := analysis.NewClient(store.Pool(), &analyzertest.Fake{}, selection.NewService(store))
	if err != nil {
		t.Fatal(err)
	}
	analysisService := analysis.NewService(store.Pool(), analysisRiver)
	analysisHandle, err := analysisService.SubmitScopedAnalysis(ctx, owner.ID, source.ID, scope.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	artifactHash := "sha256:prepared-analysis-artifact"
	if _, err = store.Pool().Exec(ctx, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version) VALUES($1,'de','1','casefold','1','fake','1')`, artifactHash); err != nil {
		t.Fatal(err)
	}
	var corpusID string
	if err = store.Pool().QueryRow(ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash,reviewed_scope_id,analysis_run_id,status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count) VALUES($1,$2,$3,$4,$5,'complete',0,0,1,1,0,1,1,0) RETURNING id::text`, owner.ID, source.ID, artifactHash, scope.ScopeID, analysisHandle.RunID).Scan(&corpusID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET state='completed',corpus_id=$2,completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$3`, owner.ID, corpusID, analysisHandle.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE analysis_run_attempts SET state='completed',finalized_at=now() WHERE run_id=$1`, analysisHandle.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE analysis_jobs SET corpus_id=$2,progress=100,updated_at=now() WHERE owner_id=$1 AND analysis_run_id=$3`, owner.ID, corpusID, analysisHandle.RunID); err != nil {
		t.Fatal(err)
	}
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	if err != nil {
		t.Fatal(err)
	}
	AddBatchWorker(workers, store, cardexport.NewService(store), client, nil, BatchConfig{}, false)
	service := &Service{pool: store.Pool(), client: &unconfirmedRiverClient{client: client}, store: store}
	analysisID := strconv.FormatInt(analysisHandle.ID, 10)
	handle, err := service.Submit(ctx, owner.ID, analysisID, true)
	if err != nil {
		t.Fatal(err)
	}
	if handle.Preparation.State != domain.DeckPreparationQueued || handle.Preparation.ContentHash != source.ContentHash || handle.JobID == 0 {
		t.Fatalf("handle=%+v", handle)
	}
	var args JobArgs
	job, err := client.JobGet(ctx, handle.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(job.EncodedArgs, &args); err != nil {
		t.Fatal(err)
	}
	if args.OwnerID != owner.ID || args.SourceMaterialID != source.ID || args.ContentHash != source.ContentHash || args.AnalysisRunID != analysisHandle.RunID || !args.ExternalTranslationConsent {
		t.Fatalf("args=%+v", args)
	}
	repeated, err := service.Submit(ctx, owner.ID, analysisID, false)
	if err != nil || repeated.Preparation.ID != handle.Preparation.ID || repeated.JobID != handle.JobID {
		t.Fatalf("duplicate submit=%+v err=%v", repeated, err)
	}
	exact, err := service.GetForAnalysis(ctx, owner.ID, source.ID, analysisHandle.RunID)
	if err != nil || exact.ID != handle.Preparation.ID || exact.AnalysisRunID != analysisHandle.RunID {
		t.Fatalf("exact preparation lookup=%+v err=%v", exact, err)
	}
	if _, err = service.GetForAnalysis(ctx, other.ID, source.ID, analysisHandle.RunID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("cross-owner exact preparation lookup=%v", err)
	}
	if _, err = service.GetForAnalysis(ctx, owner.ID, other.ID, analysisHandle.RunID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("cross-book exact preparation lookup=%v", err)
	}
	var submissions sync.WaitGroup
	results := make(chan Handle, 8)
	errorsCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
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
		t.Fatalf("concurrent submit: %v", submitErr)
	}
	for result := range results {
		if result.Preparation.ID != handle.Preparation.ID || result.JobID != handle.JobID {
			t.Fatalf("concurrent duplicate=%+v want job %d", result, handle.JobID)
		}
	}
	var jobCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'preparation_id'=$2`, (JobArgs{}).Kind(), handle.Preparation.ID).Scan(&jobCount); err != nil || jobCount != 1 {
		t.Fatalf("duplicate jobs=%d err=%v", jobCount, err)
	}
	if _, err = service.Get(ctx, other.ID, handle.Preparation.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("cross-owner get=%v", err)
	}
	cancelled, err := service.Cancel(ctx, owner.ID, handle.Preparation.ID)
	if err != nil || cancelled.State != domain.DeckPreparationCancelled {
		t.Fatalf("cancelled=%+v err=%v", cancelled, err)
	}
	retried, err := service.Retry(ctx, owner.ID, handle.Preparation.ID, false)
	if err != nil || retried.Preparation.State != domain.DeckPreparationQueued || retried.JobID == handle.JobID {
		t.Fatalf("retried=%+v err=%v", retried, err)
	}
	var retries sync.WaitGroup
	retryResults := make(chan Handle, 8)
	retryErrors := make(chan error, 8)
	for i := 0; i < 8; i++ {
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
		t.Fatalf("concurrent retry: %v", retryErr)
	}
	for result := range retryResults {
		if result.Preparation.ID != handle.Preparation.ID || result.JobID != retried.JobID {
			t.Fatalf("concurrent retry=%+v want job %d", result, retried.JobID)
		}
	}
}

func TestServiceReconcilesOrphanedPreparationStates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := store.CreateUser(ctx, "reconcile-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "reconcile-book", Title: "A Book", MediaType: "text/plain", ContentHash: "reconcile-hash", Content: []byte("text"), FullText: "text"})
	if err != nil {
		t.Fatal(err)
	}
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, client)
	queued, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: "queued.apkg", DeckName: "queued", ContentHash: "queued-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := service.Get(ctx, owner.ID, queued.ID); err != nil || got.State != domain.DeckPreparationQueued {
		t.Fatalf("queued reconciliation=%+v err=%v", got, err)
	}
	var queuedJobs int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'preparation_id'=$2 AND state IN ('available','pending','running','retryable','scheduled')`, (JobArgs{}).Kind(), queued.ID).Scan(&queuedJobs); err != nil || queuedJobs != 1 {
		t.Fatalf("re-enqueued queued jobs=%d err=%v", queuedJobs, err)
	}
	preparing, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: "preparing.apkg", DeckName: "preparing", ContentHash: "preparing-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimDeckPreparation(ctx, owner.ID, preparing.ID); err != nil {
		t.Fatal(err)
	}
	got, err := service.Get(ctx, owner.ID, preparing.ID)
	if err != nil || got.State != domain.DeckPreparationFailed || got.Error != orphanedPreparationError {
		t.Fatalf("preparing reconciliation=%+v err=%v", got, err)
	}
	var historyCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='prepared_deck' AND details->>'preparation_id'=$2`, owner.ID, preparing.ID).Scan(&historyCount); err != nil || historyCount == 0 {
		t.Fatalf("reconciliation history=%d err=%v", historyCount, err)
	}
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

func TestServiceEnqueueFailureDoesNotLeaveWaitingPreparation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := store.CreateUser(ctx, "enqueue-failure-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "enqueue-failure-book", Title: "A Book", MediaType: "text/plain", ContentHash: "enqueue-failure-hash", Content: []byte("text"), FullText: "text"})
	if err != nil {
		t.Fatal(err)
	}
	failing := &failingRiverClient{err: errors.New("River unavailable")}
	service := &Service{pool: store.Pool(), client: failing, store: store}
	if _, err = service.Submit(ctx, owner.ID, source.ID, false); err == nil {
		t.Fatal("expected initial enqueue failure")
	}
	var count int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparations WHERE owner_id=$1`, owner.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("initial enqueue left preparations=%d err=%v", count, err)
	}
	p, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: "retry.apkg", DeckName: "retry", ContentHash: "retry-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimDeckPreparation(ctx, owner.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.FailDeckPreparation(ctx, owner.ID, p.ID, "previous attempt failed"); err != nil {
		t.Fatal(err)
	}
	retried, err := service.Retry(ctx, owner.ID, p.ID, false)
	if err != nil || retried.Preparation.State != domain.DeckPreparationFailed || !strings.Contains(retried.Preparation.Error, "River unavailable") {
		t.Fatalf("retry enqueue failure=%+v err=%v", retried, err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='prepared_deck' AND details->>'preparation_id'=$2`, owner.ID, p.ID).Scan(&count); err != nil || count < 3 {
		t.Fatalf("retry history=%d err=%v", count, err)
	}
}
