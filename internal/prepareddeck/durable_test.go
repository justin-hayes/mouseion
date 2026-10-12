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
	failClass, failCode                                string
	loadErr, claimErr, completeErr, failErr            error
}

func (s *finalizerStoreStub) ClaimPreparedDeckFinalization(_ context.Context, _, _, _ string, generation int, token string, leaseExpiresAt time.Time) (domain.PreparedDeckRun, error) {
	s.claimedGeneration, s.claimedToken, s.claimLeaseExpiresAt = generation, token, leaseExpiresAt
	return s.run, s.claimErr
}

func (s *finalizerStoreStub) LoadPreparedDeckFinalization(context.Context, string, string, string) (cardexport.StorageProjection, []cardexport.StoredResult, error) {
	s.loadCalls++
	return s.projection, s.stored, s.loadErr
}

func (s *finalizerStoreStub) CompletePreparedDeckRun(_ context.Context, _, _, _, token string, _ cardexport.Artifact) (domain.DeckPreparation, error) {
	s.completeCalls++
	s.completedToken = token
	return s.preparation, s.completeErr
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

func (s *finalizerStoreStub) FailPreparedDeckFinalization(_ context.Context, _, _, _, _, class, code string) error {
	s.failCalls++
	s.failClass, s.failCode = class, code
	return s.failErr
}

type finalizerRendererStub struct {
	artifact    cardexport.Artifact
	diagnostics cardexport.FinalizeDiagnostics
	err         error
	restoreErr  error
	calls       int
}

func (r *finalizerRendererStub) Restore(cardexport.StorageProjection) (cardexport.FrozenDeck, error) {
	return cardexport.FrozenDeck{}, r.restoreErr
}

func (r *finalizerRendererStub) RestoreWithRecoveredRenderInputs(projection cardexport.StorageProjection, _ []cardexport.RecoveredRenderInput) (cardexport.FrozenDeck, error) {
	return r.Restore(projection)
}

func (r *finalizerRendererStub) Finalize(context.Context, cardexport.FrozenDeck, []cardexport.StoredResult, cardexport.RunFacts) (cardexport.FinalArtifact, cardexport.FinalizeDiagnostics, error) {
	r.calls++
	return r.artifact, r.diagnostics, r.err
}

type corpusRendererStub struct {
	artifact cardexport.Artifact
	calls    int
}

func (r *corpusRendererStub) Restore(projection cardexport.StorageProjection) (cardexport.FrozenDeck, error) {
	return cardexport.NewPresentation(nil).Restore(projection)
}

func (r *corpusRendererStub) RestoreWithRecoveredRenderInputs(projection cardexport.StorageProjection, recovered []cardexport.RecoveredRenderInput) (cardexport.FrozenDeck, error) {
	return cardexport.NewPresentation(nil).RestoreWithRecoveredRenderInputs(projection, recovered)
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
	t.Run("propagates the render error", func(t *testing.T) {
		require.ErrorIs(t, err, renderErr)
	})
	t.Run("does not publish", func(t *testing.T) {
		require.Zero(t, store.completeCalls)
	})
	t.Run("records the failure", func(t *testing.T) {
		require.Equal(t, 1, store.failCalls)
	})
}

func TestDurableFinalizerFailsRestoreFailure(t *testing.T) {
	restoreErr := errors.New("restore failed")
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}}
	renderer := &finalizerRendererStub{restoreErr: restoreErr}

	_, err := (&DurableFinalizer{Store: store, Renderer: renderer}).Finalize(context.Background(), "owner", "preparation", "run", 0)

	t.Run("propagates the restore error", func(t *testing.T) {
		require.ErrorIs(t, err, restoreErr)
	})
	t.Run("records the failure", func(t *testing.T) {
		require.Equal(t, 1, store.failCalls)
	})
	t.Run("does not publish", func(t *testing.T) {
		require.Zero(t, store.completeCalls)
	})
}

func TestDurableFinalizerFailsRunForPresentationValidationError(t *testing.T) {
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}}
	renderer := &finalizerRendererStub{err: cardexport.ErrInvalidInput}
	_, err := (&DurableFinalizer{Store: store, Renderer: renderer}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	t.Run("classifies the validation error", func(t *testing.T) {
		require.ErrorIs(t, err, cardexport.ErrInvalidInput)
	})
	t.Run("records the failure", func(t *testing.T) {
		require.Equal(t, 1, store.failCalls)
	})
	t.Run("does not publish", func(t *testing.T) {
		require.Zero(t, store.completeCalls)
	})
}

func TestDurableFinalizerRejectsIncompleteInput(t *testing.T) {
	store := &finalizerStoreStub{}
	cases := map[string]*DurableFinalizer{
		"nil finalizer": nil,
		"no store":      {Renderer: &finalizerRendererStub{}},
		"no renderer":   {Store: store},
	}
	for name, finalizer := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := finalizer.Finalize(context.Background(), "owner", "preparation", "run", 0)
			require.ErrorIs(t, err, ErrInvalidInput)
		})
	}
	t.Run("blank identity", func(t *testing.T) {
		_, err := (&DurableFinalizer{Store: store, Renderer: &finalizerRendererStub{}}).Finalize(context.Background(), "owner", " ", "run", 0)
		require.ErrorIs(t, err, ErrInvalidInput)
	})
	assert.Empty(t, store.claimedToken, "invalid input must not claim the run")
}

