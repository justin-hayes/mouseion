package webapp

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deckJourneyActionStore struct {
	GoalStore
	goal                   domain.PrimaryGoal
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

func (s *deckJourneyActionStore) GetPrimaryGoal(context.Context, string, string) (domain.PrimaryGoal, error) {
	return s.goal, nil
}

func (s *deckJourneyActionStore) CountPrimaryGoalVocabularyToGraduate(context.Context, string, string) (int, error) {
	return 0, nil
}

type journeyIntentStore struct {
	*deckJourneyActionStore
	BookStore
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

func (s *journeyIntentStore) SetBookAside(_ context.Context, _, _, _ string) error {
	s.detail.Disposition = domain.BookDispositionSetAside
	return nil
}

type journeyIntentCatalogue struct {
	target cataloguesync.AcquisitionTarget
	err    error
}

func (journeyIntentCatalogue) RegisterConnection(context.Context, string, string) error { return nil }
func (journeyIntentCatalogue) UnregisterConnection(string, string) error                { return nil }
func (c journeyIntentCatalogue) FindAcquisitionTarget(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
	return c.target, c.err
}

type journeyIntentOPDS struct {
	acquire func()
}

func (o journeyIntentOPDS) AcquireForBook(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
	if o.acquire != nil {
		o.acquire()
	}
	return epub.ImportResult{}, nil
}

type journeyIntentAnalysis struct{ calls int }

func (a *journeyIntentAnalysis) SubmitAnalysis(context.Context, string, string) (analysis.Handle, error) {
	a.calls++
	return analysis.Handle{ID: int64(a.calls), DisplayNumber: int64(a.calls)}, nil
}
func (*journeyIntentAnalysis) Get(context.Context, string, int64) (analysis.Status, error) {
	return analysis.Status{}, nil
}

func TestMovingBookToReadEnsuresAcquisitionAndAnalysisOnce(t *testing.T) {
	store := &journeyIntentStore{
		deckJourneyActionStore: &deckJourneyActionStore{},
		detail:                 domain.MyBook{Book: domain.Book{ID: "book-1", OwnerID: "owner-1", Title: "Book one"}},
	}
	analysisService := &journeyIntentAnalysis{}
	store.detail.Acquired = &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-1", MediaType: opds.EPUBMediaType, ContentRevisionID: "revision-1", ContentSnapshotID: "snapshot-1"}}
	h := &Handler{services: Services{
		Store:         StoreDependencies{Books: store, Goals: store},
		Analysis:      analysisService,
		CatalogueSync: journeyIntentCatalogue{},
	}}

	first, err := h.addBookToReadingJourney(context.Background(), "owner-1", "", "book-1")
	require.NoError(t, err)
	assert.Equal(t, deckIsToRead, first.State)
	assert.Equal(t, 1, analysisService.calls)

	second, err := h.addBookToReadingJourney(context.Background(), "owner-1", "", "book-1")
	require.NoError(t, err)
	assert.Equal(t, deckIsToRead, second.State)
	assert.Equal(t, 1, analysisService.calls)
	assert.True(t, strings.Contains(second.Message, "already in To Read"))
}

func TestMovingMetadataOnlyBookToReadRetainsDispositionWhenAcquisitionUnavailable(t *testing.T) {
	store := &journeyIntentStore{
		deckJourneyActionStore: &deckJourneyActionStore{},
		detail:                 domain.MyBook{Book: domain.Book{ID: "book-1", OwnerID: "owner-1", Title: "Unavailable book"}},
	}
	h := &Handler{services: Services{
		Store:         StoreDependencies{Books: store, Goals: store},
		Analysis:      &journeyIntentAnalysis{},
		CatalogueSync: journeyIntentCatalogue{err: cataloguesync.ErrNotFound},
	}}

	action, err := h.addBookToReadingJourney(context.Background(), "owner-1", "", "book-1")
	require.NoError(t, err)
	assert.Equal(t, deckIsToRead, action.State)
	assert.True(t, strings.Contains(action.Error, "To Read status is retained"))
	assert.Equal(t, domain.BookDispositionToRead, store.detail.Disposition)
}

func TestMovingMetadataOnlyBookToReadAcquiresAndSubmitsAnalysis(t *testing.T) {
	store := &journeyIntentStore{
		deckJourneyActionStore: &deckJourneyActionStore{},
		detail:                 domain.MyBook{Book: domain.Book{ID: "book-1", OwnerID: "owner-1", Title: "Metadata book"}},
	}
	analysisService := &journeyIntentAnalysis{}
	acquired := false
	h := &Handler{services: Services{
		Store:         StoreDependencies{Books: store, Goals: store},
		Analysis:      analysisService,
		CatalogueSync: journeyIntentCatalogue{target: cataloguesync.AcquisitionTarget{ConnectionID: "catalogue-1", Language: "de"}},
		OPDS: journeyIntentOPDS{acquire: func() {
			acquired = true
			store.detail.Acquired = &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-1", MediaType: opds.EPUBMediaType}}
		}},
	}}

	action, err := h.addBookToReadingJourney(context.Background(), "owner-1", "", "book-1")
	require.NoError(t, err)
	assert.Equal(t, deckIsToRead, action.State)
	assert.True(t, acquired)
	assert.Equal(t, 1, analysisService.calls)
	assert.True(t, strings.Contains(action.Message, "Analysis job #1"))
}
