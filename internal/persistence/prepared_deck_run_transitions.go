package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func validClaim(token string, leaseExpiresAt time.Time) bool {
	_, err := uuid.Parse(token)
	return err == nil && leaseExpiresAt.After(time.Now().UTC())
}

func (s *PostgresStore) outcomeTransitionError(ctx context.Context, owner, preparationID, runID string, ordinal int, err error) error {
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	if _, getErr := s.GetPreparedDeckTranslationOutcome(ctx, owner, preparationID, runID, ordinal); getErr != nil {
		return getErr
	}
	return ErrPreparedDeckClaimLost
}

// ClaimPreparedDeckTranslationOutcome grants one generation a leased item
// claim. The token is required by every subsequent mutation.
func (s *PostgresStore) ClaimPreparedDeckTranslationOutcome(ctx context.Context, owner, preparationID, runID string, ordinal, generation int, token string, leaseExpiresAt time.Time) (domain.PreparedDeckTranslationOutcome, error) {
	if !validClaim(token, leaseExpiresAt) {
		return domain.PreparedDeckTranslationOutcome{}, ErrInvalidTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PreparedDeckTranslationOutcome{}, err
	}
	defer tx.Rollback(ctx)
	outcome, err := scanPreparedDeckOutcome(tx.QueryRow(ctx, `UPDATE deck_preparation_translation_outcomes o SET state='running',dispatch_count=dispatch_count+1,claim_token=$6,claimed_at=now(),lease_expires_at=$7,error_class='',error_code='',updated_at=now()
		FROM deck_preparation_runs r WHERE o.owner_id=$1 AND o.preparation_id=$2 AND o.run_id=$3 AND o.ordinal=$4 AND o.dispatch_generation=$5 AND o.state='pending' AND o.next_attempt_at<=now()
		AND r.owner_id=o.owner_id AND r.preparation_id=o.preparation_id AND r.id=o.run_id AND r.state='translating' AND r.translation_state IN ('pending','running') RETURNING `+qualifiedColumns("o", preparedDeckOutcomeColumns), owner, preparationID, runID, ordinal, generation, token, leaseExpiresAt))
	if err != nil {
		return outcome, s.outcomeTransitionError(ctx, owner, preparationID, runID, ordinal, err)
	}
	if _, err = tx.Exec(ctx, `UPDATE deck_preparation_runs SET translation_state='running',updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND translation_state='pending'`, owner, preparationID, runID); err != nil {
		return domain.PreparedDeckTranslationOutcome{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PreparedDeckTranslationOutcome{}, err
	}
	return outcome, nil
}

// RetryPreparedDeckTranslationOutcome persists one failed provider attempt and
// releases the item claim for a later, fenced dispatch.
func (s *PostgresStore) RetryPreparedDeckTranslationOutcome(ctx context.Context, owner, preparationID, runID string, ordinal, generation int, token string, nextAttemptAt time.Time, errorClass, errorCode string) (domain.PreparedDeckTranslationOutcome, error) {
	if err := validateBoundedError(errorClass, errorCode); err != nil {
		return domain.PreparedDeckTranslationOutcome{}, err
	}
	outcome, err := scanPreparedDeckOutcome(s.pool.QueryRow(ctx, `UPDATE deck_preparation_translation_outcomes o SET state='pending',provider_attempt_count=provider_attempt_count+1,next_attempt_at=$7,claim_token=NULL,claimed_at=NULL,lease_expires_at=NULL,error_class=$8,error_code=$9,updated_at=now()
		FROM deck_preparation_runs r WHERE o.owner_id=$1 AND o.preparation_id=$2 AND o.run_id=$3 AND o.ordinal=$4 AND o.dispatch_generation=$5 AND o.claim_token=$6 AND o.state='running' AND o.provider_attempt_count<o.max_provider_attempts
		AND r.owner_id=o.owner_id AND r.preparation_id=o.preparation_id AND r.id=o.run_id AND r.state='translating' AND r.translation_state IN ('pending','running') RETURNING `+qualifiedColumns("o", preparedDeckOutcomeColumns), owner, preparationID, runID, ordinal, generation, token, nextAttemptAt, errorClass, errorCode))
	if err != nil {
		return outcome, s.outcomeTransitionError(ctx, owner, preparationID, runID, ordinal, err)
	}
	return outcome, nil
}

type PreparedDeckOutcomeTerminalUpdate struct {
	State           domain.PreparedDeckOutcomeState
	ErrorClass      string
	ErrorCode       string
	ProviderAttempt bool
	CacheHit        bool
	ProviderCall    bool
	CacheLatency    time.Duration
	ProviderLatency time.Duration
}

// FinishPreparedDeckTranslationOutcome makes one claimed item terminal and,
// when it is the last item, atomically advances the run and inserts finalizer
// work through insertFinalizer.
func (s *PostgresStore) FinishPreparedDeckTranslationOutcome(ctx context.Context, owner, preparationID, runID string, ordinal, generation int, token string, update PreparedDeckOutcomeTerminalUpdate, insertFinalizer PreparedDeckRunJobInserter) (domain.PreparedDeckTranslationOutcome, domain.PreparedDeckRun, error) {
	if update.State != domain.PreparedDeckOutcomeCompleted && update.State != domain.PreparedDeckOutcomeFailed {
		return domain.PreparedDeckTranslationOutcome{}, domain.PreparedDeckRun{}, ErrInvalidTransition
	}
	if err := validateBoundedError(update.ErrorClass, update.ErrorCode); err != nil {
		return domain.PreparedDeckTranslationOutcome{}, domain.PreparedDeckRun{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PreparedDeckTranslationOutcome{}, domain.PreparedDeckRun{}, err
	}
	defer tx.Rollback(ctx)
	run, err := scanPreparedDeckRun(tx.QueryRow(ctx, `SELECT `+preparedDeckRunColumns+` FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 FOR UPDATE`, owner, preparationID, runID))
	if err != nil {
		return domain.PreparedDeckTranslationOutcome{}, domain.PreparedDeckRun{}, err
	}
	if run.State != domain.PreparedDeckRunTranslating || (run.TranslationState != domain.PreparedDeckTranslationPending && run.TranslationState != domain.PreparedDeckTranslationRunning) {
		existing, getErr := scanPreparedDeckOutcome(tx.QueryRow(ctx, `SELECT `+preparedDeckOutcomeColumns+` FROM deck_preparation_translation_outcomes WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND ordinal=$4`, owner, preparationID, runID, ordinal))
		if getErr == nil && existing.State == update.State {
			if err = tx.Commit(ctx); err != nil {
				return domain.PreparedDeckTranslationOutcome{}, domain.PreparedDeckRun{}, err
			}
			return existing, run, nil
		}
		if getErr != nil && !errors.Is(getErr, ErrNotFound) {
			return domain.PreparedDeckTranslationOutcome{}, run, getErr
		}
		return domain.PreparedDeckTranslationOutcome{}, run, ErrInvalidTransition
	}
	cacheLatencyMS, providerLatencyMS := update.CacheLatency.Milliseconds(), update.ProviderLatency.Milliseconds()
	if cacheLatencyMS < 0 || providerLatencyMS < 0 {
		return domain.PreparedDeckTranslationOutcome{}, run, ErrInvalidTransition
	}
	outcome, err := scanPreparedDeckOutcome(tx.QueryRow(ctx, `UPDATE deck_preparation_translation_outcomes SET state=$7,provider_attempt_count=provider_attempt_count+$8,claim_token=NULL,claimed_at=NULL,lease_expires_at=NULL,terminal_at=now(),error_class=$9,error_code=$10,cache_hit_count=cache_hit_count+$11,provider_call_count=provider_call_count+$12,cache_latency_ms=cache_latency_ms+$13,provider_latency_ms=provider_latency_ms+$14,updated_at=now()
		WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND ordinal=$4 AND dispatch_generation=$5 AND claim_token=$6 AND state='running' RETURNING `+preparedDeckOutcomeColumns,
		owner, preparationID, runID, ordinal, generation, token, update.State, boolInt(update.ProviderAttempt), update.ErrorClass, update.ErrorCode, boolInt(update.CacheHit), boolInt(update.ProviderCall), cacheLatencyMS, providerLatencyMS))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			existing, getErr := scanPreparedDeckOutcome(tx.QueryRow(ctx, `SELECT `+preparedDeckOutcomeColumns+` FROM deck_preparation_translation_outcomes WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND ordinal=$4`, owner, preparationID, runID, ordinal))
			if getErr == nil && existing.State == update.State {
				if err = tx.Commit(ctx); err != nil {
					return domain.PreparedDeckTranslationOutcome{}, domain.PreparedDeckRun{}, err
				}
				return existing, run, nil
			}
			if getErr != nil && !errors.Is(getErr, ErrNotFound) {
				return domain.PreparedDeckTranslationOutcome{}, run, getErr
			}
			return domain.PreparedDeckTranslationOutcome{}, run, ErrPreparedDeckClaimLost
		}
		return domain.PreparedDeckTranslationOutcome{}, run, err
	}
	var nonterminal int
	if err = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE state='completed'),count(*) FILTER (WHERE state='failed'),count(*) FILTER (WHERE state IN ('pending','running')) FROM deck_preparation_translation_outcomes WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3`, owner, preparationID, runID).Scan(&run.CompletedCount, &run.FailedCount, &nonterminal); err != nil {
		return domain.PreparedDeckTranslationOutcome{}, run, err
	}
	if nonterminal == 0 {
		run, err = scanPreparedDeckRun(tx.QueryRow(ctx, `UPDATE deck_preparation_runs SET state='finalizing',translation_state='completed',completed_count=$4,failed_count=$5,translation_completed_at=COALESCE(translation_completed_at,now()),updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND state='translating' RETURNING `+preparedDeckRunColumns, owner, preparationID, runID, run.CompletedCount, run.FailedCount))
		if err != nil {
			return domain.PreparedDeckTranslationOutcome{}, run, err
		}
		if insertFinalizer != nil {
			if err = insertFinalizer(ctx, tx, run); err != nil {
				return domain.PreparedDeckTranslationOutcome{}, run, err
			}
		}
	} else {
		run, err = scanPreparedDeckRun(tx.QueryRow(ctx, `UPDATE deck_preparation_runs SET translation_state='running',completed_count=$4,failed_count=$5,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 RETURNING `+preparedDeckRunColumns, owner, preparationID, runID, run.CompletedCount, run.FailedCount))
		if err != nil {
			return domain.PreparedDeckTranslationOutcome{}, run, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PreparedDeckTranslationOutcome{}, domain.PreparedDeckRun{}, err
	}
	return outcome, run, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// RedispatchPreparedDeckTranslationOutcome reclaims only expired work and
// increments the generation that stale workers must match.
func (s *PostgresStore) RedispatchPreparedDeckTranslationOutcome(ctx context.Context, owner, preparationID, runID string, ordinal, expectedGeneration int) (domain.PreparedDeckTranslationOutcome, error) {
	outcome, err := scanPreparedDeckOutcome(s.pool.QueryRow(ctx, `UPDATE deck_preparation_translation_outcomes o SET state='pending',dispatch_generation=dispatch_generation+1,river_job_id=NULL,claim_token=NULL,claimed_at=NULL,lease_expires_at=NULL,next_attempt_at=now(),error_class='orchestration',error_code='expired_lease',updated_at=now()
		FROM deck_preparation_runs r WHERE o.owner_id=$1 AND o.preparation_id=$2 AND o.run_id=$3 AND o.ordinal=$4 AND o.dispatch_generation=$5 AND o.state='running' AND o.lease_expires_at<=now()
		AND r.owner_id=o.owner_id AND r.preparation_id=o.preparation_id AND r.id=o.run_id AND r.state='translating' RETURNING `+qualifiedColumns("o", preparedDeckOutcomeColumns), owner, preparationID, runID, ordinal, expectedGeneration))
	if err != nil {
		return outcome, s.outcomeTransitionError(ctx, owner, preparationID, runID, ordinal, err)
	}
	return outcome, nil
}

// ClaimPreparedDeckFinalization grants a leased, generation-fenced finalizer
// claim only for the current owner-scoped run.
func (s *PostgresStore) ClaimPreparedDeckFinalization(ctx context.Context, owner, preparationID, runID string, generation int, token string, leaseExpiresAt time.Time) (domain.PreparedDeckRun, error) {
	if !validClaim(token, leaseExpiresAt) {
		return domain.PreparedDeckRun{}, ErrInvalidTransition
	}
	run, err := scanPreparedDeckRun(s.pool.QueryRow(ctx, `UPDATE deck_preparation_runs r SET finalization_claim_token=$5,finalization_claimed_at=now(),finalization_lease_expires_at=$6,finalization_dispatch_count=finalization_dispatch_count+1,updated_at=now()
		FROM deck_preparations p WHERE r.owner_id=$1 AND r.preparation_id=$2 AND r.id=$3 AND r.finalization_dispatch_generation=$4 AND r.state='finalizing' AND r.translation_state='completed'
		AND (r.finalization_claim_token IS NULL OR r.finalization_lease_expires_at<=now()) AND p.owner_id=r.owner_id AND p.id=r.preparation_id AND p.current_run_id=r.id AND p.state='preparing' RETURNING `+qualifiedColumns("r", preparedDeckRunColumns), owner, preparationID, runID, generation, token, leaseExpiresAt))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			existing, getErr := s.GetPreparedDeckRun(ctx, owner, preparationID, runID)
			if getErr != nil {
				return run, getErr
			}
			if existing.State == domain.PreparedDeckRunCompleted {
				return existing, nil
			}
			return run, ErrPreparedDeckClaimLost
		}
	}
	return run, err
}

func (s *PostgresStore) AssignPreparedDeckFinalizationJob(ctx context.Context, owner, preparationID, runID string, expectedGeneration int, jobID int64) (domain.PreparedDeckRun, error) {
	if jobID < 1 {
		return domain.PreparedDeckRun{}, ErrInvalidTransition
	}
	run, err := scanPreparedDeckRun(s.pool.QueryRow(ctx, `UPDATE deck_preparation_runs SET finalization_dispatch_generation=finalization_dispatch_generation+1,finalization_job_id=$5,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND finalization_dispatch_generation=$4 AND state='finalizing' AND translation_state='completed' RETURNING `+preparedDeckRunColumns, owner, preparationID, runID, expectedGeneration, jobID))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return run, ErrInvalidTransition
		}
	}
	return run, err
}

func (s *PostgresStore) AssignPreparedDeckBatchSubmissionJob(ctx context.Context, owner, preparationID, runID, chunkID string, expectedGeneration int, jobID int64) (domain.PreparedDeckBatchChunk, error) {
	if jobID < 1 {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	chunk, err := scanPreparedDeckBatchChunk(s.pool.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks c SET submission_generation=submission_generation+1,submission_job_id=$6,updated_at=now() FROM deck_preparation_runs r WHERE c.owner_id=$1 AND c.preparation_id=$2 AND c.run_id=$3 AND c.id=$4 AND c.submission_generation=$5 AND c.state='pending' AND r.owner_id=c.owner_id AND r.preparation_id=c.preparation_id AND r.id=c.run_id AND r.state='translating' RETURNING `+qualifiedColumns("c", preparedDeckBatchChunkColumns), owner, preparationID, runID, chunkID, expectedGeneration, jobID))
	if err != nil && errors.Is(err, ErrNotFound) {
		return chunk, ErrInvalidTransition
	}
	return chunk, err
}

