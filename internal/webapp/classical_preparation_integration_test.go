//go:build integration

package webapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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

// classicalPreparationFixture is one synthetic OGA work file cut into two
// Reading units, served by a local layer. It never contains a real OGA file.
type classicalPreparationFixture struct {
	definitions []classical.Definition
	data        []byte
	unitOneKey  string
	unitTwoKey  string
}

// preparationHarness provisions a migrated database, a reconciled synthetic
// release, a local layer server and a web handler. The River client is created
// but not started, so a test can inspect the queued jobs first.
func preparationHarness(t *testing.T) (context.Context, *persistence.PostgresStore, *pgxpool.Pool, *Handler, *analysis.Service, *river.Client[pgx.Tx], *atomic.Int64, classicalPreparationFixture) {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "classical-preparation-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	pin := classical.SourcePin{
		Layer: "synthetic-layer", FileName: "synthetic-work.conllu",
		Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Format: classical.SourceFormatOGACoNLLU,
	}
	one, two := 1, 2
	fixture := classicalPreparationFixture{
		data:       data,
		unitOneKey: "synthetic-work-1",
		unitTwoKey: "synthetic-work-2",
	}
	fixture.definitions = []classical.Definition{
		preparationDefinition("synthetic-work-1", "Synthetic Work", "Author", pin, &one, "1.1.1", "1.1.2", 4),
		preparationDefinition("synthetic-work-2", "Synthetic Work", "Author", pin, &two, "2.1.1", "2.1.1", 2),
	}
	require.NoError(t, store.ReconcileClassicalManifest(ctx, fixture.definitions))
	catalog, err := classical.NewCatalog(fixture.definitions)
	require.NoError(t, err)

	var layerRequests atomic.Int64
	layer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		layerRequests.Add(1)
		_, writeErr := w.Write(data)
		if writeErr != nil {
			return
		}
	}))
	t.Cleanup(layer.Close)
	layers := classical.Layers{Layers: []classical.Layer{{
		Key: "synthetic-layer", Commit: "synthetic", BaseURL: layer.URL + "/",
		LicenceClass: classical.LicenceClassSourceContent, Restriction: classical.RestrictionShareAlike,
		DeployerRegisterRow: "synthetic test layer",
	}}}
	config := analysis.ClassicalPreparationConfig{Layers: layers, HTTPClient: layer.Client(), TempDir: t.TempDir()}
	workers := river.NewWorkers()
	analysis.AddClassicalPreparationWorker(workers, store, config)
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
	return ctx, store, pool, h, service, client, &layerRequests, fixture
}

func preparationDefinition(key, title, author string, pin classical.SourcePin, book *int, start, end string, words int) classical.Definition {
	return classical.Definition{
		Key: key, WorkKey: "synthetic-work", WorkTitle: title, WorkAuthor: author,
		EditionKey: "synthetic-edition", EditionLabel: "Synthetic Edition", Language: classical.Language,
		ConstituentBook: book, ConstituentBooks: 2,
		CitationStart: start, CitationEnd: end, WordCount: words,
		Source: pin, InterpretationVersion: "oga-agdt-upos/5", Revision: 1,
	}
}

// booksByReadingUnit maps the learner's Books to their permanent Reading-unit
// keys, so a test never assumes the browse order.
func booksByReadingUnit(t *testing.T, store *persistence.PostgresStore, owner string) map[string]string {
	t.Helper()
	owned, err := store.ListOwnedClassicalReadingUnits(context.Background(), owner)
	require.NoError(t, err)
	byUnit := make(map[string]string, len(owned))
	for _, unit := range owned {
		byUnit[unit.ReadingUnitKey] = unit.BookID
	}
	return byUnit
}

func sourceMaterialFor(t *testing.T, pool *pgxpool.Pool, owner, bookID string) (identifier string, content []byte, fullText string) {
	t.Helper()
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT source_identifier, content, full_text FROM source_materials WHERE owner_id=$1 AND book_id=$2`,
		owner, bookID).Scan(&identifier, &content, &fullText))
	return identifier, content, fullText
}

func waitForPreparedBooks(t *testing.T, pool *pgxpool.Pool, owner string, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var count int
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM book_current_analyses WHERE owner_id=$1`, owner).Scan(&count); err != nil {
			return false
		}
		return count == want
	}, 60*time.Second, 50*time.Millisecond, "Books did not publish their classical analysis")
}

