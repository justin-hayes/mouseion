//go:build integration

package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingTransport counts every default-client request. Add must make none.
type countingTransport struct {
	count *atomic.Int64
}

func (c countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.count.Add(1)
	return nil, errors.New("unexpected network request")
}

// classicalAddHarness provisions a migrated database, reconciles the fixture
// release, and wires the web handler to it. layerRequests counts every request
// a hypothetical layer fetch would make; Add must leave it at zero.
func classicalAddHarness(t *testing.T) (context.Context, *persistence.PostgresStore, *pgxpool.Pool, *Handler, *atomic.Int64) {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "classical-add-secret-012345678900")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	workers := river.NewWorkers()
	analysis.AddClassicalPreparationWorker(workers, store, analysis.ClassicalPreparationConfig{})
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{analysis.ClassicalPreparationQueue: {MaxWorkers: 1}},
		Workers: workers,
	})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "classical add client", client.Stop)
	preparation := analysis.NewService(store.Pool(), client).WithClassicalPreparationStore(store)
	definitions := catalogFixtureDefinitions()
	require.NoError(t, store.ReconcileClassicalManifest(ctx, definitions))
	catalog, err := classical.NewCatalog(definitions)
	require.NoError(t, err)
	authService := auth.New(store, time.Hour)
	var layerRequests atomic.Int64
	originalTransport := http.DefaultTransport
	http.DefaultTransport = countingTransport{count: &layerRequests}
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	h := New(Services{
		Auth:                 authService,
		WebAuth:              webauth.New(authService, false, time.Hour),
		Store:                storeDependencies(store),
		Analysis:             analysis.NewService(store.Pool(), nil),
		ClassicalPreparation: preparation,
		Classical:            catalog,
		CatalogueSync:        fixtures.NewCatalogueSync(fixtures.NewStore()),
		SessionLifetime:      time.Hour,
	})
	return ctx, store, pool, h, &layerRequests
}

