// Package storecontract holds the scenarios that pin Current reading lifecycle
// and deck admission behavior. Each store adapter that carries contractual
// transitions (ADR 0088) implements Harness and runs every scenario, so the
// fixtures and PostgreSQL adapters are held to the same outcomes.
//
// The package imports only domain. Adapters supply the transitions and the
// seeding through Harness; they do not share types with this package.
package storecontract

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// Language is the study language every scenario reads and writes.
const Language = "de"

// BookSeed describes the analyzed Book a scenario needs.
type BookSeed struct {
	// Vocabulary is the identities the Book's analysis contributes. A Start
	// freezes the eligible ones into the reading's snapshot.
	Vocabulary []domain.SnapshotIdentity
	// ToRead reports whether the Book is To Read. A Book that is not cannot
	// become the current reading.
	ToRead bool
}

// Harness is what an adapter implements to run the scenarios. Store exposes the
// operations under test, which return the adapter's own errors; Seeds builds the
// facts a scenario needs and fails the test on error.
type Harness interface {
	Store() Store
	Seeds() Seeder
}

// Store is the Current reading and deck admission surface under test.
type Store interface {
	GetCurrentReading(owner, language string) (domain.CurrentReading, error)
	StartCurrentReading(owner, language, bookID string) (domain.CurrentReading, error)
	SwitchCurrentReading(owner, language, bookID, expectedBookID, expectedSnapshotID string) (domain.CurrentReading, error)
	EndCurrentReading(owner, language, expectedBookID, expectedSnapshotID string) error
	FinishCurrentReading(owner, language, expectedBookID, expectedSnapshotID string) (domain.CurrentReadingFinishResult, error)
	ListKnownVocabulary(owner, language string) ([]domain.KnownVocabulary, error)

	// PrepareCurrentReadingDeck submits the Book's deck for the snapshot the
	// request names and returns the resulting preparation.
	PrepareCurrentReadingDeck(owner, bookID, expectedSnapshotID string) (domain.DeckPreparation, error)
	// RepreparePreparation rolls the preparation forward for the snapshot the
	// request names and returns the resulting preparation.
	RepreparePreparation(owner, id, expectedSnapshotID string) (domain.DeckPreparation, error)
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

// Scenarios returns every Current reading lifecycle and deck admission scenario.
func Scenarios() []Scenario {
	scenarios := lifecycleScenarios()
	for _, tc := range finishCases() {
		scenarios = append(scenarios, finishScenario(tc))
	}
	return append(scenarios, admissionScenarios()...)
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
