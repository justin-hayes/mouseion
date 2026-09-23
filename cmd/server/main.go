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
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/enrichmentjob"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/selection"
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

func run() (err error) {
	ctx := context.Background()
	if err := persistence.ValidateSecret(os.Getenv("MOUSEION_SECRET")); err != nil {
		return fmt.Errorf("invalid MOUSEION_SECRET: %w", err)
	}
	databaseURL := os.Getenv("MOUSEION_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("MOUSEION_DATABASE_URL is required")
	}
	if err := persistence.Migrate(databaseURL); err != nil {
		return err
	}
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer closeIntoResult("PostgreSQL store", store.Close, &err)
	addr := os.Getenv("MOUSEION_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprintln(w, "ok"); err != nil {
			log.Printf("write health response: %v", err)
		}
	})
	secureCookies := strings.EqualFold(os.Getenv("MOUSEION_COOKIE_SECURE"), "true")
	lifetime := 24 * time.Hour
	authService := auth.New(store, lifetime)
	authHandler := webauth.New(authService, secureCookies, lifetime)
	epubService := epub.NewService(store)
	opdsService := opds.NewService(store, epubService, nil)
	nlp, err := analyzer.NewGRPCAnalyzer("")
	if err != nil {
		return err
	}
	defer closeIntoResult("NLP analyzer", nlp.Close, &err)
	selectionService := selection.NewService(store)
	llmConfig, err := enrichment.LLMConfigFromEnv()
	if err != nil {
		return err
	}
	var dictionaryIndex *dictionary.Index
	if path := strings.TrimSpace(os.Getenv("MOUSEION_DICTIONARY_INDEX")); path != "" {
		dictionaryIndex, err = openDictionaryIndex(ctx, path)
		if err != nil {
			return err
		}
	}
	if dictionaryIndex != nil {
		defer closeIntoResult("dictionary index", dictionaryIndex.Close, &err)
	}
	translationProvider, err := enrichment.NewConfiguredLLMProvider(llmConfig, nil)
	if err != nil {
		return err
	}
	enrichmentService := enrichment.NewService(enrichment.Config{ExternalEnabled: llmConfig.Enabled, UserOptIn: true, ContextMode: enrichment.SentenceContext}, nil, nil, nil, translationProvider, store)
	capabilities := analyzer.NewCachedCapabilityProvider(nlp, 5*time.Minute)
	batchConfig, err := prepareddeck.BatchConfigFromEnv()
	if err != nil {
		return err
	}
	preparedDeckConfig, err := prepareddeck.PreparedDeckConfigFromEnv()
	if err != nil {
		return fmt.Errorf("invalid prepared-deck configuration: %w", err)
	}
	var batchProvider *enrichment.OpenAIBatchClient
	var batchCodec *enrichment.TranslationCodec
	if llmConfig.Enabled {
		batchCodec, err = enrichment.NewTranslationCodec(llmConfig)
		if err != nil {
			return err
		}
		batchProvider, err = enrichment.NewOpenAIBatchClient(llmConfig, nil)
		if err != nil {
			return fmt.Errorf("prepared-deck external translation requires the official OpenAI Batch endpoint: %w", err)
		}
	}
	batchMetrics := prepareddeck.NewMetricsCollector()
	catalogueSyncDeps := cataloguesync.StoreDependencies{Connections: store, Catalogue: store, Aliases: store, Statuses: store, Pool: store.Pool()}
	workers := river.NewWorkers()
	knownvocab.AddWorker(workers, store.Pool())
	enrichmentjob.AddWorker(workers, store.Pool(), enrichmentService)
	catalogueWorker := cataloguesync.AddWorker(workers, catalogueSyncDeps, opdsService, capabilities)
	presentation := cardexport.NewPresentation(dictionaryIndex)
	riverClient, err := analysis.NewClientWithPreparedDeckConcurrency(store.Pool(), nlp, capabilities, selectionService, preparedDeckConfig.StandardMaxConcurrency, workers)
	if err != nil {
		return err
	}
	catalogueWorker.Client = riverClient
	prepareddeck.AddPreparedDeckWorker(workers, store, presentation, riverClient, batchCodec, batchConfig, preparedDeckConfig, llmConfig.Enabled)
	registerPreparedDeckWorkersWithStandard(workers, store, riverClient, batchProvider, batchCodec, batchConfig.PollInterval, batchMetrics, translationProvider, preparedDeckConfig, llmConfig.Timeout)
	if err = prepareddeck.EnsureRecoveryJob(ctx, store, riverClient); err != nil {
		return err
	}
	if err = riverClient.Start(ctx); err != nil {
		return err
	}
	defer func() {
		if stopErr := riverClient.Stop(ctx); stopErr != nil {
			stopErr = fmt.Errorf("stop analysis workers: %w", stopErr)
			if err == nil {
				err = stopErr
			} else {
				err = errors.Join(err, stopErr)
			}
		}
	}()
	analysisService := analysis.NewService(store.Pool(), riverClient)
	knownVocabService := knownvocab.NewJobService(store.Pool(), riverClient)
	externalEnrichmentService := enrichmentjob.NewService(store.Pool(), riverClient, enrichmentService)
	var preparedDeckService *prepareddeck.Service
	if batchProvider != nil {
		preparedDeckService = prepareddeck.NewServiceWithBatchCanceller(store, riverClient, batchProvider)
	} else {
		preparedDeckService = prepareddeck.NewService(store, riverClient)
	}
	catalogueSyncService := cataloguesync.NewService(catalogueSyncDeps, riverClient, opdsService, capabilities)
	if err = catalogueSyncService.RegisterAll(ctx); err != nil {
		return err
	}
	mux.Handle("/static/", webapp.StaticHandler())
	storeDeps := webapp.StoreDependencies{StudyLanguages: store, Books: store, Journey: store, Goals: store, Catalog: store, AnalysisJobs: store, Covers: store}
	webHandler, err := webapp.NewWithError(webapp.Services{Auth: authService, WebAuth: authHandler, Store: storeDeps, OPDS: opdsService, Analysis: analysisService, AnalysisInsights: analysisinsights.NewService(store), KnownVocab: knownVocabService, Enrichment: externalEnrichmentService, PreparedDeck: preparedDeckService, Capabilities: capabilities, CatalogueSync: catalogueSyncService, SecureCookies: secureCookies, SessionLifetime: lifetime})
	if err != nil {
		return fmt.Errorf("initialize web application: %w", err)
	}
	mux.Handle("/", webHandler)
	log.Print("mouseion web server listening")
	return newHTTPServer(addr, mux).ListenAndServe()
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

// openDictionaryIndex loads the optional local dictionary index. An absent file
// means the operator has not derived the index, so glosses fall back to the
// morphology heuristic. A path that exists but is not a usable index is a
// misconfiguration and must not silently degrade card output.
func openDictionaryIndex(ctx context.Context, path string) (*dictionary.Index, error) {
	index, err := dictionary.OpenIndex(ctx, path)
	if err == nil {
		log.Print("dictionary index loaded")
		return index, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		log.Print("dictionary index not found; continuing without local glosses")
		// The local dictionary is optional; absence is a successful startup mode.
		//nolint:nilnil // nil index explicitly represents the absent optional index.
		return nil, nil
	}
	return nil, fmt.Errorf("invalid dictionary index at %s: %w", path, err)
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
