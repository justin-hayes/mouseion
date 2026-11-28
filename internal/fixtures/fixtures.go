// Package fixtures provides the deterministic, infrastructure-free data set
// used by the browser acceptance harness.
package fixtures

// Contract status: illustrative. Shared canned state for the browser
// acceptance harness; each feature file states its own status.

import (
	"strconv"
	"sync"
	"time"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
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
	BrowserSyncBookID      = "fixture-browser-sync-book"
	catalogueRestoreBookID = "fixture-catalogue-restore"
	catalogueRestoreSecond = "fixture-catalogue-restore-second"
	routeMatchBookID       = "fixture-route-match"
	routeDiffersBookID     = "fixture-route-differs"
	routeTieABookID        = "fixture-route-tie-a"
	routeTieBBookID        = "fixture-route-tie-b"
	routeUnavailableBookID = "fixture-route-unavailable"
)

const edgeBookID = "fixture-edge-content"

const italianRouteBookID = "fixture-italian-route"

var errNotFound = domain.ErrNotFound

var fixtureJourneyTime = time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)

func fixtureGoalKey(owner, language string) string {
	return fixtureLanguageKey(owner, language)
}

// fixtureReadingHistoryKey names the next Finish completion. Every Finish is a
// new row, so the key is a sequence number, never a Book or snapshot identity.
func fixtureReadingHistoryKey(owner, language string, sequence int) string {
	return fixtureGoalKey(owner, language) + "\x00" + strconv.Itoa(sequence)
}

