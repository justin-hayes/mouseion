//go:build integration

package persistence

import (
	"context"
	"sync"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookDispositionCatalogueLifecycle(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	alice, err := store.CreateUser(ctx, "disposition-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "disposition-bob", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Disposition catalog", URL: "https://disposition.example/opds"})
	require.NoError(t, err)

	created, err := seedCatalogueBook(t, ctx, store, alice.ID, connection.ID, "disposition-entry", "Disposition book", "Author", "de")
	require.NoError(t, err)
	bookID := created.Book.ID
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, bookID), "Add creates a To Read Book")

	updated, err := store.UpdateCatalogueBookMetadata(ctx, alice.ID, connection.ID, "disposition-entry", "Refreshed title", "New author")
	require.NoError(t, err)
	assert.True(t, updated.Updated)
	assert.Equal(t, bookID, updated.Book.ID, "metadata update created a duplicate Book")
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, bookID), "sync reset disposition")
	assert.Equal(t, "de", updated.Book.LanguageTag, "sync changed the Book's language")

	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, bookID, domain.BookDispositionInbox))
	_, err = store.UpdateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{ID: connection.ID, Name: "Disposition catalog updated", URL: "https://updated-disposition.example/opds"})
	require.NoError(t, err)
	reconnected, err := store.UpdateCatalogueBookMetadata(ctx, alice.ID, connection.ID, "disposition-entry", "Refreshed from connection change", "New author")
	require.NoError(t, err)
	assert.Equal(t, bookID, reconnected.Book.ID, "connection change created a duplicate Book")
	assert.Equal(t, domain.BookDispositionInbox, mustBookDisposition(t, store, alice.ID, bookID), "sync rewrote the disposition")

	require.NoError(t, store.RemoveBookFromMyBooks(ctx, alice.ID, bookID))
	assert.Equal(t, domain.BookDispositionInbox, mustBookDisposition(t, store, alice.ID, bookID), "removal from My Books rewrote disposition")
	_, err = store.UpdateCatalogueBookMetadata(ctx, alice.ID, connection.ID, "disposition-entry", "Refreshed after removal", "New author")
	require.NoError(t, err)
	_, err = store.GetBookDetail(ctx, alice.ID, bookID)
	require.ErrorIs(t, err, ErrNotFound, "sync restored a Book that left My Books")

	toReadBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "To Read disposition", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, toReadBook.ID, domain.BookDispositionToRead))
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, toReadBook.ID))
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, toReadBook.ID, domain.BookDispositionInbox))
	assert.Equal(t, domain.BookDispositionInbox, mustBookDisposition(t, store, alice.ID, toReadBook.ID))
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, toReadBook.ID, domain.BookDispositionInbox), "disposition retries are idempotent")
	inboxBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Inbox disposition", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, inboxBook.ID, domain.BookDispositionInbox))
	assert.Equal(t, domain.BookDispositionInbox, mustBookDisposition(t, store, alice.ID, inboxBook.ID))
	retagged, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Retagged disposition", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, retagged.ID, domain.BookDispositionToRead))
	_, err = store.UpdateBookMetadata(ctx, alice.ID, retagged.ID, retagged.Title, retagged.Author, domain.LanguageUnknown, "")
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, retagged.ID), "language correction reset disposition")

	_, err = store.GetBookDisposition(ctx, bob.ID, bookID)
	require.ErrorIs(t, err, ErrNotFound)
	err = store.SetBookDisposition(ctx, bob.ID, bookID, domain.BookDispositionToRead)
	require.ErrorIs(t, err, ErrNotFound)

	var rows int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM book_dispositions WHERE owner_id=$1 AND book_id=$2`, alice.ID, bookID).Scan(&rows)
	require.NoError(t, err)
	assert.Equal(t, 1, rows, "catalogue writes created duplicate dispositions")
	_, err = pool.Exec(ctx, `INSERT INTO book_dispositions(owner_id, book_id, disposition) VALUES($1, $2, 'invalid')`, alice.ID, bookID)
	assert.Error(t, err, "database accepted an invalid disposition")
}

func TestBookDispositionConcurrentWritesAreOwnerScoped(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	alice, err := store.CreateUser(ctx, "concurrent-disposition-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "concurrent-disposition-bob", false)
	require.NoError(t, err)
	aliceBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice disposition", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	bobBook, err := store.CreateBook(ctx, domain.Book{OwnerID: bob.ID, Title: "Bob disposition", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)

	const writers = 12
	results := make(chan error, writers)
	var wait sync.WaitGroup
	wait.Add(writers)
	for i := range writers {
		go func(i int) {
			defer wait.Done()
			disposition := domain.BookDispositionToRead
			if i%2 == 0 {
				disposition = domain.BookDispositionInbox
			}
			results <- store.SetBookDisposition(ctx, alice.ID, aliceBook.ID, disposition)
		}(i)
	}
	wait.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}

	final := mustBookDisposition(t, store, alice.ID, aliceBook.ID)
	assert.Contains(t, []domain.BookDisposition{domain.BookDispositionToRead, domain.BookDispositionInbox}, final)
	_, err = store.GetBookDisposition(ctx, alice.ID, bobBook.ID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = store.GetBookDisposition(ctx, bob.ID, aliceBook.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestBookDispositionRevisionRejectsStaleDecisionsAndAllowsReplay(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "revision-disposition-owner", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "revision-disposition-other", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Revision guarded", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	initial, err := store.GetBookDetail(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), initial.DispositionRevision)

	applied, err := store.TransitionBookDisposition(ctx, owner.ID, book.ID, initial.DispositionRevision, domain.BookDispositionToRead)
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = store.TransitionBookDisposition(ctx, owner.ID, book.ID, initial.DispositionRevision, domain.BookDispositionToRead)
	require.NoError(t, err, "replaying the immediately accepted decision is safe")
	require.False(t, applied, "replay must not repeat downstream work")
	_, err = store.TransitionBookDisposition(ctx, owner.ID, book.ID, initial.DispositionRevision, domain.BookDispositionInbox)
	require.ErrorIs(t, err, ErrStaleBookDisposition)

	current, err := store.GetBookDetail(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), current.DispositionRevision, "retry must not advance the state revision")
	_, err = store.TransitionBookDisposition(ctx, owner.ID, book.ID, current.DispositionRevision, domain.BookDispositionInbox)
	require.NoError(t, err)
	current, err = store.GetBookDetail(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	_, err = store.TransitionBookDisposition(ctx, owner.ID, book.ID, current.DispositionRevision, domain.BookDispositionToRead)
	require.NoError(t, err)
	beforeNoOp, err := store.GetBookDetail(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	afterNoOp, err := store.GetBookDetail(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, beforeNoOp.DispositionRevision, afterNoOp.DispositionRevision, "an unchanged disposition does not invalidate open forms")
	_, err = store.TransitionBookDisposition(ctx, owner.ID, book.ID, initial.DispositionRevision, domain.BookDispositionToRead)
	require.ErrorIs(t, err, ErrStaleBookDisposition, "a change away and back must not make the old decision current")
	_, err = store.TransitionBookDisposition(ctx, other.ID, book.ID, current.DispositionRevision, domain.BookDispositionInbox)
	require.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, owner.ID, book.ID))
	var historyRows, knownRows int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&historyRows))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&knownRows))
	assert.Zero(t, historyRows)
	assert.Zero(t, knownRows)
}

func mustBookDisposition(t *testing.T, store *PostgresStore, owner, bookID string) domain.BookDisposition {
	t.Helper()
	disposition, err := store.GetBookDisposition(context.Background(), owner, bookID)
	require.NoError(t, err)
	return disposition
}