func (s *PostgresStore) ClaimPreparedDeckBatchSubmission(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string, leaseExpiresAt time.Time) (domain.PreparedDeckBatchChunk, error) {
	if !validClaim(token, leaseExpiresAt) {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	chunk, err := scanPreparedDeckBatchChunk(s.pool.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks c SET state='submitting',submission_claim_token=$6,submission_claimed_at=now(),submission_lease_expires_at=$7,updated_at=now() FROM deck_preparation_runs r WHERE c.owner_id=$1 AND c.preparation_id=$2 AND c.run_id=$3 AND c.id=$4 AND c.submission_generation=$5 AND c.state IN ('pending','submitting') AND (c.submission_claim_token IS NULL OR c.submission_lease_expires_at<=now()) AND r.owner_id=c.owner_id AND r.preparation_id=c.preparation_id AND r.id=c.run_id AND r.state='translating' RETURNING `+qualifiedColumns("c", preparedDeckBatchChunkColumns), owner, preparationID, runID, chunkID, generation, token, leaseExpiresAt))
	if err != nil && errors.Is(err, ErrNotFound) {
		return chunk, ErrInvalidTransition
	}
	return chunk, err
}

func (s *PostgresStore) RecordPreparedDeckBatchSubmitted(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token, inputFileID, batchID string, submittedAt time.Time) (domain.PreparedDeckBatchChunk, error) {
	if strings.TrimSpace(inputFileID) == "" || strings.TrimSpace(batchID) == "" {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PreparedDeckBatchChunk{}, err
	}
	defer tx.Rollback(ctx)
	chunk, err := s.RecordPreparedDeckBatchSubmittedTx(ctx, tx, owner, preparationID, runID, chunkID, generation, token, inputFileID, batchID, submittedAt, nil)
	if err != nil {
		return chunk, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PreparedDeckBatchChunk{}, err
	}
	return chunk, nil
}

func (s *PostgresStore) AssignPreparedDeckBatchReconciliationJob(ctx context.Context, owner, preparationID, runID, chunkID string, expectedGeneration int, jobID int64) (domain.PreparedDeckBatchChunk, error) {
	if jobID < 1 {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	chunk, err := scanPreparedDeckBatchChunk(s.pool.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks c SET reconciliation_generation=reconciliation_generation+1,reconciliation_job_id=$6,updated_at=now() FROM deck_preparation_runs r WHERE c.owner_id=$1 AND c.preparation_id=$2 AND c.run_id=$3 AND c.id=$4 AND c.reconciliation_generation=$5 AND c.state IN ('submitted','polling') AND r.owner_id=c.owner_id AND r.preparation_id=c.preparation_id AND r.id=c.run_id AND r.state='translating' RETURNING `+qualifiedColumns("c", preparedDeckBatchChunkColumns), owner, preparationID, runID, chunkID, expectedGeneration, jobID))
	if err != nil && errors.Is(err, ErrNotFound) {
		return chunk, ErrInvalidTransition
	}
	return chunk, err
}

