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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/deploysecret"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/readingintent"
	"github.com/justin-hayes/mouseion/internal/sourcecheck"
	"github.com/justin-hayes/mouseion/internal/study"
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
	// acquisitionTempDir holds Catalog files while they are acquired. Empty
	// uses the system temporary directory.
	acquisitionTempDir string
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
	catalogueSyncDeps cataloguesync.StoreDependencies
	riverClient       *river.Client[pgx.Tx]
	studyService      *study.Service
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
	if err := classical.Reconcile(ctx, store, classical.ManifestJSONL); err != nil {
		return fmt.Errorf("reconcile Classical catalog manifest: %w", err)
	}
	log.Print("classical catalog manifest reconciled")
	conversion, err := analysis.ConvertLegacyClassicalBooks(ctx, store)
	if err != nil {
		// Conversion is a fail-closed catalog precondition, not a startup gate:
		// existing Books stay readable and Add/preparation remain unavailable
		// until a later start converts the legacy rows.
		log.Printf("classical catalog legacy conversion could not run; catalog remains fail-closed: %v", err)
	} else {
		log.Printf("classical catalog legacy conversion: converted=%d unpublished=%d withdrawn=%d evidence_differs=%d skipped=%d",
			conversion.Converted, conversion.Unpublished, conversion.Withdrawn, conversion.EvidenceDiffers, conversion.Skipped)
	}
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
	acquisitionTempDir, err := acquisitionTempDirFromEnv()
	if err != nil {
		return serverConfig{}, err
	}
	return serverConfig{
		databaseURL:        databaseURL,
		addr:               addr,
		secureCookies:      strings.EqualFold(os.Getenv("MOUSEION_COOKIE_SECURE"), "true"),
		sessionLifetime:    24 * time.Hour,
		acquisitionTempDir: acquisitionTempDir,
	}, nil
}

