package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyBookEvidenceEPUBAcquisitionStages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		status      EPUBAcquisitionStatus
		acquisition AcquisitionState
		evidence    BookEvidenceState
		phase       AnalysisPhase
		recovery    BookEvidenceRecovery
	}{
		{
			name:        "queued",
			status:      EPUBAcquisitionStatus{State: JobLifecycleQueued},
			acquisition: AcquisitionInProgress, evidence: BookAcquiredUnassessed, phase: PhaseAnalyzing, recovery: RecoveryNone,
		},
		{
			name:        "running",
			status:      EPUBAcquisitionStatus{State: JobLifecycleRunning},
			acquisition: AcquisitionInProgress, evidence: BookAcquiredUnassessed, phase: PhaseAnalyzing, recovery: RecoveryNone,
		},
		{
			// A completed acquisition is no longer the Book's evidence; the
			// analysis carries the Book from here, so the classification makes
			// no acquisition claim for it.
			name:        "completed",
			status:      EPUBAcquisitionStatus{State: JobLifecycleCompleted},
			acquisition: AcquisitionNone, evidence: BookNotAcquired, phase: PhaseNotAnalyzed, recovery: RecoveryNone,
		},
		{
			name:        "entry missing",
			status:      EPUBAcquisitionStatus{Stage: BookPreparationStageEntry, Code: BookPreparationCodeCatalogEntryMissing},
			acquisition: AcquisitionEntryMissing, evidence: BookUnavailable, phase: PhaseNotAnalyzed, recovery: RecoveryRetryAcquisition,
		},
		{
			name:        "download failed",
			status:      EPUBAcquisitionStatus{Stage: BookPreparationStageFetch, Code: BookPreparationCodeSourceDownloadFailed},
			acquisition: AcquisitionFetchFailed, evidence: BookUnavailable, phase: PhaseNotAnalyzed, recovery: RecoveryRetryAcquisition,
		},
		{
			name:        "oversize source",
			status:      EPUBAcquisitionStatus{Stage: BookPreparationStageSource, Code: BookPreparationCodeSourceOversize},
			acquisition: AcquisitionSourceOversize, evidence: BookUnavailable, phase: PhaseNotAnalyzed, recovery: RecoveryNone,
		},
		{
			name:        "unknown source format",
			status:      EPUBAcquisitionStatus{Stage: BookPreparationStageSource, Code: BookPreparationCodeSourceFormatUnknown},
			acquisition: AcquisitionSourceFormatUnknown, evidence: BookUnavailable, phase: PhaseNotAnalyzed, recovery: RecoveryNone,
		},
		{
			name:        "refused source",
			status:      EPUBAcquisitionStatus{Stage: BookPreparationStageSource, Code: BookPreparationCodeSourcePinMismatch},
			acquisition: AcquisitionSourceRefused, evidence: BookUnavailable, phase: PhaseNotAnalyzed, recovery: RecoveryNone,
		},
		{
			name:        "import failed",
			status:      EPUBAcquisitionStatus{Stage: BookPreparationStageImport, Code: BookPreparationCodeEPUBImportFailed},
			acquisition: AcquisitionImportFailed, evidence: BookUnavailable, phase: PhaseFailed, recovery: RecoveryRetryAcquisition,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			classification := ClassifyBookEvidence(AnalysisSignals{EPUBAcquisition: &tt.status}, BookDispositionToRead, "de")
			assert.Equal(t, tt.acquisition, classification.Acquisition.State)
			assert.Equal(t, tt.evidence, classification.Evidence)
			assert.Equal(t, tt.phase, classification.Phase)
			assert.Equal(t, tt.recovery, classification.Recovery)
			assert.Equal(t, CurrentReadingNeedsCurrentContent, classification.Eligibility)
		})
	}
}

func TestClassifyBookEvidenceDoesNotDeriveTheStageFromTheCode(t *testing.T) {
	t.Parallel()
	classification := ClassifyBookEvidence(AnalysisSignals{
		EPUBAcquisition: &EPUBAcquisitionStatus{Code: BookPreparationCodeSourceFormatUnknown},
	}, BookDispositionToRead, "de")
	assert.Equal(t, AcquisitionNone, classification.Acquisition.State, "a row without a stage makes no acquisition claim")
}

