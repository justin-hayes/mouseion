package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDecideEndCurrentReading(t *testing.T) {
	released := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	expected := CurrentReadingCommitment{BookID: "book", SnapshotID: "snap"}
	current := CurrentReading{BookID: "book", SnapshotID: "snap"}
	ended := &CurrentReadingSnapshotLifecycle{BookID: "book", ReleasedAt: released}

	tests := []struct {
		name  string
		facts EndCurrentReadingFacts
		want  CurrentReadingDecision
	}{
		{"exact match", EndCurrentReadingFacts{Expected: expected, Current: current}, CurrentReadingDecision{Verdict: CurrentReadingApply, ReleaseSnapshotID: "snap"}},
		{"incomplete commitment", EndCurrentReadingFacts{Expected: CurrentReadingCommitment{BookID: "book"}, Current: current}, CurrentReadingDecision{Verdict: CurrentReadingRejectStale}},
		{"stale book", EndCurrentReadingFacts{Expected: CurrentReadingCommitment{BookID: "other", SnapshotID: "snap"}, Current: current}, CurrentReadingDecision{Verdict: CurrentReadingRejectStale}},
		{"stale snapshot", EndCurrentReadingFacts{Expected: CurrentReadingCommitment{BookID: "book", SnapshotID: "old"}, Current: current, Snapshot: ended}, CurrentReadingDecision{Verdict: CurrentReadingRejectStale}},
		{"proven replay", EndCurrentReadingFacts{Expected: expected, Snapshot: ended}, CurrentReadingDecision{Verdict: CurrentReadingReplay}},
		{"unproven replay without lifecycle", EndCurrentReadingFacts{Expected: expected}, CurrentReadingDecision{Verdict: CurrentReadingRejectStale}},
		{"unproven replay for another book", EndCurrentReadingFacts{Expected: expected, Snapshot: &CurrentReadingSnapshotLifecycle{BookID: "other", ReleasedAt: released}}, CurrentReadingDecision{Verdict: CurrentReadingRejectStale}},
		{"unproven replay of unreleased snapshot", EndCurrentReadingFacts{Expected: expected, Snapshot: &CurrentReadingSnapshotLifecycle{BookID: "book"}}, CurrentReadingDecision{Verdict: CurrentReadingRejectStale}},
		{"completed not released", EndCurrentReadingFacts{Expected: expected, Snapshot: &CurrentReadingSnapshotLifecycle{BookID: "book", ReleasedAt: released, Completed: true}}, CurrentReadingDecision{Verdict: CurrentReadingRejectStale}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, DecideEndCurrentReading(test.facts))
		})
	}
}

