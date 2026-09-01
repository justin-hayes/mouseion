package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type deckJourneyActionStore struct {
	Store
	journey domain.ReadingJourney
	goal    domain.PrimaryGoal
	addErr  error
	adds    int
}

func (s *deckJourneyActionStore) GetReadingJourney(context.Context, string) (domain.ReadingJourney, error) {
	journey := s.journey
	journey.Entries = append([]domain.ReadingJourneyEntry(nil), s.journey.Entries...)
	return journey, nil
}

func (s *deckJourneyActionStore) GetPrimaryGoal(context.Context, string) (domain.PrimaryGoal, error) {
	return s.goal, nil
}

func (s *deckJourneyActionStore) AddToReadingJourney(_ context.Context, _ string, bookID string, expectedRevision int64) (int64, error) {
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
