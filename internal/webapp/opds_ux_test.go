package webapp

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestFeedFragmentShowsSearchBreadcrumbsAndEmptyState(t *testing.T) {
	feed := opds.Feed{Title: "A–C", Links: []opds.Link{{Rel: "search", Href: "https://catalog.example/search{?q}"}}}
	trail := []CatalogCrumb{{Title: "Authors", URL: "https://catalog.example/authors"}, {Title: "A–C", URL: "https://catalog.example/a-c"}}
	var output bytes.Buffer
	if err := FeedFragment("csrf", "connection-1", feed, trail, true).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Search this catalog", "Catalog root", "Authors", `aria-current="page"`, "No books or collections were found here"} {
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
