//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// workUpdateHarness prepares a two-Book synthetic Work, then replaces the
// handler's update submissions with a River client that is not started. The
// preparation client is stopped, so queued update jobs stay inspectable.
type workUpdateHarness struct {
	ctx       context.Context
	store     *persistence.PostgresStore
	pool      *pgxpool.Pool
	h         *Handler
	updates   *analysis.Service
	learnerID string
	cookies   []*http.Cookie
	csrf      string
	fixture   classicalPreparationFixture
	firstID   string
	secondID  string
}

// newWorkUpdateHarness uses the real preparation path once, then exercises
// Analysis updates without a running worker.
func newWorkUpdateHarness(t *testing.T) workUpdateHarness {
	t.Helper()
	ctx, store, pool, h, _, client, _, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "work-analysis-update", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "synthetic-work")
	moved := perform(t, h, http.MethodPost, "/library/works/synthetic-work/to-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "work analysis update preparation client", client.Stop)
	waitForPreparedBooks(t, pool, learner.ID, 2)
	require.NoError(t, client.Stop(ctx))

	updatesClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "work analysis update client", updatesClient.Stop)
	updates := analysis.NewService(store.Pool(), updatesClient).WithClassicalPreparationStore(store)
	h.services.AnalysisUpdates = updates
	h.services.ClassicalPreparation = updates

	owned := booksByReadingUnit(t, store, learner.ID)
	require.NotEmpty(t, owned[fixture.unitOneKey])
	require.NotEmpty(t, owned[fixture.unitTwoKey])
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))
	return workUpdateHarness{
		ctx: ctx, store: store, pool: pool, h: h, updates: updates,
		learnerID: learner.ID, cookies: cookies, csrf: csrf, fixture: fixture,
		firstID: owned[fixture.unitOneKey], secondID: owned[fixture.unitTwoKey],
	}
}

// bumpInterpretation applies the same content-bearing change to one or more
// Reading units, as a release that re-interprets the stored source would.
func bumpInterpretation(t *testing.T, pool *pgxpool.Pool, version string, unitKeys ...string) {
	t.Helper()
	for _, key := range unitKeys {
		_, err := pool.Exec(context.Background(), `UPDATE reading_units
			SET interpretation_version=$1, revision=revision+1
			WHERE definition_key=$2`, version, key)
		require.NoError(t, err)
	}
}

func preparedRunID(t *testing.T, pool *pgxpool.Pool, owner, bookID string) string {
	t.Helper()
	var runID string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT analysis_run_id::text FROM classical_prepared_definitions WHERE owner_id=$1 AND book_id=$2`,
		owner, bookID).Scan(&runID))
	return runID
}

// addLemmaDecisions attaches corrections and exclusions to a Book's published
// analysis so the Work confirmation must disclose them.
func addLemmaDecisions(t *testing.T, pool *pgxpool.Pool, owner, bookID, runID string, corrections, exclusions int) {
	t.Helper()
	var corpusID string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT id::text FROM corpora WHERE owner_id=$1 AND analysis_run_id=$2`, owner, runID).Scan(&corpusID))
	for i := range corrections {
		_, err := pool.Exec(context.Background(), `INSERT INTO occurrence_lemma_corrections(
			owner_id, book_id, corpus_id, analysis_run_id, source_document_id,
			start_offset, end_offset, canonical_lemma, normalization_profile, normalization_version, excluded)
			VALUES($1,$2,$3,$4,'unit',$5,$6,'λέγω','ancient-greek','1',false)`,
			owner, bookID, corpusID, runID, i*10, i*10+1)
		require.NoError(t, err)
	}
	for i := range exclusions {
		_, err := pool.Exec(context.Background(), `INSERT INTO occurrence_lemma_corrections(
			owner_id, book_id, corpus_id, analysis_run_id, source_document_id,
			start_offset, end_offset, canonical_lemma, normalization_profile, normalization_version, excluded)
			VALUES($1,$2,$3,$4,'unit',$5,$6,NULL,NULL,NULL,true)`,
			owner, bookID, corpusID, runID, 100+i*10, 100+i*10+1)
		require.NoError(t, err)
	}
}

