package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// RecordManualPosition records the learner's own Position report for a Book in
// their Reading. The Book's Reading position is its latest report by recorded
// time, with insertion order breaking ties. A Book outside the owner's Reading,
// or another owner's, is ErrNotFound. Recording changes no disposition and no
// Current reading.
func (s *PostgresStore) RecordManualPosition(ctx context.Context, owner, bookID string, fraction float64, recordedAt time.Time) error {
	if owner == "" || bookID == "" {
		return errors.New("persistence: reading position identity is incomplete")
	}
	inserted, err := s.queries().InsertManualPositionReport(ctx, sqlcgen.InsertManualPositionReportParams{
		Fraction: fraction, RecordedAt: recordedAt, Owner: owner, Book: bookID,
	})
	if err != nil {
		return err
	}
	if inserted == 0 {
		return ErrNotFound
	}
	return nil
}

// readingPosition is the Book's latest Position report; ok is false when it
// has none.
func (s *PostgresStore) readingPosition(ctx context.Context, owner, bookID string) (position domain.ReadingPosition, ok bool, err error) {
	row, err := s.queries().GetLatestPositionReport(ctx, sqlcgen.GetLatestPositionReportParams{Owner: owner, Book: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReadingPosition{}, false, nil
	}
	if err != nil {
		return domain.ReadingPosition{}, false, err
	}
	return domain.ReadingPosition{Fraction: row.Fraction, Source: domain.PositionSource(row.Source), RecordedAt: row.RecordedAt}, true, nil
}

// attachReadingPosition sets the Book's Reading position when it has one.
func (s *PostgresStore) attachReadingPosition(ctx context.Context, owner string, book *domain.MyBook) error {
	position, ok, err := s.readingPosition(ctx, owner, book.Book.ID)
	if err != nil || !ok {
		return err
	}
	book.ReadingPosition = &position
	return nil
}
