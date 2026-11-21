//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogueIndexReconcileScopingAndCascade(t *testing.T) {
	ctx := context.Background()
	databaseURL := integrationDatabase(t, ctx)
	store := openIntegrationStore(t, ctx, databaseURL)
	alice, err := store.CreateUser(ctx, "index-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "index-bob", false)
	require.NoError(t, err)
	first, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "First catalog", URL: "https://first.example/opds"})
	require.NoError(t, err)
	second, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Second catalog", URL: "https://second.example/opds"})
	require.NoError(t, err)

	firstRun := time.Now().Add(-time.Minute)
	require.NoError(t, store.ReconcileCatalogueIndex(ctx, alice.ID, first.ID, []domain.CatalogueIndexEntry{
		{EntryIdentifier: "z-entry", Title: "Zulu title", Author: "Zulu author", LanguageTag: "de", EPUBHref: "https://first.example/z.epub"},
		{EntryIdentifier: "a-entry", Title: "Alpha title", Author: "Alpha author", LanguageTag: "de", CoverHref: "https://first.example/a.png", CoverMediaType: "image/png", EPUBHref: "https://first.example/a.epub"},
	}, firstRun))
	entries, err := store.ListCatalogueEntries(ctx, alice.ID, first.ID)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "a-entry", entries[0].EntryIdentifier, "index was not ordered by title")
	assert.Equal(t, "Alpha title", entries[0].Title)
	assert.Equal(t, "Alpha author", entries[0].Author)
	assert.Equal(t, "de", entries[0].LanguageTag)
	assert.Equal(t, "https://first.example/a.png", entries[0].CoverHref)
	assert.Equal(t, "image/png", entries[0].CoverMediaType)
	assert.Equal(t, "https://first.example/a.epub", entries[0].EPUBHref)
	assert.WithinDuration(t, firstRun, entries[0].LastSeenSync, time.Second)
	assert.Empty(t, entries[1].CoverHref, "an entry without a cover kept a cover link")

	got, err := store.GetCatalogueEntry(ctx, alice.ID, first.ID, "a-entry")
	require.NoError(t, err)
	assert.Equal(t, entries[0], got)
	_, err = store.GetCatalogueEntry(ctx, alice.ID, first.ID, "missing")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = store.GetCatalogueEntry(ctx, bob.ID, first.ID, "a-entry")
	require.ErrorIs(t, err, ErrNotFound, "another owner read the index entry")

	// A complete second run updates upstream metadata and deletes the entry it
	// no longer saw.
	secondRun := time.Now()
	require.NoError(t, store.ReconcileCatalogueIndex(ctx, alice.ID, first.ID, []domain.CatalogueIndexEntry{
		{EntryIdentifier: "a-entry", Title: "Renamed title", Author: "", LanguageTag: "fr", EPUBHref: "https://first.example/a-new.epub"},
	}, secondRun))
	entries, err = store.ListCatalogueEntries(ctx, alice.ID, first.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "Renamed title", entries[0].Title)
	assert.Empty(t, entries[0].Author)
	assert.Equal(t, "fr", entries[0].LanguageTag)
	assert.Equal(t, "https://first.example/a-new.epub", entries[0].EPUBHref)
	assert.Empty(t, entries[0].CoverHref)
	assert.WithinDuration(t, secondRun, entries[0].LastSeenSync, time.Second)

	// The same identifier under another connection is a separate entry.
	require.NoError(t, store.ReconcileCatalogueIndex(ctx, alice.ID, second.ID, []domain.CatalogueIndexEntry{
		{EntryIdentifier: "a-entry", Title: "Other catalog title", LanguageTag: "de", EPUBHref: "https://second.example/a.epub"},
	}, secondRun))
	secondEntries, err := store.ListCatalogueEntries(ctx, alice.ID, second.ID)
	require.NoError(t, err)
	require.Len(t, secondEntries, 1)
	assert.Equal(t, "Other catalog title", secondEntries[0].Title)

	t.Run("invalid reconciliation changes nothing", func(t *testing.T) {
		require.Error(t, store.ReconcileCatalogueIndex(ctx, alice.ID, first.ID, []domain.CatalogueIndexEntry{{EntryIdentifier: "bad", Title: "Missing links", LanguageTag: "de"}}, time.Now()))
		require.Error(t, store.ReconcileCatalogueIndex(ctx, alice.ID, first.ID, nil, time.Time{}))
		require.Error(t, store.ReconcileCatalogueIndex(ctx, "", first.ID, nil, time.Now()))
		entries, listErr := store.ListCatalogueEntries(ctx, alice.ID, first.ID)
		require.NoError(t, listErr)
		assert.Len(t, entries, 1, "an invalid run changed the index")
	})

	t.Run("another owner's connection is untouched", func(t *testing.T) {
		require.NoError(t, store.ReconcileCatalogueIndex(ctx, bob.ID, first.ID, nil, time.Now().Add(time.Hour)))
		entries, listErr := store.ListCatalogueEntries(ctx, alice.ID, first.ID)
		require.NoError(t, listErr)
		assert.Len(t, entries, 1, "another owner's run deleted or changed entries")
	})

	t.Run("deleting a connection removes its index only", func(t *testing.T) {
		require.NoError(t, store.DeleteOpdsConnection(ctx, alice.ID, first.ID))
		remaining, listErr := store.ListCatalogueEntries(ctx, alice.ID, first.ID)
		require.NoError(t, listErr)
		assert.Empty(t, remaining)
		secondEntries, listErr := store.ListCatalogueEntries(ctx, alice.ID, second.ID)
		require.NoError(t, listErr)
		assert.Len(t, secondEntries, 1, "deleting one connection removed another connection's index")
	})
}

