package persistence

import (
	"context"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

func (s *PostgresStore) Get(ctx context.Context, key enrichment.CacheKey) (entry enrichment.CacheEntry, found bool, err error) {
	entry.CacheKey = key
	row, err := s.queries().GetEnrichmentCache(ctx, sqlcgen.GetEnrichmentCacheParams{Language: key.Language, TargetLanguage: key.TargetLanguage, CanonicalLemma: key.CanonicalLemma, Upos: key.UPOS, Provider: key.Provider, ProviderVersion: key.ProviderVersion, SentenceHash: key.SentenceHash})
	if err == pgx.ErrNoRows {
		return enrichment.CacheEntry{}, false, nil
	}
	if err != nil {
		return entry, false, err
	}
	entry.Translation, entry.Gloss = row.Translation, row.Gloss
	entry.SentenceTranslation, entry.SentenceTranslationTarget = row.SentenceTranslation, row.SentenceTranslationTarget
	entry.CachedAt = row.CachedAt
	return entry, true, nil
}

// Put is immutable. On a racing insert, the first stored value is returned.
func (s *PostgresStore) Put(ctx context.Context, entry enrichment.CacheEntry) (enrichment.CacheEntry, error) {
	if err := s.queries().UpsertEnrichmentCache(ctx, sqlcgen.UpsertEnrichmentCacheParams{Language: entry.Language, TargetLanguage: entry.TargetLanguage, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Provider: entry.Provider, ProviderVersion: entry.ProviderVersion, SentenceHash: entry.SentenceHash, Translation: entry.Translation, Gloss: entry.Gloss, SentenceTranslation: entry.SentenceTranslation, SentenceTranslationTarget: entry.SentenceTranslationTarget, CachedAt: entry.CachedAt}); err != nil {
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
