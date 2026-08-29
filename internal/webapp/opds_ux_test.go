package webapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/riverqueue/river/rivertype"
)

func explicitTestWebKey(purpose string) []byte {
	sum := sha256.Sum256([]byte("issue-374-explicit-test-key-" + purpose))
	return sum[:]
}

type catalogFailureStore struct {
	Store
	connection domain.OpdsConnection
}

func (s catalogFailureStore) GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error) {
	return s.connection, nil
}

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

func TestSearchBackPathDefaultsToSelectedCatalog(t *testing.T) {
	got := searchBackPath("connection-1", "de", "", []CatalogCrumb{{Title: "Authors", URL: "https://catalog.example/authors"}})
	if !strings.Contains(got, "/opds/browse?") || !strings.Contains(got, "connection=connection-1") || !strings.Contains(got, "language=de") || strings.Contains(got, "/connections") {
		t.Fatalf("search fallback=%q", got)
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
	for _, want := range []string{"Browse EPUBs by language", "German", `value="de"`, `method="get"`, `action="/opds/language"`, `hx-get="/opds/language"`} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("catalog page missing %q: %s", want, output.String())
		}
	}
}

func TestConnectionFormsHaveNoBookLanguageField(t *testing.T) {
	var output bytes.Buffer
	connections := []domain.OpdsConnection{{ID: "connection-1", Name: "Library", URL: "https://catalog.example/opds", Username: "reader", Password: "super-secret"}}
	if err := ConnectionsPage(domain.User{}, "csrf", connections, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if strings.Contains(html, "Book language") || strings.Contains(html, `name="language"`) {
		t.Fatalf("connection forms still contain a language field: %s", html)
	}
	if strings.Contains(html, "super-secret") || strings.Contains(html, `name="password" value=`) {
		t.Fatalf("connection secret rendered into the page: %s", html)
	}
	for _, want := range []string{`class="confirmation confirmation--danger"`, "Delete catalog connection", "Confirm deletion"} {
		if !strings.Contains(html, want) {
			t.Errorf("connection deletion missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "window.confirm") || strings.Contains(html, "onsubmit=") {
		t.Fatalf("connection deletion must use the server-rendered confirmation pattern: %s", html)
	}
}

func TestConnectionsPageDistinguishesFirstSetupFromCatalogChoice(t *testing.T) {
	var first bytes.Buffer
	if err := ConnectionsPage(domain.User{}, "csrf", nil, "").Render(context.Background(), &first); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Add your first catalog connection", "A connection tells Mouseion where to look for EPUB books"} {
		if !strings.Contains(first.String(), want) {
			t.Errorf("first-connection page missing %q: %s", want, first.String())
		}
	}

	var configured bytes.Buffer
	connections := []domain.OpdsConnection{
		{ID: "connection-1", Name: "Home library", URL: "https://home.example/opds"},
		{ID: "connection-2", Name: "Work library", URL: "https://work.example/opds"},
	}
	if err := ConnectionsPage(domain.User{}, "csrf", connections, "").Render(context.Background(), &configured); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Choose a catalog to browse", "Catalog maintenance", "Home library", "Work library"} {
		if !strings.Contains(configured.String(), want) {
			t.Errorf("configured-connection page missing %q: %s", want, configured.String())
		}
	}
}

func TestCatalogPageExplainsCapabilityBoundaryWhenNoLanguageIsReady(t *testing.T) {
	var output bytes.Buffer
	if err := CatalogPage(domain.User{}, "csrf", domain.OpdsConnection{ID: "connection-1", Name: "Home library"}, nil, false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"No ready analysis languages", "NLP service", "Return later"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("no-ready-language page missing %q: %s", want, output.String())
		}
	}
}

func TestCatalogPageShowsDegradedReadinessWithoutOfferingBrowse(t *testing.T) {
	var output bytes.Buffer
	if err := CatalogPage(domain.User{}, "csrf", domain.OpdsConnection{ID: "connection-1", Name: "Home library"}, []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}}, true).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Language readiness is degraded") || !strings.Contains(output.String(), "return later") || strings.Contains(output.String(), "Browse language") {
		t.Fatalf("degraded catalog page offered an unsafe browse action: %s", output.String())
	}
}

func TestLanguageResultsShowOnlyProvidedEPUBEntries(t *testing.T) {
	feed := opds.Feed{Title: "German", Entries: []opds.Entry{{ID: "book", Title: "Book", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}}
	var output bytes.Buffer
	if err := LanguageResults("csrf", "connection-1", "de", "/opds/language?connection=connection-1&language=de", "", feed).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"German", "Showing EPUB editions only", "Book", "Add to library", "Analysis starts separately"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("language results missing %q: %s", want, output.String())
		}
	}
	if !strings.Contains(output.String(), `name="language" value="de"`) {
		t.Fatalf("language results did not carry NLP code: %s", output.String())
	}
	if !strings.Contains(output.String(), `name="acquisition"`) || strings.Contains(output.String(), `name="href"`) || strings.Contains(output.String(), `name="entry_id"`) || strings.Contains(output.String(), `name="title"`) {
		t.Fatalf("acquisition form exposes client-controlled target fields: %s", output.String())
	}
}

func TestLanguageResultsProvideSearchAndCatalogReturnPath(t *testing.T) {
	feed := opds.Feed{Title: "German", Links: []opds.Link{{Rel: "search", Href: "https://catalog.example/search{?q}"}}}
	var output bytes.Buffer
	if err := LanguageResults("csrf", "connection-1", "de", "/opds/language?connection=connection-1&language=de", "", feed).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Search this catalog", "name=\"return_to\"", "← Back to catalog", "/opds/browse?connection=connection-1"} {
		if !strings.Contains(html, want) {
			t.Errorf("language results missing %q: %s", want, html)
		}
	}
}

