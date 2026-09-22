package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func TestJourneyEntryRedirectsCompletedMemberUsingBookLanguage(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))
	handler := requireHandler(t, h)
	handler.services.AnalysisInsights = fixtures.Insights{JourneyStore: store}

	response := journeyEntryRequest(t, h, "/journey/fixture-route-match", cookies)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/journey?language_handoff_book=fixture-route-match&language_handoff_language=de", response.Header().Get("Location"))
	assert.NotContains(t, response.Body.String(), "<h1>")

	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "de"))
	response = journeyEntryRequest(t, h, "/journey/fixture-book", cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/journey#journey-book-fixture-book", response.Header().Get("Location"))
}

func TestJourneyPageRendersValidatedCrossLanguageHandoff(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))
	handler := requireHandler(t, h)
	handler.services.AnalysisInsights = fixtures.Insights{JourneyStore: store}

	response := journeyEntryRequest(t, h, "/journey/fixture-route-match", cookies)
	location := response.Header().Get("Location")
	handoff := journeyEntryRequest(t, h, location, cookies)

	assert.Equal(t, http.StatusOK, handoff.Code)
	assert.Contains(t, handoff.Body.String(), "This Book is in German")
	assert.Contains(t, handoff.Body.String(), `name="language" value="de"`)
	assert.Contains(t, handoff.Body.String(), `name="return_to" value="/journey#journey-book-fixture-route-match"`)
	assert.NotContains(t, handoff.Body.String(), `id="journey-book-fixture-route-match"`)
}

func TestJourneyEntryRemovalUsesTheEntryBookLanguage(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))

	response := goalRequest(t, h, "/journey/books/fixture-route-match/remove", url.Values{
		"csrf_token": {csrf}, "expected_revision": {"1"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	journey, err := store.GetReadingJourney(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	for _, entry := range journey.Entries {
		assert.NotEqual(t, "fixture-route-match", entry.BookID, "German Journey still contains removed entry: %+v", journey.Entries)
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
		"unavailable": {
			AnalysisStatus: "content unavailable",
			AnalysisState:  "unavailable",
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
			handler := requireHandler(t, h)
			handler.services.Store = storeDependencies(store)

			response := journeyEntryRequest(t, h, "/journey/"+bookID, cookies)
			assert.Equal(t, http.StatusNotFound, response.Code)
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
			requireHandler(t, h).services.Store = storeDependencies(store)

			response := journeyEntryRequest(t, h, "/journey/"+test.bookID, cookies)
			assert.Equal(t, http.StatusNotFound, response.Code)
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
	requireHandler(t, h).services.Store = storeDependencies(store)

	response := journeyEntryRequest(t, h, "/books/"+bookID+"/analyses/run-"+bookID, cookies)
	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestAnalysisCompatibilityRouteRedirectsToReadingJourneyAnchor(t *testing.T) {
	h, cookies, _, _ := goalFixtureSession(t)
	response := journeyEntryRequest(t, h, "/books/fixture-book/analyses/fixture-run", cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/journey#journey-book-fixture-book", response.Header().Get("Location"))
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
	handler := requireHandler(t, h)
	handler.services.Store = storeDependencies(store)
	handler.services.Analysis = analysisService

	response := goalRequest(t, h, "/journey/books/stale-book/reanalyze", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/journey?message=Analysis+job+%231+submitted.", response.Header().Get("Location"))
	assert.Equal(t, 1, analysisService.calls)
}

func TestReanalyzeJourneyMemberAcceptsUnavailableAcquisition(t *testing.T) {
	h, cookies, csrf, fixtureStore := goalFixtureSession(t)
	bookID := "unavailable-acquisition-book"
	store := &journeyEntryStore{
		Store: fixtureStore,
		detail: domain.MyBook{
			Book: domain.Book{ID: bookID, OwnerID: fixtures.OwnerID, Title: "Unavailable acquisition", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		},
		journey: domain.ReadingJourney{OwnerID: fixtures.OwnerID, Language: "de", Entries: []domain.ReadingJourneyEntry{{BookID: bookID}}},
	}
	handler := requireHandler(t, h)
	handler.services.Store = storeDependencies(store)

	response := goalRequest(t, h, "/journey/books/"+bookID+"/reanalyze", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Contains(t, response.Header().Get("Location"), "Could+not+acquire")
}
