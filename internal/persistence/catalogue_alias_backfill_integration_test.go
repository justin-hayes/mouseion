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

func TestCatalogueAliasBackfillPersistenceIsScopedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	alice, err := store.CreateUser(ctx, "alias-backfill-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "alias-backfill-bob", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Legacy", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `ALTER TABLE book_aliases DROP CONSTRAINT book_aliases_connection_contract`)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_aliases(owner_id,book_id,alias_type,namespace,value) VALUES($1,$2,'catalog_entry','source_identifier','legacy-entry')`, alice.ID, book.ID)
	require.NoError(t, err)
	err = store.AddBookAlias(ctx, alice.ID, book.ID, domain.AliasStrongBibliographic, "isbn", "978-legacy")
	require.NoError(t, err)
	aliceConnection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Alice", URL: "https://alice.example/opds"})
	require.NoError(t, err)
	bobConnection, err := store.CreateOpdsConnection(ctx, bob.ID, domain.OpdsConnection{Name: "Bob", URL: "https://bob.example/opds"})
	require.NoError(t, err)
	aliases, err := store.ListUnscopedCatalogueEntryAliases(ctx)
	require.NoError(t, err)
	require.Len(t, aliases, 1)
	assert.Equal(t, "legacy-entry", aliases[0].Value)
	err = store.SetCatalogueEntryAliasConnection(ctx, alice.ID, aliases[0].ID, bobConnection.ID)
	assert.ErrorIs(t, err, ErrAliasConflict)
	err = store.SetCatalogueEntryAliasConnection(ctx, alice.ID, aliases[0].ID, aliceConnection.ID)
	require.NoError(t, err)
	err = store.SetCatalogueEntryAliasConnection(ctx, alice.ID, aliases[0].ID, aliceConnection.ID)
	require.NoError(t, err, "idempotent assignment")
	aliases, err = store.ListUnscopedCatalogueEntryAliases(ctx)
	require.NoError(t, err)
	assert.Empty(t, aliases)
	alias, err := store.GetBookCatalogEntryAlias(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, aliceConnection.ID, alias.ConnectionID)
	_, err = store.Pool().Exec(ctx, `ALTER TABLE book_aliases ADD CONSTRAINT book_aliases_connection_contract CHECK ((alias_type = 'catalog_entry' AND connection_id IS NOT NULL) OR (alias_type = 'strong_bibliographic' AND connection_id IS NULL))`)
	require.NoError(t, err)
}
