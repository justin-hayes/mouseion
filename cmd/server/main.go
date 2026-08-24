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

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
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
	translationProvider, err := enrichment.NewConfiguredLLMProvider(llmConfig, nil)
	if err != nil {
		log.Fatal(err)
	}
	enrichmentService := enrichment.NewService(enrichment.Config{ExternalEnabled: llmConfig.Enabled, UserOptIn: true, ContextMode: enrichment.SentenceContext}, nil, nil, nil, translationProvider, store)
	workers := river.NewWorkers()
	knownvocab.AddWorker(workers, store.Pool())
	enrichmentjob.AddWorker(workers, store.Pool(), enrichmentService)
	exportService := cardexport.NewService(store)
	prepareddeck.AddWorker(workers, store, exportService, enrichmentService)
	riverClient, err := analysis.NewClient(store.Pool(), nlp, selectionService, workers)
	if err != nil {
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
	preparedDeckService := prepareddeck.NewService(store, riverClient)
	mux.Handle("/static/", webapp.StaticHandler())
	mux.Handle("/", webapp.New(webapp.Services{Auth: authService, WebAuth: authHandler, Store: store, OPDS: opdsService, Analysis: analysisService, KnownVocab: knownVocabService, CardExport: exportService, Enrichment: externalEnrichmentService, PreparedDeck: preparedDeckService, SecureCookies: secureCookies, SessionLifetime: lifetime}))
	log.Printf("mouseion web server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
