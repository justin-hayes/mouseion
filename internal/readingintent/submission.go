package readingintent

import (
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// SubmissionKind is what preparing an owner's Book produced.
type SubmissionKind int

const (
	// SubmissionUnspecified is the zero value. Classification reports it as a
	// submission that could not start.
	SubmissionUnspecified SubmissionKind = iota
	// SubmissionQueued reports a submitted job; JobNumber names it.
	SubmissionQueued
	// SubmissionAlreadyCurrent reports evidence that needs no work.
	SubmissionAlreadyCurrent
	// SubmissionRefused reports a stable refusal; Refusal names it.
	SubmissionRefused
	// SubmissionCouldNotStart reports a submission that failed without a
	// stable cause.
	SubmissionCouldNotStart
)

// RefusalCode is a stable reason preparation refused a Book.
type RefusalCode string

const (
	// RefusalPreparationUnavailable reports a deployment with no Classical
	// preparation seam. The Book still moves and starts no job.
	RefusalPreparationUnavailable RefusalCode = "preparation_unavailable"
	// RefusalLegacyRows reports the installation-wide Classical catalog
	// refusal.
	RefusalLegacyRows RefusalCode = "legacy_rows"
	// RefusalWithdrawnDefinition reports a Reading unit no longer in the
	// catalog.
	RefusalWithdrawnDefinition RefusalCode = "withdrawn_definition"
	// RefusalSourceRefused reports a source refused for the current
	// definition revision.
	RefusalSourceRefused RefusalCode = "source_refused"
	// RefusalNoUsableEPUBUnits reports a source with no units analysis can
	// use.
	RefusalNoUsableEPUBUnits RefusalCode = "no_usable_epub_units"
)

// Submission is the typed result of preparing an owner's Book. A queued
// submission names its job; a refused one names its stable code. Book is the
// Book preparation last loaded, so a caller can render its hidden scope.
// Target, Title, Err and AcquisitionFailed describe an EPUB acquisition that
// did not finish.
type Submission struct {
	Kind              SubmissionKind
	Route             Route
	JobNumber         int64
	Refusal           RefusalCode
	Book              domain.MyBook
	Target            cataloguesync.AcquisitionTarget
	Title             string
	Err               error
	AcquisitionFailed bool
}
