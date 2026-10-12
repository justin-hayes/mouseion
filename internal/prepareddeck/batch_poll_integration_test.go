//go:build integration

package prepareddeck

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"
)

// pollFixtureProvider returns one configurable provider observation per call.
type pollFixtureProvider struct {
	batch enrichment.Batch
	err   error
	calls int
}

func (p *pollFixtureProvider) GetBatch(context.Context, string) (enrichment.Batch, error) {
	p.calls++
	if p.err != nil {
		return enrichment.Batch{}, p.err
	}
	return p.batch, nil
}

func (p *pollFixtureProvider) FileContent(context.Context, string, io.Writer) error { return nil }

// preparePollChunk moves the frozen chunk into an in-flight provider
// observation with a provider Batch ID.
func preparePollChunk(t *testing.T, ctx context.Context, fixture batchSubmitFixture, chunkID string) domain.PreparedDeckBatchChunk {
	t.Helper()
	_, err := fixture.store.Pool().Exec(ctx, `UPDATE deck_preparation_batch_chunks SET state='submitted', batch_id='provider-batch', input_file_id='file-input', provider_status=NULL WHERE id=$1`, chunkID)
	require.NoError(t, err)
	return fixture.loadChunk(t, ctx, chunkID)
}

func pollArgs(fixture batchSubmitFixture, chunkID string, generation int) BatchPollJobArgs {
	return BatchPollJobArgs{OwnerID: fixture.owner.ID, PreparationID: fixture.preparation.ID, RunID: fixture.run.Run.ID, ChunkID: chunkID, Generation: generation}
}

// pollBatchFor builds a provider observation with the exact identity the
// durable chunk expects.
func pollBatchFor(fixture batchSubmitFixture, chunk domain.PreparedDeckBatchChunk, status enrichment.BatchStatus) enrichment.Batch {
	return enrichment.Batch{
		ID:            chunk.BatchID,
		InputFileID:   chunk.InputFileID,
		Endpoint:      chunk.Endpoint,
		Status:        status,
		Metadata:      batchMetadata(fixture.run.Run.ID, chunk.ID, chunk.Generation),
		RequestCounts: enrichment.BatchRequestCounts{Total: chunk.RequestCount},
	}
}

// TestBatchPollSkipsSettledAndStaleChunks pins the short-circuits that run
// before the provider is read: a stale generation, an unclaimable nonterminal
// chunk, and an already-settled chunk must leave the observation untouched.
func TestBatchPollSkipsSettledAndStaleChunks(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	chunk := fixture.run.Chunks[0]
	loaded := preparePollChunk(t, ctx, fixture, chunk.ID)
	provider := &pollFixtureProvider{}
	worker := &BatchPollWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}

	t.Run("stale generation is a no-op", func(t *testing.T) {
		require.NoError(t, worker.Poll(ctx, pollArgs(fixture, chunk.ID, loaded.ReconciliationGeneration+1)))
		require.Zero(t, provider.calls)
		require.Equal(t, domain.PreparedDeckBatchSubmitted, fixture.loadChunk(t, ctx, chunk.ID).State)
	})
	t.Run("unknown chunk is surfaced", func(t *testing.T) {
		require.ErrorIs(t, worker.Poll(ctx, pollArgs(fixture, uuid.NewString(), loaded.ReconciliationGeneration)), persistence.ErrNotFound)
	})
	t.Run("unclaimable nonterminal chunk is a no-op", func(t *testing.T) {
		setBatchChunkState(t, ctx, fixture.store, chunk.ID, domain.PreparedDeckBatchPending)
		require.NoError(t, worker.Poll(ctx, pollArgs(fixture, chunk.ID, loaded.ReconciliationGeneration)))
		require.Zero(t, provider.calls)
		require.Equal(t, domain.PreparedDeckBatchPending, fixture.loadChunk(t, ctx, chunk.ID).State)
	})
	t.Run("settled chunk is a no-op", func(t *testing.T) {
		setBatchChunkState(t, ctx, fixture.store, chunk.ID, domain.PreparedDeckBatchCompleted)
		require.NoError(t, worker.Poll(ctx, pollArgs(fixture, chunk.ID, loaded.ReconciliationGeneration)))
		require.Zero(t, provider.calls)
	})
}

