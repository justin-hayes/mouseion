//go:build integration

package prepareddeck

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type barrierTranslationProvider struct {
	mu       sync.Mutex
	calls    int
	inFlight int
	max      int
	barrier  int
	released chan struct{}
	attempts map[string]int
	onCall   func(int, enrichment.TranslationRequest) (enrichment.TranslationResponse, error)
	once     sync.Once
}

func (p *barrierTranslationProvider) Name() string    { return "integration-provider" }
func (p *barrierTranslationProvider) Version() string { return "1" }

func (p *barrierTranslationProvider) Translate(ctx context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.mu.Lock()
	p.calls++
	if p.attempts == nil {
		p.attempts = make(map[string]int)
	}
	p.attempts[request.CanonicalLemma]++
	attempt := p.attempts[request.CanonicalLemma]
	p.inFlight++
	if p.inFlight > p.max {
		p.max = p.inFlight
	}
	if p.barrier > 0 && p.inFlight >= p.barrier {
		p.once.Do(func() { close(p.released) })
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.inFlight--
		p.mu.Unlock()
	}()
	if p.released != nil {
		select {
		case <-p.released:
		case <-ctx.Done():
			return enrichment.TranslationResponse{}, ctx.Err()
		}
	}
	if p.onCall != nil {
		return p.onCall(attempt, request)
	}
	return validTranslationResponse(request), nil
}

func validTranslationResponse(request enrichment.TranslationRequest) enrichment.TranslationResponse {
	if request.RequireContextualGloss {
		return enrichment.TranslationResponse{Translation: request.CanonicalLemma + "-translated", Gloss: "contextual integration gloss", ContextOnly: true, SentenceTranslation: "The translated sentence.", SentenceTranslationTargets: []string{"translated"}}
	}
	return enrichment.TranslationResponse{
		Translation:                request.CanonicalLemma + "-translated",
		FallbackGloss:              "integration gloss",
		SentenceTranslation:        "The translated sentence.",
		SentenceTranslationTargets: []string{"translated"},
	}
}

func (p *barrierTranslationProvider) stats() (calls, maxInFlight int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls, p.max
}

type fixedStandardPlanner struct {
	params persistence.FreezePreparedDeckRunParams
}

type integrationFinalizeWorker struct {
	river.WorkerDefaults[FinalizeJobArgs]
}

func (*integrationFinalizeWorker) Work(context.Context, *river.Job[FinalizeJobArgs]) error {
	return nil
}

func (p fixedStandardPlanner) PlanPreparedDeckRun(context.Context, pgx.Tx, domain.DeckPreparation) (persistence.FreezePreparedDeckRunParams, error) {
	return p.params, nil
}

type standardIntegrationRun struct {
	store *persistence.PostgresStore
	owner string
	prep  domain.DeckPreparation
	run   domain.PreparedDeckRun
	keys  []enrichment.CacheKey
}

func newStandardIntegrationRun(t *testing.T, ctx context.Context, itemCount, maxAttempts int) (standardIntegrationRun, *river.Client[pgx.Tx], *barrierTranslationProvider) {
	return newStandardIntegrationRunWithExample(t, ctx, itemCount, maxAttempts, false)
}

