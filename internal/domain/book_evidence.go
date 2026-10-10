package domain

// ContentSignal reports whether a Book has the acquired content that analysis
// and Reading need. Persistence derives it from the source row.
type ContentSignal string

const (
	ContentNotAcquired       ContentSignal = "not-acquired"
	ContentNoCurrentRevision ContentSignal = "no-current-revision"
	ContentNonEPUB           ContentSignal = "non-epub"
	ContentCurrentEPUB       ContentSignal = "current-epub"
)

// PublishedAnalysisSignal reports the analysis published for the Book's current
// content, taken from the current-analysis identity.
type PublishedAnalysisSignal string

const (
	PublishedNone    PublishedAnalysisSignal = "none"
	PublishedCurrent PublishedAnalysisSignal = "current"
	PublishedStale   PublishedAnalysisSignal = "stale"
)

// LatestRunSignal is the state of the Book's most recent analysis attempt.
// Queued and running are kept apart because the lifecycle copy names them
// differently. The two failure forms are kept apart because only an analysis
// run's own failure is presented as a failed analysis; a job that failed before
// a run was recorded and a run that completed but did not publish are not.
type LatestRunSignal string

const (
	RunNone               LatestRunSignal = "none"
	RunQueued             LatestRunSignal = "queued"
	RunRunning            LatestRunSignal = "running"
	RunPublicationPending LatestRunSignal = "publication-pending"
	RunPublicationFailed  LatestRunSignal = "publication-failed"
	RunFailed             LatestRunSignal = "failed"
	RunJobFailed          LatestRunSignal = "job-failed"
	RunCancelled          LatestRunSignal = "cancelled"
	RunCompleted          LatestRunSignal = "completed"
)

// AnalysisSignals is the typed input to ClassifyBookEvidence. The zero value
// describes a Book with no acquired content and no analysis.
type AnalysisSignals struct {
	Content   ContentSignal
	Published PublishedAnalysisSignal
	LatestRun LatestRunSignal
}

// AnalysisPhase is the Book's analysis status as My Books and Reading present
// it. Phase precedence is part of the classification: an in-progress run hides
// a stale or unpublished result, and a failed run hides a stale one. A current
// published analysis is not hidden by a newer queued, running, failed, or
// cancelled run; that run is secondary information.
type AnalysisPhase string

const (
	PhaseNotAnalyzed AnalysisPhase = "not-analyzed"
	PhaseAnalyzing   AnalysisPhase = "analyzing"
	PhaseFailed      AnalysisPhase = "failed"
	PhaseCancelled   AnalysisPhase = "cancelled"
	PhaseStale       AnalysisPhase = "stale"
	PhaseAnalyzed    AnalysisPhase = "analyzed"
)

// BookEvidenceRecovery names the one recovery action a Book offers.
type BookEvidenceRecovery string

const (
	RecoveryNone             BookEvidenceRecovery = "none"
	RecoveryRetryAcquisition BookEvidenceRecovery = "retry-acquisition"
	RecoveryRetryAnalysis    BookEvidenceRecovery = "retry-analysis"
)

// BookEvidenceClassification is the domain's reading of a Book's signals,
// disposition, and language. Presentation reads these results rather than
// re-deriving them from signals.
type BookEvidenceClassification struct {
	Content     ContentSignal
	Evidence    BookEvidenceState
	Phase       AnalysisPhase
	Run         LatestRunSignal
	Eligibility CurrentReadingEligibilityReason
	Recovery    BookEvidenceRecovery
}

// PublicationPending reports a completed analysis run whose result is not yet
// published and whose Book is not stale or failed.
func (c BookEvidenceClassification) PublicationPending() bool {
	return (c.Run == RunPublicationPending || c.Run == RunPublicationFailed) &&
		(c.Phase == PhaseAnalyzing || c.Phase == PhaseNotAnalyzed)
}

// CompletedAnalysis reports a published analysis of the Book's current content
// whose latest run completed. Coverage statistics are a separate concern and
// are not part of the classification.
func (c BookEvidenceClassification) CompletedAnalysis() bool {
	return c.Evidence == BookAnalyzed && c.Run == RunCompleted
}

// RunFinished reports that the latest run reached its completed state, whether
// or not its result has been published.
func (c BookEvidenceClassification) RunFinished() bool {
	return c.Run == RunCompleted || c.Run == RunPublicationPending || c.Run == RunPublicationFailed
}

// ReAnalysisShadowed reports a newer analysis run that is queued, running,
// failed, or cancelled while the published analysis still matches the current
// content. The published analysis stays in effect; the run is secondary.
func (c BookEvidenceClassification) ReAnalysisShadowed() bool {
	return c.Phase == PhaseAnalyzed && newerAttempt(c.Run)
}

// AnalysisInEffect reports that the published analysis is what Reading and the
// chooser use: its latest run completed, or a newer attempt is shadowed by it.
func (c BookEvidenceClassification) AnalysisInEffect() bool {
	return c.Phase == PhaseAnalyzed && (c.Run == RunCompleted || newerAttempt(c.Run))
}

