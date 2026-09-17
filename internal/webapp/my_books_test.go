package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMyBooksMetadataOnlyRowExposesOnlySupportedActions(t *testing.T) {
	book := domain.MyBook{Book: domain.Book{ID: "metadata-book", OwnerID: "owner", Title: "A book without an EPUB", LanguageState: domain.LanguageUnknown}}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, MyBooksBrowseState{RefreshableBookIDs: map[string]bool{"metadata-book": true}}).Render(context.Background(), &output))
	html := output.String()
	if main := strings.Index(html, "<main"); main >= 0 {
		html = html[main:]
	}
	for _, want := range []string{"A book without an EPUB", "Not acquired / metadata only", "Refresh metadata", `hx-post="/library/books/metadata-book/refresh"`, `hx-target="#book-row-metadata-book"`, "Add to Reading Journey", `action="/journey/books/metadata-book/add"`, `name="expected_revision" value="0"`, "Remove from My Books", `action="/library/books/metadata-book/remove"`} {
		assert.True(t, strings.Contains(html, want), "metadata-only My Books row missing %q: %s", want, html)
	}
	assert.False(t, strings.Contains(html, `href="/books/metadata-book"`), "metadata-only My Books row linked to the retired Book detail page: %s", html)
	row := html
	if start := strings.Index(row, `aria-labelledby="book-title-metadata-book"`); start >= 0 {
		row = row[start:]
		if end := strings.Index(row, "</article>"); end >= 0 {
			row = row[:end+len("</article>")]
		}
	}
	for _, forbidden := range []string{"Review scope", "Prepare deck", "View analysis result", "Start analysis", `action="/books/metadata-book/analyze"`, "coverage"} {
		assert.False(t, strings.Contains(row, forbidden), "metadata-only My Books row exposed unsupported action %q: %s", forbidden, row)
	}
}

func TestMyBooksMetadataOnlyRowHidesRefreshWhenIneligible(t *testing.T) {
	book := domain.MyBook{Book: domain.Book{ID: "metadata-book", OwnerID: "owner", Title: "A book without an EPUB"}}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output))
	assert.False(t, strings.Contains(output.String(), "Refresh metadata"), "ineligible metadata-only My Books row exposed refresh: %s", output.String())
}

