//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreparedDeckBatchWritesEnforceFromStatesOnTheDatabase pins the SQL
// predicates that make the database the authority for the two Batch writes that
// previously relied only on a Go check under a row lock. A write against a row
// in a disallowed state must return the invalid-transition error and leave the
// row unchanged.
func TestPreparedDeckBatchWritesEnforceFromStatesOnTheDatabase(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	fixture := newPreparedDeckFreezeFixture(t, ctx, store, testutil.PresentationProvider{Name: "openai", Version: "v1", TargetLanguage: "en"}, []cardexport.Entry{preparedDeckFreezeEntry()})
	params := FreezePreparedDeckRunParams{
		OwnerID: fixture.ownerID, PreparationID: fixture.preparation.ID, Projection: fixture.projection,
		Config: preparedDeckExternalConfig(),
		Chunks: []PreparedDeckBatchChunkPlan{preparedDeckSingleChunkPlan()},
	}
	frozen := freezePreparedDeckRun(t, ctx, store, params, 0)
	chunk := frozen.Chunks[0]

	t.Run("completing a chunk from a non-reconciling state is rejected", func(t *testing.T) {
		_, err := store.Pool().Exec(ctx, `UPDATE deck_preparation_batch_chunks SET state = 'polling' WHERE owner_id = $1 AND id = $2`, fixture.ownerID, chunk.ID)
		require.NoError(t, err)

		tx, err := store.Pool().Begin(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { rollbackIntegrationTx(t, ctx, tx) })
		_, err = completePreparedDeckBatchChunk(ctx, tx, PreparedDeckBatchReconcileParams{
			OwnerID: fixture.ownerID, PreparationID: fixture.preparation.ID, RunID: frozen.Run.ID, ChunkID: chunk.ID,
			Chunk: PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchCompleted, ProviderStatus: "completed", CompletedCount: 1},
		})
		require.ErrorIs(t, err, ErrInvalidTransition)

		var state string
		var completedCount int
		var reconciledAtUnset bool
		require.NoError(t, tx.QueryRow(ctx, `SELECT state, completed_count, reconciled_at IS NULL FROM deck_preparation_batch_chunks WHERE owner_id = $1 AND id = $2`, fixture.ownerID, chunk.ID).Scan(&state, &completedCount, &reconciledAtUnset))
		assert.Equal(t, "polling", state, "rejected write must not change the chunk state")
		assert.Zero(t, completedCount, "rejected write must not record counts")
		assert.True(t, reconciledAtUnset, "rejected write must not settle the chunk")
	})

	t.Run("updating a terminal outcome from a Batch result is rejected", func(t *testing.T) {
		_, err := store.Pool().Exec(ctx, `UPDATE deck_preparation_translation_outcomes SET state = 'completed', terminal_at = now() WHERE owner_id = $1 AND run_id = $2 AND ordinal = 0`, fixture.ownerID, frozen.Run.ID)
		require.NoError(t, err)

		tx, err := store.Pool().Begin(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { rollbackIntegrationTx(t, ctx, tx) })
		terminalAt := time.Now().UTC()
		err = updatePreparedDeckBatchOutcome(ctx, tx, PreparedDeckBatchReconcileParams{
			OwnerID: fixture.ownerID, PreparationID: fixture.preparation.ID, RunID: frozen.Run.ID, ChunkID: chunk.ID,
		}, chunk, PreparedDeckBatchItemReconciliation{Ordinal: 0, State: domain.PreparedDeckOutcomeFailed, ErrorClass: "provider", ErrorCode: "timeout"}, &terminalAt)
		require.ErrorIs(t, err, ErrInvalidTransition)

		var state string
		var attempts int
		var terminalSet bool
		require.NoError(t, tx.QueryRow(ctx, `SELECT state, provider_attempt_count, terminal_at IS NOT NULL FROM deck_preparation_translation_outcomes WHERE owner_id = $1 AND run_id = $2 AND ordinal = 0`, fixture.ownerID, frozen.Run.ID).Scan(&state, &attempts, &terminalSet))
		assert.Equal(t, "completed", state, "rejected write must not change the outcome state")
		assert.Zero(t, attempts, "rejected write must not record a provider attempt")
		assert.True(t, terminalSet, "rejected write must not clear the terminal timestamp")
	})

	t.Run("database errors are returned unchanged", func(t *testing.T) {
		_, err := store.Pool().Exec(ctx, `UPDATE deck_preparation_translation_outcomes SET state = 'pending', terminal_at = NULL WHERE owner_id = $1 AND run_id = $2 AND ordinal = 0`, fixture.ownerID, frozen.Run.ID)
		require.NoError(t, err)

		tx, err := store.Pool().Begin(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { rollbackIntegrationTx(t, ctx, tx) })
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		err = updatePreparedDeckBatchOutcome(cancelled, tx, PreparedDeckBatchReconcileParams{
			OwnerID: fixture.ownerID, PreparationID: fixture.preparation.ID, RunID: frozen.Run.ID, ChunkID: chunk.ID,
		}, chunk, PreparedDeckBatchItemReconciliation{Ordinal: 0, State: domain.PreparedDeckOutcomeCompleted}, nil)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrInvalidTransition)
	})
}

// TestPreparedDeckBatchCleanupStatesMatchDatabaseConstraint proves that every
// domain cleanup-state value is accepted by the columns' check constraint.
func TestPreparedDeckBatchCleanupStatesMatchDatabaseConstraint(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	fixture := newPreparedDeckFreezeFixture(t, ctx, store, testutil.PresentationProvider{Name: "openai", Version: "v1", TargetLanguage: "en"}, []cardexport.Entry{preparedDeckFreezeEntry()})
	params := FreezePreparedDeckRunParams{
		OwnerID: fixture.ownerID, PreparationID: fixture.preparation.ID, Projection: fixture.projection,
		Config: preparedDeckExternalConfig(),
		Chunks: []PreparedDeckBatchChunkPlan{preparedDeckSingleChunkPlan()},
	}
	frozen := freezePreparedDeckRun(t, ctx, store, params, 0)
	chunk := frozen.Chunks[0]

	states := []domain.PreparedDeckBatchFileCleanupState{
		domain.PreparedDeckBatchCleanupPending,
		domain.PreparedDeckBatchCleanupDeleted,
		domain.PreparedDeckBatchCleanupFailed,
		domain.PreparedDeckBatchCleanupNotNeeded,
	}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			_, err := store.Pool().Exec(ctx, `UPDATE deck_preparation_batch_chunks
				SET input_file_cleanup_state = $3, output_file_cleanup_state = $3, error_file_cleanup_state = $3
				WHERE owner_id = $1 AND id = $2`, fixture.ownerID, chunk.ID, string(state))
			require.NoError(t, err, "domain cleanup state %q must be accepted by the database constraint", state)
		})
	}
}
