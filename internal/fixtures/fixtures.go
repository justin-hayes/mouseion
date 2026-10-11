// Package fixtures provides the deterministic, infrastructure-free data set
// used by the browser acceptance harness.
package fixtures

// Contract status: illustrative. Shared canned state for the browser
// acceptance harness; each feature file states its own status.

import (
	"sync"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const (
	OwnerID                = "fixture-learner"
	Username               = "fixture-learner"
	Password               = "fixture-password"
	BookID                 = "fixture-book"
	ItalianGoalBookID      = "fixture-italian-goal"
	LemmaFlagBookID        = "fixture-lemma-flag-book"
	SourceID               = "fixture-source"
	ResultRunID            = "fixture-run"
	DeckID                 = "fixture-deck"
	PrepID                 = "fixture-preparation"
	QueuedPrepID           = "fixture-queued-preparation"
	JourneyPrepID          = "fixture-journey-preparation"
	OutsidePrepID          = "fixture-outside-journey-preparation"
	BrowserSyncBookID      = "fixture-browser-sync-book"
	LegacyGeneratedLemma   = "fixture-legacy-generated"
	GraduatedKnownLemma    = "fixture-graduated-known"
	IndependentKnownLemma  = "fixture-independent-known"
	routeMatchBookID       = "fixture-route-match"
	routeDiffersBookID     = "fixture-route-differs"
	routeTieABookID        = "fixture-route-tie-a"
	routeTieBBookID        = "fixture-route-tie-b"
	routeUnavailableBookID = "fixture-route-unavailable"
)

const edgeBookID = "fixture-edge-content"

const italianRouteBookID = "fixture-italian-route"

var errNotFound = persistence.ErrNotFound

var fixtureJourneyTime = time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)

func fixtureGoalKey(owner, language string) string {
	return fixtureLanguageKey(owner, language)
}

func fixtureReadingHistoryKey(owner, language, snapshotID string) string {
	return fixtureGoalKey(owner, language) + "\x00" + snapshotID
}

type Store struct {
	mu                     sync.Mutex
	books                  []domain.SourceMaterialSummary
	jobs                   []domain.AnalysisJob
	supported              []domain.SupportedLanguage
	connections            []domain.OpdsConnection
	aliases                []domain.BookAlias
	preps                  []domain.DeckPreparation
	deckVocabulary         []domain.DeckPreparationVocabulary
	known                  []domain.KnownVocabulary
	goalSnapshotVocabulary map[string][]domain.DeckPreparationVocabulary
	legacyGenerated        []domain.GeneratedVocabulary
	myBooks                []domain.MyBook
	dispositions           map[string]domain.BookDisposition
	dispositionRevisions   map[string]int64
	visibility             map[string]fixtureVisibility
	currentReadings        map[string]domain.CurrentReading
	goalSnapshotSequence   map[string]int
	snapshotLifecycles     map[string]domain.CurrentReadingSnapshotLifecycle
	readingHistory         map[string]domain.ReadingCompletion
	importedHistory        map[string]domain.ReadingCompletion
	syncStatuses           []domain.CatalogueSyncStatus
	storedActiveLanguage   *string
	mostRecentLanguage     string
	lemmaCorrections       map[string]string
	lemmaExclusions        map[string]bool
	lemmaReviewFlags       map[string]domain.LemmaReviewFlag
	browseScenario         string
}

// Fixture Books state the typed analysis evidence they present. Eligibility is
// always derived by the domain classifier, never restated here.
var (
	analyzedSignals    = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedCurrent, LatestRun: domain.RunCompleted}
	notAnalyzedSignals = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedNone, LatestRun: domain.RunNone}
	failedSignals      = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedNone, LatestRun: domain.RunFailed}
	runningSignals     = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedNone, LatestRun: domain.RunRunning}
	noContentSignals   = domain.AnalysisSignals{Content: domain.ContentNoCurrentRevision, Published: domain.PublishedNone, LatestRun: domain.RunNone}
	// The five Analysis evidence situations SQL can produce beyond the above.
	staleSignals              = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedStale, LatestRun: domain.RunCompleted}
	cancelledSignals          = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedNone, LatestRun: domain.RunCancelled}
	publicationPendingSignals = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedNone, LatestRun: domain.RunPublicationPending}
	reAnalysisRunningSignals  = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedCurrent, LatestRun: domain.RunRunning}
	reAnalysisFailedSignals   = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedCurrent, LatestRun: domain.RunFailed}
)

