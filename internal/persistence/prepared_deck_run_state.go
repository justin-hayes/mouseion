package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

func (s *PostgresStore) GetPreparedDeckTranslationOutcome(ctx context.Context, owner, preparationID, runID string, ordinal int) (domain.PreparedDeckTranslationOutcome, error) {
	model, err := s.queries().GetPreparedDeckTranslationOutcome(ctx, sqlcgen.GetPreparedDeckTranslationOutcomeParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, Ordinal: ordinal,
	})
	if err != nil {
		return domain.PreparedDeckTranslationOutcome{}, missing(err)
	}
	return preparedDeckOutcomeFromModel(model), nil
}

func (s *PostgresStore) ListPreparedDeckTranslationOutcomes(ctx context.Context, owner, preparationID, runID string) ([]domain.PreparedDeckTranslationOutcome, error) {
	models, err := s.queries().ListPreparedDeckTranslationOutcomes(ctx, sqlcgen.ListPreparedDeckTranslationOutcomesParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID,
	})
	if err != nil {
		return nil, err
	}
	var outcomes []domain.PreparedDeckTranslationOutcome
	for _, model := range models {
		outcomes = append(outcomes, preparedDeckOutcomeFromModel(model))
	}
	if outcomes == nil {
		exists, existsErr := s.queries().PreparedDeckRunExists(ctx, sqlcgen.PreparedDeckRunExistsParams{
			Owner: owner, Preparation: preparationID, Run: runID,
		})
		if existsErr != nil {
			return nil, existsErr
		}
		if !exists {
			return nil, ErrNotFound
		}
	}
	return outcomes, nil
}

func listPreparedDeckBatchChunks(ctx context.Context, q sqlcgen.DBTX, owner, preparationID, runID string) ([]domain.PreparedDeckBatchChunk, error) {
	models, err := sqlcgen.New(q).ListPreparedDeckBatchChunks(ctx, sqlcgen.ListPreparedDeckBatchChunksParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID,
	})
	if err != nil {
		return nil, err
	}
	var chunks []domain.PreparedDeckBatchChunk
	for _, model := range models {
		chunks = append(chunks, preparedDeckBatchChunkFromModel(model))
	}
	for i := range chunks {
		ordinals, queryErr := sqlcgen.New(q).ListPreparedDeckBatchChunkOrdinals(ctx, sqlcgen.ListPreparedDeckBatchChunkOrdinalsParams{
			OwnerID: owner, PreparationID: preparationID, RunID: runID, ChunkID: chunks[i].ID,
		})
		if queryErr != nil {
			return nil, queryErr
		}
		chunks[i].Ordinals = append(chunks[i].Ordinals, ordinals...)
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
	rows, err := sqlcgen.New(tx).SetPreparedDeckBatchSubmissionJob(ctx, sqlcgen.SetPreparedDeckBatchSubmissionJobParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, SubmissionGeneration: generation, SubmissionJobID: pgtype.Int8{Int64: jobID, Valid: true},
	})
	if err == nil && rows == 0 {
		return ErrPreparedDeckClaimLost
	}
	return err
}

func (s *PostgresStore) SetPreparedDeckTranslationJobTx(ctx context.Context, tx pgx.Tx, owner, preparationID, runID string, ordinal, generation int, jobID int64) error {
	if jobID < 1 {
		return ErrInvalidTransition
	}
	rows, err := sqlcgen.New(tx).SetPreparedDeckTranslationJob(ctx, sqlcgen.SetPreparedDeckTranslationJobParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, Ordinal: ordinal, DispatchGeneration: generation, RiverJobID: pgtype.Int8{Int64: jobID, Valid: true},
	})
	if err == nil && rows == 0 {
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
	model, err := sqlcgen.New(tx).RecordPreparedDeckBatchSubmitted(ctx, sqlcgen.RecordPreparedDeckBatchSubmittedParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, Generation: generation, SubmissionClaimToken: uuidArg(token), InputFileID: textArg(inputFileID), BatchID: textArg(batchID), SubmittedAt: pgtype.Timestamptz{Time: submittedAt, Valid: true},
	})
	chunk := preparedDeckBatchChunkFromModel(model)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
	model, err = sqlcgen.New(tx).AttachPreparedDeckBatchReconciliationJob(ctx, sqlcgen.AttachPreparedDeckBatchReconciliationJobParams{
		OwnerID: owner, PreparationID: preparationID, RunID: runID, ID: chunkID, ReconciliationJobID: pgtype.Int8{Int64: jobID, Valid: true}, BatchID: textArg(batchID),
	})
	chunk = preparedDeckBatchChunkFromModel(model)
	return chunk, missing(err)
}

