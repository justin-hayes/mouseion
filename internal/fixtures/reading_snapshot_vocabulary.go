package fixtures

// Contract status: contractual. These reads must match internal/storecontract
// scenarios shared with PostgreSQL (ADR 0088).

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListReservedVocabulary returns no vocabulary: the Current reading reserves
// nothing (ADR 0136 §4). It remains as the read seam the coverage service uses
// until that read is removed.
func (s *Store) ListReservedVocabulary(context.Context, string, string) ([]domain.DeckPreparationVocabulary, error) {
	return nil, nil
}

func (s *Store) GetExtractedUnitSnapshot(context.Context, string, string) (string, domain.ExtractedUnits, error) {
	text := "Haus. Ein kurzer deutscher Satz."
	second := "Ein sehr langer Beispielsatz mit vielen Wörtern für die Anzeige von realistischem Randinhalt im Browser."
	return "fixture-snapshot", domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{
		{ID: domain.EPUBUnitID(0, "fixture-001"), Order: 0, SpineIndex: 0, ManifestID: "fixture-001", Title: "Chapter one", Text: text, StartOffset: 0, EndOffset: uint64(len([]rune(text))), MediaType: "application/xhtml+xml", Linear: true},
		{ID: domain.EPUBUnitID(1, "fixture-002"), Order: 1, SpineIndex: 1, ManifestID: "fixture-002", Title: "Chapter two", Text: second, StartOffset: uint64(len([]rune(text)) + 2), EndOffset: uint64(len([]rune(text)) + 2 + len([]rune(second))), MediaType: "application/xhtml+xml", Linear: true},
	}}, nil
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
