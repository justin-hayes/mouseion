package analysis

import (
	"context"
	"errors"
	"strings"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// ReferenceTranslationWriter persists an operator-imported English reference
// translation keyed by citation. It never calls the NLP or translation provider.
type ReferenceTranslationWriter struct {
	store *persistence.PostgresStore
}

func NewReferenceTranslationWriter(store *persistence.PostgresStore) *ReferenceTranslationWriter {
	return &ReferenceTranslationWriter{store: store}
}

// WriteReferenceTranslations upserts the cited sections and returns how many
// were written. A re-import of the same pinned file writes the same rows.
func (w *ReferenceTranslationWriter) WriteReferenceTranslations(ctx context.Context, in ancientgreek.ReferenceTranslationWriteRequest) (int, error) {
	if w == nil || w.store == nil {
		return 0, errors.New("analysis: reference translation writer store is required")
	}
	if strings.TrimSpace(in.OwnerID) == "" {
		return 0, errors.New("analysis: reference translation owner is required")
	}
	translations := make(map[string]string, len(in.Translations))
	for _, translation := range in.Translations {
		if _, duplicate := translations[translation.Citation]; duplicate {
			return 0, errors.New("analysis: duplicate reference translation citation")
		}
		translations[translation.Citation] = translation.Text
	}
	return w.store.PutReferenceTranslations(ctx, in.OwnerID, in.Language, in.SourcePin, translations)
}
