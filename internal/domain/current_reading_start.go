package domain

// StartFacts are the loaded facts that decide whether a Book may become the
// owner's current reading in Language (Read next, ADR 0136 §4). Adapters load
// them under their own concurrency control and hold no rule themselves.
type StartFacts struct {
	// Language is the study language the reading would start in.
	Language string
	// CurrentIsBook reports that the current reading in Language already names
	// the requested Book.
	CurrentIsBook bool
	// Signals, Disposition, and BookLanguage feed the Analysis evidence
	// classifier. BookLanguage is empty unless the Book has a chosen language.
	Signals      AnalysisSignals
	Disposition  BookDisposition
	BookLanguage string
	// IdentityPublished reports that the Book has a published analysis
	// identity.
	IdentityPublished bool
}

// StartOutcome is the result class of a Read next decision.
type StartOutcome string

const (
	// StartAccepted means the Book becomes the current reading, replacing any
	// other current reading in the language.
	StartAccepted StartOutcome = "accepted"
	// StartReplayed means the requested Book is already the current reading,
	// so Read next changes nothing and succeeds.
	StartReplayed           StartOutcome = "replayed"
	StartRejectedIneligible StartOutcome = "ineligible"
)

// StartDecision accepts, replays, or rejects Read next. Reason carries the
// classifier's eligibility reason when the Book is ineligible and is
// CurrentReadingEligible otherwise.
type StartDecision struct {
	Outcome StartOutcome
	Reason  CurrentReadingEligibilityReason
}

// DecideStart applies the Read next rules: the Book that is already current is
// a replay; otherwise the Book must be To Read, analysed, and have a Book
// language matching the study language. Lemma review flags never gate it.
func DecideStart(facts StartFacts) StartDecision {
	if facts.CurrentIsBook {
		return StartDecision{Outcome: StartReplayed, Reason: CurrentReadingEligible}
	}
	classification := ClassifyBookEvidence(facts.Signals, facts.Disposition, facts.BookLanguage)
	if reason := CurrentReadingEligibilityIn(classification, facts.BookLanguage, facts.Language); reason != CurrentReadingEligible {
		return StartDecision{Outcome: StartRejectedIneligible, Reason: reason}
	}
	if !facts.IdentityPublished {
		return StartDecision{Outcome: StartRejectedIneligible, Reason: CurrentReadingNoCompletedAnalysis}
	}
	return StartDecision{Outcome: StartAccepted, Reason: CurrentReadingEligible}
}
