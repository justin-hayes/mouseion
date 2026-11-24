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

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/readingintent"
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
		Opening:               "ἄρχεται ὁ λόγος.",
		Source:                classical.SourcePin{Layer: "oga-src-v0.2.0-text", FileName: "work.conllu", Size: 10, SHA256: strings.Repeat("a", 64), Format: classical.SourceFormatOGACoNLLU},
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

// catalogFixtureWholeWorkWithBooks builds a translated whole-work unit that
// lists two constituent books, as the shipped Anabasis does for seven.
func catalogFixtureWholeWorkWithBooks() classical.Definition {
	definition := catalogFixtureDefinition("plato-republic", "plato-republic", "Republic", "Plato", "Πολιτεία",
		"plato-republic-burnet-1905", "1.327", "2.383", nil, 2, 18316, false, true)
	definition.Contents = []classical.ConstituentBookSpan{
		{Book: 1, CitationStart: "1.327", CitationEnd: "1.354", WordCount: 9553},
		{Book: 2, CitationStart: "2.357", CitationEnd: "2.383", WordCount: 8763},
	}
	return definition
}

// catalogFixtureDefinitions builds a small release: a translated constituent
// book, a whole-work passage, a two-book work without a translation and a
// withdrawn tombstone.
func catalogFixtureDefinitions() []classical.Definition {
	one, two := 1, 2
	return []classical.Definition{
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
	}
}

func catalogFixture(t *testing.T) *classical.Catalog {
	t.Helper()
	catalog, err := classical.NewCatalog(catalogFixtureDefinitions())
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

func TestCatalogShowsSearchFilterSourceStatusLinesAndOpdsSections(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
	})

	response := catalogPage(t, h, cookies, "/catalogs")

	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	for _, want := range []string{
		"<title>Catalog · Mouseion</title>",
		"<h1>Catalog</h1>",
		`href="/catalogs" aria-current="page"`,
		`href="/catalogs/classical">Classical catalog</a>`,
		`href="/catalogs/sources">Manage sources</a>`,
		`<form class="catalog-search" role="search" method="get" action="/catalogs">`,
		`id="catalog-search-query"`,
		`placeholder="Title, author, Greek title, or Book number"`,
		`id="catalog-language-filter"`,
		`<option value="de" selected>German</option>`,
		`<option value="">All languages</option>`,
		`<option value="grc">Ancient Greek</option>`,
		`<button class="button" type="submit">Search</button>`,
		`id="source-status-fixture-connection"`,
		`id="source-status-fixture-failed-connection"`,
		`id="source-status-fixture-syncing-connection"`,
		`id="source-status-fixture-never-synced-connection"`,
		"Last synced",
		"Sync failed",
		"Syncing",
		"Never synced",
		`action="/catalogs/sources/fixture-connection/sync"`,
		`name="return" value="catalog"`,
		`href="/catalogs/sources#connection-fixture-failed-connection"`,
		`id="catalog-result-summary"`,
		`role="status" aria-live="polite" aria-atomic="true"`,
		`id="catalog-source-fixture-connection"`,
		`id="catalog-source-fixture-browser-sync-connection"`,
		`id="catalog-source-title-fixture-connection"`,
		"Der lange Weg nach Hause",
		`class="catalog-cover-grid"`,
		`aria-label="Pages of Fixture catalog"`,
		`href="/catalogs?language=de&amp;page_fixture-connection=2"`,
		`href="/catalogs/books/fixture-book/in-my-books">In My Books</a>`,
		`class="catalog-entry__add"`,
		`action="/catalogs/sources/fixture-connection/add"`,
		`name="entry_identifier" value="fixture-sommer"`,
		`Adding downloads and analyzes the text.`,
		`class="catalog-entry__read"`,
		`action="/catalogs/sources/fixture-connection/previously-read"`,
		`>I've read this</button>`,
	} {
		assert.Contains(t, body, want, "GET /catalogs missing %q", want)
	}
	assert.NotContains(t, body, `id="catalog-connection-form"`, "connection management belongs on Manage sources")
	assert.NotContains(t, body, "Cyropaedia", "a withdrawn definition is hidden")
	assert.NotContains(t, body, `id="catalog-source-classical"`, "the default language filter shows German sources, not Ancient Greek")
	assert.Less(t, strings.Index(body, `id="catalog-sources-heading"`), strings.Index(body, `id="catalog-source-fixture-connection"`), "source status lines sit above the OPDS sections")
}

func TestCatalogSearchAndLanguageFilterCoverClassicalAndOpdsSources(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
	})

	// One query, all languages: Homer matches the Classical author and the
	// accented OPDS author without contacting either server.
	response := catalogPage(t, h, cookies, "/catalogs?language=&q=homer")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	for _, want := range []string{
		"2 results from 2 sources.",
		`id="catalog-source-classical"`,
		`id="catalog-source-fixture-connection"`,
		`id="work-homer-iliad"`,
		"Ödipus auf Kolonos",
		"Hómēr Nachfolger",
		`href="/catalogs?language=">Clear search</a>`,
	} {
		assert.Contains(t, body, want, "combined search missing %q", want)
	}
	assert.NotContains(t, body, "Der lange Weg nach Hause", "non-matching entries stay out of the results")
	assert.NotContains(t, body, "Xenophon", "the Classical side uses the same search key")

	// The language filter narrows to one source language and is carried in the
	// paging link.
	italian := catalogPage(t, h, cookies, "/catalogs?language=it")
	require.Equal(t, http.StatusOK, italian.Code)
	assert.Contains(t, italian.Body.String(), "La città delle meraviglie")
	assert.NotContains(t, italian.Body.String(), "Der lange Weg nach Hause")
	assert.Contains(t, italian.Body.String(), `<option value="it" selected>Italian</option>`)
}

