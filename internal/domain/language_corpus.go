package domain

// LanguageCorpusView is derived on demand from current book analyses and the
// owner's current vocabulary. It is never persisted.
type LanguageCorpusView struct {
	OwnerID              string
	Language             string
	AnalyzedBookCount    int
	KnownTokenCount      int64
	AnalyzableTokenCount int64
	TopUnknownLemmas     []LemmaOccurrence
	PerBook              []LanguageCorpusBookSpread
}

type LanguageCorpusBookSpread struct {
	BookID               string
	Title                string
	SourceMaterialID     string
	CorpusID             string
	AnalysisRunID        string
	KnownTokenCount      int64
	AnalyzableTokenCount int64
	EvidenceState        BookEvidenceState
	Included             bool
	ExclusionReason      string
}

// LanguageCorpusBookEvidence is the lemma/statistics-only persistence input
// for LanguageCorpusView. It deliberately contains no source text or
// sentences.
type LanguageCorpusBookEvidence struct {
	Book                     Book
	SourceMaterialID         string
	SourceLanguage           string
	CurrentContentRevisionID string
	CurrentSnapshotID        string
	CurrentSourceMaterialID  string
	CurrentAnalysisRunID     string
	AnalysisSourceMaterialID string
	CorpusID                 string
	AnalysisRunID            string
	Statistics               *AnalysisStatistics
	Lemmas                   []LemmaOccurrence
}

// EvidenceState classifies corpus evidence from its source and analysis
// identities. A completed current analysis is analyzed; an older or broken
// current projection is stale; otherwise acquired evidence is unassessed.
func (b LanguageCorpusBookEvidence) EvidenceState() BookEvidenceState {
	if b.SourceMaterialID == "" {
		return BookNotAcquired
	}
	if b.CurrentContentRevisionID == "" || b.CurrentSnapshotID == "" {
		return BookUnavailable
	}
	if (b.AnalysisSourceMaterialID != "" && b.AnalysisSourceMaterialID != b.SourceMaterialID) ||
		(b.CurrentSourceMaterialID != "" && b.CurrentSourceMaterialID != b.SourceMaterialID) ||
		(b.AnalysisRunID != "" && b.CurrentAnalysisRunID == "") ||
		(b.CurrentAnalysisRunID != "" && b.CorpusID == "") {
		return BookStale
	}
	if b.CurrentAnalysisRunID != "" && b.CorpusID != "" {
		return BookAnalyzed
	}
	return BookAcquiredUnassessed
}
