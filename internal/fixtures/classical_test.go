package fixtures

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureClassicalCatalogCarriesConstituentBooksAndAWholeWork(t *testing.T) {
	catalog, err := NewClassicalCatalog()
	require.NoError(t, err)

	assert.Equal(t, classical.CatalogCounts{Works: 2, Authors: 2, Units: 4}, catalog.Counts())

	iliad, ok := catalog.Work("fixture-iliad")
	require.True(t, ok)
	assert.Equal(t, "Iliad", iliad.Title)
	assert.Equal(t, "Homer", iliad.Author)
	assert.Equal(t, "Fixture reviewed edition", iliad.EditionLabel)
	assert.Equal(t, "μῆνιν ἄειδε θεὰ Πηληϊάδεω Ἀχιλῆος", iliad.Opening)
	assert.Equal(t, 3, iliad.ConstituentBooks)
	assert.False(t, iliad.WholeWork)
	require.Len(t, iliad.Units, 3)
	assert.Equal(t, []int{1, 2, 3}, []int{iliad.Units[0].ConstituentBook, iliad.Units[1].ConstituentBook, iliad.Units[2].ConstituentBook})

	hellenica, ok := catalog.Work("fixture-hellenica")
	require.True(t, ok)
	assert.True(t, hellenica.WholeWork, "the second Work is one whole-work unit")
	require.Len(t, hellenica.Units, 1)
	assert.Zero(t, hellenica.Units[0].ConstituentBook)
}

func TestFixtureClassicalAddCreatesMetadataOnlyToReadBooks(t *testing.T) {
	store := NewStore()
	report, err := store.AddClassicalReadingUnits(context.Background(), OwnerID, []string{
		"fixture-iliad-1", "fixture-iliad-2", "fixture-iliad-3",
	})
	require.NoError(t, err)
	assert.Equal(t, 3, report.CreatedBooks())

	owned, err := store.ListOwnedClassicalReadingUnits(context.Background(), OwnerID)
	require.NoError(t, err)
	require.Len(t, owned, 3)
	assert.Equal(t, []string{"fixture-iliad-1", "fixture-iliad-2", "fixture-iliad-3"},
		[]string{owned[0].ReadingUnitKey, owned[1].ReadingUnitKey, owned[2].ReadingUnitKey})

	for _, unit := range owned {
		detail, err := store.GetBookDetail(context.Background(), OwnerID, unit.BookID)
		require.NoError(t, err)
		assert.Equal(t, "Iliad", detail.Book.Title)
		assert.Equal(t, "Homer", detail.Book.Author)
		assert.Equal(t, classical.Language, detail.Book.LanguageTag)
		assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
		require.NotNil(t, detail.ReadingUnit)
		assert.Equal(t, unit.ReadingUnitKey, detail.ReadingUnit.ReadingUnitKey)
		assert.Equal(t, "fixture-iliad", detail.ReadingUnit.WorkKey)
	}

	supported, err := store.ListSupportedLanguages(context.Background())
	require.NoError(t, err)
	assert.Contains(t, supported, domain.SupportedLanguage{Language: classical.Language, DisplayName: classical.LanguageDisplayName})
}

func TestFixtureClassicalReAddReopensTheSameBooks(t *testing.T) {
	store := NewStore()
	first, err := store.AddClassicalReadingUnits(context.Background(), OwnerID, []string{"fixture-iliad-1", "fixture-iliad-2"})
	require.NoError(t, err)
	require.Len(t, first.Books, 2)
	bookID := first.Books[0].BookID

	hidden, err := store.SetBookHidden(context.Background(), OwnerID, bookID, 0, true)
	require.NoError(t, err)
	require.True(t, hidden)

	again, err := store.AddClassicalReadingUnits(context.Background(), OwnerID, []string{"fixture-iliad-1", "fixture-iliad-2", "fixture-iliad-2"})
	require.NoError(t, err)
	assert.Zero(t, again.CreatedBooks())
	assert.Equal(t, []string{bookID, first.Books[1].BookID}, []string{again.Books[0].BookID, again.Books[1].BookID})
	visible, _, err := store.GetBookVisibility(context.Background(), OwnerID, bookID)
	require.NoError(t, err)
	assert.False(t, visible, "re-adding reopens a hidden Book")
}

