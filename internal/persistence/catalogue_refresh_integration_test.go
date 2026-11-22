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

// TestCatalogueMetadataUpdatePreservesAcquiredEvidence covers sync's only
// My Books write: a linked Book follows its entry's title and author, while its
// language, membership, disposition, aliases and acquired evidence stay put.
func TestCatalogueMetadataUpdatePreservesAcquiredEvidence(t *testing.T) {
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
	seeded, err := seedCatalogueBook(t, ctx, store, owner.ID, connection.ID, source.SourceIdentifier, source.Title, "Old author", source.Language)
	require.NoError(t, err)
	bookID := seeded.Book.ID
	err = store.LinkSourceToBook(ctx, owner.ID, bookID, source.ID)
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, bookID, domain.BookDispositionToRead))

	beforeSource, err := store.GetSourceMaterial(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	var beforeRevisions, beforeAliases, beforeMemberships, beforeDispositions int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&beforeRevisions)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&beforeAliases)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_membership WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&beforeMemberships)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_dispositions WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&beforeDispositions)
	require.NoError(t, err)

	result, err := store.UpdateCatalogueBookMetadata(ctx, owner.ID, connection.ID, source.SourceIdentifier, "New title", "New author")
	require.NoError(t, err)
	assert.True(t, result.Updated)
	assert.Equal(t, "New title", result.Book.Title)
	assert.Equal(t, "New author", result.Book.Author)
	assert.Equal(t, seeded.Book.LanguageState, result.Book.LanguageState, "sync changed the Book's language state")
	assert.Equal(t, seeded.Book.LanguageTag, result.Book.LanguageTag, "sync changed the Book's language")
	alias, err := store.GetBookCatalogEntryAlias(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, connection.ID, alias.ConnectionID)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, owner.ID, bookID), "sync rewrote the learner's disposition")
	afterSource, err := store.GetSourceMaterial(ctx, owner.ID, source.ID)
	require.NoError(t, err)
	var afterRevisions, afterAliases, afterMemberships, afterDispositions int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&afterRevisions)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&afterAliases)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_membership WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&afterMemberships)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_dispositions WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&afterDispositions)
	require.NoError(t, err)
	assert.Equal(t, beforeSource.Title, afterSource.Title)
	assert.Equal(t, beforeSource.ContentHash, afterSource.ContentHash)
	assert.Equal(t, beforeSource.ContentDigest, afterSource.ContentDigest)
	assert.Equal(t, beforeSource.ContentRevisionID, afterSource.ContentRevisionID)
	assert.True(t, bytes.Equal(beforeSource.Content, afterSource.Content), "metadata update changed acquired content")
	assert.Equal(t, beforeRevisions, afterRevisions)
	assert.Equal(t, beforeAliases, afterAliases)
	assert.Equal(t, beforeMemberships, afterMemberships)
	assert.Equal(t, beforeDispositions, afterDispositions)
	updatedLibrary, err := store.ListSourceMaterials(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, updatedLibrary, 1)
	assert.Equal(t, "New title", updatedLibrary[0].BookTitle, "updated canonical title was not projected into source summary")
	assert.Equal(t, "New author", updatedLibrary[0].BookAuthor, "updated canonical author was not projected into source summary")

	t.Run("an unlinked entry updates no Book", func(t *testing.T) {
		unchanged, err := store.UpdateCatalogueBookMetadata(ctx, owner.ID, connection.ID, "not-linked", "Title", "Author")
		require.NoError(t, err)
		assert.Empty(t, unchanged.Book.ID)
		assert.False(t, unchanged.Updated)
	})
	t.Run("cross-owner entries update no Book", func(t *testing.T) {
		unchanged, err := store.UpdateCatalogueBookMetadata(ctx, other.ID, connection.ID, source.SourceIdentifier, "Other title", "Other author")
		require.NoError(t, err)
		assert.Empty(t, unchanged.Book.ID)
		assert.False(t, unchanged.Updated)
	})
	t.Run("cross-owner alias lookup is not found", func(t *testing.T) {
		_, err := store.GetBookCatalogEntryAlias(ctx, other.ID, bookID)
		require.ErrorIs(t, err, ErrNotFound)
	})
	t.Run("cross-owner book lookup is not found", func(t *testing.T) {
		_, err := store.GetBook(ctx, other.ID, bookID)
		require.ErrorIs(t, err, ErrNotFound)
	})
}

func TestUpdateCatalogueBookMetadataRejectsIncompleteIdentity(t *testing.T) {
	ctx := context.Background()
	store, _ := newFailureStore(t, ctx, "catalogue-metadata-identity-owner")
	for _, tc := range []struct {
		name                                 string
		owner, connection, identifier, title string
	}{
		{name: "owner", connection: "connection", identifier: "entry", title: "Title"},
		{name: "connection", owner: "owner", identifier: "entry", title: "Title"},
		{name: "entry identifier", owner: "owner", connection: "connection", title: "Title"},
		{name: "title", owner: "owner", connection: "connection", identifier: "entry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.UpdateCatalogueBookMetadata(ctx, tc.owner, tc.connection, tc.identifier, tc.title, "")
			require.Error(t, err)
		})
	}
}

func TestUpdateCatalogueBookMetadataReportsStoreFailures(t *testing.T) {
	ctx := context.Background()
	store, owner := newFailureStore(t, ctx, "catalogue-metadata-store-failure-owner")
	connection := addFailureConnection(t, ctx, store, owner, "Failure")
	require.NoError(t, store.Close())

	_, err := store.UpdateCatalogueBookMetadata(ctx, owner, connection.ID, "entry", "Title", "")
	require.Error(t, err)
}
