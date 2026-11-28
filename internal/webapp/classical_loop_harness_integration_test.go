//go:build integration

package webapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"
)

type classicalLoopLayer struct {
	source, translation []byte
	failSource          atomic.Bool
	sourceAttempts      atomic.Int64
	sourceServed        atomic.Int64
	translationServed   atomic.Int64
}

// handler serves the layer. It is a closure over the layer rather than a
// ServeHTTP method: gosec's G705 taint pass resolves a ServeHTTP receiver
// through every http.Handler call site, misreads the ResponseWriter there as
// the receiver, and so flags these fixed test bytes nondeterministically.
func (l *classicalLoopLayer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "synthetic-translation.xml") {
			l.translationServed.Add(1)
			if _, err := w.Write(l.translation); err != nil {
				return
			}
			return
		}
		l.sourceAttempts.Add(1)
		if l.failSource.Load() {
			http.Error(w, "layer unavailable", http.StatusBadGateway)
			return
		}
		l.sourceServed.Add(1)
		if _, err := w.Write(l.source); err != nil {
			return
		}
	})
}

// loopTreatiseKey is the Work and whole-work unit key of the multi-book
// fixture Work used by the whole-Work loop.
const loopTreatiseKey = "synthetic-treatise"

// classicalLoopSeed is the pre-upgrade Anabasis state conversion must handle.
type classicalLoopSeed struct {
	ownerID, bookID, sourceID, runID string
}

type classicalLoopHarness struct {
	ctx          context.Context
	store        *persistence.PostgresStore
	pool         *pgxpool.Pool
	handler      *Handler
	service      *analysis.Service
	client       *river.Client[pgx.Tx]
	layer        *classicalLoopLayer
	definitions  []classical.Definition
	unitOneKey   string
	unitTwoKey   string
	wholeUnitKey string
	anabasis     classicalLoopSeed
	logs         *lockedBuffer
	translation  *classical.TranslationPin
}

