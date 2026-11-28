package webapp

import "github.com/justin-hayes/mouseion/internal/domain"

// Classical preparation learner labels reuse the existing Analysis evidence and
// Reading wording. They never carry a stable code, a source detail or Greek.
const (
	classicalAnalysisIncompleteLabel = "Analysis incomplete"
	classicalAnalysisNotStarted      = "Analysis not started"
	classicalAssessmentUnavailable   = "Assessment unavailable"
	classicalAnalysisFailedLabel     = "Analysis failed"
	classicalSourceMismatchLabel     = "Source doesn't match its reviewed definition"
	classicalWithdrawnLabel          = "No longer in the Classical catalog"

	classicalRetryAcquisitionLabel = "Retry acquisition"
	classicalRetryAnalysisLabel    = "Retry analysis"
	classicalRetryPreparationLabel = "Retry preparation"

	classicalWithdrawnDescription = "This Book's Reading unit is no longer in the Classical catalog. It cannot be prepared, and no retry is available."

	classicalWithdrawnRefusal      = classicalWithdrawnLabel + ". This Book cannot be prepared, and no retry is available."
	classicalSourceMismatchRefusal = classicalSourceMismatchLabel + ". No retry is available until a newer definition revision arrives."
)

// classicalPreparationView is the one presentation of a classical Book's
// preparation state. It is returned only while the Book has no published
// analysis in effect; a prepared Book is ordinary Analysis evidence.
type classicalPreparationView struct {
	Label       string
	Description string
	Recovery    string
	Actionable  bool
	InProgress  bool
}

// classicalPreparationFor returns the Book's preparation presentation, or
// ok=false for an ordinary Book, a prepared Book, or a Book that never
// attempted preparation.
func classicalPreparationFor(book domain.MyBook) (classicalPreparationView, bool) {
	return classicalPreparationForClassification(book.Classification())
}

// classicalPreparationForClassification returns the presentation for a
// classification's preparation state. The domain decides whether preparation
// applies; the web app only names it.
func classicalPreparationForClassification(classification domain.BookEvidenceClassification) (classicalPreparationView, bool) {
	if !classification.Preparation.Applies() {
		return classicalPreparationView{}, false
	}
	return classicalPreparationViewFor(classification), true
}

// classicalPreparationViewFor maps the classified preparation state onto
// learner words and the one recovery the classification returned.
func classicalPreparationViewFor(classification domain.BookEvidenceClassification) classicalPreparationView {
	recovery := classicalRecoveryLabel(classification.Recovery)
	view := classicalPreparationView{Recovery: recovery, Actionable: recovery != ""}
	switch classification.Preparation.State {
	case domain.PreparationWithdrawn:
		view.Label = classicalWithdrawnLabel
		view.Description = classicalWithdrawnDescription
	case domain.PreparationQueued, domain.PreparationRunning:
		view.Label = classicalAnalysisIncompleteLabel
		view.Description = "Preparation is queued or running. No action is needed; this Book appears in Reading when its analysis is ready."
		view.InProgress = true
	case domain.PreparationFetchFailed:
		view.Label = classicalAssessmentUnavailable
		view.Description = "The reviewed source could not be downloaded, so there is no assessment yet. Retry acquisition to prepare this Book."
	case domain.PreparationImportFailed:
		view.Label = classicalAnalysisFailedLabel
		view.Description = "The source was acquired, but its analysis could not be published. Retry analysis to prepare this Book."
	case domain.PreparationSourceRefused:
		view.Label = classicalSourceMismatchLabel
		view.Description = "The pinned source failed its reviewed checks, so preparation stopped. This Book stays To Read; a newer definition revision is needed before it can be prepared."
	case domain.PreparationCatalogUnavailable:
		view.Label = "Classical catalog unavailable"
		view.Description = "Preparation is unavailable while this installation converts its legacy Ancient Greek rows. Restart Mouseion to retry the conversion. Existing Books stay readable."
	case domain.PreparationCleared:
		view.Label = classicalAnalysisNotStarted
		view.Description = "A newer definition revision is available. Retry preparation to prepare this Book."
	case domain.PreparationNone:
	}
	return view
}

// classicalRecoveryLabel names the recovery action the domain classification
// chose. Empty means no recovery is offered.
func classicalRecoveryLabel(recovery domain.BookEvidenceRecovery) string {
	switch recovery {
	case domain.RecoveryRetryAcquisition:
		return classicalRetryAcquisitionLabel
	case domain.RecoveryRetryAnalysis:
		return classicalRetryAnalysisLabel
	case domain.RecoveryRetryPreparation:
		return classicalRetryPreparationLabel
	case domain.RecoveryNone:
	}
	return ""
}
