package persistence

import (
	"context"
	"errors"
	"fmt"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// RecordEPUBAcquisitionFailure records the latest failed EPUB acquisition for
// a Book, replacing any previous record. It stores a stage, a stable code and
// the catalog entry identity only, never source text.
func (s *PostgresStore) RecordEPUBAcquisitionFailure(ctx context.Context, record domain.EPUBAcquisitionFailureRecord) error {
	if err := validateEPUBAcquisitionFailure(record); err != nil {
		return err
	}
	err := s.queries().PutEPUBAcquisitionFailure(ctx, sqlcgen.PutEPUBAcquisitionFailureParams{
		OwnerID:         record.OwnerID,
		BookID:          record.BookID,
		ConnectionID:    nullableUUIDArg(record.ConnectionID),
		EntryIdentifier: nullableTextArg(record.EntryIdentifier),
		Stage:           string(record.Stage),
		Code:            string(record.Code),
	})
	if err != nil {
		return fmt.Errorf("write EPUB acquisition failure: %w", err)
	}
	return nil
}

// ClearEPUBAcquisitionFailure deletes a Book's failure after a successful
// acquisition. A missing row is not an error.
func (s *PostgresStore) ClearEPUBAcquisitionFailure(ctx context.Context, owner, bookID string) error {
	err := s.queries().DeleteEPUBAcquisitionFailure(ctx, sqlcgen.DeleteEPUBAcquisitionFailureParams{
		OwnerID: owner,
		BookID:  bookID,
	})
	if err != nil {
		return fmt.Errorf("clear EPUB acquisition failure: %w", err)
	}
	return nil
}

// validateEPUBAcquisitionFailure refuses an incomplete or unsafe failure record.
func validateEPUBAcquisitionFailure(record domain.EPUBAcquisitionFailureRecord) error {
	switch {
	case record.OwnerID == "":
		return errors.New("persistence: EPUB acquisition failure owner is required")
	case record.BookID == "":
		return errors.New("persistence: EPUB acquisition failure Book is required")
	case record.Stage == domain.BookPreparationStageNone:
		return errors.New("persistence: EPUB acquisition failure stage is required")
	case !record.Stage.IsEPUBAcquisitionStage():
		return errors.New("persistence: EPUB acquisition failure stage is invalid")
	case record.Code == "":
		return errors.New("persistence: EPUB acquisition failure code is required")
	case (record.ConnectionID == "") != (record.EntryIdentifier == ""):
		return errors.New("persistence: EPUB acquisition failure connection and entry are recorded together")
	case record.Stage != domain.BookPreparationStageEntry && record.ConnectionID == "":
		// Only an entry-stage failure may omit the catalog identity; it is
		// recorded when acquisition could not resolve an entry at all.
		return errors.New("persistence: EPUB acquisition failure outside the entry stage requires a catalog entry")
	}
	return nil
}

// populateEPUBAcquisitionStatuses attaches each Book's live acquisition job or
// latest failed EPUB acquisition. The status is evidence only while no analysis
// is in effect, which the domain classification decides. A live job supersedes
// a failure recorded by an earlier attempt.
func (s *PostgresStore) populateEPUBAcquisitionStatuses(ctx context.Context, owner string, books []*domain.MyBook) error {
	ids := make([]string, 0, len(books))
	byID := make(map[string]*domain.MyBook, len(books))
	for _, book := range books {
		book.EPUBAcquisition = nil
		ids = append(ids, book.Book.ID)
		byID[book.Book.ID] = book
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT b.id::text,
		       COALESCE(job.state::text, ''),
		       COALESCE(failure.stage, ''),
		       COALESCE(failure.code, '')
		FROM books b
		LEFT JOIN LATERAL (
		    SELECT j.state
		    FROM river_job j
		    WHERE j.kind = 'epub_acquisition'
		      AND j.args->>'owner_id' = b.owner_id::text
		      AND j.args->>'book_id' = b.id::text
		    ORDER BY j.id DESC
		    LIMIT 1
		) job ON true
		LEFT JOIN book_preparation_failures failure
		       ON failure.owner_id = b.owner_id AND failure.book_id = b.id
		      AND failure.reading_unit_key IS NULL
		WHERE b.owner_id = $1 AND b.id = ANY($2::uuid[])`, owner, ids)
	if err != nil {
		return fmt.Errorf("read EPUB acquisition status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var bookID, jobState, stage, code string
		if err := rows.Scan(&bookID, &jobState, &stage, &code); err != nil {
			return fmt.Errorf("scan EPUB acquisition status: %w", err)
		}
		book := byID[bookID]
		if book == nil {
			continue
		}
		lifecycle := domain.JobLifecycleForRiverState(jobState)
		switch {
		case jobState != "" && lifecycle.Active():
			book.EPUBAcquisition = &domain.EPUBAcquisitionStatus{State: lifecycle}
		case stage != "":
			book.EPUBAcquisition = &domain.EPUBAcquisitionStatus{
				Stage: domain.BookPreparationStage(stage),
				Code:  domain.BookPreparationCode(code),
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read EPUB acquisition status: %w", err)
	}
	return nil
}
