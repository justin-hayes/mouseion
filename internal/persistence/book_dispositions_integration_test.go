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

	created, err := store.ReconcileCatalogueEntry(ctx, alice.ID, connection.ID, "disposition-entry", "Disposition book", "Author", "de")
	require.NoError(t, err)
	bookID := created.Book.ID
	assert.Equal(t, domain.BookDispositionInbox, mustBookDisposition(t, store, alice.ID, bookID))

	_, err = store.ReconcileCatalogueEntry(ctx, alice.ID, connection.ID, "disposition-entry", "Refreshed title", "New author", "it")
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, mustBookDisposition(t, store, alice.ID, bookID), "metadata refresh reset disposition")

	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, bookID, domain.BookDispositionToRead))
	_, err = store.UpdateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{ID: connection.ID, Name: "Disposition catalog updated", URL: "https://updated-disposition.example/opds"})
	require.NoError(t, err)
	reconnected, err := store.ReconcileCatalogueEntry(ctx, alice.ID, connection.ID, "disposition-entry", "Refreshed from connection change", "New author", "de")
	require.NoError(t, err)
	assert.Equal(t, bookID, reconnected.Book.ID, "connection change created a duplicate Book")
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, bookID), "connection change reset disposition")

	require.NoError(t, store.RemoveBookFromMyBooks(ctx, alice.ID, bookID))
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, bookID), "removal from My Books rewrote disposition")
	require.NoError(t, store.AddBookToMyBooks(ctx, alice.ID, bookID))
	_, err = store.ReconcileCatalogueEntry(ctx, alice.ID, connection.ID, "disposition-entry", "Refreshed after reappearance", "New author", "de")
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, bookID), "reappearance reset disposition")

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
	assert.Equal(t, 1, rows, "catalogue reconciliation created duplicate dispositions")
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

func TestCatalogueLanguageCorrectionEndsIncompatibleCurrentReading(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	owner, err := store.CreateUser(ctx, "catalogue-language-correction", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Language catalog", URL: "https://language.example/opds"})
	require.NoError(t, err)
	reconciled, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "language-entry", "Language-corrected book", "Author", "de")
	require.NoError(t, err)
	book, source, _ := createReadingSourceAndPreparationInLanguage(t, ctx, store, owner.ID, reconciled.Book, "de", "language-correction")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	require.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, owner.ID, book.ID))

	corrected, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "language-entry", "Updated title", "Updated author", "it")
	require.NoError(t, err)
	assert.True(t, corrected.LanguageChanged)
	assert.True(t, corrected.CurrentReadingEnded, "the correction must report that it ended the old role so the learner is told")
	assert.Equal(t, "it", corrected.Book.LanguageTag)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, owner.ID, book.ID), "language correction rewrote learner disposition")

	oldLanguage, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, oldLanguage.BookID, "incompatible current reading remained in its former language")
	newLanguage, err := store.GetCurrentReading(ctx, owner.ID, "it")
	require.NoError(t, err)
	assert.Empty(t, newLanguage.BookID, "language correction consumed the new language's current-reading slot")
	var activeGoalRows, reservedSnapshotRows int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&activeGoalRows)
	require.NoError(t, err)
	assert.Zero(t, activeGoalRows)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NULL`, owner.ID, reading.SnapshotID).Scan(&reservedSnapshotRows)
	require.NoError(t, err)
	assert.Zero(t, reservedSnapshotRows, "released current-reading snapshot remained reserved")
	preparations, err := store.ListDeckPreparationsForSourceMaterial(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	assert.Len(t, preparations, 1, "language correction erased local deck state")
	_, err = store.ImportPreviouslyRead(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	require.NoError(t, store.DeleteOpdsConnection(ctx, owner.ID, connection.ID))
	retained, err := store.GetBook(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, "it", retained.LanguageTag)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, owner.ID, book.ID))
	var completionCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&completionCount)
	require.NoError(t, err)
	assert.Equal(t, 1, completionCount, "connection deletion erased reading history")
	sources, err := store.ListSourceMaterials(ctx, owner.ID)
	require.NoError(t, err)
	assert.Len(t, sources, 1, "connection deletion erased acquired provenance")
	var analysisCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&analysisCount)
	require.NoError(t, err)
	assert.Equal(t, 1, analysisCount, "connection deletion erased analysis provenance")
	preparations, err = store.ListDeckPreparationsForSourceMaterial(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	assert.Len(t, preparations, 1, "connection deletion erased local deck state")
}

func mustBookDisposition(t *testing.T, store *PostgresStore, owner, bookID string) domain.BookDisposition {
	t.Helper()
	disposition, err := store.GetBookDisposition(context.Background(), owner, bookID)
	require.NoError(t, err)
	return disposition
}