func TestCatalogEmptyStatesAreDistinct(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
	})

	noMatch := catalogPage(t, h, cookies, "/catalogs?language=&q=zzzz-nothing")
	require.Equal(t, http.StatusOK, noMatch.Code)
	assert.Contains(t, noMatch.Body.String(), "No matches")
	assert.Contains(t, noMatch.Body.String(), "No entry in the Catalog matches “zzzz-nothing”.")
	assert.Contains(t, noMatch.Body.String(), `href="/catalogs?language=">Clear search</a>`)

	nothingInLanguage := catalogPage(t, h, cookies, "/catalogs?language=fr")
	require.Equal(t, http.StatusOK, nothingInLanguage.Code)
	assert.Contains(t, nothingInLanguage.Body.String(), "Nothing in fr")
	assert.Contains(t, nothingInLanguage.Body.String(), "No Catalog entry is in fr.")
	assert.Contains(t, nothingInLanguage.Body.String(), `href="/catalogs?language=">Show all languages</a>`)

	noSources := catalogPage(t, h, cookies, "/catalogs?language=&q=zzzz-nothing")
	assert.Contains(t, noSources.Body.String(), "No matches")
}

func TestCatalogWithoutConnectionsOffersConnectAnOPDSCatalog(t *testing.T) {
	h, cookies, _, store := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
	})
	connections, err := store.ListOpdsConnections(t.Context(), fixtures.OwnerID)
	require.NoError(t, err)
	for _, connection := range connections {
		require.NoError(t, store.DeleteOpdsConnection(t.Context(), fixtures.OwnerID, connection.ID))
	}

	var catalog bytes.Buffer
	classicalCatalog := catalogFixture(t)
	browse := catalogBrowseView{ClassicalAuthors: classicalCatalog.Authors(), ClassicalTotal: catalogAuthorsWorkCount(classicalCatalog.Authors()), HasClassical: true}
	require.NoError(t, CatalogPage(domain.User{Username: "learner"}, "csrf", nil, "", "", nil, browse).Render(t.Context(), &catalog))
	for _, want := range []string{
		"No OPDS catalogs connected",
		`href="/catalogs/sources">Connect an OPDS catalog</a>`,
		"Classical catalog",
	} {
		assert.Contains(t, catalog.String(), want, "empty Catalog missing %q", want)
	}
	response := catalogPage(t, h, cookies, "/catalogs?language=")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "No OPDS catalogs connected")
}

func TestClassicalCatalogIndexRetiredAuthorPageAndWorkPage(t *testing.T) {
	h, cookies, _, store := readingFixtureSessionWith(t, func(services *Services) {
		catalog, err := classical.NewCatalog(append(catalogFixtureDefinitions(), catalogFixtureWholeWorkWithBooks()))
		require.NoError(t, err)
		services.Classical = catalog
	})
	require.NoError(t, store.SetActiveStudyLanguage(t.Context(), fixtures.OwnerID, "de"))

	// The learner's active language is German; the catalog is not scoped to
	// the study language.
	index := catalogPage(t, h, cookies, "/catalogs/classical")
	require.Equal(t, http.StatusOK, index.Code)
	for _, want := range []string{"Xenophon", "Homer", "Anabasis", "Hellenica", "Iliad", "Republic", `href="/catalogs/classical/works/xenophon-anabasis"`} {
		assert.Contains(t, index.Body.String(), want, "author index missing %q", want)
	}
	assert.NotContains(t, index.Body.String(), "Cyropaedia")
	assert.NotContains(t, index.Body.String(), "/catalogs/classical/authors/", "the index never links to the retired author page")

	retired := catalogPage(t, h, cookies, "/catalogs/classical/authors/"+url.PathEscape("Xenophon"))
	require.Equal(t, http.StatusMovedPermanently, retired.Code)
	assert.Equal(t, "/catalogs/classical#author-Xenophon", retired.Header().Get("Location"))

	work := catalogPage(t, h, cookies, "/catalogs/classical/works/plato-republic")
	require.Equal(t, http.StatusOK, work.Code)
	for _, want := range []string{
		"<th>Book</th>", "<th>Passage</th>", "<th>Words</th>", "Book 1", "Book 2", "1.327–1.354", "2.357–2.383", "9553", "8763",
		"Add and prepare the work", `action="/catalogs/classical/units/plato-republic/add"`,
		"I've read this", `action="/catalogs/classical/units/plato-republic/previously-read"`,
		`href="/catalogs/classical#author-Plato"`,
	} {
		assert.Contains(t, work.Body.String(), want, "work page missing %q", want)
	}
	assert.Equal(t, 1, strings.Count(work.Body.String(), "Add and prepare the work"), "one Add per Edition")
	assert.Equal(t, 1, strings.Count(work.Body.String(), `action="/catalogs/classical/units/plato-republic/previously-read"`), "one I've read this per Edition")
	for _, absent := range []string{"Choose books", "remaining books", "plato-republic-1"} {
		assert.NotContains(t, work.Body.String(), absent)
	}
	assert.NotContains(t, work.Body.String(), "/catalogs/classical/authors/", "the work breadcrumb leads to the index section")

	whole := catalogPage(t, h, cookies, "/catalogs/classical/works/xenophon-hellenica")
	require.Equal(t, http.StatusOK, whole.Code)
	assert.Contains(t, whole.Body.String(), "Passage")
	assert.Contains(t, whole.Body.String(), "1.1.1–1.1.10")
}

