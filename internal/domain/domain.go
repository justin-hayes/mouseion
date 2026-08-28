// Package domain contains the core corpus and vocabulary models.
package domain

import "time"

type DeckPreparationState string

const (
	DeckPreparationQueued    DeckPreparationState = "queued"
	DeckPreparationPreparing DeckPreparationState = "preparing"
	DeckPreparationReady     DeckPreparationState = "ready"
	DeckPreparationFailed    DeckPreparationState = "failed"
	DeckPreparationCancelled DeckPreparationState = "cancelled"
)

// CanTransitionTo describes the persisted deck-preparation state machine.
func (s DeckPreparationState) CanTransitionTo(next DeckPreparationState) bool {
	if s == next {
		return true
	}
	switch s {
	case DeckPreparationQueued:
		return next == DeckPreparationPreparing || next == DeckPreparationCancelled
	case DeckPreparationPreparing:
		return next == DeckPreparationReady || next == DeckPreparationFailed || next == DeckPreparationCancelled
	case DeckPreparationFailed, DeckPreparationCancelled:
		return next == DeckPreparationQueued
	default:
		return false
	}
}

type DeckPreparation struct {
	ID, OwnerID, SourceMaterialID, AnalysisRunID, CurrentRunID, Filename, DeckName, ContentHash, Error string
	State                                                                                              DeckPreparationState
	Artifact                                                                                           []byte
	TotalCards, CardsWithEnglish, CardsWithContextualSentenceTranslations                              int
	QualityOmissions                                                                                   int
	// The fields below are a derived, owner-scoped status projection. They are
	// deliberately not part of the public state machine; they describe the
	// durable run and Batch work behind the existing preparing state.
	Phase                                                                                string
	FailureClass                                                                         string
	BatchAge                                                                             time.Duration
	TranslationEligible, TranslationDone                                                 int
	TranslationPending, TranslationRunning                                               int
	TranslationFailed, TranslationCancelled                                              int
	TranslationRetrying                                                                  int
	BatchChunkCount, BatchSubmittedChunks                                                int
	BatchPollingChunks, BatchReconcilingChunks                                           int
	BatchCompletedChunks, BatchFailedChunks, BatchCancelledChunks                        int
	BatchRequestCount, BatchCompletedRequests, BatchFailedRequests, BatchExpiredRequests int
	BatchInputTokens, BatchOutputTokens                                                  int64
	CreatedAt, UpdatedAt                                                                 time.Time
	StartedAt, CompletedAt                                                               *time.Time
}

type User struct {
	ID, Username string
	CreatedAt    time.Time
}
type LanguageProfile struct {
	ID, OwnerID, Language, DisplayName string
	CreatedAt                          time.Time
}

// SupportedLanguage is server-wide reference data discovered from the analyzer.
// LanguageProfile remains an owner-scoped learner study-language selection.
type SupportedLanguage struct {
	Language, DisplayName string
	CreatedAt             time.Time
}
type SourceMaterial struct {
	ID, OwnerID, Language, SourceIdentifier, Title, MediaType, ContentHash, FullText string
	ContentRevisionID                                                                string
	ContentDigest                                                                    string
	ContentDigestVersion                                                             int
	Content                                                                          []byte
	CreatedAt                                                                        time.Time
}

// SourceMaterialSummary adds the learner-facing state and logical analysis
// identity derived from the latest analysis job and corpus without loading the
// book's content.
type SourceMaterialSummary struct {
	Source           SourceMaterial
	AnalysisStatus   string
	AnalysisState    string
	AnalysisRunID    string
	CorpusID         string
	ReviewedScopeID  string
	ConfirmedScopeID string
	AnalysisJobID    int64
}
type OpdsConnection struct {
	ID, OwnerID, Name, URL, Username, Password string
	CreatedAt, UpdatedAt                       time.Time
}
type Corpus struct {
	ID, OwnerID, SourceMaterialID, ArtifactHash, ReviewedScopeID, AnalysisRunID, Status string
	Statistics                                                                          *AnalysisStatistics
	SelectedUnits                                                                       []CorpusSelectedUnit
	CreatedAt                                                                           time.Time
}