func TestCatalogueIndexListEmptyForUnknownConnection(t *testing.T) {
	ctx := context.Background()
	databaseURL := integrationDatabase(t, ctx)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "index-unknown", false)
	require.NoError(t, err)
	entries, err := store.ListCatalogueEntries(ctx, owner.ID, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Empty(t, entries)
	_, err = store.GetCatalogueEntry(ctx, owner.ID, "00000000-0000-0000-0000-000000000000", "entry")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestCatalogueBrowseFiltersOwnershipAndPaging(t *testing.T) {
	ctx := context.Background()
	databaseURL := integrationDatabase(t, ctx)
	store := openIntegrationStore(t, ctx, databaseURL)
	alice, err := store.CreateUser(ctx, "browse-alice", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Browse catalog", URL: "https://browse.example/opds"})
	require.NoError(t, err)
	require.NoError(t, store.ReconcileCatalogueIndex(ctx, alice.ID, connection.ID, []domain.CatalogueIndexEntry{
		{EntryIdentifier: "owned", Title: "Über die Berge", Author: "Anna Berg", LanguageTag: "de", EPUBHref: "https://browse.example/owned.epub"},
		{EntryIdentifier: "unowned", Title: "Über das Meer", Author: "Bernd Alt", LanguageTag: "de", EPUBHref: "https://browse.example/unowned.epub"},
		{EntryIdentifier: "italian", Title: "La città", Author: "Luca Bianchi", LanguageTag: "it", EPUBHref: "https://browse.example/italian.epub"},
	}, time.Now()))
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Über die Berge", LanguageState: domain.LanguageChosen, LanguageTag: "de", MetadataProvenance: domain.MetadataProvenanceCatalogueSync})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_aliases(owner_id,book_id,connection_id,alias_type,namespace,value)
		VALUES($1,$2,$3,$4,$5,$6)`, alice.ID, book.ID, connection.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, "owned")
	require.NoError(t, err)

	page, err := store.BrowseCatalogueEntries(ctx, alice.ID, connection.ID, domain.CatalogueQuery{Search: "UBER", Language: "DE", Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 2, page.Total)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, "unowned", page.Entries[0].Index.EntryIdentifier)
	assert.Empty(t, page.Entries[0].BookID, "an unlinked entry has no Book")

	second, err := store.BrowseCatalogueEntries(ctx, alice.ID, connection.ID, domain.CatalogueQuery{Search: "uber", Language: "de", Offset: 1, Limit: 1})
	require.NoError(t, err)
	require.Len(t, second.Entries, 1)
	assert.Equal(t, "owned", second.Entries[0].Index.EntryIdentifier)
	assert.Equal(t, book.ID, second.Entries[0].BookID, "the active catalog-entry alias supplies ownership")

	italian, err := store.BrowseCatalogueEntries(ctx, alice.ID, connection.ID, domain.CatalogueQuery{Language: "it"})
	require.NoError(t, err)
	require.Len(t, italian.Entries, 1)
	assert.Equal(t, "italian", italian.Entries[0].Index.EntryIdentifier)

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.BrowseCatalogueEntries(cancelled, alice.ID, connection.ID, domain.CatalogueQuery{})
	require.Error(t, err, "a cancelled read reports its failure")
}
