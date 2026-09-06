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
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/opds"
)

func TestMyBooksMetadataOnlyRowExposesOnlySupportedActions(t *testing.T) {
	book := domain.MyBook{Book: domain.Book{ID: "metadata-book", OwnerID: "owner", Title: "A book without an EPUB", LanguageState: domain.LanguageUnknown}, EvidenceState: domain.MyBookNotAcquired}
	var output bytes.Buffer
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if main := strings.Index(html, "<main"); main >= 0 {
		html = html[main:]
	}
	for _, want := range []string{"A book without an EPUB", `href="/books/metadata-book"`, "Not acquired", "Open book", "Remove from My Books", `action="/library/books/metadata-book/remove"`, `action="/library/books/metadata-book"`, "Fix book language", `name="language_state"`, "language not chosen"} {
		if !strings.Contains(html, want) {
			t.Errorf("metadata-only My Books row missing %q: %s", want, html)
		}
	}
	row := html
	if start := strings.Index(row, `aria-labelledby="book-title-metadata-book"`); start >= 0 {
		row = row[start:]
		if end := strings.Index(row, "</article>"); end >= 0 {
			row = row[:end+len("</article>")]
		}
	}
	for _, forbidden := range []string{"Review scope", "Start analysis", "Prepare deck", "View analysis result", "coverage"} {
		if strings.Contains(row, forbidden) {
			t.Errorf("metadata-only My Books row exposed unsupported action %q: %s", forbidden, row)
		}
	}
}

func TestBookLanguageCanBeCorrectedFromMyBooks(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	response := goalRequest(t, h, "/library/books/fixture-empty", url.Values{
		"csrf_token": {csrf}, "language_state": {domain.LanguageChosen}, "language_tag": {"fr-FR"},
	}, cookies)
	if response.Code != http.StatusSeeOther || !strings.Contains(response.Header().Get("Location"), "Book+language+updated") {
		t.Fatalf("update status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	languages, err := store.ListStudyLanguages(context.Background(), fixtures.OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range languages {
		if language.Language == "fr" {
			return
		}
	}
	t.Fatalf("corrected language missing from derived set: %+v", languages)
}

func TestMyBooksEvidenceStatesRemainDistinct(t *testing.T) {
	states := []domain.MyBookEvidenceState{
		domain.MyBookUnavailable,
		domain.MyBookNotAcquired,
		domain.MyBookAcquiredUnassessed,
		domain.MyBookAnalyzed,
		domain.MyBookStale,
	}
	books := make([]domain.MyBook, 0, len(states))
	for i, state := range states {
		book := domain.MyBook{Book: domain.Book{ID: "book-" + string(rune('a'+i)), OwnerID: "owner", Title: "Book " + string(rune('A'+i)), LanguageState: domain.LanguageChosen, LanguageTag: "de"}, EvidenceState: state}
		if state != domain.MyBookNotAcquired {
			book.Acquired = &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-" + book.Book.ID, OwnerID: "owner", Title: book.Book.Title, Language: "de", MediaType: "application/epub+zip", ContentRevisionID: "revision"}, BookID: book.Book.ID, AnalysisStatus: "not analyzed"}
		}
		books = append(books, book)
	}
	var output bytes.Buffer
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, label := range []string{"Unavailable", "Not acquired", "Acquired — unassessed", "Analyzed", "Stale analysis"} {
		if strings.Count(html, label) != 1 {
			t.Errorf("evidence label %q count=%d", label, strings.Count(html, label))
		}
	}
	if !strings.Contains(html, `href="/books/book-d"`) || strings.Contains(html, `href="/books/source-book-d"`) {
		t.Fatalf("learner My Books links were not canonical: %s", html)
	}
}

func TestMyBooksEmptyOnboardingGuidesConnectionLanguageAndSync(t *testing.T) {
	var output bytes.Buffer
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", false, MyBooksBrowseState{}).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Connect a catalogue", `href="/connections"`, "acquire EPUB content"} {
		if !strings.Contains(html, want) {
			t.Errorf("empty My Books onboarding missing %q: %s", want, html)
		}
	}

	output.Reset()
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", true, MyBooksBrowseState{}).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Sync catalogue") {
		t.Fatalf("connected empty state omitted sync guidance: %s", output.String())
	}
}

