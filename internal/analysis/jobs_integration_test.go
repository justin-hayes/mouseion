//go:build integration

package analysis

import (
	"context"
	"encoding/json"
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

func TestRiverAnalysisFailsFastWhenDependencyParsingIsUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "depparse-missing", false)
	require.NoError(t, err)
	source, err := putAnalysisSource(ctx, store, owner.ID, "depparse-source", "Dependency parsing", "Hallo Berlin.", "sha256:depparse-missing")
	require.NoError(t, err)
	fake := &analyzertest.Fake{AnalyzeFunc: func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{}, fmt.Errorf("Analyze should not be called")
	}}
	capabilities := analyzertest.CapabilityProvider{Value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{
		Language: "de", SupportedFeatures: []string{"tokenize", "pos", "lemma"}, Ready: true,
	}}}}
	client, err := NewClient(store.Pool(), fake, capabilities, selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	defer client.Stop(context.Background())

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
	defer store.Close()
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
	defer client.Stop(context.Background())
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
	assert.Equal(t, float64(1), provenance["body_matter_start"])
	assert.Equal(t, float64(2), provenance["back_matter_start"])
	assert.Equal(t, float64(1), provenance["selected_unit_count"])
	assert.Equal(t, float64(3), provenance["total_unit_count"])
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
	assert.Equal(t, float64(2), wholeProvenance["selected_unit_count"])
	assert.Equal(t, float64(2), wholeProvenance["total_unit_count"])
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
		return analyzer.Result{SchemaVersion: "1.0.0", Language: req.Language, Analysis: analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"}, NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"}, Sentences: []analyzer.Sentence{{Text: req.Document.Text, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: uint64(len([]rune(req.Document.Text)))}, Tokens: []analyzer.Token{{Surface: "Häuser", RawLemma: "Häuser", CanonicalLemma: "haus", UPOS: "NOUN", Dependency: "root", Head: 0, Morphology: map[string]string{"Number": "Plur"}, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: 6}}}}}}, nil
	}}
	client, err := NewClient(store.Pool(), fake, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
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
		return analyzer.Result{SchemaVersion: "1.0.0", Language: req.Language, Analysis: analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"}, NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"}, Sentences: []analyzer.Sentence{{Text: req.Document.Text, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: uint64(len([]rune(req.Document.Text)))}, Tokens: []analyzer.Token{{Surface: "Haus", RawLemma: "Haus", CanonicalLemma: "haus", UPOS: "NOUN", Dependency: "root", Head: 0, Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: 4}}}}}}, nil
	}}
	client, err := NewClient(store.Pool(), fake, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
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