func TestClassifyBookEvidenceKeepsAnalysisFailureCodeOnlyForAFailedRun(t *testing.T) {
	t.Parallel()
	failed := ClassifyBookEvidence(AnalysisSignals{
		Content:     ContentCurrentEPUB,
		LatestRun:   RunFailed,
		FailureCode: BookPreparationCodeDependencyParsingUnavailable,
	}, BookDispositionToRead, "de")
	assert.Equal(t, PhaseFailed, failed.Phase)
	assert.Equal(t, BookPreparationCodeDependencyParsingUnavailable, failed.FailureCode)

	queued := ClassifyBookEvidence(AnalysisSignals{
		Content:     ContentCurrentEPUB,
		LatestRun:   RunQueued,
		FailureCode: BookPreparationCodeAnalysisFailed,
	}, BookDispositionToRead, "de")
	assert.Equal(t, BookPreparationCodeNone, queued.FailureCode, "a queued run carries no failure")

	legacy := ClassifyBookEvidence(AnalysisSignals{Content: ContentCurrentEPUB, LatestRun: RunFailed}, BookDispositionToRead, "de")
	assert.Equal(t, BookPreparationCodeNone, legacy.FailureCode, "a run that failed before codes still classifies")
}

func TestEPUBAcquisitionFailureNeverOverridesAnAnalysisInEffect(t *testing.T) {
	t.Parallel()
	status := EPUBAcquisitionStatus{Stage: BookPreparationStageFetch, Code: BookPreparationCodeSourceDownloadFailed}
	classification := ClassifyBookEvidence(AnalysisSignals{
		Content:         ContentCurrentEPUB,
		Published:       PublishedCurrent,
		LatestRun:       RunCompleted,
		EPUBAcquisition: &status,
	}, BookDispositionToRead, "de")
	assert.Equal(t, AcquisitionNone, classification.Acquisition.State)
	assert.Equal(t, BookAnalyzed, classification.Evidence)
	assert.Equal(t, PhaseAnalyzed, classification.Phase)
	assert.True(t, classification.AnalysisInEffect())
}

func TestMyBookClassificationCarriesEPUBAcquisition(t *testing.T) {
	t.Parallel()
	book := MyBook{
		Book:        Book{LanguageState: LanguageChosen, LanguageTag: "de"},
		Disposition: BookDispositionToRead,
		EPUBAcquisition: &EPUBAcquisitionStatus{
			Stage: BookPreparationStageFetch, Code: BookPreparationCodeSourceDownloadFailed,
		},
	}
	classification := book.Classification()
	assert.Equal(t, AcquisitionFetchFailed, classification.Acquisition.State)
	assert.Equal(t, RecoveryRetryAcquisition, classification.Recovery)

	book.Acquired = &SourceMaterialSummary{
		Source:  SourceMaterial{MediaType: "application/epub+zip"},
		Signals: AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunCompleted},
	}
	assert.Equal(t, AcquisitionNone, book.Classification().Acquisition.State, "an analysis in effect ignores the failure")
}

func TestEPUBAcquisitionIgnoresOtherKindsStages(t *testing.T) {
	t.Parallel()
	for _, stage := range []BookPreparationStage{
		BookPreparationStageCatalog,
		BookPreparationStageWithdrawn,
		BookPreparationStageAnalysis,
		BookPreparationStageFinalize,
	} {
		classification := ClassifyBookEvidence(AnalysisSignals{
			EPUBAcquisition: &EPUBAcquisitionStatus{Stage: stage, Code: BookPreparationCodeClassicalCatalogLegacyRows},
		}, BookDispositionToRead, "de")
		assert.Equal(t, AcquisitionNone, classification.Acquisition.State, stage)
	}
}

func TestClassicalPreparationIgnoresOtherKindsStages(t *testing.T) {
	t.Parallel()
	for _, stage := range []BookPreparationStage{
		BookPreparationStageEntry,
		BookPreparationStageAnalysis,
		BookPreparationStageFinalize,
	} {
		classification := ClassifyBookEvidence(AnalysisSignals{
			ClassicalPreparation: &ClassicalPreparationStatus{Stage: stage, CurrentRevision: 1},
		}, BookDispositionToRead, "grc")
		assert.Equal(t, PreparationNone, classification.Preparation.State, stage)
	}
}

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
			name:        "imported current content admits a reading",
			signals:     AnalysisSignals{Content: ContentCurrentImported, Published: PublishedCurrent, LatestRun: RunCompleted},
			disposition: BookDispositionToRead, language: "grc",
			evidence: BookAnalyzed, phase: PhaseAnalyzed, eligibility: CurrentReadingEligible, recovery: RecoveryNone,
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
	contents := []ContentSignal{ContentNotAcquired, ContentNoCurrentRevision, ContentNonEPUB, ContentCurrentEPUB, ContentCurrentImported}
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
							ready := content.IsCurrentContent() && analyzed && disposition == BookDispositionToRead && language != ""
							assert.Equal(t, ready, got.Eligibility == CurrentReadingEligible, "eligibility for %+v %v %q", signals, disposition, language)

							switch content {
							case ContentNotAcquired:
								assert.Equal(t, BookNotAcquired, got.Evidence, "%+v", signals)
								assert.Equal(t, RecoveryNone, got.Recovery, "%+v", signals)
							case ContentNoCurrentRevision:
								assert.Equal(t, BookUnavailable, got.Evidence, "%+v", signals)
							case ContentNonEPUB, ContentCurrentEPUB, ContentCurrentImported:
							}

							shadowedRetry := published == PublishedCurrent && !current && (run == RunFailed || run == RunJobFailed || run == RunCancelled)
							retryAnalysis := got.Phase == PhaseFailed || got.Phase == PhaseCancelled || got.Phase == PhaseStale || shadowedRetry
							if content != ContentNotAcquired {
								assert.Equal(t, retryAnalysis, got.Recovery == RecoveryRetryAnalysis, "recovery for %+v", signals)
							}
							hasRevision := content != ContentNotAcquired && content != ContentNoCurrentRevision
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