func TestMyBooksJourneyActionHidesAddForExistingMember(t *testing.T) {
	book := domain.MyBook{
		Book:            domain.Book{ID: "journey-book", OwnerID: "owner", Title: "Journey book"},
		JourneyMember:   true,
		JourneyRevision: 7,
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.False(t, strings.Contains(html, `action="/journey/books/journey-book/add"`) || strings.Contains(html, "Add to Reading Journey"), "existing Journey member still exposed add action: %s", html)
	assert.True(t, strings.Contains(html, `href="/journey#journey-book-journey-book"`), "existing Journey member omitted Journey link: %s", html)
}

func TestMyBooksJourneyActionHidesAddForPrimaryGoal(t *testing.T) {
	book := domain.MyBook{
		Book:        domain.Book{ID: "goal-book", OwnerID: "owner", Title: "Goal book"},
		JourneyGoal: true,
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.False(t, strings.Contains(html, `action="/journey/books/goal-book/add"`) || strings.Contains(html, "Add to Reading Journey"), "Primary Goal still exposed add action: %s", html)
	assert.True(t, strings.Contains(html, "Primary Goal") && strings.Contains(html, `href="/journey#journey-book-goal-book"`), "Primary Goal omitted Journey link: %s", html)
}

func TestAnalyzedMyBookShowsCurrentResultWithoutDuplicateStartAction(t *testing.T) {
	book := domain.MyBook{
		Book:          domain.Book{ID: "analyzed-book", OwnerID: "owner", Title: "Analyzed book"},
		JourneyMember: true,
		Acquired: &domain.SourceMaterialSummary{
			Source:         domain.SourceMaterial{ID: "source-analyzed-book", MediaType: "application/epub+zip", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
			AnalysisStatus: "analyzed",
			AnalysisState:  "completed",
			AnalysisRunID:  "run-analyzed-book",
			CorpusID:       "corpus-analyzed-book",
		},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.True(t, strings.Contains(html, `href="/journey/analyzed-book"`) && !strings.Contains(html, `href="/books/analyzed-book"`) && !strings.Contains(html, "View analysis result") && !strings.Contains(html, `action="/books/analyzed-book/analyze"`), "analyzed Journey member exposed an invalid My Books action or link: %s", html)
}

func TestAnalyzedNonJourneyMyBookKeepsEvidenceWithoutLink(t *testing.T) {
	book := domain.MyBook{
		Book: domain.Book{ID: "analyzed-outside-journey", OwnerID: "owner", Title: "Analyzed outside Journey"},
		Acquired: &domain.SourceMaterialSummary{
			Source:         domain.SourceMaterial{ID: "source-analyzed-outside-journey", MediaType: "application/epub+zip", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
			AnalysisStatus: "analyzed",
			AnalysisState:  "completed",
			AnalysisRunID:  "run-analyzed-outside-journey",
			CorpusID:       "corpus-analyzed-outside-journey",
		},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.True(t, strings.Contains(html, "Analyzed") && !strings.Contains(html, `href="/journey/analyzed-outside-journey"`) && !strings.Contains(html, `href="/books/`) && !strings.Contains(html, "View analysis result"), "analyzed non-member My Books row exposed an invalid link: %s", html)
}

func TestMyBooksRowRendersCanonicalBookTitle(t *testing.T) {
	book := domain.MyBook{
		Book: domain.Book{ID: "canonical-book", OwnerID: "owner", Title: "Refreshed catalogue title"},
		Acquired: &domain.SourceMaterialSummary{
			Source: domain.SourceMaterial{ID: "source-canonical-book", Title: "Acquisition-internal title"},
		},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.True(t, strings.Contains(html, "Refreshed catalogue title"), "My Books omitted canonical Book title: %s", html)
	assert.False(t, strings.Contains(html, "Acquisition-internal title"), "My Books rendered acquisition-internal title: %s", html)
}

func TestMyBooksEvidenceStatesRemainDistinct(t *testing.T) {
	states := []domain.BookEvidenceState{
		domain.BookUnavailable,
		domain.BookNotAcquired,
		domain.BookAcquiredUnassessed,
		domain.BookAnalyzed,
		domain.BookStale,
	}
	books := make([]domain.MyBook, 0, len(states))
	for i, state := range states {
		book := domain.MyBook{Book: domain.Book{ID: "book-" + string(rune('a'+i)), OwnerID: "owner", Title: "Book " + string(rune('A'+i)), LanguageState: domain.LanguageChosen, LanguageTag: "de"}}
		if state != domain.BookNotAcquired {
			status := "not analyzed"
			revision := "revision"
			if state == domain.BookUnavailable {
				revision = ""
			}
			if state == domain.BookAnalyzed {
				status = "analyzed"
			}
			if state == domain.BookStale {
				status = "stale"
			}
			book.Acquired = &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-" + book.Book.ID, OwnerID: "owner", Title: book.Book.Title, Language: "de", MediaType: "application/epub+zip", ContentRevisionID: revision, ContentSnapshotID: "snapshot"}, BookID: book.Book.ID, AnalysisStatus: status}
		}
		books = append(books, book)
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	for label, count := range map[string]int{"Unavailable": 1, "Not acquired": 2, "Ready to analyze": 1, "Analysis not started": 2, "Analyzed": 1, "Stale analysis": 1} {
		assert.Equal(t, count, strings.Count(html, label), label)
	}
	assert.False(t, strings.Contains(html, `href="/books/`), "learner My Books linked to the retired Book detail page: %s", html)
	assert.False(t, strings.Contains(html, "Start analysis") || strings.Contains(html, `/analyze`), "My Books rows exposed an explicit analysis action: %s", html)
}

func TestMyBooksEmptyOnboardingGuidesConnectionLanguageAndSync(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{"Set up a catalog connection", "Mouseion needs a catalog connection owned by your learner account", `href="/catalogs">Set up a catalog</a>`} {
		assert.True(t, strings.Contains(html, want), "empty My Books onboarding missing %q: %s", want, html)
	}
	assert.False(t, strings.Contains(html, "Add books") || strings.Contains(html, "Add a book"), "empty My Books onboarding exposed retired acquisition wording: %s", html)

	output.Reset()
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", true, MyBooksBrowseState{}).Render(context.Background(), &output))
	assert.True(t, strings.Contains(output.String(), `href="/catalogs">Sync catalog</a>`), "connected empty state omitted sync guidance: %s", output.String())
	assert.False(t, strings.Contains(output.String(), "Add books") || strings.Contains(output.String(), "Add a book"), "connected empty state exposed manual book creation: %s", output.String())
}

func TestUpstreamBrowserRoutesAreRetired(t *testing.T) {
	h, cookies, _, _ := goalFixtureSession(t)
	for _, route := range []string{"/catalog", "/opds/browse", "/opds/language", "/opds/search", "/library/books/book-id"} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, route, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		h.ServeHTTP(response, r)
		assert.Equal(t, http.StatusNotFound, response.Code, route)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/library/books/book-id", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestUnassessedBookDetailAndStandaloneAnalysisRoutesAreRetired(t *testing.T) {
	h, cookies, csrf, _ := goalFixtureSession(t)
	for _, path := range []string{"/books/fixture-empty", "/books/fixture-book", "/books/fixture-failed"} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		assert.Equal(t, http.StatusNotFound, response.Code, path)
	}
	assert.Equal(t, http.StatusNotFound, goalRequest(t, h, "/books/fixture-empty/analyze", url.Values{"csrf_token": {csrf}}, cookies).Code)
	assert.Equal(t, http.StatusNotFound, goalRequest(t, h, "/books/fixture-metadata-only/refresh", url.Values{"csrf_token": {csrf}}, cookies).Code)
	assert.Equal(t, http.StatusNotFound, goalRequest(t, h, "/books/fixture-book/deck/preparations", url.Values{"csrf_token": {csrf}}, cookies).Code)
}

func TestCompletedAnalysisCompatibilityRouteRedirectsToJourneyEntry(t *testing.T) {
	h, cookies, _, _ := goalFixtureSession(t)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/books/fixture-book/analyses/fixture-run", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/journey/fixture-book", response.Header().Get("Location"))
	request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/books/fixture-book/analyses/old-run", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestBookDetailHeaderUsesTheBooksOwnLanguage(t *testing.T) {
	book := domain.SourceMaterialSummary{
		Source: domain.SourceMaterial{
			ID:        "italian-book",
			Title:     "Una storia italiana",
			Language:  "it",
			MediaType: "application/epub+zip",
		},
		AnalysisStatus: "analyzed",
		AnalysisState:  "completed",
		AnalysisRunID:  "italian-run",
		CorpusID:       "italian-corpus",
	}
	var output bytes.Buffer
	require.NoError(t, BookPageWithOptions(domain.User{Username: "learner"}, "csrf", book, nil, false, "", journeyBookPageOptions(book), nil, emptyDeckJourneyAction()).Render(context.Background(), &output))
	html := output.String()
	assert.True(t, strings.Contains(html, "<h1>Una storia italiana</h1>") && strings.Contains(html, "it · application/epub+zip"), "book detail omitted its own language: %s", html)
}

func TestBookDetailHeaderRendersCanonicalBookTitle(t *testing.T) {
	book := domain.SourceMaterialSummary{
		Source:         domain.SourceMaterial{ID: "canonical-book", Title: "Acquisition-internal title", Language: "de", MediaType: "application/epub+zip"},
		BookTitle:      "Refreshed catalogue title",
		AnalysisStatus: "analyzed",
		AnalysisState:  "completed",
		AnalysisRunID:  "canonical-run",
		CorpusID:       "canonical-corpus",
	}
	var output bytes.Buffer
	require.NoError(t, BookPageWithOptions(domain.User{Username: "learner"}, "csrf", book, nil, false, "", journeyBookPageOptions(book), nil, emptyDeckJourneyAction()).Render(context.Background(), &output))
	html := output.String()
	assert.True(t, strings.Contains(html, "<h1>Refreshed catalogue title</h1>"), "analyzed page omitted canonical Book title: %s", html)
	assert.False(t, strings.Contains(html, "Acquisition-internal title"), "analyzed page rendered acquisition-internal title: %s", html)
}

func TestAnalyzedBookPageUsesParameterizedJourneyContext(t *testing.T) {
	book := domain.SourceMaterialSummary{
		Source:    domain.SourceMaterial{ID: "journey-book", Title: "Internal title", Language: "de", MediaType: "application/epub+zip"},
		BookTitle: "Journey title",
	}
	page := bookPageOptions{
		BreadcrumbURL:   "/journey",
		BreadcrumbLabel: "Reading Journey",
		Navigation:      NavigationReadingJourney,
		Journey:         bookPageJourneyState{Member: true, Revision: 3},
	}
	var output bytes.Buffer
	require.NoError(t, BookPageWithOptions(domain.User{Username: "learner"}, "csrf", book, nil, false, "", page, nil, emptyDeckJourneyAction()).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{
		`data-navigation-context="reading-journey"`,
		`href="/journey"`,
		"← Reading Journey",
		"In Reading Journey.",
		`action="/journey/books/journey-book/remove"`,
	} {
		assert.True(t, strings.Contains(html, want), "parameterized Journey page missing %q: %s", want, html)
	}
	assert.False(t, strings.Contains(html, "← My Books") || strings.Contains(html, `data-navigation-context="library"`), "parameterized Journey page retained My Books context: %s", html)
}

type bookRefreshStub struct {
	result cataloguesync.RefreshResult
	owner  string
	calls  int
}

func (s *bookRefreshStub) RegisterConnection(context.Context, string, string) error { return nil }
func (s *bookRefreshStub) UnregisterConnection(string, string) error                { return nil }
func (s *bookRefreshStub) RefreshEntry(_ context.Context, owner, _ string) (cataloguesync.RefreshResult, error) {
	s.owner = owner
	s.calls++
	return s.result, nil
}

func TestBookMetadataRefreshNativeAndHTMXFlowsEnforceCSRF(t *testing.T) {
	h, cookies, csrf, _ := goalFixtureSession(t)
	stub := &bookRefreshStub{result: cataloguesync.RefreshResult{Book: domain.Book{ID: "fixture-metadata-only", OwnerID: "fixture-learner", Title: "Updated catalogue title"}, Updated: true}}
	requireHandler(t, h).services.CatalogueSync = stub
	missingCSRF := goalRequest(t, h, "/library/books/fixture-metadata-only/refresh", url.Values{}, cookies)
	assert.Equal(t, http.StatusForbidden, missingCSRF.Code)
	assert.Equal(t, 0, stub.calls)
	native := goalRequest(t, h, "/library/books/fixture-metadata-only/refresh", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, native.Code)
	assert.True(t, strings.Contains(native.Header().Get("Location"), "Metadata+refreshed"), "native refresh location=%q", native.Header().Get("Location"))
	assert.Equal(t, "fixture-learner", stub.owner)

	h, cookies, csrf, _ = goalFixtureSession(t)
	stub = &bookRefreshStub{result: cataloguesync.RefreshResult{Book: domain.Book{ID: "fixture-metadata-only", OwnerID: "fixture-learner", Title: "Updated catalogue title"}, Missing: true}}
	requireHandler(t, h).services.CatalogueSync = stub
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/library/books/fixture-metadata-only/refresh", strings.NewReader(url.Values{"csrf_token": {csrf}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.True(t, strings.Contains(response.Body.String(), `id="book-row-fixture-metadata-only"`), "HTMX refresh body=%s", response.Body.String())
	assert.True(t, strings.Contains(response.Body.String(), "catalog entry is no longer available"), "HTMX refresh body=%s", response.Body.String())

	h, cookies, csrf, _ = goalFixtureSession(t)
	stub = &bookRefreshStub{result: cataloguesync.RefreshResult{Book: domain.Book{ID: "fixture-metadata-only", OwnerID: "fixture-learner", Title: "Updated row title"}, Updated: true}}
	requireHandler(t, h).services.CatalogueSync = stub
	request = httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/library/books/fixture-metadata-only/refresh", strings.NewReader(url.Values{"csrf_token": {csrf}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	request.Header.Set("HX-Target", "book-row-fixture-metadata-only")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.True(t, strings.Contains(response.Body.String(), `id="book-row-fixture-metadata-only"`), "HTMX row refresh body=%s", response.Body.String())
	assert.True(t, strings.Contains(response.Body.String(), "Updated row title"), "HTMX row refresh body=%s", response.Body.String())
	assert.True(t, strings.Contains(response.Body.String(), "Metadata refreshed."), "HTMX row refresh body=%s", response.Body.String())
}

func TestUnavailableCatalogueRefresherKeepsRowTargetIntact(t *testing.T) {
	h, cookies, csrf, _ := goalFixtureSession(t)
	requireHandler(t, h).services.CatalogueSync = nil
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/library/books/fixture-metadata-only/refresh", strings.NewReader(url.Values{"csrf_token": {csrf}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	request.Header.Set("HX-Target", "book-row-fixture-metadata-only")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.True(t, strings.Contains(response.Body.String(), `id="book-row-fixture-metadata-only"`), "unavailable refresher row response body=%s", response.Body.String())
	assert.True(t, strings.Contains(response.Body.String(), "Metadata could not be refreshed"), "unavailable refresher row response body=%s", response.Body.String())
	assert.False(t, strings.Contains(response.Body.String(), `id="book-metadata-region"`), "unavailable refresher row response body=%s", response.Body.String())
}
