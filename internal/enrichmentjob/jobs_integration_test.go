//go:build integration

package enrichmentjob

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river/rivertype"
)

func TestRiverEnrichmentLifecycleCacheProgressAndOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	provider := &fakeProvider{}
	enrich := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 2, RetryBaseDelay: time.Millisecond}, nil, nil, nil, provider, store)
	client, err := NewClient(store.Pool(), enrich)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store.Pool(), client, enrich)
	cancelledHandle, err := service.SubmitEnrichment(ctx, "11111111-1111-1111-1111-111111111111", []enrichment.Candidate{{Identity: enrichment.Identity{Language: "de", CanonicalLemma: "weg", UPOS: "NOUN"}, ExampleSentence: "Der lange Weg führt heute durch den stillen Wald."}})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := service.Cancel(ctx, "11111111-1111-1111-1111-111111111111", cancelledHandle.ID)
	if err != nil || cancelled.State != rivertype.JobStateCancelled {
		t.Fatalf("cancelled status=%+v err=%v", cancelled, err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	candidates := []enrichment.Candidate{{Identity: enrichment.Identity{Language: "de", CanonicalLemma: "haus", UPOS: "noun"}, ExampleSentence: "Das Haus ist groß."}, {Identity: enrichment.Identity{Language: "de", CanonicalLemma: "baum", UPOS: "NOUN"}, ExampleSentence: "Der Baum ist groß."}}
	handle, err := service.SubmitEnrichment(ctx, "11111111-1111-1111-1111-111111111111", candidates)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Get(ctx, "22222222-2222-2222-2222-222222222222", handle.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner get = %v", err)
	}
	var status Status
	for {
		status, err = service.Get(ctx, "11111111-1111-1111-1111-111111111111", handle.ID)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == rivertype.JobStateCompleted {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	if status.Completed != 2 || status.Total != 2 || status.Attempt != 1 {
		t.Fatalf("status = %+v", status)
	}
	for _, candidate := range candidates {
		key := enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: "NOUN", Provider: "llm", ProviderVersion: "model-1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
		entry, found, err := store.Get(ctx, key)
		if err != nil || !found || entry.Translation != "house" || entry.CachedAt.IsZero() {
			t.Fatalf("cache entry=%+v found=%v err=%v", entry, found, err)
		}
	}
	before := len(provider.requests)
	second, err := service.SubmitEnrichment(ctx, "22222222-2222-2222-2222-222222222222", candidates)
	if err != nil {
		t.Fatal(err)
	}
	for {
		status, err = service.Get(ctx, "22222222-2222-2222-2222-222222222222", second.ID)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == rivertype.JobStateCompleted {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	if len(provider.requests) != before {
		t.Fatalf("shared cache missed: calls %d -> %d", before, len(provider.requests))
	}
}