func (s *PostgresStore) ClaimPreparedDeckBatchReconciliation(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string, leaseExpiresAt time.Time) (domain.PreparedDeckBatchChunk, error) {
	if !validClaim(token, leaseExpiresAt) {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	chunk, err := scanPreparedDeckBatchChunk(s.pool.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks c SET state='reconciling',reconciliation_claim_token=$6,reconciliation_claimed_at=now(),reconciliation_lease_expires_at=$7,updated_at=now() FROM deck_preparation_runs r WHERE c.owner_id=$1 AND c.preparation_id=$2 AND c.run_id=$3 AND c.id=$4 AND c.reconciliation_generation=$5 AND c.state IN ('submitted','polling','reconciling') AND (c.reconciliation_claim_token IS NULL OR c.reconciliation_lease_expires_at<=now()) AND r.owner_id=c.owner_id AND r.preparation_id=c.preparation_id AND r.id=c.run_id AND r.state='translating' RETURNING `+qualifiedColumns("c", preparedDeckBatchChunkColumns), owner, preparationID, runID, chunkID, generation, token, leaseExpiresAt))
	if err != nil && errors.Is(err, ErrNotFound) {
		return chunk, ErrInvalidTransition
	}
	return chunk, err
}

type PreparedDeckBatchReconciliationUpdate struct {
	State               domain.PreparedDeckBatchChunkState
	ProviderStatus      string
	OutputFileID        string
	ErrorFileID         string
	CompletedCount      int
	FailedCount         int
	ExpiredCount        int
	InputTokens         int64
	OutputTokens        int64
	ErrorClass          string
	ErrorCode           string
	ProviderCompletedAt *time.Time
}

// CompletePreparedDeckBatchCacheHits closes exact cache hits while a chunk is
// fenced for submission. It never broadens identity or changes a running or
// terminal outcome.
func (s *PostgresStore) CompletePreparedDeckBatchCacheHits(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var count int
	err = tx.QueryRow(ctx, `WITH hit AS (
		UPDATE deck_preparation_translation_outcomes o SET state='completed',terminal_at=now(),cache_hit_count=cache_hit_count+1,updated_at=now(),claim_token=NULL,claimed_at=NULL,lease_expires_at=NULL
		FROM deck_preparation_batch_chunk_items ci
		JOIN deck_preparation_batch_chunks c ON c.owner_id=ci.owner_id AND c.preparation_id=ci.preparation_id AND c.run_id=ci.run_id AND c.id=ci.chunk_id AND c.generation=ci.generation
		JOIN deck_preparation_manifest_items mi ON mi.owner_id=ci.owner_id AND mi.preparation_id=ci.preparation_id AND mi.run_id=ci.run_id AND mi.ordinal=ci.ordinal
		JOIN enrichment_cache ec ON ec.language=mi.language AND ec.canonical_lemma=mi.canonical_lemma AND ec.upos=mi.upos AND ec.provider=mi.provider AND ec.provider_version=mi.provider_version AND ec.sentence_hash=COALESCE(mi.sentence_hash,'')
		WHERE o.owner_id=$1 AND o.preparation_id=$2 AND o.run_id=$3 AND o.ordinal=ci.ordinal AND o.state='pending' AND c.id=$4 AND c.generation=$5 AND c.state='submitting' AND c.submission_claim_token=$6
		RETURNING o.ordinal
	) SELECT count(*) FROM hit`, owner, preparationID, runID, chunkID, generation, token).Scan(&count)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

// FinishPreparedDeckBatchSubmission records a pre-network terminal outcome
// (for example, a chunk whose remaining items became cache hits) behind the
// submission fence.
func (s *PostgresStore) FinishPreparedDeckBatchSubmission(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string, state domain.PreparedDeckBatchChunkState, errorClass, errorCode string) (domain.PreparedDeckBatchChunk, error) {
	if state != domain.PreparedDeckBatchCompleted && state != domain.PreparedDeckBatchFailed && state != domain.PreparedDeckBatchAmbiguous {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	if err := validateBoundedError(errorClass, errorCode); err != nil {
		return domain.PreparedDeckBatchChunk{}, err
	}
	chunk, err := scanPreparedDeckBatchChunk(s.pool.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks c SET state=$7,error_class=$8,error_code=$9,completed_count=CASE WHEN $7='completed' THEN request_count ELSE completed_count END,submission_claim_token=NULL,submission_claimed_at=NULL,submission_lease_expires_at=NULL,updated_at=now() FROM deck_preparation_runs r WHERE c.owner_id=$1 AND c.preparation_id=$2 AND c.run_id=$3 AND c.id=$4 AND c.generation=$5 AND c.submission_claim_token=$6 AND c.state='submitting' AND r.owner_id=c.owner_id AND r.preparation_id=c.preparation_id AND r.id=c.run_id AND r.state='translating' RETURNING `+qualifiedColumns("c", preparedDeckBatchChunkColumns), owner, preparationID, runID, chunkID, generation, token, state, errorClass, errorCode))
	if err != nil && errors.Is(err, ErrNotFound) {
		return chunk, ErrPreparedDeckClaimLost
	}
	return chunk, err
}

