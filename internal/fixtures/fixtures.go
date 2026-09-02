// Package fixtures provides the deterministic, infrastructure-free data set
// used by the browser acceptance harness.
package fixtures

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
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
	ResultRunID            = "fixture-run"
	DeckID                 = "fixture-deck"
	CampaignID             = "fixture-campaign"
	QueuedCampaignID       = "fixture-queued-campaign"
	PrepID                 = "fixture-preparation"
	QueuedPrepID           = "fixture-queued-preparation"
	JourneyPrepID          = "fixture-journey-preparation"
	OutsidePrepID          = "fixture-outside-journey-preparation"
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

var errNotFound = persistence.ErrNotFound
var fixtureJourneyTime = time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)

type Store struct {
	mu              sync.Mutex
	books           []domain.SourceMaterialSummary
	jobs            []domain.AnalysisJob
	campaigns       []domain.LearningCampaign
	profiles        []domain.LanguageProfile
	connections     []domain.OpdsConnection
	preps           []domain.DeckPreparation
	known           []domain.KnownVocabulary
	campaignVocab   []domain.CampaignVocabulary
	legacyGenerated []domain.GeneratedVocabulary
	myBooks         []domain.MyBook
	readingJourneys map[string]domain.ReadingJourney
	primaryGoals    map[string]domain.PrimaryGoal
	reviewedScopes  []domain.EPUBReviewedScopeSnapshot
}

