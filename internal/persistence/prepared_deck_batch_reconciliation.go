package persistence

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

type PreparedDeckBatchItemReconciliation struct {
	Ordinal    int
	State      domain.PreparedDeckOutcomeState
	CacheEntry *enrichment.CacheEntry
	ErrorClass string
	ErrorCode  string
}

type PreparedDeckBatchReconcileParams struct {
	OwnerID, PreparationID, RunID, ChunkID string
	ReconciliationGeneration               int
	ClaimToken                             string
	Chunk                                  PreparedDeckBatchReconciliationUpdate
	Items                                  []PreparedDeckBatchItemReconciliation
	RetryChunks                            []PreparedDeckBatchChunkPlan
}

type PreparedDeckBatchReconcileResult struct {
	Chunk       domain.PreparedDeckBatchChunk
	Run         domain.PreparedDeckRun
	RetryChunks []domain.PreparedDeckBatchChunk
}

// PreparedDeckBatchSubmissionJobInserter inserts one retry-generation job in
// the same transaction that persists the retry selection.
type PreparedDeckBatchSubmissionJobInserter func(context.Context, pgx.Tx, domain.PreparedDeckBatchChunk) (int64, error)

type preparedDeckBatchMember struct {
	Ordinal int
	Key     enrichment.CacheKey
}