// TestClassicalPreparationFetchesOnceAndPublishesEachUnit is the acceptance
// seam for classical preparation: Add -> To Read -> the in-process worker runs
// -> ordinary analyses publish -> the Book is readable, with no NLP service and
// no whole-work file stored per Book.
func TestClassicalPreparationFetchesOnceAndPublishesEachUnit(t *testing.T) {
	ctx, store, pool, h, service, client, layerRequests, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "classical-prep-one", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	added := perform(t, h, http.MethodPost, "/catalogs/classical/works/synthetic-work/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	owned := booksByReadingUnit(t, store, learner.ID)
	require.Len(t, owned, 2)
	firstID, secondID := owned[fixture.unitOneKey], owned[fixture.unitTwoKey]
	require.NotEmpty(t, firstID)
	require.NotEmpty(t, secondID)
	ids := []string{firstID, secondID}

	// To Read queues the dedicated job; the shared analysis queue is never used.
	for _, id := range ids {
		detail, err := store.GetBookDetail(ctx, learner.ID, id)
		require.NoError(t, err)
		moved := perform(t, h, http.MethodPost, "/library/books/"+id+"/to-read", url.Values{
			"csrf_token":        {csrf},
			"expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
		}, cookies)
		require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())
		after, err := store.GetBookDetail(ctx, learner.ID, id)
		require.NoError(t, err)
		assert.Equal(t, domain.BookDispositionToRead, after.Disposition)
	}
	assert.Zero(t, layerRequests.Load(), "enqueueing a preparation fetches nothing")
	var prepJobs, analysisJobs int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue).Scan(&prepJobs))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='analyze_corpus'`).Scan(&analysisJobs))
	assert.Equal(t, 2, prepJobs)
	assert.Zero(t, analysisJobs, "classical preparation never uses the shared analysis queue")
	var classicalQueueRows, analysisQueueRows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE queue=$1`, analysis.ClassicalPreparationQueue).Scan(&classicalQueueRows))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE queue='analysis'`).Scan(&analysisQueueRows))
	assert.Equal(t, 2, classicalQueueRows)
	assert.Zero(t, analysisQueueRows, "EPUB analysis is not delayed by classical jobs")

	// A second submission for the same Book and definition reuses the live job
	// instead of refusing or duplicating it.
	firstHandle, err := service.SubmitClassicalPreparation(ctx, learner.ID, ids[0])
	require.NoError(t, err)
	assert.True(t, firstHandle.Queued)
	var firstJobID int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM river_job WHERE kind=$1 AND args->>'book_id'=$2`, analysis.ClassicalPreparationQueue, ids[0]).Scan(&firstJobID))
	assert.Equal(t, firstJobID, firstHandle.JobID)
	var prepJobsAfter int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue).Scan(&prepJobsAfter))
	assert.Equal(t, 2, prepJobsAfter, "ensure-once keeps one job per Book and revision")

	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "classical preparation client", client.Stop)
	waitForPreparedBooks(t, pool, learner.ID, 2)
	assert.Equal(t, int64(1), layerRequests.Load(), "the work file downloads once for both Books")

	// The jobs surface shows the Book, attempt and state.
	jobsPage := perform(t, h, http.MethodGet, "/jobs", nil, cookies)
	require.Equal(t, http.StatusOK, jobsPage.Code, jobsPage.Body.String())
	for _, want := range []string{"Classical preparation history", "Classical preparation", "Synthetic Work"} {
		assert.Contains(t, jobsPage.Body.String(), want)
	}

	// The store reuses and reports the installation-wide source, and an unknown
	// Book is not a preparation target.
	storedSource, found, err := store.GetClassicalSource(ctx, fixture.definitions[0].Source.SHA256)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, fixture.definitions[0].Source.FileName, storedSource.FileName)
	require.NoError(t, store.PutClassicalSource(ctx, storedSource), "a duplicate source insert is a no-op")
	_, found, err = store.GetClassicalSource(ctx, strings.Repeat("b", 64))
	require.NoError(t, err)
	assert.False(t, found)
	_, err = store.LoadClassicalPreparationTarget(ctx, learner.ID, "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, persistence.ErrNotFound)

	// Each Book stores only its own Reading unit, keyed by the Reading-unit key.
	first := bookWithUnit(t, pool, learner.ID, firstID)
	second := bookWithUnit(t, pool, learner.ID, secondID)
	firstIdentifier, firstContent, firstText := sourceMaterialFor(t, pool, learner.ID, firstID)
	secondIdentifier, secondContent, secondText := sourceMaterialFor(t, pool, learner.ID, secondID)
	assert.Equal(t, first.unit, firstIdentifier, "the source identifier is the Reading-unit key")
	assert.Equal(t, firstText, string(firstContent), "the stored content is the unit text")
	assert.Equal(t, second.unit, secondIdentifier)
	assert.Equal(t, secondText, string(secondContent))
	assert.Less(t, len(firstContent), len(fixture.data), "the whole work file is not stored per Book")
	assert.Less(t, len(secondContent), len(fixture.data))
	assert.Contains(t, firstText, "λέγει")
	assert.NotContains(t, firstText, "Ἀρταξέρξης", "Book 1 stores only Book 1's unit")
	assert.Contains(t, secondText, "Ἀρταξέρξης")
	assert.NotContains(t, secondText, "λέγει", "Book 2 stores only Book 2's unit")

	// The prepared-definition record matches each Book's definition.
	for _, book := range []struct{ id, unit string }{{firstID, first.unit}, {secondID, second.unit}} {
		var key string
		var revision int
		var pin, interpretation, runID string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT reading_unit_key, definition_revision, source_file_sha256, interpretation_version, analysis_run_id::text
			 FROM classical_prepared_definitions WHERE owner_id=$1 AND book_id=$2`,
			learner.ID, book.id).Scan(&key, &revision, &pin, &interpretation, &runID))
		assert.Equal(t, book.unit, key)
		assert.Equal(t, 1, revision)
		assert.Equal(t, fixture.definitions[0].Source.SHA256, pin)
		assert.Equal(t, "oga-agdt-upos/5", interpretation)
		assert.NotEmpty(t, runID)
	}

	// Reading's retry path routes a classical Book to preparation. A current
	// analysis is reported as current rather than queued again.
	reanalyzed := perform(t, h, http.MethodPost, "/reading/books/"+secondID+"/reanalyze", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, reanalyzed.Code, reanalyzed.Body.String())
	assert.Contains(t, reanalyzed.Header().Get("Location"), url.QueryEscape("already current"))

	// The Book is readable: starting it freezes its published analysis.
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))
	started := perform(t, h, http.MethodPost, "/reading/books/"+firstID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code, started.Body.String())
	reading, err := store.GetCurrentReading(ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	assert.Equal(t, firstID, reading.BookID)

	// A second To Read after publication is a no-op.
	beforeSecondToRead := countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue)
	detail, err := store.GetBookDetail(ctx, learner.ID, secondID)
	require.NoError(t, err)
	again := perform(t, h, http.MethodPost, "/library/books/"+secondID+"/to-read", url.Values{
		"csrf_token":        {csrf},
		"expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, again.Code)
	assert.Equal(t, beforeSecondToRead, countRows(t, pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue))
	complete, err := service.SubmitClassicalPreparation(ctx, learner.ID, secondID)
	require.NoError(t, err)
	assert.True(t, complete.Complete, "a published preparation is already current")
	assert.False(t, complete.Queued, "no new job is queued for a current analysis")

	// An interrupted run is resumed, not refused: discard the job and remove
	// the record, then submit again. The published run is reused and the
	// record is rewritten.
	_, err = pool.Exec(ctx, `DELETE FROM classical_prepared_definitions WHERE owner_id=$1 AND book_id=$2`, learner.ID, secondID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE river_job SET state='discarded', finalized_at=now(), unique_key=NULL
		WHERE kind=$1 AND args->>'book_id'=$2`, analysis.ClassicalPreparationQueue, secondID)
	require.NoError(t, err)
	resumed, err := service.SubmitClassicalPreparation(ctx, learner.ID, secondID)
	require.NoError(t, err)
	assert.True(t, resumed.Queued, "a failed or interrupted run is submitted again")
	require.Eventually(t, func() bool {
		var records int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM classical_prepared_definitions WHERE owner_id=$1 AND book_id=$2`, learner.ID, secondID).Scan(&records); err != nil {
			return false
		}
		return records == 1
	}, 30*time.Second, 50*time.Millisecond, "the resumed run rewrites the prepared-definition record")
	var runs int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, learner.ID).Scan(&runs))
	assert.Equal(t, 2, runs, "resuming reuses the existing completed run")

	// A second learner reuses the stored work file and downloads nothing.
	other := createAccount(t, ctx, store, "classical-prep-two", "learner-password")
	otherCookies, otherCSRF := loginCookies(t, h, other.Username, "learner-password")
	otherAdd := perform(t, h, http.MethodPost, "/catalogs/classical/works/synthetic-work/add", url.Values{"csrf_token": {otherCSRF}}, otherCookies)
	require.Equal(t, http.StatusSeeOther, otherAdd.Code, otherAdd.Body.String())
	otherOwned := booksByReadingUnit(t, store, other.ID)
	require.Len(t, otherOwned, 2)
	otherID := otherOwned[fixture.unitOneKey]
	require.NotEmpty(t, otherID)
	otherDetail, err := store.GetBookDetail(ctx, other.ID, otherID)
	require.NoError(t, err)
	otherMoved := perform(t, h, http.MethodPost, "/library/books/"+otherID+"/to-read", url.Values{
		"csrf_token":        {otherCSRF},
		"expected_revision": {strconv.FormatInt(otherDetail.DispositionRevision, 10)},
	}, otherCookies)
	require.Equal(t, http.StatusSeeOther, otherMoved.Code, otherMoved.Body.String())
	waitForPreparedBooks(t, pool, other.ID, 1)
	assert.Equal(t, int64(1), layerRequests.Load(), "a second learner reuses the installation-wide source")
}

type preparedBookUnit struct {
	unit string
}

func bookWithUnit(t *testing.T, pool *pgxpool.Pool, owner, bookID string) preparedBookUnit {
	t.Helper()
	var unit string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT ru.definition_key FROM book_reading_units bru
		 JOIN reading_units ru ON ru.id = bru.reading_unit_id
		 WHERE bru.owner_id=$1 AND bru.book_id=$2`, owner, bookID).Scan(&unit))
	return preparedBookUnit{unit: unit}
}