func ownerBooks(t *testing.T, pool *pgxpool.Pool, owner string) (ids []string) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT id::text FROM books WHERE owner_id = $1 AND language_tag = $2 ORDER BY title, id`, owner, classical.Language)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(context.Background(), query, args...).Scan(&count))
	return count
}

func TestClassicalCatalogAddsWholeWorkAndReopensTheSameBook(t *testing.T) {
	ctx, store, pool, h, layerRequests := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-whole", "learner-password")
	_, err := store.CreateBook(ctx, domain.Book{
		OwnerID: learner.ID, Title: "Ein deutsches Buch", MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	work := perform(t, h, "GET", "/catalogs/classical/works/xenophon-hellenica", nil, cookies)
	require.Equal(t, http.StatusOK, work.Code, work.Body.String())
	for _, want := range []string{"Adding prepares the text for reading.", `action="/catalogs/classical/units/xenophon-hellenica/add"`} {
		assert.Contains(t, work.Body.String(), want)
	}
	assert.NotContains(t, work.Body.String(), `href="/catalogs/classical/books/`, "an unowned unit has no In My Books link")

	added := perform(t, h, "POST", "/catalogs/classical/units/xenophon-hellenica/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	assert.Contains(t, added.Header().Get("Location"), url.QueryEscape("Added 1 Book to My Books in Ancient Greek. Your study language is unchanged. Downloading and preparing the text."))
	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 1)
	assert.Contains(t, added.Header().Get("Location"), "added_book="+ids[0])

	addedBooksLink := classicalBookInMyBooksURL(ids[0])
	t.Run("the confirmation names Ancient Greek and links to the added Books", func(t *testing.T) {
		confirmation := perform(t, h, "GET", added.Header().Get("Location"), nil, cookies)
		require.Equal(t, http.StatusOK, confirmation.Code)
		require.Contains(t, confirmation.Body.String(), "Added 1 Book to My Books in Ancient Greek. Your study language is unchanged. Downloading and preparing the text.")
		require.Contains(t, confirmation.Body.String(), `href="`+addedBooksLink+`"`)
		require.Contains(t, confirmation.Body.String(), "Open Ancient Greek in My Books")
	})
	t.Run("a page query never invents a link for an unknown Book", func(t *testing.T) {
		unknownBook := perform(t, h, "GET", "/catalogs/classical/works/xenophon-hellenica?message="+url.QueryEscape("Added 1 Book to My Books in Ancient Greek.")+"&added_book=no-such-book", nil, cookies)
		require.Equal(t, http.StatusOK, unknownBook.Code)
		require.NotContains(t, unknownBook.Body.String(), "Open Ancient Greek in My Books")
	})

	first, err := store.GetBookDetail(ctx, learner.ID, ids[0])
	require.NoError(t, err)
	assert.Equal(t, "Hellenica", first.Book.Title)
	assert.Equal(t, "Xenophon", first.Book.Author)
	assert.Equal(t, classical.Language, first.Book.LanguageTag)
	assert.Equal(t, domain.BookDispositionToRead, first.Disposition)
	require.NotNil(t, first.ReadingUnit, "the Book is linked permanently to its Reading unit")

	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM analysis_jobs WHERE owner_id = $1`, learner.ID), "Add never uses the shared analysis path")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue), "Add queues one preparation per Book")
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM source_materials WHERE owner_id = $1`, learner.ID), "the worker, not Add, downloads the source")
	assert.Zero(t, layerRequests.Load(), "Add makes no layer request")

	active, err := store.GetStoredActiveStudyLanguage(ctx, learner.ID)
	require.NoError(t, err)
	assert.Equal(t, "de", active, "Add does not change the active study language")

	// Ancient Greek joins the study languages while German stays active.
	libraryAfterAdd := perform(t, h, "GET", "/library", nil, cookies)
	require.Equal(t, http.StatusOK, libraryAfterAdd.Code)
	assert.Contains(t, libraryAfterAdd.Body.String(), "Ancient Greek")
	assert.Contains(t, libraryAfterAdd.Body.String(), "Deutsch")

	// The added language is now a study language; the catalog shows ownership
	// instead of preparation or coverage.
	ownedPage := perform(t, h, "GET", "/catalogs/classical/works/xenophon-hellenica", nil, cookies)
	require.Equal(t, http.StatusOK, ownedPage.Code)
	assert.Contains(t, ownedPage.Body.String(), "In My Books")
	assert.NotContains(t, ownedPage.Body.String(), "Not prepared")

	// Re-add the work: the existing Book is reopened and no duplicate
	// appears.
	again := perform(t, h, "POST", "/catalogs/classical/units/xenophon-hellenica/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, again.Code)
	assert.Contains(t, again.Header().Get("Location"), url.QueryEscape("Opened the existing Book in Ancient Greek. Your study language is unchanged. Downloading and preparing the text."))
	assert.Contains(t, again.Header().Get("Location"), "added_book="+ids[0])
	assert.Equal(t, ids, ownerBooks(t, pool, learner.ID))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue), "ensure-once keeps one live job per Book")

	// An ordinary non-classical Book never reaches the classical landing route.
	deBook, err := store.CreateBook(ctx, domain.Book{
		OwnerID: learner.ID, Title: "Ein anderes deutsches Buch", MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)
	ordinary := perform(t, h, "GET", "/catalogs/classical/books/"+deBook.ID+"/in-my-books", nil, cookies)
	assert.Equal(t, http.StatusNotFound, ordinary.Code)

	// Following the confirmation link opens My Books in Ancient Greek at the
	// added work's row.
	workInMyBooks := perform(t, h, "GET", addedBooksLink, nil, cookies)
	require.Equal(t, http.StatusSeeOther, workInMyBooks.Code)
	assert.Contains(t, workInMyBooks.Header().Get("Location"), "/library?")
	assert.Contains(t, workInMyBooks.Header().Get("Location"), "#"+myBookRowID(ids[0]))

	// Following "In My Books" makes Ancient Greek active and lands on the row.
	inMyBooks := perform(t, h, "GET", "/catalogs/classical/books/"+ids[0]+"/in-my-books", nil, cookies)
	require.Equal(t, http.StatusSeeOther, inMyBooks.Code)
	location := inMyBooks.Header().Get("Location")
	assert.Contains(t, location, "/library?")
	assert.Contains(t, location, "#"+myBookRowID(ids[0]))
	active, err = store.GetStoredActiveStudyLanguage(ctx, learner.ID)
	require.NoError(t, err)
	assert.Equal(t, classical.Language, active)

	library := perform(t, h, "GET", "/library", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code)
	assert.Contains(t, library.Body.String(), "Hellenica")
	assert.Contains(t, library.Body.String(), "Ancient Greek")
}

