package persistence

import (
	"context"
	"errors"
	"time"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
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
