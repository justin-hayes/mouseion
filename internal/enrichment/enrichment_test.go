package enrichment

import (
	"context"
	"errors"
	"reflect"
	"strings"
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
	context       string
}

func (s *translationStub) Name() string    { return s.name }
func (s *translationStub) Version() string { return s.version }
func (s *translationStub) Translate(_ context.Context, r TranslationRequest) (TranslationResponse, error) {
	s.requests = append(s.requests, r)
	if len(s.requests) <= s.failures {
		return TranslationResponse{}, errors.New("unavailable")
	}
	return TranslationResponse{Translation: "house", Gloss: "a building for people", SentenceTranslation: "The house is large.", ContextSentence: s.context}, nil
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
	if cache.puts != 1 || !second.Translation.Available || !second.SentenceTranslation.Available || first.Translation.Provenance.CachedAt.IsZero() {
		t.Fatalf("first=%+v second=%+v cache=%+v", first, second, cache)
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

func TestContextSentenceIsValidatedAndCachedWithSafeFallback(t *testing.T) {
	source := "Das Haus steht am Rand," + strings.Repeat(" während die Kinder im großen Garten spielen", 10) + "."
	cache := &memoryCache{values: map[CacheKey]CacheEntry{}}
	provider := &translationStub{name: "llm", version: "model-1", context: "Das Haus steht am Rand"}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, cache)
	candidate := Candidate{Identity: Identity{"de", "haus", "NOUN"}, TargetWord: "Haus", ExampleSentence: source}
	first := service.Enrich(context.Background(), []Candidate{candidate})[0]
	if !first.ContextSentence.Available || first.ContextSentence.Value != provider.context {
		t.Fatalf("context=%+v", first.ContextSentence)
	}
	entry := cache.values[CacheKey{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "model-1", SentenceHash: SentenceHash(source)}]
	if entry.ContextSentence != provider.context {
		t.Fatalf("cached context=%q", entry.ContextSentence)
	}
	second := service.Enrich(context.Background(), []Candidate{candidate})[0]
	if second.ContextSentence.Value != provider.context || len(provider.requests) != 1 {
		t.Fatalf("second=%+v requests=%d", second, len(provider.requests))
	}

	provider.context = "Das Haus wurde paraphrasiert"
	uncached := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, nil).
		Enrich(context.Background(), []Candidate{candidate})[0]
	if uncached.ContextSentence.Value != source {
		t.Fatalf("invalid context did not fall back to source: %q", uncached.ContextSentence.Value)
	}
}

func TestLongContextThresholdUsesWordOrUnicodeCharacterLimit(t *testing.T) {
	if NeedsShortContext(strings.Repeat("x ", LongContextWordLimit)) {
		t.Fatal("exactly the word limit should not trigger shortening")
	}
	if !NeedsShortContext(strings.Repeat("x ", LongContextWordLimit+1)) {
		t.Fatal("one word over the limit should trigger shortening")
	}
	if NeedsShortContext(strings.Repeat("x", LongContextCharacterLimit)) {
		t.Fatal("exactly the Unicode character limit should not trigger shortening")
	}
	if !NeedsShortContext(strings.Repeat("x", LongContextCharacterLimit+1)) {
		t.Fatal("one Unicode character over the limit should trigger shortening")
	}
}

func TestValidatedContextSentenceRejectsUnsafeOrNonContiguousOutput(t *testing.T) {
	source := "Das Haus steht am Rand des stillen Waldes," + strings.Repeat(" während die Kinder im Garten spielen", 10) + "."
	for _, test := range []struct {
		name, target, proposed, want string
	}{
		{name: "valid substring", target: "Haus", proposed: "Das Haus steht am Rand", want: "Das Haus steht am Rand"},
		{name: "missing target", target: "Haus", proposed: "Das Gebäude steht am Rand", want: source},
		{name: "paraphrase", target: "Haus", proposed: "Das Haus befindet sich am Rand", want: source},
		{name: "inside larger word", target: "Haus", proposed: "Das Hausboot steht", want: source},
		{name: "inadequate fragment", target: "Haus", proposed: "Das Haus", want: source},
		{name: "still over limit", target: "Haus", proposed: "Das Haus steht am Rand des stillen Waldes," + strings.Repeat(" während die Kinder im Garten spielen", 9), want: source},
		{name: "empty", target: "Haus", proposed: "", want: source},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidatedContextSentence(source, test.target, test.proposed); got != test.want {
				t.Fatalf("context=%q want %q", got, test.want)
			}
		})
	}
	if got := ValidatedContextSentence("Das Haus steht am Rand.", "Haus", "Das Haus"); got != "Das Haus steht am Rand." {
		t.Fatalf("short source accepted provider shortening: %q", got)
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
