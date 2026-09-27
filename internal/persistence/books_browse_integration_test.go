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
	authorBook := create(alice.ID, "The Lost Daughter", domain.LanguageChosen, "it", domain.MetadataProvenanceCatalogueSync)
	_, err = store.UpdateBookMetadata(ctx, alice.ID, authorBook.ID, authorBook.Title, "Elena Ferrante", authorBook.LanguageState, authorBook.LanguageTag)
	require.NoError(t, err)
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

	result, err := store.ListMyBooksBrowse(ctx, alice.ID, "Donaudampf", "", "", false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	assert.Equal(t, 2, len(result.Items))
	assert.Equal(t, 34, result.AllCount)
	for _, item := range result.Items {
		assert.Equal(t, alice.ID, item.Book.OwnerID, "cross-owner item leaked: %+v", item)
	}
	filtered, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "it", string(domain.BookDispositionInbox), false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 27, filtered.Total, "new catalogue books start in Inbox")
	assert.Equal(t, 27, filtered.ScopeTotal, "language scope count should include all dispositions")
	assert.Len(t, filtered.DispositionCounts, 1, "workflow counts should include every populated bucket")
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, authorBook.ID, domain.BookDispositionSetAside))
	filtered, err = store.ListMyBooksBrowse(ctx, alice.ID, "", "it", string(domain.BookDispositionSetAside), false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 27, filtered.ScopeTotal)
	require.Len(t, filtered.Items, 1)
	assert.Equal(t, authorBook.ID, filtered.Items[0].Book.ID)
	assert.Equal(t, domain.BookDispositionSetAside, filtered.Items[0].Disposition)
	search, err := store.ListMyBooksBrowse(ctx, alice.ID, "lost", "it", string(domain.BookDispositionSetAside), false, 0, 25)
	require.NoError(t, err)
	require.Len(t, search.Items, 1, "search should compose with disposition filtering")
	assert.Equal(t, authorBook.ID, search.Items[0].Book.ID)
	inboxPage, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "it", string(domain.BookDispositionInbox), false, 25, 25)
	require.NoError(t, err)
	assert.Equal(t, 26, inboxPage.Total)
	assert.Len(t, inboxPage.Items, 1, "paging should compose with disposition filtering")
	otherOwner, err := store.ListMyBooksBrowse(ctx, bob.ID, "", "it", string(domain.BookDispositionSetAside), false, 0, 25)
	require.NoError(t, err)
	assert.Zero(t, otherOwner.Total, "another owner's disposition results leaked")

	literal, err := store.ListMyBooksBrowse(ctx, alice.ID, "%", "", "", false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 1, literal.Total)
	assert.Equal(t, percent.ID, literal.Items[0].Book.ID, "LIKE wildcard was not literal")
	authorResult, err := store.ListMyBooksBrowse(ctx, alice.ID, "ferrante", "", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, authorResult.Items, 1)
	assert.Equal(t, authorBook.ID, authorResult.Items[0].Book.ID)

	deResult, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "DE", "", false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 3, deResult.Total, "German filter")
	require.Len(t, deResult.Counts, 3)
	assert.Equal(t, "de", deResult.Counts[0].Tag)
	assert.Equal(t, 3, deResult.Counts[0].Count)
	assert.Equal(t, "it", deResult.Counts[1].Tag)
	assert.Equal(t, 27, deResult.Counts[1].Count)
	assert.Equal(t, "unknown", deResult.Counts[2].Tag)
	assert.Equal(t, 4, deResult.Counts[2].Count)
	unknown, err := store.ListMyBooksBrowse(ctx, alice.ID, "Donaudampf", domain.LanguageUnknown, "", false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 1, unknown.Total)
	assert.Equal(t, metadataDampf.ID, unknown.Items[0].Book.ID, "unknown combined filter")

	page, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "", "", false, 25, 25)
	require.NoError(t, err)
	assert.Equal(t, 34, page.Total)
	assert.Equal(t, 9, len(page.Items), "page slice")
	tieIDs := []string{tieA.ID, tieB.ID}
	orderedTies, err := store.ListMyBooksBrowse(ctx, alice.ID, "Same title", "", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, orderedTies.Items, 2)
	assert.Equal(t, minString(tieIDs[0], tieIDs[1]), orderedTies.Items[0].Book.ID)
	assert.Equal(t, maxString(tieIDs[0], tieIDs[1]), orderedTies.Items[1].Book.ID)

	err = store.RemoveBookFromMyBooks(ctx, alice.ID, percent.ID)
	require.NoError(t, err)
	remaining, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "", "", false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 33, remaining.AllCount)
	assert.Equal(t, 33, remaining.Total)
	assert.False(t, strings.Contains(fmt.Sprint(remaining.Items), percent.ID), "removed membership remained in browse")
}

