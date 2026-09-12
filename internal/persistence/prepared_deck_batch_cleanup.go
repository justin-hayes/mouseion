package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const preparedDeckBatchCleanupMaxAttempts = 3

type PreparedDeckBatchCleanupUpdate struct {
	InputFileState, OutputFileState, ErrorFileState          string
	InputFileAttempts, OutputFileAttempts, ErrorFileAttempts int
	ErrorClass, ErrorCode                                    string
}

func cleanupStateValid(state string) bool {
	return state == "pending" || state == "deleted" || state == "failed" || state == "not_needed"
}

// ClaimPreparedDeckBatchCleanup leases the cleanup projection after a chunk
// has been reconciled or cancelled. Cleanup is independent of result
// publication, so a lost cleanup worker cannot change run/artifact state.
func (s *PostgresStore) ClaimPreparedDeckBatchCleanup(ctx context.Context, owner, preparationID, runID, chunkID string, token string, leaseExpiresAt time.Time) (domain.PreparedDeckBatchChunk, error) {
	if token == "" || leaseExpiresAt.Before(time.Now()) {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	model, err := s.queries().ClaimPreparedDeckBatchCleanup(ctx, sqlcgen.ClaimPreparedDeckBatchCleanupParams{OwnerID: uuidArg(owner), PreparationID: uuidArg(preparationID), RunID: uuidArg(runID), ID: uuidArg(chunkID), CleanupClaimToken: uuidArg(token), CleanupLeaseExpiresAt: pgtype.Timestamptz{Time: leaseExpiresAt, Valid: true}, InputFileCleanupAttempts: preparedDeckBatchCleanupMaxAttempts})
	chunk := preparedDeckBatchChunkFromModel(model)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	return chunk, err
}

// FinishPreparedDeckBatchCleanup records each file independently. A provider
// deletion failure is retained as observable bounded cleanup state and never
// alters a reconciled chunk or published artifact.
func (s *PostgresStore) FinishPreparedDeckBatchCleanup(ctx context.Context, owner, preparationID, runID, chunkID, token string, update PreparedDeckBatchCleanupUpdate) (domain.PreparedDeckBatchChunk, error) {
	if !cleanupStateValid(update.InputFileState) || !cleanupStateValid(update.OutputFileState) || !cleanupStateValid(update.ErrorFileState) || update.InputFileAttempts < 0 || update.OutputFileAttempts < 0 || update.ErrorFileAttempts < 0 || update.InputFileAttempts > preparedDeckBatchCleanupMaxAttempts || update.OutputFileAttempts > preparedDeckBatchCleanupMaxAttempts || update.ErrorFileAttempts > preparedDeckBatchCleanupMaxAttempts {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	if err := validateBoundedError(update.ErrorClass, update.ErrorCode); err != nil {
		return domain.PreparedDeckBatchChunk{}, err
	}
	model, err := s.queries().FinishPreparedDeckBatchCleanup(ctx, sqlcgen.FinishPreparedDeckBatchCleanupParams{OwnerID: uuidArg(owner), PreparationID: uuidArg(preparationID), RunID: uuidArg(runID), ID: uuidArg(chunkID), CleanupClaimToken: uuidArg(token), InputFileCleanupState: update.InputFileState, OutputFileCleanupState: update.OutputFileState, ErrorFileCleanupState: update.ErrorFileState, InputFileCleanupAttempts: int32(update.InputFileAttempts), OutputFileCleanupAttempts: int32(update.OutputFileAttempts), ErrorFileCleanupAttempts: int32(update.ErrorFileAttempts), CleanupErrorClass: update.ErrorClass, CleanupErrorCode: update.ErrorCode})
	chunk := preparedDeckBatchChunkFromModel(model)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PreparedDeckBatchChunk{}, ErrPreparedDeckClaimLost
	}
	return chunk, err
}
