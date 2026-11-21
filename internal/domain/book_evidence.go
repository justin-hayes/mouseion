package domain

// ContentSignal reports whether a Book has the acquired content that analysis
// and Reading need. Persistence derives it from the source row.
type ContentSignal string

const (
	ContentNotAcquired       ContentSignal = "not-acquired"
	ContentNoCurrentRevision ContentSignal = "no-current-revision"
	ContentNonEPUB           ContentSignal = "non-epub"
	ContentCurrentEPUB       ContentSignal = "current-epub"
	// ContentCurrentImported is a current imported classical source (a pinned
	// CoNLL-U annotation) rather than an EPUB. Like a current EPUB, it is the
	// acquired content analysis and Reading need, so it admits a Reading.
	ContentCurrentImported ContentSignal = "current-imported"
)

// IsCurrentContent reports whether the signal names acquired current content
// that analysis and Reading can use, whether it is an EPUB or an imported
// classical source.
func (c ContentSignal) IsCurrentContent() bool {
	return c == ContentCurrentEPUB || c == ContentCurrentImported
}

// PublishedAnalysisSignal reports the analysis published for the Book's current
// content, taken from the current-analysis identity.
type PublishedAnalysisSignal string

const (
	PublishedNone    PublishedAnalysisSignal = "none"
	PublishedCurrent PublishedAnalysisSignal = "current"
	PublishedStale   PublishedAnalysisSignal = "stale"
)

// LatestRunSignal is the state of the Book's most recent analysis run.
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
	// CurrentReading reports that the Book is the Current reading. Its frozen
	// evidence cannot be refreshed, so a newer run that failed or was cancelled
	// offers no retry while the published analysis stays in effect.
	CurrentReading bool
	// PipelineOutdated reports that the published analysis was produced under
	// an Analysis pipeline identity other than the current one. It never changes
	// the evidence state; it only feeds the Analysis update offer (ADR 0107).
	PipelineOutdated bool
	// ClassicalPreparation is the Book's durable preparation evidence. It
	// applies only while no analysis is in effect; classification ignores it
	// once a published analysis is what Reading uses.
	ClassicalPreparation *ClassicalPreparationStatus
	// EPUBAcquisition is the Book's durable EPUB acquisition failure. Like
	// ClassicalPreparation it applies only while no analysis is in effect.
	EPUBAcquisition *EPUBAcquisitionStatus
}

// PreparationState is the learner-facing state of a Book's classical
// preparation, derived from ClassicalPreparationStatus. It names the stage
// without carrying prose, a raw code, or source detail.
type PreparationState string

const (
	PreparationNone               PreparationState = ""
	PreparationQueued             PreparationState = "queued"
	PreparationRunning            PreparationState = "running"
	PreparationFetchFailed        PreparationState = "fetch-failed"
	PreparationImportFailed       PreparationState = "import-failed"
	PreparationSourceRefused      PreparationState = "source-refused"
	PreparationWithdrawn          PreparationState = "withdrawn"
	PreparationCatalogUnavailable PreparationState = "catalog-unavailable"
	PreparationCleared            PreparationState = "cleared"
)

// PreparationClassification is the Book's classical preparation state. It is
// empty when preparation does not apply, which includes every Book whose
// analysis is already in effect.
type PreparationClassification struct {
	State PreparationState
}

// Applies reports whether the classification presents a classical preparation
// state for the Book.
func (p PreparationClassification) Applies() bool {
	return p.State != PreparationNone
}

// InProgress reports preparation that is queued or running.
func (p PreparationClassification) InProgress() bool {
	return p.State == PreparationQueued || p.State == PreparationRunning
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
	RecoveryRetryPreparation BookEvidenceRecovery = "retry-preparation"
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
	// Preparation is the classical preparation state while no analysis is in
	// effect. Presentation reads it instead of the raw preparation failure.
	Preparation PreparationClassification
	// Acquisition is the EPUB acquisition state while no analysis is in
	// effect. Presentation reads it instead of the raw failure.
	Acquisition AcquisitionClassification
	// Update is the Analysis update offer, which is independent of Evidence.
	Update AnalysisUpdateOffer
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
	return c.Phase == PhaseAnalyzed && newerRun(c.Run)
}

// AnalysisInEffect reports that the published analysis is what Reading and the
// chooser use: its latest run completed, or a newer run is shadowed by it.
func (c BookEvidenceClassification) AnalysisInEffect() bool {
	return analysisInEffect(c.Phase, c.Run)
}

// analysisInEffect is the shared phase-and-run test for a published analysis
// that Reading uses.
func analysisInEffect(phase AnalysisPhase, run LatestRunSignal) bool {
	return phase == PhaseAnalyzed && (run == RunCompleted || newerRun(run))
}