func (s *PostgresStore) SetPreparedDeckFinalizationJobTx(ctx context.Context, tx pgx.Tx, owner, preparationID, runID string, generation int, jobID int64) error {
	rows, err := sqlcgen.New(tx).SetPreparedDeckFinalizationJob(ctx, sqlcgen.SetPreparedDeckFinalizationJobParams{
		OwnerID: owner, PreparationID: preparationID, ID: runID, FinalizationDispatchGeneration: generation, FinalizationJobID: pgtype.Int8{Int64: jobID, Valid: true},
	})
	if err == nil && rows == 0 {
		return ErrPreparedDeckClaimLost
	}
	return err
}

func (s *PostgresStore) LoadPreparedDeckFinalization(ctx context.Context, owner, preparationID, runID string) (cardexport.StorageProjection, []cardexport.StoredResult, error) {
	projection, outcomes, err := s.loadPreparedDeckFinalizationInputs(ctx, owner, preparationID, runID)
	if err != nil {
		return cardexport.StorageProjection{}, nil, err
	}
	if !preparedDeckProjectionHasCacheKeys(projection) {
		return projection, nil, nil
	}
	records, err := s.loadPreparedDeckStoredRecords(ctx, projection)
	if err != nil {
		return cardexport.StorageProjection{}, nil, err
	}
	stored, err := derivePreparedDeckStoredResults(projection, outcomes, records)
	if err != nil {
		return cardexport.StorageProjection{}, nil, err
	}
	return projection, stored, nil
}

// loadPreparedDeckFinalizationInputs reads and guards the rows a finalizer
// needs: a finalizing or completed run with completed translation, its durable
// projection, and its translation outcomes.
func (s *PostgresStore) loadPreparedDeckFinalizationInputs(ctx context.Context, owner, preparationID, runID string) (cardexport.StorageProjection, []domain.PreparedDeckTranslationOutcome, error) {
	run, err := s.GetPreparedDeckRun(ctx, owner, preparationID, runID)
	if err != nil {
		return cardexport.StorageProjection{}, nil, err
	}
	if (run.State != domain.PreparedDeckRunFinalizing && run.State != domain.PreparedDeckRunCompleted) || run.TranslationState != domain.PreparedDeckTranslationCompleted {
		return cardexport.StorageProjection{}, nil, ErrInvalidTransition
	}
	projection, _, err := s.LoadPreparedDeckStorageProjection(ctx, owner, preparationID, runID)
	if err != nil {
		return cardexport.StorageProjection{}, nil, err
	}
	outcomes, err := s.ListPreparedDeckTranslationOutcomes(ctx, owner, preparationID, runID)
	if err != nil {
		return cardexport.StorageProjection{}, nil, err
	}
	return projection, outcomes, nil
}

// preparedDeckProjectionHasCacheKeys reports whether any manifest item carries
// a frozen enrichment cache identity.
func preparedDeckProjectionHasCacheKeys(projection cardexport.StorageProjection) bool {
	for _, item := range projection.Items {
		if item.CacheKey != nil {
			return true
		}
	}
	return false
}

// derivePreparedDeckStoredResults pairs each accepted manifest item with its
// terminal outcome and, where completed, its stored record or omission.
func derivePreparedDeckStoredResults(projection cardexport.StorageProjection, outcomes []domain.PreparedDeckTranslationOutcome, records []PreparedDeckStoredRecord) ([]cardexport.StoredResult, error) {
	byOrdinal := make(map[int]domain.PreparedDeckTranslationOutcome, len(outcomes))
	for _, outcome := range outcomes {
		byOrdinal[outcome.Ordinal] = outcome
	}
	byStoredOrdinal := make(map[int]PreparedDeckStoredRecord, len(records))
	for _, record := range records {
		byStoredOrdinal[record.Ordinal] = record
	}
	stored := make([]cardexport.StoredResult, 0, len(outcomes))
	for _, item := range projection.Items {
		if item.Disposition != cardexport.ManifestAccepted {
			continue
		}
		result, include, err := derivePreparedDeckStoredResult(item, byOrdinal, byStoredOrdinal)
		if err != nil {
			return nil, err
		}
		if include {
			stored = append(stored, result)
		}
	}
	return stored, nil
}

