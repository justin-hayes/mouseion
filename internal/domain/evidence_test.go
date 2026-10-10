package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyBookEvidenceReproducesPresentationStates(t *testing.T) {
	current := AnalysisSignals{Content: ContentCurrentEPUB}
	with := func(published PublishedAnalysisSignal, run LatestRunSignal) AnalysisSignals {
		signals := current
		signals.Published = published
		signals.LatestRun = run
		return signals
	}
	tests := []struct {
		name        string
		signals     AnalysisSignals
		disposition BookDisposition
		language    string
		evidence    BookEvidenceState
		phase       AnalysisPhase
		eligibility CurrentReadingEligibilityReason
		recovery    BookEvidenceRecovery
	}{
		{
			name:        "not acquired",
			disposition: BookDispositionToRead, language: "de",
			evidence: BookNotAcquired, phase: PhaseNotAnalyzed, eligibility: CurrentReadingNeedsCurrentContent, recovery: RecoveryNone,
		},
		{
			name:        "no current revision",
			signals:     AnalysisSignals{Content: ContentNoCurrentRevision},
			disposition: BookDispositionToRead, language: "de",
			evidence: BookUnavailable, phase: PhaseNotAnalyzed, eligibility: CurrentReadingNeedsCurrentContent, recovery: RecoveryRetryAcquisition,
		},
		{
			name:        "non-EPUB content is current for evidence but cannot be started",
			signals:     AnalysisSignals{Content: ContentNonEPUB, Published: PublishedCurrent, LatestRun: RunCompleted},
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingNeedsCurrentContent, recovery: RecoveryNone,
		},
		{
			name:        "queued analysis",
			signals:     with(PublishedNone, RunQueued),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAcquiredUnassessed, phase: PhaseAnalyzing, eligibility: CurrentReadingAnalysisInProgress, recovery: RecoveryNone,
		},
		{
			name:        "running analysis",
			signals:     with(PublishedNone, RunRunning),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAcquiredUnassessed, phase: PhaseAnalyzing, eligibility: CurrentReadingAnalysisInProgress, recovery: RecoveryNone,
		},
		{
			name:        "completed run awaiting publication",
			signals:     with(PublishedNone, RunPublicationPending),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAcquiredUnassessed, phase: PhaseAnalyzing, eligibility: CurrentReadingAnalysisInProgress, recovery: RecoveryNone,
		},
		{
			name:        "completed run whose publication failed",
			signals:     with(PublishedNone, RunPublicationFailed),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAcquiredUnassessed, phase: PhaseNotAnalyzed, eligibility: CurrentReadingNoCompletedAnalysis, recovery: RecoveryNone,
		},
		{
			name:        "failed analysis run",
			signals:     with(PublishedNone, RunFailed),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAcquiredUnassessed, phase: PhaseFailed, eligibility: CurrentReadingFailed, recovery: RecoveryRetryAnalysis,
		},
		{
			name:        "job failed before a run was recorded",
			signals:     with(PublishedNone, RunJobFailed),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAcquiredUnassessed, phase: PhaseNotAnalyzed, eligibility: CurrentReadingFailed, recovery: RecoveryNone,
		},
		{
			name:        "cancelled analysis",
			signals:     with(PublishedNone, RunCancelled),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAcquiredUnassessed, phase: PhaseCancelled, eligibility: CurrentReadingCancelled, recovery: RecoveryRetryAnalysis,
		},
		{
			name:        "stale publication",
			signals:     with(PublishedStale, RunNone),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookStale, phase: PhaseStale, eligibility: CurrentReadingStale, recovery: RecoveryRetryAnalysis,
		},
		{
			name:        "stale publication hides a pending run",
			signals:     with(PublishedStale, RunPublicationPending),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookStale, phase: PhaseStale, eligibility: CurrentReadingStale, recovery: RecoveryRetryAnalysis,
		},
		{
			name:        "published analysis with no run record",
			signals:     with(PublishedCurrent, RunNone),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingNoCompletedAnalysis, recovery: RecoveryNone,
		},
		{
			name:        "published analysis under a failed newer run",
			signals:     with(PublishedCurrent, RunFailed),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAcquiredUnassessed, phase: PhaseFailed, eligibility: CurrentReadingFailed, recovery: RecoveryRetryAnalysis,
		},
		{
			name:        "published analysis under a queued newer run",
			signals:     with(PublishedCurrent, RunQueued),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAcquiredUnassessed, phase: PhaseAnalyzing, eligibility: CurrentReadingAnalysisInProgress, recovery: RecoveryNone,
		},
		{
			name:        "eligible",
			signals:     with(PublishedCurrent, RunCompleted),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingEligible, recovery: RecoveryNone,
		},
		{
			name:        "not To Read",
			signals:     with(PublishedCurrent, RunCompleted),
			disposition: BookDispositionInbox, language: "de",
			evidence: BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingNotToRead, recovery: RecoveryNone,
		},
		{
			name:        "no chosen language",
			signals:     with(PublishedCurrent, RunCompleted),
			disposition: BookDispositionToRead,
			evidence:    BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingNoChosenLanguage, recovery: RecoveryNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyBookEvidence(tt.signals, tt.disposition, tt.language)
			assert.Equal(t, tt.evidence, got.Evidence, "evidence")
			assert.Equal(t, tt.phase, got.Phase, "phase")
			assert.Equal(t, tt.eligibility, got.Eligibility, "eligibility")
			assert.Equal(t, tt.recovery, got.Recovery, "recovery")
		})
	}
}