func TestFixtureClassicalRemoveEndsMembershipAndAddRestores(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	first, err := store.AddClassicalReadingUnits(ctx, OwnerID, []string{"fixture-iliad-1"})
	require.NoError(t, err)
	bookID := first.Books[0].BookID

	require.NoError(t, store.RemoveBookFromMyBooks(ctx, OwnerID, bookID))
	_, err = store.GetBookDetail(ctx, OwnerID, bookID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	owned, err := store.ListOwnedClassicalReadingUnits(ctx, OwnerID)
	require.NoError(t, err)
	assert.Empty(t, owned)
	languages, err := store.ListStudyLanguages(ctx, OwnerID)
	require.NoError(t, err)
	assert.NotContains(t, languages, domain.StudyLanguage{Language: classical.Language, DisplayName: classical.LanguageDisplayName})

	again, err := store.AddClassicalReadingUnits(ctx, OwnerID, []string{"fixture-iliad-1"})
	require.NoError(t, err)
	assert.Zero(t, again.CreatedBooks(), "re-adding restores rather than creates")
	assert.Equal(t, bookID, again.Books[0].BookID)
	detail, err := store.GetBookDetail(ctx, OwnerID, bookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
}

func TestFixtureRemoveRefusesCurrentAndUnknownBooks(t *testing.T) {
	store := NewStore()
	ctx := context.Background()

	err := store.RemoveBookFromMyBooks(ctx, OwnerID, BookID)
	require.ErrorIs(t, err, domain.ErrBookIsCurrentReading)
	_, err = store.GetBookDetail(ctx, OwnerID, BookID)
	require.NoError(t, err, "the current Book stays in My Books")

	err = store.RemoveBookFromMyBooks(ctx, OwnerID, "not-a-book")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestFixtureRemoveWorkSkipsTheCurrentBookAndRefusesUnknownWorks(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	report, err := store.AddClassicalReadingUnits(ctx, OwnerID, []string{"fixture-iliad-1", "fixture-iliad-2"})
	require.NoError(t, err)
	require.Len(t, report.Books, 2)
	currentID, otherID := report.Books[0].BookID, report.Books[1].BookID
	store.currentReadings[fixtureGoalKey(OwnerID, classical.Language)] = domain.CurrentReading{
		OwnerID: OwnerID, Language: classical.Language, BookID: currentID,
	}

	removal, err := store.RemoveWorkFromMyBooks(ctx, OwnerID, "fixture-iliad")
	require.NoError(t, err)
	assert.Equal(t, 1, removal.RemovedBooks)
	require.NotNil(t, removal.CurrentReading)
	assert.Equal(t, currentID, removal.CurrentReading.ID)
	_, err = store.GetBookDetail(ctx, OwnerID, currentID)
	require.NoError(t, err, "the current Book stays")
	_, err = store.GetBookDetail(ctx, OwnerID, otherID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	_, err = store.RemoveWorkFromMyBooks(ctx, OwnerID, " ")
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = store.RemoveWorkFromMyBooks(ctx, OwnerID, "not-a-work")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestFixtureClassicalToReadProjectionMakesOneBookReadyForTheChooser(t *testing.T) {
	store := NewStore()
	report, err := store.AddClassicalReadingUnits(context.Background(), OwnerID, []string{"fixture-iliad-1"})
	require.NoError(t, err)
	bookID := report.Books[0].BookID

	preparation := ClassicalPreparation{}
	handle, err := preparation.SubmitClassicalPreparation(context.Background(), OwnerID, bookID)
	require.NoError(t, err)
	assert.True(t, handle.Complete)

	detail, err := store.GetBookDetail(context.Background(), OwnerID, bookID)
	require.NoError(t, err)
	require.NotNil(t, detail.Acquired)
	assert.Equal(t, fixtureClassicalCorpusID("fixture-iliad-1"), detail.Acquired.CorpusID)
	assert.True(t, detail.Classification().AnalysisInEffect())
}

func TestFixtureClassicalAddRejectsIncompleteRequests(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	_, err := store.AddClassicalReadingUnits(ctx, "  ", []string{"fixture-iliad-1"})
	require.ErrorContains(t, err, "owner is required")
	_, err = store.AddClassicalReadingUnits(ctx, OwnerID, nil)
	require.ErrorContains(t, err, "at least one Reading unit")
	_, err = store.AddClassicalReadingUnits(ctx, OwnerID, []string{"fixture-iliad-1", "  "})
	require.ErrorContains(t, err, "key is required")
}

func TestFixtureClassicalInsightsSpreadConstituentBooksAcrossBands(t *testing.T) {
	insights := ClassicalInsights{}
	for key, want := range map[string]domain.CoverageBand{
		"fixture-iliad-1": domain.CoverageBand99Plus,
		"fixture-iliad-2": domain.CoverageBand97To99,
		"fixture-iliad-3": domain.CoverageBandBelow95,
	} {
		coverage, err := insights.Coverage(context.Background(), OwnerID, fixtureClassicalCorpusID(key))
		require.NoError(t, err)
		assert.Equal(t, want, domain.CoverageBandFor(coverage.KnownTokenCount, coverage.AnalyzableTokenCount), key)
	}
}