func TestClassicalCatalogIndexRendersOpeningsLengthsAndAttribution(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		catalog, err := fixtures.NewClassicalCatalog()
		require.NoError(t, err)
		services.Classical = catalog
	})

	index := catalogPage(t, h, cookies, "/catalogs/classical")
	require.Equal(t, http.StatusOK, index.Code)
	body := index.Body.String()
	for _, want := range []string{
		"Homer",
		"Xenophon",
		"μῆνιν ἄειδε θεὰ Πηληϊάδεω Ἀχιλῆος",
		"μετὰ δὲ ταῦτα οὐ πολλαῖς ἡμέραις ὕστερον",
		`lang="grc"`,
		"Begins at",
		"1.1.1",
		"600 words in 3 books",
		"500 words, read whole",
		"Fixture reviewed edition",
		"Passage 1.1.1–3.10.19",
		"Passage 1.1.1–1.1.10",
		"English translation not available",
		"Add and prepare the work",
		"Greek text source:",
		"OGA oga_src v0.2.0 Greek text",
		"CC BY-SA 4.0",
		`style="width:5.000%;"`,
		`style="width:6.000%;"`,
		`action="/catalogs/classical/units/fixture-odyssey/add"`,
		`name="csrf_token" value="`,
		`name="return" value="index"`,
	} {
		assert.Contains(t, body, want, "catalog index missing %q", want)
	}
	for _, absent := range []string{"Choose books", "Add and prepare the whole work", "remaining books", "classical-books", "previously-read"} {
		assert.NotContains(t, body, absent, "the index offers no per-book control")
	}
}

