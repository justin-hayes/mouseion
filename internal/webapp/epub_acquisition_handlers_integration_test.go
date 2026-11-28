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
	"github.com/justin-hayes/mouseion/internal/fixtures"
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
		CatalogueSync:   fixtures.NewCatalogueSync(fixtures.NewStore()),
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

	page := perform(t, fixture.handler, http.MethodGet, "/reading?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	assert.Contains(t, page.Body.String(), epubAcquisitionInProgressLabel+".", "Reading shows the queued acquisition")

	_, err := fixture.pool.Exec(context.Background(), `UPDATE river_job SET state='running' WHERE kind=$1`, analysis.EPUBAcquisitionQueue)
	require.NoError(t, err)
	running := perform(t, fixture.handler, http.MethodGet, "/reading?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, running.Code, running.Body.String())
	assert.Contains(t, running.Body.String(), epubAcquisitionInProgressLabel+".", "Reading shows the running acquisition")

	_, err = fixture.pool.Exec(context.Background(), `UPDATE river_job SET state='discarded', finalized_at=now() WHERE kind=$1`, analysis.EPUBAcquisitionQueue)
	require.NoError(t, err)
	discarded := perform(t, fixture.handler, http.MethodGet, "/reading?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, discarded.Code, discarded.Body.String())
	assert.NotContains(t, discarded.Body.String(), epubAcquisitionInProgressLabel+".", "a settled job is not live work")
}

// TestEPUBAcquisitionLifecycleOnMyBooks walks one EPUB Book through the four
// states its newest acquisition job gives Reading. While the job is queued or
// running the Book carries the same in-progress label a classical Book shows
// and offers no Retry; once the job has failed the recorded Book preparation
// failure takes over with its Retry; once a later acquisition succeeds the
// preparation leaves no evidence and the Book shows its analysis state as
// today.
func TestEPUBAcquisitionLifecycleOnMyBooks(t *testing.T) {
	fixture := newAcquisitionHandlerFixture(t)
	ctx := context.Background()
	require.NoError(t, fixture.store.ReconcileCatalogueIndex(ctx, fixture.owner.ID, fixture.connection.ID, []domain.CatalogueIndexEntry{{
		ConnectionID: fixture.connection.ID, EntryIdentifier: "lifecycle-entry", Title: "Lifecycle Entry",
		Author: "Author", LanguageTag: "de", EPUBHref: "https://catalog.invalid/lifecycle-entry.epub",
	}}, time.Now()))

	added := perform(t, fixture.handler, http.MethodPost, "/catalogs/sources/"+fixture.connection.ID+"/add", url.Values{
		"csrf_token":       {fixture.csrf},
		"entry_identifier": {"lifecycle-entry"},
	}, fixture.cookies)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	entries, err := fixture.store.BrowseCatalogueEntries(ctx, fixture.owner.ID, fixture.connection.ID, domain.CatalogueQuery{})
	require.NoError(t, err)
	require.NotEmpty(t, entries.Entries)
	bookID := entries.Entries[0].BookID
	require.NotEmpty(t, bookID, "the added entry has no Book")
	retryAction := `action="/reading/books/` + bookID + `/reanalyze"`

	myBooksBody := func() string {
		page := perform(t, fixture.handler, http.MethodGet, "/reading?disposition=to_read", nil, fixture.cookies)
		require.Equal(t, http.StatusOK, page.Code, page.Body.String())
		return page.Body.String()
	}

	// Queued: the one in-progress label, and no Retry.
	queued := myBooksBody()
	assert.Contains(t, queued, epubAcquisitionInProgressLabel+".")
	assert.NotContains(t, queued, retryAction, "no Retry while the acquisition is queued")

	// Running: the same in-progress label, and still no Retry.
	_, err = fixture.pool.Exec(ctx, `UPDATE river_job SET state='running' WHERE kind=$1`, analysis.EPUBAcquisitionQueue)
	require.NoError(t, err)
	running := myBooksBody()
	assert.Contains(t, running, epubAcquisitionInProgressLabel+".")
	assert.NotContains(t, running, retryAction, "no Retry while the acquisition is running")

	// The job fails and records its Book preparation failure: the failure
	// reason takes over, with Retry.
	_, err = fixture.pool.Exec(ctx, `UPDATE river_job SET state='discarded', finalized_at=now() WHERE kind=$1`, analysis.EPUBAcquisitionQueue)
	require.NoError(t, err)
	require.NoError(t, fixture.store.RecordEPUBAcquisitionFailure(ctx, domain.EPUBAcquisitionFailureRecord{
		OwnerID: fixture.owner.ID, BookID: bookID,
		ConnectionID: fixture.connection.ID, EntryIdentifier: "lifecycle-entry",
		Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed,
	}))
	failed := myBooksBody()
	assert.NotContains(t, failed, epubAcquisitionInProgressLabel+".", "the in-progress label leaves with the live job")
	assert.Contains(t, failed, epubAcquisitionDownloadFailedLabel)
	assert.Contains(t, failed, retryAction)

	// The learner retries, and the later acquisition succeeds: the failure is
	// cleared, the newest job completes, and the Book shows its analysis state
	// as today, with no acquisition evidence left.
	retried := perform(t, fixture.handler, http.MethodPost, "/reading/books/"+bookID+"/reanalyze", url.Values{
		"csrf_token": {fixture.csrf},
	}, fixture.cookies)
	require.Equal(t, http.StatusSeeOther, retried.Code, retried.Body.String())
	assert.Contains(t, retried.Header().Get("Location"), url.QueryEscape(epubPreparationConfirmation))
	assert.Equal(t, 2, fixture.acquisitionJobCount(t), "the learner's Retry starts a new job")

	imported, err := epub.NewService(fixture.store).ImportForAcquisitionForBook(ctx, fixture.owner.ID, "de", bookID, epub.AcquisitionContent{
		Content: testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "epub3-main-text")),
	})
	require.NoError(t, err)
	seedNewerAnalysisRun(t, ctx, fixture.store, fixture.owner.ID, imported.Source)
	require.NoError(t, fixture.store.ClearEPUBAcquisitionFailure(ctx, fixture.owner.ID, bookID))
	_, err = fixture.pool.Exec(ctx, `UPDATE river_job SET state='completed', finalized_at=now() WHERE kind=$1 AND state='available'`, analysis.EPUBAcquisitionQueue)
	require.NoError(t, err)

	completed := myBooksBody()
	assert.NotContains(t, completed, epubAcquisitionInProgressLabel+".", "a completed acquisition is no longer in progress")
	assert.NotContains(t, completed, epubAcquisitionDownloadFailedLabel, "the later success clears the recorded failure")
	assert.Contains(t, completed, "Analysis running.", "the Book shows its analysis state as today")
	assert.NotContains(t, completed, retryAction, "an analysis under way offers no Retry")
}

