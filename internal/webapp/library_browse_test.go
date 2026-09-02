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

func TestMyBooksBrowseControlsRenderAccessiblePillsAndPaging(t *testing.T) {
	books := []domain.MyBook{
		{Book: domain.Book{ID: "de-book", OwnerID: "owner", Title: "De book", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "unknown-book", OwnerID: "owner", Title: "Unknown book", LanguageState: domain.LanguageUnknown}},
	}
	state := MyBooksBrowseState{Enabled: true, Language: "de", Counts: []MyBooksLanguageCount{{Tag: "de", Count: 2}, {Tag: "it", Count: 1}, {Tag: domain.LanguageUnknown, Count: 1}}, AllCount: 4, Total: 2, Page: 2, PageCount: 2}
	var output bytes.Buffer
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "", false, state).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{
		`role="search"`, `label for="library-search-query">Search My Books`, `name="language" value="de"`,
		`aria-label="Languages"`, `aria-current="page"`, "de (2) (selected)", "it (1)", "Unknown language (1)",
		`id="library-results"`, `data-focus-id="library-books-heading"`, `Page 2 of 2`,
		`href="/library?language=de"`, `href="/library?language=de&amp;page=2"`, `href="/books/de-book"`, `href="/books/unknown-book"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("browse markup missing %q: %s", want, html)
		}
	}
	if strings.Index(html, "All languages") > strings.Index(html, "Unknown language") {
		t.Fatal("language pills are not ordered with All first and Unknown last")
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
		{name: "text", state: MyBooksBrowseState{Enabled: true, Query: "missing title", AllCount: 3, TextNoMatch: true}, wants: []string{"No books in your local collection match “missing title”", "Clear search", `href="/library"`}},
		{name: "combined", state: MyBooksBrowseState{Enabled: true, Query: "missing title", Language: "de", AllCount: 3, CombinedNoMatch: true}, wants: []string{"No books in your local collection match “missing title” with language “de”", "Clear filters", `href="/library"`}},
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
	response := request("/library?q=%20Dampf%20&language=DE&page=3")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/library?language=DE&page=2&q=Dampf" {
		t.Fatalf("stale page status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	if store.query != "Dampf" || store.language != "DE" || store.offset != 50 {
		t.Fatalf("browse filters were not passed through: query=%q language=%q offset=%d", store.query, store.language, store.offset)
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
}

func TestMyBooksBrowseRequestDefaults(t *testing.T) {
	query, language, page := parseMyBooksBrowseRequest(&url.URL{RawQuery: "q=+title+&language=de&page=-4"})
	if query != "title" || language != "de" || page != 1 {
		t.Fatalf("parsed browse state=%q,%q,%d", query, language, page)
	}
}
