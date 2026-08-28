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
		keys[i] = enrichment.CacheKey{Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "prompt-v3", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
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
	key := enrichment.CacheKey{Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
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
	if _, err = store.CancelCurrentPreparedDeckRun(ctx, owner.ID, preparation.ID); err != nil {
		t.Fatal(err)
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
