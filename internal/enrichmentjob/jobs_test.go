package enrichmentjob

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
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
	args := JobArgs{OwnerID: "owner-private", Language: "de", Items: []Item{{CanonicalLemma: "haus", UPOS: "noun", ExampleSentence: "Das Haus ist groß."}, {CanonicalLemma: "baum", UPOS: "NOUN"}}}
	encoded, err := json.Marshal(args)
	if err != nil || args.Kind() != "enrich_external_translation" {
		t.Fatalf("kind/json: %q %v", args.Kind(), err)
	}
	if string(encoded) == "" {
		t.Fatal("empty args JSON")
	}
	var progress [][2]int
	worker := &Worker{Enrichment: service, Progress: func(_ context.Context, id int64, completed, total int) error {
		if id != 42 {
			t.Fatalf("job id = %d", id)
		}
		progress = append(progress, [2]int{completed, total})
		return nil
	}}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 42}, Args: args}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(progress, [][2]int{{1, 2}, {2, 2}}) {
		t.Fatalf("progress = %v", progress)
	}
	want := enrichment.TranslationRequest{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", ExampleSentence: "Das Haus ist groß."}
	if len(provider.requests) != 3 || provider.requests[0] != want || provider.requests[1] != want {
		t.Fatalf("requests = %+v", provider.requests)
	}
	key := enrichment.CacheKey{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "model-1"}
	entry := cache.values[key]
	if entry.Translation != "house" || entry.Gloss != "building" || entry.CachedAt.IsZero() {
		t.Fatalf("cache/provenance = %+v", entry)
	}
}

func TestSubmitNoOpWhenExternalDisabled(t *testing.T) {
	service := NewService(nil, nil, enrichment.NewService(enrichment.Config{}, nil, nil, nil, &fakeProvider{}, nil))
	handle, err := service.SubmitEnrichment(context.Background(), "owner", []enrichment.Candidate{{Identity: enrichment.Identity{CanonicalLemma: "haus"}}})
	if err != nil || handle.ID != 0 {
		t.Fatalf("handle=%+v err=%v", handle, err)
	}
}

func TestSubmitRejectsMixedLanguagesBeforeDatabaseWork(t *testing.T) {
	service := NewService(nil, nil, enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, &fakeProvider{}, nil))
	_, err := service.SubmitEnrichment(context.Background(), "owner", []enrichment.Candidate{
		{Identity: enrichment.Identity{Language: "de", CanonicalLemma: "haus"}},
		{Identity: enrichment.Identity{Language: "nl", CanonicalLemma: "huis"}},
	})
	if !errors.Is(err, ErrMixedLanguages) {
		t.Fatalf("err = %v", err)
	}
}

func TestWorkerReturnsProviderFailureForRiverRetry(t *testing.T) {
	provider := &fakeProvider{failures: 2}
	service := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, &fakeCache{values: make(map[enrichment.CacheKey]enrichment.CacheEntry)})
	worker := &Worker{Enrichment: service, Progress: func(context.Context, int64, int, int) error { return nil }}
	err := worker.Work(context.Background(), &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1}, Args: JobArgs{Language: "de", Items: []Item{{CanonicalLemma: "haus", UPOS: "NOUN"}}}})
	if err == nil || len(provider.requests) != 1 {
		t.Fatalf("err=%v calls=%d", err, len(provider.requests))
	}
}
