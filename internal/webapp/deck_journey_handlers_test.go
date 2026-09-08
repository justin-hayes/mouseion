package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type deckJourneyActionStore struct {
	Store
	journey                domain.ReadingJourney
	goal                   domain.PrimaryGoal
	addErr                 error
	adds                   int
	bookIDBySourceMaterial map[string]string
	noBookIdentity         map[string]bool
}

func (s *deckJourneyActionStore) GetReadingJourney(context.Context, string, string) (domain.ReadingJourney, error) {
	journey := s.journey
	journey.Entries = append([]domain.ReadingJourneyEntry(nil), s.journey.Entries...)
	return journey, nil
}

// ResolveJourneyBookID maps a deck-journey action identity to the canonical
// book id; tests drive the mapping table to exercise source-material ids that
// differ from book ids. Without a mapping the id is already the book id.
func (s *deckJourneyActionStore) ResolveJourneyBookID(_ context.Context, _ string, id string) (string, bool, error) {
	if s.noBookIdentity[id] {
		return "", false, nil
	}
	if resolved, ok := s.bookIDBySourceMaterial[id]; ok {
		return resolved, true, nil
	}
	return id, true, nil
}

func (s *deckJourneyActionStore) GetPrimaryGoal(context.Context, string) (domain.PrimaryGoal, error) {
	return s.goal, nil
}

func (s *deckJourneyActionStore) AddToReadingJourney(_ context.Context, _ string, _ string, bookID string, expectedRevision int64) (int64, error) {
	s.adds++
	if s.addErr != nil {
		return 0, s.addErr
	}
	if expectedRevision != s.journey.Revision {
		return 0, persistence.ErrJourneyStale
	}
	for _, entry := range s.journey.Entries {
		if entry.BookID == bookID {
			return s.journey.Revision, nil
		}
	}
	s.journey.Entries = append(s.journey.Entries, domain.ReadingJourneyEntry{BookID: bookID, Position: len(s.journey.Entries) + 1})
	s.journey.Revision++
	return s.journey.Revision, nil
}

func TestAddBookToReadingJourneyHandlesIdempotentStaleAndErrorStates(t *testing.T) {
	tests := []struct {
		name      string
		store     deckJourneyActionStore
		wantState deckJourneyState
		wantText  string
		wantRev   int64
		wantAdds  int
		wantErr   bool
	}{
		{
			name:      "idempotent existing member",
			store:     deckJourneyActionStore{journey: domain.ReadingJourney{Revision: 4, Entries: []domain.ReadingJourneyEntry{{BookID: "book-1", Position: 1}}}},
			wantState: deckJourneyMember, wantText: "already in your Reading Journey", wantRev: 4, wantAdds: 1,
		},
		{
			name:      "stale refetch",
			store:     deckJourneyActionStore{journey: domain.ReadingJourney{Revision: 4}, addErr: persistence.ErrJourneyStale},
			wantState: deckJourneyNotMember, wantText: "This Journey changed since this page was loaded", wantRev: 4, wantAdds: 1,
		},
		{
			name:      "ordinary error",
			store:     deckJourneyActionStore{journey: domain.ReadingJourney{Revision: 4}, addErr: errors.New("write failed")},
			wantState: deckJourneyNotMember, wantText: "No Journey changes were made", wantRev: 4, wantAdds: 1, wantErr: true,
		},
		{
			name:      "language required",
			store:     deckJourneyActionStore{journey: domain.ReadingJourney{Revision: 4}, addErr: persistence.ErrBookLanguageRequired},
			wantState: deckJourneyNotMember, wantText: "Fix the language in the catalogue", wantRev: 4, wantAdds: 1, wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := &Handler{services: Services{Store: &test.store}}
			action, err := h.addBookToReadingJourney(context.Background(), "owner-1", "prep-1", "book-1", 4)
			if err != nil {
				t.Fatal(err)
			}
			if action.State != test.wantState || action.Revision != test.wantRev || test.store.adds != test.wantAdds {
				t.Fatalf("action=%+v adds=%d, want state=%s revision=%d adds=%d", action, test.store.adds, test.wantState, test.wantRev, test.wantAdds)
			}
			if !strings.Contains(action.Message+action.Error, test.wantText) {
				t.Fatalf("action outcome=%+v missing %q", action, test.wantText)
			}
			if test.wantErr != (action.Error != "") {
				t.Fatalf("action error=%q, want error=%t", action.Error, test.wantErr)
			}
		})
	}
}

