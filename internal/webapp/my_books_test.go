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

func TestMyBooksMetadataOnlyGridItemExposesOnlySupportedActions(t *testing.T) {
	book := domain.MyBook{Book: domain.Book{ID: "metadata-book", OwnerID: "owner", Title: "A book without an EPUB", LanguageState: domain.LanguageUnknown}}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, MyBooksBrowseState{RefreshableBookIDs: map[string]bool{"metadata-book": true}}).Render(context.Background(), &output))
	html := output.String()
	if main := strings.Index(html, "<main"); main >= 0 {
		html = html[main:]
	}
	for _, want := range []string{"A book without an EPUB", "No cover available", `aria-hidden="true"`, "More actions", "Refresh metadata", `hx-post="/library/books/metadata-book/refresh"`, `hx-target="#book-row-metadata-book"`, "Add to Reading Journey", `action="/reading/books/metadata-book/journey/add"`, `name="expected_revision" value="0"`, "Remove from My Books", `action="/library/books/metadata-book/remove"`} {
		assert.True(t, strings.Contains(html, want), "metadata-only My Books item missing %q: %s", want, html)
	}
	assert.False(t, strings.Contains(html, `href="/books/metadata-book"`), "metadata-only My Books row linked to the retired Book detail page: %s", html)
	row := html
	if start := strings.Index(row, `aria-labelledby="book-title-metadata-book"`); start >= 0 {
		row = row[start:]
		if end := strings.Index(row, "</li>"); end >= 0 {
			row = row[:end+len("</li>")]
		}
	}
	for _, forbidden := range []string{"Not acquired / metadata only", "Review scope", "Prepare deck", "View analysis result", "Start analysis", `action="/books/metadata-book/analyze"`, "coverage", "Primary Goal", "Analysis evidence"} {
		assert.False(t, strings.Contains(row, forbidden), "metadata-only My Books item exposed unsupported state or action %q: %s", forbidden, row)
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
	assert.False(t, strings.Contains(html, `action="/reading/books/journey-book/journey/add"`) || strings.Contains(html, "Add to Reading Journey"), "existing Journey member still exposed add action: %s", html)
	assert.Contains(t, html, "In Reading Journey")
	assert.Contains(t, html, "View in Reading Journey")
	assert.Contains(t, html, `href="/reading#journey-book-journey-book"`)
}

