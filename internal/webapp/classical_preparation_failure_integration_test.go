//go:build integration

package webapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"html"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
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

const failureUnitKey = "synthetic-failure-1"

// mutableLayer serves the failure tests' source bytes. A test can replace the
// bytes or force an HTTP failure without rebuilding the harness.
type mutableLayer struct {
	mu     sync.Mutex
	data   []byte
	status int
}

func (l *mutableLayer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	l.mu.Lock()
	data, status := append([]byte(nil), l.data...), l.status
	l.mu.Unlock()
	if status != 0 && status != http.StatusOK {
		w.WriteHeader(status)
		return
	}
	if _, err := w.Write(data); err != nil {
		return
	}
}

func (l *mutableLayer) set(data []byte, status int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data, l.status = append([]byte(nil), data...), status
}

type classicalFailureHarness struct {
	t             *testing.T
	ctx           context.Context
	store         *persistence.PostgresStore
	pool          *pgxpool.Pool
	h             *Handler
	service       *analysis.Service
	client        *river.Client[pgx.Tx]
	layer         *mutableLayer
	layerRequests *atomic.Int64
	definition    classical.Definition
	data          []byte
}

type classicalFailureOptions struct {
	data       []byte
	writer     ancientgreek.ImportWriter
	withdrawal bool
}

// newClassicalFailureHarness provisions one synthetic work whose single
// whole-work Reading unit is served by a local layer. It can substitute a
// failing import writer so every failure stage is exercised through the real
// worker and PostgreSQL store.
func newClassicalFailureHarness(t *testing.T, opts classicalFailureOptions) *classicalFailureHarness {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "classical-failure-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	data := opts.data
	if data == nil {
		data, err = os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-book1.conllu"))
		require.NoError(t, err)
	}
	sum := sha256.Sum256(data)
	pin := classical.SourcePin{
		Layer: "synthetic-layer", FileName: "synthetic-failure.conllu",
		Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Format: classical.SourceFormatOGACoNLLU,
	}
	definition := preparationDefinition(failureUnitKey, "Synthetic Failure Work", pin, nil, "1.1.1", "2.1.1", 13)
	definition.WorkKey, definition.EditionKey = "synthetic-failure", "synthetic-failure-edition"
	definition.ConstituentBooks = 1
	definition.Withdrawn = opts.withdrawal
	require.NoError(t, store.ReconcileClassicalManifest(ctx, []classical.Definition{definition}))
	catalog, err := classical.NewCatalog([]classical.Definition{definition})
	require.NoError(t, err)

	var requests atomic.Int64
	layer := &mutableLayer{data: data}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		layer.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	layers := classical.Layers{Layers: []classical.Layer{{
		Key: "synthetic-layer", Commit: "synthetic", BaseURL: server.URL + "/",
		LicenceClass: classical.LicenceClassSourceContent, Restriction: classical.RestrictionShareAlike,
		DeployerRegisterRow: "synthetic failure layer",
	}}}
	config := analysis.ClassicalPreparationConfig{Layers: layers, HTTPClient: server.Client(), TempDir: t.TempDir()}
	writer := opts.writer
	if writer == nil {
		writer = analysis.NewGreekImportWriter(store)
	}
	workers := river.NewWorkers()
	require.NoError(t, river.AddWorkerSafely(workers, &analysis.ClassicalPreparationWorker{Store: store, Writer: writer, Config: config}))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{analysis.ClassicalPreparationQueue: {MaxWorkers: 1}},
		Workers: workers,
	})
	require.NoError(t, err)

	service := analysis.NewService(store.Pool(), client).WithClassicalPreparationStore(store)
	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth:                 authService,
		WebAuth:              webauth.New(authService, false, time.Hour),
		Store:                storeDependencies(store),
		Analysis:             service,
		ClassicalPreparation: service,
		Classical:            catalog,
		CatalogueSync:        fixtures.NewCatalogueSync(fixtures.NewStore()),
		SessionLifetime:      time.Hour,
	})
	return &classicalFailureHarness{
		t: t, ctx: ctx, store: store, pool: pool, h: h, service: service, client: client,
		layer: layer, layerRequests: &requests, definition: definition, data: data,
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) WriteImportedAnalysis(context.Context, ancientgreek.ImportWriteRequest) (ancientgreek.ImportWriteResult, error) {
	return ancientgreek.ImportWriteResult{}, w.err
}

