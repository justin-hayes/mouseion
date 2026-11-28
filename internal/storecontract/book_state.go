package storecontract

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bookStateScenarios cover Book workflow disposition and the previously read
// import. Revisions are read back from the store rather than assumed, so a
// scenario holds whatever revision an adapter starts from.
func bookStateScenarios() []Scenario {
	scenarios := dispositionScenarios()
	scenarios = append(scenarios, bucketScenarios()...)
	scenarios = append(scenarios, importScenarios()...)
	scenarios = append(scenarios, removalScenarios()...)
	return append(scenarios, epubAcquisitionFailureScenarios()...)
}

// removalScenarios pin Remove from My Books: it ends membership only, and it
// refuses the Book a reading is on.
func removalScenarios() []Scenario {
	return []Scenario{{
		Name: "Remove ends My Books membership and changes nothing else",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{Vocabulary: defaultVocabulary})
			detail := requireDetail(t, h, owner, bookID)
			completion, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.NoError(t, err)
			before := requireDetail(t, h, owner, bookID)
			assert.Equal(t, bookID, before.Book.ID)
			assert.Equal(t, detail.Book.Title, before.Book.Title)

			require.NoError(t, h.BookState().RemoveBookFromMyBooks(owner, bookID))
			_, err = h.BookState().GetBookDetail(owner, bookID)
			require.ErrorIs(t, err, domain.ErrNotFound, "Remove ends membership")

			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Removal contract catalog")
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{{
				Entry:  domain.CatalogueIndexEntry{EntryIdentifier: "removed-read-book", Title: before.Book.Title, Author: before.Book.Author, LanguageTag: Language, EPUBHref: "https://catalog.example/removed.epub"},
				BookID: bookID,
			}})
			page, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{})
			require.NoError(t, err)
			require.Len(t, page.Entries, 1)
			assert.Empty(t, page.Entries[0].BookID, "the Catalog offers Add once the Book leaves My Books")
			restored, err := h.Catalogue().AddCatalogueEntry(owner, connectionID, "removed-read-book")
			require.NoError(t, err)
			assert.Equal(t, bookID, restored.Book.ID, "Add restores the same Book ID")
			restoredDetail := requireDetail(t, h, owner, bookID)
			assert.Equal(t, domain.BookDispositionToRead, restoredDetail.Disposition)
			require.NotNil(t, restoredDetail.Acquired, "the restored Book keeps its analysis evidence")
			assert.True(t, restoredDetail.Classification().AnalysisInEffect())
			assert.Equal(t, 1, restoredDetail.CompletionCount, "Remove keeps completion history")
			assert.Equal(t, domain.ReadingCompletionPreviouslyRead, restoredDetail.LatestCompletionSource)
			require.NotNil(t, restoredDetail.LatestCompletionAt)
			assert.True(t, restoredDetail.LatestCompletionAt.Equal(completion.CompletedAt), "the recorded completion is unchanged")
		},
	}, {
		Name: "Remove refuses the current Book and leaves it in My Books",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID, reading := startReading(t, h, defaultVocabulary)

			err := h.BookState().RemoveBookFromMyBooks(owner, bookID)
			require.ErrorIs(t, err, domain.ErrBookIsCurrentReading)

			detail := requireDetail(t, h, owner, bookID)
			assert.Equal(t, bookID, detail.Book.ID)
			current, err := h.Store().GetCurrentReading(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, reading.BookID, current.BookID, "the refused Remove never ends the reading")
		},
	}, {
		Name: "Remove is owner-scoped",
		Run: func(t *testing.T, h Harness) {
			bookID := seedBook(t, h, BookSeed{})
			err := h.BookState().RemoveBookFromMyBooks(h.Seeds().OtherOwner(), bookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
			requireDetail(t, h, h.Seeds().Owner(), bookID)
		},
	}}
}

