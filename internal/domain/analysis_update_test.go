package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnalysisUpdateOfferNeverChangesEvidence(t *testing.T) {
	contents := []ContentSignal{ContentNotAcquired, ContentNoCurrentRevision, ContentNonEPUB, ContentCurrentEPUB, ContentCurrentImported}
	publications := []PublishedAnalysisSignal{PublishedNone, PublishedCurrent, PublishedStale}
	runs := []LatestRunSignal{RunNone, RunQueued, RunRunning, RunPublicationPending, RunPublicationFailed, RunFailed, RunJobFailed, RunCancelled, RunCompleted}
	for _, content := range contents {
		for _, published := range publications {
			for _, run := range runs {
				for _, disposition := range []BookDisposition{BookDispositionInbox, BookDispositionToRead} {
					for _, current := range []bool{false, true} {
						signals := AnalysisSignals{Content: content, Published: published, LatestRun: run, CurrentReading: current}
						before := ClassifyBookEvidence(signals, disposition, "de")
						signals.PipelineOutdated = true
						after := ClassifyBookEvidence(signals, disposition, "de")
						assert.Equal(t, AnalysisUpdateNone, before.Update, "%+v", signals)
						after.Update = before.Update
						assert.Equal(t, before, after, "an outdated pipeline changes only the offer for %+v", signals)
					}
				}
			}
		}
	}
}

func TestAnalysisUpdateOfferEligibility(t *testing.T) {
	outdated := AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunCompleted, PipelineOutdated: true}
	assert.Equal(t, AnalysisUpdateAvailable, ClassifyBookEvidence(outdated, BookDispositionToRead, "de").Update)
	assert.Equal(t, AnalysisUpdateNone, ClassifyBookEvidence(outdated, BookDispositionInbox, "de").Update, "other dispositions get no offer")

	current := outdated
	current.CurrentReading = true
	assert.Equal(t, AnalysisUpdateEndReadingFirst, ClassifyBookEvidence(current, BookDispositionToRead, "de").Update)

	imported := outdated
	imported.Content = ContentCurrentImported
	assert.Equal(t, AnalysisUpdateNone, ClassifyBookEvidence(imported, BookDispositionToRead, "de").Update, "imported analyses are never offered")

	for _, run := range []LatestRunSignal{RunQueued, RunRunning, RunFailed, RunJobFailed, RunCancelled, RunPublicationPending} {
		inFlight := outdated
		inFlight.LatestRun = run
		got := ClassifyBookEvidence(inFlight, BookDispositionToRead, "de")
		assert.Equal(t, AnalysisUpdateNone, got.Update, "a newer %s run keeps its own status", run)
	}

	upToDate := outdated
	upToDate.PipelineOutdated = false
	assert.Equal(t, AnalysisUpdateNone, ClassifyBookEvidence(upToDate, BookDispositionToRead, "de").Update)
}

func TestSummaryAndMyBookDeriveTheOfferFromTheCandidate(t *testing.T) {
	candidate := &AnalysisUpdateCandidate{PublishedRunID: "run"}
	summary := SourceMaterialSummary{Source: SourceMaterial{Language: "de"}, Signals: AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunCompleted}, AnalysisUpdate: candidate}
	assert.Equal(t, AnalysisUpdateAvailable, summary.EvidenceClassification().Update)
	book := MyBook{Book: Book{LanguageState: LanguageChosen, LanguageTag: "de"}, Disposition: BookDispositionToRead, Acquired: &summary}
	assert.Equal(t, AnalysisUpdateAvailable, book.Classification().Update)
	book.IsCurrentReading = true
	assert.Equal(t, AnalysisUpdateEndReadingFirst, book.Classification().Update)
	summary.AnalysisUpdate = nil
	assert.Equal(t, AnalysisUpdateNone, summary.EvidenceClassification().Update)
}

func TestAnalysisUpdateTargetKey(t *testing.T) {
	assert.Equal(t, "a/3/french/2", AnalysisUpdateTarget{AnalyzerName: "a", AnalyzerVersion: "3", NormalizationProfile: "french", NormalizationVersion: "2"}.Key())
}
