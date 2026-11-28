package webapp

import (
	"strings"

	"github.com/a-h/templ"
)

// currentReadingHeroStatus is the hero's status line: present only when the
// Current reading needs the learner's attention. A Book that is ready, updating
// or merely being prepared has no line.
type currentReadingHeroStatus struct {
	Label       string
	Description string
}

// currentReadingRecoveryHint says where recovery works. The Current reading's
// analysis is frozen, so the server refuses a retry until the reading ends
// (ADR 0107); the hero therefore names the way out instead of offering a form
// the server would reject.
const currentReadingRecoveryHint = "End this reading to retry it from To Read."

// currentReadingHeroStatusFor reports what the Current reading needs, the most
// basic first: a missing Book language, then a failed or stale analysis.
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
		return currentReadingHeroStatus{Label: view.Label + ".", Description: view.Description + " " + currentReadingRecoveryHint}, true
	}
	attention, ok := analysisAttentionFor(classification)
	if !ok {
		return currentReadingHeroStatus{}, false
	}
	switch attention {
	case analysisAttentionFailed:
		return currentReadingHeroStatus{Label: "Analysis failed.", Description: "The last analysis failed. " + currentReadingRecoveryHint}, true
	case analysisAttentionStale:
		return currentReadingHeroStatus{Label: "Analysis out of date.", Description: "The content no longer matches this analysis. " + currentReadingRecoveryHint}, true
	case analysisAttentionReAnalysisFailed:
		return currentReadingHeroStatus{Label: "Re-analysis failed.", Description: "The current analysis stays in effect. " + currentReadingRecoveryHint}, true
	}
	return currentReadingHeroStatus{}, false
}

// currentReadingHeroTitle is the Work title for a classical Book, which stays
// right when the manifest is corrected after registration, else the Book title.
func currentReadingHeroTitle(item readingBookView) string {
	if item.ReadingUnit != nil && strings.TrimSpace(item.ReadingUnit.WorkTitle) != "" {
		return item.ReadingUnit.WorkTitle
	}
	return canonicalBookTitle(item.Book)
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