type Store struct {
	mu          sync.Mutex
	books       []domain.SourceMaterialSummary
	jobs        []domain.AnalysisJob
	supported   []domain.SupportedLanguage
	connections []domain.OpdsConnection
	// catalogueEntries is each connection's Catalog index, keyed by connection
	// ID. It mirrors upstream metadata only; ownership stays in aliases and
	// Book membership.
	catalogueEntries map[string][]domain.CatalogueIndexEntry
	// entryCovers holds seeded retained Catalog entry covers, keyed by
	// connection and entry identity.
	entryCovers          map[string]domain.CatalogEntryCoverResource
	catalogueBookCount   int
	aliases              []domain.BookAlias
	myBooks              []domain.MyBook
	dispositions         map[string]domain.BookDisposition
	dispositionRevisions map[string]int64
	// unchosen holds legacy active members with no chosen workflow role, keyed
	// like dispositions. My Books neither lists nor counts them.
	unchosen map[string]bool
	// epubAcquisitionFailures is the latest failed EPUB acquisition per Book,
	// keyed by fixtureDispositionKey. A successful acquisition deletes the row.
	epubAcquisitionFailures map[string]domain.EPUBAcquisitionFailureRecord
	// removedBooks holds the Books whose My Books membership is inactive. The
	// Book and its evidence stay so a later Add can restore them.
	removedBooks map[string]struct{}
	// positionReports are the recorded Position reports in insertion order.
	positionReports      []fixturePositionReport
	currentReadings      map[string]domain.CurrentReading
	readingHistory       map[string]domain.ReadingCompletion
	importedHistory      map[string]domain.ReadingCompletion
	syncStatuses         []domain.CatalogueSyncStatus
	storedActiveLanguage *string
	mostRecentLanguage   string
	lemmaCorrections     map[string]string
	lemmaExclusions      map[string]bool
	// lemmaOccurrences are the occurrences each Book's analysis contributes to
	// lemma review, keyed by lemmaOccurrenceKey. Decisions validate
	// against them.
	lemmaOccurrences map[string][]domain.LemmaReviewOccurrence
	browseScenario   string
	// studyMaterial is every Study material record, current and superseded,
	// keyed by owner and record ID order of creation.
	studyMaterial []domain.StudyMaterial
	// studyProgress is every word in study, studyReviewLog the append-only log
	// of its graded reviews and studyLedger the batches answered per owner,
	// language and Study day.
	studyProgress []domain.StudyProgress
	// studyCandidates are extra Study candidates a browser fixture seeds
	// without an analyzed Book behind them.
	studyCandidates []seededStudyCandidate
	studyReviewLog  []domain.StudyReviewLog
	// studyAlreadyKnown is the Already-known events, never grades.
	studyAlreadyKnown []domain.StudyAlreadyKnownEvent
	studyLedger       map[string]domain.StudyBatchLedger
	// studyReports is every Study report, open and ended. studyActions lists
	// Study actions and studyReportSeq records how many there were when each
	// report was made, so Undo can tell whether another action followed.
	studyReports   []domain.StudyReport
	studyActions   []studyAction
	studyReportSeq map[string]int
	// studyProgressSeq numbers Study progress IDs, which are never reused.
	studyProgressSeq int
	// studySuperseded holds the IDs of Study material a correction replaced.
	studySuperseded map[string]bool
	// studyFluencyContexts are the Fluency contexts of mature words and
	// studyFluencyResponses the append-only answers to them.
	studyFluencyContexts  []domain.FluencyContext
	studyFluencyResponses []StudyFluencyResponse
	// classicalDefinitions is the release the fixture's Add resolves Reading
	// units from. Tests may install the shared contract's release.
	classicalDefinitions map[string]classical.Definition
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
		catalogueEntries: fixtureCatalogueIndexEntries(),
		aliases: []domain.BookAlias{
			{ID: "fixture-metadata-only-alias", OwnerID: OwnerID, BookID: "fixture-metadata-only", ConnectionID: "fixture-connection", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "fixture-entry"},
			{ID: "fixture-browser-sync-alias", OwnerID: OwnerID, BookID: BrowserSyncBookID, ConnectionID: "fixture-browser-sync-connection", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "fixture-browser-sync-entry"},
			{ID: "fixture-catalog-owned-alias", OwnerID: OwnerID, BookID: BookID, ConnectionID: "fixture-browser-sync-connection", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "fixture-browser-second"},
			{ID: "fixture-catalogue-restore-alias", OwnerID: OwnerID, BookID: catalogueRestoreBookID, ConnectionID: "fixture-connection", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "fixture-sommer"},
			{ID: "fixture-catalogue-restore-second-alias", OwnerID: OwnerID, BookID: catalogueRestoreSecond, ConnectionID: "fixture-connection", AliasType: domain.AliasCatalogEntry, Namespace: domain.NamespaceSourceIdentifier, Value: "fixture-gedicht"},
		},
		syncStatuses: []domain.CatalogueSyncStatus{
			{OwnerID: OwnerID, ConnectionID: "fixture-connection", State: domain.CatalogueSyncSynced, LastSyncedAt: &lastSyncedAt, LastUpsertedCount: 3, UpdatedAt: fixtureJourneyTime},
			{OwnerID: OwnerID, ConnectionID: "fixture-failed-connection", State: domain.CatalogueSyncFailed, LastError: "Authentication failed for this connection. Check the saved credentials and try again.", UpdatedAt: fixtureJourneyTime},
			{OwnerID: OwnerID, ConnectionID: "fixture-syncing-connection", State: domain.CatalogueSyncSyncing, UpdatedAt: fixtureJourneyTime},
		},
		// French has Study progress and no Book, so the language stays
		// selectable as "(no books)".
		studyProgress: []domain.StudyProgress{{
			ID: "fixture-study-progress-fr-bonjour", OwnerID: OwnerID, StudyItem: domain.WordItem("fr", "bonjour", "NOUN", ""),
			IntroducedAt: fixtureJourneyTime, Memory: domain.NewStudyMemory(fixtureJourneyTime), CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime,
		}},
		myBooks: []domain.MyBook{{
			Book: domain.Book{ID: "fixture-metadata-only", OwnerID: OwnerID, Title: "Metadata-only migration book", Author: "Fixture Catalogue Author", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de", CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime}, CompletionCount: 1,
		}, {
			Book: domain.Book{ID: BrowserSyncBookID, OwnerID: OwnerID, Title: "Browser sync metadata book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de", CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime}, CompletionCount: 1,
		}, {
			// These acquired German Books have left My Books, so the Catalog
			// offers Add for their entries and browser acceptance can exercise
			// the restore path without a fixture acquisition transition.
			Book:     domain.Book{ID: catalogueRestoreBookID, OwnerID: OwnerID, Title: "Ein kurzer Sommer", Author: "Bernd Alt", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de", CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
			Acquired: &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "fixture-catalogue-restore-source", OwnerID: OwnerID, Language: "de", Title: "Ein kurzer Sommer", MediaType: "application/epub+zip", ContentRevisionID: "fixture-catalogue-restore-revision", ContentSnapshotID: "fixture-catalogue-restore-snapshot"}, BookID: catalogueRestoreBookID, BookTitle: "Ein kurzer Sommer", BookAuthor: "Bernd Alt", Signals: analyzedSignals, AnalysisRunID: "fixture-catalogue-restore-run", CorpusID: "fixture-catalogue-restore-corpus"},
		}, {
			Book:     domain.Book{ID: catalogueRestoreSecond, OwnerID: OwnerID, Title: "Gedichte aus dem Norden", Author: "Klara Frost", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de", CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
			Acquired: &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "fixture-catalogue-restore-second-source", OwnerID: OwnerID, Language: "de", Title: "Gedichte aus dem Norden", MediaType: "application/epub+zip", ContentRevisionID: "fixture-catalogue-restore-second-revision", ContentSnapshotID: "fixture-catalogue-restore-second-snapshot"}, BookID: catalogueRestoreSecond, BookTitle: "Gedichte aus dem Norden", BookAuthor: "Klara Frost", Signals: analyzedSignals, AnalysisRunID: "fixture-catalogue-restore-second-run", CorpusID: "fixture-catalogue-restore-second-corpus"},
		}, {
			Book:            domain.Book{ID: "fixture-read-history", OwnerID: OwnerID, Title: "Previously finished fixture", Author: "Elise Sommer", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de", CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
			CompletionCount: 1, LatestCompletionAt: new(fixtureJourneyTime.AddDate(0, 0, -12)), LatestCompletionSource: domain.ReadingCompletionPreviouslyRead,
		}, {
			Book: domain.Book{ID: "fixture-acquisition-failed", OwnerID: OwnerID, Title: "Nicht herunterladbar", Author: "Hanna Voss", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de", CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
		}},
		dispositions: map[string]domain.BookDisposition{
			fixtureDispositionKey(OwnerID, "fixture-metadata-only"):      domain.BookDispositionInbox,
			fixtureDispositionKey(OwnerID, BrowserSyncBookID):            domain.BookDispositionInbox,
			fixtureDispositionKey(OwnerID, BookID):                       domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, "fixture-failed"):             domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, "fixture-running"):            domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, "fixture-not-analyzed"):       domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, routeMatchBookID):             domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, LemmaFlagBookID):              domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, routeDiffersBookID):           domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, routeTieABookID):              domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, routeTieBBookID):              domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, routeUnavailableBookID):       domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, "fixture-empty"):              domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, edgeBookID):                   domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, italianRouteBookID):           domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, ItalianGoalBookID):            domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, staleBookID):                  domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, cancelledBookID):              domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, publicationPendingBookID):     domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, reAnalysisRunningBookID):      domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, reAnalysisFailedBookID):       domain.BookDispositionToRead,
			fixtureDispositionKey(OwnerID, "fixture-acquisition-failed"): domain.BookDispositionToRead,
		},
		epubAcquisitionFailures: map[string]domain.EPUBAcquisitionFailureRecord{
			fixtureDispositionKey(OwnerID, "fixture-acquisition-failed"): {
				OwnerID: OwnerID, BookID: "fixture-acquisition-failed",
				ConnectionID: "fixture-connection", EntryIdentifier: "fixture-entry",
				Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed,
			},
		},
		dispositionRevisions: make(map[string]int64),
		unchosen:             make(map[string]bool),
		removedBooks:         make(map[string]struct{}),
		importedHistory:      make(map[string]domain.ReadingCompletion),
		currentReadings: map[string]domain.CurrentReading{
			fixtureGoalKey(OwnerID, "de"): {OwnerID: OwnerID, Language: "de", BookID: BookID, CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
			fixtureGoalKey(OwnerID, "it"): {OwnerID: OwnerID, Language: "it", BookID: ItalianGoalBookID, CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
		},
		readingHistory:       make(map[string]domain.ReadingCompletion),
		storedActiveLanguage: &initialActiveLanguage,
		mostRecentLanguage:   "it",
		lemmaCorrections:     make(map[string]string),
		lemmaExclusions:      make(map[string]bool),
		lemmaOccurrences:     fixtureCannedLemmaOccurrences(),
		classicalDefinitions: fixtureClassicalDefinitionsByKey(),
	}
	// Their entries stay browsable and offer Add.
	store.removedBooks[fixtureDispositionKey(OwnerID, catalogueRestoreBookID)] = struct{}{}
	store.removedBooks[fixtureDispositionKey(OwnerID, catalogueRestoreSecond)] = struct{}{}
	return store
}
