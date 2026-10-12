//go:build integration

package prepareddeck

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unusedBatchProvider fails the owning test if any provider boundary is
// crossed. An all-cache-hit chunk must complete without one.
type unusedBatchProvider struct{ t *testing.T }

func (p *unusedBatchProvider) UploadFile(context.Context, string, io.Reader) (enrichment.OpenAIFile, error) {
	p.t.Error("provider UploadFile was called for an all-cache-hit chunk")
	return enrichment.OpenAIFile{}, errors.New("provider must not be used")
}

func (p *unusedBatchProvider) CreateBatch(context.Context, enrichment.CreateBatchRequest) (enrichment.Batch, error) {
	p.t.Error("provider CreateBatch was called for an all-cache-hit chunk")
	return enrichment.Batch{}, errors.New("provider must not be used")
}

func (p *unusedBatchProvider) ListBatches(context.Context, enrichment.ListBatchesRequest) (enrichment.BatchList, error) {
	p.t.Error("provider ListBatches was called for an all-cache-hit chunk")
	return enrichment.BatchList{}, errors.New("provider must not be used")
}

// TestBatchSubmitCompletesChunkWhenEveryMemberIsACacheHit covers the
// all-cache-hit submission path: a chunk frozen pending whose every member
// gains an exact cache row before submission must complete locally instead of
// crossing the provider boundary. The unqualified completed_count in
// FinishPreparedDeckBatchSubmission previously made this statement fail with an
// ambiguous column reference.
func TestBatchSubmitCompletesChunkWhenEveryMemberIsACacheHit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "batch-all-cache-hit-"+uuid.NewString(), false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: "All Cache Hit", MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text"})
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: source.Title, ContentHash: source.ContentHash})
	require.NoError(t, err)
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	entries := []cardexport.Entry{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: source.Title, FirstEncounter: 1},
		{Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", Sentence: "Der grüne Baum steht vor dem Haus.", TargetWord: "Baum", SourceDocument: source.Title, FirstEncounter: 2},
	}
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, entries, testutil.PresentationProvider{Name: codec.ProviderName(), Version: codec.ContextualGlossProviderVersion(), TargetLanguage: "en"})
	require.NoError(t, err)
	runID := uuid.NewString()
	chunks, err := PlanBatchChunks(codec, runID, 1, codec.Model(), enrichment.OpenAIChatCompletionsEndpoint, deck.WorkProjection(), BatchChunkLimits{MaxRequests: 10})
	require.NoError(t, err)
	config := persistence.PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ExecutionMode: string(domain.PreparedDeckExecutionBatch), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext), Provider: codec.ProviderName(), ProviderVersion: codec.ContextualGlossProviderVersion(), Endpoint: enrichment.OpenAIChatCompletionsEndpoint, Model: codec.Model(), BatchMaxRequests: 10, BatchMaxBytes: persistence.DefaultBatchMaxBytes}
	planner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: runID, Projection: deck.StorageProjection(), Config: config, Chunks: chunks}}
	workers := river.NewWorkers()
	river.AddWorker(workers, &BatchSubmitWorker{})
	river.AddWorker(workers, &integrationFinalizeWorker{})
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	coordinator := NewDurableCoordinator(store, client, planner)
	run, err := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: preparation.ID})
	require.NoError(t, err)
	require.Len(t, run.Chunks, 1)
	chunk := run.Chunks[0]
	require.Equal(t, len(entries), chunk.RequestCount)

	// Every chunk member becomes an exact cache hit after the run was frozen
	// pending. The submission must then finish the chunk without a provider call.
	for _, ordinal := range chunk.Ordinals {
		item, ok := deck.WorkByOrdinal(ordinal)
		require.True(t, ok)
		_, putErr := store.Put(ctx, enrichment.CacheEntry{CacheKey: item.CacheKey, Translation: "cached translation", FallbackGloss: "cached gloss", SentenceTranslation: "The cached sentence.", SentenceTranslationTarget: item.Request.TargetWord, CachedAt: time.Now().UTC()})
		require.NoError(t, putErr)
	}

	submitter := &BatchSubmitWorker{Store: store, Client: client, Provider: &unusedBatchProvider{t: t}, Codec: codec}
	require.NoError(t, submitter.Submit(ctx, BatchSubmitJobArgs{OwnerID: owner.ID, PreparationID: preparation.ID, RunID: runID, ChunkID: chunk.ID, Generation: chunk.Generation}))

	stored, err := store.GetPreparedDeckBatchChunk(ctx, owner.ID, preparation.ID, runID, chunk.ID)
	require.NoError(t, err)
	t.Run("chunk completes with every member counted", func(t *testing.T) {
		require.Equal(t, domain.PreparedDeckBatchCompleted, stored.State)
		require.Equal(t, chunk.RequestCount, stored.CompletedCount)
		require.Empty(t, stored.SubmissionClaimToken, "the completed chunk must release its submission claim")
	})

	t.Run("every outcome completes as a cache hit", func(t *testing.T) {
		outcomes, err := store.ListPreparedDeckTranslationOutcomes(ctx, owner.ID, preparation.ID, runID)
		require.NoError(t, err)
		require.Len(t, outcomes, len(entries))
		states := make([]domain.PreparedDeckOutcomeState, len(outcomes))
		cacheHits := make([]int, len(outcomes))
		for index, outcome := range outcomes {
			states[index] = outcome.State
			cacheHits[index] = outcome.CacheHitCount
		}
		assert.Equal(t, []domain.PreparedDeckOutcomeState{domain.PreparedDeckOutcomeCompleted, domain.PreparedDeckOutcomeCompleted}, states)
		assert.Equal(t, []int{1, 1}, cacheHits)
	})

	t.Run("run advances to finalizing", func(t *testing.T) {
		finalRun, err := store.GetPreparedDeckRun(ctx, owner.ID, preparation.ID, runID)
		require.NoError(t, err)
		assert.Equal(t, domain.PreparedDeckRunFinalizing, finalRun.State, "a fully completed run must advance to finalizing")
	})
}
