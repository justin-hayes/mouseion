//go:build integration

package persistence

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestPreparedDeckBatchReconciliationRejectsInvalidRetryPlans pins every retry
// selection rule that runs before a retry chunk is written. The reconciliation
// transaction rolls back on each rejection, so one claimed chunk can drive every
// case through the public seam.
func TestPreparedDeckBatchReconciliationRejectsInvalidRetryPlans(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "retry-plan-owner", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "retry-plan-book", Title: "Retry Plan", MediaType: "text/plain", ContentHash: "retry-plan-hash", Content: []byte("Haus Baum"), FullText: "Haus Baum"})
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	require.NoError(t, err)
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, []cardexport.Entry{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: source.Title, FirstEncounter: 1},
		{Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", Sentence: "Der alte Baum trägt heute viele grüne Blätter.", TargetWord: "Baum", SourceDocument: source.Title, FirstEncounter: 2},
	}, testutil.PresentationProvider{Name: "openai", Version: "v1", TargetLanguage: "en"})
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 2)
	keys := make([]enrichment.CacheKey, len(work))
	for i, item := range work {
		candidate := item.RequestCandidate()
		keys[i] = enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
	}
	const maxBytes = int64(1 << 20)
	params := FreezePreparedDeckRunParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, Projection: deck.StorageProjection(),
		Config: PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ExecutionMode: string(domain.PreparedDeckExecutionBatch), TargetLanguage: "en", ContextMode: "sentence", Provider: "openai", ProviderVersion: "v1", Endpoint: "/v1/chat/completions", Model: "gpt-test", MaxBatchGenerations: 2, MaxProviderAttempts: 2, BatchMaxRequests: 2, BatchMaxBytes: maxBytes},
		Chunks: []PreparedDeckBatchChunkPlan{{ChunkIndex: 0, Generation: 1, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "run", InputDigest: strings.Repeat("e", 64), InputBytes: 128, Ordinals: []int{0, 1}}},
	}
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	frozen, err := store.FreezePreparedDeckRunTx(ctx, tx, params)
	if err != nil {
		rollbackIntegrationTx(t, ctx, tx)
		require.NoError(t, err)
	}
	if err := store.SetPreparedDeckBatchSubmissionJobTx(ctx, tx, owner.ID, preparation.ID, frozen.Run.ID, frozen.Chunks[0].ID, 1, 9301); err != nil {
		rollbackIntegrationTx(t, ctx, tx)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit(ctx))
	chunk := frozen.Chunks[0]

	submissionToken := uuid.NewString()
	_, err = store.ClaimPreparedDeckBatchSubmission(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunk.ID, chunk.Generation, submissionToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	_, err = store.RecordPreparedDeckBatchSubmitted(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunk.ID, chunk.Generation, submissionToken, "retry-input", "retry-batch", time.Now().UTC())
	require.NoError(t, err)
	reconcileToken := uuid.NewString()
	_, err = store.ClaimPreparedDeckBatchReconciliation(ctx, owner.ID, preparation.ID, frozen.Run.ID, chunk.ID, 0, reconcileToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)

	cacheEntry := enrichment.CacheEntry{CacheKey: keys[0], Translation: "house", CachedAt: time.Now().UTC()}
	base := func() PreparedDeckBatchReconcileParams {
		return PreparedDeckBatchReconcileParams{
			OwnerID: owner.ID, PreparationID: preparation.ID, RunID: frozen.Run.ID, ChunkID: chunk.ID, ClaimToken: reconcileToken,
			Chunk: PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchCompleted, ProviderStatus: "expired", CompletedCount: 1, ExpiredCount: 1},
			Items: []PreparedDeckBatchItemReconciliation{
				{Ordinal: 0, State: domain.PreparedDeckOutcomeCompleted, CacheEntry: &cacheEntry},
				{Ordinal: 1, State: domain.PreparedDeckOutcomePending, ErrorClass: "expired", ErrorCode: "expired"},
			},
			RetryChunks: []PreparedDeckBatchChunkPlan{{Generation: 2, Model: "gpt-test", Endpoint: "/v1/chat/completions", SplitReason: "retry", InputDigest: strings.Repeat("f", 64), InputBytes: 64, Ordinals: []int{1}}},
		}
	}
	inserter := func(context.Context, pgx.Tx, domain.PreparedDeckBatchChunk) (int64, error) { return 9302, nil }
	tests := []struct {
		name        string
		mutate      func(*PreparedDeckBatchReconcileParams)
		nilInserter bool
		zeroJob     bool
		wantErr     error
	}{
		{name: "plan generation mismatch", mutate: func(p *PreparedDeckBatchReconcileParams) { p.RetryChunks[0].Generation = 3 }, wantErr: ErrInvalidTransition},
		{name: "plan model mismatch", mutate: func(p *PreparedDeckBatchReconcileParams) { p.RetryChunks[0].Model = "other-model" }, wantErr: ErrInvalidTransition},
		{name: "plan endpoint mismatch", mutate: func(p *PreparedDeckBatchReconcileParams) { p.RetryChunks[0].Endpoint = "/v1/responses" }, wantErr: ErrInvalidTransition},
		{name: "plan split reason is not retry", mutate: func(p *PreparedDeckBatchReconcileParams) { p.RetryChunks[0].SplitReason = "run" }, wantErr: ErrInvalidTransition},
		{name: "plan has no ordinals", mutate: func(p *PreparedDeckBatchReconcileParams) { p.RetryChunks[0].Ordinals = nil }, wantErr: ErrInvalidTransition},
		{name: "plan exceeds the request limit", mutate: func(p *PreparedDeckBatchReconcileParams) { p.RetryChunks[0].Ordinals = []int{1, 1, 1} }, wantErr: ErrInvalidTransition},
		{name: "plan input bytes below one", mutate: func(p *PreparedDeckBatchReconcileParams) { p.RetryChunks[0].InputBytes = 0 }, wantErr: ErrInvalidTransition},
		{name: "plan input bytes above max", mutate: func(p *PreparedDeckBatchReconcileParams) { p.RetryChunks[0].InputBytes = maxBytes + 1 }, wantErr: ErrInvalidTransition},
		{name: "plan selects an unretried ordinal", mutate: func(p *PreparedDeckBatchReconcileParams) { p.RetryChunks[0].Ordinals = []int{0} }, wantErr: ErrPreparedDeckIdentity},
		{name: "plan repeats an ordinal", mutate: func(p *PreparedDeckBatchReconcileParams) { p.RetryChunks[0].Ordinals = []int{1, 1} }, wantErr: ErrPreparedDeckIdentity},
		{name: "plan omits a retried ordinal", mutate: func(p *PreparedDeckBatchReconcileParams) { p.RetryChunks = nil }, wantErr: ErrInvalidTransition},
		{name: "missing submission inserter", nilInserter: true, wantErr: ErrInvalidTransition},
		{name: "submission inserter returns no job", zeroJob: true, wantErr: ErrInvalidTransition},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reconcile := base()
			if test.mutate != nil {
				test.mutate(&reconcile)
			}
			insert := PreparedDeckBatchSubmissionJobInserter(inserter)
			if test.nilInserter {
				insert = nil
			} else if test.zeroJob {
				insert = func(context.Context, pgx.Tx, domain.PreparedDeckBatchChunk) (int64, error) { return 0, nil }
			}
			_, err := store.ReconcilePreparedDeckBatch(ctx, reconcile, insert, nil)
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}
