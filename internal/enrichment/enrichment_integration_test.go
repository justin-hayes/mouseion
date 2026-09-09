//go:build integration

package enrichment_test

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestPostgresExternalCacheSharedScopedVersionedAndImmutable(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	when := time.Date(2026, 8, 21, 2, 3, 4, 0, time.UTC)
	key := enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "1", SentenceHash: enrichment.SentenceHash("Das Haus ist groß.")}
	stored, err := store.Put(ctx, enrichment.CacheEntry{CacheKey: key, Translation: "house", Gloss: "building", SentenceTranslation: "The house is large.", SentenceTranslationTarget: "house", CachedAt: when})
	if err != nil {
		t.Fatal(err)
	}
	// No owner is part of the API or schema: all users read the same entry.
	for range 2 {
		got, found, err := store.Get(ctx, key)
		if err != nil || !found || got.Translation != "house" || got.SentenceTranslation != "The house is large." || got.SentenceTranslationTarget != "house" || !got.CachedAt.Equal(stored.CachedAt) {
			t.Fatalf("got=%+v found=%v err=%v", got, found, err)
		}
	}
	other := key
	other.Language = "nl"
	if _, found, err := store.Get(ctx, other); err != nil || found {
		t.Fatalf("language isolation found=%v err=%v", found, err)
	}
	other = key
	other.ProviderVersion = "2"
	if _, found, err := store.Get(ctx, other); err != nil || found {
		t.Fatalf("version isolation found=%v err=%v", found, err)
	}
	other = key
	other.SentenceHash = enrichment.SentenceHash("Das Haus ist alt.")
	if _, found, err := store.Get(ctx, other); err != nil || found {
		t.Fatalf("sentence isolation found=%v err=%v", found, err)
	}
	second, err := store.Put(ctx, enrichment.CacheEntry{CacheKey: other, Translation: "house", SentenceTranslation: "The house is old.", CachedAt: when})
	if err != nil || second.SentenceTranslation != "The house is old." {
		t.Fatalf("second sentence put=%+v err=%v", second, err)
	}
	again, err := store.Put(ctx, enrichment.CacheEntry{CacheKey: key, Translation: "changed", CachedAt: when.Add(time.Hour)})
	if err != nil || again.Translation != "house" || again.SentenceTranslation != "The house is large." {
		t.Fatalf("immutable put=%+v err=%v", again, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE enrichment_cache SET translation='changed' WHERE language='de'`); err == nil {
		t.Fatal("direct update unexpectedly succeeded")
	}
}
