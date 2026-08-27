package prepareddeck

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

const benchmarkCacheLatency = 2 * time.Millisecond

type benchmarkFailure string

const (
	benchmarkSuccess   benchmarkFailure = "success"
	benchmarkRateLimit benchmarkFailure = "429"
	benchmarkServer5xx benchmarkFailure = "5xx"
	benchmarkTimeout   benchmarkFailure = "timeout"
)

type translationBenchmarkCase struct {
	Candidates, HitPercent, Concurrency int
	ProviderLatency                     time.Duration
	Failure                             benchmarkFailure
}

func (c translationBenchmarkCase) name() string {
	return fmt.Sprintf("candidates_%d/hits_%dpct/latency_%s/concurrency_%d/outcome_%s", c.Candidates, c.HitPercent, c.ProviderLatency, c.Concurrency, c.Failure)
}

func translationBenchmarkCases() []translationBenchmarkCase {
	cases := make([]translationBenchmarkCase, 0, 3*3*3*4*4)
	for _, candidates := range []int{10, 50, 200} {
		for _, hitPercent := range []int{0, 50, 100} {
			for _, latency := range []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, time.Second} {
				for _, concurrency := range []int{1, 2, 4, 8} {
					for _, failure := range []benchmarkFailure{benchmarkSuccess, benchmarkRateLimit, benchmarkServer5xx, benchmarkTimeout} {
						cases = append(cases, translationBenchmarkCase{candidates, hitPercent, concurrency, latency, failure})
					}
				}
			}
		}
	}
	return cases
}

type benchmarkCache struct {
	mu     sync.RWMutex
	values map[enrichment.CacheKey]enrichment.CacheEntry
}

func (c *benchmarkCache) Get(_ context.Context, key enrichment.CacheKey) (enrichment.CacheEntry, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.values[key]
	return entry, ok, nil
}

func (c *benchmarkCache) Put(_ context.Context, entry enrichment.CacheEntry) (enrichment.CacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.values[entry.CacheKey]; ok {
		return existing, nil
	}
	c.values[entry.CacheKey] = entry
	return entry, nil
}

// benchmarkProvider uses a release gate to make measured peak in-flight calls
// deterministic. ProviderLatency is advanced in the modeled timeline below;
// the fake never sleeps, so all 432 cases remain practical with -benchtime=1x.
type benchmarkProvider struct {
	failure      benchmarkFailure
	expectedPeak int64
	inFlight     atomic.Int64
	peak         atomic.Int64
	release      chan struct{}
	releaseOnce  sync.Once
}

func (*benchmarkProvider) Name() string    { return "deterministic-fake" }
func (*benchmarkProvider) Version() string { return "1" }

func (p *benchmarkProvider) Translate(context.Context, enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	inFlight := p.inFlight.Add(1)
	for peak := p.peak.Load(); inFlight > peak && !p.peak.CompareAndSwap(peak, inFlight); peak = p.peak.Load() {
	}
	if inFlight == p.expectedPeak {
		p.releaseOnce.Do(func() { close(p.release) })
	}
	if p.expectedPeak > 0 {
		<-p.release
	}
	defer p.inFlight.Add(-1)
	switch p.failure {
	case benchmarkRateLimit:
		return enrichment.TranslationResponse{}, &enrichment.LLMHTTPError{StatusCode: http.StatusTooManyRequests, Message: "private fake response"}
	case benchmarkServer5xx:
		return enrichment.TranslationResponse{}, &enrichment.LLMHTTPError{StatusCode: http.StatusServiceUnavailable, Message: "private fake response"}
	case benchmarkTimeout:
		return enrichment.TranslationResponse{}, context.DeadlineExceeded
	default:
		return enrichment.TranslationResponse{Translation: "translation", SentenceTranslation: "translated sentence", SentenceTranslationTarget: "target"}, nil
	}
}

type benchmarkResult struct {
	metrics                                     enrichment.ExternalMetrics
	translated, untranslated                    int
	configuredConcurrency, effectiveConcurrency int
	peakInFlight                                int
	modeledProviderLatency, modeledCacheLatency time.Duration
	modeledWallLatency                          time.Duration
	artifact                                    [sha256.Size]byte
}