// newerAttempt reports the runs that a published analysis outlives while its
// content still matches. Publication states are not included: a run awaiting
// publication keeps the Book out of the Current reading until it resolves.
func newerAttempt(run LatestRunSignal) bool {
	switch run {
	case RunQueued, RunRunning, RunFailed, RunJobFailed, RunCancelled:
		return true
	case RunNone, RunPublicationPending, RunPublicationFailed, RunCompleted:
	}
	return false
}

// ClassifyBookEvidence is the single domain entry point for Analysis evidence,
// Current reading eligibility, and recovery. Disposition and language affect
// only eligibility; a zero disposition or an empty language is not To Read or
// chosen.
func ClassifyBookEvidence(signals AnalysisSignals, disposition BookDisposition, language string) BookEvidenceClassification {
	content := signals.Content
	if content == "" {
		content = ContentNotAcquired
	}
	phase := analysisPhase(signals)
	return BookEvidenceClassification{
		Content:     content,
		Evidence:    evidenceState(content, phase),
		Phase:       phase,
		Run:         runOrNone(signals.LatestRun),
		Eligibility: eligibilityReason(content, signals, phase, disposition, language),
		Recovery:    recoveryAction(content, signals, phase),
	}
}

func analysisPhase(signals AnalysisSignals) AnalysisPhase {
	if signals.Published == PublishedCurrent && newerAttempt(signals.LatestRun) {
		// The published analysis matches the current content, so a newer run
		// that is still queued or running, or that failed or was cancelled, is
		// secondary information rather than the Book's analysis phase.
		return PhaseAnalyzed
	}
	switch signals.LatestRun {
	case RunQueued, RunRunning:
		return PhaseAnalyzing
	case RunFailed:
		return PhaseFailed
	case RunCancelled:
		return PhaseCancelled
	case RunNone, RunPublicationPending, RunPublicationFailed, RunJobFailed, RunCompleted:
	}
	switch signals.Published {
	case PublishedStale:
		return PhaseStale
	case PublishedCurrent:
		return PhaseAnalyzed
	case PublishedNone:
	}
	if signals.LatestRun == RunPublicationPending {
		return PhaseAnalyzing
	}
	return PhaseNotAnalyzed
}

func evidenceState(content ContentSignal, phase AnalysisPhase) BookEvidenceState {
	switch content {
	case ContentNotAcquired:
		return BookNotAcquired
	case ContentNoCurrentRevision:
		return BookUnavailable
	case ContentNonEPUB, ContentCurrentEPUB:
	}
	switch phase {
	case PhaseStale:
		return BookStale
	case PhaseAnalyzed:
		return BookAnalyzed
	case PhaseNotAnalyzed, PhaseAnalyzing, PhaseFailed, PhaseCancelled:
		return BookAcquiredUnassessed
	}
	return BookAcquiredUnassessed
}

func eligibilityReason(content ContentSignal, signals AnalysisSignals, phase AnalysisPhase, disposition BookDisposition, language string) CurrentReadingEligibilityReason {
	switch {
	case disposition != BookDispositionToRead:
		return CurrentReadingNotToRead
	case language == "":
		return CurrentReadingNoChosenLanguage
	case content != ContentCurrentEPUB:
		return CurrentReadingNeedsCurrentContent
	case phase == PhaseAnalyzed && (signals.LatestRun == RunCompleted || newerAttempt(signals.LatestRun)):
		return CurrentReadingEligible
	case phase == PhaseAnalyzing:
		return CurrentReadingAnalysisInProgress
	case signals.LatestRun == RunFailed, signals.LatestRun == RunJobFailed:
		return CurrentReadingFailed
	case phase == PhaseCancelled:
		return CurrentReadingCancelled
	case phase == PhaseStale:
		return CurrentReadingStale
	default:
		return CurrentReadingNoCompletedAnalysis
	}
}

func recoveryAction(content ContentSignal, signals AnalysisSignals, phase AnalysisPhase) BookEvidenceRecovery {
	if content == ContentNotAcquired {
		return RecoveryNone
	}
	if signals.Published == PublishedCurrent && retryableRun(signals.LatestRun) {
		return RecoveryRetryAnalysis
	}
	switch phase {
	case PhaseFailed, PhaseCancelled, PhaseStale:
		return RecoveryRetryAnalysis
	case PhaseNotAnalyzed, PhaseAnalyzing, PhaseAnalyzed:
	}
	if content == ContentNoCurrentRevision {
		return RecoveryRetryAcquisition
	}
	return RecoveryNone
}

// retryableRun reports a newer run whose failure or cancellation offers a retry
// while the published analysis stays in effect.
func retryableRun(run LatestRunSignal) bool {
	return run == RunFailed || run == RunJobFailed || run == RunCancelled
}

func runOrNone(run LatestRunSignal) LatestRunSignal {
	if run == "" {
		return RunNone
	}
	return run
}
