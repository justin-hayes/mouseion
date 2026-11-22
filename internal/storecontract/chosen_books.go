package storecontract

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chosenBookScenarios pin ADR 0117's invariant: My Books holds only chosen
// Books, so every store operation that creates a Book or activates membership
// leaves the Book Currently reading, To Read or Read. No path creates a member
// the learner never chose, and none brings a removed Book back unchosen.
func chosenBookScenarios() []Scenario {
	return []Scenario{{
		Name: "every operation that creates a Book or activates membership leaves it chosen",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			h.Seeds().SeedClassicalCatalog(t)
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Chosen contract catalog")
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{
				{Entry: domain.CatalogueIndexEntry{EntryIdentifier: "chosen-add", Title: "Add", LanguageTag: Language, EPUBHref: "https://catalog.example/add.epub"}},
				{Entry: domain.CatalogueIndexEntry{EntryIdentifier: "chosen-read", Title: "Read", LanguageTag: Language, EPUBHref: "https://catalog.example/read.epub"}},
			})

			added, err := h.Catalogue().AddCatalogueEntry(owner, connectionID, "chosen-add")
			require.NoError(t, err)
			requireChosen(t, h, owner, added.Book.ID, "Catalog Add")

			read, err := h.Catalogue().MarkCatalogueEntryPreviouslyRead(owner, connectionID, "chosen-read")
			require.NoError(t, err)
			requireChosen(t, h, owner, read.Book.ID, "Catalog I've read this")

			report, err := h.Classical().AddClassicalReadingUnits(owner, []string{ClassicalUnitBook1Key})
			require.NoError(t, err)
			requireChosen(t, h, owner, report.Books[0].BookID, "Classical Add")

			classicalRead, err := h.Classical().MarkClassicalReadingUnitPreviouslyRead(owner, ClassicalUnitBook2Key)
			require.NoError(t, err)
			requireChosen(t, h, owner, classicalRead.Book.ID, "Classical I've read this")

			acquired, err := h.BookState().ResolveOrCreateBookForAcquisition(owner, "chosen-new-source", Language, "Acquired")
			require.NoError(t, err)
			requireChosen(t, h, owner, acquired, "EPUB ingest that creates a Book")
		},
	}, {
		Name: "EPUB ingest that matches a removed Book restores it To Read",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			legacy := seedBook(t, h, BookSeed{MetadataOnly: true, Inbox: true, SourceIdentifier: "chosen-legacy-source"})
			assert.Equal(t, domain.BookDispositionInbox, requireDetail(t, h, owner, legacy).Disposition, "the seeded Book is not To Read")
			require.NoError(t, h.BookState().RemoveBookFromMyBooks(owner, legacy))
			_, err := h.BookState().GetBookDetail(owner, legacy)
			require.ErrorIs(t, err, domain.ErrNotFound, "the Book is out of My Books")

			resolved, err := h.BookState().ResolveOrCreateBookForAcquisition(owner, "chosen-legacy-source", Language, "Legacy")
			require.NoError(t, err)
			assert.Equal(t, legacy, resolved, "ingest matches the existing Book by its source")
			requireChosen(t, h, owner, legacy, "EPUB ingest that re-activates a removed Book")
			assert.Equal(t, domain.BookDispositionToRead, requireDetail(t, h, owner, legacy).Disposition)
		},
	}}
}

// requireChosen fails unless the Book is in My Books as To Read (which includes
// the Currently reading Book) or Read through its completion history.
func requireChosen(t *testing.T, h Harness, owner, bookID, operation string) {
	t.Helper()
	detail, err := h.BookState().GetBookDetail(owner, bookID)
	require.NoError(t, err, "%s leaves the Book in My Books", operation)
	chosen := detail.Disposition == domain.BookDispositionToRead || detail.CompletionCount > 0
	require.True(t, chosen, "%s leaves the Book To Read or Read, got %s with %d completions", operation, detail.Disposition, detail.CompletionCount)
}
