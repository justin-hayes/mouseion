//go:build integration

package webapp

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
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
	addClassicalTestUnits(t, h, cookies, csrf, "synthetic-work-1", "synthetic-work-2")
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
// analysis so the confirmation must disclose them.
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

// analysisUpdateCandidatesFor reads the offers the page will render.
func analysisUpdateCandidatesFor(t *testing.T, h workUpdateHarness, books ...domain.MyBook) map[string]domain.AnalysisUpdateCandidate {
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

// submitBookAnalysisUpdate confirms one Book's offer as its rendered form does.
func submitBookAnalysisUpdate(t *testing.T, h workUpdateHarness, book domain.MyBook) *httptest.ResponseRecorder {
	t.Helper()
	candidate, ok := analysisUpdateCandidatesFor(t, h, book)[book.Acquired.Source.ID]
	require.True(t, ok, "the Book has an offer to confirm")
	return perform(t, h.h, http.MethodPost, readingAnalysisUpdateURL(book.Book.ID), url.Values{
		"csrf_token":      {h.csrf},
		"expected_run_id": {candidate.PublishedRunID},
		"expected_target": {candidate.Target.Key()},
	}, h.cookies)
}

func (h workUpdateHarness) book(t *testing.T, id string) domain.MyBook {
	t.Helper()
	book, err := h.store.GetBookDetail(context.Background(), h.learnerID, id)
	require.NoError(t, err)
	return book
}

func TestClassicalAnalysisUpdateSkipsTheCurrentBookUntilEnd(t *testing.T) {
	h := newWorkUpdateHarness(t)
	second := h.book(t, h.secondID)

	// Seed the second Book's published analysis with decisions the confirmation
	// must disclose, then offer the update to both Books.
	secondRun := preparedRunID(t, h.pool, h.learnerID, h.secondID)
	addLemmaDecisions(t, h.pool, h.learnerID, h.secondID, secondRun, 2, 1)
	bumpInterpretation(t, h.pool, "oga-agdt-upos/6", h.fixture.unitOneKey, h.fixture.unitTwoKey)

	// The current Reading's Book asks to end first; only the other Book has an
	// update to confirm.
	started, err := h.store.StartCurrentReadingResult(context.Background(), h.learnerID, classical.Language, h.firstID)
	require.NoError(t, err)
	body := readingGet(t, h.h, "/library", h.cookies)
	assert.Contains(t, body, html.EscapeString(analysisUpdateEndReadingFirst), "the current Book's own row asks to end the Reading first")
	assert.Contains(t, body, `action="`+readingAnalysisUpdateURL(h.secondID)+`"`, "the other Book has its own ordinary update")
	assert.NotContains(t, body, `action="`+readingAnalysisUpdateURL(h.firstID)+`"`)
	assert.NotContains(t, body, "/library/works/", "My Books carries no Work-level update")

	submitted := submitBookAnalysisUpdate(t, h, second)
	require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())
	assert.Contains(t, submitted.Header().Get("Location"), url.QueryEscape("Analysis update for"))

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
	assert.Equal(t, started.Reading.BookID, reading.BookID)

	// After End the Book is To Read and its offer returns with the other Book.
	require.NoError(t, h.store.EndCurrentReading(context.Background(), h.learnerID, classical.Language))
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.Equal(t, 2, strings.Count(body, analysisUpdateNotice), "each Book's row offers its own update")
	assert.Contains(t, body, `action="`+readingAnalysisUpdateURL(h.firstID)+`"`)
	assert.Contains(t, body, `action="`+readingAnalysisUpdateURL(h.secondID)+`"`)
	assert.NotContains(t, body, html.EscapeString(analysisUpdateEndReadingFirst))
}

