package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

const preparedDeckOutcomeColumns = `owner_id::text,preparation_id::text,run_id::text,ordinal,state,dispatch_count,provider_attempt_count,max_provider_attempts,next_attempt_at,dispatch_generation,COALESCE(river_job_id,0),COALESCE(claim_token::text,''),claimed_at,lease_expires_at,terminal_at,error_class,error_code,cache_hit_count,provider_call_count,cache_latency_ms,provider_latency_ms,updated_at`

func qualifiedColumns(alias, columns string) string {
	var parts []string
	start, depth := 0, 0
	for i, r := range columns {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, columns[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, columns[start:])
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "COALESCE(") {
			part = strings.Replace(part, "COALESCE(", "COALESCE("+alias+".", 1)
		} else {
			part = alias + "." + part
		}
		parts[i] = part
	}
	return strings.Join(parts, ",")
}

func scanPreparedDeckOutcome(row rowScanner) (domain.PreparedDeckTranslationOutcome, error) {
	var outcome domain.PreparedDeckTranslationOutcome
	var cacheLatencyMS, providerLatencyMS int64
	err := row.Scan(&outcome.OwnerID, &outcome.PreparationID, &outcome.RunID, &outcome.Ordinal, &outcome.State, &outcome.DispatchCount, &outcome.ProviderAttemptCount, &outcome.MaxProviderAttempts, &outcome.NextAttemptAt, &outcome.DispatchGeneration, &outcome.RiverJobID, &outcome.ClaimToken, &outcome.ClaimedAt, &outcome.LeaseExpiresAt, &outcome.TerminalAt, &outcome.ErrorClass, &outcome.ErrorCode, &outcome.CacheHitCount, &outcome.ProviderCallCount, &cacheLatencyMS, &providerLatencyMS, &outcome.UpdatedAt)
	outcome.CacheLatency = time.Duration(cacheLatencyMS) * time.Millisecond
	outcome.ProviderLatency = time.Duration(providerLatencyMS) * time.Millisecond
	return outcome, missing(err)
}

func (s *PostgresStore) GetPreparedDeckTranslationOutcome(ctx context.Context, owner, preparationID, runID string, ordinal int) (domain.PreparedDeckTranslationOutcome, error) {
	return scanPreparedDeckOutcome(s.pool.QueryRow(ctx, `SELECT `+preparedDeckOutcomeColumns+` FROM deck_preparation_translation_outcomes WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND ordinal=$4`, owner, preparationID, runID, ordinal))
}

