package webapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catalogFixtureDefinition builds one valid fixture manifest entry.
func catalogFixtureDefinition(key, workKey, title, author, greekTitle, editionKey, citationStart, citationEnd string, book *int, books, words int, withdrawn bool, translation bool) classical.Definition {
	definition := classical.Definition{
		Key:                   key,
		WorkKey:               workKey,
		WorkTitle:             title,
		WorkAuthor:            author,
		GreekTitle:            greekTitle,
		EditionKey:            editionKey,
		EditionLabel:          "Reviewed edition",
		Language:              classical.Language,
		ConstituentBook:       book,
		ConstituentBooks:      books,
		CitationStart:         citationStart,
		CitationEnd:           citationEnd,
		WordCount:             words,
		Source:                classical.SourcePin{Layer: "oga-src", FileName: "work.conllu", Size: 10, SHA256: strings.Repeat("a", 64), Format: classical.SourceFormatOGACoNLLU},
		InterpretationVersion: "oga-agdt-upos/5",
		Revision:              1,
		Withdrawn:             withdrawn,
	}
	if translation {
		definition.Translation = &classical.TranslationPin{
			Layer: "translation", FileName: "work.xml", Size: 10, SHA256: strings.Repeat("b", 64),
			CitationStart: citationStart, CitationEnd: citationEnd,
		}
	}
	return definition
}

// catalogFixture builds a small release: a translated constituent book, a
// whole-work passage, a two-book work without a translation and a withdrawn
// tombstone.
func catalogFixture(t *testing.T) *classical.Catalog {
	t.Helper()
	one, two := 1, 2
	catalog, err := classical.NewCatalog([]classical.Definition{
		catalogFixtureDefinition("xenophon-anabasis-1", "xenophon-anabasis", "Anabasis", "Xenophon", "Ἀνάβασις",
			"xenophon-anabasis-marchant-1904", "1.1.1", "1.10.19", &one, 7, 9305, false, true),
		catalogFixtureDefinition("xenophon-hellenica-1", "xenophon-hellenica", "Hellenica", "Xenophon", "Ἑλληνικά",
			"xenophon-hellenica-dindorf-1866", "1.1.1", "1.1.10", nil, 1, 500, false, false),
		catalogFixtureDefinition("homer-iliad-1", "homer-iliad", "Iliad", "Homer", "Ἰλιάς",
			"homer-iliad-monro-1902", "1.1.1", "1.1.10", &one, 24, 100, false, false),
		catalogFixtureDefinition("homer-iliad-2", "homer-iliad", "Iliad", "Homer", "Ἰλιάς",
			"homer-iliad-monro-1902", "2.1.1", "2.1.10", &two, 24, 200, false, false),
		catalogFixtureDefinition("xenophon-cyropaedia-1", "xenophon-cyropaedia", "Cyropaedia", "Xenophon", "Κύρου παιδεία",
			"xenophon-cyropaedia-hutchinson-1914", "1.1.1", "1.1.10", &one, 8, 999, true, false),
	})
	require.NoError(t, err)
	return catalog
}

func catalogPage(t *testing.T, h http.Handler, cookies []*http.Cookie, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func TestCatalogsRendersTheClassicalCatalogRecordAndMarginCounts(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
	})

	response := catalogPage(t, h, cookies, "/catalogs")

	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	for _, want := range []string{
		"Classical catalog",
		`aria-label="Running release counts"`,
		"<dt>Works</dt><dd>3</dd>",
		"<dt>Authors</dt><dd>2</dd>",
		"<dt>Reading units</dt><dd>4</dd>",
		"Sources are fetched at To Read",
		`href="/catalogs/classical"`,
	} {
		assert.Contains(t, body, want, "GET /catalogs missing %q", want)
	}
	assert.Less(t, strings.Index(body, "Classical catalog"), strings.Index(body, "Add catalog connection"), "the Classical catalog is above OPDS connections")
	assert.NotContains(t, body, "Cyropaedia", "a withdrawn definition is hidden")
}