func TestDurableFinalizerReturnsClaimFailureWithoutLoading(t *testing.T) {
	store := &finalizerStoreStub{claimErr: persistence.ErrPreparedDeckClaimLost}
	_, err := (&DurableFinalizer{Store: store, Renderer: &finalizerRendererStub{}}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	require.ErrorIs(t, err, persistence.ErrPreparedDeckClaimLost)
	assert.Zero(t, store.loadCalls)
	assert.Zero(t, store.failCalls)
}

func TestDurableFinalizerLoadFailure(t *testing.T) {
	cases := []struct {
		name      string
		loadErr   error
		wantFails int
	}{
		{name: "identity mismatch fails the run", loadErr: persistence.ErrPreparedDeckIdentity, wantFails: 1},
		{name: "invalid presentation input fails the run", loadErr: cardexport.ErrInvalidInput, wantFails: 1},
		{name: "transient load error leaves the run for retry", loadErr: errors.New("database unavailable")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}, loadErr: tc.loadErr}
			renderer := &finalizerRendererStub{}
			_, err := (&DurableFinalizer{Store: store, Renderer: renderer}).Finalize(context.Background(), "owner", "preparation", "run", 0)
			require.ErrorIs(t, err, tc.loadErr)
			assert.Equal(t, tc.wantFails, store.failCalls)
			if tc.wantFails > 0 {
				assert.Equal(t, "presentation", store.failClass)
				assert.Equal(t, "invalid_manifest", store.failCode)
			}
			assert.Zero(t, renderer.calls)
			assert.Zero(t, store.completeCalls)
		})
	}
}

func TestDurableFinalizerClassifiesRecordedFailures(t *testing.T) {
	cases := []struct {
		name                 string
		renderer             *finalizerRendererStub
		wantClass, wantCode  string
		wantRenderErrWrapped bool
	}{
		{name: "restore failure", renderer: &finalizerRendererStub{restoreErr: errors.New("restore failed")}, wantClass: "presentation", wantCode: "invalid_manifest"},
		{name: "render failure", renderer: &finalizerRendererStub{err: errors.New("render failed")}, wantClass: "presentation", wantCode: "render_failed", wantRenderErrWrapped: true},
		{name: "all meanings unresolved", renderer: &finalizerRendererStub{err: cardexport.ErrAllMeaningsUnresolved}, wantClass: "validation", wantCode: "all_meanings_unresolved", wantRenderErrWrapped: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failErr := errors.New("failure write unavailable")
			store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}, failErr: failErr}
			_, err := (&DurableFinalizer{Store: store, Renderer: tc.renderer}).Finalize(context.Background(), "owner", "preparation", "run", 0)
			require.ErrorIs(t, err, failErr, "a failed failure write is joined to the returned error")
			assert.Contains(t, err.Error(), "record finalization failure")
			if tc.wantRenderErrWrapped {
				assert.Contains(t, err.Error(), "render durable prepared deck")
			}
			assert.Equal(t, tc.wantClass, store.failClass)
			assert.Equal(t, tc.wantCode, store.failCode)
			assert.Zero(t, store.completeCalls)
		})
	}
}

func TestDurableFinalizerDoesNotRecordFailureAfterCancellation(t *testing.T) {
	cases := map[string]struct {
		store    *finalizerStoreStub
		renderer *finalizerRendererStub
	}{
		"load":    {store: &finalizerStoreStub{loadErr: persistence.ErrPreparedDeckIdentity}, renderer: &finalizerRendererStub{}},
		"restore": {store: &finalizerStoreStub{}, renderer: &finalizerRendererStub{restoreErr: errors.New("restore failed")}},
		"render":  {store: &finalizerStoreStub{}, renderer: &finalizerRendererStub{err: errors.New("render failed")}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			tc.store.run = domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}
			_, err := (&DurableFinalizer{Store: tc.store, Renderer: tc.renderer}).Finalize(ctx, "owner", "preparation", "run", 0)
			require.Error(t, err)
			assert.Zero(t, tc.store.failCalls, "a cancelled finalizer leaves the run for its successor")
		})
	}
}

func TestDurableFinalizerReturnsPublicationFailure(t *testing.T) {
	completeErr := persistence.ErrPreparedDeckClaimLost
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}, completeErr: completeErr}
	metrics := &recordingBatchMetrics{}
	_, err := (&DurableFinalizer{Store: store, Renderer: &finalizerRendererStub{}, Metrics: metrics}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	require.ErrorIs(t, err, completeErr)
	assert.Equal(t, 1, store.completeCalls)
	assert.Zero(t, store.failCalls)
	assert.Contains(t, metrics.metrics, BatchMetric{Mode: "unknown", Name: MetricAPKGOutcome, Phase: "finalizing", State: "failed", ErrorClass: "terminal", Provider: "openai", Value: 1})
}

