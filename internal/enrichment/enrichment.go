// Package enrichment enriches vocabulary candidates inline while keeping all
// external-provider policy and privacy decisions at the package boundary.
package enrichment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/justin-hayes/mouseion/internal/textmatch"
)

type Identity struct{ Language, CanonicalLemma, UPOS string }
type Candidate struct {
	Identity
	Morphology                map[string]string
	TargetWord                string
	ExampleSentence           string
	DictionaryProviderVersion string
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
	Candidate                                      Candidate
	Frequency                                      Field[float64]
	Morphology                                     Field[map[string]string]
	Pronunciation, Translation, FallbackGloss      Field[string]
	SentenceTranslation, SentenceTranslationTarget Field[string]
	SenseSelection                                 Field[[]int]
	Warnings                                       []string
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
type TranslationRequest struct{ Language, TargetLanguage, CanonicalLemma, UPOS, TargetWord, ExampleSentence string }
type TranslationResponse struct {
	Translation               string   `json:"translation"`
	SentenceTranslation       string   `json:"sentence_translation"`
	SentenceTranslationTarget string   `json:"sentence_translation_target"`
	SenseOrder                []int    `json:"sense_order"`
	FallbackGloss             string   `json:"fallback_gloss"`
	Warnings                  []string `json:"-"`
}

// TranslationUsage is the bounded usage portion of a provider response. It
// is exposed for the operator validation harness; it is not part of cache
// identity or the persisted prepared-deck result.
type TranslationUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}
type TranslationProvider interface {
	Name() string
	Version() string
	Translate(context.Context, TranslationRequest) (TranslationResponse, error)
}

type CacheKey struct {
	Language, TargetLanguage, CanonicalLemma, UPOS, Provider, ProviderVersion string
	DictionaryProviderVersion                                                 string
	SentenceHash                                                              string
}
type CacheEntry struct {
	CacheKey
	Translation, FallbackGloss, SentenceTranslation, SentenceTranslationTarget string
	SenseSelection                                                             []int
	CachedAt                                                                   time.Time
}

// HasRequiredTranslationFields reports whether a cached result satisfies the
// frozen prepared-deck translation contract. The target phrase is optional:
// the codec permits it to be empty when there is no reliable literal match.
func HasRequiredTranslationFields(entry CacheEntry, sourceSentence string) bool {
	if strings.TrimSpace(entry.Translation) == "" {
		return false
	}
	return strings.TrimSpace(sourceSentence) == "" || strings.TrimSpace(entry.SentenceTranslation) != ""
}

type ExternalCache interface {
	Get(context.Context, CacheKey) (CacheEntry, bool, error)
	Put(context.Context, CacheEntry) (CacheEntry, error)
}

// ExternalMetrics contains only aggregate, content-free measurements for one
// external enrichment operation. It is safe to roll up into job telemetry;
// candidate identity, requests, responses, and provider error messages are
// deliberately excluded.
type ExternalMetrics struct {
	CacheHits, CacheMisses                  int
	ProviderCalls, Attempts, Retries        int
	Cancellations                           int
	CacheLatency, ProviderLatency           time.Duration
	RateLimitErrors, Provider5xxErrors      int
	TimeoutErrors, CacheErrors, OtherErrors int
}

func (m *ExternalMetrics) addError(class ExternalErrorClass) {
	switch class {
	case ExternalErrorRateLimit:
		m.RateLimitErrors++
	case ExternalErrorProvider5xx:
		m.Provider5xxErrors++
	case ExternalErrorTimeout:
		m.TimeoutErrors++
	case ExternalErrorCancellation:
		m.Cancellations++
	case ExternalErrorCache:
		m.CacheErrors++
	default:
		m.OtherErrors++
	}
}

// ExternalErrorClass is a bounded, privacy-safe provider/cache outcome label.
// It intentionally never includes an error string or provider response body.
type ExternalErrorClass string

