// Command server is the web-only v1 client over the shared Go core.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
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

func main() {
	if err := persistence.ValidateSecret(os.Getenv("MOUSEION_SECRET")); err != nil {
		log.Fatalf("invalid MOUSEION_SECRET: %v", err)
	}
	databaseURL := os.Getenv("MOUSEION_DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("MOUSEION_DATABASE_URL is required")
	}
	if err := persistence.Migrate(databaseURL); err != nil {
		log.Fatal(err)
	}
	store, err := persistence.Open(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	addr := os.Getenv("MOUSEION_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "ok")
	})
	secureCookies := strings.EqualFold(os.Getenv("MOUSEION_COOKIE_SECURE"), "true")
	lifetime := 24 * time.Hour
	authService := auth.New(store, lifetime)
	authHandler := webauth.New(authService, secureCookies, lifetime)
	epubService := epub.NewService(store)
	opdsService := opds.NewService(store, epubService, nil)
	nlp, err := analyzer.NewGRPCAnalyzer("")
	if err != nil {
		log.Fatal(err)
	}
	defer nlp.Close()
	selectionService := selection.NewService(store)
	llmConfig, err := enrichment.LLMConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	var dictionaryIndex *dictionary.Index
	if path := strings.TrimSpace(os.Getenv("MOUSEION_DICTIONARY_INDEX")); path != "" {
		dictionaryIndex, err = dictionary.OpenIndex(path)
		if err != nil {
			log.Printf("dictionary index unavailable, continuing without local glosses: %v", err)
		}
	}
	if dictionaryIndex != nil {
		defer dictionaryIndex.Close()
	}
	translationProvider, err := enrichment.NewConfiguredLLMProvider(llmConfig, nil)
	if err != nil {
		log.Fatal(err)
	}
	enrichmentService := enrichment.NewService(enrichment.Config{ExternalEnabled: llmConfig.Enabled, UserOptIn: true, ContextMode: enrichment.SentenceContext}, nil, nil, nil, translationProvider, store)
	capabilities := analyzer.NewCachedCapabilityProvider(nlp, 5*time.Minute)
	batchConfig, err := prepareddeck.BatchConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	preparedDeckConfig, err := prepareddeck.PreparedDeckConfigFromEnv()
	if err != nil {
		log.Fatalf("invalid prepared-deck configuration: %v", err)
	}
	var batchProvider *enrichment.OpenAIBatchClient
	var batchCodec *enrichment.TranslationCodec
	if llmConfig.Enabled {
		batchCodec, err = enrichment.NewTranslationCodec(llmConfig)
		if err != nil {
			log.Fatal(err)
		}
		batchProvider, err = enrichment.NewOpenAIBatchClient(llmConfig, nil)
		if err != nil {
			log.Fatalf("prepared-deck external translation requires the official OpenAI Batch endpoint: %v", err)
		}
	}
	batchMetrics := prepareddeck.NewMetricsCollector()
	workers := river.NewWorkers()
	knownvocab.AddWorker(workers, store.Pool())
	enrichmentjob.AddWorker(workers, store.Pool(), enrichmentService)
	cataloguesync.AddWorker(workers, store, opdsService, capabilities)
	exportService := cardexport.NewServiceWithDictionary(store, dictionaryIndex)
	riverClient, err := analysis.NewClientWithPreparedDeckConcurrency(store.Pool(), nlp, capabilities, selectionService, preparedDeckConfig.StandardMaxConcurrency, workers)
	if err != nil {
		log.Fatal(err)
	}
	prepareddeck.AddPreparedDeckWorker(workers, store, exportService, riverClient, batchCodec, batchConfig, preparedDeckConfig, llmConfig.Enabled)
	registerPreparedDeckWorkersWithStandard(workers, store, exportService, riverClient, batchProvider, batchCodec, batchConfig.PollInterval, batchMetrics, translationProvider, preparedDeckConfig, llmConfig.Timeout)
	if err = prepareddeck.EnsureRecoveryJob(context.Background(), store, riverClient); err != nil {
		log.Fatal(err)
	}
	if err = riverClient.Start(context.Background()); err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := riverClient.Stop(context.Background()); err != nil {
			log.Printf("stop analysis workers: %v", err)
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
	catalogueSyncService := cataloguesync.NewService(store, riverClient, opdsService, capabilities)
	if err = catalogueSyncService.RegisterAll(context.Background()); err != nil {
		log.Fatal(err)
	}
	mux.Handle("/static/", webapp.StaticHandler())
	webHandler, err := webapp.NewWithError(webapp.Services{Auth: authService, WebAuth: authHandler, Store: store, OPDS: opdsService, Analysis: analysisService, AnalysisInsights: analysisinsights.NewService(store), KnownVocab: knownVocabService, Enrichment: externalEnrichmentService, PreparedDeck: preparedDeckService, Capabilities: capabilities, CatalogueSync: catalogueSyncService, SecureCookies: secureCookies, SessionLifetime: lifetime})
	if err != nil {
		log.Fatalf("initialize web application: %v", err)
	}
	mux.Handle("/", webHandler)
	log.Printf("mouseion web server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func registerPreparedDeckWorkers(workers *river.Workers, store *persistence.PostgresStore, export *cardexport.Service, client *river.Client[pgx.Tx], provider *enrichment.OpenAIBatchClient, codec *enrichment.TranslationCodec, pollInterval time.Duration, metrics prepareddeck.BatchMetrics) {
	registerPreparedDeckWorkersWithStandard(workers, store, export, client, provider, codec, pollInterval, metrics, nil, prepareddeck.PreparedDeckConfig{}, 0)
}

func registerPreparedDeckWorkersWithStandard(workers *river.Workers, store *persistence.PostgresStore, export *cardexport.Service, client *river.Client[pgx.Tx], provider *enrichment.OpenAIBatchClient, codec *enrichment.TranslationCodec, pollInterval time.Duration, metrics prepareddeck.BatchMetrics, translationProvider enrichment.TranslationProvider, preparedDeckConfig prepareddeck.PreparedDeckConfig, llmTimeout time.Duration) {
	prepareddeck.AddStandardTranslationWorkerWithDependencies(workers, store, client, translationProvider, preparedDeckConfig, llmTimeout, metrics)
	prepareddeck.AddBatchSubmitWorkerWithMetrics(workers, store, client, provider, codec, metrics)
	prepareddeck.AddBatchPollWorker(workers, &prepareddeck.BatchPollWorker{Store: store, Client: client, Provider: provider, Codec: codec, PollInterval: pollInterval, Metrics: metrics})
	prepareddeck.AddFinalizeWorker(workers, &prepareddeck.DurableFinalizer{Store: store, Renderer: export, Metrics: metrics})
	prepareddeck.AddBatchCleanupWorker(workers, &prepareddeck.BatchCleanupWorker{Store: store, Provider: provider, Metrics: metrics})
	prepareddeck.AddRecoveryWorkerWithMetrics(workers, store, client, pollInterval, metrics)
}
