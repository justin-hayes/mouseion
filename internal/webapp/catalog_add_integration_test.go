//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/testwrite"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catalogueAddServer serves only the EPUB the Catalog index advertises. Add
// must never walk a feed to acquire it.
func catalogueAddServer(t *testing.T, requests *catalogRequestLog, fail bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.record(r.URL.Path)
		if r.URL.Path != "/book.epub" {
			http.NotFound(w, r)
			return
		}
		if fail {
			http.Error(w, "catalog unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", opds.EPUBMediaType)
		testwrite.Bytes(t, w, testEPUBVariant(t, "add-entry", "Add Entry", "Hallo Welt."))
	}))
	t.Cleanup(server.Close)
	return server
}

type catalogueAddFixture struct {
	store      *persistence.PostgresStore
	handler    http.Handler
	owner      domain.User
	cookies    []*http.Cookie
	csrf       string
	connection domain.OpdsConnection
	analysis   *recordingAnalysis
	requests   *catalogRequestLog
}

// newCatalogueAddFixture seeds one synced connection and one unowned index
// entry, then composes the real reading-intent, OPDS, and Catalogue sync
// services over the fake catalog server.
func newCatalogueAddFixture(t *testing.T, fail bool) catalogueAddFixture {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "catalog-add-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	owner := createAccount(t, ctx, store, "catalog-add-owner", "owner-password")
	requests := &catalogRequestLog{}
	catalog := catalogueAddServer(t, requests, fail)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Add catalog", URL: catalog.URL + "/opds"})
	require.NoError(t, err)
	require.NoError(t, store.ReconcileCatalogueIndex(ctx, owner.ID, connection.ID, []domain.CatalogueIndexEntry{{
		ConnectionID:    connection.ID,
		EntryIdentifier: "add-entry",
		Title:           "Add Entry",
		Author:          "Add Author",
		LanguageTag:     "de",
		EPUBHref:        catalog.URL + "/book.epub",
	}}, time.Now()))
	opdsService := opds.NewService(store, epub.NewService(store), catalog.Client())
	syncService := cataloguesync.NewService(cataloguesync.StoreDependencies{Connections: store, Catalogue: store, Statuses: store}, nil, opdsService, readyGerman())
	recorder := &recordingAnalysis{}
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		OPDS: opdsService, Analysis: recorder, CatalogueSync: syncService,
		Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "owner-password")
	return catalogueAddFixture{
		store: store, handler: h, owner: owner, cookies: cookies, csrf: csrf,
		connection: connection, analysis: recorder, requests: requests,
	}
}

func (f catalogueAddFixture) add(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	return perform(t, f.handler, http.MethodPost, "/catalogs/sources/"+f.connection.ID+"/add", url.Values{
		"csrf_token":       {f.csrf},
		"entry_identifier": {"add-entry"},
	}, f.cookies)
}

func (f catalogueAddFixture) ownedBook(t *testing.T) domain.CatalogueEntry {
	t.Helper()
	page, err := f.store.BrowseCatalogueEntries(context.Background(), f.owner.ID, f.connection.ID, domain.CatalogueQuery{})
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	require.NotEmpty(t, page.Entries[0].BookID, "the added entry has no Book")
	return page.Entries[0]
}