const (
	ExternalErrorRateLimit    ExternalErrorClass = "rate_limit"
	ExternalErrorProvider5xx  ExternalErrorClass = "provider_5xx"
	ExternalErrorTimeout      ExternalErrorClass = "timeout"
	ExternalErrorCancellation ExternalErrorClass = "cancellation"
	ExternalErrorCache        ExternalErrorClass = "cache"
	ExternalErrorOther        ExternalErrorClass = "other"
)

// ClassifyExternalError maps provider failures to a bounded label without
// retaining private provider messages.
func ClassifyExternalError(err error) ExternalErrorClass {
	if errors.Is(err, context.Canceled) {
		return ExternalErrorCancellation
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ExternalErrorTimeout
	}
	var httpErr *LLMHTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == http.StatusTooManyRequests {
			return ExternalErrorRateLimit
		}
		if httpErr.StatusCode == http.StatusRequestTimeout {
			return ExternalErrorTimeout
		}
		if httpErr.StatusCode >= 500 {
			return ExternalErrorProvider5xx
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ExternalErrorTimeout
	}
	return ExternalErrorOther
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
	r.Translation, r.FallbackGloss, r.SentenceTranslation = external.Translation, external.FallbackGloss, external.SentenceTranslation
	r.SentenceTranslationTarget = external.SentenceTranslationTarget
	r.SenseSelection = external.SenseSelection
	r.Warnings = append(r.Warnings, external.Warnings...)
	return r
}

// ExternalConfigured reports whether admin configuration, user consent, and a
// provider are all present. Callers use it to avoid enqueueing no-op work.
func (s *Service) ExternalConfigured() bool {
	return s.config.ExternalEnabled && s.config.UserOptIn && s.translation != nil
}

// ExternalCacheKey returns the complete immutable cache identity that
// EnrichExternal will use for this candidate.
func (s *Service) ExternalCacheKey(c Candidate) (CacheKey, bool) {
	if !s.ExternalConfigured() {
		return CacheKey{}, false
	}
	sentence := ""
	if s.config.ContextMode == SentenceContext {
		sentence = c.ExampleSentence
		if SentenceHash(sentence) == "" {
			sentence = ""
		}
	}
	return CacheKey{
		Language: c.Language, TargetLanguage: "en", CanonicalLemma: c.CanonicalLemma, UPOS: strings.ToUpper(c.UPOS),
		Provider: s.translation.Name(), ProviderVersion: s.translation.Version(), DictionaryProviderVersion: c.DictionaryProviderVersion, SentenceHash: SentenceHash(sentence),
	}, true
}

// EnrichExternal performs only the cache-backed external translation portion
// of enrichment. It is shared by the inline compatibility path and River jobs.
func (s *Service) EnrichExternal(ctx context.Context, c Candidate) (Result, error) {
	r, _, err := s.EnrichExternalObserved(ctx, c)
	return r, err
}

// EnrichExternalObserved is EnrichExternal with aggregate timing and outcome
// data for prepared-deck observability and deterministic benchmarks.
func (s *Service) EnrichExternalObserved(ctx context.Context, c Candidate) (Result, ExternalMetrics, error) {
	return s.enrichExternalObserved(ctx, c, true)
}

func (s *Service) enrichExternal(ctx context.Context, c Candidate, requireCache bool) (Result, error) {
	r, _, err := s.enrichExternalObserved(ctx, c, requireCache)
	return r, err
}

