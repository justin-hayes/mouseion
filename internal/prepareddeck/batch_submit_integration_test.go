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

// submitFixtureProvider is a batchProvider stub whose upload and creation
// boundaries fail or return contract-violating values on demand.
type submitFixtureProvider struct {
	uploadErr  error
	uploadFile *enrichment.OpenAIFile
	createErr  error
	create     *enrichment.Batch
	listErr    error
	batches    []enrichment.BatchList
}

func (p *submitFixtureProvider) UploadFile(_ context.Context, _ string, content io.Reader) (enrichment.OpenAIFile, error) {
	if p.uploadErr != nil {
		return enrichment.OpenAIFile{}, p.uploadErr
	}
	data, err := io.ReadAll(content)
	if err != nil {
		return enrichment.OpenAIFile{}, err
	}
	if p.uploadFile != nil {
		return *p.uploadFile, nil
	}
	return enrichment.OpenAIFile{ID: "submit-input", Bytes: int64(len(data))}, nil
}

func (p *submitFixtureProvider) CreateBatch(_ context.Context, request enrichment.CreateBatchRequest) (enrichment.Batch, error) {
	if p.createErr != nil {
		return enrichment.Batch{}, p.createErr
	}
	if p.create != nil {
		return *p.create, nil
	}
	return enrichment.Batch{ID: "submit-batch", InputFileID: request.InputFileID, Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CompletionWindow: "24h", Metadata: request.Metadata}, nil
}

func (p *submitFixtureProvider) ListBatches(context.Context, enrichment.ListBatchesRequest) (enrichment.BatchList, error) {
	if p.listErr != nil {
		return enrichment.BatchList{}, p.listErr
	}
	if len(p.batches) == 0 {
		return enrichment.BatchList{}, nil
	}
	page := p.batches[0]
	p.batches = p.batches[1:]
	return page, nil
}

type batchSubmitFixture struct {
	store       *persistence.PostgresStore
	client      riverClient
	owner       domain.User
	preparation domain.DeckPreparation
	codec       *enrichment.TranslationCodec
	run         persistence.FreezePreparedDeckRunResult
}

func (f batchSubmitFixture) args(chunkID string, generation int) BatchSubmitJobArgs {
	return BatchSubmitJobArgs{OwnerID: f.owner.ID, PreparationID: f.preparation.ID, RunID: f.run.Run.ID, ChunkID: chunkID, Generation: generation}
}

// newBatchSubmitFixture freezes one Batch run through the public durable
// coordinator so Submit can be exercised against a real pending chunk without
// starting a River queue.
func newBatchSubmitFixture(t *testing.T, ctx context.Context) batchSubmitFixture {
	t.Helper()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "batch-submit-"+uuid.NewString(), false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: "Batch Submit", MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text"})
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: source.Title, ContentHash: source.ContentHash})
	require.NoError(t, err)
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, []cardexport.Entry{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: source.Title, FirstEncounter: 1},
	}, testutil.PresentationProvider{Name: codec.ProviderName(), Version: codec.ContextualGlossProviderVersion(), TargetLanguage: "en"})
	require.NoError(t, err)
	runID := uuid.NewString()
	chunks, err := PlanBatchChunks(codec, runID, 1, codec.Model(), enrichment.OpenAIChatCompletionsEndpoint, deck.WorkProjection(), BatchChunkLimits{MaxRequests: 10})
	require.NoError(t, err)
	config := persistence.PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ExecutionMode: string(domain.PreparedDeckExecutionBatch), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext), Provider: codec.ProviderName(), ProviderVersion: codec.ContextualGlossProviderVersion(), Endpoint: enrichment.OpenAIChatCompletionsEndpoint, Model: codec.Model(), BatchMaxRequests: 10, BatchMaxBytes: persistence.DefaultBatchMaxBytes}
	workers := river.NewWorkers()
	river.AddWorker(workers, &BatchSubmitWorker{})
	river.AddWorker(workers, &BatchPollWorker{})
	river.AddWorker(workers, &StandardTranslationWorker{})
	river.AddWorker(workers, &integrationFinalizeWorker{})
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	planner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: runID, Projection: deck.StorageProjection(), Config: config, Chunks: chunks}}
	run, err := NewDurableCoordinator(store, client, planner).Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: preparation.ID})
	require.NoError(t, err)
	require.Len(t, run.Chunks, 1)
	return batchSubmitFixture{store: store, client: client, owner: owner, preparation: preparation, codec: codec, run: run}
}

func (f batchSubmitFixture) loadChunk(t *testing.T, ctx context.Context, chunkID string) domain.PreparedDeckBatchChunk {
	t.Helper()
	chunk, err := f.store.GetPreparedDeckBatchChunk(ctx, f.owner.ID, f.preparation.ID, f.run.Run.ID, chunkID)
	require.NoError(t, err)
	return chunk
}

