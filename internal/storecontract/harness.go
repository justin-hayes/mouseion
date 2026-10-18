// Package storecontract holds the scenarios that pin Current reading lifecycle,
// deck admission, lemma review, and Book visibility, disposition, and previously
// read import behavior. Each store adapter that carries contractual
// transitions (ADR 0088) implements Harness and runs every scenario, so the
// fixtures and PostgreSQL adapters are held to the same outcomes.
//
// The package imports only domain. Adapters supply the transitions and the
// seeding through Harness; they do not share types with this package.
package storecontract

import (
	"slices"
	"testing"

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
	Seeds() Seeder
}

// BookStateStore is the Book visibility, workflow disposition, and previously
// read import surface under test. Each write takes the revision the learner
// rendered; the adapter returns its own stale and not-found errors.
type BookStateStore interface {
	// GetBookDetail returns the Book with its disposition and visibility and
	// the revisions a write must present.
	GetBookDetail(owner, bookID string) (domain.MyBook, error)
	GetBookVisibility(owner, bookID string) (hidden bool, revision int64, err error)
	SetBookHidden(owner, bookID string, expectedRevision int64, hidden bool) (applied bool, err error)
	TransitionBookDisposition(owner, bookID string, expectedRevision int64, disposition domain.BookDisposition) (applied bool, err error)
	ImportPreviouslyRead(owner, bookID string) (domain.ReadingCompletion, error)
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
	// ClearSnapshotID leaves owner's current reading without a snapshot, the
	// shape of a legacy reading.
	ClearSnapshotID(t *testing.T, owner, language string)
}

// Scenario is one behavior every adapter must hold.
type Scenario struct {
	Name string
	Run  func(t *testing.T, h Harness)
}

// Scenarios returns every store contract scenario.
func Scenarios() []Scenario {
	scenarios := lifecycleScenarios()
	for _, tc := range finishCases() {
		scenarios = append(scenarios, finishScenario(tc))
	}
	scenarios = append(scenarios, admissionScenarios()...)
	scenarios = append(scenarios, lemmaReviewScenarios()...)
	scenarios = append(scenarios, greekScenarios()...)
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
