//go:build integration

package prepareddeck

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/require"
)

// cleanupProvider records each provider file deletion so tests can assert which
// files were attempted and which were skipped.
type cleanupProvider struct {
	deleted []string
	err     error
}

func (p *cleanupProvider) DeleteFile(_ context.Context, id string) (enrichment.OpenAIFileDeletion, error) {
	p.deleted = append(p.deleted, id)
	if p.err != nil {
		return enrichment.OpenAIFileDeletion{}, p.err
	}
	return enrichment.OpenAIFileDeletion{ID: id, Deleted: true}, nil
}

// settleChunkForCleanup marks the frozen chunk settled with a present input
// file and assigns the given output and error file IDs. An empty ID means the
// file is absent.
func settleChunkForCleanup(t *testing.T, ctx context.Context, fixture batchSubmitFixture, chunkID, state, output, errFile string) {
	t.Helper()
	_, err := fixture.store.Pool().Exec(ctx, `UPDATE deck_preparation_batch_chunks SET state=$2, input_file_id='file-input', output_file_id=NULLIF($3,''), error_file_id=NULLIF($4,'') WHERE id=$1`, chunkID, state, output, errFile)
	require.NoError(t, err)
}

func cleanupArgs(fixture batchSubmitFixture, chunkID string) BatchCleanupJobArgs {
	return BatchCleanupJobArgs{OwnerID: fixture.owner.ID, PreparationID: fixture.preparation.ID, RunID: fixture.run.Run.ID, ChunkID: chunkID}
}

// TestBatchCleanupDeletesEachFileIndependently pins one cleanup pass: every
// provider file is deleted once, each file's state advances independently, and
// cleanup completes only when no file is left pending. A second pass has
// nothing to claim.
func TestBatchCleanupDeletesEachFileIndependently(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	chunk := fixture.run.Chunks[0]
	settleChunkForCleanup(t, ctx, fixture, chunk.ID, string(domain.PreparedDeckBatchCompleted), "file-output", "file-error")
	provider := &cleanupProvider{}
	worker := &BatchCleanupWorker{Store: fixture.store, Provider: provider}

	require.NoError(t, worker.Cleanup(ctx, cleanupArgs(fixture, chunk.ID)))

	t.Run("deletes every provider file", func(t *testing.T) {
		require.ElementsMatch(t, []string{"file-input", "file-output", "file-error"}, provider.deleted)
	})
	t.Run("marks every file deleted", func(t *testing.T) {
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, "deleted", stored.InputFileCleanupState)
		require.Equal(t, "deleted", stored.OutputFileCleanupState)
		require.Equal(t, "deleted", stored.ErrorFileCleanupState)
	})
	t.Run("records one attempt per file", func(t *testing.T) {
		require.Equal(t, 1, fixture.loadChunk(t, ctx, chunk.ID).InputFileCleanupAttempts)
	})
	t.Run("completes cleanup only when every file is settled", func(t *testing.T) {
		require.NotNil(t, fixture.loadChunk(t, ctx, chunk.ID).CleanupCompletedAt)
	})
	t.Run("a settled chunk is not deleted twice", func(t *testing.T) {
		require.NoError(t, worker.Cleanup(ctx, cleanupArgs(fixture, chunk.ID)))
		require.Len(t, provider.deleted, 3)
	})
}

// TestBatchCleanupSkipsUnsettledChunk pins the state guard: a chunk that has
// not completed or been cancelled is never claimed or deleted.
func TestBatchCleanupSkipsUnsettledChunk(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	chunk := fixture.run.Chunks[0]
	settleChunkForCleanup(t, ctx, fixture, chunk.ID, string(domain.PreparedDeckBatchPending), "file-output", "file-error")
	provider := &cleanupProvider{}
	worker := &BatchCleanupWorker{Store: fixture.store, Provider: provider}

	require.NoError(t, worker.Cleanup(ctx, cleanupArgs(fixture, chunk.ID)))

	t.Run("does not delete any file", func(t *testing.T) {
		require.Empty(t, provider.deleted)
	})
	t.Run("leaves the chunk untouched", func(t *testing.T) {
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, domain.PreparedDeckBatchPending, stored.State)
		require.Equal(t, "pending", stored.InputFileCleanupState)
	})
}

// TestBatchCleanupMarksAbsentFilesNotNeeded pins the absent-file branch: a
// missing provider file is recorded as not_needed without a delete attempt.
func TestBatchCleanupMarksAbsentFilesNotNeeded(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	chunk := fixture.run.Chunks[0]
	settleChunkForCleanup(t, ctx, fixture, chunk.ID, string(domain.PreparedDeckBatchCompleted), "", "")
	provider := &cleanupProvider{}
	worker := &BatchCleanupWorker{Store: fixture.store, Provider: provider}

	require.NoError(t, worker.Cleanup(ctx, cleanupArgs(fixture, chunk.ID)))

	t.Run("deletes only the present file", func(t *testing.T) {
		require.Equal(t, []string{"file-input"}, provider.deleted)
	})
	t.Run("records absent files as not needed", func(t *testing.T) {
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, "deleted", stored.InputFileCleanupState)
		require.Equal(t, "not_needed", stored.OutputFileCleanupState)
		require.Equal(t, "not_needed", stored.ErrorFileCleanupState)
	})
	t.Run("completes cleanup", func(t *testing.T) {
		require.NotNil(t, fixture.loadChunk(t, ctx, chunk.ID).CleanupCompletedAt)
	})
}