func (s *Service) enrichExternalObserved(ctx context.Context, c Candidate, requireCache bool) (Result, ExternalMetrics, error) {
	r := Result{Candidate: c}
	var metrics ExternalMetrics
	if !s.ExternalConfigured() {
		return r, metrics, nil
	}
	if requireCache && s.cache == nil {
		metrics.CacheErrors++
		return r, metrics, errors.New("external enrichment cache is required")
	}
	key, _ := s.ExternalCacheKey(c)
	sentence := ""
	if key.SentenceHash != "" {
		sentence = c.ExampleSentence
	}
	if s.cache != nil {
		started := time.Now()
		entry, ok, err := s.cache.Get(ctx, key)
		metrics.CacheLatency += time.Since(started)
		if err != nil {
			metrics.CacheErrors++
			if requireCache {
				return r, metrics, fmt.Errorf("translation cache get: %w", err)
			}
			r.Warnings = append(r.Warnings, "translation cache: "+err.Error())
		} else if ok {
			metrics.CacheHits++
			s.setExternal(&r, entry)
			return r, metrics, nil
		} else {
			metrics.CacheMisses++
		}
	}
	target := textmatch.CleanLexicalSurface(c.TargetWord)
	if target == "" {
		target = textmatch.CleanLexicalSurface(c.CanonicalLemma)
	}
	req := TranslationRequest{Language: c.Language, TargetLanguage: "en", CanonicalLemma: c.CanonicalLemma, UPOS: strings.ToUpper(c.UPOS)}
	if sentence != "" {
		req.TargetWord = target
	}
	req.ExampleSentence = sentence
	metrics.ProviderCalls++
	response, err := backoff.RetryWithData(func() (TranslationResponse, error) {
		metrics.Attempts++
		if metrics.Attempts > 1 {
			metrics.Retries++
		}
		started := time.Now()
		response, err := s.translation.Translate(ctx, req)
		metrics.ProviderLatency += time.Since(started)
		if err == nil {
			return response, nil
		}
		if ctx.Err() != nil {
			return response, ctx.Err()
		}
		var retryable interface{ Temporary() bool }
		if errors.As(err, &retryable) && !retryable.Temporary() {
			return response, backoff.Permanent(err)
		}
		return response, err
	}, backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(s.config.RetryBaseDelay),
		backoff.WithMultiplier(2),
		backoff.WithRandomizationFactor(0),
		backoff.WithMaxInterval(time.Duration(1<<63-1)),
		backoff.WithMaxElapsedTime(0),
	), uint64(s.config.MaxAttempts-1)), ctx))
	if err != nil {
		metrics.addError(ClassifyExternalError(err))
		return r, metrics, err
	}
	response, err = NormalizeTranslationResponse(req, response)
	if err != nil {
		metrics.addError(ExternalErrorOther)
		return r, metrics, err
	}
	r.Warnings = append(r.Warnings, response.Warnings...)
	entry := CacheEntry{
		CacheKey:                  key,
		Translation:               response.Translation,
		FallbackGloss:             response.FallbackGloss,
		SentenceTranslation:       response.SentenceTranslation,
		SentenceTranslationTarget: response.SentenceTranslationTarget,
		SenseSelection:            append([]int(nil), response.SenseOrder...),
		CachedAt:                  s.now().UTC(),
	}
	if s.cache != nil {
		started := time.Now()
		stored, putErr := s.cache.Put(ctx, entry)
		metrics.CacheLatency += time.Since(started)
		if putErr != nil {
			metrics.CacheErrors++
			if requireCache {
				return r, metrics, fmt.Errorf("translation cache put: %w", putErr)
			}
			r.Warnings = append(r.Warnings, "translation cache: "+putErr.Error())
		} else {
			entry = stored
		}
	}
	s.setExternal(&r, entry)
	return r, metrics, nil
}

func (s *Service) setExternal(r *Result, e CacheEntry) {
	p := Provenance{e.Provider, e.ProviderVersion, e.CachedAt, true}
	if e.Translation != "" {
		r.Translation = Field[string]{e.Translation, true, p}
	}
	if e.FallbackGloss != "" {
		r.FallbackGloss = Field[string]{e.FallbackGloss, true, p}
	}
	if e.SentenceTranslation != "" {
		r.SentenceTranslation = Field[string]{e.SentenceTranslation, true, p}
	}
	if e.SentenceTranslation != "" && e.SentenceTranslationTarget != "" {
		r.SentenceTranslationTarget = Field[string]{e.SentenceTranslationTarget, true, p}
	}
	if len(e.SenseSelection) > 0 {
		r.SenseSelection = Field[[]int]{append([]int(nil), e.SenseSelection...), true, p}
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