// acquisitionTempDirFromEnv resolves where acquisition streams Catalog files.
// An explicitly configured directory must already exist, so a typo stops
// startup instead of the first Move to To Read.
func acquisitionTempDirFromEnv() (string, error) {
	dir := strings.TrimSpace(os.Getenv("MOUSEION_ACQUISITION_TEMP_DIR"))
	if dir == "" {
		return "", nil
	}
	info, err := os.Stat(dir) //nolint:gosec // the operator configures this directory; it is not untrusted request input.
	if err != nil {
		return "", fmt.Errorf("invalid MOUSEION_ACQUISITION_TEMP_DIR: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("invalid MOUSEION_ACQUISITION_TEMP_DIR: not a directory")
	}
	return dir, nil
}

// migrateAndOpenStore migrates before opening, so the shared store only ever
// talks to the current schema.
func migrateAndOpenStore(ctx context.Context, databaseURL string) (*persistence.PostgresStore, error) {
	if err := persistence.Migrate(databaseURL); err != nil {
		return nil, err
	}
	return persistence.Open(ctx, databaseURL)
}

// registerClassicalWorkers registers the Classical preparation and translation
// workers over the embedded catalog layers and manifest, and returns the
// preparation worker so its River client can be attached later.
func registerClassicalWorkers(workers *river.Workers, store *persistence.PostgresStore, tempDir string) (*analysis.ClassicalPreparationWorker, error) {
	layers, err := classical.ParseLayers(classical.LayersJSON)
	if err != nil {
		return nil, fmt.Errorf("parse Classical catalog layers: %w", err)
	}
	classicalDefinitions, err := classical.ParseDefinitions(classical.ManifestJSONL)
	if err != nil {
		return nil, fmt.Errorf("parse Classical catalog manifest: %w", err)
	}
	worker := analysis.AddClassicalPreparationWorker(workers, store, analysis.ClassicalPreparationConfig{
		Layers:      layers,
		Definitions: classicalDefinitions,
		TempDir:     tempDir,
	})
	analysis.AddClassicalTranslationWorker(workers, store, analysis.ClassicalPreparationConfig{
		Layers:  layers,
		TempDir: tempDir,
	})
	return worker, nil
}

// startServerDependencies builds every long-lived client and provider and
// starts the River client, in the order the request handlers and workers expect.
func startServerDependencies(ctx context.Context, cfg serverConfig, store *persistence.PostgresStore, nlp *analyzer.GRPCAnalyzer, dictionaryIndex *dictionary.Index, llmConfig enrichment.LLMConfig) (*serverDependencies, error) {
	authService := auth.New(store, cfg.sessionLifetime)
	authHandler := webauth.New(authService, cfg.secureCookies, cfg.sessionLifetime)
	epubService := epub.NewService(store)
	opdsService := opds.NewService(store, epubService, nil).WithAcquisitionTempDir(cfg.acquisitionTempDir)
	translationProvider, err := enrichment.NewConfiguredLLMProvider(llmConfig, nil)
	if err != nil {
		return nil, err
	}
	capabilities := analyzer.NewCachedCapabilityProvider(nlp, 5*time.Minute)
	idiomLanguages, err := study.IdiomLanguagesFromEnv()
	if err != nil {
		return nil, err
	}
	var codec *enrichment.TranslationCodec
	if llmConfig.Enabled {
		codec, err = enrichment.NewTranslationCodec(llmConfig)
		if err != nil {
			return nil, err
		}
	}
	catalogueSyncDeps := cataloguesync.StoreDependencies{Connections: store, Catalogue: store, Statuses: store, Pool: store.Pool()}
	workers := river.NewWorkers()
	catalogueWorker := cataloguesync.AddWorker(workers, catalogueSyncDeps, opdsService, capabilities)
	sourcecheck.AddWorker(workers, store)
	classicalWorker, err := registerClassicalWorkers(workers, store, cfg.acquisitionTempDir)
	if err != nil {
		return nil, err
	}
	acquisitionWorker := registerEPUBAcquisitionWorker(workers, store, catalogueSyncDeps, opdsService, capabilities)
	studyIdioms, err := newStudyIdioms(ctx, dictionaryIndex, idiomLanguages, codec)
	if err != nil {
		return nil, err
	}
	studyPlanWorker := registerStudyWorker(workers, store, translationProvider, codec, dictionaryIndex, studyIdioms)
	riverClient, err := newRiverClient(ctx, store, nlp, capabilities, workers, dictionaryIndex)
	if err != nil {
		return nil, err
	}
	attachEPUBAcquisitionAnalysis(acquisitionWorker, store.Pool(), riverClient)
	// The preparation worker enqueues the separate translation import only after
	// the analysis publishes, so the client is attached after construction.
	classicalWorker.Client = riverClient
	sourceStructureChecks := sourcecheck.NewService(store, riverClient, sourcecheck.ConfiguredTarget())
	epubService.WithSourceStructureCheckEnqueuer(sourceStructureChecks)
	catalogueWorker.Client = riverClient
	studyService := study.NewService(store, riverClient).WithDictionaryPin(studyDictionaryPin(dictionaryIndex)).WithIdioms(studyIdioms)
	studyPlanWorker.Planner = studyService
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
		catalogueSyncDeps: catalogueSyncDeps,
		riverClient:       riverClient,
		studyService:      studyService,
	}, nil
}

// idiomIndexVersion stamps Idiom candidate counts with the matching rules, the
// dictionary index version and a digest of its idiom entries (which carry the
// reviewed idiom list), so a change to any of them rebuilds the counts.
func idiomIndexVersion(ctx context.Context, dictionaryIndex *dictionary.Index) (string, error) {
	digest, err := dictionaryIndex.IdiomDigest(ctx)
	if err != nil {
		return "", fmt.Errorf("fingerprint idiom entries: %w", err)
	}
	return persistence.IdiomIndexVersion(dictionaryIndex.Version(), digest), nil
}

// newRiverClient registers the Idiom candidate count rebuild, which derives
// counts against the loaded dictionary index and stamps them with its idiom
// version, and builds the River client over the complete worker set.
func newRiverClient(ctx context.Context, store *persistence.PostgresStore, nlp *analyzer.GRPCAnalyzer, capabilities analyzer.CapabilityProvider, workers *river.Workers, dictionaryIndex *dictionary.Index) (*river.Client[pgx.Tx], error) {
	version, err := idiomIndexVersion(ctx, dictionaryIndex)
	if err != nil {
		return nil, err
	}
	analysis.AddIdiomCountsWorker(workers, store.Pool(), dictionaryIndex, version)
	return analysis.NewClient(store.Pool(), nlp, capabilities, workers)
}