func TestClassicalAnalysisUpdateOffersAndQueuesEachBookOnItsOwn(t *testing.T) {
	h := newWorkUpdateHarness(t)
	first := h.book(t, h.firstID)
	second := h.book(t, h.secondID)
	bumpInterpretation(t, h.pool, "oga-agdt-upos/6", h.fixture.unitOneKey, h.fixture.unitTwoKey)

	for _, surface := range []string{"/library", "/reading"} {
		body := readingGet(t, h.h, surface, h.cookies)
		assert.Equal(t, 2, strings.Count(body, analysisUpdateNotice), "%s offers each Book its own ordinary update", surface)
		assert.Contains(t, body, `action="`+readingAnalysisUpdateURL(h.firstID)+`"`, surface)
		assert.Contains(t, body, `action="`+readingAnalysisUpdateURL(h.secondID)+`"`, surface)
		assert.NotContains(t, body, "/library/works/", "%s carries no Work-level update", surface)
	}

	for _, book := range []domain.MyBook{first, second} {
		submitted := submitBookAnalysisUpdate(t, h, book)
		require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())
		assert.Contains(t, submitted.Header().Get("Location"), url.QueryEscape("Analysis update for"))
	}

	queues, bookIDs := workPreparationJobs(t, h.pool)
	require.GreaterOrEqual(t, len(bookIDs), 2)
	assert.Equal(t, []string{h.firstID, h.secondID}, bookIDs[len(bookIDs)-2:])
	for _, queue := range queues[len(queues)-2:] {
		assert.Equal(t, analysis.ClassicalPreparationQueue, queue)
	}
	assert.Zero(t, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind='analyze_corpus'`), "no NLP cost")
}

// TestClassicalAnalysisUpdateNamesTheCurrentBook keeps the current-Book
// handling: My Books asks to end the current Reading first and offers the other
// Book its own update.
func TestClassicalAnalysisUpdateNamesTheCurrentBook(t *testing.T) {
	h := newWorkUpdateHarness(t)
	_, err := h.store.StartCurrentReadingResult(h.ctx, h.learnerID, classical.Language, h.firstID)
	require.NoError(t, err)
	bumpInterpretation(t, h.pool, "oga-agdt-upos/6", h.fixture.unitOneKey, h.fixture.unitTwoKey)

	body := readingGet(t, h.h, "/library", h.cookies)
	require.Contains(t, body, html.EscapeString(analysisUpdateEndReadingFirst))
	require.Contains(t, body, `action="`+readingAnalysisUpdateURL(h.secondID)+`"`)
	require.NotContains(t, body, `action="`+readingAnalysisUpdateURL(h.firstID)+`"`, "the current Book is never queued")
}

func TestClassicalAnalysisUpdateRevalidatesEachBook(t *testing.T) {
	h := newWorkUpdateHarness(t)
	first := h.book(t, h.firstID)
	second := h.book(t, h.secondID)
	bumpInterpretation(t, h.pool, "oga-agdt-upos/6", h.fixture.unitOneKey, h.fixture.unitTwoKey)

	// Another tab changes only the second Book's target after the page was
	// rendered. Its confirmation is stale; the first Book still goes ahead.
	staleOffer := analysisUpdateCandidatesFor(t, h, second)[second.Acquired.Source.ID]
	bumpInterpretation(t, h.pool, "oga-agdt-upos/7", h.fixture.unitTwoKey)

	submitted := submitBookAnalysisUpdate(t, h, first)
	require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())
	assert.Contains(t, submitted.Header().Get("Location"), url.QueryEscape("Analysis update for"))
	stale := perform(t, h.h, http.MethodPost, readingAnalysisUpdateURL(h.secondID), url.Values{
		"csrf_token":      {h.csrf},
		"expected_run_id": {staleOffer.PublishedRunID},
		"expected_target": {staleOffer.Target.Key()},
	}, h.cookies)
	require.Equal(t, http.StatusSeeOther, stale.Code, stale.Body.String())
	assert.Contains(t, stale.Header().Get("Location"), url.QueryEscape(analysisUpdateChangedMessage))

	_, bookIDs := workPreparationJobs(t, h.pool)
	require.NotEmpty(t, bookIDs)
	assert.Equal(t, h.firstID, bookIDs[len(bookIDs)-1], "the changed Book is not queued and the other goes ahead")

	// The stale Book keeps its published analysis and shows the refreshed target.
	var version string
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT interpretation_version FROM reading_units WHERE definition_key=$1`, h.fixture.unitTwoKey).Scan(&version))
	assert.Equal(t, "oga-agdt-upos/7", version)
	refreshed := analysisUpdateCandidatesFor(t, h, second)[second.Acquired.Source.ID]
	assert.Contains(t, refreshed.Target.Key(), "oga-agdt-upos/7")
}

