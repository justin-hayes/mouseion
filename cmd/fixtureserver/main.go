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
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
	"github.com/justin-hayes/mouseion/internal/readingintent"
	"github.com/justin-hayes/mouseion/internal/study"
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
	store.SeedWithdrawnClassicalBooks()
	// The Books present one German word; Study's browser journey needs a batch
	// of several, so seed more.
	for i, lemma := range []string{"haus", "hund"} {
		store.SeedStudyCandidate(fixtures.OwnerID, domain.StudyCandidate{
			Language: "de", CanonicalLemma: lemma, UPOS: "NOUN",
			PersonalOccurrences: int64(10 - i),
		})
	}
	seedFluencyWordIfRequested(store)
	classicalCatalog, err := fixtures.NewClassicalCatalog()
	if err != nil {
		log.Fatalf("initialize fixture Classical catalog: %v", err)
	}
	books := fixtureBooksStore{Store: store}
	catalogueSync := fixtures.NewCatalogueSync(store)
	storeDeps := webapp.StoreDependencies{Shell: store, MyBooks: books, Reading: books, Vocabulary: store, Catalogs: store, CatalogCovers: store, CatalogueAdd: store, CatalogueRead: store, Jobs: store, Study: store}
	preparedDeck := fixtures.PreparedDeck{Store: store}
	lemmaReview := lemmareview.New(lemmareview.Config{
		Store: books, Suggestions: fixtureLemmaSuggestions{}, Preparer: preparedDeck, BrowseCounts: fixtures.Analysis{Store: store},
	})
	fixtureOPDS := fixtures.OPDS{}
	preparation, err := readingintent.NewPreparation(readingintent.PreparationConfig{
		Classical:       fixtures.ClassicalPreparation{},
		Analysis:        fixtures.Analysis{Store: store},
		EPUBAcquisition: fixtures.EPUBAcquisition{},
	})
	if err != nil {
		log.Fatalf("initialize fixture reading intent preparation: %v", err)
	}
	readingIntent, err := readingintent.New(readingintent.Config{
		Books:           books,
		CurrentReadings: books,
		Dispositions:    books,
		Preparation:     preparation,
	})
	if err != nil {
		log.Fatalf("initialize fixture reading intent: %v", err)
	}
	h, err := webapp.NewWithError(webapp.Services{
		Auth: authService, WebAuth: authHandler, Store: storeDeps, OPDS: fixtureOPDS,
		Analysis: fixtures.Analysis{Store: store}, AnalysisInsights: fixtures.ClassicalInsights{},
		PreparedDeck: preparedDeck, Study: fixtures.Study{Store: store}, StudyScheduler: study.NewScheduler(), Capabilities: fixtures.Capabilities{},
		ClassicalPreparation: fixtures.ClassicalPreparation{}, ReadingIntent: readingIntent,
		Classical:     classicalCatalog,
		CatalogueSync: catalogueSync, LemmaReview: lemmaReview,
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
	// Browser acceptance selects the Working desk's degraded and accounted states
	// here; the route exists only in the fixture server.
	mux.HandleFunc("POST /fixture/vocabulary-browse-scenario", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.SetVocabularyBrowseScenario(r.PostForm.Get("scenario")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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

// fixtureBooksStore exposes one deterministic real handler failure for browser
// acceptance of enhanced My Books error responses.
type fixtureBooksStore struct{ *fixtures.Store }

func (s fixtureBooksStore) ListMyBooksBrowse(ctx context.Context, owner, query, language, disposition string, history bool, offset, limit int) (domain.MyBooksBrowseResult, error) {
	if query == "fixture-handler-error" {
		return domain.MyBooksBrowseResult{}, errors.New("fixture My Books read failure")
	}
	return s.Store.ListMyBooksBrowse(ctx, owner, query, language, disposition, history, offset, limit)
}

type fixtureLemmaSuggestions struct{}

func (fixtureLemmaSuggestions) Name() string    { return "fixture-lemma-model" }
func (fixtureLemmaSuggestions) Version() string { return "fixture-v1" }
func (fixtureLemmaSuggestions) Enabled() bool   { return true }
func (fixtureLemmaSuggestions) SuggestLemma(_ context.Context, request enrichment.LemmaSuggestionRequest) (enrichment.LemmaSuggestion, error) {
	if request.LexicalAlternative != "" {
		return enrichment.LemmaSuggestion{}, errors.New("fixture provider unavailable")
	}
	if request.Surface == "Weg" {
		return enrichment.LemmaSuggestion{Lemma: "Pfad"}, nil
	}
	return enrichment.LemmaSuggestion{}, errors.New("fixture provider unavailable")
}

// seedFluencyWordIfRequested seeds the Fluency journey's word when the browser
// spec asks for it, so every other spec keeps its usual Study state.
func seedFluencyWordIfRequested(store *fixtures.Store) {
	if os.Getenv("MOUSEION_FIXTURE_STUDY_FLUENCY") == "1" {
		seedFluencyWord(store)
	}
}

// seedFluencyWord gives the learner a mature German word with two Fluency
// contexts, so the browser journey can show a Fluency card without weeks of
// reviews. The fixture server binds no provider or River to prepare them.
func seedFluencyWord(store *fixtures.Store) {
	identity := domain.StudyIdentity{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB"}
	progressID := store.SeedMatureStudyWord(fixtures.OwnerID, identity, domain.StudyMaterialReadyContent{
		BookID: fixtures.BookID, AnalysisRunID: fixtures.ResultRunID, CorpusID: "fixture-corpus",
		SentenceOrdinal: 1, TokenOrdinal: 1,
		Sentence: "Ich gehe jeden Morgen zur Arbeit.", TargetWord: "gehe",
		Gloss: "to go", SentenceTranslation: "I go to work every morning.", SentenceTranslationTargets: []string{"go"},
	}, time.Now())
	for i, context := range []domain.FluencyContextRecord{
		{Sentence: "Gestern ging er allein nach Hause.", TargetWord: "ging", SentenceTranslation: "Yesterday he went home alone.", SentenceTranslationTargets: []string{"went"}, Form: domain.FluencyForm{Surface: "ging", Features: "Tense=Past"}},
		{Sentence: "Wir gehen morgen zusammen ins Kino.", TargetWord: "gehen", SentenceTranslation: "Tomorrow we are going to the cinema together.", SentenceTranslationTargets: []string{"going"}, Form: domain.FluencyForm{Surface: "gehen", Features: "Number=Plur"}},
	} {
		context.Agreement, context.Gloss = domain.FluencyAgrees, "to go"
		context.BookID, context.AnalysisRunID, context.CorpusID = fixtures.BookID, fixtures.ResultRunID, "fixture-corpus"
		context.SentenceOrdinal, context.TokenOrdinal = int64(10+i), 1
		if err := store.SeedFluencyContext(fixtures.OwnerID, progressID, context); err != nil {
			log.Fatalf("seed Fluency context: %v", err)
		}
	}
}
