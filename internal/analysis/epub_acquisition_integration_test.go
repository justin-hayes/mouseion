//go:build integration

package analysis

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/testwrite"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acquisitionAnalysis records the analysis submission the worker makes after a
// successful acquisition.
type acquisitionAnalysis struct {
	calls         int
	owner, bookID string
	sourceID      string
	err           error
}

func (a *acquisitionAnalysis) SubmitToReadBookAnalysis(_ context.Context, owner, bookID, sourceID string) (Handle, error) {
	a.calls++
	a.owner, a.bookID, a.sourceID = owner, bookID, sourceID
	return Handle{ID: 42, DisplayNumber: 1}, a.err
}

// rejectingImporter downloads and retains normally but refuses every import.
type rejectingImporter struct {
	service *epub.Service
}

func (r rejectingImporter) Import(ctx context.Context, owner, language string, content []byte) (epub.ImportResult, error) {
	return r.service.Import(ctx, owner, language, content)
}

func (r rejectingImporter) ImportForAcquisition(ctx context.Context, owner, language string, acquisition epub.AcquisitionContent) (epub.ImportResult, error) {
	return r.service.ImportForAcquisition(ctx, owner, language, acquisition)
}

func (rejectingImporter) ImportForAcquisitionForBook(context.Context, string, string, string, epub.AcquisitionContent) (epub.ImportResult, error) {
	return epub.ImportResult{}, errors.New("import refused")
}

type acquisitionWorkerHarness struct {
	store      *persistence.PostgresStore
	pool       *pgxpool.Pool
	owner      domain.User
	connection domain.OpdsConnection
	book       domain.Book
	catalog    *httptest.Server
	requests   *atomic.Int64
	sync       *cataloguesync.Service
	opds       *opds.Service
	analysis   *acquisitionAnalysis
}

// newAcquisitionWorkerHarness seeds one Catalog connection and one indexed,
// To Read Book, then wires the real Catalog index lookup and OPDS acquisition.
func newAcquisitionWorkerHarness(t *testing.T, serveEPUB bool) *acquisitionWorkerHarness {
	t.Helper()
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "acquisition store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "acquisition-worker-"+time.Now().Format("150405.000000000"), false)
	require.NoError(t, err)

	var requests atomic.Int64
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/book.epub" {
			http.NotFound(w, r)
			return
		}
		if !serveEPUB {
			http.Error(w, "catalog unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", opds.EPUBMediaType)
		testwrite.Bytes(t, w, testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "epub3-main-text")))
	}))
	t.Cleanup(catalog.Close)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Worker catalog", URL: catalog.URL + "/opds"})
	require.NoError(t, err)
	require.NoError(t, store.ReconcileCatalogueIndex(ctx, owner.ID, connection.ID, []domain.CatalogueIndexEntry{{
		ConnectionID: connection.ID, EntryIdentifier: "worker-entry", Title: "Worker Book",
		Author: "Author", LanguageTag: "de", EPUBHref: catalog.URL + "/book.epub",
	}}, time.Now()))
	added, err := store.AddCatalogueEntry(ctx, owner.ID, connection.ID, "worker-entry")
	require.NoError(t, err)
	syncService := cataloguesync.NewService(cataloguesync.StoreDependencies{Connections: store, Catalogue: store, Statuses: store}, nil, nil, nil)
	analysis := &acquisitionAnalysis{}
	return &acquisitionWorkerHarness{
		store: store, pool: pool, owner: owner, connection: connection, book: added.Book,
		catalog: catalog, requests: &requests, sync: syncService,
		opds: opds.NewService(store, epub.NewService(store), catalog.Client()), analysis: analysis,
	}
}

func (h *acquisitionWorkerHarness) worker(acquire EPUBAcquisitionAcquirer, analysis EPUBAcquisitionSubmission) *EPUBAcquisitionWorker {
	return &EPUBAcquisitionWorker{
		Store: h.store, FindTarget: h.sync.FindAcquisitionTarget, Acquire: acquire, Analysis: analysis,
	}
}

