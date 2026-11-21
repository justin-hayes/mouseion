package prepareddeck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"testing"
	"time"

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
	cases := map[string]struct {
		finalizer     *DurableFinalizer
		preparationID string
	}{
		"nil finalizer":  {finalizer: nil, preparationID: "preparation"},
		"no store":       {finalizer: &DurableFinalizer{Renderer: &finalizerRendererStub{}}, preparationID: "preparation"},
		"no renderer":    {finalizer: &DurableFinalizer{Store: store}, preparationID: "preparation"},
		"blank identity": {finalizer: &DurableFinalizer{Store: store, Renderer: &finalizerRendererStub{}}, preparationID: " "},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tc.finalizer.Finalize(context.Background(), "owner", tc.preparationID, "run", 0)
			require.ErrorIs(t, err, ErrInvalidInput)
		})
	}
	t.Run("does not claim the run", func(t *testing.T) {
		require.Empty(t, store.claimedToken)
	})
}

func TestDurableFinalizerReturnsClaimFailureWithoutLoading(t *testing.T) {
	store := &finalizerStoreStub{claimErr: persistence.ErrPreparedDeckClaimLost}
	_, err := (&DurableFinalizer{Store: store, Renderer: &finalizerRendererStub{}}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	t.Run("propagates the claim error", func(t *testing.T) {
		require.ErrorIs(t, err, persistence.ErrPreparedDeckClaimLost)
	})
	t.Run("does not load the manifest", func(t *testing.T) {
		require.Zero(t, store.loadCalls)
	})
	t.Run("does not record a failure", func(t *testing.T) {
		require.Zero(t, store.failCalls)
	})
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
			t.Run("propagates the load error", func(t *testing.T) {
				require.ErrorIs(t, err, tc.loadErr)
			})
			t.Run("records the expected failures", func(t *testing.T) {
				require.Equal(t, tc.wantFails, store.failCalls)
			})
			if tc.wantFails > 0 {
				t.Run("classifies the manifest as invalid", func(t *testing.T) {
					require.Equal(t, []string{"presentation", "invalid_manifest"}, []string{store.failClass, store.failCode})
				})
			}
			t.Run("neither renders nor publishes", func(t *testing.T) {
				require.Equal(t, []int{0, 0}, []int{renderer.calls, store.completeCalls})
			})
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
			t.Run("names the failed failure write", func(t *testing.T) {
				require.ErrorContains(t, err, "record finalization failure")
			})
			if tc.wantRenderErrWrapped {
				t.Run("wraps the render error", func(t *testing.T) {
					require.ErrorContains(t, err, "render durable prepared deck")
				})
			}
			t.Run("records the failure class and code", func(t *testing.T) {
				require.Equal(t, []string{tc.wantClass, tc.wantCode}, []string{store.failClass, store.failCode})
			})
			t.Run("does not publish", func(t *testing.T) {
				require.Zero(t, store.completeCalls)
			})
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
			require.Zero(t, tc.store.failCalls, "a cancelled finalizer leaves the run for its successor")
		})
	}
}

func TestDurableFinalizerReturnsPublicationFailure(t *testing.T) {
	completeErr := persistence.ErrPreparedDeckClaimLost
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing}, completeErr: completeErr}
	metrics := &recordingBatchMetrics{}
	_, err := (&DurableFinalizer{Store: store, Renderer: &finalizerRendererStub{}, Metrics: metrics}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	t.Run("propagates the publication error", func(t *testing.T) {
		require.ErrorIs(t, err, completeErr)
	})
	t.Run("attempts publication once", func(t *testing.T) {
		require.Equal(t, 1, store.completeCalls)
	})
	t.Run("does not record a failure", func(t *testing.T) {
		require.Zero(t, store.failCalls)
	})
	t.Run("records a failed outcome", func(t *testing.T) {
		require.Contains(t, metrics.metrics, BatchMetric{Mode: "unknown", Name: MetricAPKGOutcome, Phase: "finalizing", State: "failed", ErrorClass: "terminal", Provider: "openai", Value: 1})
	})
}

func TestDurableFinalizerRecordsOutcomeMetricsByExecutionMode(t *testing.T) {
	store := &finalizerStoreStub{run: domain.PreparedDeckRun{State: domain.PreparedDeckRunFinalizing, ExecutionMode: domain.PreparedDeckExecutionStandard}}
	metrics := &recordingBatchMetrics{}
	_, err := (&DurableFinalizer{Store: store, Renderer: &finalizerRendererStub{}, Metrics: metrics}).Finalize(context.Background(), "owner", "preparation", "run", 0)
	require.NoError(t, err)
	t.Run("records a completed outcome for the execution mode", func(t *testing.T) {
		require.Contains(t, metrics.metrics, BatchMetric{Mode: "standard", Name: MetricAPKGOutcome, Phase: "finalizing", State: "completed", Provider: "openai", Value: 1})
	})
	t.Run("records latencies after the outcome", func(t *testing.T) {
		names := make([]string, 0, len(metrics.metrics))
		for _, metric := range metrics.metrics {
			names = append(names, metric.Name)
		}
		require.Equal(t, []string{MetricAPKGOutcome, MetricBatchPhaseLatency, MetricBatchTotalLatency}, names)
	})
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
		run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, RenderInputVersion: cardexport.RenderInputVersion, PresentationVersion: 1},
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
		run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, RenderInputVersion: cardexport.RenderInputVersion, PresentationVersion: 1},
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