func TestClassifyBookEvidenceClassicalPreparationStages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		status      ClassicalPreparationStatus
		preparation PreparationState
		evidence    BookEvidenceState
		phase       AnalysisPhase
		recovery    BookEvidenceRecovery
	}{
		{
			name:        "queued",
			status:      ClassicalPreparationStatus{State: ClassicalPreparationQueued, CurrentRevision: 1},
			preparation: PreparationQueued, evidence: BookAcquiredUnassessed, phase: PhaseAnalyzing, recovery: RecoveryNone,
		},
		{
			name:        "running",
			status:      ClassicalPreparationStatus{State: ClassicalPreparationRunning, CurrentRevision: 1},
			preparation: PreparationRunning, evidence: BookAcquiredUnassessed, phase: PhaseAnalyzing, recovery: RecoveryNone,
		},
		{
			name:        "fetch",
			status:      ClassicalPreparationStatus{Stage: BookPreparationStageFetch, CurrentRevision: 1},
			preparation: PreparationFetchFailed, evidence: BookUnavailable, phase: PhaseNotAnalyzed, recovery: RecoveryRetryAcquisition,
		},
		{
			name:        "import",
			status:      ClassicalPreparationStatus{Stage: BookPreparationStageImport, CurrentRevision: 1},
			preparation: PreparationImportFailed, evidence: BookAcquiredUnassessed, phase: PhaseFailed, recovery: RecoveryRetryAnalysis,
		},
		{
			name:        "source",
			status:      ClassicalPreparationStatus{Stage: BookPreparationStageSource, CurrentRevision: 1},
			preparation: PreparationSourceRefused, evidence: BookAcquiredUnassessed, phase: PhaseFailed, recovery: RecoveryNone,
		},
		{
			name:        "withdrawn",
			status:      ClassicalPreparationStatus{Stage: BookPreparationStageWithdrawn, Withdrawn: true, CurrentRevision: 1},
			preparation: PreparationWithdrawn, evidence: BookUnavailable, phase: PhaseFailed, recovery: RecoveryNone,
		},
		{
			name:        "withdrawn failure without the tombstone flag",
			status:      ClassicalPreparationStatus{Stage: BookPreparationStageWithdrawn, CurrentRevision: 1},
			preparation: PreparationWithdrawn, evidence: BookUnavailable, phase: PhaseFailed, recovery: RecoveryNone,
		},
		{
			name:        "catalog",
			status:      ClassicalPreparationStatus{Stage: BookPreparationStageCatalog, CurrentRevision: 1},
			preparation: PreparationCatalogUnavailable, evidence: BookUnavailable, phase: PhaseNotAnalyzed, recovery: RecoveryNone,
		},
		{
			name:        "cleared",
			status:      ClassicalPreparationStatus{Cleared: true, FailureRevision: 1, CurrentRevision: 2},
			preparation: PreparationCleared, evidence: BookAcquiredUnassessed, phase: PhaseNotAnalyzed, recovery: RecoveryRetryPreparation,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyBookEvidence(AnalysisSignals{ClassicalPreparation: &tt.status}, BookDispositionToRead, "grc")
			assert.Equal(t, tt.preparation, got.Preparation.State, "preparation")
			assert.Equal(t, tt.evidence, got.Evidence, "evidence")
			assert.Equal(t, tt.phase, got.Phase, "phase")
			assert.Equal(t, tt.recovery, got.Recovery, "recovery")
			assert.False(t, got.AnalysisInEffect(), "a preparation state is not an analysis in effect")
		})
	}
}