func TestCatalogResultFragmentsReplaceTheirTarget(t *testing.T) {
	feed := opds.Feed{Title: "German", Links: []opds.Link{{Rel: "next", Href: "https://catalog.example/page-2"}}}
	var output bytes.Buffer
	if err := CatalogPage(domain.User{}, "csrf", domain.OpdsConnection{ID: "connection-1", Name: "Library"}, []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}}, false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `hx-swap="outerHTML"`) {
		t.Fatalf("language form does not replace result target: %s", output.String())
	}
	output.Reset()
	if err := FeedFragmentWithState("csrf", "connection-1", "de", "/opds/browse", "", feed, nil, nil).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if strings.Count(html, `id="catalog-results"`) != 1 || !strings.Contains(html, `hx-swap="outerHTML"`) {
		t.Fatalf("feed fragment replacement contract broken: %s", html)
	}
}

func TestHTMXCatalogFailurePreservesFragmentAndStatus(t *testing.T) {
	h := &Handler{services: Services{Store: catalogFailureStore{connection: domain.OpdsConnection{ID: "connection-1", Name: "Library"}}}, targetKey: explicitTestWebKey("target")}
	r := httptest.NewRequest("GET", "/opds/browse?connection=connection-1&language=de", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.catalogFailure(w, r, "connection-1", errors.New("upstream temporarily unavailable"), r.URL.RequestURI())
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("HTMX failure status=%d want %d body=%s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `<section id="catalog-results"`) || strings.Contains(w.Body.String(), "<!doctype html>") {
		t.Fatalf("HTMX failure did not preserve fragment rendering: %s", w.Body.String())
	}
}

func TestCatalogURLsDoNotRenderUserinfoOrSensitiveQueryValues(t *testing.T) {
	var output bytes.Buffer
	connection := domain.OpdsConnection{ID: "connection-1", Name: "Library", URL: "https://reader:password@catalog.example/opds?access_token=secret-value&view=books"}
	if err := ConnectionsPage(domain.User{}, "csrf", []domain.OpdsConnection{connection}, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, forbidden := range []string{"reader:password", "password@", "secret-value"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("catalog credential rendered in page (%q): %s", forbidden, html)
		}
	}
	if !strings.Contains(html, `name="url" value=""`) || strings.Contains(html, "[redacted]") {
		t.Fatalf("credential-bearing edit URL did not use keep-current contract: %s", html)
	}
}

func TestCredentialBearingCatalogEditUsesKeepCurrentContract(t *testing.T) {
	connection := domain.OpdsConnection{URL: "https://catalog.example/opds?access_token=keep-this&view=books"}
	if got := connectionURLForEdit(connection); got != "" {
		t.Fatalf("credential-bearing URL was rendered into edit input: %q", got)
	}
	if got := safeAcquisitionURL(connection.URL); got != "https://catalog.example/opds?view=books" {
		t.Fatalf("safe URL=%q", got)
	}
	if strings.Contains(safeAcquisitionURL(connection.URL), "[redacted]") {
		t.Fatal("safe URL rendered a redacted placeholder")
	}
	plain := domain.OpdsConnection{URL: "https://catalog.example/opds?view=books"}
	if got := connectionURLForEdit(plain); got != plain.URL {
		t.Fatalf("plain URL=%q want %q", got, plain.URL)
	}
}

func TestSensitiveCatalogQueryNamesAreRedactedWithoutHidingUnrelatedValues(t *testing.T) {
	for _, name := range []string{"key", "apikey", "api-key", "accesskey", "access_key", "access-key", "access_token", "client_secret", "X-Amz-Signature"} {
		t.Run(name, func(t *testing.T) {
			connection := domain.OpdsConnection{URL: "https://catalog.example/opds?author=Le%20Guin&" + name + "=secret-value&view=books"}
			if got := connectionURLForEdit(connection); got != "" {
				t.Fatalf("credential-bearing URL was rendered into edit input: %q", got)
			}
			if got := safeAcquisitionURL(connection.URL); got != "https://catalog.example/opds?author=Le+Guin&view=books" {
				t.Fatalf("safe URL=%q", got)
			}
			var output bytes.Buffer
			if err := ConnectionsPage(domain.User{}, "csrf", []domain.OpdsConnection{connection}, "").Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "secret-value") {
				t.Fatalf("catalog query secret rendered: %s", output.String())
			}
		})
	}

	plain := domain.OpdsConnection{URL: "https://catalog.example/opds?author=Le%20Guin&monkey=business&view=books"}
	if got := connectionURLForEdit(plain); got != plain.URL {
		t.Fatalf("unrelated query names were hidden: %q", got)
	}
	if got := safeAcquisitionURL(plain.URL); got != "https://catalog.example/opds?author=Le+Guin&monkey=business&view=books" {
		t.Fatalf("unrelated query values changed: %q", got)
	}
}

