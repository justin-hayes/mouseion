// Package storecontract holds the scenarios that pin Current reading lifecycle,
// deck admission, lemma review, Book visibility, disposition, previously read
// import, and Classical catalog membership behavior. Each store adapter that
// carries contractual transitions (ADR 0088) implements Harness and runs every
// scenario, so the fixtures and PostgreSQL adapters are held to the same
// outcomes.
//
// The package shares only the domain and Classical catalog types. Adapters
// supply the transitions and the seeding through Harness; they do not share
// adapter-specific types with this package.
package storecontract

import (
	"slices"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// Language is the study language every scenario reads and writes.
const Language = "de"

// GreekLanguage is the imported classical study language whose discriminated
// identities the grc scenarios exercise. It never shares readings, Reserved
// vocabulary or counts with Language.
const GreekLanguage = "grc"

// BookSeed describes the analyzed Book a scenario needs.
type BookSeed struct {
	// Language is the Book's chosen study language. The empty value is
	// Language, so every modern-language scenario is unchanged.
	Language string
	// Imported reports that the Book's analysis is an imported classical
	// annotation rather than a modern analysis, so its vocabulary is seeded as
	// analysis tokens and its run carries a non-UD syntax state.
	Imported bool
	// Vocabulary is the identities the Book's analysis contributes. A Start
	// freezes the eligible ones into the reading's snapshot.
	Vocabulary []domain.SnapshotIdentity
	// Unusable names identities in Vocabulary that the Book's analysis holds
	// only as gate-rejected fragments, so a gated Start must not freeze them.
	// An adapter that models the gate by omission simply leaves them out of the
	// snapshot vocabulary.
	Unusable []domain.SnapshotIdentity
	// Occurrences are the observed tokens the Book's analysis contributes to
	// lemma review, in sentence order. Each one becomes its own sentence.
	Occurrences []OccurrenceSeed
	// ToRead reports whether the Book is To Read. A Book that is not cannot
	// become the current reading.
	ToRead bool
	// LanguageUnknown reports that the Book's language is not chosen, so it
	// cannot be imported as previously read.
	LanguageUnknown bool
	// MetadataOnly reports a Book with no acquired source, as a chosen Book
	// looks before its EPUB is acquired.
	MetadataOnly bool
}

// OccurrenceSeed is one observed token of a Book's analysis. LexicalDiscriminator
// is empty for modern-language tokens and scheme-qualified for source homographs
// such as morpheus:2.
type OccurrenceSeed struct {
	Surface, Lemma, UPOS, LexicalDiscriminator string
}

// DeclaresUnusable reports whether the seed names identity as one the Book's
// analysis cannot gate, so a gated Start must not freeze it.
func (seed BookSeed) DeclaresUnusable(identity domain.SnapshotIdentity) bool {
	return slices.Contains(seed.Unusable, identity)
}

// Harness is what an adapter implements to run the scenarios. Store exposes the
// operations under test, which return the adapter's own errors; Seeds builds the
// facts a scenario needs and fails the test on error.
type Harness interface {
	Store() Store
	LemmaReview() LemmaReviewStore
	BookState() BookStateStore
	Classical() ClassicalStore
	Catalogue() CatalogueStore
	Seeds() Seeder
}

// BookStateStore is the Book visibility, workflow disposition, My Books
// membership, and previously read import surface under test. Each write takes
// the revision the learner rendered; the adapter returns its own stale and
// not-found errors.
type BookStateStore interface {
	// GetBookDetail returns the Book with its disposition and visibility and
	// the revisions a write must present.
	GetBookDetail(owner, bookID string) (domain.MyBook, error)
	GetBookVisibility(owner, bookID string) (hidden bool, revision int64, err error)
	SetBookHidden(owner, bookID string, expectedRevision int64, hidden bool) (applied bool, err error)
	TransitionBookDisposition(owner, bookID string, expectedRevision int64, disposition domain.BookDisposition) (applied bool, err error)
	// RemoveBookFromMyBooks ends the Book's membership without deleting its
	// evidence. A later Add restores the same Book. The current reading is
	// refused with domain.ErrBookIsCurrentReading.
	RemoveBookFromMyBooks(owner, bookID string) error
	// RemoveWorkFromMyBooks ends every Book of one classical Work's membership,
	// skipping the current reading and naming it in the result.
	RemoveWorkFromMyBooks(owner, workKey string) (domain.MyBooksWorkRemoval, error)
	ImportPreviouslyRead(owner, bookID string) (domain.ReadingCompletion, error)
	// MarkBookPreviouslyRead records an unstarted To Read Book as Previously
	// read and clears its To Read disposition in one transition. It refuses the
	// Current reading with domain.ErrBookIsCurrentReading, and a replay or an
	// already read Book changes nothing.
	MarkBookPreviouslyRead(owner, bookID string) error
	// RecordEPUBAcquisitionFailure records the latest failed EPUB acquisition
	// for a Book, replacing any previous record.
	RecordEPUBAcquisitionFailure(record domain.EPUBAcquisitionFailureRecord) error
	// ClearEPUBAcquisitionFailure deletes a Book's failure after a successful
	// acquisition. A missing row is not an error.
	ClearEPUBAcquisitionFailure(owner, bookID string) error
}

// ClassicalStore is the built-in Classical catalog membership surface under
// test. Add creates one To Read Book per current Reading unit or restores the
// learner's existing Book as To Read; the list is owner-scoped and resumes an
// existing Book rather than creating a duplicate.
type ClassicalStore interface {
	AddClassicalReadingUnits(owner string, readingUnitKeys []string) (classical.AddReport, error)
	// MarkClassicalReadingUnitPreviouslyRead records one current Reading unit as
	// read outside Mouseion. It creates or restores the Book as Read with a
	// Previously read record and starts no preparation.
	MarkClassicalReadingUnitPreviouslyRead(owner, readingUnitKey string) (domain.CatalogueReadResult, error)
	ListOwnedClassicalReadingUnits(owner string) ([]classical.OwnedReadingUnit, error)
	// ListStudyLanguages includes the languages of Books with active My Books
	// membership, so a removed Book contributes nothing until Add restores it.
	ListStudyLanguages(owner string) ([]domain.StudyLanguage, error)
}

// CatalogueStore is the Catalog browse and Add surface under test. Browse
// returns one connection's local index entries in title-then-author order with
// the learner's ownership attached, and it never contacts the OPDS server. Add
// creates or restores the learner's Book for one indexed entry as To Read.
type CatalogueStore interface {
	BrowseCatalogueEntries(owner, connectionID string, query domain.CatalogueQuery) (domain.CataloguePage, error)
	// AddCatalogueEntry creates the Book for one indexed entry or restores the
	// learner's existing Book as To Read. An entry missing from the index
	// returns domain.ErrNotFound. Replaying Add on an active To Read Book
	// changes nothing.
	AddCatalogueEntry(owner, connectionID, entryIdentifier string) (domain.CatalogueAddResult, error)
	// MarkCatalogueEntryPreviouslyRead creates or restores the Book for one
	// indexed entry as Read with a Previously read record. It acquires and
	// prepares nothing and changes no Known vocabulary. An entry missing from
	// the index returns domain.ErrNotFound.
	MarkCatalogueEntryPreviouslyRead(owner, connectionID, entryIdentifier string) (domain.CatalogueReadResult, error)
}

// CatalogueEntrySeed is one local Catalog index entry and, optionally, an
// existing Book the learner owns for it. An empty BookID leaves the entry
// unowned, so the Catalog offers Add.
type CatalogueEntrySeed struct {
	Entry  domain.CatalogueIndexEntry
	BookID string
}

// Store is the Current reading and deck admission surface under test.
type Store interface {
	GetCurrentReading(owner, language string) (domain.CurrentReading, error)
	StartCurrentReading(owner, language, bookID string) (domain.CurrentReading, error)
	SwitchCurrentReading(owner, language, bookID, expectedBookID, expectedSnapshotID string) (domain.CurrentReading, error)
	EndCurrentReading(owner, language, expectedBookID, expectedSnapshotID string) error
	FinishCurrentReading(owner, language, expectedBookID, expectedSnapshotID string) (domain.CurrentReadingFinishResult, error)
	ListKnownVocabulary(owner, language string) ([]domain.KnownVocabulary, error)
	// ListReservedVocabulary returns the vocabulary the owner's active reading
	// reserves in language.
	ListReservedVocabulary(owner, language string) ([]domain.DeckPreparationVocabulary, error)

	// PrepareCurrentReadingDeck submits the Book's deck for the snapshot the
	// request names and returns the resulting preparation.
	PrepareCurrentReadingDeck(owner, bookID, expectedSnapshotID string) (domain.DeckPreparation, error)
	// RepreparePreparation rolls the preparation forward for the snapshot the
	// request names and returns the resulting preparation.
	RepreparePreparation(owner, id, expectedSnapshotID string) (domain.DeckPreparation, error)
}

// LemmaReviewStore is the lemma review surface under test: occurrences, flags,
// and the previewed and confirmed vocabulary decisions.
type LemmaReviewStore interface {
	// ListLemmaReviewOccurrences returns the occurrences of surface in the
	// Book's current analysis, with any learner decision and flag applied. An
	// empty surface returns every occurrence.
	ListLemmaReviewOccurrences(owner, bookID, surface string) ([]domain.LemmaReviewOccurrence, error)
	// SaveLemmaReviewFlags records detector flags without reopening a flag the
	// learner has already resolved.
	SaveLemmaReviewFlags(flags []domain.LemmaReviewFlag) error
	// ReadLemmaReviewProposal previews a proposal and returns the fingerprint a
	// confirm must present.
	ReadLemmaReviewProposal(proposal domain.LemmaReviewProposal) (domain.LemmaReviewPreview, error)
	// PutLemmaDecisionProposal confirms a previewed proposal against its
	// fingerprint and commits the decisions atomically.
	PutLemmaDecisionProposal(proposal domain.LemmaReviewProposal, expectedFingerprint string) error
}

// Seeder builds the learner, Book, and preparation facts a scenario starts from.
type Seeder interface {
	// Owner is the learner under test. OtherOwner is a second learner in the
	// same store whose learner state must never affect Owner.
	Owner() string
	OtherOwner() string

	// SeedBook creates an analyzed Book for owner and returns its ID.
	SeedBook(t *testing.T, owner string, seed BookSeed) string
	// SeedKnownVocabulary records Known identities for owner.
	SeedKnownVocabulary(t *testing.T, owner string, identities []domain.SnapshotIdentity)
	// SeedReadyPreparation makes the snapshot's preparation ready and returns its ID.
	SeedReadyPreparation(t *testing.T, owner, bookID, snapshotID string) string
	// SeedCurrentReading records an already-current Book without a Start
	// transition, for scenarios that need a Work's current Book before Remove.
	SeedCurrentReading(t *testing.T, owner, language, bookID string)
	// SeedClassicalCatalog installs the Classical release the contract's Add
	// scenarios resolve Reading units from.
	SeedClassicalCatalog(t *testing.T)
	// SeedClassicalPreparationFailure records one settled preparation failure
	// for an already added Book. The adapter derives the Reading unit and
	// definition revision from the Book, exactly as a worker would.
	SeedClassicalPreparationFailure(t *testing.T, owner, bookID string, stage domain.BookPreparationStage)
	// SeedCatalogueConnection creates one owner-scoped OPDS connection and
	// returns its ID.
	SeedCatalogueConnection(t *testing.T, owner, name string) string
	// SeedCatalogueEntries installs the connection's local Catalog index and
	// links each seeded entry to its Book through an active catalog-entry
	// alias.
	SeedCatalogueEntries(t *testing.T, owner, connectionID string, entries []CatalogueEntrySeed)
}

// Scenario is one behavior every adapter must hold.
type Scenario struct {
	Name string
	Run  func(t *testing.T, h Harness)
}

// Scenarios returns every store contract scenario.
func Scenarios() []Scenario {
	scenarios := lifecycleScenarios()
	scenarios = append(scenarios, gateScenarios()...)
	for _, tc := range finishCases() {
		scenarios = append(scenarios, finishScenario(tc))
	}
	scenarios = append(scenarios, admissionScenarios()...)
	scenarios = append(scenarios, lemmaReviewScenarios()...)
	scenarios = append(scenarios, greekScenarios()...)
	scenarios = append(scenarios, classicalScenarios()...)
	scenarios = append(scenarios, classicalPreparationFailureScenarios()...)
	scenarios = append(scenarios, catalogueScenarios()...)
	return append(scenarios, bookStateScenarios()...)
}

// Run runs every scenario as a subtest, each against a fresh harness.
func Run(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Helper()
	for _, scenario := range Scenarios() {
		t.Run(scenario.Name, func(t *testing.T) {
			scenario.Run(t, newHarness(t))
		})
	}
}