// ReconcilePreparedDeckBatch atomically applies one fully validated terminal
// provider result. Exact cache rows, fenced item outcomes, retry chunks, the
// terminal chunk, and a possible finalizer job either all commit or all roll
// back together.
func (s *PostgresStore) ReconcilePreparedDeckBatch(ctx context.Context, params PreparedDeckBatchReconcileParams, insertSubmission PreparedDeckBatchSubmissionJobInserter, insertFinalizer PreparedDeckRunJobInserter) (PreparedDeckBatchReconcileResult, error) {
	if s == nil || strings.TrimSpace(params.OwnerID) == "" || strings.TrimSpace(params.PreparationID) == "" || strings.TrimSpace(params.RunID) == "" || strings.TrimSpace(params.ChunkID) == "" || strings.TrimSpace(params.ClaimToken) == "" {
		return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
	}
	if params.Chunk.State != domain.PreparedDeckBatchCompleted || !terminalProviderStatus(params.Chunk.ProviderStatus) {
		return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
	}
	if params.Chunk.CompletedCount < 0 || params.Chunk.FailedCount < 0 || params.Chunk.ExpiredCount < 0 || params.Chunk.InputTokens < 0 || params.Chunk.OutputTokens < 0 {
		return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
	}
	if err := validateBoundedError(params.Chunk.ErrorClass, params.Chunk.ErrorCode); err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	defer tx.Rollback(ctx)
	run, err := scanPreparedDeckRun(tx.QueryRow(ctx, `SELECT `+preparedDeckRunColumns+` FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 FOR UPDATE`, params.OwnerID, params.PreparationID, params.RunID))
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	if run.State != domain.PreparedDeckRunTranslating || (run.TranslationState != domain.PreparedDeckTranslationPending && run.TranslationState != domain.PreparedDeckTranslationRunning) {
		return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckClaimLost
	}
	chunk, err := scanPreparedDeckBatchChunk(tx.QueryRow(ctx, `SELECT `+preparedDeckBatchChunkColumns+` FROM deck_preparation_batch_chunks WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4 FOR UPDATE`, params.OwnerID, params.PreparationID, params.RunID, params.ChunkID))
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	if chunk.State != domain.PreparedDeckBatchReconciling || chunk.ReconciliationGeneration != params.ReconciliationGeneration || chunk.ReconciliationClaimToken != params.ClaimToken {
		return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckClaimLost
	}

	members, err := loadPreparedDeckBatchMembers(ctx, tx, params)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	if len(members) != chunk.RequestCount || len(params.Items) != len(members) || params.Chunk.CompletedCount+params.Chunk.FailedCount+params.Chunk.ExpiredCount > chunk.RequestCount {
		return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
	}
	memberByOrdinal := make(map[int]preparedDeckBatchMember, len(members))
	for _, member := range members {
		memberByOrdinal[member.Ordinal] = member
	}
	seen := make(map[int]struct{}, len(params.Items))
	retryOrdinals := make(map[int]struct{})
	for _, item := range params.Items {
		member, ok := memberByOrdinal[item.Ordinal]
		if !ok {
			return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
		}
		if _, duplicate := seen[item.Ordinal]; duplicate {
			return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
		}
		seen[item.Ordinal] = struct{}{}
		if item.State != domain.PreparedDeckOutcomeCompleted && item.State != domain.PreparedDeckOutcomePending && item.State != domain.PreparedDeckOutcomeFailed {
			return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
		}
		if item.State == domain.PreparedDeckOutcomeFailed && strings.TrimSpace(item.ErrorClass) == "" {
			return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
		}
		if err = validateBoundedError(item.ErrorClass, item.ErrorCode); err != nil {
			return PreparedDeckBatchReconcileResult{}, err
		}
		outcome, getErr := scanPreparedDeckOutcome(tx.QueryRow(ctx, `SELECT `+preparedDeckOutcomeColumns+` FROM deck_preparation_translation_outcomes WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND ordinal=$4 FOR UPDATE`, params.OwnerID, params.PreparationID, params.RunID, item.Ordinal))
		if getErr != nil {
			return PreparedDeckBatchReconcileResult{}, getErr
		}
		if outcome.State == domain.PreparedDeckOutcomeCompleted || outcome.State == domain.PreparedDeckOutcomeFailed || outcome.State == domain.PreparedDeckOutcomeCancelled {
			continue
		}
		if outcome.State != domain.PreparedDeckOutcomePending && outcome.State != domain.PreparedDeckOutcomeRunning {
			return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
		}
		if item.State == domain.PreparedDeckOutcomePending && chunk.Generation >= run.MaxBatchGenerations {
			return PreparedDeckBatchReconcileResult{}, ErrInvalidTransition
		}
		if item.State == domain.PreparedDeckOutcomeCompleted {
			if item.CacheEntry == nil || item.CacheEntry.CacheKey != member.Key || item.CacheEntry.CachedAt.IsZero() {
				return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
			}
			entry := item.CacheEntry
			if _, err = tx.Exec(ctx, `INSERT INTO enrichment_cache(language,target_language,canonical_lemma,upos,provider,provider_version,sentence_hash,translation,gloss,sentence_translation,sentence_translation_target,cached_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`, entry.Language, entry.TargetLanguage, entry.CanonicalLemma, entry.UPOS, entry.Provider, entry.ProviderVersion, entry.SentenceHash, entry.Translation, entry.Gloss, entry.SentenceTranslation, entry.SentenceTranslationTarget, entry.CachedAt); err != nil {
				return PreparedDeckBatchReconcileResult{}, err
			}
			// The cache is immutable and first-writer-wins. Another run may have
			// populated this exact key after this Batch was submitted; that row is
			// the trusted result the finalizer must consume even when a stochastic
			// provider returned different text or this attempt has a later timestamp.
			var stored int
			if err = tx.QueryRow(ctx, `SELECT 1 FROM enrichment_cache WHERE language=$1 AND target_language=$2 AND canonical_lemma=$3 AND upos=$4 AND provider=$5 AND provider_version=$6 AND sentence_hash=$7`, entry.Language, entry.TargetLanguage, entry.CanonicalLemma, entry.UPOS, entry.Provider, entry.ProviderVersion, entry.SentenceHash).Scan(&stored); err != nil {
				return PreparedDeckBatchReconcileResult{}, err
			}
		} else if item.CacheEntry != nil {
			return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
		}
		var terminalAt any
		if item.State != domain.PreparedDeckOutcomePending {
			terminalAt = time.Now().UTC()
		} else {
			retryOrdinals[item.Ordinal] = struct{}{}
		}
		if _, err = tx.Exec(ctx, `UPDATE deck_preparation_translation_outcomes SET state=$5,provider_attempt_count=GREATEST(provider_attempt_count,$6),next_attempt_at=now(),claim_token=NULL,claimed_at=NULL,lease_expires_at=NULL,terminal_at=$7,error_class=$8,error_code=$9,provider_call_count=provider_call_count+1,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND ordinal=$4`, params.OwnerID, params.PreparationID, params.RunID, item.Ordinal, item.State, chunk.Generation, terminalAt, item.ErrorClass, item.ErrorCode); err != nil {
			return PreparedDeckBatchReconcileResult{}, err
		}
	}
	if len(seen) != len(members) {
		return PreparedDeckBatchReconcileResult{}, ErrPreparedDeckIdentity
	}

	retryChunks, err := insertPreparedDeckRetryChunks(ctx, tx, params, run, chunk.Generation+1, retryOrdinals, insertSubmission)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	chunk, err = scanPreparedDeckBatchChunk(tx.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks SET state='completed',provider_status=$5,output_file_id=NULLIF($6,''),error_file_id=NULLIF($7,''),completed_count=$8,failed_count=$9,expired_count=$10,input_tokens=$11,output_tokens=$12,total_tokens=$11::bigint+$12::bigint,error_class=$13,error_code=$14,provider_completed_at=$15,reconciled_at=now(),last_polled_at=now(),reconciliation_claim_token=NULL,reconciliation_claimed_at=NULL,reconciliation_lease_expires_at=NULL,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4 RETURNING `+preparedDeckBatchChunkColumns, params.OwnerID, params.PreparationID, params.RunID, params.ChunkID, params.Chunk.ProviderStatus, params.Chunk.OutputFileID, params.Chunk.ErrorFileID, params.Chunk.CompletedCount, params.Chunk.FailedCount, params.Chunk.ExpiredCount, params.Chunk.InputTokens, params.Chunk.OutputTokens, params.Chunk.ErrorClass, params.Chunk.ErrorCode, params.Chunk.ProviderCompletedAt))
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}

	run, err = advancePreparedDeckRunTx(ctx, tx, run, insertFinalizer)
	if err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PreparedDeckBatchReconcileResult{}, err
	}
	return PreparedDeckBatchReconcileResult{Chunk: chunk, Run: run, RetryChunks: retryChunks}, nil
}

func loadPreparedDeckBatchMembers(ctx context.Context, tx pgx.Tx, params PreparedDeckBatchReconcileParams) ([]preparedDeckBatchMember, error) {
	rows, err := tx.Query(ctx, `SELECT ci.ordinal,mi.language,mi.canonical_lemma,mi.upos,COALESCE(mi.provider,''),COALESCE(mi.provider_version,''),COALESCE(mi.sentence_hash,'') FROM deck_preparation_batch_chunk_items ci JOIN deck_preparation_manifest_items mi ON mi.owner_id=ci.owner_id AND mi.preparation_id=ci.preparation_id AND mi.run_id=ci.run_id AND mi.ordinal=ci.ordinal WHERE ci.owner_id=$1 AND ci.preparation_id=$2 AND ci.run_id=$3 AND ci.chunk_id=$4 ORDER BY ci.position`, params.OwnerID, params.PreparationID, params.RunID, params.ChunkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []preparedDeckBatchMember
	for rows.Next() {
		var member preparedDeckBatchMember
		if err = rows.Scan(&member.Ordinal, &member.Key.Language, &member.Key.CanonicalLemma, &member.Key.UPOS, &member.Key.Provider, &member.Key.ProviderVersion, &member.Key.SentenceHash); err != nil {
			return nil, err
		}
		if member.Key.Provider == "" || member.Key.ProviderVersion == "" {
			return nil, ErrPreparedDeckIdentity
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func insertPreparedDeckRetryChunks(ctx context.Context, tx pgx.Tx, params PreparedDeckBatchReconcileParams, run domain.PreparedDeckRun, generation int, retryOrdinals map[int]struct{}, insertSubmission PreparedDeckBatchSubmissionJobInserter) ([]domain.PreparedDeckBatchChunk, error) {
	planned := make(map[int]struct{}, len(retryOrdinals))
	for _, plan := range params.RetryChunks {
		if plan.Generation != generation || generation > run.MaxBatchGenerations || plan.Model != run.Model || plan.Endpoint != run.Endpoint || plan.SplitReason != "retry" || len(plan.Ordinals) == 0 || len(plan.Ordinals) > run.BatchMaxRequests || plan.InputBytes < 1 || plan.InputBytes > run.BatchMaxBytes {
			return nil, ErrInvalidTransition
		}
		for _, ordinal := range plan.Ordinals {
			if _, ok := retryOrdinals[ordinal]; !ok {
				return nil, ErrPreparedDeckIdentity
			}
			if _, duplicate := planned[ordinal]; duplicate {
				return nil, ErrPreparedDeckIdentity
			}
			planned[ordinal] = struct{}{}
		}
	}
	if len(planned) != len(retryOrdinals) || (len(retryOrdinals) > 0 && insertSubmission == nil) {
		return nil, ErrInvalidTransition
	}
	var nextIndex int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(chunk_index),-1)+1 FROM deck_preparation_batch_chunks WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND generation=$4`, params.OwnerID, params.PreparationID, params.RunID, generation).Scan(&nextIndex); err != nil {
		return nil, err
	}
	chunks := make([]domain.PreparedDeckBatchChunk, 0, len(params.RetryChunks))
	for _, plan := range params.RetryChunks {
		chunkID := plan.ID
		if chunkID == "" {
			chunkID = uuid.NewString()
		}
		first, last := plan.Ordinals[0], plan.Ordinals[len(plan.Ordinals)-1]
		chunkIndex := nextIndex + len(chunks)
		if _, err := tx.Exec(ctx, `INSERT INTO deck_preparation_batch_chunks(id,owner_id,preparation_id,run_id,chunk_index,generation,model,endpoint,split_reason,first_ordinal,last_ordinal,input_digest,request_count,input_bytes,estimated_prompt_tokens) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'retry',$9,$10,$11,$12,$13,$14)`, chunkID, params.OwnerID, params.PreparationID, params.RunID, chunkIndex, generation, plan.Model, plan.Endpoint, first, last, plan.InputDigest, len(plan.Ordinals), plan.InputBytes, plan.EstimatedPromptTokens); err != nil {
			return nil, err
		}
		for position, ordinal := range plan.Ordinals {
			if _, err := tx.Exec(ctx, `INSERT INTO deck_preparation_batch_chunk_items(owner_id,preparation_id,run_id,chunk_id,generation,position,ordinal,candidate_digest) SELECT $1,$2,$3,$4,$5,$6,$7,candidate_digest FROM deck_preparation_manifest_items WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND ordinal=$7`, params.OwnerID, params.PreparationID, params.RunID, chunkID, generation, position, ordinal); err != nil {
				return nil, err
			}
		}
		chunk := domain.PreparedDeckBatchChunk{ID: chunkID, OwnerID: params.OwnerID, PreparationID: params.PreparationID, RunID: params.RunID, ChunkIndex: chunkIndex, Generation: generation, State: domain.PreparedDeckBatchPending, Model: plan.Model, Endpoint: plan.Endpoint, SplitReason: "retry", FirstOrdinal: first, LastOrdinal: last, InputDigest: plan.InputDigest, RequestCount: len(plan.Ordinals), InputBytes: plan.InputBytes, EstimatedPromptTokens: plan.EstimatedPromptTokens, Ordinals: append([]int(nil), plan.Ordinals...)}
		jobID, err := insertSubmission(ctx, tx, chunk)
		if err != nil || jobID < 1 {
			if err == nil {
				err = ErrInvalidTransition
			}
			return nil, err
		}
		if err = setPreparedDeckBatchSubmissionJobTx(ctx, tx, chunk, jobID); err != nil {
			return nil, err
		}
		chunk.SubmissionJobID, chunk.SubmissionGeneration = jobID, generation
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

func setPreparedDeckBatchSubmissionJobTx(ctx context.Context, tx pgx.Tx, chunk domain.PreparedDeckBatchChunk, jobID int64) error {
	tag, err := tx.Exec(ctx, `UPDATE deck_preparation_batch_chunks SET submission_job_id=$6,submission_generation=$5,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4 AND generation=$5 AND state='pending'`, chunk.OwnerID, chunk.PreparationID, chunk.RunID, chunk.ID, chunk.Generation, jobID)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrPreparedDeckClaimLost
	}
	return err
}

func advancePreparedDeckRunTx(ctx context.Context, tx pgx.Tx, run domain.PreparedDeckRun, insertFinalizer PreparedDeckRunJobInserter) (domain.PreparedDeckRun, error) {
	var completed, failed, nonterminal int
	if err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE state='completed'),count(*) FILTER (WHERE state='failed'),count(*) FILTER (WHERE state IN ('pending','running')) FROM deck_preparation_translation_outcomes WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3`, run.OwnerID, run.PreparationID, run.ID).Scan(&completed, &failed, &nonterminal); err != nil {
		return run, err
	}
	if nonterminal > 0 {
		return scanPreparedDeckRun(tx.QueryRow(ctx, `UPDATE deck_preparation_runs SET translation_state='running',completed_count=$4,failed_count=$5,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND state='translating' RETURNING `+preparedDeckRunColumns, run.OwnerID, run.PreparationID, run.ID, completed, failed))
	}
	if run.State != domain.PreparedDeckRunTranslating {
		return run, nil
	}
	run, err := scanPreparedDeckRun(tx.QueryRow(ctx, `UPDATE deck_preparation_runs SET state='finalizing',translation_state='completed',completed_count=$4,failed_count=$5,translation_completed_at=COALESCE(translation_completed_at,now()),updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND state='translating' RETURNING `+preparedDeckRunColumns, run.OwnerID, run.PreparationID, run.ID, completed, failed))
	if err != nil {
		return run, err
	}
	if insertFinalizer == nil {
		return run, ErrInvalidTransition
	}
	if err = insertFinalizer(ctx, tx, run); err != nil {
		return domain.PreparedDeckRun{}, err
	}
	return run, nil
}

func (s *PostgresStore) AdvancePreparedDeckRunIfTerminal(ctx context.Context, owner, preparationID, runID string, insertFinalizer PreparedDeckRunJobInserter) (domain.PreparedDeckRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PreparedDeckRun{}, err
	}
	defer tx.Rollback(ctx)
	run, err := scanPreparedDeckRun(tx.QueryRow(ctx, `SELECT `+preparedDeckRunColumns+` FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 FOR UPDATE`, owner, preparationID, runID))
	if err != nil {
		return run, err
	}
	if run.State == domain.PreparedDeckRunFinalizing || run.State == domain.PreparedDeckRunCompleted {
		return run, tx.Commit(ctx)
	}
	if run.State != domain.PreparedDeckRunTranslating {
		return run, ErrInvalidTransition
	}
	run, err = advancePreparedDeckRunTx(ctx, tx, run, insertFinalizer)
	if err != nil {
		return run, err
	}
	return run, tx.Commit(ctx)
}

func (s *PostgresStore) FailPreparedDeckBatchReconciliation(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token, providerStatus, errorClass, errorCode string) error {
	if !terminalProviderStatus(providerStatus) || validateBoundedError(errorClass, errorCode) != nil {
		return ErrInvalidTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var preparationState domain.DeckPreparationState
	if err = tx.QueryRow(ctx, `SELECT state FROM deck_preparations WHERE owner_id=$1 AND id=$2 AND current_run_id=$3 FOR UPDATE`, owner, preparationID, runID).Scan(&preparationState); err != nil {
		return missing(err)
	}
	if preparationState != domain.DeckPreparationPreparing {
		return ErrPreparedDeckClaimLost
	}
	var lockedRun int
	if err = tx.QueryRow(ctx, `SELECT 1 FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND state='translating' FOR UPDATE`, owner, preparationID, runID).Scan(&lockedRun); err != nil {
		return missing(err)
	}
	tag, err := tx.Exec(ctx, `UPDATE deck_preparation_batch_chunks SET state='failed',provider_status=$7,error_class=$8,error_code=$9,reconciled_at=now(),last_polled_at=now(),reconciliation_claim_token=NULL,reconciliation_claimed_at=NULL,reconciliation_lease_expires_at=NULL,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4 AND reconciliation_generation=$5 AND reconciliation_claim_token=$6 AND state='reconciling'`, owner, preparationID, runID, chunkID, generation, token, providerStatus, errorClass, errorCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrPreparedDeckClaimLost
	}
	runErrorClass := preparedDeckRunReconciliationErrorClass(errorClass)
	tag, err = tx.Exec(ctx, `UPDATE deck_preparation_runs SET state='failed',translation_state='failed',error_class=$4,error_code=$5,completed_at=now(),updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND state='translating'`, owner, preparationID, runID, runErrorClass, errorCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrPreparedDeckClaimLost
	}
	if _, err = tx.Exec(ctx, `UPDATE deck_preparations SET state='failed',error='prepared-deck translation results could not be verified',completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 AND current_run_id=$3 AND state='preparing'`, owner, preparationID, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// preparedDeckRunReconciliationErrorClass preserves the most useful bounded
// operator/user recovery category supported by the run schema. Provider
// messages and arbitrary error codes never reach this projection.
func preparedDeckRunReconciliationErrorClass(chunkClass string) string {
	switch chunkClass {
	case "provider", "upload", "poll", "expired", "cancelled":
		return "provider"
	case "validation", "malformed_result", "missing_result", "duplicate_result", "unknown_result":
		return "validation"
	case "configuration", "unsupported_model":
		return "configuration"
	default:
		return "reconciliation"
	}
}

func (s *PostgresStore) FailPreparedDeckBatchSubmission(ctx context.Context, owner, preparationID, runID, chunkID string, generation int, token string, state domain.PreparedDeckBatchChunkState, errorClass, errorCode string) error {
	if state != domain.PreparedDeckBatchFailed && state != domain.PreparedDeckBatchAmbiguous || validateBoundedError(errorClass, errorCode) != nil {
		return ErrInvalidTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var preparationState domain.DeckPreparationState
	if err = tx.QueryRow(ctx, `SELECT state FROM deck_preparations WHERE owner_id=$1 AND id=$2 AND current_run_id=$3 FOR UPDATE`, owner, preparationID, runID).Scan(&preparationState); err != nil {
		return missing(err)
	}
	if preparationState != domain.DeckPreparationPreparing {
		return ErrPreparedDeckClaimLost
	}
	var lockedRun int
	if err = tx.QueryRow(ctx, `SELECT 1 FROM deck_preparation_runs WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND state='translating' FOR UPDATE`, owner, preparationID, runID).Scan(&lockedRun); err != nil {
		return missing(err)
	}
	tag, err := tx.Exec(ctx, `UPDATE deck_preparation_batch_chunks SET state=$7,error_class=$8,error_code=$9,submission_claim_token=NULL,submission_claimed_at=NULL,submission_lease_expires_at=NULL,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4 AND generation=$5 AND submission_claim_token=$6 AND state='submitting'`, owner, preparationID, runID, chunkID, generation, token, state, errorClass, errorCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrPreparedDeckClaimLost
	}
	runClass := "orchestration"
	if state == domain.PreparedDeckBatchAmbiguous {
		runClass = "ambiguous_submission"
	}
	if _, err = tx.Exec(ctx, `UPDATE deck_preparation_runs SET state='failed',translation_state='failed',error_class=$4,error_code=$5,completed_at=now(),updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND state='translating'`, owner, preparationID, runID, runClass, errorCode); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE deck_preparations SET state='failed',error='prepared-deck translation could not be submitted safely',completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 AND current_run_id=$3 AND state='preparing'`, owner, preparationID, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func terminalProviderStatus(status string) bool {
	switch status {
	case "completed", "failed", "expired", "cancelled":
		return true
	default:
		return false
	}
}

func (s *PostgresStore) SetPreparedDeckBatchReconciliationJobTx(ctx context.Context, tx pgx.Tx, owner, preparationID, runID, chunkID string, generation int, jobID int64) error {
	if tx == nil || jobID < 1 {
		return ErrInvalidTransition
	}
	tag, err := tx.Exec(ctx, `UPDATE deck_preparation_batch_chunks SET reconciliation_job_id=$6,reconciliation_claim_token=CASE WHEN state='reconciling' AND reconciliation_lease_expires_at<=now() THEN NULL ELSE reconciliation_claim_token END,reconciliation_claimed_at=CASE WHEN state='reconciling' AND reconciliation_lease_expires_at<=now() THEN NULL ELSE reconciliation_claimed_at END,reconciliation_lease_expires_at=CASE WHEN state='reconciling' AND reconciliation_lease_expires_at<=now() THEN NULL ELSE reconciliation_lease_expires_at END,state=CASE WHEN state='reconciling' AND reconciliation_lease_expires_at<=now() THEN 'polling' ELSE state END,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4 AND reconciliation_generation=$5 AND (state IN ('submitted','polling') OR (state='reconciling' AND reconciliation_lease_expires_at<=now()))`, owner, preparationID, runID, chunkID, generation, jobID)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrPreparedDeckClaimLost
	}
	return err
}

func (s *PostgresStore) ListPreparedDeckLiveBatchIDs(ctx context.Context, owner, preparationID, runID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT batch_id FROM deck_preparation_batch_chunks WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND batch_id IS NOT NULL AND COALESCE(provider_status,'') NOT IN ('completed','failed','expired','cancelled') ORDER BY generation,chunk_index`, owner, preparationID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
