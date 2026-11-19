package readingintent

import (
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// OutcomeKind is the classified result of expressing reading intent for one
// Book. The web app maps each kind to its learner-facing sentence.
type OutcomeKind string

const (
	// OutcomeNotFound reports a Book the owner no longer has.
	OutcomeNotFound OutcomeKind = "not_found"
	// OutcomeStaleRevision reports a form rendered before another decision.
	OutcomeStaleRevision OutcomeKind = "stale_revision"
	// OutcomeUnchanged reports a Book that was already To Read, or a replayed form.
	OutcomeUnchanged OutcomeKind = "unchanged"
	// OutcomeMoved reports a disposition change with nothing left to submit.
	OutcomeMoved OutcomeKind = "moved"
	// OutcomePreparationUnavailable reports a classical move in a deployment
	// with no preparation seam: the Book moved, but no job was submitted.
	OutcomePreparationUnavailable OutcomeKind = "preparation_unavailable"
	// OutcomeAlreadyCurrent reports a classical Book whose analysis is in effect.
	OutcomeAlreadyCurrent OutcomeKind = "already_current"
	// OutcomeJobQueued reports a submitted job and its number.
	OutcomeJobQueued OutcomeKind = "job_queued"
	// OutcomeLegacyRows reports the installation-wide Classical catalog refusal.
	OutcomeLegacyRows OutcomeKind = "legacy_rows"
	// OutcomeWithdrawnDefinition reports a Reading unit no longer in the catalog.
	OutcomeWithdrawnDefinition OutcomeKind = "withdrawn_definition"
	// OutcomeSourceRefused reports a source refused for the current definition revision.
	OutcomeSourceRefused OutcomeKind = "source_refused"
	// OutcomeCouldNotStart reports a submission that failed without a stable cause.
	OutcomeCouldNotStart OutcomeKind = "could_not_start"
	// OutcomeAcquisitionFailed reports a source that could not be acquired.
	OutcomeAcquisitionFailed OutcomeKind = "acquisition_failed"
	// OutcomeNoUsableEPUBUnits reports a source with no units analysis can use.
	OutcomeNoUsableEPUBUnits OutcomeKind = "no_usable_epub_units"
)

// Route names the submission a moved Book took.
type Route string

const (
	// RouteClassical is the dedicated Classical preparation queue.
	RouteClassical Route = "classical"
	// RouteEPUB is acquisition followed by ordinary EPUB analysis.
	RouteEPUB Route = "epub"
)

// Outcome is what Move decided. Book is the Book the module loaded, so a caller
// can render its hidden scope without reading it again. The acquisition fields
// are set for EPUB failures: Target, Title and Err describe where acquisition
// looked, and AcquisitionFailed says whether acquisition itself was the failing
// step.
type Outcome struct {
	Kind              OutcomeKind
	Route             Route
	JobNumber         int64
	Book              domain.MyBook
	Target            cataloguesync.AcquisitionTarget
	Title             string
	Err               error
	AcquisitionFailed bool
}
