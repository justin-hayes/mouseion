// Command server is the web-only v1 client over the shared Go core.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/deploysecret"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
	"github.com/justin-hayes/mouseion/internal/lemmarisk"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/sourcecheck"
	"github.com/justin-hayes/mouseion/internal/webapp"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
)

const (
	serverReadHeaderTimeout = 10 * time.Second
	serverReadTimeout       = 30 * time.Second
	serverWriteTimeout      = 30 * time.Second
	serverIdleTimeout       = 2 * time.Minute
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(context.Background()); err != nil {
			log.Fatalf("healthcheck failed: %v", err)
		}
		return
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// serverConfig is the environment the server needs before it opens any
// dependency: a usable secret, a database URL, and the listen address. Loading
// these first stops those misconfigurations with no partial startup.
type serverConfig struct {
	databaseURL     string
	addr            string
	secureCookies   bool
	sessionLifetime time.Duration
}

// serverDependencies holds the clients and providers built after the store
// opens and before the web application. The HTTP handler reads the same
// instances, so requests and workers share one capability cache and one River
// client.
type serverDependencies struct {
	authService       *auth.Service
	authHandler       *webauth.Handler
	opdsService       *opds.Service
	capabilities      analyzer.CapabilityProvider
	lemmaSuggestions  enrichment.LemmaSuggestionProvider
	batchProvider     *enrichment.OpenAIBatchClient
	catalogueSyncDeps cataloguesync.StoreDependencies
	riverClient       *river.Client[pgx.Tx]
}

func run() (err error) {
	ctx := context.Background()
	cfg, err := loadServerConfig()
	if err != nil {
		return err
	}
	store, err := migrateAndOpenStore(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer closeIntoResult("PostgreSQL store", store.Close, &err)
	nlp, err := analyzer.NewGRPCAnalyzer("")
	if err != nil {
		return err
	}
	defer closeIntoResult("NLP analyzer", nlp.Close, &err)
	llmConfig, err := enrichment.LLMConfigFromEnv()
	if err != nil {
		return err
	}
	dictionaryIndex, hasDictionaryIndex, err := openOptionalDictionaryIndex(ctx)
	if err != nil {
		return err
	}
	defer closeIntoResult("dictionary index", dictionaryIndex.Close, &err)
	deps, err := startServerDependencies(ctx, cfg, store, nlp, dictionaryIndex, llmConfig)
	if err != nil {
		return err
	}
	defer stopRiverClient(ctx, deps.riverClient, &err)
	mux := newServerMux()
	mux.Handle("/static/", webapp.StaticHandler())
	handler, err := buildWebHandler(ctx, cfg, store, dictionaryIndex, hasDictionaryIndex, deps)
	if err != nil {
		return err
	}
	mux.Handle("/", handler)
	log.Print("mouseion web server listening")
	return newHTTPServer(cfg.addr, mux).ListenAndServe()
}

// loadServerConfig validates the environment every start depends on: a usable
// secret, a database URL, and the listen address. It never opens a dependency.
func loadServerConfig() (serverConfig, error) {
	if err := deploysecret.ValidateSecret(os.Getenv("MOUSEION_SECRET")); err != nil {
		return serverConfig{}, fmt.Errorf("invalid MOUSEION_SECRET: %w", err)
	}
	databaseURL := os.Getenv("MOUSEION_DATABASE_URL")
	if databaseURL == "" {
		return serverConfig{}, errors.New("MOUSEION_DATABASE_URL is required")
	}
	addr := os.Getenv("MOUSEION_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	return serverConfig{
		databaseURL:     databaseURL,
		addr:            addr,
		secureCookies:   strings.EqualFold(os.Getenv("MOUSEION_COOKIE_SECURE"), "true"),
		sessionLifetime: 24 * time.Hour,
	}, nil
}

// migrateAndOpenStore migrates before opening, so the shared store only ever
// talks to the current schema.
func migrateAndOpenStore(ctx context.Context, databaseURL string) (*persistence.PostgresStore, error) {
	if err := persistence.Migrate(databaseURL); err != nil {
		return nil, err
	}
	return persistence.Open(ctx, databaseURL)
}

// startServerDependencies builds every long-lived client and provider and
// starts the River client, in the order the request handlers and workers expect.
func startServerDependencies(ctx context.Context, cfg serverConfig, store *persistence.PostgresStore, nlp *analyzer.GRPCAnalyzer, dictionaryIndex *dictionary.Index, llmConfig enrichment.LLMConfig) (*serverDependencies, error) {
	authService := auth.New(store, cfg.sessionLifetime)
	authHandler := webauth.New(authService, cfg.secureCookies, cfg.sessionLifetime)
	epubService := epub.NewService(store)
	opdsService := opds.NewService(store, epubService, nil)
	selectionService := selection.NewService(store)
	translationProvider, err := enrichment.NewConfiguredLLMProvider(llmConfig, nil)
	if err != nil {
		return nil, err
	}
	lemmaSuggestions, err := enrichment.NewConfiguredLemmaSuggestionProvider(llmConfig, nil)
	if err != nil {
		return nil, err
	}
	capabilities := analyzer.NewCachedCapabilityProvider(nlp, 5*time.Minute)
	batchConfig, err := prepareddeck.BatchConfigFromEnv()
	if err != nil {
		return nil, err
	}
	preparedDeckConfig, err := prepareddeck.PreparedDeckConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("invalid prepared-deck configuration: %w", err)
	}
	var batchProvider *enrichment.OpenAIBatchClient
	var batchCodec *enrichment.TranslationCodec
	if llmConfig.Enabled {
		batchCodec, err = enrichment.NewTranslationCodec(llmConfig)
		if err != nil {
			return nil, err
		}
		batchProvider, err = enrichment.NewOpenAIBatchClient(llmConfig, nil)
		if err != nil {
			return nil, fmt.Errorf("prepared-deck external translation requires the official OpenAI Batch endpoint: %w", err)
		}
	}
	batchMetrics := prepareddeck.NewMetricsCollector()
	catalogueSyncDeps := cataloguesync.StoreDependencies{Connections: store, Catalogue: store, Aliases: store, Statuses: store, Pool: store.Pool()}
	workers := river.NewWorkers()
	catalogueWorker := cataloguesync.AddWorker(workers, catalogueSyncDeps, opdsService, capabilities)
	river.AddWorker(workers, &sourcecheck.Worker{Store: store})
	presentation := cardexport.NewPresentation(dictionaryIndex)
	riverClient, err := analysis.NewClientWithPreparedDeckConcurrency(store.Pool(), nlp, capabilities, selectionService, preparedDeckConfig.StandardMaxConcurrency, workers)
	if err != nil {
		return nil, err
	}
	sourceStructureChecks := sourcecheck.NewService(store, riverClient, sourcecheck.ConfiguredTarget())
	epubService.WithSourceStructureCheckEnqueuer(sourceStructureChecks)
	catalogueWorker.Client = riverClient
	prepareddeck.AddPreparedDeckWorker(workers, store, presentation, riverClient, batchCodec, batchConfig, preparedDeckConfig, llmConfig.Enabled)
	prepareddeck.AddCustomDeckPreparationWorker(workers, store)
	registerPreparedDeckWorkersWithStandard(workers, store, riverClient, batchProvider, batchCodec, batchConfig.PollInterval, batchMetrics, translationProvider, preparedDeckConfig, llmConfig.Timeout)
	if err := prepareddeck.EnsureRecoveryJob(ctx, store, riverClient); err != nil {
		return nil, err
	}
	if err := sourceStructureChecks.EnqueueStartupChecks(ctx); err != nil {
		return nil, fmt.Errorf("schedule source structure checks: %w", err)
	}
	if err := riverClient.Start(ctx); err != nil {
		return nil, err
	}
	return &serverDependencies{
		authService:       authService,
		authHandler:       authHandler,
		opdsService:       opdsService,
		capabilities:      capabilities,
		lemmaSuggestions:  lemmaSuggestions,
		batchProvider:     batchProvider,
		catalogueSyncDeps: catalogueSyncDeps,
		riverClient:       riverClient,
	}, nil
}

// buildWebHandler registers the catalogue-sync schedules, then assembles the web
// application from the dependencies the workers share.
func buildWebHandler(ctx context.Context, cfg serverConfig, store *persistence.PostgresStore, dictionaryIndex *dictionary.Index, hasDictionaryIndex bool, deps *serverDependencies) (*webapp.Handler, error) {
	analysisService := analysis.NewService(store.Pool(), deps.riverClient).WithCapabilities(deps.capabilities)
	if err := analysisService.EnqueueMissingBrowseCountRebuilds(ctx); err != nil {
		return nil, fmt.Errorf("schedule missing Vocabulary Browse counts: %w", err)
	}
	var preparedDeckService *prepareddeck.Service
	if deps.batchProvider != nil {
		preparedDeckService = prepareddeck.NewServiceWithBatchCanceller(store, deps.riverClient, deps.batchProvider)
	} else {
		preparedDeckService = prepareddeck.NewService(store, deps.riverClient)
	}
	catalogueSyncService := cataloguesync.NewService(deps.catalogueSyncDeps, deps.riverClient, deps.opdsService, deps.capabilities)
	if err := catalogueSyncService.RegisterAll(ctx); err != nil {
		return nil, err
	}
	storeDeps := webapp.StoreDependencies{Shell: store, MyBooks: store, Reading: store, Vocabulary: store, Catalogs: store, Jobs: store}
	riskIndex := dictionaryRiskIndex(dictionaryIndex, hasDictionaryIndex)
	lemmaReview := lemmareview.New(lemmareview.Config{Store: store, RiskIndex: riskIndex, Suggestions: deps.lemmaSuggestions, Preparer: preparedDeckService, BrowseCounts: analysisService})
	webHandler, err := webapp.NewWithError(webapp.Services{Auth: deps.authService, WebAuth: deps.authHandler, Store: storeDeps, OPDS: deps.opdsService, Analysis: analysisService, AnalysisUpdates: analysisService, AnalysisInsights: analysisinsights.NewService(store), PreparedDeck: preparedDeckService, Capabilities: deps.capabilities, LemmaReview: lemmaReview, CatalogueSync: catalogueSyncService, SecureCookies: cfg.secureCookies, SessionLifetime: cfg.sessionLifetime})
	if err != nil {
		return nil, fmt.Errorf("initialize web application: %w", err)
	}
	return webHandler, nil
}

// newServerMux builds the outer mux and the liveness route that answers before
// the web application is ready.
func newServerMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprintln(w, "ok"); err != nil {
			log.Printf("write health response: %v", err)
		}
	})
	return mux
}

