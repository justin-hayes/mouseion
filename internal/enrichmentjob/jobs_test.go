package enrichmentjob

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProvider struct {
	requests []enrichment.TranslationRequest
	failures int
}

func (*fakeProvider) Name() string    { return "llm" }
func (*fakeProvider) Version() string { return "model-1" }
func (p *fakeProvider) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.requests = append(p.requests, request)
	if len(p.requests) <= p.failures {
		return enrichment.TranslationResponse{}, errors.New("temporary provider failure")
	}
	return enrichment.TranslationResponse{Translation: "house", Gloss: "building"}, nil
}

type fakeCache struct {
	values map[enrichment.CacheKey]enrichment.CacheEntry
}

func (c *fakeCache) Get(_ context.Context, key enrichment.CacheKey) (enrichment.CacheEntry, bool, error) {
	entry, ok := c.values[key]
	return entry, ok, nil
}
func (c *fakeCache) Put(_ context.Context, entry enrichment.CacheEntry) (enrichment.CacheEntry, error) {
	if stored, ok := c.values[entry.CacheKey]; ok {
		return stored, nil
	}
	c.values[entry.CacheKey] = entry
	return entry, nil
}

func TestJobArgsAndWorkerProgressRetryPrivacy(t *testing.T) {
	provider := &fakeProvider{failures: 1}
	cache := &fakeCache{values: make(map[enrichment.CacheKey]enrichment.CacheEntry)}
	service := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, ContextMode: enrichment.SentenceContext, MaxAttempts: 2, RetryBaseDelay: time.Nanosecond}, nil, nil, nil, provider, cache)
	args := JobArgs{OwnerID: "owner-private", Language: "de", Items: []Item{{CanonicalLemma: "haus", UPOS: "noun", TargetWord: "‹Haus›", ExampleSentence: "Das Haus ist groß."}, {CanonicalLemma: "baum", UPOS: "NOUN"}}}
	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	assert.Equal(t, "enrich_external_translation", args.Kind())
	assert.NotEmpty(t, string(encoded), "empty args JSON")
	var progress [][2]int
	worker := &Worker{Enrichment: service, Progress: func(_ context.Context, id int64, completed, total int) error {
		assert.Equal(t, int64(42), id, "job id")
		progress = append(progress, [2]int{completed, total})
		return nil
	}}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 42}, Args: args}
	require.NoError(t, worker.Work(context.Background(), job))
	assert.Equal(t, [][2]int{{1, 2}, {2, 2}}, progress)
	want := enrichment.TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}
	require.Len(t, provider.requests, 3)
	assert.Equal(t, want, provider.requests[0])
	assert.Equal(t, want, provider.requests[1])
	key := enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "model-1", SentenceHash: enrichment.SentenceHash("Das Haus ist groß.")}
	entry := cache.values[key]
	assert.Equal(t, "house", entry.Translation)
	assert.Equal(t, "building", entry.Gloss)
	assert.False(t, entry.CachedAt.IsZero(), "cache/provenance = %+v", entry)
}

func TestSubmitNoOpWithoutExternalConfigurationOrConsent(t *testing.T) {
	for name, tc := range map[string]struct {
		config   enrichment.Config
		provider enrichment.TranslationProvider
	}{
		"disabled":             {enrichment.Config{}, &fakeProvider{}},
		"no consent":           {enrichment.Config{ExternalEnabled: true}, &fakeProvider{}},
		"provider unavailable": {enrichment.Config{ExternalEnabled: true, UserOptIn: true}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			service := NewService(nil, nil, enrichment.NewService(tc.config, nil, nil, nil, tc.provider, nil))
			handle, err := service.SubmitEnrichment(context.Background(), "owner", []enrichment.Candidate{{Identity: enrichment.Identity{Language: "de", CanonicalLemma: "haus"}, ExampleSentence: "Das Haus ist heute sehr ruhig."}})
			require.NoError(t, err)
			assert.Zero(t, handle.ID)
		})
	}
}

func TestSubmitNoOpWhenSentencesAreEmpty(t *testing.T) {
	service := NewService(nil, nil, enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, &fakeProvider{}, nil))
	handle, err := service.SubmitEnrichment(context.Background(), "owner", []enrichment.Candidate{{Identity: enrichment.Identity{Language: "de", CanonicalLemma: "haus"}, ExampleSentence: "  "}})
	require.NoError(t, err)
	assert.Zero(t, handle.ID)
}

func TestSubmitRejectsMixedLanguagesBeforeDatabaseWork(t *testing.T) {
	service := NewService(nil, nil, enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, &fakeProvider{}, nil))
	_, err := service.SubmitEnrichment(context.Background(), "owner", []enrichment.Candidate{
		{Identity: enrichment.Identity{Language: "de", CanonicalLemma: "haus"}},
		{Identity: enrichment.Identity{Language: "nl", CanonicalLemma: "huis"}},
	})
	assert.ErrorIs(t, err, ErrMixedLanguages)
}

func TestWorkerReturnsProviderFailureForRiverRetry(t *testing.T) {
	provider := &fakeProvider{failures: 2}
	service := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, &fakeCache{values: make(map[enrichment.CacheKey]enrichment.CacheEntry)})
	worker := &Worker{Enrichment: service, Progress: func(context.Context, int64, int, int) error { return nil }}
	err := worker.Work(context.Background(), &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1}, Args: JobArgs{Language: "de", Items: []Item{{CanonicalLemma: "haus", UPOS: "NOUN"}}}})
	require.Error(t, err)
	assert.Len(t, provider.requests, 1)
}

func TestWorkerResumesAfterRecordedProgress(t *testing.T) {
	provider := &fakeProvider{}
	service := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, &fakeCache{values: make(map[enrichment.CacheKey]enrichment.CacheEntry)})
	worker := &Worker{Enrichment: service, Progress: func(context.Context, int64, int, int) error { return nil }}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1, Metadata: []byte(`{"completed":1,"total":2}`)}, Args: JobArgs{Language: "de", Items: []Item{{CanonicalLemma: "haus", UPOS: "NOUN", ExampleSentence: "Das Haus ist heute sehr ruhig."}, {CanonicalLemma: "baum", UPOS: "NOUN", ExampleSentence: "Der Baum ist heute besonders schön gewachsen."}}}}
	require.NoError(t, worker.Work(context.Background(), job))
	require.Len(t, provider.requests, 1)
	assert.Equal(t, "baum", provider.requests[0].CanonicalLemma)
}
