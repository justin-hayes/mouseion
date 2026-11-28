package fixtures

// Contract status: contractual. Held to internal/storecontract parity.

import (
	"context"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// fixturePositionReport is one recorded Position report. Reports are kept in
// insertion order, which breaks a tie between equal recorded times.
type fixturePositionReport struct {
	owner, bookID string
	fraction      float64
	recordedAt    time.Time
}

// RecordManualPosition records the learner's own Position report for a Book in
// their Reading, as the Postgres store does.
func (s *Store) RecordManualPosition(_ context.Context, owner, bookID string, fraction float64, recordedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for _, book := range s.myBooksForOwner(owner) {
		if book.Book.ID == bookID {
			found = true
			break
		}
	}
	if !found {
		return domain.ErrNotFound
	}
	s.positionReports = append(s.positionReports, fixturePositionReport{owner: owner, bookID: bookID, fraction: fraction, recordedAt: recordedAt})
	return nil
}

// readingPositionLocked is the Book's latest report by recorded time, the later
// insertion winning a tie. Callers hold s.mu.
func (s *Store) readingPositionLocked(owner, bookID string) *domain.ReadingPosition {
	var latest *fixturePositionReport
	for i := range s.positionReports {
		report := &s.positionReports[i]
		if report.owner != owner || report.bookID != bookID {
			continue
		}
		if latest == nil || !report.recordedAt.Before(latest.recordedAt) {
			latest = report
		}
	}
	if latest == nil {
		return nil
	}
	return &domain.ReadingPosition{Fraction: latest.fraction, Source: domain.PositionSourceManual, RecordedAt: latest.recordedAt}
}
