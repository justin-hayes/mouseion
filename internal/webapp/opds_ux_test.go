package webapp

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
)

func TestBrowseURLRoundTripsBreadcrumbTrail(t *testing.T) {
	want := []CatalogCrumb{{Title: "Authors", URL: "https://catalog.example/authors"}, {Title: "A–C", URL: "https://catalog.example/a-c"}}
	request := httptest.NewRequest("GET", browseURL("connection-1", "https://catalog.example/books", want), nil)
	got := decodeTrail(request.URL.Query()["trail"])
	if len(got) != len(want) {
		t.Fatalf("trail=%+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("trail[%d]=%+v want %+v", i, got[i], want[i])
		}
	}
}

func TestCatalogUsesReadyNLPCodeSeparatelyFromCatalogLanguageID(t *testing.T) {
	feed := opds.Feed{Entries: []opds.Entry{
		{Title: "Spanish", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/4"}}},
		{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/7"}}},
	}}
	capability := domain.SupportedLanguage{Language: "de", DisplayName: "German"}
	if got := catalogLanguageID(capability, feed); got != "7" {
		t.Fatalf("catalog language ID=%q", got)
	}
	var output bytes.Buffer
	if err := CatalogPage(domain.User{}, "csrf", domain.OpdsConnection{ID: "connection-1", Name: "Library"}, []domain.SupportedLanguage{capability}, false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Browse EPUBs by language", "German", `value="de"`, "/opds/language"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("catalog page missing %q: %s", want, output.String())
		}
	}
}

func TestConnectionFormsHaveNoBookLanguageField(t *testing.T) {
	var output bytes.Buffer
	connections := []domain.OpdsConnection{{ID: "connection-1", Name: "Library", URL: "https://catalog.example/opds"}}
	if err := ConnectionsPage(domain.User{}, "csrf", connections, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if strings.Contains(html, "Book language") || strings.Contains(html, `name="language"`) {
		t.Fatalf("connection forms still contain a language field: %s", html)
	}
}

func TestLanguageResultsShowOnlyProvidedEPUBEntries(t *testing.T) {
	feed := opds.Feed{Title: "German", Entries: []opds.Entry{{ID: "book", Title: "Book", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}}
	var output bytes.Buffer
	if err := LanguageResults("csrf", "connection-1", "de", feed).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"German", "Showing EPUB editions only", "Book", "Import &amp; analyze"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("language results missing %q: %s", want, output.String())
		}
	}
	if !strings.Contains(output.String(), `name="language" value="de"`) {
		t.Fatalf("language results did not carry NLP code: %s", output.String())
	}
}

func TestCatalogRootFragmentShowsSearchWithoutCategories(t *testing.T) {
	feed := opds.Feed{Title: "Catalog", Links: []opds.Link{{Rel: "search", Href: "https://catalog.example/search{?q}"}}, Entries: []opds.Entry{{Title: "Authors"}, {Title: "Newest books"}}}
	var output bytes.Buffer
	if err := CatalogRootFragment("connection-1", feed).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, "Search this catalog") {
		t.Errorf("root fragment missing search: %s", html)
	}
	for _, unwanted := range []string{"Catalog root", "Authors", "Newest books"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("root fragment unexpectedly contains %q: %s", unwanted, html)
		}
	}
}

func TestFeedFragmentShowsBreadcrumbsAndEmptyState(t *testing.T) {
	feed := opds.Feed{Title: "A–C", Links: []opds.Link{{Rel: "search", Href: "https://catalog.example/search{?q}"}}}
	trail := []CatalogCrumb{{Title: "Authors", URL: "https://catalog.example/authors"}, {Title: "A–C", URL: "https://catalog.example/a-c"}}
	var output bytes.Buffer
	if err := FeedFragment("csrf", "connection-1", feed, trail).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Catalog root", "Authors", `aria-current="page"`, "No books or collections were found here"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered fragment missing %q: %s", want, html)
		}
	}
}

func TestOPDSErrorsAreActionable(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{errors.New("opds: HTTP 401 Unauthorized"), "rejected the credentials"},
		{errors.New("opds: parse Atom feed: unexpected EOF"), "web page instead of an OPDS feed"},
		{opds.ErrNoEPUB, "not available as an EPUB"},
		{errors.New("opds: fetch feed: dial tcp: connection refused"), "could not be reached"},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		opdsFail(response, test.err)
		if response.Code != 502 || !strings.Contains(response.Body.String(), test.want) {
			t.Errorf("error %q produced %d %q", test.err, response.Code, response.Body.String())
		}
	}
}

func TestPrepareDeckFormRendersAccessibleAsynchronousWorkflow(t *testing.T) {
	var output bytes.Buffer
	book := domain.SourceMaterialSummary{
		Source:         domain.SourceMaterial{ID: "book-1", Title: "Book", Language: "de", MediaType: "application/epub+zip"},
		AnalysisStatus: "analyzed",
	}
	if err := BookPage(domain.User{Username: "learner"}, "csrf", book, nil, false, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{
		`form[data-prepare-deck]`,
		`action="/books/book-1/deck/preparations"`,
		`name="external_translation_consent"`,
		`data-deck-preparation`,
		`aria-live="polite"`,
		`evt.preventDefault()`,
		`await fetch(form.action`,
		`if (!response.ok)`,
		`window.pollDeckPreparation(response.url`,
		`preparation.state === 'queued'`,
		`preparation.state === 'preparing'`,
		`data-cancel-preparation`,
		`data-retry-preparation`,
		`preparation.completeness`,
		`preparation.deck_name`,
		`preparation.filename`,
		`preparation.download_url`,
		`Download deck`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("deck page missing client behavior %q", want)
		}
	}
	for _, unwanted := range []string{`action="/books/book-1/deck"`, `await response.blob()`, `URL.createObjectURL`, `The deck downloads immediately`} {
		if strings.Contains(html, unwanted) {
			t.Errorf("deck page still contains legacy immediate-download behavior %q", unwanted)
		}
	}
}

func TestAnalyzedBookCoverageSummaryExplainsMetrics(t *testing.T) {
	var output bytes.Buffer
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-1", Title: "Book", Language: "de"}, AnalysisStatus: "analyzed"}
	coverage := domain.AnalysisCoverage{
		AnalyzableTokenCount: 40, DistinctLemmaCount: 12, KnownTokenCount: 30, KnownLemmaCount: 7, UnknownTokenCount: 10, UnknownLemmaCount: 5,
		Thresholds: []domain.CoverageThreshold{{TargetPercent: 95, LemmaCount: 3}, {TargetPercent: 97, LemmaCount: 4}, {TargetPercent: 99, LemmaCount: 5}},
	}
	if err := BookPage(domain.User{Username: "learner"}, "csrf", book, &coverage, false, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"75.0%", "current token coverage", "analyzable tokens", "distinct lemmas", "explicitly known vocabulary", "unknown vocabulary", "lemmas for 95%", "lemmas for 97%", "lemmas for 99%", "Previously generated vocabulary", "deck-eligible vocabulary"} {
		if !strings.Contains(html, want) {
			t.Errorf("coverage summary missing %q", want)
		}
	}
	if strings.Contains(html, "top unknown") || strings.Contains(html, "structural difficulty") {
		t.Errorf("coverage summary includes deferred insights: %s", html)
	}
}
