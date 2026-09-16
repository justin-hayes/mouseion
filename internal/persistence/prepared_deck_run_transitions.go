package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
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

func outcomeTransitionErrorTx(ctx context.Context, q *sqlcgen.Queries, owner, preparationID, runID string, ordinal int, err error) error {
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	_, getErr := q.GetPreparedDeckTranslationOutcome(ctx, sqlcgen.GetPreparedDeckTranslationOutcomeParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, Ordinal: ordinal,
	})
	if getErr != nil {
		return missing(getErr)
	}
	return ErrPreparedDeckClaimLost
}

// ClaimPreparedDeckTranslationOutcome grants one generation a leased item
// claim. The token is required by every subsequent mutation.
func (s *PostgresStore) ClaimPreparedDeckTranslationOutcome(ctx context.Context, owner, preparationID, runID string, ordinal, generation int, token string, leaseExpiresAt time.Time) (domain.PreparedDeckTranslationOutcome, error) {
	if !validClaim(token, leaseExpiresAt) {
		return domain.PreparedDeckTranslationOutcome{}, ErrInvalidTransition
	}
	var outcome domain.PreparedDeckTranslationOutcome
	err := withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		model, err := q.ClaimPreparedDeckTranslationOutcome(ctx, sqlcgen.ClaimPreparedDeckTranslationOutcomeParams{
			OwnerID:            owner,
			PreparationID:      preparationID,
			RunID:              runID,
			Ordinal:            ordinal,
			DispatchGeneration: generation,
			ClaimToken:         uuidArg(token),
			LeaseExpiresAt:     pgtype.Timestamptz{Time: leaseExpiresAt, Valid: true},
		})
		if err != nil {
			return outcomeTransitionErrorTx(ctx, q, owner, preparationID, runID, ordinal, missing(err))
		}
		outcome = preparedDeckOutcomeFromModel(model)
		if err = q.StartPreparedDeckTranslation(ctx, sqlcgen.StartPreparedDeckTranslationParams{
			OwnerID: owner, PreparationID: preparationID, ID: runID,
		}); err != nil {
			return err
		}
		return nil
	})
	return outcome, err
}

