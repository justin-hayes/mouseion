//go:build integration

package webapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassicalCatalogCompleteLoop walks the whole Classical catalog flow
// through the real HTTP routes, the real PostgreSQL store and the real
// in-process River workers, with no NLP service configured.
//
// Scenario map (spec #2401, Testing Decisions). The loop directly covers the
// main path of scenarios 1, 2, 5, 6, 7, 9, 10, 11, 13 and 14 and the grouped
// surfaces of 3 and 8. Detailed slice coverage stays in these tests, which
// this loop references rather than duplicates:
//
//   - scenario 3 search, work hide/unhide and compact markup:
//     TestMyBooksWorkSearchMatchesAuthorEnglishAndGreekTitlesAndNarrowsByBook,
//     TestMyBooksWorkHideAllAndUnhideAllChangeEveryBookAndTheCounts
//   - scenario 4 history and mixed-state bulk moves:
//     TestMyBooksWorkToReadSkipsReadingHistoryAndTheCurrentBook,
//     TestMyBooksWorkToReadMovesTheRestAroundTheCurrentBook
//   - scenario 5 terminal failures and newer-revision retry:
//     classical_preparation_failure_integration_test.go
//   - scenario 7 failed translation import: TestClassicalTranslationFailureLeavesBookAnalyzed
//   - scenario 8 band order and omitted empty rows:
//     TestReadingChooserGroupsAWorkAndOrdersBands
//   - scenario 12 installation-wide translation migration:
//     TestReferenceTranslationMigrationCopiesAndDeduplicates; frozen
//     pre-upgrade evidence: TestConvertLegacyAnabasisBookPreservesEveryPieceOfState
//   - scenario 15 unchanged modern-language outcomes:
//     TestCompleteLearnerLoopFromOnboardingToCurrentReadingCompletion (German),
//     TestReadingChooserModernLanguageCandidatesStayOrdinary (de/it/fr/el),
//     internal/cataloguesync sync integration tests, and the prepared-deck
//     integration tests. This change adds no production code, so those suites
//     remain the regression gate.
//
// The detailed failure matrix (pin mismatch, oversize, adapter refusal, newer
// revision, withdrawn definitions) lives in classical_preparation_failure_integration_test.go;
// this loop exercises the retryable fetch failure and recovery.
func TestClassicalCatalogCompleteLoop(t *testing.T) {
	h := newClassicalLoopHarness(t)

	learner := createAccount(t, h.ctx, h.store, "classical-loop-learner", "learner-password")
	cookies, csrf := loginCookies(t, h.handler, learner.Username, "learner-password")

	// 1. Catalogs -> author index -> author -> work.
	index := perform(t, h.handler, http.MethodGet, "/catalogs/classical", nil, cookies)
	require.Equal(t, http.StatusOK, index.Code, index.Body.String())
	for _, want := range []string{"Classical catalog", "Synthetic Author", "Synthetic Work", "Synthetic Whole"} {
		assert.Contains(t, index.Body.String(), want, "author index missing %q", want)
	}
	author := perform(t, h.handler, http.MethodGet, "/catalogs/classical/authors/"+url.PathEscape("Synthetic Author"), nil, cookies)
	require.Equal(t, http.StatusOK, author.Code, author.Body.String())
	authorBody := author.Body.String()
	for _, want := range []string{
		"Synthetic Work", "Synthetic Whole",
		"English translation available", "English translation not available",
		`action="/catalogs/classical/works/synthetic-whole/add"`,
	} {
		assert.Contains(t, authorBody, want, "author page missing %q", want)
	}
	work := perform(t, h.handler, http.MethodGet, "/catalogs/classical/works/synthetic-work", nil, cookies)
	require.Equal(t, http.StatusOK, work.Code, work.Body.String())
	for _, want := range []string{
		"Book 1", "Book 2", `action="/catalogs/classical/works/synthetic-work/add"`,
		`action="/catalogs/classical/units/synthetic-work-1/add"`,
	} {
		assert.Contains(t, work.Body.String(), want, "work page missing %q", want)
	}

	// 2. Add a single book from the work page, then the whole work, then the
	// whole-work unit from the author page. Re-adding reopens without a
	// duplicate or a download.
	single := perform(t, h.handler, http.MethodPost, "/catalogs/classical/units/synthetic-work-2/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, single.Code, single.Body.String())
	assert.Contains(t, single.Header().Get("Location"), url.QueryEscape("Added 1 Book to My Books."))
	added := perform(t, h.handler, http.MethodPost, "/catalogs/classical/works/synthetic-work/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	assert.Contains(t, added.Header().Get("Location"), url.QueryEscape("Added 1 Book and opened 1 existing Book."))
	whole := perform(t, h.handler, http.MethodPost, "/catalogs/classical/works/synthetic-whole/add", url.Values{
		"csrf_token": {csrf}, "return": {"author"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, whole.Code, whole.Body.String())
	assert.Contains(t, whole.Header().Get("Location"), "/catalogs/classical/authors/"+url.PathEscape("Synthetic Author"))
	owned := h.booksByUnit(t, learner.ID)
	require.Len(t, owned, 3)
	bookOne, bookTwo := owned[h.unitOneKey], owned[h.unitTwoKey]
	wholeBook := owned[h.wholeUnitKey]
	require.NotEmpty(t, bookOne)
	require.NotEmpty(t, bookTwo)
	require.NotEmpty(t, wholeBook)
	assert.Zero(t, h.sourceServed(), "Add downloads nothing")
	assert.Zero(t, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue), "Add starts no preparation job")
	require.NoError(t, h.store.SetActiveStudyLanguage(h.ctx, learner.ID, classical.Language))
	require.Equal(t, domain.BookDispositionInbox, h.detail(t, learner.ID, bookOne).Disposition)

	// A whole-work unit stays an ordinary row without a position.
	library := perform(t, h.handler, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code, library.Body.String())
	assert.Contains(t, library.Body.String(), "Synthetic Whole")
	assert.NotContains(t, library.Body.String(), "Book 1 of 1")

	// Re-add is idempotent, and reopens the hidden Book without a duplicate.
	_, err := h.store.SetBookHidden(h.ctx, learner.ID, bookOne, 0, true)
	require.NoError(t, err)
	again := perform(t, h.handler, http.MethodPost, "/catalogs/classical/works/synthetic-work/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, again.Code, again.Body.String())
	assert.Contains(t, again.Header().Get("Location"), url.QueryEscape("Opened 2 existing Books."))
	require.Equal(t, owned, h.booksByUnit(t, learner.ID))
	hidden, _, err := h.store.GetBookVisibility(h.ctx, learner.ID, bookOne)
	require.NoError(t, err)
	assert.False(t, hidden, "re-adding reopens the hidden Book")

	// 3. My Books groups the reviewed work: identity, one badge, counts, strip,
	// one legend, Book-counting filters.
	library = perform(t, h.handler, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code, library.Body.String())
	grouped := library.Body.String()
	for _, want := range []string{
		"Synthetic Work", "By Synthetic Author", "Συνθετικὸν ἔργον", "all 2 books", "Classical catalog",
		"Inbox", `aria-label="Synthetic Work, Book 1 of 2, Inbox"`,
		`aria-label="Synthetic Work, Book 2 of 2, Inbox"`,
		`href="/reading#journey-book-` + bookOne + `"`,
		`href="/reading#journey-book-` + bookTwo + `"`,
		"All (3)", "Inbox (3)",
	} {
		assert.Contains(t, grouped, want, "grouped Work row missing %q", want)
	}
	assert.Equal(t, 1, strings.Count(grouped, `class="library-work-legend"`), "one legend per page")
	assert.Contains(t, grouped, "3 Books in 2 works")

	// 4. Bulk To Read first meets a fetch failure, then recovers through the
	// Reading retry route. Jobs are enqueued in constituent-book order.
	h.layer.failSource.Store(true)
	moved := perform(t, h.handler, http.MethodPost, "/library/works/synthetic-work/to-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())
	for _, id := range []string{bookOne, bookTwo} {
		assert.Equal(t, domain.BookDispositionToRead, h.detail(t, learner.ID, id).Disposition)
	}
	queues, bookIDs := h.preparationJobs(t)
	require.Equal(t, []string{bookOne, bookTwo}, bookIDs, "jobs are enqueued in constituent-book order")
	for _, queue := range queues {
		assert.Equal(t, analysis.ClassicalPreparationQueue, queue)
	}
	assert.Zero(t, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE queue='analysis'`), "classical preparation never delays EPUB analysis")

	queuedPage := perform(t, h.handler, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, queuedPage.Code, queuedPage.Body.String())
	assert.Contains(t, queuedPage.Body.String(), "Preparing Books 1–2…", "a grouped Work collapses queued Books into one line")
	assert.NotContains(t, queuedPage.Body.String(), classicalRetryAcquisitionLabel)

	h.startWorkers(t)
	for _, id := range []string{bookOne, bookTwo} {
		record := h.waitForFailure(t, learner.ID, id)
		assert.Equal(t, domain.ClassicalPreparationFailureFetch, record.Stage)
		assert.Equal(t, ancientgreek.CodeSourceDownloadFailed, record.Code)
	}

	// 5. A queued/running Book is Analysis incomplete; a fetch failure keeps its
	// own note and Retry acquisition.
	page := perform(t, h.handler, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	assert.Contains(t, page.Body.String(), classicalAssessmentUnavailable)
	assert.Contains(t, page.Body.String(), classicalRetryAcquisitionLabel)
	jobsPage := perform(t, h.handler, http.MethodGet, "/jobs", nil, cookies)
	require.Equal(t, http.StatusOK, jobsPage.Code, jobsPage.Body.String())
	assert.Contains(t, jobsPage.Body.String(), ancientgreek.CodeSourceDownloadFailed)
	assert.Contains(t, jobsPage.Body.String(), "Classical preparation")
	assert.False(t, containsGreek(jobsPage.Body.String()), "the jobs surface prints stable codes, never Greek")

	// Retrying acquisition re-runs preparation, and the recovered layer then
	// serves the work file once for both Books.
	h.layer.failSource.Store(false)
	for _, id := range []string{bookOne, bookTwo} {
		retried := perform(t, h.handler, http.MethodPost, "/reading/books/"+id+"/reanalyze", url.Values{"csrf_token": {csrf}}, cookies)
		require.Equal(t, http.StatusSeeOther, retried.Code, retried.Body.String())
	}
	for _, id := range []string{bookOne, bookTwo} {
		h.waitForBookAnalysis(t, learner.ID, id)
	}
	assert.Equal(t, int64(1), h.sourceServed(), "two Books and one installation fetch the work file once")
	h.waitForTranslationSections(t, 4)
	assert.Equal(t, int64(1), h.translationServed(), "the translation is fetched once")

	// Each Book stores only its own Reading unit, and the prepared-definition
	// record names the definition it used.
	for _, id := range []string{bookOne, bookTwo} {
		unit, content := h.sourceMaterial(t, learner.ID, id)
		assert.Contains(t, []string{h.unitOneKey, h.unitTwoKey}, unit, "the source identifier is the Reading-unit key")
		assert.Less(t, len(content), len(h.layer.source), "the whole work file is never stored per Book")
		assert.NotEmpty(t, h.preparedDefinitionRunID(t, learner.ID, id))
	}
	assert.Equal(t, int64(1), h.sourceServed(), "the retried failures reused one stored work file")

	// 6. The grouped chooser lists the Work as one entry with a band table and
	// Book-order links; a single-book work stays ordinary.
	require.NoError(t, h.store.SetActiveStudyLanguage(h.ctx, learner.ID, classical.Language))
	chooser := perform(t, h.handler, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, chooser.Code, chooser.Body.String())
	chooserBody := chooser.Body.String()
	for _, want := range []string{
		"Synthetic Work", "Synthetic Author", "Συνθετικὸν ἔργον", "2 To Read Books, 2 assessed",
		"Known coverage", "Book 1 of 2", "Book 2 of 2",
		`href="#reading-chooser-book-` + bookOne + `"`,
		`href="#reading-chooser-book-` + bookTwo + `"`,
	} {
		assert.Contains(t, chooserBody, want, "grouped chooser missing %q", want)
	}
	assert.Contains(t, chooserBody, "Choose a To Read book when you are ready.")

	// 7. Start Book 1, prepare a deck from its frozen snapshot, then switch,
	// end and finish. The deck freezes its evidence and makes no provider call.
	started := perform(t, h.handler, http.MethodPost, "/reading/books/"+bookOne+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code, started.Body.String())
	firstReading, err := h.store.GetCurrentReading(h.ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	require.Equal(t, bookOne, firstReading.BookID)
	readingPage := perform(t, h.handler, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, readingPage.Code)
	assert.Contains(t, readingPage.Body.String(), "Book 1 of 2", "Reading shows the reviewed position")

	prepared := perform(t, h.handler, http.MethodPost, "/reading/books/"+bookOne+"/deck/retry", url.Values{
		"csrf_token": {csrf}, "expected_current_snapshot_id": {firstReading.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, prepared.Code, prepared.Body.String())
	deck := waitForPreparedDeck(t, h.ctx, h.store, learner.ID, firstReading.SnapshotID)
	require.Equal(t, domain.DeckPreparationReady, deck.State, "deck error=%q", deck.Error)
	assert.Positive(t, deck.TotalCards)
	assert.Zero(t, h.provider.callCount(), "a Greek deck makes no provider request")
	deckEvidence := h.deckEvidence(t, learner.ID, deck.ID)

	// Freeze Book 2's deck as well, then return to Book 1 so the update must
	// skip the current Book while proving the updated Book's deck is frozen.
	switchedToSecond := perform(t, h.handler, http.MethodPost, "/reading/books/"+bookTwo+"/switch", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {firstReading.BookID},
		"expected_current_snapshot_id": {firstReading.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, switchedToSecond.Code, switchedToSecond.Body.String())
	secondForDeck, err := h.store.GetCurrentReading(h.ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	require.Equal(t, bookTwo, secondForDeck.BookID)
	preparedSecond := perform(t, h.handler, http.MethodPost, "/reading/books/"+bookTwo+"/deck/retry", url.Values{
		"csrf_token": {csrf}, "expected_current_snapshot_id": {secondForDeck.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, preparedSecond.Code, preparedSecond.Body.String())
	deckTwo := waitForPreparedDeck(t, h.ctx, h.store, learner.ID, secondForDeck.SnapshotID)
	require.Equal(t, domain.DeckPreparationReady, deckTwo.State, "deck error=%q", deckTwo.Error)
	secondDeckEvidence := h.deckEvidence(t, learner.ID, deckTwo.ID)
	switchedBack := perform(t, h.handler, http.MethodPost, "/reading/books/"+bookOne+"/switch", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {secondForDeck.BookID},
		"expected_current_snapshot_id": {secondForDeck.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, switchedBack.Code, switchedBack.Body.String())

	// 8. An Analysis update is offered once per work. The current Book is named
	// and skipped; the other Book updates and keeps the published evidence
	// until its own update publishes.
	// A translation-pin revision is content-bearing for the definition but not
	// for the Analysis update, so it offers nothing.
	_, err = h.pool.Exec(h.ctx, `UPDATE reading_units
		SET translation_citation_end='1.2.18', revision=revision+1 WHERE definition_key=$1`, h.unitOneKey)
	require.NoError(t, err)
	library = perform(t, h.handler, http.MethodGet, "/library", nil, cookies)
	assert.NotContains(t, library.Body.String(), "A newer analysis is available for")

	bumpInterpretation(t, h.pool, "oga-agdt-upos/6", h.unitOneKey, h.unitTwoKey)
	library = perform(t, h.handler, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code, library.Body.String())
	updateBody := library.Body.String()
	for _, want := range []string{
		"A newer analysis is available for 1 book.", "Book 1: End the Reading first",
		"Update 1 book of Synthetic Work", `action="/library/works/synthetic-work/analysis-update"`,
	} {
		assert.Contains(t, updateBody, want, "update offer missing %q", want)
	}
	oneRun := preparedRunID(t, h.pool, learner.ID, bookOne)
	twoRun := preparedRunID(t, h.pool, learner.ID, bookTwo)

	update := workUpdateHarness{
		ctx: h.ctx, store: h.store, pool: h.pool, h: h.handler, updates: h.service,
		learnerID: learner.ID, cookies: cookies, csrf: csrf,
		fixture: classicalPreparationFixture{unitOneKey: h.unitOneKey, unitTwoKey: h.unitTwoKey},
		firstID: bookOne, secondID: bookTwo,
	}
	form := workAnalysisUpdateForm(t, update, h.detail(t, learner.ID, bookTwo))
	require.Len(t, form["book_id"], 1)
	submitted := perform(t, h.handler, http.MethodPost, "/library/works/synthetic-work/analysis-update", form, cookies)
	require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())
	assert.Contains(t, submitted.Header().Get("Location"), url.QueryEscape("Queued analysis updates for 1 book."))

	h.waitForInterpretation(t, learner.ID, "oga-agdt-upos/6", 1)
	assert.Equal(t, oneRun, preparedRunID(t, h.pool, learner.ID, bookOne), "the current Book keeps its published analysis")
	assert.NotEqual(t, twoRun, preparedRunID(t, h.pool, learner.ID, bookTwo), "the eligible Book publishes its update")
	library = perform(t, h.handler, http.MethodGet, "/library", nil, cookies)
	assert.Contains(t, library.Body.String(), "Book 1: End the Reading first")
	assert.Equal(t, deckEvidence, h.deckEvidence(t, learner.ID, deck.ID), "a skipped current Book's deck is untouched")
	assert.Equal(t, secondDeckEvidence, h.deckEvidence(t, learner.ID, deckTwo.ID), "an Analysis update never rewrites the updated Book's frozen deck")

	// 9. Switch to Book 2, end it, start it again and finish it. The completion
	// receipt returns to the neutral chooser and never points at another Book.
	current, err := h.store.GetCurrentReading(h.ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	switched := perform(t, h.handler, http.MethodPost, "/reading/books/"+bookTwo+"/switch", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {current.BookID},
		"expected_current_snapshot_id": {current.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, switched.Code, switched.Body.String())
	secondReading, err := h.store.GetCurrentReading(h.ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	require.Equal(t, bookTwo, secondReading.BookID)
	assert.NotEqual(t, firstReading.SnapshotID, secondReading.SnapshotID)

	ended := perform(t, h.handler, http.MethodPost, "/reading/end", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {bookTwo},
		"expected_current_snapshot_id": {secondReading.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, ended.Code, ended.Body.String())
	assert.Equal(t, domain.BookDispositionToRead, h.detail(t, learner.ID, bookTwo).Disposition)

	restarted := perform(t, h.handler, http.MethodPost, "/reading/books/"+bookTwo+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, restarted.Code, restarted.Body.String())
	finishReading, err := h.store.GetCurrentReading(h.ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	finished := perform(t, h.handler, http.MethodPost, "/reading/finish", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {bookTwo},
		"expected_current_snapshot_id": {finishReading.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusOK, finished.Code)
	assert.Contains(t, finished.Body.String(), "Reading finished")
	assert.Contains(t, finished.Body.String(), `href="/reading">Choose a To Read book</a>`)
	for _, unwanted := range []string{"next Book", "Continue", "Book 1 of 2"} {
		assert.NotContains(t, finished.Body.String(), unwanted, "the completion receipt must not plan the next Book")
	}
	chooser = perform(t, h.handler, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, chooser.Code)
	assert.NotContains(t, chooser.Body.String(), `href="#reading-chooser-book-`+bookTwo+`"`, "a Read Book gets no chooser cell")

	// 10. A second learner reuses both installation-wide stores: no source and
	// no translation is fetched again.
	other := createAccount(t, h.ctx, h.store, "classical-loop-other", "learner-password")
	otherCookies, otherCSRF := loginCookies(t, h.handler, other.Username, "learner-password")
	otherAdd := perform(t, h.handler, http.MethodPost, "/catalogs/classical/works/synthetic-work/add", url.Values{"csrf_token": {otherCSRF}}, otherCookies)
	require.Equal(t, http.StatusSeeOther, otherAdd.Code, otherAdd.Body.String())
	otherMoved := perform(t, h.handler, http.MethodPost, "/library/works/synthetic-work/to-read", url.Values{"csrf_token": {otherCSRF}}, otherCookies)
	require.Equal(t, http.StatusSeeOther, otherMoved.Code, otherMoved.Body.String())
	otherOwned := h.booksByUnit(t, other.ID)
	require.Len(t, otherOwned, 2)
	h.waitForBookAnalysis(t, other.ID, otherOwned[h.unitOneKey])
	h.waitForBookAnalysis(t, other.ID, otherOwned[h.unitTwoKey])
	assert.Equal(t, int64(1), h.sourceServed(), "a second learner reuses the installation-wide work file")
	assert.Equal(t, int64(1), h.translationServed(), "a second learner reuses the installation-wide translation")

	// 11. Startup reconciliation prunes a stored work file no current
	// definition pins.
	replacement := h.replacementDefinition()
	manifest, err := classical.RenderDefinitions([]classical.Definition{replacement})
	require.NoError(t, err)
	require.NoError(t, classical.Reconcile(h.ctx, h.store, manifest))
	assert.Zero(t, countRows(t, h.pool, `SELECT count(*) FROM classical_sources`), "an unpinned stored source is pruned")

	// 12. Withdrawn definitions hide the catalog row, keep prepared Books
	// readable, and refuse a new preparation of an unprepared Book.
	withdrawnDefinitions := make([]classical.Definition, 0, len(h.definitions)+1)
	for _, definition := range h.definitions {
		if definition.WorkKey == "synthetic-work" || definition.WorkKey == "synthetic-whole" {
			definition.Withdrawn = true
		}
		withdrawnDefinitions = append(withdrawnDefinitions, definition)
	}
	withdrawnDefinitions = append(withdrawnDefinitions, replacement)
	for i := range withdrawnDefinitions {
		var revision int
		var interpretation string
		if err := h.pool.QueryRow(h.ctx, `SELECT revision, interpretation_version FROM reading_units WHERE definition_key=$1`,
			withdrawnDefinitions[i].Key).Scan(&revision, &interpretation); err == nil {
			withdrawnDefinitions[i].Revision = revision
			withdrawnDefinitions[i].InterpretationVersion = interpretation
		}
	}
	manifest, err = classical.RenderDefinitions(withdrawnDefinitions)
	require.NoError(t, err)
	require.NoError(t, classical.Reconcile(h.ctx, h.store, manifest))
	withdrawnCatalog, err := classical.NewCatalog(withdrawnDefinitions)
	require.NoError(t, err)
	h.handler.services.Classical = withdrawnCatalog
	index = perform(t, h.handler, http.MethodGet, "/catalogs/classical", nil, cookies)
	require.Equal(t, http.StatusOK, index.Code)
	assert.NotContains(t, index.Body.String(), "Synthetic Work", "a withdrawn Work is hidden from the catalog")

	require.NoError(t, h.store.SetActiveStudyLanguage(h.ctx, learner.ID, classical.Language))
	library = perform(t, h.handler, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code)
	assert.Contains(t, library.Body.String(), "Synthetic Work", "a prepared Book stays in My Books after withdrawal")
	readable := perform(t, h.handler, http.MethodPost, "/reading/books/"+bookOne+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, readable.Code, readable.Body.String())
	withdrawnReading, err := h.store.GetCurrentReading(h.ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	assert.Equal(t, bookOne, withdrawnReading.BookID, "a prepared withdrawn Book stays readable")

	beforeWithdrawnJobs := countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue)
	wholeDetail := h.detail(t, learner.ID, wholeBook)
	refusedWhole := perform(t, h.handler, http.MethodPost, "/library/books/"+wholeBook+"/to-read", url.Values{
		"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(wholeDetail.DispositionRevision, 10)},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, refusedWhole.Code, refusedWhole.Body.String())
	assert.Contains(t, refusedWhole.Header().Get("Location"), url.QueryEscape(classicalWithdrawnLabel))
	assert.Equal(t, beforeWithdrawnJobs, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue))

	// 13. Seeded per-learner legacy rows fail the catalog closed with a stable
	// code while OPDS connection management still works.
	legacy := createAccount(t, h.ctx, h.store, "classical-loop-legacy", "learner-password")
	legacyCookies, legacyCSRF := loginCookies(t, h.handler, legacy.Username, "learner-password")
	_, err = h.pool.Exec(h.ctx, `INSERT INTO books(owner_id, title, metadata_provenance, language_state, language_tag)
		VALUES ($1, 'Legacy Greek', 'catalog_sync', 'chosen', $2)`, legacy.ID, classical.Language)
	require.NoError(t, err)
	blocked := perform(t, h.handler, http.MethodGet, "/catalogs/classical/works/replacement-work", nil, legacyCookies)
	require.Equal(t, http.StatusOK, blocked.Code, blocked.Body.String())
	assert.Contains(t, blocked.Body.String(), classical.LegacyRowsCode)
	assert.Contains(t, blocked.Body.String(), "Add is unavailable")
	refused := perform(t, h.handler, http.MethodPost, "/catalogs/classical/works/replacement-work/add", url.Values{"csrf_token": {legacyCSRF}}, legacyCookies)
	require.Equal(t, http.StatusSeeOther, refused.Code)
	assert.Contains(t, refused.Header().Get("Location"), url.QueryEscape(classical.LegacyRowsCode))
	connection := perform(t, h.handler, http.MethodPost, "/connections", url.Values{
		"csrf_token": {legacyCSRF}, "name": {"Still works"}, "url": {"https://example.invalid/opds"},
	}, legacyCookies)
	require.Equal(t, http.StatusSeeOther, connection.Code)
	jobsPage = perform(t, h.handler, http.MethodGet, "/jobs", nil, legacyCookies)
	assert.Contains(t, jobsPage.Body.String(), classical.LegacyRowsCode)

	// 14. The seeded pre-upgrade Anabasis was converted at startup (before the
	// rows above were seeded): identity, unit-only source and prepared
	// definition are current. Detailed frozen state is asserted by
	// TestConvertLegacyAnabasisBookPreservesEveryPieceOfState.
	seed := h.anabasis
	unit, content := h.sourceMaterial(t, seed.ownerID, seed.bookID)
	assert.Equal(t, "xenophon-anabasis-1", unit, "the converted source is keyed by the Reading-unit key")
	assert.Contains(t, string(content), "λέγει")
	assert.NotContains(t, string(content), "Ἀρταξέρξης", "the converted source holds only Book 1")
	assert.Equal(t, seed.runID, h.preparedDefinitionRunID(t, seed.ownerID, seed.bookID), "the converted Book keeps its published run")

	// 14b. No Greek text reaches job output or the operator log.
	var jobErrors string
	require.NoError(t, h.pool.QueryRow(h.ctx, `
		SELECT COALESCE(string_agg(error_text::text, ' '), '')
		FROM river_job job, unnest(job.errors) AS error_text`).Scan(&jobErrors))
	assert.False(t, containsGreek(jobErrors), "job output must not print Greek")
	assert.False(t, containsGreek(h.logs.String()), "operator logs must not print Greek")
}

// classicalLoopLayer serves synthetic OGA source bytes and an aligned TEI
// translation. Source failures are toggled by the test so the retry path runs
// through the real downloader.
type classicalLoopLayer struct {
	source, translation []byte
	failSource          atomic.Bool
	sourceAttempts      atomic.Int64
	sourceServed        atomic.Int64
	translationServed   atomic.Int64
}

func (l *classicalLoopLayer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "synthetic-translation.xml") {
		l.translationServed.Add(1)
		if _, err := w.Write(l.translation); err != nil {
			return
		}
		return
	}
	l.sourceAttempts.Add(1)
	if l.failSource.Load() {
		http.Error(w, "layer unavailable", http.StatusBadGateway)
		return
	}
	l.sourceServed.Add(1)
	if _, err := w.Write(l.source); err != nil {
		return
	}
}

// classicalLoopSeed is the pre-upgrade Anabasis state conversion must handle.
type classicalLoopSeed struct {
	ownerID, bookID, sourceID, runID string
}

type classicalLoopHarness struct {
	ctx          context.Context
	store        *persistence.PostgresStore
	pool         *pgxpool.Pool
	handler      *Handler
	service      *analysis.Service
	client       *river.Client[pgx.Tx]
	layer        *classicalLoopLayer
	provider     *greekDeckRecordingProvider
	definitions  []classical.Definition
	unitOneKey   string
	unitTwoKey   string
	wholeUnitKey string
	anabasis     classicalLoopSeed
	logs         *lockedBuffer
	translation  *classical.TranslationPin
}

// newClassicalLoopHarness seeds a pre-upgrade Anabasis Book, reconciles a
// fixture manifest through the same sequence cmd/server runs at startup,
// converts the Book, and wires the real web application to in-process workers.
// The workers are not started until the test has observed the queued jobs.
func newClassicalLoopHarness(t *testing.T) *classicalLoopHarness {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "classical-complete-loop-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	logs := &lockedBuffer{}
	previous := log.Writer()
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(previous) })

	source, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-deck.conllu"))
	require.NoError(t, err)
	anabasisSource, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	translation, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "perseus-eng2-section.xml"))
	require.NoError(t, err)
	layer := &classicalLoopLayer{source: source, translation: translation}
	server := httptest.NewServer(layer)
	t.Cleanup(server.Close)

	sum := sha256.Sum256(source)
	pin := classical.SourcePin{
		Layer: "loop-layer", FileName: "synthetic-loop.conllu",
		Size: int64(len(source)), SHA256: hex.EncodeToString(sum[:]), Format: classical.SourceFormatOGACoNLLU,
	}
	anabasisSum := sha256.Sum256(anabasisSource)
	anabasisPin := classical.SourcePin{
		Layer: "loop-layer", FileName: "anabasis.conllu",
		Size: int64(len(anabasisSource)), SHA256: hex.EncodeToString(anabasisSum[:]), Format: classical.SourceFormatOGACoNLLU,
	}
	translationSum := sha256.Sum256(translation)
	translationPin := &classical.TranslationPin{
		Layer: "loop-layer", FileName: "synthetic-translation.xml",
		Size: int64(len(translation)), SHA256: hex.EncodeToString(translationSum[:]),
		CitationStart: "1.1.1", CitationEnd: "1.2.18",
	}
	one, two := 1, 2
	anabasis := classical.Definition{
		Key: "xenophon-anabasis-1", WorkKey: "xenophon-anabasis", WorkTitle: "Anabasis",
		WorkAuthor: "Xenophon", GreekTitle: "Ἀνάβασις", EditionKey: "xenophon-anabasis-marchant-1904",
		EditionLabel: "Marchant 1904", Language: classical.Language, ConstituentBook: &one, ConstituentBooks: 7,
		CitationStart: "1.1.1", CitationEnd: "1.10.19", WordCount: 7, Source: anabasisPin,
		InterpretationVersion: "oga-agdt-upos/5", Revision: 1,
	}
	firstUnit := classical.Definition{
		Key: "synthetic-work-1", WorkKey: "synthetic-work", WorkTitle: "Synthetic Work",
		WorkAuthor: "Synthetic Author", GreekTitle: "Συνθετικὸν ἔργον", EditionKey: "synthetic-work-edition",
		EditionLabel: "Synthetic Edition", Language: classical.Language, ConstituentBook: &one, ConstituentBooks: 2,
		CitationStart: "1.1.1", CitationEnd: "1.1.3", WordCount: 16, Source: pin, Translation: translationPin,
		InterpretationVersion: "oga-agdt-upos/5", Revision: 1,
	}
	secondUnit := firstUnit
	secondUnit.Key = "synthetic-work-2"
	secondUnit.ConstituentBook = &two
	secondUnit.CitationStart = "1.1.4"
	secondUnit.CitationEnd = "1.1.6"
	secondUnit.WordCount = 16
	wholeUnit := classical.Definition{
		Key: "synthetic-whole-1", WorkKey: "synthetic-whole", WorkTitle: "Synthetic Whole",
		WorkAuthor: "Synthetic Author", GreekTitle: "Συνθετικὸν ὅλον", EditionKey: "synthetic-whole-edition",
		EditionLabel: "Synthetic Whole Edition", Language: classical.Language, ConstituentBooks: 1,
		CitationStart: "1.1.1", CitationEnd: "1.1.6", WordCount: 32, Source: pin,
		InterpretationVersion: "oga-agdt-upos/5", Revision: 1,
	}
	definitions := []classical.Definition{anabasis, firstUnit, secondUnit, wholeUnit}

	seed := seedLoopPreUpgradeAnabasis(t, ctx, store, pool, anabasis)
	manifest, err := classical.RenderDefinitions(definitions)
	require.NoError(t, err)
	require.NoError(t, classical.Reconcile(ctx, store, manifest), "startup reconciles the fixture manifest")
	conversion, err := analysis.ConvertLegacyClassicalBooks(ctx, store, definitions)
	require.NoError(t, err)
	require.Equal(t, 1, conversion.Converted, "the seeded pre-upgrade Book converts at startup")

	catalog, err := classical.NewCatalog(definitions)
	require.NoError(t, err)
	layers := classical.Layers{Layers: []classical.Layer{{
		Key: "loop-layer", Commit: "synthetic", BaseURL: server.URL + "/",
		LicenceClass: classical.LicenceClassSourceContent, Restriction: classical.RestrictionShareAlike,
		DeployerRegisterRow: "synthetic complete-loop layer",
	}}}
	config := analysis.ClassicalPreparationConfig{Layers: layers, HTTPClient: server.Client(), TempDir: t.TempDir()}
	workers := river.NewWorkers()
	preparationWorker := analysis.AddClassicalPreparationWorker(workers, store, config)
	analysis.AddClassicalTranslationWorker(workers, store, config)
	presentation := cardexport.NewPresentation(greekDeckLexicon{})
	preparedConfig := prepareddeck.PreparedDeckConfig{
		TranslationMode:        prepareddeck.DefaultTranslationMode,
		StandardMaxConcurrency: 1,
		StandardMaxAttempts:    1,
		StandardRetryBaseDelay: time.Millisecond,
		StandardRetryMaxDelay:  time.Millisecond,
	}
	batchConfig := prepareddeck.BatchConfig{MaxRequests: 1, PollInterval: time.Millisecond}
	metrics := prepareddeck.NewMetricsCollector()
	provider := &greekDeckRecordingProvider{}
	unusedAnalyzer := &analyzertest.Fake{AnalyzeFunc: func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{}, errors.New("the Classical catalog never calls the NLP service")
	}}
	client, err := analysis.NewClientWithPreparedDeckConcurrency(pool, unusedAnalyzer, analyzertest.CapabilityProvider{}, selection.NewService(store), 1, workers)
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "classical complete loop client", client.Stop)
	preparationWorker.Client = client
	prepareddeck.AddPreparedDeckWorker(workers, store, presentation, client, nil, batchConfig, preparedConfig, false)
	prepareddeck.AddStandardTranslationWorkerWithDependencies(workers, store, client, provider, preparedConfig, time.Second, metrics)
	prepareddeck.AddBatchSubmitWorkerWithMetrics(workers, store, client, nil, nil, metrics)
	prepareddeck.AddBatchPollWorker(workers, &prepareddeck.BatchPollWorker{Store: store, Client: client, PollInterval: batchConfig.PollInterval, Metrics: metrics})
	prepareddeck.AddFinalizeWorker(workers, &prepareddeck.DurableFinalizer{Store: store, Renderer: presentation, Metrics: metrics})
	prepareddeck.AddBatchCleanupWorker(workers, &prepareddeck.BatchCleanupWorker{Store: store, Metrics: metrics})
	prepareddeck.AddRecoveryWorkerWithMetrics(workers, store, client, batchConfig.PollInterval, metrics)
	require.NoError(t, prepareddeck.EnsureRecoveryJob(ctx, store, client))

	service := analysis.NewService(store.Pool(), client).WithClassicalPreparationStore(store)
	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth:                 authService,
		WebAuth:              webauth.New(authService, false, time.Hour),
		Store:                storeDependencies(store),
		Analysis:             service,
		AnalysisUpdates:      service,
		AnalysisInsights:     analysisinsights.NewService(store),
		PreparedDeck:         prepareddeck.NewService(store, client),
		Capabilities:         analyzertest.CapabilityProvider{},
		ClassicalPreparation: service,
		Classical:            catalog,
		CatalogueSync:        fixtures.NewCatalogueSync(fixtures.NewStore()),
		SessionLifetime:      time.Hour,
	})
	return &classicalLoopHarness{
		ctx: ctx, store: store, pool: pool, handler: h, service: service, client: client,
		layer: layer, provider: provider, definitions: definitions, translation: translationPin,
		unitOneKey: firstUnit.Key, unitTwoKey: secondUnit.Key, wholeUnitKey: wholeUnit.Key,
		anabasis: seed, logs: logs,
	}
}

func (h *classicalLoopHarness) startWorkers(t *testing.T) {
	t.Helper()
	require.NoError(t, h.client.Start(h.ctx))
}

func (h *classicalLoopHarness) booksByUnit(t *testing.T, owner string) map[string]string {
	t.Helper()
	return booksByReadingUnit(t, h.store, owner)
}

func (h *classicalLoopHarness) detail(t *testing.T, owner, bookID string) domain.MyBook {
	t.Helper()
	book, err := h.store.GetBookDetail(h.ctx, owner, bookID)
	require.NoError(t, err)
	return book
}

func (h *classicalLoopHarness) sourceServed() int64      { return h.layer.sourceServed.Load() }
func (h *classicalLoopHarness) translationServed() int64 { return h.layer.translationServed.Load() }

func (h *classicalLoopHarness) preparationJobs(t *testing.T) (queues, bookIDs []string) {
	t.Helper()
	return workPreparationJobs(t, h.pool)
}

func (h *classicalLoopHarness) sourceMaterial(t *testing.T, owner, bookID string) (string, []byte) {
	t.Helper()
	var identifier string
	var content []byte
	require.NoError(t, h.pool.QueryRow(h.ctx,
		`SELECT sm.source_identifier, COALESCE(r.content, sm.content)
		 FROM source_materials sm
		 LEFT JOIN source_content_revisions r
		   ON r.owner_id = sm.owner_id AND r.revision_id = sm.current_content_revision_id
		 WHERE sm.owner_id=$1 AND sm.book_id=$2`, owner, bookID).Scan(&identifier, &content))
	return identifier, content
}

func (h *classicalLoopHarness) preparedDefinitionRunID(t *testing.T, owner, bookID string) string {
	t.Helper()
	var runID string
	require.NoError(t, h.pool.QueryRow(h.ctx,
		`SELECT analysis_run_id::text FROM classical_prepared_definitions WHERE owner_id=$1 AND book_id=$2`,
		owner, bookID).Scan(&runID))
	return runID
}

func (h *classicalLoopHarness) waitForBookAnalysis(t *testing.T, owner, bookID string) {
	t.Helper()
	testutil.Eventually(t, 60*time.Second, "Book analysed", func() (bool, string) {
		var count int
		if err := h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`,
			owner, bookID).Scan(&count); err != nil {
			return false, err.Error()
		}
		return count == 1, "analysis not published"
	})
}

func (h *classicalLoopHarness) waitForFailure(t *testing.T, owner, bookID string) domain.ClassicalPreparationFailureRecord {
	t.Helper()
	var record domain.ClassicalPreparationFailureRecord
	testutil.Eventually(t, 60*time.Second, "settled preparation failure", func() (bool, string) {
		got, found, err := h.store.GetClassicalPreparationFailure(h.ctx, owner, bookID)
		if err != nil || !found {
			return false, "no failure record"
		}
		var state string
		if err := h.pool.QueryRow(h.ctx, `SELECT state::text FROM river_job
			WHERE kind=$1 AND args->>'book_id'=$2 ORDER BY id DESC LIMIT 1`,
			analysis.ClassicalPreparationQueue, bookID).Scan(&state); err != nil {
			return false, err.Error()
		}
		if state != "discarded" && state != "cancelled" {
			return false, "job still " + state
		}
		record = got
		return true, ""
	})
	return record
}

func (h *classicalLoopHarness) waitForInterpretation(t *testing.T, owner, version string, want int) {
	t.Helper()
	testutil.Eventually(t, 60*time.Second, "analysis update published", func() (bool, string) {
		var count int
		if err := h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM classical_prepared_definitions WHERE owner_id=$1 AND interpretation_version=$2`,
			owner, version).Scan(&count); err != nil {
			return false, err.Error()
		}
		return count == want, "updated definitions not published"
	})
}

func (h *classicalLoopHarness) waitForTranslationSections(t *testing.T, want int) {
	t.Helper()
	pin := h.translation.SHA256
	require.Eventually(t, func() bool {
		var count int
		if err := h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM reference_translations WHERE translation_pin_sha256=$1`, pin).Scan(&count); err != nil {
			return false
		}
		return count == want
	}, 60*time.Second, 50*time.Millisecond, "translation rows did not import")
}

type loopDeckEvidence struct {
	totalCards     int
	manifestDigest string
	items          int
	sentence       string
	passage        string
}

func (h *classicalLoopHarness) deckEvidence(t *testing.T, owner, preparationID string) loopDeckEvidence {
	t.Helper()
	var evidence loopDeckEvidence
	require.NoError(t, h.pool.QueryRow(h.ctx,
		`SELECT dp.total_cards, dpm.manifest_digest
		 FROM deck_preparations dp
		 JOIN deck_preparation_manifests dpm
		   ON dpm.owner_id = dp.owner_id AND dpm.preparation_id = dp.id AND dpm.run_id = dp.current_run_id
		 WHERE dp.owner_id=$1 AND dp.id=$2`, owner, preparationID).Scan(&evidence.totalCards, &evidence.manifestDigest))
	require.NoError(t, h.pool.QueryRow(h.ctx,
		`SELECT count(*), COALESCE(min(source_sentence), ''), COALESCE(min(passage_context), '')
		 FROM deck_preparation_manifest_items WHERE owner_id=$1 AND preparation_id=$2`,
		owner, preparationID).Scan(&evidence.items, &evidence.sentence, &evidence.passage))
	return evidence
}

func (h *classicalLoopHarness) replacementDefinition() classical.Definition {
	return classical.Definition{
		Key: "replacement-work-1", WorkKey: "replacement-work", WorkTitle: "Replacement",
		WorkAuthor: "Synthetic Author", EditionKey: "replacement-edition", EditionLabel: "Replacement Edition",
		Language: classical.Language, ConstituentBooks: 1, CitationStart: "1.1.1", CitationEnd: "1.1.1",
		WordCount: 1, InterpretationVersion: "oga-agdt-upos/5", Revision: 1,
		Source: classical.SourcePin{
			Layer: "loop-layer", FileName: "replacement.conllu", Size: 1,
			SHA256: strings.Repeat("a", 64), Format: classical.SourceFormatOGACoNLLU,
		},
	}
}

// lockedBuffer makes the captured operator log safe to read while workers
// write to it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// seedLoopPreUpgradeAnabasis writes the legacy registration shape and the
// learner state that startup conversion must preserve. It mirrors the analysis
// package's migration-scenario seed through exported store methods.
func seedLoopPreUpgradeAnabasis(t *testing.T, ctx context.Context, store *persistence.PostgresStore, pool *pgxpool.Pool, definition classical.Definition) classicalLoopSeed {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	owner, err := store.CreateUser(ctx, "classical-loop-anabasis", false)
	require.NoError(t, err)
	readingDefinition := domain.ReadingUnitDefinition{
		Key: definition.Key, Language: definition.Language, WorkTitle: definition.WorkTitle,
		WorkAuthor: definition.WorkAuthor, EditionLabel: definition.EditionLabel,
		CitationStart: definition.CitationStart, CitationEnd: definition.CitationEnd,
		SourceFileName: definition.Source.FileName, SourceFileSize: definition.Source.Size,
		SourceFileSHA256: definition.Source.SHA256, InterpretationVersion: definition.InterpretationVersion,
	}
	registration, err := store.RegisterReadingUnitBook(ctx, owner.ID, readingDefinition)
	require.NoError(t, err)
	parsed, err := ancientgreek.ParseOgaSource(data)
	require.NoError(t, err)
	unit, err := parsed.CutUnit(readingDefinition)
	require.NoError(t, err)
	written, err := analysis.NewGreekImportWriter(store).WriteImportedAnalysis(ctx, ancientgreek.ImportWriteRequest{
		OwnerID: owner.ID, BookID: registration.BookID, Definition: readingDefinition,
		SourceBytes: data, FullText: unit.FullText, UnitID: unit.UnitID, UnitTitle: definition.WorkTitle,
		Result: unit.Result, SyntaxState: unit.SyntaxState, AnalyzerName: unit.AnalyzerName,
		AnalyzerVersion: unit.AnalyzerVersion, ConfigIdentity: unit.ConfigIdentity,
	})
	require.NoError(t, err)
	seed := classicalLoopSeed{ownerID: owner.ID, bookID: registration.BookID, runID: written.RunID}
	var oldRevisionID, oldSnapshotID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT sm.id::text, sm.current_content_revision_id::text, sm.current_snapshot_id::text
		FROM source_materials sm WHERE sm.owner_id=$1 AND sm.book_id=$2`, owner.ID, registration.BookID).
		Scan(&seed.sourceID, &oldRevisionID, &oldSnapshotID))
	_, err = pool.Exec(ctx, `UPDATE source_materials SET source_identifier=$3 WHERE owner_id=$1 AND id=$2`,
		owner.ID, seed.sourceID, definition.Source.FileName)
	require.NoError(t, err)

	// A confirmed Lemma correction and an Occurrence exclusion.
	var firstStart, firstEnd, secondStart, secondEnd int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT start_offset, end_offset FROM corpus_tokens
		WHERE owner_id=$1 AND analysis_run_id=$2 AND sentence_ordinal=0 AND token_ordinal=0`,
		owner.ID, written.RunID).Scan(&firstStart, &firstEnd))
	require.NoError(t, pool.QueryRow(ctx, `SELECT start_offset, end_offset FROM corpus_tokens
		WHERE owner_id=$1 AND analysis_run_id=$2 AND sentence_ordinal=0 AND token_ordinal=1`,
		owner.ID, written.RunID).Scan(&secondStart, &secondEnd))
	var corpusID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND analysis_run_id=$2`,
		owner.ID, written.RunID).Scan(&corpusID))
	_, err = pool.Exec(ctx, `INSERT INTO occurrence_lemma_corrections(
		owner_id, book_id, corpus_id, analysis_run_id, source_document_id,
		start_offset, end_offset, canonical_lemma, normalization_profile, normalization_version, excluded)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'λέγω','ancient-greek','1',false),
		       ($1,$2,$3,$4,$5,$8,$9,NULL,NULL,NULL,true)`,
		owner.ID, registration.BookID, corpusID, written.RunID, unit.UnitID,
		firstStart, firstEnd, secondStart, secondEnd)
	require.NoError(t, err)
	_, err = store.PutKnownVocabularyIdentity(ctx, owner.ID, classical.Language, "λέγω", "VERB", "morpheus:1")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO reading_history(owner_id, language, book_id, completed_at)
		VALUES($1,$2,$3,now())`, owner.ID, classical.Language, registration.BookID)
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, owner.ID, classical.Language))
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, registration.BookID, domain.BookDispositionToRead))
	started, err := store.StartCurrentReadingResult(ctx, owner.ID, classical.Language, registration.BookID)
	require.NoError(t, err)
	snapshotID := started.Reading.SnapshotID
	require.NotEmpty(t, snapshotID)

	// A prepared deck whose frozen passage evidence must survive conversion.
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: seed.sourceID, AnalysisRunID: written.RunID,
		SnapshotID: snapshotID, Filename: "anabasis.apkg", DeckName: "Mouseion::grc::Anabasis",
		ContentHash: "sha256:pre-upgrade",
	})
	require.NoError(t, err)
	var deckRunID string
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO deck_preparation_runs(
			owner_id, preparation_id, run_number, state, translation_state,
			external_translation_consent, external_translation_configured,
			manifest_schema_version, retry_policy_version, max_provider_attempts,
			max_batch_generations, batch_max_requests, batch_max_bytes, candidate_count,
			completed_count, completed_at, translation_completed_at)
		VALUES($1,$2,1,'completed','completed',false,false,1,1,1,1,1,1,1,1,now(),now())
		RETURNING id::text`, owner.ID, preparation.ID).Scan(&deckRunID))
	_, err = pool.Exec(ctx, `
		INSERT INTO deck_preparation_manifests(
			owner_id, preparation_id, run_id, schema_version, manifest_digest,
			deck_name, filename, selected_count, accepted_count, omitted_count)
		VALUES($1,$2,$3,1,$4,'Mouseion::grc::Anabasis','anabasis.apkg',1,1,0)`,
		owner.ID, preparation.ID, deckRunID, strings.Repeat("b", 64))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO deck_preparation_manifest_items(
			owner_id, preparation_id, run_id, ordinal, disposition, language, canonical_lemma, upos,
			source_sentence, tested_target, first_encounter, quality_score, quality_reasons,
			render_payload, candidate_digest, target_language, quality_gdex_score,
			passage_context, passage_citation, brownson_pin)
		VALUES($1,$2,$3,0,'accepted','grc','λόγοσ','NOUN','λέγει Κῦρος·','λόγος',1,50,'{}','{}',
		       $4,'en',0,'Cyrus speaks.','1.1.1','f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7')`,
		owner.ID, preparation.ID, deckRunID, strings.Repeat("a", 64))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE deck_preparations SET state='ready', artifact=decode('00','hex'),
		total_cards=1, completed_at=now(), current_run_id=$3 WHERE owner_id=$1 AND id=$2`,
		owner.ID, preparation.ID, deckRunID)
	require.NoError(t, err)
	return seed
}
