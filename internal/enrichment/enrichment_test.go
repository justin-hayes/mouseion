package enrichment

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"
)

type memoryCache struct {
	values     map[CacheKey]CacheEntry
	gets, puts int
}

func (c *memoryCache) Get(_ context.Context, k CacheKey) (CacheEntry, bool, error) {
	c.gets++
	v, ok := c.values[k]
	return v, ok, nil
}
func (c *memoryCache) Put(_ context.Context, v CacheEntry) (CacheEntry, error) {
	c.puts++
	if old, ok := c.values[v.CacheKey]; ok {
		return old, nil
	}
	c.values[v.CacheKey] = v
	return v, nil
}

type translationStub struct {
	name, version string
	requests      []TranslationRequest
	failures      int
}

func (s *translationStub) Name() string    { return s.name }
func (s *translationStub) Version() string { return s.version }
func (s *translationStub) Translate(_ context.Context, r TranslationRequest) (TranslationResponse, error) {
	s.requests = append(s.requests, r)
	if len(s.requests) <= s.failures {
		return TranslationResponse{}, errors.New("unavailable")
	}
	return TranslationResponse{Translation: "house", Gloss: "a building for people", SentenceTranslation: "The house is large.", SentenceTranslationTarget: "house"}, nil
}

type frequencyStub struct{}

func (frequencyStub) Name() string    { return "DWDS" }
func (frequencyStub) Version() string { return "2026-08" }
func (frequencyStub) Frequency(context.Context, Identity) (float64, bool, error) {
	return .75, true, nil
}

func TestServiceLocalPipelineNeedsNoExternalProvider(t *testing.T) {
	c := Candidate{Identity: Identity{"de", "haus", "NOUN"}, Morphology: map[string]string{"Number": "Sing"}}
	r := NewService(Config{}, frequencyStub{}, NewStanzaMorphology("1.8"), GermanIPA{}, nil, nil).Enrich(context.Background(), []Candidate{c})[0]
	if !r.Frequency.Available || !r.Morphology.Available || !r.Pronunciation.Available || r.Translation.Available || len(r.Warnings) != 0 {
		t.Fatalf("result=%+v", r)
	}
	if r.Frequency.Provenance.Provider != "DWDS" || r.Frequency.Provenance.External {
		t.Fatalf("provenance=%+v", r.Frequency.Provenance)
	}
}

func TestExternalRequiresAdminAndUserConsent(t *testing.T) {
	for _, cfg := range []Config{{ExternalEnabled: true}, {UserOptIn: true}, {}} {
		provider := &translationStub{name: "llm", version: "1"}
		NewService(cfg, nil, nil, nil, provider, nil).Enrich(context.Background(), []Candidate{{Identity: Identity{"de", "haus", "NOUN"}}})
		if len(provider.requests) != 0 {
			t.Fatalf("external request with config %+v", cfg)
		}
	}
}

func TestTranslationPrivacyContextAndCacheSharing(t *testing.T) {
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	provider := &translationStub{name: "llm", version: "model-1"}
	s := NewService(Config{ExternalEnabled: true, UserOptIn: true, ContextMode: SentenceContext}, nil, nil, nil, provider, cache)
	s.now = func() time.Time { return time.Date(2026, 8, 21, 1, 2, 3, 0, time.UTC) }
	c := Candidate{Identity: Identity{"de", "haus", "noun"}, TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}
	first := s.Enrich(context.Background(), []Candidate{c})[0]
	// A second user's identity cannot affect the shared key because it is not
	// accepted by either Candidate identity or TranslationRequest.
	second := s.Enrich(context.Background(), []Candidate{c})[0]
	want := TranslationRequest{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}
	if len(provider.requests) != 1 || provider.requests[0] != want {
		t.Fatalf("requests=%+v", provider.requests)
	}
	if cache.puts != 1 || !second.Translation.Available || !second.SentenceTranslation.Available || !second.SentenceTranslationTarget.Available || first.Translation.Provenance.CachedAt.IsZero() {
		t.Fatalf("first=%+v second=%+v cache=%+v", first, second, cache)
	}
}

