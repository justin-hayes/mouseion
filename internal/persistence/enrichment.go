package persistence

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

func (s *PostgresStore) Get(ctx context.Context, key enrichment.CacheKey) (entry enrichment.CacheEntry, found bool, err error) {
	entry.CacheKey = key
	err = s.pool.QueryRow(ctx, `SELECT translation,gloss,cached_at FROM enrichment_cache WHERE language=$1 AND canonical_lemma=$2 AND upos=$3 AND provider=$4 AND provider_version=$5`, key.Language, key.CanonicalLemma, key.UPOS, key.Provider, key.ProviderVersion).Scan(&entry.Translation, &entry.Gloss, &entry.CachedAt)
	if err == pgx.ErrNoRows {
		return enrichment.CacheEntry{}, false, nil
	}
	return entry, err == nil, err
}

// Put is immutable. On a racing insert, the first stored value is returned.
func (s *PostgresStore) Put(ctx context.Context, entry enrichment.CacheEntry) (enrichment.CacheEntry, error) {
	_, err := s.pool.Exec(ctx, `INSERT INTO enrichment_cache(language,canonical_lemma,upos,provider,provider_version,translation,gloss,cached_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, entry.Language, entry.CanonicalLemma, entry.UPOS, entry.Provider, entry.ProviderVersion, entry.Translation, entry.Gloss, entry.CachedAt)
	if err != nil {
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
