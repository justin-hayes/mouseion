package fixtures

// Contract status: contractual. The frozen Current reading vocabulary and its
// reads must match internal/storecontract scenarios shared with PostgreSQL (ADR
// 0088).

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// SetGoalSnapshotVocabulary lets acceptance tests add a deterministic frozen
// snapshot without exposing the fixture store's internal maps.
func (s *Store) SetGoalSnapshotVocabulary(snapshotID string, vocabulary []domain.DeckPreparationVocabulary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goalSnapshotVocabulary[snapshotID] = append([]domain.DeckPreparationVocabulary(nil), vocabulary...)
}

// reservedDeckVocabularyLocked returns the active reading's frozen vocabulary.
// Callers hold s.mu.
func (s *Store) reservedDeckVocabularyLocked(owner, language string) []domain.DeckPreparationVocabulary {
	var result []domain.DeckPreparationVocabulary
	seen := make(map[string]struct{})
	goal := s.currentReadings[fixtureGoalKey(owner, language)]
	if goal.IsActive() && goal.SnapshotID != "" {
		snapshot := s.goalSnapshotVocabulary[goal.SnapshotID]
		result = appendReservedVocabulary(result, seen, snapshot, language)
	}
	return result
}

// appendReservedVocabulary appends the items in language to result with any
// graduated marker cleared, recording each identity in seen.
func appendReservedVocabulary(result []domain.DeckPreparationVocabulary, seen map[string]struct{}, items []domain.DeckPreparationVocabulary, language string) []domain.DeckPreparationVocabulary {
	for _, item := range items {
		if item.Language != language {
			continue
		}
		item.GraduatedAt = nil
		result = append(result, item)
		seen[item.Language+"\x00"+item.CanonicalLemma+"\x00"+item.UPOS+"\x00"+item.LexicalDiscriminator] = struct{}{}
	}
	return result
}

func (s *Store) CountCurrentReadingVocabularyToAccept(_ context.Context, owner, language string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return domain.PlanSnapshotAcceptance(s.fixtureSnapshotIdentitiesLocked(owner, language), s.known).EligibleCount, nil
}

// fixtureSnapshotIdentitiesLocked returns the active reading's frozen identities.
func (s *Store) fixtureSnapshotIdentitiesLocked(owner, language string) []domain.SnapshotIdentity {
	snapshot := s.goalSnapshotVocabularyLocked(owner, language)
	identities := make([]domain.SnapshotIdentity, 0, len(snapshot))
	for _, item := range snapshot {
		identities = append(identities, domain.SnapshotIdentity{Language: item.Language, CanonicalLemma: item.CanonicalLemma, UPOS: item.UPOS, LexicalDiscriminator: item.LexicalDiscriminator, Headword: item.Headword})
	}
	return identities
}

func (s *Store) goalSnapshotVocabularyLocked(owner, language string) []domain.DeckPreparationVocabulary {
	goal := s.currentReadings[fixtureGoalKey(owner, language)]
	if !goal.IsActive() || goal.SnapshotID == "" {
		return nil
	}
	snapshot, exists := s.goalSnapshotVocabulary[goal.SnapshotID]
	if !exists {
		return nil
	}
	result := make([]domain.DeckPreparationVocabulary, 0, len(snapshot))
	for _, item := range snapshot {
		if item.Language == language {
			item.GraduatedAt = nil
			result = append(result, item)
		}
	}
	return result
}

// ListReservedVocabulary returns the vocabulary currently reserved by the
// owner's active reading snapshot, scoped to one language. It is the read seam
// used by the coverage service.
func (s *Store) ListReservedVocabulary(_ context.Context, owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reservedDeckVocabularyLocked(owner, language), nil
}

func (s *Store) GetExtractedUnitSnapshot(context.Context, string, string) (string, domain.ExtractedUnits, error) {
	text := "Haus. Ein kurzer deutscher Satz."
	second := "Ein sehr langer Beispielsatz mit vielen Wörtern für die Anzeige von realistischem Randinhalt im Browser."
	return "fixture-snapshot", domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{
		{ID: domain.EPUBUnitID(0, "fixture-001"), Order: 0, SpineIndex: 0, ManifestID: "fixture-001", Title: "Chapter one", Text: text, StartOffset: 0, EndOffset: uint64(len([]rune(text))), MediaType: "application/xhtml+xml", Linear: true},
		{ID: domain.EPUBUnitID(1, "fixture-002"), Order: 1, SpineIndex: 1, ManifestID: "fixture-002", Title: "Chapter two", Text: second, StartOffset: uint64(len([]rune(text)) + 2), EndOffset: uint64(len([]rune(text)) + 2 + len([]rune(second))), MediaType: "application/xhtml+xml", Linear: true},
	}}, nil
}

func (s *Store) ListCurrentReadingSnapshotVocabulary(_ context.Context, owner, snapshotID string) ([]domain.SelectionCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	goal, found := s.goalSnapshotVocabulary[snapshotID]
	if !found {
		return nil, nil
	}
	result := make([]domain.SelectionCandidate, 0, len(goal))
	for _, item := range goal {
		result = append(result, domain.SelectionCandidate{OwnerID: owner, Language: item.Language, CanonicalLemma: item.CanonicalLemma, UPOS: item.UPOS, LexicalDiscriminator: item.LexicalDiscriminator, Headword: item.Headword})
	}
	return result, nil
}

// fixtureCurrentReadingFor returns the owner's active current reading for a Book
// and the Book's analysis identity. The fixture analysis is the reading's own
// identity, so an active reading always pins its analysis. The caller holds mu.
func (s *Store) fixtureCurrentReadingFor(owner, bookID string) (domain.CurrentReading, domain.CurrentAnalysis) {
	if bookID == "" {
		return domain.CurrentReading{}, domain.CurrentAnalysis{}
	}
	for _, current := range s.currentReadings {
		if current.OwnerID == owner && current.BookID == bookID {
			return current, domain.CurrentAnalysis{SourceMaterialID: current.SourceMaterialID, AnalysisRunID: current.AnalysisRunID, ContentRevisionID: current.ContentRevisionID, ContentSnapshotID: current.ContentSnapshotID, CorpusID: current.CorpusID}
		}
	}
	return domain.CurrentReading{}, domain.CurrentAnalysis{}
}
