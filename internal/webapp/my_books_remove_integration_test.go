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

// workPreparationJobs returns queued classical preparation jobs in insertion order.
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

func TestMyBooksRemoveBookEndsMembershipAndRefusesTheCurrentBook(t *testing.T) {
	ctx, store, pool, h, _ := classicalAddHarness(t)
	learner := createAccount(t, ctx, store, "remove-book", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalTestUnits(t, h, cookies, csrf, "homer-iliad", "xenophon-hellenica")

	ids := ownerBooks(t, pool, learner.ID)
	require.Len(t, ids, 2)
	currentID, otherID := booksByUnitKey(t, ctx, store, learner.ID, "homer-iliad"), booksByUnitKey(t, ctx, store, learner.ID, "xenophon-hellenica")
	currentReadingBook(t, pool, learner.ID, currentID)
	other, err := store.GetBookDetail(ctx, learner.ID, otherID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, other.Disposition)
	require.NotNil(t, other.ReadingUnit)

	removed := perform(t, h, http.MethodPost, "/library/books/"+otherID+"/remove", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, removed.Code, removed.Body.String())
	assert.Contains(t, removed.Header().Get("Location"), url.QueryEscape("Removed from Reading."))
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
