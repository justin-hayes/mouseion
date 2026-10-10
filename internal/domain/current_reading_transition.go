package domain

import "time"

// CurrentReadingCommitment names the exact reading a caller believes is
// current: the Book and the frozen snapshot it was shown.
type CurrentReadingCommitment struct {
	BookID     string
	SnapshotID string
}

// IsComplete reports whether both identities are present. An incomplete
// commitment can never match a current reading.
func (c CurrentReadingCommitment) IsComplete() bool {
	return c.BookID != "" && c.SnapshotID != ""
}

// matches reports whether the commitment names exactly this current reading.
func (c CurrentReadingCommitment) matches(current CurrentReading) bool {
	return current.BookID == c.BookID && current.SnapshotID == c.SnapshotID
}

// CurrentReadingSnapshotLifecycle holds the durable facts about one frozen
// snapshot that prove a lifecycle replay: which Book it belonged to, when it
// was frozen and released, and whether it was completed.
type CurrentReadingSnapshotLifecycle struct {
	BookID     string
	CreatedAt  time.Time
	ReleasedAt time.Time // zero while the snapshot is still held
	Completed  bool
}

// Released reports whether the snapshot's Reserved vocabulary was released.
func (l CurrentReadingSnapshotLifecycle) Released() bool {
	return !l.ReleasedAt.IsZero()
}

// CurrentReadingVerdict is the outcome of deciding an End or Switch.
type CurrentReadingVerdict int

const (
	// CurrentReadingApply means the transition is accepted and the plan must
	// be applied.
	CurrentReadingApply CurrentReadingVerdict = iota
	// CurrentReadingReplay means durable facts prove this exact transition
	// already ran, so nothing is applied and the caller sees success.
	CurrentReadingReplay
	// CurrentReadingRejectStale means the expected commitment no longer names
	// the current reading and no replay is proven.
	CurrentReadingRejectStale
	// CurrentReadingRejectNoCurrent means there is no current reading to
	// switch away from.
	CurrentReadingRejectNoCurrent
	// CurrentReadingRejectIneligible means the target Book cannot become the
	// current reading; CurrentReadingDecision.Ineligible says why.
	CurrentReadingRejectIneligible
	// CurrentReadingRejectUnresolvedFlags means the target Book has
	// unresolved high-risk lemma review flags.
	CurrentReadingRejectUnresolvedFlags
)

// CurrentReadingDecision is the plan an adapter applies, or the reason it
// must not. ReleaseSnapshotID names the snapshot whose Reserved vocabulary an
// accepted transition releases; it is set only for CurrentReadingApply.
type CurrentReadingDecision struct {
	Verdict           CurrentReadingVerdict
	Ineligible        CurrentReadingEligibilityReason
	ReleaseSnapshotID string
}

// EndCurrentReadingFacts are the loaded facts End decides over. Expected is
// the lifecycle of the expected snapshot, nil when none is recorded.
type EndCurrentReadingFacts struct {
	Expected CurrentReadingCommitment
	Current  CurrentReading
	Snapshot *CurrentReadingSnapshotLifecycle
}

// DecideEndCurrentReading accepts End only for the exact current commitment.
// A replay is proven only by a recorded release of the expected snapshot,
// without completion, while no reading is current; Book identity or
// disposition alone is never proof.
func DecideEndCurrentReading(f EndCurrentReadingFacts) CurrentReadingDecision {
	if !f.Expected.IsComplete() {
		return CurrentReadingDecision{Verdict: CurrentReadingRejectStale}
	}
	if f.Current.IsActive() {
		if !f.Expected.matches(f.Current) {
			return CurrentReadingDecision{Verdict: CurrentReadingRejectStale}
		}
		return CurrentReadingDecision{Verdict: CurrentReadingApply, ReleaseSnapshotID: f.Current.SnapshotID}
	}
	if f.Snapshot != nil && f.Snapshot.BookID == f.Expected.BookID && f.Snapshot.Released() && !f.Snapshot.Completed {
		return CurrentReadingDecision{Verdict: CurrentReadingReplay}
	}
	return CurrentReadingDecision{Verdict: CurrentReadingRejectStale}
}