// AnalysisStatistics records immutable counts from the analyzed corpus before
// owner vocabulary state is applied. Legacy corpora may not have statistics.
type AnalysisStatistics struct {
	AnalyzableTokenCount int64
	DistinctLemmaCount   int64
	TextProfile          *TextProfile
}

// TextProfile records explainable aggregate signals from the normalized
// sentence stream. Legacy corpora may not have a profile.
type TextProfile struct {
	SentenceCount            int64
	NormalizedTokenCount     int64
	EmptySentenceCount       int64
	MedianSentenceTokenCount float64
	P90SentenceTokenCount    int64
	LongSentenceCount        int64
}

// AnalysisCorpusVocabulary is the persisted, owner-scoped input for dynamic
// vocabulary coverage calculations.
type AnalysisCorpusVocabulary struct {
	CorpusID, SourceMaterialID, AnalysisRunID string
	ReviewedScopeID                           string
	SelectedUnits                             []CorpusSelectedUnit
	Statistics                                *AnalysisStatistics
	Lemmas                                    []LemmaOccurrence
}

type LemmaOccurrence struct {
	Language, CanonicalLemma, UPOS string
	OccurrenceCount                int64
}

type CoverageThreshold struct {
	TargetPercent      int
	LemmaCount         int64
	OccurrenceCount    int64
	EligibleTokenCount int64
	Reachable          bool
}

// CoverageProjection describes the effect of learning a frequency-ranked prefix
// of the deck-eligible unknown vocabulary.
type CoverageProjection struct {
	TopLemmaCount       int64
	SelectedLemmaCount  int64
	OccurrenceCount     int64
	EligibleTokenCount  int64
	ProjectedTokenCount int64
}

// AnalysisCoverage separates explicit mastery from projected study investment.
type AnalysisCoverage struct {
	SourceMaterialID         string
	ReviewedScopeID          string
	AnalysisRunID            string
	SelectedUnits            []CorpusSelectedUnit
	AnalyzableTokenCount     int64
	DistinctLemmaCount       int64
	KnownTokenCount          int64
	KnownLemmaCount          int64
	ActiveCampaignTokenCount int64
	ActiveCampaignLemmaCount int64
	UnknownTokenCount        int64
	UnknownLemmaCount        int64
	TopUnknownLemmas         []LemmaOccurrence
	UnknownConcentration     CoverageProjection
	Projections              []CoverageProjection
	Thresholds               []CoverageThreshold
	TextProfile              *TextProfile
}
type AnalysisJob struct {
	ID, DisplayNumber                                                        int64
	OwnerID, SourceMaterialID, ContentHash, CorpusID, ReviewedScopeID, Error string
	AnalysisRunID                                                            string
	AnalysisState                                                            string
	Progress                                                                 int
	CreatedAt, UpdatedAt                                                     time.Time
}
type CorpusSelectedUnit struct {
	UnitID, SourceHref, ResolvedHref, Title string
	Order                                   int
	StartOffset, EndOffset                  uint64
}
type NormalizedArtifact struct {
	ContentHash, Language, SchemaVersion, NormalizationProfile, NormalizationVersion, AnalyzerName, AnalyzerVersion string
	CreatedAt                                                                                                       time.Time
}
type SharedLemma struct {
	ContentHash, Language, CanonicalLemma, UPOS string
	Morphology                                  []byte
	Frequency                                   int64
}
type KnownVocabulary struct {
	ID, OwnerID, Language, CanonicalLemma, UPOS string
	CreatedAt                                   time.Time
}
type GeneratedVocabulary struct {
	OwnerID, Language, CanonicalLemma, UPOS string
	FirstDeckID                             string
	FirstSourceMaterialID                   *string
	FirstGeneratedAt                        time.Time
}

