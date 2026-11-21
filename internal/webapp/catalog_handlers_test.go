package webapp

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
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

func TestCatalogKeepsTheClassicalRecordAndShowsSourceStatusLines(t *testing.T) {
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
		`href="/catalogs/sources">Manage sources</a>`,
		"Classical catalog",
		`aria-label="Running release counts"`,
		"<dt>Works</dt><dd>3</dd>",
		"<dt>Authors</dt><dd>2</dd>",
		"<dt>Reading units</dt><dd>4</dd>",
		"Sources are fetched at To Read",
		`href="/catalogs/classical"`,
		`id="source-status-fixture-connection"`,
		`id="source-status-fixture-failed-connection"`,
		`id="source-status-fixture-syncing-connection"`,
		`id="source-status-fixture-never-synced-connection"`,
		"Last synced",
		"Sync failed",
		"Syncing",
		"Never synced",
		`action="/connections/fixture-connection/sync"`,
		`href="/catalogs/sources#connection-fixture-failed-connection"`,
	} {
		assert.Contains(t, body, want, "GET /catalogs missing %q", want)
	}
	assert.NotContains(t, body, `id="catalog-connection-form"`, "connection management belongs on Manage sources")
	assert.NotContains(t, body, "Cyropaedia", "a withdrawn definition is hidden")
	assert.Less(t, strings.Index(body, `id="catalog-sources-heading"`), strings.Index(body, `id="classical-catalog"`), "source status lines sit above the Classical catalog")
}

