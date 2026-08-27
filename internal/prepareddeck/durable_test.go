package prepareddeck

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
)

type finalizerStoreStub struct {
	run                                domain.PreparedDeckRun
	preparation                        domain.DeckPreparation
	manifest                           cardexport.Manifest
	exact                              []cardexport.ExactEnrichment
	claimedGeneration                  int
	claimedToken                       string
	claimLeaseExpiresAt                time.Time
	completedToken                     string
	loadCalls, completeCalls, getCalls int
}

func (s *finalizerStoreStub) ClaimPreparedDeckFinalization(_ context.Context, _, _, _ string, generation int, token string, leaseExpiresAt time.Time) (domain.PreparedDeckRun, error) {
	s.claimedGeneration, s.claimedToken, s.claimLeaseExpiresAt = generation, token, leaseExpiresAt
	return s.run, nil
}

func (s *finalizerStoreStub) LoadPreparedDeckFinalization(context.Context, string, string, string) (cardexport.Manifest, []cardexport.ExactEnrichment, error) {
	s.loadCalls++
	return s.manifest, s.exact, nil
}

func (s *finalizerStoreStub) CompletePreparedDeckRun(_ context.Context, _, _, _, token string, _ cardexport.Artifact) (domain.DeckPreparation, error) {
	s.completeCalls++
	s.completedToken = token
	return s.preparation, nil
}

func (s *finalizerStoreStub) GetDeckPreparation(context.Context, string, string) (domain.DeckPreparation, error) {
	s.getCalls++
	return s.preparation, nil
}

type finalizerRendererStub struct {
	artifact cardexport.Artifact
	err      error
	calls    int
}

func (r *finalizerRendererStub) RenderManifest(context.Context, cardexport.Manifest, []cardexport.ExactEnrichment) (cardexport.Artifact, error) {
	r.calls++
	return r.artifact, r.err
}

func TestDurableFinalizerUsesGenerationAndOneFencedPublication(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}, preparation: domain.DeckPreparation{State: domain.DeckPreparationReady}}
	renderer := &finalizerRendererStub{artifact: cardexport.Artifact{APKG: []byte("apkg")}}
	finalizer := &DurableFinalizer{Store: store, Renderer: renderer, Now: func() time.Time { return now }, LeaseDuration: 2 * time.Minute}
	ready, err := finalizer.Finalize(context.Background(), "owner", "preparation", "run", 7)
	if err != nil || ready.State != domain.DeckPreparationReady {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}
	if store.claimedGeneration != 7 || store.claimedToken == "" || store.claimLeaseExpiresAt != now.Add(2*time.Minute) {
		t.Fatalf("claim generation=%d token=%q lease=%s", store.claimedGeneration, store.claimedToken, store.claimLeaseExpiresAt)
	}
	if store.loadCalls != 1 || renderer.calls != 1 || store.completeCalls != 1 || store.completedToken != store.claimedToken || store.getCalls != 0 {
		t.Fatalf("calls load=%d render=%d complete=%d get=%d tokens=%q/%q", store.loadCalls, renderer.calls, store.completeCalls, store.getCalls, store.claimedToken, store.completedToken)
	}
}

func TestDurableFinalizerTreatsCompletedRunAsIdempotentSuccess(t *testing.T) {
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted}, preparation: domain.DeckPreparation{State: domain.DeckPreparationReady}}
	renderer := &finalizerRendererStub{}
	ready, err := (&DurableFinalizer{Store: store, Renderer: renderer}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	if err != nil || ready.State != domain.DeckPreparationReady {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}
	if store.getCalls != 1 || store.loadCalls != 0 || store.completeCalls != 0 || renderer.calls != 0 {
		t.Fatalf("calls get=%d load=%d complete=%d render=%d", store.getCalls, store.loadCalls, store.completeCalls, renderer.calls)
	}
}

func TestDurableFinalizerDoesNotPublishRenderFailure(t *testing.T) {
	renderErr := errors.New("render failed")
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}}
	renderer := &finalizerRendererStub{err: renderErr}
	_, err := (&DurableFinalizer{Store: store, Renderer: renderer}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	if !errors.Is(err, renderErr) || store.completeCalls != 0 {
		t.Fatalf("err=%v complete calls=%d", err, store.completeCalls)
	}
}