// TestBatchPollSnoozesNonterminalProviderStatus pins the nonterminal branch:
// the observed status is persisted and the job snoozes instead of reconciling.
func TestBatchPollSnoozesNonterminalProviderStatus(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	chunk := fixture.run.Chunks[0]
	loaded := preparePollChunk(t, ctx, fixture, chunk.ID)
	provider := &pollFixtureProvider{batch: pollBatchFor(fixture, loaded, enrichment.BatchStatusInProgress)}
	worker := &BatchPollWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}

	err := worker.Poll(ctx, pollArgs(fixture, chunk.ID, loaded.ReconciliationGeneration))

	var snooze *river.JobSnoozeError
	require.ErrorAs(t, err, &snooze)
	t.Run("reads the provider once", func(t *testing.T) {
		require.Equal(t, 1, provider.calls)
	})
	t.Run("persists the nonterminal status", func(t *testing.T) {
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, domain.PreparedDeckBatchPolling, stored.State)
		require.Equal(t, string(enrichment.BatchStatusInProgress), stored.ProviderStatus)
	})
}

// TestBatchPollFailsUntrustworthyObservations pins every observation that must
// not reconcile results: a temporary provider error snoozes with a poll
// diagnostic, while a permanent error, an unknown status, an identity mismatch,
// and a cancelled provider Batch all fail the chunk with the bounded class and
// code the split must preserve.
func TestBatchPollFailsUntrustworthyObservations(t *testing.T) {
	ctx := context.Background()

	t.Run("temporary provider error snoozes", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		loaded := preparePollChunk(t, ctx, fixture, chunk.ID)
		provider := &pollFixtureProvider{err: &enrichment.ProviderError{Operation: "get", Class: enrichment.ProviderErrorRateLimit, StatusCode: http.StatusTooManyRequests}}
		worker := &BatchPollWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}

		err := worker.Poll(ctx, pollArgs(fixture, chunk.ID, loaded.ReconciliationGeneration))

		var snooze *river.JobSnoozeError
		require.ErrorAs(t, err, &snooze)
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, domain.PreparedDeckBatchPolling, stored.State)
		require.Equal(t, "poll", stored.ErrorClass)
		require.Equal(t, "rate_limit", stored.ErrorCode)
	})

	t.Run("permanent provider error fails the chunk", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		loaded := preparePollChunk(t, ctx, fixture, chunk.ID)
		provider := &pollFixtureProvider{err: &enrichment.ProviderError{Operation: "get", Class: enrichment.ProviderErrorInvalidRequest}}
		worker := &BatchPollWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}

		require.NoError(t, worker.Poll(ctx, pollArgs(fixture, chunk.ID, loaded.ReconciliationGeneration)))

		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, domain.PreparedDeckBatchFailed, stored.State)
		require.Equal(t, "reconciliation", stored.ErrorClass)
		require.Equal(t, "invalid_request", stored.ErrorCode)
	})

	t.Run("unknown provider status fails the chunk", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		loaded := preparePollChunk(t, ctx, fixture, chunk.ID)
		provider := &pollFixtureProvider{batch: pollBatchFor(fixture, loaded, enrichment.BatchStatus("future_status"))}
		worker := &BatchPollWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}

		require.NoError(t, worker.Poll(ctx, pollArgs(fixture, chunk.ID, loaded.ReconciliationGeneration)))

		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, domain.PreparedDeckBatchFailed, stored.State)
		require.Equal(t, "validation", stored.ErrorClass)
		require.Equal(t, "unsupported_status", stored.ErrorCode)
	})

	t.Run("identity mismatch fails the chunk", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		loaded := preparePollChunk(t, ctx, fixture, chunk.ID)
		batch := pollBatchFor(fixture, loaded, enrichment.BatchStatusInProgress)
		batch.ID = "other-batch"
		provider := &pollFixtureProvider{batch: batch}
		worker := &BatchPollWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}

		require.NoError(t, worker.Poll(ctx, pollArgs(fixture, chunk.ID, loaded.ReconciliationGeneration)))

		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, domain.PreparedDeckBatchFailed, stored.State)
		require.Equal(t, "validation", stored.ErrorClass)
		require.Equal(t, "batch_identity", stored.ErrorCode)
	})

	t.Run("cancelled provider batch fails the chunk", func(t *testing.T) {
		fixture := newBatchSubmitFixture(t, ctx)
		chunk := fixture.run.Chunks[0]
		loaded := preparePollChunk(t, ctx, fixture, chunk.ID)
		provider := &pollFixtureProvider{batch: pollBatchFor(fixture, loaded, enrichment.BatchStatusCancelled)}
		worker := &BatchPollWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}

		require.NoError(t, worker.Poll(ctx, pollArgs(fixture, chunk.ID, loaded.ReconciliationGeneration)))

		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, domain.PreparedDeckBatchFailed, stored.State)
		require.Equal(t, "cancelled", stored.ErrorClass)
		require.Equal(t, "cancelled", stored.ErrorCode)
	})
}