func TestClassicalAnalysisUpdatePublishesAndRefreshesPreparedDefinition(t *testing.T) {
	ctx, store, pool, h, service, client, requests, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "work-analysis-update-publish", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalTestUnits(t, h, cookies, csrf, "synthetic-work-1", "synthetic-work-2")
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
	// Three decisions on the old run: a correction and an exclusion on spans no
	// token holds (they cannot carry), then an exclusion on a real Greek token
	// (it carries). Expect 1 of 3 carried.
	addLemmaDecisions(t, pool, learner.ID, secondID, oldRun, 1, 1)
	var corpusID, documentID string
	var start, end int
	require.NoError(t, pool.QueryRow(ctx, `SELECT r.corpus_id::text, s.unit_id, t.start_offset, t.end_offset FROM analysis_runs r
		JOIN corpus_sentences s ON s.owner_id=r.owner_id AND s.analysis_run_id=r.id
		JOIN corpus_tokens t ON t.owner_id=s.owner_id AND t.analysis_run_id=s.analysis_run_id AND t.sentence_ordinal=s.sentence_ordinal
		WHERE r.owner_id=$1 AND r.id=$2 ORDER BY s.sentence_ordinal, t.token_ordinal LIMIT 1`, learner.ID, oldRun).Scan(&corpusID, &documentID, &start, &end))
	_, err := pool.Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,excluded)
		VALUES($1,$2,$3,$4,$5,$6,$7,true)`, learner.ID, secondID, corpusID, oldRun, documentID, start, end)
	require.NoError(t, err)
	_, err = store.PutKnownVocabularyIdentity(ctx, learner.ID, classical.Language, "λέγω", "VERB", "morpheus:1")
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
	for _, book := range []domain.MyBook{first, second} {
		submitted := submitBookAnalysisUpdate(t, harness, book)
		require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())
	}

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
	assert.Equal(t, 3, countRows(t, pool, `SELECT count(*) FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, learner.ID, secondID, oldRun), "the earlier decisions stay frozen")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND excluded AND source_document_id=$4 AND start_offset=$5`, learner.ID, secondID, newRun, documentID, start),
		"only the exclusion on the identical token carries onto the new analysis")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM analysis_decision_carry_overs WHERE owner_id=$1 AND analysis_run_id=$2 AND carried=1 AND total=3`, learner.ID, newRun))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM analysis_decision_carry_overs WHERE owner_id=$1 AND analysis_run_id=$2 AND carried=0 AND total=0`, learner.ID, preparedRunID(t, pool, learner.ID, firstID)),
		"a Book with no decisions still records 0 of 0")
	jobs := readingGet(t, h, "/jobs", cookies)
	assert.Equal(t, 1, strings.Count(jobs, "1 of 3 decisions carried over"), "the publishing job's row reports the carry-over")
	assert.NotContains(t, jobs, "0 of 0 decisions", "a row with no decisions shows no line")

	// Publication commits before the prepared-definition write, so a job that
	// fails after publishing and is retried or discarded still published the run.
	// Its attempt timestamps move on, yet the carry-over stays on that job.
	_, err = pool.Exec(ctx, `UPDATE river_job SET state='discarded', attempted_at=now()+interval '1 hour', finalized_at=now()+interval '2 hours'
		WHERE kind='classical_preparation' AND args->>'book_id'=$1`, secondID)
	require.NoError(t, err)
	jobs = readingGet(t, h, "/jobs", cookies)
	assert.Equal(t, 1, strings.Count(jobs, "1 of 3 decisions carried over"), "a job that failed after publishing keeps its carry-over line")
	assert.Contains(t, testutil.LegacyKnownLemmas(t, ctx, store.Pool(), learner.ID, classical.Language), "λέγω", "Known vocabulary is unchanged by an Analysis update")
}

func TestClassicalAnalysisUpdateIgnoresCuratorialAndTranslationRevisions(t *testing.T) {
	h := newWorkUpdateHarness(t)

	body := readingGet(t, h.h, "/library", h.cookies)
	assert.NotContains(t, body, "A newer analysis is available.", "a prepared Book starts current")

	// A confirmation replayed after the offer disappeared queues nothing and
	// returns the refreshed state.
	stale := perform(t, h.h, http.MethodPost, readingAnalysisUpdateURL(h.firstID), url.Values{
		"csrf_token":      {h.csrf},
		"expected_run_id": {"stale-run"},
		"expected_target": {"stale-target"},
	}, h.cookies)
	require.Equal(t, http.StatusSeeOther, stale.Code, stale.Body.String())
	assert.Contains(t, stale.Header().Get("Location"), url.QueryEscape(analysisUpdateChangedMessage))

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
	// exactly one Book is offered an update.
	_, err = h.pool.Exec(context.Background(), `UPDATE reading_units
		SET source_file_sha256=$1, revision=revision+1 WHERE definition_key=$2`, strings.Repeat("d", 64), h.fixture.unitTwoKey)
	require.NoError(t, err)
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.Equal(t, 1, strings.Count(body, analysisUpdateNotice))
	assert.Contains(t, body, `action="`+readingAnalysisUpdateURL(h.secondID)+`"`)

	// A change to the other unit as well offers both Books.
	_, err = h.pool.Exec(context.Background(), `UPDATE reading_units
		SET source_file_sha256=$1, revision=revision+1 WHERE definition_key=$2`, strings.Repeat("e", 64), h.fixture.unitOneKey)
	require.NoError(t, err)
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.Equal(t, 2, strings.Count(body, analysisUpdateNotice))

	// A Read Book is not offered. The second Book is still To Read, so one offer
	// remains.
	require.NoError(t, h.store.MarkBookPreviouslyRead(context.Background(), h.learnerID, h.firstID))
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.Equal(t, 1, strings.Count(body, analysisUpdateNotice))

	// With both Books outside To Read there is no offer.
	second, err := h.store.GetBookDetail(context.Background(), h.learnerID, h.secondID)
	require.NoError(t, err)
	_, err = h.store.ImportPreviouslyRead(context.Background(), h.learnerID, h.secondID)
	require.NoError(t, err)
	_, err = h.store.TransitionBookDisposition(context.Background(), h.learnerID, h.secondID, second.DispositionRevision, domain.BookDispositionInbox)
	require.NoError(t, err)
	body = readingGet(t, h.h, "/library", h.cookies)
	assert.NotContains(t, body, "A newer analysis is available.")
}
