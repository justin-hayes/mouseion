//go:build integration

package persistence

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

func TestPreparedDeckBatchCleanupIsOwnerScopedAndIndependentOfOutcome(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := store.CreateUser(ctx, "cleanup-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateUser(ctx, "cleanup-other", false)
	if err != nil {
		t.Fatal(err)
	}
	var sourceID string
	if err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,'de','cleanup-book','Cleanup Book','text/plain','cleanup-hash','Haus','Haus') RETURNING id::text`, owner.ID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: sourceID, Filename: "cleanup.apkg", DeckName: "Cleanup", ContentHash: "cleanup-hash"})
	if err != nil {
		t.Fatal(err)
	}
	manifest := cardexport.NewManifest(owner.ID, "Cleanup", []cardexport.Entry{{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus.", TargetWord: "Haus", SourceDocument: "Cleanup", FirstEncounter: 1}})
	key := enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash("Das Haus.")}
	manifest, err = manifest.BindCacheKeys([]enrichment.CacheKey{key})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := store.FreezePreparedDeckRunTx(ctx, tx, FreezePreparedDeckRunParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, Manifest: manifest.Snapshot(),
		Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions", Model: "gpt-test"},
		Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("a", 64), InputBytes: 64, Ordinals: []int{0}}},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	chunkID := frozen.Chunks[0].ID
	if _, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_batch_chunks SET state='completed',input_file_id='file-input',output_file_id='file-output',error_file_id='file-error',completed_count=1 WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4`, owner.ID, preparation.ID, frozen.Run.ID, chunkID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetDeckPreparationStatus(ctx, other.ID, preparation.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner status error=%v", err)
	}
	claimToken := uuid.NewString()
	claimed, err := store.ClaimPreparedDeckBatchCleanup(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunkID, claimToken, time.Now().UTC().Add(time.Minute))
	if err != nil || claimed.CleanupClaimToken != claimToken {
		t.Fatalf("cleanup claim=%+v err=%v", claimed, err)
	}
	finished, err := store.FinishPreparedDeckBatchCleanup(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunkID, claimToken, PreparedDeckBatchCleanupUpdate{InputFileState: "deleted", OutputFileState: "failed", ErrorFileState: "deleted", InputFileAttempts: 1, OutputFileAttempts: 1, ErrorFileAttempts: 1, ErrorClass: "provider", ErrorCode: "delete_file"})
	if err != nil || finished.InputFileCleanupState != "deleted" || finished.OutputFileCleanupState != "failed" || finished.CleanupCompletedAt != nil {
		t.Fatalf("cleanup result=%+v err=%v", finished, err)
	}
	status, err := store.GetDeckPreparationStatus(ctx, owner.ID, preparation.ID)
	if err != nil || status.State != domain.DeckPreparationPreparing || status.CurrentRunID != frozen.Run.ID {
		t.Fatalf("preparation status=%+v err=%v", status, err)
	}
}
