//go:build integration

package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
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

func putAnalysisSource(ctx context.Context, store *persistence.PostgresStore, owner, identifier, title, text, hash string) (domain.SourceMaterial, error) {
	return putAnalysisSourceWithUnits(ctx, store, owner, identifier, title, text, hash, []domain.ExtractedUnit{{
		ID: domain.EPUBUnitID(0, identifier), Order: 0, SpineIndex: 0, ManifestID: identifier,
		SourceHref: identifier, ResolvedHref: identifier, Text: text,
		EndOffset: uint64(len([]rune(text))), MediaType: "application/xhtml+xml", Linear: true,
	}})
}

func putAnalysisSourceWithUnits(ctx context.Context, store *persistence.PostgresStore, owner, identifier, title, text, hash string, units []domain.ExtractedUnit) (domain.SourceMaterial, error) {
	return store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner, Language: "de", SourceIdentifier: identifier, Title: title,
		MediaType: "application/epub+zip", ContentHash: hash, Content: []byte(text), FullText: text,
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: units})
}

func countingTagAnalyzer(calls *int) *analyzertest.Fake {
	return &analyzertest.Fake{AnalyzeFunc: func(_ context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
		*calls = *calls + 1
		length := uint64(len([]rune(req.Document.Text)))
		return analyzer.Result{SchemaVersion: "1.0.0", Language: req.Language,
			Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"},
			NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"},
			Sentences:            []analyzer.Sentence{{Text: req.Document.Text, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: length}, Tokens: []analyzer.Token{{Surface: "Tag", RawLemma: "Tag", CanonicalLemma: "tag", UPOS: "NOUN", Dependency: "root", Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: 3}}}}}}, nil
	}}
}

func makeCurrentAnalyzedBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner string) (domain.Book, domain.SourceMaterial) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: "Current retry guard", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner, "current-retry-guard", book.Title, "Guten Tag.", "sha256:current-retry-guard")
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner, book.ID, source.ID))
	require.NoError(t, store.SetBookDisposition(ctx, owner, book.ID, domain.BookDispositionToRead))
	require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{
		ContentHash: source.ContentHash, Language: source.Language, SchemaVersion: "1",
		NormalizationProfile: source.Language, NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1",
	}, nil))
	var snapshotID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, source.ID).Scan(&snapshotID))
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'test','1',$5,'completed',now()) RETURNING id::text`, owner, source.ID, source.ContentRevisionID, snapshotID, source.ID).Scan(&runID))
	corpus, err := store.PutCorpus(ctx, owner, source.ID, source.ContentHash)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, runID, owner, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, owner, runID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner, book.ID, source.ID, runID)
	require.NoError(t, err)
	return book, source
}

func TestSubmitToReadBookAnalysisRejectsCurrentBookBeforeOperationalWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, store.Pool()))
	owner, err := store.CreateUser(ctx, "current-refresh-guard", false)
	require.NoError(t, err)
	book, source := makeCurrentAnalyzedBook(t, ctx, store, owner.ID)

	capabilities := analyzertest.CapabilityProvider{Value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{
		Language: "de", SupportedFeatures: []string{"tokenize", "pos", "lemma", "depparse"}, Ready: true,
	}}}}
	client, err := NewClient(store.Pool(), &analyzertest.Fake{}, capabilities, selection.NewService(store))
	require.NoError(t, err)
	service := NewService(store.Pool(), client)
	accepted, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	_, err = service.SubmitToReadBookAnalysis(ctx, owner.ID, book.ID, source.ID)
	require.ErrorIs(t, err, ErrCurrentBook)

	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, current.BookID)
	assert.NotEmpty(t, current.SnapshotID)
	var runCount, historyCount, jobCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, owner.ID).Scan(&runCount))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='analysis'`, owner.ID).Scan(&historyCount))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1`, owner.ID).Scan(&jobCount))
	assert.Equal(t, 2, runCount, "rejected refresh created no analysis run")
	assert.Zero(t, historyCount, "rejected refresh wrote no operational history")
	assert.Equal(t, 1, jobCount, "the previously accepted job remains independent")
	var acceptedState string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner.ID, accepted.RunID).Scan(&acceptedState))
	assert.Equal(t, "queued", acceptedState, "the accepted job remains queued")

	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, current.SnapshotID))
	transition, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		rollbackErr := transition.Rollback(context.Background())
		if !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			require.NoError(t, rollbackErr)
		}
	})
	var lockedBook string
	require.NoError(t, transition.QueryRow(ctx, `SELECT id::text FROM books WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner.ID, book.ID).Scan(&lockedBook))
	racedSubmission := make(chan error, 1)
	go func() {
		_, submitErr := service.SubmitToReadBookAnalysis(ctx, owner.ID, book.ID, source.ID)
		racedSubmission <- submitErr
	}()
	require.Eventually(t, func() bool {
		var waiting int
		queryErr := store.Pool().QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE 'SELECT id::text FROM books%'`).Scan(&waiting)
		return queryErr == nil && waiting > 0
	}, 5*time.Second, 10*time.Millisecond, "guarded submission should wait for the concurrent current-reading transition")
	_, err = transition.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id,snapshot_id) VALUES($1,'de',$2,$3)`, owner.ID, book.ID, current.SnapshotID)
	require.NoError(t, err)
	require.NoError(t, transition.Commit(ctx))
	require.ErrorIs(t, <-racedSubmission, ErrCurrentBook, "submission must observe the current state after acquiring the shared Book lock")
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, current.SnapshotID))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, owner.ID).Scan(&runCount))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='analysis'`, owner.ID).Scan(&historyCount))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1`, owner.ID).Scan(&jobCount))
	assert.Equal(t, 2, runCount, "racing refresh created no analysis run")
	assert.Zero(t, historyCount, "racing refresh wrote no operational history")
	assert.Equal(t, 1, jobCount, "racing refresh did not cancel the previously accepted job")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner.ID, accepted.RunID).Scan(&acceptedState))
	assert.Equal(t, "queued", acceptedState, "the accepted job remains queued after the race")

	handle, err := service.SubmitToReadBookAnalysis(ctx, owner.ID, book.ID, source.ID)
	require.NoError(t, err, "non-current To Read recovery remains permitted")
	assert.Equal(t, accepted, handle, "permitted recovery reuses the already accepted queued job")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1`, owner.ID).Scan(&jobCount))
	assert.Equal(t, 1, jobCount, "permitted recovery enqueued a job")
}

func TestRiverAnalysisFailsFastWhenDependencyParsingIsUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "depparse-missing", false)
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner.ID, "depparse-source", "Dependency parsing", "Hallo Berlin.", "sha256:depparse-missing")
	require.NoError(t, err)
	fake := &analyzertest.Fake{AnalyzeFunc: func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{}, errors.New("Analyze should not be called")
	}}
	capabilities := analyzertest.CapabilityProvider{Value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{
		Language: "de", SupportedFeatures: []string{"tokenize", "pos", "lemma"}, Ready: true,
	}}}}
	client, err := NewClient(store.Pool(), fake, capabilities, selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)

	service := NewService(store.Pool(), client)
	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	status, err := service.Wait(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateDiscarded, status.State)
	assert.Contains(t, status.Error, "NLP service is misconfigured")
	assert.Contains(t, status.Error, "dependency parsing is unavailable")
	assert.Empty(t, fake.Requests())

	var corpusCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM corpora WHERE owner_id=$1`, owner.ID).Scan(&corpusCount))
	assert.Zero(t, corpusCount)
}

