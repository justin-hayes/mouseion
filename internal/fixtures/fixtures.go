// Package fixtures provides the deterministic, infrastructure-free data set
// used by the browser acceptance harness.
package fixtures

import (
	"context"
	"errors"
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
	OwnerID     = "fixture-learner"
	Username    = "fixture-learner"
	Password    = "fixture-password"
	BookID      = "fixture-book"
	ResultRunID = "fixture-run"
	DeckID      = "fixture-deck"
	CampaignID  = "fixture-campaign"
	PrepID      = "fixture-preparation"
)

var errNotFound = errors.New("fixture: not found")

type Store struct {
	mu          sync.Mutex
	books       []domain.SourceMaterialSummary
	jobs        []domain.AnalysisJob
	campaigns   []domain.LearningCampaign
	profiles    []domain.LanguageProfile
	connections []domain.OpdsConnection
	preps       []domain.DeckPreparation
}

func NewStore() *Store {
	return &Store{
		books: []domain.SourceMaterialSummary{
			{Source: domain.SourceMaterial{ID: BookID, OwnerID: OwnerID, Language: "de", Title: "Der lange Weg nach Hause", MediaType: "application/epub+zip", SourceIdentifier: "fixture-de", FullText: "Haus. Ein kurzer deutscher Satz.\n\n" + "Ein sehr langer Beispielsatz mit vielen Wörtern für die Anzeige von realistischem Randinhalt im Browser."}, AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: ResultRunID, CorpusID: "fixture-corpus", AnalysisJobID: 42},
			{Source: domain.SourceMaterial{ID: "fixture-empty", OwnerID: OwnerID, Language: "it", Title: "Empty chapter", MediaType: "application/epub+zip"}, AnalysisStatus: "ready", AnalysisState: "scope confirmed"},
			{Source: domain.SourceMaterial{ID: "fixture-failed", OwnerID: OwnerID, Language: "de", Title: "Fehlgeschlagene Analyse", MediaType: "application/epub+zip"}, AnalysisStatus: "analysis failed", AnalysisState: "failed", AnalysisJobID: 43},
		},
		jobs:        []domain.AnalysisJob{{ID: 42, DisplayNumber: 1, OwnerID: OwnerID, SourceMaterialID: BookID, AnalysisRunID: ResultRunID, CorpusID: "fixture-corpus", AnalysisState: "completed"}, {ID: 43, DisplayNumber: 2, OwnerID: OwnerID, SourceMaterialID: "fixture-failed", AnalysisState: "failed", Error: "fixture analysis failed"}},
		campaigns:   []domain.LearningCampaign{{ID: CampaignID, OwnerID: OwnerID, SourceMaterialID: BookID, DeckPreparationID: PrepID, BookProgress: domain.BookReading, DeckProgress: domain.DeckStudying, Status: domain.CampaignActive}},
		profiles:    []domain.LanguageProfile{{ID: "fixture-profile-de", OwnerID: OwnerID, Language: "de", DisplayName: "German"}, {ID: "fixture-profile-it", OwnerID: OwnerID, Language: "it", DisplayName: "Italian"}},
		connections: []domain.OpdsConnection{{ID: "fixture-connection", OwnerID: OwnerID, Name: "Fixture catalog", URL: "https://fixture.invalid/opds"}},
		preps:       []domain.DeckPreparation{{ID: PrepID, OwnerID: OwnerID, SourceMaterialID: BookID, AnalysisRunID: ResultRunID, State: domain.DeckPreparationReady, Filename: "Fixture German deck.apkg", DeckName: "Mouseion::de::Fixture", TotalCards: 3}},
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
func (s *Store) ListAnalysisJobs(context.Context, string) ([]domain.AnalysisJob, error) {
	return append([]domain.AnalysisJob(nil), s.jobs...), nil
}
func (s *Store) ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error) {
	return []domain.KnownVocabulary{{ID: "fixture-known", OwnerID: OwnerID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}}, nil
}
func (s *Store) ListLearningCampaigns(context.Context, string) ([]domain.LearningCampaign, error) {
	return append([]domain.LearningCampaign(nil), s.campaigns...), nil
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
	c, e := s.GetLearningCampaign(context.Background(), o, id)
	if e != nil {
		return c, e
	}
	c.BookProgress = b
	c.DeckProgress = d
	c.Status = domain.DeriveCampaignStatus(b, d)
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
	return "fixture-snapshot", domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion}, nil
}
func (s *Store) GetEPUBUnitClassifications(context.Context, string, string, string, string) ([]domain.EPUBUnitClassification, error) {
	return nil, nil
}
func (s *Store) GetEPUBReviewedScope(context.Context, string, string, string) (domain.EPUBReviewedScopeSnapshot, error) {
	return domain.EPUBReviewedScopeSnapshot{}, errNotFound
}
func (s *Store) CreateEPUBReviewedScope(context.Context, domain.EPUBReviewedScopeSnapshot) (domain.EPUBReviewedScopeSnapshot, error) {
	return domain.EPUBReviewedScopeSnapshot{}, nil
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
func (Analysis) Get(context.Context, string, int64) (analysis.Status, error) {
	return analysis.Status{ID: 42, DisplayNumber: 1, State: rivertype.JobStateCompleted, Progress: 100, SourceMaterialID: BookID, CorpusID: "fixture-corpus", RunID: ResultRunID, LogicalState: "completed"}, nil
}
func (Analysis) GetCompletedAnalysis(context.Context, string, string, string) (analysis.CompletedAnalysis, error) {
	return analysis.CompletedAnalysis{RunID: ResultRunID, OwnerID: OwnerID, SourceMaterialID: BookID, ScopeID: "fixture-scope", JobID: 42, DisplayNumber: 1, Source: domain.SourceMaterial{ID: BookID, OwnerID: OwnerID, Language: "de", Title: "Der lange Weg nach Hause"}, Corpus: domain.Corpus{ID: "fixture-corpus", OwnerID: OwnerID, SourceMaterialID: BookID, AnalysisRunID: ResultRunID, Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: 12, DistinctLemmaCount: 8}}}, nil
}

type Insights struct{}

func (Insights) Coverage(context.Context, string, string) (domain.AnalysisCoverage, error) {
	return domain.AnalysisCoverage{SourceMaterialID: BookID, AnalysisRunID: ResultRunID, AnalyzableTokenCount: 12, DistinctLemmaCount: 8, KnownTokenCount: 4, UnknownTokenCount: 8, TopUnknownLemmas: []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 4}}}, nil
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
func (PreparedDeck) Get(context.Context, string, string) (domain.DeckPreparation, error) {
	return domain.DeckPreparation{ID: PrepID, OwnerID: OwnerID, SourceMaterialID: BookID, AnalysisRunID: ResultRunID, State: domain.DeckPreparationReady, Filename: "Fixture German deck.apkg", DeckName: "Mouseion::de::Fixture", TotalCards: 3}, nil
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
	return opds.Feed{Title: "Fixture catalog", Entries: []opds.Entry{{ID: "fixture-entry", Title: "Ein deutsches Buch", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://fixture.invalid/book.epub"}}}, {ID: "fixture-entry-it", Title: "Un libro italiano"}}}
}
