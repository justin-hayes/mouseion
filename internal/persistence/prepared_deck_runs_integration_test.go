//go:build integration

package persistence

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurablePreparedDeckRunFreezeTransitionAndAtomicFinalization(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()

	owner, err := store.CreateUser(ctx, "durable-run-owner", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "durable-run-other", false)
	require.NoError(t, err)
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "durable-run-book", Title: "Durable Book", MediaType: "text/plain", ContentHash: "durable-run-hash", Content: []byte("Haus Baum"), FullText: "Haus Baum"}
	err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, source.OwnerID, source.Language, source.SourceIdentifier, source.Title, source.MediaType, source.ContentHash, source.Content, source.FullText).Scan(&source.ID)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	require.NoError(t, err)
	for _, lemma := range []string{"haus", "baum"} {
		_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,'de',$2,'NOUN','candidate')`, owner.ID, lemma)
		require.NoError(t, err)
	}

	manifest := cardexport.NewManifest(owner.ID, source.Title, []cardexport.Entry{
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "noun", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", Gloss: "house", Plural: "Häuser", DictionaryProviderVersion: "fixture-v1", Morphology: `{"Gender":"Neut"}`, SourceDocument: source.Title, FirstEncounter: 10},
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "baum", UPOS: "noun", Sentence: "Der alte Baum trägt heute viele grüne Blätter.", TargetWord: "Baum", Morphology: `{"Gender":"Masc"}`, SourceDocument: source.Title, FirstEncounter: 20},
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "fragment", UPOS: "noun", Sentence: "Fragment.", TargetWord: "Fragment", SourceDocument: source.Title, FirstEncounter: 30},
	})
	candidates := manifest.EnrichmentCandidates()
	keys := make([]enrichment.CacheKey, len(candidates))
	for i, candidate := range candidates {
		keys[i] = enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "prompt-v3", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	}
	manifest, err = manifest.BindCacheKeys(keys)
	require.NoError(t, err)
	snapshot := manifest.Snapshot()
	snapshot.Items[0].Quality.Score = 94
	snapshot.Items[0].Quality.GDEXScore = 0.5
	snapshot.Items[0].Quality.Reasons = []string{"target present", "optimal length"}
	snapshot.Items[2].Quality.Reasons = []string{"too short or fragmented"}
	_, err = store.Put(ctx, enrichment.CacheEntry{CacheKey: keys[0], Translation: "house", SentenceTranslation: "The old house is surprisingly large.", SentenceTranslationTarget: "house", CachedAt: time.Now().UTC()})
	require.NoError(t, err)

	params := FreezePreparedDeckRunParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, Manifest: snapshot,
		Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "prompt-v3", Endpoint: "/v1/chat/completions", Model: "gpt-test"},
		Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("a", 64), InputBytes: 128, EstimatedPromptTokens: 32, Ordinals: []int{1}}},
	}
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	result, err := store.FreezePreparedDeckRunTx(ctx, tx, params)
	if err != nil {
		_ = tx.Rollback(ctx)
		require.NoError(t, err)
	}
	err = tx.Commit(ctx)
	require.NoError(t, err)
	assert.False(t, result.Existing, "freeze result")
	assert.False(t, result.NeedsFinalizer, "freeze result")
	assert.Equal(t, 1, result.Run.RunNumber, "freeze result")
	assert.Equal(t, 2, result.Run.CandidateCount, "freeze result")
	assert.Equal(t, 1, result.Run.CompletedCount, "freeze result")
	require.Len(t, result.Chunks, 1, "freeze result")
	assert.Equal(t, domain.PreparedDeckRunTranslating, result.Run.State)
	assert.Equal(t, domain.PreparedDeckTranslationPending, result.Run.TranslationState)
	require.NotEmpty(t, result.Chunks[0].Ordinals)
	assert.Equal(t, 1, result.Chunks[0].Ordinals[0])
	retryTx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	repeated, err := store.FreezePreparedDeckRunTx(ctx, retryTx, params)
	if err != nil {
		_ = retryTx.Rollback(ctx)
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
			_ = changedTx.Rollback(ctx)
			return result, freezeErr
		}
		return result, changedTx.Commit(ctx)
	}()
	assert.ErrorIs(t, changedModeErr, ErrImmutable)
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
			_ = changedTx.Rollback(ctx)
			return result, freezeErr
		}
		return result, changedTx.Commit(ctx)
	}()
	assert.Error(t, changedTargetErr, "changed target")
	assert.False(t, changedTargetResult.Existing, "changed target")
	current, getErr := store.GetDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, getErr)
	assert.Equal(t, domain.DeckPreparationPreparing, current.State)
	assert.Equal(t, result.Run.ID, current.CurrentRunID)
	_, getErr = store.GetPreparedDeckRun(ctx, other.ID, preparation.ID, result.Run.ID)
	assert.ErrorIs(t, getErr, ErrNotFound)

	loaded, loadedDigest, err := store.LoadPreparedDeckManifest(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	assert.Equal(t, 94, loaded.Items[0].Quality.Score)
	assert.Equal(t, 0.5, loaded.Items[0].Quality.GDEXScore)
	assert.Equal(t, []string{"target present", "optimal length"}, loaded.Items[0].Quality.Reasons)
	assert.Equal(t, "house", loaded.Items[0].Entry.Gloss)
	assert.Equal(t, "fixture-v1", loaded.Items[0].Entry.DictionaryProviderVersion)
	assert.Equal(t, "Häuser", loaded.Items[0].Entry.Plural)
	assert.Equal(t, cardexport.ManifestQualityOmitted, loaded.Items[2].Disposition)
	assert.Equal(t, []string{"too short or fragmented"}, loaded.Items[2].Quality.Reasons)
	wantDigest, err := snapshot.Digest()
	require.NoError(t, err)
	assert.Equal(t, wantDigest, loadedDigest)
	_, err = cardexport.ManifestFromSnapshot(loaded)
	require.NoError(t, err, "manifest round trip")
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_manifests SET deck_name='mutated' WHERE run_id=$1`, result.Run.ID)
	assert.Error(t, err, "immutable manifest update succeeded")
	jobTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	err = store.SetPreparedDeckBatchSubmissionJobTx(ctx, jobTx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 1, 9001)
	if err != nil {
		_ = jobTx.Rollback(ctx)
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
		_ = submitTx.Rollback(ctx)
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

	frozenManifest, exact, err := store.LoadPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	require.Len(t, exact, 2, "finalization inputs")
	assert.Equal(t, keys[0], exact[0].CacheKey, "finalization inputs")
	assert.Equal(t, keys[1], exact[1].CacheKey, "finalization inputs")
	artifact, err := cardexport.NewService(store).RenderManifest(ctx, frozenManifest, exact)
	require.NoError(t, err)
	finalizationToken := uuid.NewString()
	claimed, err := store.ClaimPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID, 0, finalizationToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, finalizationToken, claimed.FinalizationClaimToken, "claim finalization")
	ready, err := store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, finalizationToken, artifact)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, ready.State)
	assert.Equal(t, result.Run.ID, ready.CurrentRunID)
	assert.Equal(t, 2, ready.TotalCards)
	assert.Equal(t, 1, ready.QualityOmissions)
	completed, err := store.GetPreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PreparedDeckRunCompleted, completed.State)
	assert.NotNil(t, completed.CompletedAt)
	_, err = store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, finalizationToken, artifact)
	require.NoError(t, err, "idempotent completion")
	retried, retryErr := store.ClaimPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID, 0, uuid.NewString(), time.Now().UTC().Add(time.Minute))
	require.NoError(t, retryErr)
	assert.Equal(t, domain.PreparedDeckRunCompleted, retried.State, "post-commit finalizer retry")
	changed := artifact
	changed.APKG = []byte("different")
	_, err = store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, finalizationToken, changed)
	assert.ErrorIs(t, err, ErrImmutable)
	var cards, generated int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, owner.ID).Scan(&cards)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&generated)
	require.NoError(t, err)
	assert.Equal(t, 2, cards, "published cards")
	assert.Equal(t, 2, generated, "published cards")
}