// newerRun reports the runs that a published analysis outlives while its
// content still matches. Publication states are not included: a run awaiting
// publication keeps the Book out of the Current reading until it resolves.
func newerRun(run LatestRunSignal) bool {
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
	evidencePhase := analysisPhase(signals)
	phase := evidencePhase
	evidence := evidenceState(content, evidencePhase)
	recovery := recoveryAction(content, signals, evidencePhase)
	preparation := classifyPreparation(signals.ClassicalPreparation)
	acquisition := classifyEPUBAcquisition(signals.EPUBAcquisition)
	if preparation.classification.Applies() && !analysisInEffect(evidencePhase, runOrNone(signals.LatestRun)) {
		evidence = preparation.evidence
		phase = preparation.phase
		recovery = preparation.recovery
	} else if acquisition.classification.Applies() && !analysisInEffect(evidencePhase, runOrNone(signals.LatestRun)) {
		evidence = acquisition.evidence
		phase = acquisition.phase
		recovery = acquisition.recovery
	} else {
		// A failure never applies beside an analysis in effect, and an empty
		// status never presents a preparation or acquisition state.
		preparation = preparationEvidence{}
		acquisition = acquisitionEvidence{}
	}
	return BookEvidenceClassification{
		Content:     content,
		Evidence:    evidence,
		Phase:       phase,
		Run:         runOrNone(signals.LatestRun),
		Eligibility: eligibilityReason(content, signals, evidencePhase, disposition, language),
		Recovery:    recovery,
		Preparation: preparation.classification,
		Acquisition: acquisition.classification,
		Update:      analysisUpdateOffer(content, signals, disposition),
	}
}

// preparationEvidence is the classification fragment a preparation state
// contributes while no analysis is in effect.
type preparationEvidence struct {
	classification PreparationClassification
	evidence       BookEvidenceState
	phase          AnalysisPhase
	recovery       BookEvidenceRecovery
}

// classifyPreparation maps durable classical preparation evidence onto the
// evidence state, phase and one recovery action the learner sees. A nil or
// empty status classifies as no preparation.
func classifyPreparation(status *ClassicalPreparationStatus) preparationEvidence {
	if status == nil {
		return preparationEvidence{}
	}
	if status.Withdrawn {
		return preparationEvidence{PreparationClassification{State: PreparationWithdrawn}, BookUnavailable, PhaseFailed, RecoveryNone}
	}
	switch status.State {
	case ClassicalPreparationQueued:
		return preparationEvidence{PreparationClassification{State: PreparationQueued}, BookAcquiredUnassessed, PhaseAnalyzing, RecoveryNone}
	case ClassicalPreparationRunning:
		return preparationEvidence{PreparationClassification{State: PreparationRunning}, BookAcquiredUnassessed, PhaseAnalyzing, RecoveryNone}
	case "":
	}
	switch status.Failure {
	case ClassicalPreparationFailureFetch:
		return preparationEvidence{PreparationClassification{State: PreparationFetchFailed}, BookUnavailable, PhaseNotAnalyzed, RecoveryRetryAcquisition}
	case ClassicalPreparationFailureImport:
		return preparationEvidence{PreparationClassification{State: PreparationImportFailed}, BookAcquiredUnassessed, PhaseFailed, RecoveryRetryAnalysis}
	case ClassicalPreparationFailureSource:
		return preparationEvidence{PreparationClassification{State: PreparationSourceRefused}, BookAcquiredUnassessed, PhaseFailed, RecoveryNone}
	case ClassicalPreparationFailureWithdrawn:
		return preparationEvidence{PreparationClassification{State: PreparationWithdrawn}, BookUnavailable, PhaseFailed, RecoveryNone}
	case ClassicalPreparationFailureCatalog:
		return preparationEvidence{PreparationClassification{State: PreparationCatalogUnavailable}, BookUnavailable, PhaseNotAnalyzed, RecoveryNone}
	case ClassicalPreparationFailureNone:
	}
	if status.Cleared {
		return preparationEvidence{PreparationClassification{State: PreparationCleared}, BookAcquiredUnassessed, PhaseNotAnalyzed, RecoveryRetryPreparation}
	}
	return preparationEvidence{}
}

func analysisPhase(signals AnalysisSignals) AnalysisPhase {
	if signals.Published == PublishedCurrent && newerRun(signals.LatestRun) {
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
	case ContentNonEPUB, ContentCurrentEPUB, ContentCurrentImported:
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
	case !content.IsCurrentContent():
		return CurrentReadingNeedsCurrentContent
	case phase == PhaseAnalyzed && (signals.LatestRun == RunCompleted || newerRun(signals.LatestRun)):
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
		if signals.CurrentReading {
			return RecoveryNone
		}
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
