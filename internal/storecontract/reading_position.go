package storecontract

import (
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var positionEpoch = time.Date(2026, time.November, 28, 9, 0, 0, 0, time.UTC)

// requirePosition asserts the Book's Reading position has the fraction.
func requirePosition(t *testing.T, h Harness, bookID string, fraction float64) *domain.ReadingPosition {
	t.Helper()
	detail := requireDetail(t, h, h.Seeds().Owner(), bookID)
	require.NotNil(t, detail.ReadingPosition, "the Book has a Reading position")
	assert.InDelta(t, fraction, detail.ReadingPosition.Fraction, 1e-9)
	assert.Equal(t, domain.PositionSourceManual, detail.ReadingPosition.Source)
	return detail.ReadingPosition
}

// readingPositionScenarios pin the Reading position: the latest Position report
// by recorded time, insertion order breaking ties, and a signal only.
func readingPositionScenarios() []Scenario {
	return []Scenario{{
		Name: "a Book has no Reading position until one is recorded",
		Run: func(t *testing.T, h Harness) {
			bookID := seedBook(t, h, BookSeed{})
			assert.Nil(t, requireDetail(t, h, h.Seeds().Owner(), bookID).ReadingPosition)
		},
	}, {
		Name: "the latest Position report is the Reading position, even when lower",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			require.NoError(t, h.Positions().RecordManualPosition(owner, bookID, 0.5, positionEpoch))
			position := requirePosition(t, h, bookID, 0.5)
			assert.True(t, position.RecordedAt.Equal(positionEpoch))
			require.NoError(t, h.Positions().RecordManualPosition(owner, bookID, 0.25, positionEpoch.Add(time.Hour)))
			requirePosition(t, h, bookID, 0.25)
			// A report recorded at an earlier time never overtakes a later one.
			require.NoError(t, h.Positions().RecordManualPosition(owner, bookID, 0.9, positionEpoch.Add(-time.Hour)))
			requirePosition(t, h, bookID, 0.25)
		},
	}, {
		Name: "reports recorded at the same time go to insertion order",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			require.NoError(t, h.Positions().RecordManualPosition(owner, bookID, 0.1, positionEpoch))
			require.NoError(t, h.Positions().RecordManualPosition(owner, bookID, 0.7, positionEpoch))
			require.NoError(t, h.Positions().RecordManualPosition(owner, bookID, 0.3, positionEpoch))
			requirePosition(t, h, bookID, 0.3)
		},
	}, {
		Name: "a Book outside Reading or another owner's refuses a position",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			require.ErrorIs(t, h.Positions().RecordManualPosition(h.Seeds().OtherOwner(), bookID, 0.5, positionEpoch), domain.ErrNotFound)
			require.NoError(t, h.BookState().RemoveBookFromMyBooks(owner, bookID))
			require.ErrorIs(t, h.Positions().RecordManualPosition(owner, bookID, 0.5, positionEpoch), domain.ErrNotFound)
		},
	}, {
		Name: "recording a position changes no disposition and starts no reading",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{Vocabulary: defaultVocabulary})
			before := requireDetail(t, h, owner, bookID)
			require.NoError(t, h.Positions().RecordManualPosition(owner, bookID, 1, positionEpoch))
			after := requireDetail(t, h, owner, bookID)
			assert.Equal(t, before.Disposition, after.Disposition)
			assert.Equal(t, before.DispositionRevision, after.DispositionRevision)
			assert.False(t, after.IsCurrentReading)
			assert.Zero(t, after.CompletionCount, "100% does not finish the Book")
			started, err := h.Store().GetCurrentReading(owner, Language)
			require.NoError(t, err)
			assert.Empty(t, started.BookID, "no Current reading was started")

			current, _ := startReading(t, h, defaultVocabulary)
			require.NoError(t, h.Positions().RecordManualPosition(owner, bookID, 0.4, positionEpoch))
			requireCurrent(t, h, current)
		},
	}, {
		Name: "Read next, End, Finish and Read again leave reports unchanged",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID, _ := startReading(t, h, defaultVocabulary)
			require.NoError(t, h.Positions().RecordManualPosition(owner, bookID, 0.6, positionEpoch))
			requirePosition(t, h, bookID, 0.6) // Read next came first

			require.NoError(t, h.Store().EndCurrentReading(owner, Language))
			requirePosition(t, h, bookID, 0.6)

			_, err := h.Store().StartCurrentReading(owner, Language, bookID)
			require.NoError(t, err)
			_, err = h.Store().FinishCurrentReading(owner, Language)
			require.NoError(t, err)
			requirePosition(t, h, bookID, 0.6)

			detail := requireDetail(t, h, owner, bookID)
			applied, err := h.BookState().TransitionBookDisposition(owner, bookID, detail.DispositionRevision, domain.BookDispositionToRead)
			require.NoError(t, err)
			require.True(t, applied)
			requirePosition(t, h, bookID, 0.6)
		},
	}, {
		Name: "Remove keeps positions and Add brings them back",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			book := requireDetail(t, h, owner, bookID).Book
			require.NoError(t, h.Positions().RecordManualPosition(owner, bookID, 0.45, positionEpoch))
			require.NoError(t, h.BookState().RemoveBookFromMyBooks(owner, bookID))
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Position catalog")
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{{
				Entry:  domain.CatalogueIndexEntry{EntryIdentifier: "position-book", Title: book.Title, Author: book.Author, LanguageTag: Language, EPUBHref: "https://catalog.example/position.epub"},
				BookID: bookID,
			}})
			_, err := h.Catalogue().AddCatalogueEntry(owner, connectionID, "position-book")
			require.NoError(t, err)
			requirePosition(t, h, bookID, 0.45)
		},
	}}
}