func TestClassicalCatalogIndexOwnershipStatesAndAddsInPlace(t *testing.T) {
	ctx := t.Context()
	h, cookies, csrf, store := readingFixtureSessionWith(t, func(services *Services) {
		catalog, err := fixtures.NewClassicalCatalog()
		require.NoError(t, err)
		services.Classical = catalog
	})

	index := catalogPage(t, h, cookies, "/catalogs/classical")
	require.Equal(t, http.StatusOK, index.Code)
	body := index.Body.String()
	assert.Contains(t, body, `action="/catalogs/classical/units/fixture-odyssey/add"`, "a whole-work unit with books offers one Add")
	assert.NotContains(t, body, "I've read this", "the index offers no I've read this; the Work page does")
	assert.NotContains(t, body, "books in My Books")
	assert.NotContains(t, body, "In My Books")

	// Add the whole Work, which has three constituent books, as one Book. The
	// existing endpoint returns to the index with the existing confirmation.
	addedOdyssey := readingTestRequest(t, h, "/catalogs/classical/units/fixture-odyssey/add", url.Values{
		"csrf_token": {csrf},
		"return":     {"index"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, addedOdyssey.Code)
	location := addedOdyssey.Header().Get("Location")
	assert.Contains(t, location, "/catalogs/classical?")
	assert.Contains(t, location, url.QueryEscape("Added 1 Book to My Books"))

	owned := catalogPage(t, h, cookies, location)
	require.Equal(t, http.StatusOK, owned.Code)
	ownedBody := owned.Body.String()
	assert.Contains(t, ownedBody, "Added 1 Book to My Books")
	assert.Contains(t, ownedBody, "In My Books")
	assert.NotContains(t, ownedBody, `action="/catalogs/classical/units/fixture-odyssey/add"`, "an owned whole-work unit offers no Add")
	assert.NotContains(t, ownedBody, "/catalogs/classical/authors/", "the Add confirmation never links to the retired author page")

	odyssey, err := store.ListOwnedClassicalReadingUnits(ctx, fixtures.OwnerID)
	require.NoError(t, err)
	require.Len(t, odyssey, 1, "adding the Work creates one Book")
	assert.Equal(t, "fixture-odyssey", odyssey[0].ReadingUnitKey)

	// Add a whole-work unit from the index margin.
	addedWork := readingTestRequest(t, h, "/catalogs/classical/units/fixture-hellenica/add", url.Values{
		"csrf_token": {csrf},
		"return":     {"index"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, addedWork.Code)
	workLocation := addedWork.Header().Get("Location")
	assert.Contains(t, workLocation, "/catalogs/classical?")
	assert.Contains(t, workLocation, url.QueryEscape("Added 1 Book to My Books"))

	ownedWhole := catalogPage(t, h, cookies, workLocation)
	require.Equal(t, http.StatusOK, ownedWhole.Code)
	ownedWholeBody := ownedWhole.Body.String()
	assert.Contains(t, ownedWholeBody, "In My Books")
	assert.NotContains(t, ownedWholeBody, `action="/catalogs/classical/units/fixture-hellenica/add"`, "a fully owned whole-work unit offers no Add")
}

type failingCatalogueAdd struct{}

func (failingCatalogueAdd) AddCatalogueEntry(context.Context, string, string, string) (domain.CatalogueAddResult, error) {
	return domain.CatalogueAddResult{}, errors.New("catalog Add failed")
}

type failingCatalogueRead struct{ err error }

func (f failingCatalogueRead) MarkCatalogueEntryPreviouslyRead(context.Context, string, string, string) (domain.CatalogueReadResult, error) {
	return domain.CatalogueReadResult{}, f.err
}

func (f failingCatalogueRead) MarkClassicalReadingUnitPreviouslyRead(context.Context, string, string) (domain.CatalogueReadResult, error) {
	return domain.CatalogueReadResult{}, f.err
}

func TestCatalogueReadRoutesReportRefusalsOnThePage(t *testing.T) {
	t.Run("OPDS current reading redirects with feedback", func(t *testing.T) {
		h, cookies, csrf, _ := readingFixtureSessionWith(t, func(services *Services) {
			services.Store.CatalogueRead = failingCatalogueRead{err: domain.ErrBookIsCurrentReading}
		})
		response := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/previously-read", url.Values{
			"csrf_token": {csrf}, "entry_identifier": {"fixture-uber"},
		}, cookies)
		assert.Equal(t, http.StatusSeeOther, response.Code)
		assert.Contains(t, response.Header().Get("Location"), "/catalogs?error=")
		assert.Contains(t, response.Header().Get("Location"), url.QueryEscape(catalogueReadCurrentReadingRefusal))
	})

	t.Run("OPDS unknown failure is a server error", func(t *testing.T) {
		h, cookies, csrf, _ := readingFixtureSessionWith(t, func(services *Services) {
			services.Store.CatalogueRead = failingCatalogueRead{err: errors.New("catalog read failed")}
		})
		response := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/previously-read", url.Values{
			"csrf_token": {csrf}, "entry_identifier": {"fixture-uber"},
		}, cookies)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})

	t.Run("classical legacy rows redirect with the stable reason", func(t *testing.T) {
		h, cookies, csrf, _ := readingFixtureSessionWith(t, func(services *Services) {
			catalog, err := fixtures.NewClassicalCatalog()
			require.NoError(t, err)
			services.Classical = catalog
			services.Store.CatalogueRead = failingCatalogueRead{err: classical.ErrLegacyRows}
		})
		response := readingTestRequest(t, h, "/catalogs/classical/units/fixture-iliad/previously-read", url.Values{
			"csrf_token": {csrf}, "return": {"index"},
		}, cookies)
		assert.Equal(t, http.StatusSeeOther, response.Code)
		assert.Contains(t, response.Header().Get("Location"), "/catalogs/classical?error=")
		assert.Contains(t, response.Header().Get("Location"), url.QueryEscape(classical.LegacyRowsReason))
	})

	t.Run("classical current reading redirects with feedback", func(t *testing.T) {
		h, cookies, csrf, _ := readingFixtureSessionWith(t, func(services *Services) {
			catalog, err := fixtures.NewClassicalCatalog()
			require.NoError(t, err)
			services.Classical = catalog
			services.Store.CatalogueRead = failingCatalogueRead{err: domain.ErrBookIsCurrentReading}
		})
		response := readingTestRequest(t, h, "/catalogs/classical/units/fixture-iliad/previously-read", url.Values{
			"csrf_token": {csrf}, "return": {"index"},
		}, cookies)
		assert.Equal(t, http.StatusSeeOther, response.Code)
		assert.Contains(t, response.Header().Get("Location"), "error=")
	})

	t.Run("classical unknown unit is not found", func(t *testing.T) {
		h, cookies, csrf, _ := readingFixtureSessionWith(t, func(services *Services) {
			catalog, err := fixtures.NewClassicalCatalog()
			require.NoError(t, err)
			services.Classical = catalog
		})
		response := readingTestRequest(t, h, "/catalogs/classical/units/no-such-unit/previously-read", url.Values{
			"csrf_token": {csrf},
		}, cookies)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}

func TestCatalogueReadMessageCoversCreatedChangedAndReplay(t *testing.T) {
	assert.Equal(t, "Recorded “Book” as previously read.", catalogueReadMessage(domain.CatalogueReadResult{Book: domain.Book{Title: "Book"}, Created: true}))
	assert.Equal(t, "Marked “Book” as previously read.", catalogueReadMessage(domain.CatalogueReadResult{Book: domain.Book{Title: "Book"}, Changed: true}))
	assert.Equal(t, "“Book” is already in Read.", catalogueReadMessage(domain.CatalogueReadResult{Book: domain.Book{Title: "Book"}}))
}

func TestCatalogueAddRouteRejectsMissingCSRFAndUnknownEntries(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSession(t)

	noCSRF := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/add", url.Values{
		"entry_identifier": {"fixture-sommer"},
	}, cookies)
	assert.Equal(t, http.StatusForbidden, noCSRF.Code)

	unknown := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/add", url.Values{
		"csrf_token":       {csrf},
		"entry_identifier": {"no-such-entry"},
	}, cookies)
	assert.Equal(t, http.StatusNotFound, unknown.Code)

	failing, failingCookies, failingCSRF, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Store.CatalogueAdd = failingCatalogueAdd{}
	})
	failed := readingTestRequest(t, failing, "/catalogs/sources/fixture-connection/add", url.Values{
		"csrf_token":       {failingCSRF},
		"entry_identifier": {"fixture-sommer"},
	}, failingCookies)
	assert.Equal(t, http.StatusInternalServerError, failed.Code)
}

func TestCataloguePreviouslyReadCreatesAReadBookOnTheCatalog(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)

	read := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/previously-read", url.Values{
		"csrf_token":       {csrf},
		"entry_identifier": {"fixture-uber"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, read.Code)
	location, err := url.Parse(read.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/catalogs", location.Path)
	assert.Equal(t, "Recorded “Über die Berge” as previously read.", location.Query().Get("message"))
	bookID := location.Query().Get("read_book")
	require.NotEmpty(t, bookID)

	detail, err := store.GetBookDetail(t.Context(), fixtures.OwnerID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, detail.Disposition, "a Read Book is not To Read")
	assert.Equal(t, domain.MyBookBucketRead, detail.WorkflowBucket())
	assert.Equal(t, 1, detail.CompletionCount)
	assert.Equal(t, domain.ReadingCompletionPreviouslyRead, detail.LatestCompletionSource)
	assert.False(t, detail.EverCurrentReading)

	page := catalogPage(t, h, cookies, read.Header().Get("Location"))
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, `id="catalog-read-confirmation"`)
	assert.Contains(t, body, `autofocus`)
	assert.Contains(t, body, "Recorded “Über die Berge” as previously read.")
	assert.Contains(t, body, `href="/catalogs/books/`+bookID+`/in-my-books">View in My Books</a>`)
}