func (h *classicalFailureHarness) learner(name string) (domain.User, []*http.Cookie, string) {
	h.t.Helper()
	learner := createAccount(h.t, h.ctx, h.store, name, "learner-password")
	cookies, csrf := loginCookies(h.t, h.h, learner.Username, "learner-password")
	return learner, cookies, csrf
}

func (h *classicalFailureHarness) addBook(owner string) string {
	h.t.Helper()
	report, err := h.store.AddClassicalReadingUnits(h.ctx, owner, []string{failureUnitKey})
	require.NoError(h.t, err)
	require.Len(h.t, report.Books, 1)
	return report.Books[0].BookID
}

func (h *classicalFailureHarness) moveToRead(owner, bookID string, cookies []*http.Cookie, csrf string) *httptest.ResponseRecorder {
	h.t.Helper()
	detail, err := h.store.GetBookDetail(h.ctx, owner, bookID)
	require.NoError(h.t, err)
	return perform(h.t, h.h, http.MethodPost, "/library/books/"+bookID+"/to-read", url.Values{
		"csrf_token":        {csrf},
		"expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)
}

func (h *classicalFailureHarness) submit(owner, bookID string) (analysis.ClassicalPreparationHandle, error) {
	return h.service.SubmitClassicalPreparation(h.ctx, owner, bookID)
}

func (h *classicalFailureHarness) startClient() {
	h.t.Helper()
	require.NoError(h.t, h.client.Start(h.ctx))
	testutil.StopOnCleanup(h.t, "classical failure client", h.client.Stop)
}

func (h *classicalFailureHarness) waitForFailure(owner, bookID string) domain.ClassicalPreparationFailureRecord {
	h.t.Helper()
	var record domain.ClassicalPreparationFailureRecord
	require.Eventually(h.t, func() bool {
		got, found, err := h.store.GetClassicalPreparationFailure(context.Background(), owner, bookID)
		if err != nil || !found {
			return false
		}
		record = got
		return true
	}, 60*time.Second, 50*time.Millisecond, "preparation did not record a failure")
	return record
}

func (h *classicalFailureHarness) waitForPrepared(owner, bookID string) {
	h.t.Helper()
	require.Eventually(h.t, func() bool {
		book, err := h.store.GetBookDetail(context.Background(), owner, bookID)
		return err == nil && book.Classification().AnalysisInEffect()
	}, 60*time.Second, 50*time.Millisecond, "Book did not publish its preparation")
}

func (h *classicalFailureHarness) detail(owner, bookID string) domain.MyBook {
	h.t.Helper()
	book, err := h.store.GetBookDetail(h.ctx, owner, bookID)
	require.NoError(h.t, err)
	return book
}

func (h *classicalFailureHarness) jobsPage(cookies []*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return perform(h.t, h.h, http.MethodGet, "/jobs", nil, cookies)
}

func containsGreek(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Greek) {
			return true
		}
	}
	return false
}

