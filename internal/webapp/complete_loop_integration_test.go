//go:build integration

package webapp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
	"github.com/justin-hayes/mouseion/internal/testwrite"
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
		FallbackGloss:             "a building",
		SentenceTranslation:       "The house.",
		SentenceTranslationTarget: "house",
	}, nil
}

func TestCompleteLearnerLoopFromOnboardingToGoalCompletion(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "complete-learner-loop-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	bookEPUB := testEPUBVariant(t, "complete-loop-book", "Complete Loop Book", "Heute liest Anna das alte Haus. Heute liest Anna das alte Haus. Heute liest Anna das alte Haus.")

	catalogue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		switch r.URL.Path {
		case "/opds/language":
			testwrite.String(t, w, `<?xml version="1.0"?><feed><title>Languages</title><entry><title>German</title><link rel="subsection" href="/opds/language/7"/></entry></feed>`)
		case "/opds/language/7":
			testwrite.String(t, w, `<?xml version="1.0"?><feed><title>German books</title><entry><id>complete-loop-book</id><title>Complete Loop Book</title><link rel="http://opds-spec.org/acquisition" type="application/epub+zip" href="/book.epub"/></entry></feed>`)
		case "/book.epub":
			w.Header().Set("Content-Type", opds.EPUBMediaType)
			testwrite.Bytes(t, w, bookEPUB)
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
	catalogueSyncDeps := cataloguesync.StoreDependencies{Connections: store, Catalogue: store, Aliases: store, Statuses: store, Pool: store.Pool()}
	cataloguesync.AddWorker(workers, catalogueSyncDeps, opdsService, capabilities)
	selectionService := selection.NewService(store)
	analysisClient, err := analysis.NewClientWithPreparedDeckConcurrency(store.Pool(), &analyzertest.Fake{AnalyzeFunc: completeLoopAnalysis}, analyzertest.ReadyDepparseCapabilityProvider(), selectionService, 1, workers)
	require.NoError(t, err)
	catalogueSyncService := cataloguesync.NewService(catalogueSyncDeps, analysisClient, opdsService, capabilities)
	preparedExport := cardexport.NewPresentation(nil)
	metrics := prepareddeck.NewMetricsCollector()
	prepareddeck.AddPreparedDeckWorker(workers, store, preparedExport, analysisClient, codec, batchConfig, preparedConfig, true)
	prepareddeck.AddStandardTranslationWorkerWithDependencies(workers, store, analysisClient, translator, preparedConfig, time.Second, metrics)
	prepareddeck.AddBatchSubmitWorkerWithMetrics(workers, store, analysisClient, nil, nil, metrics)
	prepareddeck.AddBatchPollWorker(workers, &prepareddeck.BatchPollWorker{Store: store, Client: analysisClient, PollInterval: batchConfig.PollInterval, Metrics: metrics})
	prepareddeck.AddFinalizeWorker(workers, &prepareddeck.DurableFinalizer{Store: store, Renderer: cardexport.NewPresentation(nil), Metrics: metrics})
	prepareddeck.AddBatchCleanupWorker(workers, &prepareddeck.BatchCleanupWorker{Store: store, Metrics: metrics})
	prepareddeck.AddRecoveryWorkerWithMetrics(workers, store, analysisClient, batchConfig.PollInterval, metrics)
	require.NoError(t, prepareddeck.EnsureRecoveryJob(ctx, store, analysisClient))
	require.NoError(t, catalogueSyncService.RegisterAll(ctx))
	require.NoError(t, analysisClient.Start(ctx))
	testutil.Cleanup(t, "analysis River client", func() error { return analysisClient.Stop(context.Background()) })

	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth:             authService,
		WebAuth:          webauth.New(authService, false, time.Hour),
		Store:            storeDependencies(store),
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

	added := perform(t, h, http.MethodPost, "/library/books/"+bookID+"/to-read", url.Values{"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(metadataOnly.DispositionRevision, 10)}}, cookies)
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

	chosen := perform(t, h, http.MethodPost, "/reading/books/"+bookID+"/start", url.Values{
		"csrf_token": {csrf},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, chosen.Code)
	assert.Contains(t, chosen.Header().Get("Location"), "/reading")
	goal, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.NotEmpty(t, goal.SnapshotID)
	myBooks := perform(t, h, http.MethodGet, "/library", nil, cookies)
	assert.Contains(t, myBooks.Body.String(), "Workflow</strong>: Currently reading")
	toReadPage := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	assert.Contains(t, toReadPage.Body.String(), "To Read (1)")
	assert.Contains(t, toReadPage.Body.String(), "Complete Loop Book")
	assert.Contains(t, toReadPage.Body.String(), "Workflow</strong>: Currently reading")
	var preparation domain.DeckPreparation
	waitForCompleteLoop(t, ctx, func() (bool, string) {
		preparations, listErr := store.ListDeckPreparationsForSourceMaterial(ctx, owner.ID, detail.Acquired.Source.ID)
		if listErr != nil {
			return false, listErr.Error()
		}
		if len(preparations) == 0 {
			return false, "Goal preparation has not been created"
		}
		preparation = preparations[0]
		if preparation.State != domain.DeckPreparationReady {
			if preparation.State == domain.DeckPreparationFailed {
				return true, ""
			}
			return false, fmt.Sprintf("preparation=%+v", preparation)
		}
		return true, ""
	})
	if preparation.TotalCards != 1 || preparation.CurrentRunID == "" {
		require.Failf(t, "ready preparation failure", "ready preparation state=%s total_cards=%d current_run=%q translation=%d/%d error=%q", string(preparation.State), preparation.TotalCards, preparation.CurrentRunID, preparation.TranslationDone, preparation.TranslationEligible, preparation.Error)
	}
	assert.Equal(t, goal.SnapshotID, preparation.GoalSnapshotID)
	var consent bool
	err = store.Pool().QueryRow(ctx, `SELECT COALESCE((args->>'external_translation_consent')::boolean, false) FROM river_job WHERE args->>'preparation_id'=$1 ORDER BY id DESC LIMIT 1`, preparation.ID).Scan(&consent)
	require.NoError(t, err)
	assert.False(t, consent)

	preparations, err := store.ListDeckPreparationsForSourceMaterial(ctx, owner.ID, preparation.SourceMaterialID)
	require.NoError(t, err)
	assert.Equal(t, 1, len(preparations))

	reservedCoverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, detail.Acquired.CorpusID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), reservedCoverage.KnownTokenCount)
	assert.Equal(t, int64(3), reservedCoverage.ReservedTokenCount)
	finished := perform(t, h, http.MethodPost, "/reading/finish", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_book_id":     {bookID},
		"expected_current_snapshot_id": {goal.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusOK, finished.Code)
	assert.Contains(t, finished.Body.String(), "Reading finished")
	assert.Contains(t, finished.Body.String(), `href="/reading">Choose what to read next</a>`)

	chooser := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	assert.Equal(t, http.StatusOK, chooser.Code)
	assert.Contains(t, chooser.Body.String(), "Choose a To Read book when you are ready.")
	readAgainState, err := store.GetBookDetail(ctx, owner.ID, bookID)
	require.NoError(t, err)
	readAgain := perform(t, h, http.MethodPost, "/library/books/"+bookID+"/read-again", url.Values{
		"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(readAgainState.DispositionRevision, 10)},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, readAgain.Code)
	assert.Contains(t, readAgain.Header().Get("Location"), "disposition=to_read")
	readPage := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	assert.Equal(t, http.StatusOK, readPage.Code)
	assert.Contains(t, readPage.Body.String(), "Read again")
	assert.Contains(t, readPage.Body.String(), "Workflow</strong>: To Read")

	startAgain := perform(t, h, http.MethodPost, "/reading/books/"+bookID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, startAgain.Code)
	myBooks = perform(t, h, http.MethodGet, "/library", nil, cookies)
	assert.Contains(t, myBooks.Body.String(), "Workflow</strong>: Currently reading")
	assert.Contains(t, myBooks.Body.String(), "Reading history")
	toReadPage = perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	assert.Contains(t, toReadPage.Body.String(), "To Read (1)")
	assert.Contains(t, toReadPage.Body.String(), "Complete Loop Book")
	assert.Contains(t, toReadPage.Body.String(), "Workflow</strong>: Currently reading")
	newGoal, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.NotEqual(t, goal.SnapshotID, newGoal.SnapshotID, "starting the reread must freeze a fresh snapshot")

	known, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.Equal(t, 1, len(known))
	assert.Equal(t, "haus", known[0].CanonicalLemma)
	assert.Equal(t, "NOUN", known[0].UPOS)
	coverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, detail.Acquired.CorpusID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), coverage.KnownTokenCount)
	assert.Equal(t, int64(0), coverage.ReservedTokenCount)
	var historyCount int
	var historyBookID string
	err = store.Pool().QueryRow(ctx, `SELECT count(*), max(book_id::text) FROM reading_history WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&historyCount, &historyBookID)
	require.NoError(t, err)
	assert.Equal(t, 1, historyCount)
	assert.Equal(t, bookID, historyBookID)
	goal, err = store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, bookID, goal.BookID)
	assert.NotEqual(t, "", goal.SnapshotID)
	stopped := perform(t, h, http.MethodPost, "/reading/stop", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {bookID}, "expected_current_snapshot_id": {goal.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, stopped.Code)
	toReadPage = perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	assert.Contains(t, toReadPage.Body.String(), "To Read (1)")
	assert.Contains(t, toReadPage.Body.String(), "Workflow</strong>: To Read")
	assert.Contains(t, toReadPage.Body.String(), "Reading history", "stopping restores To Read while retaining history")
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
	text := "Heute liest Anna das alte Haus."
	return analyzer.Sentence{
		Text:     text,
		Location: analyzer.SourceLocation{SourceDocumentID: documentID, StartOffset: start, EndOffset: start + uint64(len([]rune(text)))},
		Tokens: []analyzer.Token{
			{Surface: "Haus", RawLemma: "Haus", CanonicalLemma: "haus", UPOS: "NOUN", Dependency: "obj", Head: 1,
				Morphology: map[string]string{"Gender": "Neut"},
				Location:   analyzer.SourceLocation{SourceDocumentID: documentID, StartOffset: start + 26, EndOffset: start + 30}},
			{Surface: "liest", RawLemma: "lesen", CanonicalLemma: "lesen", UPOS: "AUX", Dependency: "root", Head: 1, Morphology: map[string]string{"VerbForm": "Fin"}},
			{Surface: "Anna", RawLemma: "Anna", CanonicalLemma: "anna", UPOS: "PROPN", Dependency: "nsubj", Head: 1},
		},
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
