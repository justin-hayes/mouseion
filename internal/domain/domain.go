// Package domain contains the core corpus and vocabulary models.
package domain

import (
	"slices"
	"strings"
	"time"
)

type DeckPreparationState string

const DeckPreparationRequiresRepreparationError = "This deck's frozen render inputs are unavailable, so its presentation cannot be updated. Re-prepare the deck from the book's current analysis."

// DeckPreparationProviderOmissionReason is the fixed, learner-facing reason for
// an accepted identity a ready deck had to leave out because the translation
// provider gave an unusable answer.
const DeckPreparationProviderOmissionReason = "the translation provider gave an unusable answer"

// DeckPreparationEvidenceAssistedPassAbstentionReason is the internal outcome
// reason an Ancient Greek evidence-assisted pass item records when the provider
// added nothing. The pass only adds to the source-evidence card, so it is never
// reported as a meaning or provider omission (ADR 0101).
const DeckPreparationEvidenceAssistedPassAbstentionReason = "the evidence-assisted provider pass added nothing"

// DeckPreparationLemmaMismatchLabel is the learner-facing label for a card the
// contextual-gloss provider omitted because the representative sentence uses a
// different lexeme than the target lemma.
const DeckPreparationLemmaMismatchLabel = "the sentence uses a different word than this lemma"

// DeckPreparationIdiomLiteralUseLabel is the learner-facing label for an idiom
// card the provider omitted because the sentence uses the words literally
// (ADR 0114). An idiom omission never raises a lemma review flag.
const DeckPreparationIdiomLiteralUseLabel = "the words are used literally here"

// DeckPreparationLemmaMismatchPrefix is the reserved, domain-owned prefix that
// distinguishes a lemma-mismatch omission from an ambiguous one in the existing
// free-text omission reason. It is deliberately independent of the learner
// label so rewording the label cannot reclassify already stored rows. No schema
// change is needed.
const DeckPreparationLemmaMismatchPrefix = "lemma_mismatch: "

// maxDeckPreparationOmissionReasonRunes matches the persisted omission-reason
// bound.
const maxDeckPreparationOmissionReasonRunes = 200

// DeckPreparationLemmaMismatchDetail bounds a provider's lemma-mismatch reason
// to the space available in the stored omission reason's detail, so evidence
// provenance and the stored omission carry the same bounded text.
func DeckPreparationLemmaMismatchDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	available := maxDeckPreparationOmissionReasonRunes - len([]rune(DeckPreparationLemmaMismatchPrefix))
	if runes := []rune(detail); len(runes) > available {
		detail = string(runes[:available])
	}
	return detail
}

// DeckPreparationLemmaMismatchOmissionReason builds the stored omission reason
// for a lemma-mismatch outcome: the reserved prefix plus the bounded provider
// reason, truncated so the persisted value stays within its existing bound.
func DeckPreparationLemmaMismatchOmissionReason(detail string) string {
	return DeckPreparationLemmaMismatchPrefix + DeckPreparationLemmaMismatchDetail(detail)
}

// IsDeckPreparationLemmaMismatchOmission reports whether a stored omission
// reason uses the reserved lemma-mismatch prefix.
func IsDeckPreparationLemmaMismatchOmission(reason string) bool {
	return strings.HasPrefix(reason, DeckPreparationLemmaMismatchPrefix)
}

const (
	DeckPreparationQueued    DeckPreparationState = "queued"
	DeckPreparationPreparing DeckPreparationState = "preparing"
	DeckPreparationReady     DeckPreparationState = "ready"
	DeckPreparationFailed    DeckPreparationState = "failed"
	DeckPreparationCancelled DeckPreparationState = "cancelled"
)

// Active reports whether the preparation is still in flight. The SQL
// predicates that attach a run or cancel a preparation accept exactly these
// states.
func (s DeckPreparationState) Active() bool {
	return s == DeckPreparationQueued || s == DeckPreparationPreparing
}

// RequeueableDeckPreparationStates is the set a settled attempt may requeue
// from. The SQL predicate that requeues a preparation accepts exactly these
// states.
func RequeueableDeckPreparationStates() []DeckPreparationState {
	return []DeckPreparationState{DeckPreparationFailed, DeckPreparationCancelled}
}

