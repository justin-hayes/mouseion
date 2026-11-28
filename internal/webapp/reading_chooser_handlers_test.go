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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type readingChooserBooks struct {
	*fixtures.Store
	books []domain.MyBook
}

func (b readingChooserBooks) ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error) {
	return b.books, nil
}

// stubbedMyBooksBrowse is the browse page a stubbed store serves: the given
// Books, filtered to the requested language and disposition.
func stubbedMyBooksBrowse(books []domain.MyBook, language, disposition string) domain.MyBooksBrowseResult {
	var items []domain.MyBook
	toRead := 0
	for _, book := range books {
		if language != "" && book.Book.LanguageTag != language {
			continue
		}
		if book.Disposition == domain.BookDispositionToRead {
			toRead++
		}
		if disposition == "" || string(book.Disposition) == disposition {
			items = append(items, book)
		}
	}
	return domain.MyBooksBrowseResult{
		Items: items, Total: len(items), ScopeTotal: len(items), AllCount: len(books),
		DispositionCounts: []domain.DispositionCount{{Disposition: domain.BookDispositionToRead, Count: toRead}},
	}
}

func (b readingChooserBooks) ListMyBooksBrowse(_ context.Context, _, _, language, disposition string, _ bool, _, _ int) (domain.MyBooksBrowseResult, error) {
	return stubbedMyBooksBrowse(b.books, language, disposition), nil
}

type readingChooserInsights struct{ fixtures.Insights }

func (i readingChooserInsights) Coverage(ctx context.Context, owner, corpusID string) (domain.AnalysisCoverage, error) {
	if corpusID == "fixture-route-tie-a-corpus" {
		return domain.AnalysisCoverage{}, nil
	}
	return i.Insights.Coverage(ctx, owner, corpusID)
}

// endGermanFixtureReading ends the fixture's German current reading through the
// exact-commitment End path, reading its snapshot identity first.
func endGermanFixtureReading(t *testing.T, store *fixtures.Store) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, store.EndCurrentReading(ctx, fixtures.OwnerID, "de"))
}

func TestMyBooksOffersReadNextOnlyOnAnalysedToReadBooks(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	books, err := store.ListMyBooksWithEvidence(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	for i := range books {
		if books[i].Acquired != nil && books[i].Acquired.Source.ID == "fixture-route-differs" {
			books[i].Acquired.Signals = testStale
		}
	}
	handler.services.Store = storeDependencies(readingChooserBooks{Store: store, books: books})

	response := getAs(t, h, "/de/library?disposition=to_read", cookies)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "Read next")
	assert.NotContains(t, body, "Confirm start reading")
	assert.True(t, strings.Contains(body, `action="/reading/books/fixture-route-match/start"`) && strings.Contains(body, `name="csrf_token"`))
	assert.NotContains(t, body, `action="/reading/books/fixture-route-differs/start"`, "a stale analysis offers no Read next")
	assert.NotContains(t, body, "reading-chooser", "the chooser page is gone")
}

func TestReanalyzeReportsAnUnclassifiedFailureAsAnError(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	requireHandler(t, h).services.Analysis = nil

	response := readingTestRequest(t, h, "/reading/books/fixture-route-match/reanalyze", url.Values{"csrf_token": {csrf}}, cookies)

	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Contains(t, response.Header().Get("Location"), "/de/library?error="+url.QueryEscape(analysisCouldNotStartMessage))
}

func TestReadNextReturnsToMyBooks(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	response := readingTestRequest(t, h, "/reading/books/fixture-route-match/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Contains(t, response.Header().Get("Location"), "/de/library?message=")
	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, "fixture-route-match", current.BookID)
	retry := readingTestRequest(t, h, "/reading/books/fixture-route-match/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, retry.Code)
	retried, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, current, retried, "a retry leaves the reading unchanged")
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/de/library", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	page := httptest.NewRecorder()
	h.ServeHTTP(page, request)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "Route match: familiar German")
	assert.NotContains(t, page.Body.String(), "On arrival")
	assert.NotContains(t, page.Body.String(), "Lower bound")
}