func TestCatalogueAddCreatesToReadBookAndPreparesOnce(t *testing.T) {
	fixture := newCatalogueAddFixture(t, false)

	added := fixture.add(t)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	location, err := url.Parse(added.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/catalogs", location.Path)
	message := location.Query().Get("message")
	assert.Contains(t, message, "Added “Add Entry” to My Books.")
	assert.Contains(t, message, "Downloading and analyzing the text.")
	assert.Equal(t, "add-entry", location.Query().Get("added_entry"))
	assert.Equal(t, fixture.connection.ID, location.Query().Get("added_connection"))
	assert.Equal(t, []string{"/book.epub"}, fixture.requests.snapshot(), "Add walked a feed instead of the stored EPUB link")

	entry := fixture.ownedBook(t)
	detail, err := fixture.store.GetBookDetail(context.Background(), fixture.owner.ID, entry.BookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
	assert.Equal(t, "de", detail.Book.LanguageTag)
	assert.NotNil(t, detail.Acquired, "Add acquired the indexed EPUB")
	assert.Equal(t, 1, fixture.analysis.calls, "Add queues analysis once")

	languages, err := fixture.store.ListStudyLanguages(context.Background(), fixture.owner.ID)
	require.NoError(t, err)
	assert.Contains(t, languageTags(languages), "de", "the first Book in a language makes it a study language")

	// The confirmation names the Book, links to My Books, and receives focus
	// after the ordinary POST redirect.
	page := perform(t, fixture.handler, http.MethodGet, added.Header().Get("Location"), nil, fixture.cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	body := page.Body.String()
	assert.Contains(t, body, `id="catalog-add-confirmation"`)
	assert.Contains(t, body, `autofocus`)
	assert.Contains(t, body, `href="/catalogs/books/`+entry.BookID+`/in-my-books">View in My Books</a>`)
	assert.Contains(t, body, `href="/catalogs/books/`+entry.BookID+`/in-my-books">In My Books</a>`)

	// Replaying Add changes nothing and starts no second job.
	replayed := fixture.add(t)
	require.Equal(t, http.StatusSeeOther, replayed.Code, replayed.Body.String())
	assert.Contains(t, replayed.Header().Get("Location"), url.QueryEscape("“Add Entry” is already in My Books."))
	assert.Equal(t, 1, fixture.analysis.calls, "a replayed Add starts no duplicate job")
	again := fixture.ownedBook(t)
	assert.Equal(t, entry.BookID, again.BookID, "a replayed Add creates no duplicate Book")

	// Hiding and removing the Book, then adding the entry again, restores the
	// same Book visible and To Read.
	hidden, visibilityRevision, err := fixture.store.GetBookVisibility(context.Background(), fixture.owner.ID, entry.BookID)
	require.NoError(t, err)
	require.False(t, hidden)
	applied, err := fixture.store.SetBookHidden(context.Background(), fixture.owner.ID, entry.BookID, visibilityRevision, true)
	require.NoError(t, err)
	require.True(t, applied)
	removed := perform(t, fixture.handler, http.MethodPost, "/library/books/"+entry.BookID+"/remove", url.Values{
		"csrf_token": {fixture.csrf},
	}, fixture.cookies)
	require.Equal(t, http.StatusSeeOther, removed.Code, removed.Body.String())
	require.Contains(t, removed.Header().Get("Location"), url.QueryEscape("Removed from My Books."))
	restored := fixture.add(t)
	require.Equal(t, http.StatusSeeOther, restored.Code, restored.Body.String())
	restoredEntry := fixture.ownedBook(t)
	assert.Equal(t, entry.BookID, restoredEntry.BookID, "Add restores the same Book ID")
	restoredDetail, err := fixture.store.GetBookDetail(context.Background(), fixture.owner.ID, entry.BookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, restoredDetail.Disposition)
	assert.False(t, restoredDetail.Hidden, "Add makes the restored Book visible")
	assert.NotNil(t, restoredDetail.Acquired, "the restored Book keeps its acquired source")
}

func TestCatalogueAddRecordsAcquisitionFailureAndStaysToRead(t *testing.T) {
	fixture := newCatalogueAddFixture(t, true)

	added := fixture.add(t)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	location, err := url.Parse(added.Header().Get("Location"))
	require.NoError(t, err)
	assert.Contains(t, location.Query().Get("message"), "Added “Add Entry” to My Books.")
	assert.Contains(t, location.Query().Get("message"), "The Book stays To Read")

	entry := fixture.ownedBook(t)
	detail, err := fixture.store.GetBookDetail(context.Background(), fixture.owner.ID, entry.BookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
	require.NotNil(t, detail.EPUBAcquisition, "the failed acquisition is recorded as Book evidence")
	assert.Nil(t, detail.Acquired)
	assert.Zero(t, fixture.analysis.calls, "a failed acquisition starts no analysis")

	page := perform(t, fixture.handler, http.MethodGet, "/library?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	body := page.Body.String()
	assert.Contains(t, body, "Add Entry")
	assert.Contains(t, body, `action="/reading/books/`+entry.BookID+`/reanalyze"`, "My Books offers Retry")
	assert.Contains(t, body, "Retry", body)
}

func languageTags(languages []domain.StudyLanguage) []string {
	tags := make([]string, 0, len(languages))
	for _, language := range languages {
		tags = append(tags, language.Language)
	}
	return tags
}