// Requeueable reports whether a settled attempt may start again.
func (s DeckPreparationState) Requeueable() bool {
	return slices.Contains(RequeueableDeckPreparationStates(), s)
}

type DeckPreparation struct {
	ID, OwnerID, SourceMaterialID, AnalysisRunID, CurrentRunID, BookID, SnapshotID, Filename, DeckName, ContentHash, Error string
	State                                                                                                                  DeckPreparationState
	Artifact                                                                                                               []byte
	TotalCards, CardsWithEnglish, CardsWithContextualSentenceTranslations, CardsWithFallbackGloss                          int
	// VocabularyCards and IdiomCards split TotalCards by frozen item kind. The
	// values are derived from the manifest, so no persisted column changes.
	VocabularyCards, IdiomCards int
	// VocabularyOmissions and IdiomOmissions split the provider's explicit
	// abstentions by frozen item kind. An idiom omission is never a vocabulary
	// omission (ADR 0114). Both are derived from the durable outcomes.
	VocabularyOmissions, IdiomOmissions int
	// IdiomOverflow is how many candidate idioms the per-deck bound left out
	// while freezing. It is derived from the manifest and is not an omission
	// (doc/features/idiom-cards.md).
	IdiomOverflow                         int
	ContextualGlosses, ContextOnlyGlosses int
	ContextualGlossesReported             bool
	QualityOmissions                      int
	// ProviderOmissions counts accepted identities the translation provider
	// could not answer usably. They were left out of the ready deck and can be
	// retried; unlike QualityOmissions they would otherwise have produced cards.
	ProviderOmissions                                     int
	RenderInputVersion, PresentationVersion, DeckRevision int
	VocabularyCount                                       int
	EvidenceCoverage                                      []DeckPreparationEvidenceCoverage
	// MeaningOmissions lists vocabulary items the provider left unresolved.
	// Idiom abstentions are listed separately so the two are never conflated.
	MeaningOmissions     []DeckPreparationMeaningOmission
	IdiomOmissionDetails []DeckPreparationIdiomOmission
	// LemmaMismatchOmissions counts the subset of MeaningOmissions the
	// contextual-gloss provider attributed to a different lexeme than the
	// target lemma. Ambiguous omissions are not included.
	LemmaMismatchOmissions int
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

// DeckPreparationIdiomOmission is an idiom the provider declined in context.
// It is deliberately distinct from a vocabulary meaning omission (ADR 0114).
type DeckPreparationIdiomOmission struct {
	Headword string
	Reason   string
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
	LexicalDiscriminator                                       string
	// Headword is the imported citation form shown to learners, empty for
	// modern-language vocabulary.
	Headword    string
	GeneratedAt time.Time
	GraduatedAt *time.Time
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
// learner's selected study language. LexicalDiscriminator keeps source
// homographs with the same canonical lemma and part of speech apart. Headword is
// the imported citation form shown to learners, empty for modern languages;
// BareLemmaWithNumberedSibling marks a bare identity whose work also numbers a
// form of the same lemma.
type VocabularyBrowseRow struct {
	CanonicalLemma               string
	UPOS                         string
	LexicalDiscriminator         string
	Headword                     string
	OccurrenceCount              int64
	AcrossBooksOccurrenceCount   int64
	BookCount                    int64
	Known                        bool
	Reserved                     bool
	Generated                    bool
	InBookDeck                   bool
	Corrected                    bool
	BareLemmaWithNumberedSibling bool
}

type VocabularyBrowsePage struct {
	Language                     string
	Rows                         []VocabularyBrowseRow
	Books                        []VocabularyBrowseBook
	CurrentBookID, ReadingBookID string
	Total                        int64
	InventoryTotal               int64
	ScopedInventoryTotal         int64
	AnalyzedBooks                int64
	ContributingBooks            int64
	NoncontributingBooks         int64
	BooksWithoutCurrentAnalysis  int64
	BrowseCountsUpdating         bool
	BrowseCountsUnavailable      bool
	SelectedBooks                []string
	SelectedUPOS                 []string
	KnownFilter                  string
	ReservedFilter               string
	Sort                         string
	IncludeAll                   bool
	CorpusRevision               string
	Page                         int
}

type VocabularyBrowseQuery struct {
	Language       string
	Prefix         string
	CurrentBookID  string
	ReadingBookID  string
	BookIDs        []string
	UPOS           []string
	KnownFilter    string
	ReservedFilter string
	Sort           string
	IncludeAll     bool
	Revision       string
	Page           int
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
	BookTitle  string
	BookAuthor string
	BookID     string
	// Signals is the typed analysis evidence, mapped from the source row by
	// persistence. Presentation reads it through EvidenceClassification.
	Signals          AnalysisSignals
	AnalysisRunID    string
	CorpusID         string
	AnalysisJobID    int64
	IsToRead         bool
	IsCurrentReading bool
	// AnalysisUpdate is set when the published analysis's Analysis pipeline
	// identity differs from the current one reported by the analysis service.
	AnalysisUpdate *AnalysisUpdateCandidate
	// SourceStructureUpdate is set when a successful extraction-only check for
	// this exact source evidence offers a learner-confirmed structure update
	// (ADR 0103). Nothing is adopted until the learner confirms it.
	SourceStructureUpdate *SourceStructureUpdateCandidate
}

// EvidenceClassification classifies this source's Analysis evidence. Eligibility
// is reported as if the Book were To Read with its source language chosen;
// callers that need the learner's actual eligibility use MyBook.Classification.
func (s SourceMaterialSummary) EvidenceClassification() BookEvidenceClassification {
	return s.EvidenceClassificationWithPreparation(nil)
}

// EvidenceClassificationWithPreparation classifies this source's Analysis
// evidence together with the Book's classical preparation evidence. A failure
// applies only while no analysis is in effect; the classifier decides that.
func (s SourceMaterialSummary) EvidenceClassificationWithPreparation(preparation *ClassicalPreparationStatus) BookEvidenceClassification {
	signals := s.Signals
	signals.CurrentReading = s.IsCurrentReading
	signals.PipelineOutdated = s.AnalysisUpdate != nil
	signals.ClassicalPreparation = preparation
	return ClassifyBookEvidence(signals, BookDispositionToRead, s.Source.Language)
}

// EvidenceState classifies the acquisition and analysis evidence of this source.
func (s SourceMaterialSummary) EvidenceState() BookEvidenceState {
	return s.EvidenceClassification().Evidence
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
	LexicalDiscriminator                     string
	Dependency                               string
	SentenceText, CorrectedLemma             string
	Excluded                                 bool
	ReviewFlagReason, ReviewFlagResolution   string
	ReviewFlagProvenance                     map[string]any
	// NormalizationProfile and NormalizationVersion are the profile the
	// occurrence's analysis recorded, so a correction is normalized by exactly
	// that version rather than by whatever profile is active now. Both are empty
	// when the store records no profile, and the active profile applies instead.
	NormalizationProfile, NormalizationVersion string
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
	Surface, CanonicalLemma, UPOS string
	// Headword is the imported source's learner-facing citation form, empty for
	// modern-language occurrences. A learner-facing summary shows it instead of
	// the stored canonical key.
	Headword                                  string
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

// Concordance lookup interpretations. A lemma lookup expands every observed
// form of an evidenced effective lemma; a form lookup matches only the typed
// observed source form.
const (
	ConcordanceMatchLemma = "lemma"
	ConcordanceMatchForm  = "form"
	// ConcordanceMatchConstruction lists other occurrences of one Ancient Greek
	// construction (ADR 0125). Its term is the construction id.
	ConcordanceMatchConstruction = "construction"
)

// ConcordanceConstructionLanguage is the one study language whose analysis has a
// construction scheme (ADR 0125). Any other language gets no parallels.
const ConcordanceConstructionLanguage = "grc"

// Concordance construction tiers, in the order a parallels page lists them.
const (
	ConcordanceTierThisBook     = "this_book"
	ConcordanceTierThisAuthor   = "this_author"
	ConcordanceTierOtherAuthors = "other_authors"
)

// ConcordanceConstructionOrigin is the Construction occurrence a parallels
// lookup came from: the origin sentence and anchor of one analysis run. It
// supplies the origin Book, Work and author, and is left out of the results.
type ConcordanceConstructionOrigin struct {
	RunID            string
	Sentence, Anchor int
}

// ConcordanceLookup describes one learner-facing lookup of a single literal
// lemma or word form across the eligible analyzed corpus.
type ConcordanceLookup struct {
	// Language is the study language the request was issued for. Supported
	// links and forms carry it so an old-language request is recognized.
	Language string
	// Term is the submitted term with surrounding whitespace trimmed.
	Term string
	// Match is the applied interpretation. It is empty for an independent
	// submission, which recognizes it against the corpus; supported page and
	// Browse links carry it so paging never reruns recognition.
	Match string
	// UPOS restricts a lemma lookup to one exact part of speech. Only a
	// supported Browse identity link supplies it.
	UPOS string
	// Priority is the Book captured when the lookup was applied: a Book ID, or
	// ConcordancePriorityNone for the explicit absence of a current reading.
	// Empty means no priority has been captured yet (a fresh lookup).
	Priority string
	Page     int
	Revision string
	// Subtype restricts a construction lookup to one shipped subtype label.
	Subtype string
	// Origin is the occurrence a construction lookup came from; it is required
	// for the construction match and unused otherwise.
	Origin *ConcordanceConstructionOrigin
	// Lemma narrows a construction lookup to parallels whose anchor's effective
	// lemma is this lemma key ("Only with {lemma}"). Empty means no filter.
	Lemma string
	// OriginLemma is the origin anchor's effective lemma, carried so the page can
	// offer the filter. It never affects which rows match.
	OriginLemma string
}

// ConcordancePriorityNone captures that no Book was being read when the
// lookup was applied.
const ConcordancePriorityNone = "none"

// Priority-Book evidence states reported with a result that has a priority.
const (
	ConcordancePriorityMatches     = "matches"
	ConcordancePriorityNoMatches   = "no-matches"
	ConcordancePriorityUnavailable = "unavailable"
)

type ConcordanceResult struct {
	Occurrences []ConcordanceResultOccurrence
	Page        int
	HasPrevious bool
	HasNext     bool
	Revision    string
	Stale       bool
	// Match and Term describe the applied interpretation; Term is normalized
	// for the language (the canonical lemma or the lowercased source form).
	Match string
	Term  string
	// PriorityBookID and PriorityTitle name the validated captured priority
	// Book; both are empty when no Book was captured. PriorityState says
	// whether that Book contributed matches, had none, or has no current
	// analysis to search.
	PriorityBookID, PriorityTitle, PriorityState string
	// InvalidPriority reports a captured Book that is not an owned Book in the
	// study language.
	InvalidPriority bool
	// Continued is true when the first page-local Book group began on an
	// earlier page.
	Continued bool
	// InvalidOrigin reports a construction lookup whose origin occurrence is
	// missing, unowned, outside the captured Book, or has no Reading unit.
	InvalidOrigin bool
	// Origin is the validated origin of a construction lookup.
	Origin ConcordanceConstructionOriginSentence
	// UnsearchedBooks are the owner's chosen-language Books in the lookup
	// language whose current analysis has no construction evidence, by title.
	UnsearchedBooks []ConcordanceUnsearchedBook
}

// ConcordanceUnsearchedBook is a Book a parallels lookup could not search. A
// Book whose syntax is merely rebuilding is never one.
type ConcordanceUnsearchedBook struct {
	BookID, Title string
	// UpdateAvailable reports an Analysis update offer for the Book.
	UpdateAvailable bool
}

// ConcordanceConstructionOriginSentence is the validated origin of a parallels
// lookup: the sentence Sentence study reopens, and the Work and author the
// tiers are measured against.
type ConcordanceConstructionOriginSentence struct {
	BookID, BookTitle, CorpusID, UnitID, AnchorSurface string
	WorkTitle, Author                                  string
	// Current reports that the origin run is still the Book's current
	// analysis, so Sentence study can reopen its sentence.
	Current bool
}

type ConcordanceResultOccurrence struct {
	ConcordanceOccurrence
	RawLemma, EffectiveLemma string
	Corrected, Excluded      bool
	// SpeakerLabel reports that this occurrence overlaps a recorded speaker
	// label in its exact snapshot, so Concordance presents it as speaker-label
	// evidence. It remains searchable and is never deleted.
	SpeakerLabel bool
	// StudyUnavailable reports that the occurrence's sentence overlaps a
	// recorded speaker label. Such a sentence supplies no Study affordance and
	// cannot be studied directly (ADR 0103).
	StudyUnavailable bool
	// Attribution is the quiet structural metadata for the sentence's exact
	// source span: a speaker name (nested directions omitted) or a stage
	// direction. It is never part of the sentence or a target.
	Attribution SourceAttribution
	// Tier, Anchor, ConstructionExtent, ConstructionID, SubtypeID and Study
	// describe a construction parallel; they are empty for a word lookup.
	// Study carries the sentence's words and the one occurrence, so the
	// expanded row reuses the Sentence study extract.
	Tier               string
	Anchor             int
	ConstructionExtent []int
	ConstructionID     string
	SubtypeID          string
	Study              SentenceStudy
}

type SentenceStudy struct {
	// AnalysisRunID is the run the sentence was read from.
	AnalysisRunID                                 string
	BookID, BookTitle, ChapterTitle, SentenceText string
	TargetSurface                                 string
	SentenceOrdinal, TargetOrdinal                int64
	// UnitText is the immutable extracted source text bound to the analysis's
	// snapshot (source_material_units.text). SentenceStartOffset and
	// SentenceEndOffset are half-open Unicode code-point offsets into it, so a
	// reader validates the sentence against its authoritative source slice
	// before aligning any word.
	UnitText                               string
	SentenceStartOffset, SentenceEndOffset int64
	// SyntaxState is the analysis run's recorded syntax state: "ud" for an
	// ordinary run, or "native_agdt"/"absent" for an imported run whose stored
	// dependency and head are a non-committal skeleton. An empty state is an
	// ordinary run for callers that construct a study without a run.
	SyntaxState string
	// SpeakerLabelOverlap reports that the sentence overlaps a recorded speaker
	// label in its exact source snapshot. Such a sentence is raw evidence, not
	// study context, so a direct Study request is refused (ADR 0103).
	SpeakerLabelOverlap bool
	// Attribution is the structural attribution of the sentence's exact source
	// span: a speaker name (nested directions omitted) or a stage direction.
	// It is quiet metadata, never part of the sentence or a target. It is empty
	// when the snapshot recorded no structure or no label precedes the span.
	Attribution SourceAttribution
	Tokens      []SentenceStudyToken
	// UnprintedNodes are the sentence's Unprinted syntax nodes, read only for a
	// native_agdt run. They have no form, lemma or occurrence.
	UnprintedNodes []UnprintedSyntaxNode
	// Constructions are the sentence's Construction occurrences, present only
	// when the run's marker carries the current detector version.
	Constructions []SentenceStudyConstruction
	// UnresolvedDecisions are stored occurrence decisions that address this
	// sentence but cannot be attributed to exactly one persisted analyzed word.
	// They are shown separately and are never applied to a word.
	UnresolvedDecisions []SentenceStudyDecision
}

type SentenceStudyToken struct {
	Surface, RawLemma, EffectiveLemma, UPOS, Dependency, HeadSurface string
	// Headword is the imported source's learner-facing citation form, empty for
	// modern-language tokens. A learner-facing view shows it instead of the
	// stored canonical key.
	Headword             string
	Ordinal, HeadOrdinal int64
	// StartOffset and EndOffset are the word's half-open source span within its
	// source unit; they match a decision's span when it addresses this word, and
	// a reader aligns the word only when they are valid, non-overlapping with
	// siblings, and their source slice exactly matches the stored surface.
	StartOffset, EndOffset int64
	// Morphology is the stored analyzer morphology for this token, unfiltered.
	// An empty map means the analyzer supplied no features for the word.
	Morphology map[string]string
	// Corrected and Excluded report an occurrence decision attributed to this
	// word because the decision addresses exactly this persisted word.
	Corrected, Excluded bool
	// HeadUnprintedNode is the source id of the Unprinted syntax node this word
	// depends on, or empty. When set, HeadOrdinal is the word's own ordinal and
	// HeadSurface is meaningless: a reader checks this first.
	HeadUnprintedNode string
}

// SentenceStudyConstruction is one stored Construction occurrence of a
// sentence: identifiers and token ordinals only. The note's name, definition and
// citation are looked up when the page renders.
type SentenceStudyConstruction struct {
	ConstructionID, SubtypeID string
	// Anchor is the printed token ordinal that signals the construction.
	Anchor int
	// Extent holds the sorted printed token ordinals of the construction. It may
	// be discontinuous and contains the anchor.
	Extent []int
}

// UnprintedSyntaxNode is one stored AGDT ellipsis node of a sentence: a node
// of the syntax tree with no printed word.
type UnprintedSyntaxNode struct {
	SourceID, Dependency string
	// HeadTokenOrdinal is the printed word it depends on; HasHeadToken is false
	// when it depends on no printed word.
	HeadTokenOrdinal int64
	HasHeadToken     bool
	// HeadUnprintedNode is the source id of the unprinted node it depends on, or
	// empty. With no head of either kind the node is a root.
	HeadUnprintedNode string
}

// SentenceStudyDecision is one stored occurrence decision addressing a sentence
// that Sentence study could not attribute to a single analyzed word.
type SentenceStudyDecision struct {
	StartOffset, EndOffset int64
	// CanonicalLemma is the correction's proposed lemma; it is empty for an
	// exclusion and is never presented as a confirmed effective lemma.
	CanonicalLemma string
	Excluded       bool
	// AddressedWords counts every persisted analyzed word the decision addresses
	// by its exact owner/Book/analysis/source-document/span. Exactly one permits
	// attribution; zero or more than one leaves it unresolved.
	AddressedWords int64
}

type AnalysisJob struct {
	ID, DisplayNumber                                int64
	OwnerID, SourceMaterialID, ContentHash, CorpusID string
	AnalysisRunID                                    string
	// BookID and BookTitle name the Book the job's row links to. The title
	// falls back to the acquired source's title when the Book row no longer
	// resolves one.
	BookID, BookTitle    string
	Lifecycle            JobLifecycleState
	Stage                BookPreparationStage
	FailureCode          BookPreparationCode
	Progress             int
	CreatedAt, UpdatedAt time.Time
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
	LexicalDiscriminator                        string
	// Headword is the imported citation form the accepted snapshot showed,
	// empty for modern-language vocabulary. Display-only; never part of the
	// identity.
	Headword  string
	CreatedAt time.Time
}
type GeneratedVocabulary struct {
	OwnerID, Language, CanonicalLemma, UPOS string
	// LexicalDiscriminator and Headword carry the imported identity's
	// scheme-qualified discriminator and citation form, empty for
	// modern-language vocabulary.
	LexicalDiscriminator  string
	Headword              string
	FirstDeckID           string
	FirstSourceMaterialID *string
	FirstGeneratedAt      time.Time
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
	LexicalDiscriminator                              string
	// Headword is the imported citation form shown to learners, empty for
	// modern-language vocabulary.
	Headword                                      string
	OccurrenceCount                               int
	FirstEncounter                                int64
	ObservedForms, SentenceReferences, Provenance []byte
	SelectedAt                                    time.Time
}

// ProjectedCorpusVocabulary is the per-Book effective vocabulary of one
// corpus, read from the Browse count projection. Ready is false while the
// Book's current analysis has no ready projection; Lemmas is then empty and
// must not be replaced by counts derived any other way.
type ProjectedCorpusVocabulary struct {
	AnalysisCorpusVocabulary
	Ready bool
}
