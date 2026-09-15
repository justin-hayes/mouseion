//go:build integration

package enrichment_test

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
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
	key := enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "1", DictionaryProviderVersion: "dictionary-v4", SentenceHash: enrichment.SentenceHash("Das Haus ist groß.")}
	stored, err := store.Put(ctx, enrichment.CacheEntry{CacheKey: key, Translation: "house", FallbackGloss: "building", SentenceTranslation: "The house is large.", SentenceTranslationTarget: "house", CachedAt: when})
	require.NoError(t, err)
	// No owner is part of the API or schema: all users read the same entry.
	for range 2 {
		got, found, err := store.Get(ctx, key)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "house", got.Translation)
		assert.Equal(t, "building", got.FallbackGloss)
		assert.Equal(t, "dictionary-v4", got.CacheKey.DictionaryProviderVersion)
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
	other.DictionaryProviderVersion = "dictionary-v5"
	_, found, err = store.Get(ctx, other)
	require.NoError(t, err)
	assert.False(t, found, "dictionary version isolation")
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

type postgresFallbackProvider struct {
	calls int
}

func (*postgresFallbackProvider) Name() string    { return "llm" }
func (*postgresFallbackProvider) Version() string { return "model-1" }
func (p *postgresFallbackProvider) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.calls++
	response := enrichment.TranslationResponse{
		Translation:         "rare word",
		SentenceTranslation: "The rare thing is important today.",
	}
	if len(request.CandidateSenses) == 0 {
		response.FallbackGloss = "something uncommon"
	}
	return response, nil
}

func TestPostgresExternalEmptyCandidateFallbackPersistsByDictionaryIdentity(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()

	provider := &postgresFallbackProvider{}
	service := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, ContextMode: enrichment.SentenceContext}, nil, nil, nil, provider, store)
	entry := cardexport.Entry{
		Language: "de", CanonicalLemma: "seltenes-wort", UPOS: "NOUN", Sentence: "Das seltene Wort ist heute wirklich wichtig.", TargetWord: "seltene",
		DictionaryProviderVersion: "dictionary-v4", FirstEncounter: 1,
	}
	manifest := cardexport.NewManifest("owner-1", "Book", []cardexport.Entry{entry})
	require.Len(t, manifest.EnrichmentCandidates(), 1)
	candidate := manifest.EnrichmentCandidates()[0]

	first, err := service.EnrichExternal(ctx, candidate)
	require.NoError(t, err)
	assert.Equal(t, "something uncommon", first.FallbackGloss.Value)
	assert.Equal(t, 1, provider.calls)

	second, err := service.EnrichExternal(ctx, candidate)
	require.NoError(t, err)
	assert.Equal(t, "something uncommon", second.FallbackGloss.Value)
	assert.Equal(t, 1, provider.calls, "durable cache miss for unchanged dictionary identity")

	key, ok := service.ExternalCacheKey(candidate)
	require.True(t, ok)
	stored, found, err := store.Get(ctx, key)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "something uncommon", stored.FallbackGloss)
	assert.Equal(t, "dictionary-v4", stored.CacheKey.DictionaryProviderVersion)
	assert.Empty(t, stored.SenseSelection)

	bound, err := manifest.BindCacheKeys([]enrichment.CacheKey{key})
	require.NoError(t, err)
	artifact, err := cardexport.NewService(nil).RenderManifest(ctx, bound, []cardexport.ExactEnrichment{{CacheKey: key, Result: first}})
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 1)
	assert.Equal(t, "something uncommon", artifact.Generated[0].Note.Gloss)
	assert.Equal(t, 1, artifact.Completeness.CardsWithFallbackGloss)

	otherDictionary := candidate
	otherDictionary.DictionaryProviderVersion = "dictionary-v5"
	_, err = service.EnrichExternal(ctx, otherDictionary)
	require.NoError(t, err)
	assert.Equal(t, 2, provider.calls, "dictionary identity did not invalidate the durable fallback")
}