func TestAddDeckBookToJourneyRouteRendersConflictAndKeepsRetryForm(t *testing.T) {
	store := &deckJourneyActionStore{journey: domain.ReadingJourney{Revision: 9}, addErr: persistence.ErrJourneyStale}
	h := &Handler{services: Services{Store: store, SessionLifetime: 0}}
	csrf := strings.Repeat("c", 32)
	form := url.Values{"csrf_token": {csrf}, "expected_revision": {"8"}, "deck_preparation_id": {"prep-1"}}
	r := httptest.NewRequest(http.MethodPost, "/journey/books/book-1/add", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("HX-Request", "true")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: csrf})
	r.SetPathValue("id", "book-1")
	recorder := httptest.NewRecorder()
	h.addDeckBookToJourney(recorder, r)
	if recorder.Code != http.StatusOK {
		t.Fatalf("route status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{"role=\"alert\"", "This Journey changed since this page was loaded", `name="expected_revision" value="9"`, "Add to Reading Journey"} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Errorf("conflict response missing %q: %s", want, recorder.Body.String())
		}
	}
}

func TestAddBookToReadingJourneyDoesNotMutatePrimaryGoal(t *testing.T) {
	store := &deckJourneyActionStore{journey: domain.ReadingJourney{Revision: 2}, goal: domain.PrimaryGoal{BookID: "book-1"}}
	h := &Handler{services: Services{Store: store}}
	action, err := h.addBookToReadingJourney(context.Background(), "owner-1", "prep-1", "book-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if action.State != deckJourneyGoal || store.adds != 0 {
		t.Fatalf("goal action=%+v add calls=%d", action, store.adds)
	}
}

func TestDeckJourneyActionResolvesSourceMaterialToBook(t *testing.T) {
	store := &deckJourneyActionStore{
		journey:                domain.ReadingJourney{Revision: 4, Entries: []domain.ReadingJourneyEntry{{BookID: "book-x", Position: 1}}},
		bookIDBySourceMaterial: map[string]string{"source-1": "book-x"},
	}
	h := &Handler{services: Services{Store: store}}
	action, err := h.deckJourneyAction(context.Background(), "owner-1", "prep-1", "source-1")
	if err != nil {
		t.Fatal(err)
	}
	if action.State != deckJourneyMember || action.BookID != "book-x" || action.Revision != 4 {
		t.Fatalf("resolved member action=%+v", action)
	}
}

func TestAddBookToReadingJourneyUsesResolvedBookIdentity(t *testing.T) {
	store := &deckJourneyActionStore{
		journey:                domain.ReadingJourney{Revision: 3},
		bookIDBySourceMaterial: map[string]string{"source-1": "book-1"},
	}
	h := &Handler{services: Services{Store: store}}
	action, err := h.addBookToReadingJourney(context.Background(), "owner-1", "prep-1", "source-1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if action.State != deckJourneyMember || action.BookID != "book-1" || store.adds != 1 {
		t.Fatalf("resolved add action=%+v add calls=%d", action, store.adds)
	}
	if len(store.journey.Entries) != 1 || store.journey.Entries[0].BookID != "book-1" {
		t.Fatalf("resolved add journey entries=%+v", store.journey.Entries)
	}
}

func TestSourceMaterialWithoutBookIdentityOffersNoJourneyAction(t *testing.T) {
	store := &deckJourneyActionStore{
		journey:        domain.ReadingJourney{Revision: 4},
		noBookIdentity: map[string]bool{"source-orphan": true},
	}
	h := &Handler{services: Services{Store: store}}
	action, err := h.deckJourneyAction(context.Background(), "owner-1", "prep-1", "source-orphan")
	if err != nil {
		t.Fatal(err)
	}
	if action.State != deckJourneyUnknown || action.BookID != "" {
		t.Fatalf("unresolvable action=%+v", action)
	}
	action, err = h.addBookToReadingJourney(context.Background(), "owner-1", "prep-1", "source-orphan", 4)
	if err != nil {
		t.Fatal(err)
	}
	if action.State != deckJourneyUnknown || store.adds != 0 {
		t.Fatalf("unresolvable add action=%+v add calls=%d", action, store.adds)
	}
}

