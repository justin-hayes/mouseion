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
	with := func(change func(*AnalysisSignals)) AnalysisSignals {
		signals := outdated
		change(&signals)
		return signals
	}
	tests := []struct {
		name        string
		signals     AnalysisSignals
		disposition BookDisposition
		want        AnalysisUpdateOffer
	}{
		{"outdated To Read Book", outdated, BookDispositionToRead, AnalysisUpdateAvailable},
		{"other disposition", outdated, BookDispositionInbox, AnalysisUpdateNone},
		{"current Reading", with(func(s *AnalysisSignals) { s.CurrentReading = true }), BookDispositionToRead, AnalysisUpdateEndReadingFirst},
		{"imported analysis", with(func(s *AnalysisSignals) { s.Content = ContentCurrentImported }), BookDispositionToRead, AnalysisUpdateNone},
		{"current pipeline", with(func(s *AnalysisSignals) { s.PipelineOutdated = false }), BookDispositionToRead, AnalysisUpdateNone},
	}
	for _, run := range []LatestRunSignal{RunQueued, RunRunning, RunFailed, RunJobFailed, RunCancelled, RunPublicationPending} {
		tests = append(tests, struct {
			name        string
			signals     AnalysisSignals
			disposition BookDisposition
			want        AnalysisUpdateOffer
		}{"newer " + string(run) + " run keeps its own status", with(func(s *AnalysisSignals) { s.LatestRun = run }), BookDispositionToRead, AnalysisUpdateNone})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifyBookEvidence(tt.signals, tt.disposition, "de").Update)
		})
	}
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
