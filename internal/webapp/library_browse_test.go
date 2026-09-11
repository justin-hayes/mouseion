package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMyBooksBrowseControlsRenderScopedSearchAndPaging(t *testing.T) {
	books := []domain.MyBook{
		{Book: domain.Book{ID: "de-book", OwnerID: "owner", Title: "De book", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "unknown-book", OwnerID: "owner", Title: "Unknown book", LanguageState: domain.LanguageUnknown}},
	}
	state := MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", NeedsLanguageCount: 1, AllCount: 4, Total: 2, Page: 2, PageCount: 2}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "", false, state).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{
		`role="search"`, `label for="library-search-query">Search My Books`,
		`id="library-results"`, `data-focus-id="library-books-heading"`, `Page 2 of 2`, `href="/library?needs-language"`,
		`href="/library"`, `href="/library?page=2"`,
	} {
		assert.True(t, strings.Contains(html, want), "browse markup missing %q: %s", want, html)
	}
	assert.False(t, strings.Contains(html, `href="/books/de-book"`), "My Books browse linked to the retired Book detail page: %s", html)
	for _, forbidden := range []string{"All languages", "Unknown language", `aria-label="Languages"`, `name="language"`, `language: de`, "language not chosen", `href="/books/unknown-book"`} {
		assert.False(t, strings.Contains(html, forbidden), "legacy language browse markup remains %q: %s", forbidden, html)
	}
	assert.True(t, strings.Contains(html, `hx-push-url="true"`), "browse controls are missing HTMX URL enhancement: %s", html)
	assert.True(t, strings.Contains(html, `hx-target="#library-results"`), "browse controls are missing HTMX URL enhancement: %s", html)
}

func TestMyBooksBrowseNoMatchStatesPreserveTheRightFilters(t *testing.T) {
	cases := []struct {
		name  string
		state MyBooksBrowseState
		wants []string
	}{
		{name: "text", state: MyBooksBrowseState{Enabled: true, Query: "missing title", Language: "de", LanguageLabel: "German", AllCount: 3, TextNoMatch: true}, wants: []string{"No books in your local collection in German match “missing title”", "Clear search", `href="/library"`}},
		{name: "collection", state: MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", AllCount: 3}, wants: []string{"No books in your local collection are in German."}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", false, tc.state).Render(context.Background(), &output))
			html := output.String()
			for _, want := range tc.wants {
				assert.True(t, strings.Contains(html, want), "no-match markup missing %q: %s", want, html)
			}
			assert.Equal(t, 1, strings.Count(html, `role="status"`), "no-match state should have one scoped live region: %s", html)
		})
	}
}

type browseRecordingStore struct {
	*fixtures.Store
	result   persistence.MyBooksBrowseResult
	owner    string
	query    string
	language string
	offset   int
	limit    int
}

func (s *browseRecordingStore) ListMyBooksBrowse(_ context.Context, owner, query, language string, offset, limit int) (persistence.MyBooksBrowseResult, error) {
	s.owner, s.query, s.language, s.offset, s.limit = owner, query, language, offset, limit
	return s.result, nil
}

func TestLibraryHandlerParsesBrowseStateAndClampsStalePages(t *testing.T) {
	h, cookies, _, fixtureStore := goalFixtureSession(t)
	handler := h.(*Handler)
	store := &browseRecordingStore{Store: fixtureStore, result: persistence.MyBooksBrowseResult{
		Items: []domain.MyBook{{Book: domain.Book{ID: "book-1", OwnerID: fixtures.OwnerID, Title: "Book 1", LanguageState: domain.LanguageChosen, LanguageTag: "de"}}},
		Total: 26, AllCount: 26, Counts: []persistence.LanguageCount{{Tag: "de", Count: 26}},
	}}
	handler.services.Store = store

	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}

	for _, path := range []string{"/library", "/library?page=0", "/library?page=invalid"} {
		response := request(path)
		assert.Equal(t, http.StatusOK, response.Code, path)
		assert.Equal(t, fixtures.OwnerID, store.owner, "GET %s", path)
		assert.Equal(t, 0, store.offset, "GET %s", path)
		assert.Equal(t, myBooksPageSize, store.limit, "GET %s", path)
	}
	response := request("/library?q=%20Dampf%20&page=3")
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/library?page=2&q=Dampf", response.Header().Get("Location"))
	assert.Equal(t, "Dampf", store.query)
	assert.Equal(t, "de", store.language)
	assert.Equal(t, 50, store.offset)
	legacy := request("/library?language=it&q=Dampf")
	assert.Equal(t, http.StatusSeeOther, legacy.Code)
	assert.Equal(t, "/library?q=Dampf", legacy.Header().Get("Location"))
	htmxRequest := httptest.NewRequest(http.MethodGet, "/library?q=Dampf", nil)
	htmxRequest.Header.Set("HX-Request", "true")
	for _, cookie := range cookies {
		htmxRequest.AddCookie(cookie)
	}
	htmxResponse := httptest.NewRecorder()
	handler.ServeHTTP(htmxResponse, htmxRequest)
	assert.Equal(t, http.StatusOK, htmxResponse.Code)
	assert.True(t, strings.Contains(htmxResponse.Body.String(), `<section id="library-results"`), "HTMX library response was not a results fragment: body=%s", htmxResponse.Body.String())
	assert.False(t, strings.Contains(htmxResponse.Body.String(), "<!doctype html>"), "HTMX library response was not a results fragment: body=%s", htmxResponse.Body.String())
	store.result.Total = 0
	store.result.ScopeTotal = 26
	response = request("/library?page=3")
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/library?page=2", response.Header().Get("Location"))
	store.result = persistence.MyBooksBrowseResult{
		Items: []domain.MyBook{{Book: domain.Book{ID: "unknown-book", OwnerID: fixtures.OwnerID, Title: "Unknown Book", LanguageState: domain.LanguageUnknown}}},
		Total: 1, ScopeTotal: 1, AllCount: 26, Counts: []persistence.LanguageCount{{Tag: domain.LanguageUnknown, Count: 1}},
	}
	response = request("/library?needs-language")
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, domain.LanguageUnknown, store.language)
	assert.True(t, strings.Contains(response.Body.String(), "Books awaiting a language"), "needs-language browse body=%s", response.Body.String())
	assert.True(t, strings.Contains(response.Body.String(), "Unknown Book"), "needs-language browse body=%s", response.Body.String())
	assert.False(t, strings.Contains(response.Body.String(), `href="/books/unknown-book"`), "needs-language browse exposed mutation actions: %s", response.Body.String())
	assert.False(t, strings.Contains(response.Body.String(), "Add to Reading Journey"), "needs-language browse exposed mutation actions: %s", response.Body.String())
	assert.False(t, strings.Contains(response.Body.String(), "Remove from My Books"), "needs-language browse exposed mutation actions: %s", response.Body.String())
}