// TestClassicalPreparationFailureRows covers every learner-facing row of the
// failure table through the real worker and database.
func TestClassicalPreparationFailureRows(t *testing.T) {
	t.Run("queued and running are analysis incomplete", func(t *testing.T) {
		h := newClassicalFailureHarness(t, classicalFailureOptions{})
		learner, cookies, csrf := h.learner("classical-failure-in-progress")
		bookID := h.addBook(learner.ID)
		moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
		require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

		detail := h.detail(learner.ID, bookID)
		require.NotNil(t, detail.ClassicalPreparation, "a queued preparation is visible")
		assert.Equal(t, domain.ClassicalPreparationQueued, detail.ClassicalPreparation.State)
		page := perform(t, h.h, http.MethodGet, "/library", nil, cookies)
		require.Equal(t, http.StatusOK, page.Code)
		assert.Contains(t, page.Body.String(), classicalAnalysisIncompleteLabel)
		assert.NotContains(t, page.Body.String(), "Retry acquisition")
		assert.NotContains(t, page.Body.String(), "Retry analysis")

		// A worker that has claimed the job presents running, still as incomplete.
		_, err := h.pool.Exec(h.ctx, `UPDATE river_job SET state='running' WHERE kind=$1 AND args->>'book_id'=$2`,
			analysis.ClassicalPreparationQueue, bookID)
		require.NoError(t, err)
		detail = h.detail(learner.ID, bookID)
		require.NotNil(t, detail.ClassicalPreparation)
		assert.Equal(t, domain.ClassicalPreparationRunning, detail.ClassicalPreparation.State)

		// Return the simulated claim so a real worker can finish it.
		_, err = h.pool.Exec(h.ctx, `UPDATE river_job SET state='available', attempted_at=NULL, attempt=0 WHERE kind=$1 AND args->>'book_id'=$2`,
			analysis.ClassicalPreparationQueue, bookID)
		require.NoError(t, err)
		h.startClient()
		h.waitForPrepared(learner.ID, bookID)
		detail = h.detail(learner.ID, bookID)
		assert.Nil(t, detail.ClassicalPreparation, "a prepared Book has ordinary Analysis evidence")
	})

	t.Run("fetch failure is assessment unavailable with retry acquisition", func(t *testing.T) {
		h := newClassicalFailureHarness(t, classicalFailureOptions{})
		h.layer.set(nil, http.StatusBadGateway)
		learner, cookies, csrf := h.learner("classical-failure-fetch")
		bookID := h.addBook(learner.ID)
		moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
		require.Equal(t, http.StatusSeeOther, moved.Code)

		h.startClient()
		record := h.waitForFailure(learner.ID, bookID)
		assert.Equal(t, domain.ClassicalPreparationFailureFetch, record.Stage)
		assert.Equal(t, ancientgreek.CodeSourceDownloadFailed, record.Code)
		detail := h.detail(learner.ID, bookID)
		require.NotNil(t, detail.ClassicalPreparation)
		assert.Equal(t, domain.ClassicalPreparationFailureFetch, detail.ClassicalPreparation.Failure)

		page := perform(t, h.h, http.MethodGet, "/library", nil, cookies)
		require.Equal(t, http.StatusOK, page.Code)
		assert.Contains(t, page.Body.String(), classicalAssessmentUnavailable)
		assert.Contains(t, page.Body.String(), classicalRetryAcquisitionLabel)
		assert.Contains(t, h.jobsPage(cookies).Body.String(), ancientgreek.CodeSourceDownloadFailed)

		// Retry re-runs preparation, and the retry can succeed after the layer
		// recovers.
		h.layer.set(h.data, http.StatusOK)
		handle, err := h.submit(learner.ID, bookID)
		require.NoError(t, err)
		assert.True(t, handle.Queued, "a fetch failure is retryable")
		h.waitForPrepared(learner.ID, bookID)
	})

	t.Run("import failure is analysis failed with retry analysis", func(t *testing.T) {
		h := newClassicalFailureHarness(t, classicalFailureOptions{writer: failingWriter{err: errors.New("write failed")}})
		learner, cookies, csrf := h.learner("classical-failure-import")
		bookID := h.addBook(learner.ID)
		moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
		require.Equal(t, http.StatusSeeOther, moved.Code)

		h.startClient()
		record := h.waitForFailure(learner.ID, bookID)
		assert.Equal(t, domain.ClassicalPreparationFailureImport, record.Stage)
		assert.Equal(t, classical.PreparationFailedCode, record.Code)
		detail := h.detail(learner.ID, bookID)
		require.NotNil(t, detail.ClassicalPreparation)
		assert.Equal(t, domain.ClassicalPreparationFailureImport, detail.ClassicalPreparation.Failure)

		page := perform(t, h.h, http.MethodGet, "/library", nil, cookies)
		assert.Contains(t, page.Body.String(), classicalAnalysisFailedLabel)
		assert.Contains(t, page.Body.String(), classicalRetryAnalysisLabel)
		assert.Contains(t, h.jobsPage(cookies).Body.String(), classical.PreparationFailedCode)

		handle, err := h.submit(learner.ID, bookID)
		require.NoError(t, err)
		assert.True(t, handle.Queued, "an import failure is retryable")
	})

	t.Run("pin mismatch is terminal with no retry", func(t *testing.T) {
		h := newClassicalFailureHarness(t, classicalFailureOptions{})
		h.layer.set([]byte("different bytes\n"), http.StatusOK)
		learner, cookies, csrf := h.learner("classical-failure-pin")
		bookID := h.addBook(learner.ID)
		moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
		require.Equal(t, http.StatusSeeOther, moved.Code)

		h.startClient()
		record := h.waitForFailure(learner.ID, bookID)
		assert.Equal(t, domain.ClassicalPreparationFailureSource, record.Stage)
		assert.Equal(t, ancientgreek.CodePinMismatch, record.Code)
		detail := h.detail(learner.ID, bookID)
		assert.Equal(t, domain.BookDispositionToRead, detail.Disposition, "a terminal failure leaves the Book To Read")
		require.NotNil(t, detail.ClassicalPreparation)
		assert.Equal(t, classicalSourceMismatchLabel, classicalPreparationViewFor(*detail.ClassicalPreparation).Label)

		page := perform(t, h.h, http.MethodGet, "/library", nil, cookies)
		assert.Contains(t, page.Body.String(), html.EscapeString(classicalSourceMismatchLabel))
		assert.NotContains(t, page.Body.String(), classicalRetryAcquisitionLabel)
		assert.NotContains(t, page.Body.String(), classicalRetryAnalysisLabel)
		assert.Contains(t, h.jobsPage(cookies).Body.String(), ancientgreek.CodePinMismatch)

		handle, err := h.submit(learner.ID, bookID)
		require.NoError(t, err)
		assert.True(t, handle.Terminal, "a current terminal source failure is not retried")
		assert.False(t, handle.Queued)
		assert.Equal(t, 1, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue))
	})

	t.Run("oversize is terminal with no retry", func(t *testing.T) {
		h := newClassicalFailureHarness(t, classicalFailureOptions{})
		h.layer.set(append(append([]byte(nil), h.data...), 'x'), http.StatusOK)
		learner, cookies, csrf := h.learner("classical-failure-oversize")
		bookID := h.addBook(learner.ID)
		moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
		require.Equal(t, http.StatusSeeOther, moved.Code)

		h.startClient()
		record := h.waitForFailure(learner.ID, bookID)
		assert.Equal(t, domain.ClassicalPreparationFailureSource, record.Stage)
		assert.Equal(t, ancientgreek.CodeOversize, record.Code)
		page := perform(t, h.h, http.MethodGet, "/library", nil, cookies)
		assert.Contains(t, page.Body.String(), html.EscapeString(classicalSourceMismatchLabel))
		assert.NotContains(t, page.Body.String(), classicalRetryAcquisitionLabel)
		assert.Contains(t, h.jobsPage(cookies).Body.String(), ancientgreek.CodeOversize)
	})

	t.Run("adapter refusal is terminal with no retry", func(t *testing.T) {
		malformed := []byte("this is not an oga_src file\n")
		h := newClassicalFailureHarness(t, classicalFailureOptions{data: malformed})
		learner, cookies, csrf := h.learner("classical-failure-adapter")
		bookID := h.addBook(learner.ID)
		moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
		require.Equal(t, http.StatusSeeOther, moved.Code)

		h.startClient()
		record := h.waitForFailure(learner.ID, bookID)
		assert.Equal(t, domain.ClassicalPreparationFailureSource, record.Stage)
		assert.Equal(t, ancientgreek.CodeAdapterRefused, record.Code)
		page := perform(t, h.h, http.MethodGet, "/library", nil, cookies)
		assert.Contains(t, page.Body.String(), html.EscapeString(classicalSourceMismatchLabel))
		assert.NotContains(t, page.Body.String(), classicalRetryAcquisitionLabel)
		assert.NotContains(t, page.Body.String(), classicalRetryAnalysisLabel)
		assert.Contains(t, h.jobsPage(cookies).Body.String(), ancientgreek.CodeAdapterRefused)
	})

	t.Run("withdrawn unprepared Book cannot be prepared", func(t *testing.T) {
		h := newClassicalFailureHarness(t, classicalFailureOptions{})
		learner, cookies, csrf := h.learner("classical-failure-withdrawn")
		bookID := h.addBook(learner.ID)
		withdrawn := h.definition
		withdrawn.Withdrawn = true
		require.NoError(t, h.store.ReconcileClassicalManifest(h.ctx, []classical.Definition{withdrawn}))

		moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
		require.Equal(t, http.StatusSeeOther, moved.Code)
		assert.Contains(t, moved.Header().Get("Location"), url.QueryEscape(classicalWithdrawnLabel))
		assert.Zero(t, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue))

		detail := h.detail(learner.ID, bookID)
		require.NotNil(t, detail.ClassicalPreparation)
		assert.True(t, detail.ClassicalPreparation.Withdrawn)
		page := perform(t, h.h, http.MethodGet, "/library", nil, cookies)
		assert.Contains(t, page.Body.String(), classicalWithdrawnLabel)
		assert.NotContains(t, page.Body.String(), `action="/reading/books/`+bookID+`/reanalyze"`)

		_, err := h.submit(learner.ID, bookID)
		require.ErrorIs(t, err, analysis.ErrWithdrawnClassicalDefinition)
		jobs := h.jobsPage(cookies).Body.String()
		assert.Contains(t, jobs, classical.WithdrawnCode, "a refused withdrawn preparation still appears on the jobs surface")
		assert.Contains(t, jobs, ">Failed</span>", "an orphan failure row is presented as failed, not queued")
	})

	t.Run("withdrawal after enqueue records the withdrawn code", func(t *testing.T) {
		h := newClassicalFailureHarness(t, classicalFailureOptions{})
		learner, cookies, csrf := h.learner("classical-failure-withdrawn-job")
		bookID := h.addBook(learner.ID)
		moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
		require.Equal(t, http.StatusSeeOther, moved.Code)
		withdrawn := h.definition
		withdrawn.Withdrawn = true
		require.NoError(t, h.store.ReconcileClassicalManifest(h.ctx, []classical.Definition{withdrawn}))

		h.startClient()
		record := h.waitForFailure(learner.ID, bookID)
		assert.Equal(t, domain.ClassicalPreparationFailureWithdrawn, record.Stage)
		assert.Equal(t, classical.WithdrawnCode, record.Code)
		page := h.jobsPage(cookies)
		require.Equal(t, http.StatusOK, page.Code, page.Body.String())
		assert.Contains(t, page.Body.String(), classical.WithdrawnCode)
	})
}