func TestDurableRerendererRefusesLegacyRenderInputs(t *testing.T) {
	for _, version := range []int{0, cardexport.RenderInputVersion - 1} {
		t.Run(fmt.Sprintf("render inputs version %d", version), func(t *testing.T) {
			store := &finalizerStoreStub{
				run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, RenderInputVersion: version, PresentationVersion: 0},
				preparation: domain.DeckPreparation{State: domain.DeckPreparationReady, CurrentRunID: "run", DeckRevision: 1},
			}
			renderer := &finalizerRendererStub{}

			_, err := (&DurableRerenderer{Store: store, Renderer: renderer}).Rerender(context.Background(), "owner", "preparation", "run", cardexport.PresentationVersion)

			require.ErrorIs(t, err, ErrRequiresRepreparation)
			require.ErrorContains(t, err, fmt.Sprintf("render inputs version %d is below the required version %d", version, cardexport.RenderInputVersion))
			assert.Zero(t, store.loadCalls, "a refused rerender loads no manifest")
			assert.Zero(t, renderer.calls, "a refused rerender renders nothing")
			assert.Zero(t, store.supersedeCalls, "a refused rerender publishes nothing")
		})
	}
}

// TestDurableRerendererKeepsFrozenAttributionWithoutEvidenceCalls proves a
// presentation-only rebuild consumes the frozen specification: it reads no
// corpus/NLP evidence and keeps the frozen attribution and source lines.
func TestDurableRerendererKeepsFrozenAttributionWithoutEvidenceCalls(t *testing.T) {
	sentence := "Der Dichter\nspricht heute sehr laut."
	store := &finalizerStoreStub{
		run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, RenderInputVersion: cardexport.RenderInputVersion, PresentationVersion: 1},
		preparation: domain.DeckPreparation{State: domain.DeckPreparationReady, CurrentRunID: "run", DeckRevision: 1},
		projection: cardexport.ManifestSnapshot{
			SchemaVersion: cardexport.ManifestSchemaVersion, Owner: "owner", DeckName: "Book", Filename: cardexport.DownloadFilename("Book"),
			Items: []cardexport.ManifestItem{{
				Ordinal: 0, Disposition: cardexport.ManifestAccepted,
				Entry: cardexport.Entry{
					Language: "de", CanonicalLemma: "dichter", UPOS: "NOUN", Sentence: sentence, TargetWord: "Dichter",
					Attribution: domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Erster"},
				},
				Quality: cardexport.SentenceQuality{Accepted: true, Score: 90, Reasons: []string{"target present"}},
			}},
		},
	}
	renderer := &corpusRendererStub{}
	updated, err := (&DurableRerenderer{Store: store, Renderer: renderer}).Rerender(context.Background(), "owner", "preparation", "run", cardexport.PresentationVersion)
	require.NoError(t, err)
	assert.Equal(t, 2, updated.DeckRevision)
	require.Len(t, renderer.artifact.Generated, 1)
	assert.Equal(t, "Erster", renderer.artifact.Generated[0].Note.Attribution)
	assert.Contains(t, renderer.artifact.Generated[0].Note.Text, `<span class="source-line">`)
	assert.Contains(t, renderer.artifact.Generated[0].Note.Text, `<b>Dichter</b>`)
}

func TestRerenderWorkerMarksLegacyRenderInputsForRepreparation(t *testing.T) {
	store := &finalizerStoreStub{
		run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, RenderInputVersion: 0, PresentationVersion: 0},
		preparation: domain.DeckPreparation{State: domain.DeckPreparationReady, CurrentRunID: "run", DeckRevision: 1},
	}
	worker := &RerenderWorker{Rerenderer: &DurableRerenderer{Store: store, Renderer: &corpusRendererStub{}}}

	err := worker.Work(context.Background(), &river.Job[RerenderJobArgs]{Args: RerenderJobArgs{OwnerID: "owner", PreparationID: "preparation", RunID: "run", PresentationVersion: 1}})

	require.NoError(t, err, "a legacy deck is marked for repreparation, not retried")
	assert.Equal(t, 1, store.repreparationCalls)
	assert.Zero(t, store.supersedeCalls)
}

func TestRerenderWorkerMarksIdentityCorruptionForRepreparation(t *testing.T) {
	store := &finalizerStoreStub{
		run:         domain.PreparedDeckRun{State: domain.PreparedDeckRunCompleted, RenderInputVersion: cardexport.RenderInputVersion, PresentationVersion: 1},
		preparation: domain.DeckPreparation{State: domain.DeckPreparationReady, CurrentRunID: "run"},
		loadErr:     persistence.ErrPreparedDeckIdentity,
	}
	worker := &RerenderWorker{Rerenderer: &DurableRerenderer{Store: store, Renderer: &corpusRendererStub{}}}

	err := worker.Work(context.Background(), &river.Job[RerenderJobArgs]{Args: RerenderJobArgs{OwnerID: "owner", PreparationID: "preparation", RunID: "run", PresentationVersion: 2}})

	require.NoError(t, err)
	assert.Equal(t, 1, store.repreparationCalls)
}
