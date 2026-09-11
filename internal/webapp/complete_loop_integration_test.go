//go:build integration

package webapp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/enrichmentjob"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type completeLoopTranslationProvider struct{}

func (completeLoopTranslationProvider) Name() string    { return "openai-compatible" }
func (completeLoopTranslationProvider) Version() string { return "complete-loop" }

func (completeLoopTranslationProvider) Translate(context.Context, enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	return enrichment.TranslationResponse{
		Translation:               "house",
		Gloss:                     "a building",
		SentenceTranslation:       "The house.",
		SentenceTranslationTarget: "house",
	}, nil
}

func TestCompleteLearnerLoopFromOnboardingToConfirmedGraduation(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "complete-learner-loop-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	bookEPUB := testEPUBVariant(t, "complete-loop-book", "Complete Loop Book", "Heute liest Anna das alte Haus. Heute liest Anna das alte Haus. Heute liest Anna das alte Haus.")

	catalogue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		switch r.URL.Path {
		case "/opds/language":
			_, _ = fmt.Fprint(w, `<?xml version="1.0"?><feed><title>Languages</title><entry><title>German</title><link rel="subsection" href="/opds/language/7"/></entry></feed>`)
		case "/opds/language/7":
			_, _ = fmt.Fprint(w, `<?xml version="1.0"?><feed><title>German books</title><entry><id>complete-loop-book</id><title>Complete Loop Book</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/book.epub"/></entry></feed>`)
		case "/book.epub":
			w.Header().Set("Content-Type", opds.EPUBMediaType)
			_, _ = w.Write(bookEPUB)
		default:
			http.NotFound(w, r)
		}
	}))
	defer catalogue.Close()

	capabilities := readyGerman()
	translator := completeLoopTranslationProvider{}
	enrichmentService := enrichment.NewService(enrichment.Config{
		ExternalEnabled: true,
		UserOptIn:       true,
		ContextMode:     enrichment.SentenceContext,
	}, nil, nil, nil, translator, store)
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "complete-loop-model", BaseURL: "http://example.invalid/v1"})
	require.NoError(t, err)
	preparedConfig := prepareddeck.PreparedDeckConfig{
		TranslationMode:        prepareddeck.DefaultTranslationMode,
		StandardMaxConcurrency: 1,
		StandardMaxAttempts:    1,
		StandardRetryBaseDelay: time.Millisecond,
		StandardRetryMaxDelay:  time.Millisecond,
	}
	batchConfig := prepareddeck.BatchConfig{MaxRequests: 1, PollInterval: time.Millisecond}

	workers := river.NewWorkers()
	knownvocab.AddWorker(workers, store.Pool())
	enrichmentjob.AddWorker(workers, store.Pool(), enrichmentService)
	epPubService := epub.NewService(store)
	opdsService := opds.NewService(store, epPubService, nil)
	cataloguesync.AddWorker(workers, store, opdsService, capabilities)
	selectionService := selection.NewService(store)
	analysisClient, err := analysis.NewClientWithPreparedDeckConcurrency(store.Pool(), &analyzertest.Fake{AnalyzeFunc: completeLoopAnalysis}, selectionService, 1, workers)
	require.NoError(t, err)
	catalogueSyncService := cataloguesync.NewService(store, analysisClient, opdsService, capabilities)
	preparedExport := cardexport.NewService(store)
	metrics := prepareddeck.NewMetricsCollector()
	prepareddeck.AddPreparedDeckWorker(workers, store, preparedExport, analysisClient, codec, batchConfig, preparedConfig, true)
	prepareddeck.AddStandardTranslationWorkerWithDependencies(workers, store, analysisClient, translator, preparedConfig, time.Second, metrics)
	prepareddeck.AddBatchSubmitWorkerWithMetrics(workers, store, analysisClient, nil, nil, metrics)
	prepareddeck.AddBatchPollWorker(workers, &prepareddeck.BatchPollWorker{Store: store, Client: analysisClient, PollInterval: batchConfig.PollInterval, Metrics: metrics})
	prepareddeck.AddFinalizeWorker(workers, &prepareddeck.DurableFinalizer{Store: store, Renderer: preparedExport, Metrics: metrics})
	prepareddeck.AddBatchCleanupWorker(workers, &prepareddeck.BatchCleanupWorker{Store: store, Metrics: metrics})
	prepareddeck.AddRecoveryWorkerWithMetrics(workers, store, analysisClient, batchConfig.PollInterval, metrics)
	require.NoError(t, prepareddeck.EnsureRecoveryJob(ctx, store, analysisClient))
	require.NoError(t, catalogueSyncService.RegisterAll(ctx))
	require.NoError(t, analysisClient.Start(ctx))
	defer analysisClient.Stop(context.Background())

	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth:             authService,
		WebAuth:          webauth.New(authService, false, time.Hour),
		Store:            store,
		OPDS:             opdsService,
		Analysis:         analysis.NewService(store.Pool(), analysisClient),
		AnalysisInsights: analysisinsights.NewService(store),
		KnownVocab:       knownvocab.NewJobService(store.Pool(), analysisClient),
		Enrichment:       enrichmentjob.NewService(store.Pool(), analysisClient, enrichmentService),
		PreparedDeck:     prepareddeck.NewService(store, analysisClient),
		Capabilities:     capabilities,
		CatalogueSync:    catalogueSyncService,
		SessionLifetime:  time.Hour,
	})

	loginPage := perform(t, h, http.MethodGet, "/login", nil, nil)
	csrf := hiddenToken(t, loginPage.Body.String())
	created := perform(t, h, http.MethodPost, "/onboarding", url.Values{
		"csrf_token": {csrf},
		"username":   {"loop-learner"},
		"password":   {"loop-password"},
	}, []*http.Cookie{cookieNamed(t, loginPage.Result().Cookies(), csrfCookie)})
	assert.Equal(t, http.StatusSeeOther, created.Code)
	assert.Equal(t, "/", created.Header().Get("Location"))
	cookies := []*http.Cookie{cookieNamed(t, created.Result().Cookies(), csrfCookie), cookieNamed(t, created.Result().Cookies(), webauth.CookieName)}
	csrf = cookies[0].Value
	owner, _, err := store.GetUserByUsername(ctx, "loop-learner")
	require.NoError(t, err)

	connectionResponse := perform(t, h, http.MethodPost, "/connections", url.Values{
		"csrf_token": {csrf},
		"name":       {"Loop catalogue"},
		"url":        {catalogue.URL + "/opds"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, connectionResponse.Code)
	connections, err := store.ListOpdsConnections(ctx, owner.ID)
	require.NoError(t, err)
	require.Equal(t, 1, len(connections))
	connection := connections[0]

	syncResponse := perform(t, h, http.MethodPost, "/connections/"+connection.ID+"/sync", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, syncResponse.Code)
	waitForCompleteLoop(t, ctx, func() (bool, string) {
		books, listErr := store.ListMyBooks(ctx, owner.ID)
		if listErr != nil {
			return false, listErr.Error()
		}
		if len(books) != 1 {
			return false, fmt.Sprintf("My Books=%d", len(books))
		}
		status, statusErr := store.GetCatalogueSyncStatus(ctx, owner.ID, connection.ID)
		if statusErr != nil || status.State != domain.CatalogueSyncSynced {
			return false, fmt.Sprintf("sync status=%+v err=%v", status, statusErr)
		}
		return true, ""
	})
	books, err := store.ListMyBooks(ctx, owner.ID)
	require.NoError(t, err)
	require.Equal(t, 1, len(books))
	assert.Equal(t, "Complete Loop Book", books[0].Title)
	bookID := books[0].ID
	metadataOnly, err := store.GetBookDetail(ctx, owner.ID, bookID)
	require.NoError(t, err)
	assert.Nil(t, metadataOnly.Acquired, "synced catalogue book was acquired before Journey action")

	journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	added := perform(t, h, http.MethodPost, "/journey/books/"+bookID+"/add", url.Values{
		"csrf_token":        {csrf},
		"expected_revision": {fmt.Sprintf("%d", journey.Revision)},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, added.Code)

	var detail domain.MyBook
	waitForCompleteLoop(t, ctx, func() (bool, string) {
		var getErr error
		detail, getErr = store.GetBookDetail(ctx, owner.ID, bookID)
		if getErr != nil {
			return false, getErr.Error()
		}
		if detail.Acquired == nil || detail.Acquired.EvidenceState() != domain.BookAnalyzed {
			return false, fmt.Sprintf("book detail=%+v", detail)
		}
		return true, ""
	})
	require.NotNil(t, detail.Acquired, "completed current analysis=%+v", detail.Acquired)
	require.NotEmpty(t, detail.Acquired.AnalysisRunID, "completed current analysis=%+v", detail.Acquired)
	require.NotEmpty(t, detail.Acquired.CorpusID, "completed current analysis=%+v", detail.Acquired)

	journeyPage := perform(t, h, http.MethodGet, "/journey/"+bookID, nil, cookies)
	assert.Equal(t, http.StatusOK, journeyPage.Code)
	assert.True(t, strings.Contains(journeyPage.Body.String(), "Prepare deck"), "completed Journey entry body=%s", journeyPage.Body.String())
	prepared := perform(t, h, http.MethodPost, "/journey/books/"+bookID+"/deck/preparations", url.Values{
		"csrf_token":                   {csrf},
		"external_translation_consent": {"on"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, prepared.Code)
	preparationID := strings.TrimSuffix(strings.TrimPrefix(prepared.Header().Get("Location"), "/deck-preparations/"), "/status")
	require.NotEmpty(t, preparationID, "unexpected preparation location=%q", prepared.Header().Get("Location"))
	require.NotEqual(t, prepared.Header().Get("Location"), preparationID, "unexpected preparation location=%q", prepared.Header().Get("Location"))
	var preparation domain.DeckPreparation
	waitForCompleteLoop(t, ctx, func() (bool, string) {
		var getErr error
		preparation, getErr = store.GetDeckPreparation(ctx, owner.ID, preparationID)
		if getErr != nil {
			return false, getErr.Error()
		}
		if preparation.State != domain.DeckPreparationReady {
			return false, fmt.Sprintf("preparation=%+v", preparation)
		}
		return true, ""
	})
	if preparation.TotalCards != 1 || preparation.CurrentRunID == "" {
		candidates, candidateErr := store.ListSelectionCandidatesForCorpus(ctx, owner.ID, detail.Acquired.CorpusID)
		require.Failf(t, "ready preparation failure", "ready preparation state=%s total_cards=%d current_run=%q translation=%d/%d error=%q candidates=%+v candidate_err=%v", string(preparation.State), preparation.TotalCards, preparation.CurrentRunID, preparation.TranslationDone, preparation.TranslationEligible, preparation.Error, candidates, candidateErr)
	}

	// Repeating the same request must reuse the current Book deck rather than
	// creating a competing current preparation.
	repeated := perform(t, h, http.MethodPost, "/journey/books/"+bookID+"/deck/preparations", url.Values{
		"csrf_token":                   {csrf},
		"external_translation_consent": {"on"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, repeated.Code)
	assert.Equal(t, prepared.Header().Get("Location"), repeated.Header().Get("Location"))
	preparations, err := store.ListDeckPreparationsForSourceMaterial(ctx, owner.ID, preparation.SourceMaterialID)
	require.NoError(t, err)
	assert.Equal(t, 1, len(preparations))

	started := perform(t, h, http.MethodPost, "/journey/books/"+bookID+"/vocabulary-study", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, started.Code)
	preparation, err = store.GetDeckPreparation(ctx, owner.ID, preparationID)
	require.NoError(t, err)
	assert.Equal(t, domain.VocabularyStudyStudying, preparation.VocabularyStudyStatus())
	reservedCoverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, detail.Acquired.CorpusID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), reservedCoverage.KnownTokenCount)
	assert.Equal(t, int64(3), reservedCoverage.ReservedTokenCount)
	confirmed := perform(t, h, http.MethodPost, "/journey/books/"+bookID+"/vocabulary-study/confirm", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, confirmed.Code)
	assert.True(t, strings.Contains(confirmed.Header().Get("Location"), "graduated+to+known"), "confirm vocabulary study location=%q body=%s", confirmed.Header().Get("Location"), confirmed.Body.String())

	preparation, err = store.GetDeckPreparation(ctx, owner.ID, preparationID)
	require.NoError(t, err)
	assert.NotNil(t, preparation.GraduatedAt)
	assert.NotNil(t, preparation.ReviewedAt)
	snapshot, err := store.ListDeckPreparationVocabulary(ctx, owner.ID, preparationID)
	require.NoError(t, err)
	require.Equal(t, 1, len(snapshot))
	assert.Equal(t, "de", snapshot[0].Language)
	assert.Equal(t, "haus", snapshot[0].CanonicalLemma)
	assert.Equal(t, "NOUN", snapshot[0].UPOS)
	assert.NotNil(t, snapshot[0].GraduatedAt)
	known, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.Equal(t, 1, len(known))
	assert.Equal(t, "haus", known[0].CanonicalLemma)
	assert.Equal(t, "NOUN", known[0].UPOS)
	coverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, detail.Acquired.CorpusID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), coverage.KnownTokenCount)
	assert.Equal(t, int64(0), coverage.ReservedTokenCount)
}

func completeLoopAnalysis(ctx context.Context, request analyzer.AnalyzeRequest) (analyzer.Result, error) {
	return analyzer.Result{
		SchemaVersion: "1.0.0",
		Language:      request.Language,
		SourceDocuments: []analyzer.SourceDocumentMetadata{{
			ID: request.Document.ID, SourceIdentifier: request.Document.SourceIdentifier, Title: request.Document.Title,
		}},
		Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "complete-loop-fake", AnalyzerVersion: "1"},
		NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"},
		Sentences: []analyzer.Sentence{
			completeLoopSentence(request.Document.ID, 26),
			completeLoopSentence(request.Document.ID, 58),
			completeLoopSentence(request.Document.ID, 90),
		},
	}, nil
}

func completeLoopSentence(documentID string, start uint64) analyzer.Sentence {
	return analyzer.Sentence{
		Text: "Heute liest Anna das alte Haus.",
		Tokens: []analyzer.Token{{
			Surface: "Haus", RawLemma: "Haus", CanonicalLemma: "haus", UPOS: "NOUN",
			Morphology: map[string]string{"Gender": "Neut"},
			Location:   analyzer.SourceLocation{SourceDocumentID: documentID, StartOffset: start, EndOffset: start + 4},
		}},
	}
}

func waitForCompleteLoop(t *testing.T, ctx context.Context, condition func() (bool, string)) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var last string
	for {
		if done, reason := condition(); done {
			return
		} else {
			last = reason
		}
		select {
		case <-ctx.Done():
			require.Failf(t, "complete learner loop timed out", "complete learner loop timed out: %s: %v", last, ctx.Err())
		case <-ticker.C:
		}
	}
}

var _ enrichment.TranslationProvider = completeLoopTranslationProvider{}
