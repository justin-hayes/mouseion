package fixtures

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureCatalogueAddRestoresACannedAcquiredBook(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	added, err := store.AddCatalogueEntry(ctx, OwnerID, "fixture-connection", "fixture-sommer")
	require.NoError(t, err)
	assert.False(t, added.Created, "the canned owned entry keeps its Book")
	assert.True(t, added.Changed, "restoring membership changes the learner's choice")
	assert.Equal(t, catalogueRestoreBookID, added.Book.ID)

	detail, err := store.GetBookDetail(ctx, OwnerID, catalogueRestoreBookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
	require.NotNil(t, detail.Acquired, "the restored Book keeps its acquired source")
	assert.Equal(t, "Ein kurzer Sommer", detail.Book.Title)
	assert.Equal(t, "de", detail.Book.LanguageTag)

	// A Book represented by a source summary rather than a metadata row is
	// restored through the same source it already has.
	sourceBook, err := store.AddCatalogueEntry(ctx, OwnerID, "fixture-browser-sync-connection", "fixture-browser-second")
	require.NoError(t, err)
	assert.Equal(t, BookID, sourceBook.Book.ID)
}
