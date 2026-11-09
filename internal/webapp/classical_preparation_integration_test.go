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

// preparationRequests counts the layer downloads one harness saw. Source and
// translation downloads are separate so a test can prove the work file and its
// translation are each fetched once.
type preparationRequests struct {
	source, translation *atomic.Int64
}

// preparationHarness provisions a fixture without a reference translation.
func preparationHarness(t *testing.T) (context.Context, *persistence.PostgresStore, *pgxpool.Pool, *Handler, *analysis.Service, *river.Client[pgx.Tx], preparationRequests, classicalPreparationFixture) {
	t.Helper()
	return preparationHarnessWithTranslation(t, nil)
}

// preparationHarnessWithTranslation provisions a migrated database, a
// reconciled synthetic release, a local layer server and a web handler. A
// non-nil translationData adds the reviewed translation pin to both Reading
// units and serves those bytes from the translation layer. The River client is
// created but not started, so a test can inspect the queued jobs first.
func preparationHarnessWithTranslation(t *testing.T, translationData []byte) (context.Context, *persistence.PostgresStore, *pgxpool.Pool, *Handler, *analysis.Service, *river.Client[pgx.Tx], preparationRequests, classicalPreparationFixture) {
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
	var translation *classical.TranslationPin
	if translationData != nil {
		translationSum := sha256.Sum256(translationData)
		translation = &classical.TranslationPin{
			Layer: "synthetic-layer", FileName: "synthetic-translation.xml",
			Size: int64(len(translationData)), SHA256: hex.EncodeToString(translationSum[:]),
			CitationStart: "1.1.1", CitationEnd: "1.2.18",
		}
	}
	first := preparationDefinition("synthetic-work-1", "Synthetic Work", "Author", pin, &one, "1.1.1", "1.1.2", 4)
	second := preparationDefinition("synthetic-work-2", "Synthetic Work", "Author", pin, &two, "2.1.1", "2.1.1", 2)
	first.Translation, second.Translation = translation, translation
	fixture.definitions = []classical.Definition{first, second}
	require.NoError(t, store.ReconcileClassicalManifest(ctx, fixture.definitions))
	catalog, err := classical.NewCatalog(fixture.definitions)
	require.NoError(t, err)

	requests := preparationRequests{source: &atomic.Int64{}, translation: &atomic.Int64{}}
	layer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := data
		counter := requests.source
		if translationData != nil && strings.HasSuffix(r.URL.Path, "synthetic-translation.xml") {
			body = translationData
			counter = requests.translation
		}
		counter.Add(1)
		if _, writeErr := w.Write(body); writeErr != nil {
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
	preparationWorker := analysis.AddClassicalPreparationWorker(workers, store, config)
	analysis.AddClassicalTranslationWorker(workers, store, config)
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			analysis.ClassicalPreparationQueue: {MaxWorkers: 1},
			analysis.ClassicalTranslationQueue: {MaxWorkers: 1},
		},
		Workers: workers,
	})
	require.NoError(t, err)
	preparationWorker.Client = client

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
	return ctx, store, pool, h, service, client, requests, fixture
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
	assert.Zero(t, layerRequests.source.Load(), "enqueueing a preparation fetches nothing")
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
	assert.Equal(t, int64(1), layerRequests.source.Load(), "the work file downloads once for both Books")

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
	assert.Equal(t, int64(1), layerRequests.source.Load(), "a second learner reuses the installation-wide source")
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

