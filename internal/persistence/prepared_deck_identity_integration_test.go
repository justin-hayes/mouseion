//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

func TestEnrichmentCacheSeparatesTargetLanguages(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	base := enrichment.CacheKey{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "test", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash("Das Haus ist groß.")}
	english := base
	english.TargetLanguage = "en"
	french := base
	french.TargetLanguage = "fr"
	if _, err = store.Put(ctx, enrichment.CacheEntry{CacheKey: english, Translation: "house", CachedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Put(ctx, enrichment.CacheEntry{CacheKey: french, Translation: "maison", CachedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[enrichment.CacheKey]string{english: "house", french: "maison"} {
		entry, found, getErr := store.Get(ctx, key)
		if getErr != nil || !found || entry.Translation != want {
			t.Fatalf("key=%+v entry=%+v found=%v err=%v", key, entry, found, getErr)
		}
	}
}
