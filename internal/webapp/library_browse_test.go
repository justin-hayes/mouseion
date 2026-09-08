package webapp

import (
	"bytes"
	"context"
	"errors"
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
	state := MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", AllCount: 4, Total: 2, Page: 2, PageCount: 2}
	var output bytes.Buffer
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "", false, state).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{
		`role="search"`, `label for="library-search-query">Search My Books`,
		`id="library-results"`, `data-focus-id="library-books-heading"`, `Page 2 of 2`,
		`href="/library"`, `href="/library?page=2"`, `href="/books/de-book"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("browse markup missing %q: %s", want, html)
		}
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
}

func TestMyBooksBrowseRequestDefaults(t *testing.T) {
	query, page := parseMyBooksBrowseRequest(&url.URL{RawQuery: "q=+title+&language=de&page=-4"})
	if query != "title" || page != 1 {
		t.Fatalf("parsed browse state=%q,%d", query, page)
	}
}

func TestMyBooksWithoutActiveLanguageKeepsCatalogueSetupAction(t *testing.T) {
	var output bytes.Buffer
	state := MyBooksBrowseState{Enabled: true}
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", false, state).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, "Build My Books from your catalogue") || !strings.Contains(html, `href="/connections"`) {
		t.Fatalf("empty unscoped My Books state lost catalogue setup: %s", html)
	}
}

func TestMyBooksLanguageViewRendersFourEvidenceRegionsAndBookLinks(t *testing.T) {
	panel, err := buildLanguageCorpusPanel(context.Background(), fixtures.Insights{}, []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}}, fixtures.OwnerID, "de")
	if err != nil {
		t.Fatal(err)
	}
	state := MyBooksBrowseState{
		Enabled:        true,
		Language:       "de",
		LanguageLabel:  "German",
		AllCount:       1,
		Total:          1,
		Page:           1,
		PageCount:      1,
		LanguageCorpus: &panel,
	}
	var output bytes.Buffer
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", false, state).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{
		"Coverage across German",
		`id="language-view-analyzed-heading"`,
		`id="language-view-coverage-heading"`,
		`id="language-view-unknown-heading"`,
		`id="language-view-books-heading"`,
		"37.0%",
		"45678 of 123456 tokens",
		"Highest-impact unknown vocabulary",
		"analysis failed or incomplete",
		`href="/books/fixture-book"`,
		`href="/books/fixture-failed"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("language view markup missing %q: %s", want, html)
		}
	}
	panelHTML := html[strings.Index(html, `id="language-view-panel"`):]
	if strings.Contains(panelHTML, "<button") || strings.Contains(panelHTML, `action=`) {
		t.Fatalf("language view exposed an action: %s", panelHTML)
	}
	if strings.Count(panelHTML, `aria-live="polite"`) != 1 {
		t.Fatalf("language view should have one scoped live announcement: %s", panelHTML)
	}
}

type languageCorpusErrorInsights struct{}

func (languageCorpusErrorInsights) Coverage(context.Context, string, string) (domain.AnalysisCoverage, error) {
	return domain.AnalysisCoverage{}, nil
}

func (languageCorpusErrorInsights) LanguageCorpus(context.Context, string, string) (domain.LanguageCorpusView, error) {
	return domain.LanguageCorpusView{}, errors.New("fixture language view failed")
}

func TestLibraryHandlerLanguageViewCapabilityAndFailureFallback(t *testing.T) {
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

	if response := request("/library"); strings.Contains(response.Body.String(), `id="language-view-panel"`) {
		t.Fatal("language view rendered without the optional provider capability")
	}
	handler.services.AnalysisInsights = fixtures.Insights{JourneyStore: store}
	if response := request("/library?language=de"); response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/library" {
		t.Fatalf("legacy language URL status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	if response := request("/library"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `id="language-view-panel"`) {
		t.Fatalf("active language view status=%d body=%s", response.Code, response.Body.String())
	}
	if response := request("/library?language=unknown"); response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/library" {
		t.Fatalf("unknown-language URL status=%d location=%q", response.Code, response.Header().Get("Location"))
	}

	handler.services.AnalysisInsights = languageCorpusErrorInsights{}
	response := request("/library")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Current analyzed evidence could not be loaded") || !strings.Contains(response.Body.String(), "Language view for German is unavailable") {
		t.Fatalf("unavailable language view status=%d body=%s", response.Code, response.Body.String())
	}
}
