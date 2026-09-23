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

// TestBookCoverAdvertisementStateMachine proves the selected-source rules are
// transactional and owner-scoped: only the selected source can replace or
// remove an image, and concurrent initial candidates resolve to one winner.
func TestBookCoverAdvertisementStateMachine(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "book cover store", store.Close)

	owner, err := store.CreateUser(ctx, "cover-state-owner", false)
	require.NoError(t, err)
	first, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "First", URL: "https://first.example/opds"})
	require.NoError(t, err)
	second, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Second", URL: "https://second.example/opds"})
	require.NoError(t, err)
	reconciled, err := store.ReconcileCatalogueEntry(ctx, owner.ID, first.ID, "entry-1", "Title", "", "de")
	require.NoError(t, err)
	bookID := reconciled.Book.ID

	// No advertisement leaves the Book without a cover row.
	_, err = store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.ErrorIs(t, err, ErrNotFound)

	// A first advertisement creates a pending candidate and requests retrieval.
	retrieve, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, first.ID, "entry-1", true)
	require.NoError(t, err)
	assert.True(t, retrieve)
	cover, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverPending, cover.State)
	assert.Equal(t, first.ID, cover.SelectedConnectionID)

	// Concurrent successful candidates resolve atomically: exactly one source
	// wins and a later completion cannot displace it.
	var wg sync.WaitGroup
	for _, candidate := range []struct {
		connectionID, source, hash string
		body                       []byte
	}{
		{first.ID, "entry-1", "sha256:first", []byte("first-image")},
		{second.ID, "entry-2", "sha256:second", []byte("second-image")},
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, candidate.connectionID, candidate.source, "image/png", 4, 6, candidate.hash, candidate.body))
		}()
	}
	wg.Wait()

	winner, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	require.Equal(t, domain.BookCoverAvailable, winner.State)
	require.Contains(t, []string{first.ID, second.ID}, winner.SelectedConnectionID)
	resource, err := store.GetBookCoverResource(ctx, owner.ID, bookID)
	require.NoError(t, err)
	expectedHash := "sha256:first"
	if winner.SelectedConnectionID == second.ID {
		expectedHash = "sha256:second"
	}
	assert.Equal(t, expectedHash, resource.ContentHash, "retained image does not belong to the selected source")

	// The non-selected alias cannot replace the winner.
	loserConnection, loserSource := second.ID, "entry-2"
	if winner.SelectedConnectionID == second.ID {
		loserConnection, loserSource = first.ID, "entry-1"
	}
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, loserConnection, loserSource, "image/png", 4, 6, "sha256:loser", []byte("loser-image")))
	afterLoser, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, winner.SelectedConnectionID, afterLoser.SelectedConnectionID)

	// A failed replacement preserves the available image but records the failure.
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, bookID, "replacement failed"))
	failed, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, failed.State)
	assert.Equal(t, "replacement failed", failed.FailureReason)

	// Only the selected entry may explicitly remove the retained image.
	_, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, loserConnection, loserSource, false)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, mustCoverState(t, store, owner.ID, bookID))
	retrieve, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, winner.SelectedConnectionID, winner.SelectedSourceIdentifier, false)
	require.NoError(t, err)
	assert.False(t, retrieve)
	assert.Equal(t, domain.BookCoverNone, mustCoverState(t, store, owner.ID, bookID))
}

func mustCoverState(t *testing.T, store *PostgresStore, owner, bookID string) string {
	t.Helper()
	cover, err := store.GetBookCoverForRetrieval(context.Background(), owner, bookID)
	require.NoError(t, err)
	return cover.State
}
