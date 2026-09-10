package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type journeyEntryStore struct {
	*fixtures.Store
	detail  domain.MyBook
	journey domain.ReadingJourney
	missing bool
}

func (s *journeyEntryStore) GetBookDetail(_ context.Context, owner, id string) (domain.MyBook, error) {
	if s.missing || owner != s.detail.Book.OwnerID || id != s.detail.Book.ID {
		return domain.MyBook{}, persistence.ErrNotFound
	}
	return s.detail, nil
}

func (s *journeyEntryStore) GetReadingJourney(context.Context, string, string) (domain.ReadingJourney, error) {
	return s.journey, nil
}

func journeyEntryRequest(t *testing.T, h http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func TestJourneyEntryRendersCompletedMemberUsingBookLanguage(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	if err := store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"); err != nil {
		t.Fatal(err)
	}
	handler := h.(*Handler)
	handler.services.AnalysisInsights = fixtures.Insights{JourneyStore: store}

	response := journeyEntryRequest(t, h, "/journey/fixture-route-match", cookies)

	if response.Code != http.StatusOK {
		t.Fatalf("GET Journey entry status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{
		"<h1>Route match: familiar German</h1>",
		"← Reading Journey",
		`data-navigation-context="reading-journey"`,
		"Vocabulary coverage",
		"Vocabulary investment",
		"Prepare deck",
		"In Reading Journey.",
		`action="/journey/books/fixture-route-match/remove"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Journey entry missing %q: %s", want, body)
		}
	}

	response = journeyEntryRequest(t, h, "/journey/fixture-book", cookies)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `action="/journey/books/fixture-book/remove"`) {
		t.Fatalf("Primary Goal Journey entry omitted removal action: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestJourneyEntryRemovalUsesTheEntryBookLanguage(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	if err := store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"); err != nil {
		t.Fatal(err)
	}

	response := goalRequest(t, h, "/journey/books/fixture-route-match/remove", url.Values{
		"csrf_token": {csrf}, "expected_revision": {"1"},
	}, cookies)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("remove Journey entry status=%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	journey, err := store.GetReadingJourney(context.Background(), fixtures.OwnerID, "de")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range journey.Entries {
		if entry.BookID == "fixture-route-match" {
			t.Fatalf("German Journey still contains removed entry: %+v", journey.Entries)
		}
	}
}

func TestJourneyEntryRequiresCurrentCompletedMemberAnalysis(t *testing.T) {
	states := map[string]domain.SourceMaterialSummary{
		"queued": {
			AnalysisStatus: "analysis queued",
			AnalysisState:  "queued",
			AnalysisJobID:  1,
		},
		"running": {
			AnalysisStatus: "analyzing",
			AnalysisState:  "running",
			AnalysisJobID:  2,
		},
		"failed": {
			AnalysisStatus: "analysis failed",
			AnalysisState:  "failed",
			AnalysisJobID:  3,
		},
		"stale": {
			AnalysisStatus: "stale",
			AnalysisState:  "stale",
			AnalysisRunID:  "old-run",
			CorpusID:       "old-corpus",
		},
	}

	for name, acquired := range states {
		t.Run(name, func(t *testing.T) {
			h, cookies, _, fixtureStore := goalFixtureSession(t)
			bookID := "journey-entry-" + name
			acquired.BookID = bookID
			acquired.Source = domain.SourceMaterial{ID: "source-" + bookID, OwnerID: fixtures.OwnerID, Language: "de", MediaType: "application/epub+zip", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"}
			store := &journeyEntryStore{
				Store: fixtureStore,
				detail: domain.MyBook{
					Book:     domain.Book{ID: bookID, OwnerID: fixtures.OwnerID, Title: "Journey entry " + name, LanguageState: domain.LanguageChosen, LanguageTag: "de"},
					Acquired: &acquired,
				},
				journey: domain.ReadingJourney{OwnerID: fixtures.OwnerID, Language: "de", Revision: 4, Entries: []domain.ReadingJourneyEntry{{BookID: bookID}}},
			}
			handler := h.(*Handler)
			handler.services.Store = store

			response := journeyEntryRequest(t, h, "/journey/"+bookID, cookies)
			if response.Code != http.StatusNotFound {
				t.Fatalf("GET Journey entry status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestJourneyEntryRequiresMembershipAndOwnerScopedBook(t *testing.T) {
	tests := []struct {
		name    string
		bookID  string
		owner   string
		journey domain.ReadingJourney
		missing bool
	}{
		{name: "non-member", bookID: "journey-entry-not-member", owner: fixtures.OwnerID, journey: domain.ReadingJourney{Language: "de"}},
		{name: "unknown-book", bookID: "journey-entry-unknown", owner: fixtures.OwnerID, missing: true, journey: domain.ReadingJourney{Language: "de", Entries: []domain.ReadingJourneyEntry{{BookID: "journey-entry-unknown"}}}},
		{name: "foreign-book", bookID: "journey-entry-foreign", owner: "other-owner", journey: domain.ReadingJourney{Language: "de", Entries: []domain.ReadingJourneyEntry{{BookID: "journey-entry-foreign"}}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h, cookies, _, fixtureStore := goalFixtureSession(t)
			store := &journeyEntryStore{
				Store: fixtureStore,
				detail: domain.MyBook{
					Book: domain.Book{ID: test.bookID, OwnerID: test.owner, Title: "Unavailable entry", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
					Acquired: &domain.SourceMaterialSummary{
						Source:         domain.SourceMaterial{ID: "source-" + test.bookID, OwnerID: fixtures.OwnerID, Language: "de", MediaType: "application/epub+zip"},
						AnalysisStatus: "analyzed",
						AnalysisState:  "completed",
						AnalysisRunID:  "run-" + test.bookID,
						CorpusID:       "corpus-" + test.bookID,
					},
				},
				journey: test.journey,
				missing: test.missing,
			}
			h.(*Handler).services.Store = store

			response := journeyEntryRequest(t, h, "/journey/"+test.bookID, cookies)
			if response.Code != http.StatusNotFound {
				t.Fatalf("GET Journey entry status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAnalysisCompatibilityRouteRequiresJourneyMembership(t *testing.T) {
	h, cookies, _, fixtureStore := goalFixtureSession(t)
	bookID := "completed-outside-journey"
	store := &journeyEntryStore{
		Store: fixtureStore,
		detail: domain.MyBook{
			Book: domain.Book{ID: bookID, OwnerID: fixtures.OwnerID, Title: "Completed outside Journey", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
			Acquired: &domain.SourceMaterialSummary{
				BookID:         bookID,
				Source:         domain.SourceMaterial{ID: "source-" + bookID, OwnerID: fixtures.OwnerID, Language: "de", MediaType: "application/epub+zip"},
				AnalysisStatus: "analyzed",
				AnalysisState:  "completed",
				AnalysisRunID:  "run-" + bookID,
				CorpusID:       "corpus-" + bookID,
			},
		},
		journey: domain.ReadingJourney{OwnerID: fixtures.OwnerID, Language: "de"},
	}
	h.(*Handler).services.Store = store

	response := journeyEntryRequest(t, h, "/books/"+bookID+"/analyses/run-"+bookID, cookies)
	if response.Code != http.StatusNotFound {
		t.Fatalf("non-member analysis compatibility status=%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
}

func TestReanalyzeJourneyMemberUsesSharedAnalysisTrigger(t *testing.T) {
	h, cookies, csrf, fixtureStore := goalFixtureSession(t)
	analysisService := &journeyIntentAnalysis{}
	store := &journeyEntryStore{
		Store: fixtureStore,
		detail: domain.MyBook{
			Book: domain.Book{ID: "stale-book", OwnerID: fixtures.OwnerID, Title: "Stale book", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
			Acquired: &domain.SourceMaterialSummary{
				Source:         domain.SourceMaterial{ID: "stale-source", OwnerID: fixtures.OwnerID, Language: "de", MediaType: "application/epub+zip", ContentRevisionID: "current-revision", ContentSnapshotID: "current-snapshot"},
				BookID:         "stale-book",
				AnalysisStatus: "stale",
				AnalysisState:  "stale",
				AnalysisRunID:  "old-run",
				CorpusID:       "old-corpus",
			},
		},
		journey: domain.ReadingJourney{OwnerID: fixtures.OwnerID, Language: "de", Entries: []domain.ReadingJourneyEntry{{BookID: "stale-book"}}},
	}
	handler := h.(*Handler)
	handler.services.Store = store
	handler.services.Analysis = analysisService

	response := goalRequest(t, h, "/journey/books/stale-book/reanalyze", url.Values{"csrf_token": {csrf}}, cookies)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/journey?message=Analysis+job+%231+submitted." {
		t.Fatalf("re-analyze response status=%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	if analysisService.calls != 1 {
		t.Fatalf("re-analyze submitted %d analysis jobs, want one", analysisService.calls)
	}
}
