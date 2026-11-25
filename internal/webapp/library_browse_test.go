package webapp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
	state := MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", Disposition: domain.BookDispositionToRead, AllCount: 4, Total: 2, Page: 2, PageCount: 2}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", state).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{
		`role="search"`, `method="get"`, `label class="sr-only" for="library-search-query">Search My Books`,
		`type="search" name="q"`, `class="btn btn-outline" type="submit">Search</button>`,
		`id="library-results"`, `data-focus-id="library-results"`, `aria-label="My Books results"`, `Page 2 of 2`, `href="/library?history=read"`,
		`href="/library?disposition=to_read&amp;page=2"`,
	} {
		assert.Contains(t, html, want, "browse markup missing %q: %s", want, html)
	}
	assert.NotContains(t, html, `href="/books/de-book"`, "My Books browse linked to the retired Book detail page: %s", html)
	for _, forbidden := range []string{"All languages", "Unknown language", `aria-label="Languages"`, `name="language"`, `language: de`, "language not chosen", `href="/books/unknown-book"`} {
		assert.NotContains(t, html, forbidden, "legacy language browse markup remains %q: %s", forbidden, html)
	}
	assert.Contains(t, html, `hx-push-url="true"`, "browse controls are missing HTMX URL enhancement: %s", html)
	assert.Contains(t, html, `hx-target="#library-results"`, "browse controls are missing HTMX URL enhancement: %s", html)
}

func TestMyBooksBrowseNoMatchStatesPreserveTheRightFilters(t *testing.T) {
	cases := []struct {
		name  string
		state MyBooksBrowseState
		wants []string
	}{
		{name: "text", state: MyBooksBrowseState{Enabled: true, Query: "missing title", Language: "de", LanguageLabel: "German", Disposition: domain.BookDispositionToRead, AllCount: 3, TextNoMatch: true}, wants: []string{"No To Read books in German match “missing title”", "Clear search", `href="/library?disposition=to_read"`}},
		{name: "collection", state: MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", Disposition: domain.BookDispositionToRead, AllCount: 3}, wants: []string{"No books in your To Read list in German.", `href="/catalogs"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", tc.state).Render(context.Background(), &output))
			html := output.String()
			for _, want := range tc.wants {
				assert.Contains(t, html, want, "no-match markup missing %q: %s", want, html)
			}
			assert.Equal(t, 1, strings.Count(html, `role="status"`), "no-match state should have one scoped live region: %s", html)
		})
	}
}

func TestClearSearchResetsQueryAndPreservesTab(t *testing.T) {
	state := MyBooksBrowseState{Enabled: true, Query: "missing", Language: "de", LanguageLabel: "German", Disposition: domain.BookDispositionToRead, AllCount: 3, TextNoMatch: true}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", state).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, `href="/library?disposition=to_read"`)
}

// A GET form submit replaces its action's query string with the form fields, so
// the active tab has to travel as hidden fields for a search to stay on it.
func TestMyBooksSearchFormCarriesTheActiveTabWithoutJavaScript(t *testing.T) {
	cases := []struct {
		name      string
		state     MyBooksBrowseState
		wants     []string
		forbidden []string
	}{
		{name: "read", state: MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", AllCount: 2, History: true}, wants: []string{`<input type="hidden" name="history" value="read">`}, forbidden: []string{`name="disposition"`}},
		{name: "to read", state: MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", AllCount: 2, Disposition: domain.BookDispositionToRead}, wants: []string{`<input type="hidden" name="disposition" value="to_read">`}, forbidden: []string{`name="history"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", tc.state).Render(context.Background(), &output))
			html := output.String()
			start := strings.Index(html, `<form class="library-search"`)
			require.GreaterOrEqual(t, start, 0, html)
			form := html[start : start+strings.Index(html[start:], "</form>")]
			assert.Contains(t, form, `action="/library"`)
			assert.Contains(t, form, `hx-get="/library"`)
			for _, want := range tc.wants {
				assert.Contains(t, form, want)
			}
			for _, bad := range tc.forbidden {
				assert.NotContains(t, form, bad)
			}
		})
	}
}