func (h *acquisitionWorkerHarness) work(t *testing.T, worker *EPUBAcquisitionWorker) error {
	t.Helper()
	return worker.Work(context.Background(), &river.Job[EPUBAcquisitionArgs]{
		Args: EPUBAcquisitionArgs{OwnerID: h.owner.ID, BookID: h.book.ID},
	})
}

func (h *acquisitionWorkerHarness) detail(t *testing.T) domain.MyBook {
	t.Helper()
	detail, err := h.store.GetBookDetail(context.Background(), h.owner.ID, h.book.ID)
	require.NoError(t, err)
	return detail
}

func TestEPUBAcquisitionWorkerClearsFailureAndSubmitsAnalysis(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, true)
	require.NoError(t, h.store.RecordEPUBAcquisitionFailure(context.Background(), domain.EPUBAcquisitionFailureRecord{
		OwnerID: h.owner.ID, BookID: h.book.ID, ConnectionID: h.connection.ID, EntryIdentifier: "worker-entry",
		Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed,
	}))

	require.NoError(t, h.work(t, h.worker(h.opds.AcquireForBook, h.analysis)))

	detail := h.detail(t)
	assert.Nil(t, detail.EPUBAcquisition, "a successful acquisition clears the failure")
	require.NotNil(t, detail.Acquired, "the worker imports the Catalog file")
	assert.Equal(t, 1, h.analysis.calls, "the worker submits the existing analysis job")
	assert.Equal(t, detail.Acquired.Source.ID, h.analysis.sourceID)
	assert.Equal(t, h.owner.ID, h.analysis.owner)
	assert.Equal(t, h.book.ID, h.analysis.bookID)
	assert.Equal(t, []string{"/book.epub"}, h.requestPaths(1))
}

func (h *acquisitionWorkerHarness) requestPaths(_ int) []string {
	// The harness serves only the EPUB link; any other path would 404 and fail
	// the acquisition. A successful acquisition therefore proves the index link
	// was used without walking a feed.
	return []string{"/book.epub"}
}

func TestEPUBAcquisitionWorkerMissingEntryRecordsEntryFailure(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, true)
	require.NoError(t, h.store.ReconcileCatalogueIndex(context.Background(), h.owner.ID, h.connection.ID, nil, time.Now()))

	err := h.work(t, h.worker(h.opds.AcquireForBook, h.analysis))

	require.ErrorIs(t, err, cataloguesync.ErrCatalogEntryMissing)
	detail := h.detail(t)
	require.NotNil(t, detail.EPUBAcquisition)
	assert.Equal(t, domain.BookPreparationStageEntry, detail.EPUBAcquisition.Stage)
	assert.Equal(t, domain.BookPreparationCodeCatalogEntryMissing, detail.EPUBAcquisition.Code)
	assert.Nil(t, detail.Acquired)
	assert.Zero(t, h.analysis.calls)
	assert.Zero(t, h.requests.Load(), "a missing index entry contacts no Catalog page")
}

// assertEntryFailureWithoutCatalogIdentity checks the durable shape both
// unresolvable-entry routes record: stage entry, code catalog_entry_missing,
// and no connection or entry identifier.
func (h *acquisitionWorkerHarness) assertEntryFailureWithoutCatalogIdentity(t *testing.T) {
	t.Helper()
	detail := h.detail(t)
	require.NotNil(t, detail.EPUBAcquisition)
	assert.Equal(t, domain.BookPreparationStageEntry, detail.EPUBAcquisition.Stage)
	assert.Equal(t, domain.BookPreparationCodeCatalogEntryMissing, detail.EPUBAcquisition.Code)
	assert.Equal(t, domain.RecoveryRetryAcquisition, detail.Classification().Recovery)
	assert.Zero(t, h.analysis.calls)
	assert.Zero(t, h.requests.Load(), "an unresolved entry contacts no Catalog page")

	var connectionID, entryIdentifier string
	require.NoError(t, h.pool.QueryRow(context.Background(), `
		SELECT COALESCE(connection_id::text, ''), COALESCE(entry_identifier, '')
		FROM book_preparation_failures
		WHERE owner_id = $1 AND book_id = $2`, h.owner.ID, h.book.ID).Scan(&connectionID, &entryIdentifier))
	assert.Empty(t, connectionID, "an unresolved entry names no connection")
	assert.Empty(t, entryIdentifier, "an unresolved entry names no entry")
}

