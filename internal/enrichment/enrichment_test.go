package enrichment

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasRequiredTranslationFields(t *testing.T) {
	complete := CacheEntry{Translation: "house", FallbackGloss: "dwelling", SentenceTranslation: "The house."}
	assert.True(t, HasRequiredTranslationFields(complete, "Das Haus."), "complete cache entry was rejected")
	for _, entry := range []CacheEntry{{FallbackGloss: "dwelling", SentenceTranslation: "The house."}, {Translation: "house", FallbackGloss: "dwelling"}} {
		assert.False(t, HasRequiredTranslationFields(entry, "Das Haus."), "incomplete cache entry accepted: %+v", entry)
	}
	assert.True(t, HasRequiredTranslationFields(CacheEntry{Translation: "house", SentenceTranslation: "The house."}, "Das Haus."), "translation with sentence was rejected")
	assert.True(t, HasRequiredTranslationFields(CacheEntry{Translation: "house"}, ""), "lemma-only cache entry was rejected")
}

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

type emptyCandidateFallbackStub struct {
	requests []TranslationRequest
}

func (*emptyCandidateFallbackStub) Name() string    { return "llm" }
func (*emptyCandidateFallbackStub) Version() string { return "model-1" }
func (s *emptyCandidateFallbackStub) Translate(_ context.Context, r TranslationRequest) (TranslationResponse, error) {
	s.requests = append(s.requests, r)
	response := TranslationResponse{Translation: "rare word", SentenceTranslation: "The rare thing is important today.", SentenceTranslationTarget: "rare"}
	if len(r.CandidateSenses) == 0 {
		response.FallbackGloss = "something uncommon"
	} else {
		response.SenseOrder = []int{0}
	}
	return response, nil
}

func (s *translationStub) Name() string    { return s.name }
func (s *translationStub) Version() string { return s.version }
func (s *translationStub) Translate(_ context.Context, r TranslationRequest) (TranslationResponse, error) {
	s.requests = append(s.requests, r)
	if len(s.requests) <= s.failures {
		return TranslationResponse{}, errors.New("unavailable")
	}
	return TranslationResponse{Translation: "house", FallbackGloss: "a building for people", SentenceTranslation: "The house is large.", SentenceTranslationTarget: "house"}, nil
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
	assert.True(t, r.Frequency.Available)
	assert.True(t, r.Morphology.Available)
	assert.True(t, r.Pronunciation.Available)
	assert.False(t, r.Translation.Available)
	assert.Len(t, r.Warnings, 0)
	assert.Equal(t, "DWDS", r.Frequency.Provenance.Provider)
	assert.False(t, r.Frequency.Provenance.External)
}

func TestExternalRequiresAdminAndUserConsent(t *testing.T) {
	for _, cfg := range []Config{{ExternalEnabled: true}, {UserOptIn: true}, {}} {
		provider := &translationStub{name: "llm", version: "1"}
		NewService(cfg, nil, nil, nil, provider, nil).Enrich(context.Background(), []Candidate{{Identity: Identity{"de", "haus", "NOUN"}}})
		assert.Empty(t, provider.requests, "external request with config %+v", cfg)
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
	want := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}
	require.Len(t, provider.requests, 1)
	assert.Equal(t, want, provider.requests[0])
	assert.Equal(t, 1, cache.puts)
	assert.True(t, second.Translation.Available)
	assert.True(t, second.SentenceTranslation.Available)
	assert.True(t, second.SentenceTranslationTarget.Available)
	assert.False(t, first.Translation.Provenance.CachedAt.IsZero(), "first=%+v second=%+v cache=%+v", first, second, cache)
}

func TestExternalFallbackGlossIsCachedAndUsesDictionaryIdentity(t *testing.T) {
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	provider := &translationStub{name: "llm", version: "model-1"}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, cache)
	candidate := Candidate{Identity: Identity{"de", "seltenes-wort", "NOUN"}, TargetWord: "Seltenes", ExampleSentence: "Das Seltene ist heute wichtig."}
	// The dictionary version is part of the frozen candidate identity, not the
	// provider's version.
	candidate.DictionaryProviderVersion = "dictionary-v4"

	result, err := service.EnrichExternal(context.Background(), candidate)
	require.NoError(t, err)
	assert.Equal(t, "a building for people", result.FallbackGloss.Value)
	key, ok := service.ExternalCacheKey(candidate)
	require.True(t, ok)
	assert.Equal(t, "dictionary-v4", key.DictionaryProviderVersion)
	assert.Equal(t, "a building for people", cache.values[key].FallbackGloss)

	otherDictionary := candidate
	otherDictionary.DictionaryProviderVersion = "dictionary-v5"
	_, err = service.EnrichExternal(context.Background(), otherDictionary)
	require.NoError(t, err)
	assert.Len(t, provider.requests, 2, "regenerated dictionary must not reuse the old selection")
}