// workAnalysisUpdateCandidates reads the offers the page will render.
func workAnalysisUpdateCandidates(t *testing.T, h workUpdateHarness, books ...domain.MyBook) map[string]domain.AnalysisUpdateCandidate {
	t.Helper()
	sourceIDs := make([]string, 0, len(books))
	for _, book := range books {
		require.NotNil(t, book.Acquired)
		sourceIDs = append(sourceIDs, book.Acquired.Source.ID)
	}
	candidates, err := h.updates.AnalysisUpdateCandidates(context.Background(), h.learnerID, sourceIDs)
	require.NoError(t, err)
	return candidates
}

// workAnalysisUpdateForm mirrors the hidden fields the rendered confirmation
// carries back, limited to the Books that currently have offers.
func workAnalysisUpdateForm(t *testing.T, h workUpdateHarness, books ...domain.MyBook) url.Values {
	t.Helper()
	form := url.Values{"csrf_token": {h.csrf}, "return_to": {"/library"}}
	candidates := workAnalysisUpdateCandidates(t, h, books...)
	for _, book := range books {
		candidate, ok := candidates[book.Acquired.Source.ID]
		if !ok {
			continue
		}
		form["book_id"] = append(form["book_id"], book.Book.ID)
		form["source_id"] = append(form["source_id"], book.Acquired.Source.ID)
		form["expected_run_id"] = append(form["expected_run_id"], candidate.PublishedRunID)
		form["expected_target"] = append(form["expected_target"], candidate.Target.Key())
	}
	return form
}

func (h workUpdateHarness) book(t *testing.T, id string) domain.MyBook {
	t.Helper()
	book, err := h.store.GetBookDetail(context.Background(), h.learnerID, id)
	require.NoError(t, err)
	return book
}

