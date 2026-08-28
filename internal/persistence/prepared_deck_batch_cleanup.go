package persistence

import (
	"context"
	"errors"
	"time"

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
	chunk, err := scanPreparedDeckBatchChunk(s.pool.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks SET cleanup_claim_token=$5,cleanup_claimed_at=now(),cleanup_lease_expires_at=$6,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4 AND state IN ('completed','cancelled') AND (cleanup_claim_token IS NULL OR cleanup_lease_expires_at<=now()) AND ((input_file_id IS NOT NULL AND input_file_cleanup_state IN ('pending','failed') AND input_file_cleanup_attempts < $7) OR (output_file_id IS NOT NULL AND output_file_cleanup_state IN ('pending','failed') AND output_file_cleanup_attempts < $7) OR (error_file_id IS NOT NULL AND error_file_cleanup_state IN ('pending','failed') AND error_file_cleanup_attempts < $7)) RETURNING `+preparedDeckBatchChunkColumns, owner, preparationID, runID, chunkID, token, leaseExpiresAt, preparedDeckBatchCleanupMaxAttempts))
	if errors.Is(err, ErrNotFound) {
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
	chunk, err := scanPreparedDeckBatchChunk(s.pool.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks SET input_file_cleanup_state=$6,output_file_cleanup_state=$7,error_file_cleanup_state=$8,input_file_cleanup_attempts=$9,output_file_cleanup_attempts=$10,error_file_cleanup_attempts=$11,cleanup_error_class=$12,cleanup_error_code=$13,cleanup_claim_token=NULL,cleanup_claimed_at=NULL,cleanup_lease_expires_at=NULL,cleanup_completed_at=CASE WHEN $6 IN ('deleted','not_needed') AND $7 IN ('deleted','not_needed') AND $8 IN ('deleted','not_needed') THEN COALESCE(cleanup_completed_at,now()) ELSE NULL END,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4 AND cleanup_claim_token=$5 AND state IN ('completed','cancelled') RETURNING `+preparedDeckBatchChunkColumns, owner, preparationID, runID, chunkID, token, update.InputFileState, update.OutputFileState, update.ErrorFileState, update.InputFileAttempts, update.OutputFileAttempts, update.ErrorFileAttempts, update.ErrorClass, update.ErrorCode))
	if errors.Is(err, ErrNotFound) {
		return domain.PreparedDeckBatchChunk{}, ErrPreparedDeckClaimLost
	}
	return chunk, err
}