func TestExternalEmptyCandidateFallbackIsCachedAndReused(t *testing.T) {
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	provider := &emptyCandidateFallbackStub{}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, cache)
	candidate := Candidate{
		Identity:                  Identity{"de", "seltenes-wort", "NOUN"},
		TargetWord:                "Seltenes",
		ExampleSentence:           "Das Seltene ist heute wichtig.",
		DictionaryProviderVersion: "dictionary-v4",
	}

	first, err := service.EnrichExternal(context.Background(), candidate)
	require.NoError(t, err)
	require.Len(t, provider.requests, 1)
	assert.Empty(t, provider.requests[0].CandidateSenses)
	assert.Equal(t, "something uncommon", first.FallbackGloss.Value)

	key, ok := service.ExternalCacheKey(candidate)
	require.True(t, ok)
	assert.Equal(t, "dictionary-v4", key.DictionaryProviderVersion)
	assert.Equal(t, "something uncommon", cache.values[key].FallbackGloss)

	second, err := service.EnrichExternal(context.Background(), candidate)
	require.NoError(t, err)
	assert.Equal(t, "something uncommon", second.FallbackGloss.Value)
	assert.Len(t, provider.requests, 1, "cached empty-candidate fallback called provider again")

	otherDictionary := candidate
	otherDictionary.DictionaryProviderVersion = "dictionary-v5"
	third, err := service.EnrichExternal(context.Background(), otherDictionary)
	require.NoError(t, err)
	assert.Equal(t, "something uncommon", third.FallbackGloss.Value)
	assert.Len(t, provider.requests, 2, "regenerated dictionary must not reuse the old fallback")

}

func TestExternalObservationCountsCacheProviderRetriesAndBoundedErrors(t *testing.T) {
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	provider := &translationStub{name: "llm", version: "model-1"}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 2, RetryBaseDelay: time.Nanosecond}, nil, nil, nil, provider, cache)
	candidate := Candidate{Identity: Identity{"de", "haus", "NOUN"}, TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}

	result, metrics, err := service.EnrichExternalObserved(context.Background(), candidate)
	require.NoError(t, err)
	assert.True(t, result.SentenceTranslation.Available)
	assert.Equal(t, 1, metrics.CacheMisses)
	assert.Zero(t, metrics.CacheHits)
	assert.Equal(t, 1, metrics.ProviderCalls)
	assert.Equal(t, 1, metrics.Attempts)
	assert.Zero(t, metrics.Retries)
	assert.GreaterOrEqual(t, metrics.CacheLatency, time.Duration(0))
	assert.GreaterOrEqual(t, metrics.ProviderLatency, time.Duration(0))

	_, metrics, err = service.EnrichExternalObserved(context.Background(), candidate)
	require.NoError(t, err)
	assert.Equal(t, 1, metrics.CacheHits)
	assert.Zero(t, metrics.CacheMisses)
	assert.Zero(t, metrics.ProviderCalls)
	assert.Zero(t, metrics.Attempts)

	failing := &classifiedProvider{err: &LLMHTTPError{StatusCode: http.StatusTooManyRequests, Message: "private provider response"}}
	service = NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 2, RetryBaseDelay: time.Nanosecond}, nil, nil, nil, failing, &memoryCache{values: map[CacheKey]CacheEntry{}})
	_, metrics, err = service.EnrichExternalObserved(context.Background(), candidate)
	require.Error(t, err)
	assert.Equal(t, 1, metrics.ProviderCalls)
	assert.Equal(t, 2, metrics.Attempts)
	assert.Equal(t, 1, metrics.Retries)
	assert.Equal(t, 1, metrics.RateLimitErrors)
	assert.Zero(t, metrics.OtherErrors)
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
		assert.Equal(t, test.want, ClassifyExternalError(test.err), "ClassifyExternalError(%T)", test.err)
	}
}

