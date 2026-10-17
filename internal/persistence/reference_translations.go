package persistence

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
)

// errInvalidReferenceTranslation refuses a caller-supplied reference
// translation that cannot be stored.
var errInvalidReferenceTranslation = errors.New("persistence: invalid reference translation")

// PutReferenceTranslations upserts the operator-imported English translation of
// each cited section and returns how many sections were written. A re-import of
// the same pinned file writes the same rows, so an already-prepared deck never
// changes.
func (s *PostgresStore) PutReferenceTranslations(ctx context.Context, ownerID, language, sourcePin string, translations map[string]string) (int, error) {
	if s == nil || strings.TrimSpace(ownerID) == "" || strings.TrimSpace(language) == "" || strings.TrimSpace(sourcePin) == "" || len(translations) == 0 {
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
				OwnerID: ownerID, Language: language, Citation: citation, Translation: translations[citation], SourcePin: sourcePin,
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

// ListReferenceTranslations loads an owner's imported translations for one
// language together with the pin they were imported from. An installation
// without an imported translation reports an empty map and an empty pin.
func (s *PostgresStore) ListReferenceTranslations(ctx context.Context, ownerID, language string) (map[string]string, string, error) {
	if s == nil {
		return nil, "", ErrInvalidTransition
	}
	return listReferenceTranslations(ctx, s.pool, ownerID, language)
}

// listReferenceTranslations is the transaction-scoped form of
// ListReferenceTranslations.
func listReferenceTranslations(ctx context.Context, q sqlcgen.DBTX, ownerID, language string) (map[string]string, string, error) {
	if strings.TrimSpace(ownerID) == "" || strings.TrimSpace(language) == "" {
		return map[string]string{}, "", nil
	}
	rows, err := sqlcgen.New(q).ListReferenceTranslations(ctx, sqlcgen.ListReferenceTranslationsParams{OwnerID: ownerID, Language: language})
	if err != nil {
		return nil, "", err
	}
	translations := make(map[string]string, len(rows))
	pin := ""
	for _, row := range rows {
		if pin == "" {
			pin = row.SourcePin
		}
		translations[row.Citation] = row.Translation
	}
	if len(translations) == 0 {
		return map[string]string{}, "", nil
	}
	return translations, pin, nil
}

// mergedReferenceTranslations loads and merges one owner's imported
// translations across the languages a preparation's candidates use, and reports
// the pin they share. A preparation with no imported translation reports empty
// values. Two non-empty pins for the same owner are an inconsistency and are
// refused rather than silently relabelled.
func mergedReferenceTranslations(ctx context.Context, q sqlcgen.DBTX, ownerID string, languages map[string]struct{}) (map[string]string, string, error) {
	merged := make(map[string]string)
	pin := ""
	ordered := make([]string, 0, len(languages))
	for language := range languages {
		ordered = append(ordered, language)
	}
	sort.Strings(ordered)
	for _, language := range ordered {
		translations, languagePin, err := listReferenceTranslations(ctx, q, ownerID, language)
		if err != nil {
			return nil, "", err
		}
		if languagePin != "" {
			if pin != "" && pin != languagePin {
				return nil, "", fmt.Errorf("persistence: conflicting reference translation pins for owner %s", ownerID)
			}
			pin = languagePin
		}
		maps.Copy(merged, translations)
	}
	return merged, pin, nil
}
