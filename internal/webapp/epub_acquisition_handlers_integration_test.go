//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type acquisitionHandlerFixture struct {
	store      *persistence.PostgresStore
	pool       *pgxpool.Pool
	handler    http.Handler
	owner      domain.User
	cookies    []*http.Cookie
	csrf       string
	connection domain.OpdsConnection
	analysis   *recordingAnalysis
	requests   *atomic.Int64
}

// newAcquisitionHandlerFixture seeds one owner and connection, migrates River,
// and wires the real acquisition submission service over a River client that is
// never started. The Catalog records every request so the handler tests can
// prove Add, Move and Retry do not contact it.
func newAcquisitionHandlerFixture(t *testing.T) acquisitionHandlerFixture {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "epub-acquisition-handler-secret-0123456789")
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	authService := auth.New(store, time.Hour)
	owner := createAccount(t, ctx, store, "acquisition-handler-owner", "owner-password")

	var requests atomic.Int64
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(catalog.Close)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Handler catalog", URL: catalog.URL + "/opds"})
	require.NoError(t, err)

	workers := river.NewWorkers()
	analysis.AddEPUBAcquisitionWorker(workers, nil, nil, nil)
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{analysis.EPUBAcquisitionQueue: {MaxWorkers: 1}},
		Workers: workers,
	})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "acquisition handler client", client.Stop)

	recorder := &recordingAnalysis{}
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		OPDS:            opds.NewService(store, epub.NewService(store), catalog.Client()),
		Analysis:        recorder,
		EPUBAcquisition: analysis.NewService(store.Pool(), client),
		Capabilities:    readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "owner-password")
	return acquisitionHandlerFixture{
		store: store, pool: pool, handler: h, owner: owner, cookies: cookies, csrf: csrf,
		connection: connection, analysis: recorder, requests: &requests,
	}
}

func (f acquisitionHandlerFixture) acquisitionJobCount(t *testing.T) int {
	t.Helper()
	return countRows(t, f.pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.EPUBAcquisitionQueue)
}