func setBatchChunkState(t *testing.T, ctx context.Context, store *persistence.PostgresStore, chunkID string, state domain.PreparedDeckBatchChunkState) {
	t.Helper()
	_, err := store.Pool().Exec(ctx, `UPDATE deck_preparation_batch_chunks SET state=$2, submission_claim_token=NULL, submission_lease_expires_at=NULL WHERE id=$1`, chunkID, string(state))
	require.NoError(t, err)
}

// TestBatchSubmitSkipsSettledAndStaleChunks pins the short-circuits that run
// before any claim: a stale generation and every already-settled chunk state
// must leave the durable row untouched.
func TestBatchSubmitSkipsSettledAndStaleChunks(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	chunk := fixture.run.Chunks[0]
	worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{}, Codec: fixture.codec}

	t.Run("stale generation is a no-op", func(t *testing.T) {
		require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation+1)))
		assert.Equal(t, domain.PreparedDeckBatchPending, fixture.loadChunk(t, ctx, chunk.ID).State)
	})
	settled := []domain.PreparedDeckBatchChunkState{
		domain.PreparedDeckBatchSubmitted,
		domain.PreparedDeckBatchPolling,
		domain.PreparedDeckBatchReconciling,
		domain.PreparedDeckBatchCompleted,
		domain.PreparedDeckBatchFailed,
		domain.PreparedDeckBatchCancelled,
		domain.PreparedDeckBatchAmbiguous,
	}
	for _, state := range settled {
		t.Run("settled "+string(state)+" is a no-op", func(t *testing.T) {
			setBatchChunkState(t, ctx, fixture.store, chunk.ID, state)
			require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
			assert.Equal(t, state, fixture.loadChunk(t, ctx, chunk.ID).State)
		})
	}
}

// TestBatchSubmitSurfacesUnclaimableChunks pins the read and claim failures
// that stop an attempt without writing an outcome.
func TestBatchSubmitSurfacesUnclaimableChunks(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	chunk := fixture.run.Chunks[0]
	worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{}, Codec: fixture.codec}

	t.Run("unknown chunk is surfaced", func(t *testing.T) {
		require.ErrorIs(t, worker.Submit(ctx, fixture.args(uuid.NewString(), chunk.Generation)), persistence.ErrNotFound)
	})
	t.Run("claim generation mismatch is a no-op", func(t *testing.T) {
		_, err := fixture.store.Pool().Exec(ctx, `UPDATE deck_preparation_batch_chunks SET submission_generation=submission_generation+1 WHERE id=$1`, chunk.ID)
		require.NoError(t, err)
		require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
		assert.Equal(t, domain.PreparedDeckBatchPending, fixture.loadChunk(t, ctx, chunk.ID).State)
	})
}

// TestBatchSubmitSurfacesInvalidDurableItems pins the membership guard: a
// durable outcome that is neither pending nor completed makes the rebuilt item
// set invalid before the provider boundary.
func TestBatchSubmitSurfacesInvalidDurableItems(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	chunk := fixture.run.Chunks[0]
	_, err := fixture.store.Pool().Exec(ctx, `DELETE FROM deck_preparation_translation_outcomes WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3`, fixture.owner.ID, fixture.preparation.ID, fixture.run.Run.ID)
	require.NoError(t, err)
	worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{}, Codec: fixture.codec}
	require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
	stored := fixture.loadChunk(t, ctx, chunk.ID)
	assert.Equal(t, domain.PreparedDeckBatchFailed, stored.State)
	assert.Equal(t, "validation", stored.ErrorClass)
	assert.Equal(t, "chunk_membership", stored.ErrorCode)
}

// TestBatchSubmitSurfacesProviderFailures pins each pre-network and external
// boundary failure against the durable chunk outcome the split must preserve:
// temporary uploads return to pending for a snooze, permanent uploads and
// contract violations fail, and creation uncertainty is recorded as ambiguous.
func TestBatchSubmitSurfacesProviderFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("temporary upload returns the chunk to pending", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{uploadErr: &enrichment.ProviderError{Operation: "upload", Class: enrichment.ProviderErrorRateLimit}}, Codec: fixture.codec}
		require.Error(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		assert.Equal(t, domain.PreparedDeckBatchPending, stored.State)
		assert.Equal(t, "upload", stored.ErrorClass)
		assert.Equal(t, "rate_limit", stored.ErrorCode)
	})

	t.Run("permanent upload fails the chunk", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{uploadErr: &enrichment.ProviderError{Operation: "upload", Class: enrichment.ProviderErrorInvalidRequest}}, Codec: fixture.codec}
		require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		assert.Equal(t, domain.PreparedDeckBatchFailed, stored.State)
		assert.Equal(t, "upload", stored.ErrorClass)
		assert.Equal(t, "invalid_request", stored.ErrorCode)
	})

	t.Run("uploaded file contract violation fails the chunk", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		contract := enrichment.OpenAIFile{ID: "", Bytes: 0}
		worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{uploadFile: &contract}, Codec: fixture.codec}
		require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		assert.Equal(t, domain.PreparedDeckBatchFailed, stored.State)
		assert.Equal(t, "upload", stored.ErrorClass)
		assert.Equal(t, "file_contract", stored.ErrorCode)
	})

	t.Run("create response loss is ambiguous", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{createErr: &enrichment.ProviderError{Operation: "create", Class: enrichment.ProviderErrorTransport}}, Codec: fixture.codec}
		require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		assert.Equal(t, domain.PreparedDeckBatchAmbiguous, stored.State)
		assert.Equal(t, "ambiguous_submission", stored.ErrorClass)
		assert.Equal(t, "create_response_lost", stored.ErrorCode)
	})

	t.Run("created batch contract violation is ambiguous", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		bad := enrichment.Batch{ID: "submit-batch", InputFileID: "other-file", Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CompletionWindow: "24h"}
		worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{create: &bad}, Codec: fixture.codec}
		require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		assert.Equal(t, domain.PreparedDeckBatchAmbiguous, stored.State)
		assert.Equal(t, "ambiguous_submission", stored.ErrorClass)
		assert.Equal(t, "create_contract", stored.ErrorCode)
	})

	t.Run("poll dispatch insert failure is surfaced", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		worker := &BatchSubmitWorker{Store: fixture.store, Client: &failingRiverClient{err: errors.New("insert unavailable")}, Provider: &submitFixtureProvider{}, Codec: fixture.codec}
		require.Error(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
	})
}

