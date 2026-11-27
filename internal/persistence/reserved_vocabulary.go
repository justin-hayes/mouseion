package persistence

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListReservedVocabulary returns the vocabulary currently reserved by the
// owner's active reading snapshot, scoped to one language. Reserved vocabulary
// is neither counted as known nor eligible for another snapshot until the
// reading is changed, cleared, or completed.
func (s *PostgresStore) ListReservedVocabulary(ctx context.Context, owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	return listReservedVocabulary(ctx, s.pool, owner, language)
}
