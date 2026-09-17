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
		for _, ordinal := range ordinals {
			chunks[i].Ordinals = append(chunks[i].Ordinals, int(ordinal))
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
	byOrdinal := make(map[int]domain.PreparedDeckTranslationOutcome, len(outcomes))
	for _, outcome := range outcomes {
		byOrdinal[outcome.Ordinal] = outcome
	}
	withCacheKeys := false
	for _, item := range projection.Items {
		withCacheKeys = withCacheKeys || item.CacheKey != nil
	}
	if !withCacheKeys {
		return projection, nil, nil
	}
	records, err := s.loadPreparedDeckStoredRecords(ctx, projection)
	if err != nil {
		return cardexport.StorageProjection{}, nil, err
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
		outcome, ok := byOrdinal[item.Ordinal]
		if !ok || (outcome.State != domain.PreparedDeckOutcomeCompleted && outcome.State != domain.PreparedDeckOutcomeFailed) || item.CacheKey == nil {
			return cardexport.StorageProjection{}, nil, ErrPreparedDeckIdentity
		}
		key := *item.CacheKey
		if outcome.State == domain.PreparedDeckOutcomeCompleted {
			record, found := byStoredOrdinal[item.Ordinal]
			if !found || !record.Found {
				return cardexport.StorageProjection{}, nil, ErrPreparedDeckIdentity
			}
			entry := record.Entry
			stored = append(stored, cardexport.StoredResult{CacheKey: key, Record: entry})
		}
	}
	return projection, stored, nil
}

func (s *PostgresStore) CancelCurrentPreparedDeckRun(ctx context.Context, owner, preparationID string) (result domain.DeckPreparation, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	model, err := q.GetDeckPreparationForUpdate(ctx, sqlcgen.GetDeckPreparationForUpdateParams{Owner: owner, ID: preparationID})
	p := deckPreparationFromModel(model)
	err = missing(err)
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
	model, err = q.CancelDeckPreparation(ctx, sqlcgen.CancelDeckPreparationParams{Owner: owner, ID: preparationID})
	err = missing(err)
	p = deckPreparationFromModel(model)
	if err != nil {
		return p, err
	}
	if runID != "" {
		if err = q.CancelPreparedDeckRun(ctx, sqlcgen.CancelPreparedDeckRunParams{OwnerID: owner, PreparationID: preparationID, ID: runID}); err != nil {
			return p, err
		}
		if err = q.CancelPreparedDeckOutcomes(ctx, sqlcgen.CancelPreparedDeckOutcomesParams{OwnerID: owner, PreparationID: preparationID, RunID: runID}); err != nil {
			return p, err
		}
		if err = q.CancelPreparedDeckBatchChunks(ctx, sqlcgen.CancelPreparedDeckBatchChunksParams{OwnerID: owner, PreparationID: preparationID, RunID: runID}); err != nil {
			return p, err
		}
	}
	details, err := json.Marshal(map[string]string{"preparation_id": preparationID})
	if err != nil {
		return p, err
	}
	if err = q.InsertDeckPreparationHistory(ctx, sqlcgen.InsertDeckPreparationHistoryParams{Owner: owner, Status: "cancelled", Details: details}); err != nil {
		return p, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return p, nil
}