func TestMyBooksUnenhancedSearchSubmitStaysOnItsTab(t *testing.T) {
	readSubmit := url.Values{"history": {"read"}, "q": {"iliad"}}
	request, ok := parseMyBooksBrowseRequest(&url.URL{Path: "/library", RawQuery: readSubmit.Encode()})
	require.True(t, ok)
	assert.Equal(t, "iliad", request.query)
	assert.Equal(t, 1, request.page)
	assert.Empty(t, request.disposition)
	assert.True(t, request.history)

	toReadSubmit := url.Values{"disposition": {"to_read"}, "q": {"iliad"}}
	request, ok = parseMyBooksBrowseRequest(&url.URL{Path: "/library", RawQuery: toReadSubmit.Encode()})
	require.True(t, ok)
	assert.Equal(t, domain.BookDispositionToRead, request.disposition)
	assert.False(t, request.history)
}

type pagedBookBrowseReader struct {
	*fixtures.Store
	bookID      string
	requestedAt int
	requests    int
	gotOwner    string
	gotLanguage string
	gotBucket   string
	gotHistory  bool
}

func (r *pagedBookBrowseReader) ListMyBooksBrowse(_ context.Context, owner, _, language, disposition string, history bool, offset, limit int) (persistence.MyBooksBrowseResult, error) {
	r.requests++
	r.gotOwner, r.gotLanguage, r.gotBucket, r.gotHistory = owner, language, disposition, history
	if offset == r.requestedAt {
		return persistence.MyBooksBrowseResult{Items: []domain.MyBook{{Book: domain.Book{ID: r.bookID}}}, Total: offset + 1}, nil
	}
	return persistence.MyBooksBrowseResult{Items: make([]domain.MyBook, limit), Total: r.requestedAt + 1}, nil
}

func TestMyBooksPageForBookFindsBookOnLaterReadPage(t *testing.T) {
	reader := &pagedBookBrowseReader{Store: fixtures.NewStore(), bookID: "historical-book", requestedAt: myBooksPageSize}
	book := domain.MyBook{
		Book:            domain.Book{ID: "historical-book"},
		Disposition:     domain.BookDispositionInbox,
		CompletionCount: 1,
	}

	page, err := myBooksPageForBook(context.Background(), reader, "owner-1", "de", book)
	require.NoError(t, err)
	assert.Equal(t, 2, page)
	assert.Equal(t, 2, reader.requests)
	assert.Equal(t, "owner-1", reader.gotOwner)
	assert.Equal(t, "de", reader.gotLanguage)
	assert.Empty(t, reader.gotBucket)
	assert.True(t, reader.gotHistory)
	assert.Equal(t, "/library?history=read&page=2", myBookBucketURL(book, page))
}

func TestMyBooksPageForCurrentReadingUsesToReadTab(t *testing.T) {
	reader := &pagedBookBrowseReader{Store: fixtures.NewStore(), bookID: "current-book", requestedAt: 0}
	book := domain.MyBook{
		Book: domain.Book{ID: "current-book"}, Disposition: domain.BookDispositionToRead, IsCurrentReading: true,
	}
	page, err := myBooksPageForBook(context.Background(), reader, "owner-1", "de", book)
	require.NoError(t, err)
	assert.Equal(t, 1, page)
	assert.Equal(t, domain.BookDispositionToRead, domain.BookDisposition(reader.gotBucket))
	assert.False(t, reader.gotHistory)
	assert.Equal(t, "/library?disposition=to_read", myBookBucketURL(book, page))
}

type browseRecordingStore struct {
	*fixtures.Store
	result      persistence.MyBooksBrowseResult
	err         error
	owner       string
	query       string
	language    string
	disposition string
	offset      int
	limit       int
}

func (s *browseRecordingStore) ListMyBooksBrowse(_ context.Context, owner, query, language, disposition string, history bool, offset, limit int) (persistence.MyBooksBrowseResult, error) {
	s.owner, s.query, s.language, s.disposition, s.offset, s.limit = owner, query, language, disposition, offset, limit
	return s.result, s.err
}

func TestLibraryHandlerUsesRequestAppropriateErrorRepresentationAndPreservesVary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		partial bool
	}{
		{name: "full page"},
		{name: "partial", partial: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, cookies, _, fixtureStore := readingFixtureSession(t)
			handler := requireHandler(t, h)
			store := &browseRecordingStore{Store: fixtureStore, err: errors.New("read failed")}
			handler.services.Store = storeDependencies(store)
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/library?q=Dampf", nil)
			if tc.partial {
				request.Header.Set("Hx-Request-Type", "partial")
			}
			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}
			response := httptest.NewRecorder()
			response.Header().Add("Vary", "Accept-Encoding")
			handler.ServeHTTP(response, request)

			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
			vary := strings.Join(response.Header().Values("Vary"), ",")
			assert.Contains(t, vary, "Accept-Encoding")
			assert.Contains(t, vary, "HX-Request-Type")
			assert.Contains(t, response.Body.String(), "Try again")
			if tc.partial {
				assert.Contains(t, response.Body.String(), `<section id="library-results"`)
				assert.NotContains(t, response.Body.String(), "<!doctype html>")
				assert.Contains(t, response.Body.String(), `href="/library?q=Dampf"`)
			} else {
				assert.Contains(t, response.Body.String(), "<!doctype html>")
				assert.Contains(t, response.Body.String(), `href="/library?q=Dampf"`)
			}
		})
	}
}

