package domain

import "errors"

// Sentinel errors name why a Book deck request was refused. Services return
// them under their locks; the webapp maps each one to a learner response.
var (
	ErrDeckPreparationStale             = errors.New("deck preparation: expected Current reading snapshot is stale")
	ErrDeckPreparationNotCurrentReading = errors.New("deck preparation: Book is not the current reading")
	ErrDeckPreparationNotRequired       = errors.New("deck preparation: no deck is required for an empty snapshot")
	ErrDeckPreparationInvalidTransition = errors.New("deck preparation: invalid transition for this state")
)

// DeckAction is the operation a Book deck request asks for.
type DeckAction uint8

const (
	// DeckActionSubmit creates the deck for the current snapshot, or retries the
	// live preparation of it.
	DeckActionSubmit DeckAction = iota
	// DeckActionReprepare rolls a ready deck forward to a new generation.
	DeckActionReprepare
	// DeckActionRerender rebuilds the presentation of a ready deck.
	DeckActionRerender
)

// CurrentAnalysis is the Book's current completed analysis identity. A Current
// reading must pin exactly this analysis to be admitted.
type CurrentAnalysis struct {
	SourceMaterialID, AnalysisRunID, ContentRevisionID, ContentSnapshotID, CorpusID string
}

// DeckAdmissionFacts are the loaded facts one admission decision reads.
type DeckAdmissionFacts struct {
	Action  DeckAction
	BookID  string
	Current CurrentReading
	// Analysis is the Book's current analysis. Zero when it has none.
	Analysis CurrentAnalysis
	// ExpectedSnapshotID is the snapshot the request names.
	ExpectedSnapshotID string
	// Preparation is the preparation the request names, or the live
	// preparation of the snapshot for a submission. Nil when none exists.
	Preparation *DeckPreparation
}

// DeckAdmission is the outcome of one admission decision.
type DeckAdmission uint8

const (
	DeckAdmit DeckAdmission = iota
	DeckAdmissionNotCurrentReading
	DeckAdmissionStale
	DeckAdmissionNotRequired
	DeckAdmissionInvalidTransition
)

// Admitted reports whether the request may proceed.
func (a DeckAdmission) Admitted() bool { return a == DeckAdmit }

// Err returns the sentinel that explains a refusal, or nil when admitted.
func (a DeckAdmission) Err() error {
	switch a {
	case DeckAdmit:
		return nil
	case DeckAdmissionNotCurrentReading:
		return ErrDeckPreparationNotCurrentReading
	case DeckAdmissionStale:
		return ErrDeckPreparationStale
	case DeckAdmissionNotRequired:
		return ErrDeckPreparationNotRequired
	case DeckAdmissionInvalidTransition:
		return ErrDeckPreparationInvalidTransition
	}
	return ErrDeckPreparationInvalidTransition
}

// DecideDeckAdmission is the single admission rule for Book decks. Outcomes
// are checked in this order, and the first that applies wins:
//
//  1. not the current reading: no active reading for the Book, a reading that
//     does not pin the Book's current analysis, or a preparation bound to a
//     different snapshot;
//  2. stale: the request does not name the exact current snapshot;
//  3. not required: the current snapshot holds no vocabulary;
//  4. invalid transition: the preparation's state does not allow the action.
func DecideDeckAdmission(f DeckAdmissionFacts) DeckAdmission {
	current, analysis := f.Current, f.Analysis
	if !current.IsActive() || current.BookID != f.BookID ||
		current.SourceMaterialID != analysis.SourceMaterialID || current.AnalysisRunID != analysis.AnalysisRunID ||
		current.ContentRevisionID != analysis.ContentRevisionID || current.ContentSnapshotID != analysis.ContentSnapshotID ||
		current.CorpusID != analysis.CorpusID {
		return DeckAdmissionNotCurrentReading
	}
	if f.Preparation != nil && f.Preparation.SnapshotID != current.SnapshotID {
		return DeckAdmissionNotCurrentReading
	}
	if f.ExpectedSnapshotID == "" || f.ExpectedSnapshotID != current.SnapshotID {
		return DeckAdmissionStale
	}
	if current.SnapshotSize == 0 {
		return DeckAdmissionNotRequired
	}
	if !preparationAllows(f.Action, f.Preparation) {
		return DeckAdmissionInvalidTransition
	}
	return DeckAdmit
}

// preparationAllows encodes which preparation states each action may start
// from. A submission without a preparation creates one; a retired preparation
// never accepts an action.
func preparationAllows(action DeckAction, p *DeckPreparation) bool {
	if p == nil {
		return action == DeckActionSubmit
	}
	if p.RetiredAt != nil {
		return false
	}
	switch action {
	case DeckActionSubmit:
		switch p.State {
		case DeckPreparationQueued, DeckPreparationPreparing, DeckPreparationFailed, DeckPreparationCancelled:
			return true
		case DeckPreparationReady:
			// Only an explicit "requires re-preparation" ready deck rolls forward
			// through submission; a healthy ready deck is not retried.
			return p.Error == DeckPreparationRequiresRepreparationError
		}
		return false
	case DeckActionReprepare:
		return p.State == DeckPreparationReady
	case DeckActionRerender:
		return p.State == DeckPreparationReady && p.CurrentRunID != ""
	}
	return false
}