// epubAcquisitionFailureScenarios pin the durable reason a failed EPUB
// acquisition leaves on a To Read Book, and its removal after success.
func epubAcquisitionFailureScenarios() []Scenario {
	return []Scenario{{
		Name: "an EPUB acquisition failure is recorded against the Book and cleared on success",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{MetadataOnly: true})
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Acquisition catalog")
			record := domain.EPUBAcquisitionFailureRecord{
				OwnerID: owner, BookID: bookID,
				ConnectionID: connectionID, EntryIdentifier: "entry",
				Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed,
			}
			require.NoError(t, h.BookState().RecordEPUBAcquisitionFailure(record))

			detail := requireDetail(t, h, owner, bookID)
			require.NotNil(t, detail.EPUBAcquisition)
			assert.Equal(t, domain.BookPreparationStageFetch, detail.EPUBAcquisition.Stage)
			assert.Equal(t, domain.BookPreparationCodeSourceDownloadFailed, detail.EPUBAcquisition.Code)
			classification := detail.Classification()
			assert.Equal(t, domain.AcquisitionFetchFailed, classification.Acquisition.State)
			assert.Equal(t, domain.BookUnavailable, classification.Evidence)
			assert.Equal(t, domain.RecoveryRetryAcquisition, classification.Recovery)

			_, err := h.BookState().GetBookDetail(h.Seeds().OtherOwner(), bookID)
			require.ErrorIs(t, err, domain.ErrNotFound, "the failure never crosses owners")

			require.NoError(t, h.BookState().ClearEPUBAcquisitionFailure(owner, bookID))
			detail = requireDetail(t, h, owner, bookID)
			assert.Nil(t, detail.EPUBAcquisition)
			assert.Equal(t, domain.AcquisitionNone, detail.Classification().Acquisition.State)
		},
	}, {
		Name: "a source-stage EPUB acquisition failure offers no retry",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{MetadataOnly: true})
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Acquisition catalog")
			require.NoError(t, h.BookState().RecordEPUBAcquisitionFailure(domain.EPUBAcquisitionFailureRecord{
				OwnerID: owner, BookID: bookID,
				ConnectionID: connectionID, EntryIdentifier: "entry",
				Stage: domain.BookPreparationStageSource, Code: domain.BookPreparationCodeSourceOversize,
			}))
			detail := requireDetail(t, h, owner, bookID)
			classification := detail.Classification()
			assert.Equal(t, domain.AcquisitionSourceOversize, classification.Acquisition.State)
			assert.Equal(t, domain.RecoveryNone, classification.Recovery)
		},
	}}
}

// seedBook seeds an analyzed To Read Book for the learner under test, unless
// the seed names the retired Inbox state.
func seedBook(t *testing.T, h Harness, seed BookSeed) string {
	t.Helper()
	return h.Seeds().SeedBook(t, h.Seeds().Owner(), seed)
}

// requireDetail returns the Book as the learner sees it, failing on error.
func requireDetail(t *testing.T, h Harness, owner, bookID string) domain.MyBook {
	t.Helper()
	detail, err := h.BookState().GetBookDetail(owner, bookID)
	require.NoError(t, err)
	return detail
}

func dispositionScenarios() []Scenario {
	return []Scenario{{
		Name: "a disposition change applies at the expected revision and a verified replay is a no-op",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedReadBook(t, h)
			initial := requireDetail(t, h, owner, bookID).DispositionRevision
			applied, err := h.BookState().TransitionBookDisposition(owner, bookID, initial, domain.BookDispositionToRead)
			require.NoError(t, err)
			assert.True(t, applied)
			applied, err = h.BookState().TransitionBookDisposition(owner, bookID, initial, domain.BookDispositionToRead)
			require.NoError(t, err)
			assert.False(t, applied)
			detail := requireDetail(t, h, owner, bookID)
			assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
			assert.Equal(t, initial+1, detail.DispositionRevision)
		},
	}, {
		Name: "a stale disposition choice cannot reverse a newer one",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedReadBook(t, h)
			initial := requireDetail(t, h, owner, bookID).DispositionRevision
			applied, err := h.BookState().TransitionBookDisposition(owner, bookID, initial, domain.BookDispositionToRead)
			require.NoError(t, err)
			require.True(t, applied)
			applied, err = h.BookState().TransitionBookDisposition(owner, bookID, initial+1, domain.BookDispositionInbox)
			require.NoError(t, err)
			require.True(t, applied)
			_, err = h.BookState().TransitionBookDisposition(owner, bookID, initial, domain.BookDispositionToRead)
			require.ErrorIs(t, err, domain.ErrStaleBookDisposition)
			detail := requireDetail(t, h, owner, bookID)
			assert.Equal(t, domain.BookDispositionInbox, detail.Disposition)
			assert.Equal(t, initial+2, detail.DispositionRevision)
			assert.Equal(t, domain.MyBookBucketRead, detail.WorkflowBucket(), "recorded history keeps the reversed Book Read")
		},
	}, {
		Name: "disposition is owner-scoped",
		Run: func(t *testing.T, h Harness) {
			owner, other := h.Seeds().Owner(), h.Seeds().OtherOwner()
			bookID := seedBook(t, h, BookSeed{})
			initial := requireDetail(t, h, owner, bookID).DispositionRevision
			_, err := h.BookState().TransitionBookDisposition(other, bookID, initial, domain.BookDispositionToRead)
			require.ErrorIs(t, err, domain.ErrNotFound)
			detail := requireDetail(t, h, owner, bookID)
			assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
			assert.Equal(t, initial, detail.DispositionRevision)
		},
	}}
}

