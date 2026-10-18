//go:build integration

package webapp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const greekPassSecret = "ancient-greek-pass-integration-secret-0123456789"

// greekPassProvider is a fake configured provider for the Ancient Greek
// evidence-assisted pass. It records every request and answers each one through
// the supplied responder.
type greekPassProvider struct {
	mu        sync.Mutex
	calls     int
	requests  []enrichment.TranslationRequest
	responder func(enrichment.TranslationRequest) (enrichment.TranslationResponse, error)
}

func (p *greekPassProvider) Name() string    { return "openai-compatible" }
func (p *greekPassProvider) Version() string { return "greek-pass-test" }
func (p *greekPassProvider) Enabled() bool   { return true }

func (p *greekPassProvider) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.mu.Lock()
	p.calls++
	p.requests = append(p.requests, request)
	p.mu.Unlock()
	return p.responder(request)
}

// greekPassFixture is one prepared Book 1 deck whose freeze worker is created
// but not yet started, so a test can set the deck specification's pass flag
// before the run is frozen.
type greekPassFixture struct {
	store    *persistence.PostgresStore
	pool     *pgxpool.Pool
	learner  domain.User
	bookID   string
	codec    *enrichment.TranslationCodec
	provider *greekPassProvider
	deck     *prepareddeck.Service
	handler  http.Handler
	cookies  []*http.Cookie
	csrf     string
	client   *river.Client[pgx.Tx]
}

func newGreekPassFixture(t *testing.T, ctx context.Context, username string, responder func(enrichment.TranslationRequest) (enrichment.TranslationResponse, error)) *greekPassFixture {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", greekPassSecret)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	learner := createAccount(t, ctx, store, username, "learner-password")
	registration, err := ancientgreek.NewService(store).Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)
	importGreekFixtureWithDefinition(t, ctx, store, learner.ID, registration.BookID, "oga-src-deck.conllu", ancientgreek.AnabasisBook1)
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, registration.BookID, domain.BookDispositionToRead))
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))

	provider := &greekPassProvider{responder: responder}
	codec := greekPassCodec(t)
	client := newGreekPassClient(t, ctx, store, pool, codec, provider)
	deck := prepareddeck.NewService(store, client)
	webAuth := auth.New(store, time.Hour)
	h := New(Services{
		Auth: webAuth, WebAuth: webauth.New(webAuth, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), client), PreparedDeck: deck,
		Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	return &greekPassFixture{
		store: store, pool: pool, learner: learner, bookID: registration.BookID,
		codec: codec, provider: provider, deck: deck, handler: h, cookies: cookies, csrf: csrf, client: client,
	}
}

// startReadingAndDeck starts the reading, creates the deck specification and,
// when pass is set, opts into the pass before the freeze worker starts. It
// returns the ready preparation.
func (f *greekPassFixture) startReadingAndDeck(t *testing.T, ctx context.Context, pass bool) domain.DeckPreparation {
	t.Helper()
	started := perform(t, f.handler, http.MethodPost, "/reading/books/"+f.bookID+"/start", url.Values{"csrf_token": {f.csrf}}, f.cookies)
	require.Equal(t, http.StatusSeeOther, started.Code, "start body=%s", started.Body.String())
	reading, err := f.store.GetCurrentReading(ctx, f.learner.ID, "grc")
	require.NoError(t, err)
	require.Equal(t, 3, reading.SnapshotSize, "δέω 1, δέω 2 and λόγος are the recurring Book vocabulary")

	handle, err := f.deck.PrepareCurrentReadingDeck(ctx, f.learner.ID, f.bookID, reading.SnapshotID)
	require.NoError(t, err)
	if pass {
		// The deck specification is created with the pass off, then opts in
		// before the freeze worker starts. This is the only seam that sets the
		// flag today: the app never offers it while the payload assessment is
		// unrecorded.
		_, err = f.pool.Exec(ctx, `UPDATE deck_preparations SET evidence_assisted_provider_pass=true WHERE owner_id=$1 AND id=$2`, f.learner.ID, handle.Preparation.ID)
		require.NoError(t, err)
	}
	require.NoError(t, f.client.Start(ctx))

	preparation := waitForPreparedDeck(t, ctx, f.store, f.learner.ID, reading.SnapshotID)
	require.Equal(t, domain.DeckPreparationReady, preparation.State, "preparation error=%q", preparation.Error)
	return preparation
}

