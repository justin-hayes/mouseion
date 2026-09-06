// Command fixtureserver serves the deterministic browser-acceptance fixture.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/webapp"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

func main() {
	if os.Getenv("MOUSEION_SECRET") == "" {
		_ = os.Setenv("MOUSEION_SECRET", "fixture-server-secret-0123456789")
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
	h, err := webapp.NewWithError(webapp.Services{
		Auth: authService, WebAuth: authHandler, Store: store, OPDS: fixtures.OPDS{},
		Analysis: fixtures.Analysis{}, AnalysisInsights: fixtures.Insights{JourneyStore: store}, KnownVocab: fixtures.KnownVocab{},
		Enrichment: fixtures.Enrichment{}, PreparedDeck: fixtures.PreparedDeck{}, Capabilities: fixtures.Capabilities{},
		CatalogueSync: catalogueSync,
		SecureCookies: false, SessionLifetime: auth.DefaultSessionLifetime,
	})
	if err != nil {
		log.Fatalf("initialize fixture webapp: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprintln(w, "ok") })
	mux.Handle("/static/", webapp.StaticHandler())
	mux.Handle("/", h)
	log.Printf("mouseion fixture server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
