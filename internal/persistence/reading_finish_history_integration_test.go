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
	result, err := store.ListMyBooksBrowseWithVisibility(ctx, owner, "", "de", "", false, true, 0, 50)
	require.NoError(t, err)
	for _, item := range result.Items {
		if item.Book.ID == bookID {
			return item
		}
	}
	require.Failf(t, "Book missing from My Books", "book %s", bookID)
	return domain.MyBook{}
}

func TestFinishProjectsReadHistoryWithoutHidingAndReadAgainOnlySetsToRead(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "finish-read-history", false)
	require.NoError(t, err)

	// A Book without any prepared deck, which the learner has hidden.
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Deckless", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	source := putBookSourceInLanguage(t, ctx, store, owner.ID, "de", "reading-deckless", "Deckless", []byte("deckless"), "deckless")
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	applied, err := store.SetBookHidden(ctx, owner.ID, book.ID, 0, true)
	require.NoError(t, err)
	require.True(t, applied)

	goal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.MyBookBucketCurrentReading, finishedBookState(t, ctx, store, owner.ID, book.ID).WorkflowBucket())

	first, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	replay, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, first.Completion, replay.Completion)

	state := finishedBookState(t, ctx, store, owner.ID, book.ID)
	assert.Equal(t, domain.BookDispositionInbox, state.Disposition)
	assert.Equal(t, domain.MyBookBucketRead, state.WorkflowBucket())
	assert.True(t, state.Hidden, "Finish must not change visibility")
	hidden, revision, err := store.GetBookVisibility(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.True(t, hidden)
	assert.Equal(t, int64(1), revision)
	current, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, current.BookID, "Finish must not select a next Book")

	// Read again requests To Read only: no reading starts, history and
	// visibility stay, and the released snapshot is not adopted.
	applied, err = store.TransitionBookDisposition(ctx, owner.ID, book.ID, state.DispositionRevision, domain.BookDispositionToRead)
	require.NoError(t, err)
	assert.True(t, applied)
	again := finishedBookState(t, ctx, store, owner.ID, book.ID)
	assert.Equal(t, domain.MyBookBucketToRead, again.WorkflowBucket())
	assert.Equal(t, 1, again.CompletionCount)
	assert.True(t, again.Hidden)
	current, err = store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, current.BookID, "Read again must not start a reading")
	var active int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND released_at IS NULL`, owner.ID).Scan(&active))
	assert.Zero(t, active, "Read again adopted a released snapshot")
	var history int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&history))
	assert.Equal(t, 1, history)

	// A previously-read assertion changes neither disposition nor visibility
	// and creates no Known vocabulary.
	asserted, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Asserted", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	knownBefore, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	for range 2 {
		_, err = store.ImportPreviouslyRead(ctx, owner.ID, asserted.ID)
		require.NoError(t, err)
	}
	assertedState := finishedBookState(t, ctx, store, owner.ID, asserted.ID)
	assert.Equal(t, domain.BookDispositionInbox, assertedState.Disposition)
	assert.Equal(t, domain.MyBookBucketRead, assertedState.WorkflowBucket())
	assert.False(t, assertedState.Hidden)
	assert.Equal(t, 1, assertedState.CompletionCount)
	knownAfter, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, knownAfter, len(knownBefore))
}