type journeyIntentStore struct {
	*deckJourneyActionStore
	detail domain.MyBook
}

func (s *journeyIntentStore) GetBookDetail(context.Context, string, string) (domain.MyBook, error) {
	return s.detail, nil
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
func (journeyIntentAnalysis) Get(context.Context, string, int64) (analysis.Status, error) {
	return analysis.Status{}, nil
}

func TestAddingJourneyMemberEnsuresAcquisitionAndAnalysisOnce(t *testing.T) {
	store := &journeyIntentStore{
		deckJourneyActionStore: &deckJourneyActionStore{journey: domain.ReadingJourney{Revision: 2}},
		detail:                 domain.MyBook{Book: domain.Book{ID: "book-1", OwnerID: "owner-1", Title: "Book one"}},
	}
	analysisService := &journeyIntentAnalysis{}
	store.detail.Acquired = &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-1", MediaType: opds.EPUBMediaType}}
	h := &Handler{services: Services{
		Store:         store,
		Analysis:      analysisService,
		CatalogueSync: journeyIntentCatalogue{},
	}}

	first, err := h.addBookToReadingJourney(context.Background(), "owner-1", "", "book-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != deckJourneyMember || analysisService.calls != 1 {
		t.Fatalf("first add=%+v analysis calls=%d", first, analysisService.calls)
	}

	second, err := h.addBookToReadingJourney(context.Background(), "owner-1", "", "book-1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if second.State != deckJourneyMember || analysisService.calls != 1 || !strings.Contains(second.Message, "already in your Reading Journey") {
		t.Fatalf("re-add=%+v analysis calls=%d", second, analysisService.calls)
	}
}

func TestAddingMetadataOnlyBookRetainsJourneyMembershipWhenAcquisitionUnavailable(t *testing.T) {
	store := &journeyIntentStore{
		deckJourneyActionStore: &deckJourneyActionStore{journey: domain.ReadingJourney{Revision: 1}},
		detail:                 domain.MyBook{Book: domain.Book{ID: "book-1", OwnerID: "owner-1", Title: "Unavailable book"}},
	}
	h := &Handler{services: Services{
		Store:         store,
		Analysis:      &journeyIntentAnalysis{},
		CatalogueSync: journeyIntentCatalogue{err: cataloguesync.ErrNotFound},
	}}

	action, err := h.addBookToReadingJourney(context.Background(), "owner-1", "", "book-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if action.State != deckJourneyMember || !strings.Contains(action.Error, "Journey entry is retained") || len(store.journey.Entries) != 1 {
		t.Fatalf("unavailable acquisition action=%+v journey=%+v", action, store.journey)
	}
}

func TestAddingMetadataOnlyBookAcquiresAndSubmitsAnalysis(t *testing.T) {
	store := &journeyIntentStore{
		deckJourneyActionStore: &deckJourneyActionStore{journey: domain.ReadingJourney{Revision: 1}},
		detail:                 domain.MyBook{Book: domain.Book{ID: "book-1", OwnerID: "owner-1", Title: "Metadata book"}},
	}
	analysisService := &journeyIntentAnalysis{}
	acquired := false
	h := &Handler{services: Services{
		Store:         store,
		Analysis:      analysisService,
		CatalogueSync: journeyIntentCatalogue{target: cataloguesync.AcquisitionTarget{ConnectionID: "catalogue-1", Language: "de"}},
		OPDS: journeyIntentOPDS{acquire: func() {
			acquired = true
			store.detail.Acquired = &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-1", MediaType: opds.EPUBMediaType}}
		}},
	}}

	action, err := h.addBookToReadingJourney(context.Background(), "owner-1", "", "book-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if action.State != deckJourneyMember || !acquired || analysisService.calls != 1 || !strings.Contains(action.Message, "Analysis job #1") {
		t.Fatalf("metadata add action=%+v acquired=%t analysis calls=%d", action, acquired, analysisService.calls)
	}
}