func TestCatalogTargetTokensPreserveExactSignedLinksWithoutRenderingThem(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "target-token-test-secret")
	cookieKey := explicitTestWebKey("cookie")
	targetKey := explicitTestWebKey("target")
	if bytes.Equal(cookieKey, targetKey) {
		t.Fatal("cookie and target keys are not separated")
	}
	h := &Handler{targetKey: targetKey}
	exact := "https://catalog.example/book.epub?X-Amz-Signature=keep-this&access_token=keep-that"
	token := h.clientTargetToken("connection-1", "de", nil, exact)
	if strings.Contains(token, "keep-this") || strings.Contains(token, "keep-that") {
		t.Fatalf("target token exposed query credentials: %q", token)
	}
	decoded, err := decodeAcquisitionTarget(targetKey, token)
	if err != nil || decoded.Href != exact {
		t.Fatalf("decoded target=%+v err=%v", decoded, err)
	}
	path := browseURL("connection-1", token, nil)
	if strings.Contains(path, "[redacted]") || !strings.Contains(path, url.QueryEscape(token)) {
		t.Fatalf("browse path changed opaque target: %q", path)
	}
	if _, err := decodeAcquisitionTarget(cookieKey, token); err == nil {
		t.Fatal("target token accepted cookie MAC key")
	}
}

func TestAcquisitionFormTokenUsesExplicitInjectedHandlerKey(t *testing.T) {
	h := &Handler{targetKey: explicitTestWebKey("target")}
	entry := opds.Entry{ID: "book-1", Title: "Book"}
	token := h.clientTargetToken("connection-1", "de", &entry, "https://catalog.example/book.epub?access_token=keep-secret")
	var output bytes.Buffer
	if err := AcquisitionForm("csrf", "connection-1", "de", "/opds/language", entry, token).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	marker := `name="acquisition" value="`
	start := strings.Index(output.String(), marker)
	if start < 0 {
		t.Fatalf("acquisition token absent: %s", output.String())
	}
	got := strings.Split(output.String()[start+len(marker):], `"`)[0]
	if got != token {
		t.Fatalf("form token=%q want %q", got, token)
	}
	decoded, err := decodeAcquisitionTarget(h.targetKey, got)
	if err != nil || decoded.Href != "https://catalog.example/book.epub?access_token=keep-secret" {
		t.Fatalf("decoded target=%+v err=%v", decoded, err)
	}
}

func TestAcquisitionSuccessOffersContinueAndOwnedBookChoices(t *testing.T) {
	var output bytes.Buffer
	if err := AcquisitionSuccessCard(opds.Entry{Title: "Book"}, "book-1", false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Continue browsing", "Open owned book", "/books/book-1"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("acquisition success missing %q: %s", want, output.String())
		}
	}
}

func TestFullPageCatalogReturnsPreserveAcquisitionMessagesWithOwnedState(t *testing.T) {
	connection := domain.OpdsConnection{ID: "connection-1", Name: "Library"}
	entry := opds.Entry{ID: "book-1", Title: "Book", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}
	owned := []acquisitionEntryState{{Connection: connection.ID, Language: "de", EntryID: entry.ID, HrefDigest: acquisitionHrefDigest(entry.Links[0].Href), SourceID: "source-1"}}
	feed := opds.Feed{Title: "Books", Entries: []opds.Entry{entry}}
	messageTests := []struct {
		name, message string
	}{
		{name: "added", message: "Added to My Library. Continue browsing or open the owned book; analysis starts separately."},
		{name: "already present", message: "That book is already in My Library. Continue browsing or open the existing book."},
	}
	pageRenderers := map[string]func(string, *bytes.Buffer) error{
		"root": func(message string, output *bytes.Buffer) error {
			return CatalogRootPage(domain.User{}, "csrf", connection, "de", message, feed, owned).Render(context.Background(), output)
		},
		"feed": func(message string, output *bytes.Buffer) error {
			return CatalogFeedPage(domain.User{}, "csrf", connection, feed, nil, "de", "/opds/browse", message, owned).Render(context.Background(), output)
		},
		"language": func(message string, output *bytes.Buffer) error {
			return CatalogLanguagePage(domain.User{}, "csrf", connection, "de", "/opds/language", message, feed, owned).Render(context.Background(), output)
		},
		"search": func(message string, output *bytes.Buffer) error {
			return CatalogSearchPage(domain.User{}, "csrf", connection, "de", "/opds/search", "/opds/language", "Book", message, feed, owned).Render(context.Background(), output)
		},
	}
	for page, renderPage := range pageRenderers {
		for _, test := range messageTests {
			t.Run(page+"/"+test.name, func(t *testing.T) {
				var output bytes.Buffer
				if err := renderPage(test.message, &output); err != nil {
					t.Fatal(err)
				}
				html := output.String()
				if !strings.Contains(html, test.message) || !strings.Contains(html, "Already in My Library") || !strings.Contains(html, "/books/source-1") {
					t.Fatalf("full-page acquisition state lost message or owned book: %s", html)
				}
			})
		}
	}
}

func TestAcquisitionStateIsSessionLocalAndContainsNoCredentials(t *testing.T) {
	h := &Handler{services: Services{SessionLifetime: time.Hour}, acquisitionKey: explicitTestWebKey("cookie")}
	entry := opds.Entry{ID: "entry-1", Title: "Book"}
	payload := acquisitionCookiePayload{OwnerID: "user-1", SessionHash: "session-1", Entries: []acquisitionEntryState{{Connection: "connection-1", Language: "de", EntryID: entry.ID, Href: "https://catalog.example/book.epub", SourceID: "book-1"}}}
	_, value, err := encodeAcquisitionCookie(h.acquisitionKey, payload)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: acquisitionCookie, Value: value}
	if strings.Contains(cookie.Value, "password") || strings.Contains(cookie.Value, "secret") {
		t.Fatalf("acquisition cookie contains credential material: %q", cookie.Value)
	}
	next := httptest.NewRequest("GET", "/opds/language", nil)
	next.AddCookie(cookie)
	if got := acquisitionEntrySource(h.acquisitionStateFor(value, "user-1", "session-1"), "connection-1", "de", entry, "https://catalog.example/book.epub"); got != "book-1" {
		t.Fatalf("session acquisition state=%q", got)
	}
	if got := h.acquisitionStateFor(value, "user-2", "session-1"); got != nil {
		t.Fatalf("state crossed user boundary: %+v", got)
	}
	if got := h.acquisitionStateFor(value+"x", "user-1", "session-1"); got != nil {
		t.Fatalf("tampered state accepted: %+v", got)
	}
}