// TestMyBooksRedirectPageNeverTargetsAPageTheTabCannotList pins that the Read
// tab's empty page recovers to page 1: the scope total counts Books the Read
// tab does not list, so it cannot size the tab's last page.
func TestMyBooksRedirectPageNeverTargetsAPageTheTabCannotList(t *testing.T) {
	result := domain.MyBooksBrowseResult{Total: 0, ScopeTotal: 26}

	page, redirect := myBooksRedirectPage(myBooksLibraryRequest{page: 3, history: true}, result)
	assert.True(t, redirect)
	assert.Equal(t, 1, page, "the Read tab lists none of the scope total")

	page, redirect = myBooksRedirectPage(myBooksLibraryRequest{page: 3}, result)
	assert.True(t, redirect)
	assert.Equal(t, 1, page, "an empty view has no page beyond the first")
}

func TestLibraryHandlerParsesBrowseStateAndClampsStalePages(t *testing.T) {
	h, cookies, _, fixtureStore := readingFixtureSession(t)
	handler := requireHandler(t, h)
	store := &browseRecordingStore{Store: fixtureStore, result: persistence.MyBooksBrowseResult{
		Items: []domain.MyBook{{Book: domain.Book{ID: "book-1", OwnerID: fixtures.OwnerID, Title: "Book 1", LanguageState: domain.LanguageChosen, LanguageTag: "de"}}},
		Total: 26, AllCount: 26,
	}}
	handler.services.Store = storeDependencies(store)

	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		response.Header().Add("Vary", "Accept-Encoding")
		handler.ServeHTTP(response, r)
		return response
	}

	for _, path := range []string{"/library", "/library?page=0", "/library?page=invalid"} {
		response := request(path)
		assert.Equal(t, http.StatusOK, response.Code, path)
		vary := strings.Join(response.Header().Values("Vary"), ",")
		assert.Contains(t, vary, "HX-Request-Type", "GET %s", path)
		assert.Contains(t, vary, "Accept-Encoding", "GET %s", path)
		assert.Equal(t, fixtures.OwnerID, store.owner, "GET %s", path)
		assert.Equal(t, 0, store.offset, "GET %s", path)
		assert.Equal(t, myBooksPageSize, store.limit, "GET %s", path)
	}
	response := request("/library?q=%20Dampf%20&page=3")
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/library?disposition=to_read&page=2&q=Dampf", response.Header().Get("Location"))
	assert.Equal(t, "Dampf", store.query)
	assert.Equal(t, "de", store.language)
	assert.Equal(t, 50, store.offset)
	legacy := request("/library?language=it&q=Dampf")
	assert.Equal(t, http.StatusSeeOther, legacy.Code)
	assert.Equal(t, languageChangedPath(), legacy.Header().Get("Location"), "an old-language request recovers to My Books instead of reinterpreting its filters")
	partialRequest := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		r.Header.Set("Hx-Request-Type", "partial")
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}
	partialLegacy := partialRequest("/library?language=it&q=Dampf")
	assert.Equal(t, http.StatusOK, partialLegacy.Code)
	assert.Equal(t, languageChangedPath(), partialLegacy.Header().Get("Hx-Redirect"))
	assert.Empty(t, partialLegacy.Header().Get("Location"))
	htmxRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/library?q=Dampf", nil)
	htmxRequest.Header.Set("Hx-Request-Type", "partial")
	for _, cookie := range cookies {
		htmxRequest.AddCookie(cookie)
	}
	htmxResponse := httptest.NewRecorder()
	htmxResponse.Header().Add("Vary", "Accept-Encoding")
	handler.ServeHTTP(htmxResponse, htmxRequest)
	assert.Equal(t, http.StatusOK, htmxResponse.Code)
	partialVary := strings.Join(htmxResponse.Header().Values("Vary"), ",")
	assert.Contains(t, partialVary, "HX-Request-Type")
	assert.Contains(t, partialVary, "Accept-Encoding")
	assert.Contains(t, htmxResponse.Body.String(), `<section id="library-results"`, "HTMX library response was not a results fragment: body=%s", htmxResponse.Body.String())
	assert.NotContains(t, htmxResponse.Body.String(), "<!doctype html>", "HTMX library response was not a results fragment: body=%s", htmxResponse.Body.String())
	store.result.Total = 0
	store.result.ScopeTotal = 26
	response = request("/library?page=3")
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/library?disposition=to_read", response.Header().Get("Location"))
	partialStalePage := partialRequest("/library?page=3")
	assert.Equal(t, http.StatusOK, partialStalePage.Code)
	assert.Equal(t, "/library?disposition=to_read", partialStalePage.Header().Get("Hx-Redirect"))
	assert.Empty(t, partialStalePage.Header().Get("Location"))
	// The retired needs-language scope and the Inbox filter are unsupported
	// rather than translated.
	for _, path := range []string{"/library?needs-language", "/library?disposition=inbox", "/library?disposition=set_aside", "/library?disposition=to_read&history=read"} {
		response = request(path)
		assert.Equal(t, http.StatusNotFound, response.Code, "retired My Books state %s is unsupported", path)
	}
}