// TestClassicalPreparationNewerRevisionClearsTerminalFailure proves a terminal
// source failure clears when a newer definition revision arrives and the Book
// then prepares successfully.
func TestClassicalPreparationNewerRevisionClearsTerminalFailure(t *testing.T) {
	h := newClassicalFailureHarness(t, classicalFailureOptions{})
	h.layer.set([]byte("different bytes\n"), http.StatusOK)
	learner, cookies, csrf := h.learner("classical-failure-revision")
	bookID := h.addBook(learner.ID)
	moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
	require.Equal(t, http.StatusSeeOther, moved.Code)
	h.startClient()
	record := h.waitForFailure(learner.ID, bookID)
	require.Equal(t, ancientgreek.CodePinMismatch, record.Code)
	require.True(t, h.submitTerminal(learner.ID, bookID))

	// A newer revision with the correct pin makes the recorded failure stale.
	h.layer.set(h.data, http.StatusOK)
	pin := h.definition.Source
	revised := h.definition
	revised.Revision = 2
	require.NoError(t, h.store.ReconcileClassicalManifest(h.ctx, []classical.Definition{revised}))
	require.Equal(t, pin.SHA256, revised.Source.SHA256)

	detail := h.detail(learner.ID, bookID)
	require.NotNil(t, detail.ClassicalPreparation, "the cleared failure is still visible as a retryable state")
	assert.True(t, detail.ClassicalPreparation.Cleared)
	assert.Empty(t, detail.ClassicalPreparation.Failure)
	page := perform(t, h.h, http.MethodGet, "/library", nil, cookies)
	assert.Contains(t, page.Body.String(), `action="/reading/books/`+bookID+`/reanalyze"`)
	assert.Contains(t, page.Body.String(), ">Retry preparation</button>")
	assert.NotContains(t, page.Body.String(), html.EscapeString(classicalSourceMismatchLabel))

	handle, err := h.submit(learner.ID, bookID)
	require.NoError(t, err)
	assert.True(t, handle.Queued, "a newer revision enables preparation again")
	h.waitForPrepared(learner.ID, bookID)
	detail = h.detail(learner.ID, bookID)
	assert.Nil(t, detail.ClassicalPreparation)
}