// TestAncientGreekEvidenceAssistedPassAddsLabelledProviderFields proves the
// opt-in pass sends the representative sentence, its passage context and the
// pinned LSJ senses, labels the provider-proposed fields it adds and leaves a
// homograph whose answer abstained as the default card, with no meaning or
// provider omissions.
func TestAncientGreekEvidenceAssistedPassAddsLabelledProviderFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newGreekPassFixture(t, ctx, "greek-pass-learner", greekPassResolvedResponse)
	preparation := fixture.startReadingAndDeck(t, ctx, true)

	assert.Equal(t, 3, preparation.TotalCards)

	// The pass asks the provider once for every accepted item, and each request
	// carries the representative sentence, its passage context and the pinned
	// LSJ senses with their native IDs.
	assert.Equal(t, 3, fixture.provider.calls, "the pass makes one request per accepted item")
	logos := 0
	for _, request := range fixture.provider.requests {
		assert.True(t, request.RequireEvidenceAssistedPass)
		assert.False(t, request.RequireContextualGloss)
		if request.CanonicalLemma != "λόγοσ" {
			assert.Empty(t, request.CandidateSenses, "an ambiguous homograph has no pinned LSJ evidence")
			continue
		}
		logos++
		require.NotEmpty(t, request.CandidateSenses)
		assert.Equal(t, "lsj-logos.0", request.CandidateSenses[0].EvidenceID, "the LSJ sense's native ID is the pass evidence ID")
	}

	// Exactly one card added provider-proposed fields; the abstaining homograph
	// cards keep their default card.
	assert.Equal(t, 1, logos)
	assert.True(t, bytes.Contains(preparation.Artifact, []byte("Provider-proposed: ")), "the provider-proposed fields are labelled on the card")
	assert.Equal(t, 1, countGreekPassCacheRows(t, ctx, fixture.pool, fixture.codec), "only the resolved gloss is cached")

	status, err := fixture.store.GetDeckPreparationStatus(ctx, fixture.learner.ID, preparation.ID)
	require.NoError(t, err)
	requireGreekDeckOmissionKinds(t, status, 0)

	// Re-rendering the frozen specification and its exact overlay reproduces the
	// same labelled artifact without contacting the provider again.
	projection, results, err := fixture.store.LoadPreparedDeckFinalization(ctx, fixture.learner.ID, preparation.ID, preparation.CurrentRunID)
	require.NoError(t, err)
	restored, err := cardexport.NewPresentation(greekDeckLexicon{}).Restore(projection)
	require.NoError(t, err)
	facts := cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: fixture.codec.ProviderName(), ProviderVersion: fixture.codec.EvidenceAssistedPassProviderVersion()}
	rerendered, _, err := cardexport.NewPresentation(greekDeckLexicon{}).Finalize(ctx, restored, results, facts)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(preparation.Artifact, rerendered.APKG), "the pass deck re-renders byte-identically")
	assert.Equal(t, 3, fixture.provider.calls, "a re-render never contacts the provider again")
}

// TestAncientGreekEvidenceAssistedPassAbstentionKeepsDefaultCard proves an
// abstaining answer on a card that has pinned evidence adds nothing: the card is
// the default card and the run reports no meaning or provider omission.
func TestAncientGreekEvidenceAssistedPassAbstentionKeepsDefaultCard(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newGreekPassFixture(t, ctx, "greek-pass-abstain-learner", func(enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		return enrichment.TranslationResponse{Translation: "word", Gloss: "", ContextOnly: true, UnresolvedReason: "no defensible contextual sense", SentenceTranslation: "The man sees the word."}, nil
	})
	preparation := fixture.startReadingAndDeck(t, ctx, true)

	assert.Equal(t, 3, fixture.provider.calls, "the pass still asks the provider")
	assert.Equal(t, 3, preparation.TotalCards, "an abstaining pass still produces every default card")
	assert.False(t, bytes.Contains(preparation.Artifact, []byte("Provider-proposed: ")), "an abstaining pass adds no labelled field")
	assert.Equal(t, 0, countGreekPassCacheRows(t, ctx, fixture.pool, fixture.codec), "an abstaining pass caches nothing")

	status, err := fixture.store.GetDeckPreparationStatus(ctx, fixture.learner.ID, preparation.ID)
	require.NoError(t, err)
	requireGreekDeckOmissionKinds(t, status, 0)
}

// TestAncientGreekEvidenceAssistedPassFailureKeepsDefaultCard proves a failed
// provider answer adds nothing and drops nothing: the deck is prepared with the
// default cards and no provider or meaning omission.
func TestAncientGreekEvidenceAssistedPassFailureKeepsDefaultCard(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newGreekPassFixture(t, ctx, "greek-pass-failure-learner", func(enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		return enrichment.TranslationResponse{}, errors.New("provider unavailable")
	})
	preparation := fixture.startReadingAndDeck(t, ctx, true)

	assert.Equal(t, 3, preparation.TotalCards, "a failed pass still produces every default card")
	assert.False(t, bytes.Contains(preparation.Artifact, []byte("Provider-proposed: ")), "a failed pass adds no labelled field")
	assert.Equal(t, 0, countGreekPassCacheRows(t, ctx, fixture.pool, fixture.codec))

	status, err := fixture.store.GetDeckPreparationStatus(ctx, fixture.learner.ID, preparation.ID)
	require.NoError(t, err)
	requireGreekDeckOmissionKinds(t, status, 0)
}