func TestClassicalWorkAnalysisUpdateSkippedCurrentBookReturnsAfterEnd(t *testing.T) {
	h := newWorkUpdateHarness(t)
	second := h.book(t, h.secondID)

	// Seed the second Book's published analysis with decisions the confirmation
	// must disclose, then offer the update to both Books.
	secondRun := preparedRunID(t, h.pool, h.learnerID, h.secondID)
	addLemmaDecisions(t, h.pool, h.learnerID, h.secondID, secondRun, 2, 1)
	bumpInterpretation(t, h.pool, "oga-agdt-upos/6", h.fixture.unitOneKey, h.fixture.unitTwoKey)

	// The current Reading's Book is named and skipped; only the other Book has
	// a bulk action to confirm.
	started, err := h.store.StartCurrentReadingResult(context.Background(), h.learnerID, classical.Language, h.firstID)
	require.NoError(t, err)
	body := readingGet(t, h.h, "/library", h.cookies)
	assert.Contains(t, body, "A newer analysis is available for 1 book.")
	assert.Contains(t, body, "Book 1: End the Reading first")
	assert.Contains(t, body, "Update 1 book of Synthetic Work")
	assert.Contains(t, body, "Together, the Books in this work have 2 confirmed lemma corrections and 1 occurrence exclusion, which will not carry over to their new analyses.")
	assert.Contains(t, body, "Synthetic Work, Book 2: 2 confirmed lemma corrections and 1 occurrence exclusion")
	assert.Contains(t, body, `action="/library/works/synthetic-work/analysis-update"`)
	assert.Equal(t, 1, strings.Count(body, `action="/library/works/synthetic-work/analysis-update"`), "one bulk action per Work row")

	form := workAnalysisUpdateForm(t, h, second)
	require.Len(t, form["book_id"], 1)
	submitted := perform(t, h.h, http.MethodPost, "/library/works/synthetic-work/analysis-update", form, h.cookies)
	require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())
	location := submitted.Header().Get("Location")
	assert.Contains(t, location, url.QueryEscape("Queued analysis updates for 1 book."))

	queues, bookIDs := workPreparationJobs(t, h.pool)
	require.NotEmpty(t, bookIDs)
	assert.Equal(t, h.secondID, bookIDs[len(bookIDs)-1], "only the non-current Book is queued")
	assert.Equal(t, analysis.ClassicalPreparationQueue, queues[len(queues)-1])
	assert.Zero(t, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind='analyze_corpus'`), "a classical update never calls the NLP service")
	assert.Zero(t, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE queue='analysis'`))

	// The published analysis and its decisions stay in effect until the update
	// publishes; the current Reading keeps its exact snapshot.
	assert.Equal(t, secondRun, preparedRunID(t, h.pool, h.learnerID, h.secondID))
	assert.Equal(t, 3, countRows(t, h.pool, `SELECT count(*) FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, h.learnerID, h.secondID, secondRun))
	reading, err := h.store.GetCurrentReading(context.Background(), h.learnerID, classical.Language)
	require.NoError(t, err)
	assert.Equal(t, started.Reading.SnapshotID, reading.SnapshotID)

	// After End the Book is To Read and its offer returns with the other Book.
	require.NoError(t, h.store.EndCurrentReading(context.Background(), h.learnerID, classical.Language, h.firstID, started.Reading.SnapshotID))
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.Contains(t, body, "A newer analysis is available for 2 books.")
	assert.Contains(t, body, "Update 2 books of Synthetic Work")
	assert.NotContains(t, body, "End the Reading first")
}

func TestClassicalWorkAnalysisUpdateQueuesEveryBookInBookOrder(t *testing.T) {
	h := newWorkUpdateHarness(t)
	first := h.book(t, h.firstID)
	second := h.book(t, h.secondID)
	bumpInterpretation(t, h.pool, "oga-agdt-upos/6", h.fixture.unitOneKey, h.fixture.unitTwoKey)

	body := readingGet(t, h.h, "/library", h.cookies)
	assert.Contains(t, body, "A newer analysis is available for 2 books.")
	assert.Contains(t, body, "Update 2 books of Synthetic Work")

	// A search that narrows the row's cells to one Book does not narrow the
	// Work's offer: the bulk action still covers the whole Work.
	filtered := readingGet(t, h.h, "/library?q=Synthetic+Work+1", h.cookies)
	assert.Contains(t, filtered, "A newer analysis is available for 2 books.")
	assert.Contains(t, filtered, `name="book_id" value="`+h.firstID+`"`)
	assert.Contains(t, filtered, `name="book_id" value="`+h.secondID+`"`)

	// Reading's chooser carries the same grouped offer.
	chooser := readingGet(t, h.h, "/reading", h.cookies)
	assert.Contains(t, chooser, "A newer analysis is available for 2 books.")
	assert.Contains(t, chooser, "Update 2 books of Synthetic Work")
	assert.Contains(t, chooser, `action="/library/works/synthetic-work/analysis-update"`)

	form := workAnalysisUpdateForm(t, h, first, second)
	require.Len(t, form["book_id"], 2)
	submitted := perform(t, h.h, http.MethodPost, "/library/works/synthetic-work/analysis-update", form, h.cookies)
	require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())
	assert.Contains(t, submitted.Header().Get("Location"), url.QueryEscape("Queued analysis updates for 2 books."))

	queues, bookIDs := workPreparationJobs(t, h.pool)
	require.GreaterOrEqual(t, len(bookIDs), 2)
	assert.Equal(t, []string{h.firstID, h.secondID}, bookIDs[len(bookIDs)-2:], "jobs are enqueued in constituent-book order")
	for _, queue := range queues[len(queues)-2:] {
		assert.Equal(t, analysis.ClassicalPreparationQueue, queue)
	}
	assert.Zero(t, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind='analyze_corpus'`), "no NLP cost")
}

