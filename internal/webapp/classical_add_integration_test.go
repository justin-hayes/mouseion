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
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
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
		Auth:            authService,
		WebAuth:         webauth.New(authService, false, time.Hour),
		Store:           storeDependencies(store),
		Analysis:        analysis.NewService(store.Pool(), nil),
		Classical:       catalog,
		SessionLifetime: time.Hour,
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

func TestClassicalCatalogAddsWholeWorkAndReopensTheSameBooks(t *testing.T) {
	ctx, store, pool, h, layerRequests := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-whole", "learner-password")
	_, err := store.CreateBook(ctx, domain.Book{
		OwnerID: learner.ID, Title: "Ein deutsches Buch", MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	work := perform(t, h, "GET", "/catalogs/classical/works/homer-iliad", nil, cookies)
	require.Equal(t, http.StatusOK, work.Code, work.Body.String())
	for _, want := range []string{"Add the whole work", `action="/catalogs/classical/works/homer-iliad/add"`, `action="/catalogs/classical/units/homer-iliad-1/add"`} {
		assert.Contains(t, work.Body.String(), want)
	}
	assert.NotContains(t, work.Body.String(), `href="/catalogs/classical/books/`, "an unowned unit has no In My Books link")

	added := perform(t, h, "POST", "/catalogs/classical/works/homer-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	assert.Contains(t, added.Header().Get("Location"), url.QueryEscape("Added 2 Books to My Books."))

	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	first, err := store.GetBookDetail(ctx, learner.ID, ids[0])
	require.NoError(t, err)
	assert.Equal(t, "Iliad", first.Book.Title)
	assert.Equal(t, "Homer", first.Book.Author)
	assert.Equal(t, classical.Language, first.Book.LanguageTag)
	assert.Equal(t, domain.BookDispositionInbox, first.Disposition)
	require.NotNil(t, first.ReadingUnit, "the Book is linked permanently to its Reading unit")

	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM analysis_jobs WHERE owner_id = $1`, learner.ID), "Add starts no job")
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM river_job`), "Add starts no River job")
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM source_materials WHERE owner_id = $1`, learner.ID), "Add downloads nothing")
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
	ownedPage := perform(t, h, "GET", "/catalogs/classical/works/homer-iliad", nil, cookies)
	require.Equal(t, http.StatusOK, ownedPage.Code)
	assert.Contains(t, ownedPage.Body.String(), "In My Books")
	assert.NotContains(t, ownedPage.Body.String(), "Not prepared")

	// Hide one Book, then re-add the work: the existing Book is reopened and
	// no duplicate appears.
	_, err = store.SetBookHidden(ctx, learner.ID, ids[0], 0, true)
	require.NoError(t, err)
	again := perform(t, h, "POST", "/catalogs/classical/works/homer-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, again.Code)
	assert.Contains(t, again.Header().Get("Location"), url.QueryEscape("Opened 2 existing Books."))
	assert.Equal(t, ids, ownerBooks(t, pool, learner.ID))
	hidden, _, err := store.GetBookVisibility(ctx, learner.ID, ids[0])
	require.NoError(t, err)
	assert.False(t, hidden, "re-adding reopens a hidden existing Book")

	singleAgain := perform(t, h, "POST", "/catalogs/classical/units/homer-iliad-1/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, singleAgain.Code)
	assert.Contains(t, singleAgain.Header().Get("Location"), url.QueryEscape("Opened the existing Book."))

	// An ordinary non-classical Book never reaches the classical landing route.
	deBook, err := store.CreateBook(ctx, domain.Book{
		OwnerID: learner.ID, Title: "Ein anderes deutsches Buch", MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)
	ordinary := perform(t, h, "GET", "/catalogs/classical/books/"+deBook.ID+"/in-my-books", nil, cookies)
	assert.Equal(t, http.StatusNotFound, ordinary.Code)

	// Following the work link opens the first owned Book in book order.
	owned, err := store.ListOwnedClassicalReadingUnits(ctx, learner.ID)
	require.NoError(t, err)
	firstIliadBook := ""
	for _, unit := range owned {
		if unit.ReadingUnitKey == "homer-iliad-1" {
			firstIliadBook = unit.BookID
		}
	}
	require.NotEmpty(t, firstIliadBook)
	workInMyBooks := perform(t, h, "GET", "/catalogs/classical/works/homer-iliad/in-my-books", nil, cookies)
	require.Equal(t, http.StatusSeeOther, workInMyBooks.Code)
	assert.Contains(t, workInMyBooks.Header().Get("Location"), "#"+myBookRowID(firstIliadBook))

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
	assert.Contains(t, library.Body.String(), "Iliad")
	assert.Contains(t, library.Body.String(), "Ancient Greek")
}

func TestClassicalCatalogAddsOneBookAndAWholeWorkUnitFromTheAuthorPage(t *testing.T) {
	ctx, store, pool, h, layerRequests := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-single", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	single := perform(t, h, "POST", "/catalogs/classical/units/xenophon-anabasis-1/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, single.Code)
	assert.Contains(t, single.Header().Get("Location"), url.QueryEscape("Added 1 Book to My Books."))
	require.Len(t, ownerBooks(t, pool, learner.ID), 1)

	author := perform(t, h, "GET", "/catalogs/classical/authors/"+url.PathEscape("Xenophon"), nil, cookies)
	require.Equal(t, http.StatusOK, author.Code)
	for _, want := range []string{`action="/catalogs/classical/works/xenophon-hellenica/add"`, `name="return" value="author"`} {
		assert.Contains(t, author.Body.String(), want, "whole-work unit is added from the author page")
	}

	whole := perform(t, h, "POST", "/catalogs/classical/works/xenophon-hellenica/add", url.Values{"csrf_token": {csrf}, "return": {"author"}}, cookies)
	require.Equal(t, http.StatusSeeOther, whole.Code)
	assert.Contains(t, whole.Header().Get("Location"), "/catalogs/classical/authors/"+url.PathEscape("Xenophon"))
	assert.Contains(t, whole.Header().Get("Location"), url.QueryEscape("Added 1 Book to My Books."))
	assert.Len(t, ownerBooks(t, pool, learner.ID), 2)
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM river_job`))
	assert.Zero(t, layerRequests.Load())
}

