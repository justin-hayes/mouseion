//go:build integration

package persistence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMyBooksBrowseFiltersCountsPagingAndOwnership(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	alice, err := store.CreateUser(ctx, "browse-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "browse-bob", false)
	require.NoError(t, err)
	create := func(owner, title, state, tag, provenance string) domain.Book {
		input, createErr := domain.NewBook(owner, title, provenance, state, tag)
		require.NoError(t, createErr)
		book, createErr := store.CreateBook(ctx, input)
		require.NoError(t, createErr)
		return book
	}

	percent := create(alice.ID, "100% real", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	create(alice.ID, "German Upper", domain.LanguageChosen, "DE", domain.MetadataProvenanceCatalogueSync)
	create(alice.ID, "German Lower", domain.LanguageChosen, "de", domain.MetadataProvenanceCatalogueSync)
	metadataDampf := create(alice.ID, "Metadata Donaudampf", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	acquiredSource := putBookSource(t, ctx, store, alice.ID, "browse-acquired", "Acquired Donaudampf", []byte("browse-content"), "browse content")
	acquiredID, err := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, acquiredSource.SourceIdentifier, acquiredSource.Language, acquiredSource.Title)
	require.NoError(t, err)
	err = store.LinkSourceToBook(ctx, alice.ID, acquiredID, acquiredSource.ID)
	require.NoError(t, err)
	for i := range 26 {
		create(alice.ID, fmt.Sprintf("Page %02d", i), domain.LanguageChosen, "it", domain.MetadataProvenanceCatalogueSync)
	}
	tieA := create(alice.ID, "Same title", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	tieB := create(alice.ID, "Same title", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	create(bob.ID, "Metadata Donaudampf", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)

	result, err := store.ListMyBooksBrowse(ctx, alice.ID, "Donaudampf", "", 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	assert.Equal(t, 2, len(result.Items))
	assert.Equal(t, 33, result.AllCount)
	for _, item := range result.Items {
		assert.Equal(t, alice.ID, item.Book.OwnerID, "cross-owner item leaked: %+v", item)
	}

	literal, err := store.ListMyBooksBrowse(ctx, alice.ID, "%", "", 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 1, literal.Total)
	assert.Equal(t, percent.ID, literal.Items[0].Book.ID, "LIKE wildcard was not literal")

	deResult, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "DE", 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 3, deResult.Total, "German filter")
	require.Len(t, deResult.Counts, 3)
	assert.Equal(t, "de", deResult.Counts[0].Tag)
	assert.Equal(t, 3, deResult.Counts[0].Count)
	assert.Equal(t, "it", deResult.Counts[1].Tag)
	assert.Equal(t, 26, deResult.Counts[1].Count)
	assert.Equal(t, "unknown", deResult.Counts[2].Tag)
	assert.Equal(t, 4, deResult.Counts[2].Count)
	unknown, err := store.ListMyBooksBrowse(ctx, alice.ID, "Donaudampf", domain.LanguageUnknown, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 1, unknown.Total)
	assert.Equal(t, metadataDampf.ID, unknown.Items[0].Book.ID, "unknown combined filter")

	page, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "", 25, 25)
	require.NoError(t, err)
	assert.Equal(t, 33, page.Total)
	assert.Equal(t, 8, len(page.Items), "page slice")
	tieIDs := []string{tieA.ID, tieB.ID}
	orderedTies, err := store.ListMyBooksBrowse(ctx, alice.ID, "Same title", "", 0, 25)
	require.NoError(t, err)
	require.Len(t, orderedTies.Items, 2)
	assert.Equal(t, minString(tieIDs[0], tieIDs[1]), orderedTies.Items[0].Book.ID)
	assert.Equal(t, maxString(tieIDs[0], tieIDs[1]), orderedTies.Items[1].Book.ID)

	err = store.RemoveBookFromMyBooks(ctx, alice.ID, percent.ID)
	require.NoError(t, err)
	remaining, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "", 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 32, remaining.AllCount)
	assert.Equal(t, 32, remaining.Total)
	assert.False(t, strings.Contains(fmt.Sprint(remaining.Items), percent.ID), "removed membership remained in browse")
}

func minString(left, right string) string {
	if left < right {
		return left
	}
	return right
}

func maxString(left, right string) string {
	if left > right {
		return left
	}
	return right
}