// Fixture Books for the Analysis evidence situations that browser acceptance
// checks in My Books and the Reading chooser.
const (
	staleBookID              = "fixture-stale"
	cancelledBookID          = "fixture-cancelled"
	publicationPendingBookID = "fixture-publication-pending"
	reAnalysisRunningBookID  = "fixture-reanalysis-running"
	reAnalysisFailedBookID   = "fixture-reanalysis-failed"
)

func NewStore() *Store {
	lastSyncedAt := fixtureJourneyTime
	initialActiveLanguage := "de"
	store := &Store{
		books: []domain.SourceMaterialSummary{
			{Source: domain.SourceMaterial{ID: SourceID, OwnerID: OwnerID, Language: "de", Title: "Der lange Weg nach Hause", MediaType: "application/epub+zip", SourceIdentifier: "fixture-de", ContentRevisionID: "fixture-revision", ContentSnapshotID: "fixture-snapshot", FullText: "Haus. Ein kurzer deutscher Satz.\n\n" + "Ein sehr langer Beispielsatz mit vielen Wörtern für die Anzeige von realistischem Randinhalt im Browser."}, BookID: BookID, BookAuthor: "Mara Weiss, Herausgeberin der langen deutschen Ausgabe", Signals: analyzedSignals, AnalysisRunID: ResultRunID, CorpusID: "fixture-corpus", AnalysisJobID: 42},
			{Source: domain.SourceMaterial{ID: "fixture-empty", OwnerID: OwnerID, Language: "it", Title: "Empty chapter", MediaType: "application/epub+zip", ContentRevisionID: "fixture-empty-revision", ContentSnapshotID: "fixture-empty-snapshot"}, Signals: notAnalyzedSignals},
			{Source: domain.SourceMaterial{ID: ItalianGoalBookID, OwnerID: OwnerID, Language: "it", Title: "Una meta italiana", MediaType: "application/epub+zip", ContentRevisionID: "fixture-italian-goal-revision", ContentSnapshotID: "fixture-italian-goal-snapshot"}, BookAuthor: "Giulia Conti", Signals: analyzedSignals, AnalysisRunID: "fixture-italian-goal-run", CorpusID: "fixture-italian-goal-corpus"},
			{Source: domain.SourceMaterial{ID: "fixture-failed", OwnerID: OwnerID, Language: "de", Title: "Fehlgeschlagene Analyse", MediaType: "application/epub+zip", ContentRevisionID: "fixture-failed-revision", ContentSnapshotID: "fixture-failed-snapshot"}, BookAuthor: "Jonas Keller", Signals: failedSignals, AnalysisJobID: 43},
			{Source: domain.SourceMaterial{ID: "fixture-running", OwnerID: OwnerID, Language: "de", Title: "Analyse läuft", MediaType: "application/epub+zip", ContentRevisionID: "fixture-running-revision", ContentSnapshotID: "fixture-running-snapshot"}, BookAuthor: "Nina Weber", Signals: runningSignals, AnalysisJobID: 44},
			{Source: domain.SourceMaterial{ID: "fixture-not-analyzed", OwnerID: OwnerID, Language: "de", Title: "Noch nicht analysiert", MediaType: "application/epub+zip", ContentRevisionID: "fixture-not-analyzed-revision", ContentSnapshotID: "fixture-not-analyzed-snapshot"}, BookAuthor: "Mila Braun", Signals: notAnalyzedSignals},
			{Source: domain.SourceMaterial{ID: routeMatchBookID, OwnerID: OwnerID, Language: "de", Title: "Route match: familiar German", MediaType: "application/epub+zip", ContentRevisionID: "fixture-route-match-revision", ContentSnapshotID: "fixture-route-match-snapshot"}, BookAuthor: "Anja Roth", Signals: analyzedSignals, AnalysisRunID: "fixture-route-match-run", CorpusID: "fixture-route-match-corpus"},
			{Source: domain.SourceMaterial{ID: LemmaFlagBookID, OwnerID: OwnerID, Language: "de", Title: "Flagged lemma review fixture", MediaType: "application/epub+zip", ContentRevisionID: "fixture-lemma-flag-revision", ContentSnapshotID: "fixture-lemma-flag-snapshot"}, BookAuthor: "Fixture Learner", Signals: analyzedSignals, AnalysisRunID: "fixture-lemma-flag-run", CorpusID: "fixture-lemma-flag-corpus"},
			{Source: domain.SourceMaterial{ID: routeDiffersBookID, OwnerID: OwnerID, Language: "de", Title: "Route differs: new German", MediaType: "application/epub+zip", ContentRevisionID: "fixture-route-differs-revision", ContentSnapshotID: "fixture-route-differs-snapshot"}, BookAuthor: "Paul Stein", Signals: analyzedSignals, AnalysisRunID: "fixture-route-differs-run", CorpusID: "fixture-route-differs-corpus"},
			{Source: domain.SourceMaterial{ID: routeTieABookID, OwnerID: OwnerID, Language: "de", Title: "Route tie A", MediaType: "application/epub+zip", ContentRevisionID: "fixture-route-tie-a-revision", ContentSnapshotID: "fixture-route-tie-a-snapshot"}, Signals: analyzedSignals, AnalysisRunID: "fixture-route-tie-a-run", CorpusID: "fixture-route-tie-a-corpus"},
			{Source: domain.SourceMaterial{ID: routeTieBBookID, OwnerID: OwnerID, Language: "de", Title: "Route tie B", MediaType: "application/epub+zip", ContentRevisionID: "fixture-route-tie-b-revision", ContentSnapshotID: "fixture-route-tie-b-snapshot"}, Signals: analyzedSignals, AnalysisRunID: "fixture-route-tie-b-run", CorpusID: "fixture-route-tie-b-corpus"},
			{Source: domain.SourceMaterial{ID: routeUnavailableBookID, OwnerID: OwnerID, Language: "de", Title: "Route evidence pending", MediaType: "application/epub+zip"}, Signals: noContentSignals},
			{Source: domain.SourceMaterial{ID: edgeBookID, OwnerID: OwnerID, Title: "Donaudampfschifffahrtsgesellschaftskapitänsmütze: Eine Geschichte der deutschen Wörter, langen Reisen und unerwarteten Begegnungen am Fluss", Language: "it", FullText: "La biblioteca conserva una storia italiana con molte parole e una descrizione volutamente assente."}, Signals: noContentSignals},
			{Source: domain.SourceMaterial{ID: italianRouteBookID, OwnerID: OwnerID, Language: "it", Title: "Italian route baseline", MediaType: "application/epub+zip", ContentRevisionID: "fixture-italian-route-revision", ContentSnapshotID: "fixture-italian-route-snapshot"}, BookAuthor: "Luca Bianchi", Signals: analyzedSignals, AnalysisRunID: "fixture-italian-route-run", CorpusID: "fixture-italian-route-corpus"},
			{Source: domain.SourceMaterial{ID: staleBookID, OwnerID: OwnerID, Language: "de", Title: "Veraltete Analyse", MediaType: "application/epub+zip", ContentRevisionID: "fixture-stale-revision", ContentSnapshotID: "fixture-stale-snapshot"}, BookAuthor: "Ruth Koch", Signals: staleSignals, AnalysisRunID: "fixture-stale-run", CorpusID: "fixture-stale-corpus"},
			{Source: domain.SourceMaterial{ID: cancelledBookID, OwnerID: OwnerID, Language: "de", Title: "Abgebrochene Analyse", MediaType: "application/epub+zip", ContentRevisionID: "fixture-cancelled-revision", ContentSnapshotID: "fixture-cancelled-snapshot"}, BookAuthor: "Tim Lang", Signals: cancelledSignals},
			{Source: domain.SourceMaterial{ID: publicationPendingBookID, OwnerID: OwnerID, Language: "de", Title: "Veröffentlichung ausstehend", MediaType: "application/epub+zip", ContentRevisionID: "fixture-publication-pending-revision", ContentSnapshotID: "fixture-publication-pending-snapshot"}, BookAuthor: "Lea Hahn", Signals: publicationPendingSignals},
			{Source: domain.SourceMaterial{ID: reAnalysisRunningBookID, OwnerID: OwnerID, Language: "de", Title: "Neue Analyse läuft", MediaType: "application/epub+zip", ContentRevisionID: "fixture-reanalysis-running-revision", ContentSnapshotID: "fixture-reanalysis-running-snapshot"}, BookAuthor: "Oskar Ruhl", Signals: reAnalysisRunningSignals, AnalysisRunID: "fixture-reanalysis-running-run", CorpusID: "fixture-reanalysis-running-corpus"},
			{Source: domain.SourceMaterial{ID: reAnalysisFailedBookID, OwnerID: OwnerID, Language: "de", Title: "Neue Analyse fehlgeschlagen", MediaType: "application/epub+zip", ContentRevisionID: "fixture-reanalysis-failed-revision", ContentSnapshotID: "fixture-reanalysis-failed-snapshot"}, BookAuthor: "Clara Vogt", Signals: reAnalysisFailedSignals, AnalysisRunID: "fixture-reanalysis-failed-run", CorpusID: "fixture-reanalysis-failed-corpus"},
		},
		jobs:      fixtureJobs(),
		supported: []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}, {Language: "it", DisplayName: "Italian"}},
		connections: []domain.OpdsConnection{
			{ID: "fixture-connection", OwnerID: OwnerID, Name: "Fixture catalog", URL: "https://fixture.invalid/opds"},
			{ID: "fixture-failed-connection", OwnerID: OwnerID, Name: "Fixture failed catalog", URL: "https://failed.fixture.invalid/opds"},
			{ID: "fixture-syncing-connection", OwnerID: OwnerID, Name: "Fixture syncing catalog", URL: "https://syncing.fixture.invalid/opds"},
			{ID: "fixture-never-synced-connection", OwnerID: OwnerID, Name: "Fixture never-synced catalog", URL: "https://never.fixture.invalid/opds"},
			{ID: "fixture-browser-sync-connection", OwnerID: OwnerID, Name: "Browser sync catalog", URL: "https://browser-sync.fixture.invalid/opds"},
		},
		aliases: []domain.BookAlias{
			{ID: "fixture-metadata-only-alias", OwnerID: OwnerID, BookID: "fixture-metadata-only", ConnectionID: "fixture-connection", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "fixture-entry"},
			{ID: "fixture-browser-sync-alias", OwnerID: OwnerID, BookID: BrowserSyncBookID, ConnectionID: "fixture-browser-sync-connection", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "fixture-browser-sync-entry"},
		},
		syncStatuses: []domain.CatalogueSyncStatus{
			{OwnerID: OwnerID, ConnectionID: "fixture-connection", State: domain.CatalogueSyncSynced, LastSyncedAt: &lastSyncedAt, LastUpsertedCount: 3, UpdatedAt: fixtureJourneyTime},
			{OwnerID: OwnerID, ConnectionID: "fixture-failed-connection", State: domain.CatalogueSyncFailed, LastError: "Authentication failed for this connection. Check the saved credentials and try again.", UpdatedAt: fixtureJourneyTime},
			{OwnerID: OwnerID, ConnectionID: "fixture-syncing-connection", State: domain.CatalogueSyncSyncing, UpdatedAt: fixtureJourneyTime},
		},
		preps: []domain.DeckPreparation{
			{ID: PrepID, OwnerID: OwnerID, BookID: BookID, SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, SnapshotID: "fixture-de-goal-snapshot", State: domain.DeckPreparationReady, Filename: "Fixture German deck.apkg", DeckName: "Mouseion::de::Fixture", TotalCards: 3},
			{ID: QueuedPrepID, OwnerID: OwnerID, SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, State: domain.DeckPreparationReady, Filename: "Fixture German queued deck.apkg", DeckName: "Mouseion::de::Queued", TotalCards: 3},
		},
		deckVocabulary: []domain.DeckPreparationVocabulary{
			{OwnerID: OwnerID, DeckPreparationID: PrepID, Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", GeneratedAt: fixtureJourneyTime},
			{OwnerID: OwnerID, DeckPreparationID: PrepID, Language: "de", CanonicalLemma: "Weg", UPOS: "NOUN", GeneratedAt: fixtureJourneyTime},
		},
		known: []domain.KnownVocabulary{
			{ID: "fixture-known", OwnerID: OwnerID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", CreatedAt: fixtureJourneyTime},
			{ID: "fixture-known-only", OwnerID: OwnerID, Language: "fr", CanonicalLemma: "bonjour", UPOS: "NOUN", CreatedAt: fixtureJourneyTime},
			{ID: "fixture-independent-known", OwnerID: OwnerID, Language: "de", CanonicalLemma: IndependentKnownLemma, UPOS: "NOUN", CreatedAt: fixtureJourneyTime},
			{ID: "fixture-graduated-known", OwnerID: OwnerID, Language: "de", CanonicalLemma: GraduatedKnownLemma, UPOS: "VERB", CreatedAt: fixtureJourneyTime.Add(2 * time.Hour)},
		},
		legacyGenerated: []domain.GeneratedVocabulary{{OwnerID: OwnerID, Language: "de", CanonicalLemma: LegacyGeneratedLemma, UPOS: "ADJ", FirstDeckID: "fixture-legacy-generated-deck", FirstGeneratedAt: fixtureJourneyTime}},
		myBooks: []domain.MyBook{{
			Book: domain.Book{ID: "fixture-metadata-only", OwnerID: OwnerID, Title: "Metadata-only migration book", Author: "Fixture Catalogue Author", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown, CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
		}, {
			Book: domain.Book{ID: BrowserSyncBookID, OwnerID: OwnerID, Title: "Browser sync metadata book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown, CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
		}, {
			Book:            domain.Book{ID: "fixture-read-history", OwnerID: OwnerID, Title: "Previously finished fixture", Author: "Elise Sommer", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de", CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
			CompletionCount: 1, LatestCompletionAt: timePtr(fixtureJourneyTime.AddDate(0, 0, -12)), LatestCompletionSource: domain.ReadingCompletionPreviouslyRead,
		}},
		dispositions: map[string]domain.BookDisposition{
			fixtureDispositionKey(OwnerID, BookID):                   domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, "fixture-failed"):         domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, "fixture-running"):        domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, "fixture-not-analyzed"):   domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, routeMatchBookID):         domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, LemmaFlagBookID):          domain.BookDispositionInbox,
			fixtureDispositionKey(OwnerID, routeDiffersBookID):       domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, routeTieABookID):          domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, routeTieBBookID):          domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, routeUnavailableBookID):   domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, "fixture-empty"):          domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, edgeBookID):               domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, italianRouteBookID):       domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, ItalianGoalBookID):        domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, staleBookID):              domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, cancelledBookID):          domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, publicationPendingBookID): domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, reAnalysisRunningBookID):  domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, reAnalysisFailedBookID):   domain.BookDispositionToRead,
		},
		dispositionRevisions: make(map[string]int64),
		visibility:           make(map[string]fixtureVisibility),
		importedHistory:      make(map[string]domain.ReadingCompletion),
		currentReadings: map[string]domain.CurrentReading{
			fixtureGoalKey(OwnerID, "de"): {OwnerID: OwnerID, Language: "de", BookID: BookID, SnapshotID: "fixture-de-goal-snapshot", SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, ContentRevisionID: "fixture-revision", ContentSnapshotID: "fixture-snapshot", CorpusID: "fixture-corpus", SnapshotSize: 2, CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
			fixtureGoalKey(OwnerID, "it"): {OwnerID: OwnerID, Language: "it", BookID: ItalianGoalBookID, SnapshotID: "fixture-it-goal-snapshot", SourceMaterialID: ItalianGoalBookID, AnalysisRunID: "fixture-italian-goal-run", ContentRevisionID: "fixture-italian-goal-revision", ContentSnapshotID: "fixture-italian-goal-snapshot", CorpusID: "fixture-italian-goal-corpus", CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
		},
		goalSnapshotSequence: make(map[string]int),
		snapshotLifecycles:   make(map[string]domain.CurrentReadingSnapshotLifecycle),
		goalSnapshotVocabulary: map[string][]domain.DeckPreparationVocabulary{
			"fixture-de-goal-snapshot": {
				{OwnerID: OwnerID, Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", GeneratedAt: fixtureJourneyTime},
				{OwnerID: OwnerID, Language: "de", CanonicalLemma: "Weg", UPOS: "NOUN", GeneratedAt: fixtureJourneyTime},
			},
		},
		readingHistory:       make(map[string]domain.ReadingCompletion),
		storedActiveLanguage: &initialActiveLanguage,
		mostRecentLanguage:   "it",
		lemmaCorrections:     make(map[string]string),
		lemmaExclusions:      make(map[string]bool),
		lemmaReviewFlags:     make(map[string]domain.LemmaReviewFlag),
	}
	store.lemmaReviewFlags[fixtureLemmaCorrectionKey(OwnerID, LemmaFlagBookID, "fixture-lemma-flag-run", 4, 7)] = domain.LemmaReviewFlag{
		Occurrence: domain.LemmaReviewOccurrence{OwnerID: OwnerID, BookID: LemmaFlagBookID, AnalysisRunID: "fixture-lemma-flag-run", SourceDocumentID: "fixture-lemma-flag-unit", StartOffset: 4, EndOffset: 7},
		Reason:     "The analyzer lemma is a local-index miss and sentence evidence supports a competing recurring lemma.",
		Provenance: map[string]any{"alternative_lemma": "pfad", "source": "wiktionary", "version": "fixture-1", "evidence_id": "fixture-lemma-evidence"},
	}
	return store
}