// bucketScenarios pin the retired Inbox: every active member has one of the
// three visible roles, a member with no chosen role is rejected, the stores
// refuse to write that state for a Book without reading history, and the
// stored Inbox value stays allowed for a read Book and the current reading.
func bucketScenarios() []Scenario {
	return []Scenario{{
		Name: "every active member is Currently reading, To Read or Read",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			toRead := requireDetail(t, h, owner, seedBook(t, h, BookSeed{}))
			assert.Equal(t, domain.MyBookBucketToRead, toRead.WorkflowBucket())

			readID := seedReadBook(t, h)
			read := requireDetail(t, h, owner, readID)
			assert.Equal(t, domain.MyBookBucketRead, read.WorkflowBucket())

			started, _ := startReading(t, h, defaultVocabulary)
			current := requireDetail(t, h, owner, started)
			assert.Equal(t, domain.MyBookBucketCurrentReading, current.WorkflowBucket())
		},
	}, {
		Name: "a member with no chosen role is rejected and no store write can create it",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()

			// Only the seeding can build the retired state, bypassing the
			// write path; the domain read model reports it as a member with no
			// visible role.
			legacy := seedBook(t, h, BookSeed{Inbox: true})
			detail := requireDetail(t, h, owner, legacy)
			assert.Empty(t, detail.WorkflowBucket(), "the retired Inbox has no visible role")
			require.ErrorIs(t, detail.ValidateMembership(), domain.ErrBookNotChosen)

			// Neither store write may recreate it: a To Read Book without
			// reading history is refused and left To Read.
			bookID := seedBook(t, h, BookSeed{})
			initial := requireDetail(t, h, owner, bookID).DispositionRevision
			require.ErrorIs(t, h.BookState().SetBookDisposition(owner, bookID, domain.BookDispositionInbox), domain.ErrBookNotChosen)
			applied, err := h.BookState().TransitionBookDisposition(owner, bookID, initial, domain.BookDispositionInbox)
			require.ErrorIs(t, err, domain.ErrBookNotChosen)
			assert.False(t, applied)
			refused := requireDetail(t, h, owner, bookID)
			assert.Equal(t, domain.BookDispositionToRead, refused.Disposition, "the refusal leaves the Book To Read")
			assert.Equal(t, initial, refused.DispositionRevision)
		},
	}, {
		Name: "storing Inbox is allowed for a Book with reading history and for the current reading",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()

			// Reading history keeps the stored Inbox value a Read membership.
			read := seedBook(t, h, BookSeed{})
			_, err := h.BookState().ImportPreviouslyRead(owner, read)
			require.NoError(t, err)
			require.NoError(t, h.BookState().SetBookDisposition(owner, read, domain.BookDispositionInbox))
			assert.Equal(t, domain.MyBookBucketRead, requireDetail(t, h, owner, read).WorkflowBucket(), "recorded history keeps the Book Read")

			// The current reading keeps its role over the stored value.
			started, _ := startReading(t, h, defaultVocabulary)
			initial := requireDetail(t, h, owner, started).DispositionRevision
			applied, err := h.BookState().TransitionBookDisposition(owner, started, initial, domain.BookDispositionInbox)
			require.NoError(t, err)
			require.True(t, applied)
			require.NoError(t, h.BookState().SetBookDisposition(owner, started, domain.BookDispositionInbox))
			current := requireDetail(t, h, owner, started)
			assert.Equal(t, domain.MyBookBucketCurrentReading, current.WorkflowBucket(), "the current reading keeps its role")
			stillCurrent, err := h.Store().GetCurrentReading(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, started, stillCurrent.BookID, "the allowed write never ends the reading")
		},
	}}
}