func TestExternalObservationCountsCacheProviderRetriesAndBoundedErrors(t *testing.T) {
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	provider := &translationStub{name: "llm", version: "model-1"}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 2, RetryBaseDelay: time.Nanosecond}, nil, nil, nil, provider, cache)
	candidate := Candidate{Identity: Identity{"de", "haus", "NOUN"}, TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}

	result, metrics, err := service.EnrichExternalObserved(context.Background(), candidate)
	if err != nil || !result.SentenceTranslation.Available {
		t.Fatalf("first result=%+v metrics=%+v err=%v", result, metrics, err)
	}
	if metrics.CacheMisses != 1 || metrics.CacheHits != 0 || metrics.ProviderCalls != 1 || metrics.Attempts != 1 || metrics.Retries != 0 || metrics.CacheLatency < 0 || metrics.ProviderLatency < 0 {
		t.Fatalf("first metrics=%+v", metrics)
	}

	_, metrics, err = service.EnrichExternalObserved(context.Background(), candidate)
	if err != nil || metrics.CacheHits != 1 || metrics.CacheMisses != 0 || metrics.ProviderCalls != 0 || metrics.Attempts != 0 {
		t.Fatalf("cached metrics=%+v err=%v", metrics, err)
	}

	failing := &classifiedProvider{err: &LLMHTTPError{StatusCode: http.StatusTooManyRequests, Message: "private provider response"}}
	service = NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 2, RetryBaseDelay: time.Nanosecond}, nil, nil, nil, failing, &memoryCache{values: map[CacheKey]CacheEntry{}})
	_, metrics, err = service.EnrichExternalObserved(context.Background(), candidate)
	if err == nil || metrics.ProviderCalls != 1 || metrics.Attempts != 2 || metrics.Retries != 1 || metrics.RateLimitErrors != 1 || metrics.OtherErrors != 0 {
		t.Fatalf("failure metrics=%+v err=%v", metrics, err)
	}
}

type classifiedProvider struct{ err error }

func (*classifiedProvider) Name() string    { return "fake" }
func (*classifiedProvider) Version() string { return "1" }
func (p *classifiedProvider) Translate(context.Context, TranslationRequest) (TranslationResponse, error) {
	if p.err != nil {
		return TranslationResponse{}, p.err
	}
	return TranslationResponse{Translation: "house", SentenceTranslation: "The house is large."}, nil
}

func TestExternalErrorClassification(t *testing.T) {
	tests := []struct {
		err  error
		want ExternalErrorClass
	}{
		{context.Canceled, ExternalErrorCancellation},
		{context.DeadlineExceeded, ExternalErrorTimeout},
		{&LLMHTTPError{StatusCode: http.StatusTooManyRequests}, ExternalErrorRateLimit},
		{&LLMHTTPError{StatusCode: http.StatusServiceUnavailable}, ExternalErrorProvider5xx},
		{errors.New("opaque provider failure"), ExternalErrorOther},
	}
	for _, test := range tests {
		if got := ClassifyExternalError(test.err); got != test.want {
			t.Errorf("ClassifyExternalError(%T)=%q want %q", test.err, got, test.want)
		}
	}
}

func TestSentenceHashConservativeNormalizationAndSeparation(t *testing.T) {
	first := SentenceHash("  Das Haus ist groß.\r\n")
	if first == "" || len(first) != 64 || first != SentenceHash("Das Haus ist groß.\n") {
		t.Fatalf("unexpected deterministic hash %q", first)
	}
	for _, sentence := range []string{"Das Haus ist groß!", "Das  Haus ist groß.", "das Haus ist groß."} {
		if got := SentenceHash(sentence); got == first {
			t.Fatalf("meaningful difference collided for %q", sentence)
		}
	}
	if SentenceHash(" \r\n ") != "" {
		t.Fatal("blank sentence should use legacy lemma-only identity")
	}
}

func TestSameLemmaDifferentSentencesUseSeparateCacheEntries(t *testing.T) {
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	provider := &translationStub{name: "llm", version: "model-1"}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, cache)
	for _, sentence := range []string{"Das Haus ist groß.", "Das Haus ist alt.", "Das Haus ist groß."} {
		result := service.Enrich(context.Background(), []Candidate{{Identity: Identity{"de", "haus", "NOUN"}, ExampleSentence: sentence}})[0]
		if !result.SentenceTranslation.Available {
			t.Fatalf("sentence translation unavailable for %q", sentence)
		}
	}
	if len(provider.requests) != 2 || len(cache.values) != 2 || cache.puts != 2 {
		t.Fatalf("requests=%d entries=%d puts=%d", len(provider.requests), len(cache.values), cache.puts)
	}
}

