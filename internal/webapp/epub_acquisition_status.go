package webapp

import "github.com/justin-hayes/mouseion/internal/domain"

// EPUB acquisition failure labels name the reason Reading shows. They never
// carry a stable code or a raw error.
const (
	epubAcquisitionInProgressLabel     = classicalAnalysisIncompleteLabel
	epubAcquisitionEntryMissingLabel   = "The catalog no longer offers this Book."
	epubAcquisitionDownloadFailedLabel = "The EPUB could not be downloaded."
	epubAcquisitionOversizeLabel       = "The EPUB is too large to store."
	epubAcquisitionFormatLabel         = "The catalog file is not a usable EPUB."
	epubAcquisitionImportFailedLabel   = "The EPUB could not be imported."

	epubAcquisitionRetryLabel = "Retry"

	// epubPreparationConfirmation is the one plain confirmation Add, Move to
	// To Read and Retry return while EPUB preparation runs.
	epubPreparationConfirmation = "Downloading and analyzing the text."
)

// epubAcquisitionView is the one presentation of a Book's latest failed EPUB
// acquisition. It applies only while no analysis is in effect.
type epubAcquisitionView struct {
	Label      string
	Recovery   string
	Actionable bool
	InProgress bool
}

// epubAcquisitionFor returns the Book's acquisition failure presentation, or
// ok=false for a Book with no applicable failure.
func epubAcquisitionFor(book domain.MyBook) (epubAcquisitionView, bool) {
	if book.Disposition != domain.BookDispositionToRead {
		// A failed acquisition only exists on a Book the learner chose to read;
		// a stale row never presents on a Book with another disposition.
		return epubAcquisitionView{}, false
	}
	classification := book.Classification()
	if !classification.Acquisition.Applies() {
		return epubAcquisitionView{}, false
	}
	return epubAcquisitionViewFor(classification), true
}

// epubAcquisitionViewFor maps the classified failure onto the learner reason
// and the one recovery the classification returned.
func epubAcquisitionViewFor(classification domain.BookEvidenceClassification) epubAcquisitionView {
	view := epubAcquisitionView{}
	if classification.Recovery == domain.RecoveryRetryAcquisition {
		view.Recovery = epubAcquisitionRetryLabel
		view.Actionable = true
	}
	switch classification.Acquisition.State {
	case domain.AcquisitionInProgress:
		view.Label = epubAcquisitionInProgressLabel
		view.InProgress = true
	case domain.AcquisitionEntryMissing:
		view.Label = epubAcquisitionEntryMissingLabel
	case domain.AcquisitionFetchFailed:
		view.Label = epubAcquisitionDownloadFailedLabel
	case domain.AcquisitionSourceOversize:
		view.Label = epubAcquisitionOversizeLabel
	case domain.AcquisitionSourceFormatUnknown, domain.AcquisitionSourceRefused:
		view.Label = epubAcquisitionFormatLabel
	case domain.AcquisitionImportFailed:
		view.Label = epubAcquisitionImportFailedLabel
	case domain.AcquisitionNone:
	}
	return view
}