// studyDictionaryPin reads the loaded dictionary index's pin for a language, so
// Study tries a no-dictionary-entry word again when the index is rebuilt. It is
// empty for a language with no dictionary, and without an index.
func studyDictionaryPin(dictionaryIndex *dictionary.Index) func(language string) string {
	return func(language string) string {
		if dictionaryIndex == nil {
			return ""
		}
		return study.DictionaryPin(dictionaryIndex, cardpolicy.For(language))
	}
}

// newStudyIdioms is Study's Idiom switch over the dictionary's idiom index: a
// language's Idioms are candidates, and its words' sentences avoid Idiom
// candidates, only when the operator enabled it and its idiom-card payload
// assessment is recorded, the gate ADR 0137 sets. The
// counts it reads are the ones stamped with the same idiom index version. It is
// nil without a dictionary index or an enabled language.
func newStudyIdioms(ctx context.Context, dictionaryIndex *dictionary.Index, idiomLanguages map[string]bool, codec *enrichment.TranslationCodec) (*study.IdiomAvoidance, error) {
	var idioms *study.IdiomAvoidance
	if dictionaryIndex == nil || len(idiomLanguages) == 0 {
		return idioms, nil
	}
	version, err := idiomIndexVersion(ctx, dictionaryIndex)
	if err != nil {
		return nil, err
	}
	return &study.IdiomAvoidance{Index: dictionaryIndex, Enabled: study.IdiomSwitch(idiomLanguages, enrichment.ProviderAssessments(), codec), IndexVersion: version}, nil
}

// registerStudyWorker registers the Study material worker. The translation
// provider is the configured one; Study asks it for a contextual Gloss
// only from a job, never from a request.
//
// It also registers the Fluency worker, which asks the provider whether another
// occurrence of a mature word uses the learned meaning. That payload sends the
// learned meaning, so the worker sends nothing until its provider payload
// assessment is recorded and matches the codec (ADR 0133).
//
// With a language's idiom switch on, a word's material and Fluency contexts
// prefer a sentence outside every Idiom candidate (ADR 0137 §3).
//
// A source-evidence language's material (ADR 0138) also reads the imported
// dictionary and the Book's passage translations, and sends its evidence-assisted
// pass payload only while that assessment is recorded. Its Fluency contexts run
// that pass per occurrence and then the separate source-evidence agreement
// variant, which is sent only while its own assessment is recorded. Without a dictionary
// index such a word waits instead of being recorded as having no entry.
func registerStudyWorker(workers *river.Workers, store *persistence.PostgresStore, provider enrichment.TranslationProvider, codec *enrichment.TranslationCodec, dictionaryIndex *dictionary.Index, idioms *study.IdiomAvoidance) *study.PlanWorker {
	enricher := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 3}, nil, nil, nil, provider, nil)
	assessments := enrichment.ProviderAssessments()
	evidence := &study.SourceEvidence{
		Translations: store,
		MaySend: func(language string) error {
			return assessments.MaySend(enrichment.StudyEvidenceAssistedPassKind(language), language, codec)
		},
	}
	if dictionaryIndex != nil {
		evidence.Lexical = dictionaryIndex
		evidence.Available = func(policy cardpolicy.Policy) bool {
			switch policy.Entries {
			case cardpolicy.EntryLSJ:
				return dictionaryIndex.AncientGreekLSJAvailable()
			case cardpolicy.EntryLewisShort:
				return dictionaryIndex.LatinLSAvailable()
			case cardpolicy.EntryNone:
			}
			return false
		}
	}
	study.AddMaterialWorker(workers, store, enricher, evidence, idioms)
	planWorker := study.AddPlanWorker(workers)
	study.AddFluencyWorker(workers, store, enricher, func(language string) error {
		return assessments.MaySend(enrichment.FluencyAgreementPayload, language, codec)
	}, evidence, func(language string) error {
		return assessments.MaySend(enrichment.FluencyAgreementSourceEvidencePayload, language, codec)
	}, idioms)
	return planWorker
}

