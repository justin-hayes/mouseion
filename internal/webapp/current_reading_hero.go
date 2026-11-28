package webapp

import (
	"github.com/a-h/templ"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// currentReadingHeroStatus is the hero's status line: present only when the
// Current reading needs the learner's action. A Book that is ready, updating or
// merely being prepared has no line.
type currentReadingHeroStatus struct {
	Label       string
	Description string
	// Evidence is true when the line comes from the Book's analysis, so the hero
	// keeps the analysis details and their recovery directly after it.
	Evidence bool
}

// currentReadingHeroStatusFor reports the action the Current reading needs, the
// most basic first: a missing Book language, then a failed or stale analysis.
func currentReadingHeroStatusFor(item readingBookView) (currentReadingHeroStatus, bool) {
	if item.LanguageMissing {
		return currentReadingHeroStatus{Label: "Choose a Book language."}, true
	}
	classification := item.classification()
	if classification.Preparation.Applies() {
		view := classicalPreparationViewFor(classification)
		if !view.Actionable {
			return currentReadingHeroStatus{}, false
		}
		return currentReadingHeroStatus{Label: view.Label + ".", Description: view.Description, Evidence: true}, true
	}
	switch readingEvidenceState(item) {
	case readingEvidenceFailed, readingEvidenceStale:
		return currentReadingHeroStatus{Label: readingEvidenceLabel(item) + ".", Description: readingEvidenceDescription(item), Evidence: true}, true
	case readingEvidenceCurrent, readingEvidenceIncomplete, readingEvidenceUnavailable, readingEvidencePublishing, readingEvidenceUpdating:
	}
	if classification.ReAnalysisShadowed() && classification.Run == domain.RunFailed {
		return currentReadingHeroStatus{Label: "Re-analysis failed.", Evidence: true}, true
	}
	return currentReadingHeroStatus{}, false
}

func currentReadingHeroTitleID() string { return "current-reading-title" }

// currentReadingSectionLabel names the section after the hero's heading, which
// names the Book; with no Current reading it keeps a plain label.
func currentReadingSectionLabel(currentReading *readingBookView) templ.Attributes {
	if currentReading == nil {
		return templ.Attributes{"aria-label": "Current reading"}
	}
	return templ.Attributes{"aria-labelledby": currentReadingHeroTitleID()}
}
