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

func TestBookDispositionCatalogueLifecycleAndLegacySynchronization(t *testing.T) {
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
	assert.Equal(t, domain.BookDispositionSetAside, mustBookDisposition(t, store, alice.ID, bookID))
	require.NoError(t, store.AddBookToMyBooks(ctx, alice.ID, bookID))
	_, err = store.ReconcileCatalogueEntry(ctx, alice.ID, connection.ID, "disposition-entry", "Refreshed after reappearance", "New author", "de")
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, mustBookDisposition(t, store, alice.ID, bookID), "reappearance reset disposition")

	journeyBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Journey disposition", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	journey, err := store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, alice.ID, "de", journeyBook.ID, journey.Revision)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, journeyBook.ID))
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	err = store.SetBookAsideAtJourneyRevision(ctx, alice.ID, "de", journeyBook.ID, journey.Revision)
	require.ErrorIs(t, err, ErrJourneyStale, "a Journey member must not be set aside through the Inbox path")
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, journeyBook.ID))
	inboxBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Inbox disposition", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	err = store.SetBookAsideAtJourneyRevision(ctx, alice.ID, "de", inboxBook.ID, journey.Revision)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, mustBookDisposition(t, store, alice.ID, inboxBook.ID))
	err = store.SetBookAsideAtJourneyRevision(ctx, alice.ID, "de", inboxBook.ID, journey.Revision-1)
	require.ErrorIs(t, err, ErrJourneyStale, "stale Journey revisions must not overwrite a disposition")
	assert.Equal(t, domain.BookDispositionSetAside, mustBookDisposition(t, store, alice.ID, inboxBook.ID))
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, journeyBook.ID, domain.BookDispositionSetAside))
	_, err = store.AddToReadingJourney(ctx, alice.ID, "de", journeyBook.ID, journey.Revision)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, journeyBook.ID), "idempotent Journey add did not commit disposition")
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	_, err = store.RemoveFromReadingJourney(ctx, alice.ID, "de", journeyBook.ID, journey.Revision)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, mustBookDisposition(t, store, alice.ID, journeyBook.ID))
	retagged, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Retagged disposition", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, alice.ID, "de", retagged.ID, journey.Revision)
	require.NoError(t, err)
	_, err = store.UpdateBookMetadata(ctx, alice.ID, retagged.ID, retagged.Title, retagged.Author, domain.LanguageUnknown, "")
	require.NoError(t, err)
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	_, err = store.RemoveFromReadingJourney(ctx, alice.ID, "de", retagged.ID, journey.Revision)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, mustBookDisposition(t, store, alice.ID, retagged.ID), "language cleanup reset disposition")

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
				disposition = domain.BookDispositionSetAside
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
	assert.Contains(t, []domain.BookDisposition{domain.BookDispositionToRead, domain.BookDispositionSetAside}, final)
	_, err = store.GetBookDisposition(ctx, alice.ID, bobBook.ID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = store.GetBookDisposition(ctx, bob.ID, aliceBook.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func mustBookDisposition(t *testing.T, store *PostgresStore, owner, bookID string) domain.BookDisposition {
	t.Helper()
	disposition, err := store.GetBookDisposition(context.Background(), owner, bookID)
	require.NoError(t, err)
	return disposition
}
