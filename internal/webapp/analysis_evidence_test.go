package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

// testPublicationFailed is a finished run whose publication failed before any
// result was published for the current content.
var testPublicationFailed = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, LatestRun: domain.RunPublicationFailed}

func TestReadingEvidenceStatusMapsClassificationAndCoverage(t *testing.T) {
	tests := []struct {
		name     string
		c        domain.BookEvidenceClassification
		coverage bool
		want     readingEvidenceStatus
	}{
		{name: "analyzed with coverage is current", c: domain.BookEvidenceClassification{Evidence: domain.BookAnalyzed, Phase: domain.PhaseAnalyzed, Run: domain.RunCompleted}, coverage: true, want: readingEvidenceCurrent},
		{name: "analyzed without coverage is incomplete", c: domain.BookEvidenceClassification{Evidence: domain.BookAnalyzed, Phase: domain.PhaseAnalyzed, Run: domain.RunCompleted}, want: readingEvidenceIncomplete},
		{name: "publication pending is publishing", c: domain.BookEvidenceClassification{Content: domain.ContentCurrentEPUB, Evidence: domain.BookAcquiredUnassessed, Phase: domain.PhaseAnalyzing, Run: domain.RunPublicationPending}, want: readingEvidencePublishing},
		{name: "publication failed before publishing is publishing", c: domain.BookEvidenceClassification{Content: domain.ContentCurrentEPUB, Evidence: domain.BookAcquiredUnassessed, Phase: domain.PhaseNotAnalyzed, Run: domain.RunPublicationFailed}, want: readingEvidencePublishing},
		{name: "publication pending without content is unavailable", c: domain.BookEvidenceClassification{Content: domain.ContentNoCurrentRevision, Evidence: domain.BookUnavailable, Phase: domain.PhaseAnalyzing, Run: domain.RunPublicationPending}, want: readingEvidenceUnavailable},
		{name: "publication pending over stale publication is stale", c: domain.BookEvidenceClassification{Content: domain.ContentCurrentEPUB, Evidence: domain.BookStale, Phase: domain.PhaseStale, Run: domain.RunPublicationPending}, want: readingEvidenceStale},
		{name: "failed run is failed", c: domain.BookEvidenceClassification{Content: domain.ContentCurrentEPUB, Evidence: domain.BookAcquiredUnassessed, Phase: domain.PhaseFailed, Run: domain.RunFailed}, want: readingEvidenceFailed},
		{name: "unassessed book is incomplete", c: domain.BookEvidenceClassification{Content: domain.ContentCurrentEPUB, Evidence: domain.BookAcquiredUnassessed, Phase: domain.PhaseNotAnalyzed, Run: domain.RunNone}, want: readingEvidenceIncomplete},
		{name: "not acquired is unavailable", c: domain.BookEvidenceClassification{Content: domain.ContentNotAcquired, Evidence: domain.BookNotAcquired}, want: readingEvidenceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, readingEvidenceStatusFor(tt.c, tt.coverage))
		})
	}
}

// TestPublicationPendingHasOnePresentationOnEverySurface checks the surfaces
// that present a finished run awaiting publication. None may read as not
// started, and none may offer a recovery action.
func TestPublicationPendingHasOnePresentationOnEverySurface(t *testing.T) {
	for name, signals := range map[string]domain.AnalysisSignals{"pending": testPending, "publication failed": testPublicationFailed} {
		t.Run(name, func(t *testing.T) {
			source := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book", OwnerID: "owner", Language: "de", MediaType: "application/epub+zip", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"}, AnalysisRunID: "run", Signals: signals}
			book := domain.MyBook{Book: domain.Book{ID: "book", OwnerID: "owner", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, Disposition: domain.BookDispositionToRead, Acquired: &source}

			lifecycle := bookLifecycleActionFor(source)
			assert.Equal(t, analysisPublicationPendingLabel, lifecycle.Status)
			assert.Empty(t, lifecycle.Label)
			assert.Empty(t, lifecycle.URL)

			reading := readingAnalysisAction(readingBookView{Book: source, BookID: "book"})
			assert.Equal(t, analysisPublicationPendingLabel, reading.Status)
			assert.Empty(t, reading.Label, "publication pending offers no recovery action in Reading")
			assert.False(t, reading.Submit)

			assert.Equal(t, "Analysis in progress.", myBookMarginEvidence(book))
			assert.Empty(t, myBookEvidenceRecovery(book))
		})
	}
}

func TestAnalysisReadyForReadingRequiresCompletedPublishedAnalysisAndCorpus(t *testing.T) {
	ready := domain.SourceMaterialSummary{Source: domain.SourceMaterial{MediaType: "application/epub+zip", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"}, AnalysisRunID: "run", CorpusID: "corpus", Signals: testAnalyzed}
	assert.True(t, analysisReadyForReading(ready))

	withoutCorpus := ready
	withoutCorpus.CorpusID = ""
	assert.False(t, analysisReadyForReading(withoutCorpus), "Reading needs the corpus for coverage")

	pending := ready
	pending.Signals = testPending
	assert.False(t, analysisReadyForReading(pending))

	stale := ready
	stale.Signals = testStale
	assert.False(t, analysisReadyForReading(stale))
}
