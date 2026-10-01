// Command fixtureserver serves the deterministic browser-acceptance fixture.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/webapp"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

func main() {
	if os.Getenv("MOUSEION_SECRET") == "" {
		if err := os.Setenv("MOUSEION_SECRET", "fixture-server-secret-0123456789"); err != nil {
			log.Fatalf("set fixture server secret: %v", err)
		}
	}
	addr := os.Getenv("MOUSEION_FIXTURE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8099"
	}
	authStore := fixtures.NewAuthStore()
	authService := auth.New(authStore, auth.DefaultSessionLifetime)
	authHandler := webauth.New(authService, false, auth.DefaultSessionLifetime)
	store := fixtures.NewStore()
	catalogueSync := fixtures.NewCatalogueSync(store)
	storeDeps := webapp.StoreDependencies{StudyLanguages: store, Books: store, Goals: store, CurrentReading: store, Catalog: store, AnalysisJobs: store, Covers: store, LemmaReview: store, VocabularyBrowse: store}
	h, err := webapp.NewWithError(webapp.Services{
		Auth: authService, WebAuth: authHandler, Store: storeDeps, OPDS: fixtures.OPDS{},
		Analysis: fixtures.Analysis{}, AnalysisInsights: fixtures.Insights{}, KnownVocab: fixtures.KnownVocab{},
		Enrichment: fixtures.Enrichment{}, PreparedDeck: fixtures.PreparedDeck{Store: store}, Capabilities: fixtures.Capabilities{},
		CatalogueSync: catalogueSync, LemmaSuggestions: fixtureLemmaSuggestions{},
		SecureCookies: false, SessionLifetime: auth.DefaultSessionLifetime,
	})
	if err != nil {
		log.Fatalf("initialize fixture webapp: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprintln(w, "ok"); err != nil {
			log.Printf("write health response: %v", err)
		}
	})
	mux.Handle("/static/", webapp.StaticHandler())
	mux.Handle("/", h)
	log.Print("mouseion fixture server listening")
	log.Fatal((&http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}).ListenAndServe())
}

type fixtureLemmaSuggestions struct{}

func (fixtureLemmaSuggestions) Name() string    { return "fixture-lemma-model" }
func (fixtureLemmaSuggestions) Version() string { return "fixture-v1" }
func (fixtureLemmaSuggestions) SuggestLemma(_ context.Context, request enrichment.LemmaSuggestionRequest) (enrichment.LemmaSuggestion, error) {
	if request.LexicalAlternative != "" {
		return enrichment.LemmaSuggestion{}, errors.New("fixture provider unavailable")
	}
	if request.Surface == "Weg" {
		return enrichment.LemmaSuggestion{Lemma: "Pfad"}, nil
	}
	return enrichment.LemmaSuggestion{}, errors.New("fixture provider unavailable")
}
