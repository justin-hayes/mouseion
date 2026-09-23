//go:build integration

package persistence

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

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
	retrieve, _, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, first.ID, "entry-1", true)
	require.NoError(t, err)
	assert.True(t, retrieve)
	cover, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverPending, cover.State)
	assert.Equal(t, first.ID, cover.SelectedConnectionID)
	advertisedAt := cover.AdvertisedAt

	// A second candidate joins the same pending generation. Re-advertising the
	// provisional source refreshes its retrieval without dropping the fallback.
	retrieve, _, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, second.ID, "entry-2", true)
	require.NoError(t, err)
	assert.True(t, retrieve)
	retrieve, refreshedAt, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, first.ID, "entry-1", true)
	require.NoError(t, err)
	assert.True(t, retrieve)
	assert.Equal(t, advertisedAt, refreshedAt)
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, second.ID, "entry-2", advertisedAt, "image/png", 4, 6, "sha256:second", []byte("second-image")))
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, first.ID, "entry-1", advertisedAt, "image/png", 4, 6, "sha256:first", []byte("first-image")))

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
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, loserConnection, loserSource, winner.AdvertisedAt, "image/png", 4, 6, "sha256:loser", []byte("loser-image")))
	afterLoser, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, winner.SelectedConnectionID, afterLoser.SelectedConnectionID)

	// A failed replacement preserves the available image but records the failure.
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, bookID, winner.SelectedConnectionID, winner.SelectedSourceIdentifier, winner.AdvertisedAt, "replacement failed"))
	failed, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, failed.State)
	assert.Equal(t, "replacement failed", failed.FailureReason)

	// Only the selected entry may explicitly remove the retained image.
	_, _, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, loserConnection, loserSource, false)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, mustCoverState(t, store, owner.ID, bookID))
	retrieve, _, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, winner.SelectedConnectionID, winner.SelectedSourceIdentifier, false)
	require.NoError(t, err)
	assert.False(t, retrieve)
	assert.Equal(t, domain.BookCoverNone, mustCoverState(t, store, owner.ID, bookID))
}

func TestBookCoverConcurrentInitialAdvertisementsChooseOneCandidate(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "concurrent cover store", store.Close)

	owner, err := store.CreateUser(ctx, "concurrent-cover-owner", false)
	require.NoError(t, err)
	first, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "First", URL: "https://first.example/opds"})
	require.NoError(t, err)
	second, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Second", URL: "https://second.example/opds"})
	require.NoError(t, err)
	reconciled, err := store.ReconcileCatalogueEntry(ctx, owner.ID, first.ID, "entry-1", "Title", "", "de")
	require.NoError(t, err)

	type result struct {
		retrieve bool
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, candidate := range []struct {
		connectionID, source string
	}{
		{first.ID, "entry-1"},
		{second.ID, "entry-2"},
	} {
		go func() {
			<-start
			retrieve, _, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, reconciled.Book.ID, candidate.connectionID, candidate.source, true)
			results <- result{retrieve: retrieve, err: err}
		}()
	}
	close(start)
	var retrieved int
	for range 2 {
		outcome := <-results
		require.NoError(t, outcome.err)
		if outcome.retrieve {
			retrieved++
		}
	}
	assert.GreaterOrEqual(t, retrieved, 1)

	cover, err := store.GetBookCoverForRetrieval(ctx, owner.ID, reconciled.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverPending, cover.State)
	assert.Contains(t, []string{first.ID, second.ID}, cover.SelectedConnectionID)
	advertisedAt := cover.AdvertisedAt

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
			assert.NoError(t, store.SaveBookCover(ctx, owner.ID, reconciled.Book.ID, candidate.connectionID, candidate.source, advertisedAt, "image/png", 4, 6, candidate.hash, candidate.body))
		}()
	}
	wg.Wait()

	winner, err := store.GetBookCoverForRetrieval(ctx, owner.ID, reconciled.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, winner.State)
	assert.Contains(t, []string{first.ID, second.ID}, winner.SelectedConnectionID)
}

