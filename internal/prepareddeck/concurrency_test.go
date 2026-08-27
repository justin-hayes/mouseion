package prepareddeck

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type translationState struct {
	mu     sync.RWMutex
	values map[string]string
}

func newTranslationState() *translationState {
	return &translationState{values: make(map[string]string)}
}

func (s *translationState) put(candidate enrichment.Candidate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[candidate.CanonicalLemma] = "translated-" + candidate.CanonicalLemma
}

type orderedStateBuilder struct {
	candidates   []enrichment.Candidate
	state        *translationState
	prepareCalls int
	renderCalls  int
}

func (b *orderedStateBuilder) PrepareCoverage(context.Context, string, string) (cardexport.Manifest, error) {
	b.prepareCalls++
	return testManifest(b.candidates, 0), nil
}

func (b *orderedStateBuilder) RenderManifest(_ context.Context, _ cardexport.Manifest, outcomes []cardexport.ExactEnrichment) (cardexport.Artifact, error) {
	b.renderCalls++
	completeness := cardexport.Completeness{TotalCards: len(b.candidates)}
	var rows strings.Builder
	for i, candidate := range b.candidates {
		translation := ""
		if i < len(outcomes) && outcomes[i].Result.SentenceTranslation.Available {
			translation = outcomes[i].Result.SentenceTranslation.Value
		}
		fmt.Fprintf(&rows, "%s\t%s\n", candidate.CanonicalLemma, translation)
		if translation != "" {
			completeness.CardsWithEnglish++
			completeness.CardsWithEnglishSentence++
		}
	}
	artifact := rows.String()
	return cardexport.Artifact{APKG: []byte(artifact), TSV: artifact, Count: len(b.candidates), Completeness: completeness}, nil
}

type timedObservedEnricher struct {
	state    *translationState
	delay    time.Duration
	failures map[string]bool
	calls    atomic.Int64
	inFlight atomic.Int64
	peak     atomic.Int64
}

func (*timedObservedEnricher) ExternalConfigured() bool { return true }
func (*timedObservedEnricher) ExternalCacheKey(candidate enrichment.Candidate) (enrichment.CacheKey, bool) {
	return testCacheKey(candidate), true
}

func (e *timedObservedEnricher) EnrichExternal(ctx context.Context, candidate enrichment.Candidate) (enrichment.Result, error) {
	result, _, err := e.EnrichExternalObserved(ctx, candidate)
	return result, err
}

func (e *timedObservedEnricher) EnrichExternalObserved(ctx context.Context, candidate enrichment.Candidate) (enrichment.Result, enrichment.ExternalMetrics, error) {
	e.calls.Add(1)
	active := e.inFlight.Add(1)
	defer e.inFlight.Add(-1)
	for peak := e.peak.Load(); active > peak && !e.peak.CompareAndSwap(peak, active); peak = e.peak.Load() {
	}
	started := time.Now()
	timer := time.NewTimer(e.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return enrichment.Result{}, enrichment.ExternalMetrics{CacheMisses: 1, ProviderCalls: 1, Attempts: 1, Cancellations: 1, ProviderLatency: time.Since(started)}, ctx.Err()
	case <-timer.C:
	}
	metrics := enrichment.ExternalMetrics{CacheMisses: 1, ProviderCalls: 1, Attempts: 1, ProviderLatency: time.Since(started)}
	if e.failures[candidate.CanonicalLemma] {
		metrics.Provider5xxErrors = 1
		return enrichment.Result{}, metrics, errors.New("synthetic provider failure")
	}
	e.state.put(candidate)
	return enrichment.Result{Candidate: candidate, SentenceTranslation: enrichment.Field[string]{Value: "translated-" + candidate.CanonicalLemma, Available: true, Provenance: enrichment.Provenance{Provider: "test", ProviderVersion: "1"}}}, metrics, nil
}

type workerScenario struct {
	artifact    cardexport.Artifact
	observation Observation
	elapsed     time.Duration
	calls       int
	peak        int
	builds      int
	failed      string
}

