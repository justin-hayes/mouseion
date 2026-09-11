//go:build integration

package enrichment_test

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresExternalCacheSharedScopedVersionedAndImmutable(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()
	when := time.Date(2026, 8, 21, 2, 3, 4, 0, time.UTC)
	key := enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "1", SentenceHash: enrichment.SentenceHash("Das Haus ist groß.")}
	stored, err := store.Put(ctx, enrichment.CacheEntry{CacheKey: key, Translation: "house", Gloss: "building", SentenceTranslation: "The house is large.", SentenceTranslationTarget: "house", CachedAt: when})
	require.NoError(t, err)
	// No owner is part of the API or schema: all users read the same entry.
	for range 2 {
		got, found, err := store.Get(ctx, key)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "house", got.Translation)
		assert.Equal(t, "The house is large.", got.SentenceTranslation)
		assert.Equal(t, "house", got.SentenceTranslationTarget)
		assert.True(t, got.CachedAt.Equal(stored.CachedAt))
	}
	other := key
	other.Language = "nl"
	_, found, err := store.Get(ctx, other)
	require.NoError(t, err)
	assert.False(t, found, "language isolation")
	other = key
	other.ProviderVersion = "2"
	_, found, err = store.Get(ctx, other)
	require.NoError(t, err)
	assert.False(t, found, "version isolation")
	other = key
	other.SentenceHash = enrichment.SentenceHash("Das Haus ist alt.")
	_, found, err = store.Get(ctx, other)
	require.NoError(t, err)
	assert.False(t, found, "sentence isolation")
	second, err := store.Put(ctx, enrichment.CacheEntry{CacheKey: other, Translation: "house", SentenceTranslation: "The house is old.", CachedAt: when})
	require.NoError(t, err)
	assert.Equal(t, "The house is old.", second.SentenceTranslation)
	again, err := store.Put(ctx, enrichment.CacheEntry{CacheKey: key, Translation: "changed", CachedAt: when.Add(time.Hour)})
	require.NoError(t, err)
	assert.Equal(t, "house", again.Translation)
	assert.Equal(t, "The house is large.", again.SentenceTranslation)
	_, err = pool.Exec(ctx, `UPDATE enrichment_cache SET translation='changed' WHERE language='de'`)
	assert.Error(t, err, "direct update unexpectedly succeeded")
}
