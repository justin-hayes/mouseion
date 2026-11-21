package fixtures

// Contract status: contractual. The fixture adapter passes the shared EPUB
// acquisition failure scenario alongside PostgreSQL.

import (
	"context"
	"errors"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// RecordEPUBAcquisitionFailure stores the latest failed EPUB acquisition for a
// Book, replacing any previous record.
func (s *Store) RecordEPUBAcquisitionFailure(_ context.Context, record domain.EPUBAcquisitionFailureRecord) error {
	switch {
	case record.OwnerID == "" || record.BookID == "":
		return errors.New("fixtures: EPUB acquisition failure owner and Book are required")
	case record.Stage != domain.EPUBAcquisitionFailureEntry &&
		record.Stage != domain.EPUBAcquisitionFailureFetch &&
		record.Stage != domain.EPUBAcquisitionFailureSource &&
		record.Stage != domain.EPUBAcquisitionFailureImport:
		return errors.New("fixtures: EPUB acquisition failure stage is invalid")
	case record.Code == "":
		return errors.New("fixtures: EPUB acquisition failure code is required")
	case (record.ConnectionID == "") != (record.EntryIdentifier == ""):
		return errors.New("fixtures: EPUB acquisition failure connection and entry are recorded together")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epubAcquisitionFailures[fixtureDispositionKey(record.OwnerID, record.BookID)] = record
	return nil
}

// ClearEPUBAcquisitionFailure deletes a Book's failure after a successful
// acquisition. A missing row is not an error.
func (s *Store) ClearEPUBAcquisitionFailure(_ context.Context, owner, bookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.epubAcquisitionFailures, fixtureDispositionKey(owner, bookID))
	return nil
}

// applyEPUBAcquisitionFailure attaches one Book's latest failed acquisition.
// Callers hold s.mu.
func (s *Store) applyEPUBAcquisitionFailure(book *domain.MyBook) {
	failure, ok := s.epubAcquisitionFailures[fixtureDispositionKey(book.Book.OwnerID, book.Book.ID)]
	if !ok {
		book.EPUBAcquisition = nil
		return
	}
	book.EPUBAcquisition = &domain.EPUBAcquisitionStatus{Stage: failure.Stage, Code: failure.Code}
}
