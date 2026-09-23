// Package fixtures provides the deterministic, infrastructure-free data set
// used by the browser acceptance harness.
package fixtures

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/enrichmentjob"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/riverqueue/river/rivertype"
)

const (
	OwnerID                = "fixture-learner"
	Username               = "fixture-learner"
	Password               = "fixture-password"
	BookID                 = "fixture-book"
	ItalianGoalBookID      = "fixture-italian-goal"
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

var fixtureSyncArrivals = []domain.SupportedLanguage{
	{Language: "es", DisplayName: "Spanish"},
	{Language: "nl", DisplayName: "Dutch"},
	{Language: "pt", DisplayName: "Portuguese"},
	{Language: "sv", DisplayName: "Swedish"},
}

func normalizeFixtureLanguage(raw string) string {
	return canonicalization.NormalizeLanguage(raw)
}

func fixtureJourneyKey(owner, language string) string {
	return owner + "\x00" + normalizeFixtureLanguage(language)
}

func fixtureGoalKey(owner, language string) string {
	return fixtureJourneyKey(owner, language)
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
	readingJourneys        map[string]domain.ReadingJourney
	primaryGoals           map[string]domain.PrimaryGoal
	readingHistory         map[string]domain.ReadingCompletion
	syncStatuses           []domain.CatalogueSyncStatus
	storedActiveLanguage   *string
	mostRecentLanguage     string
}

func NewStore() *Store {
	lastSyncedAt := fixtureJourneyTime
	initialActiveLanguage := "de"
	return &Store{
		books: []domain.SourceMaterialSummary{
			{Source: domain.SourceMaterial{ID: SourceID, OwnerID: OwnerID, Language: "de", Title: "Der lange Weg nach Hause", MediaType: "application/epub+zip", SourceIdentifier: "fixture-de", ContentRevisionID: "fixture-revision", ContentSnapshotID: "fixture-snapshot", FullText: "Haus. Ein kurzer deutscher Satz.\n\n" + "Ein sehr langer Beispielsatz mit vielen Wörtern für die Anzeige von realistischem Randinhalt im Browser."}, BookID: BookID, BookAuthor: "Mara Weiss, Herausgeberin der langen deutschen Ausgabe", AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: ResultRunID, CorpusID: "fixture-corpus", AnalysisJobID: 42},
			{Source: domain.SourceMaterial{ID: "fixture-empty", OwnerID: OwnerID, Language: "it", Title: "Empty chapter", MediaType: "application/epub+zip"}, AnalysisStatus: "not analyzed", AnalysisState: ""},
			{Source: domain.SourceMaterial{ID: ItalianGoalBookID, OwnerID: OwnerID, Language: "it", Title: "Una meta italiana", MediaType: "application/epub+zip", ContentRevisionID: "fixture-italian-goal-revision", ContentSnapshotID: "fixture-italian-goal-snapshot"}, BookAuthor: "Giulia Conti", AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "fixture-italian-goal-run", CorpusID: "fixture-italian-goal-corpus"},
			{Source: domain.SourceMaterial{ID: "fixture-failed", OwnerID: OwnerID, Language: "de", Title: "Fehlgeschlagene Analyse", MediaType: "application/epub+zip", ContentRevisionID: "fixture-failed-revision", ContentSnapshotID: "fixture-failed-snapshot"}, BookAuthor: "Jonas Keller", AnalysisStatus: "analysis failed", AnalysisState: "failed", AnalysisJobID: 43},
			{Source: domain.SourceMaterial{ID: routeMatchBookID, OwnerID: OwnerID, Language: "de", Title: "Route match: familiar German", MediaType: "application/epub+zip", ContentRevisionID: "fixture-route-match-revision", ContentSnapshotID: "fixture-route-match-snapshot"}, BookAuthor: "Anja Roth", AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "fixture-route-match-run", CorpusID: "fixture-route-match-corpus"},
			{Source: domain.SourceMaterial{ID: routeDiffersBookID, OwnerID: OwnerID, Language: "de", Title: "Route differs: new German", MediaType: "application/epub+zip", ContentRevisionID: "fixture-route-differs-revision", ContentSnapshotID: "fixture-route-differs-snapshot"}, BookAuthor: "Paul Stein", AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "fixture-route-differs-run", CorpusID: "fixture-route-differs-corpus"},
			{Source: domain.SourceMaterial{ID: routeTieABookID, OwnerID: OwnerID, Language: "de", Title: "Route tie A", MediaType: "application/epub+zip", ContentRevisionID: "fixture-route-tie-a-revision", ContentSnapshotID: "fixture-route-tie-a-snapshot"}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "fixture-route-tie-a-run", CorpusID: "fixture-route-tie-a-corpus"},
			{Source: domain.SourceMaterial{ID: routeTieBBookID, OwnerID: OwnerID, Language: "de", Title: "Route tie B", MediaType: "application/epub+zip", ContentRevisionID: "fixture-route-tie-b-revision", ContentSnapshotID: "fixture-route-tie-b-snapshot"}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "fixture-route-tie-b-run", CorpusID: "fixture-route-tie-b-corpus"},
			{Source: domain.SourceMaterial{ID: routeUnavailableBookID, OwnerID: OwnerID, Language: "de", Title: "Route evidence pending", MediaType: "application/epub+zip"}, AnalysisStatus: "not analyzed", AnalysisState: ""},
			{Source: domain.SourceMaterial{ID: edgeBookID, OwnerID: OwnerID, Title: "Donaudampfschifffahrtsgesellschaftskapitänsmütze: Eine Geschichte der deutschen Wörter, langen Reisen und unerwarteten Begegnungen am Fluss", Language: "it", FullText: "La biblioteca conserva una storia italiana con molte parole e una descrizione volutamente assente."}, AnalysisStatus: "not analyzed", AnalysisState: ""},
			{Source: domain.SourceMaterial{ID: italianRouteBookID, OwnerID: OwnerID, Language: "it", Title: "Italian route baseline", MediaType: "application/epub+zip", ContentRevisionID: "fixture-italian-route-revision", ContentSnapshotID: "fixture-italian-route-snapshot"}, BookAuthor: "Luca Bianchi", AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "fixture-italian-route-run", CorpusID: "fixture-italian-route-corpus"},
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
			{ID: PrepID, OwnerID: OwnerID, SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, GoalSnapshotID: "fixture-de-goal-snapshot", State: domain.DeckPreparationReady, Filename: "Fixture German deck.apkg", DeckName: "Mouseion::de::Fixture", TotalCards: 3},
			{ID: QueuedPrepID, OwnerID: OwnerID, SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, State: domain.DeckPreparationReady, Filename: "Fixture German queued deck.apkg", DeckName: "Mouseion::de::Queued", TotalCards: 3},
		},
		deckVocabulary: []domain.DeckPreparationVocabulary{
			{OwnerID: OwnerID, DeckPreparationID: PrepID, Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", GeneratedAt: fixtureJourneyTime},
			{OwnerID: OwnerID, DeckPreparationID: PrepID, Language: "de", CanonicalLemma: "Weg", UPOS: "NOUN", GeneratedAt: fixtureJourneyTime},
		},
		known: []domain.KnownVocabulary{
			{ID: "fixture-known", OwnerID: OwnerID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", Provenance: "Explicitly recorded", CreatedAt: fixtureJourneyTime},
			{ID: "fixture-known-only", OwnerID: OwnerID, Language: "fr", CanonicalLemma: "bonjour", UPOS: "NOUN", Provenance: "Explicitly recorded", CreatedAt: fixtureJourneyTime},
			{ID: "fixture-independent-known", OwnerID: OwnerID, Language: "de", CanonicalLemma: IndependentKnownLemma, UPOS: "NOUN", Provenance: "Explicitly recorded", CreatedAt: fixtureJourneyTime},
			{ID: "fixture-graduated-known", OwnerID: OwnerID, Language: "de", CanonicalLemma: GraduatedKnownLemma, UPOS: "VERB", Provenance: "Graduated from reviewed deck", CreatedAt: fixtureJourneyTime.Add(2 * time.Hour)},
		},
		legacyGenerated: []domain.GeneratedVocabulary{{OwnerID: OwnerID, Language: "de", CanonicalLemma: LegacyGeneratedLemma, UPOS: "ADJ", FirstDeckID: "fixture-legacy-generated-deck", FirstGeneratedAt: fixtureJourneyTime}},
		myBooks: []domain.MyBook{{
			Book: domain.Book{ID: "fixture-metadata-only", OwnerID: OwnerID, Title: "Metadata-only migration book", Author: "Fixture Catalogue Author", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown, CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
		}, {
			Book: domain.Book{ID: BrowserSyncBookID, OwnerID: OwnerID, Title: "Browser sync metadata book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown, CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
		}},
		readingJourneys: map[string]domain.ReadingJourney{
			fixtureJourneyKey(OwnerID, "de"): {
				OwnerID: OwnerID, Language: "de", Revision: 1, UpdatedAt: fixtureJourneyTime,
				Entries: []domain.ReadingJourneyEntry{
					{OwnerID: OwnerID, Language: "de", BookID: BookID, Position: 1, CreatedAt: fixtureJourneyTime},
					{OwnerID: OwnerID, Language: "de", BookID: "fixture-failed", Position: 2, CreatedAt: fixtureJourneyTime.Add(time.Minute)},
					{OwnerID: OwnerID, Language: "de", BookID: routeMatchBookID, Position: 3, CreatedAt: fixtureJourneyTime.Add(2 * time.Minute)},
					{OwnerID: OwnerID, Language: "de", BookID: routeDiffersBookID, Position: 4, CreatedAt: fixtureJourneyTime.Add(3 * time.Minute)},
					{OwnerID: OwnerID, Language: "de", BookID: routeTieABookID, Position: 5, CreatedAt: fixtureJourneyTime.Add(4 * time.Minute)},
					{OwnerID: OwnerID, Language: "de", BookID: routeTieBBookID, Position: 6, CreatedAt: fixtureJourneyTime.Add(5 * time.Minute)},
					{OwnerID: OwnerID, Language: "de", BookID: routeUnavailableBookID, Position: 7, CreatedAt: fixtureJourneyTime.Add(6 * time.Minute)},
				},
			},
			fixtureJourneyKey(OwnerID, "it"): {
				OwnerID: OwnerID, Language: "it", Revision: 1, UpdatedAt: fixtureJourneyTime,
				Entries: []domain.ReadingJourneyEntry{
					{OwnerID: OwnerID, Language: "it", BookID: "fixture-empty", Position: 1, CreatedAt: fixtureJourneyTime},
					{OwnerID: OwnerID, Language: "it", BookID: edgeBookID, Position: 2, CreatedAt: fixtureJourneyTime.Add(time.Minute)},
					{OwnerID: OwnerID, Language: "it", BookID: italianRouteBookID, Position: 3, CreatedAt: fixtureJourneyTime.Add(2 * time.Minute)},
					{OwnerID: OwnerID, Language: "it", BookID: ItalianGoalBookID, Position: 4, CreatedAt: fixtureJourneyTime.Add(3 * time.Minute)},
				},
			},
		},
		primaryGoals: map[string]domain.PrimaryGoal{
			fixtureGoalKey(OwnerID, "de"): {OwnerID: OwnerID, Language: "de", BookID: BookID, SnapshotID: "fixture-de-goal-snapshot", SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, ContentRevisionID: "fixture-revision", ContentSnapshotID: "fixture-snapshot", CorpusID: "fixture-corpus", SnapshotSize: 2, CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
			fixtureGoalKey(OwnerID, "it"): {OwnerID: OwnerID, Language: "it", BookID: ItalianGoalBookID, SnapshotID: "fixture-it-goal-snapshot", SourceMaterialID: ItalianGoalBookID, AnalysisRunID: "fixture-italian-goal-run", ContentRevisionID: "fixture-italian-goal-revision", ContentSnapshotID: "fixture-italian-goal-snapshot", CorpusID: "fixture-italian-goal-corpus", CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
		},
		goalSnapshotVocabulary: map[string][]domain.DeckPreparationVocabulary{
			"fixture-de-goal-snapshot": {
				{OwnerID: OwnerID, Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", GeneratedAt: fixtureJourneyTime},
				{OwnerID: OwnerID, Language: "de", CanonicalLemma: "Weg", UPOS: "NOUN", GeneratedAt: fixtureJourneyTime},
			},
		},
		readingHistory:       make(map[string]domain.ReadingCompletion),
		storedActiveLanguage: &initialActiveLanguage,
		mostRecentLanguage:   "it",
	}
}

// SetGoalSnapshotVocabulary lets acceptance tests add a deterministic frozen
// snapshot without exposing the fixture store's internal maps.
func (s *Store) SetGoalSnapshotVocabulary(snapshotID string, vocabulary []domain.DeckPreparationVocabulary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goalSnapshotVocabulary[snapshotID] = append([]domain.DeckPreparationVocabulary(nil), vocabulary...)
}

func (s *Store) GetStoredActiveStudyLanguage(_ context.Context, owner string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != OwnerID {
		return "", errNotFound
	}
	if s.storedActiveLanguage == nil {
		return "", nil
	}
	return *s.storedActiveLanguage, nil
}

func (s *Store) SetActiveStudyLanguage(_ context.Context, owner, language string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != OwnerID {
		return errNotFound
	}
	language = normalizeFixtureLanguage(language)
	if language == "" {
		s.storedActiveLanguage = nil
	} else {
		s.storedActiveLanguage = &language
	}
	return nil
}

func (s *Store) MostRecentlyActivatedStudyLanguage(_ context.Context, owner string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != OwnerID {
		return "", errNotFound
	}
	return s.mostRecentLanguage, nil
}

func (s *Store) arriveNextFixtureStudyLanguage() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, arrival := range fixtureSyncArrivals {
		language := normalizeFixtureLanguage(arrival.Language)
		alreadyPresent := false
		for _, book := range s.books {
			if book.Source.OwnerID == OwnerID && normalizeFixtureLanguage(book.Source.Language) == language {
				alreadyPresent = true
				break
			}
		}
		if alreadyPresent {
			continue
		}
		s.books = append(s.books, domain.SourceMaterialSummary{
			Source: domain.SourceMaterial{
				ID:        "fixture-arrival-" + language,
				OwnerID:   OwnerID,
				Language:  language,
				Title:     arrival.DisplayName + " arrival",
				MediaType: "application/epub+zip",
			},
			BookID: "fixture-arrival-" + language,
		})
		s.supported = append(s.supported, arrival)
		s.mostRecentLanguage = language
		return
	}
}
func (s *Store) PutSupportedLanguage(context.Context, string, string) (domain.SupportedLanguage, error) {
	return domain.SupportedLanguage{}, nil
}
func (s *Store) ListSupportedLanguages(_ context.Context) ([]domain.SupportedLanguage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.SupportedLanguage(nil), s.supported...), nil
}
func (s *Store) SyncSupportedLanguages(context.Context, []domain.SupportedLanguage) error { return nil }
func (s *Store) ListStudyLanguages(_ context.Context, owner string) ([]domain.StudyLanguage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Fixture source summaries represent the active acquired library books; the
	// fixture does not model a separate membership row for them.
	seen := make(map[string]struct{})
	displayNames := make(map[string]string, len(s.supported))
	for _, language := range s.supported {
		tag := normalizeFixtureLanguage(language.Language)
		if tag != "" {
			displayNames[tag] = language.DisplayName
		}
	}
	var out []domain.StudyLanguage
	addStudyLanguage := func(raw string) {
		language := normalizeFixtureLanguage(raw)
		if language == "" {
			return
		}
		if _, ok := seen[language]; ok {
			return
		}
		seen[language] = struct{}{}
		displayName := displayNames[language]
		if displayName == "" {
			displayName = language
		}
		out = append(out, domain.StudyLanguage{Language: language, DisplayName: displayName})
	}
	for _, book := range s.books {
		if book.Source.OwnerID != owner || strings.TrimSpace(book.Source.Language) == "" {
			continue
		}
		addStudyLanguage(book.Source.Language)
	}
	for _, book := range s.myBooks {
		if book.Book.OwnerID != owner || book.Book.LanguageState != domain.LanguageChosen || strings.TrimSpace(book.Book.LanguageTag) == "" {
			continue
		}
		addStudyLanguage(book.Book.LanguageTag)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Language < out[j].Language })
	return out, nil
}
func (s *Store) ListKnownVocabularyLanguages(_ context.Context, owner string) ([]domain.StudyLanguage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]struct{})
	displayNames := make(map[string]string, len(s.supported))
	for _, language := range s.supported {
		tag := normalizeFixtureLanguage(language.Language)
		if tag != "" {
			displayNames[tag] = language.DisplayName
		}
	}
	var out []domain.StudyLanguage
	for _, entry := range s.known {
		if entry.OwnerID != owner {
			continue
		}
		language := normalizeFixtureLanguage(entry.Language)
		if language == "" {
			continue
		}
		if _, ok := seen[language]; ok {
			continue
		}
		seen[language] = struct{}{}
		name := displayNames[language]
		if name == "" {
			name = language
		}
		out = append(out, domain.StudyLanguage{Language: language, DisplayName: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Language < out[j].Language })
	return out, nil
}
func (s *Store) CreateOpdsConnection(_ context.Context, o string, c domain.OpdsConnection) (domain.OpdsConnection, error) {
	c.ID = "fixture-new-connection"
	c.OwnerID = o
	s.connections = append(s.connections, c)
	return c, nil
}
func (s *Store) GetOpdsConnection(_ context.Context, o, id string) (domain.OpdsConnection, error) {
	for _, c := range s.connections {
		if c.ID == id && c.OwnerID == o {
			return c, nil
		}
	}
	return domain.OpdsConnection{}, errNotFound
}
func (s *Store) ListOpdsConnections(context.Context, string) ([]domain.OpdsConnection, error) {
	return append([]domain.OpdsConnection(nil), s.connections...), nil
}
func (s *Store) ListCatalogueSyncStatuses(_ context.Context, owner string) ([]domain.CatalogueSyncStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []domain.CatalogueSyncStatus
	for _, status := range s.syncStatuses {
		if status.OwnerID == owner {
			result = append(result, status)
		}
	}
	return result, nil
}
func (s *Store) SetCatalogueSyncStatus(_ context.Context, status domain.CatalogueSyncStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if status.UpdatedAt.IsZero() {
		status.UpdatedAt = fixtureJourneyTime
	}
	for i := range s.syncStatuses {
		if s.syncStatuses[i].OwnerID == status.OwnerID && s.syncStatuses[i].ConnectionID == status.ConnectionID {
			s.syncStatuses[i] = status
			return nil
		}
	}
	s.syncStatuses = append(s.syncStatuses, status)
	return nil
}
func (s *Store) UpdateOpdsConnection(_ context.Context, _ string, c domain.OpdsConnection) (domain.OpdsConnection, error) {
	return c, nil
}
func (s *Store) DeleteOpdsConnection(context.Context, string, string) error { return nil }
func (s *Store) ListSourceMaterials(_ context.Context, owner string) ([]domain.SourceMaterialSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []domain.SourceMaterialSummary
	for _, book := range s.books {
		if book.Source.OwnerID == owner {
			if book.BookTitle == "" {
				book.BookTitle = book.Source.Title
			}
			result = append(result, book)
		}
	}
	return result, nil
}
func (s *Store) ListMyBooksWithEvidence(_ context.Context, owner string) ([]domain.MyBook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.myBooksForOwner(owner), nil
}

// GetAnalysisCorpusVocabulary provides deterministic corpus facts for the
// forecast provider. The shared recurring identity makes sequential changes
// visible in browser smoke tests without involving a real analyzer.
func (s *Store) GetAnalysisCorpusVocabulary(_ context.Context, _ string, corpusID string) (domain.AnalysisCorpusVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var source domain.SourceMaterialSummary
	for _, book := range s.books {
		if book.CorpusID == corpusID {
			source = book
			break
		}
	}
	if source.CorpusID == "" {
		return domain.AnalysisCorpusVocabulary{}, errNotFound
	}
	knownTokens := fixtureForecastKnownTokens(corpusID)
	sharedTokens := int64(5)
	if knownTokens+sharedTokens > 100 {
		sharedTokens = max(100-knownTokens, 0)
	}
	lemmas := []domain.LemmaOccurrence{}
	if knownTokens > 0 {
		lemmas = append(lemmas, domain.LemmaOccurrence{Language: source.Source.Language, CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: knownTokens})
	}
	if sharedTokens > 0 {
		lemmas = append(lemmas, domain.LemmaOccurrence{Language: source.Source.Language, CanonicalLemma: "fixture-recurring", UPOS: "NOUN", OccurrenceCount: sharedTokens})
	}
	if remaining := 100 - knownTokens - sharedTokens; remaining > 0 {
		lemmas = append(lemmas, domain.LemmaOccurrence{Language: source.Source.Language, CanonicalLemma: "fixture-" + corpusID, UPOS: "NOUN", OccurrenceCount: remaining})
	}
	return domain.AnalysisCorpusVocabulary{
		CorpusID: corpusID, SourceMaterialID: source.Source.ID, AnalysisRunID: source.AnalysisRunID,
		Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: int64(len(lemmas))}, Lemmas: lemmas,
	}, nil
}

func fixtureForecastKnownTokens(corpusID string) int64 {
	switch corpusID {
	case "fixture-corpus", "fixture-route-match-corpus":
		return 90
	case "fixture-route-differs-corpus":
		return 20
	case "fixture-route-tie-a-corpus", "fixture-route-tie-b-corpus":
		return 50
	case "fixture-italian-goal-corpus":
		return 60
	default:
		return 0
	}
}

func (s *Store) GetBookDetail(_ context.Context, owner, id string) (domain.MyBook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	books := s.myBooksForOwner(owner)
	for _, book := range books {
		if book.Book.ID == id {
			return book, nil
		}
	}
	for _, book := range books {
		if book.Acquired != nil && book.Acquired.Source.ID == id {
			return book, nil
		}
	}
	return domain.MyBook{}, errNotFound
}

func (s *Store) myBooksForOwner(owner string) []domain.MyBook {
	out := make([]domain.MyBook, 0, len(s.books)+len(s.myBooks))
	for i := range s.books {
		source := s.books[i]
		if owner != "" && source.Source.OwnerID != owner {
			continue
		}
		languageState := domain.LanguageChosen
		languageTag := normalizeFixtureLanguage(source.Source.Language)
		if strings.TrimSpace(languageTag) == "" {
			languageState = domain.LanguageUnknown
			languageTag = ""
		}
		bookID := source.BookID
		if bookID == "" {
			bookID = source.Source.ID
		}
		if source.BookTitle == "" {
			source.BookTitle = source.Source.Title
		}
		out = append(out, domain.MyBook{Book: domain.Book{ID: bookID, OwnerID: source.Source.OwnerID, Title: source.BookTitle, Author: source.BookAuthor, LanguageState: languageState, LanguageTag: languageTag}, Acquired: &source})
	}
	for _, book := range s.myBooks {
		if owner == "" || book.Book.OwnerID == owner {
			out = append(out, book)
		}
	}
	return out
}

// ListMyBooksBrowse mirrors the production collection browser in memory for
// the shared browser fixture store: literal case-insensitive title substring
// search, one language filter, lowercased deterministic title ordering, and
// counts over the complete active owner collection.
func (s *Store) ListMyBooksBrowse(_ context.Context, owner, query, language string, offset, limit int) (persistence.MyBooksBrowseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query = strings.ToLower(strings.TrimSpace(query))
	language = strings.TrimSpace(language)
	if language != domain.LanguageUnknown {
		language = normalizeFixtureLanguage(language)
	}
	all := s.myBooksForOwner(owner)
	result := persistence.MyBooksBrowseResult{AllCount: len(all)}
	counts := map[string]int{}
	for _, book := range all {
		tag := domain.LanguageUnknown
		if book.Book.LanguageState == domain.LanguageChosen {
			tag = normalizeFixtureLanguage(book.Book.LanguageTag)
		}
		counts[tag]++
	}
	for tag, count := range counts {
		result.Counts = append(result.Counts, persistence.LanguageCount{Tag: tag, Count: count})
	}
	sort.Slice(result.Counts, func(i, j int) bool {
		if result.Counts[i].Tag == domain.LanguageUnknown {
			return false
		}
		if result.Counts[j].Tag == domain.LanguageUnknown {
			return true
		}
		return result.Counts[i].Tag < result.Counts[j].Tag
	})

	filtered := make([]domain.MyBook, 0, len(all))
	for _, book := range all {
		if language == "" || (language == domain.LanguageUnknown && book.Book.LanguageState == domain.LanguageUnknown) || (language != domain.LanguageUnknown && book.Book.LanguageState == domain.LanguageChosen && normalizeFixtureLanguage(book.Book.LanguageTag) == language) {
			result.ScopeTotal++
		}
		if query != "" && !strings.Contains(strings.ToLower(book.Book.Title), query) {
			continue
		}
		if language == domain.LanguageUnknown {
			if book.Book.LanguageState != domain.LanguageUnknown {
				continue
			}
		} else if language != "" && (book.Book.LanguageState != domain.LanguageChosen || normalizeFixtureLanguage(book.Book.LanguageTag) != language) {
			continue
		}
		filtered = append(filtered, book)
	}
	sort.Slice(filtered, func(i, j int) bool {
		left, right := filtered[i].Book, filtered[j].Book
		leftTitle, rightTitle := strings.ToLower(left.Title), strings.ToLower(right.Title)
		if leftTitle != rightTitle {
			return leftTitle < rightTitle
		}
		if left.Title != right.Title {
			return left.Title < right.Title
		}
		return left.ID < right.ID
	})
	result.Total = len(filtered)
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	if offset < len(filtered) {
		end := offset + limit
		if end < offset || end > len(filtered) {
			end = len(filtered)
		}
		result.Items = filtered[offset:end]
	}
	return result, nil
}
func (s *Store) IsMetadataOnlyMyBook(_ context.Context, owner, bookID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, source := range s.books {
		if source.Source.OwnerID == owner && (source.Source.ID == bookID || source.BookID == bookID) {
			return false, nil
		}
	}
	for _, book := range s.myBooks {
		if book.Book.OwnerID == owner && book.Book.ID == bookID {
			return true, nil
		}
	}
	return false, nil
}
func (s *Store) ListAnalysisJobs(context.Context, string) ([]domain.AnalysisJob, error) {
	return append([]domain.AnalysisJob(nil), s.jobs...), nil
}
func (s *Store) ListKnownVocabulary(_ context.Context, owner, language string) ([]domain.KnownVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	var result []domain.KnownVocabulary
	for _, entry := range s.known {
		if entry.OwnerID == owner && normalizeFixtureLanguage(entry.Language) == language {
			result = append(result, entry)
		}
	}
	return result, nil
}

// reservedDeckVocabularyLocked returns the active Goal's frozen vocabulary.
// Callers hold s.mu.
func (s *Store) reservedDeckVocabularyLocked(owner, language string) []domain.DeckPreparationVocabulary {
	var result []domain.DeckPreparationVocabulary
	seen := make(map[string]struct{})
	goal := s.primaryGoals[fixtureGoalKey(owner, language)]
	hasSnapshot := false
	if goal.IsActive() && goal.SnapshotID != "" {
		snapshot, snapshotExists := s.goalSnapshotVocabulary[goal.SnapshotID]
		hasSnapshot = snapshotExists
		for _, item := range snapshot {
			if item.Language == language {
				item.GraduatedAt = nil
				result = append(result, item)
				seen[item.Language+"\x00"+item.CanonicalLemma+"\x00"+item.UPOS] = struct{}{}
			}
		}
	}
	if !hasSnapshot && goal.IsActive() && goal.SourceMaterialID != "" {
		for _, preparation := range s.preps {
			if preparation.OwnerID != owner || preparation.SourceMaterialID != goal.SourceMaterialID || preparation.AnalysisRunID != goal.AnalysisRunID {
				continue
			}
			for _, item := range s.deckVocabularyFor(owner, preparation.ID) {
				if item.Language == language {
					item.GraduatedAt = nil
					result = append(result, item)
					seen[item.Language+"\x00"+item.CanonicalLemma+"\x00"+item.UPOS] = struct{}{}
				}
			}
		}
	}
	return result
}

func (s *Store) CountPrimaryGoalVocabularyToGraduate(_ context.Context, owner, language string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, item := range s.goalSnapshotVocabularyLocked(owner, language) {
		if !fixtureKnown(s.known, item.Language, item.CanonicalLemma, item.UPOS) {
			count++
		}
	}
	return count, nil
}

func (s *Store) goalSnapshotVocabularyLocked(owner, language string) []domain.DeckPreparationVocabulary {
	goal := s.primaryGoals[fixtureGoalKey(owner, language)]
	if !goal.IsActive() || goal.SnapshotID == "" {
		return nil
	}
	snapshot, exists := s.goalSnapshotVocabulary[goal.SnapshotID]
	if !exists {
		return nil
	}
	result := make([]domain.DeckPreparationVocabulary, 0, len(snapshot))
	for _, item := range snapshot {
		if item.Language == language {
			item.GraduatedAt = nil
			result = append(result, item)
		}
	}
	return result
}

// ListReservedVocabulary returns the vocabulary currently reserved by the
// owner's active Goal snapshot, scoped to one language. It is the read seam
// used by the coverage service.
func (s *Store) ListReservedVocabulary(_ context.Context, owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reservedDeckVocabularyLocked(owner, language), nil
}

func (s *Store) deckVocabularyFor(owner, preparationID string) []domain.DeckPreparationVocabulary {
	var result []domain.DeckPreparationVocabulary
	for _, item := range s.deckVocabulary {
		if item.OwnerID == owner && item.DeckPreparationID == preparationID {
			result = append(result, item)
		}
	}
	return result
}

func (s *Store) ListUnattachedGeneratedVocabulary(_ context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []domain.GeneratedVocabulary
	for _, item := range s.legacyGenerated {
		if item.OwnerID == owner && item.Language == language {
			result = append(result, item)
		}
	}
	return result, nil
}
func (s *Store) GetDeckPreparationForAnalysis(_ context.Context, owner, sourceMaterialID, analysisRunID string) (domain.DeckPreparation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, preparation := range s.preps {
		if preparation.OwnerID == owner && preparation.SourceMaterialID == sourceMaterialID && preparation.AnalysisRunID == analysisRunID && preparation.GoalSnapshotID == "" && preparation.RetiredAt == nil {
			preparation.VocabularyCount = len(s.deckVocabularyFor(owner, preparation.ID))
			return preparation, nil
		}
	}
	return domain.DeckPreparation{}, errNotFound
}

func (s *Store) GetDeckPreparationForGoalSnapshot(_ context.Context, owner, snapshotID string) (domain.DeckPreparation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, preparation := range s.preps {
		if preparation.OwnerID == owner && preparation.GoalSnapshotID == snapshotID && preparation.RetiredAt == nil {
			preparation.VocabularyCount = len(s.deckVocabularyFor(owner, preparation.ID))
			return preparation, nil
		}
	}
	return domain.DeckPreparation{}, errNotFound
}

func (s *Store) ListDeckPreparationsForSourceMaterial(_ context.Context, owner, sourceMaterialID string) ([]domain.DeckPreparation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []domain.DeckPreparation
	for _, preparation := range s.preps {
		if preparation.OwnerID == owner && preparation.SourceMaterialID == sourceMaterialID {
			preparation.VocabularyCount = len(s.deckVocabularyFor(owner, preparation.ID))
			result = append(result, preparation)
		}
	}
	return result, nil
}

func (s *Store) GetSourceMaterial(_ context.Context, o, id string) (domain.SourceMaterial, error) {
	for _, b := range s.books {
		if b.Source.ID == id && b.Source.OwnerID == o {
			return b.Source, nil
		}
	}
	return domain.SourceMaterial{}, errNotFound
}
func (s *Store) GetExtractedUnitSnapshot(context.Context, string, string) (string, domain.ExtractedUnits, error) {
	text := "Haus. Ein kurzer deutscher Satz."
	second := "Ein sehr langer Beispielsatz mit vielen Wörtern für die Anzeige von realistischem Randinhalt im Browser."
	return "fixture-snapshot", domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{
		{ID: domain.EPUBUnitID(0, "fixture-001"), Order: 0, SpineIndex: 0, ManifestID: "fixture-001", Title: "Chapter one", Text: text, StartOffset: 0, EndOffset: uint64(len([]rune(text))), MediaType: "application/xhtml+xml", Linear: true},
		{ID: domain.EPUBUnitID(1, "fixture-002"), Order: 1, SpineIndex: 1, ManifestID: "fixture-002", Title: "Chapter two", Text: second, StartOffset: uint64(len([]rune(text)) + 2), EndOffset: uint64(len([]rune(text)) + 2 + len([]rune(second))), MediaType: "application/xhtml+xml", Linear: true},
	}}, nil
}

// My Books persistence is intentionally small in the browser fixture; these
// methods cover the learner-facing metadata controls without a database.
func (s *Store) ListMyBooks(context.Context, string) ([]domain.Book, error) { return nil, nil }

func (s *Store) GetBookCoverResource(context.Context, string, string) (domain.BookCoverResource, error) {
	return domain.BookCoverResource{}, errNotFound
}

func (s *Store) GetBook(_ context.Context, owner, bookID string) (domain.Book, error) {
	for _, book := range s.myBooks {
		if book.Book.OwnerID == owner && book.Book.ID == bookID {
			return book.Book, nil
		}
	}
	for _, source := range s.books {
		if source.Source.OwnerID == owner && (source.Source.ID == bookID || source.BookID == bookID) {
			state := domain.LanguageChosen
			if strings.TrimSpace(source.Source.Language) == "" {
				state = domain.LanguageUnknown
			}
			resolvedBookID := source.BookID
			if resolvedBookID == "" {
				resolvedBookID = source.Source.ID
			}
			title := source.BookTitle
			if title == "" {
				title = source.Source.Title
			}
			return domain.Book{ID: resolvedBookID, OwnerID: owner, Title: title, Author: source.BookAuthor, LanguageState: state, LanguageTag: source.Source.Language}, nil
		}
	}
	return domain.Book{}, errNotFound
}
func (s *Store) CreateBook(_ context.Context, book domain.Book) (domain.Book, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	book.ID = fmt.Sprintf("fixture-metadata-%d", len(s.myBooks)+1)
	s.myBooks = append(s.myBooks, domain.MyBook{Book: book})
	return book, nil
}
func (s *Store) UpdateBookMetadata(_ context.Context, owner, bookID, title, author, languageState, languageTag string) (domain.Book, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if languageState == domain.LanguageChosen {
		languageTag = canonicalization.NormalizeLanguage(languageTag)
	}
	for i := range s.books {
		if s.books[i].Source.OwnerID == owner && (s.books[i].Source.ID == bookID || s.books[i].BookID == bookID) {
			resolvedBookID := s.books[i].BookID
			if resolvedBookID == "" {
				resolvedBookID = s.books[i].Source.ID
			}
			s.books[i].BookTitle = title
			s.books[i].BookAuthor = strings.TrimSpace(author)
			if languageState == domain.LanguageUnknown {
				languageTag = ""
			}
			s.books[i].Source.Language = languageTag
			s.clearFixtureGoalsForRetaggedBook(owner, resolvedBookID, languageState, languageTag)
			return domain.Book{ID: resolvedBookID, OwnerID: owner, Title: title, Author: strings.TrimSpace(author), LanguageState: languageState, LanguageTag: languageTag}, nil
		}
	}
	for i := range s.myBooks {
		if s.myBooks[i].Book.OwnerID == owner && s.myBooks[i].Book.ID == bookID {
			s.myBooks[i].Book.Title = title
			s.myBooks[i].Book.Author = strings.TrimSpace(author)
			s.myBooks[i].Book.LanguageState = languageState
			s.myBooks[i].Book.LanguageTag = languageTag
			s.clearFixtureGoalsForRetaggedBook(owner, bookID, languageState, languageTag)
			return s.myBooks[i].Book, nil
		}
	}
	return domain.Book{}, errNotFound
}

func (s *Store) clearFixtureGoalsForRetaggedBook(owner, bookID, languageState, language string) {
	language = normalizeFixtureLanguage(language)
	for key, goal := range s.primaryGoals {
		if goal.OwnerID != owner || goal.BookID != bookID {
			continue
		}
		if languageState != domain.LanguageChosen || goal.Language != language {
			delete(s.primaryGoals, key)
		}
	}
}
func (s *Store) AddBookToMyBooks(context.Context, string, string) error { return nil }
func (s *Store) RemoveBookFromMyBooks(_ context.Context, owner, bookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.myBooks {
		if s.myBooks[i].Book.OwnerID == owner && s.myBooks[i].Book.ID == bookID {
			s.myBooks = append(s.myBooks[:i], s.myBooks[i+1:]...)
			return nil
		}
	}
	return nil
}
func (s *Store) ResolveBookByAlias(ctx context.Context, owner, namespace, value string) (domain.Book, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, alias := range s.aliases {
		if alias.OwnerID == owner && alias.Namespace == namespace && alias.Value == value {
			for _, book := range s.myBooksForOwner(owner) {
				if book.Book.ID == alias.BookID {
					return book.Book, true, nil
				}
			}
		}
	}
	return domain.Book{}, false, nil
}
func (s *Store) AddBookAlias(context.Context, string, string, string, string, string) error {
	return nil
}

func (s *Store) GetBookCatalogEntryAlias(_ context.Context, owner, bookID string) (domain.BookAlias, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, alias := range s.aliases {
		if alias.OwnerID == owner && alias.BookID == bookID && alias.AliasType == domain.AliasCatalogEntry && alias.Namespace == domain.NamespaceSourceIdentifier {
			return alias, nil
		}
	}
	return domain.BookAlias{}, errNotFound
}
func (s *Store) LinkSourceToBook(context.Context, string, string, string) error { return nil }
func (s *Store) ResolveOrCreateBookForAcquisition(context.Context, string, string, string, string) (string, error) {
	return "", errNotFound
}
func (s *Store) GetReadingJourney(_ context.Context, owner, language string) (domain.ReadingJourney, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	journey := s.readingJourneys[fixtureJourneyKey(owner, language)]
	journey.OwnerID, journey.Language = owner, language
	journey.Entries = append([]domain.ReadingJourneyEntry(nil), journey.Entries...)
	return journey, nil
}
func (s *Store) ResolveJourneyBookID(_ context.Context, owner, id string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bookID := s.fixtureBookID(owner, id)
	return bookID, bookID != "", nil
}
func (s *Store) AddToReadingJourney(_ context.Context, owner, language, bookID string, expectedRevision int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	if language == "" {
		return 0, persistence.ErrJourneyLanguageRequired
	}
	key := fixtureJourneyKey(owner, language)
	journey := s.readingJourneys[key]
	if expectedRevision != journey.Revision {
		return 0, persistence.ErrJourneyStale
	}
	if !s.fixtureBookExists(owner, bookID) {
		return 0, errNotFound
	}
	bookID = s.fixtureBookID(owner, bookID)
	if !s.fixtureBookHasChosenLanguage(owner, bookID) || s.fixtureBookLanguage(owner, bookID) != language {
		return 0, persistence.ErrBookLanguageRequired
	}
	for _, entry := range journey.Entries {
		if entry.BookID == bookID {
			return journey.Revision, nil
		}
	}
	journey.OwnerID, journey.Language = owner, language
	journey.Entries = append(journey.Entries, domain.ReadingJourneyEntry{OwnerID: owner, Language: language, BookID: bookID, Position: len(journey.Entries) + 1, CreatedAt: time.Now()})
	journey.Revision++
	journey.UpdatedAt = time.Now()
	s.readingJourneys[key] = journey
	return journey.Revision, nil
}
func (s *Store) RemoveFromReadingJourney(_ context.Context, owner, language, bookID string, expectedRevision int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	key := fixtureJourneyKey(owner, language)
	journey, exists := s.readingJourneys[key]
	if expectedRevision != journey.Revision {
		return 0, persistence.ErrJourneyStale
	}
	if !exists {
		return 0, nil
	}
	bookID = s.fixtureBookID(owner, bookID)
	member := -1
	for i, entry := range journey.Entries {
		if entry.BookID == bookID {
			member = i
			break
		}
	}
	if member < 0 {
		return journey.Revision, nil
	}
	journey.Entries = append(journey.Entries[:member], journey.Entries[member+1:]...)
	for i := range journey.Entries {
		journey.Entries[i].Position = i + 1
	}
	journey.Revision++
	journey.UpdatedAt = time.Now()
	if goal := s.primaryGoals[fixtureGoalKey(owner, language)]; goal.BookID == bookID {
		delete(s.primaryGoals, fixtureGoalKey(owner, language))
	}
	if len(journey.Entries) == 0 && !s.fixtureLanguageDerived(owner, language) {
		delete(s.readingJourneys, key)
		return 0, nil
	}
	s.readingJourneys[key] = journey
	return journey.Revision, nil
}

// MoveReadingJourneyEntry mirrors the Postgres store: when a Primary Goal book is
// a Journey member it is anchored and invisible to the provisional order, so
// newPosition is interpreted within the Goal-excluded order and the Goal entry
// is never moved.
func (s *Store) MoveReadingJourneyEntry(_ context.Context, owner, language, bookID string, newPosition int, expectedRevision int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	key := fixtureJourneyKey(owner, language)
	journey, exists := s.readingJourneys[key]
	if expectedRevision != journey.Revision {
		return 0, persistence.ErrJourneyStale
	}
	if !exists {
		return 0, persistence.ErrNotFound
	}
	memberIndex := -1
	for i, entry := range journey.Entries {
		if entry.BookID == bookID {
			memberIndex = i
			break
		}
	}
	if memberIndex == -1 {
		return 0, persistence.ErrNotFound
	}
	if newPosition < 1 {
		newPosition = 1
	}
	if newPosition > len(journey.Entries) {
		newPosition = len(journey.Entries)
	}
	goalBookID := s.primaryGoals[fixtureGoalKey(owner, language)].BookID
	goalIndex := -1
	if goalBookID != "" && goalBookID != bookID {
		visible := make([]domain.ReadingJourneyEntry, 0, len(journey.Entries)-1)
		visibleIndex := 0
		for index, entry := range journey.Entries {
			if entry.BookID == goalBookID {
				goalIndex = index
				continue
			}
			if entry.BookID == bookID {
				visibleIndex = len(visible)
			}
			visible = append(visible, entry)
		}
		if goalIndex >= 0 {
			if newPosition < 1 {
				newPosition = 1
			}
			if newPosition > len(visible) {
				newPosition = len(visible)
			}
			if visibleIndex == newPosition-1 {
				return journey.Revision, nil
			}
			entry := visible[visibleIndex]
			visible = append(visible[:visibleIndex], visible[visibleIndex+1:]...)
			visible = append(visible, domain.ReadingJourneyEntry{})
			copy(visible[newPosition:], visible[newPosition-1:])
			visible[newPosition-1] = entry
			journey.Entries = make([]domain.ReadingJourneyEntry, 0, len(visible)+1)
			visibleIndex = 0
			for index := range len(visible) + 1 {
				if index == goalIndex {
					journey.Entries = append(journey.Entries, domain.ReadingJourneyEntry{OwnerID: owner, Language: language, BookID: goalBookID})
					continue
				}
				journey.Entries = append(journey.Entries, visible[visibleIndex])
				visibleIndex++
			}
		} else {
			goalBookID = ""
		}
	}
	if goalBookID == "" {
		if memberIndex == newPosition-1 {
			return journey.Revision, nil
		}
		entry := journey.Entries[memberIndex]
		journey.Entries = append(journey.Entries[:memberIndex], journey.Entries[memberIndex+1:]...)
		journey.Entries = append(journey.Entries, domain.ReadingJourneyEntry{})
		copy(journey.Entries[newPosition:], journey.Entries[newPosition-1:])
		journey.Entries[newPosition-1] = entry
	}
	for i := range journey.Entries {
		journey.Entries[i].OwnerID = owner
		journey.Entries[i].Position = i + 1
	}
	journey.Revision++
	journey.UpdatedAt = time.Now()
	s.readingJourneys[key] = journey
	return journey.Revision, nil
}
func (s *Store) GetPrimaryGoal(_ context.Context, owner, language string) (domain.PrimaryGoal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	goal, ok := s.primaryGoals[fixtureGoalKey(owner, language)]
	if !ok {
		return domain.PrimaryGoal{}, nil
	}
	return goal, nil
}

func (s *Store) ListPrimaryGoalSnapshotVocabulary(_ context.Context, owner, snapshotID string) ([]domain.SelectionCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	goal, found := s.goalSnapshotVocabulary[snapshotID]
	if !found {
		return nil, nil
	}
	result := make([]domain.SelectionCandidate, 0, len(goal))
	for _, item := range goal {
		result = append(result, domain.SelectionCandidate{OwnerID: owner, Language: item.Language, CanonicalLemma: item.CanonicalLemma, UPOS: item.UPOS})
	}
	return result, nil
}
func (s *Store) CreatePrimaryGoal(_ context.Context, owner, language, bookID string) (domain.PrimaryGoal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	goal := domain.PrimaryGoal{OwnerID: owner, Language: language, BookID: bookID}
	if err := goal.Validate(); err != nil {
		return domain.PrimaryGoal{}, err
	}
	if !s.fixtureBookExists(owner, bookID) {
		return domain.PrimaryGoal{}, errNotFound
	}
	bookID = s.fixtureBookID(owner, bookID)
	key := fixtureGoalKey(owner, language)
	if _, ok := s.primaryGoals[key]; ok {
		return domain.PrimaryGoal{}, persistence.ErrGoalExists
	}
	if !s.fixturePrimaryGoalEligible(owner, language, bookID) {
		return domain.PrimaryGoal{}, persistence.ErrGoalIneligible
	}
	now := time.Now()
	goal = s.fixtureGoalFromBook(owner, language, bookID, now)
	goal.CreatedAt, goal.UpdatedAt = now, now
	s.primaryGoals[key] = goal
	return goal, nil
}
func (s *Store) ChangePrimaryGoal(_ context.Context, owner, language, bookID, expectedBookID string) (domain.PrimaryGoal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	key := fixtureGoalKey(owner, language)
	goal, ok := s.primaryGoals[key]
	if !ok {
		return domain.PrimaryGoal{}, persistence.ErrNotFound
	}
	if goal.BookID != expectedBookID {
		return domain.PrimaryGoal{}, persistence.ErrGoalStale
	}
	if !s.fixtureBookExists(owner, bookID) {
		return domain.PrimaryGoal{}, errNotFound
	}
	bookID = s.fixtureBookID(owner, bookID)
	if !s.fixturePrimaryGoalEligible(owner, language, bookID) {
		return domain.PrimaryGoal{}, persistence.ErrGoalIneligible
	}
	goal = s.fixtureGoalFromBook(owner, language, bookID, goal.CreatedAt)
	goal.UpdatedAt = time.Now()
	s.primaryGoals[key] = goal
	return goal, nil
}

func (s *Store) fixtureGoalFromBook(owner, language, bookID string, createdAt time.Time) domain.PrimaryGoal {
	snapshotID := "fixture-goal-" + language + "-" + bookID
	for suffix := 2; s.readingHistory[fixtureReadingHistoryKey(owner, language, snapshotID)].BookID != ""; suffix++ {
		snapshotID = fmt.Sprintf("fixture-goal-%s-%s-%d", language, bookID, suffix)
	}
	goal := domain.PrimaryGoal{OwnerID: owner, Language: language, BookID: bookID, SnapshotID: snapshotID, CreatedAt: createdAt}
	goal.SnapshotSize = len(s.goalSnapshotVocabulary[goal.SnapshotID])
	for _, book := range s.books {
		if book.Source.OwnerID != owner || s.fixtureBookID(owner, book.Source.ID) != bookID {
			continue
		}
		goal.SourceMaterialID = book.Source.ID
		goal.AnalysisRunID = book.AnalysisRunID
		goal.ContentRevisionID = book.Source.ContentRevisionID
		goal.ContentSnapshotID = book.Source.ContentSnapshotID
		goal.CorpusID = book.CorpusID
		break
	}
	return goal
}

func (s *Store) fixturePrimaryGoalEligible(owner, language, bookID string) bool {
	language = normalizeFixtureLanguage(language)
	journey, exists := s.readingJourneys[fixtureJourneyKey(owner, language)]
	if !exists {
		return false
	}
	for _, entry := range journey.Entries {
		if entry.BookID != bookID {
			continue
		}
		for _, book := range s.books {
			if s.fixtureBookID(owner, book.Source.ID) == bookID && normalizeFixtureLanguage(book.Source.Language) == language && strings.EqualFold(book.Source.MediaType, opds.EPUBMediaType) && book.AnalysisStatus == "analyzed" && book.AnalysisState == "completed" && book.AnalysisRunID != "" && book.CorpusID != "" && book.Source.ContentRevisionID != "" {
				return true
			}
		}
	}
	return false
}
func (s *Store) ClearPrimaryGoal(_ context.Context, owner, language, expectedBookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	key := fixtureGoalKey(owner, language)
	goal, ok := s.primaryGoals[key]
	if !ok {
		return persistence.ErrNotFound
	}
	if goal.BookID != expectedBookID {
		return persistence.ErrGoalStale
	}
	delete(s.primaryGoals, key)
	return nil
}

// RecordReadingFinishedPrimaryGoal records the reading fact and clears the
// active Goal and Journey membership.
func (s *Store) RecordReadingFinishedPrimaryGoal(_ context.Context, owner, language, expectedBookID, expectedSnapshotID string) (persistence.ReadingFinishResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	key := fixtureGoalKey(owner, language)
	goal, ok := s.primaryGoals[key]
	if !ok {
		completion, completed := s.readingHistory[fixtureReadingHistoryKey(owner, language, expectedSnapshotID)]
		if !completed {
			return persistence.ReadingFinishResult{}, persistence.ErrNotFound
		}
		return persistence.ReadingFinishResult{Completion: completion}, nil
	}
	if goal.BookID != expectedBookID || goal.SnapshotID != expectedSnapshotID {
		return persistence.ReadingFinishResult{}, persistence.ErrGoalStale
	}
	now := time.Now()
	snapshot := s.goalSnapshotVocabularyLocked(owner, language)
	snapshotCount := len(snapshot)
	eligibleCount := 0
	graduatedCount := 0
	for _, item := range snapshot {
		if fixtureKnown(s.known, item.Language, item.CanonicalLemma, item.UPOS) {
			continue
		}
		eligibleCount++
		graduatedCount++
		s.known = append(s.known, domain.KnownVocabulary{
			OwnerID: owner, Language: language, CanonicalLemma: item.CanonicalLemma,
			UPOS: item.UPOS, Provenance: "Accepted on Primary Goal completion", CreatedAt: now,
		})
	}
	completion := domain.ReadingCompletion{
		OwnerID: owner, Language: language, BookID: expectedBookID, CompletedAt: now,
		GoalSnapshotID: goal.SnapshotID, SnapshotVocabularyCount: snapshotCount,
		EligibleVocabularyCount: eligibleCount, GraduatedVocabularyCount: graduatedCount,
		AlreadyKnownVocabularyCount: snapshotCount - eligibleCount,
	}
	if existing, exists := s.readingHistory[fixtureReadingHistoryKey(owner, language, expectedSnapshotID)]; exists {
		completion = existing
	} else {
		s.readingHistory[fixtureReadingHistoryKey(owner, language, expectedSnapshotID)] = completion
	}
	for index := range s.preps {
		preparation := &s.preps[index]
		if preparation.OwnerID != owner || preparation.GraduatedAt != nil || preparation.StudyingAt == nil {
			continue
		}
		matchesSnapshot := preparation.GoalSnapshotID != "" && preparation.GoalSnapshotID == goal.SnapshotID
		matchesLegacyIdentity := preparation.SourceMaterialID == goal.SourceMaterialID && preparation.AnalysisRunID == goal.AnalysisRunID
		if !matchesSnapshot && !matchesLegacyIdentity {
			continue
		}
		preparation.StudyingAt = nil
		preparation.ReleasedAt = &now
		preparation.UpdatedAt = now
	}
	journeyKey := fixtureJourneyKey(owner, language)
	journey, journeyExists := s.readingJourneys[journeyKey]
	if journeyExists {
		for index, entry := range journey.Entries {
			if entry.BookID != expectedBookID {
				continue
			}
			journey.Entries = append(journey.Entries[:index], journey.Entries[index+1:]...)
			for position := range journey.Entries {
				journey.Entries[position].Position = position + 1
			}
			journey.Revision++
			journey.UpdatedAt = now
			if len(journey.Entries) == 0 && !s.fixtureLanguageDerived(owner, language) {
				delete(s.readingJourneys, journeyKey)
			} else {
				s.readingJourneys[journeyKey] = journey
			}
			break
		}
	}
	delete(s.primaryGoals, key)
	return persistence.ReadingFinishResult{Completion: completion}, nil
}

func (s *Store) fixtureBookExists(owner, bookID string) bool {
	return s.fixtureBookID(owner, bookID) != ""
}

func (s *Store) fixtureBookHasChosenLanguage(owner, bookID string) bool {
	bookID = s.fixtureBookID(owner, bookID)
	for _, source := range s.books {
		resolved := source.BookID
		if resolved == "" {
			resolved = source.Source.ID
		}
		if source.Source.OwnerID == owner && resolved == bookID {
			return strings.TrimSpace(source.Source.Language) != ""
		}
	}
	for _, book := range s.myBooks {
		if book.Book.OwnerID == owner && book.Book.ID == bookID {
			return book.Book.LanguageState == domain.LanguageChosen && strings.TrimSpace(book.Book.LanguageTag) != ""
		}
	}
	return false
}

func (s *Store) fixtureBookLanguage(owner, bookID string) string {
	bookID = s.fixtureBookID(owner, bookID)
	for _, source := range s.books {
		resolved := source.BookID
		if resolved == "" {
			resolved = source.Source.ID
		}
		if source.Source.OwnerID == owner && resolved == bookID {
			return normalizeFixtureLanguage(source.Source.Language)
		}
	}
	for _, book := range s.myBooks {
		if book.Book.OwnerID == owner && book.Book.ID == bookID && book.Book.LanguageState == domain.LanguageChosen {
			return normalizeFixtureLanguage(book.Book.LanguageTag)
		}
	}
	return ""
}

func (s *Store) fixtureLanguageDerived(owner, language string) bool {
	for _, book := range s.books {
		if book.Source.OwnerID == owner && normalizeFixtureLanguage(book.Source.Language) == language {
			return true
		}
	}
	for _, book := range s.myBooks {
		if book.Book.OwnerID == owner && book.Book.LanguageState == domain.LanguageChosen && normalizeFixtureLanguage(book.Book.LanguageTag) == language {
			return true
		}
	}
	return false
}

func (s *Store) admitFixtureCatalogueLanguage(owner, connectionID string) {
	if connectionID != "fixture-browser-sync-connection" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, alias := range s.aliases {
		if alias.OwnerID != owner || alias.ConnectionID != connectionID {
			continue
		}
		for i := range s.myBooks {
			if s.myBooks[i].Book.OwnerID == owner && s.myBooks[i].Book.ID == alias.BookID && s.myBooks[i].Book.LanguageState == domain.LanguageUnknown {
				s.myBooks[i].Book.LanguageState = domain.LanguageChosen
				s.myBooks[i].Book.LanguageTag = fixtureCatalogueLanguage
			}
		}
	}
}

func (s *Store) fixtureBookID(owner, id string) string {
	for _, source := range s.books {
		if source.Source.OwnerID == owner && (source.Source.ID == id || source.BookID == id) {
			if source.BookID != "" {
				return source.BookID
			}
			return source.Source.ID
		}
	}
	for _, book := range s.myBooks {
		if book.Book.OwnerID == owner && book.Book.ID == id {
			return book.Book.ID
		}
	}
	return ""
}

func fixtureJobs() []domain.AnalysisJob {
	jobs := []domain.AnalysisJob{{ID: 42, DisplayNumber: 1, OwnerID: OwnerID, SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, CorpusID: "fixture-corpus", AnalysisState: "completed", Progress: 100}, {ID: 43, DisplayNumber: 2, OwnerID: OwnerID, SourceMaterialID: "fixture-failed", AnalysisState: "failed", Error: "The analyzer stopped after the normalized corpus could not be read.\nRetry the analysis when you are ready.", Progress: 42}}
	for i := int64(3); i <= 18; i++ {
		jobs = append(jobs, domain.AnalysisJob{ID: 40 + i, DisplayNumber: i, OwnerID: OwnerID, SourceMaterialID: SourceID, AnalysisRunID: "fixture-history-" + strconv.FormatInt(i, 10), CorpusID: "fixture-corpus", AnalysisState: "completed", Progress: 100})
	}
	return jobs
}

func fixtureKnown(known []domain.KnownVocabulary, language, lemma, upos string) bool {
	for _, item := range known {
		if item.Language == language && item.CanonicalLemma == lemma && (item.UPOS == upos || item.UPOS == "") {
			return true
		}
	}
	return false
}

type AuthStore struct {
	mu       sync.Mutex
	user     domain.User
	hash     string
	sessions map[string]domain.User
}

func NewAuthStore() *AuthStore {
	h, err := auth.HashPassword(Password)
	if err != nil {
		panic("fixture password hash failed: " + err.Error())
	}
	return &AuthStore{user: domain.User{ID: OwnerID, Username: Username}, hash: h, sessions: map[string]domain.User{}}
}
func (s *AuthStore) HasUsers(context.Context) (bool, error) { return true, nil }
func (s *AuthStore) CreateFirstUserAndSession(context.Context, string, string, string, time.Time) (domain.User, bool, error) {
	return s.user, false, nil
}
func (s *AuthStore) GetUserByUsername(_ context.Context, u string) (domain.User, string, error) {
	if u == Username {
		return s.user, s.hash, nil
	}
	return domain.User{}, "", errNotFound
}
func (s *AuthStore) CreateSession(_ context.Context, id, token string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = s.user
	return nil
}
func (s *AuthStore) GetSession(_ context.Context, token string) (domain.User, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.sessions[token]
	if !ok {
		return domain.User{}, time.Time{}, errNotFound
	}
	return u, time.Now().Add(time.Hour), nil
}
func (s *AuthStore) DeleteSession(context.Context, string) error      { return nil }
func (s *AuthStore) DeleteUserSessions(context.Context, string) error { return nil }

type Analysis struct{}

func (Analysis) SubmitAnalysis(context.Context, string, string) (analysis.Handle, error) {
	return analysis.Handle{ID: 42, DisplayNumber: 1, RunID: ResultRunID}, nil
}
func (Analysis) Get(_ context.Context, _ string, id int64) (analysis.Status, error) {
	if id == 43 {
		return analysis.Status{ID: 43, DisplayNumber: 2, State: rivertype.JobStateDiscarded, SourceMaterialID: "fixture-failed", Error: "The analyzer stopped after the normalized corpus could not be read.\nRetry the analysis when you are ready.", LogicalState: "failed", Progress: 42}, nil
	}
	return analysis.Status{ID: 42, DisplayNumber: 1, State: rivertype.JobStateCompleted, Progress: 100, SourceMaterialID: SourceID, CorpusID: "fixture-corpus", RunID: ResultRunID, LogicalState: "completed"}, nil
}
func (Analysis) Retry(context.Context, string, int64) (analysis.Handle, error) {
	return analysis.Handle{ID: 43, DisplayNumber: 2}, nil
}
func (Analysis) GetCompletedAnalysis(_ context.Context, _ string, sourceMaterialID, runID string) (analysis.CompletedAnalysis, error) {
	result := analysis.CompletedAnalysis{RunID: ResultRunID, OwnerID: OwnerID, SourceMaterialID: SourceID, SnapshotID: "fixture-snapshot", JobID: 42, DisplayNumber: 1, Source: domain.SourceMaterial{ID: SourceID, OwnerID: OwnerID, Language: "de", Title: "Der lange Weg nach Hause"}, Corpus: domain.Corpus{ID: "fixture-corpus", OwnerID: OwnerID, SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: 123456, DistinctLemmaCount: 45678, TextProfile: &domain.TextProfile{SentenceCount: 2048, NormalizedTokenCount: 130000, EmptySentenceCount: 3, MedianSentenceTokenCount: 12.5, P90SentenceTokenCount: 38, LongSentenceCount: 117}}}}
	if sourceMaterialID == routeMatchBookID && runID == "fixture-route-match-run" {
		result.RunID = runID
		result.SourceMaterialID = sourceMaterialID
		result.Source = domain.SourceMaterial{ID: sourceMaterialID, OwnerID: OwnerID, Language: "de", Title: "Route match: familiar German"}
		result.Corpus = domain.Corpus{ID: "fixture-route-match-corpus", OwnerID: OwnerID, SourceMaterialID: sourceMaterialID, AnalysisRunID: runID, Statistics: result.Corpus.Statistics}
	}
	return result, nil
}

type Insights struct {
	JourneyStore *Store
}

func (insights Insights) JourneyForecast(ctx context.Context, owner, language string) (domain.JourneyForecast, error) {
	if insights.JourneyStore == nil {
		return domain.JourneyForecast{}, errors.New("fixture Journey store is unavailable")
	}
	return analysisinsights.NewService(insights.JourneyStore).JourneyForecast(ctx, owner, language)
}

func (Insights) Coverage(context.Context, string, string) (domain.AnalysisCoverage, error) {
	lemmas := make([]domain.LemmaOccurrence, 0, 18)
	for i := 1; i <= 18; i++ {
		lemmas = append(lemmas, domain.LemmaOccurrence{Language: "de", CanonicalLemma: fmt.Sprintf("Randlemma-%02d", i), UPOS: "NOUN", OccurrenceCount: int64(100 - i)})
	}
	projections := make([]domain.CoverageProjection, 0, 8)
	for i := int64(1); i <= 8; i++ {
		projections = append(projections, domain.CoverageProjection{TopLemmaCount: i * 3, SelectedLemmaCount: i * 3, OccurrenceCount: i * 2400, EligibleTokenCount: 80000, ProjectedTokenCount: 50000 + i*7000})
	}
	thresholds := []domain.CoverageThreshold{{TargetPercent: 90, LemmaCount: 120, OccurrenceCount: 90000, EligibleTokenCount: 100000, Reachable: true}, {TargetPercent: 95, LemmaCount: 240, OccurrenceCount: 95000, EligibleTokenCount: 100000, Reachable: true}, {TargetPercent: 97, LemmaCount: 390, OccurrenceCount: 97000, EligibleTokenCount: 100000, Reachable: true}, {TargetPercent: 99, LemmaCount: 999, OccurrenceCount: 0, EligibleTokenCount: 100000, Reachable: false}}
	return domain.AnalysisCoverage{SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, AnalyzableTokenCount: 123456, DistinctLemmaCount: 45678, KnownTokenCount: 45678, KnownLemmaCount: 12000, ReservedTokenCount: 12000, ReservedLemmaCount: 1500, UnknownTokenCount: 77778, UnknownLemmaCount: 33678, TopUnknownLemmas: lemmas, UnknownConcentration: domain.CoverageProjection{TopLemmaCount: 10, OccurrenceCount: 1000, EligibleTokenCount: 5000}, Projections: projections, Thresholds: thresholds, TextProfile: &domain.TextProfile{SentenceCount: 2048, NormalizedTokenCount: 130000, EmptySentenceCount: 3, MedianSentenceTokenCount: 12.5, P90SentenceTokenCount: 38, LongSentenceCount: 117}}, nil
}

type KnownVocab struct{}

func (KnownVocab) Submit(context.Context, string, string, string) (knownvocab.Handle, error) {
	return knownvocab.Handle{ID: 7}, nil
}
func (KnownVocab) Get(context.Context, string, int64) (knownvocab.Status, error) {
	return knownvocab.Status{ID: 7, State: rivertype.JobStateCompleted}, nil
}

type Enrichment struct{}

func (Enrichment) SubmitEnrichment(context.Context, string, []enrichment.Candidate) (enrichmentjob.Handle, error) {
	return enrichmentjob.Handle{ID: 8}, nil
}
func (Enrichment) Get(context.Context, string, int64) (enrichmentjob.Status, error) {
	return enrichmentjob.Status{ID: 8, State: rivertype.JobStateCompleted}, nil
}
func (Enrichment) Cancel(context.Context, string, int64) (enrichmentjob.Status, error) {
	return enrichmentjob.Status{ID: 8, State: rivertype.JobStateCancelled}, nil
}

type PreparedDeck struct{ Store *Store }

func (PreparedDeck) Submit(_ context.Context, owner, analysisID string, _ bool) (prepareddeck.Handle, error) {
	sourceMaterialID := SourceID
	if analysisID == "fixture-route-match-run" {
		sourceMaterialID = routeMatchBookID
	}
	return prepareddeck.Handle{Preparation: domain.DeckPreparation{ID: "fixture-submitted-" + analysisID, OwnerID: owner, SourceMaterialID: sourceMaterialID, AnalysisRunID: analysisID, State: domain.DeckPreparationQueued}, JobID: 9}, nil
}
func (p PreparedDeck) SubmitForGoal(_ context.Context, owner, analysisID, snapshotID string) (prepareddeck.Handle, error) {
	sourceMaterialID := SourceID
	if analysisID == "fixture-route-match-run" {
		sourceMaterialID = routeMatchBookID
	}
	preparation := domain.DeckPreparation{ID: "fixture-goal-preparation-" + snapshotID, OwnerID: owner, SourceMaterialID: sourceMaterialID, AnalysisRunID: analysisID, GoalSnapshotID: snapshotID, State: domain.DeckPreparationQueued}
	if p.Store != nil {
		p.Store.mu.Lock()
		defer p.Store.mu.Unlock()
		for _, existing := range p.Store.preps {
			if existing.OwnerID == owner && existing.GoalSnapshotID == snapshotID && existing.RetiredAt == nil {
				return prepareddeck.Handle{Preparation: existing, JobID: 9}, nil
			}
		}
		p.Store.preps = append(p.Store.preps, preparation)
	}
	return prepareddeck.Handle{Preparation: preparation, JobID: 9}, nil
}
func fixturePreparationFor(owner, id string) domain.DeckPreparation {
	preparation := domain.DeckPreparation{ID: id, OwnerID: owner, SourceMaterialID: SourceID, AnalysisRunID: ResultRunID, State: domain.DeckPreparationReady, Filename: "Fixture German deck.apkg", DeckName: "Mouseion::de::Fixture", TotalCards: 3}
	switch id {
	case JourneyPrepID:
		preparation.SourceMaterialID = "fixture-empty"
		preparation.AnalysisRunID = "fixture-empty-run"
		preparation.DeckName = "Mouseion::it::Journey"
	case OutsidePrepID:
		preparation.SourceMaterialID = "fixture-failed"
		preparation.AnalysisRunID = "fixture-failed-run"
		preparation.DeckName = "Mouseion::de::Outside Journey"
	}
	return preparation
}

func (p PreparedDeck) Get(_ context.Context, owner, id string) (domain.DeckPreparation, error) {
	if p.Store != nil {
		p.Store.mu.Lock()
		defer p.Store.mu.Unlock()
		for _, preparation := range p.Store.preps {
			if preparation.OwnerID == owner && preparation.ID == id {
				preparation.VocabularyCount = len(p.Store.deckVocabularyFor(owner, id))
				return preparation, nil
			}
		}
	}
	if analysisID, ok := strings.CutPrefix(id, "fixture-submitted-"); ok {
		sourceMaterialID := SourceID
		if analysisID == "fixture-route-match-run" {
			sourceMaterialID = routeMatchBookID
		}
		return domain.DeckPreparation{ID: id, OwnerID: owner, SourceMaterialID: sourceMaterialID, AnalysisRunID: analysisID, State: domain.DeckPreparationQueued}, nil
	}
	return fixturePreparationFor(owner, id), nil
}
func (p PreparedDeck) GetForAnalysis(ctx context.Context, owner, sourceMaterialID, analysisRunID string) (domain.DeckPreparation, error) {
	if p.Store != nil {
		return p.Store.GetDeckPreparationForAnalysis(ctx, owner, sourceMaterialID, analysisRunID)
	}
	return fixturePreparationFor(owner, PrepID), nil
}
func (p PreparedDeck) GetForGoalSnapshot(ctx context.Context, owner, snapshotID string) (domain.DeckPreparation, error) {
	if p.Store != nil {
		return p.Store.GetDeckPreparationForGoalSnapshot(ctx, owner, snapshotID)
	}
	return fixturePreparationFor(owner, PrepID), nil
}
func (PreparedDeck) Cancel(context.Context, string, string) (domain.DeckPreparation, error) {
	return domain.DeckPreparation{ID: PrepID, State: domain.DeckPreparationCancelled}, nil
}
func (PreparedDeck) Retry(context.Context, string, string, bool) (prepareddeck.Handle, error) {
	return prepareddeck.Handle{JobID: 9}, nil
}
func (PreparedDeck) Rerender(context.Context, string, string) (prepareddeck.Handle, error) {
	return prepareddeck.Handle{JobID: 10}, nil
}
func (p PreparedDeck) Download(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	preparation, err := p.Get(ctx, owner, id)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	preparation.State = domain.DeckPreparationReady
	preparation.Artifact = []byte("fixture")
	return preparation, nil
}

type Capabilities struct{}

func (Capabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}, {Language: "it", DisplayName: "Italian", Ready: true}}, Degraded: false}, nil
}

type OPDS struct{}

func (OPDS) AcquireForBook(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
	return epub.ImportResult{Source: domain.SourceMaterial{ID: "fixture-metadata-only", OwnerID: OwnerID, Language: "de", Title: "Metadata-only migration book"}}, nil
}