func NewStore() *Store {
	return &Store{
		books: []domain.SourceMaterialSummary{
			{Source: domain.SourceMaterial{ID: BookID, OwnerID: OwnerID, Language: "de", Title: "Der lange Weg nach Hause", MediaType: "application/epub+zip", SourceIdentifier: "fixture-de", FullText: "Haus. Ein kurzer deutscher Satz.\n\n" + "Ein sehr langer Beispielsatz mit vielen Wörtern für die Anzeige von realistischem Randinhalt im Browser."}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: ResultRunID, CorpusID: "fixture-corpus", AnalysisJobID: 42},
			{Source: domain.SourceMaterial{ID: "fixture-empty", OwnerID: OwnerID, Language: "it", Title: "Empty chapter", MediaType: "application/epub+zip"}, AnalysisStatus: "ready", AnalysisState: "scope confirmed"},
			{Source: domain.SourceMaterial{ID: "fixture-failed", OwnerID: OwnerID, Language: "de", Title: "Fehlgeschlagene Analyse", MediaType: "application/epub+zip"}, AnalysisStatus: "analysis failed", AnalysisState: "failed", AnalysisJobID: 43},
			{Source: domain.SourceMaterial{ID: routeMatchBookID, OwnerID: OwnerID, Language: "de", Title: "Route match: familiar German", MediaType: "application/epub+zip"}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "fixture-route-match-run", CorpusID: "fixture-route-match-corpus"},
			{Source: domain.SourceMaterial{ID: routeDiffersBookID, OwnerID: OwnerID, Language: "de", Title: "Route differs: new German", MediaType: "application/epub+zip"}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "fixture-route-differs-run", CorpusID: "fixture-route-differs-corpus"},
			{Source: domain.SourceMaterial{ID: routeTieABookID, OwnerID: OwnerID, Language: "de", Title: "Route tie A", MediaType: "application/epub+zip"}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "fixture-route-tie-a-run", CorpusID: "fixture-route-tie-a-corpus"},
			{Source: domain.SourceMaterial{ID: routeTieBBookID, OwnerID: OwnerID, Language: "de", Title: "Route tie B", MediaType: "application/epub+zip"}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "fixture-route-tie-b-run", CorpusID: "fixture-route-tie-b-corpus"},
			{Source: domain.SourceMaterial{ID: routeUnavailableBookID, OwnerID: OwnerID, Language: "de", Title: "Route evidence pending", MediaType: "application/epub+zip"}, AnalysisStatus: "ready", AnalysisState: "scope confirmed"},
			{Source: domain.SourceMaterial{ID: edgeBookID, OwnerID: OwnerID, Title: "Donaudampfschifffahrtsgesellschaftskapitänsmütze: Eine Geschichte der deutschen Wörter, langen Reisen und unerwarteten Begegnungen am Fluss", FullText: "La biblioteca conserva una storia italiana con molte parole e una descrizione volutamente assente."}, AnalysisStatus: "ready", AnalysisState: "scope confirmed"},
		},
		jobs:        fixtureJobs(),
		campaigns:   fixtureCampaigns(),
		profiles:    []domain.LanguageProfile{{ID: "fixture-profile-de", OwnerID: OwnerID, Language: "de", DisplayName: "German"}, {ID: "fixture-profile-it", OwnerID: OwnerID, Language: "it", DisplayName: "Italian"}},
		connections: []domain.OpdsConnection{{ID: "fixture-connection", OwnerID: OwnerID, Name: "Fixture catalog", URL: "https://fixture.invalid/opds"}},
		preps: []domain.DeckPreparation{
			{ID: PrepID, OwnerID: OwnerID, SourceMaterialID: BookID, AnalysisRunID: ResultRunID, State: domain.DeckPreparationReady, Filename: "Fixture German deck.apkg", DeckName: "Mouseion::de::Fixture", TotalCards: 3},
			{ID: QueuedPrepID, OwnerID: OwnerID, SourceMaterialID: BookID, AnalysisRunID: ResultRunID, State: domain.DeckPreparationReady, Filename: "Fixture German queued deck.apkg", DeckName: "Mouseion::de::Queued", TotalCards: 3},
		},
		known: []domain.KnownVocabulary{
			{ID: "fixture-known", OwnerID: OwnerID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", Provenance: "Explicitly recorded", CreatedAt: fixtureJourneyTime},
			{ID: "fixture-independent-known", OwnerID: OwnerID, Language: "de", CanonicalLemma: IndependentKnownLemma, UPOS: "NOUN", Provenance: "Explicitly recorded", CreatedAt: fixtureJourneyTime},
			{ID: "fixture-graduated-known", OwnerID: OwnerID, Language: "de", CanonicalLemma: GraduatedKnownLemma, UPOS: "VERB", Provenance: "Graduated from completed campaign", CreatedAt: fixtureJourneyTime.Add(2 * time.Hour)},
		},
		campaignVocab:   fixtureCampaignVocabulary(),
		legacyGenerated: []domain.GeneratedVocabulary{{OwnerID: OwnerID, Language: "de", CanonicalLemma: LegacyGeneratedLemma, UPOS: "ADJ", FirstDeckID: "fixture-legacy-generated-deck", FirstGeneratedAt: fixtureJourneyTime}},
		myBooks: []domain.MyBook{{
			Book:          domain.Book{ID: "fixture-metadata-only", OwnerID: OwnerID, Title: "Metadata-only migration book", MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown, CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
			EvidenceState: domain.MyBookNotAcquired,
		}},
		readingJourneys: map[string]domain.ReadingJourney{
			OwnerID: {
				OwnerID: OwnerID, Revision: 1, UpdatedAt: fixtureJourneyTime,
				Entries: []domain.ReadingJourneyEntry{
					{OwnerID: OwnerID, BookID: "fixture-empty", Position: 1, CreatedAt: fixtureJourneyTime},
					{OwnerID: OwnerID, BookID: edgeBookID, Position: 2, CreatedAt: fixtureJourneyTime.Add(time.Minute)},
					{OwnerID: OwnerID, BookID: routeMatchBookID, Position: 3, CreatedAt: fixtureJourneyTime.Add(2 * time.Minute)},
					{OwnerID: OwnerID, BookID: routeDiffersBookID, Position: 4, CreatedAt: fixtureJourneyTime.Add(3 * time.Minute)},
					{OwnerID: OwnerID, BookID: routeTieABookID, Position: 5, CreatedAt: fixtureJourneyTime.Add(4 * time.Minute)},
					{OwnerID: OwnerID, BookID: routeTieBBookID, Position: 6, CreatedAt: fixtureJourneyTime.Add(5 * time.Minute)},
					{OwnerID: OwnerID, BookID: routeUnavailableBookID, Position: 7, CreatedAt: fixtureJourneyTime.Add(6 * time.Minute)},
				},
			},
		},
		primaryGoals: map[string]domain.PrimaryGoal{
			OwnerID: {OwnerID: OwnerID, BookID: BookID, CreatedAt: fixtureJourneyTime, UpdatedAt: fixtureJourneyTime},
		},
	}
}
func (s *Store) PutSupportedLanguage(context.Context, string, string) (domain.SupportedLanguage, error) {
	return domain.SupportedLanguage{}, nil
}
func (s *Store) PutLanguageProfile(_ context.Context, o, l, n string) (domain.LanguageProfile, error) {
	p := domain.LanguageProfile{ID: "fixture-profile-" + l, OwnerID: o, Language: l, DisplayName: n}
	s.profiles = append(s.profiles, p)
	return p, nil
}
func (s *Store) ListLanguageProfiles(context.Context, string) ([]domain.LanguageProfile, error) {
	return append([]domain.LanguageProfile(nil), s.profiles...), nil
}
func (s *Store) DeleteLanguageProfile(context.Context, string, string) error { return nil }
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
func (s *Store) UpdateOpdsConnection(_ context.Context, _ string, c domain.OpdsConnection) (domain.OpdsConnection, error) {
	return c, nil
}
func (s *Store) DeleteOpdsConnection(context.Context, string, string) error { return nil }
func (s *Store) ListSourceMaterials(context.Context, string) ([]domain.SourceMaterialSummary, error) {
	return append([]domain.SourceMaterialSummary(nil), s.books...), nil
}
func (s *Store) ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.MyBook, 0, len(s.books)+len(s.myBooks))
	for _, source := range s.books {
		state := domain.MyBookAcquiredUnassessed
		if source.AnalysisStatus == "analyzed" {
			state = domain.MyBookAnalyzed
		}
		out = append(out, domain.MyBook{Book: domain.Book{ID: source.Source.ID, OwnerID: source.Source.OwnerID, Title: source.Source.Title, LanguageState: domain.LanguageChosen, LanguageTag: source.Source.Language}, Acquired: &source, EvidenceState: state})
	}
	out = append(out, s.myBooks...)
	return out, nil
}
func (s *Store) ListAnalysisJobs(context.Context, string) ([]domain.AnalysisJob, error) {
	return append([]domain.AnalysisJob(nil), s.jobs...), nil
}
func (s *Store) ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.KnownVocabulary(nil), s.known...), nil
}
func (s *Store) ListLearningCampaigns(context.Context, string) ([]domain.LearningCampaign, error) {
	return append([]domain.LearningCampaign(nil), s.campaigns...), nil
}