func TestRiverAnalysisPersistsNormalizedCorpus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Cleanup(t, "analysis store", store.Close)
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
	unitID := domain.EPUBUnitID(0, "corpus-source")
	tokens := []analyzer.Token{
		{Surface: "Hallo", RawLemma: "hallo", CanonicalLemma: "hallo", UPOS: "INTJ", Dependency: "dep", Head: 1, Morphology: map[string]string{"Polite": "No"}, Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 0, EndOffset: 5}},
		{Surface: "Berlin", RawLemma: "Berlin", CanonicalLemma: "berlin", UPOS: "PROPN", Dependency: "root", Head: 1, Morphology: map[string]string{"Case": "Nom"}, Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 6, EndOffset: 12}},
		{Surface: "stehe", RawLemma: "stehen", CanonicalLemma: "aufstehen", UPOS: "VERB", Dependency: "root", Head: 2, Location: analyzer.SourceLocation{SourceDocumentID: unitID}},
		{Surface: "auf", RawLemma: "auf", CanonicalLemma: "auf", UPOS: "ADV", Dependency: "compound:prt", Head: 2, Location: analyzer.SourceLocation{SourceDocumentID: unitID}},
	}
	for i := 0; i <= normalizedCorpusTokenInsertBatchSize; i++ {
		tokens = append(tokens, analyzer.Token{Surface: "x", RawLemma: "x", CanonicalLemma: "x", UPOS: "X", Dependency: "dep", Head: 1, Morphology: map[string]string{}, Location: analyzer.SourceLocation{SourceDocumentID: unitID}})
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
			NormalizationProfile: analyzer.NormalizationProfile{Name: "german-standard-post-1996", Version: "6"},
		}, nil
	}}
	client, err := NewClient(store.Pool(), fake, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	testutil.StopOnCleanup(t, "analysis client", client.Stop)
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
	var normalizationProfile, normalizationVersion string
	if err = pool.QueryRow(ctx, `SELECT normalization_profile,normalization_version FROM normalized_corpus_artifacts WHERE content_hash=$1`, corpus.ArtifactHash).Scan(&normalizationProfile, &normalizationVersion); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "german-standard-post-1996", normalizationProfile)
	assert.Equal(t, "6", normalizationVersion)
	var surface, rawLemma, canonicalLemma, upos, morphology, storedUnitID string
	var sentenceOrdinal, tokenOrdinal, startOffset, endOffset, head int64
	var dependency string
	if err = pool.QueryRow(ctx, `SELECT s.unit_id,t.surface,t.raw_lemma,t.canonical_lemma,t.upos,t.dependency,t.head,t.morphology::text,s.sentence_ordinal,t.token_ordinal,t.start_offset,t.end_offset
		FROM corpus_tokens t JOIN corpus_sentences s ON s.owner_id=t.owner_id AND s.corpus_id=t.corpus_id AND s.analysis_run_id=t.analysis_run_id AND s.sentence_ordinal=t.sentence_ordinal
		WHERE t.owner_id=$1 AND t.corpus_id=$2 AND t.canonical_lemma='berlin'`, owner.ID, corpus.ID).Scan(&storedUnitID, &surface, &rawLemma, &canonicalLemma, &upos, &dependency, &head, &morphology, &sentenceOrdinal, &tokenOrdinal, &startOffset, &endOffset); err != nil {
		t.Fatal(err)
	}
	if storedUnitID != unitID || surface != "Berlin" || rawLemma != "Berlin" || canonicalLemma != "berlin" || upos != "PROPN" || dependency != "root" || head != 1 || sentenceOrdinal != 0 || tokenOrdinal != 1 || startOffset != 6 || endOffset != 12 {
		t.Fatalf("stored token = unit=%q surface=%q raw=%q canonical=%q upos=%q dependency=%q head=%d morphology=%q sentence=%d token=%d offsets=%d:%d", storedUnitID, surface, rawLemma, canonicalLemma, upos, dependency, head, morphology, sentenceOrdinal, tokenOrdinal, startOffset, endOffset)
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
	var separatedRawLemma, separatedCanonicalLemma, separatedDependency string
	var separatedHead int64
	if err = pool.QueryRow(ctx, `SELECT raw_lemma,canonical_lemma,dependency,head FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND canonical_lemma='aufstehen'`, owner.ID, corpus.ID).Scan(&separatedRawLemma, &separatedCanonicalLemma, &separatedDependency, &separatedHead); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "stehen", separatedRawLemma)
	assert.Equal(t, "aufstehen", separatedCanonicalLemma)
	assert.Equal(t, "root", separatedDependency)
	assert.Equal(t, int64(2), separatedHead)
	var particleCandidateCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2 AND canonical_lemma='auf'`, owner.ID, corpus.ID).Scan(&particleCandidateCount); err != nil {
		t.Fatal(err)
	}
	assert.Zero(t, particleCandidateCount)
}

func TestRiverAnalysisSelectsMainTextAndVersionsTheRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "main-text-analysis", false)
	require.NoError(t, err)

	makeSourceUnits := func(identifier string, texts []string, landmarks [][]string) (string, []domain.ExtractedUnit) {
		fullText := strings.Join(texts, "\n\n")
		units := make([]domain.ExtractedUnit, len(texts))
		var start uint64
		for i, text := range texts {
			end := start + uint64(len([]rune(text)))
			manifestID := fmt.Sprintf("%s-%d", identifier, i)
			units[i] = domain.ExtractedUnit{
				ID: domain.EPUBUnitID(uint64(i), manifestID), Order: uint64(i), SpineIndex: uint64(i),
				ManifestID: manifestID, SourceHref: manifestID, ResolvedHref: manifestID,
				Text: text, StartOffset: start, EndOffset: end, MediaType: "application/xhtml+xml", Linear: true,
				LandmarkTypes: landmarks[i],
			}
			start = end + 2
		}
		return fullText, units
	}

	selectedText, selectedUnits := makeSourceUnits("selected", []string{"Vorwort.", "Kapitel Inhalt.", "Quellen."}, [][]string{{"titlepage"}, {"bodymatter"}, {"bibliography"}})
	selectedSource, err := putAnalysisSourceWithUnits(ctx, store, owner.ID, "selected", "Selected", selectedText, "sha256:selected", selectedUnits)
	require.NoError(t, err)
	snapshotID, beforeSnapshot, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, selectedSource.ID)
	require.NoError(t, err)

	var analyzed []string
	fake := &analyzertest.Fake{AnalyzeFunc: func(_ context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
		analyzed = append(analyzed, req.Document.Text)
		length := uint64(len([]rune(req.Document.Text)))
		return analyzer.Result{
			SchemaVersion: "1.0.0", Language: req.Language,
			Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"},
			NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"},
			Sentences: []analyzer.Sentence{{
				Text:     req.Document.Text,
				Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: length},
				Tokens:   []analyzer.Token{{Surface: "content", RawLemma: "content", CanonicalLemma: "content", UPOS: "NOUN", Dependency: "root", Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: length}}},
			}},
		}, nil
	}}
	client, err := NewClient(store.Pool(), fake, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)
	service := NewService(store.Pool(), client)

	handle, err := service.SubmitAnalysis(ctx, owner.ID, selectedSource.ID)
	require.NoError(t, err)
	status, err := service.Wait(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, status.State)
	assert.Equal(t, []string{"Kapitel Inhalt."}, analyzed)

	var configIdentity string
	require.NoError(t, pool.QueryRow(ctx, `SELECT config_identity FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID).Scan(&configIdentity))
	assert.Equal(t, mainTextConfigIdentity, configIdentity)
	corpus, err := service.Result(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	require.NotNil(t, corpus.Statistics)
	assert.Equal(t, int64(1), corpus.Statistics.AnalyzableTokenCount)
	require.NotNil(t, corpus.Statistics.TextProfile)
	assert.Equal(t, int64(1), corpus.Statistics.TextProfile.SentenceCount)

	var details []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT details FROM processing_history WHERE owner_id=$1 AND corpus_id=$2 AND operation='analysis' AND status='complete'`, owner.ID, corpus.ID).Scan(&details))
	var provenance map[string]any
	require.NoError(t, json.Unmarshal(details, &provenance))
	assert.Equal(t, mainTextConfigIdentity, provenance["selection_algorithm"])
	assert.Equal(t, true, provenance["selection_identified"])
	assert.Equal(t, true, provenance["selection_applied"])
	// Provenance counts are JSON integers, which decode exactly; 1e-9 rejects any fractional value.
	assert.InDelta(t, float64(1), provenance["body_matter_start"], 1e-9)
	assert.InDelta(t, float64(2), provenance["back_matter_start"], 1e-9)
	assert.InDelta(t, float64(1), provenance["selected_unit_count"], 1e-9)
	assert.InDelta(t, float64(3), provenance["total_unit_count"], 1e-9)
	assert.Equal(t, []any{selectedUnits[0].ID, selectedUnits[2].ID}, provenance["excluded_unit_ids"])

	gotSnapshotID, afterSnapshot, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, selectedSource.ID)
	require.NoError(t, err)
	assert.Equal(t, snapshotID, gotSnapshotID)
	assert.Equal(t, beforeSnapshot, afterSnapshot)

	duplicate, err := service.SubmitAnalysis(ctx, owner.ID, selectedSource.ID)
	require.NoError(t, err)
	assert.Equal(t, handle.ID, duplicate.ID)
	assert.Len(t, analyzed, 1, "duplicate submission created another analysis")

	wholeText, wholeUnits := makeSourceUnits("whole", []string{"Erster Inhalt.", "Zweiter Inhalt."}, [][]string{nil, nil})
	wholeSource, err := putAnalysisSourceWithUnits(ctx, store, owner.ID, "whole", "Whole", wholeText, "sha256:whole", wholeUnits)
	require.NoError(t, err)
	wholeHandle, err := service.SubmitAnalysis(ctx, owner.ID, wholeSource.ID)
	require.NoError(t, err)
	wholeStatus, err := service.Wait(ctx, owner.ID, wholeHandle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, wholeStatus.State)
	assert.Equal(t, []string{"Kapitel Inhalt.", "Erster Inhalt.", "Zweiter Inhalt."}, analyzed)
	require.NoError(t, pool.QueryRow(ctx, `SELECT config_identity FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner.ID, wholeHandle.RunID).Scan(&configIdentity))
	assert.Equal(t, snapshotConfigIdentity, configIdentity)
	var wholeDetails []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT details FROM processing_history WHERE owner_id=$1 AND corpus_id=(SELECT corpus_id FROM analysis_runs WHERE owner_id=$1 AND id=$2) AND operation='analysis' AND status='complete'`, owner.ID, wholeHandle.RunID).Scan(&wholeDetails))
	var wholeProvenance map[string]any
	require.NoError(t, json.Unmarshal(wholeDetails, &wholeProvenance))
	assert.Equal(t, snapshotConfigIdentity, wholeProvenance["selection_algorithm"])
	assert.Equal(t, false, wholeProvenance["selection_identified"])
	assert.Equal(t, false, wholeProvenance["selection_applied"])
	assert.InDelta(t, float64(2), wholeProvenance["selected_unit_count"], 1e-9)
	assert.InDelta(t, float64(2), wholeProvenance["total_unit_count"], 1e-9)
	assert.Equal(t, []any{}, wholeProvenance["excluded_unit_ids"])

	noOpText, noOpUnits := makeSourceUnits("noop", []string{"Main Inhalt.", "Weiterer Inhalt."}, [][]string{{"bodymatter"}, nil})
	noOpSource, err := putAnalysisSourceWithUnits(ctx, store, owner.ID, "noop", "No-op", noOpText, "sha256:noop", noOpUnits)
	require.NoError(t, err)
	noOpHandle, err := service.SubmitAnalysis(ctx, owner.ID, noOpSource.ID)
	require.NoError(t, err)
	noOpStatus, err := service.Wait(ctx, owner.ID, noOpHandle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, noOpStatus.State)
	assert.Equal(t, []string{"Kapitel Inhalt.", "Erster Inhalt.", "Zweiter Inhalt.", "Main Inhalt.", "Weiterer Inhalt."}, analyzed)
	require.NoError(t, pool.QueryRow(ctx, `SELECT config_identity FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner.ID, noOpHandle.RunID).Scan(&configIdentity))
	assert.Equal(t, snapshotConfigIdentity, configIdentity)
	noOpDuplicate, err := service.SubmitAnalysis(ctx, owner.ID, noOpSource.ID)
	require.NoError(t, err)
	assert.Equal(t, noOpHandle.ID, noOpDuplicate.ID)
	assert.Len(t, analyzed, 5, "no-op duplicate submission created another analysis")
}