func TestClassicalCatalogAddConfirmationIsUnchangedWhenAncientGreekIsActive(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-greek-active", "learner-password")
	// Ancient Greek is already the learner's study language and active.
	_, err := store.AddClassicalReadingUnits(ctx, learner.ID, []string{"xenophon-anabasis"})
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	added := perform(t, h, "POST", "/catalogs/classical/units/homer-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	location := added.Header().Get("Location")
	t.Run("the confirmation is unchanged", func(t *testing.T) {
		require.Contains(t, location, url.QueryEscape("Added 1 Book to My Books. Downloading and preparing the text."))
		require.NotContains(t, location, "Ancient Greek")
		require.NotContains(t, location, "added_book")
		page := perform(t, h, "GET", location, nil, cookies)
		require.Equal(t, http.StatusOK, page.Code)
		require.Contains(t, page.Body.String(), "Added 1 Book to My Books. Downloading and preparing the text.")
		require.NotContains(t, page.Body.String(), "Open Ancient Greek in My Books")
	})
	t.Run("Add does not change the active study language", func(t *testing.T) {
		active, err := store.GetStoredActiveStudyLanguage(ctx, learner.ID)
		require.NoError(t, err)
		require.Equal(t, classical.Language, active)
	})
	t.Run("the Books were added", func(t *testing.T) {
		require.Len(t, ownerBooks(t, pool, learner.ID), 2)
	})

	// A stale or crafted link query still names no language while Ancient Greek
	// is active.
	t.Run("a stale link query names no language", func(t *testing.T) {
		crafted := perform(t, h, "GET", "/catalogs/classical/works/homer-iliad?message="+url.QueryEscape("Added 1 Book to My Books.")+"&added_book="+booksByUnitKey(t, ctx, store, learner.ID, "homer-iliad"), nil, cookies)
		require.Equal(t, http.StatusOK, crafted.Code)
		require.NotContains(t, crafted.Body.String(), "Open Ancient Greek in My Books")
	})
}

func TestClassicalCatalogAddNamesAncientGreekForOneBook(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-one-book", "learner-password")
	_, err := store.CreateBook(ctx, domain.Book{
		OwnerID: learner.ID, Title: "Ein deutsches Buch", MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	added := perform(t, h, "POST", "/catalogs/classical/units/homer-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	location := added.Header().Get("Location")
	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 1)

	t.Run("the confirmation names Ancient Greek and keeps the language", func(t *testing.T) {
		require.Contains(t, location, url.QueryEscape("Added 1 Book to My Books in Ancient Greek. Your study language is unchanged. Downloading and preparing the text."))
		require.Contains(t, location, "added_book="+ids[0])
		active, err := store.GetStoredActiveStudyLanguage(ctx, learner.ID)
		require.NoError(t, err)
		require.Equal(t, "de", active)
	})
	t.Run("the confirmation links to the added Book", func(t *testing.T) {
		confirmation := perform(t, h, "GET", location, nil, cookies)
		require.Equal(t, http.StatusOK, confirmation.Code)
		require.Contains(t, confirmation.Body.String(), `href="`+classicalBookInMyBooksURL(ids[0])+`"`)
	})

	// Following the confirmation link opens My Books in Ancient Greek with the
	// added Book's work row visible.
	t.Run("following the link opens the added Book in Ancient Greek", func(t *testing.T) {
		follow := perform(t, h, "GET", classicalBookInMyBooksURL(ids[0]), nil, cookies)
		require.Equal(t, http.StatusSeeOther, follow.Code)
		require.Contains(t, follow.Header().Get("Location"), "/library?")
		require.Contains(t, follow.Header().Get("Location"), "#"+myBookRowID(ids[0]))
		active, err := store.GetStoredActiveStudyLanguage(ctx, learner.ID)
		require.NoError(t, err)
		require.Equal(t, classical.Language, active)
		library := perform(t, h, "GET", "/library", nil, cookies)
		require.Equal(t, http.StatusOK, library.Code)
		require.Contains(t, library.Body.String(), "Iliad")
		require.Contains(t, library.Body.String(), "Ancient Greek")
	})

	// Re-adding while another language is active follows the same rule.
	t.Run("reopening follows the same rule", func(t *testing.T) {
		require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))
		again := perform(t, h, "POST", "/catalogs/classical/units/homer-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
		require.Equal(t, http.StatusSeeOther, again.Code)
		require.Contains(t, again.Header().Get("Location"), url.QueryEscape("Opened the existing Book in Ancient Greek. Your study language is unchanged. Downloading and preparing the text."))
	})
}

func TestClassicalCatalogAddsOneBookAndAWholeWorkUnitFromTheIndex(t *testing.T) {
	ctx, store, pool, h, layerRequests := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-single", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	single := perform(t, h, "POST", "/catalogs/classical/units/xenophon-anabasis/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, single.Code)
	assert.Contains(t, single.Header().Get("Location"), url.QueryEscape("Added 1 Book to My Books. Downloading and preparing the text."))
	require.Len(t, ownerBooks(t, pool, learner.ID), 1)

	index := perform(t, h, "GET", "/catalogs?language=grc", nil, cookies)
	require.Equal(t, http.StatusOK, index.Code)
	for _, want := range []string{`action="/catalogs/classical/units/xenophon-hellenica/add"`, `name="return" value="index"`} {
		assert.Contains(t, index.Body.String(), want, "whole-work unit is added from its Catalog card")
	}

	whole := perform(t, h, "POST", "/catalogs/classical/units/xenophon-hellenica/add", url.Values{"csrf_token": {csrf}, "return": {"index"}, "catalog_language": {"grc"}}, cookies)
	require.Equal(t, http.StatusSeeOther, whole.Code)
	assert.Contains(t, whole.Header().Get("Location"), "/catalogs?language=grc&")
	assert.Contains(t, whole.Header().Get("Location"), url.QueryEscape("Added 1 Book to My Books. Downloading and preparing the text."))
	assert.Len(t, ownerBooks(t, pool, learner.ID), 2)
	assert.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue))
	assert.Zero(t, layerRequests.Load())
}