func TestMyBooksBrowseUsesKeysetBatchesAcrossLargeCollection(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "browse-keyset", false)
	require.NoError(t, err)
	for i := range 260 {
		input, createErr := domain.NewBook(owner.ID, fmt.Sprintf("Batch Book %03d", i), domain.MetadataProvenanceCatalogueSync, domain.LanguageUnknown, "")
		require.NoError(t, createErr)
		_, createErr = store.CreateBook(ctx, input)
		require.NoError(t, createErr)
	}

	page, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "", "", false, 250, 20)
	require.NoError(t, err)
	assert.Equal(t, 260, page.Total)
	require.Len(t, page.Items, 10)
	assert.Equal(t, "Batch Book 250", page.Items[0].Book.Title)
	assert.Equal(t, "Batch Book 259", page.Items[9].Book.Title)

	filtered, err := store.ListMyBooksBrowse(ctx, owner.ID, "Batch Book 25", "", "", false, 5, 5)
	require.NoError(t, err)
	assert.Equal(t, 10, filtered.Total)
	require.Len(t, filtered.Items, 5)
	assert.Equal(t, "Batch Book 255", filtered.Items[0].Book.Title)
	assert.Equal(t, "Batch Book 259", filtered.Items[4].Book.Title)
}

func TestMyBooksBrowseUsesOneWorkflowBucketForFiltersAndCounts(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "browse-buckets", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "bucket-german")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	current, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)

	toRead, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", string(domain.BookDispositionToRead), false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 1, toRead.Total, "current reading belongs in the To Read filter")
	require.Len(t, toRead.Items, 1)
	assert.Equal(t, book.ID, toRead.Items[0].Book.ID)
	assert.Equal(t, domain.MyBookBucketCurrentReading, toRead.Items[0].WorkflowBucket())
	assert.Equal(t, 1, dispositionCountForBucket(toRead.DispositionCounts, domain.BookDispositionToRead))
	all, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, all.Items, 1)
	assert.True(t, all.Items[0].IsCurrentReading)
	assert.Equal(t, domain.MyBookBucketCurrentReading, all.Items[0].WorkflowBucket())
	assert.Equal(t, domain.BookDispositionToRead, all.Items[0].Disposition, "visible bucket does not rewrite persisted disposition")

	_, err = store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, current.SnapshotID)
	require.NoError(t, err)
	read, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", true, 0, 25)
	require.NoError(t, err)
	require.Len(t, read.Items, 1)
	assert.Equal(t, domain.MyBookBucketRead, read.Items[0].WorkflowBucket())
	assert.Equal(t, 1, read.ReadCount)
	assert.Equal(t, domain.BookDispositionSetAside, read.Items[0].Disposition, "Read is a projection and preserves the underlying disposition")

	// Reconsideration retains append-only history but moves the visible Book to
	// To Read. Setting it aside again projects that same Book back into Read.
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	read, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", true, 0, 25)
	require.NoError(t, err)
	assert.Empty(t, read.Items, "To Read history is not also in Read")
	assert.Zero(t, read.ReadCount)
	toRead, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", string(domain.BookDispositionToRead), false, 0, 25)
	require.NoError(t, err)
	require.Len(t, toRead.Items, 1)
	assert.Equal(t, domain.MyBookBucketToRead, toRead.Items[0].WorkflowBucket())
	assert.Equal(t, 1, toRead.Items[0].CompletionCount, "reconsideration retains history")
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionSetAside))
	read, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", true, 0, 25)
	require.NoError(t, err)
	require.Len(t, read.Items, 1)
	assert.Equal(t, domain.MyBookBucketRead, read.Items[0].WorkflowBucket())
	assert.Equal(t, 1, read.ReadCount)
	all, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, all.Items, 1, "a Book with history appears once in All")
	assert.Equal(t, book.ID, all.Items[0].Book.ID)

	italian, italianSource, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "it", "bucket-italian")
	makeAnalyzedToReadBook(t, ctx, store, italian, italianSource)
	italianCurrent, err := store.StartCurrentReading(ctx, owner.ID, "it", italian.ID)
	require.NoError(t, err)
	italianHistoryInput, err := domain.NewBook(owner.ID, "Italian historical book", domain.MetadataProvenanceCatalogueSync, domain.LanguageChosen, "it")
	require.NoError(t, err)
	italianHistoryBook, err := store.CreateBook(ctx, italianHistoryInput)
	require.NoError(t, err)
	_, err = store.ImportPreviouslyRead(ctx, owner.ID, italianHistoryBook.ID)
	require.NoError(t, err)
	inbox, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "it", string(domain.BookDispositionInbox), false, 0, 25)
	require.NoError(t, err)
	assert.Empty(t, inbox.Items, "previously read Book leaves Inbox")
	read, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "it", "", true, 0, 25)
	require.NoError(t, err)
	require.Len(t, read.Items, 1)
	assert.Equal(t, italianHistoryBook.ID, read.Items[0].Book.ID)
	assert.Equal(t, 1, read.ReadCount)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	for language, bookID := range map[string]string{"de": book.ID, "it": italian.ID} {
		currentBooks, browseErr := store.ListMyBooksBrowse(ctx, owner.ID, "", language, "", false, 0, 25)
		require.NoError(t, browseErr)
		if language == "de" {
			assert.Zero(t, currentBooks.ReadCount, "current reading with history is not also counted in Read")
			readBooks, readErr := store.ListMyBooksBrowse(ctx, owner.ID, "", language, "", true, 0, 25)
			require.NoError(t, readErr)
			assert.Empty(t, readBooks.Items, "current reading with history is excluded from Read")
		} else {
			readBooks, readErr := store.ListMyBooksBrowse(ctx, owner.ID, "", language, "", true, 0, 25)
			require.NoError(t, readErr)
			require.Len(t, readBooks.Items, 1, "Read history is scoped to the selected language")
			assert.Equal(t, italianHistoryBook.ID, readBooks.Items[0].Book.ID)
			assert.Equal(t, 1, readBooks.ReadCount)
		}
		var currentCount int
		for _, item := range currentBooks.Items {
			if item.WorkflowBucket() == domain.MyBookBucketCurrentReading {
				currentCount++
				assert.Equal(t, bookID, item.Book.ID)
			}
		}
		assert.Equal(t, 1, currentCount, "current reading remains independent in %s", language)
	}
	stopped, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.NoError(t, store.StopCurrentReading(ctx, owner.ID, "de", book.ID, stopped.SnapshotID))
	toRead, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", string(domain.BookDispositionToRead), false, 0, 25)
	require.NoError(t, err)
	require.Len(t, toRead.Items, 1)
	assert.Equal(t, domain.MyBookBucketToRead, toRead.Items[0].WorkflowBucket(), "stopping preserves To Read intent")

	stopped, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	replacement, replacementSource, _ := createReadingFixture(t, ctx, store, owner.ID, "bucket-switch")
	makeAnalyzedToReadBook(t, ctx, store, replacement, replacementSource)
	switched, err := store.SwitchCurrentReading(ctx, owner.ID, "de", replacement.ID, book.ID, stopped.SnapshotID)
	require.NoError(t, err)
	all, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	buckets := make(map[string]domain.MyBookBucket, len(all.Items))
	for _, item := range all.Items {
		buckets[item.Book.ID] = item.WorkflowBucket()
	}
	assert.Equal(t, domain.MyBookBucketToRead, buckets[book.ID], "switch returns the former current Book to To Read")
	assert.Equal(t, domain.MyBookBucketCurrentReading, buckets[replacement.ID])
	require.NoError(t, store.SetAsideCurrentReading(ctx, owner.ID, "de", replacement.ID, switched.SnapshotID))
	all, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	for _, item := range all.Items {
		if item.Book.ID == replacement.ID {
			assert.Equal(t, domain.BookDispositionSetAside, item.Disposition)
			assert.Equal(t, domain.MyBookBucketSetAside, item.WorkflowBucket(), "setting aside ends current reading and preserves the explicit disposition")
		}
	}
	assert.NotEmpty(t, italianCurrent.SnapshotID)
}

func dispositionCountForBucket(counts []DispositionCount, disposition domain.BookDisposition) int {
	for _, count := range counts {
		if count.Disposition == disposition {
			return count.Count
		}
	}
	return 0
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
