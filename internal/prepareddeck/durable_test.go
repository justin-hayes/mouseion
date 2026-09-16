package prepareddeck

import (
	"bytes"
	"context"
	"errors"
	"log"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type finalizerStoreStub struct {
	run                                                domain.PreparedDeckRun
	preparation                                        domain.DeckPreparation
	projection                                         cardexport.StorageProjection
	stored                                             []cardexport.StoredResult
	claimedGeneration                                  int
	claimedToken                                       string
	claimLeaseExpiresAt                                time.Time
	completedToken                                     string
	loadCalls, completeCalls, getCalls, supersedeCalls int
	supersedeVersion                                   int
	repreparationCalls                                 int
	corpusSentences                                    map[string]map[int64]analyzer.Sentence
	corpusCalls                                        int
	failCalls                                          int
}

func (s *finalizerStoreStub) ClaimPreparedDeckFinalization(_ context.Context, _, _, _ string, generation int, token string, leaseExpiresAt time.Time) (domain.PreparedDeckRun, error) {
	s.claimedGeneration, s.claimedToken, s.claimLeaseExpiresAt = generation, token, leaseExpiresAt
	return s.run, nil
}

func (s *finalizerStoreStub) LoadPreparedDeckFinalization(context.Context, string, string, string) (cardexport.StorageProjection, []cardexport.StoredResult, error) {
	s.loadCalls++
	return s.projection, s.stored, nil
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

func (s *finalizerStoreStub) GetPreparedDeckRun(context.Context, string, string, string) (domain.PreparedDeckRun, error) {
	return s.run, nil
}

func (s *finalizerStoreStub) MarkPreparedDeckRequiresRepreparation(context.Context, string, string, string) error {
	s.repreparationCalls++
	return nil
}

func (s *finalizerStoreStub) SupersedePreparedDeckArtifact(_ context.Context, _, _, _ string, version int, _ cardexport.Artifact) (domain.DeckPreparation, error) {
	s.supersedeCalls++
	s.supersedeVersion = version
	s.run.PresentationVersion = version
	s.run.RenderInputVersion = cardexport.RenderInputVersion
	s.preparation.PresentationVersion = version
	s.preparation.DeckRevision++
	return s.preparation, nil
}

func (s *finalizerStoreStub) ListCorpusSentences(_ context.Context, _, corpusID string, ordinals []int64) (map[int64]analyzer.Sentence, error) {
	s.corpusCalls++
	result := make(map[int64]analyzer.Sentence)
	for _, ordinal := range ordinals {
		if sentence, ok := s.corpusSentences[corpusID][ordinal]; ok {
			result[ordinal] = sentence
		}
	}
	return result, nil
}

func (s *finalizerStoreStub) FailPreparedDeckFinalization(context.Context, string, string, string, string, string, string) error {
	s.failCalls++
	return nil
}

type finalizerRendererStub struct {
	artifact cardexport.Artifact
	err      error
	calls    int
}

func (r *finalizerRendererStub) Restore(cardexport.StorageProjection) (cardexport.FrozenDeck, error) {
	return cardexport.FrozenDeck{}, nil
}

func (r *finalizerRendererStub) Finalize(context.Context, cardexport.FrozenDeck, []cardexport.StoredResult, cardexport.RunFacts) (cardexport.FinalArtifact, cardexport.FinalizeDiagnostics, error) {
	r.calls++
	return r.artifact, cardexport.FinalizeDiagnostics{}, r.err
}

type corpusRendererStub struct {
	artifact cardexport.Artifact
	calls    int
}

func (r *corpusRendererStub) Restore(projection cardexport.StorageProjection) (cardexport.FrozenDeck, error) {
	return cardexport.NewPresentation(nil).Restore(projection)
}

func (r *corpusRendererStub) Finalize(ctx context.Context, deck cardexport.FrozenDeck, stored []cardexport.StoredResult, facts cardexport.RunFacts) (cardexport.FinalArtifact, cardexport.FinalizeDiagnostics, error) {
	r.calls++
	artifact, diagnostics, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, stored, facts)
	r.artifact = artifact
	return artifact, diagnostics, err
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

func TestDurableFinalizerFailsRunForPresentationValidationError(t *testing.T) {
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}}
	renderer := &finalizerRendererStub{err: cardexport.ErrInvalidInput}
	_, err := (&DurableFinalizer{Store: store, Renderer: renderer}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	assert.ErrorIs(t, err, cardexport.ErrInvalidInput)
	assert.Equal(t, 1, store.failCalls)
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

func TestDurableRerendererReplaysCompletedRunAndIsIdempotent(t *testing.T) {
	store := &finalizerStoreStub{
		run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, PresentationVersion: 1},
		preparation: domain.DeckPreparation{State: domain.DeckPreparationReady, CurrentRunID: "run", DeckRevision: 1},
	}
	renderer := &finalizerRendererStub{artifact: cardexport.Artifact{APKG: []byte("rerendered")}}
	rerenderer := &DurableRerenderer{Store: store, Renderer: renderer}

	updated, err := rerenderer.Rerender(context.Background(), "owner", "preparation", "run", 2)
	require.NoError(t, err)
	assert.Equal(t, 2, updated.PresentationVersion)
	assert.Equal(t, 2, updated.DeckRevision)
	assert.Equal(t, 1, renderer.calls)
	assert.Equal(t, 1, store.loadCalls)
	assert.Equal(t, 1, store.supersedeCalls)
	assert.Equal(t, 2, store.supersedeVersion)

	updated, err = rerenderer.Rerender(context.Background(), "owner", "preparation", "run", 2)
	require.NoError(t, err)
	assert.Equal(t, 2, updated.DeckRevision)
	assert.Equal(t, 1, renderer.calls, "same run and presentation version must not render twice")
	assert.Equal(t, 1, store.supersedeCalls)
}

func TestDurableRerendererDoesNotRenderRetiredPreparation(t *testing.T) {
	retiredAt := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	store := &finalizerStoreStub{
		run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, PresentationVersion: 1},
		preparation: domain.DeckPreparation{State: domain.DeckPreparationReady, RetiredAt: &retiredAt},
	}
	renderer := &finalizerRendererStub{}

	_, err := (&DurableRerenderer{Store: store, Renderer: renderer}).Rerender(context.Background(), "owner", "preparation", "run", 2)

	assert.ErrorIs(t, err, persistence.ErrInvalidTransition)
	assert.Zero(t, renderer.calls)
	assert.Zero(t, store.loadCalls)
	assert.Zero(t, store.supersedeCalls)
}