// TestEPUBAcquisitionEntryFailureWithoutCatalogIdentityShowsEverywhere covers
// the failure shape acquisition records when it cannot resolve an entry at all.
// Reading explains it with Retry, and /jobs lists the same failed preparation.
func TestEPUBAcquisitionEntryFailureWithoutCatalogIdentityShowsEverywhere(t *testing.T) {
	fixture := newAcquisitionHandlerFixture(t)
	book := fixture.createToReadBook(t, "No catalog identity")
	require.NoError(t, fixture.store.RecordEPUBAcquisitionFailure(context.Background(), domain.EPUBAcquisitionFailureRecord{
		OwnerID: fixture.owner.ID, BookID: book.ID,
		Stage: domain.BookPreparationStageEntry, Code: domain.BookPreparationCodeCatalogEntryMissing,
	}))
	retryAction := `action="/reading/books/` + book.ID + `/reanalyze"`

	myBooks := perform(t, fixture.handler, http.MethodGet, "/reading?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, myBooks.Code, myBooks.Body.String())
	assert.Contains(t, myBooks.Body.String(), epubAcquisitionEntryMissingLabel)
	assert.Contains(t, myBooks.Body.String(), retryAction)

	jobs := perform(t, fixture.handler, http.MethodGet, "/jobs", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, jobs.Code, jobs.Body.String())
	assert.Contains(t, jobs.Body.String(), "No catalog identity")
	assert.Contains(t, jobs.Body.String(), epubAcquisitionEntryMissingLabel)
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

			page := perform(t, fixture.handler, http.MethodGet, "/reading?disposition=to_read", nil, fixture.cookies)
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

func TestMyBooksNeedsActionFilterAgainstPostgres(t *testing.T) {
	fixture := newAcquisitionHandlerFixture(t)
	ctx := context.Background()
	failing := fixture.createToReadBook(t, "Failing Book")
	healthy := fixture.createToReadBook(t, "Quiet Book")
	require.NoError(t, fixture.store.RecordEPUBAcquisitionFailure(ctx, domain.EPUBAcquisitionFailureRecord{
		OwnerID: fixture.owner.ID, BookID: failing.ID,
		ConnectionID: fixture.connection.ID, EntryIdentifier: "needs-action-entry",
		Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed,
	}))
	require.NoError(t, fixture.store.SetActiveStudyLanguage(ctx, fixture.owner.ID, "de"))

	all := perform(t, fixture.handler, http.MethodGet, "/de/reading?disposition=to_read", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, all.Code, all.Body.String())
	assert.Contains(t, all.Body.String(), "Acquisition failed.")
	assert.Contains(t, all.Body.String(), `id="book-row-`+healthy.ID+`"`)

	filtered := perform(t, fixture.handler, http.MethodGet, "/de/reading?needs=action", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, filtered.Code, filtered.Body.String())
	body := filtered.Body.String()
	assert.Contains(t, body, `id="book-row-`+failing.ID+`"`)
	assert.NotContains(t, body, `id="book-row-`+healthy.ID+`"`)

	searched := perform(t, fixture.handler, http.MethodGet, "/de/reading?needs=action&q=quiet", nil, fixture.cookies)
	require.Equal(t, http.StatusOK, searched.Code)
	assert.Contains(t, searched.Body.String(), "need action.")
	assert.NotContains(t, searched.Body.String(), "match “quiet”")
}