func TestClassicalWorkAnalysisUpdateRevalidatesEachBook(t *testing.T) {
	h := newWorkUpdateHarness(t)
	first := h.book(t, h.firstID)
	second := h.book(t, h.secondID)
	bumpInterpretation(t, h.pool, "oga-agdt-upos/6", h.fixture.unitOneKey, h.fixture.unitTwoKey)

	form := workAnalysisUpdateForm(t, h, first, second)
	require.Len(t, form["book_id"], 2)

	// Another tab changes only the second Book's target after the page was
	// rendered. Its confirmation is stale; the first Book still goes ahead.
	bumpInterpretation(t, h.pool, "oga-agdt-upos/7", h.fixture.unitTwoKey)
	submitted := perform(t, h.h, http.MethodPost, "/library/works/synthetic-work/analysis-update", form, h.cookies)
	require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())
	location := submitted.Header().Get("Location")
	assert.Contains(t, location, url.QueryEscape("Queued analysis updates for 1 book."))
	assert.Contains(t, location, url.QueryEscape("Skipped 1 book whose offer changed"))

	_, bookIDs := workPreparationJobs(t, h.pool)
	require.NotEmpty(t, bookIDs)
	assert.Equal(t, h.firstID, bookIDs[len(bookIDs)-1], "the changed Book is skipped and the rest go ahead")

	// The skipped Book keeps its published analysis and shows the refreshed
	// target on the next page.
	var version string
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT interpretation_version FROM reading_units WHERE definition_key=$1`, h.fixture.unitTwoKey).Scan(&version))
	assert.Equal(t, "oga-agdt-upos/7", version)
	refreshed := workAnalysisUpdateCandidates(t, h, second)[second.Acquired.Source.ID]
	assert.Contains(t, refreshed.Target.Key(), "oga-agdt-upos/7")
}

func TestClassicalWorkAnalysisUpdatePublishesAndRefreshesPreparedDefinition(t *testing.T) {
	ctx, store, pool, h, service, client, requests, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "work-analysis-update-publish", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "synthetic-work")
	moved := perform(t, h, http.MethodPost, "/library/works/synthetic-work/to-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "work analysis update publish client", client.Stop)
	waitForPreparedBooks(t, pool, learner.ID, 2)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))
	h.services.AnalysisUpdates = service

	owned := booksByReadingUnit(t, store, learner.ID)
	firstID, secondID := owned[fixture.unitOneKey], owned[fixture.unitTwoKey]
	require.NotEmpty(t, firstID)
	require.NotEmpty(t, secondID)
	oldRun := preparedRunID(t, pool, learner.ID, secondID)
	addLemmaDecisions(t, pool, learner.ID, secondID, oldRun, 1, 1)
	_, err := store.PutKnownVocabularyIdentity(ctx, learner.ID, classical.Language, "λέγω", "VERB", "morpheus:1")
	require.NoError(t, err)

	// An interpretation-only change reuses the stored work file. The update
	// publishes and rewrites the prepared-definition record, while the earlier
	// run and its decisions stay in place.
	bumpInterpretation(t, pool, "oga-agdt-upos/6", fixture.unitOneKey, fixture.unitTwoKey)
	harness := workUpdateHarness{
		ctx: ctx, store: store, pool: pool, h: h, updates: service,
		learnerID: learner.ID, cookies: cookies, csrf: csrf, fixture: fixture,
		firstID: firstID, secondID: secondID,
	}
	first := harness.book(t, firstID)
	second := harness.book(t, secondID)
	form := workAnalysisUpdateForm(t, harness, first, second)
	submitted := perform(t, harness.h, http.MethodPost, "/library/works/synthetic-work/analysis-update", form, harness.cookies)
	require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())

	require.Eventually(t, func() bool {
		var count int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM classical_prepared_definitions
			WHERE owner_id=$1 AND interpretation_version=$2`, learner.ID, "oga-agdt-upos/6").Scan(&count); err != nil {
			return false
		}
		return count == 2
	}, 60*time.Second, 50*time.Millisecond, "both successful updates refresh the prepared-definition record")
	assert.Equal(t, int64(1), requests.source.Load(), "an interpretation-only update reuses the stored work file")

	newRun := preparedRunID(t, pool, learner.ID, secondID)
	assert.NotEqual(t, oldRun, newRun)
	var published string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`,
		learner.ID, secondID).Scan(&published))
	assert.Equal(t, newRun, published, "the successful update publishes its new analysis")
	assert.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, learner.ID, secondID, oldRun), "the earlier correction and exclusion stay frozen")
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, learner.ID, secondID, newRun), "a correction does not carry over")
	known, err := store.IsKnownVocabularyIdentity(ctx, learner.ID, classical.Language, "λέγω", "VERB")
	require.NoError(t, err)
	assert.True(t, known, "Known vocabulary is unchanged by an Analysis update")
}

func TestClassicalAnalysisUpdateIgnoresCuratorialAndTranslationRevisions(t *testing.T) {
	h := newWorkUpdateHarness(t)

	body := readingGet(t, h.h, "/library", h.cookies)
	assert.NotContains(t, body, "A newer analysis is available.", "a prepared Book starts current")

	// A confirmation replayed after the offer disappeared queues nothing and
	// returns the refreshed state.
	first := h.book(t, h.firstID)
	stale := perform(t, h.h, http.MethodPost, "/library/works/synthetic-work/analysis-update", url.Values{
		"csrf_token":      {h.csrf},
		"return_to":       {"/library"},
		"book_id":         {h.firstID},
		"source_id":       {first.Acquired.Source.ID},
		"expected_run_id": {"stale-run"},
		"expected_target": {"stale-target"},
	}, h.cookies)
	require.Equal(t, http.StatusSeeOther, stale.Code, stale.Body.String())
	assert.Contains(t, stale.Header().Get("Location"), url.QueryEscape("No analysis update was queued."))
	assert.Contains(t, stale.Header().Get("Location"), url.QueryEscape("Skipped 1 book whose offer changed"))

	// A curatorial correction alone is not content-bearing.
	_, err := h.pool.Exec(context.Background(), `UPDATE works SET title='Synthetic Work Revised' WHERE title='Synthetic Work'`)
	require.NoError(t, err)
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.NotContains(t, body, "A newer analysis is available.")

	// A revision that changes only the translation pin is not content-bearing
	// for the analysis, even though it bumps the definition revision.
	_, err = h.pool.Exec(context.Background(), `UPDATE reading_units
		SET translation_layer='synthetic-layer', translation_file_name='synthetic-translation.xml',
		    translation_file_size=1, translation_file_sha256=$1,
		    translation_citation_start='1.1.1', translation_citation_end='1.1.2',
		    revision=revision+1
		WHERE definition_key=$2`, strings.Repeat("c", 64), h.fixture.unitOneKey)
	require.NoError(t, err)
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.NotContains(t, body, "A newer analysis is available.")

	// A source-pin change is content-bearing. Only the second unit changes, so
	// the Work offers exactly one Book.
	_, err = h.pool.Exec(context.Background(), `UPDATE reading_units
		SET source_file_sha256=$1, revision=revision+1 WHERE definition_key=$2`, strings.Repeat("d", 64), h.fixture.unitTwoKey)
	require.NoError(t, err)
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.Contains(t, body, "A newer analysis is available for 1 book.")
	assert.Contains(t, body, "Update 1 book of Synthetic Work Revised")

	// A change to the other unit as well offers both Books.
	_, err = h.pool.Exec(context.Background(), `UPDATE reading_units
		SET source_file_sha256=$1, revision=revision+1 WHERE definition_key=$2`, strings.Repeat("e", 64), h.fixture.unitOneKey)
	require.NoError(t, err)
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.Contains(t, body, "A newer analysis is available for 2 books.")

	// An Inbox Book is not offered. The second Book is still To Read, so the
	// Work keeps one offer.
	require.NoError(t, h.store.SetBookDisposition(context.Background(), h.learnerID, h.firstID, domain.BookDispositionInbox))
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.Contains(t, body, "A newer analysis is available for 1 book.")

	// A Read Book is not offered either; with both Books outside To Read the
	// Work has no offer.
	_, err = h.store.ImportPreviouslyRead(context.Background(), h.learnerID, h.secondID)
	require.NoError(t, err)
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.NotContains(t, body, "A newer analysis is available.")
}