// newClassicalLoopHarness seeds a pre-upgrade Anabasis Book, reconciles a
// fixture manifest through the same sequence cmd/server runs at startup,
// converts the Book, and wires the real web application to in-process workers.
// The workers are not started until the test has observed the queued jobs.
func newClassicalLoopHarness(t *testing.T) *classicalLoopHarness {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "classical-complete-loop-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	logs := &lockedBuffer{}
	previous := log.Writer()
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(previous) })

	source, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-deck.conllu"))
	require.NoError(t, err)
	anabasisSource, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	translation, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "perseus-eng2-section.xml"))
	require.NoError(t, err)
	layer := &classicalLoopLayer{source: source, translation: translation}
	server := httptest.NewServer(layer.handler())
	t.Cleanup(server.Close)

	sum := sha256.Sum256(source)
	pin := classical.SourcePin{
		Layer: "loop-layer", FileName: "synthetic-loop.conllu",
		Size: int64(len(source)), SHA256: hex.EncodeToString(sum[:]), Format: classical.SourceFormatOGACoNLLU,
	}
	anabasisSum := sha256.Sum256(anabasisSource)
	anabasisPin := classical.SourcePin{
		Layer: "loop-layer", FileName: "anabasis.conllu",
		Size: int64(len(anabasisSource)), SHA256: hex.EncodeToString(anabasisSum[:]), Format: classical.SourceFormatOGACoNLLU,
	}
	// The synthetic layer stands in for the pinned Brownson translation. Its
	// frozen pin must be the listed one, because a card export now refuses an
	// unlisted passage pin instead of silently crediting Brownson.
	translationPin := &classical.TranslationPin{
		Layer: "loop-layer", FileName: "synthetic-translation.xml",
		Size: int64(len(translation)), SHA256: referenceTranslationFixturePin().FileSHA256,
		CitationStart: "1.1.1", CitationEnd: "1.2.18",
	}
	translationSections, err := ancientgreek.ParseReferenceTranslation(translation, translationPin.CitationStart, translationPin.CitationEnd)
	require.NoError(t, err)
	translations := make(map[string]string, len(translationSections))
	for _, section := range translationSections {
		translations[section.Citation] = section.Text
	}
	_, err = store.PutReferenceTranslations(ctx, translationPin.SHA256, classical.Language, translations)
	require.NoError(t, err)
	one, two := 1, 2
	anabasis := classical.Definition{
		Key: "xenophon-anabasis-1", WorkKey: "xenophon-anabasis", WorkTitle: "Anabasis",
		WorkAuthor: "Xenophon", OriginalTitle: "Ἀνάβασις", EditionKey: "xenophon-anabasis-marchant-1904",
		EditionLabel: "Marchant 1904", Language: classical.Language, ConstituentBooks: 1,
		CitationStart: "1.1.1", CitationEnd: "1.10.19", WordCount: 7, Opening: "ἄρχεται ὁ λόγος.", Source: anabasisPin,
		InterpretationVersion: "oga-agdt-upos/5", Revision: 1,
	}
	firstUnit := classical.Definition{
		Key: "synthetic-work-1", WorkKey: "synthetic-work", WorkTitle: "Synthetic Work",
		WorkAuthor: "Synthetic Author", OriginalTitle: "Συνθετικὸν ἔργον", EditionKey: "synthetic-work-edition",
		EditionLabel: "Synthetic Edition", Language: classical.Language, ConstituentBooks: 1,
		CitationStart: "1.1.1", CitationEnd: "1.1.3", WordCount: 16, Opening: "ἄρχεται ὁ λόγος.", Source: pin, Translation: translationPin,
		InterpretationVersion: "oga-agdt-upos/5", Revision: 1,
	}
	secondUnit := firstUnit
	secondUnit.Key = "synthetic-work-2"
	secondUnit.EditionKey, secondUnit.EditionLabel = "synthetic-work-second-edition", "Second Synthetic Edition"
	secondUnit.CitationStart = "1.1.4"
	secondUnit.CitationEnd = "1.1.6"
	secondUnit.WordCount = 16
	wholeUnit := classical.Definition{
		Key: "synthetic-whole-1", WorkKey: "synthetic-whole", WorkTitle: "Synthetic Whole",
		WorkAuthor: "Synthetic Author", OriginalTitle: "Συνθετικὸν ὅλον", EditionKey: "synthetic-whole-edition",
		EditionLabel: "Synthetic Whole Edition", Language: classical.Language, ConstituentBooks: 1,
		CitationStart: "1.1.1", CitationEnd: "1.1.6", WordCount: 32, Opening: "ἄρχεται ὁ λόγος.", Source: pin,
		InterpretationVersion: "oga-agdt-upos/5", Revision: 1,
	}
	// A multi-book Work is one whole-work unit with a contents list; its
	// per-book definitions from before ADR 0128 survive only as tombstones.
	treatise := classical.Definition{
		Key: loopTreatiseKey, WorkKey: loopTreatiseKey, WorkTitle: "Synthetic Treatise",
		WorkAuthor: "Synthetic Author", OriginalTitle: "Συνθετικὸν σύγγραμμα", EditionKey: "synthetic-treatise-edition",
		EditionLabel: "Synthetic Treatise Edition", Language: classical.Language, ConstituentBooks: 2,
		CitationStart: "1.1.1", CitationEnd: "1.1.6", WordCount: 32, Opening: "ἄρχεται ὁ λόγος.", Source: pin,
		InterpretationVersion: "oga-agdt-upos/5", Revision: 1,
		Contents: []classical.ConstituentBookSpan{
			{Book: 1, CitationStart: "1.1.1", CitationEnd: "1.1.3", WordCount: 16},
			{Book: 2, CitationStart: "1.1.4", CitationEnd: "1.1.6", WordCount: 16},
		},
	}
	treatiseBookOne, treatiseBookTwo := treatise, treatise
	treatiseBookOne.Key, treatiseBookTwo.Key = loopTreatiseKey+"-1", loopTreatiseKey+"-2"
	treatiseBookOne.ConstituentBook, treatiseBookTwo.ConstituentBook = &one, &two
	treatiseBookOne.Contents, treatiseBookTwo.Contents = nil, nil
	for i, tombstone := range []*classical.Definition{&treatiseBookOne, &treatiseBookTwo} {
		span := treatise.Contents[i]
		tombstone.CitationStart, tombstone.CitationEnd, tombstone.WordCount = span.CitationStart, span.CitationEnd, span.WordCount
	}
	treatiseBookOne.Withdrawn, treatiseBookTwo.Withdrawn = true, true
	definitions := []classical.Definition{anabasis, firstUnit, secondUnit, wholeUnit, treatise, treatiseBookOne, treatiseBookTwo}

	seed := seedLoopPreUpgradeAnabasis(t, ctx, store, pool, anabasis)
	manifest, err := classical.RenderDefinitions(definitions)
	require.NoError(t, err)
	require.NoError(t, classical.Reconcile(ctx, store, manifest), "startup reconciles the fixture manifest")
	conversion, err := analysis.ConvertLegacyClassicalBooks(ctx, store)
	require.NoError(t, err)
	require.Equal(t, 1, conversion.Converted, "the seeded pre-upgrade Book converts at startup")

	catalog, err := classical.NewCatalog(definitions)
	require.NoError(t, err)
	layers := classical.Layers{Layers: []classical.Layer{{
		Key: "loop-layer", Commit: "synthetic", BaseURL: server.URL + "/",
		LicenceClass: classical.LicenceClassSourceContent, Restriction: classical.RestrictionShareAlike,
		DeployerRegisterRow: "synthetic complete-loop layer",
	}}}
	config := analysis.ClassicalPreparationConfig{Layers: layers, HTTPClient: server.Client(), TempDir: t.TempDir()}
	workers := river.NewWorkers()
	preparationWorker := analysis.AddClassicalPreparationWorker(workers, store, config)
	analysis.AddClassicalTranslationWorker(workers, store, config)
	unusedAnalyzer := &analyzertest.Fake{AnalyzeFunc: func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{}, errors.New("the Classical catalog never calls the NLP service")
	}}
	client, err := analysis.NewClient(pool, unusedAnalyzer, analyzertest.CapabilityProvider{}, workers)
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "classical complete loop client", client.Stop)
	preparationWorker.Client = client
	service := analysis.NewService(store.Pool(), client).WithClassicalPreparationStore(store)
	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth:                 authService,
		WebAuth:              webauth.New(authService, false, time.Hour),
		Store:                storeDependencies(store),
		Analysis:             service,
		AnalysisUpdates:      service,
		AnalysisInsights:     analysisinsights.NewService(store),
		Capabilities:         analyzertest.CapabilityProvider{},
		ClassicalPreparation: service,
		Classical:            catalog,
		CatalogueSync:        fixtures.NewCatalogueSync(fixtures.NewStore()),
		SessionLifetime:      time.Hour,
	})
	return &classicalLoopHarness{
		ctx: ctx, store: store, pool: pool, handler: h, service: service, client: client,
		layer: layer, definitions: definitions, translation: translationPin,
		unitOneKey: firstUnit.Key, unitTwoKey: secondUnit.Key, wholeUnitKey: wholeUnit.Key,
		anabasis: seed, logs: logs,
	}
}

