//go:build integration

package analysis

import (
	"context"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRiverAnalysisPersistsNormalizedCorpus(t *testing.T) {
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
	owner, err := store.CreateUser(ctx, "corpus-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	source, err := putAnalysisSource(ctx, store, owner.ID, "corpus-source", "Corpus", "Hallo Berlin.", "sha256:corpus")
	if err != nil {
		t.Fatal(err)
	}
	namedEntity := "LOC"
	unitID := domain.EPUBUnitID(0, "corpus-source")
	tokens := []analyzer.Token{
		{Surface: "Hallo", RawLemma: "hallo", CanonicalLemma: "hallo", UPOS: "INTJ", Morphology: map[string]string{"Polite": "No"}, Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 0, EndOffset: 5}},
		{Surface: "Berlin", RawLemma: "Berlin", CanonicalLemma: "berlin", UPOS: "PROPN", Morphology: map[string]string{"Case": "Nom"}, NamedEntity: &namedEntity, Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 6, EndOffset: 12}},
	}
	for i := 0; i <= normalizedCorpusTokenInsertBatchSize; i++ {
		tokens = append(tokens, analyzer.Token{Surface: "x", RawLemma: "x", CanonicalLemma: "x", UPOS: "X", Morphology: map[string]string{}, Location: analyzer.SourceLocation{SourceDocumentID: unitID}})
	}
	fake := &analyzertest.Fake{AnalyzeFunc: func(_ context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{
			SchemaVersion: "1.0.0",
			Language:      req.Language,
			Sentences: []analyzer.Sentence{
				{
					Text:     "Hallo Berlin.",
					Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 0, EndOffset: 13},
					Tokens:   tokens,
				},
				{Text: "", Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 13, EndOffset: 13}},
			},
			Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "fixture", AnalyzerVersion: "1"},
			NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"},
		}, nil
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
	status, err := service.Wait(ctx, owner.ID, handle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != rivertype.JobStateCompleted {
		t.Fatalf("status = %+v", status)
	}
	corpus, err := service.Result(ctx, owner.ID, handle.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sentenceCount, tokenCount int64
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM corpus_sentences WHERE owner_id=$1 AND corpus_id=$2`, owner.ID, corpus.ID).Scan(&sentenceCount); err != nil {
		t.Fatal(err)
	}
	if sentenceCount != 2 {
		t.Fatalf("sentence count = %d, want 2", sentenceCount)
	}
	var emptySentenceCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM corpus_sentences WHERE owner_id=$1 AND corpus_id=$2 AND sentence_text='' AND start_offset=13 AND end_offset=13`, owner.ID, corpus.ID).Scan(&emptySentenceCount); err != nil {
		t.Fatal(err)
	}
	if emptySentenceCount != 1 {
		t.Fatalf("empty sentence count = %d, want 1", emptySentenceCount)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2`, owner.ID, corpus.ID).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if tokenCount != int64(len(tokens)) {
		t.Fatalf("token count = %d, want %d", tokenCount, len(tokens))
	}
	if corpus.Statistics == nil || corpus.Statistics.TextProfile == nil || corpus.Statistics.TextProfile.SentenceCount != sentenceCount || corpus.Statistics.TextProfile.EmptySentenceCount != 1 {
		t.Fatalf("text profile = %+v, want two sentences and one empty sentence", corpus.Statistics)
	}
	var surface, rawLemma, canonicalLemma, upos, morphology, storedUnitID string
	var namedEntityValue *string
	var sentenceOrdinal, tokenOrdinal, startOffset, endOffset int64
	if err = pool.QueryRow(ctx, `SELECT s.unit_id,t.surface,t.raw_lemma,t.canonical_lemma,t.upos,t.morphology::text,t.named_entity,s.sentence_ordinal,t.token_ordinal,t.start_offset,t.end_offset
		FROM corpus_tokens t JOIN corpus_sentences s ON s.owner_id=t.owner_id AND s.corpus_id=t.corpus_id AND s.analysis_run_id=t.analysis_run_id AND s.sentence_ordinal=t.sentence_ordinal
		WHERE t.owner_id=$1 AND t.corpus_id=$2 AND t.canonical_lemma='berlin'`, owner.ID, corpus.ID).Scan(&storedUnitID, &surface, &rawLemma, &canonicalLemma, &upos, &morphology, &namedEntityValue, &sentenceOrdinal, &tokenOrdinal, &startOffset, &endOffset); err != nil {
		t.Fatal(err)
	}
	if storedUnitID != unitID || surface != "Berlin" || rawLemma != "Berlin" || canonicalLemma != "berlin" || upos != "PROPN" || namedEntityValue == nil || *namedEntityValue != namedEntity || sentenceOrdinal != 0 || tokenOrdinal != 1 || startOffset != 6 || endOffset != 12 {
		t.Fatalf("stored token = unit=%q surface=%q raw=%q canonical=%q upos=%q morphology=%q entity=%v sentence=%d token=%d offsets=%d:%d", storedUnitID, surface, rawLemma, canonicalLemma, upos, morphology, namedEntityValue, sentenceOrdinal, tokenOrdinal, startOffset, endOffset)
	}
	assert.JSONEq(t, `{"Case":"Nom"}`, morphology)
	var functionSurface, functionRawLemma, functionCanonicalLemma, functionUPOS, functionMorphology string
	if err = pool.QueryRow(ctx, `SELECT surface,raw_lemma,canonical_lemma,upos,morphology::text FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND canonical_lemma='hallo'`, owner.ID, corpus.ID).Scan(&functionSurface, &functionRawLemma, &functionCanonicalLemma, &functionUPOS, &functionMorphology); err != nil {
		t.Fatal(err)
	}
	if functionSurface != "Hallo" || functionRawLemma != "hallo" || functionCanonicalLemma != "hallo" || functionUPOS != "INTJ" {
		t.Fatalf("stored function token = surface=%q raw=%q canonical=%q upos=%q", functionSurface, functionRawLemma, functionCanonicalLemma, functionUPOS)
	}
	assert.JSONEq(t, `{"Polite":"No"}`, functionMorphology)
}

func TestRiverAnalysisLifecycleDedupAndOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, MigrateRiver(ctx, pool))
	alice, _ := store.CreateUser(ctx, "jobs-alice", false)
	bob, _ := store.CreateUser(ctx, "jobs-bob", false)
	fullText := strings.Repeat("Häuser. ", 13_000)
	source, err := putAnalysisSource(ctx, store, alice.ID, "job-source", "Job", fullText, "sha256:job-success")
	require.NoError(t, err)
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
		return analyzer.Result{SchemaVersion: "1.0.0", Language: req.Language, Analysis: analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"}, NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"}, Sentences: []analyzer.Sentence{{Text: req.Document.Text, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: uint64(len([]rune(req.Document.Text)))}, Tokens: []analyzer.Token{{Surface: "Häuser", RawLemma: "Häuser", CanonicalLemma: "haus", UPOS: "NOUN", Morphology: map[string]string{"Number": "Plur"}, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: 6}}}}}}, nil
	}}
	client, err := NewClient(store.Pool(), fake, selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	defer client.Stop(context.Background())
	service := NewService(store.Pool(), client)
	bobSource, err := putAnalysisSource(ctx, store, bob.ID, "bob-job-source", "Bob Job", "Haus.", "sha256:bob-job")
	require.NoError(t, err)
	bobHandle, err := service.SubmitAnalysis(ctx, bob.ID, bobSource.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), bobHandle.DisplayNumber, "Bob's first display number = %d, want 1", bobHandle.DisplayNumber)
	handle, err := service.SubmitAnalysis(ctx, alice.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), handle.DisplayNumber, "Alice's first display number = %d, want 1 (River ID %d)", handle.DisplayNumber, handle.ID)
	assert.NotEqual(t, handle.DisplayNumber, handle.ID, "River ID %d unexpectedly matches display number; regression setup did not decouple sequences", handle.ID)
	duplicate, err := service.SubmitAnalysis(ctx, alice.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, handle.ID, duplicate.ID, "dedup IDs differ")
	assert.Equal(t, handle.DisplayNumber, duplicate.DisplayNumber, "dedup display numbers differ")
	_, err = service.Get(ctx, bob.ID, handle.ID)
	assert.ErrorIs(t, err, ErrNotFound, "cross-owner get")
	_, err = service.Cancel(ctx, bob.ID, handle.ID)
	assert.ErrorIs(t, err, ErrNotFound, "cross-owner cancel")
	status, err := service.Wait(ctx, alice.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, status.State)
	assert.Equal(t, 100, status.Progress)
	analyzedChunksMu.Lock()
	require.Len(t, analyzedChunks, 2, "Analyze() call count = %d, want 2", len(analyzedChunks))
	for i, chunk := range analyzedChunks {
		assert.LessOrEqual(t, len([]rune(chunk)), DefaultMaxChunkChars, "analyzed chunk %d length = %d, exceeds %d", i, len([]rune(chunk)), DefaultMaxChunkChars)
	}
	assert.Equal(t, fullText, strings.Join(analyzedChunks, ""), "analyzed chunks do not reconstruct source text")
	analyzedChunksMu.Unlock()
	corpus, err := service.Result(ctx, alice.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, alice.ID, corpus.OwnerID)
	assert.NotEqual(t, source.ContentHash, corpus.ArtifactHash)
	require.NotNil(t, corpus.Statistics)
	assert.Equal(t, int64(2), corpus.Statistics.AnalyzableTokenCount)
	assert.Equal(t, int64(1), corpus.Statistics.DistinctLemmaCount)
	require.NotNil(t, corpus.Statistics.TextProfile)
	assert.Equal(t, int64(2), corpus.Statistics.TextProfile.SentenceCount)
	assert.Equal(t, float64(1), corpus.Statistics.TextProfile.MedianSentenceTokenCount)
	var candidateCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2`, alice.ID, corpus.ID).Scan(&candidateCount))
	assert.Equal(t, 1, candidateCount)
	var bobCandidateCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, bob.ID).Scan(&bobCandidateCount))
	assert.Equal(t, 1, bobCandidateCount)
	_, err = service.Result(ctx, bob.ID, handle.ID)
	assert.ErrorIs(t, err, ErrNotFound, "cross-owner result")

	legacyEPUB, err := putAnalysisSource(ctx, store, alice.ID, "legacy-epub-job", "Legacy EPUB", "Legacy complete text.", "sha256:legacy-epub-job")
	require.NoError(t, err)
	legacyHandle, err := service.SubmitAnalysis(ctx, alice.ID, legacyEPUB.ID)
	require.NoError(t, err)
	legacyStatus, err := service.Wait(ctx, alice.ID, legacyHandle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, legacyStatus.State)
	legacyCorpus, err := service.Result(ctx, alice.ID, legacyHandle.ID)
	require.NoError(t, err, "legacy EPUB corpus = %+v, %v", legacyCorpus, err)

	failing, err := putAnalysisSource(ctx, store, alice.ID, "job-fail", "Fail", "fail", "sha256:job-fail")
	require.NoError(t, err)
	failureHandle, err := service.SubmitAnalysis(ctx, alice.ID, failing.ID)
	require.NoError(t, err)
	failure, err := service.Wait(ctx, alice.ID, failureHandle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateDiscarded, failure.State)
	assert.NotEmpty(t, failure.Error)
	failureRetry, err := service.SubmitAnalysis(ctx, alice.ID, failing.ID)
	require.NoError(t, err)
	assert.Equal(t, failureHandle.ID, failureRetry.ID)
	assert.NotEqual(t, failureHandle.JobID, failureRetry.JobID)
	failureRetried, err := service.Wait(ctx, alice.ID, failureRetry.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateDiscarded, failureRetried.State)
	assert.Equal(t, 2, failureRetried.AttemptCount)

	cancelSource, err := putAnalysisSource(ctx, store, alice.ID, "job-cancel", "Cancel", "cancel", "sha256:job-cancel")
	require.NoError(t, err)
	cancelHandle, err := service.SubmitAnalysis(ctx, alice.ID, cancelSource.ID)
	require.NoError(t, err)
	cancelled, err := service.Cancel(ctx, alice.ID, cancelHandle.ID)
	require.NoError(t, err)
	if cancelled.State != rivertype.JobStateCancelled {
		cancelled, err = service.Wait(ctx, alice.ID, cancelHandle.ID)
		require.NoError(t, err)
	}
	assert.Equal(t, rivertype.JobStateCancelled, cancelled.State)
	cancelRetry, err := service.SubmitAnalysis(ctx, alice.ID, cancelSource.ID)
	require.NoError(t, err)
	assert.Equal(t, cancelHandle.ID, cancelRetry.ID)
	assert.NotEqual(t, cancelHandle.JobID, cancelRetry.JobID)
	_, err = service.Cancel(ctx, alice.ID, cancelRetry.ID)
	require.NoError(t, err)

	// Model a legacy job row missing its old identity. Snapshot-run identity is
	// authoritative, so the immutable revision remains idempotent.
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_jobs SET analysis_identity=NULL WHERE river_job_id=$1`, handle.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE river_job SET args=args-'analysis_identity' WHERE id=$1`, handle.ID)
	require.NoError(t, err)
	var legacyDuplicateID int64
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT river_job_id FROM analysis_jobs WHERE owner_id=$1 AND content_hash=$2`, alice.ID, source.ContentHash).Scan(&legacyDuplicateID))
	assert.Equal(t, handle.ID, legacyDuplicateID, "legacy duplicate lookup")
	current, err := service.SubmitAnalysis(ctx, alice.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, handle.ID, current.ID, "current analysis did not reuse snapshot run")
	currentStatus, err := service.Wait(ctx, alice.ID, current.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, currentStatus.State)
	updated, err := putAnalysisSource(ctx, store, alice.ID, "job-source", "Job updated", "updated text", "sha256:job-success-updated")
	require.NoError(t, err)
	assert.Equal(t, source.ID, updated.ID)
	assert.NotEqual(t, source.ContentRevisionID, updated.ContentRevisionID)
	changed, err := service.SubmitAnalysis(ctx, alice.ID, updated.ID)
	require.NoError(t, err)
	assert.NotEqual(t, handle.ID, changed.ID, "content revision reused completed analysis handle %d", handle.ID)
	changedStatus, err := service.Wait(ctx, alice.ID, changed.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, changedStatus.State)
}

func TestAnalysisRunSurvivesJourneyRemovalAndReAdd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "journey-lifecycle", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Journey lifecycle", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner.ID, "journey-lifecycle", "Journey lifecycle", "Haus.", "sha256:journey-lifecycle")
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, owner.ID, "de", book.ID)
	require.NoError(t, err)
	journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
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
		return analyzer.Result{SchemaVersion: "1.0.0", Language: req.Language, Analysis: analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"}, NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"}, Sentences: []analyzer.Sentence{{Text: req.Document.Text, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: uint64(len([]rune(req.Document.Text)))}, Tokens: []analyzer.Token{{Surface: "Haus", RawLemma: "Haus", CanonicalLemma: "haus", UPOS: "NOUN", Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: 4}}}}}}, nil
	}}
	client, err := NewClient(store.Pool(), fake, selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	defer client.Stop(context.Background())
	service := NewService(store.Pool(), client)
	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	select {
	case <-started:
	case <-ctx.Done():
		require.Fail(t, "analysis did not reach running state")
	}
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.RemoveFromReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
	goal, goalErr := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, goalErr)
	assert.Empty(t, goal.BookID, "Goal after Journey removal")
	var state string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID).Scan(&state))
	assert.Equal(t, "running", state, "analysis state after Journey removal")
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
	reused, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, handle.ID, reused.ID)
	assert.Equal(t, handle.RunID, reused.RunID)
	releaseOnce.Do(func() { close(release) })
	status, err := service.Wait(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, status.State)
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.RemoveFromReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
	reusedCompleted, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, handle.ID, reusedCompleted.ID)
	var runs, jobs int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&runs))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&jobs))
	assert.Equal(t, 1, runs)
	assert.Equal(t, 1, jobs)
	releaseOnce.Do(func() { close(release) })
}