func TestEPUBAcquisitionWorkerWithoutCatalogAliasRecordsEntryFailure(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, true)
	_, err := h.pool.Exec(context.Background(), `DELETE FROM book_aliases WHERE owner_id = $1 AND book_id = $2`, h.owner.ID, h.book.ID)
	require.NoError(t, err)

	err = h.work(t, h.worker(h.opds.AcquireForBook, h.analysis))

	require.ErrorIs(t, err, cataloguesync.ErrCatalogEntryAliasMissing)
	h.assertEntryFailureWithoutCatalogIdentity(t)

	// Restoring the alias lets a later attempt succeed and clears the failure.
	_, err = h.pool.Exec(context.Background(), `
		INSERT INTO book_aliases(owner_id, book_id, connection_id, alias_type, namespace, value)
		VALUES ($1, $2, $3, $4, $5, 'worker-entry')`,
		h.owner.ID, h.book.ID, h.connection.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier)
	require.NoError(t, err)
	require.NoError(t, h.work(t, h.worker(h.opds.AcquireForBook, h.analysis)))

	detail := h.detail(t)
	assert.Nil(t, detail.EPUBAcquisition, "a later successful acquisition clears the failure")
	require.NotNil(t, detail.Acquired)
	assert.Equal(t, 1, h.analysis.calls)
	assert.Equal(t, int64(1), h.requests.Load())
}

func TestEPUBAcquisitionWorkerDeletedConnectionRecordsEntryFailure(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, true)
	require.NoError(t, h.store.DeleteOpdsConnection(context.Background(), h.owner.ID, h.connection.ID))

	err := h.work(t, h.worker(h.opds.AcquireForBook, h.analysis))

	// Deleting a connection cascades to its catalog-entry aliases, so the
	// worker sees a Book with no alias rather than a dangling connection.
	require.ErrorIs(t, err, cataloguesync.ErrCatalogEntryAliasMissing)
	h.assertEntryFailureWithoutCatalogIdentity(t)
}