// RetryPreparedDeckBatchSubmission releases a pre-creation provider failure
// back to pending. It must never be used after CreateBatch may have reached the
// provider, because that boundary requires ambiguous-submission recovery.
func (s *PostgresStore) RetryPreparedDeckBatchSubmission(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token, errorClass, errorCode string) (domain.PreparedDeckBatchChunk, error) {
	if err := validateBoundedError(errorClass, errorCode); err != nil {
		return domain.PreparedDeckBatchChunk{}, err
	}
	chunk, err := scanPreparedDeckBatchChunk(s.pool.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks c SET state='pending',error_class=$7,error_code=$8,submission_claim_token=NULL,submission_claimed_at=NULL,submission_lease_expires_at=NULL,updated_at=now() FROM deck_preparation_runs r WHERE c.owner_id=$1 AND c.preparation_id=$2 AND c.run_id=$3 AND c.id=$4 AND c.generation=$5 AND c.submission_claim_token=$6 AND c.state='submitting' AND c.input_file_id IS NULL AND c.batch_id IS NULL AND r.owner_id=c.owner_id AND r.preparation_id=c.preparation_id AND r.id=c.run_id AND r.state='translating' RETURNING `+qualifiedColumns("c", preparedDeckBatchChunkColumns), owner, preparationID, runID, chunkID, generation, token, errorClass, errorCode))
	if err != nil && errors.Is(err, ErrNotFound) {
		return chunk, ErrPreparedDeckClaimLost
	}
	return chunk, err
}

// VerifyPreparedDeckBatchSubmissionClaim closes the read/prepare gap before
// the external boundary. Cancellation or supersession that wins first makes
// the provider call ineligible.
func (s *PostgresStore) VerifyPreparedDeckBatchSubmissionClaim(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string) error {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM deck_preparation_batch_chunks c
		JOIN deck_preparation_runs r ON r.owner_id=c.owner_id AND r.preparation_id=c.preparation_id AND r.id=c.run_id
		WHERE c.owner_id=$1 AND c.preparation_id=$2 AND c.run_id=$3 AND c.id=$4 AND c.generation=$5 AND c.submission_generation=$5 AND c.submission_claim_token=$6 AND c.state='submitting' AND r.state='translating'
	)`, owner, preparationID, runID, chunkID, generation, token).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrPreparedDeckClaimLost
	}
	return nil
}