type BookProgress string

const (
	BookQueued    BookProgress = "queued"
	BookReading   BookProgress = "reading"
	BookFinished  BookProgress = "finished"
	BookAbandoned BookProgress = "abandoned"
)

func (s BookProgress) CanTransitionTo(next BookProgress) bool {
	if s == next {
		return true
	}
	switch s {
	case BookQueued:
		return next == BookReading || next == BookFinished || next == BookAbandoned
	case BookReading:
		return next == BookFinished || next == BookAbandoned
	default:
		return false
	}
}

type DeckProgress string

const (
	DeckQueued    DeckProgress = "queued"
	DeckStudying  DeckProgress = "studying"
	DeckReviewed  DeckProgress = "reviewed"
	DeckAbandoned DeckProgress = "abandoned"
)

func (s DeckProgress) CanTransitionTo(next DeckProgress) bool {
	if s == next {
		return true
	}
	switch s {
	case DeckQueued:
		return next == DeckStudying || next == DeckReviewed || next == DeckAbandoned
	case DeckStudying:
		return next == DeckReviewed || next == DeckAbandoned
	default:
		return false
	}
}

type CampaignStatus string

const (
	CampaignQueued    CampaignStatus = "queued"
	CampaignActive    CampaignStatus = "active"
	CampaignComplete  CampaignStatus = "complete"
	CampaignAbandoned CampaignStatus = "abandoned"
)

func DeriveCampaignStatus(book BookProgress, deck DeckProgress) CampaignStatus {
	if book == BookAbandoned || deck == DeckAbandoned {
		return CampaignAbandoned
	}
	if book == BookFinished && deck == DeckReviewed {
		return CampaignComplete
	}
	if book == BookQueued && deck == DeckQueued {
		return CampaignQueued
	}
	return CampaignActive
}

type LearningCampaign struct {
	ID, OwnerID, SourceMaterialID, DeckPreparationID string
	BookProgress                                     BookProgress
	DeckProgress                                     DeckProgress
	Status                                           CampaignStatus
	CreatedAt, UpdatedAt                             time.Time
	ActivatedAt, BookFinishedAt, DeckReviewedAt      *time.Time
	CompletedAt, AbandonedAt, VocabularyGraduatedAt  *time.Time
}

type CampaignVocabulary struct {
	OwnerID, CampaignID, Language, CanonicalLemma, UPOS string
	GeneratedAt                                         time.Time
	GraduatedAt                                         *time.Time
}
type VocabularyState struct {
	ID, OwnerID, Language, CanonicalLemma, UPOS, State string
	UpdatedAt                                          time.Time
}
type ExampleSentence struct {
	ID, OwnerID, CorpusID, SentenceKey, Text string
	Language, CanonicalLemma, UPOS           string
	SourceLocation, SelectionReasons         []byte
	SelectionRank, SelectionScore            int
	Chosen                                   bool
	CreatedAt                                time.Time
}
type CuratedSentence struct {
	ID, OwnerID, ExampleSentenceID, Language, CanonicalLemma, UPOS, Notes string
	CreatedAt                                                             time.Time
}
type Deck struct {
	ID, OwnerID, Language, Name string
	CreatedAt                   time.Time
}
type Card struct {
	ID, OwnerID, DeckID, DedupKey, CanonicalLemma, UPOS, Front, Back string
	CreatedAt                                                        time.Time
}
type ProcessingHistory struct {
	ID, OwnerID, CorpusID, Operation, Status string
	Details                                  []byte
	StartedAt                                time.Time
	CompletedAt                              *time.Time
}
type SelectionCandidate struct {
	OwnerID, CorpusID, Language, CanonicalLemma, UPOS string
	OccurrenceCount                                   int
	FirstEncounter                                    int64
	ObservedForms, SentenceReferences, Provenance     []byte
	SelectedAt                                        time.Time
}
