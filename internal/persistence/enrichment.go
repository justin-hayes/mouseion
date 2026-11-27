package persistence

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

func (s *PostgresStore) Get(ctx context.Context, key enrichment.CacheKey) (entry enrichment.CacheEntry, found bool, err error) {
	entry.CacheKey = key
	row, err := s.queries().GetEnrichmentCache(ctx, sqlcgen.GetEnrichmentCacheParams{Language: key.Language, TargetLanguage: key.TargetLanguage, CanonicalLemma: key.CanonicalLemma, Upos: key.UPOS, ItemKind: string(key.ItemKind.Normalized()), Provider: key.Provider, ProviderVersion: key.ProviderVersion, SentenceHash: key.SentenceHash, DictionaryProviderVersion: key.DictionaryProviderVersion, MeaningEvidenceHash: key.MeaningEvidenceHash})
	if errors.Is(err, pgx.ErrNoRows) {
		return enrichment.CacheEntry{}, false, nil
	}
	if err != nil {
		return entry, false, err
	}
	entry, err = cacheEntryFromRow(key, row)
	return entry, true, err
}

// Put is immutable. On a racing insert, the first stored value is returned.
func (s *PostgresStore) Put(ctx context.Context, entry enrichment.CacheEntry) (enrichment.CacheEntry, error) {
	selection, err := marshalSenseSelection(entry.SenseSelection)
	if err != nil {
		return enrichment.CacheEntry{}, err
	}
	targets := append([]string{}, entry.SentenceTranslationTargets...)
	if err := s.queries().UpsertEnrichmentCache(ctx, sqlcgen.UpsertEnrichmentCacheParams{Language: entry.Language, TargetLanguage: entry.TargetLanguage, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, ItemKind: string(entry.ItemKind.Normalized()), Provider: entry.Provider, ProviderVersion: entry.ProviderVersion, SentenceHash: entry.SentenceHash, DictionaryProviderVersion: entry.DictionaryProviderVersion, MeaningEvidenceHash: entry.MeaningEvidenceHash, Translation: entry.Translation, FallbackGloss: entry.FallbackGloss, SenseSelection: selection, SentenceTranslation: entry.SentenceTranslation, SentenceTranslationTarget: entry.SentenceTranslationTarget, SentenceTranslationTargets: targets, CachedAt: entry.CachedAt}); err != nil {
		return enrichment.CacheEntry{}, err
	}
	stored, found, err := s.Get(ctx, entry.CacheKey)
	if err != nil {
		return enrichment.CacheEntry{}, err
	}
	if !found {
		return enrichment.CacheEntry{}, ErrNotFound
	}
	return stored, nil
}

func cacheEntryFromRow(key enrichment.CacheKey, row sqlcgen.GetEnrichmentCacheRow) (enrichment.CacheEntry, error) {
	entry := enrichment.CacheEntry{CacheKey: key, Translation: row.Translation, FallbackGloss: row.FallbackGloss, SentenceTranslation: row.SentenceTranslation, SentenceTranslationTarget: row.SentenceTranslationTarget, SentenceTranslationTargets: append([]string(nil), row.SentenceTranslationTargets...), CachedAt: row.CachedAt}
	if len(row.SenseSelection) > 0 {
		if err := json.Unmarshal(row.SenseSelection, &entry.SenseSelection); err != nil {
			return enrichment.CacheEntry{}, err
		}
	}
	return entry, nil
}

func marshalSenseSelection(selection []int) ([]byte, error) {
	if selection == nil {
		selection = []int{}
	}
	return json.Marshal(selection)
}