func TestDurableRerendererRecoversLegacyParseFromCorpus(t *testing.T) {
	store := &finalizerStoreStub{
		run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, RenderInputVersion: 0, PresentationVersion: 0},
		preparation: domain.DeckPreparation{State: domain.DeckPreparationReady, CurrentRunID: "run", DeckRevision: 1},
		corpusSentences: map[string]map[int64]analyzer.Sentence{
			"corpus": {7: {Text: "Ich stehe heute auf.", Tokens: []analyzer.Token{
				{Surface: "Ich", Dependency: "nsubj", Head: 1},
				{Surface: "stehe", Dependency: "root", Head: 1},
				{Surface: "heute", Dependency: "advmod", Head: 1},
				{Surface: "auf", Dependency: "compound:prt", Head: 1},
			}}},
		},
	}
	store.projection = legacyRerenderManifest(t)
	renderer := &corpusRendererStub{}

	updated, err := (&DurableRerenderer{Store: store, Renderer: renderer}).Rerender(context.Background(), "owner", "preparation", "run", cardexport.PresentationVersion)

	require.NoError(t, err)
	assert.Equal(t, 2, updated.DeckRevision)
	assert.Equal(t, 1, store.corpusCalls)
	assert.Equal(t, 1, renderer.calls)
	require.Len(t, renderer.artifact.Generated, 1)
	assert.Equal(t, "Ich <b>stehe</b> heute <b>auf</b>.", renderer.artifact.Generated[0].Note.Text)
	assert.Equal(t, cardexport.PresentationVersion, store.supersedeVersion)
}

func TestDurableRerendererReportsMissingLegacyInputWithoutPublishing(t *testing.T) {
	store := &finalizerStoreStub{
		run:             domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, RenderInputVersion: 0, PresentationVersion: 1},
		preparation:     domain.DeckPreparation{State: domain.DeckPreparationReady, CurrentRunID: "run", DeckRevision: 1},
		corpusSentences: map[string]map[int64]analyzer.Sentence{},
	}
	store.projection = legacyRerenderManifest(t)
	renderer := &corpusRendererStub{}

	_, err := (&DurableRerenderer{Store: store, Renderer: renderer}).Rerender(context.Background(), "owner", "preparation", "run", 1)

	assert.ErrorIs(t, err, ErrRequiresRepreparation)
	assert.Equal(t, 1, store.corpusCalls)
	assert.Zero(t, renderer.calls)
	assert.Zero(t, store.supersedeCalls)
}

func TestRerenderWorkerSurfacesMissingLegacyInputWithoutRetrying(t *testing.T) {
	store := &finalizerStoreStub{
		run:             domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, RenderInputVersion: 0, PresentationVersion: 0},
		preparation:     domain.DeckPreparation{State: domain.DeckPreparationReady, CurrentRunID: "run", DeckRevision: 1},
		corpusSentences: map[string]map[int64]analyzer.Sentence{},
		projection:      legacyRerenderManifest(t),
	}
	worker := &RerenderWorker{Rerenderer: &DurableRerenderer{Store: store, Renderer: &corpusRendererStub{}}}

	err := worker.Work(context.Background(), &river.Job[RerenderJobArgs]{Args: RerenderJobArgs{OwnerID: "owner", PreparationID: "preparation", RunID: "run", PresentationVersion: 1}})

	require.NoError(t, err)
	assert.Equal(t, 1, store.repreparationCalls)
}

func legacyRerenderManifest(t *testing.T) cardexport.StorageProjection {
	t.Helper()
	return cardexport.StorageProjection{
		SchemaVersion: cardexport.ManifestSchemaVersion,
		Owner:         "owner",
		DeckName:      "Legacy",
		Filename:      cardexport.DownloadFilename("Legacy"),
		Items: []cardexport.ManifestItem{{
			Ordinal: 0, Disposition: cardexport.ManifestAccepted, CorpusID: "corpus", SentenceOrdinal: 7,
			Entry:   cardexport.Entry{Language: "de", CanonicalLemma: "aufstehen", UPOS: "VERB", Sentence: "Ich stehe heute auf.", TargetWord: "stehe", SourceDocument: "Legacy", FirstEncounter: 1},
			Quality: cardexport.SentenceQuality{Accepted: true, Reasons: []string{"target present"}},
		}},
	}
}
