package fixtures

import (
	"context"
	"fmt"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/classical"
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
	books int
}

func newContractHarness(*testing.T) storecontract.Harness {
	store := NewStore()
	return &contractHarness{store: store}
}

func TestStoreContracts(t *testing.T) {
	storecontract.Run(t, newContractHarness)
}

func (h *contractHarness) Owner() string      { return contractOwner }
func (h *contractHarness) OtherOwner() string { return otherLearnerID }

func (h *contractHarness) SeedBook(_ *testing.T, owner string, seed storecontract.BookSeed) string {
	h.books++
	bookID := fmt.Sprintf("contract-book-%d", h.books)
	language := seed.Language
	if language == "" {
		language = storecontract.Language
	}
	if seed.LanguageUnknown {
		// An empty source language is how the fixtures shelf reports an unchosen language.
		language = ""
	}
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	if seed.MetadataOnly {
		languageState, languageTag := domain.LanguageChosen, language
		if seed.LanguageUnknown {
			languageState, languageTag = domain.LanguageUnknown, ""
		}
		h.store.myBooks = append(h.store.myBooks, domain.MyBook{
			Book: domain.Book{ID: bookID, OwnerID: owner, Title: "Contract Book", LanguageState: languageState, LanguageTag: languageTag},
		})
		if !seed.Inbox {
			h.store.dispositions[fixtureDispositionKey(owner, bookID)] = domain.BookDispositionToRead
		}
		if seed.SourceIdentifier != "" {
			h.store.aliases = append(h.store.aliases, domain.BookAlias{
				ID: "contract-source-alias-" + bookID, OwnerID: owner, BookID: bookID,
				AliasType: domain.AliasStrongBibliographic, Namespace: domain.NamespaceSourceIdentifier, Value: seed.SourceIdentifier,
			})
		}
		return bookID
	}
	h.store.books = append(h.store.books, domain.SourceMaterialSummary{
		Source: domain.SourceMaterial{
			ID: bookID, OwnerID: owner, Language: language, Title: "Contract Book",
			MediaType: "application/epub+zip", ContentRevisionID: bookID + "-revision", ContentSnapshotID: bookID + "-snapshot",
		},
		BookID:  bookID,
		Signals: analyzedSignals, AnalysisRunID: bookID + "-run", CorpusID: bookID + "-corpus",
	})
	if !seed.Inbox {
		h.store.dispositions[fixtureDispositionKey(owner, bookID)] = domain.BookDispositionToRead
	}
	// The fixtures hold no dependency parses, so they model the sentence-quality
	// gate by leaving out the identities a seed declares unusable: a Start then
	// freezes the rest, as the gate does against PostgreSQL.
	vocabulary := make([]domain.DeckPreparationVocabulary, 0, len(seed.Vocabulary))
	for _, identity := range seed.Vocabulary {
		if seed.DeclaresUnusable(identity) {
			continue
		}
		vocabulary = append(vocabulary, domain.DeckPreparationVocabulary{OwnerID: owner, Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS, LexicalDiscriminator: identity.LexicalDiscriminator, Headword: identity.Headword})
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
			CanonicalLemma: seed.Lemma, UPOS: seed.UPOS, LexicalDiscriminator: seed.LexicalDiscriminator, Dependency: "root", SentenceText: seed.Surface,
		})
		start = end + 1
	}
	return occurrences
}

func (h *contractHarness) SeedKnownVocabulary(_ *testing.T, owner string, identities []domain.SnapshotIdentity) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	for _, identity := range identities {
		h.store.known = append(h.store.known, domain.KnownVocabulary{OwnerID: owner, Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS, LexicalDiscriminator: identity.LexicalDiscriminator, Headword: identity.Headword, CreatedAt: time.Now()})
	}
}

func (h *contractHarness) GetCurrentReading(owner, language string) (domain.CurrentReading, error) {
	return h.store.GetCurrentReading(context.Background(), owner, language)
}

func (h *contractHarness) SeedCurrentReading(_ *testing.T, owner, language, bookID string) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	h.store.currentReadings[fixtureGoalKey(owner, language)] = domain.CurrentReading{OwnerID: owner, Language: language, BookID: bookID}
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

