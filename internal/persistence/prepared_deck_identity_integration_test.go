//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnrichmentCacheSeparatesTargetLanguages(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()

	base := enrichment.CacheKey{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "test", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash("Das Haus ist groß.")}
	english := base
	english.TargetLanguage = "en"
	french := base
	french.TargetLanguage = "fr"
	_, err = store.Put(ctx, enrichment.CacheEntry{CacheKey: english, Translation: "house", CachedAt: time.Now().UTC()})
	require.NoError(t, err)
	_, err = store.Put(ctx, enrichment.CacheEntry{CacheKey: french, Translation: "maison", CachedAt: time.Now().UTC()})
	require.NoError(t, err)
	for key, want := range map[enrichment.CacheKey]string{english: "house", french: "maison"} {
		entry, found, getErr := store.Get(ctx, key)
		require.NoError(t, getErr)
		assert.True(t, found, "key=%+v", key)
		assert.Equal(t, want, entry.Translation, "key=%+v", key)
	}
}