func TestClassicalCatalogIndexRetiredAuthorPageAndWorkPage(t *testing.T) {
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
	assert.NotContains(t, index.Body.String(), "/catalogs/classical/authors/", "the index never links to the retired author page")

	retired := catalogPage(t, h, cookies, "/catalogs/classical/authors/"+url.PathEscape("Xenophon"))
	require.Equal(t, http.StatusMovedPermanently, retired.Code)
	assert.Equal(t, "/catalogs/classical#author-Xenophon", retired.Header().Get("Location"))

	work := catalogPage(t, h, cookies, "/catalogs/classical/works/xenophon-anabasis")
	require.Equal(t, http.StatusOK, work.Code)
	for _, want := range []string{
		"<th>Book</th>", "<th>Passage</th>", "<th>Words</th>", "Book 1", "1.1.1–1.10.19", "9305",
		`href="/catalogs/classical#author-Xenophon"`,
	} {
		assert.Contains(t, work.Body.String(), want, "work page missing %q", want)
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
		"Choose books",
		"Add the whole work",
		"Add the work",
		"Greek text source:",
		"OGA oga_src v0.2.0 Greek text",
		"CC BY-SA 4.0",
		`style="width:1.000%;"`,
		`style="width:2.000%;"`,
		`style="width:3.000%;"`,
		`style="width:5.000%;"`,
		`action="/catalogs/classical/works/fixture-iliad/add"`,
		`action="/catalogs/classical/units/fixture-iliad-2/add"`,
		`name="csrf_token" value="`,
		`name="return" value="index"`,
	} {
		assert.Contains(t, body, want, "catalog index missing %q", want)
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
	assert.Contains(t, body, "Add the whole work", "a multi-book Work with none owned offers the whole Work")
	assert.Contains(t, body, "Add the work", "a whole-work unit offers the work")
	assert.NotContains(t, body, "books in My Books")
	assert.NotContains(t, body, "In My Books")

	// Add one constituent book from the "Choose books" table. The existing
	// endpoint returns to the index with the existing confirmation.
	addedBook := readingTestRequest(t, h, "/catalogs/classical/units/fixture-iliad-1/add", url.Values{
		"csrf_token": {csrf},
		"return":     {"index"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, addedBook.Code)
	location := addedBook.Header().Get("Location")
	assert.Contains(t, location, "/catalogs/classical?")
	assert.Contains(t, location, url.QueryEscape("Added 1 Book to My Books"))

	partial := catalogPage(t, h, cookies, location)
	require.Equal(t, http.StatusOK, partial.Code)
	partialBody := partial.Body.String()
	for _, want := range []string{
		"Added 1 Book to My Books",
		"1 of 3 books in My Books",
		"Add the remaining books",
		`action="/catalogs/classical/units/fixture-iliad-2/add"`,
	} {
		assert.Contains(t, partialBody, want, "partly owned index missing %q", want)
	}
	assert.NotContains(t, partialBody, `action="/catalogs/classical/units/fixture-iliad-1/add"`, "an owned book offers no Add")
	assert.NotContains(t, partialBody, "Add the whole work")
	assert.NotContains(t, partialBody, "/catalogs/classical/authors/", "the Add confirmation never links to the retired author page")

	// Add a whole-work unit from the index margin.
	addedWork := readingTestRequest(t, h, "/catalogs/classical/works/fixture-hellenica/add", url.Values{
		"csrf_token": {csrf},
		"return":     {"index"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, addedWork.Code)
	workLocation := addedWork.Header().Get("Location")
	assert.Contains(t, workLocation, "/catalogs/classical?")
	assert.Contains(t, workLocation, url.QueryEscape("Added 1 Book to My Books"))

	ownedWhole := catalogPage(t, h, cookies, workLocation)
	require.Equal(t, http.StatusOK, ownedWhole.Code)
	ownedBody := ownedWhole.Body.String()
	assert.Contains(t, ownedBody, "In My Books")
	assert.NotContains(t, ownedBody, `action="/catalogs/classical/works/fixture-hellenica/add"`, "a fully owned whole-work unit offers no Add")

	// Complete the Work: every Book owned means no Add control for it.
	_, err := store.AddClassicalReadingUnits(ctx, fixtures.OwnerID, []string{"fixture-iliad-2", "fixture-iliad-3"})
	require.NoError(t, err)
	full := catalogPage(t, h, cookies, "/catalogs/classical")
	require.Equal(t, http.StatusOK, full.Code)
	fullBody := full.Body.String()
	assert.Contains(t, fullBody, "3 of 3 books in My Books")
	assert.NotContains(t, fullBody, `action="/catalogs/classical/works/fixture-iliad/add"`)
	assert.NotContains(t, fullBody, `action="/catalogs/classical/units/fixture-iliad-1/add"`)
	assert.NotContains(t, fullBody, `action="/catalogs/classical/units/fixture-iliad-2/add"`)
	assert.NotContains(t, fullBody, `action="/catalogs/classical/units/fixture-iliad-3/add"`)
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
	created := classical.AddReport{Books: []classical.AddedBook{{Created: true}}}
	opened := classical.AddReport{Books: []classical.AddedBook{{}}}
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
		{name: "created with another language active", report: created, activeLanguage: "de", expected: classicalAddConfirmation{Message: languageMessage("Added 1 Book to My Books."), WorkKey: "homer-iliad"}},
		{name: "reopened with another language active", report: opened, activeLanguage: "de", expected: classicalAddConfirmation{Message: languageMessage("Opened the existing Book."), WorkKey: "homer-iliad"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, classicalAddConfirmationFor(test.report, test.activeLanguage, "homer-iliad"))
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

	unknownWorkMyBooks := catalogPage(t, h, cookies, "/catalogs/classical/works/no-such-work/in-my-books")
	assert.Equal(t, http.StatusNotFound, unknownWorkMyBooks.Code)

	unownedWorkMyBooks := catalogPage(t, h, cookies, "/catalogs/classical/works/homer-iliad/in-my-books")
	assert.Equal(t, http.StatusNotFound, unownedWorkMyBooks.Code)

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

func TestCatalogWithoutConnectionsOffersConnectAnOPDSCatalog(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalogFixture(t)
	})

	var catalog bytes.Buffer
	require.NoError(t, CatalogPage(domain.User{Username: "learner"}, "csrf", nil, "", "", nil, catalogFixture(t)).Render(t.Context(), &catalog))
	for _, want := range []string{
		"No OPDS catalogs connected",
		`href="/catalogs/sources">Connect an OPDS catalog</a>`,
		"Classical catalog",
	} {
		assert.Contains(t, catalog.String(), want, "empty Catalog missing %q", want)
	}
	assert.Equal(t, http.StatusOK, catalogPage(t, h, cookies, "/catalogs").Code)
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

	deleted := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/delete", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, deleted.Code)
	assert.Equal(t, "/catalogs/sources?message=Catalog+deleted", deleted.Header().Get("Location"))

	synced := readingTestRequest(t, h, "/catalogs/sources/fixture-connection/sync", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, synced.Code)
	assert.Equal(t, "/catalogs/sources?message=Catalog+sync+submitted.", synced.Header().Get("Location"))

	// The Catalog's own status lines keep returning to the Catalog.
	catalogSync := readingTestRequest(t, h, "/connections/fixture-connection/sync", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, catalogSync.Code)
	assert.Equal(t, "/catalogs?message=Catalog+sync+submitted.", catalogSync.Header().Get("Location"))

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