func (f acquisitionHandlerFixture) createToReadBook(t *testing.T, title string) domain.Book {
	t.Helper()
	book, err := f.store.CreateBook(context.Background(), domain.Book{
		OwnerID: f.owner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)
	require.NoError(t, f.store.SetBookDisposition(context.Background(), f.owner.ID, book.ID, domain.BookDispositionToRead))
	return book
}

func TestEPUBAcquisitionAddEnqueuesOneJobWithoutContactingTheCatalog(t *testing.T) {
	fixture := newAcquisitionHandlerFixture(t)
	ctx := context.Background()
	require.NoError(t, fixture.store.ReconcileCatalogueIndex(ctx, fixture.owner.ID, fixture.connection.ID, []domain.CatalogueIndexEntry{{
		ConnectionID: fixture.connection.ID, EntryIdentifier: "add-entry", Title: "Add Entry",
		Author: "Author", LanguageTag: "de", EPUBHref: "https://catalog.invalid/add-entry.epub",
	}}, time.Now()))

	added := perform(t, fixture.handler, http.MethodPost, "/catalogs/sources/"+fixture.connection.ID+"/add", url.Values{
		"csrf_token":       {fixture.csrf},
		"entry_identifier": {"add-entry"},
	}, fixture.cookies)

	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	assert.Contains(t, added.Header().Get("Location"), url.QueryEscape("Downloading and analyzing the text."))
	assert.Equal(t, 1, fixture.acquisitionJobCount(t))
	assert.Zero(t, fixture.requests.Load(), "Add must not contact the OPDS server")
	assert.Zero(t, fixture.analysis.calls, "Add must not submit analysis before the EPUB exists")

	page := perform(t, fixture.handler, http.MethodGet, "/library?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	assert.Contains(t, page.Body.String(), epubAcquisitionInProgressLabel+".", "My Books shows the queued acquisition")

	_, err := fixture.pool.Exec(context.Background(), `UPDATE river_job SET state='running' WHERE kind=$1`, analysis.EPUBAcquisitionQueue)
	require.NoError(t, err)
	running := perform(t, fixture.handler, http.MethodGet, "/library?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, running.Code, running.Body.String())
	assert.Contains(t, running.Body.String(), epubAcquisitionInProgressLabel+".", "My Books shows the running acquisition")

	_, err = fixture.pool.Exec(context.Background(), `UPDATE river_job SET state='discarded', finalized_at=now() WHERE kind=$1`, analysis.EPUBAcquisitionQueue)
	require.NoError(t, err)
	discarded := perform(t, fixture.handler, http.MethodGet, "/library?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, discarded.Code, discarded.Body.String())
	assert.NotContains(t, discarded.Body.String(), epubAcquisitionInProgressLabel+".", "a settled job is not live work")
}

func TestEPUBAcquisitionMoveAndRetryEnqueueOneLiveJob(t *testing.T) {
	fixture := newAcquisitionHandlerFixture(t)
	book := fixture.createToReadBook(t, "Move and retry")
	require.NoError(t, fixture.store.MarkBookPreviouslyRead(context.Background(), fixture.owner.ID, book.ID))
	state, err := fixture.store.GetBookDetail(context.Background(), fixture.owner.ID, book.ID)
	require.NoError(t, err)

	moved := perform(t, fixture.handler, http.MethodPost, "/library/books/"+book.ID+"/to-read", url.Values{
		"csrf_token":        {fixture.csrf},
		"expected_revision": {strconv.FormatInt(state.DispositionRevision, 10)},
	}, fixture.cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())
	assert.Contains(t, moved.Header().Get("Location"), url.QueryEscape("Downloading and analyzing the text."))
	assert.Equal(t, 1, fixture.acquisitionJobCount(t))

	retried := perform(t, fixture.handler, http.MethodPost, "/reading/books/"+book.ID+"/reanalyze", url.Values{
		"csrf_token": {fixture.csrf},
	}, fixture.cookies)
	require.Equal(t, http.StatusSeeOther, retried.Code, retried.Body.String())
	assert.Contains(t, retried.Header().Get("Location"), url.QueryEscape("Downloading and analyzing the text."))
	assert.Equal(t, 1, fixture.acquisitionJobCount(t), "a second Retry while the job is queued enqueues nothing")

	_, err = fixture.pool.Exec(context.Background(), `UPDATE river_job SET state='running' WHERE kind=$1`, analysis.EPUBAcquisitionQueue)
	require.NoError(t, err)
	retriedAgain := perform(t, fixture.handler, http.MethodPost, "/reading/books/"+book.ID+"/reanalyze", url.Values{
		"csrf_token": {fixture.csrf},
	}, fixture.cookies)
	require.Equal(t, http.StatusSeeOther, retriedAgain.Code, retriedAgain.Body.String())
	assert.Equal(t, 1, fixture.acquisitionJobCount(t), "a second Retry while the job is running enqueues nothing")
	assert.Zero(t, fixture.requests.Load(), "the actions must not contact the OPDS server")
	assert.Zero(t, fixture.analysis.calls)
}

func TestEPUBBookThatAlreadyHasItsEPUBRetriesAnalysisWithoutAcquisition(t *testing.T) {
	fixture := newAcquisitionHandlerFixture(t)
	book := fixture.createToReadBook(t, "Already acquired")
	imported, err := epub.NewService(fixture.store).ImportForAcquisitionForBook(context.Background(), fixture.owner.ID, "de", book.ID, epub.AcquisitionContent{
		Content: testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "epub3-main-text")),
	})
	require.NoError(t, err)

	retried := perform(t, fixture.handler, http.MethodPost, "/reading/books/"+book.ID+"/reanalyze", url.Values{
		"csrf_token": {fixture.csrf},
	}, fixture.cookies)

	require.Equal(t, http.StatusSeeOther, retried.Code, retried.Body.String())
	assert.Contains(t, retried.Header().Get("Location"), url.QueryEscape("Analysis job #1 submitted."))
	assert.Zero(t, fixture.acquisitionJobCount(t), "an imported but unanalyzed Book needs no second download")
	assert.Equal(t, 1, fixture.analysis.calls, "Retry submits analysis directly")
	assert.Equal(t, imported.Source.ID, fixture.analysis.source)
	assert.Zero(t, fixture.requests.Load())
}

func TestEPUBAcquisitionFailurePresentationOffersRetryOnlyWhenItCanHelp(t *testing.T) {
	tests := []struct {
		name        string
		stage       domain.BookPreparationStage
		code        domain.BookPreparationCode
		label       string
		wantRetry   bool
		withCatalog bool
	}{
		{name: "entry", stage: domain.BookPreparationStageEntry, code: domain.BookPreparationCodeCatalogEntryMissing, label: epubAcquisitionEntryMissingLabel, wantRetry: true},
		{name: "fetch", stage: domain.BookPreparationStageFetch, code: domain.BookPreparationCodeSourceDownloadFailed, label: epubAcquisitionDownloadFailedLabel, wantRetry: true, withCatalog: true},
		{name: "oversize", stage: domain.BookPreparationStageSource, code: domain.BookPreparationCodeSourceOversize, label: epubAcquisitionOversizeLabel, withCatalog: true},
		{name: "format", stage: domain.BookPreparationStageSource, code: domain.BookPreparationCodeSourceFormatUnknown, label: epubAcquisitionFormatLabel, withCatalog: true},
		{name: "import", stage: domain.BookPreparationStageImport, code: domain.BookPreparationCodeEPUBImportFailed, label: epubAcquisitionImportFailedLabel, wantRetry: true, withCatalog: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newAcquisitionHandlerFixture(t)
			book := fixture.createToReadBook(t, "Failure "+tt.name)
			record := domain.EPUBAcquisitionFailureRecord{
				OwnerID: fixture.owner.ID, BookID: book.ID, Stage: tt.stage, Code: tt.code,
			}
			if tt.withCatalog {
				record.ConnectionID, record.EntryIdentifier = fixture.connection.ID, "failure-entry"
			}
			require.NoError(t, fixture.store.RecordEPUBAcquisitionFailure(context.Background(), record))

			page := perform(t, fixture.handler, http.MethodGet, "/library?disposition=to_read", nil, fixture.cookies)
			require.Equal(t, http.StatusOK, page.Code, page.Body.String())
			body := page.Body.String()
			assert.Contains(t, body, tt.label)
			retryAction := `action="/reading/books/` + book.ID + `/reanalyze"`
			if tt.wantRetry {
				assert.Contains(t, body, retryAction)
			} else {
				assert.NotContains(t, body, retryAction)
			}
		})
	}
}