// VerifyPreparedDeckTranslationClaim fences side effects that happen after a
// provider call but before the terminal outcome transition.
func (s *PostgresStore) VerifyPreparedDeckTranslationClaim(ctx context.Context, owner, preparationID, runID string, ordinal, generation int, token string) error {
	var valid bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deck_preparation_translation_outcomes WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND ordinal=$4 AND dispatch_generation=$5 AND state='running' AND claim_token=$6 AND lease_expires_at > now())`, owner, preparationID, runID, ordinal, generation, uuidArg(token)).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrPreparedDeckClaimLost
	}
	return nil
}

// RetryPreparedDeckTranslationOutcome persists one failed provider attempt and
// releases the item claim for a later, fenced dispatch.
func (s *PostgresStore) RetryPreparedDeckTranslationOutcome(ctx context.Context, owner, preparationID, runID string, ordinal, generation int, token string, nextAttemptAt time.Time, errorClass, errorCode string) (domain.PreparedDeckTranslationOutcome, error) {
	if err := validateBoundedError(errorClass, errorCode); err != nil {
		return domain.PreparedDeckTranslationOutcome{}, err
	}
	var outcome domain.PreparedDeckTranslationOutcome
	err := withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		model, err := q.RetryPreparedDeckTranslationOutcome(ctx, sqlcgen.RetryPreparedDeckTranslationOutcomeParams{
			OwnerID:            owner,
			PreparationID:      preparationID,
			RunID:              runID,
			Ordinal:            ordinal,
			DispatchGeneration: generation,
			ClaimToken:         uuidArg(token),
			NextAttemptAt:      nextAttemptAt,
			ErrorClass:         errorClass,
			ErrorCode:          errorCode,
		})
		if err != nil {
			return outcomeTransitionErrorTx(ctx, q, owner, preparationID, runID, ordinal, missing(err))
		}
		outcome = preparedDeckOutcomeFromModel(model)
		return nil
	})
	return outcome, err
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
	var outcome domain.PreparedDeckTranslationOutcome
	var run domain.PreparedDeckRun
	err := withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		runModel, err := q.GetPreparedDeckRunForUpdate(ctx, sqlcgen.GetPreparedDeckRunForUpdateParams{
			OwnerID: owner, PreparationID: preparationID, ID: runID,
		})
		run = preparedDeckRunFromModel(runModel)
		if err != nil {
			return missing(err)
		}
		if run.State != domain.PreparedDeckRunTranslating || (run.TranslationState != domain.PreparedDeckTranslationPending && run.TranslationState != domain.PreparedDeckTranslationRunning) {
			existing, getErr := getPreparedDeckTranslationOutcomeTx(ctx, q, owner, preparationID, runID, ordinal)
			if getErr == nil && existing.State == update.State {
				outcome = existing
				return nil
			}
			if getErr != nil && !errors.Is(getErr, ErrNotFound) {
				return getErr
			}
			return ErrInvalidTransition
		}
		cacheLatencyMS, providerLatencyMS := update.CacheLatency.Milliseconds(), update.ProviderLatency.Milliseconds()
		if cacheLatencyMS < 0 || providerLatencyMS < 0 {
			return ErrInvalidTransition
		}
		model, err := q.FinishPreparedDeckTranslationOutcome(ctx, sqlcgen.FinishPreparedDeckTranslationOutcomeParams{
			OwnerID: owner, PreparationID: preparationID, RunID: runID, Ordinal: ordinal, DispatchGeneration: generation, ClaimToken: uuidArg(token),
			State: string(update.State), ProviderAttemptCount: boolInt(update.ProviderAttempt), ErrorClass: update.ErrorClass, ErrorCode: update.ErrorCode,
			CacheHitCount: boolInt(update.CacheHit), ProviderCallCount: boolInt(update.ProviderCall), CacheLatencyMs: cacheLatencyMS, ProviderLatencyMs: providerLatencyMS,
		})
		if err != nil {
			if errors.Is(missing(err), ErrNotFound) {
				existing, getErr := getPreparedDeckTranslationOutcomeTx(ctx, q, owner, preparationID, runID, ordinal)
				if getErr == nil && existing.State == update.State {
					outcome = existing
					return nil
				}
				if getErr != nil && !errors.Is(getErr, ErrNotFound) {
					return getErr
				}
				return ErrPreparedDeckClaimLost
			}
			return err
		}
		outcome = preparedDeckOutcomeFromModel(model)

		counts, err := q.CountPreparedDeckRunOutcomeStates(ctx, sqlcgen.CountPreparedDeckRunOutcomeStatesParams{
			OwnerID: owner, PreparationID: preparationID, RunID: runID,
		})
		if err != nil {
			return err
		}
		run.CompletedCount, run.FailedCount = int(counts.CompletedCount), int(counts.FailedCount)
		if counts.NonterminalCount == 0 {
			if run.ExecutionMode == domain.PreparedDeckExecutionStandard && run.ExternalTranslationConsent && run.ExternalTranslationConfigured && run.FailedCount > 0 {
				if runModel, err = q.FailPreparedDeckRunWithCounts(ctx, sqlcgen.FailPreparedDeckRunWithCountsParams{
					OwnerID: owner, PreparationID: preparationID, ID: runID, CompletedCount: run.CompletedCount, FailedCount: run.FailedCount, ErrorClass: update.ErrorClass, ErrorCode: update.ErrorCode,
				}); err != nil {
					return err
				}
				run = preparedDeckRunFromModel(runModel)
				if _, err = q.FailDeckPreparationTranslation(ctx, sqlcgen.FailDeckPreparationTranslationParams{OwnerID: owner, ID: preparationID, CurrentRunID: uuidArg(runID), Error: "prepared-deck translation was incomplete"}); err != nil {
					return err
				}
			} else {
				runModel, err = q.FinalizePreparedDeckRun(ctx, sqlcgen.FinalizePreparedDeckRunParams{OwnerID: owner, PreparationID: preparationID, ID: runID, CompletedCount: run.CompletedCount, FailedCount: run.FailedCount})
				if err != nil {
					return err
				}
				run = preparedDeckRunFromModel(runModel)
				if insertFinalizer != nil {
					if err = insertFinalizer(ctx, tx, run); err != nil {
						return err
					}
				}
			}
		} else {
			runModel, err = q.UpdatePreparedDeckRunTranslationRunning(ctx, sqlcgen.UpdatePreparedDeckRunTranslationRunningParams{OwnerID: owner, PreparationID: preparationID, ID: runID, CompletedCount: run.CompletedCount, FailedCount: run.FailedCount})
			if err != nil {
				return err
			}
			run = preparedDeckRunFromModel(runModel)
		}
		return nil
	})
	return outcome, run, err
}

func getPreparedDeckTranslationOutcomeTx(ctx context.Context, q *sqlcgen.Queries, owner, preparationID, runID string, ordinal int) (domain.PreparedDeckTranslationOutcome, error) {
	model, err := q.GetPreparedDeckTranslationOutcome(ctx, sqlcgen.GetPreparedDeckTranslationOutcomeParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, Ordinal: ordinal,
	})
	if err != nil {
		return domain.PreparedDeckTranslationOutcome{}, missing(err)
	}
	return preparedDeckOutcomeFromModel(model), nil
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
	model, err := s.queries().RedispatchPreparedDeckTranslationOutcome(ctx, sqlcgen.RedispatchPreparedDeckTranslationOutcomeParams{
		OwnerID:            owner,
		PreparationID:      preparationID,
		RunID:              runID,
		Ordinal:            ordinal,
		DispatchGeneration: expectedGeneration,
	})
	if err != nil {
		return domain.PreparedDeckTranslationOutcome{}, s.outcomeTransitionError(ctx, owner, preparationID, runID, ordinal, missing(err))
	}
	return preparedDeckOutcomeFromModel(model), nil
}

// ClaimPreparedDeckFinalization grants a leased, generation-fenced finalizer
// claim only for the current owner-scoped run.
func (s *PostgresStore) ClaimPreparedDeckFinalization(ctx context.Context, owner, preparationID, runID string, generation int, token string, leaseExpiresAt time.Time) (domain.PreparedDeckRun, error) {
	if !validClaim(token, leaseExpiresAt) {
		return domain.PreparedDeckRun{}, ErrInvalidTransition
	}
	row, err := s.queries().ClaimPreparedDeckFinalization(ctx, sqlcgen.ClaimPreparedDeckFinalizationParams{
		OwnerID:                        owner,
		PreparationID:                  preparationID,
		ID:                             runID,
		FinalizationDispatchGeneration: generation,
		FinalizationClaimToken:         uuidArg(token),
		FinalizationLeaseExpiresAt:     pgtype.Timestamptz{Time: leaseExpiresAt, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			existing, getErr := s.GetPreparedDeckRun(ctx, owner, preparationID, runID)
			if getErr != nil {
				return domain.PreparedDeckRun{}, getErr
			}
			if existing.State == domain.PreparedDeckRunCompleted {
				return existing, nil
			}
			return domain.PreparedDeckRun{}, ErrPreparedDeckClaimLost
		}
		return domain.PreparedDeckRun{}, err
	}
	return preparedDeckRunFromFields(row.ID, row.OwnerID, row.PreparationID, row.RunNumber, row.State, row.TranslationState, row.ExecutionMode, row.TargetLanguage, row.ExternalTranslationConsent, row.ExternalTranslationConfigured, row.ContextMode, row.Provider, row.ProviderVersion, row.Endpoint, row.Model, row.ManifestSchemaVersion, row.RenderInputVersion, row.PresentationVersion, row.RetryPolicyVersion, row.MaxProviderAttempts, row.MaxBatchGenerations, row.BatchMaxRequests, row.BatchMaxBytes, row.CandidateCount, row.CompletedCount, row.FailedCount, row.FinalizationDispatchGeneration, row.FinalizationDispatchCount, row.FinalizationJobID, row.FinalizationClaimToken, row.ErrorClass, row.ErrorCode, row.CreatedAt, row.UpdatedAt, row.FinalizationClaimedAt, row.FinalizationLeaseExpiresAt, row.TranslationCompletedAt, row.CompletedAt), nil
}

func (s *PostgresStore) AssignPreparedDeckFinalizationJob(ctx context.Context, owner, preparationID, runID string, expectedGeneration int, jobID int64) (domain.PreparedDeckRun, error) {
	if jobID < 1 {
		return domain.PreparedDeckRun{}, ErrInvalidTransition
	}
	row, err := s.queries().AssignPreparedDeckFinalizationJob(ctx, sqlcgen.AssignPreparedDeckFinalizationJobParams{
		OwnerID:                        owner,
		PreparationID:                  preparationID,
		ID:                             runID,
		FinalizationDispatchGeneration: expectedGeneration,
		FinalizationJobID:              pgtype.Int8{Int64: jobID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PreparedDeckRun{}, ErrInvalidTransition
		}
		return domain.PreparedDeckRun{}, err
	}
	return preparedDeckRunFromFields(row.ID, row.OwnerID, row.PreparationID, row.RunNumber, row.State, row.TranslationState, row.ExecutionMode, row.TargetLanguage, row.ExternalTranslationConsent, row.ExternalTranslationConfigured, row.ContextMode, row.Provider, row.ProviderVersion, row.Endpoint, row.Model, row.ManifestSchemaVersion, row.RenderInputVersion, row.PresentationVersion, row.RetryPolicyVersion, row.MaxProviderAttempts, row.MaxBatchGenerations, row.BatchMaxRequests, row.BatchMaxBytes, row.CandidateCount, row.CompletedCount, row.FailedCount, row.FinalizationDispatchGeneration, row.FinalizationDispatchCount, row.FinalizationJobID, row.FinalizationClaimToken, row.ErrorClass, row.ErrorCode, row.CreatedAt, row.UpdatedAt, row.FinalizationClaimedAt, row.FinalizationLeaseExpiresAt, row.TranslationCompletedAt, row.CompletedAt), nil
}

func (s *PostgresStore) AssignPreparedDeckBatchSubmissionJob(ctx context.Context, owner, preparationID, runID, chunkID string, expectedGeneration int, jobID int64) (domain.PreparedDeckBatchChunk, error) {
	if jobID < 1 {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	model, err := s.queries().AssignPreparedDeckBatchSubmissionJob(ctx, sqlcgen.AssignPreparedDeckBatchSubmissionJobParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, SubmissionGeneration: expectedGeneration, SubmissionJobID: pgtype.Int8{Int64: jobID, Valid: true},
	})
	chunk := preparedDeckBatchChunkFromModel(model)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return chunk, ErrInvalidTransition
	}
	return chunk, err
}

func (s *PostgresStore) ClaimPreparedDeckBatchSubmission(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string, leaseExpiresAt time.Time) (domain.PreparedDeckBatchChunk, error) {
	if !validClaim(token, leaseExpiresAt) {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	model, err := s.queries().ClaimPreparedDeckBatchSubmission(ctx, sqlcgen.ClaimPreparedDeckBatchSubmissionParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, SubmissionGeneration: generation, SubmissionClaimToken: uuidArg(token), SubmissionLeaseExpiresAt: pgtype.Timestamptz{Time: leaseExpiresAt, Valid: true},
	})
	chunk := preparedDeckBatchChunkFromModel(model)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
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
	model, err := s.queries().AssignPreparedDeckBatchReconciliationJob(ctx, sqlcgen.AssignPreparedDeckBatchReconciliationJobParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, ReconciliationGeneration: expectedGeneration, ReconciliationJobID: pgtype.Int8{Int64: jobID, Valid: true},
	})
	chunk := preparedDeckBatchChunkFromModel(model)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return chunk, ErrInvalidTransition
	}
	return chunk, err
}

func (s *PostgresStore) ClaimPreparedDeckBatchReconciliation(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string, leaseExpiresAt time.Time) (domain.PreparedDeckBatchChunk, error) {
	if !validClaim(token, leaseExpiresAt) {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	model, err := s.queries().ClaimPreparedDeckBatchReconciliation(ctx, sqlcgen.ClaimPreparedDeckBatchReconciliationParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, ReconciliationGeneration: generation, ReconciliationClaimToken: uuidArg(token), ReconciliationLeaseExpiresAt: pgtype.Timestamptz{Time: leaseExpiresAt, Valid: true},
	})
	chunk := preparedDeckBatchChunkFromModel(model)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
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
	count64, err := sqlcgen.New(tx).CompletePreparedDeckBatchCacheHits(ctx, sqlcgen.CompletePreparedDeckBatchCacheHitsParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, Generation: generation, SubmissionClaimToken: uuidArg(token),
	})
	count = int(count64)
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
	model, err := s.queries().FinishPreparedDeckBatchSubmission(ctx, sqlcgen.FinishPreparedDeckBatchSubmissionParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, Generation: generation, SubmissionClaimToken: uuidArg(token), State: string(state), ErrorClass: errorClass, ErrorCode: errorCode,
	})
	chunk := preparedDeckBatchChunkFromModel(model)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
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
	model, err := s.queries().RetryPreparedDeckBatchSubmission(ctx, sqlcgen.RetryPreparedDeckBatchSubmissionParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, Generation: generation, SubmissionClaimToken: uuidArg(token), ErrorClass: errorClass, ErrorCode: errorCode,
	})
	chunk := preparedDeckBatchChunkFromModel(model)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return chunk, ErrPreparedDeckClaimLost
	}
	return chunk, err
}

// VerifyPreparedDeckBatchSubmissionClaim closes the read/prepare gap before
// the external boundary. Cancellation or supersession that wins first makes
// the provider call ineligible.
func (s *PostgresStore) VerifyPreparedDeckBatchSubmissionClaim(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string) error {
	exists, err := s.queries().VerifyPreparedDeckBatchSubmissionClaim(ctx, sqlcgen.VerifyPreparedDeckBatchSubmissionClaimParams{
		OwnerID:              owner,
		PreparationID:        preparationID,
		RunID:                runID,
		ID:                   chunkID,
		Generation:           generation,
		SubmissionClaimToken: uuidArg(token),
	})
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
	model, err := s.queries().FinishPreparedDeckBatchReconciliation(ctx, sqlcgen.FinishPreparedDeckBatchReconciliationParams{
		Owner: owner, Preparation: preparationID, Run: runID, ID: chunkID, Generation: generation, Token: uuidArg(token), State: string(update.State), ProviderStatus: update.ProviderStatus, OutputFileID: update.OutputFileID, ErrorFileID: update.ErrorFileID, CompletedCount: update.CompletedCount, FailedCount: update.FailedCount, ExpiredCount: update.ExpiredCount, InputTokens: update.InputTokens, OutputTokens: update.OutputTokens, ErrorClass: update.ErrorClass, ErrorCode: update.ErrorCode, ProviderCompletedAt: pgTimeArgPtr(update.ProviderCompletedAt),
	})
	chunk := preparedDeckBatchChunkFromModel(model)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
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
	rows, err := s.queries().ListPreparedDeckRecoveryWork(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	var work []domain.PreparedDeckRecoveryWork
	for _, row := range rows {
		work = append(work, domain.PreparedDeckRecoveryWork{
			OwnerID: row.OOwnerID, PreparationID: row.OPreparationID, RunID: row.ORunID, ChunkID: row.ChunkID,
			Ordinal: int(row.Ordinal), Generation: int(row.Generation), Kind: row.Kind, LeaseExpired: row.LeaseExpired,
		})
	}
	return work, nil
}

func validateBoundedError(errorClass, errorCode string) error {
	if len(errorCode) > 80 || strings.TrimSpace(errorClass) != errorClass || strings.TrimSpace(errorCode) != errorCode {
		return fmt.Errorf("%w: invalid bounded error", ErrInvalidTransition)
	}
	return nil
}
