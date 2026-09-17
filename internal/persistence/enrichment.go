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
	row, err := s.queries().GetEnrichmentCache(ctx, sqlcgen.GetEnrichmentCacheParams{Language: key.Language, TargetLanguage: key.TargetLanguage, CanonicalLemma: key.CanonicalLemma, Upos: key.UPOS, Provider: key.Provider, ProviderVersion: key.ProviderVersion, SentenceHash: key.SentenceHash, DictionaryProviderVersion: key.DictionaryProviderVersion})
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
	if err := s.queries().UpsertEnrichmentCache(ctx, sqlcgen.UpsertEnrichmentCacheParams{Language: entry.Language, TargetLanguage: entry.TargetLanguage, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Provider: entry.Provider, ProviderVersion: entry.ProviderVersion, SentenceHash: entry.SentenceHash, DictionaryProviderVersion: entry.DictionaryProviderVersion, Translation: entry.Translation, FallbackGloss: entry.FallbackGloss, SenseSelection: selection, SentenceTranslation: entry.SentenceTranslation, SentenceTranslationTarget: entry.SentenceTranslationTarget, CachedAt: entry.CachedAt}); err != nil {
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

// PutPreparedDeckTranslationIfClaimed keeps the provider side effect behind
// the translation claim fence. The claim row is locked until the immutable
// cache insert and read commit together.
func (s *PostgresStore) PutPreparedDeckTranslationIfClaimed(ctx context.Context, owner, preparationID, runID string, ordinal, generation int, token string, entry enrichment.CacheEntry) (enrichment.CacheEntry, error) {
	selection, err := marshalSenseSelection(entry.SenseSelection)
	if err != nil {
		return enrichment.CacheEntry{}, err
	}
	var stored enrichment.CacheEntry
	err = withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var claimMarker int
		err := tx.QueryRow(ctx, `SELECT 1 FROM deck_preparation_translation_outcomes WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND ordinal=$4 AND dispatch_generation=$5 AND state='running' AND claim_token=$6 AND lease_expires_at > clock_timestamp() FOR UPDATE`, owner, preparationID, runID, ordinal, generation, uuidArg(token)).Scan(&claimMarker)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPreparedDeckClaimLost
		}
		if err != nil {
			return err
		}
		q := sqlcgen.New(tx)
		if err := q.UpsertEnrichmentCache(ctx, sqlcgen.UpsertEnrichmentCacheParams{Language: entry.Language, TargetLanguage: entry.TargetLanguage, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Provider: entry.Provider, ProviderVersion: entry.ProviderVersion, SentenceHash: entry.SentenceHash, DictionaryProviderVersion: entry.DictionaryProviderVersion, Translation: entry.Translation, FallbackGloss: entry.FallbackGloss, SenseSelection: selection, SentenceTranslation: entry.SentenceTranslation, SentenceTranslationTarget: entry.SentenceTranslationTarget, CachedAt: entry.CachedAt}); err != nil {
			return err
		}
		row, err := q.GetEnrichmentCache(ctx, sqlcgen.GetEnrichmentCacheParams{Language: entry.Language, TargetLanguage: entry.TargetLanguage, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Provider: entry.Provider, ProviderVersion: entry.ProviderVersion, SentenceHash: entry.SentenceHash, DictionaryProviderVersion: entry.DictionaryProviderVersion})
		if err != nil {
			return err
		}
		stored, err = cacheEntryFromRow(entry.CacheKey, row)
		return err
	})
	return stored, err
}

func cacheEntryFromRow(key enrichment.CacheKey, row sqlcgen.GetEnrichmentCacheRow) (enrichment.CacheEntry, error) {
	entry := enrichment.CacheEntry{CacheKey: key, Translation: row.Translation, FallbackGloss: row.FallbackGloss, SentenceTranslation: row.SentenceTranslation, SentenceTranslationTarget: row.SentenceTranslationTarget, CachedAt: row.CachedAt}
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