func TestAcquisitionCookieEvictsOldestEntriesWithinByteLimit(t *testing.T) {
	entries := make([]acquisitionEntryState, 0, maxAcquisitionEntries)
	for i := 0; i < maxAcquisitionEntries; i++ {
		entries = append(entries, acquisitionEntryState{Connection: "connection-1", Language: "de", EntryID: fmt.Sprintf("entry-%d", i), Href: "https://catalog.example/" + strings.Repeat("x", 100), SourceID: fmt.Sprintf("book-%d", i)})
	}
	value, retained := boundedAcquisitionCookie(explicitTestWebKey("cookie"), acquisitionCookiePayload{OwnerID: "user-1", SessionHash: "session-1", Entries: entries})
	if value == "" || len(value) > maxAcquisitionCookieBytes || len(retained) >= len(entries) || retained[0].EntryID == "entry-0" {
		t.Fatalf("cookie bound=%d retained=%d first=%q", len(value), len(retained), retained[0].EntryID)
	}
}

func TestCatalogResultsPreserveBrowseContextAndPagination(t *testing.T) {
	feed := opds.Feed{Title: "A–C", Links: []opds.Link{{Rel: "search", Href: "https://catalog.example/search{?q}"}, {Rel: "previous", Href: "https://catalog.example/a-c?page=1"}, {Rel: "next", Href: "https://catalog.example/a-c?page=2"}}, Entries: []opds.Entry{{Title: "Book"}}}
	trail := []CatalogCrumb{{Title: "Authors", URL: "https://catalog.example/authors"}, {Title: "A–C", URL: "https://catalog.example/a-c"}}
	returnTo := "/opds/browse?connection=connection-1&language=de&url=https%3A%2F%2Fcatalog.example%2Fa-c&trail=Authors%1Fhttps%3A%2F%2Fcatalog.example%2Fauthors"
	var output bytes.Buffer
	if err := FeedFragmentWithState("csrf", "connection-1", "de", returnTo, "", feed, trail, nil).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"name=\"return_to\"", "name=\"trail\"", "Authors", "A–C", "Previous page", "Next page", "url="} {
		if !strings.Contains(html, want) {
			t.Errorf("browse context missing %q: %s", want, html)
		}
	}
}

func TestAlreadyOwnedEntryReplacesAcquisitionAction(t *testing.T) {
	feed := opds.Feed{Title: "German", Entries: []opds.Entry{{ID: "book-1", Title: "Book", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://catalog.example/book.epub"}}}}}
	owned := []acquisitionEntryState{{Connection: "connection-1", Language: "de", EntryID: "book-1", Href: "https://catalog.example/book.epub", SourceID: "source-1"}}
	var output bytes.Buffer
	if err := LanguageResultsWithState("csrf", "connection-1", "de", "/opds/language?connection=connection-1&language=de", "", feed, owned).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Already in My Library") || !strings.Contains(output.String(), "/books/source-1") || strings.Contains(output.String(), "Add to library") {
		t.Fatalf("owned entry still offered acquisition: %s", output.String())
	}
}

func TestConnectionFailureFragmentNamesRecoveryTarget(t *testing.T) {
	var output bytes.Buffer
	connection := domain.OpdsConnection{ID: "connection-1", Name: "Home library"}
	if err := CatalogFailureFragment(connection, "The catalog rejected the credentials. Update the connection username and password.", "/opds/browse?connection=connection-1&language=de").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Authentication failed", "Home library", "Edit connection", "Retry", "connection-1"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("failure recovery missing %q: %s", want, output.String())
		}
	}
}

func TestAcquisitionFailuresKeepInvalidEPUBDistinct(t *testing.T) {
	message := opdsErrorMessage(errors.New("opds: ingest downloaded EPUB: epub: invalid EPUB: missing container.xml"))
	for _, want := range []string{"not a valid EPUB", "No book was added"} {
		if !strings.Contains(message, want) {
			t.Errorf("invalid EPUB message missing %q: %s", want, message)
		}
	}
}

func TestSearchResultsExplainEmptySearchAndRevision(t *testing.T) {
	var output bytes.Buffer
	if err := SearchResults("csrf", "connection-1", "de", "/opds/search?connection=connection-1&language=de&q=missing", "", "missing", opds.Feed{}).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"No matching books were found", "Revise search", "connection-1", "de"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("search result missing %q: %s", want, output.String())
		}
	}
}

func cookieNamedForTest(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %s absent", name)
	return nil
}

func TestCatalogRootFragmentShowsSearchWithoutCategories(t *testing.T) {
	feed := opds.Feed{Title: "Catalog", Links: []opds.Link{{Rel: "search", Href: "https://catalog.example/search{?q}"}}, Entries: []opds.Entry{{Title: "Authors"}, {Title: "Newest books"}}}
	var output bytes.Buffer
	if err := CatalogRootFragment("connection-1", "de", feed).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Search this catalog", `method="get"`, `action="/opds/search"`, `hx-get="/opds/search"`} {
		if !strings.Contains(html, want) {
			t.Errorf("root fragment missing %q: %s", want, html)
		}
	}
	for _, unwanted := range []string{"Catalog root", "Authors", "Newest books"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("root fragment unexpectedly contains %q: %s", unwanted, html)
		}
	}
}

