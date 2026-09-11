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
)

func TestMyBooksBrowseControlsRenderScopedSearchAndPaging(t *testing.T) {
	books := []domain.MyBook{
		{Book: domain.Book{ID: "de-book", OwnerID: "owner", Title: "De book", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "unknown-book", OwnerID: "owner", Title: "Unknown book", LanguageState: domain.LanguageUnknown}},
	}
	state := MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", NeedsLanguageCount: 1, AllCount: 4, Total: 2, Page: 2, PageCount: 2}
	var output bytes.Buffer
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "", false, state).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{
		`role="search"`, `label for="library-search-query">Search My Books`,
		`id="library-results"`, `data-focus-id="library-books-heading"`, `Page 2 of 2`, `href="/library?needs-language"`,
		`href="/library"`, `href="/library?page=2"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("browse markup missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, `href="/books/de-book"`) {
		t.Fatalf("My Books browse linked to the retired Book detail page: %s", html)
	}
	for _, forbidden := range []string{"All languages", "Unknown language", `aria-label="Languages"`, `name="language"`, `language: de`, "language not chosen", `href="/books/unknown-book"`} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("legacy language browse markup remains %q: %s", forbidden, html)
		}
	}
	if !strings.Contains(html, `hx-push-url="true"`) || !strings.Contains(html, `hx-target="#library-results"`) {
		t.Fatalf("browse controls are missing HTMX URL enhancement: %s", html)
	}
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
			if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", false, tc.state).Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			html := output.String()
			for _, want := range tc.wants {
				if !strings.Contains(html, want) {
					t.Errorf("no-match markup missing %q: %s", want, html)
				}
			}
			if strings.Count(html, `role="status"`) != 1 {
				t.Fatalf("no-match state should have one scoped live region: %s", html)
			}
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
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
		if store.owner != fixtures.OwnerID || store.offset != 0 || store.limit != myBooksPageSize {
			t.Fatalf("GET %s browse call owner=%q offset=%d limit=%d", path, store.owner, store.offset, store.limit)
		}
	}
	response := request("/library?q=%20Dampf%20&page=3")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/library?page=2&q=Dampf" {
		t.Fatalf("stale page status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	if store.query != "Dampf" || store.language != "de" || store.offset != 50 {
		t.Fatalf("browse filters were not passed through: query=%q language=%q offset=%d", store.query, store.language, store.offset)
	}
	legacy := request("/library?language=it&q=Dampf")
	if legacy.Code != http.StatusSeeOther || legacy.Header().Get("Location") != "/library?q=Dampf" {
		t.Fatalf("legacy language URL status=%d location=%q", legacy.Code, legacy.Header().Get("Location"))
	}
	htmxRequest := httptest.NewRequest(http.MethodGet, "/library?q=Dampf", nil)
	htmxRequest.Header.Set("HX-Request", "true")
	for _, cookie := range cookies {
		htmxRequest.AddCookie(cookie)
	}
	htmxResponse := httptest.NewRecorder()
	handler.ServeHTTP(htmxResponse, htmxRequest)
	if htmxResponse.Code != http.StatusOK || !strings.Contains(htmxResponse.Body.String(), `<section id="library-results"`) || strings.Contains(htmxResponse.Body.String(), "<!doctype html>") {
		t.Fatalf("HTMX library response was not a results fragment: status=%d body=%s", htmxResponse.Code, htmxResponse.Body.String())
	}
	store.result.Total = 0
	store.result.ScopeTotal = 26
	response = request("/library?page=3")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/library?page=2" {
		t.Fatalf("empty stale page status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	store.result = persistence.MyBooksBrowseResult{
		Items: []domain.MyBook{{Book: domain.Book{ID: "unknown-book", OwnerID: fixtures.OwnerID, Title: "Unknown Book", LanguageState: domain.LanguageUnknown}}},
		Total: 1, ScopeTotal: 1, AllCount: 26, Counts: []persistence.LanguageCount{{Tag: domain.LanguageUnknown, Count: 1}},
	}
	response = request("/library?needs-language")
	if response.Code != http.StatusOK || store.language != domain.LanguageUnknown || !strings.Contains(response.Body.String(), "Books awaiting a language") || !strings.Contains(response.Body.String(), "Unknown Book") {
		t.Fatalf("needs-language browse status=%d language=%q body=%s", response.Code, store.language, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `href="/books/unknown-book"`) || strings.Contains(response.Body.String(), "Add to Reading Journey") || strings.Contains(response.Body.String(), "Remove from My Books") {
		t.Fatalf("needs-language browse exposed mutation actions: %s", response.Body.String())
	}
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
	if response.Code != http.StatusOK {
		t.Fatalf("GET /library status=%d body=%s", response.Code, response.Body.String())
	}
	html := response.Body.String()
	eligibleStart := strings.Index(html, `id="book-row-fixture-metadata-only"`)
	ineligibleStart := strings.Index(html, `id="book-row-no-alias"`)
	if eligibleStart < 0 || ineligibleStart < 0 {
		t.Fatalf("metadata-only rows missing: %s", html)
	}
	eligibleRow := html[eligibleStart:ineligibleStart]
	ineligibleRow := html[ineligibleStart:]
	if !strings.Contains(eligibleRow, "Refresh metadata") || !strings.Contains(eligibleRow, `hx-target="#book-row-fixture-metadata-only"`) {
		t.Fatalf("eligible metadata-only row omitted refresh: %s", eligibleRow)
	}
	if strings.Contains(ineligibleRow, "Refresh metadata") {
		t.Fatalf("ineligible metadata-only row exposed refresh: %s", ineligibleRow)
	}
}

func TestMyBooksBrowseRequestDefaults(t *testing.T) {
	query, page, needsLanguage := parseMyBooksBrowseRequest(&url.URL{RawQuery: "q=+title+&language=de&page=-4"})
	if query != "title" || page != 1 {
		t.Fatalf("parsed browse state=%q,%d", query, page)
	}
	if needsLanguage {
		t.Fatal("ordinary browse request unexpectedly selected needs-language")
	}
	query, page, needsLanguage = parseMyBooksBrowseRequest(&url.URL{RawQuery: "needs-language&q=title&page=2"})
	if query != "title" || page != 2 || !needsLanguage {
		t.Fatalf("unexpected needs-language browse state: query=%q page=%d needs=%t", query, page, needsLanguage)
	}
	if got := myBooksURL("title", 2, true); got != "/library?needs-language&page=2&q=title" {
		t.Fatalf("unexpected needs-language URL: %q", got)
	}
}

func TestMyBooksWithoutActiveLanguageKeepsCatalogSetupAction(t *testing.T) {
	var output bytes.Buffer
	state := MyBooksBrowseState{Enabled: true}
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", false, state).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, "Build My Books from your catalogue") || !strings.Contains(html, `href="/catalogs"`) {
		t.Fatalf("empty unscoped My Books state lost catalogue setup: %s", html)
	}
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
	if response.Code != http.StatusOK {
		t.Fatalf("My Books status=%d body=%s", response.Code, response.Body.String())
	}
	for _, forbidden := range []string{`id="language-view-panel"`, "Coverage across", "Language view", "Highest-impact unknown vocabulary", "Per-book spread"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("retired language view markup remains %q: %s", forbidden, response.Body.String())
		}
	}
}
