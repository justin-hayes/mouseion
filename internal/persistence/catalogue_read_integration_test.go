//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkCatalogueEntryPreviouslyReadRefusesIncompleteIdentityAndUnknownConnection(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "catalogue-read-refusals", false)
	require.NoError(t, err)

	_, err = store.MarkCatalogueEntryPreviouslyRead(ctx, "", "", "")
	require.ErrorContains(t, err, "identity is incomplete")

	_, err = store.MarkCatalogueEntryPreviouslyRead(ctx, owner.ID, "00000000-0000-0000-0000-000000000000", "entry")
	require.ErrorIs(t, err, domain.ErrNotFound, "an unknown connection is not found")
}

func TestMarkCatalogueEntryPreviouslyReadRefreshesLanguageAndReopensBook(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "catalogue-read-restore", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Read catalog", URL: "https://read.example/opds"})
	require.NoError(t, err)
	entry := domain.CatalogueIndexEntry{
		ConnectionID: connection.ID, EntryIdentifier: "read-entry", Title: "Read Entry",
		Author: "Entry Author", LanguageTag: "de", EPUBHref: "https://read.example/read.epub",
	}
	require.NoError(t, store.ReconcileCatalogueIndex(ctx, owner.ID, connection.ID, []domain.CatalogueIndexEntry{entry}, time.Now()))
	added, err := store.AddCatalogueEntry(ctx, owner.ID, connection.ID, "read-entry")
	require.NoError(t, err)
	require.NoError(t, store.RemoveBookFromMyBooks(ctx, owner.ID, added.Book.ID))

	entry.LanguageTag = "it"
	require.NoError(t, store.ReconcileCatalogueIndex(ctx, owner.ID, connection.ID, []domain.CatalogueIndexEntry{entry}, time.Now()))
	read, err := store.MarkCatalogueEntryPreviouslyRead(ctx, owner.ID, connection.ID, "read-entry")
	require.NoError(t, err)
	assert.Equal(t, added.Book.ID, read.Book.ID, "the alias restores the same Book")
	assert.Equal(t, "it", read.Book.LanguageTag, "the current entry metadata refreshes the Book")

	detail, err := store.GetBookDetail(ctx, owner.ID, added.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, detail.Disposition)
	assert.Equal(t, 1, detail.CompletionCount)
}

func TestMarkClassicalReadingUnitPreviouslyReadRefusesUnknownUnitsAndLegacyRows(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "classical-read-refusals", false)
	require.NoError(t, err)
	require.NoError(t, store.ReconcileClassicalManifest(ctx, classicalAddDefinitions()))

	_, err = store.MarkClassicalReadingUnitPreviouslyRead(ctx, "", "")
	require.ErrorContains(t, err, "identity is incomplete")
	_, err = store.MarkClassicalReadingUnitPreviouslyRead(ctx, owner.ID, "nonexistent")
	require.ErrorContains(t, err, "not current")

	_, err = pool.Exec(ctx, `INSERT INTO books(owner_id, title, metadata_provenance, language_state, language_tag) VALUES ($1, 'Legacy Greek', 'catalog_sync', 'chosen', 'grc')`, owner.ID)
	require.NoError(t, err)
	_, err = store.MarkClassicalReadingUnitPreviouslyRead(ctx, owner.ID, "work-1")
	require.ErrorIs(t, err, classical.ErrLegacyRows)
	assert.Contains(t, err.Error(), classical.LegacyRowsCode)
}

func TestMarkBookPreviouslyReadRefusesIneligibleBooks(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "previously-read-refusals", false)
	require.NoError(t, err)

	require.ErrorContains(t, store.MarkBookPreviouslyRead(ctx, "", ""), "identity is incomplete")
	require.ErrorIs(t, store.MarkBookPreviouslyRead(ctx, owner.ID, "00000000-0000-0000-0000-000000000000"), ErrNotFound)

	inbox, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Inbox", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	testutil.SeedLegacyInboxDisposition(t, ctx, store.Pool(), owner.ID, inbox.ID)
	require.ErrorIs(t, store.MarkBookPreviouslyRead(ctx, owner.ID, inbox.ID), domain.ErrPreviouslyReadIneligible)

	removed, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Removed", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, removed.ID, domain.BookDispositionToRead))
	require.NoError(t, store.RemoveBookFromMyBooks(ctx, owner.ID, removed.ID))
	require.ErrorIs(t, store.MarkBookPreviouslyRead(ctx, owner.ID, removed.ID), ErrNotFound, "an inactive Book records no assertion")
}