// ListActiveLearningCampaignVocabulary and ListLegacyGeneratedVocabulary keep
// the fixture's migration categories on the same optional read seams used by
// the production coverage service.
func (s *Store) ListActiveLearningCampaignVocabulary(_ context.Context, owner, language string) ([]domain.CampaignVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []domain.CampaignVocabulary
	for _, campaign := range s.campaigns {
		if campaign.OwnerID != owner || campaign.Status != domain.CampaignActive {
			continue
		}
		for _, item := range s.campaignVocab {
			if item.CampaignID == campaign.ID && item.Language == language && item.GraduatedAt == nil && !fixtureKnown(s.known, item) {
				result = append(result, item)
			}
		}
	}
	return result, nil
}

func (s *Store) CountCampaignVocabularyToGraduate(_ context.Context, owner, campaignID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, item := range s.campaignVocab {
		if item.OwnerID == owner && item.CampaignID == campaignID && item.GraduatedAt == nil && !fixtureKnown(s.known, item) {
			count++
		}
	}
	return count, nil
}

func (s *Store) ListLegacyGeneratedVocabulary(_ context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
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
func (s *Store) GetLearningCampaign(_ context.Context, o, id string) (domain.LearningCampaign, error) {
	for _, c := range s.campaigns {
		if c.ID == id && c.OwnerID == o {
			return c, nil
		}
	}
	return domain.LearningCampaign{}, errNotFound
}
func (s *Store) CreateLearningCampaign(_ context.Context, o, b, d string) (domain.LearningCampaign, error) {
	c := domain.LearningCampaign{ID: "fixture-new-campaign", OwnerID: o, SourceMaterialID: b, DeckPreparationID: d, BookProgress: domain.BookQueued, DeckProgress: domain.DeckQueued, Status: domain.CampaignQueued}
	s.campaigns = append(s.campaigns, c)
	return c, nil
}
func (s *Store) UpdateLearningCampaignProgress(_ context.Context, o, id string, _ persistence.LearningCampaignExpectedState, b domain.BookProgress, d domain.DeckProgress) (domain.LearningCampaign, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c domain.LearningCampaign
	found := false
	for i := range s.campaigns {
		if s.campaigns[i].ID == id && s.campaigns[i].OwnerID == o {
			c = s.campaigns[i]
			c.BookProgress = b
			c.DeckProgress = d
			c.Status = domain.DeriveCampaignStatus(b, d)
			now := time.Now()
			if b == domain.BookFinished && c.BookFinishedAt == nil {
				c.BookFinishedAt = &now
			}
			if d == domain.DeckReviewed && c.DeckReviewedAt == nil {
				c.DeckReviewedAt = &now
			}
			if c.Status == domain.CampaignComplete && c.VocabularyGraduatedAt == nil {
				c.CompletedAt = &now
				c.VocabularyGraduatedAt = &now
				for i := range s.campaignVocab {
					if s.campaignVocab[i].CampaignID == c.ID && s.campaignVocab[i].GraduatedAt == nil {
						s.campaignVocab[i].GraduatedAt = &now
						if !fixtureKnown(s.known, s.campaignVocab[i]) {
							s.known = append(s.known, domain.KnownVocabulary{ID: "fixture-known-" + s.campaignVocab[i].CanonicalLemma, OwnerID: o, Language: s.campaignVocab[i].Language, CanonicalLemma: s.campaignVocab[i].CanonicalLemma, UPOS: s.campaignVocab[i].UPOS, Provenance: "Graduated from completed campaign", CreatedAt: now})
						}
					}
				}
			}
			s.campaigns[i] = c
			found = true
			break
		}
	}
	if !found {
		return c, errNotFound
	}
	return c, nil
}
func (s *Store) AbandonLearningCampaign(_ context.Context, o, id string, _ persistence.LearningCampaignExpectedState) (domain.LearningCampaign, error) {
	return s.UpdateLearningCampaignProgress(context.Background(), o, id, persistence.LearningCampaignExpectedState{}, domain.BookAbandoned, domain.DeckAbandoned)
}
func (s *Store) ListUnassignedReadyDeckPreparations(context.Context, string) ([]domain.DeckPreparation, error) {
	return nil, nil
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
func (s *Store) GetEPUBReviewedScope(_ context.Context, owner, sourceID, scopeID string) (domain.EPUBReviewedScopeSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, scope := range s.reviewedScopes {
		if scope.OwnerID == owner && scope.SourceMaterialID == sourceID && scope.ScopeID == scopeID {
			return scope, nil
		}
	}
	return domain.EPUBReviewedScopeSnapshot{}, errNotFound
}
func (s *Store) CreateEPUBReviewedScope(_ context.Context, scope domain.EPUBReviewedScopeSnapshot) (domain.EPUBReviewedScopeSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviewedScopes = append(s.reviewedScopes, scope)
	for i := range s.books {
		if s.books[i].Source.ID == scope.SourceMaterialID && s.books[i].Source.OwnerID == scope.OwnerID {
			s.books[i].ReviewedScopeID = scope.ScopeID
			return scope, nil
		}
	}
	return domain.EPUBReviewedScopeSnapshot{}, errNotFound
}

// My Books persistence is not part of the browser fixture yet; these methods
// keep the fixture's webapp.Store contract explicit until the later UI work.
func (s *Store) ListMyBooks(context.Context, string) ([]domain.Book, error) { return nil, nil }

func (s *Store) GetBook(_ context.Context, owner, bookID string) (domain.Book, error) {
	for _, book := range s.myBooks {
		if book.Book.OwnerID == owner && book.Book.ID == bookID {
			return book.Book, nil
		}
	}
	for _, source := range s.books {
		if source.Source.OwnerID == owner && source.Source.ID == bookID {
			return domain.Book{ID: source.Source.ID, OwnerID: owner, Title: source.Source.Title, LanguageState: domain.LanguageChosen, LanguageTag: source.Source.Language}, nil
		}
	}
	return domain.Book{}, errNotFound
}
func (s *Store) CreateBook(_ context.Context, book domain.Book) (domain.Book, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	book.ID = fmt.Sprintf("fixture-metadata-%d", len(s.myBooks)+1)
	s.myBooks = append(s.myBooks, domain.MyBook{Book: book, EvidenceState: domain.MyBookNotAcquired})
	return book, nil
}
func (s *Store) UpdateBookMetadata(context.Context, string, string, string, string, string) (domain.Book, error) {
	return domain.Book{}, errNotFound
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
func (s *Store) ResolveBookByAlias(context.Context, string, string, string) (domain.Book, bool, error) {
	return domain.Book{}, false, nil
}
func (s *Store) AddBookAlias(context.Context, string, string, string, string, string) error {
	return nil
}
func (s *Store) LinkSourceToBook(context.Context, string, string, string) error { return nil }
func (s *Store) ResolveOrCreateBookForAcquisition(context.Context, string, string, string, string) (string, error) {
	return "", errNotFound
}
func (s *Store) GetReadingJourney(_ context.Context, owner string) (domain.ReadingJourney, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	journey := s.readingJourneys[owner]
	journey.OwnerID = owner
	journey.Entries = append([]domain.ReadingJourneyEntry(nil), journey.Entries...)
	return journey, nil
}
func (s *Store) ResolveJourneyBookID(_ context.Context, _ string, id string) (string, bool, error) {
	// Fixtures use one unified id for source material and book, so the
	// canonical book identity is the id itself.
	return id, true, nil
}
func (s *Store) AddToReadingJourney(_ context.Context, owner, bookID string, expectedRevision int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	journey := s.readingJourneys[owner]
	if expectedRevision != journey.Revision {
		return 0, persistence.ErrJourneyStale
	}
	if !s.fixtureBookExists(owner, bookID) {
		return 0, errNotFound
	}
	for _, entry := range journey.Entries {
		if entry.BookID == bookID {
			return journey.Revision, nil
		}
	}
	journey.OwnerID = owner
	journey.Entries = append(journey.Entries, domain.ReadingJourneyEntry{OwnerID: owner, BookID: bookID, Position: len(journey.Entries) + 1, CreatedAt: time.Now()})
	journey.Revision++
	journey.UpdatedAt = time.Now()
	s.readingJourneys[owner] = journey
	return journey.Revision, nil
}
func (s *Store) RemoveFromReadingJourney(_ context.Context, _ string, _ string, expectedRevision int64) (int64, error) {
	return expectedRevision, nil
}

// MoveReadingJourneyEntry mirrors the Postgres store: when a Primary Goal book is
// a Journey member it is anchored and invisible to the provisional order, so
// newPosition is interpreted within the Goal-excluded order and the Goal entry
// is never moved.
func (s *Store) MoveReadingJourneyEntry(_ context.Context, owner, bookID string, newPosition int, expectedRevision int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	journey := s.readingJourneys[owner]
	if expectedRevision != journey.Revision {
		return 0, persistence.ErrJourneyStale
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
	goalBookID := s.primaryGoals[owner].BookID
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
			for index := 0; index < len(visible)+1; index++ {
				if index == goalIndex {
					journey.Entries = append(journey.Entries, domain.ReadingJourneyEntry{BookID: goalBookID})
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
	s.readingJourneys[owner] = journey
	return journey.Revision, nil
}
func (s *Store) GetPrimaryGoal(_ context.Context, owner string) (domain.PrimaryGoal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	goal, ok := s.primaryGoals[owner]
	if !ok {
		return domain.PrimaryGoal{}, nil
	}
	return goal, nil
}
func (s *Store) CreatePrimaryGoal(_ context.Context, owner, bookID string) (domain.PrimaryGoal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	goal := domain.PrimaryGoal{OwnerID: owner, BookID: bookID}
	if err := goal.Validate(); err != nil {
		return domain.PrimaryGoal{}, err
	}
	if !s.fixtureBookExists(owner, bookID) {
		return domain.PrimaryGoal{}, errNotFound
	}
	if _, ok := s.primaryGoals[owner]; ok {
		goal := s.primaryGoals[owner]
		if goal.ReadingFinishedAt == nil {
			return domain.PrimaryGoal{}, persistence.ErrGoalExists
		}
		goal.BookID = bookID
		goal.ReadingFinishedAt = nil
		goal.UpdatedAt = time.Now()
		s.primaryGoals[owner] = goal
		return goal, nil
	}
	now := time.Now()
	goal.CreatedAt, goal.UpdatedAt = now, now
	s.primaryGoals[owner] = goal
	return goal, nil
}
func (s *Store) ChangePrimaryGoal(_ context.Context, owner, bookID, expectedBookID string) (domain.PrimaryGoal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	goal, ok := s.primaryGoals[owner]
	if !ok {
		return domain.PrimaryGoal{}, persistence.ErrNotFound
	}
	if goal.BookID != expectedBookID {
		return domain.PrimaryGoal{}, persistence.ErrGoalStale
	}
	if !s.fixtureBookExists(owner, bookID) {
		return domain.PrimaryGoal{}, errNotFound
	}
	goal.BookID = bookID
	goal.UpdatedAt = time.Now()
	s.primaryGoals[owner] = goal
	return goal, nil
}
func (s *Store) ClearPrimaryGoal(_ context.Context, owner, expectedBookID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	goal, ok := s.primaryGoals[owner]
	if !ok {
		return persistence.ErrNotFound
	}
	if goal.BookID != expectedBookID {
		return persistence.ErrGoalStale
	}
	if goal.ReadingFinishedAt != nil {
		return persistence.ErrNotFound
	}
	delete(s.primaryGoals, owner)
	return nil
}

// FinishReadingPrimaryGoal mirrors the production guard and keeps the browser
// fixture useful for the finish receipt without pretending fixture data is a
// source of vocabulary knowledge.
func (s *Store) FinishReadingPrimaryGoal(_ context.Context, owner, expectedBookID string) (persistence.PrimaryGoalFinishResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	goal, ok := s.primaryGoals[owner]
	if !ok {
		return persistence.PrimaryGoalFinishResult{}, persistence.ErrNotFound
	}
	if goal.BookID != expectedBookID {
		return persistence.PrimaryGoalFinishResult{}, persistence.ErrGoalStale
	}
	result := persistence.PrimaryGoalFinishResult{Goal: goal}
	if goal.ReadingFinishedAt != nil {
		return result, nil
	}
	now := time.Now()
	goal.ReadingFinishedAt = &now
	goal.UpdatedAt = now
	s.primaryGoals[owner] = goal
	result.Goal = goal
	for i := range s.campaigns {
		campaign := &s.campaigns[i]
		if campaign.OwnerID != owner || campaign.SourceMaterialID != expectedBookID || campaign.Status != domain.CampaignActive {
			continue
		}
		campaign.BookProgress = domain.BookFinished
		campaign.Status = domain.DeriveCampaignStatus(campaign.BookProgress, campaign.DeckProgress)
		campaign.BookFinishedAt = &now
		campaign.UpdatedAt = now
		if campaign.Status == domain.CampaignComplete {
			campaign.CompletedAt = &now
			campaign.VocabularyGraduatedAt = &now
			for i := range s.campaignVocab {
				if s.campaignVocab[i].CampaignID == campaign.ID && s.campaignVocab[i].GraduatedAt == nil {
					s.campaignVocab[i].GraduatedAt = &now
					if !fixtureKnown(s.known, s.campaignVocab[i]) {
						result.Graduated = append(result.Graduated, s.campaignVocab[i])
						s.known = append(s.known, domain.KnownVocabulary{ID: "fixture-known-" + s.campaignVocab[i].CanonicalLemma, OwnerID: owner, Language: s.campaignVocab[i].Language, CanonicalLemma: s.campaignVocab[i].CanonicalLemma, UPOS: s.campaignVocab[i].UPOS, Provenance: "Graduated from completed campaign", CreatedAt: now})
					}
				}
			}
		}
		if campaign.Status == domain.CampaignActive {
			for _, vocabulary := range s.campaignVocab {
				if vocabulary.CampaignID == campaign.ID && vocabulary.GraduatedAt == nil && !fixtureKnown(s.known, vocabulary) {
					result.ResidualVocabularyCount++
				}
			}
		}
		result.Campaign = campaign
		break
	}
	return result, nil
}

func (s *Store) fixtureBookExists(owner, bookID string) bool {
	for _, source := range s.books {
		if source.Source.OwnerID == owner && source.Source.ID == bookID {
			return true
		}
	}
	for _, book := range s.myBooks {
		if book.Book.OwnerID == owner && book.Book.ID == bookID {
			return true
		}
	}
	return false
}

func fixtureJobs() []domain.AnalysisJob {
	jobs := []domain.AnalysisJob{{ID: 42, DisplayNumber: 1, OwnerID: OwnerID, SourceMaterialID: BookID, AnalysisRunID: ResultRunID, CorpusID: "fixture-corpus", AnalysisState: "completed", Progress: 100}, {ID: 43, DisplayNumber: 2, OwnerID: OwnerID, SourceMaterialID: "fixture-failed", AnalysisState: "failed", Error: "The analyzer stopped after the normalized corpus could not be read.\nReload the confirmed scope and retry this analysis.", Progress: 42}}
	for i := int64(3); i <= 18; i++ {
		jobs = append(jobs, domain.AnalysisJob{ID: 40 + i, DisplayNumber: i, OwnerID: OwnerID, SourceMaterialID: BookID, AnalysisRunID: "fixture-history-" + fmt.Sprint(i), CorpusID: "fixture-corpus", AnalysisState: "completed", Progress: 100})
	}
	return jobs
}

func fixtureCampaigns() []domain.LearningCampaign {
	completedAt := fixtureJourneyTime.Add(2 * time.Hour)
	abandonedAt := fixtureJourneyTime.Add(3 * time.Hour)
	campaigns := []domain.LearningCampaign{
		{ID: CampaignID, OwnerID: OwnerID, SourceMaterialID: BookID, DeckPreparationID: PrepID, BookProgress: domain.BookReading, DeckProgress: domain.DeckStudying, Status: domain.CampaignActive},
		{ID: QueuedCampaignID, OwnerID: OwnerID, SourceMaterialID: BookID, DeckPreparationID: QueuedPrepID, BookProgress: domain.BookQueued, DeckProgress: domain.DeckQueued, Status: domain.CampaignQueued},
		{ID: "fixture-completed-campaign", OwnerID: OwnerID, SourceMaterialID: BookID, DeckPreparationID: PrepID, BookProgress: domain.BookFinished, DeckProgress: domain.DeckReviewed, Status: domain.CampaignComplete, CompletedAt: &completedAt, VocabularyGraduatedAt: &completedAt},
		{ID: "fixture-abandoned-campaign", OwnerID: OwnerID, SourceMaterialID: BookID, DeckPreparationID: QueuedPrepID, BookProgress: domain.BookAbandoned, DeckProgress: domain.DeckAbandoned, Status: domain.CampaignAbandoned, AbandonedAt: &abandonedAt},
	}
	for i := 1; i <= 6; i++ {
		campaigns = append(campaigns, domain.LearningCampaign{ID: fmt.Sprintf("fixture-queued-campaign-%d", i), OwnerID: OwnerID, SourceMaterialID: BookID, DeckPreparationID: fmt.Sprintf("fixture-queued-preparation-%d", i), BookProgress: domain.BookQueued, DeckProgress: domain.DeckQueued, Status: domain.CampaignQueued})
	}
	return campaigns
}

func fixtureCampaignVocabulary() []domain.CampaignVocabulary {
	graduatedAt := fixtureJourneyTime.Add(2 * time.Hour)
	return []domain.CampaignVocabulary{
		{OwnerID: OwnerID, CampaignID: CampaignID, Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", GeneratedAt: fixtureJourneyTime},
		{OwnerID: OwnerID, CampaignID: CampaignID, Language: "de", CanonicalLemma: "Weg", UPOS: "NOUN", GeneratedAt: fixtureJourneyTime},
		{OwnerID: OwnerID, CampaignID: CampaignID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", GeneratedAt: fixtureJourneyTime},
		{OwnerID: OwnerID, CampaignID: "fixture-completed-campaign", Language: "de", CanonicalLemma: GraduatedKnownLemma, UPOS: "VERB", GeneratedAt: fixtureJourneyTime, GraduatedAt: &graduatedAt},
	}
}

func fixtureKnown(known []domain.KnownVocabulary, vocabulary domain.CampaignVocabulary) bool {
	for _, item := range known {
		if item.Language == vocabulary.Language && item.CanonicalLemma == vocabulary.CanonicalLemma && (item.UPOS == vocabulary.UPOS || item.UPOS == "") {
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
	h, _ := auth.HashPassword(Password)
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
func (Analysis) SubmitScopedAnalysis(context.Context, string, string, string) (analysis.Handle, error) {
	return analysis.Handle{ID: 42, DisplayNumber: 1, RunID: ResultRunID}, nil
}
func (Analysis) Get(_ context.Context, _ string, id int64) (analysis.Status, error) {
	if id == 43 {
		return analysis.Status{ID: 43, DisplayNumber: 2, State: rivertype.JobStateDiscarded, SourceMaterialID: "fixture-failed", Error: "The analyzer stopped after the normalized corpus could not be read.\nReload the confirmed scope and retry this analysis.", LogicalState: "failed", Progress: 42}, nil
	}
	return analysis.Status{ID: 42, DisplayNumber: 1, State: rivertype.JobStateCompleted, Progress: 100, SourceMaterialID: BookID, CorpusID: "fixture-corpus", RunID: ResultRunID, LogicalState: "completed"}, nil
}
func (Analysis) Retry(context.Context, string, int64) (analysis.Handle, error) {
	return analysis.Handle{ID: 43, DisplayNumber: 2}, nil
}
func (Analysis) GetCompletedAnalysis(context.Context, string, string, string) (analysis.CompletedAnalysis, error) {
	return analysis.CompletedAnalysis{RunID: ResultRunID, OwnerID: OwnerID, SourceMaterialID: BookID, ScopeID: "fixture-scope", SnapshotID: "fixture-snapshot", JobID: 42, DisplayNumber: 1, Source: domain.SourceMaterial{ID: BookID, OwnerID: OwnerID, Language: "de", Title: "Der lange Weg nach Hause"}, Corpus: domain.Corpus{ID: "fixture-corpus", OwnerID: OwnerID, SourceMaterialID: BookID, AnalysisRunID: ResultRunID, SelectedUnits: []domain.CorpusSelectedUnit{{UnitID: "fixture-001", Title: "Chapter one"}, {UnitID: "fixture-002", Title: "Chapter two"}}, Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: 123456, DistinctLemmaCount: 45678, TextProfile: &domain.TextProfile{SentenceCount: 2048, NormalizedTokenCount: 130000, EmptySentenceCount: 3, MedianSentenceTokenCount: 12.5, P90SentenceTokenCount: 38, LongSentenceCount: 117}}}}, nil
}

type Insights struct {
	JourneyStore *Store
}

// JourneyProjection returns a stable fixture projection with enough variation
// for the browser harness to exercise the advisory states without deriving
// evidence from a clock, database row order, or a persisted route.
func (insights Insights) JourneyProjection(ctx context.Context, owner, _ string) (domain.JourneyProjectionResult, error) {
	thresholds := []domain.CoverageThreshold{
		{TargetPercent: 95, LemmaCount: 2, Reachable: true},
		{TargetPercent: 97, LemmaCount: 4, Reachable: true},
		{TargetPercent: 99, LemmaCount: 7, Reachable: true},
	}
	coverage := func(known int64) *domain.AnalysisCoverage {
		return &domain.AnalysisCoverage{
			AnalyzableTokenCount: 100,
			KnownTokenCount:      known,
			Thresholds:           thresholds,
		}
	}
	conditional := func(bookID string, current, projected int64, position int) domain.JourneyRouteBook {
		return domain.JourneyRouteBook{
			BookID: bookID, SourceMaterialID: bookID, Language: "de", CorpusID: bookID + "-corpus",
			Position: position, Coverage: coverage(current), ConditionalCoverage: coverage(projected),
			Comparable: true,
		}
	}
	defaultLearnerOrder := []domain.JourneyRouteBook{
		{BookID: "fixture-empty", Position: 1, IncomparableReason: "different study language"},
		{BookID: edgeBookID, Position: 2, IncomparableReason: "unavailable: no current acquired source"},
		conditional(routeMatchBookID, 90, 90, 3),
		conditional(routeDiffersBookID, 20, 95, 4),
		conditional(routeTieABookID, 50, 50, 5),
		conditional(routeTieBBookID, 50, 50, 6),
		{BookID: routeUnavailableBookID, Position: 7, IncomparableReason: "unassessed: no current analyzed corpus"},
	}
	if insights.JourneyStore != nil {
		journey, err := insights.JourneyStore.GetReadingJourney(ctx, owner)
		if err != nil {
			return domain.JourneyProjectionResult{}, err
		}
		byID := make(map[string]domain.JourneyRouteBook, len(defaultLearnerOrder))
		for _, book := range defaultLearnerOrder {
			byID[book.BookID] = book
		}
		defaultLearnerOrder = defaultLearnerOrder[:0]
		for position, entry := range journey.Entries {
			book, ok := byID[entry.BookID]
			if !ok {
				book = domain.JourneyRouteBook{BookID: entry.BookID, IncomparableReason: "unavailable: no fixture evidence"}
			}
			book.Position = position + 1
			defaultLearnerOrder = append(defaultLearnerOrder, book)
		}
	}
	order := func(conditionalOrder bool) []domain.JourneyRouteBook {
		movable := make([]domain.JourneyRouteBook, 0, len(defaultLearnerOrder))
		for _, book := range defaultLearnerOrder {
			if book.Comparable {
				movable = append(movable, book)
			}
		}
		sort.SliceStable(movable, func(i, j int) bool {
			left, right := movable[i], movable[j]
			leftCoverage, rightCoverage := left.Coverage, right.Coverage
			if conditionalOrder {
				leftCoverage, rightCoverage = left.ConditionalCoverage, right.ConditionalCoverage
			}
			if leftCoverage.KnownTokenCount != rightCoverage.KnownTokenCount {
				return leftCoverage.KnownTokenCount > rightCoverage.KnownTokenCount
			}
			if left.Position != right.Position {
				return left.Position < right.Position
			}
			return left.BookID < right.BookID
		})
		result := make([]domain.JourneyRouteBook, len(defaultLearnerOrder))
		movableIndex, rank := 0, 0
		for position, book := range defaultLearnerOrder {
			if !book.Comparable {
				book.PlacementReason = book.IncomparableReason
				result[position] = book
				continue
			}
			book = movable[movableIndex]
			movableIndex++
			rank++
			bookRank := rank
			book.Rank = &bookRank
			if conditionalOrder {
				book.PlacementReason = "ranked by conditional projected coverage"
			} else {
				book.PlacementReason = "ranked by current known-token coverage"
			}
			result[position] = book
		}
		return result
	}
	advisoryOrder := order(false)
	conditionalOrder := order(true)
	comparableCount := 0
	for _, book := range defaultLearnerOrder {
		if book.Comparable {
			comparableCount++
		}
	}
	return domain.JourneyProjectionResult{
		OwnerID: owner, Language: "de", LearnerOrder: defaultLearnerOrder, AdvisoryOrder: advisoryOrder,
		ConditionalAdvisoryOrder: conditionalOrder, ComparableCount: comparableCount,
		IncomparableCount: len(defaultLearnerOrder) - comparableCount,
	}, nil
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
	return domain.AnalysisCoverage{SourceMaterialID: BookID, AnalysisRunID: ResultRunID, ReviewedScopeID: "fixture-scope", AnalyzableTokenCount: 123456, DistinctLemmaCount: 45678, KnownTokenCount: 45678, KnownLemmaCount: 12000, ActiveCampaignTokenCount: 12000, ActiveCampaignLemmaCount: 1500, UnknownTokenCount: 77778, UnknownLemmaCount: 33678, TopUnknownLemmas: lemmas, UnknownConcentration: domain.CoverageProjection{TopLemmaCount: 10, OccurrenceCount: 1000, EligibleTokenCount: 5000}, Projections: projections, Thresholds: thresholds, TextProfile: &domain.TextProfile{SentenceCount: 2048, NormalizedTokenCount: 130000, EmptySentenceCount: 3, MedianSentenceTokenCount: 12.5, P90SentenceTokenCount: 38, LongSentenceCount: 117}}, nil
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

type PreparedDeck struct{}

func (PreparedDeck) Submit(context.Context, string, string, bool) (prepareddeck.Handle, error) {
	return prepareddeck.Handle{Preparation: domain.DeckPreparation{ID: PrepID, OwnerID: OwnerID, SourceMaterialID: BookID, AnalysisRunID: ResultRunID, State: domain.DeckPreparationQueued}, JobID: 9}, nil
}
func fixturePreparationFor(owner, id string) domain.DeckPreparation {
	preparation := domain.DeckPreparation{ID: id, OwnerID: owner, SourceMaterialID: BookID, AnalysisRunID: ResultRunID, State: domain.DeckPreparationReady, Filename: "Fixture German deck.apkg", DeckName: "Mouseion::de::Fixture", TotalCards: 3}
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

func (PreparedDeck) Get(_ context.Context, owner, id string) (domain.DeckPreparation, error) {
	return fixturePreparationFor(owner, id), nil
}
func (PreparedDeck) Cancel(context.Context, string, string) (domain.DeckPreparation, error) {
	return domain.DeckPreparation{ID: PrepID, State: domain.DeckPreparationCancelled}, nil
}
func (PreparedDeck) Retry(context.Context, string, string, bool) (prepareddeck.Handle, error) {
	return prepareddeck.Handle{JobID: 9}, nil
}
func (PreparedDeck) Download(context.Context, string, string) (domain.DeckPreparation, error) {
	return domain.DeckPreparation{ID: PrepID, State: domain.DeckPreparationReady, Artifact: []byte("fixture")}, nil
}

type Capabilities struct{}

func (Capabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}, {Language: "it", DisplayName: "Italian", Ready: true}}, Degraded: false}, nil
}

type OPDS struct{}

func (OPDS) Browse(context.Context, string, string, string) (opds.Feed, error) { return feed(), nil }
func (OPDS) BrowsePage(context.Context, string, string, string) (opds.Feed, error) {
	return feed(), nil
}
func (OPDS) Languages(context.Context, string, string) (opds.Feed, error) { return feed(), nil }
func (OPDS) BrowseLanguage(context.Context, string, string, string) (opds.Feed, error) {
	return feed(), nil
}
func (OPDS) BrowseLanguagePage(context.Context, string, string, string, string) (opds.Feed, error) {
	return feed(), nil
}
func (OPDS) Search(context.Context, string, string, string) (opds.Feed, error) { return feed(), nil }
func (OPDS) SearchPage(context.Context, string, string, string, string) (opds.Feed, error) {
	return feed(), nil
}
func (OPDS) Acquire(context.Context, string, string, string, opds.Entry) (epub.ImportResult, error) {
	return epub.ImportResult{Source: domain.SourceMaterial{ID: "fixture-acquired", OwnerID: OwnerID, Language: "de", Title: "Erworbenes Buch"}}, nil
}
func feed() opds.Feed {
	return opds.Feed{Title: "Fixture catalog", Entries: []opds.Entry{{ID: "fixture-entry", Title: "Ein deutsches Buch — Donaudampfschifffahrtsgesellschaftskapitänsmütze und ein besonders langer OPDS-Untertitel", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://fixture.invalid/book.epub"}}}, {ID: "fixture-entry-it", Title: "Un libro italiano: una passeggiata luminosa tra le colline e le biblioteche"}}}
}