func TestSentenceTranslationTargetIsCachedWithCompleteTranslation(t *testing.T) {
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	provider := &translationStub{name: "llm", version: "model-1"}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, cache)
	candidate := Candidate{Identity: Identity{"de", "haus", "NOUN"}, TargetWord: "‹Haus›", ExampleSentence: "Das Haus ist groß."}
	first := service.Enrich(context.Background(), []Candidate{candidate})[0]
	if !first.SentenceTranslationTarget.Available || first.SentenceTranslationTarget.Value != "house" || len(provider.requests) != 1 {
		t.Fatalf("result=%+v requests=%d", first, len(provider.requests))
	}
	if len(cache.values) != 1 || cache.values[CacheKey{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "model-1", SentenceHash: SentenceHash(candidate.ExampleSentence)}].SentenceTranslationTarget != "house" {
		t.Fatalf("cache=%+v", cache.values)
	}
}

func TestProviderWithoutSentenceTranslationRemainsCompatible(t *testing.T) {
	provider, err := NewDictionaryProvider("dict", "1", func(context.Context, TranslationRequest) (TranslationResponse, error) {
		return TranslationResponse{Translation: "house", Gloss: "building"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, nil).
		Enrich(context.Background(), []Candidate{{Identity: Identity{"de", "haus", "NOUN"}, ExampleSentence: "Das Haus."}})[0]
	if !result.Translation.Available || !result.Gloss.Available || result.SentenceTranslation.Available || len(result.Warnings) != 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestLemmaOnlyRetriesAndGracefulFailure(t *testing.T) {
	provider := &translationStub{name: "dictionary", version: "1", failures: 2}
	r := NewService(Config{ExternalEnabled: true, UserOptIn: true, ContextMode: LemmaOnly, MaxAttempts: 2}, nil, nil, nil, provider, nil).Enrich(context.Background(), []Candidate{{Identity: Identity{"de", "haus", "NOUN"}, ExampleSentence: "private context"}})[0]
	if r.Translation.Available || len(r.Warnings) != 1 || len(provider.requests) != 2 {
		t.Fatalf("result=%+v calls=%d", r, len(provider.requests))
	}
	for _, req := range provider.requests {
		if req.ExampleSentence != "" || req.TargetWord != "" {
			t.Fatalf("lemma-only leaked sentence-derived context: %+v", req)
		}
	}
}

func TestProviderVersionInvalidatesCache(t *testing.T) {
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	c := Candidate{Identity: Identity{"de", "haus", "NOUN"}}
	p1 := &translationStub{name: "llm", version: "1"}
	NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, p1, cache).Enrich(context.Background(), []Candidate{c})
	p2 := &translationStub{name: "llm", version: "2"}
	NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, p2, cache).Enrich(context.Background(), []Candidate{c})
	if len(p1.requests) != 1 || len(p2.requests) != 1 || len(cache.values) != 2 {
		t.Fatalf("calls=%d/%d entries=%d", len(p1.requests), len(p2.requests), len(cache.values))
	}
}

func TestDeterministicLocalProviders(t *testing.T) {
	c := Candidate{Identity: Identity{"de", "Straße", "NOUN"}, Morphology: map[string]string{"Case": "Nom"}}
	ip := GermanIPA{}
	a, ok, _ := ip.Pronunciation(context.Background(), c.Identity)
	b, _, _ := ip.Pronunciation(context.Background(), c.Identity)
	if !ok || a != b || a != "/ʃtrase/" {
		t.Fatalf("IPA=%q/%q", a, b)
	}
	m := NewStanzaMorphology("1")
	got, ok, _ := m.Morphology(context.Background(), c)
	got["Case"] = "Acc"
	if !ok || reflect.DeepEqual(got, c.Morphology) {
		t.Fatalf("morphology was not copied")
	}
}

func TestDictionaryAdapter(t *testing.T) {
	p, err := NewDictionaryProvider("dict", "2026", func(_ context.Context, r TranslationRequest) (TranslationResponse, error) {
		return TranslationResponse{Translation: r.CanonicalLemma}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Translate(context.Background(), TranslationRequest{CanonicalLemma: "Haus"})
	if err != nil || got.Translation != "Haus" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