func TestMyBooksJourneyActionHidesAddForPrimaryGoal(t *testing.T) {
	book := domain.MyBook{
		Book:        domain.Book{ID: "goal-book", OwnerID: "owner", Title: "Goal book"},
		JourneyGoal: true,
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.False(t, strings.Contains(html, `action="/reading/books/goal-book/journey/add"`) || strings.Contains(html, "Add to Reading Journey"), "Goal member still exposed add action: %s", html)
	assert.Contains(t, html, "In Reading Journey")
	assert.Contains(t, html, `href="/reading#journey-book-goal-book"`)
	assert.NotContains(t, html, "Primary Goal")
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
	assert.True(t, strings.Contains(html, `href="/reading#journey-book-analyzed-book"`) && !strings.Contains(html, `href="/books/analyzed-book"`) && !strings.Contains(html, "View analysis result") && !strings.Contains(html, `action="/books/analyzed-book/analyze"`), "analyzed Journey member exposed an invalid My Books action or link: %s", html)
	assert.NotContains(t, html, "Analysis evidence")
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
	assert.False(t, strings.Contains(html, "Analysis evidence") || strings.Contains(html, `href="/reading/analyzed-outside-journey"`) || strings.Contains(html, `href="/books/`) || strings.Contains(html, "View analysis result"), "analyzed non-member My Books item exposed an invalid link or state: %s", html)
}

func TestMyBooksRowRendersCanonicalBookTitle(t *testing.T) {
	book := domain.MyBook{
		Book: domain.Book{ID: "canonical-book", OwnerID: "owner", Title: "Refreshed catalogue title", Author: "Catalogue author"},
		Acquired: &domain.SourceMaterialSummary{
			Source: domain.SourceMaterial{ID: "source-canonical-book", Title: "Acquisition-internal title"},
		},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.True(t, strings.Contains(html, "Refreshed catalogue title"), "My Books omitted canonical Book title: %s", html)
	assert.Contains(t, html, "By Catalogue author")
	assert.False(t, strings.Contains(html, "Acquisition-internal title"), "My Books rendered acquisition-internal title: %s", html)
}

func TestMyBooksOmitsEvidenceAndAcquisitionState(t *testing.T) {
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
	for _, label := range []string{"Unavailable", "Not acquired", "Ready to analyze", "Analysis not started", "Analyzed", "Stale analysis", "Primary Goal", "Next action", "Analysis evidence"} {
		assert.NotContains(t, html, label)
	}
	assert.False(t, strings.Contains(html, `href="/books/`), "learner My Books linked to the retired Book detail page: %s", html)
	assert.False(t, strings.Contains(html, "Start analysis") || strings.Contains(html, `/analyze`), "My Books rows exposed an explicit analysis action: %s", html)
}

func TestMyBooksCoverGridUsesNativeListAndStablePresentationStates(t *testing.T) {
	books := []domain.MyBook{
		{Book: domain.Book{ID: "available", OwnerID: "owner", Title: "A very long title that remains fully visible", Author: "An author with a name long enough to wrap in a narrow grid item"}, Cover: domain.BookCover{State: domain.BookCoverAvailable, Width: 600, Height: 900}, JourneyMember: true},
		{Book: domain.Book{ID: "pending", OwnerID: "owner", Title: "Pending cover"}, Cover: domain.BookCover{State: domain.BookCoverPending}},
		{Book: domain.Book{ID: "unavailable", OwnerID: "owner", Title: "Unavailable cover"}, Cover: domain.BookCover{State: domain.BookCoverUnavailable}},
		{Book: domain.Book{ID: "none", OwnerID: "owner", Title: "No advertised cover"}},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()

	assert.Contains(t, html, `<ul class="library-grid">`)
	assert.Equal(t, 4, strings.Count(html, `class="resource-card library-book"`))
	assert.NotContains(t, html, `role="grid"`)
	assert.NotContains(t, html, `aria-rowindex`)
	for _, label := range []string{"Cover pending", "Cover unavailable", "No cover available"} {
		assert.Contains(t, html, label)
	}
	assert.Contains(t, html, `src="/books/available/cover"`)
	assert.Contains(t, html, `width="600" height="900" loading="lazy"`)
	assert.Contains(t, html, "An author with a name long enough to wrap in a narrow grid item")
	for _, forbidden := range []string{"Analysis evidence", "Not acquired", "Primary Goal", "Next action", "Ready to analyze"} {
		assert.NotContains(t, html, forbidden)
	}

	start := strings.Index(html, `id="book-row-available"`)
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(html[start:], "</li>")
	require.Greater(t, end, 0)
	item := html[start : start+end]
	for _, content := range []string{`<a class="library-book__identity-link"`, "A very long title that remains fully visible", "By An author", "In Reading Journey", "View in Reading Journey", "More actions"} {
		assert.Contains(t, item, content)
	}
	assert.Equal(t, 1, strings.Count(item, `class="library-book__identity-link"`), "cover and title must share one identity link")
	assert.Less(t, strings.Index(item, `class="book-cover-media"`), strings.Index(item, "A very long title"))
	assert.Less(t, strings.Index(item, "A very long title"), strings.Index(item, "By An author"))
	assert.Less(t, strings.Index(item, "By An author"), strings.Index(item, "In Reading Journey"))
	assert.Less(t, strings.Index(item, "In Reading Journey"), strings.Index(item, "View in Reading Journey"))
	assert.Less(t, strings.Index(item, "View in Reading Journey"), strings.Index(item, "More actions"))
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
	assert.Equal(t, "/reading#journey-book-fixture-book", response.Header().Get("Location"))
	request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/books/fixture-book/analyses/old-run", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)
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
	request.Header.Set("Hx-Request", "true")
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
	request.Header.Set("Hx-Request", "true")
	request.Header.Set("Hx-Target", "book-row-fixture-metadata-only")
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
	request.Header.Set("Hx-Request", "true")
	request.Header.Set("Hx-Target", "book-row-fixture-metadata-only")
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
