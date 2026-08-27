package prepareddeck

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type fakeStore struct {
	preparation domain.DeckPreparation
	source      domain.SourceMaterial
	completed   *cardexport.Artifact
	failed      string
}

func (s *fakeStore) ClaimDeckPreparation(context.Context, string, string) (domain.DeckPreparation, error) {
	s.preparation.State = domain.DeckPreparationPreparing
	return s.preparation, nil
}
func (s *fakeStore) GetSourceMaterial(context.Context, string, string) (domain.SourceMaterial, error) {
	return s.source, nil
}
func (s *fakeStore) FailDeckPreparation(_ context.Context, _, _, message string) (domain.DeckPreparation, error) {
	s.failed = message
	s.preparation.State = domain.DeckPreparationFailed
	return s.preparation, nil
}
func (s *fakeStore) CompletePreparedDeck(_ context.Context, _, _ string, artifact cardexport.Artifact) (domain.DeckPreparation, error) {
	s.completed = &artifact
	s.preparation.State = domain.DeckPreparationReady
	return s.preparation, nil
}

type fakeBuilder struct {
	artifacts []cardexport.Artifact
	err       error
	calls     int
}

type fakeScopedBuilder struct {
	artifact cardexport.Artifact
	runID    string
	calls    int
}

func (b *fakeScopedBuilder) BuildCoverage(context.Context, string, string) (cardexport.Artifact, error) {
	return cardexport.Artifact{}, errors.New("legacy builder should not be used")
}

func (b *fakeScopedBuilder) BuildCoverageForAnalysis(_ context.Context, _, runID string) (cardexport.Artifact, error) {
	b.calls++
	b.runID = runID
	return b.artifact, nil
}

func (b *fakeBuilder) BuildCoverage(context.Context, string, string) (cardexport.Artifact, error) {
	b.calls++
	if b.err != nil {
		return cardexport.Artifact{}, b.err
	}
	return b.artifacts[b.calls-1], nil
}

type fakeEnricher struct {
	calls int
	err   error
}

type observedEnricher struct {
	results []enrichment.Result
	metrics []enrichment.ExternalMetrics
	errors  []error
	calls   int
}

func (*observedEnricher) ExternalConfigured() bool { return true }
func (e *observedEnricher) EnrichExternal(ctx context.Context, candidate enrichment.Candidate) (enrichment.Result, error) {
	result, _, err := e.EnrichExternalObserved(ctx, candidate)
	return result, err
}
func (e *observedEnricher) EnrichExternalObserved(context.Context, enrichment.Candidate) (enrichment.Result, enrichment.ExternalMetrics, error) {
	i := e.calls
	e.calls++
	return e.results[i], e.metrics[i], e.errors[i]
}

func (*fakeEnricher) ExternalConfigured() bool { return true }
func (e *fakeEnricher) EnrichExternal(context.Context, enrichment.Candidate) (enrichment.Result, error) {
	e.calls++
	return enrichment.Result{}, e.err
}

