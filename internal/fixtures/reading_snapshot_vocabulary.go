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
	hasSnapshot := false
	if goal.IsActive() && goal.SnapshotID != "" {
		snapshot, snapshotExists := s.goalSnapshotVocabulary[goal.SnapshotID]
		hasSnapshot = snapshotExists
		result = appendReservedVocabulary(result, seen, snapshot, language)
	}
	if !hasSnapshot && goal.IsActive() && goal.SourceMaterialID != "" {
		for _, preparation := range s.preps {
			if preparation.OwnerID != owner || preparation.SourceMaterialID != goal.SourceMaterialID || preparation.AnalysisRunID != goal.AnalysisRunID {
				continue
			}
			result = appendReservedVocabulary(result, seen, s.deckVocabularyFor(owner, preparation.ID), language)
		}
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

// ListUnattachedGeneratedVocabulary returns the legacy Generated vocabulary
// history the coverage service still reads for the owner and language.
func (s *Store) ListUnattachedGeneratedVocabulary(_ context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []domain.GeneratedVocabulary
	for _, item := range s.legacyGenerated {
		if item.OwnerID == owner && item.Language == language {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *Store) deckVocabularyFor(owner, preparationID string) []domain.DeckPreparationVocabulary {
	var result []domain.DeckPreparationVocabulary
	for _, item := range s.deckVocabulary {
		if item.OwnerID == owner && item.DeckPreparationID == preparationID {
			result = append(result, item)
		}
	}
	return result
}