func runTranslationBenchmark(c translationBenchmarkCase) benchmarkResult {
	candidates := make([]enrichment.Candidate, c.Candidates)
	cache := &benchmarkCache{values: make(map[enrichment.CacheKey]enrichment.CacheEntry, c.Candidates)}
	hitCount := c.Candidates * c.HitPercent / 100
	for i := range candidates {
		candidates[i] = enrichment.Candidate{
			Identity:        enrichment.Identity{Language: "de", CanonicalLemma: fmt.Sprintf("candidate-%03d", i), UPOS: "NOUN"},
			TargetWord:      fmt.Sprintf("target-%03d", i),
			ExampleSentence: fmt.Sprintf("Synthetic sentence %03d.", i),
		}
		if i < hitCount {
			key := benchmarkCacheKey(candidates[i])
			cache.values[key] = enrichment.CacheEntry{CacheKey: key, Translation: "cached", SentenceTranslation: "cached sentence", SentenceTranslationTarget: "cached target", CachedAt: time.Unix(1, 0).UTC()}
		}
	}
	missCount := c.Candidates - hitCount
	effective := min(c.Concurrency, c.Candidates)
	expectedPeak := min(effective, missCount)
	provider := &benchmarkProvider{failure: c.Failure, expectedPeak: int64(expectedPeak), release: make(chan struct{})}
	if expectedPeak == 0 {
		close(provider.release)
	}
	service := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, ContextMode: enrichment.SentenceContext, MaxAttempts: 2, RetryBaseDelay: time.Nanosecond}, nil, nil, nil, provider, cache)

	type itemResult struct {
		available bool
		metrics   enrichment.ExternalMetrics
	}
	items := make([]itemResult, c.Candidates)
	jobs := make(chan int, c.Candidates)
	for i := range candidates {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	workers.Add(effective)
	for range effective {
		go func() {
			defer workers.Done()
			for i := range jobs {
				result, metrics, _ := service.EnrichExternalObserved(context.Background(), candidates[i])
				items[i] = itemResult{available: result.SentenceTranslation.Available, metrics: metrics}
			}
		}()
	}
	workers.Wait()

	result := benchmarkResult{configuredConcurrency: c.Concurrency, effectiveConcurrency: effective, peakInFlight: int(provider.peak.Load())}
	artifactState := make([]byte, len(items))
	for i, item := range items {
		addExternalMetrics(&result.metrics, item.metrics)
		if item.available {
			result.translated++
			artifactState[i] = 1
		} else {
			result.untranslated++
		}
	}
	result.modeledProviderLatency = time.Duration(result.metrics.Attempts) * c.ProviderLatency
	cacheOperations := c.Candidates
	if c.Failure == benchmarkSuccess {
		cacheOperations += missCount
	}
	result.modeledCacheLatency = time.Duration(cacheOperations) * benchmarkCacheLatency
	result.modeledWallLatency = modeledTranslationWall(c, hitCount)
	result.artifact = sha256.Sum256(artifactState)
	return result
}