func TestCompletedCorpusPublicationCanRetryWithoutRerunningNLP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "analysis-publication-retry", false)
	require.NoError(t, err)
	book, source := makeCurrentAnalyzedBook(t, ctx, store, owner.ID)
	var previousRunID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&previousRunID))

	calls := 0
	fake := countingTagAnalyzer(&calls)
	client, err := NewClient(pool, fake, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)
	service := NewService(pool, client)

	_, err = pool.Exec(ctx, `CREATE FUNCTION reject_analysis_publication() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'simulated publication failure'; END $$;
		CREATE TRIGGER reject_analysis_publication BEFORE UPDATE ON book_current_analyses FOR EACH ROW EXECUTE FUNCTION reject_analysis_publication()`)
	require.NoError(t, err)
	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	first, err := service.Wait(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateDiscarded, first.State)
	assert.Equal(t, "finalization-failed", first.LogicalState)
	assert.Equal(t, 1, calls, "initial analyzer execution")

	var corpusStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT c.status FROM analysis_runs r JOIN corpora c ON c.owner_id=r.owner_id AND c.id=r.corpus_id WHERE r.owner_id=$1 AND r.id=$2`, owner.ID, handle.RunID).Scan(&corpusStatus))
	assert.Equal(t, "complete", corpusStatus, "completed corpus must survive publication failure")
	var currentCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&currentCount))
	var retainedRunID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&retainedRunID))
	assert.Equal(t, previousRunID, retainedRunID, "failed replacement publication retains the previous current analysis")

	_, err = pool.Exec(ctx, `DROP TRIGGER reject_analysis_publication ON book_current_analyses; DROP FUNCTION reject_analysis_publication()`)
	require.NoError(t, err)
	retry, err := service.SubmitToReadBookAnalysis(ctx, owner.ID, book.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, handle.RunID, retry.RunID, "publication retry reuses the completed run")
	status, err := service.Wait(ctx, owner.ID, retry.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, status.State)
	assert.Equal(t, 1, calls, "publication retry must not rerun NLP")
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, owner.ID, book.ID, handle.RunID).Scan(&currentCount))
	assert.Equal(t, 1, currentCount, "retry promotes exactly the completed analysis")
	var readyRunID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&readyRunID))
	assert.Equal(t, handle.RunID, readyRunID, "published analysis and ready Browse counts share one exact run")
	var projectedOccurrences, persistedOccurrences int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT COALESCE(sum(occurrence_count),0) FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&projectedOccurrences))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM corpus_tokens t JOIN current_analysis_identity cai ON cai.owner_id=t.owner_id AND cai.analysis_run_id=t.analysis_run_id AND cai.corpus_id=t.corpus_id WHERE cai.owner_id=$1 AND cai.book_id=$2 AND t.upos IN ('NOUN','VERB','ADJ','ADV') AND t.dependency <> 'compound:prt'`, owner.ID, book.ID).Scan(&persistedOccurrences))
	assert.Equal(t, persistedOccurrences, projectedOccurrences, "projection is built from persisted corpus evidence")
	var corpusCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM corpora WHERE owner_id=$1 AND analysis_run_id=$2`, owner.ID, handle.RunID).Scan(&corpusCount))
	assert.Equal(t, 1, corpusCount, "publication replay does not duplicate corpus evidence")
	require.NoError(t, (&Worker{Pool: pool}).publishCompletedRun(ctx, JobArgs{OwnerID: owner.ID, SourceMaterialID: source.ID, RunID: handle.RunID, Language: "de"}, retry.JobID))

	_, err = putAnalysisSource(ctx, store, owner.ID, "current-retry-guard", book.Title, "Changed source.", "sha256:publication-retry-changed")
	require.NoError(t, err)
	require.NoError(t, (&Worker{Pool: pool}).publishCompletedRun(ctx, JobArgs{OwnerID: owner.ID, SourceMaterialID: source.ID, RunID: handle.RunID, Language: "de"}, retry.JobID))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, owner.ID, book.ID, handle.RunID).Scan(&currentCount))
	assert.Zero(t, currentCount, "a replay after a source revision change cannot keep stale analysis current")
}

func TestBrowseCountsRebuildWorkerRebuildsCurrentBookDurably(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	owner, err := store.CreateUser(ctx, "browse-count-rebuild", false)
	require.NoError(t, err)
	book, _ := makeCurrentAnalyzedBook(t, ctx, store, owner.ID)
	var runID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID))
	_, err = pool.Exec(ctx, `DELETE FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.NoError(t, err)

	worker := &BrowseCountsRebuildWorker{Pool: pool}
	require.NoError(t, worker.Work(ctx, &river.Job[BrowseCountsRebuildArgs]{Args: BrowseCountsRebuildArgs{OwnerID: owner.ID, BookID: book.ID, RunID: runID}}))
	var readyRun string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&readyRun))
	assert.Equal(t, runID, readyRun)
	page, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
	require.NoError(t, err)
	assert.Empty(t, page.Rows, "fixture has an empty eligible corpus, but the rebuild still publishes readiness")
	assert.Zero(t, page.InventoryTotal)
}

func TestBrowseCountsRebuildDiscardsCandidateWhenDecisionChangesDuringScan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	owner, err := store.CreateUser(ctx, "browse-count-decision-race", false)
	require.NoError(t, err)
	book, _ := makeCurrentAnalyzedBook(t, ctx, store, owner.ID)
	var runID, corpusID, unitID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT cai.analysis_run_id::text,cai.corpus_id::text,u.unit_id
		FROM current_analysis_identity cai JOIN source_material_units u ON u.owner_id=cai.owner_id AND u.source_material_id=cai.source_material_id
		WHERE cai.owner_id=$1 AND cai.book_id=$2 LIMIT 1`, owner.ID, book.ID).Scan(&runID, &corpusID, &unitID))
	_, err = pool.Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
		VALUES($1,$2,$3,$4,0,'Guten Tag.',0,10)`, owner.ID, runID, corpusID, unitID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset)
		VALUES($1,'de',$2,$3,0,0,'Tag','Tag','tag','NOUN','root',0,'{}',6,9)`, owner.ID, runID, corpusID)
	require.NoError(t, err)
	var eligibleTokens int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM corpus_tokens t JOIN corpus_sentences s ON s.owner_id=t.owner_id AND s.analysis_run_id=t.analysis_run_id AND s.corpus_id=t.corpus_id AND s.sentence_ordinal=t.sentence_ordinal
		JOIN source_material_units u ON u.owner_id=$1 AND u.source_material_id=(SELECT source_material_id FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2) AND u.snapshot_id=(SELECT snapshot_id FROM analysis_runs WHERE owner_id=$1 AND id=$3) AND u.unit_id=s.unit_id
		WHERE t.owner_id=$1 AND t.analysis_run_id=$3 AND t.corpus_id=$4 AND t.language='de' AND t.upos IN ('NOUN','VERB','ADJ','ADV') AND t.dependency <> 'compound:prt'`, owner.ID, book.ID, runID, corpusID).Scan(&eligibleTokens))
	require.Equal(t, 1, eligibleTokens)
	_, err = pool.Exec(ctx, `CREATE FUNCTION pause_browse_count_candidate() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(1); RETURN NEW; END $$;
		CREATE TRIGGER pause_browse_count_candidate BEFORE INSERT ON vocabulary_browse_counts FOR EACH ROW EXECUTE FUNCTION pause_browse_count_candidate()`)
	require.NoError(t, err)
	worker := &BrowseCountsRebuildWorker{Pool: pool}
	result := make(chan error, 1)
	go func() {
		result <- worker.Work(ctx, &river.Job[BrowseCountsRebuildArgs]{Args: BrowseCountsRebuildArgs{OwnerID: owner.ID, BookID: book.ID, RunID: runID}})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var scanning bool
		err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE state='active' AND wait_event='PgSleep' AND query ILIKE '%INSERT INTO vocabulary_browse_counts%')`).Scan(&scanning)
		require.NoError(t, err)
		if scanning {
			break
		}
		require.Less(t, time.Now(), deadline, "rebuild did not reach its staged candidate scan")
		time.Sleep(10 * time.Millisecond)
	}
	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Tag")
	require.NoError(t, err)
	require.Len(t, occurrences, 1)
	decisionStart := time.Now()
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "tageszeit", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}))
	assert.Less(t, time.Since(decisionStart), 750*time.Millisecond, "a decision on an unready Book must not wait for the full rebuild scan")
	require.ErrorIs(t, <-result, persistence.ErrBrowseCountDecisionsChanged, "the stale staged candidate must be discarded")
	_, err = pool.Exec(ctx, `DROP TRIGGER pause_browse_count_candidate ON vocabulary_browse_counts; DROP FUNCTION pause_browse_count_candidate()`)
	require.NoError(t, err)
	require.NoError(t, worker.Work(ctx, &river.Job[BrowseCountsRebuildArgs]{Args: BrowseCountsRebuildArgs{OwnerID: owner.ID, BookID: book.ID, RunID: runID}}))
	page, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
	require.NoError(t, err)
	require.False(t, page.BrowseCountsUpdating)
	require.Len(t, page.Rows, 1)
	assert.Equal(t, "tageszeit", page.Rows[0].CanonicalLemma)
	assert.Equal(t, int64(1), page.Rows[0].OccurrenceCount)
}

func TestBrowseCountsDiscardedRebuildIsReportedAsUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "browse-count-failed", false)
	require.NoError(t, err)
	book, _ := makeCurrentAnalyzedBook(t, ctx, store, owner.ID)
	var runID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID))
	_, err = pool.Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.NoError(t, err)
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	job, err := riverClient.Insert(ctx, BrowseCountsRebuildArgs{OwnerID: owner.ID, BookID: book.ID, RunID: runID}, &river.InsertOpts{Queue: Queue, MaxAttempts: 1})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE river_job SET state='discarded',finalized_at=now() WHERE id=$1`, job.Job.ID)
	require.NoError(t, err)
	page, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
	require.NoError(t, err)
	assert.True(t, page.BrowseCountsUnavailable)
	assert.False(t, page.BrowseCountsUpdating)
	assert.Empty(t, page.Rows)
	service := NewService(pool, riverClient)
	require.NoError(t, service.EnqueueMissingBrowseCountRebuilds(ctx), "operator restart reconciliation safely requeues a discarded build")
	var queued int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND state IN ('available','pending','retryable','scheduled') AND args->>'owner_id'=$2 AND args->>'book_id'=$3 AND args->>'run_id'=$4`, (BrowseCountsRebuildArgs{}).Kind(), owner.ID, book.ID, runID).Scan(&queued))
	assert.Equal(t, 1, queued)
	retried, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
	require.NoError(t, err)
	assert.False(t, retried.BrowseCountsUnavailable)
	assert.True(t, retried.BrowseCountsUpdating, "an active replacement attempt takes precedence over a discarded prior attempt")
	require.NoError(t, (&BrowseCountsRebuildWorker{Pool: pool}).Work(ctx, &river.Job[BrowseCountsRebuildArgs]{Args: BrowseCountsRebuildArgs{OwnerID: owner.ID, BookID: book.ID, RunID: runID}}))
	completedRetry, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
	require.NoError(t, err)
	assert.False(t, completedRetry.BrowseCountsUpdating)
	assert.False(t, completedRetry.BrowseCountsUnavailable)
}

func TestEmptyAnalysisPublicationAtomicallyPublishesReadyBrowseCounts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	owner, err := store.CreateUser(ctx, "browse-empty-publication", false)
	require.NoError(t, err)
	book, source := makeCurrentAnalyzedBook(t, ctx, store, owner.ID)
	var runID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID))
	_, err = pool.Exec(ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.NoError(t, err)
	worker := &Worker{Pool: pool}
	require.NoError(t, worker.publishCompletedRun(ctx, JobArgs{OwnerID: owner.ID, SourceMaterialID: source.ID, RunID: runID, Language: "de"}, 0))
	var readyRun string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&readyRun))
	assert.Equal(t, runID, readyRun)
	page, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
	require.NoError(t, err)
	assert.Empty(t, page.Rows)
	assert.Zero(t, page.InventoryTotal)
}

func TestCompletedAnalysisPublicationDoesNotPromoteSupersededRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	owner, err := store.CreateUser(ctx, "analysis-superseded-publication", false)
	require.NoError(t, err)
	book, source := makeCurrentAnalyzedBook(t, ctx, store, owner.ID)
	var previousRunID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&previousRunID))
	var snapshotID, artifactHash string
	require.NoError(t, pool.QueryRow(ctx, `SELECT r.snapshot_id::text,c.artifact_hash FROM analysis_runs r JOIN corpora c ON c.owner_id=r.owner_id AND c.id=r.corpus_id WHERE r.owner_id=$1 AND r.id=$2`, owner.ID, previousRunID).Scan(&snapshotID, &artifactHash))

	var oldRunID string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'test','1','older-candidate','completed',now()) RETURNING id::text`, owner.ID, source.ID, source.ContentRevisionID, snapshotID).Scan(&oldRunID))
	var corpusID string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash,analysis_run_id,status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count) VALUES($1,$2,$3,$4,'complete',0,0,0,0,0,0,0,0) RETURNING id::text`, owner.ID, source.ID, artifactHash, oldRunID).Scan(&corpusID))
	_, err = pool.Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpusID, owner.ID, oldRunID)
	require.NoError(t, err)
	var oldJobID, newJobID, oldDisplay, newDisplay int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT COALESCE(MAX(river_job_id),0)+1,COALESCE(MAX(display_number),0)+1 FROM analysis_jobs`).Scan(&oldJobID, &oldDisplay))
	newJobID, newDisplay = oldJobID+1, oldDisplay+1
	_, err = pool.Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,corpus_id,progress,display_number,analysis_run_id) VALUES($1,$2,$3,$4,$5,100,$6,$7)`, oldJobID, owner.ID, source.ID, source.ContentHash, corpusID, oldDisplay, oldRunID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state) VALUES($1,$2,$3,$4,'test','1','newer-candidate','queued')`, owner.ID, source.ID, source.ContentRevisionID, snapshotID)
	require.NoError(t, err)
	var newerRunID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT id::text FROM analysis_runs WHERE owner_id=$1 AND source_material_id=$2 AND config_identity='newer-candidate'`, owner.ID, source.ID).Scan(&newerRunID))
	_, err = pool.Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,progress,display_number,analysis_run_id) VALUES($1,$2,$3,$4,0,$5,$6)`, newJobID, owner.ID, source.ID, source.ContentHash, newDisplay, newerRunID)
	require.NoError(t, err)

	worker := &Worker{Pool: pool}
	require.NoError(t, worker.publishCompletedRun(ctx, JobArgs{OwnerID: owner.ID, SourceMaterialID: source.ID, RunID: oldRunID}, 0))
	var currentRunID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&currentRunID))
	assert.Equal(t, previousRunID, currentRunID, "a newer run fences publication of the older completed corpus")
	var exposed int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, owner.ID, book.ID, oldRunID).Scan(&exposed))
	assert.Zero(t, exposed)
}