func TestClassicalCatalogAddReportsMixedCreatedAndReopenedBooks(t *testing.T) {
	ctx, store, _, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-mixed", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	single := perform(t, h, "POST", "/catalogs/classical/units/homer-iliad-1/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, single.Code)

	whole := perform(t, h, "POST", "/catalogs/classical/works/homer-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, whole.Code)
	assert.Contains(t, whole.Header().Get("Location"), url.QueryEscape("Added 1 Book and opened 1 existing Book."))
}

func TestClassicalCatalogAddFailureRedirectsWithRecoveryMessage(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-stale-unit", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	_, err := pool.Exec(ctx, `UPDATE reading_units SET withdrawn = true WHERE definition_key = 'homer-iliad-1'`)
	require.NoError(t, err)

	response := perform(t, h, "POST", "/catalogs/classical/works/homer-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
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
	assert.NotContains(t, work.Body.String(), `action="/catalogs/classical/works/homer-iliad/add"`)

	refused := perform(t, h, "POST", "/catalogs/classical/works/homer-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, refused.Code)
	assert.Contains(t, refused.Header().Get("Location"), url.QueryEscape(classical.LegacyRowsCode))
	refusedUnit := perform(t, h, "POST", "/catalogs/classical/units/homer-iliad-1/add", url.Values{"csrf_token": {csrf}}, cookies)
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
	report, err := store.AddClassicalReadingUnits(ctx, owner.ID, []string{"homer-iliad-1"})
	require.NoError(t, err)
	bookID := report.Books[0].BookID

	cookies, _ := loginCookies(t, h, other.Username, "learner-password")
	response := perform(t, h, "GET", "/catalogs/classical/books/"+bookID+"/in-my-books", nil, cookies)
	assert.Equal(t, http.StatusNotFound, response.Code)
	response = perform(t, h, "GET", "/catalogs/classical/works/homer-iliad/in-my-books", nil, cookies)
	assert.Equal(t, http.StatusNotFound, response.Code)
}

// TestClassicalCatalogAuthorIndexIsNotScopedToTheStudyLanguage keeps the
// pre-existing catalog rule: an active German learner still reaches the shelf.
func TestClassicalCatalogAuthorIndexIsNotScopedToTheStudyLanguage(t *testing.T) {
	ctx, store, _, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "classical-add-catalog-scope", "learner-password")
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "de"))
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")

	index := perform(t, h, "GET", "/catalogs/classical", nil, cookies)
	require.Equal(t, http.StatusOK, index.Code)
	assert.Contains(t, index.Body.String(), "Homer")
}
