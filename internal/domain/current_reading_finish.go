package domain

// SnapshotIdentity is one study-language vocabulary identity: a canonical
// lemma with its analyzed part of speech.
type SnapshotIdentity struct {
	Language, CanonicalLemma, UPOS string
}

// CurrentReadingFinishFacts is everything Finish reads, loaded by an adapter
// while it holds the learner-state and current-reading locks. Known writes take
// the same learner-state lock, so Known cannot change during Finish.
//
// The snapshot identities are the current reading's Reserved vocabulary, so
// Reserved state never keeps one of them from being accepted and needs no
// separate fact.
type CurrentReadingFinishFacts struct {
	// Current is the language's current reading; inactive when none exists.
	Current CurrentReading
	// ExpectedBookID and ExpectedSnapshotID are the caller's commitment. A
	// legacy reading without a snapshot is named by an empty snapshot ID.
	ExpectedBookID, ExpectedSnapshotID string
	// Completion is the existing completion for the expected snapshot, if any.
	Completion *CurrentReadingCompletion
	// Snapshot holds the frozen identities of the expected snapshot.
	Snapshot []SnapshotIdentity
	// Known holds the owner's Known vocabulary in the snapshot's language. It
	// may be narrowed to the snapshot's lemmas. An empty UPOS is the lemma
	// wildcard.
	Known []KnownVocabulary
}

// CurrentReadingFinishOutcome is the kind of decision Finish reached.
type CurrentReadingFinishOutcome int

const (
	// CurrentReadingFinishRejected means the request must not change anything.
	CurrentReadingFinishRejected CurrentReadingFinishOutcome = iota
	// CurrentReadingFinishReplayed means the snapshot was already completed;
	// the existing completion is the result and nothing new is accepted.
	CurrentReadingFinishReplayed
	// CurrentReadingFinishPlanned means Finish proceeds as planned.
	CurrentReadingFinishPlanned
)

// CurrentReadingFinishRejection says why Finish was rejected.
type CurrentReadingFinishRejection int

const (
	// CurrentReadingFinishStale means the expectation no longer names the
	// current reading or its completion.
	CurrentReadingFinishStale CurrentReadingFinishRejection = iota + 1
	// CurrentReadingFinishUnknown means there is neither a current reading nor a
	// completion for the expectation.
	CurrentReadingFinishUnknown
)

// CurrentReadingFinishPlan is what a fresh Finish must write.
type CurrentReadingFinishPlan struct {
	// Accept lists the identities to add to Known vocabulary.
	Accept []SnapshotIdentity
	// SnapshotCount is the size of the frozen snapshot.
	SnapshotCount int
	// EligibleCount is the number of snapshot identities not yet Known.
	EligibleCount int
	// NewlyKnownCount is the number of identities Finish accepts as Known.
	NewlyKnownCount int
	// AlreadyKnownCount is the number of snapshot identities already Known.
	AlreadyKnownCount int
}

// CurrentReadingFinishDecision is the result of PlanCurrentReadingFinish.
type CurrentReadingFinishDecision struct {
	Outcome   CurrentReadingFinishOutcome
	Rejection CurrentReadingFinishRejection
	// Replay is the existing completion when Outcome is Replayed. When a
	// current reading still exists, the adapter still returns its Book to Inbox
	// and ends it.
	Replay CurrentReadingCompletion
	// Plan is set when Outcome is Planned.
	Plan CurrentReadingFinishPlan
}

// PlanCurrentReadingFinish decides Finish from facts alone. It rejects a stale
// expectation, replays an existing completion, or plans the Known acceptance
// and the counts of the completion fact. Adapters only apply the decision.
func PlanCurrentReadingFinish(facts CurrentReadingFinishFacts) CurrentReadingFinishDecision {
	reject := func(reason CurrentReadingFinishRejection) CurrentReadingFinishDecision {
		return CurrentReadingFinishDecision{Outcome: CurrentReadingFinishRejected, Rejection: reason}
	}
	if facts.ExpectedBookID == "" {
		return reject(CurrentReadingFinishStale)
	}
	if !facts.Current.IsActive() {
		if facts.Completion == nil {
			return reject(CurrentReadingFinishUnknown)
		}
	} else if facts.Current.BookID != facts.ExpectedBookID || facts.Current.SnapshotID != facts.ExpectedSnapshotID {
		return reject(CurrentReadingFinishStale)
	}
	if facts.Completion != nil {
		if facts.Completion.BookID != facts.ExpectedBookID {
			return reject(CurrentReadingFinishStale)
		}
		return CurrentReadingFinishDecision{Outcome: CurrentReadingFinishReplayed, Replay: *facts.Completion}
	}

	return CurrentReadingFinishDecision{Outcome: CurrentReadingFinishPlanned, Plan: PlanSnapshotAcceptance(facts.Snapshot, facts.Known)}
}

// PlanSnapshotAcceptance decides which frozen identities Finish accepts as
// Known and the counts that follow. It is also the read-only preview of a
// pending Finish.
func PlanSnapshotAcceptance(snapshot []SnapshotIdentity, known []KnownVocabulary) CurrentReadingFinishPlan {
	plan := CurrentReadingFinishPlan{SnapshotCount: len(snapshot)}
	for _, identity := range snapshot {
		if knownContains(known, identity) {
			continue
		}
		plan.Accept = append(plan.Accept, identity)
	}
	plan.EligibleCount = len(plan.Accept)
	plan.NewlyKnownCount = len(plan.Accept)
	plan.AlreadyKnownCount = plan.SnapshotCount - plan.EligibleCount
	return plan
}

func knownContains(known []KnownVocabulary, identity SnapshotIdentity) bool {
	for _, item := range known {
		if item.Language == identity.Language && item.CanonicalLemma == identity.CanonicalLemma && (item.UPOS == identity.UPOS || item.UPOS == "") {
			return true
		}
	}
	return false
}
