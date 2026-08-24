// Package enrichment enriches vocabulary candidates inline while keeping all
// external-provider policy and privacy decisions at the package boundary.
package enrichment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Identity struct{ Language, CanonicalLemma, UPOS string }
type Candidate struct {
	Identity
	Morphology      map[string]string
	ExampleSentence string
}
type Provenance struct {
	Provider, ProviderVersion string
	CachedAt                  time.Time
	External                  bool
}
type Field[T any] struct {
	Value      T
	Available  bool
	Provenance Provenance
}
type Result struct {
	Candidate                         Candidate
	Frequency                         Field[float64]
	Morphology                        Field[map[string]string]
	Pronunciation, Translation, Gloss Field[string]
	SentenceTranslation               Field[string]
	Warnings                          []string
}

type FrequencyProvider interface {
	Name() string
	Version() string
	Frequency(context.Context, Identity) (float64, bool, error)
}
type MorphologyProvider interface {
	Name() string
	Version() string
	Morphology(context.Context, Candidate) (map[string]string, bool, error)
}
type PronunciationProvider interface {
	Name() string
	Version() string
	Pronunciation(context.Context, Identity) (string, bool, error)
}

// TranslationRequest is deliberately the complete external-provider input.
// Adding user, document, corpus, or reading metadata to it is prohibited.
type TranslationRequest struct{ Language, CanonicalLemma, UPOS, ExampleSentence string }
type TranslationResponse struct {
	Translation         string `json:"translation"`
	Gloss               string `json:"gloss"`
	SentenceTranslation string `json:"sentence_translation"`
}
type TranslationProvider interface {
	Name() string
	Version() string
	Translate(context.Context, TranslationRequest) (TranslationResponse, error)
}

type CacheKey struct {
	Language, CanonicalLemma, UPOS, Provider, ProviderVersion string
	SentenceHash                                              string
}
type CacheEntry struct {
	CacheKey
	Translation, Gloss, SentenceTranslation string
	CachedAt                                time.Time
}
type ExternalCache interface {
	Get(context.Context, CacheKey) (CacheEntry, bool, error)
	Put(context.Context, CacheEntry) (CacheEntry, error)
}

type ContextMode string

const (
	SentenceContext ContextMode = "sentence"
	LemmaOnly       ContextMode = "lemma_only"
)

type EnrichmentConfig struct {
	ExternalEnabled, UserOptIn bool
	ContextMode                ContextMode
	MaxAttempts                int
	RetryBaseDelay             time.Duration
}

// Config is retained as the concise constructor-facing name.
type Config = EnrichmentConfig
type Service struct {
	config        Config
	frequency     FrequencyProvider
	morphology    MorphologyProvider
	pronunciation PronunciationProvider
	translation   TranslationProvider
	cache         ExternalCache
	now           func() time.Time
}

func NewService(config Config, frequency FrequencyProvider, morphology MorphologyProvider, pronunciation PronunciationProvider, translation TranslationProvider, cache ExternalCache) *Service {
	if config.MaxAttempts < 1 {
		config.MaxAttempts = 1
	}
	if config.ContextMode == "" {
		config.ContextMode = SentenceContext
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = 100 * time.Millisecond
	}
	return &Service{config: config, frequency: frequency, morphology: morphology, pronunciation: pronunciation, translation: translation, cache: cache, now: time.Now}
}

func (s *Service) Enrich(ctx context.Context, candidates []Candidate) []Result {
	out := make([]Result, len(candidates))
	for i, c := range candidates {
		out[i] = s.enrichOne(ctx, c)
	}
	return out
}

func (s *Service) enrichOne(ctx context.Context, c Candidate) Result {
	r := Result{Candidate: c}
	local := func(name, version string) Provenance { return Provenance{Provider: name, ProviderVersion: version} }
	if s.frequency != nil {
		v, ok, err := s.frequency.Frequency(ctx, c.Identity)
		if err != nil {
			r.Warnings = append(r.Warnings, "frequency: "+err.Error())
		} else if ok {
			r.Frequency = Field[float64]{v, true, local(s.frequency.Name(), s.frequency.Version())}
		}
	}
	if s.morphology != nil {
		v, ok, err := s.morphology.Morphology(ctx, c)
		if err != nil {
			r.Warnings = append(r.Warnings, "morphology: "+err.Error())
		} else if ok {
			r.Morphology = Field[map[string]string]{v, true, local(s.morphology.Name(), s.morphology.Version())}
		}
	}
	if s.pronunciation != nil {
		v, ok, err := s.pronunciation.Pronunciation(ctx, c.Identity)
		if err != nil {
			r.Warnings = append(r.Warnings, "pronunciation: "+err.Error())
		} else if ok {
			r.Pronunciation = Field[string]{v, true, local(s.pronunciation.Name(), s.pronunciation.Version())}
		}
	}
	external, err := s.enrichExternal(ctx, c, false)
	if err != nil {
		r.Warnings = append(r.Warnings, "translation: "+err.Error())
		return r
	}
	r.Translation, r.Gloss, r.SentenceTranslation = external.Translation, external.Gloss, external.SentenceTranslation
	r.Warnings = append(r.Warnings, external.Warnings...)
	return r
}

// ExternalConfigured reports whether admin configuration, user consent, and a
// provider are all present. Callers use it to avoid enqueueing no-op work.
func (s *Service) ExternalConfigured() bool {
	return s.config.ExternalEnabled && s.config.UserOptIn && s.translation != nil
}

