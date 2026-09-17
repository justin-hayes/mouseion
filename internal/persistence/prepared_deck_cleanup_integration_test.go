//go:build integration

package persistence

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparedDeckBatchCleanupIsOwnerScopedAndIndependentOfOutcome(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "cleanup-owner", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "cleanup-other", false)
	require.NoError(t, err)
	var sourceID string
	err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,'de','cleanup-book','Cleanup Book','text/plain','cleanup-hash','Haus','Haus') RETURNING id::text`, owner.ID).Scan(&sourceID)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: sourceID, Filename: "Cleanup.apkg", DeckName: "Cleanup", ContentHash: "cleanup-hash"})
	require.NoError(t, err)
	sentence := "Das Haus steht am Ende der stillen Straße."
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, "Cleanup", []cardexport.Entry{{OwnerID: owner.ID, Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: sentence, TargetWord: "Haus", SourceDocument: "Cleanup", FirstEncounter: 1}}, testutil.PresentationProvider{Name: "openai", Version: "v1", TargetLanguage: "en"})
	require.NoError(t, err)
	key := enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(sentence)}
	assert.Equal(t, key, deck.WorkProjection()[0].CacheKey)
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	frozen, err := store.FreezePreparedDeckRunTx(ctx, tx, FreezePreparedDeckRunParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, Projection: deck.StorageProjection(),
		Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions", Model: "gpt-test"},
		Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("a", 64), InputBytes: 64, Ordinals: []int{0}}},
	})
	if err != nil {
		rollbackIntegrationTx(t, ctx, tx)
		require.NoError(t, err)
	}
	err = tx.Commit(ctx)
	require.NoError(t, err)
	chunkID := frozen.Chunks[0].ID
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_batch_chunks SET state='completed',input_file_id='file-input',output_file_id='file-output',error_file_id='file-error',completed_count=1 WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4`, owner.ID, preparation.ID, frozen.Run.ID, chunkID)
	require.NoError(t, err)
	_, err = store.GetDeckPreparationStatus(ctx, other.ID, preparation.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	claimToken := uuid.NewString()
	claimed, err := store.ClaimPreparedDeckBatchCleanup(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunkID, claimToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, claimToken, claimed.CleanupClaimToken)
	finished, err := store.FinishPreparedDeckBatchCleanup(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunkID, claimToken, PreparedDeckBatchCleanupUpdate{InputFileState: "deleted", OutputFileState: "failed", ErrorFileState: "deleted", InputFileAttempts: 1, OutputFileAttempts: 1, ErrorFileAttempts: 1, ErrorClass: "provider", ErrorCode: "delete_file"})
	require.NoError(t, err)
	assert.Equal(t, "deleted", finished.InputFileCleanupState)
	assert.Equal(t, "failed", finished.OutputFileCleanupState)
	assert.Nil(t, finished.CleanupCompletedAt)
	status, err := store.GetDeckPreparationStatus(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationPreparing, status.State)
	assert.Equal(t, frozen.Run.ID, status.CurrentRunID)
}