func TestClassicalCatalogAuthorIndexAuthorPageAndWorkPage(t *testing.T) {
	h, cookies, _, store := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
	})
	require.NoError(t, store.SetActiveStudyLanguage(t.Context(), fixtures.OwnerID, "de"))

	// The learner's active language is German; the catalog is not scoped to
	// the study language.
	index := catalogPage(t, h, cookies, "/catalogs/classical")
	require.Equal(t, http.StatusOK, index.Code)
	for _, want := range []string{"Xenophon", "Homer", "Anabasis", "Hellenica", "Iliad", `href="/catalogs/classical/works/xenophon-anabasis"`} {
		assert.Contains(t, index.Body.String(), want, "author index missing %q", want)
	}
	assert.NotContains(t, index.Body.String(), "Cyropaedia")

	author := catalogPage(t, h, cookies, "/catalogs/classical/authors/"+url.PathEscape("Xenophon"))
	require.Equal(t, http.StatusOK, author.Code)
	for _, want := range []string{
		"<h1>Xenophon</h1>",
		"Ἀνάβασις",
		"7 books",
		"9305",
		"English translation available",
		"passage",
		"English translation not available",
	} {
		assert.Contains(t, author.Body.String(), want, "author page missing %q", want)
	}
	assert.NotContains(t, author.Body.String(), "Cyropaedia")

	work := catalogPage(t, h, cookies, "/catalogs/classical/works/xenophon-anabasis")
	require.Equal(t, http.StatusOK, work.Code)
	for _, want := range []string{"<th>Book</th>", "<th>Passage</th>", "<th>Words</th>", "Book 1", "1.1.1–1.10.19", "9305"} {
		assert.Contains(t, work.Body.String(), want, "work page missing %q", want)
	}

	whole := catalogPage(t, h, cookies, "/catalogs/classical/works/xenophon-hellenica")
	require.Equal(t, http.StatusOK, whole.Code)
	assert.Contains(t, whole.Body.String(), "Passage")
	assert.Contains(t, whole.Body.String(), "1.1.1–1.1.10")
}

func TestClassicalCatalogHidesWithdrawnAndUnknownEntries(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
	})

	assert.Equal(t, http.StatusNotFound, catalogPage(t, h, cookies, "/catalogs/classical/works/xenophon-cyropaedia").Code)
	assert.Equal(t, http.StatusNotFound, catalogPage(t, h, cookies, "/catalogs/classical/works/no-such-work").Code)
	assert.Equal(t, http.StatusNotFound, catalogPage(t, h, cookies, "/catalogs/classical/authors/"+url.PathEscape("No Such Author")).Code)
}

func TestCatalogsRouteRendersCatalogManagementScreen(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/catalogs?error=Sync+failed", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	for _, want := range []string{
		"<title>Catalogs · Mouseion</title>",
		"<h1>Catalogs</h1>",
		`href="/catalogs" aria-current="page"`,
		"Add catalog connection",
		`class="feedback feedback--error"`,
		"Sync failed",
	} {
		assert.Contains(t, response.Body.String(), want, "GET /catalogs missing %q: %s", want, response.Body.String())
	}
}

func TestLegacyConnectionsRoutePermanentlyRedirectsAndPreservesSupportedQuery(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/connections?book_id=book%2F1&message=Catalog+added&error=Try+again&ignored=secret", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	assert.Equal(t, http.StatusMovedPermanently, response.Code)
	location, err := url.Parse(response.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/catalogs", location.Path)
	got := location.Query()
	assert.Equal(t, "book/1", got.Get("book_id"))
	assert.Equal(t, "Catalog added", got.Get("message"))
	assert.Equal(t, "Try again", got.Get("error"))
	assert.False(t, got.Has("ignored"))
}

func TestOPDSErrorMessageNamesTheEPUBSizeLimits(t *testing.T) {
	t.Run("download limit", func(t *testing.T) {
		err := fmt.Errorf("opds: download EPUB: %w", opds.ErrDownloadTooLarge)
		assert.Equal(t, "This EPUB is larger than the 512 MiB download limit. Choose a smaller edition.", opdsErrorMessage(err))
	})
	t.Run("stored-content limit", func(t *testing.T) {
		err := fmt.Errorf("opds: ingest downloaded EPUB: %w", opds.ErrRetainedEPUBTooLarge)
		assert.Equal(t, "This EPUB is larger than the 100 MiB limit for stored book content. Choose a less illustrated edition.", opdsErrorMessage(err))
	})
}