func TestCataloguePreviouslyReadRouteRejectsMissingCSRFAndUnknownEntries(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSession(t)

	noCSRF := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/previously-read", url.Values{
		"entry_identifier": {"fixture-uber"},
	}, cookies)
	assert.Equal(t, http.StatusForbidden, noCSRF.Code)

	unknown := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/previously-read", url.Values{
		"csrf_token":       {csrf},
		"entry_identifier": {"no-such-entry"},
	}, cookies)
	assert.Equal(t, http.StatusNotFound, unknown.Code)
}

func TestClassicalUnitPreviouslyReadRecordsAReadBookWithoutPreparation(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSessionWith(t, func(services *Services) {
		catalog, err := fixtures.NewClassicalCatalog()
		require.NoError(t, err)
		services.Classical = catalog
	})

	read := readingTestRequest(t, h, "/catalogs/classical/units/fixture-iliad/previously-read", url.Values{
		"csrf_token": {csrf},
		"return":     {"index"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, read.Code)
	location, err := url.Parse(read.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/catalogs/classical", location.Path)
	assert.Equal(t, "Recorded “Iliad” as previously read.", location.Query().Get("message"))
	bookID := location.Query().Get("read_book")
	require.NotEmpty(t, bookID)

	detail, err := store.GetBookDetail(t.Context(), fixtures.OwnerID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, detail.Disposition, "a Read Book is not To Read")
	assert.Equal(t, domain.MyBookBucketRead, detail.WorkflowBucket())
	assert.Equal(t, 1, detail.CompletionCount)
	assert.Equal(t, domain.ReadingCompletionPreviouslyRead, detail.LatestCompletionSource)
	require.NotNil(t, detail.ReadingUnit)
	assert.Equal(t, "fixture-iliad", detail.ReadingUnit.ReadingUnitKey)

	page := catalogPage(t, h, cookies, read.Header().Get("Location"))
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, "Recorded “Iliad” as previously read.")
	assert.Contains(t, body, "Open Ancient Greek in My Books")
	assert.Contains(t, body, `href="/catalogs/classical/books/`+bookID+`/in-my-books"`)
}

func TestCatalogueAddPreparationMessagesStateWhatFollowsTheAdd(t *testing.T) {
	cases := []struct {
		name    string
		outcome readingintent.Outcome
		want    string
	}{
		{name: "queued", outcome: readingintent.Outcome{Kind: readingintent.OutcomeJobQueued}, want: " Downloading and analyzing the text."},
		{name: "already current", outcome: readingintent.Outcome{Kind: readingintent.OutcomeAlreadyCurrent}, want: " The text is already analyzed."},
		{name: "no usable units", outcome: readingintent.Outcome{Kind: readingintent.OutcomeNoUsableEPUBUnits}, want: " The current source has no usable EPUB units. The Book stays To Read."},
		{name: "could not start", outcome: readingintent.Outcome{Kind: readingintent.OutcomeCouldNotStart}, want: catalogueAddCouldNotStart},
		{name: "no submission", outcome: readingintent.Outcome{Kind: readingintent.OutcomeMoved}, want: ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, catalogueAddPreparationMessage(test.outcome))
		})
	}
}

func TestCatalogueAddPreparationWithoutAReadingIntentModuleKeepsTheAdd(t *testing.T) {
	handler := &Handler{}
	assert.Equal(t, catalogueAddCouldNotStart, handler.prepareAddedCatalogueBook(t.Context(), "owner", domain.Book{ID: "book"}))
}

func TestCatalogueAddMessageNamesTheBookAndMembershipChange(t *testing.T) {
	book := domain.Book{Title: "Add Entry"}
	assert.Equal(t, "Added “Add Entry” to My Books.", catalogueAddMessage(domain.CatalogueAddResult{Book: book, Created: true, Changed: true}))
	assert.Equal(t, "Restored “Add Entry” to My Books.", catalogueAddMessage(domain.CatalogueAddResult{Book: book, Changed: true}))
	assert.Equal(t, "“Add Entry” is already in My Books.", catalogueAddMessage(domain.CatalogueAddResult{Book: book}))
	assert.Equal(t, "Added “the text” to My Books.", catalogueAddMessage(domain.CatalogueAddResult{Created: true}))
}

func TestCatalogueAddConfirmationLinksOnlyTheEntryTheAddNamed(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)

	// The fixture Book is linked to fixture-browser-second on its connection.
	matching := catalogPage(t, h, cookies, "/catalogs?language=de&message=Added&added_book=fixture-book&added_connection=fixture-browser-sync-connection&added_entry=fixture-browser-second")
	require.Equal(t, http.StatusOK, matching.Code)
	body := matching.Body.String()
	assert.Contains(t, body, `id="catalog-add-confirmation"`)
	assert.Contains(t, body, `autofocus`)
	assert.Contains(t, body, `href="/catalogs/books/fixture-book/in-my-books">View in My Books</a>`)

	// A Book and entry that do not share an alias render the message without a
	// link or focus target.
	mismatched := catalogPage(t, h, cookies, "/catalogs?language=de&message=Added&added_book=fixture-book&added_connection=fixture-browser-sync-connection&added_entry=fixture-other")
	require.Equal(t, http.StatusOK, mismatched.Code)
	assert.NotContains(t, mismatched.Body.String(), `id="catalog-add-confirmation"`)
	assert.NotContains(t, mismatched.Body.String(), `>View in My Books</a>`)
}