func TestWorkerEnrichesRerendersAndCompletes(t *testing.T) {
	store := &fakeStore{preparation: domain.DeckPreparation{ID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash"}, source: domain.SourceMaterial{ID: "book", OwnerID: "alice", ContentHash: "hash"}}
	first := cardexport.Artifact{APKG: []byte("before"), EnrichmentCandidates: []enrichment.Candidate{{Identity: enrichment.Identity{CanonicalLemma: "Haus", UPOS: "NOUN"}}}}
	final := cardexport.Artifact{APKG: []byte("after"), TSV: "recognition-tsv", Completeness: cardexport.Completeness{TotalCards: 1, CardsWithEnglishSentence: 1}}
	builder := &fakeBuilder{artifacts: []cardexport.Artifact{first, final}}
	enricher := &fakeEnricher{}
	worker := &Worker{Store: store, Builder: builder, Enrichment: enricher}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1}, Args: JobArgs{PreparationID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash", ExternalTranslationConsent: true}}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if builder.calls != 2 || enricher.calls != 1 || store.completed == nil || string(store.completed.APKG) != "after" || store.completed.TSV != "recognition-tsv" || store.failed != "" {
		t.Fatalf("calls=%d enrichment=%d completed=%+v failed=%q", builder.calls, enricher.calls, store.completed, store.failed)
	}
}

func TestWorkerEmitsPrivacySafePhaseAndOutcomeObservation(t *testing.T) {
	store := &fakeStore{preparation: domain.DeckPreparation{ID: "private-preparation", OwnerID: "private-owner", SourceMaterialID: "book", ContentHash: "hash"}, source: domain.SourceMaterial{ID: "book", OwnerID: "private-owner", Title: "Private Title", ContentHash: "hash"}}
	candidates := []enrichment.Candidate{
		{Identity: enrichment.Identity{Language: "de", CanonicalLemma: "secret-lemma-1", UPOS: "NOUN"}, ExampleSentence: "Private sentence one."},
		{Identity: enrichment.Identity{Language: "de", CanonicalLemma: "secret-lemma-2", UPOS: "NOUN"}, ExampleSentence: "Private sentence two."},
	}
	first := cardexport.Artifact{APKG: []byte("before"), Count: 2, Completeness: cardexport.Completeness{TotalCards: 2, QualityOmitted: 1}, EnrichmentCandidates: candidates}
	final := cardexport.Artifact{APKG: []byte("after"), Count: 2, Completeness: cardexport.Completeness{TotalCards: 2, CardsWithEnglish: 1, CardsWithEnglishSentence: 1, QualityOmitted: 1}}
	enricher := &observedEnricher{
		results: []enrichment.Result{{SentenceTranslation: enrichment.Field[string]{Available: true}}, {}},
		metrics: []enrichment.ExternalMetrics{
			{CacheHits: 1, CacheLatency: time.Millisecond},
			{CacheMisses: 1, ProviderCalls: 1, Attempts: 2, Retries: 1, ProviderLatency: 2 * time.Millisecond, RateLimitErrors: 1},
		},
		errors: []error{nil, errors.New("provider response must not be observed")},
	}
	var got Observation
	worker := &Worker{Store: store, Builder: &fakeBuilder{artifacts: []cardexport.Artifact{first, final}}, Enrichment: enricher, Observer: ObserverFunc(func(observation Observation) { got = observation })}
	created := time.Now().Add(-time.Second)
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1, Attempt: 2, CreatedAt: created}, Args: JobArgs{PreparationID: "private-preparation", OwnerID: "private-owner", SourceMaterialID: "book", ContentHash: "hash", ExternalTranslationConsent: true}}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if got.Outcome != "ready" || got.JobAttempt != 2 || got.ConfiguredConcurrency != 1 || got.EffectiveConcurrency != 1 || got.PeakInFlightProviderCalls != 1 {
		t.Fatalf("observation header=%+v", got)
	}
	wantCounts := OutcomeCounts{Selected: 3, Accepted: 2, Omitted: 1, TranslationEligible: 2, Translated: 1, Untranslated: 1, CacheHits: 1, CacheMisses: 1, ProviderCalls: 1, Attempts: 2, Retries: 1}
	if got.Counts != wantCounts || got.Errors.RateLimit != 1 || got.Completeness != (Completeness{TotalCards: 2, CardsWithEnglish: 1, CardsWithContextualTranslation: 1, QualityOmissions: 1}) {
		t.Fatalf("observation=%+v", got)
	}
	if got.Durations.QueueWait <= 0 || got.Durations.Total < got.Durations.QueueWait || got.Durations.Cache != time.Millisecond || got.Durations.Provider != 2*time.Millisecond {
		t.Fatalf("durations=%+v", got.Durations)
	}

	var output bytes.Buffer
	NewLogObserver(log.New(&output, "", 0)).Observe(got)
	logged := output.String()
	for _, private := range []string{"private-preparation", "private-owner", "Private Title", "secret-lemma", "Private sentence", "provider response"} {
		if strings.Contains(logged, private) {
			t.Fatalf("observation leaked %q: %s", private, logged)
		}
	}
	if !strings.Contains(logged, `"event":"prepared_deck_preparation"`) || !strings.Contains(logged, `"cache_hits":1`) {
		t.Fatalf("missing aggregate fields: %s", logged)
	}
}

