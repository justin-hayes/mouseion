// Package domain contains the core corpus and vocabulary models.
package domain

import (
	"strings"
	"time"
)

type DeckPreparationState string

const DeckPreparationRequiresRepreparationError = "This deck's frozen render inputs are unavailable, so its presentation cannot be updated. Re-prepare the deck from the book's current analysis."

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
	case DeckPreparationReady:
		return false
	case DeckPreparationFailed, DeckPreparationCancelled:
		return next == DeckPreparationQueued
	default: // Invalid persisted states cannot transition.
		return false
	}
}

type DeckPreparation struct {
	ID, OwnerID, SourceMaterialID, AnalysisRunID, CurrentRunID, BookID, GoalSnapshotID, Filename, DeckName, ContentHash, Error string
	State                                                                                                                      DeckPreparationState
	Artifact                                                                                                                   []byte
	TotalCards, CardsWithEnglish, CardsWithContextualSentenceTranslations, CardsWithFallbackGloss                              int
	ContextualGlosses, ContextOnlyGlosses                                                                                      int
	ContextualGlossesReported                                                                                                  bool
	QualityOmissions                                                                                                           int
	RenderInputVersion, PresentationVersion, DeckRevision                                                                      int
	VocabularyCount                                                                                                            int
	EvidenceCoverage                                                                                                           []DeckPreparationEvidenceCoverage
	MeaningOmissions                                                                                                           []DeckPreparationMeaningOmission
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
	StudyingAt, ReviewedAt, GraduatedAt, ReleasedAt, RetiredAt                           *time.Time
}

type DeckPreparationMeaningOmission struct {
	TargetWord string
	Reason     string
}

type DeckPreparationEvidenceCoverage struct {
	Source     string
	Configured bool
	Selected   int
	Matched    int
	Candidates int
	Omitted    int
}

// PreparedDeckRerenderWork is the content-free identity needed to enqueue an
// automatic presentation update after a process restart.
type PreparedDeckRerenderWork struct {
	OwnerID, PreparationID, RunID string
	PresentationVersion           int
}

type DeckPreparationVocabulary struct {
	OwnerID, DeckPreparationID, Language, CanonicalLemma, UPOS string
	GeneratedAt                                                time.Time
	GraduatedAt                                                *time.Time
}

type User struct {
	ID, Username string
	CreatedAt    time.Time
}

// SupportedLanguage is server-wide reference data discovered from the analyzer.
type SupportedLanguage struct {
	Language, DisplayName string
	CreatedAt             time.Time
}

// StudyLanguage is derived from the learner's active Books rather than saved
// preferences.
type StudyLanguage struct {
	Language, DisplayName string
}

// VocabularyBrowseRow is one currently evidenced effective identity in a
// learner's selected study language.
type VocabularyBrowseRow struct {
	CanonicalLemma  string
	UPOS            string
	OccurrenceCount int64
	BookCount       int64
	Known           bool
	Reserved        bool
	Generated       bool
	Corrected       bool
	Selected        bool
}

type VocabularyBrowsePage struct {
	Rows                        []VocabularyBrowseRow
	Books                       []VocabularyBrowseBook
	Total                       int64
	InventoryTotal              int64
	ScopedInventoryTotal        int64
	AnalyzedBooks               int64
	ContributingBooks           int64
	NoncontributingBooks        int64
	BooksWithoutCurrentAnalysis int64
	SelectedBooks               []string
	SelectedUPOS                []string
	KnownFilter                 string
	ReservedFilter              string
	Sort                        string
	Page                        int
	SelectionCount              int
}

type VocabularyBrowseQuery struct {
	Prefix         string
	BookIDs        []string
	UPOS           []string
	KnownFilter    string
	ReservedFilter string
	Sort           string
	Page           int
}

// VocabularyIdentity is an effective lemma/POS identity retained by the
// learner's unnamed selection or a saved Custom deck.
type VocabularyIdentity struct {
	CanonicalLemma  string
	UPOS            string
	OccurrenceCount int64
	BookCount       int64
	MissingEvidence bool
	EvidenceBooks   []VocabularyIdentityBook
}

type VocabularyIdentityBook struct {
	ID, Title, AnalysisRunID string
	OccurrenceCount          int64
}

type CustomVocabularyDeck struct {
	ID, Language, Name string
	Identities         []VocabularyIdentity
	MissingCount       int64
	IdentityCount      int64
}