// EnrichExternal performs only the cache-backed external translation portion
// of enrichment. It is shared by the inline compatibility path and River jobs.
func (s *Service) EnrichExternal(ctx context.Context, c Candidate) (Result, error) {
	return s.enrichExternal(ctx, c, true)
}

func (s *Service) enrichExternal(ctx context.Context, c Candidate, requireCache bool) (Result, error) {
	r := Result{Candidate: c}
	if !s.ExternalConfigured() {
		return r, nil
	}
	if requireCache && s.cache == nil {
		return r, errors.New("external enrichment cache is required")
	}
	sentence := ""
	if s.config.ContextMode == SentenceContext {
		sentence = c.ExampleSentence
		if SentenceHash(sentence) == "" {
			sentence = ""
		}
	}
	key := CacheKey{
		Language: c.Language, CanonicalLemma: c.CanonicalLemma, UPOS: strings.ToUpper(c.UPOS),
		Provider: s.translation.Name(), ProviderVersion: s.translation.Version(), SentenceHash: SentenceHash(sentence),
	}
	if s.cache != nil {
		entry, ok, err := s.cache.Get(ctx, key)
		if err != nil {
			if requireCache {
				return r, fmt.Errorf("translation cache get: %w", err)
			}
			r.Warnings = append(r.Warnings, "translation cache: "+err.Error())
		} else if ok {
			s.setExternal(&r, entry)
			return r, nil
		}
	}
	req := TranslationRequest{Language: c.Language, CanonicalLemma: c.CanonicalLemma, UPOS: strings.ToUpper(c.UPOS)}
	req.ExampleSentence = sentence
	var response TranslationResponse
	var err error
	for attempt := 0; attempt < s.config.MaxAttempts; attempt++ {
		response, err = s.translation.Translate(ctx, req)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		var retryable interface{ Temporary() bool }
		if errors.As(err, &retryable) && !retryable.Temporary() {
			break
		}
		if attempt+1 < s.config.MaxAttempts {
			delay := s.config.RetryBaseDelay << attempt
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return r, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if err != nil {
		return r, err
	}
	entry := CacheEntry{CacheKey: key, Translation: response.Translation, Gloss: response.Gloss, SentenceTranslation: response.SentenceTranslation, CachedAt: s.now().UTC()}
	if s.cache != nil {
		stored, putErr := s.cache.Put(ctx, entry)
		if putErr != nil {
			if requireCache {
				return r, fmt.Errorf("translation cache put: %w", putErr)
			}
			r.Warnings = append(r.Warnings, "translation cache: "+putErr.Error())
		} else {
			entry = stored
		}
	}
	s.setExternal(&r, entry)
	return r, nil
}

func (s *Service) setExternal(r *Result, e CacheEntry) {
	p := Provenance{e.Provider, e.ProviderVersion, e.CachedAt, true}
	if e.Translation != "" {
		r.Translation = Field[string]{e.Translation, true, p}
	}
	if e.Gloss != "" {
		r.Gloss = Field[string]{e.Gloss, true, p}
	}
	if e.SentenceTranslation != "" {
		r.SentenceTranslation = Field[string]{e.SentenceTranslation, true, p}
	}
}

// SentenceHash returns the lowercase hexadecimal SHA-256 digest of the UTF-8
// sentence after conservative normalization: CRLF line endings become LF and
// leading/trailing Unicode whitespace is removed. Interior text, whitespace,
// case, and punctuation are preserved. An empty normalized sentence has an
// empty identity so legacy lemma-only cache rows remain addressable.
func SentenceHash(sentence string) string {
	normalized := normalizeSentence(sentence)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func normalizeSentence(sentence string) string {
	return strings.TrimSpace(strings.ReplaceAll(sentence, "\r\n", "\n"))
}

var ErrInvalidProvider = errors.New("enrichment: provider name and version are required")

type TranslationClient interface {
	Translate(context.Context, TranslationRequest) (TranslationResponse, error)
}
type LLMProvider struct {
	name, version string
	client        TranslationClient
}

func NewLLMProvider(name, version string, client TranslationClient) (*LLMProvider, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" || client == nil {
		return nil, ErrInvalidProvider
	}
	return &LLMProvider{name, version, client}, nil
}
func (p *LLMProvider) Name() string    { return p.name }
func (p *LLMProvider) Version() string { return p.version }
func (p *LLMProvider) Translate(ctx context.Context, r TranslationRequest) (TranslationResponse, error) {
	return p.client.Translate(ctx, r)
}

type DictionaryLookup func(context.Context, TranslationRequest) (TranslationResponse, error)
type DictionaryProvider struct {
	name, version string
	lookup        DictionaryLookup
}

func NewDictionaryProvider(name, version string, lookup DictionaryLookup) (*DictionaryProvider, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" || lookup == nil {
		return nil, ErrInvalidProvider
	}
	return &DictionaryProvider{name, version, lookup}, nil
}
func (p *DictionaryProvider) Name() string    { return p.name }
func (p *DictionaryProvider) Version() string { return p.version }
func (p *DictionaryProvider) Translate(ctx context.Context, r TranslationRequest) (TranslationResponse, error) {
	v, err := p.lookup(ctx, r)
	if err != nil {
		return TranslationResponse{}, fmt.Errorf("dictionary %s: %w", p.name, err)
	}
	return v, nil
}