func TestSentenceHashConservativeNormalizationAndSeparation(t *testing.T) {
	first := SentenceHash("  Das Haus ist groß.\r\n")
	require.Len(t, first, 64)
	assert.Equal(t, first, SentenceHash("Das Haus ist groß.\n"), "unexpected deterministic hash %q", first)
	for _, sentence := range []string{"Das Haus ist groß!", "Das  Haus ist groß.", "das Haus ist groß."} {
		assert.NotEqual(t, first, SentenceHash(sentence), "meaningful difference collided for %q", sentence)
	}
	assert.Empty(t, SentenceHash(" \r\n "), "blank sentence should use legacy lemma-only identity")
}

func TestSameLemmaDifferentSentencesUseSeparateCacheEntries(t *testing.T) {
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	provider := &translationStub{name: "llm", version: "model-1"}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, cache)
	for _, sentence := range []string{"Das Haus ist groß.", "Das Haus ist alt.", "Das Haus ist groß."} {
		result := service.Enrich(context.Background(), []Candidate{{Identity: Identity{"de", "haus", "NOUN"}, ExampleSentence: sentence}})[0]
		assert.True(t, result.SentenceTranslation.Available, "sentence translation unavailable for %q", sentence)
	}
	assert.Len(t, provider.requests, 2)
	assert.Len(t, cache.values, 2)
	assert.Equal(t, 2, cache.puts)
}

func TestSentenceTranslationTargetIsCachedWithCompleteTranslation(t *testing.T) {
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	provider := &translationStub{name: "llm", version: "model-1"}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, cache)
	candidate := Candidate{Identity: Identity{"de", "haus", "NOUN"}, TargetWord: "‹Haus›", ExampleSentence: "Das Haus ist groß."}
	first := service.Enrich(context.Background(), []Candidate{candidate})[0]
	assert.True(t, first.SentenceTranslationTarget.Available)
	assert.Equal(t, "house", first.SentenceTranslationTarget.Value)
	assert.Len(t, provider.requests, 1)
	assert.Len(t, cache.values, 1)
	assert.Equal(t, "house", cache.values[CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "model-1", SentenceHash: SentenceHash(candidate.ExampleSentence)}].SentenceTranslationTarget)
}

func TestLemmaOnlyRetriesAndGracefulFailure(t *testing.T) {
	provider := &translationStub{name: "dictionary", version: "1", failures: 2}
	r := NewService(Config{ExternalEnabled: true, UserOptIn: true, ContextMode: LemmaOnly, MaxAttempts: 2}, nil, nil, nil, provider, nil).Enrich(context.Background(), []Candidate{{Identity: Identity{"de", "haus", "NOUN"}, ExampleSentence: "private context"}})[0]
	assert.False(t, r.Translation.Available)
	require.Len(t, r.Warnings, 1)
	assert.Len(t, provider.requests, 2)
	for _, req := range provider.requests {
		assert.Empty(t, req.ExampleSentence, "lemma-only leaked sentence-derived context: %+v", req)
		assert.Empty(t, req.TargetWord, "lemma-only leaked sentence-derived context: %+v", req)
	}
}

func TestProviderVersionInvalidatesCache(t *testing.T) {
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	c := Candidate{Identity: Identity{"de", "haus", "NOUN"}}
	p1 := &translationStub{name: "llm", version: "1"}
	NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, p1, cache).Enrich(context.Background(), []Candidate{c})
	p2 := &translationStub{name: "llm", version: "2"}
	NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, p2, cache).Enrich(context.Background(), []Candidate{c})
	assert.Len(t, p1.requests, 1)
	assert.Len(t, p2.requests, 1)
	assert.Len(t, cache.values, 2)
}

func TestDeterministicLocalProviders(t *testing.T) {
	c := Candidate{Identity: Identity{"de", "Straße", "NOUN"}, Morphology: map[string]string{"Case": "Nom"}}
	ip := GermanIPA{}
	a, ok, _ := ip.Pronunciation(context.Background(), c.Identity)
	b, _, _ := ip.Pronunciation(context.Background(), c.Identity)
	assert.True(t, ok)
	assert.Equal(t, a, b)
	assert.Equal(t, "/ʃtrase/", a)
	m := NewStanzaMorphology("1")
	got, ok, _ := m.Morphology(context.Background(), c)
	got["Case"] = "Acc"
	assert.True(t, ok)
	assert.NotEqual(t, c.Morphology, got, "morphology was not copied")
}