func (s *PostgresStore) ListPreparedDeckTranslationOutcomes(ctx context.Context, owner, preparationID, runID string) ([]domain.PreparedDeckTranslationOutcome, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+preparedDeckOutcomeColumns+` FROM deck_preparation_translation_outcomes WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 ORDER BY ordinal`, owner, preparationID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var outcomes []domain.PreparedDeckTranslationOutcome
	for rows.Next() {
		outcome, scanErr := scanPreparedDeckOutcome(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		outcomes = append(outcomes, outcome)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if outcomes == nil {
		if _, err = s.GetPreparedDeckRun(ctx, owner, preparationID, runID); err != nil {
			return nil, err
		}
	}
	return outcomes, nil
}

const preparedDeckBatchChunkColumns = `id::text,owner_id::text,preparation_id::text,run_id::text,chunk_index,generation,state,COALESCE(provider_status,''),model,endpoint,split_reason,first_ordinal,last_ordinal,input_digest,request_count,input_bytes,estimated_prompt_tokens,completed_count,failed_count,expired_count,COALESCE(input_file_id,''),COALESCE(batch_id,''),COALESCE(output_file_id,''),COALESCE(error_file_id,''),COALESCE(submission_job_id,0),submission_generation,COALESCE(submission_claim_token::text,''),submission_claimed_at,submission_lease_expires_at,COALESCE(reconciliation_job_id,0),reconciliation_generation,COALESCE(reconciliation_claim_token::text,''),reconciliation_claimed_at,reconciliation_lease_expires_at,error_class,error_code,input_tokens,output_tokens,total_tokens,created_at,updated_at,submitted_at,last_polled_at,provider_completed_at,reconciled_at`

func scanPreparedDeckBatchChunk(row rowScanner) (domain.PreparedDeckBatchChunk, error) {
	var chunk domain.PreparedDeckBatchChunk
	err := row.Scan(&chunk.ID, &chunk.OwnerID, &chunk.PreparationID, &chunk.RunID, &chunk.ChunkIndex, &chunk.Generation, &chunk.State, &chunk.ProviderStatus, &chunk.Model, &chunk.Endpoint, &chunk.SplitReason, &chunk.FirstOrdinal, &chunk.LastOrdinal, &chunk.InputDigest, &chunk.RequestCount, &chunk.InputBytes, &chunk.EstimatedPromptTokens, &chunk.CompletedCount, &chunk.FailedCount, &chunk.ExpiredCount, &chunk.InputFileID, &chunk.BatchID, &chunk.OutputFileID, &chunk.ErrorFileID, &chunk.SubmissionJobID, &chunk.SubmissionGeneration, &chunk.SubmissionClaimToken, &chunk.SubmissionClaimedAt, &chunk.SubmissionLeaseExpiresAt, &chunk.ReconciliationJobID, &chunk.ReconciliationGeneration, &chunk.ReconciliationClaimToken, &chunk.ReconciliationClaimedAt, &chunk.ReconciliationLeaseExpiresAt, &chunk.ErrorClass, &chunk.ErrorCode, &chunk.InputTokens, &chunk.OutputTokens, &chunk.TotalTokens, &chunk.CreatedAt, &chunk.UpdatedAt, &chunk.SubmittedAt, &chunk.LastPolledAt, &chunk.ProviderCompletedAt, &chunk.ReconciledAt)
	return chunk, missing(err)
}

type preparedDeckQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func listPreparedDeckBatchChunks(ctx context.Context, q preparedDeckQueryer, owner, preparationID, runID string) ([]domain.PreparedDeckBatchChunk, error) {
	rows, err := q.Query(ctx, `SELECT `+preparedDeckBatchChunkColumns+` FROM deck_preparation_batch_chunks WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 ORDER BY generation,chunk_index`, owner, preparationID, runID)
	if err != nil {
		return nil, err
	}
	var chunks []domain.PreparedDeckBatchChunk
	for rows.Next() {
		chunk, scanErr := scanPreparedDeckBatchChunk(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		chunks = append(chunks, chunk)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range chunks {
		itemRows, queryErr := q.Query(ctx, `SELECT ordinal FROM deck_preparation_batch_chunk_items WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND chunk_id=$4 ORDER BY position`, owner, preparationID, runID, chunks[i].ID)
		if queryErr != nil {
			return nil, queryErr
		}
		for itemRows.Next() {
			var ordinal int
			if queryErr = itemRows.Scan(&ordinal); queryErr != nil {
				itemRows.Close()
				return nil, queryErr
			}
			chunks[i].Ordinals = append(chunks[i].Ordinals, ordinal)
		}
		queryErr = itemRows.Err()
		itemRows.Close()
		if queryErr != nil {
			return nil, queryErr
		}
	}
	return chunks, nil
}

func (s *PostgresStore) ListPreparedDeckBatchChunks(ctx context.Context, owner, preparationID, runID string) ([]domain.PreparedDeckBatchChunk, error) {
	chunks, err := listPreparedDeckBatchChunks(ctx, s.pool, owner, preparationID, runID)
	if err != nil || chunks != nil {
		return chunks, err
	}
	if _, err = s.GetPreparedDeckRun(ctx, owner, preparationID, runID); err != nil {
		return nil, err
	}
	return chunks, nil
}

func (s *PostgresStore) GetPreparedDeckBatchChunk(ctx context.Context, owner, preparationID, runID, chunkID string) (domain.PreparedDeckBatchChunk, error) {
	chunks, err := listPreparedDeckBatchChunks(ctx, s.pool, owner, preparationID, runID)
	if err != nil {
		return domain.PreparedDeckBatchChunk{}, err
	}
	for _, chunk := range chunks {
		if chunk.ID == chunkID {
			return chunk, nil
		}
	}
	return domain.PreparedDeckBatchChunk{}, ErrNotFound
}

func (s *PostgresStore) SetPreparedDeckBatchSubmissionJobTx(ctx context.Context, tx pgx.Tx, owner, preparationID, runID, chunkID string, generation int, jobID int64) error {
	tag, err := tx.Exec(ctx, `UPDATE deck_preparation_batch_chunks SET submission_job_id=$7,submission_generation=$6,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4 AND generation=$5 AND state='pending'`, owner, preparationID, runID, chunkID, generation, generation, jobID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrPreparedDeckClaimLost
	}
	return err
}

// PreparedDeckBatchPollJobInserter inserts the short polling job while the
// transaction that attaches the provider Batch ID is still open.
type PreparedDeckBatchPollJobInserter func(context.Context, pgx.Tx, domain.PreparedDeckBatchChunk) (int64, error)

func (s *PostgresStore) RecordPreparedDeckBatchSubmittedTx(ctx context.Context, tx pgx.Tx, owner, preparationID, runID, chunkID string, generation int, token, inputFileID, batchID string, submittedAt time.Time, insertPoll PreparedDeckBatchPollJobInserter) (domain.PreparedDeckBatchChunk, error) {
	if tx == nil || strings.TrimSpace(inputFileID) == "" || strings.TrimSpace(batchID) == "" {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	chunk, err := scanPreparedDeckBatchChunk(tx.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks c SET state='submitted',input_file_id=$7,batch_id=$8,submitted_at=$9,submission_claim_token=NULL,submission_claimed_at=NULL,submission_lease_expires_at=NULL,error_class='',error_code='',updated_at=now() FROM deck_preparation_runs r WHERE c.owner_id=$1 AND c.preparation_id=$2 AND c.run_id=$3 AND c.id=$4 AND c.generation=$5 AND c.submission_generation=$5 AND c.submission_claim_token=$6 AND c.state='submitting' AND r.owner_id=c.owner_id AND r.preparation_id=c.preparation_id AND r.id=c.run_id AND r.state='translating' RETURNING `+qualifiedColumns("c", preparedDeckBatchChunkColumns), owner, preparationID, runID, chunkID, generation, token, inputFileID, batchID, submittedAt))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return chunk, ErrPreparedDeckClaimLost
		}
		return chunk, err
	}
	if insertPoll == nil {
		return chunk, nil
	}
	jobID, err := insertPoll(ctx, tx, chunk)
	if err != nil {
		return domain.PreparedDeckBatchChunk{}, err
	}
	if jobID < 1 {
		return domain.PreparedDeckBatchChunk{}, ErrInvalidTransition
	}
	chunk, err = scanPreparedDeckBatchChunk(tx.QueryRow(ctx, `UPDATE deck_preparation_batch_chunks SET reconciliation_job_id=$5,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND id=$4 AND state='submitted' AND batch_id=$6 RETURNING `+preparedDeckBatchChunkColumns, owner, preparationID, runID, chunkID, jobID, batchID))
	return chunk, err
}

func (s *PostgresStore) SetPreparedDeckFinalizationJobTx(ctx context.Context, tx pgx.Tx, owner, preparationID, runID string, generation int, jobID int64) error {
	tag, err := tx.Exec(ctx, `UPDATE deck_preparation_runs SET finalization_job_id=$5,updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND finalization_dispatch_generation=$4 AND state='finalizing'`, owner, preparationID, runID, generation, jobID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrPreparedDeckClaimLost
	}
	return err
}

func (s *PostgresStore) LoadPreparedDeckFinalization(ctx context.Context, owner, preparationID, runID string) (cardexport.Manifest, []cardexport.ExactEnrichment, error) {
	run, err := s.GetPreparedDeckRun(ctx, owner, preparationID, runID)
	if err != nil {
		return cardexport.Manifest{}, nil, err
	}
	if run.State != domain.PreparedDeckRunFinalizing || run.TranslationState != domain.PreparedDeckTranslationCompleted {
		return cardexport.Manifest{}, nil, ErrInvalidTransition
	}
	snapshot, _, err := s.LoadPreparedDeckManifest(ctx, owner, preparationID, runID)
	if err != nil {
		return cardexport.Manifest{}, nil, err
	}
	manifest, err := cardexport.ManifestFromSnapshot(snapshot)
	if err != nil {
		return cardexport.Manifest{}, nil, err
	}
	outcomes, err := s.ListPreparedDeckTranslationOutcomes(ctx, owner, preparationID, runID)
	if err != nil {
		return cardexport.Manifest{}, nil, err
	}
	byOrdinal := make(map[int]domain.PreparedDeckTranslationOutcome, len(outcomes))
	for _, outcome := range outcomes {
		byOrdinal[outcome.Ordinal] = outcome
	}
	withCacheKeys := false
	for _, item := range snapshot.Items {
		withCacheKeys = withCacheKeys || item.CacheKey != nil
	}
	if !withCacheKeys {
		return manifest, nil, nil
	}
	exact := make([]cardexport.ExactEnrichment, 0, len(outcomes))
	for _, item := range snapshot.Items {
		if item.Disposition != cardexport.ManifestAccepted {
			continue
		}
		outcome, ok := byOrdinal[item.Ordinal]
		if !ok || (outcome.State != domain.PreparedDeckOutcomeCompleted && outcome.State != domain.PreparedDeckOutcomeFailed) || item.CacheKey == nil {
			return cardexport.Manifest{}, nil, ErrPreparedDeckIdentity
		}
		result := enrichment.Result{Candidate: enrichment.Candidate{Identity: enrichment.Identity{Language: item.Entry.Language, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS}, TargetWord: item.Entry.TargetWord, ExampleSentence: item.Entry.Sentence}}
		if outcome.State == domain.PreparedDeckOutcomeCompleted {
			entry, found, cacheErr := s.Get(ctx, *item.CacheKey)
			if cacheErr != nil {
				return cardexport.Manifest{}, nil, cacheErr
			}
			if !found {
				return cardexport.Manifest{}, nil, ErrPreparedDeckIdentity
			}
			provenance := enrichment.Provenance{Provider: item.CacheKey.Provider, ProviderVersion: item.CacheKey.ProviderVersion, CachedAt: entry.CachedAt, External: true}
			if entry.Translation != "" {
				result.Translation = enrichment.Field[string]{Value: entry.Translation, Available: true, Provenance: provenance}
			}
			if entry.SentenceTranslation != "" {
				result.SentenceTranslation = enrichment.Field[string]{Value: entry.SentenceTranslation, Available: true, Provenance: provenance}
			}
			if entry.SentenceTranslationTarget != "" {
				result.SentenceTranslationTarget = enrichment.Field[string]{Value: entry.SentenceTranslationTarget, Available: true, Provenance: provenance}
			}
		}
		exact = append(exact, cardexport.ExactEnrichment{CacheKey: *item.CacheKey, Result: result})
	}
	return manifest, exact, nil
}

func (s *PostgresStore) CancelCurrentPreparedDeckRun(ctx context.Context, owner, preparationID string) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanDeckPreparation(tx.QueryRow(ctx, `SELECT `+deckPreparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, preparationID))
	if err != nil {
		return p, err
	}
	if p.State == domain.DeckPreparationCancelled {
		return p, tx.Commit(ctx)
	}
	if p.State != domain.DeckPreparationQueued && p.State != domain.DeckPreparationPreparing {
		return p, ErrInvalidTransition
	}
	runID := p.CurrentRunID
	p, err = scanDeckPreparation(tx.QueryRow(ctx, `UPDATE deck_preparations SET state='cancelled',error='',completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 RETURNING `+deckPreparationColumns, owner, preparationID))
	if err != nil {
		return p, err
	}
	if runID != "" {
		if _, err = tx.Exec(ctx, `UPDATE deck_preparation_runs SET state='cancelled',translation_state=CASE WHEN translation_state IN ('pending','running') THEN 'cancelled' ELSE translation_state END,finalization_claim_token=NULL,finalization_claimed_at=NULL,finalization_lease_expires_at=NULL,error_class='',error_code='',completed_at=now(),updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND id=$3 AND state IN ('translating','finalizing')`, owner, preparationID, runID); err != nil {
			return p, err
		}
		if _, err = tx.Exec(ctx, `UPDATE deck_preparation_translation_outcomes SET state='cancelled',claim_token=NULL,claimed_at=NULL,lease_expires_at=NULL,terminal_at=now(),error_class='cancellation',error_code='',updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND state IN ('pending','running')`, owner, preparationID, runID); err != nil {
			return p, err
		}
		if _, err = tx.Exec(ctx, `UPDATE deck_preparation_batch_chunks SET state='cancelled',submission_claim_token=NULL,submission_claimed_at=NULL,submission_lease_expires_at=NULL,reconciliation_claim_token=NULL,reconciliation_claimed_at=NULL,reconciliation_lease_expires_at=NULL,error_class='cancelled',error_code='',updated_at=now() WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND state NOT IN ('completed','failed','cancelled')`, owner, preparationID, runID); err != nil {
			return p, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details,completed_at) VALUES($1,'prepared_deck','cancelled',jsonb_build_object('preparation_id',$2::text),now())`, owner, preparationID); err != nil {
		return p, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return p, nil
}
