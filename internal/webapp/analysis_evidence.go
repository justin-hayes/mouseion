package webapp

import "github.com/justin-hayes/mouseion/internal/domain"

// analysisPublicationPendingLabel and analysisPublicationPendingDescription
// present a finished run whose result is not yet published. Every surface shows
// it as in progress with no recovery action.
const (
	analysisPublicationPendingLabel       = "Analysis in progress"
	analysisPublicationPendingDescription = "Analysis finished and its result is being published. This Book is not shown as analyzed until publication completes."
	analysisPublicationPendingNote        = "Analysis in progress."
)

// analysisReadyForReading reports whether a Book's published analysis can be
// opened in Reading. A newer run that is queued, running, failed, or cancelled
// does not withdraw the published analysis while it still matches the content.
// Coverage is read from the corpus, so a Book without one stays out even when
// its classification is complete.
func analysisReadyForReading(book domain.SourceMaterialSummary) bool {
	return book.EvidenceClassification().AnalysisInEffect() && book.AnalysisRunID != "" && book.CorpusID != ""
}

// analysisAttention is an analysis state that needs the learner's action,
// shared by every surface that says so so they agree on when.
type analysisAttention int

const (
	analysisAttentionFailed analysisAttention = iota + 1
	analysisAttentionReAnalysisFailed
	analysisAttentionStale
)

// analysisAttentionFor reports whether the Book's analysis failed, is out of
// date, or has a failed re-analysis shadowed by a published one. Missing
// statistics alone need no action.
func analysisAttentionFor(c domain.BookEvidenceClassification) (analysisAttention, bool) {
	switch {
	case c.Phase == domain.PhaseFailed:
		return analysisAttentionFailed, true
	case c.ReAnalysisShadowed() && c.Run == domain.RunFailed:
		return analysisAttentionReAnalysisFailed, true
	case c.Phase == domain.PhaseStale:
		return analysisAttentionStale, true
	}
	return 0, false
}
