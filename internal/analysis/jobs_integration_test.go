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

	"github.com/google/uuid"
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
		scopedDocuments  []analyzer.SourceDocument
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
		if strings.HasPrefix(req.Document.ID, "epub-unit-v1:") && strings.HasPrefix(req.Document.SourceIdentifier, "OPS/") {
			analyzedChunksMu.Lock()
			scopedDocuments = append(scopedDocuments, req.Document)
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

	units := domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{
		{ID: domain.EPUBUnitID(0, "unit-0"), Order: 0, SpineIndex: 0, ManifestID: "unit-0", Text: "Skip", EndOffset: 4, SourceHref: "skip.xhtml", ResolvedHref: "OPS/skip.xhtml"},
		{ID: domain.EPUBUnitID(1, "unit-1"), Order: 1, SpineIndex: 1, ManifestID: "unit-1", Text: "Keep one", StartOffset: 6, EndOffset: 14, Title: "One", SourceHref: "one.xhtml", ResolvedHref: "OPS/one.xhtml"},
		{ID: domain.EPUBUnitID(2, "unit-2"), Order: 2, SpineIndex: 2, ManifestID: "unit-2", Text: "Keep two", StartOffset: 16, EndOffset: 24, Title: "Two", SourceHref: "two.xhtml", ResolvedHref: "OPS/two.xhtml"},
	}}
	scopedSource, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "scoped-job", Title: "Scoped", MediaType: "application/epub+zip", ContentHash: "sha256:scoped", Content: []byte("Skip\n\nKeep one\n\nKeep two"), FullText: "Skip\n\nKeep one\n\nKeep two"}, units)
	if err != nil {
		t.Fatal(err)
	}
	scopedBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Scoped book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.LinkSourceToBook(ctx, alice.ID, scopedBook.ID, scopedSource.ID); err != nil {
		t.Fatal(err)
	}
	snapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, scopedSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope := domain.EPUBReviewedScopeSnapshot{SchemaVersion: 1, ScopeID: uuid.NewString(), OwnerID: alice.ID, SourceMaterialID: scopedSource.ID, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: 1}, SelectedUnits: []domain.EPUBSelectedUnitReference{{UnitID: domain.EPUBUnitID(1, "unit-1"), Order: 1}, {UnitID: domain.EPUBUnitID(2, "unit-2"), Order: 2}}}
	if _, err = store.CreateEPUBReviewedScope(ctx, scope); err != nil {
		t.Fatal(err)
	}
	scopedHandle, err := service.SubmitScopedAnalysis(ctx, alice.ID, scopedSource.ID, scope.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	retryHandle, err := service.SubmitScopedAnalysis(ctx, alice.ID, scopedSource.ID, scope.ScopeID)
	if err != nil || retryHandle.ID != scopedHandle.ID {
		t.Fatalf("scoped retry = %+v, %v", retryHandle, err)
	}
	if _, err = service.SubmitScopedAnalysis(ctx, bob.ID, scopedSource.ID, scope.ScopeID); err == nil {
		t.Fatalf("cross-owner scoped submit = %v", err)
	}
	scopedStatus, err := service.Wait(ctx, alice.ID, scopedHandle.ID)
	if err != nil || scopedStatus.State != rivertype.JobStateCompleted {
		t.Fatalf("scoped status = %+v, %v", scopedStatus, err)
	}
	analyzedChunksMu.Lock()
	if len(scopedDocuments) != 2 || scopedDocuments[0].ID != domain.EPUBUnitID(1, "unit-1") || scopedDocuments[0].Text != "Keep one" || scopedDocuments[1].ID != domain.EPUBUnitID(2, "unit-2") || scopedDocuments[1].Text != "Keep two" {
		t.Fatalf("scoped analyzer documents = %+v", scopedDocuments)
	}
	analyzedChunksMu.Unlock()
	var scopeID string
	var provenanceCount int
	if err = store.Pool().QueryRow(ctx, `SELECT reviewed_scope_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, alice.ID, scopedStatus.CorpusID).Scan(&scopeID); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM corpus_selected_units WHERE owner_id=$1 AND corpus_id=$2 AND unit_id IN ($3,$4)`, alice.ID, scopedStatus.CorpusID, domain.EPUBUnitID(1, "unit-1"), domain.EPUBUnitID(2, "unit-2")).Scan(&provenanceCount); err != nil {
		t.Fatal(err)
	}
	if scopeID != scope.ScopeID || provenanceCount != 2 {
		t.Fatalf("scope provenance = %q, %d", scopeID, provenanceCount)
	}
	scopedCorpus, err := service.Result(ctx, alice.ID, scopedHandle.ID)
	if err != nil || scopedCorpus.ReviewedScopeID != scope.ScopeID || len(scopedCorpus.SelectedUnits) != 2 || scopedCorpus.SelectedUnits[0].UnitID != domain.EPUBUnitID(1, "unit-1") {
		t.Fatalf("scoped corpus = %+v, %v", scopedCorpus, err)
	}
	if scopedCorpus.Statistics == nil || scopedCorpus.Statistics.AnalyzableTokenCount != 2 || scopedCorpus.Statistics.DistinctLemmaCount != 1 {
		t.Fatalf("scoped corpus statistics = %+v, want selected-unit metrics only", scopedCorpus.Statistics)
	}
	var currentRunID, currentSourceID string
	if err = store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,source_material_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, alice.ID, scopedBook.ID).Scan(&currentRunID, &currentSourceID); err != nil || currentRunID != scopedHandle.RunID || currentSourceID != scopedSource.ID {
		t.Fatalf("current analysis after first completion=(%q,%q) err=%v", currentRunID, currentSourceID, err)
	}
	changedScope := scope
	changedScope.ScopeID = uuid.NewString()
	changedScope.SelectedUnits = changedScope.SelectedUnits[:1]
	if _, err = store.CreateEPUBReviewedScope(ctx, changedScope); err != nil {
		t.Fatal(err)
	}
	changedHandle, err := service.SubmitScopedAnalysis(ctx, alice.ID, scopedSource.ID, changedScope.ScopeID)
	if err != nil || changedHandle.ID == scopedHandle.ID {
		t.Fatalf("changed-scope submit = %+v, %v; original=%+v", changedHandle, err, scopedHandle)
	}
	changedStatus, err := service.Wait(ctx, alice.ID, changedHandle.ID)
	if err != nil || changedStatus.State != rivertype.JobStateCompleted || changedStatus.CorpusID == scopedStatus.CorpusID {
		t.Fatalf("changed-scope status = %+v, %v; original=%+v", changedStatus, err, scopedStatus)
	}
	changedCorpus, err := service.Result(ctx, alice.ID, changedHandle.ID)
	if err != nil || changedCorpus.ReviewedScopeID != changedScope.ScopeID || len(changedCorpus.SelectedUnits) != 1 || changedCorpus.Statistics == nil || changedCorpus.Statistics.AnalyzableTokenCount != 1 {
		t.Fatalf("changed-scope corpus = %+v, %v", changedCorpus, err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, alice.ID, scopedBook.ID).Scan(&currentRunID); err != nil || currentRunID != changedHandle.RunID {
		t.Fatalf("current analysis after replacement=%q err=%v", currentRunID, err)
	}
	analyzedChunksMu.Lock()
	if len(scopedDocuments) != 3 || scopedDocuments[2].ID != domain.EPUBUnitID(1, "unit-1") || scopedDocuments[2].Text != "Keep one" {
		t.Fatalf("changed-scope analyzer documents = %+v", scopedDocuments)
	}
	analyzedChunksMu.Unlock()
	replacement := domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "replacement"), Order: 0, SpineIndex: 0, ManifestID: "replacement", Text: "New", EndOffset: 3}}}
	if _, err = store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: scopedSource.SourceIdentifier, Title: "Scoped", MediaType: scopedSource.MediaType, ContentHash: "sha256:scoped-new", Content: []byte("New"), FullText: "New"}, replacement); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SubmitScopedAnalysis(ctx, alice.ID, scopedSource.ID, scope.ScopeID); err == nil {
		t.Fatal("stale scope was accepted")
	}
	updatedBooks, err := store.ListSourceMaterials(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range updatedBooks {
		if candidate.Source.ID == scopedSource.ID && (candidate.AnalysisRunID != "" || candidate.CorpusID != "") {
			t.Fatalf("changed content exposed stale current analysis: %+v", candidate)
		}
	}
	var historicalScope string
	if err = store.Pool().QueryRow(ctx, `SELECT reviewed_scope_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, alice.ID, scopedCorpus.ID).Scan(&historicalScope); err != nil || historicalScope != scope.ScopeID {
		t.Fatalf("historical corpus changed: %q, %v", historicalScope, err)
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
	if err != nil || legacyCorpus.ReviewedScopeID != "" || len(legacyCorpus.SelectedUnits) != 0 {
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

	// Model a legacy job row missing its old identity. Snapshot-run identity is
	// authoritative, so the immutable revision remains idempotent.
	if _, err = store.Pool().Exec(ctx, `UPDATE analysis_jobs SET analysis_identity=NULL WHERE river_job_id=$1`, handle.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE river_job SET args=args-'analysis_identity' WHERE id=$1`, handle.ID); err != nil {
		t.Fatal(err)
	}
	var legacyDuplicateID int64
	if err = store.Pool().QueryRow(ctx, `SELECT river_job_id FROM analysis_jobs WHERE owner_id=$1 AND reviewed_scope_id IS NULL AND content_hash=$2`, alice.ID, source.ContentHash).Scan(&legacyDuplicateID); err != nil {
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
}
