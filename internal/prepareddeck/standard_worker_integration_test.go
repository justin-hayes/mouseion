//go:build integration

package prepareddeck

import (
	"context"
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
	mu              sync.Mutex
	calls           int
	inFlight        int
	max             int
	barrier         int
	released        chan struct{}
	attempts        map[string]int
	onCall          func(int, enrichment.TranslationRequest) (enrichment.TranslationResponse, error)
	once            sync.Once
	providerName    string
	providerVersion string
}

func (p *barrierTranslationProvider) Name() string {
	if p.providerName != "" {
		return p.providerName
	}
	return "integration-provider"
}
func (p *barrierTranslationProvider) Version() string {
	if p.providerVersion != "" {
		return p.providerVersion
	}
	return "1"
}

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
	return enrichment.TranslationResponse{
		Translation:               request.CanonicalLemma + "-translated",
		FallbackGloss:             "integration gloss",
		SentenceTranslation:       "The translated sentence.",
		SentenceTranslationTarget: "translated",
	}
}

func (p *barrierTranslationProvider) stats() (calls, max int) {
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

func (p fixedStandardPlanner) PlanPreparedDeckRun(context.Context, pgx.Tx, domain.DeckPreparation, bool) (persistence.FreezePreparedDeckRunParams, error) {
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
	t.Helper()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	owner, err := store.CreateUser(ctx, "standard-worker-"+uuid.NewString(), false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: "Integration", MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text"})
	require.NoError(t, err)
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename("Integration"), DeckName: "Integration", ContentHash: source.ContentHash})
	require.NoError(t, err)
	keys := make([]enrichment.CacheKey, itemCount)
	items := make([]cardexport.ManifestItem, itemCount)
	for i := range items {
		lemma := "lemma-" + uuid.NewString()
		keys[i] = enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: lemma, UPOS: "NOUN", Provider: "integration-provider", ProviderVersion: "1", SentenceHash: enrichment.SentenceHash("Ein Satz.")}
		items[i] = cardexport.ManifestItem{Ordinal: i, Disposition: cardexport.ManifestAccepted, Entry: cardexport.Entry{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN", Sentence: "Ein Satz.", TargetWord: "Ein", SourceDocument: "Integration", FirstEncounter: int64(i + 1)}, Quality: cardexport.SentenceQuality{Accepted: true, Reasons: []string{}}, CacheKey: &keys[i]}
	}
	manifest := cardexport.ManifestSnapshot{SchemaVersion: cardexport.ManifestSchemaVersion, Owner: owner.ID, DeckName: "Integration", Filename: cardexport.DownloadFilename("Integration"), Items: items}
	provider := &barrierTranslationProvider{}
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 2}}, Workers: workers})
	require.NoError(t, err)
	AddStandardTranslationWorkerWithDependencies(workers, store, client, provider, PreparedDeckConfig{StandardRetryBaseDelay: time.Millisecond, StandardRetryMaxDelay: time.Millisecond}, time.Second)
	river.AddWorker(workers, &integrationFinalizeWorker{})
	config := persistence.PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ExecutionMode: string(domain.PreparedDeckExecutionStandard), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext), Provider: "integration-provider", ProviderVersion: "1", MaxProviderAttempts: maxAttempts}
	config.Endpoint = enrichment.OpenAIChatCompletionsEndpoint
	config.Model = "integration-model"
	planner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: uuid.NewString(), Projection: manifest, Config: config}}
	coordinator := NewDurableCoordinator(store, client, planner)
	result, err := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: prep.ID, ExternalTranslationConsent: true})
	require.NoError(t, err)
	return standardIntegrationRun{store: store, owner: owner.ID, prep: prep, run: result.Run, keys: keys}, client, provider
}

func waitStandardOutcomes(t *testing.T, ctx context.Context, run standardIntegrationRun, want int) []domain.PreparedDeckTranslationOutcome {
	t.Helper()
	for {
		outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
		require.NoError(t, err)
		terminal := 0
		for _, outcome := range outcomes {
			if outcome.State == domain.PreparedDeckOutcomeCompleted || outcome.State == domain.PreparedDeckOutcomeFailed {
				terminal++
			}
		}
		if terminal == want {
			return outcomes
		}
		select {
		case <-ctx.Done():
			require.Fail(t, ctx.Err().Error())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestStandardRiverQueueBarrierBoundsProviderConcurrencyAndStoresExactCache(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 6, 1)
	provider.barrier = 2
	provider.released = make(chan struct{})
	require.NoError(t, client.Start(ctx))
	defer client.Stop(context.Background())
	outcomes := waitStandardOutcomes(t, ctx, run, len(run.keys))
	calls, max := provider.stats()
	assert.Equal(t, len(run.keys), calls, "provider calls=%d max_in_flight=%d", calls, max)
	assert.LessOrEqual(t, max, 2, "provider max_in_flight")
	for i, outcome := range outcomes {
		assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcome.State)
		assert.Equal(t, 1, outcome.ProviderCallCount)
		entry, found, err := run.store.Get(ctx, run.keys[i])
		require.NoError(t, err)
		assert.True(t, found, "cache %d", i)
		assert.NotEmpty(t, entry.Translation, "cache %d", i)
	}
	finalRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	frozen, stored, err := run.store.LoadPreparedDeckFinalization(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	deck, err := cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, stored, preparedDeckRunFacts(finalRun))
	require.NoError(t, err)
	assert.Len(t, artifact.Generated, len(run.keys))
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

func TestStandardWorkerRejectsProviderIdentityDrift(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	provider.providerName = "different-provider"
	worker := &StandardTranslationWorker{Store: run.store, Client: client, Provider: provider}

	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}))
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeFailed, outcomes[0].State)
	assert.Equal(t, "identity", outcomes[0].ErrorClass)
	assert.Equal(t, "provider", outcomes[0].ErrorCode)
	calls, _ := provider.stats()
	assert.Zero(t, calls)
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

func TestStandardRiverRetriesMalformedResponseUntilProviderBudgetIsExhausted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 2)
	provider.onCall = func(_ int, _ enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		return enrichment.TranslationResponse{}, nil
	}
	startedAt := time.Now().UTC()
	require.NoError(t, client.Start(ctx))
	defer client.Stop(context.Background())

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
	defer restarted.Stop(context.Background())
	outcomes = waitStandardOutcomes(t, ctx, run, 1)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State)
	calls, _ := provider.stats()
	assert.Equal(t, 1, calls)
	for {
		job, err := restarted.JobGet(ctx, riverJobID)
		require.NoError(t, err)
		if job.State == rivertype.JobStateCompleted {
			break
		}
		select {
		case <-ctx.Done():
			require.FailNow(t, "restarted River job did not complete", "state=%s", job.State)
		case <-time.After(10 * time.Millisecond):
		}
	}
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
	defer restarted.Stop(context.Background())
	for {
		job, err := restarted.JobGet(ctx, inserted.Job.ID)
		require.NoError(t, err)
		if job.State == rivertype.JobStateCompleted {
			break
		}
		select {
		case <-ctx.Done():
			require.FailNow(t, "replayed River job did not complete", "state=%s", job.State)
		case <-time.After(10 * time.Millisecond):
		}
	}
	callsAfter, _ := provider.stats()
	assert.Equal(t, callsBefore, callsAfter)
}
