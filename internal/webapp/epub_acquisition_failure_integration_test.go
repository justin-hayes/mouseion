//go:build integration

package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

// failingAcquisitionImporter downloads and retains normally but refuses every
// import, so the acquisition reaches the import stage.
type failingAcquisitionImporter struct {
	service *epub.Service
	err     error
}

func (f failingAcquisitionImporter) Import(ctx context.Context, owner, language string, content []byte) (epub.ImportResult, error) {
	return f.service.Import(ctx, owner, language, content)
}

func (f failingAcquisitionImporter) ImportForAcquisition(ctx context.Context, owner, language string, acquisition epub.AcquisitionContent) (epub.ImportResult, error) {
	return f.service.ImportForAcquisition(ctx, owner, language, acquisition)
}

func (f failingAcquisitionImporter) ImportForAcquisitionForBook(context.Context, string, string, string, epub.AcquisitionContent) (epub.ImportResult, error) {
	return epub.ImportResult{}, f.err
}

type acquisitionFailureFixture struct {
	store      *persistence.PostgresStore
	handler    http.Handler
	owner      domain.User
	cookies    []*http.Cookie
	csrf       string
	bookID     string
	connection domain.OpdsConnection
	analysis   *recordingAnalysis
}

// newAcquisitionFailureFixture seeds one metadata-only catalog Book and the web
// app over the real OPDS service, the fake catalog server and an importer that
// either succeeds or refuses the Book import.
func newAcquisitionFailureFixture(t *testing.T, catalog *httptest.Server, failImport bool) acquisitionFailureFixture {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "epub-acquisition-failure-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	owner := createAccount(t, ctx, store, "acquisition-failure-owner", "owner-password")
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Failure catalog", URL: catalog.URL + "/opds"})
	require.NoError(t, err)
	bookResult, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "failure-entry", "Der lange Weg nach Hause", "", "de")
	require.NoError(t, err)
	target := cataloguesync.AcquisitionTarget{
		ConnectionID: connection.ID,
		Language:     "de",
		Entry: opds.Entry{
			ID: "failure-entry", Title: "Der lange Weg nach Hause",
			Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "/book.epub"}},
		},
		Href: "/book.epub",
	}
	epubService := epub.NewService(store)
	var importer opds.Importer = epubService
	if failImport {
		importer = failingAcquisitionImporter{service: epubService, err: errors.New("import refused")}
	}
	recorder := &recordingAnalysis{}
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		OPDS:     opds.NewService(store, importer, catalog.Client()),
		Analysis: recorder, CatalogueSync: &metadataBookAcquisitionStub{target: target},
		Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "owner-password")
	return acquisitionFailureFixture{
		store: store, handler: h, owner: owner, cookies: cookies, csrf: csrf,
		bookID: bookResult.Book.ID, connection: connection, analysis: recorder,
	}
}

func (f acquisitionFailureFixture) moveToRead(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	state, err := f.store.GetBookDetail(context.Background(), f.owner.ID, f.bookID)
	require.NoError(t, err)
	return perform(t, f.handler, http.MethodPost, "/library/books/"+f.bookID+"/to-read", url.Values{
		"csrf_token":        {f.csrf},
		"expected_revision": {strconv.FormatInt(state.DispositionRevision, 10)},
	}, f.cookies)
}

func (f acquisitionFailureFixture) retry(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	return perform(t, f.handler, http.MethodPost, "/reading/books/"+f.bookID+"/reanalyze", url.Values{
		"csrf_token": {f.csrf},
	}, f.cookies)
}