// TestClassicalPreparationPrunesSourcesAtStartup proves reconciliation deletes
// a stored work file that no current, non-withdrawn definition pins.
func TestClassicalPreparationPrunesSourcesAtStartup(t *testing.T) {
	ctx, store, pool, h, _, client, _, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "classical-prep-prune", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	added := perform(t, h, http.MethodPost, "/catalogs/classical/works/synthetic-work/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	owned := booksByReadingUnit(t, store, learner.ID)
	require.Len(t, owned, 2)
	bookID := owned[fixture.unitOneKey]
	require.NotEmpty(t, bookID)
	detail, err := store.GetBookDetail(ctx, learner.ID, bookID)
	require.NoError(t, err)
	moved := perform(t, h, http.MethodPost, "/library/books/"+bookID+"/to-read", url.Values{
		"csrf_token":        {csrf},
		"expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "classical preparation client", client.Stop)
	waitForPreparedBooks(t, pool, learner.ID, 1)
	require.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM classical_sources`))

	// A later release with a different pin withdraws the old units. Startup
	// reconciliation then removes the file no current definition pins.
	other := preparationDefinition("replacement-work-1", "Replacement", "Author", classical.SourcePin{
		Layer: "synthetic-layer", FileName: "replacement.conllu", Size: 1,
		SHA256: strings.Repeat("a", 64), Format: classical.SourceFormatOGACoNLLU,
	}, nil, "1.1.1", "1.1.1", 1)
	other.WorkKey, other.EditionKey = "replacement-work", "replacement-edition"
	other.ConstituentBooks = 1
	require.NoError(t, store.ReconcileClassicalManifest(ctx, []classical.Definition{other}))
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM classical_sources`), "unpinned stored source is pruned")
}