func TestCatalogRootResultsShowsOpaquePaginationControls(t *testing.T) {
	feed := opds.Feed{Title: "Catalog", Links: []opds.Link{{Rel: "previous", Href: "m1.previous"}, {Rel: "next", Href: "m1.next"}}}
	var output bytes.Buffer
	if err := CatalogRootResults("csrf", "connection-1", "de", "/opds/browse?connection=connection-1&language=de", "", feed, nil).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Previous page", "Next page", "url=m1.previous", "url=m1.next"} {
		if !strings.Contains(html, want) {
			t.Errorf("root pagination missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "[redacted]") {
		t.Fatal("root pagination rendered a redacted target")
	}
}

func TestFeedFragmentShowsBreadcrumbsAndEmptyState(t *testing.T) {
	feed := opds.Feed{Title: "A–C", Links: []opds.Link{{Rel: "search", Href: "https://catalog.example/search{?q}"}}}
	trail := []CatalogCrumb{{Title: "Authors", URL: "https://catalog.example/authors"}, {Title: "A–C", URL: "https://catalog.example/a-c"}}
	var output bytes.Buffer
	if err := FeedFragment("csrf", "connection-1", "de", "/opds/browse?connection=connection-1&language=de", "", feed, trail).Render(context.Background(), &output); err != nil {
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
	status := analysis.Status{ID: 42, DisplayNumber: 1, State: rivertype.JobStateCompleted, LogicalState: "completed", ScopeID: "scope-1", CorpusID: "corpus-1"}
	if err := JobPage(domain.User{Username: "learner"}, "csrf", status).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{
		`form[data-prepare-deck]`,
		`action="/jobs/42/deck/preparations"`,
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
	for _, unwanted := range []string{`action="/books/book-1/deck/preparations"`, `action="/books/book-1/deck"`, `await response.blob()`, `URL.createObjectURL`, `The deck downloads immediately`} {
		if strings.Contains(html, unwanted) {
			t.Errorf("deck page still contains legacy immediate-download behavior %q", unwanted)
		}
	}
}

func TestAnalysisResultPageUsesExactIdentityAndDocumentedOrder(t *testing.T) {
	completed := time.Date(2026, time.August, 28, 12, 34, 56, 0, time.UTC)
	result := analysis.CompletedAnalysis{
		RunID: "run-it-370", OwnerID: "owner-1", SourceMaterialID: "book-it-370", ScopeID: "scope-it-370",
		AnalyzerName: "mouseion-scoped-analyzer", AnalyzerVersion: "3", ConfigIdentity: "selection-default-v1", CompletedAt: &completed,
		JobID: 370, DisplayNumber: 4,
		Source:   domain.SourceMaterial{ID: "book-it-370", Title: "Il lettore", Language: "it", MediaType: "application/epub+zip"},
		Corpus:   domain.Corpus{ID: "corpus-it-370", SelectedUnits: []domain.CorpusSelectedUnit{{UnitID: "unit-1", Order: 0, Title: "Capitolo primo", ResolvedHref: "capitolo.xhtml"}}},
		Scope:    domain.EPUBReviewedScopeSnapshot{Classifier: domain.EPUBClassifierIdentity{Name: "epub-classifier", Version: "2"}, SelectionMode: domain.EPUBScopeSelectionRecommended},
		Artifact: domain.NormalizedArtifact{NormalizationProfile: "italian-standard", NormalizationVersion: "1"},
	}
	coverage := domain.AnalysisCoverage{
		AnalyzableTokenCount: 100, DistinctLemmaCount: 40, KnownTokenCount: 70, KnownLemmaCount: 28, UnknownTokenCount: 30, UnknownLemmaCount: 12,
		ActiveCampaignTokenCount: 8, ActiveCampaignLemmaCount: 3,
		Thresholds:           []domain.CoverageThreshold{{TargetPercent: 95, LemmaCount: 5, EligibleTokenCount: 30}, {TargetPercent: 97, LemmaCount: 8, EligibleTokenCount: 30}, {TargetPercent: 99, Reachable: false, EligibleTokenCount: 30}},
		TopUnknownLemmas:     []domain.LemmaOccurrence{{Language: "it", CanonicalLemma: "casa", UPOS: "NOUN", OccurrenceCount: 6}},
		UnknownConcentration: domain.CoverageProjection{TopLemmaCount: 10, EligibleTokenCount: 30, OccurrenceCount: 20},
		TextProfile:          &domain.TextProfile{SentenceCount: 12, NormalizedTokenCount: 130, MedianSentenceTokenCount: 9.5, P90SentenceTokenCount: 18, LongSentenceCount: 1},
	}
	var output bytes.Buffer
	if err := AnalysisResultPage(domain.User{Username: "learner"}, "csrf", result, &coverage, false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Analysis result", "Il lettore", "run-it-370", "scope-it-370", "2026-08-28 12:34:56 UTC", "Identity and trust", "Decision summary", "Vocabulary investment", "Structural and text signals", "Provenance and history", "mouseion-scoped-analyzer", "casa", "Vocabulary state and exclusions", "What next?"} {
		if !strings.Contains(html, want) {
			t.Errorf("analysis result missing %q", want)
		}
	}
	for _, pair := range [][2]string{{"Identity and trust", "Decision summary"}, {"Decision summary", "Vocabulary investment"}, {"Vocabulary investment", "Structural and text signals"}, {"Structural and text signals", "Provenance and history"}, {"Provenance and history", "What next?"}} {
		if strings.Index(html, pair[0]) > strings.Index(html, pair[1]) {
			t.Errorf("result sections out of order: %q before %q", pair[0], pair[1])
		}
	}
}

func TestCompletedJobStatusLinksToExactResult(t *testing.T) {
	status := analysis.Status{ID: 370, DisplayNumber: 4, SourceMaterialID: "book-de-370", RunID: "run-de-370", ScopeID: "scope-de-370", LogicalState: "completed", State: rivertype.JobStateCompleted, CorpusID: "corpus-de-370"}
	var output bytes.Buffer
	if err := JobPage(domain.User{Username: "learner"}, "csrf", status).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, `href="/books/book-de-370/analyses/run-de-370"`) || !strings.Contains(html, "View analysis result") {
		t.Fatalf("completed job did not link exact result: %s", html)
	}
	if strings.Contains(html, `<form method="post" action="/jobs/370/deck/preparations"`) {
		t.Error("completed operational job must not own deck preparation")
	}
}

func TestAnalysisResultPageShowsDegradedInsightsWithoutMetrics(t *testing.T) {
	completed := time.Date(2026, time.August, 28, 12, 34, 56, 0, time.UTC)
	result := analysis.CompletedAnalysis{
		RunID: "run-unavailable", SourceMaterialID: "book-unavailable", ScopeID: "scope-unavailable", CompletedAt: &completed,
		Source: domain.SourceMaterial{ID: "book-unavailable", Title: "Unavailable insights", Language: "de"},
		Corpus: domain.Corpus{ID: "corpus-unavailable", SelectedUnits: []domain.CorpusSelectedUnit{{UnitID: "unit-1", Title: "Chapter"}}},
	}
	var output bytes.Buffer
	if err := AnalysisResultPage(domain.User{Username: "learner"}, "csrf", result, nil, true).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, "Analysis insights unavailable") || strings.Contains(html, "Decision summary") || strings.Contains(html, "current-known coverage") {
		t.Fatalf("degraded result rendering=%s", html)
	}
}

func TestBookAnalysisHistoryLinksCompletedRunsToExactResults(t *testing.T) {
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-history", Title: "History", Language: "de"}}
	history := []domain.AnalysisJob{
		{ID: 11, DisplayNumber: 2, SourceMaterialID: book.Source.ID, AnalysisRunID: "run-completed", CorpusID: "corpus-completed", AnalysisState: "completed"},
		{ID: 12, DisplayNumber: 3, SourceMaterialID: book.Source.ID, AnalysisRunID: "run-running", AnalysisState: "running"},
	}
	var output bytes.Buffer
	if err := BookPageWithHistory(domain.User{Username: "learner"}, "csrf", book, nil, false, history, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, `href="/books/book-history/analyses/run-completed"`) || !strings.Contains(html, `href="/jobs/12"`) {
		t.Fatalf("analysis history links=%s", html)
	}
}

func TestAnalysisHistoryURLRejectsForeignJobs(t *testing.T) {
	if got := analysisHistoryURL("book-1", domain.AnalysisJob{ID: 9, SourceMaterialID: "book-2", AnalysisState: "running"}); got != "" {
		t.Fatalf("foreign history URL=%q", got)
	}
	if got := analysisHistoryURL("book-1", domain.AnalysisJob{ID: 9, SourceMaterialID: "book-1", AnalysisState: "completed", AnalysisRunID: "run-1", CorpusID: "corpus-1"}); got != "/books/book-1/analyses/run-1" {
		t.Fatalf("completed history URL=%q", got)
	}
	if got := analysisHistoryLabel(domain.AnalysisJob{DisplayNumber: 2, AnalysisState: "completed", AnalysisRunID: "run-1", CorpusID: "corpus-1"}); got != "Completed analysis #2" {
		t.Fatalf("completed history label=%q", got)
	}
}

func TestBookLifecycleActionsCoverEachAnalysisState(t *testing.T) {
	tests := []struct {
		name, status, state, wantStatus, wantLabel, wantURL string
		jobID                                               int64
		scope                                               string
		runID                                               string
		corpus                                              string
	}{
		{name: "scope review required", status: "not analyzed", wantStatus: "Scope review required", wantLabel: "Review scope", wantURL: "/books/book-1/scope"},
		{name: "ready to analyze", status: "scope confirmed", scope: "scope-1", wantStatus: "Ready to analyze", wantLabel: "Start analysis", wantURL: "/books/book-1/analyze"},
		{name: "queued", status: "analyzing", state: "queued", jobID: 41, wantStatus: "Analysis queued", wantLabel: "View analysis status", wantURL: "/jobs/41"},
		{name: "running", status: "analyzing", state: "running", jobID: 42, wantStatus: "Analysis running", wantLabel: "View analysis status", wantURL: "/jobs/42"},
		{name: "failed", status: "analysis failed", state: "failed", jobID: 43, wantStatus: "Analysis failed — action required", wantLabel: "Review failed analysis", wantURL: "/jobs/43"},
		{name: "cancelled", status: "analysis cancelled", state: "cancelled", jobID: 44, wantStatus: "Analysis cancelled", wantLabel: "Review cancelled analysis", wantURL: "/jobs/44"},
		{name: "exact result", status: "analyzed", state: "completed", runID: "run-1", corpus: "corpus-1", wantStatus: "Analysis result ready", wantLabel: "View analysis result", wantURL: "/books/book-1/analyses/run-1"},
		{name: "legacy result", status: "analyzed", state: "completed", jobID: 45, wantStatus: "Analysis result ready", wantLabel: "View analysis history", wantURL: "/jobs/45"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-1", MediaType: "application/epub+zip"}, AnalysisStatus: tt.status, AnalysisState: tt.state, AnalysisRunID: tt.runID, CorpusID: tt.corpus, ConfirmedScopeID: tt.scope, AnalysisJobID: tt.jobID}
			action := bookLifecycleActionFor(book, nil)
			if action.Status != tt.wantStatus || action.Label != tt.wantLabel || action.URL != tt.wantURL {
				t.Fatalf("action=%+v", action)
			}
		})
	}
}