func TestEPUBAcquisitionDownloadFailureIsRecordedAndLaterRetryClearsIt(t *testing.T) {
	failing := true
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/book.epub" {
			http.NotFound(w, r)
			return
		}
		if failing {
			http.Error(w, "catalog is down", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", opds.EPUBMediaType)
		testwrite.Bytes(t, w, testEPUBVariant(t, "failure-entry", "Der lange Weg nach Hause", "Hallo Welt."))
	}))
	defer catalog.Close()
	fixture := newAcquisitionFailureFixture(t, catalog, false)

	moved := fixture.moveToRead(t)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

	detail, err := fixture.store.GetBookDetail(context.Background(), fixture.owner.ID, fixture.bookID)
	require.NoError(t, err)
	require.Equal(t, domain.BookDispositionToRead, detail.Disposition)
	require.NotNil(t, detail.EPUBAcquisition)
	assert.Equal(t, domain.BookPreparationStageFetch, detail.EPUBAcquisition.Stage)
	assert.Equal(t, domain.BookPreparationCodeSourceDownloadFailed, detail.EPUBAcquisition.Code)
	assert.Zero(t, fixture.analysis.calls, "a failed acquisition starts no analysis")

	page := perform(t, fixture.handler, http.MethodGet, "/library?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	body := page.Body.String()
	assert.Contains(t, body, epubAcquisitionDownloadFailedLabel)
	assert.Contains(t, body, `action="/reading/books/`+fixture.bookID+`/reanalyze"`)
	assert.Contains(t, body, ">Retry<")

	failing = false
	retried := fixture.retry(t)
	require.Equal(t, http.StatusSeeOther, retried.Code, retried.Body.String())

	detail, err = fixture.store.GetBookDetail(context.Background(), fixture.owner.ID, fixture.bookID)
	require.NoError(t, err)
	assert.Nil(t, detail.EPUBAcquisition, "a successful acquisition clears the failure")
	assert.NotNil(t, detail.Acquired, "the retry acquires the EPUB")
	assert.Equal(t, 1, fixture.analysis.calls, "the retry starts analysis")

	cleared := perform(t, fixture.handler, http.MethodGet, "/library?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, cleared.Code)
	assert.NotContains(t, cleared.Body.String(), epubAcquisitionDownloadFailedLabel)
}

func TestEPUBAcquisitionSourceRefusalRecordsNoRetry(t *testing.T) {
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/book.epub" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		testwrite.Bytes(t, w, []byte("%PDF-1.7 not an EPUB"))
	}))
	defer catalog.Close()
	fixture := newAcquisitionFailureFixture(t, catalog, false)

	moved := fixture.moveToRead(t)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

	detail, err := fixture.store.GetBookDetail(context.Background(), fixture.owner.ID, fixture.bookID)
	require.NoError(t, err)
	require.NotNil(t, detail.EPUBAcquisition)
	assert.Equal(t, domain.BookPreparationStageSource, detail.EPUBAcquisition.Stage)
	assert.Equal(t, domain.BookPreparationCodeSourceFormatUnknown, detail.EPUBAcquisition.Code)
	assert.Equal(t, domain.RecoveryNone, detail.Classification().Recovery)

	page := perform(t, fixture.handler, http.MethodGet, "/library?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, epubAcquisitionFormatLabel)
	assert.NotContains(t, body, `action="/reading/books/`+fixture.bookID+`/reanalyze"`)
}

func TestEPUBAcquisitionImportFailureRecordsRetry(t *testing.T) {
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/book.epub" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", opds.EPUBMediaType)
		testwrite.Bytes(t, w, testEPUBVariant(t, "failure-entry", "Der lange Weg nach Hause", "Hallo Welt."))
	}))
	defer catalog.Close()
	// The importer refuses the explicit Book import after the download and
	// retention succeed.
	fixture := newAcquisitionFailureFixture(t, catalog, true)

	moved := fixture.moveToRead(t)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

	detail, err := fixture.store.GetBookDetail(context.Background(), fixture.owner.ID, fixture.bookID)
	require.NoError(t, err)
	require.NotNil(t, detail.EPUBAcquisition)
	assert.Equal(t, domain.BookPreparationStageImport, detail.EPUBAcquisition.Stage)
	assert.Equal(t, domain.BookPreparationCodeEPUBImportFailed, detail.EPUBAcquisition.Code)
	assert.Equal(t, domain.RecoveryRetryAcquisition, detail.Classification().Recovery)

	page := perform(t, fixture.handler, http.MethodGet, "/library?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, epubAcquisitionImportFailedLabel)
	assert.Contains(t, body, `action="/reading/books/`+fixture.bookID+`/reanalyze"`)
	assert.Contains(t, body, ">Retry<", "the row offers Retry")
}