func TestLibraryHandlerNeverOffersRetiredMetadataRefresh(t *testing.T) {
	h, cookies, _, fixtureStore := readingFixtureSession(t)
	handler := requireHandler(t, h)
	store := &browseRecordingStore{Store: fixtureStore, result: persistence.MyBooksBrowseResult{
		Items: []domain.MyBook{
			{Book: domain.Book{ID: "fixture-metadata-only", OwnerID: fixtures.OwnerID, Title: "Linked metadata", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
			{Book: domain.Book{ID: "no-alias", OwnerID: fixtures.OwnerID, Title: "Unlinked metadata", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		},
		Total: 2, ScopeTotal: 2, AllCount: 2,
	}}
	handler.services.Store = storeDependencies(store)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/library", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	html := response.Body.String()
	assert.Contains(t, html, `id="book-row-fixture-metadata-only"`, "metadata-only rows missing: %s", html)
	assert.Contains(t, html, `id="book-row-no-alias"`, "metadata-only rows missing: %s", html)
	assert.NotContains(t, html, "Refresh metadata", "My Books still offered the retired refresh action: %s", html)
	assert.NotContains(t, html, "/refresh", "My Books still linked a retired refresh route: %s", html)
}

func TestMyBooksBrowseRequestDefaultsToToReadTab(t *testing.T) {
	request, ok := parseMyBooksBrowseRequest(&url.URL{RawQuery: "q=+title+&language=de&page=-4"})
	assert.True(t, ok)
	assert.Equal(t, "title", request.query)
	assert.Equal(t, 1, request.page)
	assert.Equal(t, domain.BookDispositionToRead, request.disposition)
	assert.False(t, request.history)

	request, ok = parseMyBooksBrowseRequest(&url.URL{RawQuery: "q=title&disposition=to_read&page=2"})
	assert.True(t, ok)
	assert.Equal(t, "title", request.query)
	assert.Equal(t, 2, request.page)
	assert.Equal(t, domain.BookDispositionToRead, request.disposition)
	assert.False(t, request.history)

	request, ok = parseMyBooksBrowseRequest(&url.URL{RawQuery: "q=title&history=read"})
	assert.True(t, ok)
	assert.Empty(t, request.disposition)
	assert.True(t, request.history)
	assert.Equal(t, "/library?history=read&q=title", myBooksScopedURL("title", 1, "", true))

	for _, raw := range []string{"needs-language", "disposition=inbox", "disposition=invalid", "disposition=to_read&history=read"} {
		_, ok = parseMyBooksBrowseRequest(&url.URL{RawQuery: raw})
		assert.False(t, ok, "retired request %q is unsupported", raw)
	}
}

func TestMyBooksTabsRenderOnlyToReadAndRead(t *testing.T) {
	state := MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", AllCount: 6, ScopeTotal: 6, Total: 2, Disposition: domain.BookDispositionToRead, ToReadCount: 3, ReadCount: 1}
	var output bytes.Buffer
	books := []domain.MyBook{
		{Book: domain.Book{ID: "to-read", OwnerID: "owner", Title: "To read", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, Disposition: domain.BookDispositionToRead},
		{Book: domain.Book{ID: "read-book", OwnerID: "owner", Title: "Read book", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, Disposition: domain.BookDispositionInbox, CompletionCount: 1},
	}
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", state).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, `aria-current="page" href="/library?disposition=to_read"`)
	assert.Contains(t, html, `href="/library?history=read"`)
	assert.NotContains(t, html, "Inbox")
	assert.NotContains(t, html, "All (")
	assert.NotContains(t, html, "Set Aside")
	assert.Contains(t, html, "To Read (3)")
	assert.Contains(t, html, "Read (1)")
	filtersStart := strings.Index(html, `<nav class="library-filters"`)
	require.GreaterOrEqual(t, filtersStart, 0)
	filtersEnd := strings.Index(html[filtersStart:], `</nav>`)
	require.Positive(t, filtersEnd)
	filters := html[filtersStart : filtersStart+filtersEnd]
	assert.NotContains(t, filters, "hx-get=")
}

func TestLibraryShowsCurrentReadingInToReadWithDistinctLabel(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	current, err := store.GetCurrentReading(t.Context(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	_, err = store.FinishCurrentReading(t.Context(), fixtures.OwnerID, "de", current.BookID, current.SnapshotID)
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(t.Context(), fixtures.OwnerID, current.BookID, domain.BookDispositionToRead))
	_, err = store.StartCurrentReading(t.Context(), fixtures.OwnerID, "de", current.BookID)
	require.NoError(t, err)
	handler := requireHandler(t, h)
	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}

	all := request("/library")
	require.Equal(t, http.StatusOK, all.Code)
	assert.Contains(t, all.Body.String(), "Currently reading")
	assert.Contains(t, all.Body.String(), "Reading history")

	toRead := request("/library?disposition=to_read")
	require.Equal(t, http.StatusOK, toRead.Code)
	assert.Contains(t, toRead.Body.String(), "To Read (16)")
	assert.Contains(t, toRead.Body.String(), `id="book-row-`+fixtures.BookID+`"`)
	assert.Contains(t, toRead.Body.String(), "Currently reading")
}

func TestMyBooksDispositionTransitionsAreIdempotent(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	bookState, err := store.GetBookDetail(context.Background(), fixtures.OwnerID, fixtures.BookID)
	require.NoError(t, err)
	retiredCurrent := readingTestRequest(t, h, "/library/books/"+fixtures.BookID+"/set-aside", url.Values{
		"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(bookState.DispositionRevision, 10)},
	}, cookies)
	assert.Equal(t, http.StatusNotFound, retiredCurrent.Code, "retired Set Aside route is not served, even for the current reading")
	assert.Equal(t, domain.BookDispositionToRead, mustFixtureBookDisposition(t, store, fixtures.OwnerID, fixtures.BookID))

	bookState, err = store.GetBookDetail(context.Background(), fixtures.OwnerID, "fixture-failed")
	require.NoError(t, err)
	retired := readingTestRequest(t, h, "/library/books/fixture-failed/set-aside", url.Values{
		"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(bookState.DispositionRevision, 10)},
	}, cookies)
	assert.Equal(t, http.StatusNotFound, retired.Code)
	assert.Equal(t, domain.BookDispositionToRead, mustFixtureBookDisposition(t, store, fixtures.OwnerID, "fixture-failed"), "retired route changed disposition")
	toRead := readingTestRequest(t, h, "/library/books/fixture-failed/to-read", url.Values{
		"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(bookState.DispositionRevision, 10)},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, toRead.Code)
	assert.NotContains(t, toRead.Header().Get("Location"), "error=")
	assert.Contains(t, toRead.Header().Get("Location"), "disposition=to_read")
	disposition, err := store.GetBookDisposition(context.Background(), fixtures.OwnerID, "fixture-failed")
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition)

	repeated := readingTestRequest(t, h, "/library/books/fixture-failed/to-read", url.Values{
		"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(bookState.DispositionRevision, 10)},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, repeated.Code)
	disposition, err = store.GetBookDisposition(context.Background(), fixtures.OwnerID, "fixture-failed")
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition, "repeated disposition write changed state")
}

func mustFixtureBookDisposition(t *testing.T, store *fixtures.Store, owner, bookID string) domain.BookDisposition {
	t.Helper()
	disposition, err := store.GetBookDisposition(context.Background(), owner, bookID)
	require.NoError(t, err)
	return disposition
}

func TestEmptyMyBooksLeadsToTheCatalog(t *testing.T) {
	var output bytes.Buffer
	state := MyBooksBrowseState{Enabled: true}
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", state).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, "Find your first Book", "empty My Books state lost its Catalog path: %s", html)
	assert.Contains(t, html, `href="/catalogs">Browse the Catalog</a>`, "empty My Books state lost its Catalog path: %s", html)
}

func TestLibraryHandlerOmitsRetiredLanguageView(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	handler := requireHandler(t, h)
	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}

	handler.services.AnalysisInsights = fixtures.Insights{}
	response := request("/library")
	assert.Equal(t, http.StatusOK, response.Code)
	for _, forbidden := range []string{`id="language-view-panel"`, "Coverage across", "Language view", "Highest-impact unknown vocabulary", "Per-book spread"} {
		assert.NotContains(t, response.Body.String(), forbidden, "retired language view markup remains %q: %s", forbidden, response.Body.String())
	}
}

// libraryExtrasFailStore fails one of the My Books reads that decorate the
// result rows after the browse itself has succeeded.
type libraryExtrasFailStore struct {
	*browseRecordingStore
	connectionsErr error
}

func (s libraryExtrasFailStore) ListOpdsConnections(ctx context.Context, owner string) ([]domain.OpdsConnection, error) {
	if s.connectionsErr != nil {
		return nil, s.connectionsErr
	}
	return s.browseRecordingStore.ListOpdsConnections(ctx, owner)
}

func TestLibraryHandlerDoesNotReadCatalogConnections(t *testing.T) {
	h, cookies, _, fixtureStore := readingFixtureSession(t)
	handler := requireHandler(t, h)
	base := &browseRecordingStore{Store: fixtureStore, result: persistence.MyBooksBrowseResult{
		Items: []domain.MyBook{{Book: domain.Book{ID: "book-1", OwnerID: fixtures.OwnerID, Title: "Book 1", LanguageState: domain.LanguageChosen, LanguageTag: "de"}}},
		Total: 1, ScopeTotal: 1, AllCount: 1,
	}}
	handler.services.Store = storeDependencies(libraryExtrasFailStore{browseRecordingStore: base, connectionsErr: errors.New("connections must not be read")})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/library", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.NotContains(t, response.Body.String(), myBooksLoadFailureMessage)
}

func TestLibraryHandlerClampsAnEmptyScopeToItsFirstPage(t *testing.T) {
	h, cookies, _, fixtureStore := readingFixtureSession(t)
	handler := requireHandler(t, h)
	handler.services.Store = storeDependencies(&browseRecordingStore{Store: fixtureStore, result: persistence.MyBooksBrowseResult{Total: 0, ScopeTotal: 0}})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/library?page=2", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/library?disposition=to_read", response.Header().Get("Location"))
}

func TestLibraryHandlerClampsAnEmptyFilteredScopeToItsFirstPage(t *testing.T) {
	h, cookies, _, fixtureStore := readingFixtureSession(t)
	handler := requireHandler(t, h)
	handler.services.Store = storeDependencies(&browseRecordingStore{Store: fixtureStore, result: persistence.MyBooksBrowseResult{Total: 0, ScopeTotal: 0}})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/library?page=2&q=Dampf", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/library?disposition=to_read&q=Dampf", response.Header().Get("Location"))
}

// The To Read tab can be empty while the language scope still counts Read
// Books. The clamp must recover to the tab's own first page rather than the
// scope's last page, which would repeat the requested URL forever.
func TestLibraryHandlerClampsAnEmptyTabToItsFirstPageDespiteScopeCounts(t *testing.T) {
	h, cookies, _, fixtureStore := readingFixtureSession(t)
	handler := requireHandler(t, h)
	handler.services.Store = storeDependencies(&browseRecordingStore{Store: fixtureStore, result: persistence.MyBooksBrowseResult{Total: 0, ScopeTotal: 26, AllCount: 26}})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/library?disposition=to_read&page=2", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/library?disposition=to_read", response.Header().Get("Location"))
}

func TestLibraryHandlerDropsAnExplicitLanguageParameter(t *testing.T) {
	h, cookies, _, fixtureStore := readingFixtureSession(t)
	handler := requireHandler(t, h)
	handler.services.Store = storeDependencies(&browseRecordingStore{Store: fixtureStore})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/library?language=de&q=Dampf", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/library?disposition=to_read&q=Dampf", response.Header().Get("Location"))
}
