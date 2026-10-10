package domain

// StartFacts are the loaded facts that decide whether a Book may become the
// owner's current reading in Language. Adapters load them under their own
// concurrency control and hold no Start rule themselves.
type StartFacts struct {
	// Language is the study language the reading would start in.
	Language string
	// AlreadyCurrent reports that a reading is already current in Language.
	AlreadyCurrent bool
	// Signals, Disposition, and BookLanguage feed the Analysis evidence
	// classifier. BookLanguage is empty unless the Book has a chosen language.
	Signals      AnalysisSignals
	Disposition  BookDisposition
	BookLanguage string
	// IdentityPublished reports that the Book has the published analysis
	// identity a snapshot is frozen from.
	IdentityPublished bool
	// UnresolvedLemmaReviewFlags reports high-risk flags on the Book's current
	// analysis run that still need a resolution.
	UnresolvedLemmaReviewFlags bool
}

// StartOutcome is the result class of a Start decision.
type StartOutcome string

const (
	StartAccepted                StartOutcome = "accepted"
	StartRejectedAlreadyCurrent  StartOutcome = "already-current"
	StartRejectedIneligible      StartOutcome = "ineligible"
	StartRejectedUnresolvedFlags StartOutcome = "unresolved-lemma-review-flags"
)

// StartDecision accepts or rejects a Start. Reason carries the classifier's
// eligibility reason when the Book is ineligible and is CurrentReadingEligible
// otherwise.
type StartDecision struct {
	Outcome StartOutcome
	Reason  CurrentReadingEligibilityReason
}

// Accepted reports whether the Start may proceed to freezing a snapshot.
func (d StartDecision) Accepted() bool {
	return d.Outcome == StartAccepted
}

// DecideStart applies the Start rules in a fixed order: a reading already
// current in the language, then eligibility from the Analysis evidence
// classifier, then unresolved lemma-review flags. Snapshot candidate selection
// is not part of the decision.
func DecideStart(facts StartFacts) StartDecision {
	if facts.AlreadyCurrent {
		return StartDecision{Outcome: StartRejectedAlreadyCurrent, Reason: CurrentReadingEligible}
	}
	classification := ClassifyBookEvidence(facts.Signals, facts.Disposition, facts.BookLanguage)
	if classification.Eligibility != CurrentReadingEligible {
		return StartDecision{Outcome: StartRejectedIneligible, Reason: classification.Eligibility}
	}
	if facts.BookLanguage != facts.Language {
		return StartDecision{Outcome: StartRejectedIneligible, Reason: CurrentReadingOtherLanguage}
	}
	if !facts.IdentityPublished {
		return StartDecision{Outcome: StartRejectedIneligible, Reason: CurrentReadingNoCompletedAnalysis}
	}
	if facts.UnresolvedLemmaReviewFlags {
		return StartDecision{Outcome: StartRejectedUnresolvedFlags, Reason: CurrentReadingEligible}
	}
	return StartDecision{Outcome: StartAccepted, Reason: CurrentReadingEligible}
}
