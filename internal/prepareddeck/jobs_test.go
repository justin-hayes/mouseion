package prepareddeck

import (
	"context"
	"errors"
	"testing"

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

type fakeEnricher struct{ calls int }

func (*fakeEnricher) ExternalConfigured() bool { return true }
func (e *fakeEnricher) EnrichExternal(context.Context, enrichment.Candidate) (enrichment.Result, error) {
	e.calls++
	return enrichment.Result{}, nil
}

func TestWorkerEnrichesRerendersAndCompletes(t *testing.T) {
	store := &fakeStore{preparation: domain.DeckPreparation{ID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash"}, source: domain.SourceMaterial{ID: "book", OwnerID: "alice", ContentHash: "hash"}}
	first := cardexport.Artifact{APKG: []byte("before"), EnrichmentCandidates: []enrichment.Candidate{{Identity: enrichment.Identity{CanonicalLemma: "Haus", UPOS: "NOUN"}}}}
	final := cardexport.Artifact{APKG: []byte("after"), Completeness: cardexport.Completeness{TotalCards: 1, CardsWithEnglishSentence: 1}}
	builder := &fakeBuilder{artifacts: []cardexport.Artifact{first, final}}
	enricher := &fakeEnricher{}
	worker := &Worker{Store: store, Builder: builder, Enrichment: enricher}
	job := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 1}, Args: JobArgs{PreparationID: "p", OwnerID: "alice", SourceMaterialID: "book", ContentHash: "hash", ExternalTranslationConsent: true}}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if builder.calls != 2 || enricher.calls != 1 || store.completed == nil || string(store.completed.APKG) != "after" || store.failed != "" {
		t.Fatalf("calls=%d enrichment=%d completed=%+v failed=%q", builder.calls, enricher.calls, store.completed, store.failed)
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