// TestClassicalTranslationFetchedOnceAndSharedByTwoLearners proves a definition's
// pinned translation is downloaded once, parsed into installation-wide rows and
// reused by a second learner without another download. The file itself is never
// stored and the Book's analysis publishes before the import even starts.
func TestClassicalTranslationFetchedOnceAndSharedByTwoLearners(t *testing.T) {
	translationData, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "perseus-eng2-section.xml"))
	require.NoError(t, err)
	ctx, store, pool, h, _, client, requests, fixture := preparationHarnessWithTranslation(t, translationData)
	learner := createAccount(t, ctx, store, "classical-translation-one", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	added := perform(t, h, http.MethodPost, "/catalogs/classical/works/synthetic-work/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	owned := booksByReadingUnit(t, store, learner.ID)
	bookID := owned[fixture.unitOneKey]
	require.NotEmpty(t, bookID)
	detail, err := store.GetBookDetail(ctx, learner.ID, bookID)
	require.NoError(t, err)
	moved := perform(t, h, http.MethodPost, "/library/books/"+bookID+"/to-read", url.Values{
		"csrf_token":        {csrf},
		"expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "classical prep translation client", client.Stop)
	waitForPreparedBooks(t, pool, learner.ID, 1)
	translationPin := fixture.definitions[0].Translation.SHA256
	waitForReferenceTranslations(t, pool, translationPin, 4)
	assert.Equal(t, int64(1), requests.translation.Load(), "the translation downloads once")
	assert.Equal(t, int64(1), requests.source.Load(), "the work file downloads once")

	var storedTranslation int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM classical_sources WHERE sha256=$1`, translationPin).Scan(&storedTranslation))
	assert.Zero(t, storedTranslation, "a translation file is never stored")

	// A second learner reads the same Work: preparation reuses both the work
	// file and the parsed translation, so neither is downloaded again.
	other := createAccount(t, ctx, store, "classical-translation-two", "learner-password")
	otherCookies, otherCSRF := loginCookies(t, h, other.Username, "learner-password")
	otherAdd := perform(t, h, http.MethodPost, "/catalogs/classical/works/synthetic-work/add", url.Values{"csrf_token": {otherCSRF}}, otherCookies)
	require.Equal(t, http.StatusSeeOther, otherAdd.Code, otherAdd.Body.String())
	otherBookID := booksByReadingUnit(t, store, other.ID)[fixture.unitOneKey]
	require.NotEmpty(t, otherBookID)
	otherDetail, err := store.GetBookDetail(ctx, other.ID, otherBookID)
	require.NoError(t, err)
	otherMoved := perform(t, h, http.MethodPost, "/library/books/"+otherBookID+"/to-read", url.Values{
		"csrf_token":        {otherCSRF},
		"expected_revision": {strconv.FormatInt(otherDetail.DispositionRevision, 10)},
	}, otherCookies)
	require.Equal(t, http.StatusSeeOther, otherMoved.Code, otherMoved.Body.String())
	waitForPreparedBooks(t, pool, other.ID, 1)
	waitForReferenceTranslations(t, pool, translationPin, 4)
	assert.Equal(t, int64(1), requests.translation.Load(), "a second learner reuses the parsed translation")
	assert.Equal(t, int64(1), requests.source.Load(), "a second learner reuses the work file")

	var rows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM reference_translations WHERE translation_pin_sha256=$1`, translationPin).Scan(&rows))
	assert.Equal(t, 4, rows)
}

// waitForReferenceTranslations waits until the installation-wide rows for one
// translation pin contain want sections.
func waitForReferenceTranslations(t *testing.T, pool *pgxpool.Pool, pin string, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var count int
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM reference_translations WHERE translation_pin_sha256=$1`, pin).Scan(&count); err != nil {
			return false
		}
		return count == want
	}, 60*time.Second, 50*time.Millisecond, "translation rows did not import")
}

// TestClassicalTranslationFailureLeavesBookAnalyzed proves the translation
// import is non-blocking: its pin mismatch fails the separate job and leaves no
// rows, while the Book's own analysis stays published and readable.
func TestClassicalTranslationFailureLeavesBookAnalyzed(t *testing.T) {
	translationData, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "perseus-eng2-section.xml"))
	require.NoError(t, err)
	ctx, store, pool, h, _, client, _, fixture := preparationHarnessWithTranslation(t, translationData)
	// The definition pins a file whose bytes the layer will not serve.
	_, err = pool.Exec(ctx, `UPDATE reading_units SET translation_file_sha256=$1 WHERE definition_key=$2`, strings.Repeat("0", 64), fixture.unitOneKey)
	require.NoError(t, err)

	learner := createAccount(t, ctx, store, "classical-translation-failure", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	added := perform(t, h, http.MethodPost, "/catalogs/classical/works/synthetic-work/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	bookID := booksByReadingUnit(t, store, learner.ID)[fixture.unitOneKey]
	require.NotEmpty(t, bookID)
	detail, err := store.GetBookDetail(ctx, learner.ID, bookID)
	require.NoError(t, err)
	moved := perform(t, h, http.MethodPost, "/library/books/"+bookID+"/to-read", url.Values{
		"csrf_token":        {csrf},
		"expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "classical translation failure client", client.Stop)
	waitForPreparedBooks(t, pool, learner.ID, 1)

	// The separate import job runs, fails its pin check and becomes retryable
	// or discarded; the Book's prepared-definition record already exists.
	require.Eventually(t, func() bool {
		var failed int
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM river_job WHERE kind=$1 AND (attempt > 0 OR state IN ('retryable','discarded'))`,
			analysis.ClassicalTranslationQueue).Scan(&failed); err != nil {
			return false
		}
		return failed > 0
	}, 30*time.Second, 50*time.Millisecond, "the translation import did not fail")
	var records int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM classical_prepared_definitions WHERE owner_id=$1 AND book_id=$2`, learner.ID, bookID).Scan(&records))
	assert.Equal(t, 1, records, "the Book's analysis is published before the translation import")
	var rows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM reference_translations WHERE translation_pin_sha256=$1`, strings.Repeat("0", 64)).Scan(&rows))
	assert.Zero(t, rows, "a failed import stores no translation rows")
}
