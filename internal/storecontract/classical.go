package storecontract

import (
	"slices"
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

// classicalScenarios pin the Classical catalog Add transition: one To Read
// Book per current Reading unit, an owner-scoped membership that reopens rather
// than duplicates, and a hidden Book made visible again with its evidence.
func classicalScenarios() []Scenario {
	return []Scenario{{
		Name: "classical add creates one metadata-only To Read book per unit",
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
			assert.Equal(t, domain.BookDispositionToRead, first.Disposition)
			require.NotNil(t, first.ReadingUnit)
			assert.Equal(t, ClassicalUnitBook1Key, first.ReadingUnit.ReadingUnitKey)
			assert.Equal(t, ClassicalWorkKey, first.ReadingUnit.WorkKey)
			assert.Equal(t, 1, first.ReadingUnit.ConstituentBook)
			assert.Equal(t, 2, first.ReadingUnit.ConstituentBooks)
			assert.Equal(t, ClassicalWorkGreek, first.ReadingUnit.GreekTitle)

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
		Name: "classical re-add reopens a hidden book and restores To Read",
		Run: func(t *testing.T, h Harness) {
			h.Seeds().SeedClassicalCatalog(t)
			owner := h.Seeds().Owner()
			report, err := h.Classical().AddClassicalReadingUnits(owner, []string{ClassicalUnitBook2Key})
			require.NoError(t, err)
			bookID := report.Books[0].BookID
			detail := requireDetail(t, h, owner, bookID)
			applied, err := h.BookState().TransitionBookDisposition(owner, bookID, detail.DispositionRevision, domain.BookDispositionInbox)
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
		Name: "classical re-add restores a removed book and its evidence",
		Run: func(t *testing.T, h Harness) {
			h.Seeds().SeedClassicalCatalog(t)
			owner := h.Seeds().Owner()
			report, err := h.Classical().AddClassicalReadingUnits(owner, []string{ClassicalUnitBook1Key})
			require.NoError(t, err)
			bookID := report.Books[0].BookID
			completion, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.NoError(t, err)
			knownLemma := domain.SnapshotIdentity{Language: classical.Language, CanonicalLemma: "λόγοσ", UPOS: "NOUN"}
			h.Seeds().SeedKnownVocabulary(t, owner, []domain.SnapshotIdentity{knownLemma})
			assert.True(t, hasStudyLanguage(t, h, owner, classical.Language), "the added Book contributes its study language")

			require.NoError(t, h.BookState().RemoveBookFromMyBooks(owner, bookID))
			_, err = h.BookState().GetBookDetail(owner, bookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
			assert.NotContains(t, ownedClassicalUnits(t, h, owner), ClassicalUnitBook1Key)
			assert.False(t, hasStudyLanguage(t, h, owner, classical.Language), "a removed Book contributes no study language")

			again, err := h.Classical().AddClassicalReadingUnits(owner, []string{ClassicalUnitBook1Key})
			require.NoError(t, err)
			assert.Zero(t, again.CreatedBooks(), "re-adding restores rather than creates")
			assert.Equal(t, bookID, again.Books[0].BookID, "re-adding resumes the removed Book")
			assert.Equal(t, ClassicalUnitBook1Key, again.Books[0].ReadingUnitKey)

			restored := requireDetail(t, h, owner, bookID)
			assert.Equal(t, domain.BookDispositionToRead, restored.Disposition)
			assert.False(t, restored.Hidden)
			require.NotNil(t, restored.ReadingUnit)
			assert.Equal(t, ClassicalUnitBook1Key, restored.ReadingUnit.ReadingUnitKey)
			assert.Equal(t, 1, restored.CompletionCount, "the removed Book keeps its reading history")
			assert.Equal(t, domain.ReadingCompletionPreviouslyRead, restored.LatestCompletionSource)
			require.NotNil(t, restored.LatestCompletionAt)
			assert.True(t, restored.LatestCompletionAt.Equal(completion.CompletedAt), "the recorded completion is unchanged")
			known, err := h.Store().ListKnownVocabulary(owner, classical.Language)
			require.NoError(t, err)
			assert.True(t, slices.ContainsFunc(known, func(vocabulary domain.KnownVocabulary) bool {
				return vocabulary.OwnerID == owner && vocabulary.Language == knownLemma.Language &&
					vocabulary.CanonicalLemma == knownLemma.CanonicalLemma && vocabulary.UPOS == knownLemma.UPOS &&
					vocabulary.LexicalDiscriminator == knownLemma.LexicalDiscriminator
			}), "the removed Book keeps its Known vocabulary")
			assert.True(t, hasStudyLanguage(t, h, owner, classical.Language), "restoring brings the study language back")
			assert.Len(t, ownedClassicalUnits(t, h, owner), 1, "restoring never duplicates the Reading unit")
		},
	}, {
		Name: "remove the work ends every other Book and names the skipped current Book",
		Run: func(t *testing.T, h Harness) {
			h.Seeds().SeedClassicalCatalog(t)
			owner := h.Seeds().Owner()
			report, err := h.Classical().AddClassicalReadingUnits(owner, []string{ClassicalUnitBook1Key, ClassicalUnitBook2Key})
			require.NoError(t, err)
			require.Len(t, report.Books, 2)
			currentBookID := report.Books[0].BookID
			otherBookID := report.Books[1].BookID
			h.Seeds().SeedCurrentReading(t, owner, classical.Language, currentBookID)

			removal, err := h.BookState().RemoveWorkFromMyBooks(owner, ClassicalWorkKey)
			require.NoError(t, err)
			assert.Equal(t, 1, removal.RemovedBooks)
			require.NotNil(t, removal.CurrentReading, "the current Book is named in the result")
			assert.Equal(t, currentBookID, removal.CurrentReading.ID)
			assert.Equal(t, ClassicalWorkTitle, removal.CurrentReading.Title)

			current := requireDetail(t, h, owner, currentBookID)
			assert.Equal(t, currentBookID, current.Book.ID, "the current Book stays in My Books")
			_, err = h.BookState().GetBookDetail(owner, otherBookID)
			require.ErrorIs(t, err, domain.ErrNotFound, "the Work's other Book leaves My Books")
			assert.Contains(t, ownedClassicalUnits(t, h, owner), ClassicalUnitBook1Key)
			assert.NotContains(t, ownedClassicalUnits(t, h, owner), ClassicalUnitBook2Key)

			_, err = h.BookState().RemoveWorkFromMyBooks(owner, "not-a-work")
			require.ErrorIs(t, err, domain.ErrNotFound)
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

// classicalPreparationFailureScenarios pin the classification both adapters
// return for a seeded Classical preparation failure. The web app renders this
// classification instead of re-deriving a label or recovery from the failure.
func classicalPreparationFailureScenarios() []Scenario {
	cases := []struct {
		name        string
		stage       domain.ClassicalPreparationFailure
		preparation domain.PreparationState
		evidence    domain.BookEvidenceState
		recovery    domain.BookEvidenceRecovery
	}{
		{name: "fetch", stage: domain.ClassicalPreparationFailureFetch, preparation: domain.PreparationFetchFailed, evidence: domain.BookUnavailable, recovery: domain.RecoveryRetryAcquisition},
		{name: "import", stage: domain.ClassicalPreparationFailureImport, preparation: domain.PreparationImportFailed, evidence: domain.BookAcquiredUnassessed, recovery: domain.RecoveryRetryAnalysis},
		{name: "source", stage: domain.ClassicalPreparationFailureSource, preparation: domain.PreparationSourceRefused, evidence: domain.BookAcquiredUnassessed, recovery: domain.RecoveryNone},
		{name: "withdrawn", stage: domain.ClassicalPreparationFailureWithdrawn, preparation: domain.PreparationWithdrawn, evidence: domain.BookUnavailable, recovery: domain.RecoveryNone},
		{name: "catalog", stage: domain.ClassicalPreparationFailureCatalog, preparation: domain.PreparationCatalogUnavailable, evidence: domain.BookUnavailable, recovery: domain.RecoveryNone},
	}
	return []Scenario{{
		Name: "classical preparation failure classifies the Book's evidence",
		Run: func(t *testing.T, h Harness) {
			h.Seeds().SeedClassicalCatalog(t)
			owner := h.Seeds().Owner()
			report, err := h.Classical().AddClassicalReadingUnits(owner, []string{ClassicalUnitBook1Key})
			require.NoError(t, err)
			bookID := report.Books[0].BookID

			for _, tc := range cases {
				h.Seeds().SeedClassicalPreparationFailure(t, owner, bookID, tc.stage)
				detail := requireDetail(t, h, owner, bookID)
				require.NotNil(t, detail.ClassicalPreparation, tc.name)
				assert.Equal(t, tc.stage, detail.ClassicalPreparation.Failure, tc.name)
				classification := detail.Classification()
				assert.Equal(t, tc.preparation, classification.Preparation.State, tc.name)
				assert.Equal(t, tc.evidence, classification.Evidence, tc.name)
				assert.Equal(t, tc.recovery, classification.Recovery, tc.name)
			}
		},
	}}
}

// hasStudyLanguage reports whether the learner's active Books contribute
// language to their study languages.
func hasStudyLanguage(t *testing.T, h Harness, owner, language string) bool {
	t.Helper()
	languages, err := h.Classical().ListStudyLanguages(owner)
	require.NoError(t, err)
	return slices.ContainsFunc(languages, func(studyLanguage domain.StudyLanguage) bool {
		return studyLanguage.Language == language
	})
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