func TestLibraryRendersOneCanonicalNextAction(t *testing.T) {
	books := []domain.SourceMaterialSummary{
		{Source: domain.SourceMaterial{ID: "scope-book", Title: "Scope book", MediaType: "application/epub+zip"}, AnalysisStatus: "not analyzed"},
		{Source: domain.SourceMaterial{ID: "ready-book", Title: "Ready book", MediaType: "application/epub+zip"}, AnalysisStatus: "scope confirmed", ConfirmedScopeID: "scope-1"},
		{Source: domain.SourceMaterial{ID: "result-book", Title: "Result book", MediaType: "application/epub+zip"}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "run-1", CorpusID: "corpus-1"},
	}
	var output bytes.Buffer
	if err := LibraryPage(domain.User{Username: "learner"}, "csrf", books, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Scope review required", "Review scope", "Ready to analyze", "Start analysis", "Analysis result ready", "View analysis result", `action="/books/ready-book/analyze"`, `href="/books/result-book/analyses/run-1"`} {
		if !strings.Contains(html, want) {
			t.Errorf("library missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "not analyzed") || strings.Contains(html, "scope confirmed") || strings.Contains(html, "analyzed") {
		t.Errorf("library exposed raw lifecycle state: %s", html)
	}
}

func TestBookPromotesExactResultAsTheSingleNextAction(t *testing.T) {
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-result", Title: "Result book", Language: "de", MediaType: "application/epub+zip"}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "run-result", CorpusID: "corpus-result"}
	var output bytes.Buffer
	if err := BookPageWithHistory(domain.User{Username: "learner"}, "csrf", book, nil, false, nil, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Next action", "Analysis result ready", "Inspect the insights for this exact completed analysis.", "View analysis result", `href="/books/book-result/analyses/run-result"`} {
		if !strings.Contains(html, want) {
			t.Errorf("book page missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, `action="/books/book-result/analyze"`) {
		t.Error("completed result rendered a start-analysis form")
	}
	if strings.Contains(html, "View operational analysis history") || strings.Contains(html, "Review a new EPUB scope") {
		t.Error("completed book rendered stale competing analysis actions")
	}
}

func TestNewerFailedAnalysisKeepsHistoricalExactResultLinkWithoutPromotingIt(t *testing.T) {
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-history", Title: "History", MediaType: "application/epub+zip"}, AnalysisStatus: "analysis failed", AnalysisState: "failed", AnalysisJobID: 52}
	history := []domain.AnalysisJob{
		{ID: 52, DisplayNumber: 3, SourceMaterialID: book.Source.ID, AnalysisRunID: "run-failed", AnalysisState: "failed"},
		{ID: 51, DisplayNumber: 2, SourceMaterialID: book.Source.ID, AnalysisRunID: "run-completed", CorpusID: "corpus-completed", AnalysisState: "completed"},
	}
	action := bookLifecycleActionFor(book, history)
	if action.Status != "Analysis failed — action required" || action.Label != "Review failed analysis" || action.URL != "/jobs/52" {
		t.Fatalf("action=%+v", action)
	}
	var output bytes.Buffer
	if err := BookPageWithHistory(domain.User{Username: "learner"}, "csrf", book, nil, false, history, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, `href="/books/book-history/analyses/run-completed"`) || !strings.Contains(html, "Review failed analysis") {
		t.Fatalf("historical result or current failure missing: %s", html)
	}
}

func TestJobsPageLinksCompletedScopedRunsToExactResults(t *testing.T) {
	jobs := []domain.AnalysisJob{
		{ID: 21, DisplayNumber: 4, SourceMaterialID: "book-jobs", AnalysisRunID: "run-jobs", CorpusID: "corpus-jobs", AnalysisState: "completed"},
		{ID: 22, DisplayNumber: 5, SourceMaterialID: "book-jobs", AnalysisRunID: "run-pending", AnalysisState: "running"},
	}
	var output bytes.Buffer
	if err := JobsPage(domain.User{Username: "learner"}, "csrf", jobs, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, `href="/books/book-jobs/analyses/run-jobs"`) || !strings.Contains(html, `href="/jobs/22"`) {
		t.Fatalf("jobs page links=%s", html)
	}
}

func TestAnalyzedBookCoverageSummaryExplainsMetrics(t *testing.T) {
	var output bytes.Buffer
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-1", Title: "Book", Language: "de", MediaType: "application/epub+zip"}, AnalysisStatus: "analyzed", ReviewedScopeID: "scope-264"}
	coverage := domain.AnalysisCoverage{
		ReviewedScopeID: "scope-264", SelectedUnits: []domain.CorpusSelectedUnit{{UnitID: "one", Order: 1, Title: "Chapter One"}, {UnitID: "two", Order: 2, SourceHref: "chapter-two.xhtml"}},
		AnalyzableTokenCount: 40, DistinctLemmaCount: 12, KnownTokenCount: 30, KnownLemmaCount: 7, UnknownTokenCount: 10, UnknownLemmaCount: 5,
		TextProfile:          &domain.TextProfile{SentenceCount: 4, NormalizedTokenCount: 50, EmptySentenceCount: 1, MedianSentenceTokenCount: 12.5, P90SentenceTokenCount: 40, LongSentenceCount: 1},
		TopUnknownLemmas:     []domain.LemmaOccurrence{{CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 4}},
		UnknownConcentration: domain.CoverageProjection{TopLemmaCount: 10, SelectedLemmaCount: 5, OccurrenceCount: 10, EligibleTokenCount: 10, ProjectedTokenCount: 40},
		Projections:          []domain.CoverageProjection{{TopLemmaCount: 10, SelectedLemmaCount: 5, OccurrenceCount: 10, EligibleTokenCount: 10, ProjectedTokenCount: 40}},
		Thresholds:           []domain.CoverageThreshold{{TargetPercent: 95, LemmaCount: 3, Reachable: true}, {TargetPercent: 97, LemmaCount: 4, Reachable: true}, {TargetPercent: 99, LemmaCount: 5}},
	}
	if err := BookPage(domain.User{Username: "learner"}, "csrf", book, &coverage, false, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Analyzed scope", "2 selected units", "scope-264", "Chapter One", "chapter-two.xhtml", "not necessarily to the full EPUB", "Reanalysis", "reuses reviewed scope", "new EPUB scope", "Analysis quality", "do not assign a quality grade", "Empty sentences returned", "1 analyzer-provided sentences contained no tokens", "Text profile", "4", "12.5", "40", "25.0%", "long sentences (&gt;35 tokens)", "40 of 50", "not a difficulty score", "75.0%", "current-known coverage", "active-campaign projected coverage", "analyzable tokens", "distinct lemmas", "graduated by completed campaigns", "unknown vocabulary", "lemmas for 95%", "lemmas for 97%", "Unavailable", "99% cannot be reached with deck-eligible vocabulary", "legacy generated history", "deck-eligible vocabulary", "Highest-impact unknown vocabulary", "Haus", "4 occurrences", "top 10 deck-eligible lemmas", "100.0%", "Projected token coverage", "after top 10 lemmas"} {
		if !strings.Contains(html, want) {
			t.Errorf("coverage summary missing %q", want)
		}
	}
	for _, unwanted := range []string{"structural difficulty", "CEFR"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("coverage summary includes unsupported claim %q: %s", unwanted, html)
		}
	}
}

func TestAnalyzedBookDoesNotRenderNonLexicalTopUnknownLemma(t *testing.T) {
	coverage := domain.AnalysisCoverage{TopUnknownLemmas: []domain.LemmaOccurrence{
		{CanonicalLemma: "5", UPOS: "NOUN", OccurrenceCount: 99},
		{CanonicalLemma: "Straße", UPOS: "NOUN", OccurrenceCount: 1},
	}}
	var output bytes.Buffer
	if err := BookPage(domain.User{Username: "learner"}, "csrf", domain.SourceMaterialSummary{}, &coverage, false, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if strings.Contains(html, ">5</strong>") || !strings.Contains(html, ">Straße</strong>") {
		t.Fatalf("top unknown vocabulary = %s", html)
	}
}

func TestAnalyzedBookReportsOnlyEvidenceBackedQualityWarnings(t *testing.T) {
	tests := []struct {
		name     string
		coverage domain.AnalysisCoverage
		want     string
		unwanted string
	}{
		{
			name:     "no sentences",
			coverage: domain.AnalysisCoverage{TextProfile: &domain.TextProfile{}},
			want:     "No sentences returned.",
			unwanted: "No vocabulary-analyzable tokens.",
		},
		{
			name: "normalized tokens filtered from vocabulary analysis",
			coverage: domain.AnalysisCoverage{TextProfile: &domain.TextProfile{
				SentenceCount: 2, NormalizedTokenCount: 12,
			}},
			want:     "No vocabulary-analyzable tokens.",
			unwanted: "No sentences returned.",
		},
		{
			name: "complete analyzer output",
			coverage: domain.AnalysisCoverage{AnalyzableTokenCount: 10, TextProfile: &domain.TextProfile{
				SentenceCount: 2, NormalizedTokenCount: 12,
			}},
			unwanted: "Analysis quality",
		},
	}
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-1", Title: "Book", Language: "de"}, AnalysisStatus: "analyzed"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := BookPage(domain.User{Username: "learner"}, "csrf", book, &tt.coverage, false, "").Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			html := output.String()
			if tt.want != "" && !strings.Contains(html, tt.want) {
				t.Errorf("page missing warning %q: %s", tt.want, html)
			}
			if strings.Contains(html, tt.unwanted) {
				t.Errorf("page unexpectedly contains %q: %s", tt.unwanted, html)
			}
			for _, unsupported := range []string{"analysis is bad", "CEFR", "proficiency level:"} {
				if strings.Contains(html, unsupported) {
					t.Errorf("page contains unsupported claim %q", unsupported)
				}
			}
		})
	}
}

func TestLegacyAnalyzedBookRequestsReanalysisForAllInsights(t *testing.T) {
	var output bytes.Buffer
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "legacy", Title: "Legacy Book", Language: "de"}, AnalysisStatus: "analyzed"}
	if err := BookPage(domain.User{Username: "learner"}, "csrf", book, nil, true, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Analysis insights unavailable", "reproducible vocabulary and sentence statistics", "Analyze it again", "coverage, projections, and the structural profile"} {
		if !strings.Contains(html, want) {
			t.Errorf("legacy page missing %q", want)
		}
	}
}