func TestClassicalAddMessageNamesOnlyMembershipChanges(t *testing.T) {
	cases := []struct {
		name     string
		created  int
		opened   int
		expected string
	}{
		{name: "one reopened", opened: 1, expected: "Opened the existing Book."},
		{name: "several reopened", opened: 2, expected: "Opened 2 existing Books."},
		{name: "one created", created: 1, expected: "Added 1 Book to My Books."},
		{name: "several created", created: 2, expected: "Added 2 Books to My Books."},
		{name: "one created and one reopened", created: 1, opened: 1, expected: "Added 1 Book and opened 1 existing Book."},
		{name: "one created and several reopened", created: 1, opened: 2, expected: "Added 1 Book and opened 2 existing Books."},
		{name: "several created and one reopened", created: 2, opened: 1, expected: "Added 2 Books and opened 1 existing Book."},
		{name: "several created and reopened", created: 2, opened: 2, expected: "Added 2 Books and opened 2 existing Books."},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			report := classical.AddReport{}
			for range test.created {
				report.Books = append(report.Books, classical.AddedBook{Created: true})
			}
			for range test.opened {
				report.Books = append(report.Books, classical.AddedBook{})
			}
			assert.Equal(t, test.expected, classicalAddMessage(report))
		})
	}
}

func TestClassicalAddOutcomeNamesTheLanguageOnlyWhenAnotherIsActive(t *testing.T) {
	created := classical.AddReport{Books: []classical.AddedBook{{BookID: "added-book", Created: true}}}
	opened := classical.AddReport{Books: []classical.AddedBook{{BookID: "added-book"}}}
	languageMessage := func(message string) string {
		return strings.TrimSuffix(message, ".") + " in Ancient Greek. Your study language is unchanged."
	}
	cases := []struct {
		name           string
		report         classical.AddReport
		activeLanguage string
		expected       classicalAddConfirmation
	}{
		{name: "created with Ancient Greek active", report: created, activeLanguage: classical.Language, expected: classicalAddConfirmation{Message: "Added 1 Book to My Books."}},
		{name: "created with no study language", report: created, expected: classicalAddConfirmation{Message: "Added 1 Book to My Books."}},
		{name: "reopened with Ancient Greek active", report: opened, activeLanguage: classical.Language, expected: classicalAddConfirmation{Message: "Opened the existing Book."}},
		{name: "created with another language active", report: created, activeLanguage: "de", expected: classicalAddConfirmation{Message: languageMessage("Added 1 Book to My Books."), BookID: "added-book"}},
		{name: "reopened with another language active", report: opened, activeLanguage: "de", expected: classicalAddConfirmation{Message: languageMessage("Opened the existing Book."), BookID: "added-book"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, classicalAddConfirmationFor(test.report, classicalAddPreparation{}, test.activeLanguage))
		})
	}
}

func TestClassicalAddPreparationMessageStatesDownloadAndPreparation(t *testing.T) {
	cases := []struct {
		name        string
		preparation classicalAddPreparation
		expected    string
	}{
		{name: "queued", preparation: classicalAddPreparation{Queued: 2}, expected: " Downloading and preparing the text."},
		{name: "already prepared", preparation: classicalAddPreparation{AlreadyPrepared: 1}, expected: " The text is already prepared."},
		{name: "could not start", preparation: classicalAddPreparation{CouldNotStart: 1}, expected: " The text could not be prepared. Retry preparation from My Books when ready."},
		{name: "withdrawn", preparation: classicalAddPreparation{CouldNotStart: 1, Refusal: classicalWithdrawnRefusal}, expected: " " + classicalWithdrawnRefusal},
		{name: "nothing to submit", preparation: classicalAddPreparation{}, expected: ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, classicalAddPreparationMessage(test.preparation))
		})
	}
}

func TestClassicalAddRoutesRejectUnknownTargetsAndMissingCSRF(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
	})

	unknownWork := readingTestRequest(t, h, "/catalogs/classical/works/no-such-work/add", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusNotFound, unknownWork.Code)

	unknownUnit := readingTestRequest(t, h, "/catalogs/classical/units/no-such-unit/add", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusNotFound, unknownUnit.Code)

	noWorkCSRF := readingTestRequest(t, h, "/catalogs/classical/works/homer-iliad/add", nil, cookies)
	assert.Equal(t, http.StatusForbidden, noWorkCSRF.Code)

	noUnitCSRF := readingTestRequest(t, h, "/catalogs/classical/units/homer-iliad-1/add", nil, cookies)
	assert.Equal(t, http.StatusForbidden, noUnitCSRF.Code)

	unknownBook := catalogPage(t, h, cookies, "/catalogs/classical/books/no-such-book/in-my-books")
	assert.Equal(t, http.StatusNotFound, unknownBook.Code)

	ordinaryBook := catalogPage(t, h, cookies, "/catalogs/classical/books/"+fixtures.BookID+"/in-my-books")
	assert.Equal(t, http.StatusNotFound, ordinaryBook.Code)
}

func TestClassicalCatalogHidesWithdrawnAndUnknownEntries(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
	})

	assert.Equal(t, http.StatusNotFound, catalogPage(t, h, cookies, "/catalogs/classical/works/xenophon-cyropaedia").Code)
	assert.Equal(t, http.StatusNotFound, catalogPage(t, h, cookies, "/catalogs/classical/works/no-such-work").Code)
	assert.Equal(t, http.StatusNotFound, catalogPage(t, h, cookies, "/catalogs/classical/authors/"+url.PathEscape("No Such Author")).Code)
}

