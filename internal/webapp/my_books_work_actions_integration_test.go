//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// workPreparationJobs returns the queued classical preparation jobs in
// insertion order, which is the order the bulk action must submit them.
func workPreparationJobs(t *testing.T, pool *pgxpool.Pool) (queues, bookIDs []string) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT queue::text, args->>'book_id' FROM river_job WHERE kind=$1 ORDER BY id`, analysis.ClassicalPreparationQueue)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var queue, bookID string
		require.NoError(t, rows.Scan(&queue, &bookID))
		queues = append(queues, queue)
		bookIDs = append(bookIDs, bookID)
	}
	require.NoError(t, rows.Err())
	return queues, bookIDs
}

func currentReadingBook(t *testing.T, pool *pgxpool.Pool, owner, bookID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `INSERT INTO primary_goals(owner_id, book_id, language) VALUES ($1, $2, $3)`, owner, bookID, classical.Language)
	require.NoError(t, err)
}

func TestMyBooksWorkToReadQueuesEveryBookInBookOrder(t *testing.T) {
	ctx, store, pool, h, _, _, _, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "work-to-read-order", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "synthetic-work")

	owned := booksByReadingUnit(t, store, learner.ID)
	firstID, secondID := owned[fixture.unitOneKey], owned[fixture.unitTwoKey]
	require.NotEmpty(t, firstID)
	require.NotEmpty(t, secondID)

	moved := perform(t, h, http.MethodPost, "/library/works/synthetic-work/to-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

	for _, id := range []string{firstID, secondID} {
		detail, err := store.GetBookDetail(ctx, learner.ID, id)
		require.NoError(t, err)
		assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
	}

	queues, bookIDs := workPreparationJobs(t, pool)
	require.Equal(t, []string{firstID, secondID}, bookIDs, "jobs are enqueued in constituent-book order")
	for _, queue := range queues {
		assert.Equal(t, analysis.ClassicalPreparationQueue, queue)
	}
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM river_job WHERE queue='analysis'`), "a Work move never uses the shared analysis queue")
}

func TestMyBooksWorkToReadSkipsReadingHistoryAndTheCurrentBook(t *testing.T) {
	ctx, store, pool, h, _, _, _, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "work-to-read-skip", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "synthetic-work")

	owned := booksByReadingUnit(t, store, learner.ID)
	firstID, secondID := owned[fixture.unitOneKey], owned[fixture.unitTwoKey]
	require.NotEmpty(t, firstID)
	require.NotEmpty(t, secondID)

	// Book 2 has reading history and Book 1 is the current Reading, so neither
	// may move. Book 1 already is To Read; Book 2 must keep its completion.
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, firstID, domain.BookDispositionToRead))
	currentReadingBook(t, pool, learner.ID, firstID)
	_, err := store.ImportPreviouslyRead(ctx, learner.ID, secondID)
	require.NoError(t, err)

	before, err := store.GetBookDetail(ctx, learner.ID, firstID)
	require.NoError(t, err)
	moved := perform(t, h, http.MethodPost, "/library/works/synthetic-work/to-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

	after, err := store.GetBookDetail(ctx, learner.ID, firstID)
	require.NoError(t, err)
	reading, err := store.GetCurrentReading(ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	assert.Equal(t, firstID, reading.BookID, "the current Book stays in Reading")
	assert.Equal(t, domain.BookDispositionToRead, after.Disposition)
	assert.Equal(t, before.DispositionRevision, after.DispositionRevision, "the current Book is not moved")

	history, err := store.GetBookDetail(ctx, learner.ID, secondID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, history.Disposition, "a Book with reading history stays Inbox")
	assert.Equal(t, 1, history.CompletionCount)
	_, bookIDs := workPreparationJobs(t, pool)
	assert.Empty(t, bookIDs, "skipped Books enqueue no preparation")
}

func TestMyBooksWorkToReadMovesTheRestAroundTheCurrentBook(t *testing.T) {
	ctx, store, pool, h, _, _, _, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "work-to-read-rest", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "synthetic-work")

	owned := booksByReadingUnit(t, store, learner.ID)
	firstID, secondID := owned[fixture.unitOneKey], owned[fixture.unitTwoKey]
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, secondID, domain.BookDispositionToRead))
	currentReadingBook(t, pool, learner.ID, secondID)

	moved := perform(t, h, http.MethodPost, "/library/works/synthetic-work/to-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

	first, err := store.GetBookDetail(ctx, learner.ID, firstID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, first.Disposition, "the remaining Book moves to To Read")
	second, err := store.GetBookDetail(ctx, learner.ID, secondID)
	require.NoError(t, err)
	reading, err := store.GetCurrentReading(ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	assert.Equal(t, secondID, reading.BookID)
	assert.Equal(t, domain.BookDispositionToRead, second.Disposition)

	_, bookIDs := workPreparationJobs(t, pool)
	assert.Equal(t, []string{firstID}, bookIDs, "only the moved Book queues preparation")
}

func TestMyBooksWorkBackToInboxLeavesTheCurrentBookToRead(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "work-back-to-inbox", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "homer-iliad")

	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	byPosition := classicalBooksByPosition(t, ctx, store, learner.ID, ids)
	currentID, otherID := byPosition[1].Book.ID, byPosition[2].Book.ID
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, currentID, domain.BookDispositionToRead))
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, otherID, domain.BookDispositionToRead))
	currentReadingBook(t, pool, learner.ID, currentID)

	moved := perform(t, h, http.MethodPost, "/library/works/homer-iliad/inbox", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())

	current, err := store.GetBookDetail(ctx, learner.ID, currentID)
	require.NoError(t, err)
	reading, err := store.GetCurrentReading(ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	assert.Equal(t, currentID, reading.BookID)
	assert.Equal(t, domain.BookDispositionToRead, current.Disposition, "the current Book stays To Read")
	other, err := store.GetBookDetail(ctx, learner.ID, otherID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, other.Disposition)

	// A repeated move changes nothing: the current Book stays and the Inbox
	// Book remains Inbox.
	again := perform(t, h, http.MethodPost, "/library/works/homer-iliad/inbox", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, again.Code, again.Body.String())
	currentAgain, err := store.GetBookDetail(ctx, learner.ID, currentID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, currentAgain.Disposition)
	assert.Equal(t, current.DispositionRevision, currentAgain.DispositionRevision)
	otherAgain, err := store.GetBookDetail(ctx, learner.ID, otherID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, otherAgain.Disposition)
	assert.Equal(t, other.DispositionRevision, otherAgain.DispositionRevision)
}

