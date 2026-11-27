//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func finishedBookState(t *testing.T, ctx context.Context, store *PostgresStore, owner, bookID string) domain.MyBook {
	t.Helper()
	result, err := store.ListMyBooksBrowse(ctx, owner, "", "de", "", false, 0, 50)
	require.NoError(t, err)
	for _, item := range result.Items {
		if item.Book.ID == bookID {
			return item
		}
	}
	require.Failf(t, "Book missing from My Books", "book %s", bookID)
	return domain.MyBook{}
}

func TestFinishProjectsReadHistoryAndReadAgainOnlySetsToRead(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "finish-read-history", false)
	require.NoError(t, err)

	// A Book without any prepared deck.
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Deckless", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	source := putBookSourceInLanguage(t, ctx, store, owner.ID, "de", "reading-deckless", "Deckless", []byte("deckless"), "deckless")
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
	makeAnalyzedToReadBook(t, ctx, store, book, source)

	_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.MyBookBucketCurrentReading, finishedBookState(t, ctx, store, owner.ID, book.ID).WorkflowBucket())

	_, err = store.FinishCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.FinishCurrentReading(ctx, owner.ID, "de")
	require.ErrorIs(t, err, ErrNotFound, "a second Finish has no reading to finish and appends nothing")

	state := finishedBookState(t, ctx, store, owner.ID, book.ID)
	assert.Equal(t, domain.BookDispositionInbox, state.Disposition)
	assert.Equal(t, domain.MyBookBucketRead, state.WorkflowBucket())
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, current.BookID, "Finish must not select a next Book")

	// Read again requests To Read only: no reading starts, history stays, and
	// the released snapshot is not adopted.
	applied, err := store.TransitionBookDisposition(ctx, owner.ID, book.ID, state.DispositionRevision, domain.BookDispositionToRead)
	require.NoError(t, err)
	assert.True(t, applied)
	again := finishedBookState(t, ctx, store, owner.ID, book.ID)
	assert.Equal(t, domain.MyBookBucketToRead, again.WorkflowBucket())
	assert.Equal(t, 1, again.CompletionCount)
	current, err = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, current.BookID, "Read again must not start a reading")
	var active int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND released_at IS NULL`, owner.ID).Scan(&active))
	assert.Zero(t, active, "Read again adopted a released snapshot")
	var history int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&history))
	assert.Equal(t, 1, history)

	// A previously-read assertion changes neither disposition nor history and
	// creates no Known vocabulary.
	asserted, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Asserted", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	testutil.SeedLegacyInboxDisposition(t, ctx, store.Pool(), owner.ID, asserted.ID)
	knownBefore, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	for range 2 {
		_, err = store.ImportPreviouslyRead(ctx, owner.ID, asserted.ID)
		require.NoError(t, err)
	}
	assertedState := finishedBookState(t, ctx, store, owner.ID, asserted.ID)
	assert.Equal(t, domain.BookDispositionInbox, assertedState.Disposition)
	assert.Equal(t, domain.MyBookBucketRead, assertedState.WorkflowBucket())
	assert.Equal(t, 1, assertedState.CompletionCount)
	knownAfter, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, knownAfter, len(knownBefore))
}
