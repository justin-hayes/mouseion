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
)

func TestDurablePreparedDeckRunFreezeTransitionAndAtomicFinalization(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "durable-run-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateUser(ctx, "durable-run-other", false)
	if err != nil {
		t.Fatal(err)
	}
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "durable-run-book", Title: "Durable Book", MediaType: "text/plain", ContentHash: "durable-run-hash", Content: []byte("Haus Baum"), FullText: "Haus Baum"}
	if err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, source.OwnerID, source.Language, source.SourceIdentifier, source.Title, source.MediaType, source.ContentHash, source.Content, source.FullText).Scan(&source.ID); err != nil {
		t.Fatal(err)
	}
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	if err != nil {
		t.Fatal(err)
	}
	for _, lemma := range []string{"haus", "baum"} {
		if _, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,'de',$2,'NOUN','candidate')`, owner.ID, lemma); err != nil {
			t.Fatal(err)
		}
	}

	manifest := cardexport.NewManifest(owner.ID, source.Title, []cardexport.Entry{
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "noun", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", Morphology: `{"Gender":"Neut"}`, SourceDocument: source.Title, FirstEncounter: 10},
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "baum", UPOS: "noun", Sentence: "Der alte Baum trägt heute viele grüne Blätter.", TargetWord: "Baum", Morphology: `{"Gender":"Masc"}`, SourceDocument: source.Title, FirstEncounter: 20},
		{OwnerID: owner.ID, Language: "de", CanonicalLemma: "fragment", UPOS: "noun", Sentence: "Fragment.", TargetWord: "Fragment", SourceDocument: source.Title, FirstEncounter: 30},
	})
	candidates := manifest.EnrichmentCandidates()
	keys := make([]enrichment.CacheKey, len(candidates))
	for i, candidate := range candidates {
		keys[i] = enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "prompt-v3", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	}
	manifest, err = manifest.BindCacheKeys(keys)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := manifest.Snapshot()
	if _, err = store.Put(ctx, enrichment.CacheEntry{CacheKey: keys[0], Translation: "house", SentenceTranslation: "The old house is surprisingly large.", SentenceTranslationTarget: "house", CachedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	params := FreezePreparedDeckRunParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, Manifest: snapshot,
		Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "prompt-v3", Endpoint: "/v1/chat/completions", Model: "gpt-test"},
		Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("a", 64), InputBytes: 128, EstimatedPromptTokens: 32, Ordinals: []int{1}}},
	}
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.FreezePreparedDeckRunTx(ctx, tx, params)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if result.Existing || result.NeedsFinalizer || result.Run.RunNumber != 1 || result.Run.CandidateCount != 2 || result.Run.CompletedCount != 1 || len(result.Chunks) != 1 {
		t.Fatalf("freeze result=%+v", result)
	}
	if result.Run.State != domain.PreparedDeckRunTranslating || result.Run.TranslationState != domain.PreparedDeckTranslationPending || result.Chunks[0].Ordinals[0] != 1 {
		t.Fatalf("run=%+v chunks=%+v", result.Run, result.Chunks)
	}
	retryTx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := store.FreezePreparedDeckRunTx(ctx, retryTx, params)
	if err != nil {
		_ = retryTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = retryTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if !repeated.Existing || repeated.Run.ID != result.Run.ID || repeated.ManifestDigest != result.ManifestDigest {
		t.Fatalf("idempotent freeze=%+v want run=%s", repeated, result.Run.ID)
	}
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
	if changedModeErr == nil || !errors.Is(changedModeErr, ErrImmutable) || changedModeResult.Existing {
		t.Fatalf("changed execution mode result=%+v err=%v", changedModeResult, changedModeErr)
	}
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
	if changedTargetErr == nil || changedTargetResult.Existing {
		t.Fatalf("changed target result=%+v err=%v", changedTargetResult, changedTargetErr)
	}
	if current, getErr := store.GetDeckPreparation(ctx, owner.ID, preparation.ID); getErr != nil || current.State != domain.DeckPreparationPreparing || current.CurrentRunID != result.Run.ID {
		t.Fatalf("current preparation=%+v err=%v", current, getErr)
	}
	if _, getErr := store.GetPreparedDeckRun(ctx, other.ID, preparation.ID, result.Run.ID); !errors.Is(getErr, ErrNotFound) {
		t.Fatalf("cross-owner run read=%v", getErr)
	}

	loaded, loadedDigest, err := store.LoadPreparedDeckManifest(ctx, owner.ID, preparation.ID, result.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantDigest, err := snapshot.Digest()
	if err != nil || loadedDigest != wantDigest {
		t.Fatalf("loaded digest=%q want=%q err=%v", loadedDigest, wantDigest, err)
	}
	if _, err = cardexport.ManifestFromSnapshot(loaded); err != nil {
		t.Fatalf("manifest round trip: %v", err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_manifests SET deck_name='mutated' WHERE run_id=$1`, result.Run.ID); err == nil {
		t.Fatal("immutable manifest update succeeded")
	}
	jobTx, err := store.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SetPreparedDeckBatchSubmissionJobTx(ctx, jobTx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 1, 9001); err != nil {
		_ = jobTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = jobTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	submissionToken := uuid.NewString()
	chunk, err := store.ClaimPreparedDeckBatchSubmission(ctx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 1, submissionToken, time.Now().UTC().Add(time.Minute))
	if err != nil || chunk.State != domain.PreparedDeckBatchSubmitting {
		t.Fatalf("claim chunk=%+v err=%v", chunk, err)
	}
	submitTx, err := store.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	chunk, err = store.RecordPreparedDeckBatchSubmittedTx(ctx, submitTx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 1, submissionToken, "file-input", "batch-1", time.Now().UTC(), func(context.Context, pgx.Tx, domain.PreparedDeckBatchChunk) (int64, error) {
		return 9003, nil
	})
	if err != nil || chunk.State != domain.PreparedDeckBatchSubmitted || chunk.BatchID != "batch-1" || chunk.ReconciliationJobID != 9003 {
		_ = submitTx.Rollback(ctx)
		t.Fatalf("submitted chunk=%+v err=%v", chunk, err)
	}
	if err = submitTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	chunk, err = store.AssignPreparedDeckBatchReconciliationJob(ctx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 0, 9002)
	if err != nil || chunk.ReconciliationGeneration != 1 || chunk.ReconciliationJobID != 9002 {
		t.Fatalf("assign reconciliation=%+v err=%v", chunk, err)
	}
	reconciliationToken := uuid.NewString()
	chunk, err = store.ClaimPreparedDeckBatchReconciliation(ctx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 1, reconciliationToken, time.Now().UTC().Add(time.Minute))
	if err != nil || chunk.State != domain.PreparedDeckBatchReconciling {
		t.Fatalf("claim reconciliation=%+v err=%v", chunk, err)
	}
	chunk, err = store.FinishPreparedDeckBatchReconciliation(ctx, owner.ID, preparation.ID, result.Run.ID, result.Chunks[0].ID, 1, reconciliationToken, PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchCompleted, ProviderStatus: "completed", OutputFileID: "file-output", CompletedCount: 1, InputTokens: 10, OutputTokens: 5})
	if err != nil || chunk.State != domain.PreparedDeckBatchCompleted || chunk.TotalTokens != 15 || chunk.ReconciledAt == nil {
		t.Fatalf("finish reconciliation=%+v err=%v", chunk, err)
	}

	claimToken := uuid.NewString()
	outcome, err := store.ClaimPreparedDeckTranslationOutcome(ctx, owner.ID, preparation.ID, result.Run.ID, 1, 0, claimToken, time.Now().UTC().Add(time.Minute))
	if err != nil || outcome.State != domain.PreparedDeckOutcomeRunning {
		t.Fatalf("claim outcome=%+v err=%v", outcome, err)
	}
	if _, err = store.Put(ctx, enrichment.CacheEntry{CacheKey: keys[1], Translation: "tree", SentenceTranslation: "The old tree has many green leaves today.", SentenceTranslationTarget: "tree", CachedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	finalizerInserted := false
	outcome, run, err := store.FinishPreparedDeckTranslationOutcome(ctx, owner.ID, preparation.ID, result.Run.ID, 1, 0, claimToken, PreparedDeckOutcomeTerminalUpdate{State: domain.PreparedDeckOutcomeCompleted, ProviderAttempt: true, ProviderCall: true, ProviderLatency: 25 * time.Millisecond}, func(_ context.Context, _ pgx.Tx, finalized domain.PreparedDeckRun) error {
		finalizerInserted = finalized.State == domain.PreparedDeckRunFinalizing
		return nil
	})
	if err != nil || outcome.State != domain.PreparedDeckOutcomeCompleted || run.State != domain.PreparedDeckRunFinalizing || run.TranslationState != domain.PreparedDeckTranslationCompleted || run.CompletedCount != 2 || !finalizerInserted {
		t.Fatalf("finish outcome=%+v run=%+v inserted=%v err=%v", outcome, run, finalizerInserted, err)
	}

	frozenManifest, exact, err := store.LoadPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID)
	if err != nil || len(exact) != 2 || exact[0].CacheKey != keys[0] || exact[1].CacheKey != keys[1] {
		t.Fatalf("finalization inputs exact=%+v err=%v", exact, err)
	}
	artifact, err := cardexport.NewService(store).RenderManifest(ctx, frozenManifest, exact)
	if err != nil {
		t.Fatal(err)
	}
	finalizationToken := uuid.NewString()
	claimed, err := store.ClaimPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID, 0, finalizationToken, time.Now().UTC().Add(time.Minute))
	if err != nil || claimed.FinalizationClaimToken != finalizationToken {
		t.Fatalf("claim finalization=%+v err=%v", claimed, err)
	}
	ready, err := store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, finalizationToken, artifact)
	if err != nil || ready.State != domain.DeckPreparationReady || ready.CurrentRunID != result.Run.ID || ready.TotalCards != 2 || ready.QualityOmissions != 1 {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}
	completed, err := store.GetPreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID)
	if err != nil || completed.State != domain.PreparedDeckRunCompleted || completed.CompletedAt == nil {
		t.Fatalf("completed run=%+v err=%v", completed, err)
	}
	if _, err = store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, finalizationToken, artifact); err != nil {
		t.Fatalf("idempotent completion: %v", err)
	}
	if retried, retryErr := store.ClaimPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID, 0, uuid.NewString(), time.Now().UTC().Add(time.Minute)); retryErr != nil || retried.State != domain.PreparedDeckRunCompleted {
		t.Fatalf("post-commit finalizer retry=%+v err=%v", retried, retryErr)
	}
	changed := artifact
	changed.APKG = []byte("different")
	if _, err = store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, finalizationToken, changed); !errors.Is(err, ErrImmutable) {
		t.Fatalf("mismatched completed artifact error=%v", err)
	}
	var cards, generated int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, owner.ID).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&generated); err != nil {
		t.Fatal(err)
	}
	if cards != 2 || generated != 2 {
		t.Fatalf("published cards=%d generated=%d", cards, generated)
	}
}