func TestObservationFailureCannotFailPreparation(t *testing.T) {
	store := &fakeStore{preparation: domain.DeckPreparation{ID: "p", SourceMaterialID: "book", ContentHash: "hash"}, source: domain.SourceMaterial{ID: "book", ContentHash: "hash"}}
	worker := &Worker{
		Store:    store,
		Builder:  &fakeBuilder{artifacts: []cardexport.Artifact{{APKG: []byte("ready")}}},
		Observer: ObserverFunc(func(Observation) { panic("metrics backend unavailable") }),
	}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1}, Args: JobArgs{PreparationID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash"}}
	if err := worker.Work(context.Background(), job); err != nil || store.completed == nil {
		t.Fatalf("err=%v completed=%+v", err, store.completed)
	}
}

func TestWorkerCompletesWithDeterministicFallbackWhenEnrichmentIsUnavailable(t *testing.T) {
	store := &fakeStore{preparation: domain.DeckPreparation{ID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash"}, source: domain.SourceMaterial{ID: "book", OwnerID: "alice", ContentHash: "hash"}}
	first := cardexport.Artifact{APKG: []byte("before"), TSV: "before-tsv", EnrichmentCandidates: []enrichment.Candidate{{Identity: enrichment.Identity{CanonicalLemma: "Haus", UPOS: "NOUN"}}}}
	final := cardexport.Artifact{APKG: []byte("after"), TSV: "recognition-tsv", Completeness: cardexport.Completeness{TotalCards: 1, QualityOmitted: 1}}
	builder := &fakeBuilder{artifacts: []cardexport.Artifact{first, final}}
	worker := &Worker{Store: store, Builder: builder, Enrichment: &fakeEnricher{err: errors.New("provider unavailable")}}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1}, Args: JobArgs{PreparationID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash", ExternalTranslationConsent: true}}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if builder.calls != 2 || store.completed == nil || string(store.completed.APKG) != "after" || store.completed.TSV != "recognition-tsv" || store.completed.Completeness.QualityOmitted != 1 || store.failed != "" {
		t.Fatalf("calls=%d completed=%+v failed=%q", builder.calls, store.completed, store.failed)
	}
}

func TestWorkerFailureMarksFailedWithoutCompletion(t *testing.T) {
	store := &fakeStore{preparation: domain.DeckPreparation{ID: "p", SourceMaterialID: "book", ContentHash: "hash"}, source: domain.SourceMaterial{ID: "book", ContentHash: "hash"}}
	worker := &Worker{Store: store, Builder: &fakeBuilder{err: errors.New("render exploded")}}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1}, Args: JobArgs{PreparationID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash"}}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if store.failed == "" || store.completed != nil {
		t.Fatalf("failed=%q completed=%+v", store.failed, store.completed)
	}
}

func TestWorkerRejectsChangedImmutableSourceIdentity(t *testing.T) {
	store := &fakeStore{preparation: domain.DeckPreparation{ID: "p", SourceMaterialID: "book", ContentHash: "original"}, source: domain.SourceMaterial{ID: "book", ContentHash: "changed"}}
	worker := &Worker{Store: store, Builder: &fakeBuilder{artifacts: []cardexport.Artifact{{APKG: []byte("unexpected")}}}}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1}, Args: JobArgs{PreparationID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "original"}}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if store.failed != "source material identity changed" || store.completed != nil {
		t.Fatalf("failed=%q completed=%+v", store.failed, store.completed)
	}
}

func TestWorkerBuildsFromTheImmutableAnalysisRun(t *testing.T) {
	const runID = "analysis-run-1"
	store := &fakeStore{preparation: domain.DeckPreparation{ID: "p", OwnerID: "alice", SourceMaterialID: "book", AnalysisRunID: runID, ContentHash: "hash"}, source: domain.SourceMaterial{ID: "book", OwnerID: "alice", ContentHash: "a-newer-source-hash"}}
	builder := &fakeScopedBuilder{artifact: cardexport.Artifact{APKG: []byte("scoped")}}
	worker := &Worker{Store: store, Builder: builder}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1}, Args: JobArgs{PreparationID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash", AnalysisRunID: runID}}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if builder.calls != 1 || builder.runID != runID || store.completed == nil || string(store.completed.APKG) != "scoped" {
		t.Fatalf("calls=%d run=%q completed=%+v", builder.calls, builder.runID, store.completed)
	}
}
