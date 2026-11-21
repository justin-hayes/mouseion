package fixtures

// Contract status: illustrative. The fixture preparation seam substitutes a
// completed canned result for a real job; it writes no store state.

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// ClassicalPreparation stands in for the preparation job. Moving a fixture
// classical Book to To Read completes immediately; the Book's prepared evidence
// is projected from its To Read state rather than written to the store.
type ClassicalPreparation struct{}

// SubmitClassicalPreparation reports that the Book is already prepared.
func (ClassicalPreparation) SubmitClassicalPreparation(context.Context, string, string) (analysis.ClassicalPreparationHandle, error) {
	return analysis.ClassicalPreparationHandle{Complete: true}, nil
}

// seedClassicalPreparationFailure records the preparation state one fixture Book
// presents, the way the PostgreSQL store reads a durable failure row. It is a
// contract seeding operation, not a learner transition.
func (s *Store) seedClassicalPreparationFailure(owner, bookID string, status domain.ClassicalPreparationStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.myBooks {
		if s.myBooks[i].Book.OwnerID == owner && s.myBooks[i].Book.ID == bookID {
			s.myBooks[i].ClassicalPreparation = &status
			return true
		}
	}
	return false
}