func TestClassicalCatalogOffersAndAddsEachEditionSeparately(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-editions", "learner-password")
	_, err := store.CreateBook(ctx, domain.Book{
		OwnerID: learner.ID, Title: "Ein deutsches Buch", MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	first := catalogFixtureWholeWorkWithBooks()
	second := first
	second.Key, second.EditionKey, second.EditionLabel = "plato-republic-second", "republic-second-edition", "Second Edition"
	definitions := []classical.Definition{first, second}
	require.NoError(t, store.ReconcileClassicalManifest(ctx, definitions))
	catalog, err := classical.NewCatalog(definitions)
	require.NoError(t, err)
	h.services.Classical = catalog
	const workURL = "/catalogs/classical/works/plato-republic"
	form := url.Values{"csrf_token": {csrf}}

	index := perform(t, h, "GET", "/catalogs?language=grc", nil, cookies)
	for _, unit := range definitions {
		require.Contains(t, index.Body.String(), `action="/catalogs/classical/units/`+unit.Key+`/add"`, "each Edition is its own card")
	}
	require.Contains(t, index.Body.String(), `class="catalog-entry__detail">Second Edition</p>`)
	require.Empty(t, ownerBooks(t, pool, learner.ID))

	page := perform(t, h, "GET", workURL, nil, cookies)
	for _, unit := range definitions {
		require.Contains(t, page.Body.String(), `action="/catalogs/classical/units/`+unit.Key+`/add"`)
		require.Contains(t, page.Body.String(), `action="/catalogs/classical/units/`+unit.Key+`/previously-read"`)
	}
	read := perform(t, h, "POST", "/catalogs/classical/units/"+first.Key+"/previously-read", form, cookies)
	require.Equal(t, http.StatusSeeOther, read.Code)
	require.Len(t, ownerBooks(t, pool, learner.ID), 1)
	require.Zero(t, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue))

	page = perform(t, h, "GET", workURL, nil, cookies)
	require.Contains(t, page.Body.String(), "In My Books")
	require.NotContains(t, page.Body.String(), `action="/catalogs/classical/units/`+first.Key+`/add"`)
	require.Contains(t, page.Body.String(), `action="/catalogs/classical/units/`+second.Key+`/add"`)
	index = perform(t, h, "GET", "/catalogs?language=grc", nil, cookies)
	require.NotContains(t, index.Body.String(), `action="/catalogs/classical/units/`+first.Key+`/add"`)
	require.Contains(t, index.Body.String(), `action="/catalogs/classical/units/`+second.Key+`/add"`)

	added := perform(t, h, "POST", "/catalogs/classical/units/"+second.Key+"/add", form, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	require.Len(t, ownerBooks(t, pool, learner.ID), 2)
	require.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue))
	bookID := booksByUnitKey(t, ctx, store, learner.ID, second.Key)
	feedback := perform(t, h, "GET", added.Header().Get("Location"), nil, cookies)
	require.Contains(t, feedback.Body.String(), `href="/catalogs/classical/books/`+bookID+`/in-my-books">Open Ancient Greek in My Books</a>`)
	index = perform(t, h, "GET", "/catalogs?language=grc", nil, cookies)
	require.NotContains(t, index.Body.String(), `/add"`, "every Edition is owned, so no card offers Add")
}

