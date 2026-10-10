package webapp

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type deckJourneyActionStore struct {
	*fixtures.Store
	goal                   domain.CurrentReading
	bookIDBySourceMaterial map[string]string
	noBookIdentity         map[string]bool
}

// ResolveBookID maps a deck action identity to its canonical Book ID.
func (s *deckJourneyActionStore) ResolveBookID(_ context.Context, _ string, id string) (string, bool, error) {
	if s.noBookIdentity[id] {
		return "", false, nil
	}
	if resolved, ok := s.bookIDBySourceMaterial[id]; ok {
		return resolved, true, nil
	}
	return id, true, nil
}

func (s *deckJourneyActionStore) GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error) {
	return s.goal, nil
}

func (s *deckJourneyActionStore) CountCurrentReadingVocabularyToAccept(context.Context, string, string) (int, error) {
	return 0, nil
}

type journeyIntentStore struct {
	*deckJourneyActionStore
	detail domain.MyBook
}

func (s *journeyIntentStore) GetBookDetail(context.Context, string, string) (domain.MyBook, error) {
	return s.detail, nil
}

func (s *journeyIntentStore) GetBookDisposition(context.Context, string, string) (domain.BookDisposition, error) {
	return s.detail.Disposition, nil
}

func (s *journeyIntentStore) SetBookDisposition(_ context.Context, _, _ string, disposition domain.BookDisposition) error {
	s.detail.Disposition = disposition
	return nil
}

func (s *journeyIntentStore) TransitionBookDisposition(_ context.Context, _, _ string, expected int64, disposition domain.BookDisposition) (bool, error) {
	if s.detail.DispositionRevision != expected {
		return false, persistence.ErrStaleBookDisposition
	}
	s.detail.Disposition = disposition
	s.detail.DispositionRevision++
	return true, nil
}

type journeyIntentAnalysis struct {
	fixtures.Analysis
	calls int
}

func (a *journeyIntentAnalysis) SubmitAnalysis(context.Context, string, string) (analysis.Handle, error) {
	a.calls++
	return analysis.Handle{ID: int64(a.calls), DisplayNumber: int64(a.calls)}, nil
}

func (a *journeyIntentAnalysis) SubmitToReadBookAnalysis(ctx context.Context, owner, bookID, sourceID string) (analysis.Handle, error) {
	return a.SubmitAnalysis(ctx, owner, sourceID)
}
func (*journeyIntentAnalysis) Get(context.Context, string, int64) (analysis.Status, error) {
	return analysis.Status{}, nil
}