func TestCatalogLanguageFilterDefaultsToAllWithoutAStudyLanguage(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
		store, ok := services.Store.Shell.(*fixtures.Store)
		require.True(t, ok)
		services.Store.Shell = noStudyLanguageShell{Store: store}
	})

	response := catalogPage(t, h, cookies, "/catalogs")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, `<option value="" selected>All languages</option>`)
	assert.Contains(t, body, `id="catalog-source-classical"`, "all languages includes the Classical catalog")
	assert.Contains(t, body, "La città delle meraviglie", "all languages includes every OPDS language")
}

func TestManageSourcesRendersConnectionManagement(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/catalogs/sources?error=Sync+failed", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	for _, want := range []string{
		"<title>Manage sources · Mouseion</title>",
		"<h1>Manage sources</h1>",
		`href="/catalogs" aria-current="page"`,
		`href="/catalogs">← Catalog</a>`,
		"Add catalog connection",
		`action="/catalogs/sources"`,
		`action="/catalogs/sources/fixture-connection"`,
		`action="/catalogs/sources/fixture-connection/delete"`,
		`action="/catalogs/sources/fixture-connection/sync"`,
		"Authentication failed for this connection.",
		`class="feedback feedback--error"`,
		"Sync failed",
	} {
		assert.Contains(t, body, want, "GET /catalogs/sources missing %q", want)
	}
	assert.NotContains(t, body, "Classical catalog", "connection management does not duplicate the Catalog's classical record")
}

