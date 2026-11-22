package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// classicalPreparationRow is the raw durable evidence one classical Book has:
// the newest live or settled River job and the persisted failure record.
type classicalPreparationRow struct {
	bookID          string
	jobState        string
	jobRevision     int
	stage           string
	code            string
	failureRevision int
	prepared        bool
}

// populateClassicalPreparationStatuses attaches each classical Book's current
// preparation state. It reads the newest job for the Book and the per-Book
// failure record. The stage always comes from the record; a River attempt error
// is never parsed for it.
func (s *PostgresStore) populateClassicalPreparationStatuses(ctx context.Context, owner string, books []domain.MyBook) error {
	pointers := make([]*domain.MyBook, len(books))
	for i := range books {
		pointers[i] = &books[i]
	}
	return s.populateClassicalPreparationPointers(ctx, owner, pointers)
}

// populateClassicalPreparationPointers is the pointer form, so a grouped My
// Books row and its flattened page share one population pass.
func (s *PostgresStore) populateClassicalPreparationPointers(ctx context.Context, owner string, books []*domain.MyBook) error {
	ids := make([]string, 0, len(books))
	byID := make(map[string]*domain.MyBook, len(books))
	for i := range books {
		books[i].ClassicalPreparation = nil
		if books[i].ReadingUnit == nil {
			continue
		}
		ids = append(ids, books[i].Book.ID)
		byID[books[i].Book.ID] = books[i]
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT b.id::text,
		       COALESCE(job.state::text, ''),
		       COALESCE((job.args->>'definition_revision')::integer, 0),
		       COALESCE(failure.stage, ''),
		       COALESCE(failure.code, ''),
		       COALESCE(failure.definition_revision, 0),
		       EXISTS(
		           SELECT 1
		           FROM classical_prepared_definitions prepared
		           JOIN analysis_runs run ON run.owner_id = prepared.owner_id
		                                AND run.id = prepared.analysis_run_id
		                                AND run.state = 'completed'
		           WHERE prepared.owner_id = b.owner_id AND prepared.book_id = b.id
		       )
		FROM books b
		LEFT JOIN LATERAL (
		    SELECT j.state, j.args
		    FROM river_job j
		    WHERE j.kind = 'classical_preparation'
		      AND j.args->>'owner_id' = b.owner_id::text
		      AND j.args->>'book_id' = b.id::text
		    ORDER BY j.id DESC
		    LIMIT 1
		) job ON true
		LEFT JOIN book_preparation_failures failure
		       ON failure.owner_id = b.owner_id AND failure.book_id = b.id
		      AND failure.reading_unit_key IS NOT NULL
		WHERE b.owner_id = $1 AND b.id = ANY($2::uuid[])`, owner, ids)
	if err != nil {
		return fmt.Errorf("read classical preparation status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var row classicalPreparationRow
		if err := rows.Scan(&row.bookID, &row.jobState, &row.jobRevision,
			&row.stage, &row.code, &row.failureRevision, &row.prepared); err != nil {
			return fmt.Errorf("scan classical preparation status: %w", err)
		}
		book := byID[row.bookID]
		if book == nil {
			continue
		}
		book.ClassicalPreparation = classicalPreparationStatus(book.ReadingUnit, row)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read classical preparation status: %w", err)
	}
	return nil
}

// classicalPreparationStatus resolves one Book's state from its current
// definition revision and durable evidence. A prepared Book has no preparation
// state: its published analysis is the evidence.
func classicalPreparationStatus(unit *domain.BookReadingUnit, row classicalPreparationRow) *domain.ClassicalPreparationStatus {
	if unit == nil || row.prepared {
		return nil
	}
	if unit.Withdrawn {
		status := domain.WithdrawnClassicalPreparationStatus(*unit)
		return &status
	}
	status := &domain.ClassicalPreparationStatus{CurrentRevision: unit.Revision}
	applyLivePreparationJob(status, unit.Revision, row)
	applyPreparationFailure(status, unit.Revision, row)
	if !status.InProgress() && status.Stage == domain.BookPreparationStageNone && !status.Cleared {
		return nil
	}
	return status
}

// applyLivePreparationJob reports a queued or running job only while it targets
// the Book's current definition revision.
func applyLivePreparationJob(status *domain.ClassicalPreparationStatus, revision int, row classicalPreparationRow) {
	if row.jobRevision != revision {
		return
	}
	switch row.jobState {
	case "available", "pending", "retryable", "scheduled":
		status.State = domain.ClassicalPreparationQueued
	case "running":
		status.State = domain.ClassicalPreparationRunning
	}
}

// applyPreparationFailure applies the durable per-Book failure record. A live
// job for the same revision supersedes it; a newer revision clears it. The
// stage is always read from the row, never inferred from its code.
func applyPreparationFailure(status *domain.ClassicalPreparationStatus, revision int, row classicalPreparationRow) {
	if row.failureRevision == 0 {
		return
	}
	if row.failureRevision == revision {
		if status.InProgress() {
			return
		}
		status.Stage = domain.BookPreparationStage(row.stage)
		status.Code = domain.BookPreparationCode(row.code)
		status.FailureRevision = row.failureRevision
		return
	}
	if row.failureRevision < revision {
		status.Cleared = true
	}
}

// GetClassicalPreparationFailure reads the Book's Classical failure record, or
// reports found=false when the Book has none. An EPUB row for the same Book is
// a different kind and never satisfies this read.
func (s *PostgresStore) GetClassicalPreparationFailure(ctx context.Context, owner, bookID string) (domain.ClassicalPreparationFailureRecord, bool, error) {
	row, err := s.queries().GetClassicalPreparationFailure(ctx, sqlcgen.GetClassicalPreparationFailureParams{
		OwnerID: owner,
		BookID:  bookID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ClassicalPreparationFailureRecord{}, false, nil
	}
	if err != nil {
		return domain.ClassicalPreparationFailureRecord{}, false, fmt.Errorf("read classical preparation failure: %w", err)
	}
	return domain.ClassicalPreparationFailureRecord{
		OwnerID:            row.OwnerID,
		BookID:             row.BookID,
		ReadingUnitKey:     row.ReadingUnitKey.String,
		DefinitionRevision: int(row.DefinitionRevision.Int32),
		Stage:              domain.BookPreparationStage(row.Stage),
		Code:               domain.BookPreparationCode(row.Code),
	}, true, nil
}