// TestBatchCleanupSkipsSettledAndExhaustedFiles pins the per-file skip: an
// already-deleted file and a file that has used every attempt are neither
// deleted again nor changed.
func TestBatchCleanupSkipsSettledAndExhaustedFiles(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	chunk := fixture.run.Chunks[0]
	_, err := fixture.store.Pool().Exec(ctx, `UPDATE deck_preparation_batch_chunks SET state='completed', input_file_id='file-input', output_file_id='file-output', output_file_cleanup_state='deleted', error_file_id='file-error', error_file_cleanup_state='pending', error_file_cleanup_attempts=3 WHERE id=$1`, chunk.ID)
	require.NoError(t, err)
	provider := &cleanupProvider{}
	worker := &BatchCleanupWorker{Store: fixture.store, Provider: provider}

	require.NoError(t, worker.Cleanup(ctx, cleanupArgs(fixture, chunk.ID)))

	t.Run("deletes only the claimable file", func(t *testing.T) {
		require.Equal(t, []string{"file-input"}, provider.deleted)
	})
	t.Run("keeps the settled and exhausted files unchanged", func(t *testing.T) {
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, "deleted", stored.OutputFileCleanupState)
		require.Equal(t, "pending", stored.ErrorFileCleanupState)
		require.Equal(t, 3, stored.ErrorFileCleanupAttempts)
	})
	t.Run("leaves cleanup incomplete for recovery", func(t *testing.T) {
		require.Nil(t, fixture.loadChunk(t, ctx, chunk.ID).CleanupCompletedAt)
	})
}

// TestBatchCleanupRetainsProviderFailureForRecovery pins the failure branch:
// every attempted delete fails, each file records one attempt and a failed
// state, and the bounded delete_file diagnostic is retained for recovery.
func TestBatchCleanupRetainsProviderFailureForRecovery(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	chunk := fixture.run.Chunks[0]
	settleChunkForCleanup(t, ctx, fixture, chunk.ID, string(domain.PreparedDeckBatchCancelled), "file-output", "file-error")
	provider := &cleanupProvider{err: errors.New("delete unavailable")}
	worker := &BatchCleanupWorker{Store: fixture.store, Provider: provider}

	require.NoError(t, worker.Cleanup(ctx, cleanupArgs(fixture, chunk.ID)))

	t.Run("attempts every file", func(t *testing.T) {
		require.Len(t, provider.deleted, 3)
	})
	t.Run("marks every file failed with one attempt", func(t *testing.T) {
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, "failed", stored.InputFileCleanupState)
		require.Equal(t, "failed", stored.OutputFileCleanupState)
		require.Equal(t, "failed", stored.ErrorFileCleanupState)
		require.Equal(t, 1, stored.InputFileCleanupAttempts)
		require.Equal(t, 1, stored.OutputFileCleanupAttempts)
		require.Equal(t, 1, stored.ErrorFileCleanupAttempts)
	})
	t.Run("records the bounded delete diagnostic", func(t *testing.T) {
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, "provider", stored.CleanupErrorClass)
		require.Equal(t, "delete_file", stored.CleanupErrorCode)
	})
	t.Run("leaves cleanup incomplete", func(t *testing.T) {
		require.Nil(t, fixture.loadChunk(t, ctx, chunk.ID).CleanupCompletedAt)
	})
}

// TestBatchCleanupTreatsGoneProviderFileAsDeleted pins the 404 branch: a file
// the provider has already removed is a successful deletion.
func TestBatchCleanupTreatsGoneProviderFileAsDeleted(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	chunk := fixture.run.Chunks[0]
	settleChunkForCleanup(t, ctx, fixture, chunk.ID, string(domain.PreparedDeckBatchCompleted), "file-output", "file-error")
	provider := &cleanupProvider{err: &enrichment.ProviderError{Operation: "delete", Class: enrichment.ProviderErrorInvalidRequest, StatusCode: http.StatusNotFound}}
	worker := &BatchCleanupWorker{Store: fixture.store, Provider: provider}

	require.NoError(t, worker.Cleanup(ctx, cleanupArgs(fixture, chunk.ID)))

	t.Run("marks every file deleted", func(t *testing.T) {
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Equal(t, "deleted", stored.InputFileCleanupState)
		require.Equal(t, "deleted", stored.OutputFileCleanupState)
		require.Equal(t, "deleted", stored.ErrorFileCleanupState)
	})
	t.Run("records no cleanup error and completes", func(t *testing.T) {
		stored := fixture.loadChunk(t, ctx, chunk.ID)
		require.Empty(t, stored.CleanupErrorCode)
		require.NotNil(t, stored.CleanupCompletedAt)
	})
}

// TestBatchCleanupSurfacesUnknownChunk pins that a missing chunk is an error,
// not a silent success.
func TestBatchCleanupSurfacesUnknownChunk(t *testing.T) {
	ctx := context.Background()
	fixture := newBatchSubmitFixture(t, ctx)
	worker := &BatchCleanupWorker{Store: fixture.store, Provider: &cleanupProvider{}}

	require.ErrorIs(t, worker.Cleanup(ctx, cleanupArgs(fixture, uuid.NewString())), persistence.ErrNotFound)
}