func TestManageSourcesWithoutConnectionsOffersTheFirstConnection(t *testing.T) {
	var sources bytes.Buffer
	require.NoError(t, ManageSourcesPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", nil).Render(t.Context(), &sources))
	for _, want := range []string{
		"Add your first catalog connection",
		`href="#catalog-connection-form"`,
		`action="/catalogs/sources"`,
	} {
		assert.Contains(t, sources.String(), want, "empty Manage sources missing %q", want)
	}
}

func TestConnectionMutationsLandOnManageSources(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSession(t)

	created := readingTestRequest(t, h, "/catalogs/sources", url.Values{
		"csrf_token": {csrf},
		"name":       {"New catalog"},
		"url":        {"https://example.invalid/opds"},
		"book_id":    {"book/1"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, created.Code)
	createdLocation, err := url.Parse(created.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/catalogs/sources", createdLocation.Path)
	assert.Equal(t, "Catalog added", createdLocation.Query().Get("message"))
	assert.Equal(t, "book/1", createdLocation.Query().Get("book_id"))

	updated := readingTestRequest(t, h, "/catalogs/sources/fixture-connection", url.Values{
		"csrf_token": {csrf},
		"name":       {"Renamed catalog"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, updated.Code)
	assert.Equal(t, "/catalogs/sources?message=Catalog+updated", updated.Header().Get("Location"))

	synced := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/sync", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, synced.Code)
	assert.Equal(t, "/catalogs/sources?message=Catalog+sync+submitted.", synced.Header().Get("Location"))

	// The Catalog's own status lines keep returning to the Catalog.
	catalogSync := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/sync", url.Values{"csrf_token": {csrf}, "return": {"catalog"}}, cookies)
	require.Equal(t, http.StatusSeeOther, catalogSync.Code)
	assert.Equal(t, "/catalogs?message=Catalog+sync+submitted.", catalogSync.Header().Get("Location"))

	// The retired legacy sync route remains supported and returns to the Catalog.
	legacySync := readingTestRequest(t, h, "/connections/fixture-connection/sync", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, legacySync.Code)
	assert.Equal(t, "/catalogs?message=Catalog+sync+submitted.", legacySync.Header().Get("Location"))

	deleted := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/delete", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, deleted.Code)
	assert.Equal(t, "/catalogs/sources?message=Catalog+deleted", deleted.Header().Get("Location"))

	missingDelete := readingTestRequest(t, h, "/catalogs/sources/no-such-connection/delete", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, missingDelete.Code)
	assert.Equal(t, "/catalogs/sources?error=That+catalog+connection+no+longer+exists.", missingDelete.Header().Get("Location"))

	// The legacy mutation endpoints remain supported and land on Manage sources.
	legacy := readingTestRequest(t, h, "/connections", url.Values{
		"csrf_token": {csrf},
		"name":       {"Legacy catalog"},
		"url":        {"https://example.invalid/opds"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, legacy.Code)
	assert.Equal(t, "/catalogs/sources?message=Catalog+added", legacy.Header().Get("Location"))
}

func TestManageSourcesShowsValidationAndMissingConnectionErrors(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSession(t)

	invalid := readingTestRequest(t, h, "/catalogs/sources", url.Values{
		"csrf_token": {csrf},
		"name":       {"Bad catalog"},
		"url":        {"ftp://example.invalid/opds"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, invalid.Code)
	location, err := url.Parse(invalid.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/catalogs/sources", location.Path)
	assert.Equal(t, "The catalog URL must use HTTP or HTTPS.", location.Query().Get("error"))

	invalidUpdate := readingTestRequest(t, h, "/catalogs/sources/fixture-connection", url.Values{
		"csrf_token": {csrf},
		"name":       {"Bad catalog"},
		"url":        {"ftp://example.invalid/opds"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, invalidUpdate.Code)
	assert.Contains(t, invalidUpdate.Header().Get("Location"), "The+catalog+URL+must+use+HTTP+or+HTTPS.")
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

func TestClassicalAddPreparationForClassifiesReadingIntentOutcomes(t *testing.T) {
	cases := []struct {
		name    string
		outcome readingintent.Outcome
		want    classicalAddPreparation
	}{
		{name: "queued", outcome: readingintent.Outcome{Kind: readingintent.OutcomeJobQueued}, want: classicalAddPreparation{Queued: 1}},
		{name: "already current", outcome: readingintent.Outcome{Kind: readingintent.OutcomeAlreadyCurrent}, want: classicalAddPreparation{AlreadyPrepared: 1}},
		{name: "current book", outcome: readingintent.Outcome{Kind: readingintent.OutcomeCurrentBookRefused}, want: classicalAddPreparation{AlreadyPrepared: 1}},
		{name: "withdrawn", outcome: readingintent.Outcome{Kind: readingintent.OutcomeWithdrawnDefinition}, want: classicalAddPreparation{CouldNotStart: 1, Refusal: classicalWithdrawnRefusal}},
		{name: "source refused", outcome: readingintent.Outcome{Kind: readingintent.OutcomeSourceRefused}, want: classicalAddPreparation{CouldNotStart: 1, Refusal: classicalSourceMismatchRefusal}},
		{name: "could not start", outcome: readingintent.Outcome{Kind: readingintent.OutcomeCouldNotStart}, want: classicalAddPreparation{CouldNotStart: 1}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, classicalAddPreparationFor(test.outcome))
		})
	}
}

func TestPrepareAddedClassicalBooksHandlesEmptyUnavailableAndFailedReads(t *testing.T) {
	empty := &Handler{}
	assert.Zero(t, empty.prepareAddedClassicalBooks(t.Context(), "owner", classical.AddReport{}))

	unavailable := empty.prepareAddedClassicalBooks(t.Context(), "owner", classical.AddReport{Books: []classical.AddedBook{{BookID: "book-1"}}})
	assert.Equal(t, 1, unavailable.CouldNotStart, "a handler without reading-intent collaborators reports the recovery sentence")

	module, err := readingintent.New(readingintent.Config{
		Books: failingAddBooks{}, CurrentReadings: inactiveAddCurrentReadings{},
		Dispositions: addDispositions{}, Preparation: noopPreparation{},
	})
	require.NoError(t, err)
	failedRead := (&Handler{services: Services{ReadingIntent: module}}).prepareAddedClassicalBooks(t.Context(), "owner", classical.AddReport{Books: []classical.AddedBook{{BookID: "book-1"}}})
	assert.Equal(t, 1, failedRead.CouldNotStart, "a Book that cannot be read never blocks the Add")

	withdrawnModule, err := readingintent.New(readingintent.Config{
		Books: withdrawnAddBooks{}, CurrentReadings: inactiveAddCurrentReadings{},
		Dispositions: addDispositions{}, Preparation: refusedPreparation{refusal: readingintent.RefusalWithdrawnDefinition},
	})
	require.NoError(t, err)
	withdrawn := (&Handler{services: Services{ReadingIntent: withdrawnModule}}).prepareAddedClassicalBooks(t.Context(), "owner", classical.AddReport{Books: []classical.AddedBook{{BookID: "book-1"}}})
	assert.Equal(t, 1, withdrawn.CouldNotStart)
	assert.Equal(t, classicalWithdrawnRefusal, withdrawn.Refusal, "the specific stable refusal survives the aggregate")
}

type failingAddBooks struct{}

func (failingAddBooks) GetBookDetail(context.Context, string, string) (domain.MyBook, error) {
	return domain.MyBook{}, errors.New("book read failed")
}

type withdrawnAddBooks struct{}

func (withdrawnAddBooks) GetBookDetail(context.Context, string, string) (domain.MyBook, error) {
	return domain.MyBook{
		Book:        domain.Book{ID: "book-1"},
		ReadingUnit: &domain.BookReadingUnit{ReadingUnitKey: "unit-1"},
		Disposition: domain.BookDispositionToRead,
	}, nil
}

type inactiveAddCurrentReadings struct{}

func (inactiveAddCurrentReadings) GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error) {
	return domain.CurrentReading{}, nil
}

type addDispositions struct{}

func (addDispositions) TransitionBookDisposition(context.Context, string, string, int64, domain.BookDisposition) (bool, error) {
	return true, nil
}

// noStudyLanguageShell removes every derived and remembered study language so
// the Catalog's default filter can be exercised.
type noStudyLanguageShell struct {
	*fixtures.Store
}

func (noStudyLanguageShell) ListStudyLanguages(context.Context, string) ([]domain.StudyLanguage, error) {
	return nil, nil
}

func (noStudyLanguageShell) ListKnownVocabularyLanguages(context.Context, string) ([]domain.StudyLanguage, error) {
	return nil, nil
}

func (noStudyLanguageShell) GetStoredActiveStudyLanguage(context.Context, string) (string, error) {
	return "", nil
}

func (noStudyLanguageShell) MostRecentlyActivatedStudyLanguage(context.Context, string) (string, error) {
	return "", nil
}

func TestCatalogOwnedEntryOpensTheBookInMyBooks(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
	})

	opened := catalogPage(t, h, cookies, "/catalogs/books/"+fixtures.BookID+"/in-my-books")
	require.Equal(t, http.StatusSeeOther, opened.Code)
	location := opened.Header().Get("Location")
	assert.Contains(t, location, "/library?")
	assert.Contains(t, location, "disposition=to_read")
	assert.Contains(t, location, "#book-row-"+fixtures.BookID)

	unknown := catalogPage(t, h, cookies, "/catalogs/books/no-such-book/in-my-books")
	assert.Equal(t, http.StatusNotFound, unknown.Code)
}

func TestCatalogSearchNeverReadsAnOpdsServer(t *testing.T) {
	// The Catalog handler has no OPDS client dependency: search and the OPDS
	// sections are rendered only from the store's local index. A nil OPDS seam
	// therefore still returns matches.
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
		services.OPDS = nil
	})

	response := catalogPage(t, h, cookies, "/catalogs?language=&q=uber")
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "Über die Berge")
}
