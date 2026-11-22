//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
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
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catalogRequestLog records every path the fake OPDS server served, so a test
// can prove acquisition read only the indexed EPUB link.
type catalogRequestLog struct {
	mu    sync.Mutex
	paths []string
}

func (l *catalogRequestLog) record(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.paths = append(l.paths, path)
}

func (l *catalogRequestLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.paths...)
}

func (l *catalogRequestLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.paths = nil
}

// indexedCatalog serves a language navigation feed and one language feed whose
// entry advertises the EPUB link the index stores. Acquisition must never touch
// either feed path.
func indexedCatalog(t *testing.T, requests *catalogRequestLog) *httptest.Server {
	t.Helper()
	const languageFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>German</title>
  <id>urn:mouseion:language:7</id>
  <entry>
    <id>indexed-entry</id>
    <title>Indexed title</title>
    <author><name>Indexed author</name></author>
    <link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/book.epub"/>
  </entry>
</feed>`
	const navigationFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Catalog</title>
  <id>urn:mouseion:root</id>
  <entry>
    <title>German</title>
    <id>urn:mouseion:language:7</id>
    <link rel="subsection" href="/opds/language/7"/>
  </entry>
</feed>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.record(r.URL.Path)
		switch r.URL.Path {
		case "/book.epub":
			w.Header().Set("Content-Type", opds.EPUBMediaType)
			testwrite.Bytes(t, w, testEPUBVariant(t, "indexed-entry", "Indexed title", "Hallo Welt."))
		case "/opds", "/opds/language", "/opds/language/7":
			w.Header().Set("Content-Type", "application/atom+xml")
			body := languageFeed
			if r.URL.Path != "/opds/language/7" {
				body = navigationFeed
			}
			testwrite.Bytes(t, w, []byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

type indexedAcquisitionFixture struct {
	store      *persistence.PostgresStore
	handler    http.Handler
	owner      domain.User
	cookies    []*http.Cookie
	csrf       string
	bookID     string
	connection domain.OpdsConnection
	analysis   *recordingAnalysis
	reader     *opds.Service
	requests   *catalogRequestLog
}

// newIndexedAcquisitionFixture seeds one connection and one metadata-only Book,
// then composes the web app over the real Catalog index-backed sync service and
// OPDS service. The fixture deliberately omits an index entry until a test adds
// one, mirroring an entry that left the index after a sync.
func newIndexedAcquisitionFixture(t *testing.T) indexedAcquisitionFixture {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "epub-acquisition-index-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	owner := createAccount(t, ctx, store, "acquisition-index-owner", "owner-password")
	requests := &catalogRequestLog{}
	catalog := indexedCatalog(t, requests)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Index catalog", URL: catalog.URL + "/opds"})
	require.NoError(t, err)
	bookResult, err := seedWebappCatalogueBook(t, ctx, store, owner.ID, connection.ID, "indexed-entry", "Indexed title", "Indexed author", "de")
	require.NoError(t, err)
	bookState, err := store.GetBookDetail(ctx, owner.ID, bookResult.Book.ID)
	require.NoError(t, err)
	_, err = store.TransitionBookDisposition(ctx, owner.ID, bookResult.Book.ID, bookState.DispositionRevision, domain.BookDispositionInbox)
	require.NoError(t, err)
	// Start with no index entry so the missing-entry path is reachable.
	require.NoError(t, store.ReconcileCatalogueIndex(ctx, owner.ID, connection.ID, nil, time.Now()))
	opdsService := opds.NewService(store, epub.NewService(store), catalog.Client())
	syncService := cataloguesync.NewService(cataloguesync.StoreDependencies{Connections: store, Catalogue: store, Statuses: store}, nil, opdsService, readyGerman())
	recorder := &recordingAnalysis{}
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		OPDS: opdsService, Analysis: recorder, CatalogueSync: syncService,
		Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "owner-password")
	return indexedAcquisitionFixture{
		store: store, handler: h, owner: owner, cookies: cookies, csrf: csrf,
		bookID: bookResult.Book.ID, connection: connection, analysis: recorder, reader: opdsService, requests: requests,
	}
}

// syncConnection runs the real catalogue sync worker so the index the Retry
// test depends on is restored by the same path a learner's Sync now uses.
func (f indexedAcquisitionFixture) syncConnection(t *testing.T) {
	t.Helper()
	worker := &cataloguesync.Worker{Connections: f.store, Catalogue: f.store, Statuses: f.store, Reader: f.reader, Capabilities: readyGerman()}
	require.NoError(t, worker.Work(context.Background(), &river.Job[cataloguesync.SyncArgs]{
		Args: cataloguesync.SyncArgs{OwnerID: f.owner.ID, ConnectionID: f.connection.ID},
	}))
}

func (f indexedAcquisitionFixture) storeIndexEntry(t *testing.T) {
	t.Helper()
	require.NoError(t, f.store.ReconcileCatalogueIndex(context.Background(), f.owner.ID, f.connection.ID, []domain.CatalogueIndexEntry{{
		ConnectionID:    f.connection.ID,
		EntryIdentifier: "indexed-entry",
		Title:           "Indexed title",
		Author:          "Indexed author",
		LanguageTag:     "de",
		EPUBHref:        "/book.epub",
	}}, time.Now()))
}

func (f indexedAcquisitionFixture) moveToRead(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	state, err := f.store.GetBookDetail(context.Background(), f.owner.ID, f.bookID)
	require.NoError(t, err)
	return perform(t, f.handler, http.MethodPost, "/library/books/"+f.bookID+"/to-read", url.Values{
		"csrf_token":        {f.csrf},
		"expected_revision": {strconv.FormatInt(state.DispositionRevision, 10)},
	}, f.cookies)
}

func (f indexedAcquisitionFixture) retry(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	return perform(t, f.handler, http.MethodPost, "/reading/books/"+f.bookID+"/reanalyze", url.Values{
		"csrf_token": {f.csrf},
	}, f.cookies)
}

func TestEPUBAcquisitionFetchesOnlyTheIndexedLink(t *testing.T) {
	fixture := newIndexedAcquisitionFixture(t)
	fixture.storeIndexEntry(t)

	moved := fixture.moveToRead(t)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

	assert.Equal(t, []string{"/book.epub"}, fixture.requests.snapshot(), "acquisition walked a language feed instead of the index")
	detail, err := fixture.store.GetBookDetail(context.Background(), fixture.owner.ID, fixture.bookID)
	require.NoError(t, err)
	assert.NotNil(t, detail.Acquired, "the indexed EPUB link was not acquired")
	assert.Nil(t, detail.EPUBAcquisition)
	assert.Equal(t, 1, fixture.analysis.calls, "the acquired Book starts analysis")
}

func TestEPUBAcquisitionMissingIndexEntryRecordsRetryableEntryFailure(t *testing.T) {
	fixture := newIndexedAcquisitionFixture(t)

	moved := fixture.moveToRead(t)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

	detail, err := fixture.store.GetBookDetail(context.Background(), fixture.owner.ID, fixture.bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, detail.Disposition, "the To Read choice is retained")
	require.NotNil(t, detail.EPUBAcquisition)
	assert.Equal(t, domain.BookPreparationStageEntry, detail.EPUBAcquisition.Stage)
	assert.Equal(t, domain.BookPreparationCodeCatalogEntryMissing, detail.EPUBAcquisition.Code)
	assert.Nil(t, detail.Acquired)
	assert.Zero(t, fixture.analysis.calls, "a missing entry starts no analysis")
	assert.Empty(t, fixture.requests.snapshot(), "a missing entry requests no catalog page")

	page := perform(t, fixture.handler, http.MethodGet, "/library?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	body := page.Body.String()
	assert.Contains(t, body, epubAcquisitionEntryMissingLabel)
	assert.Contains(t, body, `action="/reading/books/`+fixture.bookID+`/reanalyze"`)

	// A later sync restores the entry; Retry then acquires it and clears the
	// recorded reason without changing the To Read choice.
	fixture.syncConnection(t)
	_, err = fixture.store.GetCatalogueEntry(context.Background(), fixture.owner.ID, fixture.connection.ID, "indexed-entry")
	require.NoError(t, err, "sync did not restore the indexed entry")
	fixture.requests.reset()
	retried := fixture.retry(t)
	require.Equal(t, http.StatusSeeOther, retried.Code, retried.Body.String())

	detail, err = fixture.store.GetBookDetail(context.Background(), fixture.owner.ID, fixture.bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
	assert.Nil(t, detail.EPUBAcquisition, "a successful retry clears the reason")
	require.NotNil(t, detail.Acquired)
	assert.Equal(t, 1, fixture.analysis.calls)
	assert.Equal(t, []string{"/book.epub"}, fixture.requests.snapshot(), "retry walked a language feed instead of the index")

	cleared := perform(t, fixture.handler, http.MethodGet, "/library?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, cleared.Code)
	assert.NotContains(t, cleared.Body.String(), epubAcquisitionEntryMissingLabel)
}