func newStandardIntegrationRunWithExample(t *testing.T, ctx context.Context, itemCount, maxAttempts int, useUmhauenExample bool) (standardIntegrationRun, *river.Client[pgx.Tx], *barrierTranslationProvider) {
	t.Helper()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "standard-worker-"+uuid.NewString(), false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: "Integration", MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text"})
	require.NoError(t, err)
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename("Integration"), DeckName: "Integration", ContentHash: source.ContentHash})
	require.NoError(t, err)
	entries := make([]cardexport.Entry, itemCount)
	for i := range entries {
		lemma := "lemma-" + uuid.NewString()
		sentence, target := "Ein alter Satz steht heute im Buch.", "Ein"
		upos := "NOUN"
		if i == 0 && useUmhauenExample {
			lemma, sentence, target = "umhauen", "Und dann haut das Motorrad Piero um.", "haut"
			upos = "VERB"
		}
		entries[i] = cardexport.Entry{Language: "de", CanonicalLemma: lemma, UPOS: upos, Sentence: sentence, TargetWord: target, SourceDocument: "Integration", FirstEncounter: int64(i + 1)}
	}
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, "Integration", entries, testutil.PresentationProvider{Name: "integration-provider", Version: "1", TargetLanguage: "en", RequireContextualGloss: true})
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, itemCount)
	keys := make([]enrichment.CacheKey, len(work))
	for i, item := range work {
		keys[i] = item.CacheKey
	}
	provider := &barrierTranslationProvider{}
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 2}}, Workers: workers})
	require.NoError(t, err)
	AddStandardTranslationWorkerWithDependencies(workers, store, client, provider, PreparedDeckConfig{StandardRetryBaseDelay: time.Millisecond, StandardRetryMaxDelay: time.Millisecond}, time.Second)
	river.AddWorker(workers, &integrationFinalizeWorker{})
	config := persistence.PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ExecutionMode: string(domain.PreparedDeckExecutionStandard), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext), Provider: "integration-provider", ProviderVersion: "1", MaxProviderAttempts: maxAttempts}
	config.Endpoint = enrichment.OpenAIChatCompletionsEndpoint
	config.Model = "integration-model"
	planner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: uuid.NewString(), Projection: deck.StorageProjection(), Config: config}}
	coordinator := NewDurableCoordinator(store, client, planner)
	result, err := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: prep.ID})
	require.NoError(t, err)
	return standardIntegrationRun{store: store, owner: owner.ID, prep: prep, run: result.Run, keys: keys}, client, provider
}

func waitStandardOutcomes(t *testing.T, ctx context.Context, run standardIntegrationRun, want int) []domain.PreparedDeckTranslationOutcome {
	t.Helper()
	var outcomes []domain.PreparedDeckTranslationOutcome
	testutil.Eventually(t, testutil.DefaultWait, fmt.Sprintf("%d terminal translation outcomes", want), func() (bool, string) {
		current, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
		require.NoError(t, err)
		outcomes = current
		terminal := 0
		for _, outcome := range current {
			if outcome.State == domain.PreparedDeckOutcomeCompleted || outcome.State == domain.PreparedDeckOutcomeFailed {
				terminal++
			}
		}
		return terminal == want, fmt.Sprintf("%d of %d terminal", terminal, want)
	})
	return outcomes
}

func TestStandardRiverQueueBarrierBoundsProviderConcurrencyAndStoresExactCache(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 6, 1)
	provider.barrier = 2
	provider.released = make(chan struct{})
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "River client", client.Stop)
	outcomes := waitStandardOutcomes(t, ctx, run, len(run.keys))
	calls, maxInFlight := provider.stats()
	assert.Equal(t, len(run.keys), calls, "provider calls=%d max_in_flight=%d", calls, maxInFlight)
	assert.LessOrEqual(t, maxInFlight, 2, "provider max_in_flight")
	for i, outcome := range outcomes {
		assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcome.State)
		assert.Equal(t, 1, outcome.ProviderCallCount)
		entry, found, err := run.store.Get(ctx, run.keys[i])
		require.NoError(t, err)
		assert.True(t, found, "cache %d", i)
		assert.NotEmpty(t, entry.Translation, "cache %d", i)
		assert.Equal(t, "contextual integration gloss", entry.FallbackGloss, "cache %d", i)
		assert.Equal(t, []string{"translated"}, entry.SentenceTranslationTargets, "cache %d", i)
	}
	finalRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	status, err := run.store.GetDeckPreparationStatus(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.Equal(t, "assembling", status.Phase)
	assert.Equal(t, len(run.keys), status.TranslationDone)
	assert.Equal(t, len(run.keys), status.CardsWithEnglish)
	assert.Equal(t, len(run.keys), status.CardsWithContextualSentenceTranslations)
	frozen, stored, err := run.store.LoadPreparedDeckFinalization(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	deck, err := cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, stored, preparedDeckRunFacts(finalRun))
	require.NoError(t, err)
	assert.Len(t, artifact.Generated, len(run.keys))
	for _, generated := range artifact.Generated {
		assert.Equal(t, "contextual integration gloss", generated.Note.Gloss)
	}
}