func runWorkerScenario(t *testing.T, candidates []enrichment.Candidate, concurrency int, delay time.Duration, failures map[string]bool) workerScenario {
	t.Helper()
	state := newTranslationState()
	builder := &orderedStateBuilder{candidates: candidates, state: state}
	enricher := &timedObservedEnricher{state: state, delay: delay, failures: failures}
	store := &fakeStore{
		preparation: domain.DeckPreparation{ID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash"},
		source:      domain.SourceMaterial{ID: "book", OwnerID: "alice", ContentHash: "hash"},
	}
	var observation Observation
	worker := &Worker{
		Store:                  store,
		Builder:                builder,
		Enrichment:             enricher,
		TranslationConcurrency: concurrency,
		Observer:               ObserverFunc(func(got Observation) { observation = got }),
	}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1}, Args: JobArgs{PreparationID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash", ExternalTranslationConsent: true}}
	started := time.Now()
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if store.completed == nil {
		t.Fatalf("preparation was not completed; failed=%q", store.failed)
	}
	return workerScenario{
		artifact:    *store.completed,
		observation: observation,
		elapsed:     time.Since(started),
		calls:       int(enricher.calls.Load()),
		peak:        int(enricher.peak.Load()),
		builds:      builder.renderCalls,
		failed:      store.failed,
	}
}

func testCandidates(count int) []enrichment.Candidate {
	candidates := make([]enrichment.Candidate, count)
	for i := range candidates {
		candidates[i] = enrichment.Candidate{
			Identity:        enrichment.Identity{Language: "de", CanonicalLemma: fmt.Sprintf("candidate-%02d", i), UPOS: "NOUN"},
			TargetWord:      fmt.Sprintf("target-%02d", i),
			ExampleSentence: fmt.Sprintf("Sentence %02d.", i),
		}
	}
	return candidates
}

func TestWorkerBoundsConcurrencyCompletesAllCandidatesAndMatchesSerialOutput(t *testing.T) {
	candidates := testCandidates(12)
	serial := runWorkerScenario(t, candidates, 1, 15*time.Millisecond, nil)
	concurrent := runWorkerScenario(t, candidates, 4, 15*time.Millisecond, nil)

	if serial.calls != len(candidates) || concurrent.calls != len(candidates) || serial.builds != 1 || concurrent.builds != 1 {
		t.Fatalf("serial calls/builds=%d/%d concurrent=%d/%d", serial.calls, serial.builds, concurrent.calls, concurrent.builds)
	}
	if serial.peak != 1 || concurrent.peak != 4 || concurrent.observation.PeakInFlightProviderCalls != 4 {
		t.Fatalf("serial peak=%d concurrent peak=%d observation=%+v", serial.peak, concurrent.peak, concurrent.observation)
	}
	if concurrent.observation.ConfiguredConcurrency != 4 || concurrent.observation.EffectiveConcurrency != 4 || concurrent.observation.Counts.Translated != len(candidates) || concurrent.observation.Counts.Untranslated != 0 {
		t.Fatalf("observation=%+v", concurrent.observation)
	}
	if string(serial.artifact.APKG) != string(concurrent.artifact.APKG) || serial.artifact.TSV != concurrent.artifact.TSV || serial.artifact.Completeness != concurrent.artifact.Completeness {
		t.Fatalf("serial artifact=%+v concurrent=%+v", serial.artifact, concurrent.artifact)
	}
	if concurrent.elapsed >= serial.elapsed*3/4 {
		t.Fatalf("bounded concurrency did not reduce elapsed time: serial=%s concurrent=%s", serial.elapsed, concurrent.elapsed)
	}
}

func TestWorkerConcurrentFailuresRemainNonFatalAndStable(t *testing.T) {
	candidates := testCandidates(8)
	failures := map[string]bool{"candidate-01": true, "candidate-06": true}
	serial := runWorkerScenario(t, candidates, 1, time.Millisecond, failures)
	concurrent := runWorkerScenario(t, candidates, 4, time.Millisecond, failures)

	if serial.failed != "" || concurrent.failed != "" || serial.calls != len(candidates) || concurrent.calls != len(candidates) {
		t.Fatalf("serial failed/calls=%q/%d concurrent=%q/%d", serial.failed, serial.calls, concurrent.failed, concurrent.calls)
	}
	if string(serial.artifact.APKG) != string(concurrent.artifact.APKG) || serial.artifact.Completeness != concurrent.artifact.Completeness {
		t.Fatalf("serial artifact=%+v concurrent=%+v", serial.artifact, concurrent.artifact)
	}
	if concurrent.artifact.Completeness.CardsWithEnglishSentence != len(candidates)-len(failures) || concurrent.observation.Counts.Translated != len(candidates)-len(failures) || concurrent.observation.Counts.Untranslated != len(failures) || concurrent.observation.Errors.Provider5xx != len(failures) {
		t.Fatalf("artifact=%+v observation=%+v", concurrent.artifact, concurrent.observation)
	}
	if serial.observation.Counts != concurrent.observation.Counts || serial.observation.Errors != concurrent.observation.Errors || serial.observation.Completeness != concurrent.observation.Completeness {
		t.Fatalf("serial observation=%+v concurrent=%+v", serial.observation, concurrent.observation)
	}
}

type cancellationEnricher struct {
	started  chan struct{}
	calls    atomic.Int64
	inFlight atomic.Int64
}

func (*cancellationEnricher) ExternalConfigured() bool { return true }
func (*cancellationEnricher) ExternalCacheKey(candidate enrichment.Candidate) (enrichment.CacheKey, bool) {
	return testCacheKey(candidate), true
}

func (e *cancellationEnricher) EnrichExternal(ctx context.Context, candidate enrichment.Candidate) (enrichment.Result, error) {
	result, _, err := e.EnrichExternalObserved(ctx, candidate)
	return result, err
}

func (e *cancellationEnricher) EnrichExternalObserved(ctx context.Context, _ enrichment.Candidate) (enrichment.Result, enrichment.ExternalMetrics, error) {
	e.calls.Add(1)
	e.inFlight.Add(1)
	defer e.inFlight.Add(-1)
	e.started <- struct{}{}
	<-ctx.Done()
	return enrichment.Result{}, enrichment.ExternalMetrics{CacheMisses: 1, ProviderCalls: 1, Attempts: 1, Cancellations: 1}, ctx.Err()
}

func TestWorkerCancellationStopsSchedulingAndWaitsForInFlightWork(t *testing.T) {
	const concurrency = 3
	state := newTranslationState()
	builder := &orderedStateBuilder{candidates: testCandidates(20), state: state}
	enricher := &cancellationEnricher{started: make(chan struct{}, concurrency)}
	store := &fakeStore{
		preparation: domain.DeckPreparation{ID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash"},
		source:      domain.SourceMaterial{ID: "book", OwnerID: "alice", ContentHash: "hash"},
	}
	var observation Observation
	worker := &Worker{Store: store, Builder: builder, Enrichment: enricher, TranslationConcurrency: concurrency, Observer: ObserverFunc(func(got Observation) { observation = got })}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1}, Args: JobArgs{PreparationID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash", ExternalTranslationConsent: true}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Work(ctx, job) }()
	for range concurrency {
		select {
		case <-enricher.started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for in-flight enrichment")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not return after cancellation")
	}

	if got := int(enricher.calls.Load()); got != concurrency {
		t.Fatalf("enrichment calls=%d, want %d", got, concurrency)
	}
	if got := enricher.inFlight.Load(); got != 0 {
		t.Fatalf("in-flight work after worker return=%d", got)
	}
	if builder.prepareCalls != 1 || builder.renderCalls != 0 || store.completed != nil || store.failed != context.Canceled.Error() {
		t.Fatalf("prepare=%d render=%d completed=%+v failed=%q", builder.prepareCalls, builder.renderCalls, store.completed, store.failed)
	}
	if observation.Outcome != "failed" || observation.Counts.Cancellations != concurrency {
		t.Fatalf("observation=%+v", observation)
	}
}
