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
	EvidenceState        MyBookEvidenceState
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
	CurrentSourceMaterialID  string
	CurrentAnalysisRunID     string
	AnalysisSourceMaterialID string
	CorpusID                 string
	AnalysisRunID            string
	EvidenceState            MyBookEvidenceState
	Statistics               *AnalysisStatistics
	Lemmas                   []LemmaOccurrence
}
