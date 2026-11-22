//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackfillChosenBooksSeedsEveryPreChangeState exercises the Inbox
// retirement backfill as a data-only operation: only unread, non-current,
// non-To-Read members leave My Books, and every other learner fact survives.
func TestBackfillChosenBooksSeedsEveryPreChangeState(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "my-books-backfill-owner", false)
	require.NoError(t, err)

	create := func(title, languageState, languageTag string) domain.Book {
		t.Helper()
		book, createErr := store.CreateBook(ctx, domain.Book{
			OwnerID: owner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
			LanguageState: languageState, LanguageTag: languageTag,
		})
		require.NoError(t, createErr)
		// CreateBook makes a To Read Book; the pre-change default was the
		// retired Inbox state, which only a direct write can seed.
		testutil.SeedLegacyInboxDisposition(t, ctx, store.Pool(), owner.ID, book.ID)
		return book
	}
	insertHistory := func(bookID string, completedAt time.Time) {
		t.Helper()
		_, execErr := store.Pool().Exec(ctx, `
			INSERT INTO reading_history(
				owner_id, language, book_id, completed_at, goal_snapshot_id,
				snapshot_vocabulary_count, eligible_vocabulary_count,
				graduated_vocabulary_count, already_known_vocabulary_count, completion_source)
			VALUES ($1, 'de', $2, $3, NULL, 0, 0, 0, 0, 'previously_read_import')`,
			owner.ID, bookID, completedAt)
		require.NoError(t, execErr)
	}
	hide := func(bookID string) {
		t.Helper()
		_, execErr := store.Pool().Exec(ctx, `
			INSERT INTO book_visibility(owner_id, book_id, hidden, revision)
			VALUES ($1, $2, true, 1)`, owner.ID, bookID)
		require.NoError(t, execErr)
	}
	setToRead := func(bookID string) {
		t.Helper()
		require.NoError(t, store.SetBookDisposition(ctx, owner.ID, bookID, domain.BookDispositionToRead))
	}

	inboxUnread := create("Inbox unread", domain.LanguageChosen, "de")
	inboxWithHistory := create("Inbox with history", domain.LanguageChosen, "de")
	insertHistory(inboxWithHistory.ID, time.Now().UTC().Add(-48*time.Hour))
	hiddenInbox := create("Hidden Inbox", domain.LanguageChosen, "de")
	hide(hiddenInbox.ID)
	hiddenToRead := create("Hidden To Read", domain.LanguageChosen, "de")
	setToRead(hiddenToRead.ID)
	hide(hiddenToRead.ID)
	classicalInbox, err := store.CreateBook(ctx, domain.Book{
		OwnerID: owner.ID, Title: "Classical Inbox", MetadataProvenance: domain.MetadataProvenanceOperatorRegistration,
		LanguageState: domain.LanguageChosen, LanguageTag: "grc",
	})
	require.NoError(t, err)
	testutil.SeedLegacyInboxDisposition(t, ctx, store.Pool(), owner.ID, classicalInbox.ID)
	languageLessWithHistory := create("Language-less with history", domain.LanguageUnknown, "")
	insertHistory(languageLessWithHistory.ID, time.Now().UTC().Add(-24*time.Hour))
	languageLessUnread := create("Language-less unread", domain.LanguageUnknown, "")
	current := create("Current reading", domain.LanguageChosen, "de")
	setToRead(current.ID)
	_, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id, book_id, language) VALUES ($1, $2, 'de')`, owner.ID, current.ID)
	require.NoError(t, err)

	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "haus", "NOUN")
	require.NoError(t, err)

	// The gate refuses to remove anything before a connection has synced.
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Backfill catalog", URL: "https://backfill.example/opds"})
	require.NoError(t, err)
	_, err = store.BackfillChosenBooks(ctx)
	require.ErrorIs(t, err, domain.ErrMyBooksBackfillBlocked)
	assert.Equal(t, "active", membershipState(t, ctx, store, owner.ID, inboxUnread.ID), "a blocked run changes nothing")

	require.NoError(t, store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{
		OwnerID: owner.ID, ConnectionID: connection.ID, State: domain.CatalogueSyncSynced, LastSyncedAt: new(time.Now()),
	}))
	// A later failed run does not erase the completed sync, so the gate stays
	// satisfied.
	require.NoError(t, store.SetCatalogueSyncStatus(ctx, domain.CatalogueSyncStatus{
		OwnerID: owner.ID, ConnectionID: connection.ID, State: domain.CatalogueSyncFailed, LastError: "later failure",
	}))

	beforeHistory := countRowsForOwner(t, ctx, store, "reading_history", owner.ID)
	beforeKnown := countRowsForOwner(t, ctx, store, "known_vocabulary", owner.ID)
	beforeBooks := countRowsForOwner(t, ctx, store, "books", owner.ID)

	report, err := store.BackfillChosenBooks(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, report.Removed, "unread Inbox, hidden Inbox, classical Inbox and both language-less Books leave")
	require.Len(t, report.LanguageLessWithHistory, 1)
	assert.Equal(t, languageLessWithHistory.ID, report.LanguageLessWithHistory[0].ID)

	for _, book := range []domain.Book{inboxUnread, hiddenInbox, classicalInbox, languageLessUnread, languageLessWithHistory} {
		assert.Equal(t, "removed", membershipState(t, ctx, store, owner.ID, book.ID), "unchosen Book left My Books")
	}
	for _, book := range []domain.Book{inboxWithHistory, hiddenToRead, current} {
		assert.Equal(t, "active", membershipState(t, ctx, store, owner.ID, book.ID), "chosen Book stayed in My Books")
	}

	assert.Equal(t, beforeHistory, countRowsForOwner(t, ctx, store, "reading_history", owner.ID), "the backfill changes no completion")
	assert.Equal(t, beforeKnown, countRowsForOwner(t, ctx, store, "known_vocabulary", owner.ID), "the backfill changes no Known vocabulary")
	assert.Equal(t, beforeBooks, countRowsForOwner(t, ctx, store, "books", owner.ID), "the backfill deletes no Book")

	second, err := store.BackfillChosenBooks(ctx)
	require.NoError(t, err)
	assert.Zero(t, second.Removed, "a repeated run changes nothing")
	assert.Empty(t, second.LanguageLessWithHistory, "an already removed Book is not reported again")
	assert.Equal(t, 1, countRowsWhere(t, ctx, store, "reading_history", "book_id", languageLessWithHistory.ID), "the removed Book keeps its completion")
}

func membershipState(t *testing.T, ctx context.Context, store *PostgresStore, owner, bookID string) string {
	t.Helper()
	var state string
	require.NoError(t, store.Pool().QueryRow(ctx, `
		SELECT state FROM book_membership WHERE owner_id = $1 AND book_id = $2`, owner, bookID).Scan(&state))
	return state
}

func countRowsWhere(t *testing.T, ctx context.Context, store *PostgresStore, table, column, value string) int {
	t.Helper()
	var count int
	// The table and column names are test constants, never learner input.
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE `+column+` = $1`, value).Scan(&count))
	return count
}

func countRowsForOwner(t *testing.T, ctx context.Context, store *PostgresStore, table, owner string) int {
	t.Helper()
	var count int
	// The table name is a test constant, never learner input.
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE owner_id = $1`, owner).Scan(&count))
	return count
}
