package storecontract

import (
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The shared Classical release. Its keys are contract identifiers, so an
// adapter seeds the same small catalog instead of depending on its own fixture
// release.
const (
	ClassicalWorkKey       = "contract-iliad"
	ClassicalWorkTitle     = "Contract Iliad"
	ClassicalWorkAuthor    = "Contract Homer"
	ClassicalWorkGreek     = "Ἰλιάς"
	ClassicalUnitBook1Key  = "contract-iliad-1"
	ClassicalUnitBook2Key  = "contract-iliad-2"
	ClassicalWholeWorkKey  = "contract-hellenica"
	ClassicalWholeUnitKey  = "contract-hellenica-1"
	ClassicalWholeWorkName = "Contract Hellenica"
	ClassicalWholeAuthor   = "Contract Xenophon"
)

// ClassicalDefinitions is the small release every adapter reconciles for the
// Classical Add scenarios: one Work of two constituent books and one
// whole-work unit.
func ClassicalDefinitions() []classical.Definition {
	one, two := 1, 2
	return []classical.Definition{
		classicalDefinition(ClassicalWorkKey, ClassicalWorkTitle, ClassicalWorkAuthor, ClassicalWorkGreek,
			ClassicalUnitBook1Key, &one, 2, "1.1.1", "1.10.19", "contract-iliad-edition"),
		classicalDefinition(ClassicalWorkKey, ClassicalWorkTitle, ClassicalWorkAuthor, ClassicalWorkGreek,
			ClassicalUnitBook2Key, &two, 2, "2.1.1", "2.10.19", "contract-iliad-edition"),
		classicalDefinition(ClassicalWholeWorkKey, ClassicalWholeWorkName, ClassicalWholeAuthor, "Ἑλληνικά",
			ClassicalWholeUnitKey, nil, 1, "1.1.1", "1.1.10", "contract-hellenica-edition"),
	}
}

func classicalDefinition(workKey, title, author, greekTitle, key string, book *int, books int, citationStart, citationEnd, editionKey string) classical.Definition {
	definition := classical.Definition{
		Key:                   key,
		WorkKey:               workKey,
		WorkTitle:             title,
		WorkAuthor:            author,
		GreekTitle:            greekTitle,
		EditionKey:            editionKey,
		EditionLabel:          "Contract edition",
		Language:              classical.Language,
		ConstituentBook:       book,
		ConstituentBooks:      books,
		CitationStart:         citationStart,
		CitationEnd:           citationEnd,
		WordCount:             100,
		Opening:               "ἄρχεται ὁ λόγος.",
		Source:                classical.SourcePin{Layer: "contract-source", FileName: workKey + ".conllu", Size: 10, SHA256: strings.Repeat("a", 64), Format: classical.SourceFormatOGACoNLLU},
		InterpretationVersion: "contract/1",
		Revision:              1,
	}
	return definition
}

// classicalScenarios pin the Classical catalog Add transition: one Book per
// current Reading unit, an owner-scoped membership that reopens rather than
// duplicates, and a hidden Book made visible again without losing its
// disposition.
func classicalScenarios() []Scenario {
	return []Scenario{{
		Name: "classical add creates one metadata-only Inbox book per unit",
		Run: func(t *testing.T, h Harness) {
			h.Seeds().SeedClassicalCatalog(t)
			owner := h.Seeds().Owner()
			report, err := h.Classical().AddClassicalReadingUnits(owner, []string{ClassicalUnitBook1Key, ClassicalUnitBook2Key, ClassicalWholeUnitKey})
			require.NoError(t, err)
			assert.Equal(t, 3, report.CreatedBooks())

			owned := ownedClassicalUnits(t, h, owner)
			require.Len(t, owned, 3)
			first := requireDetail(t, h, owner, owned[ClassicalUnitBook1Key].BookID)
			assert.Equal(t, ClassicalWorkTitle, first.Book.Title)
			assert.Equal(t, ClassicalWorkAuthor, first.Book.Author)
			assert.Equal(t, domain.MetadataProvenanceOperatorRegistration, first.Book.MetadataProvenance)
			assert.Equal(t, domain.LanguageChosen, first.Book.LanguageState)
			assert.Equal(t, classical.Language, first.Book.LanguageTag)
			assert.Equal(t, domain.BookDispositionInbox, first.Disposition)
			require.NotNil(t, first.ReadingUnit)
			assert.Equal(t, ClassicalUnitBook1Key, first.ReadingUnit.ReadingUnitKey)
			assert.Equal(t, ClassicalWorkKey, first.ReadingUnit.WorkKey)
			assert.Equal(t, 1, first.ReadingUnit.ConstituentBook)
			assert.Equal(t, 2, first.ReadingUnit.ConstituentBooks)
			assert.Equal(t, ClassicalWorkGreek, first.ReadingUnit.GreekTitle)
			assert.Nil(t, first.Acquired, "Add downloads nothing and leaves the Book metadata-only")

			whole := requireDetail(t, h, owner, owned[ClassicalWholeUnitKey].BookID)
			assert.Equal(t, ClassicalWholeWorkName, whole.Book.Title)
			require.NotNil(t, whole.ReadingUnit)
			assert.Equal(t, ClassicalWholeWorkKey, whole.ReadingUnit.WorkKey)
			assert.Zero(t, whole.ReadingUnit.ConstituentBook, "a whole-work unit has no constituent position")
			assert.Equal(t, 1, whole.ReadingUnit.ConstituentBooks)

			again, err := h.Classical().AddClassicalReadingUnits(owner, []string{ClassicalUnitBook1Key, ClassicalUnitBook2Key, ClassicalWholeUnitKey, ClassicalUnitBook1Key})
			require.NoError(t, err)
			assert.Zero(t, again.CreatedBooks(), "re-adding reports existing Books")
			assert.Equal(t, owned[ClassicalUnitBook1Key].BookID, again.Books[0].BookID)
			assert.Equal(t, owned[ClassicalUnitBook2Key].BookID, again.Books[1].BookID)
			assert.Equal(t, owned[ClassicalWholeUnitKey].BookID, again.Books[2].BookID)
			assert.Len(t, ownedClassicalUnits(t, h, owner), 3, "a repeated key never creates a duplicate")

			_, err = h.Classical().AddClassicalReadingUnits(owner, []string{"not-a-current-unit"})
			require.ErrorContains(t, err, "not current", "an unknown Reading unit is refused")
		},
	}, {
		Name: "classical re-add reopens a hidden book and keeps its disposition",
		Run: func(t *testing.T, h Harness) {
			h.Seeds().SeedClassicalCatalog(t)
			owner := h.Seeds().Owner()
			report, err := h.Classical().AddClassicalReadingUnits(owner, []string{ClassicalUnitBook2Key})
			require.NoError(t, err)
			bookID := report.Books[0].BookID
			detail := requireDetail(t, h, owner, bookID)
			applied, err := h.BookState().TransitionBookDisposition(owner, bookID, detail.DispositionRevision, domain.BookDispositionToRead)
			require.NoError(t, err)
			require.True(t, applied)
			applied, err = h.BookState().SetBookHidden(owner, bookID, 0, true)
			require.NoError(t, err)
			require.True(t, applied)
			_, hiddenRevision, err := h.BookState().GetBookVisibility(owner, bookID)
			require.NoError(t, err)

			again, err := h.Classical().AddClassicalReadingUnits(owner, []string{ClassicalUnitBook2Key})
			require.NoError(t, err)
			assert.Zero(t, again.CreatedBooks())
			assert.Equal(t, bookID, again.Books[0].BookID)
			hidden, revision, err := h.BookState().GetBookVisibility(owner, bookID)
			require.NoError(t, err)
			assert.False(t, hidden, "re-adding reopens the existing Book")
			assert.Equal(t, hiddenRevision+1, revision, "reopening advances the visibility revision")
			assert.Equal(t, domain.BookDispositionToRead, requireDetail(t, h, owner, bookID).Disposition)
		},
	}, {
		Name: "classical add is owner-scoped",
		Run: func(t *testing.T, h Harness) {
			h.Seeds().SeedClassicalCatalog(t)
			owner, other := h.Seeds().Owner(), h.Seeds().OtherOwner()
			ownerReport, err := h.Classical().AddClassicalReadingUnits(owner, []string{ClassicalUnitBook1Key})
			require.NoError(t, err)
			otherReport, err := h.Classical().AddClassicalReadingUnits(other, []string{ClassicalUnitBook1Key})
			require.NoError(t, err)
			assert.NotEqual(t, ownerReport.Books[0].BookID, otherReport.Books[0].BookID)

			assert.Contains(t, ownedClassicalUnits(t, h, owner), ClassicalUnitBook1Key)
			assert.Len(t, ownedClassicalUnits(t, h, other), 1)
		},
	}}
}

func ownedClassicalUnits(t *testing.T, h Harness, owner string) map[string]classical.OwnedReadingUnit {
	t.Helper()
	owned, err := h.Classical().ListOwnedClassicalReadingUnits(owner)
	require.NoError(t, err)
	byKey := make(map[string]classical.OwnedReadingUnit, len(owned))
	for _, unit := range owned {
		byKey[unit.ReadingUnitKey] = unit
	}
	return byKey
}
