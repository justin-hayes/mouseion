package domain

// LemmaRepreparationFacts are the loaded facts a lemma review confirmation is
// decided over. The caller reads them under the same locks the confirmation
// commits under, so they describe the state the decision applies to.
type LemmaRepreparationFacts struct {
	// IdentityChanged reports that the proposal moves an effective vocabulary
	// identity for at least one selected occurrence.
	IdentityChanged bool
	// ReadyDeck reports that the Book has a ready deck preparation.
	ReadyDeck bool
	// SnapshotBound reports that the ready deck was prepared from a Reading
	// snapshot, so the change must restart the Book to re-prepare it.
	SnapshotBound bool
	// OtherCurrentReading reports that another Book is the owner's current reading.
	OtherCurrentReading bool
	// ToRead reports that the Book is To Read, so it can be restarted.
	ToRead bool
	// Confirmed reports that the learner explicitly accepted re-preparation.
	Confirmed bool
}

// LemmaRepreparationAction is what an identity change requires of the ready deck.
type LemmaRepreparationAction int

const (
	// LemmaRepreparationNone: no ready deck is affected, so the change needs no re-preparation.
	LemmaRepreparationNone LemmaRepreparationAction = iota
	// LemmaRepreparationUnconfirmed: the change affects a ready deck, and the learner has not consented.
	LemmaRepreparationUnconfirmed
	// LemmaRepreparationBlockedByReading: restarting would need another current reading to stop first.
	LemmaRepreparationBlockedByReading
	// LemmaRepreparationNotToRead: restarting needs the Book to be To Read.
	LemmaRepreparationNotToRead
	// LemmaRepreparationRestart: stop and restart the Book under a new snapshot, then prepare it.
	LemmaRepreparationRestart
	// LemmaRepreparationDirect: explicitly re-prepare the direct deck from current identities.
	LemmaRepreparationDirect
)

// DecideLemmaReprepare decides what an identity change does to the Book's ready
// deck. Preview and Confirm both use it: a preview reports that re-preparation
// is required, and a confirmation without consent is refused.
func DecideLemmaReprepare(facts LemmaRepreparationFacts) LemmaRepreparationAction {
	switch {
	case !facts.IdentityChanged || !facts.ReadyDeck:
		return LemmaRepreparationNone
	case !facts.Confirmed:
		return LemmaRepreparationUnconfirmed
	case !facts.SnapshotBound:
		return LemmaRepreparationDirect
	case facts.OtherCurrentReading:
		return LemmaRepreparationBlockedByReading
	case !facts.ToRead:
		return LemmaRepreparationNotToRead
	default:
		return LemmaRepreparationRestart
	}
}

// Err is the rejection a confirmation receives for an action it cannot perform,
// or nil when the action may proceed.
func (a LemmaRepreparationAction) Err() error {
	switch a {
	case LemmaRepreparationUnconfirmed:
		return ErrLemmaReviewRepreparationRequired
	case LemmaRepreparationBlockedByReading:
		return ErrLemmaReviewBlockedByReading
	case LemmaRepreparationNotToRead:
		return ErrLemmaReviewNotToRead
	case LemmaRepreparationNone, LemmaRepreparationDirect, LemmaRepreparationRestart:
		return nil
	}
	return nil
}
