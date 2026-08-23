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

func TestLanguageOptionsPutStudyLanguagesFirst(t *testing.T) {
	profiles := []domain.LanguageProfile{{Language: "de", DisplayName: "German"}}
	feed := opds.Feed{Entries: []opds.Entry{
		{Title: "Spanish", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/4"}}},
		{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/opds/language/7"}}},
	}}
	options := languageOptions(profiles, feed)
	if len(options) != 2 || options[0].ID != "7" || !options[0].Study || options[1].Study {
		t.Fatalf("options=%+v", options)
	}
	var output bytes.Buffer
	if err := CatalogPage(domain.User{}, "csrf", domain.OpdsConnection{ID: "connection-1", Name: "Library"}, options).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Browse EPUBs by language", "German — study language", `value="7"`, "/opds/language"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("catalog page missing %q: %s", want, output.String())
		}
	}
}

func TestLanguageResultsShowOnlyProvidedEPUBEntries(t *testing.T) {
	feed := opds.Feed{Title: "German", Entries: []opds.Entry{{ID: "book", Title: "Book", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}}
	var output bytes.Buffer
	if err := LanguageResults("csrf", "connection-1", feed).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"German", "Showing EPUB editions only", "Book", "Import &amp; analyze"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("language results missing %q: %s", want, output.String())
		}
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