func benchmarkCacheKey(candidate enrichment.Candidate) enrichment.CacheKey {
	return enrichment.CacheKey{Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "deterministic-fake", ProviderVersion: "1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
}

func addExternalMetrics(total *enrichment.ExternalMetrics, item enrichment.ExternalMetrics) {
	total.CacheHits += item.CacheHits
	total.CacheMisses += item.CacheMisses
	total.ProviderCalls += item.ProviderCalls
	total.Attempts += item.Attempts
	total.Retries += item.Retries
	total.Cancellations += item.Cancellations
	total.CacheLatency += item.CacheLatency
	total.ProviderLatency += item.ProviderLatency
	total.RateLimitErrors += item.RateLimitErrors
	total.Provider5xxErrors += item.Provider5xxErrors
	total.TimeoutErrors += item.TimeoutErrors
	total.CacheErrors += item.CacheErrors
	total.OtherErrors += item.OtherErrors
}

// modeledTranslationWall is a deterministic work-conserving scheduler. It
// includes one cache lookup per candidate, a cache write for successful misses,
// and configured fake-provider latency for every attempt.
func modeledTranslationWall(c translationBenchmarkCase, hitCount int) time.Duration {
	workers := make([]time.Duration, min(c.Concurrency, c.Candidates))
	for i := 0; i < c.Candidates; i++ {
		worker := 0
		for j := 1; j < len(workers); j++ {
			if workers[j] < workers[worker] {
				worker = j
			}
		}
		duration := benchmarkCacheLatency
		if i >= hitCount {
			attempts := 1
			if c.Failure != benchmarkSuccess {
				attempts = 2
			}
			duration += time.Duration(attempts) * c.ProviderLatency
			if c.Failure == benchmarkSuccess {
				duration += benchmarkCacheLatency
			}
		}
		workers[worker] += duration
	}
	var wall time.Duration
	for _, duration := range workers {
		wall = max(wall, duration)
	}
	return wall
}

func TestTranslationBenchmarkMatrixAndDeterministicFailureMetrics(t *testing.T) {
	cases := translationBenchmarkCases()
	if len(cases) != 432 {
		t.Fatalf("matrix has %d cases, want 432", len(cases))
	}
	seenCandidates, seenHits, seenLatencies, seenConcurrency, seenFailures := map[int]bool{}, map[int]bool{}, map[time.Duration]bool{}, map[int]bool{}, map[benchmarkFailure]bool{}
	for _, c := range cases {
		seenCandidates[c.Candidates] = true
		seenHits[c.HitPercent] = true
		seenLatencies[c.ProviderLatency] = true
		seenConcurrency[c.Concurrency] = true
		seenFailures[c.Failure] = true
	}
	if len(seenCandidates) != 3 || len(seenHits) != 3 || len(seenLatencies) != 3 || len(seenConcurrency) != 4 || len(seenFailures) != 4 {
		t.Fatalf("incomplete matrix: candidates=%v hits=%v latencies=%v concurrency=%v failures=%v", seenCandidates, seenHits, seenLatencies, seenConcurrency, seenFailures)
	}

	c := translationBenchmarkCase{Candidates: 10, HitPercent: 50, ProviderLatency: 500 * time.Millisecond, Concurrency: 4, Failure: benchmarkRateLimit}
	first, second := runTranslationBenchmark(c), runTranslationBenchmark(c)
	if first.artifact != second.artifact || first.translated != 5 || first.untranslated != 5 || first.metrics.CacheHits != 5 || first.metrics.CacheMisses != 5 || first.metrics.ProviderCalls != 5 || first.metrics.Attempts != 10 || first.metrics.Retries != 5 || first.metrics.RateLimitErrors != 5 || first.peakInFlight != 4 || first.modeledWallLatency <= 0 {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func BenchmarkPreparedDeckTranslation(b *testing.B) {
	for _, c := range translationBenchmarkCases() {
		b.Run(c.name(), func(b *testing.B) {
			baseline := runTranslationBenchmark(c)
			deterministic := true
			var result benchmarkResult
			b.ResetTimer()
			for range b.N {
				result = runTranslationBenchmark(c)
				deterministic = deterministic && result.artifact == baseline.artifact && result.translated == baseline.translated && result.untranslated == baseline.untranslated
			}
			b.StopTimer()
			b.ReportMetric(float64(result.metrics.CacheHits), "cache_hits/op")
			b.ReportMetric(float64(result.metrics.CacheMisses), "cache_misses/op")
			b.ReportMetric(float64(result.metrics.ProviderCalls), "provider_calls/op")
			b.ReportMetric(float64(result.metrics.Attempts), "attempts/op")
			b.ReportMetric(float64(result.metrics.Retries), "retries/op")
			b.ReportMetric(float64(result.translated), "translated/op")
			b.ReportMetric(float64(result.untranslated), "untranslated/op")
			b.ReportMetric(100*float64(result.metrics.CacheHits)/float64(c.Candidates), "cache_hit_pct")
			b.ReportMetric(100*float64(result.untranslated)/float64(c.Candidates), "failure_pct")
			b.ReportMetric(float64(result.modeledProviderLatency)/float64(time.Millisecond), "provider_ms/op")
			b.ReportMetric(float64(result.modeledCacheLatency)/float64(time.Millisecond), "cache_ms/op")
			b.ReportMetric(float64(result.modeledWallLatency)/float64(time.Millisecond), "modeled_wall_ms/op")
			b.ReportMetric(float64(result.configuredConcurrency), "configured_concurrency")
			b.ReportMetric(float64(result.effectiveConcurrency), "effective_concurrency")
			b.ReportMetric(float64(result.peakInFlight), "peak_inflight")
			b.ReportMetric(100*float64(result.translated)/float64(c.Candidates), "completeness_pct")
			b.ReportMetric(float64(result.metrics.RateLimitErrors), "rate_limit_errors/op")
			b.ReportMetric(float64(result.metrics.Provider5xxErrors), "provider_5xx_errors/op")
			b.ReportMetric(float64(result.metrics.TimeoutErrors), "timeout_errors/op")
			if deterministic {
				b.ReportMetric(1, "artifact_deterministic")
			} else {
				b.ReportMetric(0, "artifact_deterministic")
			}
		})
	}
}