func (h *classicalLoopHarness) startWorkers(t *testing.T) {
	t.Helper()
	require.NoError(t, h.client.Start(h.ctx))
}

func (h *classicalLoopHarness) booksByUnit(t *testing.T, owner string) map[string]string {
	t.Helper()
	return booksByReadingUnit(t, h.store, owner)
}

func (h *classicalLoopHarness) sourceServed() int64 { return h.layer.sourceServed.Load() }

func (h *classicalLoopHarness) preparationJobs(t *testing.T) (queues, bookIDs []string) {
	t.Helper()
	return workPreparationJobs(t, h.pool)
}

// waitForPreparedDefinition waits for the prepared-definition row that a
// preparation records after it publishes the analysis, then returns its run. The
// analysis being current does not imply this row exists yet.
func (h *classicalLoopHarness) waitForPreparedDefinition(t *testing.T, owner, bookID string) string {
	t.Helper()
	var runID string
	testutil.Eventually(t, 60*time.Second, "prepared definition recorded", func() (bool, string) {
		err := h.pool.QueryRow(h.ctx,
			`SELECT analysis_run_id::text FROM classical_prepared_definitions WHERE owner_id=$1 AND book_id=$2`,
			owner, bookID).Scan(&runID)
		if err != nil {
			return false, err.Error()
		}
		return true, ""
	})
	return runID
}