func TestAuthenticatedCurrentReadingCanSwitchAndEnd(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	handler.services.AnalysisInsights = fixtures.Insights{}
	readingRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/de/library", nil)
	for _, cookie := range cookies {
		readingRequest.AddCookie(cookie)
	}
	readingPage := httptest.NewRecorder()
	h.ServeHTTP(readingPage, readingRequest)
	require.Equal(t, http.StatusOK, readingPage.Code)
	assert.Contains(t, readingPage.Body.String(), "End current reading")
	assert.NotContains(t, readingPage.Body.String(), "Set aside this Book")
	assert.NotContains(t, readingPage.Body.String(), "Stop reading for now")
	assert.NotContains(t, readingPage.Body.String(), `expected_current_snapshot_id`, "End and Finish take no commitment token")
	assert.NotContains(t, readingPage.Body.String(), `expected_current_book_id`)
	assert.Contains(t, readingPage.Body.String(), `action="/reading/finish"`)
	assert.NotContains(t, readingPage.Body.String(), `action="/goal/finish"`)

	switchPage := getAs(t, h, "/de/reading/switch", cookies)
	require.Equal(t, http.StatusSeeOther, switchPage.Code, "Switch is no longer a page")
	assert.Equal(t, "/de/library", switchPage.Header().Get("Location"))
	toRead := getAs(t, h, "/de/library?disposition=to_read", cookies)
	assert.Contains(t, toRead.Body.String(), `action="/reading/books/fixture-route-match/start"`, "Switch current reading is Read next")
	switched := readingTestRequest(t, h, "/reading/books/fixture-route-match/switch", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, switched.Code)
	assert.Contains(t, switched.Header().Get("Location"), "is+now+your+current+reading")
	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, "fixture-route-match", current.BookID)
	retriedSwitch := readingTestRequest(t, h, "/reading/books/fixture-route-match/switch", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, retriedSwitch.Code)
	assert.NotContains(t, retriedSwitch.Header().Get("Location"), "error=")
	retried, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, current, retried, "a retry leaves the switched reading unchanged")

	stopped := readingTestRequest(t, h, "/reading/end", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, stopped.Code)
	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive())
	disposition, err := store.GetBookDisposition(context.Background(), fixtures.OwnerID, "fixture-route-match")
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition)

	retriedStop := readingTestRequest(t, h, "/reading/end", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, retriedStop.Code)
	assert.Contains(t, retriedStop.Header().Get("Location"), "error=", "a second End has no reading to end")

	retired := readingTestRequest(t, h, "/reading/set-aside", url.Values{"csrf_token": {csrf}}, cookies)
	assert.GreaterOrEqual(t, retired.Code, http.StatusBadRequest, "retired Set Aside mutation is rejected")

	require.NoError(t, store.SetBookDisposition(context.Background(), fixtures.OwnerID, "fixture-route-match", domain.BookDispositionToRead))
	started := readingTestRequest(t, h, "/reading/books/fixture-route-match/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code)
	finished := readingTestRequest(t, h, "/reading/finish", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusOK, finished.Code)
	assert.Contains(t, finished.Body.String(), "Reading finished")
	assert.Contains(t, finished.Body.String(), `href="/de/library">Choose a To Read book in My Books</a>`)
	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive())
}

func TestLegacyCurrentReadingMutationsAreRejectedWithoutChangingCurrentReading(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	currentBefore, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)

	legacyRequests := []struct {
		path string
		form url.Values
	}{
		{"/goal/books/fixture-route-match", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID}}},
		{"/goal/clear", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID}}},
		{"/goal/finish", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID}}},
	}
	for _, test := range legacyRequests {
		response := readingTestRequest(t, h, test.path, test.form, cookies)
		assert.Equalf(t, http.StatusNotFound, response.Code, "legacy mutation %s should be retired", test.path)
	}

	currentAfter, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, currentBefore, currentAfter)
}

func TestReadingAddressesRedirectToMyBooksKeepingTheStudyLanguage(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)

	unauthenticated := httptest.NewRecorder()
	h.ServeHTTP(unauthenticated, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/de/reading", nil))
	assert.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	for path, want := range map[string]string{
		"/de/reading":                     "/de/library",
		"/it/reading":                     "/it/library",
		"/de/reading/switch":              "/de/library",
		"/de/reading?message=Read+it":     "/de/library?message=Read+it",
		"/it/reading?error=Nothing+moved": "/it/library?error=Nothing+moved",
	} {
		response := getAs(t, h, path, cookies)
		assert.Equal(t, http.StatusSeeOther, response.Code, path)
		assert.Equal(t, want, response.Header().Get("Location"), path)
	}

	unprefixed := getAs(t, h, "/reading", cookies)
	require.Equal(t, http.StatusSeeOther, unprefixed.Code)
	assert.Equal(t, "/de/reading", unprefixed.Header().Get("Location"), "the unprefixed address first takes the language, then redirects again")
}

// chooserEvidenceBooks supplies the chooser's candidates and a read failure that
// the real store cannot produce on demand.
type chooserEvidenceBooks struct {
	*fixtures.Store
	books []domain.MyBook
	err   error
}

func (b chooserEvidenceBooks) ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error) {
	return b.books, b.err
}

// chooserReadyCandidate is an eligible To Read Book whose analysis is published
// and whose corpus is known, so the chooser reads coverage for it.
func chooserReadyCandidate() domain.MyBook {
	return domain.MyBook{
		Book:        domain.Book{ID: "book-1", OwnerID: fixtures.OwnerID, Title: "book-1", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		Disposition: domain.BookDispositionToRead,
		Acquired: &domain.SourceMaterialSummary{
			Source:        domain.SourceMaterial{MediaType: "application/epub+zip", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
			AnalysisRunID: "run", CorpusID: "corpus-book-1", Signals: testAnalyzed,
		},
	}
}

func (b chooserEvidenceBooks) ListMyBooksBrowse(_ context.Context, _, _, language, disposition string, _ bool, _, _ int) (domain.MyBooksBrowseResult, error) {
	return stubbedMyBooksBrowse(b.books, language, disposition), b.err
}
