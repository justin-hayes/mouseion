package persistence

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
)

// errInvalidReferenceTranslation refuses a caller-supplied reference
// translation that cannot be stored.
var errInvalidReferenceTranslation = errors.New("persistence: invalid reference translation")

// PutReferenceTranslations upserts the installation-wide English translation of
// each cited section of one pinned translation file and returns how many
// sections were written. A re-import of the same pinned file writes the same
// rows, so an already-prepared deck never changes. The rows carry no owner:
// every learner and every Book of a Work share them.
func (s *PostgresStore) PutReferenceTranslations(ctx context.Context, pinSHA256, language string, translations map[string]string) (int, error) {
	if s == nil || !isLowerHexSHA256String(pinSHA256) || strings.TrimSpace(language) == "" || len(translations) == 0 {
		return 0, errInvalidReferenceTranslation
	}
	citations := make([]string, 0, len(translations))
	for citation, text := range translations {
		if strings.TrimSpace(citation) == "" || strings.TrimSpace(text) == "" {
			return 0, errInvalidReferenceTranslation
		}
		citations = append(citations, citation)
	}
	sort.Strings(citations)
	err := withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		for _, citation := range citations {
			if err := q.UpsertReferenceTranslation(ctx, sqlcgen.UpsertReferenceTranslationParams{
				TranslationPinSha256: pinSHA256, Language: language, Citation: citation, Translation: translations[citation],
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(citations), nil
}

// ListReferenceTranslationsByPin loads every cited section of one pinned
// translation file. An installation without that translation reports an empty
// map, which makes deck preparation abstain from passage context.
func (s *PostgresStore) ListReferenceTranslationsByPin(ctx context.Context, pinSHA256 string) (map[string]string, error) {
	if s == nil {
		return nil, ErrInvalidTransition
	}
	return listReferenceTranslationsByPin(ctx, s.pool, pinSHA256)
}

// listReferenceTranslationsByPin is the transaction-scoped form of
// ListReferenceTranslationsByPin.
func listReferenceTranslationsByPin(ctx context.Context, q sqlcgen.DBTX, pinSHA256 string) (map[string]string, error) {
	if !isLowerHexSHA256String(pinSHA256) {
		return map[string]string{}, nil
	}
	rows, err := sqlcgen.New(q).ListReferenceTranslationsByPin(ctx, pinSHA256)
	if err != nil {
		return nil, err
	}
	translations := make(map[string]string, len(rows))
	for _, row := range rows {
		translations[row.Citation] = row.Translation
	}
	return translations, nil
}

// HasReferenceTranslations reports whether any cited section of a pinned
// translation file is already stored. Translation import skips its download
// when this is true.
func (s *PostgresStore) HasReferenceTranslations(ctx context.Context, pinSHA256 string) (bool, error) {
	if s == nil {
		return false, ErrInvalidTransition
	}
	if !isLowerHexSHA256String(pinSHA256) {
		return false, nil
	}
	return sqlcgen.New(s.pool).HasReferenceTranslations(ctx, pinSHA256)
}

// isLowerHexSHA256String reports whether value is a 64-character lowercase
// hexadecimal SHA-256 digest.
func isLowerHexSHA256String(value string) bool {
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