// TestBatchSubmitRecoversAmbiguousSubmission pins the restart recovery path: a
// chunk left in submitting state without a provider Batch is only recovered
// when exactly one exact provider match exists; a missing match or a listing
// failure is ambiguous and never retried.
func TestBatchSubmitRecoversAmbiguousSubmission(t *testing.T) {
	ctx := context.Background()

	t.Run("exact provider match is recorded", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		metadata := batchMetadata(fixture.run.Run.ID, chunk.ID, chunk.Generation)
		match := enrichment.Batch{ID: "recovered-batch", InputFileID: "recovered-input", Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CreatedAt: time.Now().Unix(), Metadata: metadata}
		provider := &submitFixtureProvider{batches: []enrichment.BatchList{{Data: []enrichment.Batch{match}}}}
		setBatchChunkState(t, ctx, fixture.store, chunk.ID, domain.PreparedDeckBatchSubmitting)
		worker := &BatchSubmitWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}
		require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		assert.Equal(t, domain.PreparedDeckBatchSubmitted, stored.State)
		assert.Equal(t, "recovered-batch", stored.BatchID)
	})

	t.Run("missing provider match is ambiguous", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		setBatchChunkState(t, ctx, fixture.store, chunk.ID, domain.PreparedDeckBatchSubmitting)
		worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{}, Codec: fixture.codec}
		require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		assert.Equal(t, domain.PreparedDeckBatchAmbiguous, stored.State)
		assert.Equal(t, "ambiguous_submission", stored.ErrorClass)
		assert.Equal(t, "creation_unconfirmed", stored.ErrorCode)
	})

	t.Run("provider listing failure is ambiguous", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		setBatchChunkState(t, ctx, fixture.store, chunk.ID, domain.PreparedDeckBatchSubmitting)
		worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{listErr: errors.New("list unavailable")}, Codec: fixture.codec}
		require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		assert.Equal(t, domain.PreparedDeckBatchAmbiguous, stored.State)
		assert.Equal(t, "ambiguous_submission", stored.ErrorClass)
		assert.Equal(t, "provider_error", stored.ErrorCode)
	})
}

// TestBatchSubmitSurfacesFrozenContractViolations pins the durable contract
// checks that fail without crossing the provider boundary: a run whose frozen
// limits no longer cover the chunk, and a frozen chunk whose model no longer
// matches the worker codec.
func TestBatchSubmitSurfacesFrozenContractViolations(t *testing.T) {
	ctx := context.Background()

	t.Run("run limits no longer cover the chunk", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		_, err := fixture.store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET batch_max_bytes=1 WHERE id=$1`, fixture.run.Run.ID)
		require.NoError(t, err)
		worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{}, Codec: fixture.codec}
		require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		assert.Equal(t, domain.PreparedDeckBatchFailed, stored.State)
		assert.Equal(t, "configuration", stored.ErrorClass)
		assert.Equal(t, "frozen_contract", stored.ErrorCode)
	})

	t.Run("chunk model no longer matches the codec", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		otherCodec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "other-model", BaseURL: "https://api.openai.com/v1"})
		require.NoError(t, err)
		worker := &BatchSubmitWorker{Store: fixture.store, Provider: &submitFixtureProvider{}, Codec: otherCodec}
		require.NoError(t, worker.Submit(ctx, fixture.args(chunk.ID, chunk.Generation)))
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		assert.Equal(t, domain.PreparedDeckBatchFailed, stored.State)
		assert.Equal(t, "configuration", stored.ErrorClass)
		assert.Equal(t, "frozen_contract", stored.ErrorCode)
	})
}
