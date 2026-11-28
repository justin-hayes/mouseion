package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

// testPublicationFailed is a finished run whose publication failed before any
// result was published for the current content.
var testPublicationFailed = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, LatestRun: domain.RunPublicationFailed}

// TestPublicationPendingHasOnePresentationOnEverySurface checks the surfaces
// that present a finished run awaiting publication. None may read as not
// started, and none may offer a recovery action.
func TestPublicationPendingHasOnePresentationOnEverySurface(t *testing.T) {
	for name, signals := range map[string]domain.AnalysisSignals{"pending": testPending, "publication failed": testPublicationFailed} {
		t.Run(name, func(t *testing.T) {
			source := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book", OwnerID: "owner", Language: "de", MediaType: "application/epub+zip", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"}, AnalysisRunID: "run", Signals: signals}
			book := domain.MyBook{Book: domain.Book{ID: "book", OwnerID: "owner", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, Disposition: domain.BookDispositionToRead, Acquired: &source}

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