func TestDecideSwitchCurrentReading(t *testing.T) {
	released := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	expected := CurrentReadingCommitment{BookID: "book", SnapshotID: "snap"}
	current := CurrentReading{BookID: "book", SnapshotID: "snap"}
	apply := CurrentReadingDecision{Verdict: CurrentReadingApply, ReleaseSnapshotID: "snap"}
	stale := CurrentReadingDecision{Verdict: CurrentReadingRejectStale}

	// A replay's current reading is the target's successor snapshot.
	successor := CurrentReading{BookID: "target", SnapshotID: "next"}
	former := &CurrentReadingSnapshotLifecycle{BookID: "book", ReleasedAt: released}
	created := &CurrentReadingSnapshotLifecycle{BookID: "target", CreatedAt: released}
	replay := SwitchCurrentReadingFacts{
		TargetBookID: "target", Expected: expected, Current: successor,
		ExpectedSnapshot: former, CurrentSnapshot: created, TargetEligibility: CurrentReadingEligible,
	}
	with := func(change func(*SwitchCurrentReadingFacts)) SwitchCurrentReadingFacts {
		facts := replay
		change(&facts)
		return facts
	}

	tests := []struct {
		name  string
		facts SwitchCurrentReadingFacts
		want  CurrentReadingDecision
	}{
		{"exact match", SwitchCurrentReadingFacts{TargetBookID: "target", Expected: expected, Current: current, TargetEligibility: CurrentReadingEligible}, apply},
		{"incomplete commitment", SwitchCurrentReadingFacts{TargetBookID: "target", Expected: CurrentReadingCommitment{SnapshotID: "snap"}, Current: current, TargetEligibility: CurrentReadingEligible}, stale},
		{"no current reading", SwitchCurrentReadingFacts{TargetBookID: "target", Expected: expected, TargetEligibility: CurrentReadingEligible}, CurrentReadingDecision{Verdict: CurrentReadingRejectNoCurrent}},
		{"stale book", SwitchCurrentReadingFacts{TargetBookID: "target", Expected: CurrentReadingCommitment{BookID: "other", SnapshotID: "snap"}, Current: current, TargetEligibility: CurrentReadingEligible}, stale},
		{"stale snapshot", SwitchCurrentReadingFacts{TargetBookID: "target", Expected: CurrentReadingCommitment{BookID: "book", SnapshotID: "old"}, Current: current, TargetEligibility: CurrentReadingEligible}, stale},
		{"proven replay", replay, CurrentReadingDecision{Verdict: CurrentReadingReplay}},
		{"replay lifecycle missing", with(func(f *SwitchCurrentReadingFacts) { f.ExpectedSnapshot = nil }), stale},
		{"replay successor lifecycle missing", with(func(f *SwitchCurrentReadingFacts) { f.CurrentSnapshot = nil }), stale},
		{"unproven replay of another target", with(func(f *SwitchCurrentReadingFacts) { f.TargetBookID = "third" }), stale},
		{"unproven replay after later start", with(func(f *SwitchCurrentReadingFacts) {
			f.CurrentSnapshot = &CurrentReadingSnapshotLifecycle{BookID: "target", CreatedAt: released.Add(time.Hour)}
		}), stale},
		{"unproven replay from another book", with(func(f *SwitchCurrentReadingFacts) {
			f.ExpectedSnapshot = &CurrentReadingSnapshotLifecycle{BookID: "other", ReleasedAt: released}
		}), stale},
		{"unproven replay of unreleased snapshot", with(func(f *SwitchCurrentReadingFacts) {
			f.ExpectedSnapshot = &CurrentReadingSnapshotLifecycle{BookID: "book"}
		}), stale},
		{"completed not released", with(func(f *SwitchCurrentReadingFacts) {
			f.ExpectedSnapshot = &CurrentReadingSnapshotLifecycle{BookID: "book", ReleasedAt: released, Completed: true}
		}), stale},
		{"replay without successor snapshot", with(func(f *SwitchCurrentReadingFacts) { f.Current.SnapshotID = "" }), stale},
		{"ineligible target", SwitchCurrentReadingFacts{TargetBookID: "target", Expected: expected, Current: current, TargetEligibility: CurrentReadingNotToRead}, CurrentReadingDecision{Verdict: CurrentReadingRejectIneligible, Ineligible: CurrentReadingNotToRead}},
		{"unresolved flags", SwitchCurrentReadingFacts{TargetBookID: "target", Expected: expected, Current: current, TargetEligibility: CurrentReadingEligible, UnresolvedFlags: true}, CurrentReadingDecision{Verdict: CurrentReadingRejectUnresolvedFlags}},
		{"ineligibility outranks flags", SwitchCurrentReadingFacts{TargetBookID: "target", Expected: expected, Current: current, TargetEligibility: CurrentReadingFailed, UnresolvedFlags: true}, CurrentReadingDecision{Verdict: CurrentReadingRejectIneligible, Ineligible: CurrentReadingFailed}},
		{"stale outranks ineligible", SwitchCurrentReadingFacts{TargetBookID: "target", Expected: CurrentReadingCommitment{BookID: "other", SnapshotID: "snap"}, Current: current, TargetEligibility: CurrentReadingNotToRead}, stale},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, DecideSwitchCurrentReading(test.facts))
		})
	}
}

func TestCurrentReadingEligibilityIn(t *testing.T) {
	eligible := BookEvidenceClassification{Eligibility: CurrentReadingEligible}
	assert.Equal(t, CurrentReadingEligible, CurrentReadingEligibilityIn(eligible, "de", "de"))
	assert.Equal(t, CurrentReadingOtherLanguage, CurrentReadingEligibilityIn(eligible, "it", "de"))
	assert.Equal(t, CurrentReadingFailed, CurrentReadingEligibilityIn(BookEvidenceClassification{Eligibility: CurrentReadingFailed}, "it", "de"))
}