func TestBookCoverCandidateFailuresDoNotCloseInitialGeneration(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "candidate failure cover store", store.Close)

	owner, err := store.CreateUser(ctx, "candidate-failure-owner", false)
	require.NoError(t, err)
	first, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "First", URL: "https://first.example/opds"})
	require.NoError(t, err)
	second, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Second", URL: "https://second.example/opds"})
	require.NoError(t, err)

	advertise := func(t *testing.T, title, firstSource, secondSource string) (string, time.Time) {
		t.Helper()
		reconciled, err := store.ReconcileCatalogueEntry(ctx, owner.ID, first.ID, title, title, "", "de")
		require.NoError(t, err)
		retrieve, _, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, reconciled.Book.ID, first.ID, firstSource, true)
		require.NoError(t, err)
		require.True(t, retrieve)
		initial, err := store.GetBookCoverForRetrieval(ctx, owner.ID, reconciled.Book.ID)
		require.NoError(t, err)
		retrieve, _, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, reconciled.Book.ID, second.ID, secondSource, true)
		require.NoError(t, err)
		require.True(t, retrieve)
		return reconciled.Book.ID, initial.AdvertisedAt
	}

	bookID, advertisedAt := advertise(t, "Failure before success", "entry-a", "entry-b")
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, bookID, first.ID, "entry-a", advertisedAt, "first candidate failed"))
	pending, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverPending, pending.State)
	assert.Equal(t, "first candidate failed", pending.FailureReason)
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, second.ID, "entry-b", advertisedAt, "image/png", 4, 6, "second", []byte("second-image")))
	winner, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, winner.State)
	assert.Equal(t, second.ID, winner.SelectedConnectionID)
	assert.Empty(t, winner.FailureReason)

	bookID, advertisedAt = advertise(t, "Success before failure", "entry-c", "entry-d")
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, second.ID, "entry-d", advertisedAt, "image/png", 4, 6, "second", []byte("second-image")))
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, bookID, first.ID, "entry-c", advertisedAt, "late first failure"))
	winner, err = store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, winner.State)
	assert.Equal(t, second.ID, winner.SelectedConnectionID)
	assert.Empty(t, winner.FailureReason)

	bookID, advertisedAt = advertise(t, "Every candidate fails", "entry-e", "entry-f")
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, bookID, first.ID, "entry-e", advertisedAt, "first candidate failed"))
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, bookID, second.ID, "entry-f", advertisedAt, "second candidate failed"))
	failed, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverUnavailable, failed.State)
	assert.Equal(t, "second candidate failed", failed.FailureReason)

	// A River retry from the same generation can recover through a non-
	// provisional candidate after every initial attempt failed.
	retryBookID, retryAdvertisedAt := advertise(t, "Same-generation retry", "entry-g", "entry-h")
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, retryBookID, first.ID, "entry-g", retryAdvertisedAt, "first candidate failed"))
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, retryBookID, second.ID, "entry-h", retryAdvertisedAt, "second candidate failed"))
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, retryBookID, second.ID, "entry-h", retryAdvertisedAt, "image/png", 4, 6, "retry-fallback", []byte("retry-fallback-image")))
	recovered, err := store.GetBookCoverForRetrieval(ctx, owner.ID, retryBookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, recovered.State)
	assert.Equal(t, second.ID, recovered.SelectedConnectionID)
	assert.Equal(t, "entry-h", recovered.SelectedSourceIdentifier)

	// A later reconciliation starts a fresh generation and can recover without
	// requiring a learner-facing action.
	retrieve, retryAt, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, first.ID, "entry-e", true)
	require.NoError(t, err)
	assert.True(t, retrieve)
	assert.True(t, retryAt.After(advertisedAt))
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, first.ID, "entry-e", retryAt, "image/png", 4, 6, "retry", []byte("retry-image")))
	recovered, err = store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, recovered.State)
	assert.Equal(t, first.ID, recovered.SelectedConnectionID)
}