func TestDurableFinalizerRecordsOutcomeMetricsByExecutionMode(t *testing.T) {
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing, ExecutionMode: domain.PreparedDeckExecutionStandard}}
	metrics := &recordingBatchMetrics{}
	_, err := (&DurableFinalizer{Store: store, Renderer: &finalizerRendererStub{}, Metrics: metrics}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	require.NoError(t, err)
	assert.Contains(t, metrics.metrics, BatchMetric{Mode: "standard", Name: MetricAPKGOutcome, Phase: "finalizing", State: "completed", Provider: "openai", Value: 1})
	names := make([]string, 0, len(metrics.metrics))
	for _, metric := range metrics.metrics {
		names = append(names, metric.Name)
	}
	assert.Equal(t, []string{MetricAPKGOutcome, MetricBatchPhaseLatency, MetricBatchTotalLatency}, names)
}

type recordingBatchMetrics struct{ metrics []BatchMetric }

func (m *recordingBatchMetrics) ObserveBatch(metric BatchMetric) {
	m.metrics = append(m.metrics, metric)
}

func TestDurableFinalizerLogsInvalidSenseSelectionFromDiagnostics(t *testing.T) {
	var output bytes.Buffer
	previousWriter, previousFlags := log.Writer(), log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}}
	renderer := &finalizerRendererStub{diagnostics: cardexport.FinalizeDiagnostics{DegradationCodes: []string{cardexport.DegradationInvalidSenseSelection}}}
	_, err := (&DurableFinalizer{Store: store, Renderer: renderer}).Finalize(context.Background(), "owner", "preparation", "run", 0)

	require.NoError(t, err)
	assert.Contains(t, output.String(), "prepared deck translation: invalid sense selection; using deterministic order")
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

func TestDurableRerendererLogsInvalidSenseSelectionFromDiagnostics(t *testing.T) {
	var output bytes.Buffer
	previousWriter, previousFlags := log.Writer(), log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	store := &finalizerStoreStub{
		run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, PresentationVersion: 1},
		preparation: domain.DeckPreparation{State: domain.DeckPreparationReady, CurrentRunID: "run"},
	}
	renderer := &finalizerRendererStub{diagnostics: cardexport.FinalizeDiagnostics{DegradationCodes: []string{cardexport.DegradationInvalidSenseSelection}}}
	_, err := (&DurableRerenderer{Store: store, Renderer: renderer}).Rerender(context.Background(), "owner", "preparation", "run", 2)

	require.NoError(t, err)
	assert.Contains(t, output.String(), "prepared deck translation: invalid sense selection; using deterministic order")
}

func TestDurableRerendererDoesNotRenderRetiredPreparation(t *testing.T) {
	retiredAt := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	store := &finalizerStoreStub{
		run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, PresentationVersion: 1},
		preparation: domain.DeckPreparation{State: domain.DeckPreparationReady, RetiredAt: &retiredAt},
	}
	renderer := &finalizerRendererStub{}

	_, err := (&DurableRerenderer{Store: store, Renderer: renderer}).Rerender(context.Background(), "owner", "preparation", "run", 2)

	t.Run("rejects the retired preparation", func(t *testing.T) {
		require.ErrorIs(t, err, persistence.ErrInvalidTransition)
	})
	t.Run("does not render", func(t *testing.T) {
		require.Zero(t, renderer.calls)
	})
	t.Run("does not load or supersede", func(t *testing.T) {
		require.Zero(t, store.loadCalls)
		require.Zero(t, store.supersedeCalls)
	})
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

	t.Run("classifies the missing input as requiring repreparation", func(t *testing.T) {
		require.ErrorIs(t, err, ErrRequiresRepreparation)
	})
	t.Run("reads the corpus once", func(t *testing.T) {
		require.Equal(t, 1, store.corpusCalls)
	})
	t.Run("does not render", func(t *testing.T) {
		require.Zero(t, renderer.calls)
	})
	t.Run("does not publish", func(t *testing.T) {
		require.Zero(t, store.supersedeCalls)
	})
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

func TestRerenderWorkerMarksIdentityCorruptionForRepreparation(t *testing.T) {
	store := &finalizerStoreStub{
		run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, RenderInputVersion: 1, PresentationVersion: 1},
		preparation: domain.DeckPreparation{State: domain.DeckPreparationReady, CurrentRunID: "run"},
		loadErr:     persistence.ErrPreparedDeckIdentity,
	}
	worker := &RerenderWorker{Rerenderer: &DurableRerenderer{Store: store, Renderer: &corpusRendererStub{}}}

	err := worker.Work(context.Background(), &river.Job[RerenderJobArgs]{Args: RerenderJobArgs{OwnerID: "owner", PreparationID: "preparation", RunID: "run", PresentationVersion: 2}})

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