func TestDurablePreparedDeckCancellationFencesClaimsAndRetryCreatesNewRun(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()
	owner, err := store.CreateUser(ctx, "durable-cancel-owner", false)
	require.NoError(t, err)
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "durable-cancel-book", Title: "Cancel Book", MediaType: "text/plain", ContentHash: "durable-cancel-hash", Content: []byte("Haus"), FullText: "Haus"}
	err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, source.OwnerID, source.Language, source.SourceIdentifier, source.Title, source.MediaType, source.ContentHash, source.Content, source.FullText).Scan(&source.ID)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	require.NoError(t, err)
	manifest := cardexport.NewManifest(owner.ID, source.Title, []cardexport.Entry{{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: source.Title, FirstEncounter: 1}})
	candidate := manifest.EnrichmentCandidates()[0]
	key := enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	manifest, err = manifest.BindCacheKeys([]enrichment.CacheKey{key})
	require.NoError(t, err)
	params := FreezePreparedDeckRunParams{OwnerID: owner.ID, PreparationID: preparation.ID, Manifest: manifest.Snapshot(), Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions", Model: "gpt-test"}, Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("b", 64), InputBytes: 64, Ordinals: []int{0}}}}
	freeze := func() FreezePreparedDeckRunResult {
		t.Helper()
		tx, beginErr := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
		require.NoError(t, beginErr)
		result, freezeErr := store.FreezePreparedDeckRunTx(ctx, tx, params)
		if freezeErr != nil {
			_ = tx.Rollback(ctx)
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
		_ = jobTx.Rollback(ctx)
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
	assert.ErrorIs(t, err, ErrPreparedDeckClaimLost)
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
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()
	owner, err := store.CreateUser(ctx, "batch-reconcile-owner", false)
	require.NoError(t, err)
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "batch-reconcile-book", Title: "Batch Reconcile", MediaType: "text/plain", ContentHash: "batch-reconcile-hash", Content: []byte("Haus Baum"), FullText: "Haus Baum"}
	err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, source.OwnerID, source.Language, source.SourceIdentifier, source.Title, source.MediaType, source.ContentHash, source.Content, source.FullText).Scan(&source.ID)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	require.NoError(t, err)
	for _, lemma := range []string{"haus", "baum"} {
		_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,'de',$2,'NOUN','candidate')`, owner.ID, lemma)
		require.NoError(t, err)
	}
	manifest := cardexport.NewManifest(owner.ID, source.Title, []cardexport.Entry{
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: source.Title, FirstEncounter: 1},
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", Sentence: "Der alte Baum trägt heute viele grüne Blätter.", TargetWord: "Baum", SourceDocument: source.Title, FirstEncounter: 2},
	})
	candidates := manifest.EnrichmentCandidates()
	keys := make([]enrichment.CacheKey, len(candidates))
	for i, candidate := range candidates {
		keys[i] = enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	}
	manifest, err = manifest.BindCacheKeys(keys)
	require.NoError(t, err)
	params := FreezePreparedDeckRunParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, Manifest: manifest.Snapshot(),
		Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions", Model: "gpt-test", MaxBatchGenerations: 2, MaxProviderAttempts: 2},
		Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("c", 64), InputBytes: 128, Ordinals: []int{0, 1}}},
	}
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	frozen, err := store.FreezePreparedDeckRunTx(ctx, tx, params)
	if err != nil {
		_ = tx.Rollback(ctx)
		require.NoError(t, err)
	}
	err = store.SetPreparedDeckBatchSubmissionJobTx(ctx, tx, owner.ID, preparation.ID, frozen.Run.ID, frozen.Chunks[0].ID, 1, 9201)
	if err != nil {
		_ = tx.Rollback(ctx)
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
	firstWriter := enrichment.CacheEntry{CacheKey: keys[0], Translation: "first house", Gloss: "first writer", SentenceTranslation: "First cached sentence.", SentenceTranslationTarget: "house", CachedAt: time.Now().UTC().Add(-time.Minute)}
	firstWriter, err = store.Put(ctx, firstWriter)
	require.NoError(t, err)
	cacheEntry := enrichment.CacheEntry{CacheKey: keys[0], Translation: "provider house", Gloss: "provider result", SentenceTranslation: "The old house is surprisingly large.", SentenceTranslationTarget: "house", CachedAt: time.Now().UTC()}
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
	assert.ErrorIs(t, err, ErrPreparedDeckClaimLost)
	work, err := store.ListPreparedDeckRecoveryWork(ctx, 100)
	require.NoError(t, err)
	foundFinalizer := false
	for _, item := range work {
		foundFinalizer = foundFinalizer || (item.Kind == "finalizer" && item.RunID == frozen.Run.ID)
	}
	assert.True(t, foundFinalizer, "recovery work")
}
