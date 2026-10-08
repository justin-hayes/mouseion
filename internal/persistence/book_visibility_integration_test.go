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

func TestBookVisibilityIsIndependentAndRevisionProtected(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	alice, err := store.CreateUser(ctx, "visibility-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "visibility-bob", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Visibility catalog", URL: "https://visibility.example/opds"})
	require.NoError(t, err)
	created, err := store.ReconcileCatalogueEntry(ctx, alice.ID, connection.ID, "visibility-entry", "Visibility book", "Author", "de")
	require.NoError(t, err)
	bookID := created.Book.ID

	hidden, revision, err := store.GetBookVisibility(ctx, alice.ID, bookID)
	require.NoError(t, err)
	assert.False(t, hidden, "newly discovered Books are visible")
	assert.Zero(t, revision)

	// Hide, then a verified replay is harmless.
	applied, err := store.SetBookHidden(ctx, alice.ID, bookID, 0, true)
	require.NoError(t, err)
	assert.True(t, applied)
	applied, err = store.SetBookHidden(ctx, alice.ID, bookID, 0, true)
	require.NoError(t, err)
	assert.False(t, applied)

	// Unhide against the rendered revision; the stale Hide must not reverse it.
	applied, err = store.SetBookHidden(ctx, alice.ID, bookID, 1, false)
	require.NoError(t, err)
	assert.True(t, applied)
	_, err = store.SetBookHidden(ctx, alice.ID, bookID, 0, true)
	require.ErrorIs(t, err, ErrStaleBookVisibility)
	hidden, revision, err = store.GetBookVisibility(ctx, alice.ID, bookID)
	require.NoError(t, err)
	assert.False(t, hidden)
	assert.EqualValues(t, 2, revision)

	_, err = store.SetBookHidden(ctx, bob.ID, bookID, 2, true)
	require.ErrorIs(t, err, ErrNotFound)

	// Hide, then independent disposition writes, sync, and correction keep it.
	_, err = store.SetBookHidden(ctx, alice.ID, bookID, 2, true)
	require.NoError(t, err)
	detail, err := store.GetBookDetail(ctx, alice.ID, bookID)
	require.NoError(t, err)
	applied, err = store.TransitionBookDisposition(ctx, alice.ID, "de", bookID, detail.DispositionRevision, domain.BookDispositionToRead)
	require.NoError(t, err)
	require.True(t, applied)
	_, err = store.ReconcileCatalogueEntry(ctx, alice.ID, connection.ID, "visibility-entry", "Refreshed", "Other", "it")
	require.NoError(t, err)

	detail, err = store.GetBookDetail(ctx, alice.ID, bookID)
	require.NoError(t, err)
	assert.True(t, detail.Hidden)
	assert.EqualValues(t, 3, detail.VisibilityRevision)
	assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)

	// Unhide leaves disposition untouched.
	_, err = store.SetBookHidden(ctx, alice.ID, bookID, 3, false)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, mustBookDisposition(t, store, alice.ID, bookID))
}

func TestBookVisibilityScopesBrowseCounts(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	owner, err := store.CreateUser(ctx, "visibility-browse", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Browse catalog", URL: "https://browse-visibility.example/opds"})
	require.NoError(t, err)
	visible, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "visible", "Visible", "A", "de")
	require.NoError(t, err)
	hiddenDE, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "hidden-de", "Hidden de", "A", "de")
	require.NoError(t, err)
	hiddenUnknown, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Hidden unknown", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	for _, id := range []string{hiddenDE.Book.ID, hiddenUnknown.ID} {
		_, err = store.SetBookHidden(ctx, owner.ID, id, 0, true)
		require.NoError(t, err)
	}

	def, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, def.Items, 1)
	assert.Equal(t, visible.Book.ID, def.Items[0].Book.ID)
	assert.Equal(t, 1, def.ScopeTotal)
	assert.Equal(t, 1, def.HiddenCount)
	assert.Equal(t, 3, def.AllCount, "the complete collection size stays truthful")

	shown, err := store.ListMyBooksBrowseWithVisibility(ctx, owner.ID, "", "de", "", false, true, 0, 25)
	require.NoError(t, err)
	assert.Len(t, shown.Items, 2)
	assert.Equal(t, 2, shown.ScopeTotal)

	unknownDefault, err := store.ListMyBooksBrowse(ctx, owner.ID, "", domain.LanguageUnknown, "", false, 0, 25)
	require.NoError(t, err)
	assert.Empty(t, unknownDefault.Items, "Needs language omits Hidden by default")
	assert.Equal(t, 1, unknownDefault.HiddenCount)
	unknownShown, err := store.ListMyBooksBrowseWithVisibility(ctx, owner.ID, "", domain.LanguageUnknown, "", false, true, 0, 25)
	require.NoError(t, err)
	assert.Len(t, unknownShown.Items, 1)

	// Evidence listing still contains Hidden Books, flagged.
	all, err := store.ListMyBooksWithEvidence(ctx, owner.ID)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestBookVisibilityConcurrentWritesSerialize(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "visibility-race", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Race catalog", URL: "https://race-visibility.example/opds"})
	require.NoError(t, err)
	created, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "race", "Race", "A", "de")
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make([]error, 2)
	for i, hide := range []bool{true, false} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = store.SetBookHidden(ctx, owner.ID, created.Book.ID, 0, hide)
		}()
	}
	wg.Wait()
	// Unhide at revision 0 is a no-op on a visible Book, so at most one
	// request can be stale and the final revision never skips ahead.
	_, revision, err := store.GetBookVisibility(ctx, owner.ID, created.Book.ID)
	require.NoError(t, err)
	assert.LessOrEqual(t, revision, int64(1))
	for _, err := range results {
		if err != nil {
			assert.ErrorIs(t, err, ErrStaleBookVisibility)
		}
	}
}
