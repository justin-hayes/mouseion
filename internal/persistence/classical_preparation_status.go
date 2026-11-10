package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// classicalPreparationRow is the raw durable evidence one classical Book has:
// the newest live or settled River job and the persisted failure record.
type classicalPreparationRow struct {
	bookID          string
	jobState        string
	jobRevision     int
	jobError        string
	stage           string
	code            string
	failureRevision int
	prepared        bool
}

// populateClassicalPreparationStatuses attaches each classical Book's current
// preparation state. It reads the newest job for the Book and the per-Book
// failure record. River's ASCII error text is read only to recover a stable
// code; it is never returned to a learner or written to a log.
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
		       COALESCE(job.errors[array_length(job.errors, 1)]->>'error', ''),
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
		    SELECT j.state, j.args, j.errors
		    FROM river_job j
		    WHERE j.kind = 'classical_preparation'
		      AND j.args->>'owner_id' = b.owner_id::text
		      AND j.args->>'book_id' = b.id::text
		    ORDER BY j.id DESC
		    LIMIT 1
		) job ON true
		LEFT JOIN classical_preparation_failures failure
		       ON failure.owner_id = b.owner_id AND failure.book_id = b.id
		WHERE b.owner_id = $1 AND b.id = ANY($2::uuid[])`, owner, ids)
	if err != nil {
		return fmt.Errorf("read classical preparation status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var row classicalPreparationRow
		if err := rows.Scan(&row.bookID, &row.jobState, &row.jobRevision, &row.jobError,
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
		return withdrawnPreparationStatus(unit)
	}
	status := &domain.ClassicalPreparationStatus{CurrentRevision: unit.Revision}
	applyLivePreparationJob(status, unit.Revision, row)
	applyPreparationFailure(status, unit.Revision, row)
	applyJobErrorFallback(status, unit.Revision, row)
	if !status.InProgress() && status.Failure == domain.ClassicalPreparationFailureNone && !status.Cleared {
		return nil
	}
	return status
}

// withdrawnPreparationStatus is the stable state of an unprepared Book whose
// Reading unit is a withdrawn tombstone.
func withdrawnPreparationStatus(unit *domain.BookReadingUnit) *domain.ClassicalPreparationStatus {
	return &domain.ClassicalPreparationStatus{
		Failure:         domain.ClassicalPreparationFailureWithdrawn,
		Code:            classical.WithdrawnCode,
		FailureRevision: unit.Revision,
		CurrentRevision: unit.Revision,
		Withdrawn:       true,
	}
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
// job for the same revision supersedes it; a newer revision clears it.
func applyPreparationFailure(status *domain.ClassicalPreparationStatus, revision int, row classicalPreparationRow) {
	if row.failureRevision == 0 {
		return
	}
	if row.failureRevision == revision {
		if status.InProgress() {
			return
		}
		status.Failure = domain.ClassicalPreparationFailure(row.stage)
		status.Code = row.code
		status.FailureRevision = row.failureRevision
		return
	}
	if row.failureRevision < revision {
		status.Cleared = true
	}
}

// applyJobErrorFallback recovers a failure from a River attempt error for jobs
// written before the per-Book failure record existed.
func applyJobErrorFallback(status *domain.ClassicalPreparationStatus, revision int, row classicalPreparationRow) {
	if status.InProgress() || status.Cleared || status.Failure != domain.ClassicalPreparationFailureNone {
		return
	}
	if row.jobState != "discarded" && row.jobState != "cancelled" {
		return
	}
	if row.jobRevision != revision {
		return
	}
	code := classicalFailureCodeFromError(row.jobError)
	if code == "" {
		// A discarded job with no recognized code was interrupted before it
		// could record failure evidence. Present it as a retryable import
		// failure rather than leaving the Book with no state and no recovery.
		code = classical.PreparationFailedCode
	}
	status.Failure = classicalFailureStageFromCode(code)
	status.Code = code
	status.FailureRevision = revision
}

// classicalFailureCodeFromError recovers a stable code from a River attempt
// error. It exists for jobs written before the per-Book failure record, and
// never returns source text.
func classicalFailureCodeFromError(message string) string {
	for _, code := range []string{
		classical.WithdrawnCode,
		classical.LegacyRowsCode,
		ancientgreek.CodeSourceDownloadFailed,
		ancientgreek.CodeSourceLayerUnknown,
		ancientgreek.CodePinMismatch,
		ancientgreek.CodeOversize,
		ancientgreek.CodeUnknownSourceFormat,
	} {
		if strings.Contains(message, code) {
			return code
		}
	}
	if strings.Contains(message, "ancientgreek: import refused:") {
		return ancientgreek.CodeAdapterRefused
	}
	if strings.Contains(message, classical.PreparationFailedCode) {
		return classical.PreparationFailedCode
	}
	return ""
}

// classicalFailureStageFromCode maps a stable code to the stage that owns its
// recovery.
func classicalFailureStageFromCode(code string) domain.ClassicalPreparationFailure {
	switch code {
	case ancientgreek.CodeSourceDownloadFailed:
		return domain.ClassicalPreparationFailureFetch
	case classical.WithdrawnCode:
		return domain.ClassicalPreparationFailureWithdrawn
	case classical.LegacyRowsCode:
		return domain.ClassicalPreparationFailureCatalog
	case ancientgreek.CodePinMismatch, ancientgreek.CodeOversize, ancientgreek.CodeSourceLayerUnknown,
		ancientgreek.CodeUnknownSourceFormat, ancientgreek.CodeAdapterRefused:
		return domain.ClassicalPreparationFailureSource
	default:
		return domain.ClassicalPreparationFailureImport
	}
}

// GetClassicalPreparationFailure reads the per-Book failure record, or reports
// found=false when the Book has none.
func (s *PostgresStore) GetClassicalPreparationFailure(ctx context.Context, owner, bookID string) (domain.ClassicalPreparationFailureRecord, bool, error) {
	var record domain.ClassicalPreparationFailureRecord
	err := s.pool.QueryRow(ctx, `
		SELECT owner_id::text, book_id::text, reading_unit_key, definition_revision, stage, code
		FROM classical_preparation_failures
		WHERE owner_id = $1 AND book_id = $2`, owner, bookID).
		Scan(&record.OwnerID, &record.BookID, &record.ReadingUnitKey, &record.DefinitionRevision, &record.Stage, &record.Code)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ClassicalPreparationFailureRecord{}, false, nil
	}
	if err != nil {
		return domain.ClassicalPreparationFailureRecord{}, false, fmt.Errorf("read classical preparation failure: %w", err)
	}
	return record, true, nil
}
