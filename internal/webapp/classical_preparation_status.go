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
	status := book.ClassicalPreparation
	if book.ReadingUnit == nil {
		return classicalPreparationView{}, false
	}
	if book.Classification().AnalysisInEffect() {
		return classicalPreparationView{}, false
	}
	if status == nil && book.ReadingUnit.Withdrawn {
		return classicalPreparationView{
			Label:       classicalWithdrawnLabel,
			Description: classicalWithdrawnDescription,
		}, true
	}
	if status == nil {
		return classicalPreparationView{}, false
	}
	return classicalPreparationViewFor(*status), true
}

// classicalPreparationTone is the status tone for a preparation state. A
// terminal refusal is an error; a recoverable failure or a cleared failure is a
// warning; in-progress preparation is informational.
func classicalPreparationTone(status domain.ClassicalPreparationStatus) StatusTone {
	if status.InProgress() || status.Cleared {
		return StatusWarning
	}
	switch status.Failure {
	case domain.ClassicalPreparationFailureFetch, domain.ClassicalPreparationFailureImport:
		return StatusWarning
	case domain.ClassicalPreparationFailureSource, domain.ClassicalPreparationFailureWithdrawn,
		domain.ClassicalPreparationFailureCatalog:
		return StatusDanger
	case domain.ClassicalPreparationFailureNone:
	}
	return StatusInfo
}

// classicalPreparationViewFor maps the durable status onto learner words and
// the one valid recovery.
func classicalPreparationViewFor(status domain.ClassicalPreparationStatus) classicalPreparationView {
	if status.Withdrawn {
		return classicalPreparationView{
			Label:       classicalWithdrawnLabel,
			Description: classicalWithdrawnDescription,
		}
	}
	if status.InProgress() {
		return classicalPreparationView{
			Label:       classicalAnalysisIncompleteLabel,
			Description: "Preparation is queued or running. No action is needed; this Book appears in Reading when its analysis is ready.",
			InProgress:  true,
		}
	}
	switch status.Failure {
	case domain.ClassicalPreparationFailureFetch:
		return classicalPreparationView{
			Label:       classicalAssessmentUnavailable,
			Description: "The reviewed source could not be downloaded, so there is no assessment yet. Retry acquisition to prepare this Book.",
			Recovery:    classicalRetryAcquisitionLabel,
			Actionable:  true,
		}
	case domain.ClassicalPreparationFailureImport:
		return classicalPreparationView{
			Label:       classicalAnalysisFailedLabel,
			Description: "The source was acquired, but its analysis could not be published. Retry analysis to prepare this Book.",
			Recovery:    classicalRetryAnalysisLabel,
			Actionable:  true,
		}
	case domain.ClassicalPreparationFailureSource:
		return classicalPreparationView{
			Label:       classicalSourceMismatchLabel,
			Description: "The pinned source failed its reviewed checks, so preparation stopped. This Book stays To Read; a newer definition revision is needed before it can be prepared.",
		}
	case domain.ClassicalPreparationFailureCatalog:
		return classicalPreparationView{
			Label:       "Classical catalog unavailable",
			Description: "Preparation is unavailable until this installation's legacy Ancient Greek rows are converted. No retry is available until the operator finishes the conversion.",
		}
	case domain.ClassicalPreparationFailureWithdrawn:
		return classicalPreparationView{
			Label:       classicalWithdrawnLabel,
			Description: classicalWithdrawnDescription,
		}
	case domain.ClassicalPreparationFailureNone:
	}
	if status.Cleared {
		return classicalPreparationView{
			Label:       classicalAnalysisNotStarted,
			Description: "A newer definition revision is available. Retry preparation to prepare this Book.",
			Recovery:    classicalRetryPreparationLabel,
			Actionable:  true,
		}
	}
	return classicalPreparationView{}
}