// derivePreparedDeckStoredResult maps one accepted item to the stored finalizer
// input, or reports that the outcome is not a match.
func derivePreparedDeckStoredResult(item cardexport.ManifestItem, byOrdinal map[int]domain.PreparedDeckTranslationOutcome, byStoredOrdinal map[int]PreparedDeckStoredRecord) (cardexport.StoredResult, bool, error) {
	outcome, ok := byOrdinal[item.Ordinal]
	if !ok || (outcome.State != domain.PreparedDeckOutcomeCompleted && outcome.State != domain.PreparedDeckOutcomeFailed) || item.CacheKey == nil {
		return cardexport.StoredResult{}, false, ErrPreparedDeckIdentity
	}
	key := *item.CacheKey
	if outcome.State == domain.PreparedDeckOutcomeFailed {
		return cardexport.StoredResult{}, false, nil
	}
	if outcome.OmissionReason != "" {
		return cardexport.StoredResult{CacheKey: key, OmissionReason: outcome.OmissionReason}, true, nil
	}
	record, found := byStoredOrdinal[item.Ordinal]
	if !found || !record.Found {
		return cardexport.StoredResult{}, false, ErrPreparedDeckIdentity
	}
	return cardexport.StoredResult{CacheKey: key, Record: record.Entry}, true, nil
}

func (s *PostgresStore) CancelCurrentPreparedDeckRun(ctx context.Context, owner, preparationID string) (result domain.DeckPreparation, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	p, err := lockPreparedDeckPreparationForCancellation(ctx, q, owner, preparationID)
	if err != nil {
		return p, err
	}
	if p.State == domain.DeckPreparationCancelled {
		return p, tx.Commit(ctx)
	}
	if p.State != domain.DeckPreparationQueued && p.State != domain.DeckPreparationPreparing {
		return p, ErrInvalidTransition
	}
	p, err = cancelPreparedDeckPreparationTx(ctx, q, owner, preparationID, p)
	if err != nil {
		return p, err
	}
	if err := insertPreparedDeckCancellationHistory(ctx, q, owner, preparationID); err != nil {
		return p, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return p, nil
}

// lockPreparedDeckPreparationForCancellation locks the preparation row and maps
// a missing row to ErrNotFound.
func lockPreparedDeckPreparationForCancellation(ctx context.Context, q *sqlcgen.Queries, owner, preparationID string) (domain.DeckPreparation, error) {
	model, err := q.GetDeckPreparationForUpdate(ctx, sqlcgen.GetDeckPreparationForUpdateParams{Owner: owner, ID: preparationID})
	p := deckPreparationFromModel(model)
	if err := missing(err); err != nil {
		return p, err
	}
	return p, nil
}

// cancelPreparedDeckPreparationTx cancels the preparation and then, when it had
// a current run, every durable part of that run.
func cancelPreparedDeckPreparationTx(ctx context.Context, q *sqlcgen.Queries, owner, preparationID string, p domain.DeckPreparation) (domain.DeckPreparation, error) {
	runID := p.CurrentRunID
	model, cancelErr := q.CancelDeckPreparation(ctx, sqlcgen.CancelDeckPreparationParams{Owner: owner, ID: preparationID})
	p = deckPreparationFromModel(model)
	if err := missing(cancelErr); err != nil {
		return p, err
	}
	if runID == "" {
		return p, nil
	}
	if err := q.CancelPreparedDeckRun(ctx, sqlcgen.CancelPreparedDeckRunParams{OwnerID: owner, PreparationID: preparationID, ID: runID}); err != nil {
		return p, err
	}
	if err := q.CancelPreparedDeckOutcomes(ctx, sqlcgen.CancelPreparedDeckOutcomesParams{OwnerID: owner, PreparationID: preparationID, RunID: runID}); err != nil {
		return p, err
	}
	if err := q.CancelPreparedDeckBatchChunks(ctx, sqlcgen.CancelPreparedDeckBatchChunksParams{OwnerID: owner, PreparationID: preparationID, RunID: runID}); err != nil {
		return p, err
	}
	return p, nil
}

// insertPreparedDeckCancellationHistory records the cancellation audit row.
func insertPreparedDeckCancellationHistory(ctx context.Context, q *sqlcgen.Queries, owner, preparationID string) error {
	details, err := json.Marshal(map[string]string{"preparation_id": preparationID})
	if err != nil {
		return err
	}
	return q.InsertDeckPreparationHistory(ctx, sqlcgen.InsertDeckPreparationHistoryParams{Owner: owner, Status: "cancelled", Details: details})
}