// registerEPUBAcquisitionWorker wires the acquisition worker to the Catalog
// index and the OPDS downloader. Its target lookup needs no River client, so it
// is registered with the rest of the worker set.
func registerEPUBAcquisitionWorker(workers *river.Workers, store *persistence.PostgresStore, deps cataloguesync.StoreDependencies, opdsService *opds.Service, capabilities analyzer.CapabilityProvider) *analysis.EPUBAcquisitionWorker {
	catalogueIndex := cataloguesync.NewService(deps, nil, opdsService, capabilities)
	return analysis.AddEPUBAcquisitionWorker(workers, store, catalogueIndex.FindAcquisitionTarget, opdsService.AcquireForBook)
}

// attachEPUBAcquisitionAnalysis supplies the analysis submission the worker
// uses after an import, once the River client exists.
func attachEPUBAcquisitionAnalysis(worker *analysis.EPUBAcquisitionWorker, pool *pgxpool.Pool, client *river.Client[pgx.Tx]) {
	worker.Analysis = analysis.NewService(pool, client)
}

// buildWebHandler registers the catalogue-sync schedules, then assembles the web
// application from the dependencies the workers share.
func buildWebHandler(ctx context.Context, cfg serverConfig, store *persistence.PostgresStore, dictionaryIndex *dictionary.Index, hasDictionaryIndex bool, deps *serverDependencies) (*webapp.Handler, error) {
	analysisService := analysis.NewService(store.Pool(), deps.riverClient).WithCapabilities(deps.capabilities).WithClassicalPreparationStore(store)
	if err := analysisService.EnqueueMissingBrowseCountRebuilds(ctx); err != nil {
		return nil, fmt.Errorf("schedule missing Vocabulary Browse counts: %w", err)
	}
	idiomVersion, err := idiomIndexVersion(ctx, dictionaryIndex)
	if err != nil {
		return nil, err
	}
	if err := analysisService.EnqueueMissingIdiomCountRebuilds(ctx, idiomVersion); err != nil {
		return nil, fmt.Errorf("schedule missing Idiom candidate counts: %w", err)
	}
	if err := analysisService.EnqueueMissingConstructionRebuilds(ctx); err != nil {
		return nil, fmt.Errorf("schedule missing Construction occurrences: %w", err)
	}
	catalogueSyncService := cataloguesync.NewService(deps.catalogueSyncDeps, deps.riverClient, deps.opdsService, deps.capabilities)
	if err := catalogueSyncService.RegisterAll(ctx); err != nil {
		return nil, err
	}
	storeDeps := webapp.StoreDependencies{Shell: store, MyBooks: store, Reading: store, Vocabulary: store, Catalogs: store, CatalogCovers: store, CatalogueAdd: store, CatalogueRead: store, Jobs: store, Study: store}
	lemmaReview := lemmareview.New(lemmareview.Config{Store: store, BrowseCounts: analysisService})
	preparation, err := readingintent.NewPreparation(readingintent.PreparationConfig{
		Classical:       analysisService,
		Analysis:        analysisService,
		EPUBAcquisition: analysisService,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize reading intent preparation: %w", err)
	}
	readingIntent, err := readingintent.New(readingintent.Config{
		Books: store, CurrentReadings: store, Dispositions: store,
		Preparation: preparation,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize reading intent: %w", err)
	}
	var studyDictionary enrichment.LexicalProvider
	if hasDictionaryIndex {
		studyDictionary = dictionaryIndex
	}
	webHandler, err := webapp.NewWithError(webapp.Services{Dictionary: studyDictionary, Auth: deps.authService, WebAuth: deps.authHandler, Store: storeDeps, OPDS: deps.opdsService, Analysis: analysisService, AnalysisUpdates: analysisService, SourceStructures: analysisService, ClassicalPreparation: analysisService, EPUBAcquisition: analysisService, ReadingIntent: readingIntent, AnalysisInsights: analysisinsights.NewService(store), Study: deps.studyService, StudyScheduler: study.NewScheduler(), Capabilities: deps.capabilities, LemmaReview: lemmaReview, CatalogueSync: catalogueSyncService, SecureCookies: cfg.secureCookies, SessionLifetime: cfg.sessionLifetime, Clock: time.Now})
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