func TestClassicalPreparationDoesNotChangeAnalysisEligibility(t *testing.T) {
	t.Parallel()
	signals := AnalysisSignals{
		Content:              ContentCurrentImported,
		Published:            PublishedNone,
		LatestRun:            RunNone,
		ClassicalPreparation: &ClassicalPreparationStatus{State: ClassicalPreparationQueued, CurrentRevision: 1},
	}
	withPreparation := ClassifyBookEvidence(signals, BookDispositionToRead, "grc")
	signals.ClassicalPreparation = nil
	withoutPreparation := ClassifyBookEvidence(signals, BookDispositionToRead, "grc")
	assert.Equal(t, CurrentReadingNoCompletedAnalysis, withPreparation.Eligibility)
	assert.Equal(t, withoutPreparation.Eligibility, withPreparation.Eligibility)
}

func TestClassicalPreparationNeverOverridesAnalysisInEffect(t *testing.T) {
	t.Parallel()
	statuses := []struct {
		name   string
		status ClassicalPreparationStatus
	}{
		{name: "fetch", status: ClassicalPreparationStatus{Stage: BookPreparationStageFetch, CurrentRevision: 1}},
		{name: "import", status: ClassicalPreparationStatus{Stage: BookPreparationStageImport, CurrentRevision: 1}},
		{name: "source", status: ClassicalPreparationStatus{Stage: BookPreparationStageSource, CurrentRevision: 1}},
		{name: "withdrawn", status: ClassicalPreparationStatus{Stage: BookPreparationStageWithdrawn, Withdrawn: true, CurrentRevision: 1}},
		{name: "catalog", status: ClassicalPreparationStatus{Stage: BookPreparationStageCatalog, CurrentRevision: 1}},
		{name: "cleared", status: ClassicalPreparationStatus{Cleared: true, FailureRevision: 1, CurrentRevision: 2}},
		{name: "running", status: ClassicalPreparationStatus{State: ClassicalPreparationRunning, CurrentRevision: 1}},
	}
	for _, tt := range statuses {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			signals := AnalysisSignals{
				Content:              ContentCurrentImported,
				Published:            PublishedCurrent,
				LatestRun:            RunCompleted,
				ClassicalPreparation: &tt.status,
			}
			got := ClassifyBookEvidence(signals, BookDispositionToRead, "grc")
			assert.Equal(t, PreparationNone, got.Preparation.State, "a published analysis is ordinary evidence")
			assert.Equal(t, BookAnalyzed, got.Evidence)
			assert.Equal(t, PhaseAnalyzed, got.Phase)
			assert.Equal(t, RecoveryNone, got.Recovery)
			assert.True(t, got.AnalysisInEffect())
		})
	}

	// A newer run shadowed by the published analysis keeps that analysis in
	// effect, so the preparation failure still does not apply.
	status := ClassicalPreparationStatus{Stage: BookPreparationStageFetch, CurrentRevision: 1}
	shadowed := ClassifyBookEvidence(AnalysisSignals{
		Content:              ContentCurrentImported,
		Published:            PublishedCurrent,
		LatestRun:            RunFailed,
		ClassicalPreparation: &status,
	}, BookDispositionToRead, "grc")
	assert.Equal(t, PreparationNone, shadowed.Preparation.State)
	assert.True(t, shadowed.AnalysisInEffect())
	assert.Equal(t, BookAnalyzed, shadowed.Evidence)
}

func TestMyBookClassificationCarriesClassicalPreparation(t *testing.T) {
	t.Parallel()
	book := MyBook{
		Book:                 Book{LanguageState: LanguageChosen, LanguageTag: "grc"},
		Disposition:          BookDispositionToRead,
		ReadingUnit:          &BookReadingUnit{ReadingUnitKey: "greek-1", Revision: 1},
		ClassicalPreparation: &ClassicalPreparationStatus{Stage: BookPreparationStageFetch, CurrentRevision: 1},
	}
	classification := book.Classification()
	assert.Equal(t, PreparationFetchFailed, classification.Preparation.State)
	assert.Equal(t, RecoveryRetryAcquisition, classification.Recovery)

	withdrawn := MyBook{
		Book:        Book{LanguageState: LanguageChosen, LanguageTag: "grc"},
		Disposition: BookDispositionToRead,
		ReadingUnit: &BookReadingUnit{ReadingUnitKey: "greek-1", Revision: 1, Withdrawn: true},
	}
	assert.Equal(t, PreparationWithdrawn, withdrawn.Classification().Preparation.State)

	analyzed := book
	analyzed.Acquired = &SourceMaterialSummary{
		Source:  SourceMaterial{MediaType: ImportedSourceMediaType},
		Signals: AnalysisSignals{Content: ContentCurrentImported, Published: PublishedCurrent, LatestRun: RunCompleted},
	}
	assert.Equal(t, PreparationNone, analyzed.Classification().Preparation.State, "an analysis in effect ignores the failure")
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
