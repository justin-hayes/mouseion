package fixtures

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkCatalogueEntryPreviouslyReadRefusesIncompleteIdentityAndMissingBooks(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, err := store.MarkCatalogueEntryPreviouslyRead(ctx, "", "", "")
	require.ErrorContains(t, err, "identity is incomplete")

	store.catalogueEntries["fixture-connection"] = append(store.catalogueEntries["fixture-connection"], domain.CatalogueIndexEntry{
		ConnectionID: "fixture-connection", EntryIdentifier: "no-language", Title: "No Language", EPUBHref: "https://fixture.invalid/no-language.epub",
	})
	_, err = store.MarkCatalogueEntryPreviouslyRead(ctx, OwnerID, "fixture-connection", "no-language")
	require.ErrorContains(t, err, "no language")

	store.aliases = append(store.aliases, domain.BookAlias{
		OwnerID: OwnerID, BookID: "missing-book", ConnectionID: "fixture-connection",
		AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "missing-entry",
	})
	_, err = store.MarkCatalogueEntryPreviouslyRead(ctx, OwnerID, "fixture-connection", "missing-entry")
	require.ErrorIs(t, err, errNotFound)
}

func TestMarkCatalogueEntryPreviouslyReadRestoresASourceBook(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	const sourceBookID = "fixture-source-back-book"
	for i := range store.books {
		if store.books[i].Source.ID == routeDiffersBookID {
			store.books[i].BookID = sourceBookID
		}
	}
	store.catalogueEntries["fixture-connection"] = append(store.catalogueEntries["fixture-connection"], domain.CatalogueIndexEntry{
		ConnectionID: "fixture-connection", EntryIdentifier: "source-entry", Title: "Source Entry", LanguageTag: "de", EPUBHref: "https://fixture.invalid/source-entry.epub",
	})
	store.aliases = append(store.aliases, domain.BookAlias{
		OwnerID: OwnerID, BookID: sourceBookID, ConnectionID: "fixture-connection",
		AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "source-entry",
	})
	read, err := store.MarkCatalogueEntryPreviouslyRead(ctx, OwnerID, "fixture-connection", "source-entry")
	require.NoError(t, err)
	assert.Equal(t, sourceBookID, read.Book.ID)
	assert.True(t, read.Changed)
	detail, err := store.GetBookDetail(ctx, OwnerID, sourceBookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, detail.Disposition)
	assert.Equal(t, 1, detail.CompletionCount)
}

func TestMarkCatalogueEntryPreviouslyReadRefusesTheCurrentReading(t *testing.T) {
	store := NewStore()
	_, err := store.MarkCatalogueEntryPreviouslyRead(context.Background(), OwnerID, "fixture-browser-sync-connection", "fixture-browser-second")
	require.ErrorIs(t, err, domain.ErrBookIsCurrentReading)
}

func TestMarkClassicalReadingUnitPreviouslyReadRefusesIncompleteIdentityAndUnknownUnits(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	_, err := store.MarkClassicalReadingUnitPreviouslyRead(ctx, "", "")
	require.ErrorContains(t, err, "identity is incomplete")
	_, err = store.MarkClassicalReadingUnitPreviouslyRead(ctx, OwnerID, "no-such-unit")
	require.ErrorContains(t, err, "not current")
}

func TestMarkClassicalReadingUnitPreviouslyReadRefusesTheCurrentReading(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	report, err := store.AddClassicalReadingUnits(ctx, OwnerID, []string{"fixture-iliad-1"})
	require.NoError(t, err)
	bookID := report.Books[0].BookID
	store.currentReadings[fixtureGoalKey(OwnerID, "de")] = domain.CurrentReading{OwnerID: OwnerID, Language: "de", BookID: bookID}

	_, err = store.MarkClassicalReadingUnitPreviouslyRead(ctx, OwnerID, "fixture-iliad-1")
	require.ErrorIs(t, err, domain.ErrBookIsCurrentReading)
}

func TestMarkBookPreviouslyReadRefusesIneligibleBooks(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	require.ErrorIs(t, store.MarkBookPreviouslyRead(ctx, OwnerID, "fixture-metadata-only"), domain.ErrPreviouslyReadIneligible)

	store.snapshotLifecycles["fixture-started"] = domain.CurrentReadingSnapshotLifecycle{BookID: routeDiffersBookID}
	require.ErrorIs(t, store.MarkBookPreviouslyRead(ctx, OwnerID, routeDiffersBookID), domain.ErrPreviouslyReadIneligible)

	store.readingHistory["fixture-history"] = domain.ReadingCompletion{OwnerID: OwnerID, BookID: routeDiffersBookID, Source: domain.ReadingCompletionCurrentReading}
	require.ErrorIs(t, store.MarkBookPreviouslyRead(ctx, OwnerID, routeDiffersBookID), domain.ErrPreviouslyReadIneligible, "a To Read Book with history was already read")

	require.NoError(t, store.MarkBookPreviouslyRead(ctx, OwnerID, routeTieABookID))
	require.NoError(t, store.MarkBookPreviouslyRead(ctx, OwnerID, routeTieABookID), "an already Read Book is an idempotent replay")
}