func TestCompletedCorpusRetryRebuildsSelectionWithoutRerunningNLP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "analysis-selection-retry", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Selection retry", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner.ID, "selection-retry", book.Title, "Guten Tag.", "sha256:selection-retry")
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))

	calls := 0
	fake := countingTagAnalyzer(&calls)
	client, err := NewClient(pool, fake, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)
	service := NewService(pool, client)
	_, err = pool.Exec(ctx, `CREATE FUNCTION reject_selection_candidates() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'simulated selection failure'; END $$;
		CREATE TRIGGER reject_selection_candidates BEFORE INSERT ON selection_candidates FOR EACH ROW EXECUTE FUNCTION reject_selection_candidates()`)
	require.NoError(t, err)
	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	failed, err := service.Wait(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateDiscarded, failed.State)
	assert.Equal(t, 1, calls)
	var corpusID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT corpus_id::text FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID).Scan(&corpusID))
	var corpusStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, corpusID).Scan(&corpusStatus))
	assert.Equal(t, "complete", corpusStatus)
	t.Run("candidate-generation failure is recorded", func(t *testing.T) {
		var generationDetails []byte
		require.NoError(t, pool.QueryRow(ctx, `SELECT details FROM processing_history WHERE owner_id=$1 AND corpus_id=$2 AND operation='candidate_generation' AND status='failed' ORDER BY id DESC LIMIT 1`, owner.ID, corpusID).Scan(&generationDetails))
		var generationRecorded map[string]any
		require.NoError(t, json.Unmarshal(generationDetails, &generationRecorded))
		assert.Equal(t, "selection", generationRecorded["stage"])
		assert.NotEmpty(t, generationRecorded["error"])
	})

	_, err = pool.Exec(ctx, `DROP TRIGGER reject_selection_candidates ON selection_candidates; DROP FUNCTION reject_selection_candidates()`)
	require.NoError(t, err)
	retry, err := service.SubmitToReadBookAnalysis(ctx, owner.ID, book.ID, source.ID)
	require.NoError(t, err)
	status, err := service.Wait(ctx, owner.ID, retry.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, status.State)
	assert.Equal(t, 1, calls, "selection replay must use persisted evidence rather than rerunning NLP")
	var candidates, promoted int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2`, owner.ID, corpusID).Scan(&candidates))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, owner.ID, book.ID, handle.RunID).Scan(&promoted))
	assert.Equal(t, 1, candidates)
	assert.Equal(t, 1, promoted)
}

func TestQueuedPublicationCanBeCancelledAndRetriedWithoutDiscardingCorpus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "analysis-finalization-cancel", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Finalization cancel", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner.ID, "finalization-cancel", book.Title, "Guten Tag.", "sha256:finalization-cancel")
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	client, err := NewClient(pool, &analyzertest.Fake{}, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	service := NewService(pool, client)
	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: source.ContentHash, Language: "de", SchemaVersion: "1", NormalizationProfile: "casefold", NormalizationVersion: "1", AnalyzerName: "fake", AnalyzerVersion: "1"}, nil))
	corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, source.ContentHash)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete',analyzable_token_count=0,distinct_lemma_count=0 WHERE owner_id=$2 AND id=$3`, handle.RunID, owner.ID, corpus.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE analysis_runs SET state='completed',corpus_id=$3,completed_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID, corpus.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE analysis_jobs SET corpus_id=$3,progress=100 WHERE owner_id=$1 AND analysis_run_id=$2`, owner.ID, handle.RunID, corpus.ID)
	require.NoError(t, err)

	cancelled, err := service.Cancel(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCancelled, cancelled.State)
	assert.Equal(t, "finalization-cancelled", cancelled.LogicalState)
	var corpusStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, corpus.ID).Scan(&corpusStatus))
	assert.Equal(t, "complete", corpusStatus, "cancelling finalization retains the immutable corpus")

	retry, err := service.SubmitToReadBookAnalysis(ctx, owner.ID, book.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, handle.RunID, retry.RunID)
	assert.NotEqual(t, handle.JobID, retry.JobID, "retry uses a fresh River attempt")
}

type discardedFinalizationClient struct {
	row *rivertype.JobRow
}

func (discardedFinalizationClient) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, errors.New("unexpected analysis insert")
}

func (c discardedFinalizationClient) JobGet(context.Context, int64) (*rivertype.JobRow, error) {
	return c.row, nil
}

func (discardedFinalizationClient) JobCancel(context.Context, int64) (*rivertype.JobRow, error) {
	return nil, errors.New("unexpected analysis cancel")
}

// A finalization attempt whose River job was discarded before the durable
// attempt row caught up must surface as a failed finalization, carrying River's
// last error.
func TestAnalysisStatusReportsDiscardedFinalizationAsFailed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "analysis-finalization-discarded", false)
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner.ID, "finalization-discarded", "Finalization discarded", "Guten Tag.", "sha256:finalization-discarded")
	require.NoError(t, err)
	client, err := NewClient(pool, &analyzertest.Fake{}, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	service := NewService(pool, client)
	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE analysis_runs SET state='completed',completed_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE analysis_run_attempts SET state='running',started_at=now() WHERE run_id=$1`, handle.RunID)
	require.NoError(t, err)

	service.client = discardedFinalizationClient{row: &rivertype.JobRow{State: rivertype.JobStateDiscarded, Errors: []rivertype.AttemptError{{Error: "finalization worker terminated"}}}}
	status, err := service.Get(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	t.Run("reports the run as discarded", func(t *testing.T) {
		require.Equal(t, rivertype.JobStateDiscarded, status.State)
	})
	t.Run("reports the failed finalization logical state", func(t *testing.T) {
		require.Equal(t, "finalization-failed", status.LogicalState)
	})
	t.Run("carries River's last error", func(t *testing.T) {
		require.Equal(t, "finalization worker terminated", status.Error)
	})
}

// A completed run whose finalization attempt is still queued or running reports
// the finalizing logical state until the attempt settles.
func TestAnalysisStatusReportsCompletedRunAsFinalizing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "analysis-finalization-pending", false)
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner.ID, "finalization-pending", "Finalization pending", "Guten Tag.", "sha256:finalization-pending")
	require.NoError(t, err)
	client, err := NewClient(pool, &analyzertest.Fake{}, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	service := NewService(pool, client)
	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE analysis_runs SET state='completed',completed_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID)
	require.NoError(t, err)

	t.Run("queued attempt is available", func(t *testing.T) {
		status, err := service.Get(ctx, owner.ID, handle.ID)
		require.NoError(t, err)
		require.Equal(t, "finalizing", status.LogicalState)
		require.Equal(t, rivertype.JobStateAvailable, status.State)
	})

	_, err = pool.Exec(ctx, `UPDATE analysis_run_attempts SET state='running',started_at=now() WHERE run_id=$1`, handle.RunID)
	require.NoError(t, err)
	t.Run("running attempt is running", func(t *testing.T) {
		status, err := service.Get(ctx, owner.ID, handle.ID)
		require.NoError(t, err)
		require.Equal(t, "finalizing", status.LogicalState)
		require.Equal(t, rivertype.JobStateRunning, status.State)
	})
}