// TestAncientGreekEvidenceAssistedPassOffMakesNoRequest proves the pass is off
// by default: a Greek deck with a configured provider still makes no request and
// adds no provider-proposed field.
func TestAncientGreekEvidenceAssistedPassOffMakesNoRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newGreekPassFixture(t, ctx, "greek-pass-off-learner", greekPassResolvedResponse)
	preparation := fixture.startReadingAndDeck(t, ctx, false)

	assert.Equal(t, 3, preparation.TotalCards)
	assert.Zero(t, fixture.provider.calls, "a Greek deck with the pass off makes no provider request")
	assert.False(t, bytes.Contains(preparation.Artifact, []byte("Provider-proposed: ")))
	assert.Equal(t, 0, countGreekPassCacheRows(t, ctx, fixture.pool, fixture.codec))
}

// greekPassResolvedResponse answers a pass request with a resolved contextual
// Gloss and sentence translation when the card has pinned LSJ evidence, and an
// explicit abstention otherwise.
func greekPassResolvedResponse(request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	if len(request.CandidateSenses) == 0 {
		return enrichment.TranslationResponse{Translation: "bind", Gloss: "", ContextOnly: true, UnresolvedReason: "no pinned sense applies", SentenceTranslation: "The man speaks."}, nil
	}
	return enrichment.TranslationResponse{
		Translation: "word", Gloss: "word",
		EvidenceIDs:                []string{request.CandidateSenses[0].EvidenceID},
		SentenceTranslation:        "The man sees the word.",
		SentenceTranslationTargets: []string{"word"},
	}, nil
}

func greekPassCodec(t *testing.T) *enrichment.TranslationCodec {
	t.Helper()
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	return codec
}

// newGreekPassClient wires the full durable prepared-deck pipeline with a
// configured provider and a recorded evidence-assisted pass assessment. The
// client is returned unstarted, so a test can set the deck specification's pass
// flag before the freeze worker runs.
func newGreekPassClient(t *testing.T, ctx context.Context, store *persistence.PostgresStore, pool *pgxpool.Pool, codec *enrichment.TranslationCodec, provider enrichment.TranslationProvider) *river.Client[pgx.Tx] {
	t.Helper()
	workers := river.NewWorkers()
	presentation := cardexport.NewPresentation(greekDeckLexicon{})
	preparedConfig := prepareddeck.PreparedDeckConfig{
		TranslationMode:        prepareddeck.DefaultTranslationMode,
		StandardMaxConcurrency: 1,
		StandardMaxAttempts:    1,
		StandardRetryBaseDelay: time.Millisecond,
		StandardRetryMaxDelay:  time.Millisecond,
	}
	batchConfig := prepareddeck.BatchConfig{MaxRequests: 1, PollInterval: time.Millisecond}
	metrics := prepareddeck.NewMetricsCollector()
	assessments := []enrichment.ProviderAssessment{{
		Language: "grc", Provider: codec.ProviderName(),
		PromptVersion: codec.EvidenceAssistedPassPromptContract(),
		Fields:        enrichment.EvidenceAssistedPassPayloadFields(),
		Disposition:   "recorded for this test", Ticket: "#2171", Recorded: true,
	}}

	unusedAnalyzer := &analyzertest.Fake{AnalyzeFunc: func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{}, errors.New("the imported Ancient Greek Book never calls the analyzer")
	}}
	client, err := analysis.NewClientWithPreparedDeckConcurrency(pool, unusedAnalyzer, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store), 1, workers)
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "analysis River client", client.Stop)

	prepareddeck.AddPreparedDeckWorkerWithAssessments(workers, store, presentation, client, codec, batchConfig, preparedConfig, true, assessments)
	prepareddeck.AddStandardTranslationWorkerWithDependencies(workers, store, client, provider, preparedConfig, time.Second, metrics)
	prepareddeck.AddBatchSubmitWorkerWithMetrics(workers, store, client, nil, nil, metrics)
	prepareddeck.AddBatchPollWorker(workers, &prepareddeck.BatchPollWorker{Store: store, Client: client, PollInterval: batchConfig.PollInterval, Metrics: metrics})
	prepareddeck.AddFinalizeWorker(workers, &prepareddeck.DurableFinalizer{Store: store, Renderer: presentation, Metrics: metrics})
	prepareddeck.AddBatchCleanupWorker(workers, &prepareddeck.BatchCleanupWorker{Store: store, Metrics: metrics})
	prepareddeck.AddRecoveryWorkerWithMetrics(workers, store, client, batchConfig.PollInterval, metrics)
	require.NoError(t, prepareddeck.EnsureRecoveryJob(ctx, store, client))
	return client
}

// countGreekPassCacheRows counts the cached pass answers.
func countGreekPassCacheRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, codec *enrichment.TranslationCodec) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM enrichment_cache WHERE provider=$1 AND provider_version=$2`, codec.ProviderName(), codec.EvidenceAssistedPassProviderVersion()).Scan(&count))
	return count
}