// SwitchCurrentReadingFacts are the loaded facts Switch decides over.
// ExpectedSnapshot and CurrentSnapshot are the lifecycles of the expected and
// the present snapshots, nil when none is recorded. TargetEligibility is the
// Analysis evidence classification of the target Book for the study language
// (see CurrentReadingEligibilityIn); UnresolvedFlags is meaningful only when
// the target is eligible.
type SwitchCurrentReadingFacts struct {
	TargetBookID      string
	Expected          CurrentReadingCommitment
	Current           CurrentReading
	ExpectedSnapshot  *CurrentReadingSnapshotLifecycle
	CurrentSnapshot   *CurrentReadingSnapshotLifecycle
	TargetEligibility CurrentReadingEligibilityReason
	UnresolvedFlags   bool
}

// DecideSwitchCurrentReading accepts Switch only from the exact current
// commitment to an eligible target without unresolved flags. A replay is
// proven only when the current reading is the target's snapshot created by
// the same transition that released the expected, uncompleted snapshot.
// Switching to the exact current Book changes nothing, so it is a replay.
func DecideSwitchCurrentReading(f SwitchCurrentReadingFacts) CurrentReadingDecision {
	if !f.Expected.IsComplete() {
		return CurrentReadingDecision{Verdict: CurrentReadingRejectStale}
	}
	if !f.Current.IsActive() {
		return CurrentReadingDecision{Verdict: CurrentReadingRejectNoCurrent}
	}
	if !f.Expected.matches(f.Current) {
		if f.Current.BookID == f.TargetBookID && switchReplayProven(f) {
			return CurrentReadingDecision{Verdict: CurrentReadingReplay}
		}
		return CurrentReadingDecision{Verdict: CurrentReadingRejectStale}
	}
	if f.TargetBookID == f.Current.BookID {
		return CurrentReadingDecision{Verdict: CurrentReadingReplay}
	}
	if f.TargetEligibility != CurrentReadingEligible {
		return CurrentReadingDecision{Verdict: CurrentReadingRejectIneligible, Ineligible: f.TargetEligibility}
	}
	if f.UnresolvedFlags {
		return CurrentReadingDecision{Verdict: CurrentReadingRejectUnresolvedFlags}
	}
	return CurrentReadingDecision{Verdict: CurrentReadingApply, ReleaseSnapshotID: f.Current.SnapshotID}
}

// switchReplayProven reports whether the current reading is the successor an
// earlier Switch from the expected commitment created. Switch releases the old
// snapshot and freezes the new one in one transaction, so their timestamps
// coincide; an End followed by a later Start does not satisfy that, and
// neither does a completed or still-held expected snapshot.
func switchReplayProven(f SwitchCurrentReadingFacts) bool {
	if f.Current.SnapshotID == "" || f.ExpectedSnapshot == nil || f.CurrentSnapshot == nil {
		return false
	}
	former := f.ExpectedSnapshot
	if former.BookID != f.Expected.BookID || !former.Released() || former.Completed {
		return false
	}
	return f.CurrentSnapshot.CreatedAt.Equal(former.ReleasedAt)
}

// CurrentReadingEligibilityIn combines the Analysis evidence classification of
// a Book with the study language the reading would start in. bookLanguage is
// the Book's chosen language, empty when none is chosen.
func CurrentReadingEligibilityIn(classification BookEvidenceClassification, bookLanguage, language string) CurrentReadingEligibilityReason {
	if classification.Eligibility != CurrentReadingEligible {
		return classification.Eligibility
	}
	if bookLanguage != language {
		return CurrentReadingOtherLanguage
	}
	return CurrentReadingEligible
}