func TestRiverAnalysisLifecycleDedupAndOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	alice, err := store.CreateUser(ctx, "jobs-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "jobs-bob", false)
	require.NoError(t, err)
	fullText := strings.Repeat("Häuser. ", 13_000)
	source, err := putAnalysisSource(ctx, store, alice.ID, "job-source", "Job", fullText, "sha256:job-success")
	require.NoError(t, err)
	var (
		analyzedChunksMu sync.Mutex
		analyzedChunks   []string
	)
	fake := &analyzertest.Fake{AnalyzeFunc: func(analyzeCtx context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
		if req.Document.Text == "fail" {
			return analyzer.Result{}, errors.New("stanza failed")
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
		return analyzer.Result{SchemaVersion: "1.0.0", Language: req.Language, Analysis: analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"}, NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"}, Sentences: []analyzer.Sentence{{Text: req.Document.Text, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: uint64(len([]rune(req.Document.Text)))}, Tokens: []analyzer.Token{{Surface: "Häuser", RawLemma: "Häuser", CanonicalLemma: "haus", UPOS: "NOUN", Dependency: "root", Head: 0, Morphology: map[string]string{"Number": "Plur"}, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: 6}}}}}}, nil
	}}
	client, err := NewClient(store.Pool(), fake, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)
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
	t.Run("cross-owner get is not found", func(t *testing.T) {
		require.ErrorIs(t, err, ErrNotFound)
	})
	_, err = service.Cancel(ctx, bob.ID, handle.ID)
	t.Run("cross-owner cancel is not found", func(t *testing.T) {
		require.ErrorIs(t, err, ErrNotFound)
	})
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
	// A median of integer token counts is exact in float64 (an integer or .5); 1e-9 only absorbs arithmetic noise.
	assert.InDelta(t, float64(1), corpus.Statistics.TextProfile.MedianSentenceTokenCount, 1e-9)
	completedStatus, err := service.Cancel(ctx, alice.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, completedStatus.State)
	assert.Equal(t, "completed", completedStatus.LogicalState)
	var candidateCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2`, alice.ID, corpus.ID).Scan(&candidateCount))
	assert.Equal(t, 1, candidateCount)
	var bobCandidateCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, bob.ID).Scan(&bobCandidateCount))
	assert.Equal(t, 1, bobCandidateCount)
	_, err = service.Result(ctx, bob.ID, handle.ID)
	t.Run("cross-owner result is not found", func(t *testing.T) {
		require.ErrorIs(t, err, ErrNotFound)
	})

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

type failingAnalysisCancellationClient struct {
	*river.Client[pgx.Tx]
	err error
}

func (c *failingAnalysisCancellationClient) JobCancel(context.Context, int64) (*rivertype.JobRow, error) {
	return nil, c.err
}

func TestAnalysisCancellationKeepsDurableStateWhenRiverCleanupFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))

	workers := river.NewWorkers()
	river.AddWorker(workers, &Worker{})
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{Queue: {MaxWorkers: 1}},
		Workers: workers,
	})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "analysis River client", riverClient.Stop)
	owner, err := store.CreateUser(ctx, "analysis-cancel-cleanup", false)
	require.NoError(t, err)

	tests := []struct {
		name        string
		findLiveJob func(context.Context, string, string) (int64, error)
		clientErr   error
	}{
		{
			name: "live job lookup",
			findLiveJob: func(context.Context, string, string) (int64, error) {
				return 0, errors.New("River lookup unavailable")
			},
		},
		{
			name:      "River cancellation",
			clientErr: errors.New("River cancellation unavailable"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, sourceErr := putAnalysisSource(ctx, store, owner.ID, "cancel-"+test.name, "Cancel", "cancel", "sha256:cancel-"+test.name)
			require.NoError(t, sourceErr)
			service := NewService(store.Pool(), riverClient)
			handle, submitErr := service.SubmitAnalysis(ctx, owner.ID, source.ID)
			require.NoError(t, submitErr)
			if test.findLiveJob != nil {
				service.findLiveJob = test.findLiveJob
			}
			if test.clientErr != nil {
				service.client = &failingAnalysisCancellationClient{Client: riverClient, err: test.clientErr}
			}

			status, cancelErr := service.Cancel(ctx, owner.ID, handle.ID)
			require.NoError(t, cancelErr)
			assert.Equal(t, rivertype.JobStateCancelled, status.State)
			assert.Equal(t, "cancelled", status.LogicalState)

			var runState, attemptState string
			require.NoError(t, pool.QueryRow(ctx, `SELECT r.state,a.state FROM analysis_runs r JOIN analysis_run_attempts a ON a.run_id=r.id WHERE r.owner_id=$1 AND r.id=$2`, owner.ID, handle.RunID).Scan(&runState, &attemptState))
			assert.Equal(t, "cancelled", runState)
			assert.Equal(t, "cancelled", attemptState)
		})
	}
}

func TestAnalysisRunSurvivesDispositionChanges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
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
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
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
		return analyzer.Result{SchemaVersion: "1.0.0", Language: req.Language, Analysis: analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"}, NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"}, Sentences: []analyzer.Sentence{{Text: req.Document.Text, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: uint64(len([]rune(req.Document.Text)))}, Tokens: []analyzer.Token{{Surface: "Haus", RawLemma: "Haus", CanonicalLemma: "haus", UPOS: "NOUN", Dependency: "root", Head: 0, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: 4}}}}}}, nil
	}}
	client, err := NewClient(store.Pool(), fake, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)
	service := NewService(store.Pool(), client)
	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	select {
	case <-started:
	case <-ctx.Done():
		require.Fail(t, "analysis did not reach running state")
	}
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionInbox))
	var state string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID).Scan(&state))
	assert.Equal(t, "running", state, "analysis state after disposition change")
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	reused, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	assert.Equal(t, handle.ID, reused.ID)
	assert.Equal(t, handle.RunID, reused.RunID)
	releaseOnce.Do(func() { close(release) })
	status, err := service.Wait(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, status.State)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionInbox))
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
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

func TestRiverAnalysisFailsWhenSourceRevisionIsNoLongerCurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "stale-revision", false)
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner.ID, "stale-revision", "Stale revision", "Hallo Berlin.", "sha256:stale-revision")
	require.NoError(t, err)
	fake := &analyzertest.Fake{AnalyzeFunc: func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{}, errors.New("Analyze should not be called")
	}}
	client, err := NewClient(store.Pool(), fake, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	service := NewService(store.Pool(), client)

	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	updated, err := putAnalysisSource(ctx, store, owner.ID, "stale-revision", "Stale revision", "Changed text.", "sha256:stale-revision-updated")
	require.NoError(t, err)
	require.Equal(t, source.ID, updated.ID)
	require.NotEqual(t, source.ContentRevisionID, updated.ContentRevisionID)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)

	status, err := service.Wait(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateDiscarded, status.State)
	assert.Equal(t, "Analysis could not be completed. Retry the analysis or review the current source.", status.Error)
	assert.Empty(t, fake.Requests())
	var corpusCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM corpora WHERE owner_id=$1`, owner.ID).Scan(&corpusCount))
	assert.Zero(t, corpusCount)
}

func TestRiverAnalysisRecordsFailureAcrossRunAttemptJobAndHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "analysis-failure-record", false)
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner.ID, "failure-record", "Failure record", "Hallo Berlin.", "sha256:failure-record")
	require.NoError(t, err)
	analyzerErr := errors.New("stanza failed")
	fake := &analyzertest.Fake{AnalyzeFunc: func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{}, analyzerErr
	}}
	client, err := NewClient(store.Pool(), fake, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)
	service := NewService(store.Pool(), client)

	handle, err := service.SubmitAnalysis(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	status, err := service.Wait(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	require.Equal(t, rivertype.JobStateDiscarded, status.State)

	message := "Analysis could not be completed. Retry the analysis or review the current source."
	t.Run("run and attempt carry the failure", func(t *testing.T) {
		var runState, lastError, attemptState, attemptError string
		require.NoError(t, pool.QueryRow(ctx, `SELECT r.state,r.last_error,a.state,a.error FROM analysis_runs r JOIN analysis_run_attempts a ON a.run_id=r.id WHERE r.owner_id=$1 AND r.id=$2`, owner.ID, handle.RunID).Scan(&runState, &lastError, &attemptState, &attemptError))
		require.Equal(t, "failed", runState)
		require.Equal(t, message, lastError)
		require.Equal(t, "failed", attemptState)
		require.Equal(t, message, attemptError)
	})
	t.Run("analysis job carries the failure", func(t *testing.T) {
		var jobError string
		require.NoError(t, pool.QueryRow(ctx, `SELECT error FROM analysis_jobs WHERE owner_id=$1 AND analysis_run_id=$2`, owner.ID, handle.RunID).Scan(&jobError))
		require.Equal(t, message, jobError)
	})
	t.Run("processing history records the failure", func(t *testing.T) {
		var details []byte
		require.NoError(t, pool.QueryRow(ctx, `SELECT details FROM processing_history WHERE owner_id=$1 AND operation='analysis' AND status='failed' ORDER BY id DESC LIMIT 1`, owner.ID).Scan(&details))
		var recorded map[string]any
		require.NoError(t, json.Unmarshal(details, &recorded))
		require.Equal(t, handle.RunID, recorded["run_id"])
		require.EqualValues(t, 1, recorded["attempt"])
		require.Equal(t, message, recorded["error"])
	})
}

// TestAnalysisSubmissionRejectsIneligibleSourcesAndBooks pins every admission
// and source-loading rejection of the shared submission path through its public
// seam, so the split into named steps cannot silently change an error or write
// an operational row for a rejected submission.
func TestAnalysisSubmissionRejectsIneligibleSourcesAndBooks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "submission-guards", false)
	require.NoError(t, err)
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	service := NewService(pool, riverClient)

	newBook := func(title string) domain.Book {
		book, bookErr := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, bookErr)
		return book
	}
	toReadBook := newBook("Guard to read")
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, toReadBook.ID, domain.BookDispositionToRead))
	linkedSource, err := putAnalysisSource(ctx, store, owner.ID, "guard-linked", toReadBook.Title, "Guten Tag.", "sha256:guard-linked")
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, toReadBook.ID, linkedSource.ID))

	otherBook := newBook("Guard other to read")
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, otherBook.ID, domain.BookDispositionToRead))

	inboxBook := newBook("Guard inbox")
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, inboxBook.ID, domain.BookDispositionInbox))

	nonEPUB, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "guard-pdf", Title: "PDF", MediaType: "application/pdf", ContentHash: "sha256:guard-pdf", Content: []byte("pdf"), FullText: "pdf"})
	require.NoError(t, err)
	legacyEPUB, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "guard-legacy", Title: "Legacy", MediaType: "application/epub+zip", ContentHash: "sha256:guard-legacy", Content: []byte("legacy"), FullText: "legacy"})
	require.NoError(t, err)
	missingUnits, err := putAnalysisSource(ctx, store, owner.ID, "guard-missing-units", "Missing units", "Guten Tag.", "sha256:guard-missing-units")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM source_material_units WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, missingUnits.ID)
	require.NoError(t, err)

	const absentUUID = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name     string
		submit   func() (Handle, error)
		wantErr  error
		wantText string
	}{
		{name: "missing owner", submit: func() (Handle, error) { return service.SubmitAnalysis(ctx, " ", linkedSource.ID) }, wantText: "analysis owner and source are required"},
		{name: "missing source identifier", submit: func() (Handle, error) { return service.SubmitAnalysis(ctx, owner.ID, " ") }, wantText: "analysis owner and source are required"},
		{name: "missing book identifier", submit: func() (Handle, error) { return service.SubmitToReadBookAnalysis(ctx, owner.ID, "  ", linkedSource.ID) }, wantText: "analysis Book is required"},
		{name: "malformed book identifier", submit: func() (Handle, error) {
			return service.SubmitToReadBookAnalysis(ctx, owner.ID, "not-a-uuid", linkedSource.ID)
		}, wantText: "lock To Read Book for analysis"},
		{name: "missing book", submit: func() (Handle, error) {
			return service.SubmitToReadBookAnalysis(ctx, owner.ID, absentUUID, linkedSource.ID)
		}, wantErr: ErrNotFound},
		{name: "book is not to read", submit: func() (Handle, error) {
			return service.SubmitToReadBookAnalysis(ctx, owner.ID, inboxBook.ID, linkedSource.ID)
		}, wantErr: ErrNotFound},
		{name: "unknown source", submit: func() (Handle, error) { return service.SubmitAnalysis(ctx, owner.ID, absentUUID) }, wantErr: ErrNotFound},
		{name: "source belongs to another book", submit: func() (Handle, error) {
			return service.SubmitToReadBookAnalysis(ctx, owner.ID, otherBook.ID, linkedSource.ID)
		}, wantErr: ErrNotFound},
		{name: "non-EPUB source", submit: func() (Handle, error) { return service.SubmitAnalysis(ctx, owner.ID, nonEPUB.ID) }, wantErr: ErrEPUBRequired},
		{name: "EPUB without current revision", submit: func() (Handle, error) { return service.SubmitAnalysis(ctx, owner.ID, legacyEPUB.ID) }, wantErr: domain.ErrExtractedUnitsUnavailable},
		{name: "EPUB without extracted units", submit: func() (Handle, error) { return service.SubmitAnalysis(ctx, owner.ID, missingUnits.ID) }, wantErr: domain.ErrExtractedUnitsUnavailable, wantText: "load extracted EPUB units"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handle, submitErr := test.submit()
			require.Error(t, submitErr)
			assert.Equal(t, Handle{}, handle)
			if test.wantErr != nil {
				require.ErrorIs(t, submitErr, test.wantErr)
			}
			if test.wantText != "" {
				assert.Contains(t, submitErr.Error(), test.wantText)
			}
			var runs, jobs, history int
			require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, owner.ID).Scan(&runs))
			require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1`, owner.ID).Scan(&jobs))
			require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='analysis'`, owner.ID).Scan(&history))
			assert.Zero(t, runs, "rejected submission created an analysis run")
			assert.Zero(t, jobs, "rejected submission created an analysis job")
			assert.Zero(t, history, "rejected submission wrote operational history")
		})
	}
}

