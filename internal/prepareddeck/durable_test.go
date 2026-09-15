package prepareddeck

import (
	"bytes"
	"context"
	"errors"
	"log"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func (s *finalizerStoreStub) FailPreparedDeckFinalization(context.Context, string, string, string, string, string, string) error {
	return nil
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
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, ready.State)
	assert.Equal(t, 7, store.claimedGeneration)
	assert.NotEmpty(t, store.claimedToken)
	assert.Equal(t, now.Add(2*time.Minute), store.claimLeaseExpiresAt)
	assert.Equal(t, 1, store.loadCalls)
	assert.Equal(t, 1, renderer.calls)
	assert.Equal(t, 1, store.completeCalls)
	assert.Equal(t, store.claimedToken, store.completedToken)
	assert.Zero(t, store.getCalls)
}

func TestDurableFinalizerTreatsCompletedRunAsIdempotentSuccess(t *testing.T) {
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted}, preparation: domain.DeckPreparation{State: domain.DeckPreparationReady}}
	renderer := &finalizerRendererStub{}
	ready, err := (&DurableFinalizer{Store: store, Renderer: renderer}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, ready.State)
	assert.Equal(t, 1, store.getCalls)
	assert.Zero(t, store.loadCalls)
	assert.Zero(t, store.completeCalls)
	assert.Zero(t, renderer.calls)
}

func TestDurableFinalizerDoesNotPublishRenderFailure(t *testing.T) {
	renderErr := errors.New("render failed")
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}}
	renderer := &finalizerRendererStub{err: renderErr}
	_, err := (&DurableFinalizer{Store: store, Renderer: renderer}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	assert.ErrorIs(t, err, renderErr)
	assert.Zero(t, store.completeCalls)
}

func TestDurableFinalizerLogsFallbackGlossUsageOnlyForConsentedRuns(t *testing.T) {
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()
	log.SetFlags(0)

	for _, test := range []struct {
		name                  string
		consented, configured bool
	}{
		{name: "consented", consented: true, configured: true},
		{name: "non-consented", consented: false, configured: false},
		{name: "provider disabled", consented: true, configured: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			log.SetOutput(&output)
			store := &finalizerStoreStub{run: domain.PreparedDeckRun{
				State:                         domain.PreparedDeckRunFinalizing,
				ExternalTranslationConsent:    test.consented,
				ExternalTranslationConfigured: test.configured,
			}, preparation: domain.DeckPreparation{State: domain.DeckPreparationReady}}
			renderer := &finalizerRendererStub{artifact: cardexport.Artifact{Completeness: cardexport.Completeness{
				TotalCards: 2, CardsWithFallbackGloss: 1,
			}}}

			_, err := (&DurableFinalizer{Store: store, Renderer: renderer}).Finalize(context.Background(), "owner", "preparation", "run", 0)
			require.NoError(t, err)
			if test.consented && test.configured {
				assert.Contains(t, output.String(), `fallback_gloss_usage {"event":"fallback_gloss_usage","selected":2,"fallback_used":1,"fallback_rate":0.5}`)
			} else {
				assert.NotContains(t, output.String(), "fallback_gloss_usage")
			}
		})
	}
}