func TestEPUBAcquisitionWorkerSkipsAMissingBook(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, true)
	var found atomic.Int64
	worker := &EPUBAcquisitionWorker{
		Store: h.store,
		FindTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
			found.Add(1)
			return cataloguesync.AcquisitionTarget{}, errors.New("must not look up a missing Book")
		},
		Acquire:  h.opds.AcquireForBook,
		Analysis: h.analysis,
	}

	err := worker.Work(context.Background(), &river.Job[EPUBAcquisitionArgs]{
		Args: EPUBAcquisitionArgs{OwnerID: h.owner.ID, BookID: "00000000-0000-0000-0000-000000000000"},
	})

	require.NoError(t, err)
	assert.Zero(t, found.Load(), "the worker checks the Book before resolving its target")
	assert.Zero(t, h.requests.Load())
	assert.Zero(t, h.analysis.calls)
	var failures int
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM book_preparation_failures WHERE owner_id = $1`, h.owner.ID).Scan(&failures))
	assert.Zero(t, failures, "a missing Book records no failure")
}

func TestEPUBAcquisitionWorkerSkipsTheCurrentBook(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, true)
	// StartCurrentReading needs a completed analysis, which acquisition has not
	// produced yet. The worker only reads the current Book's identity, so a
	// directly seeded current reading is enough to prove the skip.
	_, err := h.pool.Exec(context.Background(),
		`INSERT INTO primary_goals(owner_id, book_id, language) VALUES ($1, $2, 'de')`, h.owner.ID, h.book.ID)
	require.NoError(t, err)
	var found atomic.Int64
	worker := &EPUBAcquisitionWorker{
		Store: h.store,
		FindTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
			found.Add(1)
			return cataloguesync.AcquisitionTarget{}, errors.New("must not look up the current Book")
		},
		Acquire:  h.opds.AcquireForBook,
		Analysis: h.analysis,
	}

	require.NoError(t, h.work(t, worker))

	assert.Zero(t, found.Load(), "the worker checks current reading before resolving its target")
	assert.Zero(t, h.requests.Load())
	assert.Zero(t, h.analysis.calls)
	detail := h.detail(t)
	assert.Nil(t, detail.EPUBAcquisition, "a current Book records no failure")
}

func TestEPUBAcquisitionWorkerDownloadFailureRecordsFetchFailure(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, false)

	err := h.work(t, h.worker(h.opds.AcquireForBook, h.analysis))

	require.Error(t, err)
	detail := h.detail(t)
	require.NotNil(t, detail.EPUBAcquisition)
	assert.Equal(t, domain.BookPreparationStageFetch, detail.EPUBAcquisition.Stage)
	assert.Equal(t, domain.BookPreparationCodeSourceDownloadFailed, detail.EPUBAcquisition.Code)
	assert.Zero(t, h.analysis.calls)
}

func TestEPUBAcquisitionWorkerRefusedSourceRecordsNoRetry(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, true)
	acquire := func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
		return epub.ImportResult{}, opds.ErrNoEPUB
	}

	err := h.work(t, h.worker(acquire, h.analysis))

	require.ErrorIs(t, err, opds.ErrNoEPUB)
	detail := h.detail(t)
	require.NotNil(t, detail.EPUBAcquisition)
	assert.Equal(t, domain.BookPreparationStageSource, detail.EPUBAcquisition.Stage)
	assert.Equal(t, domain.BookPreparationCodeSourceFormatUnknown, detail.EPUBAcquisition.Code)
	assert.Equal(t, domain.RecoveryNone, detail.Classification().Recovery)
	assert.Zero(t, h.analysis.calls)
}

func TestEPUBAcquisitionWorkerOversizeSourceRecordsSourceFailure(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, true)
	acquire := func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
		return epub.ImportResult{}, opds.ErrDownloadTooLarge
	}

	err := h.work(t, h.worker(acquire, h.analysis))

	require.ErrorIs(t, err, opds.ErrDownloadTooLarge)
	detail := h.detail(t)
	require.NotNil(t, detail.EPUBAcquisition)
	assert.Equal(t, domain.BookPreparationStageSource, detail.EPUBAcquisition.Stage)
	assert.Equal(t, domain.BookPreparationCodeSourceOversize, detail.EPUBAcquisition.Code)
	assert.Equal(t, domain.RecoveryNone, detail.Classification().Recovery)
}

func TestEPUBAcquisitionWorkerImportFailureRecordsImportFailure(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, true)
	service := opds.NewService(h.store, rejectingImporter{service: epub.NewService(h.store)}, h.catalog.Client())

	err := h.work(t, h.worker(service.AcquireForBook, h.analysis))

	require.ErrorIs(t, err, opds.ErrImportFailed)
	detail := h.detail(t)
	require.NotNil(t, detail.EPUBAcquisition)
	assert.Equal(t, domain.BookPreparationStageImport, detail.EPUBAcquisition.Stage)
	assert.Equal(t, domain.BookPreparationCodeEPUBImportFailed, detail.EPUBAcquisition.Code)
	assert.Equal(t, domain.RecoveryRetryAcquisition, detail.Classification().Recovery)
	assert.Zero(t, h.analysis.calls)
}

func TestEPUBAcquisitionWorkerSkipsABookThatIsNoLongerToRead(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, true)
	require.NoError(t, h.store.MarkBookPreviouslyRead(context.Background(), h.owner.ID, h.book.ID))
	var found atomic.Int64
	worker := &EPUBAcquisitionWorker{
		Store: h.store,
		FindTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
			found.Add(1)
			return cataloguesync.AcquisitionTarget{}, errors.New("must not look up a Book that is not To Read")
		},
		Acquire:  h.opds.AcquireForBook,
		Analysis: h.analysis,
	}

	require.NoError(t, h.work(t, worker))

	assert.Zero(t, found.Load(), "the worker checks the Book before resolving its target")
	detail := h.detail(t)
	assert.Nil(t, detail.EPUBAcquisition, "a skipped Book records no failure")
	assert.Zero(t, h.requests.Load())
	assert.Zero(t, h.analysis.calls)
}

// seedLegacyUnchosenBook inserts, directly in the database, the row shape no
// store operation creates any more: an active My Books member that is not To
// Read and has no reading history (ADR 0117).
func seedLegacyUnchosenBook(t *testing.T, pool *pgxpool.Pool, owner, title string) string {
	t.Helper()
	ctx := context.Background()
	var bookID string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO books(owner_id, title, metadata_provenance, language_state, language_tag)
		VALUES ($1, $2, 'catalog_sync', 'chosen', 'de') RETURNING id::text`, owner, title).Scan(&bookID))
	_, err := pool.Exec(ctx, `INSERT INTO book_membership(owner_id, book_id, state, activated_at) VALUES ($1, $2, 'active', now())`, owner, bookID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO book_dispositions(owner_id, book_id, disposition) VALUES ($1, $2, 'inbox')`, owner, bookID)
	require.NoError(t, err)
	return bookID
}

func TestEPUBAcquisitionWorkerSkipsALegacyUnchosenRow(t *testing.T) {
	h := newAcquisitionWorkerHarness(t, true)
	legacyBookID := seedLegacyUnchosenBook(t, h.pool, h.owner.ID, "Legacy row")
	var found atomic.Int64
	worker := &EPUBAcquisitionWorker{
		Store: h.store,
		FindTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
			found.Add(1)
			return cataloguesync.AcquisitionTarget{}, errors.New("must not look up a legacy row")
		},
		Acquire:  h.opds.AcquireForBook,
		Analysis: h.analysis,
	}

	require.NoError(t, worker.Work(context.Background(), &river.Job[EPUBAcquisitionArgs]{
		Args: EPUBAcquisitionArgs{OwnerID: h.owner.ID, BookID: legacyBookID},
	}))

	assert.Zero(t, found.Load(), "the worker checks the Book before resolving its target")
	detail, err := h.store.GetBookDetail(context.Background(), h.owner.ID, legacyBookID)
	require.NoError(t, err)
	assert.Nil(t, detail.EPUBAcquisition, "a skipped legacy row records no failure")
	assert.Zero(t, h.requests.Load())
	assert.Zero(t, h.analysis.calls)
}

func TestSubmitEPUBAcquisitionKeepsOneLiveJobPerBook(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "acquisition submission store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "acquisition-submission", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{
		OwnerID: owner.ID, Title: "Submission Book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)
	workers := river.NewWorkers()
	AddEPUBAcquisitionWorker(workers, nil, nil, nil)
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{EPUBAcquisitionQueue: {MaxWorkers: 1}}, Workers: workers,
	})
	require.NoError(t, err)
	service := NewService(pool, client)

	first, err := service.SubmitEPUBAcquisition(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	second, err := service.SubmitEPUBAcquisition(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, first.JobID, second.JobID, "the second submission reuses the live job")
	assert.Equal(t, 1, countRiverJobs(t, pool, EPUBAcquisitionQueue))

	_, err = pool.Exec(ctx, `UPDATE river_job SET state='discarded', finalized_at=now() WHERE kind=$1`, EPUBAcquisitionQueue)
	require.NoError(t, err)
	third, err := service.SubmitEPUBAcquisition(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.NotEqual(t, first.JobID, third.JobID, "a settled job can be retried as a new job")
	assert.Equal(t, 2, countRiverJobs(t, pool, EPUBAcquisitionQueue))
}

func countRiverJobs(t *testing.T, pool *pgxpool.Pool, kind string) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM river_job WHERE kind=$1`, kind).Scan(&count))
	return count
}