// stopRiverClient stops the analysis workers, joining a stop failure with any
// in-flight result so a shutdown problem never masks a startup one.
func stopRiverClient(ctx context.Context, client *river.Client[pgx.Tx], result *error) {
	stopErr := client.Stop(ctx)
	if stopErr == nil {
		return
	}
	stopErr = fmt.Errorf("stop analysis workers: %w", stopErr)
	if *result == nil {
		*result = stopErr
	} else {
		*result = errors.Join(*result, stopErr)
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
}

func closeIntoResult(name string, closeFn func() error, result *error) {
	if closeErr := closeFn(); closeErr != nil {
		closeErr = fmt.Errorf("close %s: %w", name, closeErr)
		if *result == nil {
			*result = closeErr
		} else {
			*result = errors.Join(*result, closeErr)
		}
	}
}

// runHealthcheck probes the server's own /healthz endpoint. It runs as the
// image entrypoint's `healthcheck` subcommand, since the distroless image has
// no shell or network utility for a Compose CMD-SHELL check.
func runHealthcheck(ctx context.Context) error {
	target, err := healthcheckURL(os.Getenv("MOUSEION_HTTP_ADDR"))
	if err != nil {
		return err
	}
	return probeHealth(ctx, target, &http.Client{Timeout: 5 * time.Second})
}

func healthcheckURL(addr string) (string, error) {
	if strings.TrimSpace(addr) == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid MOUSEION_HTTP_ADDR %q: %w", addr, err)
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	if !loopbackHost(host) {
		return "", fmt.Errorf("MOUSEION_HTTP_ADDR host must be loopback, got %q", host)
	}
	return "http://" + net.JoinHostPort(host, port) + "/healthz", nil
}

func probeHealth(ctx context.Context, target string, client *http.Client) (err error) {
	if err := validateHealthcheckTarget(target); err != nil {
		return err
	}
	probeClient := *client
	probeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	// validateHealthcheckTarget permits only this process's loopback health endpoint.
	//nolint:gosec // the destination is constrained before the request is sent.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	//nolint:gosec // the destination is constrained before the request is sent.
	resp, err := probeClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if _, readErr := io.Copy(io.Discard, resp.Body); readErr != nil {
		return fmt.Errorf("read health endpoint %s: %w", target, readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint %s returned %s", target, resp.Status)
	}
	return nil
}

func validateHealthcheckTarget(target string) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.User != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.Path != "/healthz" || parsed.RawQuery != "" || parsed.Fragment != "" || !loopbackHost(parsed.Hostname()) {
		return errors.New("healthcheck target must be the local HTTP health endpoint")
	}
	return nil
}

func loopbackHost(host string) bool {
	return host == "localhost" || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}

// dictionaryRiskIndex exposes the optional dictionary as the lemma-risk index.
// An absent dictionary must stay a nil interface, not a typed nil, or detection
// would call into a missing index.
func dictionaryRiskIndex(index *dictionary.Index, found bool) lemmarisk.AlternativeIndex {
	if !found {
		return nil
	}
	return index
}

// openOptionalDictionaryIndex opens the index named by MOUSEION_DICTIONARY_INDEX.
// An unset variable is the same absent state as a missing file.
func openOptionalDictionaryIndex(ctx context.Context) (*dictionary.Index, bool, error) {
	path := strings.TrimSpace(os.Getenv("MOUSEION_DICTIONARY_INDEX"))
	if path == "" {
		return nil, false, nil
	}
	return openDictionaryIndex(ctx, path)
}

// openDictionaryIndex loads the optional local dictionary index. An absent file
// means the operator has not derived the index, so glosses fall back to the
// morphology heuristic; it reports found=false and is a successful startup mode.
// A path that exists but is not a usable index is a misconfiguration and must
// not silently degrade card output.
func openDictionaryIndex(ctx context.Context, path string) (index *dictionary.Index, found bool, err error) {
	index, err = dictionary.OpenIndex(ctx, path)
	if err == nil {
		log.Print("dictionary index loaded")
		if skipped := index.SkippedGreekHeadwords(); skipped > 0 {
			//nolint:gosec // the count is an integer derived from the index, not user text.
			log.Printf("dictionary index skipped %d Ancient Greek headwords that failed admission", skipped)
		}
		return index, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		log.Print("dictionary index not found; continuing without local glosses")
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("invalid dictionary index at %s: %w", path, err)
}

func registerPreparedDeckWorkers(workers *river.Workers, store *persistence.PostgresStore, client *river.Client[pgx.Tx], provider *enrichment.OpenAIBatchClient, codec *enrichment.TranslationCodec, pollInterval time.Duration, metrics prepareddeck.BatchMetrics) {
	registerPreparedDeckWorkersWithStandard(workers, store, client, provider, codec, pollInterval, metrics, nil, prepareddeck.PreparedDeckConfig{}, 0)
}

func registerPreparedDeckWorkersWithStandard(workers *river.Workers, store *persistence.PostgresStore, client *river.Client[pgx.Tx], provider *enrichment.OpenAIBatchClient, codec *enrichment.TranslationCodec, pollInterval time.Duration, metrics prepareddeck.BatchMetrics, translationProvider enrichment.TranslationProvider, preparedDeckConfig prepareddeck.PreparedDeckConfig, llmTimeout time.Duration) {
	prepareddeck.AddStandardTranslationWorkerWithDependencies(workers, store, client, translationProvider, preparedDeckConfig, llmTimeout, metrics)
	prepareddeck.AddBatchSubmitWorkerWithMetrics(workers, store, client, provider, codec, metrics)
	prepareddeck.AddBatchPollWorker(workers, &prepareddeck.BatchPollWorker{Store: store, Client: client, Provider: provider, Codec: codec, PollInterval: pollInterval, Metrics: metrics})
	prepareddeck.AddFinalizeWorker(workers, &prepareddeck.DurableFinalizer{Store: store, Renderer: cardexport.NewPresentation(nil), Metrics: metrics})
	prepareddeck.AddRerenderWorker(workers, &prepareddeck.DurableRerenderer{Store: store, Renderer: cardexport.NewPresentation(nil)})
	prepareddeck.AddBatchCleanupWorker(workers, &prepareddeck.BatchCleanupWorker{Store: store, Provider: provider, Metrics: metrics})
	prepareddeck.AddRecoveryWorkerWithMetrics(workers, store, client, pollInterval, metrics)
}
