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
			name:        "published analysis under a failed newer run stays in effect",
			signals:     with(PublishedCurrent, RunFailed),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingEligible, recovery: RecoveryRetryAnalysis,
		},
		{
			name:        "published analysis under a job-failed newer run stays in effect",
			signals:     with(PublishedCurrent, RunJobFailed),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingEligible, recovery: RecoveryRetryAnalysis,
		},
		{
			name:        "published analysis under a cancelled newer run stays in effect",
			signals:     with(PublishedCurrent, RunCancelled),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingEligible, recovery: RecoveryRetryAnalysis,
		},
		{
			name:        "published analysis under a queued newer run stays in effect",
			signals:     with(PublishedCurrent, RunQueued),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingEligible, recovery: RecoveryNone,
		},
		{
			name:        "published analysis under a running newer run stays in effect",
			signals:     with(PublishedCurrent, RunRunning),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingEligible, recovery: RecoveryNone,
		},
		{
			name:        "published analysis under a newer run awaiting publication is not yet eligible",
			signals:     with(PublishedCurrent, RunPublicationPending),
			disposition: BookDispositionToRead, language: "de",
			evidence: BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingNoCompletedAnalysis, recovery: RecoveryNone,
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
						for _, current := range []bool{false, true} {
							signals := AnalysisSignals{Content: content, Published: published, LatestRun: run, CurrentReading: current}
							got := ClassifyBookEvidence(signals, disposition, language)

							// A published analysis stays in effect under a newer run that is
							// queued, running, failed, or cancelled; only a completed run or
							// such a shadowed run makes the analysis usable for Reading.
							newerShadowed := published == PublishedCurrent && (run == RunQueued || run == RunRunning || run == RunFailed || run == RunJobFailed || run == RunCancelled)
							analyzed := got.Phase == PhaseAnalyzed && (run == RunCompleted || newerShadowed)
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

							shadowedRetry := published == PublishedCurrent && !current && (run == RunFailed || run == RunJobFailed || run == RunCancelled)
							retryAnalysis := got.Phase == PhaseFailed || got.Phase == PhaseCancelled || got.Phase == PhaseStale || shadowedRetry
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
}

func TestBookEvidenceClassificationReAnalysisShadowedByPublishedAnalysis(t *testing.T) {
	shadowed := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunRunning}, BookDispositionToRead, "de")
	assert.True(t, shadowed.ReAnalysisShadowed())
	assert.True(t, shadowed.AnalysisInEffect())
	assert.False(t, shadowed.RunFinished(), "the newer run is still in progress")

	failed := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunFailed}, BookDispositionToRead, "de")
	assert.True(t, failed.ReAnalysisShadowed())
	assert.True(t, failed.AnalysisInEffect())

	completed := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunCompleted}, BookDispositionToRead, "de")
	assert.False(t, completed.ReAnalysisShadowed(), "a completed run is the analysis, not a shadowed run")
	assert.True(t, completed.AnalysisInEffect())

	unpublished := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, LatestRun: RunFailed}, BookDispositionToRead, "de")
	assert.False(t, unpublished.ReAnalysisShadowed())
	assert.False(t, unpublished.AnalysisInEffect())
}

func TestBookEvidenceClassificationCurrentReadingOffersNoRetryForShadowedRun(t *testing.T) {
	for _, run := range []LatestRunSignal{RunFailed, RunJobFailed, RunCancelled} {
		current := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: run, CurrentReading: true}, BookDispositionToRead, "de")
		assert.Equal(t, RecoveryNone, current.Recovery, "current reading under %s", run)
		assert.Equal(t, CurrentReadingEligible, current.Eligibility, "current reading under %s stays eligible", run)
		assert.True(t, current.AnalysisInEffect(), "current reading under %s keeps its published analysis", run)
		assert.True(t, current.ReAnalysisShadowed(), "current reading under %s keeps the newer run as secondary", run)

		other := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: run}, BookDispositionToRead, "de")
		assert.Equal(t, RecoveryRetryAnalysis, other.Recovery, "a To Read book under %s offers retry", run)
	}

	failedStale := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedStale, LatestRun: RunFailed, CurrentReading: true}, BookDispositionToRead, "de")
	assert.Equal(t, RecoveryRetryAnalysis, failedStale.Recovery, "a stale publication still offers retry even for the Current reading")

	mybook := MyBook{Book: Book{LanguageState: LanguageChosen, LanguageTag: "de"}, Disposition: BookDispositionToRead, IsCurrentReading: true, Acquired: &SourceMaterialSummary{Signals: AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunFailed}}}
	assert.Equal(t, RecoveryNone, mybook.Classification().Recovery, "My Books classifies the Current reading without retry")
	assert.Equal(t, CurrentReadingEligible, mybook.Classification().Eligibility)

	source := SourceMaterialSummary{Source: SourceMaterial{Language: "de"}, Signals: AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunCancelled}, IsCurrentReading: true}
	assert.Equal(t, RecoveryNone, source.EvidenceClassification().Recovery, "Reading classifies the Current reading without retry")
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