func TestBookCoverPendingConnectionDeletionDoesNotStrandGeneration(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "deleted candidate cover store", store.Close)

	owner, err := store.CreateUser(ctx, "deleted-candidate-owner", false)
	require.NoError(t, err)
	first, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "First", URL: "https://first.example/opds"})
	require.NoError(t, err)
	second, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Second", URL: "https://second.example/opds"})
	require.NoError(t, err)
	firstBook, err := store.ReconcileCatalogueEntry(ctx, owner.ID, first.ID, "entry-first", "First book", "", "de")
	require.NoError(t, err)
	secondBook, err := store.ReconcileCatalogueEntry(ctx, owner.ID, first.ID, "entry-second", "Second book", "", "de")
	require.NoError(t, err)

	_, firstAt, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, firstBook.Book.ID, first.ID, "entry-first", true)
	require.NoError(t, err)
	_, _, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, firstBook.Book.ID, second.ID, "entry-first-fallback", true)
	require.NoError(t, err)
	_, _, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, secondBook.Book.ID, first.ID, "entry-second", true)
	require.NoError(t, err)

	require.NoError(t, store.DeleteOpdsConnection(ctx, owner.ID, first.ID))
	pending, err := store.GetBookCoverForRetrieval(ctx, owner.ID, firstBook.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverPending, pending.State)
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, firstBook.Book.ID, second.ID, "entry-first-fallback", firstAt, "image/png", 4, 6, "fallback", []byte("fallback-image")))
	recovered, err := store.GetBookCoverForRetrieval(ctx, owner.ID, firstBook.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, recovered.State)
	assert.Equal(t, second.ID, recovered.SelectedConnectionID)

	unavailable, err := store.GetBookCoverForRetrieval(ctx, owner.ID, secondBook.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverUnavailable, unavailable.State)
	assert.Equal(t, "cover retrieval source was deleted", unavailable.FailureReason)
}

func TestBookCoverConnectionDeletionFencesCompletions(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "fenced cover store", store.Close)

	owner, err := store.CreateUser(ctx, "fenced-cover-owner", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Catalog", URL: "https://catalog.example/opds"})
	require.NoError(t, err)
	reconciled, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "entry-1", "Title", "", "de")
	require.NoError(t, err)
	bookID := reconciled.Book.ID

	_, initialAt, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, connection.ID, "entry-1", true)
	require.NoError(t, err)
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, connection.ID, "entry-1", initialAt, "image/png", 4, 6, "initial", []byte("initial-image")))
	_, replacementAt, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, connection.ID, "entry-1", true)
	require.NoError(t, err)

	// Deletion wins the race: an in-flight replacement from the retired source
	// cannot replace the retained image or add a failure diagnostic.
	require.NoError(t, store.DeleteOpdsConnection(ctx, owner.ID, connection.ID))
	retrieve, _, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, connection.ID, "entry-1", true)
	require.NoError(t, err)
	assert.False(t, retrieve)
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, connection.ID, "entry-1", replacementAt, "image/png", 4, 6, "stale", []byte("stale-image")))
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, bookID, connection.ID, "entry-1", replacementAt, "stale failure"))
	cover, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, cover.State)
	assert.Empty(t, cover.FailureReason)
	resource, err := store.GetBookCoverResource(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, "initial", resource.ContentHash)

	// Completion wins the race: deleting the source afterward preserves the
	// replacement and its provenance.
	second, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Second", URL: "https://second.example/opds"})
	require.NoError(t, err)
	secondBook, err := store.ReconcileCatalogueEntry(ctx, owner.ID, second.ID, "entry-2", "Second title", "", "de")
	require.NoError(t, err)
	_, secondInitialAt, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, secondBook.Book.ID, second.ID, "entry-2", true)
	require.NoError(t, err)
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, secondBook.Book.ID, second.ID, "entry-2", secondInitialAt, "image/png", 4, 6, "initial", []byte("initial-image")))
	_, secondReplacementAt, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, secondBook.Book.ID, second.ID, "entry-2", true)
	require.NoError(t, err)
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, secondBook.Book.ID, second.ID, "entry-2", secondReplacementAt, "image/png", 4, 6, "replacement", []byte("replacement-image")))
	require.NoError(t, store.DeleteOpdsConnection(ctx, owner.ID, second.ID))
	resource, err = store.GetBookCoverResource(ctx, owner.ID, secondBook.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, "replacement", resource.ContentHash)
}

