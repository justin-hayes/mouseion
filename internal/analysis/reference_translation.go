package analysis

import (
	"context"
	"errors"
	"strings"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// ReferenceTranslationWriter persists an operator-imported English reference
// translation keyed by the installation-wide translation pin and citation. It
// never calls the NLP or translation provider.
type ReferenceTranslationWriter struct {
	store *persistence.PostgresStore
}

func NewReferenceTranslationWriter(store *persistence.PostgresStore) *ReferenceTranslationWriter {
	return &ReferenceTranslationWriter{store: store}
}

// WriteReferenceTranslations upserts the cited sections and returns how many
// were written. A re-import of the same pinned file writes the same rows for
// every learner.
func (w *ReferenceTranslationWriter) WriteReferenceTranslations(ctx context.Context, in ancientgreek.ReferenceTranslationWriteRequest) (int, error) {
	if w == nil || w.store == nil {
		return 0, errors.New("analysis: reference translation writer store is required")
	}
	if !isLowerHexSHA256(in.TranslationPinSHA256) {
		return 0, errors.New("analysis: reference translation pin is required")
	}
	if strings.TrimSpace(in.Language) == "" {
		return 0, errors.New("analysis: reference translation language is required")
	}
	translations := make(map[string]string, len(in.Translations))
	for _, translation := range in.Translations {
		if _, duplicate := translations[translation.Citation]; duplicate {
			return 0, errors.New("analysis: duplicate reference translation citation")
		}
		translations[translation.Citation] = translation.Text
	}
	return w.store.PutReferenceTranslations(ctx, in.TranslationPinSHA256, in.Language, translations)
}

// isLowerHexSHA256 reports whether value is a 64-character lowercase
// hexadecimal SHA-256 digest.
func isLowerHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