func (h *classicalFailureHarness) submitTerminal(owner, bookID string) bool {
	h.t.Helper()
	handle, err := h.submit(owner, bookID)
	require.NoError(h.t, err)
	return handle.Terminal
}

// TestClassicalPreparationWithdrawnPreparedBookStaysReadable proves withdrawal
// does not take a published analysis away, while still refusing a new
// preparation.
func TestClassicalPreparationWithdrawnPreparedBookStaysReadable(t *testing.T) {
	h := newClassicalFailureHarness(t, classicalFailureOptions{})
	learner, cookies, csrf := h.learner("classical-prepared-withdrawn")
	bookID := h.addBook(learner.ID)
	moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
	require.Equal(t, http.StatusSeeOther, moved.Code)
	h.startClient()
	h.waitForPrepared(learner.ID, bookID)

	withdrawn := h.definition
	withdrawn.Withdrawn = true
	require.NoError(t, h.store.ReconcileClassicalManifest(h.ctx, []classical.Definition{withdrawn}))
	detail := h.detail(learner.ID, bookID)
	assert.Nil(t, detail.ClassicalPreparation, "a prepared Book keeps its published analysis")
	assert.True(t, detail.Classification().AnalysisInEffect())

	require.NoError(t, h.store.SetActiveStudyLanguage(h.ctx, learner.ID, classical.Language))
	started := perform(t, h.h, http.MethodPost, "/reading/books/"+bookID+"/start",
		url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code, started.Body.String())
	reading, err := h.store.GetCurrentReading(h.ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	assert.Equal(t, bookID, reading.BookID, "the prepared withdrawn Book stays readable")

	_, err = h.submit(learner.ID, bookID)
	require.ErrorIs(t, err, analysis.ErrWithdrawnClassicalDefinition)
}

// TestClassicalPreparationRetryRoutesToPreparation proves Reading's retry sends
// a classical Book to classical_preparation, never the EPUB analysis path.
func TestClassicalPreparationRetryRoutesToPreparation(t *testing.T) {
	h := newClassicalFailureHarness(t, classicalFailureOptions{})
	h.layer.set(nil, http.StatusBadGateway)
	learner, cookies, csrf := h.learner("classical-retry-route")
	bookID := h.addBook(learner.ID)
	moved := h.moveToRead(learner.ID, bookID, cookies, csrf)
	require.Equal(t, http.StatusSeeOther, moved.Code)
	h.startClient()
	h.waitForFailure(learner.ID, bookID)
	h.layer.set(h.data, http.StatusOK)

	before := countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue)
	retried := perform(t, h.h, http.MethodPost, "/reading/books/"+bookID+"/reanalyze",
		url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, retried.Code, retried.Body.String())
	assert.Contains(t, retried.Header().Get("Location"), url.QueryEscape("Preparation job"))
	assert.Equal(t, before+1, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue))
	assert.Zero(t, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind='analyze_corpus'`), "Reading retry never uses the EPUB path")
	assert.Zero(t, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE queue='analysis'`))
}