// CustomDeckPreparation is an immutable APKG generation for a saved Custom
// deck. The saved identity set may later change without changing this record.
type CustomDeckPreparation struct {
	ID, OwnerID, DeckID, Language, DeckName, Filename string
	State, Error                                      string
	FrozenSpec                                        []byte
	Artifact                                          []byte
	SelectedIdentities, TotalCards                    int
	CreatedAt                                         time.Time
	StartedAt, CompletedAt                            *time.Time
	Evidence                                          []CustomDeckPreparationEvidence
	Omissions                                         []CustomDeckPreparationOmission
	PreviousReadyID                                   string
	PreviousReadyCards                                int
	LatestReady                                       bool
}

type CustomDeckPreparationOmission struct {
	Lemma  string `json:"lemma"`
	UPOS   string `json:"upos"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type CustomDeckPreparationEvidence struct {
	BookID           string `json:"book_id"`
	BookTitle        string `json:"book_title"`
	SourceMaterialID string `json:"source_material_id"`
	AnalysisRunID    string `json:"analysis_run_id"`
	CorpusID         string `json:"corpus_id"`
	UnitID           string `json:"unit_id"`
	RawLemma         string `json:"raw_lemma"`
	AnalyzerLemma    string `json:"analyzer_lemma"`
	Lemma            string `json:"lemma"`
	UPOS             string `json:"upos"`
	Sentence         string `json:"sentence"`
	Target           string `json:"target"`
	SentenceOrdinal  int64  `json:"sentence_ordinal"`
	StartOffset      int64  `json:"start_offset"`
	EndOffset        int64  `json:"end_offset"`
}

type VocabularyBrowseBook struct {
	ID                    string `json:"id"`
	Title                 string `json:"title"`
	HasCurrentAnalysis    bool   `json:"has_current_analysis"`
	HasVocabularyEvidence bool   `json:"has_vocabulary_evidence"`
}

// ResolveActiveStudyLanguage applies the lazy defaulting rules for the
// learner's stored language pointer. Stored and recent values only win when
// they still belong to the derived study-language set.
func ResolveActiveStudyLanguage(languages []StudyLanguage, stored, mostRecent string) string {
	if len(languages) == 1 {
		return languages[0].Language
	}
	for _, language := range languages {
		if language.Language == stored {
			return stored
		}
	}
	for _, language := range languages {
		if language.Language == mostRecent {
			return mostRecent
		}
	}
	return ""
}

type SourceMaterial struct {
	ID, OwnerID, Language, SourceIdentifier, Title, MediaType, ContentHash, FullText string
	ContentRevisionID                                                                string
	ContentSnapshotID                                                                string
	ContentDigest                                                                    string
	ContentDigestVersion                                                             int
	Content                                                                          []byte
	CreatedAt                                                                        time.Time
}

// SourceMaterialSummary adds the learner-facing state and current-analysis
// identity without loading the book's content. Analysis IDs are populated only
// from the owner/book-scoped current-analysis projection.
type SourceMaterialSummary struct {
	Source SourceMaterial
	// BookTitle is the canonical catalogue title when this source is projected
	// onto its learner-facing Book identity.
	BookTitle        string
	BookAuthor       string
	BookID           string
	AnalysisStatus   string
	AnalysisState    string
	AnalysisRunID    string
	CorpusID         string
	AnalysisJobID    int64
	IsToRead         bool
	IsCurrentReading bool
}

// EvidenceState classifies the raw acquisition and analysis signals for a
// source summary without relying on a persisted projection.
func (s SourceMaterialSummary) EvidenceState() BookEvidenceState {
	if s.Source.ID == "" {
		return BookNotAcquired
	}
	if s.Source.ContentRevisionID == "" || s.Source.ContentSnapshotID == "" {
		return BookUnavailable
	}
	switch strings.ToLower(strings.TrimSpace(s.AnalysisStatus)) {
	case "stale":
		return BookStale
	case "analyzed":
		return BookAnalyzed
	default:
		return BookAcquiredUnassessed
	}
}

type OpdsConnection struct {
	ID, OwnerID, Name, URL, Username, Password string
	CreatedAt, UpdatedAt                       time.Time
}
type Corpus struct {
	ID, OwnerID, SourceMaterialID, ArtifactHash, AnalysisRunID, Status string
	Statistics                                                         *AnalysisStatistics
	CreatedAt                                                          time.Time
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
	Statistics                                *AnalysisStatistics
	Lemmas                                    []LemmaOccurrence
}

type LemmaOccurrence struct {
	Language, CanonicalLemma, UPOS string
	OccurrenceCount                int64
}

// LemmaReviewOccurrence is one content-word token in the current exact Book
// analysis. Analyzer attribution is retained alongside any learner correction.
type LemmaReviewOccurrence struct {
	OwnerID, BookID, CorpusID, AnalysisRunID string
	SourceDocumentID                         string
	StartOffset, EndOffset                   int64
	SentenceOrdinal, TokenOrdinal            int64
	Surface, RawLemma, CanonicalLemma, UPOS  string
	SentenceText, CorrectedLemma             string
	Excluded                                 bool
	ReviewFlagReason, ReviewFlagResolution   string
	ReviewFlagProvenance                     map[string]any
}

type LemmaReviewDecision struct {
	Occurrence           LemmaReviewOccurrence
	CanonicalLemma       string
	Excluded             bool
	NormalizationProfile string
	NormalizationVersion string
}

// LemmaReviewFlag is a source-attributed, non-authoritative review signal for
// one occurrence in one exact analysis. Resolution is empty until the learner
// explicitly keeps, corrects, or excludes that occurrence.
type LemmaReviewFlag struct {
	Occurrence LemmaReviewOccurrence
	Reason     string
	Provenance map[string]any
	Resolution string
}

type OccurrenceLemmaCorrection struct {
	SourceDocumentID string
	StartOffset      int64
	EndOffset        int64
	CanonicalLemma   string
	Excluded         bool
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
	SourceMaterialID     string
	AnalysisRunID        string
	AnalyzableTokenCount int64
	DistinctLemmaCount   int64
	KnownTokenCount      int64
	KnownLemmaCount      int64
	ReservedTokenCount   int64
	ReservedLemmaCount   int64
	UnknownTokenCount    int64
	UnknownLemmaCount    int64
	TopUnknownLemmas     []LemmaOccurrence
	UnknownConcentration CoverageProjection
	Projections          []CoverageProjection
	Thresholds           []CoverageThreshold
	TextProfile          *TextProfile
}

// ConcordanceOccurrence is one exact-match token occurrence from a current
// analysis. Sentence and unit offsets are relative to their respective
// containers; book offsets and chapter identity are derived from unit metadata.
type ConcordanceOccurrence struct {
	Surface, CanonicalLemma, UPOS             string
	Dependency, HeadSurface                   string
	HeadOrdinal                               int64
	SentenceText                              string
	SentenceStartOffset, SentenceEndOffset    int64
	UnitStartOffset, UnitEndOffset            int64
	BookStartOffset, BookEndOffset            int64
	BookID, BookTitle                         string
	SourceMaterialID, AnalysisRunID, CorpusID string
	UnitID, ChapterTitle                      string
	UnitOrder, SentenceOrdinal, TokenOrdinal  int64
}

// ConcordanceLookup describes one exact learner-facing lookup. Analyzer mode
// names preserved analyzer evidence and is deliberately distinct from effective
// vocabulary identity.
type ConcordanceLookup struct {
	Mode, Term, UPOS string
	BookIDs          []string
	GrammarDirection string
	Relation         string
	Page             int
}

type ConcordanceResult struct {
	Occurrences []ConcordanceResultOccurrence
	Page        int
	HasPrevious bool
	HasNext     bool
}

type ConcordanceResultOccurrence struct {
	ConcordanceOccurrence
	RawLemma, EffectiveLemma string
	Corrected, Excluded      bool
}

type SentenceStudy struct {
	BookID, BookTitle, ChapterTitle, SentenceText string
	TargetSurface, GrammarDirection, Relation     string
	SentenceOrdinal, TargetOrdinal                int64
	Tokens                                        []SentenceStudyToken
}

type SentenceStudyToken struct {
	Surface, RawLemma, EffectiveLemma, UPOS, Dependency, HeadSurface string
	Ordinal, HeadOrdinal                                             int64
	Corrected, Excluded                                              bool
}

type AnalysisJob struct {
	ID, DisplayNumber                                       int64
	OwnerID, SourceMaterialID, ContentHash, CorpusID, Error string
	AnalysisRunID                                           string
	AnalysisState                                           string
	Progress                                                int
	CreatedAt, UpdatedAt                                    time.Time
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
	// Provenance is a presentation-only projection. Explicitly recorded
	// entries have no durable source column; reviewed-deck graduation is
	// available when the deck snapshot can be joined.
	Provenance string
	CreatedAt  time.Time
}
type GeneratedVocabulary struct {
	OwnerID, Language, CanonicalLemma, UPOS string
	FirstDeckID                             string
	FirstSourceMaterialID                   *string
	FirstGeneratedAt                        time.Time
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