type failingInsertAnalysisClient struct {
	*river.Client[pgx.Tx]
	err error
}

func (c *failingInsertAnalysisClient) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, c.err
}

// TestAnalysisSubmissionSurfacesOperationalFailures pins the write-failure
// branches of the shared submission path through its public seam, so the split
// into named steps keeps failing loudly instead of silently returning a zero
// handle or leaving a partial run behind.
func TestAnalysisSubmissionSurfacesOperationalFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "submission-failures", false)
	require.NoError(t, err)
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	service := NewService(pool, riverClient)

	t.Run("missing existing analysis handle", func(t *testing.T) {
		source, sourceErr := putAnalysisSource(ctx, store, owner.ID, "failure-handle", "Handle", "Guten Tag.", "sha256:failure-handle")
		require.NoError(t, sourceErr)
		handle, submitErr := service.SubmitAnalysis(ctx, owner.ID, source.ID)
		require.NoError(t, submitErr)
		_, execErr := pool.Exec(ctx, `DELETE FROM analysis_jobs WHERE owner_id=$1 AND analysis_run_id=$2`, owner.ID, handle.RunID)
		require.NoError(t, execErr)
		_, submitErr = service.SubmitAnalysis(ctx, owner.ID, source.ID)
		require.ErrorContains(t, submitErr, "load existing analysis handle")
	})

	t.Run("missing book disposition", func(t *testing.T) {
		book, bookErr := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "No disposition", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, bookErr)
		_, execErr := pool.Exec(ctx, `DELETE FROM book_dispositions WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
		require.NoError(t, execErr)
		source, sourceErr := putAnalysisSource(ctx, store, owner.ID, "failure-disposition", "Disposition", "Guten Tag.", "sha256:failure-disposition")
		require.NoError(t, sourceErr)
		_, submitErr := service.SubmitToReadBookAnalysis(ctx, owner.ID, book.ID, source.ID)
		require.ErrorIs(t, submitErr, ErrNotFound)
	})

	t.Run("river insert failure", func(t *testing.T) {
		source, sourceErr := putAnalysisSource(ctx, store, owner.ID, "failure-enqueue", "Enqueue", "Guten Tag.", "sha256:failure-enqueue")
		require.NoError(t, sourceErr)
		failing := NewService(pool, &failingInsertAnalysisClient{Client: riverClient, err: errors.New("River insert unavailable")})
		_, submitErr := failing.SubmitAnalysis(ctx, owner.ID, source.ID)
		require.ErrorContains(t, submitErr, "enqueue analysis")
		var runs int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&runs))
		assert.Zero(t, runs, "failed enqueue persisted an analysis run")
	})

	triggerCases := []struct {
		name    string
		table   string
		message string
	}{
		{name: "create analysis run failure", table: "analysis_runs", message: "simulated analysis run failure"},
		{name: "analysis job failure", table: "analysis_jobs", message: "simulated analysis job failure"},
		{name: "first attempt failure", table: "analysis_run_attempts", message: "simulated analysis attempt failure"},
	}
	for i, test := range triggerCases {
		t.Run(test.name, func(t *testing.T) {
			function := fmt.Sprintf("reject_submission_%d", i)
			trigger := function + "_trg"
			_, execErr := pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION '%s'; END $$; CREATE TRIGGER %s BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION %s()`, function, test.message, trigger, test.table, function))
			require.NoError(t, execErr)
			t.Cleanup(func() {
				_, dropErr := pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER %s ON %s; DROP FUNCTION %s()`, trigger, test.table, function))
				require.NoError(t, dropErr)
			})
			source, sourceErr := putAnalysisSource(ctx, store, owner.ID, "failure-"+test.table, test.name, "Guten Tag.", "sha256:failure-"+test.table)
			require.NoError(t, sourceErr)
			_, submitErr := service.SubmitAnalysis(ctx, owner.ID, source.ID)
			require.ErrorContains(t, submitErr, test.message)
		})
	}
}

// TestAnalysisSubmissionSurfacesExistingRunFailures pins the write-failure
// branches of the existing-run step through the public seam: an attempt that
// cannot be ensured for a queued run, and a failed run whose requeue cannot
// update the run, ensure an attempt, or record history.
func TestAnalysisSubmissionSurfacesExistingRunFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "existing-run-failures", false)
	require.NoError(t, err)
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	service := NewService(pool, riverClient)
	failing := NewService(pool, &failingInsertAnalysisClient{Client: riverClient, err: errors.New("River insert unavailable")})

	newQueuedRun := func(t *testing.T, identifier string) domain.SourceMaterial {
		t.Helper()
		source, sourceErr := putAnalysisSource(ctx, store, owner.ID, identifier, identifier, "Guten Tag.", "sha256:"+identifier)
		require.NoError(t, sourceErr)
		_, submitErr := service.SubmitAnalysis(ctx, owner.ID, source.ID)
		require.NoError(t, submitErr)
		return source
	}
	dropLiveWork := func(t *testing.T, source domain.SourceMaterial) {
		t.Helper()
		_, execErr := pool.Exec(ctx, `DELETE FROM analysis_run_attempts WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID)
		require.NoError(t, execErr)
		_, execErr = pool.Exec(ctx, `DELETE FROM river_job WHERE kind=$1 AND args->>'owner_id'=$2 AND args->>'source_material_id'=$3`, (JobArgs{}).Kind(), owner.ID, source.ID)
		require.NoError(t, execErr)
	}
	failRun := func(t *testing.T, source domain.SourceMaterial) {
		t.Helper()
		_, execErr := pool.Exec(ctx, `UPDATE analysis_runs SET state='failed' WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID)
		require.NoError(t, execErr)
	}
	rejectWrites := func(t *testing.T, name, operation, table, message string) {
		t.Helper()
		_, execErr := pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION '%s'; END $$; CREATE TRIGGER %s_trg BEFORE %s ON %s FOR EACH ROW EXECUTE FUNCTION %s()`, name, message, name, operation, table, name))
		require.NoError(t, execErr)
		t.Cleanup(func() {
			_, dropErr := pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER %s_trg ON %s; DROP FUNCTION %s()`, name, table, name))
			require.NoError(t, dropErr)
		})
	}

	t.Run("queued attempt insert failure", func(t *testing.T) {
		source := newQueuedRun(t, "existing-queued-attempt")
		dropLiveWork(t, source)
		_, submitErr := failing.SubmitAnalysis(ctx, owner.ID, source.ID)
		require.ErrorContains(t, submitErr, "River insert unavailable")
	})

	t.Run("requeue attempt insert failure", func(t *testing.T) {
		source := newQueuedRun(t, "existing-requeue-attempt")
		dropLiveWork(t, source)
		failRun(t, source)
		_, submitErr := failing.SubmitAnalysis(ctx, owner.ID, source.ID)
		require.ErrorContains(t, submitErr, "River insert unavailable")
	})

	t.Run("requeue update failure", func(t *testing.T) {
		source := newQueuedRun(t, "existing-requeue-update")
		failRun(t, source)
		rejectWrites(t, "reject_requeue_update", "UPDATE", "analysis_runs", "simulated requeue update failure")
		_, submitErr := service.SubmitAnalysis(ctx, owner.ID, source.ID)
		require.ErrorContains(t, submitErr, "simulated requeue update failure")
	})

	t.Run("requeue history failure", func(t *testing.T) {
		source := newQueuedRun(t, "existing-requeue-history")
		failRun(t, source)
		rejectWrites(t, "reject_requeue_history", "INSERT", "processing_history", "simulated requeue history failure")
		_, submitErr := service.SubmitAnalysis(ctx, owner.ID, source.ID)
		require.ErrorContains(t, submitErr, "simulated requeue history failure")
	})
}

// TestAnalysisRetryTransitionsSnapshotRuns pins the retry state machine through
// its public seam: the legacy-run rejection, resuming queued work, failing an
// orphaned running run, replaying a completed publication, and requeueing a
// failed or cancelled run with its history. Every branch is driven through
// durable state, so splitting Retry into named steps cannot silently change a
// transition.
func TestAnalysisRetryTransitionsSnapshotRuns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "analysis-retry", false)
	require.NoError(t, err)
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	service := NewService(pool, riverClient)

	newSource := func(t *testing.T, identifier string) domain.SourceMaterial {
		t.Helper()
		source, sourceErr := putAnalysisSource(ctx, store, owner.ID, identifier, identifier, "Guten Tag.", "sha256:"+identifier)
		require.NoError(t, sourceErr)
		return source
	}
	submit := func(t *testing.T, identifier string) Handle {
		t.Helper()
		handle, submitErr := service.SubmitAnalysis(ctx, owner.ID, newSource(t, identifier).ID)
		require.NoError(t, submitErr)
		return handle
	}
	runState := func(t *testing.T, runID string) string {
		t.Helper()
		var state string
		require.NoError(t, pool.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner.ID, runID).Scan(&state))
		return state
	}
	requireRetryHistory := func(t *testing.T, from string) {
		t.Helper()
		var details []byte
		require.NoError(t, pool.QueryRow(ctx, `SELECT details FROM processing_history WHERE owner_id=$1 AND operation='analysis' AND status='queued' AND details->>'from'=$2`, owner.ID, from).Scan(&details))
		var recorded map[string]any
		require.NoError(t, json.Unmarshal(details, &recorded))
		assert.Equal(t, from, recorded["from"])
	}
	detachLiveWork := func(t *testing.T, handle Handle) {
		t.Helper()
		_, execErr := pool.Exec(ctx, `DELETE FROM analysis_run_attempts WHERE run_id=$1`, handle.RunID)
		require.NoError(t, execErr)
		_, execErr = pool.Exec(ctx, `DELETE FROM river_job WHERE id=$1`, handle.JobID)
		require.NoError(t, execErr)
	}

	t.Run("legacy job is rejected", func(t *testing.T) {
		source := newSource(t, "retry-legacy")
		job, insertErr := riverClient.Insert(ctx, JobArgs{OwnerID: owner.ID, SourceMaterialID: source.ID, ContentHash: source.ContentHash, Language: "de"}, &river.InsertOpts{Queue: Queue, MaxAttempts: 1})
		require.NoError(t, insertErr)
		_, execErr := pool.Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,display_number) VALUES($1,$2,$3,$4,(SELECT COALESCE(MAX(display_number),0)+1 FROM analysis_jobs WHERE owner_id=$2))`, job.Job.ID, owner.ID, source.ID, source.ContentHash)
		require.NoError(t, execErr)
		_, retryErr := service.Retry(ctx, owner.ID, job.Job.ID)
		require.ErrorContains(t, retryErr, "legacy analysis jobs cannot be retried")
	})

	t.Run("queued run with live work resumes", func(t *testing.T) {
		handle := submit(t, "retry-queued-live")
		retried, retryErr := service.Retry(ctx, owner.ID, handle.ID)
		require.NoError(t, retryErr)
		assert.Equal(t, handle.JobID, retried.JobID)
		assert.Equal(t, handle.RunID, retried.RunID)
		assert.Equal(t, "queued", runState(t, handle.RunID))
	})

	t.Run("queued run without live work re-enqueues", func(t *testing.T) {
		handle := submit(t, "retry-queued-orphan")
		detachLiveWork(t, handle)
		retried, retryErr := service.Retry(ctx, owner.ID, handle.ID)
		require.NoError(t, retryErr)
		assert.Equal(t, handle.RunID, retried.RunID)
		assert.NotEqual(t, handle.JobID, retried.JobID)
		assert.Equal(t, "queued", runState(t, handle.RunID))
	})

	t.Run("running run without live work fails and requeues", func(t *testing.T) {
		handle := submit(t, "retry-running-orphan")
		_, execErr := pool.Exec(ctx, `UPDATE analysis_runs SET state='running',started_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID)
		require.NoError(t, execErr)
		detachLiveWork(t, handle)
		retried, retryErr := service.Retry(ctx, owner.ID, handle.ID)
		require.NoError(t, retryErr)
		assert.Equal(t, handle.RunID, retried.RunID)
		assert.NotEqual(t, handle.JobID, retried.JobID)
		assert.Equal(t, "queued", runState(t, handle.RunID))
		requireRetryHistory(t, "failed")
	})

	t.Run("completed run replays publication", func(t *testing.T) {
		source := newSource(t, "retry-completed")
		book, bookErr := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Retry publication", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, bookErr)
		require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
		handle, submitErr := service.SubmitAnalysis(ctx, owner.ID, source.ID)
		require.NoError(t, submitErr)
		require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: source.ContentHash, Language: "de", SchemaVersion: "1", NormalizationProfile: "casefold", NormalizationVersion: "1", AnalyzerName: "fake", AnalyzerVersion: "1"}, nil))
		corpus, corpusErr := store.PutCorpus(ctx, owner.ID, source.ID, source.ContentHash)
		require.NoError(t, corpusErr)
		_, execErr := pool.Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, handle.RunID, owner.ID, corpus.ID)
		require.NoError(t, execErr)
		_, execErr = pool.Exec(ctx, `UPDATE analysis_runs SET state='completed',corpus_id=$1,completed_at=now() WHERE owner_id=$2 AND id=$3`, corpus.ID, owner.ID, handle.RunID)
		require.NoError(t, execErr)
		_, execErr = pool.Exec(ctx, `UPDATE analysis_run_attempts SET state='failed',finalized_at=now() WHERE run_id=$1`, handle.RunID)
		require.NoError(t, execErr)
		_, execErr = pool.Exec(ctx, `UPDATE river_job SET state='discarded',finalized_at=now() WHERE id=$1`, handle.JobID)
		require.NoError(t, execErr)
		retried, retryErr := service.Retry(ctx, owner.ID, handle.ID)
		require.NoError(t, retryErr)
		assert.Equal(t, handle.RunID, retried.RunID)
		assert.NotEqual(t, handle.JobID, retried.JobID)
	})

	t.Run("completed run without publication is refused", func(t *testing.T) {
		handle := submit(t, "retry-completed-idle")
		_, execErr := pool.Exec(ctx, `UPDATE analysis_runs SET state='completed',completed_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID)
		require.NoError(t, execErr)
		_, retryErr := service.Retry(ctx, owner.ID, handle.ID)
		require.ErrorContains(t, retryErr, "analysis publication does not need retry")
	})

	for _, state := range []string{"failed", "cancelled"} {
		t.Run(state+" run is requeued with history", func(t *testing.T) {
			handle := submit(t, "retry-"+state)
			_, execErr := pool.Exec(ctx, `UPDATE analysis_runs SET state=$3,completed_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID, state)
			require.NoError(t, execErr)
			_, execErr = pool.Exec(ctx, `UPDATE analysis_run_attempts SET state=$2,finalized_at=now() WHERE run_id=$1`, handle.RunID, state)
			require.NoError(t, execErr)
			_, execErr = pool.Exec(ctx, `UPDATE river_job SET state='discarded',finalized_at=now() WHERE id=$1`, handle.JobID)
			require.NoError(t, execErr)
			retried, retryErr := service.Retry(ctx, owner.ID, handle.ID)
			require.NoError(t, retryErr)
			assert.Equal(t, handle.RunID, retried.RunID)
			assert.NotEqual(t, handle.JobID, retried.JobID)
			assert.Equal(t, "queued", runState(t, handle.RunID))
			requireRetryHistory(t, state)
		})
	}
}

// TestAnalysisCancelLegacyJobFollowsRiverLifecycle pins the legacy cancellation
// path: a handle without a snapshot run is cancelled through River directly and
// reports River's state, which cancellation now names as its own step.
func TestAnalysisCancelLegacyJobFollowsRiverLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "analysis-legacy-cancel", false)
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner.ID, "legacy-cancel", "Legacy cancel", "Guten Tag.", "sha256:legacy-cancel")
	require.NoError(t, err)
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	service := NewService(pool, riverClient)
	job, err := riverClient.Insert(ctx, JobArgs{OwnerID: owner.ID, SourceMaterialID: source.ID, ContentHash: source.ContentHash, Language: "de"}, &river.InsertOpts{Queue: Queue, MaxAttempts: 1})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,display_number) VALUES($1,$2,$3,$4,(SELECT COALESCE(MAX(display_number),0)+1 FROM analysis_jobs WHERE owner_id=$2))`, job.Job.ID, owner.ID, source.ID, source.ContentHash)
	require.NoError(t, err)

	cancelled, err := service.Cancel(ctx, owner.ID, job.Job.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCancelled, cancelled.State)
	assert.Empty(t, cancelled.RunID)
}

// TestWorkerRejectsMalformedCorpusTokens pins the per-token validation that
// building a corpus-token row performs, through the worker's public seam: a
// malformed token fails the run rather than persisting a partial corpus.
func TestWorkerRejectsMalformedCorpusTokens(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "malformed-corpus-tokens", false)
	require.NoError(t, err)
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	service := NewService(pool, riverClient)

	tests := []struct {
		name    string
		tokens  []analyzer.Token
		wantErr string
	}{
		{
			name:    "missing dependency",
			tokens:  []analyzer.Token{{Surface: "x", RawLemma: "x", CanonicalLemma: "x", UPOS: "X", Head: 0}},
			wantErr: "dependency is required",
		},
		{
			name:    "head out of range",
			tokens:  []analyzer.Token{{Surface: "x", RawLemma: "x", CanonicalLemma: "x", UPOS: "X", Dependency: "dep", Head: 5}},
			wantErr: "head ordinal 5 is out of range",
		},
		{
			name: "root not self-headed",
			tokens: []analyzer.Token{
				{Surface: "x", RawLemma: "x", CanonicalLemma: "x", UPOS: "X", Dependency: "root", Head: 1},
				{Surface: "y", RawLemma: "y", CanonicalLemma: "y", UPOS: "X", Dependency: "dep", Head: 0},
			},
			wantErr: "root must be self-headed",
		},
		{
			name:    "start offset overflows bigint",
			tokens:  []analyzer.Token{{Surface: "x", RawLemma: "x", CanonicalLemma: "x", UPOS: "X", Dependency: "dep", Head: 0, Location: analyzer.SourceLocation{StartOffset: uint64(math.MaxInt64) + 1}}},
			wantErr: "start offset overflows bigint",
		},
		{
			name:    "end offset overflows bigint",
			tokens:  []analyzer.Token{{Surface: "x", RawLemma: "x", CanonicalLemma: "x", UPOS: "X", Dependency: "dep", Head: 0, Location: analyzer.SourceLocation{EndOffset: uint64(math.MaxInt64) + 1}}},
			wantErr: "end offset overflows bigint",
		},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identifier := fmt.Sprintf("malformed-token-%d", i)
			source, sourceErr := putAnalysisSource(ctx, store, owner.ID, identifier, identifier, "Guten Tag.", "sha256:"+identifier)
			require.NoError(t, sourceErr)
			handle, submitErr := service.SubmitAnalysis(ctx, owner.ID, source.ID)
			require.NoError(t, submitErr)
			args, argsErr := snapshotArgsTx(ctx, pool, owner.ID, handle.RunID)
			require.NoError(t, argsErr)
			unitID := domain.EPUBUnitID(0, identifier)
			fake := &analyzertest.Fake{AnalyzeFunc: func(_ context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
				return analyzer.Result{
					SchemaVersion:        "1.0.0",
					Language:             req.Language,
					Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"},
					NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"},
					Sentences: []analyzer.Sentence{{
						Text:     req.Document.Text,
						Location: analyzer.SourceLocation{SourceDocumentID: unitID, EndOffset: uint64(len([]rune(req.Document.Text)))},
						Tokens:   test.tokens,
					}},
				}, nil
			}}
			worker := &Worker{Pool: pool, Analyzer: fake, Capabilities: analyzertest.ReadyDepparseCapabilityProvider(), Selection: selection.NewService(store)}
			job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: handle.JobID}, Args: args}
			workErr := worker.Work(ctx, job)
			require.ErrorContains(t, workErr, test.wantErr)
			var tokens int
			require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM corpus_tokens WHERE owner_id=$1 AND analysis_run_id=$2`, owner.ID, handle.RunID).Scan(&tokens))
			assert.Zero(t, tokens, "a malformed token must not persist a partial corpus")
		})
	}
}

// TestAnalysisCancelAndRetrySurfaceOperationalFailures pins the write-failure
// branches of the cancellation and retry steps through their public seams, so
// splitting them into named steps keeps failing loudly instead of silently
// leaving a half-cancelled or half-requeued run.
func TestAnalysisCancelAndRetrySurfaceOperationalFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "analysis-cancel-retry-failures", false)
	require.NoError(t, err)
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	service := NewService(pool, riverClient)
	failingInsert := NewService(pool, &failingInsertAnalysisClient{Client: riverClient, err: errors.New("River insert unavailable")})

	putSource := func(t *testing.T, identifier string) domain.SourceMaterial {
		t.Helper()
		source, sourceErr := putAnalysisSource(ctx, store, owner.ID, identifier, identifier, "Guten Tag.", "sha256:"+identifier)
		require.NoError(t, sourceErr)
		return source
	}
	submit := func(t *testing.T, identifier string) Handle {
		t.Helper()
		handle, submitErr := service.SubmitAnalysis(ctx, owner.ID, putSource(t, identifier).ID)
		require.NoError(t, submitErr)
		return handle
	}
	completeRun := func(t *testing.T, handle Handle) {
		t.Helper()
		_, execErr := pool.Exec(ctx, `UPDATE analysis_runs SET state='completed',completed_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID)
		require.NoError(t, execErr)
	}
	rejectUpdate := func(t *testing.T, name, table, message string) {
		t.Helper()
		_, execErr := pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION '%s'; END $$; CREATE TRIGGER %s_trg BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION %s()`, name, message, name, table, name))
		require.NoError(t, execErr)
		t.Cleanup(func() {
			_, dropErr := pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER %s_trg ON %s; DROP FUNCTION %s()`, name, table, name))
			require.NoError(t, dropErr)
		})
	}

	t.Run("legacy cancellation surfaces River failure", func(t *testing.T) {
		source := putSource(t, "cancel-legacy-failure")
		job, insertErr := riverClient.Insert(ctx, JobArgs{OwnerID: owner.ID, SourceMaterialID: source.ID, ContentHash: source.ContentHash, Language: "de"}, &river.InsertOpts{Queue: Queue, MaxAttempts: 1})
		require.NoError(t, insertErr)
		_, execErr := pool.Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,display_number) VALUES($1,$2,$3,$4,(SELECT COALESCE(MAX(display_number),0)+1 FROM analysis_jobs WHERE owner_id=$2))`, job.Job.ID, owner.ID, source.ID, source.ContentHash)
		require.NoError(t, execErr)
		failing := NewService(pool, &failingAnalysisCancellationClient{Client: riverClient, err: errors.New("River cancellation unavailable")})
		_, cancelErr := failing.Cancel(ctx, owner.ID, job.Job.ID)
		require.ErrorContains(t, cancelErr, "cancel analysis job")
	})

	t.Run("completed finalization surfaces attempt write failure", func(t *testing.T) {
		handle := submit(t, "cancel-finalization-attempt")
		completeRun(t, handle)
		rejectUpdate(t, "reject_finalization_attempt", "analysis_run_attempts", "simulated finalization attempt failure")
		_, cancelErr := service.Cancel(ctx, owner.ID, handle.ID)
		require.ErrorContains(t, cancelErr, "simulated finalization attempt failure")
	})

	t.Run("completed finalization logs best-effort River failure", func(t *testing.T) {
		handle := submit(t, "cancel-finalization-cleanup")
		completeRun(t, handle)
		failing := NewService(pool, &failingAnalysisCancellationClient{Client: riverClient, err: errors.New("River cancellation unavailable")})
		cancelled, cancelErr := failing.Cancel(ctx, owner.ID, handle.ID)
		require.NoError(t, cancelErr)
		assert.Equal(t, rivertype.JobStateCancelled, cancelled.State)
		assert.Equal(t, "finalization-cancelled", cancelled.LogicalState)
	})

	t.Run("active cancellation surfaces run write failure", func(t *testing.T) {
		handle := submit(t, "cancel-active-run")
		rejectUpdate(t, "reject_cancel_run", "analysis_runs", "simulated cancel run failure")
		_, cancelErr := service.Cancel(ctx, owner.ID, handle.ID)
		require.ErrorContains(t, cancelErr, "simulated cancel run failure")
	})

	t.Run("active cancellation surfaces attempt write failure", func(t *testing.T) {
		handle := submit(t, "cancel-active-attempt")
		rejectUpdate(t, "reject_cancel_attempt", "analysis_run_attempts", "simulated cancel attempt failure")
		_, cancelErr := service.Cancel(ctx, owner.ID, handle.ID)
		require.ErrorContains(t, cancelErr, "simulated cancel attempt failure")
	})

	t.Run("cancelling a settled run is a no-op", func(t *testing.T) {
		handle := submit(t, "cancel-settled")
		_, execErr := pool.Exec(ctx, `UPDATE analysis_runs SET state='cancelled',completed_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID)
		require.NoError(t, execErr)
		settled, cancelErr := service.Cancel(ctx, owner.ID, handle.ID)
		require.NoError(t, cancelErr)
		assert.Equal(t, rivertype.JobStateCancelled, settled.State)
	})

	t.Run("orphaned running retry surfaces run write failure", func(t *testing.T) {
		handle := submit(t, "retry-orphan-run-write")
		_, execErr := pool.Exec(ctx, `UPDATE analysis_runs SET state='running',started_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID)
		require.NoError(t, execErr)
		_, execErr = pool.Exec(ctx, `DELETE FROM analysis_run_attempts WHERE run_id=$1`, handle.RunID)
		require.NoError(t, execErr)
		_, execErr = pool.Exec(ctx, `DELETE FROM river_job WHERE id=$1`, handle.JobID)
		require.NoError(t, execErr)
		rejectUpdate(t, "reject_retry_fail_run", "analysis_runs", "simulated retry run failure")
		_, retryErr := service.Retry(ctx, owner.ID, handle.ID)
		require.ErrorContains(t, retryErr, "simulated retry run failure")
	})

	t.Run("queued retry surfaces enqueue failure", func(t *testing.T) {
		handle := submit(t, "retry-enqueue")
		_, execErr := pool.Exec(ctx, `DELETE FROM analysis_run_attempts WHERE run_id=$1`, handle.RunID)
		require.NoError(t, execErr)
		_, execErr = pool.Exec(ctx, `DELETE FROM river_job WHERE id=$1`, handle.JobID)
		require.NoError(t, execErr)
		_, retryErr := failingInsert.Retry(ctx, owner.ID, handle.ID)
		require.ErrorContains(t, retryErr, "River insert unavailable")
	})

	t.Run("completed retry surfaces publication enqueue failure", func(t *testing.T) {
		source := putSource(t, "retry-completed-enqueue")
		book, bookErr := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Retry publication failure", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, bookErr)
		require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
		handle, submitErr := service.SubmitAnalysis(ctx, owner.ID, source.ID)
		require.NoError(t, submitErr)
		require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: source.ContentHash, Language: "de", SchemaVersion: "1", NormalizationProfile: "casefold", NormalizationVersion: "1", AnalyzerName: "fake", AnalyzerVersion: "1"}, nil))
		corpus, corpusErr := store.PutCorpus(ctx, owner.ID, source.ID, source.ContentHash)
		require.NoError(t, corpusErr)
		_, execErr := pool.Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, handle.RunID, owner.ID, corpus.ID)
		require.NoError(t, execErr)
		_, execErr = pool.Exec(ctx, `UPDATE analysis_runs SET state='completed',corpus_id=$1,completed_at=now() WHERE owner_id=$2 AND id=$3`, corpus.ID, owner.ID, handle.RunID)
		require.NoError(t, execErr)
		_, execErr = pool.Exec(ctx, `UPDATE analysis_run_attempts SET state='failed',finalized_at=now() WHERE run_id=$1`, handle.RunID)
		require.NoError(t, execErr)
		_, execErr = pool.Exec(ctx, `UPDATE river_job SET state='discarded',finalized_at=now() WHERE id=$1`, handle.JobID)
		require.NoError(t, execErr)
		_, retryErr := failingInsert.Retry(ctx, owner.ID, handle.ID)
		require.ErrorContains(t, retryErr, "River insert unavailable")
	})
}

// TestAnalysisRetrySurfacesSnapshotArgsFailure pins the retry steps' failure to
// read a run's River arguments: a queued run without live work and a completed
// run both surface the read failure instead of returning a zero handle.
func TestAnalysisRetrySurfacesSnapshotArgsFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "analysis-retry-args", false)
	require.NoError(t, err)
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	service := NewService(pool, riverClient)

	submit := func(t *testing.T, identifier string) Handle {
		t.Helper()
		source, sourceErr := putAnalysisSource(ctx, store, owner.ID, identifier, identifier, "Guten Tag.", "sha256:"+identifier)
		require.NoError(t, sourceErr)
		handle, submitErr := service.SubmitAnalysis(ctx, owner.ID, source.ID)
		require.NoError(t, submitErr)
		return handle
	}
	queued := submit(t, "retry-args-queued")
	_, err = pool.Exec(ctx, `DELETE FROM analysis_run_attempts WHERE run_id=$1`, queued.RunID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM river_job WHERE id=$1`, queued.JobID)
	require.NoError(t, err)
	completed := submit(t, "retry-args-completed")
	_, err = pool.Exec(ctx, `UPDATE analysis_runs SET state='completed',completed_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, completed.RunID)
	require.NoError(t, err)

	// Hiding the revision table leaves the run and its handle readable but makes
	// reloading the River arguments fail, which both retry steps must surface.
	_, err = pool.Exec(ctx, `ALTER TABLE source_content_revisions RENAME TO source_content_revisions_hidden`)
	require.NoError(t, err)

	_, err = service.Retry(ctx, owner.ID, queued.ID)
	require.ErrorContains(t, err, "source_content_revisions")
	_, err = service.Retry(ctx, owner.ID, completed.ID)
	require.ErrorContains(t, err, "source_content_revisions")
}