// TestClassicalPreparationJobsSurfaceShowsStableCodesAndNoGreek proves every
// emitted failure code reaches the jobs surface and neither job output nor logs
// carry Greek text.
func TestClassicalPreparationJobsSurfaceShowsStableCodesAndNoGreek(t *testing.T) {
	h := newClassicalFailureHarness(t, classicalFailureOptions{})
	learner, cookies, _ := h.learner("classical-failure-codes")
	bookID := h.addBook(learner.ID)
	require.NoError(t, h.insertSettledFailure(h.ctx, learner.ID, bookID, ancientgreek.CodeSourceDownloadFailed, domain.ClassicalPreparationFailureFetch))

	page := h.jobsPage(cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	assert.Contains(t, page.Body.String(), ancientgreek.CodeSourceDownloadFailed)
	assert.False(t, containsGreek(page.Body.String()), "jobs surface must not print Greek")

	var jobErrors string
	require.NoError(t, h.pool.QueryRow(h.ctx, `
		SELECT COALESCE(string_agg(error_text::text, ' '), '')
		FROM river_job job, unnest(job.errors) AS error_text
		WHERE job.kind = $1`, analysis.ClassicalPreparationQueue).Scan(&jobErrors))
	assert.False(t, containsGreek(jobErrors), "job output must not print Greek")
}

// insertSettledFailure inserts a settled failure and a matching job so the jobs
// surface can be inspected without waiting on a worker.
func (h *classicalFailureHarness) insertSettledFailure(ctx context.Context, owner, bookID, code string, stage domain.ClassicalPreparationFailure) error {
	require.NoError(h.t, h.store.PutClassicalPreparationFailure(ctx, domain.ClassicalPreparationFailureRecord{
		OwnerID: owner, BookID: bookID, ReadingUnitKey: failureUnitKey,
		DefinitionRevision: h.definition.Revision, Stage: stage, Code: code,
	}))
	_, err := h.pool.Exec(ctx, `
		INSERT INTO river_job(kind, args, state, queue, max_attempts, priority, attempt, errors, finalized_at)
		VALUES ($1, jsonb_build_object('owner_id', $2::text, 'book_id', $3::text, 'reading_unit_key', $4::text, 'definition_revision', $5::integer),
		        'discarded', $1, 1, 1, 1, ARRAY[jsonb_build_object('error', $6::text, 'attempt', 1)], now())`,
		analysis.ClassicalPreparationQueue, owner, bookID, failureUnitKey, h.definition.Revision, "ancientgreek: import refused: "+code)
	return err
}

// TestClassicalPreparationLegacyRowsCodeAppearsOnJobsSurface proves the
// fail-closed catalog code is visible to the operator even though no Book can
// start a preparation.
func TestClassicalPreparationLegacyRowsCodeAppearsOnJobsSurface(t *testing.T) {
	h := newClassicalFailureHarness(t, classicalFailureOptions{})
	learner, cookies, _ := h.learner("classical-failure-legacy")
	bookID := h.addBook(learner.ID)
	_, err := h.pool.Exec(h.ctx, `INSERT INTO books(owner_id, title, metadata_provenance, language_state, language_tag)
		VALUES ($1, 'Legacy Greek', 'catalog_sync', 'chosen', $2)`, learner.ID, classical.Language)
	require.NoError(t, err)

	page := h.jobsPage(cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	assert.Contains(t, page.Body.String(), classical.LegacyRowsCode)

	_, err = h.submit(learner.ID, bookID)
	require.ErrorIs(t, err, classical.ErrLegacyRows, "preparation is blocked while legacy rows exist")
}

// TestClassicalPreparationLogsCarryNoGreek runs one failure with the global
// logger captured and asserts the operator log carries the stable code only.
func TestClassicalPreparationLogsCarryNoGreek(t *testing.T) {
	h := newClassicalFailureHarness(t, classicalFailureOptions{})
	h.layer.set([]byte("different bytes\n"), http.StatusOK)
	learner, _, _ := h.learner("classical-failure-log")
	bookID := h.addBook(learner.ID)
	_, err := h.submit(learner.ID, bookID)
	require.NoError(t, err)

	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })
	h.startClient()
	h.waitForFailure(learner.ID, bookID)
	assert.Contains(t, logs.String(), ancientgreek.CodePinMismatch)
	assert.False(t, containsGreek(logs.String()), "operator logs must not print Greek")
}
