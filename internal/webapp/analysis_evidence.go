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

// readingEvidenceStatus is the Reading view's presentation of one Book's
// evidence. The domain classification supplies the Book's standing; coverage
// availability is added by the Reading view, not by the classifier.
type readingEvidenceStatus string

const (
	readingEvidenceCurrent     readingEvidenceStatus = "current"
	readingEvidenceIncomplete  readingEvidenceStatus = "incomplete"
	readingEvidencePublishing  readingEvidenceStatus = "publishing"
	readingEvidenceStale       readingEvidenceStatus = "stale"
	readingEvidenceFailed      readingEvidenceStatus = "failed"
	readingEvidenceUnavailable readingEvidenceStatus = "unavailable"
	readingEvidenceUpdating    readingEvidenceStatus = "updating"
)

// readingEvidenceStatusFor maps a classification and coverage availability to
// the Reading view's evidence status.
func readingEvidenceStatusFor(c domain.BookEvidenceClassification, coverageAvailable bool) readingEvidenceStatus {
	switch {
	case c.Phase == domain.PhaseFailed:
		return readingEvidenceFailed
	case c.Evidence == domain.BookStale:
		return readingEvidenceStale
	case c.Evidence == domain.BookNotAcquired, c.Evidence == domain.BookUnavailable:
		return readingEvidenceUnavailable
	case c.PublicationPending():
		return readingEvidencePublishing
	case c.Evidence == domain.BookAcquiredUnassessed:
		return readingEvidenceIncomplete
	case c.Evidence == domain.BookAnalyzed:
		if coverageAvailable {
			return readingEvidenceCurrent
		}
		return readingEvidenceIncomplete
	default: // Unknown evidence states are not treated as current.
		return readingEvidenceUnavailable
	}
}

// analysisReadyForReading reports whether a Book's published analysis can be
// opened in Reading. A newer run that is queued, running, failed, or cancelled
// does not withdraw the published analysis while it still matches the content.
// Coverage is read from the corpus, so a Book without one stays out even when
// its classification is complete.
func analysisReadyForReading(book domain.SourceMaterialSummary) bool {
	return book.EvidenceClassification().AnalysisInEffect() && book.AnalysisRunID != "" && book.CorpusID != ""
}
