//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnrichmentCacheItemKindIsPartOfTheIdentity proves a lemma result can
// never be served for an idiom with the same text, or vice versa.
func TestEnrichmentCacheItemKindIsPartOfTheIdentity(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)

	lemmaKey := enrichment.CacheKey{
		Language: "it", TargetLanguage: "en", CanonicalLemma: "gamba", UPOS: "NOUN",
		Provider: "test-provider", ProviderVersion: "test-v1",
		SentenceHash: enrichment.SentenceHash("È un ragazzo in gamba."),
	}
	_, err := store.Put(ctx, enrichment.CacheEntry{
		CacheKey: lemmaKey, Translation: "leg",
		SentenceTranslation: "He is a capable young man.",
	})
	require.NoError(t, err)

	idiomKey := lemmaKey
	idiomKey.ItemKind = enrichment.ItemKindIdiom
	_, found, err := store.Get(ctx, idiomKey)
	require.NoError(t, err)
	assert.False(t, found, "an idiom must not read the lemma cache row")

	_, err = store.Put(ctx, enrichment.CacheEntry{
		CacheKey: idiomKey, Translation: "capable",
		SentenceTranslation: "He is a capable young man.",
	})
	require.NoError(t, err)

	idiomEntry, found, err := store.Get(ctx, idiomKey)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "capable", idiomEntry.Translation)

	lemmaEntry, found, err := store.Get(ctx, lemmaKey)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "leg", lemmaEntry.Translation, "the idiom must not overwrite the lemma row")
}

// TestEnrichmentCacheItemKindDefaultsToLemma proves the additive migration
// keeps every pre-kind row and insert a lemma, and constrains the kind.
func TestEnrichmentCacheItemKindDefaultsToLemma(t *testing.T) {
	ctx := context.Background()
	_, pool := testutil.Postgres(t, ctx, Migrate)

	insert := func(kind string) error {
		if kind == "" {
			_, err := pool.Exec(ctx, `
				INSERT INTO enrichment_cache(
					language, canonical_lemma, upos, provider, provider_version,
					sentence_hash, target_language, translation, fallback_gloss,
					sense_selection
				) VALUES ('de', 'kind-default', 'NOUN', 'migration-test', '1',
					'', 'en', 'translated', '', '[]')`)
			return err
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO enrichment_cache(
				language, canonical_lemma, upos, item_kind, provider, provider_version,
				sentence_hash, target_language, translation, fallback_gloss,
				sense_selection
			) VALUES ('de', 'kind-default', 'NOUN', $1, 'migration-test', '1',
				'', 'en', 'translated', '', '[]')`, kind)
		return err
	}

	require.NoError(t, insert(""))

	var kind string
	require.NoError(t, pool.QueryRow(ctx, `SELECT item_kind FROM enrichment_cache WHERE canonical_lemma='kind-default'`).Scan(&kind))
	assert.Equal(t, string(enrichment.ItemKindLemma), kind, "an insert without a kind is a lemma")

	require.NoError(t, insert(string(enrichment.ItemKindIdiom)), "an idiom is a separate identity with the same text")
	require.Error(t, insert("phrase"), "the kind is constrained to lemma or idiom")
}
