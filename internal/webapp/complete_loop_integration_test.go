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
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = analysis.MigrateRiver(ctx, pool); err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
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
	if err = prepareddeck.EnsureRecoveryJob(ctx, store, analysisClient); err != nil {
		t.Fatal(err)
	}
	if err = catalogueSyncService.RegisterAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err = analysisClient.Start(ctx); err != nil {
		t.Fatal(err)
	}
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
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != "/" {
		t.Fatalf("onboarding=%d location=%q body=%s", created.Code, created.Header().Get("Location"), created.Body.String())
	}
	cookies := []*http.Cookie{cookieNamed(t, created.Result().Cookies(), csrfCookie), cookieNamed(t, created.Result().Cookies(), webauth.CookieName)}
	csrf = cookies[0].Value
	owner, _, err := store.GetUserByUsername(ctx, "loop-learner")
	if err != nil {
		t.Fatal(err)
	}

	connectionResponse := perform(t, h, http.MethodPost, "/connections", url.Values{
		"csrf_token": {csrf},
		"name":       {"Loop catalogue"},
		"url":        {catalogue.URL + "/opds"},
	}, cookies)
	if connectionResponse.Code != http.StatusSeeOther {
		t.Fatalf("create connection=%d location=%q body=%s", connectionResponse.Code, connectionResponse.Header().Get("Location"), connectionResponse.Body.String())
	}
	connections, err := store.ListOpdsConnections(ctx, owner.ID)
	if err != nil || len(connections) != 1 {
		t.Fatalf("connections=%+v err=%v", connections, err)
	}
	connection := connections[0]

	syncResponse := perform(t, h, http.MethodPost, "/connections/"+connection.ID+"/sync", url.Values{"csrf_token": {csrf}}, cookies)
	if syncResponse.Code != http.StatusSeeOther {
		t.Fatalf("sync=%d location=%q body=%s", syncResponse.Code, syncResponse.Header().Get("Location"), syncResponse.Body.String())
	}
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
	if err != nil || len(books) != 1 || books[0].Title != "Complete Loop Book" {
		t.Fatalf("synced My Books=%+v err=%v", books, err)
	}
	bookID := books[0].ID
	metadataOnly, err := store.GetBookDetail(ctx, owner.ID, bookID)
	if err != nil || metadataOnly.Acquired != nil {
		t.Fatalf("synced catalogue book was acquired before Journey action: detail=%+v err=%v", metadataOnly, err)
	}

	journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	added := perform(t, h, http.MethodPost, "/journey/books/"+bookID+"/add", url.Values{
		"csrf_token":        {csrf},
		"expected_revision": {fmt.Sprintf("%d", journey.Revision)},
	}, cookies)
	if added.Code != http.StatusSeeOther {
		t.Fatalf("add to Journey=%d location=%q body=%s", added.Code, added.Header().Get("Location"), added.Body.String())
	}

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
	if detail.Acquired == nil || detail.Acquired.AnalysisRunID == "" || detail.Acquired.CorpusID == "" {
		t.Fatalf("completed current analysis=%+v", detail.Acquired)
	}

	journeyPage := perform(t, h, http.MethodGet, "/journey/"+bookID, nil, cookies)
	if journeyPage.Code != http.StatusOK || !strings.Contains(journeyPage.Body.String(), "Prepare deck") {
		t.Fatalf("completed Journey entry=%d body=%s", journeyPage.Code, journeyPage.Body.String())
	}
	prepared := perform(t, h, http.MethodPost, "/journey/books/"+bookID+"/deck/preparations", url.Values{
		"csrf_token":                   {csrf},
		"external_translation_consent": {"on"},
	}, cookies)
	if prepared.Code != http.StatusSeeOther {
		t.Fatalf("prepare deck=%d location=%q body=%s", prepared.Code, prepared.Header().Get("Location"), prepared.Body.String())
	}
	preparationID := strings.TrimSuffix(strings.TrimPrefix(prepared.Header().Get("Location"), "/deck-preparations/"), "/status")
	if preparationID == "" || preparationID == prepared.Header().Get("Location") {
		t.Fatalf("unexpected preparation location=%q", prepared.Header().Get("Location"))
	}
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
		t.Fatalf("ready preparation state=%s total_cards=%d current_run=%q translation=%d/%d error=%q candidates=%+v candidate_err=%v", preparation.State, preparation.TotalCards, preparation.CurrentRunID, preparation.TranslationDone, preparation.TranslationEligible, preparation.Error, candidates, candidateErr)
	}

	// Repeating the same request must reuse the current Book deck rather than
	// creating a competing current preparation.
	repeated := perform(t, h, http.MethodPost, "/journey/books/"+bookID+"/deck/preparations", url.Values{
		"csrf_token":                   {csrf},
		"external_translation_consent": {"on"},
	}, cookies)
	if repeated.Code != http.StatusSeeOther || repeated.Header().Get("Location") != prepared.Header().Get("Location") {
		t.Fatalf("repeated prepare=%d location=%q want %q", repeated.Code, repeated.Header().Get("Location"), prepared.Header().Get("Location"))
	}
	preparations, err := store.ListDeckPreparationsForSourceMaterial(ctx, owner.ID, preparation.SourceMaterialID)
	if err != nil || len(preparations) != 1 {
		t.Fatalf("current deck preparations=%+v err=%v", preparations, err)
	}

	started := perform(t, h, http.MethodPost, "/journey/books/"+bookID+"/vocabulary-study", url.Values{"csrf_token": {csrf}}, cookies)
	if started.Code != http.StatusSeeOther {
		t.Fatalf("start vocabulary study=%d location=%q body=%s", started.Code, started.Header().Get("Location"), started.Body.String())
	}
	preparation, err = store.GetDeckPreparation(ctx, owner.ID, preparationID)
	if err != nil || preparation.VocabularyStudyStatus() != domain.VocabularyStudyStudying {
		t.Fatalf("studying preparation=%+v err=%v", preparation, err)
	}
	confirmed := perform(t, h, http.MethodPost, "/journey/books/"+bookID+"/vocabulary-study/confirm", url.Values{"csrf_token": {csrf}}, cookies)
	if confirmed.Code != http.StatusSeeOther || !strings.Contains(confirmed.Header().Get("Location"), "graduated+to+known") {
		t.Fatalf("confirm vocabulary study=%d location=%q body=%s", confirmed.Code, confirmed.Header().Get("Location"), confirmed.Body.String())
	}

	preparation, err = store.GetDeckPreparation(ctx, owner.ID, preparationID)
	if err != nil || preparation.GraduatedAt == nil || preparation.ReviewedAt == nil {
		t.Fatalf("graduated preparation=%+v err=%v", preparation, err)
	}
	snapshot, err := store.ListDeckPreparationVocabulary(ctx, owner.ID, preparationID)
	if err != nil || len(snapshot) != 1 || snapshot[0].Language != "de" || snapshot[0].CanonicalLemma != "haus" || snapshot[0].UPOS != "NOUN" || snapshot[0].GraduatedAt == nil {
		t.Fatalf("graduated snapshot=%+v err=%v", snapshot, err)
	}
	known, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	if err != nil || len(known) != 1 || known[0].CanonicalLemma != "haus" || known[0].UPOS != "NOUN" {
		t.Fatalf("known vocabulary=%+v err=%v", known, err)
	}
	coverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, detail.Acquired.CorpusID)
	if err != nil || coverage.KnownTokenCount != 3 || coverage.ReservedTokenCount != 0 {
		t.Fatalf("post-graduation coverage=%+v err=%v", coverage, err)
	}
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
			t.Fatalf("complete learner loop timed out: %s: %v", last, ctx.Err())
		case <-ticker.C:
		}
	}
}

var _ enrichment.TranslationProvider = completeLoopTranslationProvider{}
