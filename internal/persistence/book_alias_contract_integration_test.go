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

func TestBookAliasConnectionContract(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	defer store.Close()

	owner, err := store.CreateUser(ctx, "alias-contract-owner", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Contract", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	err = store.AddBookAlias(ctx, owner.ID, book.ID, domain.AliasStrongBibliographic, "isbn", "978-contract")
	require.NoError(t, err)
	err = store.AddBookAlias(ctx, owner.ID, book.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, "connection-required")
	assert.Error(t, err, "connectionless catalogue alias was accepted")
	var aliasCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1`, owner.ID).Scan(&aliasCount)
	require.NoError(t, err)
	assert.Equal(t, 1, aliasCount)
	err = store.SetCatalogueEntryAliasConnection(ctx, owner.ID, "00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000000")
	assert.ErrorIs(t, err, ErrNotFound)
}