func TestUpstreamBrowserRoutesAreRetired(t *testing.T) {
	h, cookies, _, _ := goalFixtureSession(t)
	for _, route := range []string{"/catalog", "/opds/browse", "/opds/language", "/opds/search"} {
		r := httptest.NewRequest(http.MethodGet, route, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		h.ServeHTTP(response, r)
		if response.Code != http.StatusNotFound {
			t.Errorf("GET %s status=%d, want 404", route, response.Code)
		}
	}
}

func TestMetadataOnlyBookPageDoesNotExposeContentActions(t *testing.T) {
	book := domain.MyBook{Book: domain.Book{ID: "metadata-book", OwnerID: "owner", Title: "Catalogue metadata", LanguageState: domain.LanguageUnknown}, EvidenceState: domain.MyBookNotAcquired}
	var output bytes.Buffer
	if err := MetadataOnlyBookPage(domain.User{Username: "learner"}, "csrf", book, "", false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if main := strings.Index(html, "<main"); main >= 0 {
		html = html[main:]
	}
	for _, want := range []string{"Catalogue metadata", "Metadata only", "Content not acquired", "Analysis and deck preparation are unavailable"} {
		if !strings.Contains(html, want) {
			t.Errorf("metadata-only page missing %q: %s", want, html)
		}
	}
	for _, forbidden := range []string{"Review scope", "Start analysis", "Prepare deck", "Refresh metadata", `action=\"/books/metadata-book/analyze\"`} {
		if strings.Contains(html, forbidden) {
			t.Errorf("metadata-only page exposed unsupported action %q: %s", forbidden, html)
		}
	}
}

func TestMetadataOnlyBookPageExposesCatalogueMetadataRefresh(t *testing.T) {
	book := domain.MyBook{Book: domain.Book{ID: "catalogue-book", OwnerID: "owner", Title: "Catalogue metadata", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, EvidenceState: domain.MyBookNotAcquired}
	var output bytes.Buffer
	if err := MetadataOnlyBookPage(domain.User{Username: "learner"}, "csrf", book, "Metadata refreshed.", true).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{`id="book-metadata-region"`, "Refresh metadata", `method="post"`, `action="/books/catalogue-book/refresh"`, `name="csrf_token"`, `hx-post="/books/catalogue-book/refresh"`, `hx-target="#book-metadata-region"`, `aria-live="polite"`, "Metadata refreshed."} {
		if !strings.Contains(html, want) {
			t.Errorf("catalogue metadata refresh page missing %q: %s", want, html)
		}
	}
}

func TestMetadataOnlyBookPageOffersPerBookAcquisition(t *testing.T) {
	book := domain.MyBook{Book: domain.Book{ID: "catalogue-book", OwnerID: "owner", Title: "Catalogue metadata", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, EvidenceState: domain.MyBookNotAcquired}
	target := &cataloguesync.AcquisitionTarget{ConnectionID: "connection-1", Language: "de", Entry: opds.Entry{ID: "entry-1", Title: book.Book.Title}, Href: "https://catalog.example/book.epub"}
	var output bytes.Buffer
	if err := MetadataOnlyBookPageWithAcquisition(domain.User{Username: "learner"}, "csrf", book, "", false, target, "signed-target").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{`action="/opds/acquire"`, "Acquire EPUB content", `name="acquisition" value="signed-target"`, `name="return_to" value="/books/catalogue-book"`, `name="connection" value="connection-1"`} {
		if !strings.Contains(html, want) {
			t.Errorf("per-book acquisition form missing %q: %s", want, html)
		}
	}
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
	h.(*Handler).services.CatalogueSync = stub
	missingCSRF := goalRequest(t, h, "/books/fixture-metadata-only/refresh", url.Values{}, cookies)
	if missingCSRF.Code != http.StatusForbidden || stub.calls != 0 {
		t.Fatalf("missing CSRF status=%d calls=%d", missingCSRF.Code, stub.calls)
	}
	native := goalRequest(t, h, "/books/fixture-metadata-only/refresh", url.Values{"csrf_token": {csrf}}, cookies)
	if native.Code != http.StatusSeeOther || !strings.Contains(native.Header().Get("Location"), "Metadata+refreshed") || stub.owner != "fixture-learner" {
		t.Fatalf("native refresh status=%d location=%q owner=%q", native.Code, native.Header().Get("Location"), stub.owner)
	}

	h, cookies, csrf, _ = goalFixtureSession(t)
	stub = &bookRefreshStub{result: cataloguesync.RefreshResult{Book: domain.Book{ID: "fixture-metadata-only", OwnerID: "fixture-learner", Title: "Updated catalogue title"}, Missing: true}}
	h.(*Handler).services.CatalogueSync = stub
	request := httptest.NewRequest(http.MethodPost, "/books/fixture-metadata-only/refresh", strings.NewReader(url.Values{"csrf_token": {csrf}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `id="book-metadata-region"`) || !strings.Contains(response.Body.String(), "catalogue entry is no longer available") {
		t.Fatalf("HTMX refresh status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestBookIDFromReturnPathRejectsExternalTargets(t *testing.T) {
	if got := bookIDFromReturnPath("/books/book-1"); got != "book-1" {
		t.Fatalf("book id=%q", got)
	}
	if got := bookIDFromReturnPath("https://evil.example/?book_id=other"); got != "" {
		t.Fatalf("external return path yielded book id=%q", got)
	}
}
