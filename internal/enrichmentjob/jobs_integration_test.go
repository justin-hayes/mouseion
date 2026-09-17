//go:build integration

package enrichmentjob

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiverEnrichmentLifecycleCacheProgressAndOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "enrichment store", store.Close)
	provider := &fakeProvider{}
	enrich := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 2, RetryBaseDelay: time.Millisecond}, nil, nil, nil, provider, store)
	client, err := NewClient(store.Pool(), enrich)
	require.NoError(t, err)
	service := NewService(store.Pool(), client, enrich)
	cancelledHandle, err := service.SubmitEnrichment(ctx, "11111111-1111-1111-1111-111111111111", []enrichment.Candidate{{Identity: enrichment.Identity{Language: "de", CanonicalLemma: "weg", UPOS: "NOUN"}, ExampleSentence: "Der lange Weg führt heute durch den stillen Wald."}})
	require.NoError(t, err)
	cancelled, err := service.Cancel(ctx, "11111111-1111-1111-1111-111111111111", cancelledHandle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCancelled, cancelled.State)
	require.NoError(t, client.Start(ctx))
	testutil.Cleanup(t, "enrichment job client", func() error {
		return client.Stop(context.Background())
	})
	candidates := []enrichment.Candidate{{Identity: enrichment.Identity{Language: "de", CanonicalLemma: "haus", UPOS: "noun"}, ExampleSentence: "Das Haus ist groß."}, {Identity: enrichment.Identity{Language: "de", CanonicalLemma: "baum", UPOS: "NOUN"}, ExampleSentence: "Der Baum ist groß."}}
	handle, err := service.SubmitEnrichment(ctx, "11111111-1111-1111-1111-111111111111", candidates)
	require.NoError(t, err)
	_, err = service.Get(ctx, "22222222-2222-2222-2222-222222222222", handle.ID)
	assert.ErrorIs(t, err, ErrNotFound, "cross-owner get")
	var status Status
	for {
		status, err = service.Get(ctx, "11111111-1111-1111-1111-111111111111", handle.ID)
		require.NoError(t, err)
		if status.State == rivertype.JobStateCompleted {
			break
		}
		select {
		case <-ctx.Done():
			require.Fail(t, ctx.Err().Error())
		case <-time.After(20 * time.Millisecond):
		}
	}
	assert.Equal(t, int64(2), status.Completed)
	assert.Equal(t, int64(2), status.Total)
	assert.Equal(t, 1, status.Attempt)
	for _, candidate := range candidates {
		key := enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: "NOUN", Provider: "llm", ProviderVersion: "model-1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
		entry, found, err := store.Get(ctx, key)
		require.NoError(t, err)
		assert.True(t, found, "cache entry found")
		assert.Equal(t, "house", entry.Translation)
		assert.False(t, entry.CachedAt.IsZero(), "cache entry cached at")
	}
	before := len(provider.requests)
	second, err := service.SubmitEnrichment(ctx, "22222222-2222-2222-2222-222222222222", candidates)
	require.NoError(t, err)
	for {
		status, err = service.Get(ctx, "22222222-2222-2222-2222-222222222222", second.ID)
		require.NoError(t, err)
		if status.State == rivertype.JobStateCompleted {
			break
		}
		select {
		case <-ctx.Done():
			require.Fail(t, ctx.Err().Error())
		case <-time.After(20 * time.Millisecond):
		}
	}
	assert.Equal(t, before, len(provider.requests), "shared cache missed: calls %d -> %d", before, len(provider.requests))
}
