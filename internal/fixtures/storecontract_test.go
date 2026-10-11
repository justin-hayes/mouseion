package fixtures

import (
	"context"
	"fmt"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/storecontract"
	"github.com/stretchr/testify/require"
)

// contractOwner is a learner with no fixture state, so every scenario starts
// from an empty reading. otherLearnerID owns Known rows the scenarios must
// never count for contractOwner.
const (
	contractOwner  = "contract-owner"
	otherLearnerID = "other-learner"
)

// contractHarness runs the shared store contracts against the in-memory store.
type contractHarness struct {
	store *Store
	deck  PreparedDeck
	books int
}

func newContractHarness(*testing.T) storecontract.Harness {
	store := NewStore()
	return &contractHarness{store: store, deck: PreparedDeck{Store: store}}
}

func TestStoreContracts(t *testing.T) {
	storecontract.Run(t, newContractHarness)
}

func (h *contractHarness) Owner() string      { return contractOwner }
func (h *contractHarness) OtherOwner() string { return otherLearnerID }

func (h *contractHarness) SeedBook(_ *testing.T, owner string, seed storecontract.BookSeed) string {
	h.books++
	bookID := fmt.Sprintf("contract-book-%d", h.books)
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	h.store.books = append(h.store.books, domain.SourceMaterialSummary{
		Source: domain.SourceMaterial{
			ID: bookID, OwnerID: owner, Language: storecontract.Language, Title: "Contract Book",
			MediaType: "application/epub+zip", ContentRevisionID: bookID + "-revision", ContentSnapshotID: bookID + "-snapshot",
		},
		Signals: analyzedSignals, AnalysisRunID: bookID + "-run", CorpusID: bookID + "-corpus",
	})
	if seed.ToRead {
		h.store.dispositions[fixtureDispositionKey(owner, bookID)] = domain.BookDispositionToRead
	}
	vocabulary := make([]domain.DeckPreparationVocabulary, 0, len(seed.Vocabulary))
	for _, identity := range seed.Vocabulary {
		vocabulary = append(vocabulary, domain.DeckPreparationVocabulary{OwnerID: owner, Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS})
	}
	h.store.bookSnapshotVocabulary[bookID] = vocabulary
	h.store.lemmaOccurrences[lemmaOccurrenceKey(owner, bookID)] = contractReviewOccurrences(owner, bookID, seed.Occurrences)
	return bookID
}

// contractReviewOccurrences lays the scenario's occurrences out in sentence order,
// one sentence each, the way the PostgreSQL harness stores them.
func contractReviewOccurrences(owner, bookID string, seeds []storecontract.OccurrenceSeed) []domain.LemmaReviewOccurrence {
	occurrences := make([]domain.LemmaReviewOccurrence, 0, len(seeds))
	start := int64(0)
	for ordinal, seed := range seeds {
		end := start + int64(utf8.RuneCountInString(seed.Surface))
		occurrences = append(occurrences, domain.LemmaReviewOccurrence{
			OwnerID: owner, BookID: bookID, CorpusID: bookID + "-corpus", AnalysisRunID: bookID + "-run",
			SourceDocumentID: "contract-unit", StartOffset: start, EndOffset: end,
			SentenceOrdinal: int64(ordinal), TokenOrdinal: 0, Surface: seed.Surface, RawLemma: seed.Lemma,
			CanonicalLemma: seed.Lemma, UPOS: seed.UPOS, Dependency: "root", SentenceText: seed.Surface,
		})
		start = end + 1
	}
	return occurrences
}

func (h *contractHarness) SeedKnownVocabulary(_ *testing.T, owner string, identities []domain.SnapshotIdentity) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	for _, identity := range identities {
		h.store.known = append(h.store.known, domain.KnownVocabulary{OwnerID: owner, Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS, CreatedAt: time.Now()})
	}
}