func (s *PostgresStore) FinishPreparedDeckBatchReconciliation(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string, update PreparedDeckBatchReconciliationUpdate) (domain.PreparedDeckBatchChunk, error) {
	if update.State != domain.PreparedDeckBatchCompleted && update.State != domain.PreparedDeckBatchFailed && update.State != domain.PreparedDeckBatchPolling {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	if update.CompletedCount < 0 || update.FailedCount < 0 || update.ExpiredCount < 0 || update.InputTokens < 0 || update.OutputTokens < 0 {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	if err := validateBoundedError(update.ErrorClass, update.ErrorCode); err != nil {
		return domain.PreparedDeckBatchChunk{}, err
	}
	chunk, err := scanPreparedDeckBatchChunk(s.pool.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks c SET state=$7,provider_status=NULLIF($8,''),output_file_id=NULLIF($9,''),error_file_id=NULLIF($10,''),completed_count=$11,failed_count=$12,expired_count=$13,input_tokens=$14,output_tokens=$15,total_tokens=$14::bigint+$15::bigint,error_class=$16,error_code=$17,provider_completed_at=$18,last_polled_at=now(),reconciled_at=CASE WHEN $7 IN ('completed','failed') THEN now() ELSE reconciled_at END,reconciliation_claim_token=NULL,reconciliation_claimed_at=NULL,reconciliation_lease_expires_at=NULL,updated_at=now()
		FROM deck_preparation_runs r WHERE c.owner_id=$1 AND c.preparation_id=$2 AND c.run_id=$3 AND c.id=$4 AND c.reconciliation_generation=$5 AND c.reconciliation_claim_token=$6 AND c.state='reconciling' AND r.owner_id=c.owner_id AND r.preparation_id=c.preparation_id AND r.id=c.run_id AND r.state='translating' RETURNING `+qualifiedColumns("c", preparedDeckBatchChunkColumns),
		owner, preparationID, runID, chunkID, generation, token, update.State, update.ProviderStatus, update.OutputFileID, update.ErrorFileID, update.CompletedCount, update.FailedCount, update.ExpiredCount, update.InputTokens, update.OutputTokens, update.ErrorClass, update.ErrorCode, update.ProviderCompletedAt))
	if err != nil && errors.Is(err, ErrNotFound) {
		return chunk, ErrInvalidTransition
	}
	return chunk, err
}

// ListPreparedDeckRecoveryWork is a privacy-safe restart projection. It
// reports only opaque identities and generations; later workflow issues decide
// which River job kind to enqueue.
func (s *PostgresStore) ListPreparedDeckRecoveryWork(ctx context.Context, limit int) ([]domain.PreparedDeckRecoveryWork, error) {
	if limit < 1 {
		return nil, ErrInvalidTransition
	}
	rows, err := s.pool.Query(ctx, `
		SELECT o.owner_id::text,o.preparation_id::text,o.run_id::text,''::text,o.ordinal,o.dispatch_generation,'outcome',o.state='running'
		FROM deck_preparation_translation_outcomes o JOIN deck_preparation_runs r ON r.owner_id=o.owner_id AND r.preparation_id=o.preparation_id AND r.id=o.run_id
		WHERE r.state='translating' AND ((o.state='pending' AND o.next_attempt_at<=now()) OR (o.state='running' AND o.lease_expires_at<=now()))
		UNION ALL
		SELECT c.owner_id::text,c.preparation_id::text,c.run_id::text,c.id::text,-1,c.submission_generation,'batch_submission',c.state='submitting'
		FROM deck_preparation_batch_chunks c JOIN deck_preparation_runs r ON r.owner_id=c.owner_id AND r.preparation_id=c.preparation_id AND r.id=c.run_id
		WHERE r.state='translating' AND (c.state='pending' OR (c.state='submitting' AND c.submission_lease_expires_at<=now()))
		UNION ALL
		SELECT c.owner_id::text,c.preparation_id::text,c.run_id::text,c.id::text,-1,c.reconciliation_generation,'batch_reconciliation',c.state='reconciling'
		FROM deck_preparation_batch_chunks c JOIN deck_preparation_runs r ON r.owner_id=c.owner_id AND r.preparation_id=c.preparation_id AND r.id=c.run_id
		WHERE r.state='translating' AND (c.state IN ('submitted','polling') OR (c.state='reconciling' AND c.reconciliation_lease_expires_at<=now()))
		UNION ALL
		SELECT owner_id::text,preparation_id::text,id::text,''::text,-1,finalization_dispatch_generation,'finalizer',finalization_claim_token IS NOT NULL AND finalization_lease_expires_at<=now()
		FROM deck_preparation_runs WHERE state='finalizing' AND translation_state='completed' AND (finalization_claim_token IS NULL OR finalization_lease_expires_at<=now())
		UNION ALL
		SELECT owner_id::text,preparation_id::text,run_id::text,id::text,-1,generation,'batch_cleanup',cleanup_claim_token IS NOT NULL AND cleanup_lease_expires_at<=now()
		FROM deck_preparation_batch_chunks
		WHERE state IN ('completed','cancelled') AND (cleanup_claim_token IS NULL OR cleanup_lease_expires_at<=now()) AND
		      ((input_file_id IS NOT NULL AND input_file_cleanup_state IN ('pending','failed') AND input_file_cleanup_attempts < 3) OR
		       (output_file_id IS NOT NULL AND output_file_cleanup_state IN ('pending','failed') AND output_file_cleanup_attempts < 3) OR
		       (error_file_id IS NOT NULL AND error_file_cleanup_state IN ('pending','failed') AND error_file_cleanup_attempts < 3))
		UNION ALL
		SELECT owner_id::text,preparation_id::text,id::text,''::text,-1,finalization_dispatch_generation,'translation_completion',false
		FROM deck_preparation_runs r WHERE state='translating' AND NOT EXISTS (SELECT 1 FROM deck_preparation_translation_outcomes o WHERE o.run_id=r.id AND o.state IN ('pending','running'))
		ORDER BY 7,3,5 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var work []domain.PreparedDeckRecoveryWork
	for rows.Next() {
		var item domain.PreparedDeckRecoveryWork
		if err = rows.Scan(&item.OwnerID, &item.PreparationID, &item.RunID, &item.ChunkID, &item.Ordinal, &item.Generation, &item.Kind, &item.LeaseExpired); err != nil {
			return nil, err
		}
		work = append(work, item)
	}
	return work, rows.Err()
}

func validateBoundedError(errorClass, errorCode string) error {
	if len(errorCode) > 80 || strings.TrimSpace(errorClass) != errorClass || strings.TrimSpace(errorCode) != errorCode {
		return fmt.Errorf("%w: invalid bounded error", ErrInvalidTransition)
	}
	return nil
}