func TestStandardWorkerPersistsAndRendersDiscontinuousTargetAlignment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRunWithExample(t, ctx, 1, 1, true)
	provider.onCall = func(_ int, _ enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		return enrichment.TranslationResponse{
			Translation: "knock over", Gloss: "knock down", ContextOnly: true,
			SentenceTranslation:        "And then the motorcycle knocks Piero over.",
			SentenceTranslationTargets: []string{"knocks", "over"},
		}, nil
	}
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "River client", client.Stop)
	outcomes := waitStandardOutcomes(t, ctx, run, 1)
	require.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State)
	entry, found, err := run.store.Get(ctx, run.keys[0])
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []string{"knocks", "over"}, entry.SentenceTranslationTargets)

	finalRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	frozen, stored, err := run.store.LoadPreparedDeckFinalization(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	deck, err := cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, stored, preparedDeckRunFacts(finalRun))
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 1)
	assert.Equal(t, "And then the motorcycle <b>knocks</b> Piero <b>over</b>.", artifact.Generated[0].Note.EnglishSentence)
	assert.Equal(t, "knock down", artifact.Generated[0].Note.Gloss)
}

func TestStandardWorkerCompletesFromFrozenCacheWithoutCallingProvider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	snapshot, _, err := run.store.LoadPreparedDeckStorageProjection(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
	require.NoError(t, err)
	work, ok := deck.WorkByOrdinal(0)
	require.True(t, ok)
	_, err = run.store.Put(ctx, enrichment.CacheEntry{
		CacheKey:                  work.CacheKey,
		Translation:               "cached translation",
		FallbackGloss:             "cached contextual gloss",
		SentenceTranslation:       "The cached sentence.",
		SentenceTranslationTarget: "cached",
		CachedAt:                  time.Now().UTC(),
	})
	require.NoError(t, err)

	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}
	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))

	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State)
	assert.Equal(t, 1, outcomes[0].CacheHitCount)
	calls, _ := provider.stats()
	assert.Zero(t, calls)

	finalRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	frozen, stored, err := run.store.LoadPreparedDeckFinalization(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	finalDeck, err := cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, finalDeck, stored, preparedDeckRunFacts(finalRun))
	require.NoError(t, err)
	assert.Len(t, artifact.Generated, 1)
	assert.Contains(t, artifact.TSV, "The <b>cached</b> sentence.")
	ready, err := (&DurableFinalizer{Store: run.store, Renderer: cardexport.NewPresentation(nil)}).Finalize(ctx, run.owner, run.prep.ID, run.run.ID, finalRun.FinalizationDispatchGeneration)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, ready.State)
	status, err := run.store.GetDeckPreparationStatus(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.True(t, status.ContextualGlossesReported)
	assert.Equal(t, 1, status.ContextualGlosses)
	assert.Equal(t, 1, status.ContextOnlyGlosses, "a cached result with no supported evidence must remain a context-only inference")
	var finalizerJobs int
	require.NoError(t, run.store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'run_id'=$2`, (FinalizeJobArgs{}).Kind(), run.run.ID).Scan(&finalizerJobs))
	assert.Equal(t, 1, finalizerJobs)
}

func TestStandardWorkerUsesRestoredFrozenRequestAndCacheKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	snapshot, _, err := run.store.LoadPreparedDeckStorageProjection(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
	require.NoError(t, err)
	expected, ok := deck.WorkByOrdinal(0)
	require.True(t, ok)
	var received enrichment.TranslationRequest
	provider.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		received = request
		return validTranslationResponse(request), nil
	}

	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}
	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))
	assert.Equal(t, expected.Request, received)
	stored, found, err := run.store.Get(ctx, expected.CacheKey)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, expected.CacheKey, stored.CacheKey)
}

func TestStandardWorkerDoesNotPersistAfterClaimLossDuringProviderCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	provider.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		_, err := run.store.Pool().Exec(ctx, `UPDATE deck_preparation_translation_outcomes SET claim_token=$1 WHERE owner_id=$2 AND preparation_id=$3 AND run_id=$4 AND ordinal=0 AND dispatch_generation=0`, uuid.NewString(), run.owner, run.prep.ID, run.run.ID)
		if err != nil {
			return enrichment.TranslationResponse{}, err
		}
		return validTranslationResponse(request), nil
	}
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}

	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))
	_, found, err := run.store.Get(ctx, run.keys[0])
	require.NoError(t, err)
	assert.False(t, found)
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeRunning, outcomes[0].State)
}

func TestStandardWorkerDoesNotCompleteWhenImmutableCacheWriteRemainsIncomplete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	snapshot, _, err := run.store.LoadPreparedDeckStorageProjection(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
	require.NoError(t, err)
	work, ok := deck.WorkByOrdinal(0)
	require.True(t, ok)
	_, err = run.store.Put(ctx, enrichment.CacheEntry{CacheKey: work.CacheKey, Translation: "partial", CachedAt: time.Now().UTC()})
	require.NoError(t, err)

	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}
	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeFailed, outcomes[0].State)
	assert.Equal(t, "cache", outcomes[0].ErrorClass)
	assert.Equal(t, "incomplete_entry", outcomes[0].ErrorCode)
	stored, found, err := run.store.Get(ctx, work.CacheKey)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "partial", stored.Translation)
	assert.Empty(t, stored.SentenceTranslation)
	calls, _ := provider.stats()
	assert.Equal(t, 1, calls)
}

func TestStandardWorkerTreatsLostClaimAsNoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	_, err := run.store.ClaimPreparedDeckTranslationOutcome(ctx, run.owner, run.prep.ID, run.run.ID, 0, 0, uuid.NewString(), time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}

	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))
	calls, _ := provider.stats()
	assert.Zero(t, calls)
}

func TestRecoveryRedispatchesExpiredStandardClaimWithNewGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, _ := newStandardIntegrationRun(t, ctx, 1, 1)
	initial, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, initial, 1)
	initialJobID := initial[0].RiverJobID
	_, err = run.store.ClaimPreparedDeckTranslationOutcome(ctx, run.owner, run.prep.ID, run.run.ID, 0, 0, uuid.NewString(), time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	_, err = run.store.Pool().Exec(ctx, `UPDATE deck_preparation_translation_outcomes SET claimed_at=now() - interval '2 seconds', lease_expires_at=now() - interval '1 second' WHERE owner_id=$1 AND preparation_id=$2 AND run_id=$3 AND ordinal=0`, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)

	recovery := &RecoveryWorker{Store: run.store, Client: client}
	require.NoError(t, recovery.repair(ctx, domain.PreparedDeckRecoveryWork{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0, Kind: "outcome", LeaseExpired: true}))
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomePending, outcomes[0].State)
	assert.Equal(t, 1, outcomes[0].DispatchGeneration)
	assert.NotZero(t, outcomes[0].RiverJobID)
	assert.NotEqual(t, initialJobID, outcomes[0].RiverJobID)
}

func TestStandardWorkerPersistsRetryGenerationAndTerminalValidationFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 3, 2)
	provider.onCall = func(call int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		switch request.CanonicalLemma {
		case run.keys[0].CanonicalLemma:
			if call == 1 {
				return enrichment.TranslationResponse{}, &enrichment.LLMHTTPError{StatusCode: 429}
			}
			return validTranslationResponse(request), nil
		case run.keys[1].CanonicalLemma:
			return enrichment.TranslationResponse{}, nil
		default:
			return enrichment.TranslationResponse{}, &enrichment.LLMHTTPError{StatusCode: 401}
		}
	}
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider, Config: PreparedDeckConfig{StandardRetryBaseDelay: time.Millisecond, StandardRetryMaxDelay: time.Millisecond}, AttemptTimeout: time.Second, Jitter: func(delay time.Duration) time.Duration { return delay }}
	for i := range run.keys {
		require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: i, Generation: 0}), "initial ordinal %d", i)
	}
	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 1}), "retry ordinal 0")
	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 1, Generation: 1}), "retry ordinal 1")
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State)
	assert.Equal(t, 1, outcomes[0].DispatchGeneration)
	assert.Equal(t, 2, outcomes[0].ProviderAttemptCount)
	assert.Equal(t, domain.PreparedDeckOutcomeFailed, outcomes[1].State)
	assert.Equal(t, "invalid_response", outcomes[1].ErrorCode)
	assert.Equal(t, 1, outcomes[1].DispatchGeneration)
	assert.Equal(t, 2, outcomes[1].ProviderAttemptCount)
	assert.Equal(t, domain.PreparedDeckOutcomeFailed, outcomes[2].State)
	assert.Equal(t, "http_401", outcomes[2].ErrorCode)
	assert.Equal(t, 1, outcomes[2].ProviderAttemptCount)
	_, found, err := run.store.Get(ctx, run.keys[1])
	require.NoError(t, err)
	assert.False(t, found, "malformed response cache")
	calls, _ := provider.stats()
	assert.Equal(t, 5, calls, "provider calls, want one retry for the transient error and one bounded retry for the malformed response")
}

func TestStandardWorkerPersistsValidUnresolvedMeaningAndFinalizesMixedDeck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 2, 1)
	_, err := run.store.PutKnownVocabulary(ctx, run.owner, "de", "known-before-preparation", "NOUN")
	require.NoError(t, err)
	knownBefore, err := run.store.ListKnownVocabulary(ctx, run.owner, "de")
	require.NoError(t, err)
	reservedBefore, err := run.store.ListReservedVocabulary(ctx, run.owner, "de")
	require.NoError(t, err)
	var snapshotRowsBefore, completionRowsBefore, dispositionRowsBefore []byte
	require.NoError(t, run.store.Pool().QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(v) ORDER BY v.snapshot_id, v.language, v.canonical_lemma, v.upos), '[]'::jsonb) FROM primary_goal_snapshot_vocabulary v WHERE owner_id=$1`, run.owner).Scan(&snapshotRowsBefore))
	require.NoError(t, run.store.Pool().QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY r.completion_id), '[]'::jsonb) FROM reading_history r WHERE owner_id=$1`, run.owner).Scan(&completionRowsBefore))
	require.NoError(t, run.store.Pool().QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(d) ORDER BY d.book_id), '[]'::jsonb) FROM book_dispositions d WHERE owner_id=$1`, run.owner).Scan(&dispositionRowsBefore))
	provider.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		if request.CanonicalLemma == run.keys[0].CanonicalLemma {
			return enrichment.TranslationResponse{
				Translation: request.CanonicalLemma + "-translated", SentenceTranslation: "The translated sentence.",
				ContextOnly: true, UnresolvedReason: "The sentence does not distinguish the meanings.",
			}, nil
		}
		return validTranslationResponse(request), nil
	}
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider, Config: PreparedDeckConfig{StandardRetryBaseDelay: time.Millisecond, StandardRetryMaxDelay: time.Millisecond}, AttemptTimeout: time.Second, Jitter: func(delay time.Duration) time.Duration { return delay }}
	for ordinal := range run.keys {
		require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: ordinal, Generation: 0}))
	}
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 2)
	assert.Equal(t, "The sentence does not distinguish the meanings.", outcomes[0].OmissionReason)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State)
	assert.Equal(t, 1, outcomes[0].ProviderAttemptCount, "a valid unresolved model response is still a provider attempt")
	assert.Equal(t, 1, outcomes[0].ProviderCallCount)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[1].State)
	_, found, err := run.store.Get(ctx, run.keys[0])
	require.NoError(t, err)
	assert.False(t, found, "unresolved meaning must not enter the shared translation cache")

	finalizer := &DurableFinalizer{Store: run.store, Renderer: cardexport.NewPresentation(nil)}
	currentRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PreparedDeckRunFinalizing, currentRun.State)
	assert.Equal(t, domain.PreparedDeckTranslationCompleted, currentRun.TranslationState)
	preparing, err := run.store.GetDeckPreparation(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationPreparing, preparing.State)
	ready, err := finalizer.Finalize(ctx, run.owner, run.prep.ID, run.run.ID, currentRun.FinalizationDispatchGeneration)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, ready.State)
	assert.Equal(t, 1, ready.TotalCards)
	var generated int
	require.NoError(t, run.store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1`, run.owner).Scan(&generated))
	assert.Equal(t, 1, generated, "only the rendered card should receive Generated vocabulary provenance")
	var omittedGenerated int
	require.NoError(t, run.store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma=$2`, run.owner, run.keys[0].CanonicalLemma).Scan(&omittedGenerated))
	assert.Zero(t, omittedGenerated, "unresolved target must not receive Generated vocabulary provenance")
	knownAfter, err := run.store.ListKnownVocabulary(ctx, run.owner, "de")
	require.NoError(t, err)
	assert.Equal(t, knownBefore, knownAfter, "deck preparation must not change Known vocabulary")
	reservedAfter, err := run.store.ListReservedVocabulary(ctx, run.owner, "de")
	require.NoError(t, err)
	assert.Equal(t, reservedBefore, reservedAfter, "deck preparation must not change Reserved vocabulary")
	var snapshotRowsAfter, completionRowsAfter, dispositionRowsAfter []byte
	require.NoError(t, run.store.Pool().QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(v) ORDER BY v.snapshot_id, v.language, v.canonical_lemma, v.upos), '[]'::jsonb) FROM primary_goal_snapshot_vocabulary v WHERE owner_id=$1`, run.owner).Scan(&snapshotRowsAfter))
	require.NoError(t, run.store.Pool().QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY r.completion_id), '[]'::jsonb) FROM reading_history r WHERE owner_id=$1`, run.owner).Scan(&completionRowsAfter))
	require.NoError(t, run.store.Pool().QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(d) ORDER BY d.book_id), '[]'::jsonb) FROM book_dispositions d WHERE owner_id=$1`, run.owner).Scan(&dispositionRowsAfter))
	assert.JSONEq(t, string(snapshotRowsBefore), string(snapshotRowsAfter), "deck preparation must not mutate a frozen reading-vocabulary snapshot")
	assert.JSONEq(t, string(completionRowsBefore), string(completionRowsAfter), "deck preparation must not create or alter completion choices")
	assert.JSONEq(t, string(dispositionRowsBefore), string(dispositionRowsAfter), "deck preparation must not change Reading dispositions")
	status, err := run.store.GetDeckPreparationStatus(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.Equal(t, []domain.DeckPreparationMeaningOmission{{TargetWord: "Ein", Reason: "The sentence does not distinguish the meanings."}}, status.MeaningOmissions)
}

func TestStandardWorkerAllUnresolvedFailsPreparationWithInspectableReason(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	provider.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		return enrichment.TranslationResponse{Translation: "translated", SentenceTranslation: "The sentence.", ContextOnly: true, UnresolvedReason: "The sentence is too ambiguous."}, nil
	}
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider, AttemptTimeout: time.Second}
	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))
	currentRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	_, err = (&DurableFinalizer{Store: run.store, Renderer: cardexport.NewPresentation(nil)}).Finalize(ctx, run.owner, run.prep.ID, run.run.ID, currentRun.FinalizationDispatchGeneration)
	require.ErrorIs(t, err, cardexport.ErrAllMeaningsUnresolved)
	assert.NotContains(t, err.Error(), "record finalization failure")
	status, err := run.store.GetDeckPreparationStatus(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, status.State)
	assert.Equal(t, []domain.DeckPreparationMeaningOmission{{TargetWord: "Ein", Reason: "The sentence is too ambiguous."}}, status.MeaningOmissions)
	assert.Empty(t, status.Artifact)
}

func TestStandardWorkerUnknownEvidenceRetriesAndNeverBecomesAnOmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 2)
	provider.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		return enrichment.TranslationResponse{
			Translation: "translated", SentenceTranslation: "The sentence.", Gloss: "unsupported gloss",
			EvidenceIDs: []string{"wikt:unknown"}, ContextOnly: false,
		}, nil
	}
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider, Config: PreparedDeckConfig{StandardRetryBaseDelay: time.Millisecond, StandardRetryMaxDelay: time.Millisecond}, AttemptTimeout: time.Second, Jitter: func(delay time.Duration) time.Duration { return delay }}
	args := StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}
	require.NoError(t, worker.execute(ctx, args))
	args.Generation = 1
	require.NoError(t, worker.execute(ctx, args))
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeFailed, outcomes[0].State)
	assert.Equal(t, "validation", outcomes[0].ErrorClass)
	assert.Equal(t, "invalid_response", outcomes[0].ErrorCode)
	assert.Empty(t, outcomes[0].OmissionReason)
	assert.Equal(t, 2, outcomes[0].ProviderAttemptCount)
	_, found, err := run.store.Get(ctx, run.keys[0])
	require.NoError(t, err)
	assert.False(t, found, "unknown evidence must not become a cached dictionary-only result")
	preparation, err := run.store.GetDeckPreparation(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, preparation.State)
	assert.Empty(t, preparation.Artifact)
}

func TestStandardRiverRetriesMalformedResponseUntilProviderBudgetIsExhausted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 2)
	provider.onCall = func(_ int, _ enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		return enrichment.TranslationResponse{}, nil
	}
	startedAt := time.Now().UTC()
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "River client", client.Stop)

	outcomes := waitStandardOutcomes(t, ctx, run, 1)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeFailed, outcomes[0].State)
	assert.Equal(t, "validation", outcomes[0].ErrorClass)
	assert.Equal(t, "invalid_response", outcomes[0].ErrorCode)
	assert.Equal(t, 2, outcomes[0].ProviderAttemptCount)
	assert.Equal(t, 2, outcomes[0].DispatchCount)
	assert.Equal(t, 1, outcomes[0].DispatchGeneration)
	assert.True(t, outcomes[0].NextAttemptAt.After(startedAt), "retry backoff timestamp was not persisted")
	calls, _ := provider.stats()
	assert.Equal(t, 2, calls)

	storedRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PreparedDeckRunFailed, storedRun.State)
	preparation, err := run.store.GetDeckPreparation(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, preparation.State)
	assert.Empty(t, preparation.Artifact)
	_, found, err := run.store.Get(ctx, run.keys[0])
	require.NoError(t, err)
	assert.False(t, found, "malformed response was persisted as a cache success")
}

func TestStandardWorkerRestartRestoresPendingProjection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	riverJobID := outcomes[0].RiverJobID
	_, err = run.store.Pool().Exec(ctx, `UPDATE river_job SET scheduled_at=now() + interval '1 hour' WHERE id=$1`, riverJobID)
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	require.NoError(t, client.Stop(ctx))
	_, err = run.store.Pool().Exec(ctx, `UPDATE river_job SET scheduled_at=now() WHERE id=$1`, riverJobID)
	require.NoError(t, err)

	workers := river.NewWorkers()
	restarted, err := river.NewClient(riverpgxv5.New(run.store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	AddStandardTranslationWorkerWithDependencies(workers, run.store, restarted, provider, PreparedDeckConfig{}, time.Second)
	river.AddWorker(workers, &integrationFinalizeWorker{})
	require.NoError(t, restarted.Start(ctx))
	testutil.StopOnCleanup(t, "restarted River client", restarted.Stop)
	outcomes = waitStandardOutcomes(t, ctx, run, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State)
	calls, _ := provider.stats()
	assert.Equal(t, 1, calls)
	testutil.Eventually(t, testutil.DefaultWait, "restarted River job to complete", func() (bool, string) {
		job, err := restarted.JobGet(ctx, riverJobID)
		require.NoError(t, err)
		return job.State == rivertype.JobStateCompleted, fmt.Sprintf("state %s", job.State)
	})
}

func TestStandardWorkerRestartSkipsCompletedOutcome(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	require.NoError(t, client.Start(ctx))
	waitStandardOutcomes(t, ctx, run, 1)
	require.NoError(t, client.Stop(ctx))
	callsBefore, _ := provider.stats()

	workers := river.NewWorkers()
	restarted, err := river.NewClient(riverpgxv5.New(run.store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	AddStandardTranslationWorkerWithDependencies(workers, run.store, restarted, provider, PreparedDeckConfig{}, time.Second)
	river.AddWorker(workers, &integrationFinalizeWorker{})
	tx, err := run.store.Pool().Begin(ctx)
	require.NoError(t, err)
	inserted, err := restarted.InsertTx(ctx, tx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}, &river.InsertOpts{Queue: TranslationQueue})
	require.NoError(t, err)
	require.NotNil(t, inserted)
	require.NotNil(t, inserted.Job)
	require.NoError(t, tx.Commit(ctx))
	require.NoError(t, restarted.Start(ctx))
	testutil.StopOnCleanup(t, "restarted River client", restarted.Stop)
	testutil.Eventually(t, testutil.DefaultWait, "restarted River job to complete", func() (bool, string) {
		job, err := restarted.JobGet(ctx, inserted.Job.ID)
		require.NoError(t, err)
		return job.State == rivertype.JobStateCompleted, fmt.Sprintf("state %s", job.State)
	})
	callsAfter, _ := provider.stats()
	assert.Equal(t, callsBefore, callsAfter)
}