func (h *contractHarness) SeedReadyPreparation(t *testing.T, owner, bookID, snapshotID string) string {
	t.Helper()
	current, err := h.store.GetCurrentReading(context.Background(), owner, storecontract.Language)
	require.NoError(t, err)
	id := "contract-ready-" + snapshotID
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	h.store.preps = append(h.store.preps, domain.DeckPreparation{
		ID: id, OwnerID: owner, BookID: bookID, SnapshotID: snapshotID, State: domain.DeckPreparationReady,
		SourceMaterialID: current.SourceMaterialID, AnalysisRunID: current.AnalysisRunID,
	})
	return id
}

func (h *contractHarness) ClearSnapshotID(_ *testing.T, owner, language string) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	key := fixtureGoalKey(owner, language)
	goal := h.store.currentReadings[key]
	goal.SnapshotID = ""
	h.store.currentReadings[key] = goal
}

func (h *contractHarness) GetCurrentReading(owner, language string) (domain.CurrentReading, error) {
	return h.store.GetCurrentReading(context.Background(), owner, language)
}

func (h *contractHarness) StartCurrentReading(owner, language, bookID string) (domain.CurrentReading, error) {
	return h.store.StartCurrentReading(context.Background(), owner, language, bookID)
}

func (h *contractHarness) SwitchCurrentReading(owner, language, bookID, expectedBookID, expectedSnapshotID string) (domain.CurrentReading, error) {
	return h.store.SwitchCurrentReading(context.Background(), owner, language, bookID, expectedBookID, expectedSnapshotID)
}

func (h *contractHarness) EndCurrentReading(owner, language, expectedBookID, expectedSnapshotID string) error {
	return h.store.EndCurrentReading(context.Background(), owner, language, expectedBookID, expectedSnapshotID)
}

func (h *contractHarness) FinishCurrentReading(owner, language, expectedBookID, expectedSnapshotID string) (domain.CurrentReadingFinishResult, error) {
	return h.store.FinishCurrentReading(context.Background(), owner, language, expectedBookID, expectedSnapshotID)
}

func (h *contractHarness) ListKnownVocabulary(owner, language string) ([]domain.KnownVocabulary, error) {
	return h.store.ListKnownVocabulary(context.Background(), owner, language)
}

func (h *contractHarness) ListLemmaReviewOccurrences(owner, bookID, surface string) ([]domain.LemmaReviewOccurrence, error) {
	return h.store.ListLemmaReviewOccurrences(context.Background(), owner, bookID, surface)
}

func (h *contractHarness) SaveLemmaReviewFlags(flags []domain.LemmaReviewFlag) error {
	return h.store.SaveLemmaReviewFlags(context.Background(), flags)
}

func (h *contractHarness) ReadLemmaReviewProposal(proposal domain.LemmaReviewProposal) (domain.LemmaReviewPreview, error) {
	return h.store.ReadLemmaReviewProposal(context.Background(), proposal)
}

func (h *contractHarness) PutLemmaDecisionProposal(proposal domain.LemmaReviewProposal, expectedFingerprint string) error {
	return h.store.PutLemmaDecisionProposal(context.Background(), proposal, expectedFingerprint)
}

func (h *contractHarness) PrepareCurrentReadingDeck(owner, bookID, expectedSnapshotID string) (domain.DeckPreparation, error) {
	handle, err := h.deck.PrepareCurrentReadingDeck(context.Background(), owner, bookID, expectedSnapshotID)
	return handle.Preparation, err
}

func (h *contractHarness) RepreparePreparation(owner, id, expectedSnapshotID string) (domain.DeckPreparation, error) {
	handle, err := h.deck.Reprepare(context.Background(), owner, id, expectedSnapshotID)
	return handle.Preparation, err
}

func (h *contractHarness) Store() storecontract.Store                  { return h }
func (h *contractHarness) LemmaReview() storecontract.LemmaReviewStore { return h }
func (h *contractHarness) Seeds() storecontract.Seeder                 { return h }