func TestBookCoverDeletionAndCompletionDoNotDeadlock(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "concurrent cover deletion store", store.Close)

	owner, err := store.CreateUser(ctx, "concurrent-cover-deletion-owner", false)
	require.NoError(t, err)
	for i := 0; i < 8; i++ {
		connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: fmt.Sprintf("Catalog %d", i), URL: fmt.Sprintf("https://catalog.example/%d", i)})
		require.NoError(t, err)
		reconciled, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "entry-"+connection.ID, "Title "+connection.ID, "", "de")
		require.NoError(t, err)
		bookID := reconciled.Book.ID
		_, advertisedAt, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, connection.ID, "entry-"+connection.ID, true)
		require.NoError(t, err)

		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			results <- store.SaveBookCover(ctx, owner.ID, bookID, connection.ID, "entry-"+connection.ID, advertisedAt, "image/png", 4, 6, "concurrent", []byte("concurrent-image"))
		}()
		go func() {
			<-start
			results <- store.DeleteOpdsConnection(ctx, owner.ID, connection.ID)
		}()
		close(start)
		for range 2 {
			require.NoError(t, <-results)
		}

		cover, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
		require.NoError(t, err)
		assert.Contains(t, []string{domain.BookCoverAvailable, domain.BookCoverUnavailable}, cover.State)
	}
}

func TestBookCoverFinalPendingConnectionDeletionSettlesInEitherOrder(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "cover deletion order store", store.Close)

	owner, err := store.CreateUser(ctx, "cover-deletion-order-owner", false)
	require.NoError(t, err)
	first, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "First", URL: "https://first.example/opds"})
	require.NoError(t, err)
	second, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Second", URL: "https://second.example/opds"})
	require.NoError(t, err)
	third, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Third", URL: "https://third.example/opds"})
	require.NoError(t, err)
	fourth, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Fourth", URL: "https://fourth.example/opds"})
	require.NoError(t, err)

	firstBook, err := store.ReconcileCatalogueEntry(ctx, owner.ID, first.ID, "entry-first", "First book", "", "de")
	require.NoError(t, err)
	secondBook, err := store.ReconcileCatalogueEntry(ctx, owner.ID, third.ID, "entry-second", "Second book", "", "de")
	require.NoError(t, err)
	for _, candidate := range []struct {
		bookID, connectionID, source string
	}{
		{firstBook.Book.ID, first.ID, "entry-first"},
		{firstBook.Book.ID, second.ID, "entry-first-fallback"},
		{secondBook.Book.ID, third.ID, "entry-second"},
		{secondBook.Book.ID, fourth.ID, "entry-second-fallback"},
	} {
		_, _, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, candidate.bookID, candidate.connectionID, candidate.source, true)
		require.NoError(t, err)
	}

	// Remove the provisional source first for one Book and the fallback first
	// for the other. Both must settle only after their final candidate is gone.
	require.NoError(t, store.DeleteOpdsConnection(ctx, owner.ID, first.ID))
	assert.Equal(t, domain.BookCoverPending, mustCoverState(t, store, owner.ID, firstBook.Book.ID))
	require.NoError(t, store.DeleteOpdsConnection(ctx, owner.ID, second.ID))
	assert.Equal(t, domain.BookCoverUnavailable, mustCoverState(t, store, owner.ID, firstBook.Book.ID))

	require.NoError(t, store.DeleteOpdsConnection(ctx, owner.ID, fourth.ID))
	assert.Equal(t, domain.BookCoverPending, mustCoverState(t, store, owner.ID, secondBook.Book.ID))
	require.NoError(t, store.DeleteOpdsConnection(ctx, owner.ID, third.ID))
	assert.Equal(t, domain.BookCoverUnavailable, mustCoverState(t, store, owner.ID, secondBook.Book.ID))
}