func TestMyBooksWorkToReadWithoutPreparationServiceStillMovesTheBooks(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "work-to-read-no-prep", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "homer-iliad")

	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	moved := perform(t, h, http.MethodPost, "/library/works/homer-iliad/to-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())
	assert.Contains(t, moved.Header().Get("Location"), url.QueryEscape("Preparation could not start for 2 Books."))
	for _, id := range ids {
		detail, err := store.GetBookDetail(ctx, learner.ID, id)
		require.NoError(t, err)
		assert.Equal(t, domain.BookDispositionToRead, detail.Disposition, "disposition still changes without a preparation service")
	}
}

func TestMyBooksWorkActionsRefuseUnknownWorksAndMissingCSRF(t *testing.T) {
	ctx, store, _, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "work-action-refusals", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "homer-iliad")

	unknown := perform(t, h, http.MethodPost, "/library/works/not-a-work/to-read", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusNotFound, unknown.Code)

	noCSRF := perform(t, h, http.MethodPost, "/library/works/homer-iliad/to-read", url.Values{}, cookies)
	assert.Equal(t, http.StatusForbidden, noCSRF.Code)
}

func TestMyBooksWorkHideAllAndUnhideAllChangeEveryBookAndTheCounts(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "work-hide-unhide", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "homer-iliad")
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))

	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	hidden := perform(t, h, http.MethodPost, "/library/works/homer-iliad/hide", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, hidden.Code, hidden.Body.String())
	for _, id := range ids {
		isHidden, _, err := store.GetBookVisibility(ctx, learner.ID, id)
		require.NoError(t, err)
		assert.True(t, isHidden)
	}

	// Hiding an already Hidden Work is a no-op for every Book.
	hiddenAgain := perform(t, h, http.MethodPost, "/library/works/homer-iliad/hide", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, hiddenAgain.Code, hiddenAgain.Body.String())

	page := perform(t, h, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	body := page.Body.String()
	assert.Contains(t, body, "2 hidden books are not shown.")
	assert.Contains(t, body, "Show hidden books (2)")
	assert.Contains(t, body, "All (0)")
	assert.NotContains(t, body, `id="work-row-homer-iliad"`)

	shown := perform(t, h, http.MethodGet, "/library?show-hidden", nil, cookies)
	require.Equal(t, http.StatusOK, shown.Code, shown.Body.String())
	assert.Contains(t, shown.Body.String(), `id="work-row-homer-iliad"`)
	assert.Contains(t, shown.Body.String(), "Unhide all 2 books of Iliad")

	unhidden := perform(t, h, http.MethodPost, "/library/works/homer-iliad/unhide", url.Values{
		"csrf_token": {csrf},
		"return_to":  {"/library?show-hidden"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, unhidden.Code, unhidden.Body.String())
	assert.Contains(t, unhidden.Header().Get("Location"), "show-hidden")
	for _, id := range ids {
		isHidden, _, err := store.GetBookVisibility(ctx, learner.ID, id)
		require.NoError(t, err)
		assert.False(t, isHidden)
	}

	after := perform(t, h, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, after.Code)
	assert.Contains(t, after.Body.String(), `id="work-row-homer-iliad"`)
	assert.Contains(t, after.Body.String(), "All (2)")
}