func (h *contractHarness) ListReservedVocabulary(owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	return h.store.ListReservedVocabulary(context.Background(), owner, language)
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

func (h *contractHarness) GetBookDetail(owner, bookID string) (domain.MyBook, error) {
	return h.store.GetBookDetail(context.Background(), owner, bookID)
}

func (h *contractHarness) TransitionBookDisposition(owner, bookID string, expectedRevision int64, disposition domain.BookDisposition) (bool, error) {
	return h.store.TransitionBookDisposition(context.Background(), owner, bookID, expectedRevision, disposition)
}

func (h *contractHarness) SetBookDisposition(owner, bookID string, disposition domain.BookDisposition) error {
	return h.store.SetBookDisposition(context.Background(), owner, bookID, disposition)
}

func (h *contractHarness) RemoveBookFromMyBooks(owner, bookID string) error {
	return h.store.RemoveBookFromMyBooks(context.Background(), owner, bookID)
}

func (h *contractHarness) ImportPreviouslyRead(owner, bookID string) (domain.ReadingCompletion, error) {
	return h.store.ImportPreviouslyRead(context.Background(), owner, bookID)
}

func (h *contractHarness) MarkBookPreviouslyRead(owner, bookID string) error {
	return h.store.MarkBookPreviouslyRead(context.Background(), owner, bookID)
}

func (h *contractHarness) ResolveOrCreateBookForAcquisition(owner, sourceIdentifier, language, title string) (string, error) {
	return h.store.ResolveOrCreateBookForAcquisition(context.Background(), owner, sourceIdentifier, language, title)
}

func (h *contractHarness) RecordEPUBAcquisitionFailure(record domain.EPUBAcquisitionFailureRecord) error {
	return h.store.RecordEPUBAcquisitionFailure(context.Background(), record)
}

func (h *contractHarness) ClearEPUBAcquisitionFailure(owner, bookID string) error {
	return h.store.ClearEPUBAcquisitionFailure(context.Background(), owner, bookID)
}

func (h *contractHarness) Store() storecontract.Store                      { return h }
func (h *contractHarness) LemmaReview() storecontract.LemmaReviewStore     { return h }
func (h *contractHarness) BookState() storecontract.BookStateStore         { return h }
func (h *contractHarness) Classical() storecontract.ClassicalStore         { return h }
func (h *contractHarness) Catalogue() storecontract.CatalogueStore         { return h }
func (h *contractHarness) Study() storecontract.StudyStore                 { return h }
func (h *contractHarness) StudyProgress() storecontract.StudyProgressStore { return h }
func (h *contractHarness) StudyFluency() storecontract.StudyFluencyStore   { return h }

func (h *contractHarness) BrowseCatalogueEntries(owner, connectionID string, query domain.CatalogueQuery) (domain.CataloguePage, error) {
	return h.store.BrowseCatalogueEntries(context.Background(), owner, connectionID, query)
}

func (h *contractHarness) AddCatalogueEntry(owner, connectionID, entryIdentifier string) (domain.CatalogueAddResult, error) {
	return h.store.AddCatalogueEntry(context.Background(), owner, connectionID, entryIdentifier)
}

func (h *contractHarness) MarkCatalogueEntryPreviouslyRead(owner, connectionID, entryIdentifier string) (domain.CatalogueReadResult, error) {
	return h.store.MarkCatalogueEntryPreviouslyRead(context.Background(), owner, connectionID, entryIdentifier)
}

func (h *contractHarness) AddClassicalReadingUnits(owner string, readingUnitKeys []string) (classical.AddReport, error) {
	return h.store.AddClassicalReadingUnits(context.Background(), owner, readingUnitKeys)
}

func (h *contractHarness) MarkClassicalReadingUnitPreviouslyRead(owner, readingUnitKey string) (domain.CatalogueReadResult, error) {
	return h.store.MarkClassicalReadingUnitPreviouslyRead(context.Background(), owner, readingUnitKey)
}

func (h *contractHarness) ListOwnedClassicalReadingUnits(owner string) ([]classical.OwnedReadingUnit, error) {
	return h.store.ListOwnedClassicalReadingUnits(context.Background(), owner)
}

func (h *contractHarness) ListStudyLanguages(owner string) ([]domain.StudyLanguage, error) {
	return h.store.ListStudyLanguages(context.Background(), owner)
}

func (h *contractHarness) SeedClassicalCatalog(*testing.T) {
	h.store.setClassicalDefinitions(storecontract.ClassicalDefinitions())
}

func (h *contractHarness) SeedCatalogueConnection(t *testing.T, owner, name string) string {
	t.Helper()
	connection, err := h.store.CreateOpdsConnection(context.Background(), owner, domain.OpdsConnection{Name: name, URL: "https://contract.example/opds"})
	require.NoError(t, err)
	return connection.ID
}

func (h *contractHarness) SeedCatalogueEntries(t *testing.T, owner, connectionID string, seeds []storecontract.CatalogueEntrySeed) {
	t.Helper()
	entries := make([]domain.CatalogueIndexEntry, 0, len(seeds))
	h.store.mu.Lock()
	for i, seed := range seeds {
		entry := seed.Entry
		entry.ConnectionID = connectionID
		entries = append(entries, entry)
		if seed.BookID == "" {
			continue
		}
		h.store.aliases = append(h.store.aliases, domain.BookAlias{
			ID:           fmt.Sprintf("contract-catalogue-alias-%d", i),
			OwnerID:      owner,
			BookID:       seed.BookID,
			ConnectionID: connectionID,
			AliasType:    domain.AliasCatalogEntry,
			Namespace:    domain.NamespaceSourceIdentifier,
			Value:        entry.EntryIdentifier,
		})
	}
	h.store.mu.Unlock()
	require.NoError(t, h.store.SeedCatalogueEntries(owner, connectionID, entries))
}

func (h *contractHarness) SeedClassicalPreparationFailure(t *testing.T, owner, bookID string, stage domain.BookPreparationStage) {
	t.Helper()
	detail, err := h.store.GetBookDetail(context.Background(), owner, bookID)
	require.NoError(t, err)
	require.NotNil(t, detail.ReadingUnit, "a classical failure needs a Reading unit")
	status := &domain.ClassicalPreparationStatus{
		Stage:           stage,
		Code:            domain.BookPreparationCode("contract_" + string(stage)),
		FailureRevision: detail.ReadingUnit.Revision,
		CurrentRevision: detail.ReadingUnit.Revision,
		Withdrawn:       stage == domain.BookPreparationStageWithdrawn,
	}
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	for i := range h.store.myBooks {
		book := &h.store.myBooks[i]
		if book.Book.OwnerID == owner && book.Book.ID == bookID {
			book.ClassicalPreparation = status
			return
		}
	}
	require.Failf(t, "Book not found", "Book %s does not exist for owner %s", bookID, owner)
}

func (h *contractHarness) Seeds() storecontract.Seeder { return h }

func (h *contractHarness) ListStudyProgress(owner, language string) ([]domain.StudyProgress, error) {
	return h.store.ListStudyProgress(context.Background(), owner, language)
}

func (h *contractHarness) StudyBatchLedger(owner, language, studyDay string) (domain.StudyBatchLedger, error) {
	return h.store.StudyBatchLedger(context.Background(), owner, language, studyDay)
}

func (h *contractHarness) RecordStudyBatchOffer(owner, language, studyDay string, offered int) error {
	return h.store.RecordStudyBatchOffer(context.Background(), owner, language, studyDay, offered)
}

func (h *contractHarness) TakeStudyBatch(owner, language, studyDay string, now time.Time, batchLimit int, words []domain.StudyBatchItem) ([]domain.StudyProgress, error) {
	return h.store.TakeStudyBatch(context.Background(), owner, language, studyDay, now, batchLimit, words)
}

func (h *contractHarness) DeclineStudyBatch(owner, language, studyDay string, batchLimit int) error {
	return h.store.DeclineStudyBatch(context.Background(), owner, language, studyDay, batchLimit)
}

func (h *contractHarness) NextStudyCard(owner, language string, now time.Time) (domain.StudyCard, error) {
	return h.store.NextStudyCard(context.Background(), owner, language, now)
}

func (h *contractHarness) RecordStudyGrade(owner string, record domain.StudyGradeRecord) error {
	return h.store.RecordStudyGrade(context.Background(), owner, record)
}

func (h *contractHarness) ListStudyReviewLog(owner, progressID string) ([]domain.StudyReviewLog, error) {
	return h.store.ListStudyReviewLog(context.Background(), owner, progressID)
}

func (h *contractHarness) ListStudyCandidatesAmong(owner, language string, identities []domain.StudyItem) ([]domain.StudyCandidate, error) {
	return h.store.ListStudyCandidatesAmong(context.Background(), owner, language, identities)
}

func (h *contractHarness) ListStudyCandidates(owner, language string) ([]domain.StudyCandidate, error) {
	return h.store.ListStudyCandidates(context.Background(), owner, language)
}

func (h *contractHarness) ListCurrentStudyMaterial(owner, language string) ([]domain.StudyMaterial, error) {
	return h.store.ListCurrentStudyMaterial(context.Background(), owner, language)
}

func (h *contractHarness) GetStudyMaterial(owner, id string) (domain.StudyMaterial, error) {
	return h.store.GetStudyMaterial(context.Background(), owner, id)
}

func (h *contractHarness) CountStudyMaterial(owner, language string) (domain.StudyMaterialCounts, error) {
	return h.store.CountStudyMaterial(context.Background(), owner, language)
}

func (h *contractHarness) ReserveStudyMaterial(owner string, identity domain.StudyItem) (string, bool, error) {
	return h.store.ReserveStudyMaterial(context.Background(), owner, identity)
}

func (h *contractHarness) RecordStudyMaterialReady(owner, id string, content domain.StudyMaterialReadyContent) error {
	return h.store.RecordStudyMaterialReady(context.Background(), owner, id, content)
}

func (h *contractHarness) RecordStudyMaterialUnpreparable(owner, id, reason string) error {
	return h.store.RecordStudyMaterialUnpreparable(context.Background(), owner, id, reason)
}

func (h *contractHarness) ListStudyFluencyContexts(owner, language string) ([]domain.FluencyContext, error) {
	return h.store.ListStudyFluencyContexts(context.Background(), owner, language)
}

func (h *contractHarness) RecordStudyFluencyContext(owner string, record domain.FluencyContextRecord) error {
	return h.store.RecordStudyFluencyContext(context.Background(), owner, record)
}

func (h *contractHarness) RecordStudyFluencyResponse(owner string, record domain.FluencyResponseRecord) error {
	return h.store.RecordStudyFluencyResponse(context.Background(), owner, record)
}

func (h *contractHarness) CountStudyFluencyResponses(owner, contextID string) (int, error) {
	responses, err := h.store.ListStudyFluencyResponses(context.Background(), owner, contextID)
	return len(responses), err
}
