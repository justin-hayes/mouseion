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
	ID, OwnerID, SourceMaterialID, Filename, DeckName, ContentHash, Error string
	State                                                                 DeckPreparationState
	Artifact                                                              []byte
	TotalCards, CardsWithEnglish, CardsWithContextualSentenceTranslations int
	QualityOmissions                                                      int
	CreatedAt, UpdatedAt                                                  time.Time
	StartedAt, CompletedAt                                                *time.Time
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
	Content                                                                          []byte
	CreatedAt                                                                        time.Time
}

// SourceMaterialSummary adds the learner-facing state derived from the latest
// analysis job and corpus without loading the book's content.
type SourceMaterialSummary struct {
	Source         SourceMaterial
	AnalysisStatus string
	CorpusID       string
	AnalysisJobID  int64
}
type OpdsConnection struct {
	ID, OwnerID, Name, URL, Username, Password string
	CreatedAt, UpdatedAt                       time.Time
}
type Corpus struct {
	ID, OwnerID, SourceMaterialID, ArtifactHash, Status string
	Statistics                                          *AnalysisStatistics
	CreatedAt                                           time.Time
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
	CorpusID, SourceMaterialID string
	Statistics                 *AnalysisStatistics
	Lemmas                     []LemmaOccurrence
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
	AnalyzableTokenCount int64
	DistinctLemmaCount   int64
	KnownTokenCount      int64
	KnownLemmaCount      int64
	UnknownTokenCount    int64
	UnknownLemmaCount    int64
	TopUnknownLemmas     []LemmaOccurrence
	UnknownConcentration CoverageProjection
	Projections          []CoverageProjection
	Thresholds           []CoverageThreshold
	TextProfile          *TextProfile
}
type AnalysisJob struct {
	ID, DisplayNumber                                       int64
	OwnerID, SourceMaterialID, ContentHash, CorpusID, Error string
	Progress                                                int
	CreatedAt, UpdatedAt                                    time.Time
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