func TestBookCoverStaleRetrievalCompletionsAreIgnored(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "stale book cover store", store.Close)

	owner, err := store.CreateUser(ctx, "stale-cover-owner", false)
	require.NoError(t, err)
	first, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "First", URL: "https://first.example/opds"})
	require.NoError(t, err)
	second, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Second", URL: "https://second.example/opds"})
	require.NoError(t, err)
	reconciled, err := store.ReconcileCatalogueEntry(ctx, owner.ID, first.ID, "entry-1", "Title", "", "de")
	require.NoError(t, err)
	bookID := reconciled.Book.ID

	// The old worker may finish after reconciliation explicitly removes its
	// source. Neither a success nor a failure may recreate that absent state.
	retrieve, _, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, first.ID, "entry-1", true)
	require.NoError(t, err)
	assert.True(t, retrieve)
	initial, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	initialAdvertisedAt := initial.AdvertisedAt
	_, _, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, first.ID, "entry-1", false)
	require.NoError(t, err)
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, first.ID, "entry-1", initialAdvertisedAt, "image/png", 4, 6, "stale-success", []byte("stale-image")))
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, bookID, first.ID, "entry-1", initialAdvertisedAt, "stale failure"))
	cover, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverNone, cover.State)
	assert.Empty(t, cover.FailureReason)

	// A newer fallback source owns the pending row. Completion and failure from
	// the old source must not overwrite its presentation or diagnostics.
	retrieve, _, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, bookID, second.ID, "entry-2", true)
	require.NoError(t, err)
	assert.True(t, retrieve)
	fallback, err := store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, first.ID, "entry-1", initialAdvertisedAt, "image/png", 4, 6, "stale-fallback-success", []byte("stale-fallback")))
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, bookID, first.ID, "entry-1", initialAdvertisedAt, "stale fallback failure"))
	cover, err = store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverPending, cover.State)
	assert.Equal(t, second.ID, cover.SelectedConnectionID)
	assert.Equal(t, "entry-2", cover.SelectedSourceIdentifier)
	assert.Empty(t, cover.FailureReason)

	require.NoError(t, store.SaveBookCover(ctx, owner.ID, bookID, second.ID, "entry-2", fallback.AdvertisedAt, "image/png", 4, 6, "fallback", []byte("fallback-image")))
	cover, err = store.GetBookCoverForRetrieval(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, cover.State)
	assert.Equal(t, second.ID, cover.SelectedConnectionID)
}

func TestBookCoverSelectedRefreshAdvancesGenerationAndPreservesImage(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "selected refresh cover store", store.Close)

	owner, err := store.CreateUser(ctx, "selected-refresh-owner", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Catalog", URL: "https://catalog.example/opds"})
	require.NoError(t, err)
	reconciled, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "entry-1", "Title", "", "de")
	require.NoError(t, err)

	retrieve, _, err := store.RecordBookCoverAdvertisement(ctx, owner.ID, reconciled.Book.ID, connection.ID, "entry-1", true)
	require.NoError(t, err)
	require.True(t, retrieve)
	initial, err := store.GetBookCoverForRetrieval(ctx, owner.ID, reconciled.Book.ID)
	require.NoError(t, err)
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, reconciled.Book.ID, connection.ID, "entry-1", initial.AdvertisedAt, "image/png", 4, 6, "initial", []byte("initial-image")))

	// Reconciliation while the old worker is still live advances the fence but
	// keeps the validated image available until the replacement succeeds.
	retrieve, _, err = store.RecordBookCoverAdvertisement(ctx, owner.ID, reconciled.Book.ID, connection.ID, "entry-1", true)
	require.NoError(t, err)
	require.True(t, retrieve)
	current, err := store.GetBookCoverForRetrieval(ctx, owner.ID, reconciled.Book.ID)
	require.NoError(t, err)
	require.True(t, current.AdvertisedAt.After(initial.AdvertisedAt))
	require.NoError(t, store.SaveBookCover(ctx, owner.ID, reconciled.Book.ID, connection.ID, "entry-1", initial.AdvertisedAt, "image/png", 4, 6, "stale", []byte("stale-image")))
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, owner.ID, reconciled.Book.ID, connection.ID, "entry-1", initial.AdvertisedAt, "stale failure"))

	cover, err := store.GetBookCoverForRetrieval(ctx, owner.ID, reconciled.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, cover.State)
	assert.Empty(t, cover.FailureReason)
	resource, err := store.GetBookCoverResource(ctx, owner.ID, reconciled.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, "initial", resource.ContentHash)

	require.NoError(t, store.SaveBookCover(ctx, owner.ID, reconciled.Book.ID, connection.ID, "entry-1", current.AdvertisedAt, "image/png", 4, 6, "replacement", []byte("replacement-image")))
	resource, err = store.GetBookCoverResource(ctx, owner.ID, reconciled.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, "replacement", resource.ContentHash)
}

func mustCoverState(t *testing.T, store *PostgresStore, owner, bookID string) string {
	t.Helper()
	cover, err := store.GetBookCoverForRetrieval(context.Background(), owner, bookID)
	require.NoError(t, err)
	return cover.State
}