func TestDurablePreparedDeckCancellationFencesClaimsAndRetryCreatesNewRun(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := store.CreateUser(ctx, "durable-cancel-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "durable-cancel-book", Title: "Cancel Book", MediaType: "text/plain", ContentHash: "durable-cancel-hash", Content: []byte("Haus"), FullText: "Haus"}
	if err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, source.OwnerID, source.Language, source.SourceIdentifier, source.Title, source.MediaType, source.ContentHash, source.Content, source.FullText).Scan(&source.ID); err != nil {
		t.Fatal(err)
	}
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	if err != nil {
		t.Fatal(err)
	}
	manifest := cardexport.NewManifest(owner.ID, source.Title, []cardexport.Entry{{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: source.Title, FirstEncounter: 1}})
	candidate := manifest.EnrichmentCandidates()[0]
	key := enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	manifest, err = manifest.BindCacheKeys([]enrichment.CacheKey{key})
	if err != nil {
		t.Fatal(err)
	}
	params := FreezePreparedDeckRunParams{OwnerID: owner.ID, PreparationID: preparation.ID, Manifest: manifest.Snapshot(), Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions", Model: "gpt-test"}, Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("b", 64), InputBytes: 64, Ordinals: []int{0}}}}
	freeze := func() FreezePreparedDeckRunResult {
		t.Helper()
		tx, beginErr := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		result, freezeErr := store.FreezePreparedDeckRunTx(ctx, tx, params)
		if freezeErr != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(freezeErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			t.Fatal(commitErr)
		}
		return result
	}
	first := freeze()
	jobTx, err := store.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SetPreparedDeckBatchSubmissionJobTx(ctx, jobTx, owner.ID, preparation.ID, first.Run.ID, first.Chunks[0].ID, 1, 9101); err != nil {
		_ = jobTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = jobTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	submissionToken := uuid.NewString()
	if _, err = store.ClaimPreparedDeckBatchSubmission(ctx, owner.ID, preparation.ID, first.Run.ID, first.Chunks[0].ID, 1, submissionToken, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.RecordPreparedDeckBatchSubmitted(ctx, owner.ID, preparation.ID, first.Run.ID, first.Chunks[0].ID, 1, submissionToken, "cancel-file", "cancel-batch", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	reconciliationToken := uuid.NewString()
	if _, err = store.ClaimPreparedDeckBatchReconciliation(ctx, owner.ID, preparation.ID, first.Run.ID, first.Chunks[0].ID, 0, reconciliationToken, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CancelCurrentPreparedDeckRun(ctx, owner.ID, preparation.ID); err != nil {
		t.Fatal(err)
	}
	lateEntry := enrichment.CacheEntry{CacheKey: key, Translation: "late house", CachedAt: time.Now().UTC()}
	_, err = store.ReconcilePreparedDeckBatch(ctx, PreparedDeckBatchReconcileParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, RunID: first.Run.ID, ChunkID: first.Chunks[0].ID, ClaimToken: reconciliationToken,
		Chunk: PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchCompleted, ProviderStatus: "completed", CompletedCount: 1},
		Items: []PreparedDeckBatchItemReconciliation{{Ordinal: 0, State: domain.PreparedDeckOutcomeCompleted, CacheEntry: &lateEntry}},
	}, nil, nil)
	if !errors.Is(err, ErrPreparedDeckClaimLost) {
		t.Fatalf("late reconciliation error=%v", err)
	}
	if _, found, cacheErr := store.Get(ctx, key); cacheErr != nil || found {
		t.Fatalf("late cancelled result reached cache found=%t err=%v", found, cacheErr)
	}
	if _, err = store.ClaimPreparedDeckTranslationOutcome(ctx, owner.ID, preparation.ID, first.Run.ID, 0, 0, uuid.NewString(), time.Now().UTC().Add(time.Minute)); !errors.Is(err, ErrPreparedDeckClaimLost) && !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("cancelled outcome claim error=%v", err)
	}
	if _, err = store.RetryDeckPreparation(ctx, owner.ID, preparation.ID); err != nil {
		t.Fatal(err)
	}
	second := freeze()
	if second.Run.ID == first.Run.ID || second.Run.RunNumber != 2 {
		t.Fatalf("manual retry did not create new run: first=%+v second=%+v", first.Run, second.Run)
	}
}

func TestPreparedDeckBatchReconciliationRetainsPartialSuccessAndExhaustsTwoGenerations(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := store.CreateUser(ctx, "batch-reconcile-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	source := domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "batch-reconcile-book", Title: "Batch Reconcile", MediaType: "text/plain", ContentHash: "batch-reconcile-hash", Content: []byte("Haus Baum"), FullText: "Haus Baum"}
	if err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, source.OwnerID, source.Language, source.SourceIdentifier, source.Title, source.MediaType, source.ContentHash, source.Content, source.FullText).Scan(&source.ID); err != nil {
		t.Fatal(err)
	}
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	if err != nil {
		t.Fatal(err)
	}
	for _, lemma := range []string{"haus", "baum"} {
		if _, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,'de',$2,'NOUN','candidate')`, owner.ID, lemma); err != nil {
			t.Fatal(err)
		}
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
	if err != nil {
		t.Fatal(err)
	}
	params := FreezePreparedDeckRunParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, Manifest: manifest.Snapshot(),
		Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions", Model: "gpt-test", MaxBatchGenerations: 2, MaxProviderAttempts: 2},
		Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("c", 64), InputBytes: 128, Ordinals: []int{0, 1}}},
	}
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := store.FreezePreparedDeckRunTx(ctx, tx, params)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = store.SetPreparedDeckBatchSubmissionJobTx(ctx, tx, owner.ID, preparation.ID, frozen.Run.ID, frozen.Chunks[0].ID, 1, 9201); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	claimAndSubmit := func(chunk domain.PreparedDeckBatchChunk, inputFile, batchID string) string {
		t.Helper()
		submissionToken := uuid.NewString()
		if _, claimErr := store.ClaimPreparedDeckBatchSubmission(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunk.ID, chunk.Generation, submissionToken, time.Now().UTC().Add(time.Minute)); claimErr != nil {
			t.Fatal(claimErr)
		}
		if _, submitErr := store.RecordPreparedDeckBatchSubmitted(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunk.ID, chunk.Generation, submissionToken, inputFile, batchID, time.Now().UTC()); submitErr != nil {
			t.Fatal(submitErr)
		}
		reconcileToken := uuid.NewString()
		if _, claimErr := store.ClaimPreparedDeckBatchReconciliation(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunk.ID, 0, reconcileToken, time.Now().UTC().Add(time.Minute)); claimErr != nil {
			t.Fatal(claimErr)
		}
		return reconcileToken
	}
	firstToken := claimAndSubmit(frozen.Chunks[0], "input-one", "batch-one")
	// A different run may populate the exact key after this Batch was submitted.
	// The immutable first writer remains authoritative even when the provider
	// later returns different valid text.
	firstWriter := enrichment.CacheEntry{CacheKey: keys[0], Translation: "first house", Gloss: "first writer", SentenceTranslation: "First cached sentence.", SentenceTranslationTarget: "house", CachedAt: time.Now().UTC().Add(-time.Minute)}
	firstWriter, err = store.Put(ctx, firstWriter)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil || len(firstResult.RetryChunks) != 1 || firstResult.RetryChunks[0].Generation != 2 || len(firstResult.RetryChunks[0].Ordinals) != 1 || firstResult.RetryChunks[0].Ordinals[0] != 1 {
		t.Fatalf("first reconciliation=%+v err=%v", firstResult, err)
	}
	if stored, found, getErr := store.Get(ctx, keys[0]); getErr != nil || !found || stored.Translation != firstWriter.Translation || !stored.CachedAt.Equal(firstWriter.CachedAt) {
		t.Fatalf("exact cache stored=%+v found=%t err=%v", stored, found, getErr)
	}
	outcomes, err := store.ListPreparedDeckTranslationOutcomes(ctx, owner.ID, preparation.ID, frozen.Run.ID)
	if err != nil || outcomes[0].State != domain.PreparedDeckOutcomeCompleted || outcomes[1].State != domain.PreparedDeckOutcomePending || outcomes[1].ProviderAttemptCount != 1 {
		t.Fatalf("partial outcomes=%+v err=%v", outcomes, err)
	}
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
	if err != nil || secondResult.Run.State != domain.PreparedDeckRunFinalizing || secondResult.Run.CompletedCount != 1 || secondResult.Run.FailedCount != 1 || finalizerCount != 1 {
		t.Fatalf("second reconciliation=%+v finalizers=%d err=%v", secondResult, finalizerCount, err)
	}
	if _, err = store.ReconcilePreparedDeckBatch(ctx, PreparedDeckBatchReconcileParams{OwnerID: owner.ID, PreparationID: preparation.ID, RunID: frozen.Run.ID, ChunkID: second.ID, ClaimToken: secondToken, Chunk: PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchCompleted, ProviderStatus: "completed"}}, nil, nil); !errors.Is(err, ErrPreparedDeckClaimLost) {
		t.Fatalf("duplicate reconciliation error=%v", err)
	}
	work, err := store.ListPreparedDeckRecoveryWork(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	foundFinalizer := false
	for _, item := range work {
		foundFinalizer = foundFinalizer || (item.Kind == "finalizer" && item.RunID == frozen.Run.ID)
	}
	if !foundFinalizer {
		t.Fatalf("recovery work=%+v", work)
	}
}