func (h *classicalLoopHarness) waitForBookAnalysis(t *testing.T, owner, bookID string) {
	t.Helper()
	testutil.Eventually(t, 60*time.Second, "Book analysed", func() (bool, string) {
		var count int
		if err := h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`,
			owner, bookID).Scan(&count); err != nil {
			return false, err.Error()
		}
		return count == 1, "analysis not published"
	})
}

// lockedBuffer makes the captured operator log safe to read while workers
// write to it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// seedLoopPreUpgradeAnabasis writes the legacy registration shape and the
// learner state that startup conversion must preserve. It mirrors the analysis
// package's migration-scenario seed through exported store methods.
func seedLoopPreUpgradeAnabasis(t *testing.T, ctx context.Context, store *persistence.PostgresStore, pool *pgxpool.Pool, definition classical.Definition) classicalLoopSeed {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	owner, err := store.CreateUser(ctx, "classical-loop-anabasis", false)
	require.NoError(t, err)
	readingDefinition := domain.ReadingUnitDefinition{
		Key: definition.Key, Language: definition.Language, WorkTitle: definition.WorkTitle,
		WorkAuthor: definition.WorkAuthor, EditionLabel: definition.EditionLabel,
		CitationStart: definition.CitationStart, CitationEnd: definition.CitationEnd,
		SourceFileName: definition.Source.FileName, SourceFileSize: definition.Source.Size,
		SourceFileSHA256: definition.Source.SHA256, InterpretationVersion: definition.InterpretationVersion,
	}
	registration, err := store.RegisterReadingUnitBook(ctx, owner.ID, readingDefinition)
	require.NoError(t, err)
	parsed, err := ancientgreek.ParseOgaSource(data)
	require.NoError(t, err)
	unit, err := parsed.CutUnit(readingDefinition)
	require.NoError(t, err)
	written, err := analysis.NewGreekImportWriter(store).WriteImportedAnalysis(ctx, ancientgreek.ImportWriteRequest{
		OwnerID: owner.ID, BookID: registration.BookID, Definition: readingDefinition,
		SourceBytes: data, FullText: unit.FullText, UnitID: unit.UnitID, UnitTitle: definition.WorkTitle,
		Result: unit.Result, SyntaxState: unit.SyntaxState, AnalyzerName: unit.AnalyzerName,
		AnalyzerVersion: unit.AnalyzerVersion, ConfigIdentity: unit.ConfigIdentity,
	})
	require.NoError(t, err)
	seed := classicalLoopSeed{ownerID: owner.ID, bookID: registration.BookID, runID: written.RunID}
	var oldRevisionID, oldSnapshotID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT sm.id::text, sm.current_content_revision_id::text, sm.current_snapshot_id::text
		FROM source_materials sm WHERE sm.owner_id=$1 AND sm.book_id=$2`, owner.ID, registration.BookID).
		Scan(&seed.sourceID, &oldRevisionID, &oldSnapshotID))
	_, err = pool.Exec(ctx, `UPDATE source_materials SET source_identifier=$3 WHERE owner_id=$1 AND id=$2`,
		owner.ID, seed.sourceID, definition.Source.FileName)
	require.NoError(t, err)

	// A confirmed Lemma correction and an Occurrence exclusion.
	var firstStart, firstEnd, secondStart, secondEnd int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT start_offset, end_offset FROM corpus_tokens
		WHERE owner_id=$1 AND analysis_run_id=$2 AND sentence_ordinal=0 AND token_ordinal=0`,
		owner.ID, written.RunID).Scan(&firstStart, &firstEnd))
	require.NoError(t, pool.QueryRow(ctx, `SELECT start_offset, end_offset FROM corpus_tokens
		WHERE owner_id=$1 AND analysis_run_id=$2 AND sentence_ordinal=0 AND token_ordinal=1`,
		owner.ID, written.RunID).Scan(&secondStart, &secondEnd))
	var corpusID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND analysis_run_id=$2`,
		owner.ID, written.RunID).Scan(&corpusID))
	_, err = pool.Exec(ctx, `INSERT INTO occurrence_lemma_corrections(
		owner_id, book_id, corpus_id, analysis_run_id, source_document_id,
		start_offset, end_offset, canonical_lemma, normalization_profile, normalization_version, excluded)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'λέγω','ancient-greek','1',false),
		       ($1,$2,$3,$4,$5,$8,$9,NULL,NULL,NULL,true)`,
		owner.ID, registration.BookID, corpusID, written.RunID, unit.UnitID,
		firstStart, firstEnd, secondStart, secondEnd)
	require.NoError(t, err)
	testutil.SeedLegacyKnownVocabulary(t, ctx, pool, owner.ID, classical.Language, "λέγω", "VERB", "morpheus:1")
	_, err = pool.Exec(ctx, `INSERT INTO reading_history(owner_id, language, book_id, completed_at)
		VALUES($1,$2,$3,now())`, owner.ID, classical.Language, registration.BookID)
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, owner.ID, classical.Language))
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, registration.BookID, domain.BookDispositionToRead))
	_, err = store.StartCurrentReadingResult(ctx, owner.ID, classical.Language, registration.BookID)
	require.NoError(t, err)

	return seed
}