// seedReadBook creates a Book with one completion, so it is a valid Read
// member of My Books.
func seedReadBook(t *testing.T, h Harness) string {
	t.Helper()
	bookID := seedBook(t, h, BookSeed{})
	require.NoError(t, h.BookState().MarkBookPreviouslyRead(h.Seeds().Owner(), bookID))
	return bookID
}

func importScenarios() []Scenario {
	return []Scenario{{
		Name: "previously-read import records a chosen-language Book that is not the current reading",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			completion, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.NoError(t, err)
			assert.Equal(t, domain.ReadingCompletionPreviouslyRead, completion.Source)
			assert.Equal(t, bookID, completion.BookID)
			assert.Equal(t, Language, completion.Language)
		},
	}, {
		Name: "previously-read import replays the same completion",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			first, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.NoError(t, err)
			second, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.NoError(t, err)
			assert.True(t, second.CompletedAt.Equal(first.CompletedAt))
			assert.Equal(t, first.Source, second.Source)
		},
	}, {
		Name: "marking an unstarted To Read Book previously read records it and moves it to Read",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			require.NoError(t, h.BookState().MarkBookPreviouslyRead(owner, bookID))
			detail := requireDetail(t, h, owner, bookID)
			assert.Equal(t, domain.BookDispositionInbox, detail.Disposition, "marking clears the To Read disposition")
			assert.Equal(t, domain.MyBookBucketRead, detail.WorkflowBucket())
			assert.Equal(t, 1, detail.CompletionCount)
			assert.Equal(t, domain.ReadingCompletionPreviouslyRead, detail.LatestCompletionSource)

			require.NoError(t, h.BookState().MarkBookPreviouslyRead(owner, bookID), "a replay is idempotent")
			assert.Equal(t, 1, requireDetail(t, h, owner, bookID).CompletionCount, "a replay adds no second history row")
		},
	}, {
		Name: "marking a Book that is not an unstarted To Read Book is refused",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			inbox := seedBook(t, h, BookSeed{Inbox: true})
			require.ErrorIs(t, h.BookState().MarkBookPreviouslyRead(owner, inbox), domain.ErrPreviouslyReadIneligible)
			detail := requireDetail(t, h, owner, inbox)
			require.ErrorIs(t, detail.ValidateMembership(), domain.ErrBookNotChosen, "a refused assertion records no history")

			started, _ := startReading(t, h, defaultVocabulary)
			require.ErrorIs(t, h.BookState().MarkBookPreviouslyRead(owner, started), domain.ErrBookIsCurrentReading)
			assert.Zero(t, requireDetail(t, h, owner, started).CompletionCount, "a started Book keeps no previous-read history")
		},
	}, {
		Name: "marking a re-read To Read Book previously read is refused",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			_, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.NoError(t, err)
			require.ErrorIs(t, h.BookState().MarkBookPreviouslyRead(owner, bookID), domain.ErrPreviouslyReadIneligible, "a Book with history was already read")
			assert.Equal(t, domain.BookDispositionToRead, requireDetail(t, h, owner, bookID).Disposition, "the refusal leaves the re-read Book To Read")
		},
	}, {
		Name: "marking the current reading previously read is refused",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID, _ := startReading(t, h, defaultVocabulary)
			require.ErrorIs(t, h.BookState().MarkBookPreviouslyRead(owner, bookID), domain.ErrBookIsCurrentReading)
		},
	}, {
		Name: "book detail reports whether a Book ever held the current reading",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			fresh := seedBook(t, h, BookSeed{})
			assert.False(t, requireDetail(t, h, owner, fresh).EverCurrentReading, "an unstarted Book never held the reading")

			currentBook, _ := startReading(t, h, defaultVocabulary)
			assert.True(t, requireDetail(t, h, owner, currentBook).EverCurrentReading, "the current reading counts as started")

			require.NoError(t, h.Store().EndCurrentReading(owner, Language))
			assert.False(t, requireDetail(t, h, owner, currentBook).IsCurrentReading)
		},
	}, {
		Name: "previously-read import refuses the current reading",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID, _ := startReading(t, h, defaultVocabulary)
			_, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
		},
	}, {
		Name: "previously-read import refuses a Book whose language is not chosen",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{LanguageUnknown: true})
			_, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
		},
	}, {
		Name: "previously-read import is owner-scoped",
		Run: func(t *testing.T, h Harness) {
			bookID := seedBook(t, h, BookSeed{})
			_, err := h.BookState().ImportPreviouslyRead(h.Seeds().OtherOwner(), bookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
		},
	}}
}
