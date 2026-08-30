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
	return enrichment.TranslationResponse{
		Translation:               request.CanonicalLemma + "-translated",
		Gloss:                     "integration gloss",
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
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	owner, err := store.CreateUser(ctx, "standard-worker-"+uuid.NewString(), false)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: "Integration", MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text"})
	if err != nil {
		t.Fatal(err)
	}
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename("Integration"), DeckName: "Integration", ContentHash: source.ContentHash})
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]enrichment.CacheKey, itemCount)
	items := make([]cardexport.ManifestItem, itemCount)
	for i := range items {
		lemma := "lemma-" + uuid.NewString()
		keys[i] = enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: lemma, UPOS: "NOUN", Provider: "integration-provider", ProviderVersion: "1", SentenceHash: enrichment.SentenceHash("Ein Satz.")}
		items[i] = cardexport.ManifestItem{Ordinal: i, Disposition: cardexport.ManifestAccepted, Entry: cardexport.Entry{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN", Sentence: "Ein Satz.", TargetWord: lemma, SourceDocument: "Integration", FirstEncounter: int64(i + 1)}, Quality: cardexport.SentenceQuality{Accepted: true, Reasons: []string{}}, CacheKey: &keys[i]}
	}
	manifest := cardexport.ManifestSnapshot{SchemaVersion: cardexport.ManifestSchemaVersion, Owner: owner.ID, DeckName: "Integration", Filename: cardexport.DownloadFilename("Integration"), Items: items}
	provider := &barrierTranslationProvider{}
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 2}}, Workers: workers})
	if err != nil {
		t.Fatal(err)
	}
	AddStandardTranslationWorkerWithDependencies(workers, store, client, provider, PreparedDeckConfig{StandardRetryBaseDelay: time.Millisecond, StandardRetryMaxDelay: time.Millisecond}, time.Second)
	river.AddWorker(workers, &integrationFinalizeWorker{})
	config := persistence.PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ExecutionMode: string(domain.PreparedDeckExecutionStandard), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext), Provider: "integration-provider", ProviderVersion: "1", MaxProviderAttempts: maxAttempts}
	config.Endpoint = enrichment.OpenAIChatCompletionsEndpoint
	config.Model = "integration-model"
	planner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: uuid.NewString(), Manifest: manifest, Config: config}}
	coordinator := NewDurableCoordinator(store, client, planner)
	result, err := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: prep.ID, ExternalTranslationConsent: true})
	if err != nil {
		t.Fatal(err)
	}
	return standardIntegrationRun{store: store, owner: owner.ID, prep: prep, run: result.Run, keys: keys}, client, provider
}

func waitStandardOutcomes(t *testing.T, ctx context.Context, run standardIntegrationRun, want int) []domain.PreparedDeckTranslationOutcome {
	t.Helper()
	for {
		outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
		if err != nil {
			t.Fatal(err)
		}
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
			t.Fatal(ctx.Err())
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
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	outcomes := waitStandardOutcomes(t, ctx, run, len(run.keys))
	calls, max := provider.stats()
	if calls != len(run.keys) || max > 2 {
		t.Fatalf("provider calls=%d max_in_flight=%d, want calls=%d and max <= 2", calls, max, len(run.keys))
	}
	for i, outcome := range outcomes {
		if outcome.State != domain.PreparedDeckOutcomeCompleted || outcome.ProviderCallCount != 1 {
			t.Fatalf("outcome %d=%+v", i, outcome)
		}
		entry, found, err := run.store.Get(ctx, run.keys[i])
		if err != nil || !found || entry.Translation == "" {
			t.Fatalf("cache %d found=%v entry=%+v err=%v", i, found, entry, err)
		}
	}
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
		if err := worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: i, Generation: 0}); err != nil {
			t.Fatalf("initial ordinal %d: %v", i, err)
		}
	}
	if err := worker.execute(ctx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 1}); err != nil {
		t.Fatalf("retry ordinal: %v", err)
	}
	outcomes, err := run.store.ListPreparedDeckTranslationOutcomes(ctx, run.owner, run.prep.ID, run.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if outcomes[0].State != domain.PreparedDeckOutcomeCompleted || outcomes[0].DispatchGeneration != 1 || outcomes[0].ProviderAttemptCount != 2 {
		t.Fatalf("retry outcome=%+v", outcomes[0])
	}
	if outcomes[1].State != domain.PreparedDeckOutcomeFailed || outcomes[1].ErrorCode != "invalid_response" {
		t.Fatalf("malformed outcome=%+v", outcomes[1])
	}
	if outcomes[2].State != domain.PreparedDeckOutcomeFailed || outcomes[2].ErrorCode != "http_401" || outcomes[2].ProviderAttemptCount != 1 {
		t.Fatalf("terminal outcome=%+v", outcomes[2])
	}
	if _, found, err := run.store.Get(ctx, run.keys[1]); err != nil || found {
		t.Fatalf("malformed response cache found=%v err=%v", found, err)
	}
	calls, _ := provider.stats()
	if calls != 4 {
		t.Fatalf("provider calls=%d, want one retry plus two terminal calls", calls)
	}
}

func TestStandardWorkerRestartSkipsCompletedOutcome(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, client, provider := newStandardIntegrationRun(t, ctx, 1, 1)
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitStandardOutcomes(t, ctx, run, 1)
	client.Stop(context.Background())
	callsBefore, _ := provider.stats()

	workers := river.NewWorkers()
	restarted, err := river.NewClient(riverpgxv5.New(run.store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 1}}, Workers: workers})
	if err != nil {
		t.Fatal(err)
	}
	AddStandardTranslationWorkerWithDependencies(workers, run.store, restarted, provider, PreparedDeckConfig{}, time.Second)
	river.AddWorker(workers, &integrationFinalizeWorker{})
	tx, err := run.store.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = restarted.InsertTx(ctx, tx, StandardTranslationJobArgs{OwnerID: run.owner, PreparationID: run.prep.ID, RunID: run.run.ID, Ordinal: 0, Generation: 0}, &river.InsertOpts{Queue: TranslationQueue})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = restarted.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer restarted.Stop(context.Background())
	time.Sleep(300 * time.Millisecond)
	callsAfter, _ := provider.stats()
	if callsAfter != callsBefore {
		t.Fatalf("completed outcome caused provider call after restart: before=%d after=%d", callsBefore, callsAfter)
	}
}