func TestLibraryHandlerOnlyOffersRefreshForEligibleMetadataOnlyBooks(t *testing.T) {
	h, cookies, _, fixtureStore := goalFixtureSession(t)
	handler := h.(*Handler)
	store := &browseRecordingStore{Store: fixtureStore, result: persistence.MyBooksBrowseResult{
		Items: []domain.MyBook{
			{Book: domain.Book{ID: "fixture-metadata-only", OwnerID: fixtures.OwnerID, Title: "Eligible metadata", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
			{Book: domain.Book{ID: "no-alias", OwnerID: fixtures.OwnerID, Title: "Ineligible metadata", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		},
		Total: 2, ScopeTotal: 2, AllCount: 2, Counts: []persistence.LanguageCount{{Tag: "de", Count: 2}},
	}}
	handler.services.Store = store
	request := httptest.NewRequest(http.MethodGet, "/library", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	html := response.Body.String()
	eligibleStart := strings.Index(html, `id="book-row-fixture-metadata-only"`)
	ineligibleStart := strings.Index(html, `id="book-row-no-alias"`)
	require.True(t, eligibleStart >= 0, "metadata-only rows missing: %s", html)
	require.True(t, ineligibleStart >= 0, "metadata-only rows missing: %s", html)
	eligibleRow := html[eligibleStart:ineligibleStart]
	ineligibleRow := html[ineligibleStart:]
	assert.True(t, strings.Contains(eligibleRow, "Refresh metadata"), "eligible metadata-only row omitted refresh: %s", eligibleRow)
	assert.True(t, strings.Contains(eligibleRow, `hx-target="#book-row-fixture-metadata-only"`), "eligible metadata-only row omitted refresh: %s", eligibleRow)
	assert.False(t, strings.Contains(ineligibleRow, "Refresh metadata"), "ineligible metadata-only row exposed refresh: %s", ineligibleRow)
}

func TestMyBooksBrowseRequestDefaults(t *testing.T) {
	query, page, needsLanguage := parseMyBooksBrowseRequest(&url.URL{RawQuery: "q=+title+&language=de&page=-4"})
	assert.Equal(t, "title", query)
	assert.Equal(t, 1, page)
	assert.False(t, needsLanguage, "ordinary browse request unexpectedly selected needs-language")
	query, page, needsLanguage = parseMyBooksBrowseRequest(&url.URL{RawQuery: "needs-language&q=title&page=2"})
	assert.Equal(t, "title", query)
	assert.Equal(t, 2, page)
	assert.True(t, needsLanguage)
	assert.Equal(t, "/library?needs-language&page=2&q=title", myBooksURL("title", 2, true))
}

func TestMyBooksWithoutActiveLanguageKeepsCatalogSetupAction(t *testing.T) {
	var output bytes.Buffer
	state := MyBooksBrowseState{Enabled: true}
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", false, state).Render(context.Background(), &output))
	html := output.String()
	assert.True(t, strings.Contains(html, "Set up a catalog connection"), "empty unscoped My Books state lost catalogue setup: %s", html)
	assert.True(t, strings.Contains(html, `href="/catalogs">Set up a catalog</a>`), "empty unscoped My Books state lost catalogue setup: %s", html)
}

func TestLibraryHandlerOmitsRetiredLanguageView(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	handler := h.(*Handler)
	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}

	handler.services.AnalysisInsights = fixtures.Insights{JourneyStore: store}
	response := request("/library")
	assert.Equal(t, http.StatusOK, response.Code)
	for _, forbidden := range []string{`id="language-view-panel"`, "Coverage across", "Language view", "Highest-impact unknown vocabulary", "Per-book spread"} {
		assert.False(t, strings.Contains(response.Body.String(), forbidden), "retired language view markup remains %q: %s", forbidden, response.Body.String())
	}
}