func TestClassicalCatalogAddFailureRedirectsWithRecoveryMessage(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-stale-unit", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	_, err := pool.Exec(ctx, `UPDATE reading_units SET withdrawn = true WHERE definition_key = 'homer-iliad'`)
	require.NoError(t, err)

	response := perform(t, h, "POST", "/catalogs/classical/units/homer-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, response.Code)
	location := response.Header().Get("Location")
	assert.Contains(t, location, url.QueryEscape("This text could not be added. Refresh the catalog and try again."))
	assert.NotContains(t, location, "not+current")
}

func TestClassicalCatalogAddIsUnavailableWithLegacyRowsWhileOPDSStillWorks(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-legacy", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	_, err := pool.Exec(ctx, `INSERT INTO books(owner_id, title, metadata_provenance, language_state, language_tag) VALUES ($1, 'Legacy Greek', 'catalog_sync', 'chosen', 'grc')`, learner.ID)
	require.NoError(t, err)

	work := perform(t, h, "GET", "/catalogs/classical/works/homer-iliad", nil, cookies)
	require.Equal(t, http.StatusOK, work.Code)
	assert.Contains(t, work.Body.String(), classical.LegacyRowsCode)
	assert.Contains(t, work.Body.String(), "Add is unavailable")

	refused := perform(t, h, "POST", "/catalogs/classical/units/homer-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, refused.Code)
	assert.Contains(t, refused.Header().Get("Location"), url.QueryEscape(classical.LegacyRowsCode))
	refusedUnit := perform(t, h, "POST", "/catalogs/classical/units/homer-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, refusedUnit.Code)
	assert.Contains(t, refusedUnit.Header().Get("Location"), url.QueryEscape(classical.LegacyRowsCode))
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM books WHERE owner_id = $1 AND metadata_provenance = $2`, learner.ID, domain.MetadataProvenanceOperatorRegistration))

	// OPDS connection management is unaffected by the classical precondition.
	created := perform(t, h, "POST", "/connections", url.Values{"csrf_token": {csrf}, "name": {"Still works"}, "url": {"https://example.invalid/opds"}}, cookies)
	require.Equal(t, http.StatusSeeOther, created.Code)
	assert.Contains(t, created.Header().Get("Location"), "/catalogs")
	connections, err := store.ListOpdsConnections(ctx, learner.ID)
	require.NoError(t, err)
	assert.Len(t, connections, 1)
}

func TestClassicalCatalogInMyBooksIsOwnerScoped(t *testing.T) {
	ctx, store, _, h, _ := classicalAddHarness(t)
	owner := createAccount(t, ctx, store, "classical-add-owner-scope", "learner-password")
	other := createAccount(t, ctx, store, "classical-add-other", "learner-password")
	report, err := store.AddClassicalReadingUnits(ctx, owner.ID, []string{"homer-iliad"})
	require.NoError(t, err)
	bookID := report.Books[0].BookID

	cookies, _ := loginCookies(t, h, other.Username, "learner-password")
	response := perform(t, h, "GET", "/catalogs/classical/books/"+bookID+"/in-my-books", nil, cookies)
	assert.Equal(t, http.StatusNotFound, response.Code)
}

// TestClassicalCatalogSectionIsNotScopedToTheStudyLanguage keeps the
// pre-existing catalog rule: an active German learner still reaches the shelf.
func TestClassicalCatalogSectionIsNotScopedToTheStudyLanguage(t *testing.T) {
	ctx, store, _, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-catalog-scope", "learner-password")
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")

	index := perform(t, h, "GET", "/catalogs?language=grc", nil, cookies)
	require.Equal(t, http.StatusOK, index.Code)
	assert.Contains(t, index.Body.String(), "Homer")
}

func TestClassicalCatalogWholeWorkAddIgnoresWithdrawnPerBookBooks(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-withdrawn-books", "learner-password")

	one, two := 1, 2
	perBook := func(book *int, key string, withdrawn bool) classical.Definition {
		start, end := "1.327", "1.354"
		if *book == 2 {
			start, end = "2.357", "2.383"
		}
		return catalogFixtureDefinition(key, "plato-republic", "Republic", "Plato", "Πολιτεία", "plato-republic-burnet-1905", start, end, book, 2, 9000, withdrawn, false)
	}
	require.NoError(t, store.ReconcileClassicalManifest(ctx, []classical.Definition{perBook(&one, "plato-republic-1", true), perBook(&two, "plato-republic-2", true)}))
	legacyUnit := perBook(&one, "plato-republic-1", true)
	registration, err := store.RegisterReadingUnitBook(ctx, learner.ID, domain.ReadingUnitDefinition{
		Key: legacyUnit.Key, Language: legacyUnit.Language, WorkTitle: legacyUnit.WorkTitle, WorkAuthor: legacyUnit.WorkAuthor,
		EditionLabel: legacyUnit.EditionLabel, CitationStart: legacyUnit.CitationStart, CitationEnd: legacyUnit.CitationEnd,
		SourceFileName: legacyUnit.Source.FileName, SourceFileSize: legacyUnit.Source.Size,
		SourceFileSHA256: legacyUnit.Source.SHA256, InterpretationVersion: legacyUnit.InterpretationVersion,
	})
	require.NoError(t, err)

	whole := catalogFixtureWholeWorkWithBooks()
	definitions := []classical.Definition{whole, perBook(&one, "plato-republic-1", true), perBook(&two, "plato-republic-2", true)}
	require.NoError(t, store.ReconcileClassicalManifest(ctx, definitions))
	catalog, err := classical.NewCatalog(definitions)
	require.NoError(t, err)
	h.services.Classical = catalog
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	page := perform(t, h, "GET", "/catalogs/classical/works/plato-republic", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), `action="/catalogs/classical/units/plato-republic/add"`, "owning only withdrawn per-book Books leaves the whole Work addable")
	assert.NotContains(t, page.Body.String(), "In My Books")
	assert.NotContains(t, page.Body.String(), "plato-republic-1")

	added := perform(t, h, "POST", "/catalogs/classical/units/plato-republic/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	assert.Contains(t, added.Header().Get("Location"), url.QueryEscape("Added 1 Book to My Books. Downloading and preparing the text."))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue), "one Classical preparation covers the whole Work")

	refused := perform(t, h, "POST", "/catalogs/classical/units/plato-republic-2/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusNotFound, refused.Code, "a withdrawn per-book definition is hidden from the Catalog")

	owned := perform(t, h, "GET", "/catalogs/classical/works/plato-republic", nil, cookies)
	assert.Contains(t, owned.Body.String(), "In My Books")
	assert.NotContains(t, owned.Body.String(), `action="/catalogs/classical/units/plato-republic/add"`)

	removed := perform(t, h, "POST", "/library/books/"+registration.BookID+"/remove", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, removed.Code)
	require.Contains(t, removed.Header().Get("Location"), url.QueryEscape("This Reading unit is no longer in the Catalog, so it can't be added back."))
	require.NotContains(t, removed.Header().Get("Location"), url.QueryEscape("the Catalog can add it back"))

	retried := perform(t, h, "POST", "/library/books/"+registration.BookID+"/remove", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, retried.Code)
	require.NotContains(t, retried.Header().Get("Location"), url.QueryEscape("The Catalog can add it back."))
}
