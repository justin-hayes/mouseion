//go:build integration

package persistence

import (
	"bytes"
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogueMetadataRefreshPreservesAcquiredEvidence(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	owner, err := store.CreateUser(ctx, "refresh-owner", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Refresh catalog", URL: "https://catalog.example/opds"})
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "refresh-other-owner", false)
	require.NoError(t, err)
	source := putBookSource(t, ctx, store, owner.ID, "refresh-entry", "Old title", []byte("acquired content"), "readable text")
	reconciled, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, source.SourceIdentifier, source.Title, source.Language)
	require.NoError(t, err)
	bookID := reconciled.Book.ID
	err = store.LinkSourceToBook(ctx, owner.ID, bookID, source.ID)
	require.NoError(t, err)

	beforeSource, err := store.GetSourceMaterial(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	var beforeRevisions, beforeAliases, beforeMemberships int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&beforeRevisions)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&beforeAliases)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_membership WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&beforeMemberships)
	require.NoError(t, err)

	result, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, source.SourceIdentifier, "New title", "it")
	require.NoError(t, err)
	assert.True(t, result.TitleChanged)
	assert.True(t, result.LanguageChanged)
	assert.Equal(t, "New title", result.Book.Title)
	assert.Equal(t, domain.LanguageChosen, result.Book.LanguageState)
	assert.Equal(t, "it", result.Book.LanguageTag)
	alias, err := store.GetBookCatalogEntryAlias(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, connection.ID, alias.ConnectionID)
	afterSource, err := store.GetSourceMaterial(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	var afterRevisions, afterAliases, afterMemberships int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&afterRevisions)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&afterAliases)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_membership WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&afterMemberships)
	require.NoError(t, err)
	assert.Equal(t, beforeSource.Title, afterSource.Title)
	assert.Equal(t, beforeSource.ContentHash, afterSource.ContentHash)
	assert.Equal(t, beforeSource.ContentDigest, afterSource.ContentDigest)
	assert.Equal(t, beforeSource.ContentRevisionID, afterSource.ContentRevisionID)
	assert.True(t, bytes.Equal(beforeSource.Content, afterSource.Content), "refresh changed acquired content")
	assert.Equal(t, beforeRevisions, afterRevisions)
	assert.Equal(t, beforeAliases, afterAliases)
	assert.Equal(t, beforeMemberships, afterMemberships)
	updatedLibrary, err := store.ListSourceMaterials(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, updatedLibrary, 1)
	assert.Equal(t, "New title", updatedLibrary[0].BookTitle, "refreshed canonical title was not projected into source summary")

	_, err = store.GetBookCatalogEntryAlias(ctx, other.ID, bookID)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = store.GetBook(ctx, other.ID, bookID)
	assert.ErrorIs(t, err, ErrNotFound)
}
