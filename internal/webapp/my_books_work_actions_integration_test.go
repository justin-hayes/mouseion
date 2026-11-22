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
	"github.com/justin-hayes/mouseion/internal/persistence"
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

// addClassicalWorkAsRead restores the pre-Add baseline these bulk-action tests
// exercise: the Books are owned and Read, so the Work-level Move to To Read has
// candidates again, and no preparation is queued. Catalog Add itself queues
// preparation.
func addClassicalWorkAsRead(t *testing.T, store *persistence.PostgresStore, pool *pgxpool.Pool, owner string, h *Handler, cookies []*http.Cookie, csrf, workKey string) {
	t.Helper()
	addClassicalWork(t, h, cookies, csrf, workKey)
	for _, id := range ownerBooks(t, pool, owner) {
		require.NoError(t, store.MarkBookPreviouslyRead(context.Background(), owner, id))
	}
	_, err := pool.Exec(context.Background(), `DELETE FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue)
	require.NoError(t, err)
}

func TestMyBooksWorkToReadQueuesEveryBookInBookOrder(t *testing.T) {
	ctx, store, pool, h, _, _, _, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "work-to-read-order", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWorkAsRead(t, store, pool, learner.ID, h, cookies, csrf, "synthetic-work")

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

func TestMyBooksWorkToReadSkipsTheCurrentBook(t *testing.T) {
	ctx, store, pool, h, _, _, _, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "work-to-read-skip", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWorkAsRead(t, store, pool, learner.ID, h, cookies, csrf, "synthetic-work")

	owned := booksByReadingUnit(t, store, learner.ID)
	firstID, secondID := owned[fixture.unitOneKey], owned[fixture.unitTwoKey]
	require.NotEmpty(t, firstID)
	require.NotEmpty(t, secondID)

	// Book 1 is the current Reading and may not move. Book 2 has reading
	// history, so the Work move makes it a candidate again (Read again).
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, firstID, domain.BookDispositionToRead))
	currentReadingBook(t, pool, learner.ID, firstID)

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
	assert.Equal(t, domain.BookDispositionToRead, history.Disposition, "a Read Book becomes a candidate again")
	assert.Equal(t, 1, history.CompletionCount, "the completion is unchanged")
	_, bookIDs := workPreparationJobs(t, pool)
	assert.Equal(t, []string{secondID}, bookIDs, "only the skipped current Book enqueues no preparation")
}

func TestMyBooksWorkToReadMovesTheRestAroundTheCurrentBook(t *testing.T) {
	ctx, store, pool, h, _, _, _, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "work-to-read-rest", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWorkAsRead(t, store, pool, learner.ID, h, cookies, csrf, "synthetic-work")

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

func TestMyBooksRemoveBookEndsMembershipAndRefusesTheCurrentBook(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "remove-book", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "homer-iliad")

	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	byPosition := classicalBooksByPosition(t, ctx, store, learner.ID, ids)
	currentID, otherID := byPosition[1].Book.ID, byPosition[2].Book.ID
	currentReadingBook(t, pool, learner.ID, currentID)
	other, err := store.GetBookDetail(ctx, learner.ID, otherID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, other.Disposition)
	require.NotNil(t, other.ReadingUnit)

	removed := perform(t, h, http.MethodPost, "/library/books/"+otherID+"/remove", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, removed.Code, removed.Body.String())
	assert.Contains(t, removed.Header().Get("Location"), url.QueryEscape("Removed from My Books."))
	_, err = store.GetBookDetail(ctx, learner.ID, otherID)
	require.ErrorIs(t, err, domain.ErrNotFound, "Remove ends membership")

	restored := perform(t, h, http.MethodPost, "/catalogs/classical/units/"+other.ReadingUnit.ReadingUnitKey+"/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, restored.Code, restored.Body.String())
	afterRestore, err := store.GetBookDetail(ctx, learner.ID, otherID)
	require.NoError(t, err)
	assert.Equal(t, otherID, afterRestore.Book.ID, "Add restores the same Book")
	assert.Equal(t, domain.BookDispositionToRead, afterRestore.Disposition)

	// The current Book cannot be removed by any route.
	refused := perform(t, h, http.MethodPost, "/library/books/"+currentID+"/remove", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, refused.Code, refused.Body.String())
	assert.Contains(t, refused.Header().Get("Location"), url.QueryEscape("This Book is your current reading."))
	current, err := store.GetBookDetail(ctx, learner.ID, currentID)
	require.NoError(t, err)
	reading, err := store.GetCurrentReading(ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	assert.Equal(t, currentID, reading.BookID, "the refused Remove never ends the reading")
	assert.Equal(t, domain.BookDispositionToRead, current.Disposition)
}

func TestMyBooksRemoveTheWorkSkipsAndNamesTheCurrentBook(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "remove-work", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "homer-iliad")

	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	byPosition := classicalBooksByPosition(t, ctx, store, learner.ID, ids)
	currentID, otherID := byPosition[1].Book.ID, byPosition[2].Book.ID
	currentReadingBook(t, pool, learner.ID, currentID)

	removed := perform(t, h, http.MethodPost, "/library/works/homer-iliad/remove", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, removed.Code, removed.Body.String())
	location := removed.Header().Get("Location")
	assert.Contains(t, location, url.QueryEscape("Removed 1 Book from My Books."))
	assert.Contains(t, location, url.QueryEscape("“Iliad, Book 1” stays because it is your current reading."))

	current, err := store.GetBookDetail(ctx, learner.ID, currentID)
	require.NoError(t, err)
	reading, err := store.GetCurrentReading(ctx, learner.ID, classical.Language)
	require.NoError(t, err)
	assert.Equal(t, currentID, reading.BookID)
	assert.Equal(t, domain.BookDispositionToRead, current.Disposition)
	_, err = store.GetBookDetail(ctx, learner.ID, otherID)
	require.ErrorIs(t, err, domain.ErrNotFound, "the Work's other Book leaves My Books")

	page := perform(t, h, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	assert.NotContains(t, page.Body.String(), "Move the work back to Inbox")
	assert.NotContains(t, page.Body.String(), `action="/library/works/homer-iliad/inbox"`)
	retired := perform(t, h, http.MethodPost, "/library/works/homer-iliad/inbox", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusNotFound, retired.Code, "the retired Work Inbox route is unsupported")
}

func TestMyBooksWorkToReadNamesTheLegacyRowsCause(t *testing.T) {
	ctx, store, pool, h, _, _, _, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "work-to-read-legacy-rows", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWorkAsRead(t, store, pool, learner.ID, h, cookies, csrf, "synthetic-work")

	owned := booksByReadingUnit(t, store, learner.ID)
	firstID, secondID := owned[fixture.unitOneKey], owned[fixture.unitTwoKey]
	require.NotEmpty(t, firstID)
	require.NotEmpty(t, secondID)

	// One unconverted Ancient Greek row blocks preparation installation-wide.
	_, err := pool.Exec(ctx, `INSERT INTO books(owner_id, title, metadata_provenance, language_state, language_tag)
		VALUES ($1, 'Legacy Greek', 'catalog_sync', 'chosen', $2)`, learner.ID, classical.Language)
	require.NoError(t, err)

	moved := perform(t, h, http.MethodPost, "/library/works/synthetic-work/to-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())
	location := moved.Header().Get("Location")
	require.Contains(t, location, url.QueryEscape("Moved 2 Books to To Read. 2 Books cannot be prepared while this installation converts its legacy Ancient Greek rows."))
	require.NotContains(t, location, url.QueryEscape("Preparation could not start"))

	for _, id := range []string{firstID, secondID} {
		detail, err := store.GetBookDetail(ctx, learner.ID, id)
		require.NoError(t, err)
		assert.Equal(t, domain.BookDispositionToRead, detail.Disposition, "the Book still moves when preparation cannot start")
	}
	_, bookIDs := workPreparationJobs(t, pool)
	assert.Empty(t, bookIDs, "a legacy-row refusal submits no preparation")
}

func TestMyBooksWorkToReadNamesTheWithdrawnDefinitionCause(t *testing.T) {
	ctx, store, pool, h, _, _, _, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "work-to-read-withdrawn", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWorkAsRead(t, store, pool, learner.ID, h, cookies, csrf, "synthetic-work")

	owned := booksByReadingUnit(t, store, learner.ID)
	firstID, secondID := owned[fixture.unitOneKey], owned[fixture.unitTwoKey]
	require.NotEmpty(t, firstID)
	require.NotEmpty(t, secondID)

	fixture.definitions[1].Withdrawn = true
	require.NoError(t, store.ReconcileClassicalManifest(ctx, fixture.definitions))

	moved := perform(t, h, http.MethodPost, "/library/works/synthetic-work/to-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())
	location := moved.Header().Get("Location")
	require.Contains(t, location, url.QueryEscape("Moved 2 Books to To Read. 1 Book has a withdrawn definition and cannot be prepared."))
	require.NotContains(t, location, url.QueryEscape("Preparation could not start"))

	for _, id := range []string{firstID, secondID} {
		detail, err := store.GetBookDetail(ctx, learner.ID, id)
		require.NoError(t, err)
		assert.Equal(t, domain.BookDispositionToRead, detail.Disposition, "the Book still moves when its definition is withdrawn")
	}
	_, bookIDs := workPreparationJobs(t, pool)
	assert.Equal(t, []string{firstID}, bookIDs, "only the current definition queues preparation")
}

func TestMyBooksWorkToReadWithoutPreparationServiceStillMovesTheBooks(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "work-to-read-no-prep", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWorkAsRead(t, store, pool, learner.ID, h, cookies, csrf, "homer-iliad")

	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	h.services.ClassicalPreparation = nil
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

	unknownUpdate := perform(t, h, http.MethodPost, "/library/works/not-a-work/analysis-update", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusNotFound, unknownUpdate.Code)

	noCSRFUpdate := perform(t, h, http.MethodPost, "/library/works/homer-iliad/analysis-update", url.Values{}, cookies)
	assert.Equal(t, http.StatusForbidden, noCSRFUpdate.Code)

	noConfirmedBooks := perform(t, h, http.MethodPost, "/library/works/homer-iliad/analysis-update", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, noConfirmedBooks.Code)
	assert.Contains(t, noConfirmedBooks.Header().Get("Location"), url.QueryEscape(analysisUpdateChangedMessage))
}