func TestClassifyBookEvidenceHoldsForEverySignalCombination(t *testing.T) {
	contents := []ContentSignal{ContentNotAcquired, ContentNoCurrentRevision, ContentNonEPUB, ContentCurrentEPUB}
	publications := []PublishedAnalysisSignal{PublishedNone, PublishedCurrent, PublishedStale}
	runs := []LatestRunSignal{RunNone, RunQueued, RunRunning, RunPublicationPending, RunPublicationFailed, RunFailed, RunJobFailed, RunCancelled, RunCompleted}
	dispositions := []BookDisposition{BookDispositionInbox, BookDispositionToRead}
	languages := []string{"", "de"}

	for _, content := range contents {
		for _, published := range publications {
			for _, run := range runs {
				for _, disposition := range dispositions {
					for _, language := range languages {
						signals := AnalysisSignals{Content: content, Published: published, LatestRun: run}
						got := ClassifyBookEvidence(signals, disposition, language)

						analyzed := got.Phase == PhaseAnalyzed && run == RunCompleted
						ready := content == ContentCurrentEPUB && analyzed && disposition == BookDispositionToRead && language != ""
						assert.Equal(t, ready, got.Eligibility == CurrentReadingEligible, "eligibility for %+v %v %q", signals, disposition, language)

						switch content {
						case ContentNotAcquired:
							assert.Equal(t, BookNotAcquired, got.Evidence, "%+v", signals)
							assert.Equal(t, RecoveryNone, got.Recovery, "%+v", signals)
						case ContentNoCurrentRevision:
							assert.Equal(t, BookUnavailable, got.Evidence, "%+v", signals)
						case ContentNonEPUB, ContentCurrentEPUB:
						}

						retryAnalysis := got.Phase == PhaseFailed || got.Phase == PhaseCancelled || got.Phase == PhaseStale
						if content != ContentNotAcquired {
							assert.Equal(t, retryAnalysis, got.Recovery == RecoveryRetryAnalysis, "recovery for %+v", signals)
						}
						hasRevision := content == ContentNonEPUB || content == ContentCurrentEPUB
						assert.Equal(t, got.Phase == PhaseAnalyzed && hasRevision, got.Evidence == BookAnalyzed, "evidence for %+v", signals)
						assert.False(t, got.Eligibility == CurrentReadingEligible && got.Phase != PhaseAnalyzed, "eligible without analysis for %+v", signals)
					}
				}
			}
		}
	}
}

func TestBookEvidenceClassificationPublicationPending(t *testing.T) {
	pending := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, LatestRun: RunPublicationPending}, BookDispositionToRead, "de")
	assert.True(t, pending.PublicationPending())
	assert.True(t, pending.RunFinished())

	stale := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedStale, LatestRun: RunPublicationPending}, BookDispositionToRead, "de")
	assert.False(t, stale.PublicationPending(), "a stale publication is not presented as pending")
	assert.True(t, stale.RunFinished())

	queued := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, LatestRun: RunQueued}, BookDispositionToRead, "de")
	assert.False(t, queued.PublicationPending())
	assert.False(t, queued.RunFinished())
}

func TestSourceMaterialSummaryEvidenceDelegatesToClassification(t *testing.T) {
	summary := SourceMaterialSummary{
		Source:  SourceMaterial{ID: "source", ContentRevisionID: "revision", ContentSnapshotID: "snapshot", MediaType: "application/epub+zip"},
		Signals: AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunCompleted},
	}

	assert.Equal(t, BookAnalyzed, summary.EvidenceState())
	assert.Equal(t, BookNotAcquired, (SourceMaterialSummary{}).EvidenceState())
	assert.Equal(t, BookAnalyzed, MyBook{Acquired: &summary}.EvidenceState())
	assert.Equal(t, BookNotAcquired, (MyBook{}).EvidenceState())
}

func TestMyBookClassificationUsesDispositionAndChosenLanguage(t *testing.T) {
	acquired := &SourceMaterialSummary{Signals: AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunCompleted}}
	book := MyBook{Book: Book{LanguageState: LanguageChosen, LanguageTag: "de"}, Disposition: BookDispositionToRead, Acquired: acquired}

	assert.Equal(t, CurrentReadingEligible, book.Classification().Eligibility)
	assert.Equal(t, CurrentReadingNoChosenLanguage, MyBook{Book: Book{LanguageTag: "de"}, Disposition: BookDispositionToRead, Acquired: acquired}.Classification().Eligibility)
	assert.Equal(t, CurrentReadingNotToRead, MyBook{Book: book.Book, Disposition: BookDispositionInbox, Acquired: acquired}.Classification().Eligibility)
}

func TestClassificationCarriesContentAndCompletedAnalysis(t *testing.T) {
	completed := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunCompleted}, BookDispositionToRead, "de")
	assert.Equal(t, ContentCurrentEPUB, completed.Content)
	assert.True(t, completed.CompletedAnalysis())

	pending := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, LatestRun: RunPublicationPending}, BookDispositionToRead, "de")
	assert.False(t, pending.CompletedAnalysis(), "a run awaiting publication is not a completed analysis")

	noContent := ClassifyBookEvidence(AnalysisSignals{Content: ContentNoCurrentRevision, Published: PublishedCurrent, LatestRun: RunCompleted}, BookDispositionToRead, "de")
	assert.False(t, noContent.CompletedAnalysis(), "analysis without current content is unavailable evidence")
	assert.Equal(t, ContentNoCurrentRevision, noContent.Content)
}
