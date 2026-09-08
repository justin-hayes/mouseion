//go:build integration

package analysis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river/rivertype"
)

func putAnalysisSource(ctx context.Context, store *persistence.PostgresStore, owner, identifier, title, text, hash string) (domain.SourceMaterial, error) {
	return store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner, Language: "de", SourceIdentifier: identifier, Title: title,
		MediaType: "application/epub+zip", ContentHash: hash, Content: []byte(text), FullText: text,
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{{
		ID: domain.EPUBUnitID(0, identifier), Order: 0, SpineIndex: 0, ManifestID: identifier,
		SourceHref: identifier, ResolvedHref: identifier, Text: text,
		EndOffset: uint64(len([]rune(text))), MediaType: "application/xhtml+xml", Linear: true,
	}}})
}

func TestRiverAnalysisLifecycleDedupAndOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = MigrateRiver(ctx, pool); err != nil {
		t.Fatal(err)
	}
	alice, _ := store.CreateUser(ctx, "jobs-alice", false)
	bob, _ := store.CreateUser(ctx, "jobs-bob", false)
	fullText := strings.Repeat("Häuser. ", 13_000)
	source, err := putAnalysisSource(ctx, store, alice.ID, "job-source", "Job", fullText, "sha256:job-success")
	if err != nil {
		t.Fatal(err)
	}
	var (
		analyzedChunksMu sync.Mutex
		analyzedChunks   []string
	)
	fake := &analyzertest.Fake{AnalyzeFunc: func(analyzeCtx context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
		if req.Document.Text == "fail" {
			return analyzer.Result{}, fmt.Errorf("stanza failed")
		}
		if req.Document.Text == "cancel" {
			<-analyzeCtx.Done()
			return analyzer.Result{}, analyzeCtx.Err()
		}
		if req.Document.SourceIdentifier == "job-source" {
			analyzedChunksMu.Lock()
			analyzedChunks = append(analyzedChunks, req.Document.Text)
			analyzedChunksMu.Unlock()
		}
		return analyzer.Result{SchemaVersion: "1.0.0", Language: req.Language, Analysis: analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"}, NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"}, Sentences: []analyzer.Sentence{{Tokens: []analyzer.Token{{CanonicalLemma: "haus", UPOS: "NOUN", Morphology: map[string]string{"Number": "Plur"}}}}}}, nil
	}}
	client, err := NewClient(store.Pool(), fake, selection.NewService(store))
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	service := NewService(store.Pool(), client)
	bobSource, err := putAnalysisSource(ctx, store, bob.ID, "bob-job-source", "Bob Job", "Haus.", "sha256:bob-job")
	if err != nil {
		t.Fatal(err)
	}
	bobHandle, err := service.SubmitAnalysis(ctx, bob.ID, bobSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bobHandle.DisplayNumber != 1 {
		t.Fatalf("Bob's first display number = %d, want 1", bobHandle.DisplayNumber)
	}
	handle, err := service.SubmitAnalysis(ctx, alice.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if handle.DisplayNumber != 1 {
		t.Fatalf("Alice's first display number = %d, want 1 (River ID %d)", handle.DisplayNumber, handle.ID)
	}
	if handle.ID == handle.DisplayNumber {
		t.Fatalf("River ID %d unexpectedly matches display number; regression setup did not decouple sequences", handle.ID)
	}
	duplicate, err := service.SubmitAnalysis(ctx, alice.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != handle.ID {
		t.Fatalf("dedup IDs differ: %d != %d", duplicate.ID, handle.ID)
	}
	if duplicate.DisplayNumber != handle.DisplayNumber {
		t.Fatalf("dedup display numbers differ: %d != %d", duplicate.DisplayNumber, handle.DisplayNumber)
	}
	if _, err = service.Get(ctx, bob.ID, handle.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner get = %v", err)
	}
	if _, err = service.Cancel(ctx, bob.ID, handle.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner cancel = %v", err)
	}
	status, err := service.Wait(ctx, alice.ID, handle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != rivertype.JobStateCompleted || status.Progress != 100 {
		t.Fatalf("status = %+v", status)
	}
	analyzedChunksMu.Lock()
	if len(analyzedChunks) != 2 {
		t.Fatalf("Analyze() call count = %d, want 2", len(analyzedChunks))
	}
	for i, chunk := range analyzedChunks {
		if len([]rune(chunk)) > DefaultMaxChunkChars {
			t.Fatalf("analyzed chunk %d length = %d, exceeds %d", i, len([]rune(chunk)), DefaultMaxChunkChars)
		}
	}
	if got := strings.Join(analyzedChunks, ""); got != fullText {
		t.Fatal("analyzed chunks do not reconstruct source text")
	}
	analyzedChunksMu.Unlock()
	corpus, err := service.Result(ctx, alice.ID, handle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if corpus.OwnerID != alice.ID || corpus.ArtifactHash == source.ContentHash {
		t.Fatalf("corpus = %+v", corpus)
	}
	if corpus.Statistics == nil || corpus.Statistics.AnalyzableTokenCount != 2 || corpus.Statistics.DistinctLemmaCount != 1 {
		t.Fatalf("corpus statistics = %+v, want 2 analyzable tokens and 1 distinct lemma", corpus.Statistics)
	}
	if corpus.Statistics.TextProfile == nil || corpus.Statistics.TextProfile.SentenceCount != 2 || corpus.Statistics.TextProfile.MedianSentenceTokenCount != 1 {
		t.Fatalf("text profile = %+v, want two one-token sentences", corpus.Statistics.TextProfile)
	}
	var candidateCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2`, alice.ID, corpus.ID).Scan(&candidateCount); err != nil {
		t.Fatal(err)
	}
	if candidateCount != 1 {
		t.Fatalf("candidates = %d", candidateCount)
	}
	var bobCandidateCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, bob.ID).Scan(&bobCandidateCount); err != nil {
		t.Fatal(err)
	}
	if bobCandidateCount != 1 {
		t.Fatalf("bob candidates = %d, want 1", bobCandidateCount)
	}
	if _, err = service.Result(ctx, bob.ID, handle.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner result = %v", err)
	}

	legacyEPUB, err := putAnalysisSource(ctx, store, alice.ID, "legacy-epub-job", "Legacy EPUB", "Legacy complete text.", "sha256:legacy-epub-job")
	if err != nil {
		t.Fatal(err)
	}
	legacyHandle, err := service.SubmitAnalysis(ctx, alice.ID, legacyEPUB.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacyStatus, err := service.Wait(ctx, alice.ID, legacyHandle.ID)
	if err != nil || legacyStatus.State != rivertype.JobStateCompleted {
		t.Fatalf("legacy EPUB status = %+v, %v", legacyStatus, err)
	}
	legacyCorpus, err := service.Result(ctx, alice.ID, legacyHandle.ID)
	if err != nil {
		t.Fatalf("legacy EPUB corpus = %+v, %v", legacyCorpus, err)
	}

	failing, err := putAnalysisSource(ctx, store, alice.ID, "job-fail", "Fail", "fail", "sha256:job-fail")
	if err != nil {
		t.Fatal(err)
	}
	failureHandle, err := service.SubmitAnalysis(ctx, alice.ID, failing.ID)
	if err != nil {
		t.Fatal(err)
	}
	failure, err := service.Wait(ctx, alice.ID, failureHandle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failure.State != rivertype.JobStateDiscarded || failure.Error == "" {
		t.Fatalf("failure status = %+v", failure)
	}
	failureRetry, err := service.SubmitAnalysis(ctx, alice.ID, failing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failureRetry.ID != failureHandle.ID || failureRetry.JobID == failureHandle.JobID {
		t.Fatalf("failed analysis retry handles = first=%+v retry=%+v", failureHandle, failureRetry)
	}
	failureRetried, err := service.Wait(ctx, alice.ID, failureRetry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failureRetried.State != rivertype.JobStateDiscarded || failureRetried.AttemptCount != 2 {
		t.Fatalf("failed analysis retry status = %+v", failureRetried)
	}

	cancelSource, err := putAnalysisSource(ctx, store, alice.ID, "job-cancel", "Cancel", "cancel", "sha256:job-cancel")
	if err != nil {
		t.Fatal(err)
	}
	cancelHandle, err := service.SubmitAnalysis(ctx, alice.ID, cancelSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := service.Cancel(ctx, alice.ID, cancelHandle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != rivertype.JobStateCancelled {
		cancelled, err = service.Wait(ctx, alice.ID, cancelHandle.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if cancelled.State != rivertype.JobStateCancelled {
		t.Fatalf("cancelled status = %+v", cancelled)
	}
	cancelRetry, err := service.SubmitAnalysis(ctx, alice.ID, cancelSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelRetry.ID != cancelHandle.ID || cancelRetry.JobID == cancelHandle.JobID {
		t.Fatalf("cancelled analysis retry handles = first=%+v retry=%+v", cancelHandle, cancelRetry)
	}
	if _, err = service.Cancel(ctx, alice.ID, cancelRetry.ID); err != nil {
		t.Fatal(err)
	}

	// Model a legacy job row missing its old identity. Snapshot-run identity is
	// authoritative, so the immutable revision remains idempotent.
	if _, err = store.Pool().Exec(ctx, `UPDATE analysis_jobs SET analysis_identity=NULL WHERE river_job_id=$1`, handle.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE river_job SET args=args-'analysis_identity' WHERE id=$1`, handle.ID); err != nil {
		t.Fatal(err)
	}
	var legacyDuplicateID int64
	if err = store.Pool().QueryRow(ctx, `SELECT river_job_id FROM analysis_jobs WHERE owner_id=$1 AND content_hash=$2`, alice.ID, source.ContentHash).Scan(&legacyDuplicateID); err != nil {
		t.Fatal(err)
	}
	if legacyDuplicateID != handle.ID {
		t.Fatalf("legacy duplicate lookup = %d, want stale job %d", legacyDuplicateID, handle.ID)
	}
	current, err := service.SubmitAnalysis(ctx, alice.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != handle.ID {
		t.Fatalf("current analysis did not reuse snapshot run: %d != %d", current.ID, handle.ID)
	}
	currentStatus, err := service.Wait(ctx, alice.ID, current.ID)
	if err != nil || currentStatus.State != rivertype.JobStateCompleted {
		t.Fatalf("current analysis status = %+v, %v", currentStatus, err)
	}
	updated, err := putAnalysisSource(ctx, store, alice.ID, "job-source", "Job updated", "updated text", "sha256:job-success-updated")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != source.ID || updated.ContentRevisionID == source.ContentRevisionID {
		t.Fatalf("updated source revision = %+v, original=%+v", updated, source)
	}
	changed, err := service.SubmitAnalysis(ctx, alice.ID, updated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID == handle.ID {
		t.Fatalf("content revision reused completed analysis handle %d", handle.ID)
	}
	changedStatus, err := service.Wait(ctx, alice.ID, changed.ID)
	if err != nil || changedStatus.State != rivertype.JobStateCompleted {
		t.Fatalf("changed content analysis status = %+v, %v", changedStatus, err)
	}
}

func TestAnalysisRunSurvivesJourneyRemovalAndReAdd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = MigrateRiver(ctx, pool); err != nil {
		t.Fatal(err)
	}
	owner, err := store.CreateUser(ctx, "journey-lifecycle", false)
	if err != nil {
		t.Fatal(err)
	}
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Journey lifecycle", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := putAnalysisSource(ctx, store, owner.ID, "journey-lifecycle", "Journey lifecycle", "Haus.", "sha256:journey-lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, owner.ID, "de", book.ID); err != nil {
		t.Fatal(err)
	}
	journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce, releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	fake := &analyzertest.Fake{AnalyzeFunc: func(analyzeCtx context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
		startOnce.Do(func() { close(started) })
		select {
		case <-release:
		case <-analyzeCtx.Done():
			return analyzer.Result{}, analyzeCtx.Err()
		}
		return analyzer.Result{SchemaVersion: "1.0.0", Language: req.Language, Analysis: analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"}, NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"}, Sentences: []analyzer.Sentence{{Tokens: []analyzer.Token{{CanonicalLemma: "haus", UPOS: "NOUN"}}}}}, nil
	}}
	client, err := NewClient(store.Pool(), fake, selection.NewService(store))
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	service := NewService(store.Pool(), client)
	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("analysis did not reach running state")
	}
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RemoveFromReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision); err != nil {
		t.Fatal(err)
	}
	if goal, goalErr := store.GetPrimaryGoal(ctx, owner.ID, "de"); goalErr != nil || goal.BookID != "" {
		t.Fatalf("Goal after Journey removal=%+v err=%v", goal, goalErr)
	}
	var state string
	if err = store.Pool().QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "running" {
		t.Fatalf("analysis state after Journey removal=%q, want running", state)
	}
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision); err != nil {
		t.Fatal(err)
	}
	reused, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reused.ID != handle.ID || reused.RunID != handle.RunID {
		t.Fatalf("re-added running analysis=%+v, original=%+v", reused, handle)
	}
	releaseOnce.Do(func() { close(release) })
	status, err := service.Wait(ctx, owner.ID, handle.ID)
	if err != nil || status.State != rivertype.JobStateCompleted {
		t.Fatalf("re-added analysis status=%+v err=%v", status, err)
	}
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RemoveFromReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision); err != nil {
		t.Fatal(err)
	}
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision); err != nil {
		t.Fatal(err)
	}
	reusedCompleted, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reusedCompleted.ID != handle.ID {
		t.Fatalf("re-added completed analysis=%+v, original=%+v", reusedCompleted, handle)
	}
	var runs, jobs int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 || jobs != 1 {
		t.Fatalf("analysis lifecycle created duplicates: runs=%d jobs=%d", runs, jobs)
	}
	releaseOnce.Do(func() { close(release) })
}
