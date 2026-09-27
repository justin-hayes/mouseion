//go:build integration

package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurablePreparedDeckRunFreezeTransitionAndAtomicFinalization(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))

	owner, err := store.CreateUser(ctx, "durable-run-owner", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "durable-run-other", false)
	require.NoError(t, err)
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "durable-run-book", Title: "Durable Book", MediaType: "text/plain", ContentHash: "durable-run-hash", Content: []byte("Haus Baum"), FullText: "Haus Baum"}
	err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, source.OwnerID, source.Language, source.SourceIdentifier, source.Title, source.MediaType, source.ContentHash, source.Content, source.FullText).Scan(&source.ID)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	require.NoError(t, err)
	dictionaryPath := filepath.Join(t.TempDir(), "fixture.sqlite")
	dictionaryDB, err := sql.Open("sqlite", dictionaryPath)
	require.NoError(t, err)
	_, err = dictionaryDB.ExecContext(ctx, `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1')`)
	require.NoError(t, err)
	senses := make([]enrichment.LexicalSense, 10)
	for index := range senses {
		senses[index].Gloss = fmt.Sprintf("house meaning %d", index+1)
	}
	senses[0].Gender, senses[0].Article, senses[0].Plural = "Neut", "das", "Häuser"
	sensesJSON, err := json.Marshal(senses)
	require.NoError(t, err)
	_, err = dictionaryDB.ExecContext(ctx, `INSERT INTO entries VALUES ('de', 'haus', 'NOUN', ?, 'Neut', 'das', 'Häuser', '/haʊ̯s/', 'geht · ging · gegangen')`, string(sensesJSON))
	require.NoError(t, err)
	require.NoError(t, dictionaryDB.Close())
	lexicalIndex, err := dictionary.OpenIndex(ctx, dictionaryPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := lexicalIndex.Close(); err != nil {
			t.Errorf("dictionary index cleanup failed: %v", err)
		}
	})
	deck, err := testutil.FreezePresentationDeckWithLexical(ctx, owner.ID, source.Title, []cardexport.Entry{
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "noun", CorpusID: uuid.NewString(), SentenceOrdinal: 7, Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", Gloss: "house", Plural: "Häuser", IPA: "/haʊ̯s/", PrincipalParts: "geht · ging · gegangen", Morphology: `{"Gender":"Neut"}`, SourceDocument: source.Title, FirstEncounter: 10},
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "baum", UPOS: "noun", Sentence: "Der alte Baum trägt heute viele grüne Blätter.", TargetWord: "Baum", Morphology: `{"Gender":"Masc"}`, SourceDocument: source.Title, FirstEncounter: 20},
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "fragment", UPOS: "noun", Sentence: "Fragment.", TargetWord: "Fragment", SourceDocument: source.Title, FirstEncounter: 30},
	}, testutil.PresentationProvider{Name: "openai", Version: "prompt-v3", TargetLanguage: "en"}, lexicalIndex)
	require.NoError(t, err)
	deckWork := deck.WorkProjection()
	keys := make([]enrichment.CacheKey, len(deckWork))
	for i, item := range deckWork {
		candidate := item.RequestCandidate()
		keys[i] = enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "prompt-v3", DictionaryProviderVersion: candidate.DictionaryProviderVersion, SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	}
	snapshot := deck.StorageProjection()
	snapshot.Items[0].Quality.Score = 94
	snapshot.Items[0].Quality.GDEXScore = 0.5
	snapshot.Items[0].Quality.Reasons = []string{"target present", "optimal length"}
	snapshot.Items[2].Quality.Reasons = []string{"too short or fragmented"}
	_, err = store.Put(ctx, enrichment.CacheEntry{CacheKey: keys[0], Translation: "house", FallbackGloss: "operate", SenseSelection: []int{}, SentenceTranslation: "The old house is surprisingly large.", SentenceTranslationTarget: "house", CachedAt: time.Now().UTC()})
	require.NoError(t, err)
	_, err = store.Put(ctx, enrichment.CacheEntry{CacheKey: keys[1], Translation: "tree", CachedAt: time.Now().UTC()})
	require.NoError(t, err)

	params := FreezePreparedDeckRunParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, Projection: snapshot,
		Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "prompt-v3", Endpoint: "/v1/chat/completions", Model: "gpt-test"},
		Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("a", 64), InputBytes: 128, EstimatedPromptTokens: 32, Ordinals: []int{1}}},
	}
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	result, err := store.FreezePreparedDeckRunTx(ctx, tx, params)
	if err != nil {
		rollbackIntegrationTx(t, ctx, tx)
		require.NoError(t, err)
	}
	err = tx.Commit(ctx)
	require.NoError(t, err)
	assert.False(t, result.Existing, "freeze result")
	assert.False(t, result.NeedsFinalizer, "freeze result")
	assert.Equal(t, 1, result.Run.RunNumber, "freeze result")
	assert.Equal(t, 2, result.Run.CandidateCount, "freeze result")
	assert.Equal(t, 1, result.Run.CompletedCount, "freeze result")
	assert.Equal(t, cardexport.RenderInputVersion, result.Run.RenderInputVersion)
	assert.Equal(t, cardexport.PresentationVersion, result.Run.PresentationVersion)
	require.Len(t, result.Chunks, 1, "freeze result")
	assert.Equal(t, domain.PreparedDeckRunTranslating, result.Run.State)
	assert.Equal(t, domain.PreparedDeckTranslationPending, result.Run.TranslationState)
	require.NotEmpty(t, result.Chunks[0].Ordinals)
	assert.Equal(t, 1, result.Chunks[0].Ordinals[0])
	retryTx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	repeated, err := store.FreezePreparedDeckRunTx(ctx, retryTx, params)
	if err != nil {
		rollbackIntegrationTx(t, ctx, retryTx)
		require.NoError(t, err)
	}
	err = retryTx.Commit(ctx)
	require.NoError(t, err)
	assert.True(t, repeated.Existing, "idempotent freeze")
	assert.Equal(t, result.Run.ID, repeated.Run.ID, "idempotent freeze")
	assert.Equal(t, result.ManifestDigest, repeated.ManifestDigest, "idempotent freeze")
	changedMode := params
	changedMode.Config.ExecutionMode = "standard"
	changedModeResult, changedModeErr := func() (FreezePreparedDeckRunResult, error) {
		changedTx, beginErr := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
		if beginErr != nil {
			return FreezePreparedDeckRunResult{}, beginErr
		}
		result, freezeErr := store.FreezePreparedDeckRunTx(ctx, changedTx, changedMode)
		if freezeErr != nil {
			rollbackIntegrationTx(t, ctx, changedTx)
			return result, freezeErr
		}
		return result, changedTx.Commit(ctx)
	}()
	assert.ErrorIs(t, changedModeErr, ErrImmutable) //nolint:testifylint // Changed-mode rejection and the changed-target case are independent.
	assert.False(t, changedModeResult.Existing, "changed execution mode")
	changedTarget := params
	changedTarget.Config.TargetLanguage = "de"
	changedTargetResult, changedTargetErr := func() (FreezePreparedDeckRunResult, error) {
		changedTx, beginErr := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
		if beginErr != nil {
			return FreezePreparedDeckRunResult{}, beginErr
		}
		result, freezeErr := store.FreezePreparedDeckRunTx(ctx, changedTx, changedTarget)
		if freezeErr != nil {
			rollbackIntegrationTx(t, ctx, changedTx)
			return result, freezeErr
		}
		return result, changedTx.Commit(ctx)
	}()
	assert.Error(t, changedTargetErr, "changed target") //nolint:testifylint // Changed-target rejection is independently checked before state inspection.
	assert.False(t, changedTargetResult.Existing, "changed target")
	current, getErr := store.GetDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, getErr)
	assert.Equal(t, domain.DeckPreparationPreparing, current.State)
	assert.Equal(t, result.Run.ID, current.CurrentRunID)
	_, getErr = store.GetPreparedDeckRun(ctx, other.ID, preparation.ID, result.Run.ID)
	assert.ErrorIs(t, getErr, ErrNotFound) //nolint:testifylint // Cross-owner run lookup is independent of the owner-scoped projection load.

	loaded, loadedDigest, err := store.LoadPreparedDeckStorageProjection(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	assert.Equal(t, 94, loaded.Items[0].Quality.Score)
	assert.Equal(t, 0.5, loaded.Items[0].Quality.GDEXScore)
	assert.Equal(t, []string{"target present", "optimal length"}, loaded.Items[0].Quality.Reasons)
	assert.True(t, strings.HasPrefix(loaded.Items[0].Entry.Gloss, "house meaning 1"))
	assert.Equal(t, "fixture-v1", loaded.Items[0].Entry.DictionaryProviderVersion)
	require.Len(t, loaded.Items[0].Entry.CandidateSenses, enrichment.DefaultMaxCandidateSenses)
	for index, candidate := range loaded.Items[0].Entry.CandidateSenses {
		assert.NotEmpty(t, candidate.EvidenceID)
		assert.Equal(t, "wiktionary", candidate.Source)
		assert.Equal(t, "meaning", candidate.Kind)
		assert.Equal(t, "Kaikki.org Wiktextract enwiktionary", candidate.Origin)
		assert.Equal(t, "fixture-v1", candidate.Version)
		assert.Equal(t, "exact_lemma_pos", candidate.MatchStrength)
		assert.Equal(t, fmt.Sprintf("house meaning %d", index+1), candidate.Gloss)
	}
	assert.Equal(t, 2, loaded.Items[0].Entry.OmittedEvidenceCount)
	assert.Equal(t, "exact_lemma_pos", loaded.Items[0].Entry.CandidateSenses[0].MatchStrength)
	status, err := store.GetDeckPreparationStatus(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []domain.DeckPreparationEvidenceCoverage{{Source: "wiktionary", Configured: true, Selected: 3, Matched: 1, Candidates: enrichment.DefaultMaxCandidateSenses, Omitted: 2}}, status.EvidenceCoverage)
	assert.Equal(t, "Häuser", loaded.Items[0].Entry.Plural)
	assert.Equal(t, "/haʊ̯s/", loaded.Items[0].Entry.IPA)
	assert.Equal(t, "geht · ging · gegangen", loaded.Items[0].Entry.PrincipalParts)
	assert.Equal(t, snapshot.Items[0].CorpusID, loaded.Items[0].CorpusID)
	assert.Equal(t, int64(7), loaded.Items[0].SentenceOrdinal)
	assert.Equal(t, cardexport.ManifestQualityOmitted, loaded.Items[2].Disposition)
	assert.Equal(t, []string{"too short or fragmented"}, loaded.Items[2].Quality.Reasons)
	wantDigest, err := snapshot.Digest()
	require.NoError(t, err)
	assert.Equal(t, wantDigest, loadedDigest)
	_, err = cardexport.NewPresentation(nil).Restore(loaded)
	require.NoError(t, err, "manifest round trip")
	records, err := store.LoadPreparedDeckStoredRecords(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, 0, records[0].Ordinal)
	assert.Equal(t, keys[0], records[0].CacheKey)
	assert.True(t, records[0].Found)
	assert.Equal(t, "house", records[0].Entry.Translation)
	assert.Equal(t, 1, records[1].Ordinal)
	assert.Equal(t, keys[1], records[1].CacheKey)
	assert.True(t, records[1].Found)
	assert.Equal(t, "tree", records[1].Entry.Translation)
	_, _, err = store.LoadPreparedDeckStorageProjection(ctx, other.ID, preparation.ID, result.Run.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Cross-owner projection lookup is independent of the record lookup.
	_, err = store.LoadPreparedDeckStoredRecords(ctx, other.ID, preparation.ID, result.Run.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Cross-owner record lookup is independently asserted.
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_manifests SET deck_name='mutated' WHERE run_id=$1`, result.Run.ID)
	assert.Error(t, err, "immutable manifest update succeeded") //nolint:testifylint // Database immutability rejection and subsequent batch setup are independent.
	jobTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	err = store.SetPreparedDeckBatchSubmissionJobTx(ctx, jobTx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 1, 9001)
	if err != nil {
		rollbackIntegrationTx(t, ctx, jobTx)
		require.NoError(t, err)
	}
	err = jobTx.Commit(ctx)
	require.NoError(t, err)
	submissionToken := uuid.NewString()
	chunk, err := store.ClaimPreparedDeckBatchSubmission(ctx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 1, submissionToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, domain.PreparedDeckBatchSubmitting, chunk.State, "claim chunk")
	submitTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	chunk, err = store.RecordPreparedDeckBatchSubmittedTx(ctx, submitTx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 1, submissionToken, "file-input", "batch-1", time.Now().UTC(), func(context.Context, pgx.Tx, domain.PreparedDeckBatchChunk) (int64, error) {
		return 9003, nil
	})
	if err != nil {
		rollbackIntegrationTx(t, ctx, submitTx)
		require.NoError(t, err)
	}
	assert.Equal(t, domain.PreparedDeckBatchSubmitted, chunk.State, "submitted chunk")
	assert.Equal(t, "batch-1", chunk.BatchID, "submitted chunk")
	assert.Equal(t, int64(9003), chunk.ReconciliationJobID, "submitted chunk")
	err = submitTx.Commit(ctx)
	require.NoError(t, err)
	chunk, err = store.AssignPreparedDeckBatchReconciliationJob(ctx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 0, 9002)
	require.NoError(t, err)
	assert.Equal(t, 1, chunk.ReconciliationGeneration, "assign reconciliation")
	assert.Equal(t, int64(9002), chunk.ReconciliationJobID, "assign reconciliation")
	reconciliationToken := uuid.NewString()
	chunk, err = store.ClaimPreparedDeckBatchReconciliation(ctx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 1, reconciliationToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, domain.PreparedDeckBatchReconciling, chunk.State, "claim reconciliation")
	chunk, err = store.FinishPreparedDeckBatchReconciliation(ctx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 1, reconciliationToken, PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchCompleted, ProviderStatus: "completed", OutputFileID: "file-output", CompletedCount: 1, InputTokens: 10, OutputTokens: 5})
	require.NoError(t, err)
	assert.Equal(t, domain.PreparedDeckBatchCompleted, chunk.State, "finish reconciliation")
	assert.Equal(t, int64(15), chunk.TotalTokens, "finish reconciliation")
	assert.NotNil(t, chunk.ReconciledAt, "finish reconciliation")

	claimToken := uuid.NewString()
	outcome, err := store.ClaimPreparedDeckTranslationOutcome(ctx, owner.ID, preparation.ID, result.Run.ID, 1, 0, claimToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, domain.PreparedDeckOutcomeRunning, outcome.State, "claim outcome")
	_, err = store.Put(ctx, enrichment.CacheEntry{CacheKey: keys[1], Translation: "tree", SentenceTranslation: "The old tree has many green leaves today.", SentenceTranslationTarget: "tree", CachedAt: time.Now().UTC()})
	require.NoError(t, err)
	finalizerInserted := false
	outcome, run, err := store.FinishPreparedDeckTranslationOutcome(ctx, owner.ID, preparation.ID, result.Run.ID, 1, 0, claimToken, PreparedDeckOutcomeTerminalUpdate{State: domain.PreparedDeckOutcomeCompleted, ProviderAttempt: true, ProviderCall: true, ProviderLatency: 25 * time.Millisecond}, func(_ context.Context, _ pgx.Tx, finalized domain.PreparedDeckRun) error {
		finalizerInserted = finalized.State == domain.PreparedDeckRunFinalizing
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcome.State, "finish outcome")
	assert.Equal(t, domain.PreparedDeckRunFinalizing, run.State, "finish outcome")
	assert.Equal(t, domain.PreparedDeckTranslationCompleted, run.TranslationState, "finish outcome")
	assert.Equal(t, 2, run.CompletedCount, "finish outcome")
	assert.True(t, finalizerInserted, "finish outcome")

	frozenProjection, stored, err := store.LoadPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	require.Len(t, stored, 2, "finalization inputs")
	assert.Equal(t, keys[0], stored[0].CacheKey, "finalization inputs")
	assert.Equal(t, keys[1], stored[1].CacheKey, "finalization inputs")
	deck, err = cardexport.NewPresentation(nil).Restore(frozenProjection)
	require.NoError(t, err)
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, stored, cardexport.RunFacts{Consent: result.Run.ExternalTranslationConsent, Configured: result.Run.ExternalTranslationConfigured, ExecutionMode: string(result.Run.ExecutionMode), TargetLanguage: result.Run.TargetLanguage, Provider: result.Run.Provider, ProviderVersion: result.Run.ProviderVersion})
	require.NoError(t, err)
	assert.Equal(t, "operate", artifact.Generated[0].Note.Gloss, "cached none-fit selection was not applied")
	assert.Equal(t, 1, artifact.Completeness.CardsWithFallbackGloss, "cached fallback gloss was not counted")
	finalizationToken := uuid.NewString()
	claimed, err := store.ClaimPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID, 0, finalizationToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, finalizationToken, claimed.FinalizationClaimToken, "claim finalization")
	ready, err := store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, finalizationToken, artifact)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, ready.State)
	assert.Equal(t, result.Run.ID, ready.CurrentRunID)
	assert.Equal(t, 2, ready.TotalCards)
	assert.Equal(t, 1, ready.CardsWithFallbackGloss, "fallback gloss count was not durable")
	assert.Equal(t, 1, ready.QualityOmissions)
	assert.Equal(t, cardexport.RenderInputVersion, ready.RenderInputVersion)
	assert.Equal(t, cardexport.PresentationVersion, ready.PresentationVersion)
	completed, err := store.GetPreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PreparedDeckRunCompleted, completed.State)
	assert.NotNil(t, completed.CompletedAt)
	assert.Equal(t, cardexport.RenderInputVersion, completed.RenderInputVersion)
	assert.Equal(t, cardexport.PresentationVersion, completed.PresentationVersion)
	_, err = store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, finalizationToken, artifact)
	require.NoError(t, err, "idempotent completion")
	retried, retryErr := store.ClaimPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID, 0, uuid.NewString(), time.Now().UTC().Add(time.Minute))
	require.NoError(t, retryErr)
	assert.Equal(t, domain.PreparedDeckRunCompleted, retried.State, "post-commit finalizer retry")
	changed := artifact
	changed.APKG = []byte("different")
	_, err = store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, finalizationToken, changed)
	assert.ErrorIs(t, err, ErrImmutable) //nolint:testifylint // Immutable completion rejection is independent of the following count queries.
	var cards, generated int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, owner.ID).Scan(&cards)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&generated)
	require.NoError(t, err)
	assert.Equal(t, 2, cards, "published cards")
	assert.Equal(t, 2, generated, "published cards")
}

func TestSupersedePreparedDeckArtifactRerendersCompletedRunWithoutChangingProvenance(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "rerender-owner", false)
	require.NoError(t, err)
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "rerender-book", Title: "Rerender Book", MediaType: "text/plain", ContentHash: "rerender-hash", Content: []byte("Haus"), FullText: "Haus"}
	err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, source.OwnerID, source.Language, source.SourceIdentifier, source.Title, source.MediaType, source.ContentHash, source.Content, source.FullText).Scan(&source.ID)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	require.NoError(t, err)
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, []cardexport.Entry{{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus steht heute am Fluss.", TargetWord: "Haus", SourceDocument: source.Title, FirstEncounter: 1}}, testutil.PresentationProvider{})
	require.NoError(t, err)
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	result, err := store.FreezePreparedDeckRunTx(ctx, tx, FreezePreparedDeckRunParams{OwnerID: owner.ID, PreparationID: preparation.ID, Projection: deck.StorageProjection()})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	claimToken := uuid.NewString()
	_, err = store.ClaimPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID, 0, claimToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	frozen, stored, err := store.LoadPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	deck, err = cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, stored, cardexport.RunFacts{Consent: result.Run.ExternalTranslationConsent, Configured: result.Run.ExternalTranslationConfigured, ExecutionMode: string(result.Run.ExecutionMode), TargetLanguage: result.Run.TargetLanguage, Provider: result.Run.Provider, ProviderVersion: result.Run.ProviderVersion})
	require.NoError(t, err)
	_, err = store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, claimToken, artifact)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET presentation_version=0 WHERE owner_id=$1 AND id=$2`, owner.ID, preparation.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET presentation_version=0 WHERE owner_id=$1 AND preparation_id=$2 AND id=$3`, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE cards SET front='stale projection' WHERE owner_id=$1`, owner.ID)
	require.NoError(t, err)
	frozen, stored, err = store.LoadPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	deck, err = cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	rerendered, _, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, stored, cardexport.RunFacts{Consent: result.Run.ExternalTranslationConsent, Configured: result.Run.ExternalTranslationConfigured, ExecutionMode: string(result.Run.ExecutionMode), TargetLanguage: result.Run.TargetLanguage, Provider: result.Run.Provider, ProviderVersion: result.Run.ProviderVersion})
	require.NoError(t, err)
	rerendered.APKG = []byte("rerendered artifact")
	updated, err := store.SupersedePreparedDeckArtifact(ctx, owner.ID, preparation.ID, result.Run.ID, cardexport.PresentationVersion, rerendered)
	require.NoError(t, err)
	assert.Equal(t, 2, updated.DeckRevision)
	assert.Equal(t, cardexport.PresentationVersion, updated.PresentationVersion)
	assert.Equal(t, preparation.ID, updated.ID)
	var front string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT front FROM cards WHERE owner_id=$1`, owner.ID).Scan(&front))
	assert.NotEqual(t, "stale projection", front)

	again, err := store.SupersedePreparedDeckArtifact(ctx, owner.ID, preparation.ID, result.Run.ID, cardexport.PresentationVersion, rerendered)
	require.NoError(t, err)
	assert.Equal(t, 2, again.DeckRevision)
	assert.Equal(t, []byte("rerendered artifact"), again.Artifact)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET render_input_version=0 WHERE owner_id=$1 AND id=$2`, owner.ID, preparation.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET render_input_version=0 WHERE owner_id=$1 AND id=$2`, owner.ID, result.Run.ID)
	require.NoError(t, err)
	updatedInput, err := store.SupersedePreparedDeckArtifact(ctx, owner.ID, preparation.ID, result.Run.ID, cardexport.PresentationVersion, rerendered)
	require.NoError(t, err)
	assert.Equal(t, 3, updatedInput.DeckRevision)
	assert.Equal(t, cardexport.RenderInputVersion, updatedInput.RenderInputVersion)
}

func TestDurablePreparedDeckCancellationFencesClaimsAndRetryCreatesNewRun(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "durable-cancel-owner", false)
	require.NoError(t, err)
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "durable-cancel-book", Title: "Cancel Book", MediaType: "text/plain", ContentHash: "durable-cancel-hash", Content: []byte("Haus"), FullText: "Haus"}
	err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, source.OwnerID, source.Language, source.SourceIdentifier, source.Title, source.MediaType, source.ContentHash, source.Content, source.FullText).Scan(&source.ID)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	require.NoError(t, err)
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, []cardexport.Entry{{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: source.Title, FirstEncounter: 1}}, testutil.PresentationProvider{Name: "openai", Version: "v1", TargetLanguage: "en"})
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 1)
	candidate := work[0].RequestCandidate()
	key := enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	params := FreezePreparedDeckRunParams{OwnerID: owner.ID, PreparationID: preparation.ID, Projection: deck.StorageProjection(), Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions", Model: "gpt-test"}, Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("b", 64), InputBytes: 64, Ordinals: []int{0}}}}
	freeze := func() FreezePreparedDeckRunResult {
		t.Helper()
		tx, beginErr := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
		require.NoError(t, beginErr)
		result, freezeErr := store.FreezePreparedDeckRunTx(ctx, tx, params)
		if freezeErr != nil {
			rollbackIntegrationTx(t, ctx, tx)
			require.NoError(t, freezeErr)
		}
		require.NoError(t, tx.Commit(ctx))
		return result
	}
	first := freeze()
	jobTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	err = store.SetPreparedDeckBatchSubmissionJobTx(ctx, jobTx, owner.ID, preparation.ID, first.Run.ID, first.Chunks[0].ID, 1, 9101)
	if err != nil {
		rollbackIntegrationTx(t, ctx, jobTx)
		require.NoError(t, err)
	}
	err = jobTx.Commit(ctx)
	require.NoError(t, err)
	submissionToken := uuid.NewString()
	_, err = store.ClaimPreparedDeckBatchSubmission(ctx, owner.ID, preparation.ID, first.Run.ID, first.Chunks[0].ID, 1, submissionToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	_, err = store.RecordPreparedDeckBatchSubmitted(ctx, owner.ID, preparation.ID, first.Run.ID, first.Chunks[0].ID, 1, submissionToken, "cancel-file", "cancel-batch", time.Now().UTC())
	require.NoError(t, err)
	reconciliationToken := uuid.NewString()
	_, err = store.ClaimPreparedDeckBatchReconciliation(ctx, owner.ID, preparation.ID, first.Run.ID, first.Chunks[0].ID, 0, reconciliationToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	_, err = store.CancelCurrentPreparedDeckRun(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	lateEntry := enrichment.CacheEntry{CacheKey: key, Translation: "late house", CachedAt: time.Now().UTC()}
	_, err = store.ReconcilePreparedDeckBatch(ctx, PreparedDeckBatchReconcileParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, RunID: first.Run.ID, ChunkID: first.Chunks[0].ID, ClaimToken: reconciliationToken,
		Chunk: PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchCompleted, ProviderStatus: "completed", CompletedCount: 1},
		Items: []PreparedDeckBatchItemReconciliation{{Ordinal: 0, State: domain.PreparedDeckOutcomeCompleted, CacheEntry: &lateEntry}},
	}, nil, nil)
	assert.ErrorIs(t, err, ErrPreparedDeckClaimLost) //nolint:testifylint // Claim-loss classification is independent of the later recovery assertions.
	_, found, cacheErr := store.Get(ctx, key)
	require.NoError(t, cacheErr)
	assert.False(t, found, "late cancelled result reached cache")
	_, err = store.ClaimPreparedDeckTranslationOutcome(ctx, owner.ID, preparation.ID, first.Run.ID, 0, 0, uuid.NewString(), time.Now().UTC().Add(time.Minute))
	assert.True(t, errors.Is(err, ErrPreparedDeckClaimLost) || errors.Is(err, ErrInvalidTransition), "cancelled outcome claim error=%v", err)
	_, err = store.RetryDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	second := freeze()
	assert.NotEqual(t, first.Run.ID, second.Run.ID, "manual retry did not create new run")
	assert.Equal(t, 2, second.Run.RunNumber, "manual retry did not create new run")
}

func TestPreparedDeckBatchReconciliationRetainsPartialSuccessAndExhaustsTwoGenerations(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "batch-reconcile-owner", false)
	require.NoError(t, err)
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "batch-reconcile-book", Title: "Batch Reconcile", MediaType: "text/plain", ContentHash: "batch-reconcile-hash", Content: []byte("Haus Baum"), FullText: "Haus Baum"}
	err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, source.OwnerID, source.Language, source.SourceIdentifier, source.Title, source.MediaType, source.ContentHash, source.Content, source.FullText).Scan(&source.ID)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	require.NoError(t, err)
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, []cardexport.Entry{
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: source.Title, FirstEncounter: 1},
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", Sentence: "Der alte Baum trägt heute viele grüne Blätter.", TargetWord: "Baum", SourceDocument: source.Title, FirstEncounter: 2},
	}, testutil.PresentationProvider{Name: "openai", Version: "v1", TargetLanguage: "en"})
	require.NoError(t, err)
	deckWork := deck.WorkProjection()
	keys := make([]enrichment.CacheKey, len(deckWork))
	for i, item := range deckWork {
		candidate := item.RequestCandidate()
		keys[i] = enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	}
	params := FreezePreparedDeckRunParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, Projection: deck.StorageProjection(),
		Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions", Model: "gpt-test", MaxBatchGenerations: 2, MaxProviderAttempts: 2},
		Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("c", 64), InputBytes: 128, Ordinals: []int{0, 1}}},
	}
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	frozen, err := store.FreezePreparedDeckRunTx(ctx, tx, params)
	if err != nil {
		rollbackIntegrationTx(t, ctx, tx)
		require.NoError(t, err)
	}
	err = store.SetPreparedDeckBatchSubmissionJobTx(ctx, tx, owner.ID, preparation.ID, frozen.Run.ID, frozen.Chunks[0].ID, 1, 9201)
	if err != nil {
		rollbackIntegrationTx(t, ctx, tx)
		require.NoError(t, err)
	}
	err = tx.Commit(ctx)
	require.NoError(t, err)
	claimAndSubmit := func(chunk domain.PreparedDeckBatchChunk, inputFile, batchID string) string {
		t.Helper()
		submissionToken := uuid.NewString()
		_, claimErr := store.ClaimPreparedDeckBatchSubmission(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunk.ID, chunk.Generation, submissionToken, time.Now().UTC().Add(time.Minute))
		require.NoError(t, claimErr)
		_, submitErr := store.RecordPreparedDeckBatchSubmitted(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunk.ID, chunk.Generation, submissionToken, inputFile, batchID, time.Now().UTC())
		require.NoError(t, submitErr)
		reconcileToken := uuid.NewString()
		_, claimErr = store.ClaimPreparedDeckBatchReconciliation(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunk.ID, 0, reconcileToken, time.Now().UTC().Add(time.Minute))
		require.NoError(t, claimErr)
		return reconcileToken
	}
	firstToken := claimAndSubmit(frozen.Chunks[0], "input-one", "batch-one")
	// A different run may populate the exact key after this Batch was submitted.
	// The immutable first writer remains authoritative even when the provider
	// later returns different valid text.
	firstWriter := enrichment.CacheEntry{CacheKey: keys[0], Translation: "first house", FallbackGloss: "first writer", SentenceTranslation: "First cached sentence.", SentenceTranslationTarget: "house", CachedAt: time.Now().UTC().Add(-time.Minute)}
	firstWriter, err = store.Put(ctx, firstWriter)
	require.NoError(t, err)
	cacheEntry := enrichment.CacheEntry{CacheKey: keys[0], Translation: "provider house", FallbackGloss: "provider result", SentenceTranslation: "The old house is surprisingly large.", SentenceTranslationTarget: "house", CachedAt: time.Now().UTC()}
	firstResult, err := store.ReconcilePreparedDeckBatch(ctx, PreparedDeckBatchReconcileParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, RunID: frozen.Run.ID, ChunkID: frozen.Chunks[0].ID, ClaimToken: firstToken,
		Chunk: PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchCompleted, ProviderStatus: "expired", OutputFileID: "output-one", CompletedCount: 1, ExpiredCount: 1},
		Items: []PreparedDeckBatchItemReconciliation{
			{Ordinal: 0, State: domain.PreparedDeckOutcomeCompleted, CacheEntry: &cacheEntry},
			{Ordinal: 1, State: domain.PreparedDeckOutcomePending, ErrorClass: "expired", ErrorCode: "expired"},
		},
		RetryChunks: []PreparedDeckBatchChunkPlan{{Generation: 2, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "retry", InputDigest: strings.Repeat("d", 64), InputBytes: 64, Ordinals: []int{1}}},
	}, func(context.Context, pgx.Tx, domain.PreparedDeckBatchChunk) (int64, error) { return 9202, nil }, func(context.Context, pgx.Tx, domain.PreparedDeckRun) error {
		return errors.New("finalizer dispatched before retries ended")
	})
	require.NoError(t, err)
	require.Len(t, firstResult.RetryChunks, 1, "first reconciliation")
	assert.Equal(t, 2, firstResult.RetryChunks[0].Generation, "first reconciliation")
	require.Len(t, firstResult.RetryChunks[0].Ordinals, 1, "first reconciliation")
	assert.Equal(t, 1, firstResult.RetryChunks[0].Ordinals[0], "first reconciliation")
	stored, found, getErr := store.Get(ctx, keys[0])
	require.NoError(t, getErr)
	assert.True(t, found, "exact cache stored")
	assert.Equal(t, firstWriter.Translation, stored.Translation, "exact cache stored")
	assert.True(t, stored.CachedAt.Equal(firstWriter.CachedAt), "exact cache stored")
	outcomes, err := store.ListPreparedDeckTranslationOutcomes(ctx, owner.ID, preparation.ID, frozen.Run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 2, "partial outcomes")
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State, "partial outcomes")
	assert.Equal(t, domain.PreparedDeckOutcomePending, outcomes[1].State, "partial outcomes")
	assert.Equal(t, 1, outcomes[1].ProviderAttemptCount, "partial outcomes")
	second := firstResult.RetryChunks[0]
	secondToken := claimAndSubmit(second, "input-two", "batch-two")
	finalizerCount := 0
	secondResult, err := store.ReconcilePreparedDeckBatch(ctx, PreparedDeckBatchReconcileParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, RunID: frozen.Run.ID, ChunkID: second.ID, ClaimToken: secondToken,
		Chunk: PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchCompleted, ProviderStatus: "completed", ErrorFileID: "error-two", FailedCount: 1},
		Items: []PreparedDeckBatchItemReconciliation{{Ordinal: 1, State: domain.PreparedDeckOutcomeFailed, ErrorClass: "retry_exhausted", ErrorCode: "timeout"}},
	}, nil, func(ctx context.Context, tx pgx.Tx, run domain.PreparedDeckRun) error {
		finalizerCount++
		return store.SetPreparedDeckFinalizationJobTx(ctx, tx, owner.ID, preparation.ID, run.ID, run.FinalizationDispatchGeneration, 9203)
	})
	require.NoError(t, err)
	assert.Equal(t, domain.PreparedDeckRunFinalizing, secondResult.Run.State, "second reconciliation")
	assert.Equal(t, 1, secondResult.Run.CompletedCount, "second reconciliation")
	assert.Equal(t, 1, secondResult.Run.FailedCount, "second reconciliation")
	assert.Equal(t, 1, finalizerCount, "second reconciliation")
	_, err = store.ReconcilePreparedDeckBatch(ctx, PreparedDeckBatchReconcileParams{OwnerID: owner.ID, PreparationID: preparation.ID, RunID: frozen.Run.ID, ChunkID: second.ID, ClaimToken: secondToken, Chunk: PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchCompleted, ProviderStatus: "completed"}}, nil, nil)
	assert.ErrorIs(t, err, ErrPreparedDeckClaimLost) //nolint:testifylint // Claim-loss classification is independent of the later recovery assertions.
	work, err := store.ListPreparedDeckRecoveryWork(ctx, 100)
	require.NoError(t, err)
	foundFinalizer := false
	for _, item := range work {
		foundFinalizer = foundFinalizer || (item.Kind == "finalizer" && item.RunID == frozen.Run.ID)
	}
	assert.True(t, foundFinalizer, "recovery work")
}

func TestDurablePreparedDeckManifestPreservesFrozenParseForBolding(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))

	owner, err := store.CreateUser(ctx, "durable-parse-owner", false)
	require.NoError(t, err)
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "durable-parse-book", Title: "Durable Parse Book", MediaType: "text/plain", ContentHash: "durable-parse-hash", Content: []byte("Haus"), FullText: "Haus"}
	err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, source.OwnerID, source.Language, source.SourceIdentifier, source.Title, source.MediaType, source.ContentHash, source.Content, source.FullText).Scan(&source.ID)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	require.NoError(t, err)

	const sentence = "Im Haus des Erpressers strahlten ihre Schwestern sie an."
	entry := cardexport.Entry{
		Language: "de", CanonicalLemma: "anstrahlen", UPOS: "VERB",
		Sentence: sentence, TargetWord: "strahlten", SourceDocument: source.Title,
		SentenceTokens: []analyzer.Token{
			{Surface: "Im", UPOS: "ADP", Dependency: "case", Head: 1},
			{Surface: "Haus", UPOS: "NOUN", Dependency: "obl", Head: 4},
			{Surface: "des", UPOS: "DET", Dependency: "det", Head: 3},
			{Surface: "Erpressers", UPOS: "NOUN", Dependency: "nmod", Head: 1},
			{Surface: "strahlten", UPOS: "VERB", Dependency: "root", Head: 4, Morphology: map[string]string{"VerbForm": "Fin"}},
			{Surface: "ihre", UPOS: "DET", Dependency: "det", Head: 6},
			{Surface: "Schwestern", UPOS: "NOUN", Dependency: "nsubj", Head: 4},
			{Surface: "sie", UPOS: "PRON", Dependency: "obj", Head: 4},
			{Surface: "an", UPOS: "ADV", Dependency: "compound:prt", Head: 4},
		},
	}
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, []cardexport.Entry{entry}, testutil.PresentationProvider{})
	require.NoError(t, err)
	snapshot := deck.StorageProjection()
	require.Len(t, snapshot.Items, 1)
	require.True(t, snapshot.Items[0].Quality.Accepted, "sentence was quality-omitted: %+v", snapshot.Items[0].Quality)

	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	result, err := store.FreezePreparedDeckRunTx(ctx, tx, FreezePreparedDeckRunParams{OwnerID: owner.ID, PreparationID: preparation.ID, Projection: snapshot, Config: PreparedDeckRunConfig{ExecutionMode: "batch"}})
	if err != nil {
		rollbackIntegrationTx(t, ctx, tx)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit(ctx))

	loaded, _, err := store.LoadPreparedDeckStorageProjection(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	require.Len(t, loaded.Items, 1)
	require.Equal(t, snapshot.Items[0].Entry.SentenceTokens, loaded.Items[0].Entry.SentenceTokens, "durable manifest lost the frozen parse")

	deck, err = cardexport.NewPresentation(nil).Restore(loaded)
	require.NoError(t, err)
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, nil, cardexport.RunFacts{})
	require.NoError(t, err)
	assert.Contains(t, artifact.TSV, "Im Haus des Erpressers <b>strahlten</b> ihre Schwestern sie <b>an</b>.")
}
